package envfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func privateFile(t *testing.T, content []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLiteralAssignmentsNeverEvaluateOrChangeEnvironment(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	t.Setenv("ARIES_ENVFILE_TEST", "process-value")
	t.Setenv("ARIES_ENVFILE_ONLY_PROCESS", "process-only")
	want := map[string]string{
		"ARIES_ENVFILE_TEST": "file-value",
		"EMPTY":              "", "SINGLE_EMPTY": "", "DOUBLE_EMPTY": "",
		"SINGLE": " a # b ", "DOUBLE": " a # b ",
		"COMMAND":      "$(touch " + marker + ")",
		"BACKTICK":     "`touch " + marker + "`",
		"BARE_COMMAND": "$(id)", "BARE_BACKTICK": "`id`",
		"VARIABLES": "$ARIES_ENVFILE_TEST/${ARIES_ENVFILE_TEST}",
		"ESCAPES":   `literal\n\t\u0041`, "EQUALS": "a=b=c",
		"NESTED_QUOTES": `say "hello"`,
	}
	content := "# private whole-line comment\r\n\t # indented comment\r\n\r\n" +
		"ARIES_ENVFILE_TEST=file-value\nEMPTY=\nSINGLE_EMPTY=''\nDOUBLE_EMPTY=\"\"\n" +
		"SINGLE=' a # b '\nDOUBLE=\" a # b \"\n" +
		"COMMAND='" + want["COMMAND"] + "'\nBACKTICK=\"" + want["BACKTICK"] + "\"\n" +
		"BARE_COMMAND=$(id)\nBARE_BACKTICK=`id`\nVARIABLES=$ARIES_ENVFILE_TEST/${ARIES_ENVFILE_TEST}\n" +
		"ESCAPES='" + want["ESCAPES"] + "'\nEQUALS=a=b=c\nNESTED_QUOTES='say \"hello\"'"
	file, err := Load(privateFile(t, []byte(content), 0o600))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for name, expected := range want {
		value, exists := file.Lookup(name)
		if !exists || string(value) != expected {
			t.Errorf("literal value mismatch for %s", name)
		}
		clear(value)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("shell command was evaluated")
	}
	if os.Getenv("ARIES_ENVFILE_TEST") != "process-value" {
		t.Fatal("process environment changed")
	}
	if _, exists := file.Lookup("ARIES_ENVFILE_ONLY_PROCESS"); exists {
		t.Fatal("file lookup fell back to the process environment")
	}
}

func TestInvalidSyntaxReportsOnlyLineAndReason(t *testing.T) {
	for index, line := range []string{
		"TOKEN=private canary", "TOKEN=private-canary#tail",
		"TOKEN='private-canary' # comment", "TOKEN=\"private-canary",
		"TOKEN='private-canary\ncontinued'", `TOKEN="private-canary\"tail"`,
		"TOKEN=private-canary\x00", "TOKEN=private-canary\rinside",
		"1TOKEN=private-canary", "TO-KEN=private-canary", "export TOKEN=private-canary",
		"TOKEN =private-canary", "TOKEN= private-canary", "TOKEN=private-canary ",
		"private-canary", "TOKEN=private-canary\nTOKEN=again",
	} {
		t.Run(fmt.Sprintf("case_%d", index), func(t *testing.T) {
			file, err := Load(privateFile(t, []byte("# comment\n"+line+"\n"), 0o600))
			if err == nil || file != nil {
				t.Fatal("invalid assignment accepted")
			}
			if !strings.Contains(err.Error(), "line ") || strings.Contains(err.Error(), "private-canary") || strings.Contains(err.Error(), "continued") {
				t.Fatal("error lacks a line number or exposes file content")
			}
		})
	}
}

func TestDuplicateKeyIncludesOffendingLineWithoutValue(t *testing.T) {
	_, err := Load(privateFile(t, []byte("# header\nTOKEN=first-canary\nTOKEN=second-canary\n"), 0o600))
	if err == nil || !strings.Contains(err.Error(), "line 3: duplicate key") || strings.Contains(err.Error(), "canary") {
		t.Fatal("duplicate-key error is missing, misplaced or exposes a value")
	}
}

func TestFilePermissionsTypeAndSize(t *testing.T) {
	for _, mode := range []os.FileMode{0o400, 0o600} {
		file, err := Load(privateFile(t, []byte("TOKEN=value\n"), mode))
		if err != nil {
			t.Fatalf("owner-only mode rejected: %04o", mode)
		}
		file.Close()
	}
	for _, mode := range []os.FileMode{0o000, 0o200, 0o440, 0o404, 0o640, 0o620, 0o601} {
		if file, err := Load(privateFile(t, []byte("TOKEN=private-canary\n"), mode)); err == nil || file != nil {
			t.Fatalf("unsafe mode accepted: %04o", mode)
		}
	}
	target := privateFile(t, []byte("TOKEN=private-canary\n"), 0o600)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, t.TempDir(), filepath.Join(t.TempDir(), "missing")} {
		if file, err := Load(path); err == nil || file != nil {
			t.Fatal("symlink, special file, directory or missing explicit path accepted")
		}
	}
	for _, size := range []int{maxFileBytes, maxFileBytes + 1} {
		file, err := Load(privateFile(t, append([]byte("TOKEN="), bytes.Repeat([]byte{'x'}, size-len("TOKEN="))...), 0o600))
		if size == maxFileBytes {
			if err != nil {
				t.Fatal("maximum-size file rejected")
			}
			file.Close()
		} else if err == nil || file != nil {
			t.Fatal("oversize file accepted")
		}
	}
}

func TestLookupCopiesAndCloseClearsOwnedBytes(t *testing.T) {
	file, err := Load(privateFile(t, []byte("TOKEN=private-canary\nEMPTY=\n"), 0o600))
	if err != nil {
		t.Fatal(err)
	}
	first, _ := file.Lookup("TOKEN")
	first[0] = 'X'
	second, exists := file.Lookup("TOKEN")
	if !exists || string(second) != "private-canary" {
		t.Fatal("lookup returned shared storage")
	}
	if empty, exists := file.Lookup("EMPTY"); !exists || len(empty) != 0 {
		t.Fatal("empty value became absent")
	}
	backing := file.values["TOKEN"][:cap(file.values["TOKEN"])]
	file.Close()
	file.Close()
	if !bytes.Equal(backing, make([]byte, len(backing))) {
		t.Fatal("owned bytes were not cleared")
	}
	if _, exists := file.Lookup("TOKEN"); exists {
		t.Fatal("closed file still exposes values")
	}
	if string(second) != "private-canary" {
		t.Fatal("Close modified a caller-owned copy")
	}
	clear(first)
	clear(second)
}

func TestConcurrentLookupAndClose(t *testing.T) {
	file, err := Load(privateFile(t, []byte("TOKEN=private-canary\n"), 0o600))
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				value, exists := file.Lookup("TOKEN")
				if exists && string(value) != "private-canary" {
					t.Error("partial value observed during Close")
				}
				clear(value)
			}
		}()
	}
	file.Close()
	workers.Wait()
}

func TestEmptyPathAndNilFile(t *testing.T) {
	file, err := Load("")
	if err != nil || file == nil {
		t.Fatal("empty path must return an empty File")
	}
	if _, exists := file.Lookup("HOME"); exists {
		t.Fatal("empty file read environment")
	}
	file.Close()
	var absent *File
	absent.Close()
	if _, exists := absent.Lookup("HOME"); exists {
		t.Fatal("nil file returned a value")
	}
}
