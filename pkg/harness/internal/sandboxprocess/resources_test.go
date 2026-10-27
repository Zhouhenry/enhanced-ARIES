package sandboxprocess

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResourceScriptPreservesKernelUnitsAndMissingVersusEmpty(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-owned resource fixture requires root")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	root := t.TempDir()
	for name, raw := range map[string]string{"cpu.stat": "usage_usec 123456789012345\nuser_usec 2\nsystem_usec 3\n", "memory.current": "4096\n", "io.stat": ""} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		t.Helper()
		raw, err := exec.Command(python, "-I", "-c", ResourceScript, root).CombinedOutput()
		if err != nil {
			t.Fatalf("read: %v %s", err, raw)
		}
		var got map[string]any
		if json.Unmarshal(raw, &got) != nil {
			t.Fatal("invalid receipt")
		}
		return got
	}
	got := read()
	files := got["files"].(map[string]any)
	if files["memory.peak"].(map[string]any)["status"] != "unsupported" || files["io.stat"].(map[string]any)["status"] != "empty" || files["cpu.stat"].(map[string]any)["raw"] != "usage_usec 123456789012345\nuser_usec 2\nsystem_usec 3\n" {
		t.Fatal("kernel units or capabilities changed")
	}
	if err := os.Mkdir(filepath.Join(root, "memory.peak"), 0700); err != nil {
		t.Fatal(err)
	}
	files = read()["files"].(map[string]any)
	if files["memory.peak"].(map[string]any)["status"] != "failed" {
		t.Fatal("I/O failure mislabeled unsupported")
	}
	// Reading must not write counters, quotas or admission files.
	entries, _ := os.ReadDir(root)
	if len(entries) != 4 {
		t.Fatal("resource read modified group")
	}
}

func TestResourceReadDoesNotReplaceStopAndIsAvailableAfterStop(t *testing.T) {
	s := &sandboxStub{}
	g := &Group{sandbox: s, dir: "/run/aries-cloud-test", cgroup: "/sys/fs/cgroup/aries-test"}
	s.stopResult.Stdout = `{"cgroup_path":"/sys/fs/cgroup/aries-test","cgroup_inode":"1"}`
	if _, err := g.ReadResources(context.Background()); err != nil {
		t.Fatal(err)
	}
	if g.stopped {
		t.Fatal("read marked isolation confirmed")
	}
	if err := g.Stop(context.Background()); err == nil {
		t.Fatal("read receipt accepted as positive stop")
	}
	s.stopResult.Stdout = "ARIES_GROUP_EMPTY"
	if err := g.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.stopResult.Stdout = `{"cgroup_path":"/sys/fs/cgroup/aries-test","cgroup_inode":"1"}`
	if _, err := g.ReadResources(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, command := range []int{0, 3} {
		if s.calls[command].User != "root" || s.calls[command].Args[3] != g.cgroup {
			t.Fatal("read entered measured group or changed owner")
		}
	}
	s.stopResult.Stdout = `{"cgroup_path":"/sys/fs/cgroup/other","cgroup_inode":"1"}`
	if _, err := g.ReadResources(context.Background()); err == nil {
		t.Fatal("different group accepted")
	}
}
