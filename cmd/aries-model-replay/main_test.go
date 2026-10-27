package main

import (
	"strings"
	"testing"
)

func TestCLIRejectsPublicListenerBeforeReadingCredentials(t *testing.T) {
	err := run([]string{"--mode", "record", "--output", "unused", "--listen", "0.0.0.0:8000"})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("public listener validation: %v", err)
	}
}

func TestCLIReplayForbidsUpstream(t *testing.T) {
	err := run([]string{"--mode", "replay", "--output", "unused", "--upstream", "https://example.invalid"})
	if err == nil || !strings.Contains(err.Error(), "forbids upstream") {
		t.Fatalf("replay upstream validation: %v", err)
	}
}

func TestCLIRequiresExplicitSessionConfiguration(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("missing session configuration accepted")
	}
}

func TestCLIRejectsUnsupportedDialectBeforeStartingListener(t *testing.T) {
	for _, dialect := range []string{"automatic", "openclaw-2026.7.1"} {
		err := run([]string{"--mode", "record", "--output", "unused", "--harness-version", "unsupported-version", "--tool-call-id-dialect", dialect})
		if err == nil || !strings.Contains(err.Error(), "dialect") {
			t.Fatalf("invalid dialect/version accepted: %v", err)
		}
	}
}
