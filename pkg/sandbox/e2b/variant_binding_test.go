package e2b

import (
	"context"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
)

func TestInstructionVariantRetainsOriginalTemplateAndObservedLogicalIdentity(t *testing.T) {
	creates := 0
	base := testManager(t, func(action string, body map[string]any) (int, any) {
		switch action {
		case "create":
			creates++
			return 200, createResponse(body)
		case "kill":
			return 200, map[string]bool{"absence_confirmed": true}
		default:
			t.Errorf("unexpected action %s", action)
			return 500, nil
		}
	})
	binding := testTaskTemplateBinding()
	options := base.options
	options.Template, options.TemplateBinding = binding.Template, binding
	options.LogicalTaskID, options.TemplateTaskID = "fix-git-variant-v1", "wrong-original-task"
	if _, err := New(options); err == nil || creates != 0 {
		t.Fatal("wrong original template task accepted")
	}
	options.TemplateTaskID = binding.TaskID
	m, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close(); _, _ = m.FinishObservation() })
	live, err := m.Start(context.Background(), core.SandboxRequest{RunID: "run", TaskID: "task", Environment: binding.RequestedEnvironment})
	if err != nil || creates != 1 {
		t.Fatalf("original binding failed under variant identity: %v", err)
	}
	if err := m.Stop(context.Background(), live); err != nil {
		t.Fatal(err)
	}
	if _, err := m.FinishObservation(); err != nil {
		t.Fatal(err)
	}
	records, _ := readOperationRecords(t, m)
	if len(records) == 0 {
		t.Fatal("missing variant operation evidence")
	}
	for _, record := range records {
		if record.LogicalTaskID != options.LogicalTaskID {
			t.Fatal("original template identity replaced the observed variant identity")
		}
	}
}
