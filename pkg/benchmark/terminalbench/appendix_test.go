package terminalbench

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestInstructionAppendixKeepsOriginalTaskAndPrivateVerifier(t *testing.T) {
	root := writeFixture(t)
	catalog, err := New(Options{Root: root, TaskIDs: []string{fixGitID}, OutputDir: t.TempDir(), Revision: fixtureGitRevision(root)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.WithInstructionAppendix(fixGitID, "fix-git-variant-v1", "public appendix"); err == nil {
		t.Fatal("unloaded original task accepted")
	}
	original, err := catalog.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	instructionPath := filepath.Join(taskDirectory(root, fixGitID), "instruction.md")
	originalBytes, err := os.ReadFile(instructionPath)
	if err != nil {
		t.Fatal(err)
	}
	const variantID = "fix-git-variant-v1"
	const appendix = "Public observation instruction.\n"
	variant, err := catalog.WithInstructionAppendix(fixGitID, variantID, appendix)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := variant.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := original[0]
	want.ID, want.Instruction = variantID, original[0].Instruction+"\n\n"+appendix
	if len(selected) != 1 || !reflect.DeepEqual(selected[0], want) || !reflect.DeepEqual(variant.details[variantID], catalog.details[fixGitID]) {
		t.Fatal("appendix changed original environment, budgets, or private evaluation inputs")
	}
	occurrence, err := variant.ForTask(variantID, variantID+"-001")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := occurrence.(*Benchmark).details[variantID+"-001"]; !ok {
		t.Fatal("variant occurrence lost original private evaluator")
	}
	if _, err := variant.ForTask(fixGitID, fixGitID+"-001"); err == nil {
		t.Fatal("variant catalog admitted original task identity")
	}
	after, err := catalog.Tasks(context.Background())
	if err != nil || !reflect.DeepEqual(original, after) {
		t.Fatal("original catalog changed")
	}
	afterBytes, err := os.ReadFile(instructionPath)
	if err != nil || string(afterBytes) != string(originalBytes) {
		t.Fatal("original instruction file changed")
	}
	if err := VerifyRevision(context.Background(), root, catalog.revision); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ id, appendix string }{{fixGitID, appendix}, {"../variant", appendix}, {variantID, " \n"}} {
		if _, err := catalog.WithInstructionAppendix(fixGitID, test.id, test.appendix); err == nil {
			t.Fatalf("invalid variant accepted: %q", test.id)
		}
	}
}
