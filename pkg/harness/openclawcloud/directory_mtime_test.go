package openclawcloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
)

type directoryMtimeSandbox struct {
	fakeSandbox
	directory string
	order     []string
}

func (s *directoryMtimeSandbox) Exec(ctx context.Context, c core.Command) (core.CommandResult, error) {
	if len(c.Args) > 3 && c.Args[2] == sandboxprocess.ControlScript && c.Args[3] == "prepare" {
		s.order = append(s.order, "group")
	}
	if len(c.Args) > 2 && c.Args[2] == directoryMtimeScript {
		s.order = append(s.order, "mtime")
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, c.Path, c.Args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		result := core.CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
		var exited *exec.ExitError
		if errors.As(err, &exited) {
			result.ExitCode = exited.ExitCode()
			return result, nil
		}
		return result, err
	}
	return s.fakeSandbox.Exec(ctx, c)
}

func (s *directoryMtimeSandbox) ExecStream(ctx context.Context, c core.Command, input io.Reader, stdout, stderr io.Writer) (core.CommandResult, error) {
	if c.User == "root" && c.Path == "/bin/sh" {
		s.order = append(s.order, "alias")
		if err := os.WriteFile(filepath.Join(s.directory, "bridge-alias-entry"), nil, 0o600); err != nil {
			return core.CommandResult{}, err
		}
	}
	return s.fakeSandbox.ExecStream(ctx, c, input, stdout, stderr)
}

func TestInitialDirectoryMtimeRunsAfterBridgeSetupAndPreservesOtherPaths(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	directory := t.TempDir()
	other := filepath.Join(directory, "untouched")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "task-file")
	if err := os.WriteFile(file, []byte("unchanged task"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Unix(1600000000, 123456789)
	for _, name := range []string{directory, other, file} {
		if err := os.Chtimes(name, old, old); err != nil {
			t.Fatal(err)
		}
	}
	requested := map[string]int64{directory: 1761350400000000000}
	want := requested[directory]
	output := t.TempDir()
	bridge, err := NewExternalBridge(ExternalBridgeOptions{RunID: "run", TaskID: "task", Workdir: "/workspace", OutputDir: output,
		ClientSourceFile: filepath.Join(t.TempDir(), "client"), InitialDirectoryMtimeUnixNS: requested})
	if err != nil {
		t.Fatal(err)
	}
	requested[directory] = 0 // The bridge must own its frozen configuration.
	sandbox := &directoryMtimeSandbox{directory: directory}
	t.Cleanup(func() { _ = bridge.Stop(context.Background()) })
	if _, err := bridge.Start(context.Background(), sandbox); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sandbox.order, []string{"group", "alias", "mtime"}) {
		t.Fatalf("preparation order = %#v", sandbox.order)
	}
	for _, name := range []string{directory, other, file} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		expected := old.UnixNano()
		if name == directory {
			expected = want
		}
		if info.ModTime().UnixNano() != expected {
			t.Fatalf("mtime %s = %d, want %d", name, info.ModTime().UnixNano(), expected)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "unchanged task" {
		t.Fatalf("task contents changed: %q %v", data, err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "initial-directory-mtimes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var evidence struct {
		Requested map[string]int64   `json:"requested_unix_ns"`
		Result    core.CommandResult `json:"command_result"`
	}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	var actual struct {
		Directories []struct {
			Path        string `json:"path"`
			Before      int64  `json:"before_mtime_unix_ns"`
			After       int64  `json:"after_mtime_unix_ns"`
			BeforeAtime int64  `json:"before_atime_unix_ns"`
			AfterAtime  int64  `json:"after_atime_unix_ns"`
		} `json:"directories"`
	}
	if err := json.Unmarshal([]byte(evidence.Result.Stdout), &actual); err != nil {
		t.Fatal(err)
	}
	if evidence.Requested[directory] != want || len(actual.Directories) != 1 || actual.Directories[0].Path != directory ||
		actual.Directories[0].Before == old.UnixNano() || actual.Directories[0].After != want || actual.Directories[0].BeforeAtime != actual.Directories[0].AfterAtime {
		t.Fatalf("metadata readback = %#v", actual)
	}
}

func TestInitialDirectoryMtimeRejectsSymlinkAndRetainsFailureEvidence(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	target, link := filepath.Join(root, "target"), filepath.Join(root, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	sandbox := &directoryMtimeSandbox{}
	output := t.TempDir()
	if err := prepareDirectoryMtimes(context.Background(), sandbox, map[string]int64{link: 0}, output); err == nil {
		t.Fatal("accepted symlink to an undeclared directory")
	}
	after, err := os.Stat(target)
	if err != nil || after.ModTime() != before.ModTime() {
		t.Fatal("changed symlink target")
	}
	raw, err := os.ReadFile(filepath.Join(output, "initial-directory-mtimes.json"))
	if err != nil || !bytes.Contains(raw, []byte(`"error"`)) {
		t.Fatalf("failure evidence absent: %s %v", raw, err)
	}
	if err := prepareDirectoryMtimes(context.Background(), sandbox, nil, output); err != nil || len(sandbox.order) != 1 {
		t.Fatal("empty binding unexpectedly executed preparation")
	}
}
