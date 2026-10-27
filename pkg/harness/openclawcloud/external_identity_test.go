package openclawcloud

import (
	"path/filepath"
	"testing"
)

func TestExternalIdentityOptionsRejectPartialOrForeignBinding(t *testing.T) {
	for _, pair := range [][2]string{{"run/task", ""}, {"", "session"}, {"foreign/task", "session"}, {"run/task", "session"}, {"", ""}} {
		options := ExternalBridgeOptions{RunID: "run", TaskID: "task", Workdir: "/work", OutputDir: t.TempDir(), ClientSourceFile: filepath.Join(t.TempDir(), "client"), IdentityScopeID: pair[0], IdentitySessionID: pair[1]}
		_, err := NewExternalBridge(options)
		valid := pair == [2]string{"run/task", "session"} || pair == [2]string{}
		if (err == nil) != valid {
			t.Fatalf("pair=%v err=%v", pair, err)
		}
	}
}
