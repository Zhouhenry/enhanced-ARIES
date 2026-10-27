package codingcase

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
)

type pptxSandbox struct{ *fakeSandbox }

type pptxCloudDownloads struct {
	*pptxSandbox
	downloader runner.LimitedDownloader
}

func (sandbox *pptxCloudDownloads) DownloadLimit(ctx context.Context, source, destination string, limit int64) error {
	return sandbox.downloader.DownloadLimit(ctx, source, destination, limit)
}

func (sandbox *pptxSandbox) Exec(ctx context.Context, command core.Command) (core.CommandResult, error) {
	if len(command.Args) == 3 && command.Args[1] == verifierDir+"/verify_pptx.py" {
		sandbox.commands = append(sandbox.commands, command)
		return core.CommandResult{ExitCode: sandbox.exitCode}, sandbox.execErr
	}
	return sandbox.fakeSandbox.Exec(ctx, command)
}

func pptxTask(t *testing.T) (*Benchmark, core.Task) {
	t.Helper()
	benchmark, err := New(Options{Workload: WorkloadPPTX, Template: "explicit-template", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := benchmark.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return benchmark, tasks[0]
}

func TestPPTXPreservesUserPromptAndContainsNoSolutionOrVerifier(t *testing.T) {
	benchmark, task := pptxTask(t)
	occurrence, err := benchmark.ForTask(task.ID, "pptx-occurrence")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := occurrence.Tasks(context.Background())
	if err != nil || len(tasks) != 1 || tasks[0].Instruction != "幻灯片 生成一份关于大模型发展的pptx材料，写5页。" || tasks[0].ID != "pptx-occurrence" {
		t.Fatalf("PPTX occurrence changed the user's prompt: %+v %v", tasks, err)
	}
	sandbox := newFakeSandbox()
	if err := occurrence.PrepareSandbox(context.Background(), tasks[0], sandbox); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sandbox.uploaded, map[string][]byte{Workdir + "/README.md": []byte(pptxInstruction)}) || sandbox.finalizeCalls != 0 {
		t.Fatal("PPTX preparation leaked a solution/verifier or reused Python fixture files")
	}
}

func TestPPTXCapturesEveryArtifactBeforeIndependentEvaluation(t *testing.T) {
	benchmark, task := pptxTask(t)
	agent := &pptxSandbox{newFakeSandbox()}
	manifest := pptxManifest{SchemaVersion: 1, Complete: true, Files: []pptxFile{}}
	// Synthetic bytes mirror all five paths/sizes from the failed real capture;
	// they do not reconstruct the lost decks or assert a real PPTX score.
	for index, fixture := range []struct {
		name string
		size int
	}{
		{"projects/llm-development/llm-development.pptx", 37534},
		{"projects/llm-development/output/content.pptx", 119845},
		{"projects/llm-development/output/final.pptx", 37534},
		{"projects/llm-development/大模型发展.pptx", 37534},
		{"大模型发展.pptx", 37534},
		{"empty.pptx", 0},
	} {
		name := fixture.name
		content := bytes.Repeat([]byte{byte(index)}, fixture.size)
		digest := sha256.Sum256(content)
		manifest.Files = append(manifest.Files, pptxFile{Path: name, SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(digest[:])})
		agent.files[Workdir+"/"+name] = content
	}
	capture, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	agent.files[verifierDir+"/pptx-capture.json"] = capture
	limits := make(map[string]int64)
	var limitsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		switch r.URL.Path {
		case "/v1/create":
			_ = json.NewEncoder(w).Encode(map[string]any{"sandbox": body["sandbox"], "state": "active"})
		case "/v1/download":
			limitsMu.Lock()
			defer limitsMu.Unlock()
			name := body["path"].(string)
			limits[name] = int64(body["max_bytes"].(float64))
			content, ok := agent.files[name]
			if !ok || int64(len(content)) > limits[name] {
				t.Errorf("missing or oversized fixture download %s", name)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"data_base64": base64.StdEncoding.EncodeToString(content)})
		default:
			t.Errorf("unexpected adapter request %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	manager, err := e2b.New(e2b.Options{Endpoint: server.URL, Token: strings.Repeat("t", 32), Template: "fixture", OutputDir: benchmark.options.OutputDir, ObservationLevel: performance.ObservationL0})
	if err != nil {
		t.Fatal(err)
	}
	live, err := manager.Start(context.Background(), core.SandboxRequest{RunID: "fixture", TaskID: task.ID, Environment: core.Environment{Workdir: Workdir}})
	if err != nil {
		t.Fatal(err)
	}
	verifier := &pptxSandbox{newFakeSandbox()}
	report := pptxReport{SchemaVersion: 1, Passed: true, Files: []pptxReportFile{}}
	for index, file := range manifest.Files {
		entry := pptxReportFile{pptxFile: file, Error: "BadZipFile: invalid intermediate deck"}
		if index == 1 {
			entry.Parseable, entry.Passed, entry.SlideCount, entry.Error = true, true, 5, ""
			if err := json.Unmarshal([]byte(`[{"part":"ppt/slides/slide1.xml","nonempty":true,"content_kinds":["text"]},{"part":"ppt/slides/slide2.xml","nonempty":true,"content_kinds":["text"]},{"part":"ppt/slides/slide3.xml","nonempty":true,"content_kinds":["image"]},{"part":"ppt/slides/slide4.xml","nonempty":true,"content_kinds":["chart"]},{"part":"ppt/slides/slide5.xml","nonempty":true,"content_kinds":["text"]}]`), &entry.Slides); err != nil {
				t.Fatal(err)
			}
		}
		report.Files = append(report.Files, entry)
	}
	reportBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	verifier.files[verifierDir+"/pptx-report.json"] = reportBytes
	factory := &freshFactory{sandbox: verifier}
	result, err := benchmark.Evaluate(context.Background(), task, &pptxCloudDownloads{agent, live.(runner.LimitedDownloader)}, factory)
	if err != nil || result.Status != core.StatusSucceeded || result.Score != 1 || result.VerifierStatus != core.StatusSucceeded {
		t.Fatalf("evaluation: %+v %v", result, err)
	}
	if factory.calls != 1 || factory.environment.AllowNetwork || result.Artifact == nil || result.Artifact.ObservedAt.After(factory.started) {
		t.Fatal("independent verifier started before artifact capture or was not fresh/network-disabled")
	}
	sealed, err := os.ReadFile(result.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(sealed)
	if result.Artifact.SHA256 != hex.EncodeToString(digest[:]) || json.Unmarshal(sealed, &manifest) != nil {
		t.Fatal("sealed manifest hash or shape changed")
	}
	limitsMu.Lock()
	defer limitsMu.Unlock()
	for _, file := range manifest.Files {
		if limits[Workdir+"/"+file.Path] != max(file.SizeBytes, 1) {
			t.Fatalf("download did not use captured size: %s limit=%d size=%d", file.Path, limits[Workdir+"/"+file.Path], file.SizeBytes)
		}
		original := agent.files[Workdir+"/"+file.Path]
		actual, readErr := os.ReadFile(file.LocalPath)
		info, statErr := os.Stat(file.LocalPath)
		if readErr != nil || statErr != nil || !reflect.DeepEqual(original, actual) || info.Mode().Perm() != 0o400 || !reflect.DeepEqual(verifier.uploaded[verifierDir+"/decks/"+file.Path], original) {
			t.Fatalf("artifact was lost, changed, or not sealed: %s", file.Path)
		}
	}
	if len(agent.uploaded) != 0 || len(verifier.uploaded) != len(manifest.Files)+2 || string(verifier.uploaded[verifierDir+"/verify_pptx.py"]) != pptxVerifierScript {
		t.Fatal("private verifier reached agent or unknown inputs reached verifier")
	}
	for _, mutate := range []func(*pptxReport){
		func(r *pptxReport) { r.Files = r.Files[1:] },
		func(r *pptxReport) { r.Files[0].SHA256 = r.Files[1].SHA256 },
		func(r *pptxReport) { r.Files[1].Slides[0].Nonempty = false },
	} {
		var altered pptxReport
		if err := json.Unmarshal(reportBytes, &altered); err != nil {
			t.Fatal(err)
		}
		mutate(&altered)
		data, err := json.Marshal(altered)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifyPPTXReport(data, manifest, 0); err == nil {
			t.Fatal("partial or contradictory verifier report was accepted")
		}
	}
}

func TestPPTXInventoryLimitsAndChangedBytesFailBeforeVerifier(t *testing.T) {
	digest := sha256.Sum256([]byte("original"))
	file := pptxFile{Path: "deck.pptx", SizeBytes: 8, SHA256: hex.EncodeToString(digest[:])}
	for _, content := range []string{"short", "modified"} {
		t.Run(content, func(t *testing.T) {
			benchmark, task := pptxTask(t)
			agent := &pptxSandbox{newFakeSandbox()}
			manifest := pptxManifest{SchemaVersion: 1, Complete: true, Files: []pptxFile{file}}
			agent.files[verifierDir+"/pptx-capture.json"], _ = json.Marshal(manifest)
			agent.files[Workdir+"/"+file.Path] = []byte(content)
			factory := &freshFactory{sandbox: &pptxSandbox{newFakeSandbox()}}
			result, err := benchmark.Evaluate(context.Background(), task, agent, factory)
			if err == nil || result.Artifact != nil || factory.calls != 0 {
				t.Fatal("changed size or hash reached independent verifier")
			}
		})
	}
	for _, tc := range []struct {
		count int
		size  int64
		valid bool
	}{
		{128, 0, true}, {129, 0, false},
		{8, 64 << 20, true}, {9, 64 << 20, false},
		{1, (64 << 20) + 1, false},
	} {
		t.Run(fmt.Sprintf("count%d_size%d", tc.count, tc.size), func(t *testing.T) {
			manifest := pptxManifest{SchemaVersion: 1, Complete: true, Files: []pptxFile{}}
			for index := 0; index < tc.count; index++ {
				entry := file
				entry.Path, entry.SizeBytes = fmt.Sprintf("%d.pptx", index), tc.size
				manifest.Files = append(manifest.Files, entry)
			}
			if err := validatePPTXManifest(manifest); (err == nil) != tc.valid {
				t.Fatalf("inventory validity: %v", err)
			}
		})
	}
}

func TestPPTXNoFilesIsIndependentFailureAndInvalidInventoryCannotPass(t *testing.T) {
	benchmark, task := pptxTask(t)
	agent := &pptxSandbox{newFakeSandbox()}
	agent.files[verifierDir+"/pptx-capture.json"] = []byte(`{"schema_version":1,"complete":true,"files":[],"error":""}`)
	verifier := &pptxSandbox{newFakeSandbox()}
	verifier.exitCode = 1
	verifier.files[verifierDir+"/pptx-report.json"] = []byte(`{"schema_version":1,"passed":false,"files":[]}`)
	factory := &freshFactory{sandbox: verifier}
	result, err := benchmark.Evaluate(context.Background(), task, agent, factory)
	if err != nil || result.Status != core.StatusFailed || result.Score != 0 || factory.calls != 1 {
		t.Fatalf("no-file task invented success or skipped independent verification: %+v %v", result, err)
	}
	for _, capture := range []string{
		`{"schema_version":1,"complete":false,"files":[],"error":"capture limit exceeded"}`,
		`{"schema_version":1,"complete":true,"files":[{"path":"../escape.pptx","size_bytes":0,"sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}],"error":""}`,
	} {
		agent.files[verifierDir+"/pptx-capture.json"] = []byte(capture)
		factory := &freshFactory{sandbox: verifier}
		result, err := benchmark.Evaluate(context.Background(), task, agent, factory)
		if err == nil || result.Artifact != nil || factory.calls != 0 {
			t.Fatal("incomplete or unsafe inventory reached verifier")
		}
	}
}

func TestPythonPPTXVerifierFixtures(t *testing.T) {
	if os.Getenv("ARIES_RUN_PYTHON_CASE_TESTS") != "1" {
		t.Skip("run explicitly on remote ECS with Python installed")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("pptx_verifier_test.py")
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(python, "-I", "-B", script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("real private PPTX verifier fixtures: %v\n%s", err, output)
	}
}
