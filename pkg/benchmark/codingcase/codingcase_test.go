package codingcase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

type fakeSandbox struct {
	files           map[string][]byte
	uploaded        map[string][]byte
	exitCode        int
	execErr         error
	finalizeErr     error
	finalizeExit    int
	finalizeCalls   int
	finalizeUploads int
	commands        []core.Command
	downloadLimits  []int64
}

func newFakeSandbox() *fakeSandbox {
	return &fakeSandbox{files: map[string][]byte{}, uploaded: map[string][]byte{}}
}
func (*fakeSandbox) Connectivity() core.HarnessConnectivity { return core.HarnessConnectivity{} }
func (s *fakeSandbox) Exec(ctx context.Context, c core.Command) (core.CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return core.CommandResult{}, err
	}
	s.commands = append(s.commands, c)
	for _, arg := range c.Args {
		if arg == finalizeScript {
			s.finalizeCalls++
			s.finalizeUploads = len(s.uploaded)
			return core.CommandResult{ExitCode: s.finalizeExit}, s.finalizeErr
		}
		if arg == "pytest" {
			return core.CommandResult{ExitCode: s.exitCode, Stdout: "real boundary supplied by fake sandbox\n"}, s.execErr
		}
	}
	return core.CommandResult{}, nil
}
func (s *fakeSandbox) Upload(_ context.Context, source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	s.uploaded[destination] = data
	return nil
}
func (s *fakeSandbox) Download(context.Context, string, string) error {
	return errors.New("unbounded download is forbidden")
}
func (s *fakeSandbox) DownloadLimit(ctx context.Context, source, destination string, limit int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.downloadLimits = append(s.downloadLimits, limit)
	data, ok := s.files[source]
	if !ok {
		return runner.ErrNotFound
	}
	if int64(len(data)) > limit {
		return errors.New("download size limit exceeded")
	}
	return os.WriteFile(destination, data, 0o600)
}

type freshFactory struct {
	sandbox     runner.Sandbox
	err         error
	started     time.Time
	environment core.Environment
	calls       int
}

func (f *freshFactory) Start(ctx context.Context, e core.Environment) (runner.Sandbox, error) {
	f.calls++
	f.started = time.Now()
	f.environment = e
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.sandbox, f.err
}

func benchmarkTask(t *testing.T) (*Benchmark, core.Task) {
	t.Helper()
	b, err := New(Options{TaskID: "merge-ranges", Template: "explicit-template", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := b.Tasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return b, tasks[0]
}
func candidate(t *testing.T) *fakeSandbox {
	t.Helper()
	gold, err := os.ReadFile("testdata/gold.py")
	if err != nil {
		t.Fatal(err)
	}
	s := newFakeSandbox()
	s.files[Workdir+"/intervals.py"] = gold
	s.files[Workdir+"/test_public.py"] = []byte("raise AssertionError('agent edited tests')")
	return s
}
func junit(failures int) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, `<testsuites><testsuite tests="12" failures="%d" errors="0" skipped="0">`, failures)
	for i := 0; i < expectedTests; i++ {
		fmt.Fprintf(&b, `<testcase name="test_%d">`, i)
		if i < failures {
			b.WriteString(`<failure message="candidate wrong"/>`)
		}
		b.WriteString(`</testcase>`)
	}
	b.WriteString(`</testsuite></testsuites>`)
	return []byte(b.String())
}

func TestPrepareExposesOnlyBugContractAndKnownPublicTests(t *testing.T) {
	b, task := benchmarkTask(t)
	sandbox := newFakeSandbox()
	if task.Environment.Image != "explicit-template" || task.Environment.Workdir != Workdir || task.Environment.ExecUser != "root" {
		t.Fatal("environment does not identify explicit template/controller user")
	}
	if err := b.PrepareSandbox(context.Background(), task, sandbox); err != nil {
		t.Fatal(err)
	}
	if len(sandbox.uploaded) != 3 || string(sandbox.uploaded[Workdir+"/intervals.py"]) != buggySource || string(sandbox.uploaded[Workdir+"/test_public.py"]) != publicTests {
		t.Fatalf("public setup: %v", sandbox.uploaded)
	}
	if sandbox.finalizeCalls != 1 || sandbox.finalizeUploads != 3 {
		t.Fatal("public input metadata was not finalized after all uploads")
	}
	for path, data := range sandbox.uploaded {
		if strings.Contains(path, "hidden") || strings.Contains(string(data), "10**40") || strings.Contains(string(data), "for start, end in sorted(ranges)") {
			t.Fatal("private verifier or gold solution reached task")
		}
	}
}

func TestPrepareFailsWhenPublicMetadataCannotBeFinalized(t *testing.T) {
	transportErr := errors.New("metadata transport failed")
	for _, tc := range []struct {
		name string
		exit int
		err  error
	}{
		{"transport", 0, transportErr},
		{"command", 1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, task := benchmarkTask(t)
			sandbox := newFakeSandbox()
			sandbox.finalizeExit, sandbox.finalizeErr = tc.exit, tc.err
			err := b.PrepareSandbox(context.Background(), task, sandbox)
			if err == nil || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatalf("metadata failure disappeared: %v", err)
			}
			if sandbox.finalizeCalls != 1 || sandbox.finalizeUploads != 3 {
				t.Fatal("metadata failure was not from finalizing the complete public fixture")
			}
		})
	}
}

func TestEvaluateCapturesImmutableCandidateBeforeFreshPrivateVerifier(t *testing.T) {
	b, task := benchmarkTask(t)
	agent := candidate(t)
	verifier := newFakeSandbox()
	verifier.files[verifierDir+"/report.xml"] = junit(0)
	factory := &freshFactory{sandbox: verifier}
	evaluation, err := b.Evaluate(context.Background(), task, agent, factory)
	if err != nil || evaluation.Status != core.StatusSucceeded || evaluation.Score != 1 {
		t.Fatalf("evaluate: %+v %v", evaluation, err)
	}
	if factory.calls != 1 || factory.environment.AllowNetwork || factory.environment.Image != task.Environment.Image {
		t.Fatal("verifier did not get a fresh network-disabled environment")
	}
	if evaluation.Artifact == nil || evaluation.Artifact.ObservedAt.After(factory.started) {
		t.Fatal("candidate readiness was fabricated after verifier start")
	}
	data, err := os.ReadFile(evaluation.Artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if evaluation.Artifact.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("candidate hash mismatch")
	}
	info, err := os.Stat(evaluation.Artifact.Path)
	if err != nil || info.Mode().Perm() != 0o400 {
		t.Fatalf("candidate was not sealed: %v %v", info, err)
	}
	if string(verifier.uploaded[Workdir+"/intervals.py"]) != string(data) || string(verifier.uploaded[verifierDir+"/test_public.py"]) != verifierPublicTests || string(verifier.uploaded[verifierDir+"/test_hidden.py"]) != hiddenTests {
		t.Fatal("verifier did not reconstruct known tests around the exact candidate")
	}
	if len(agent.uploaded) != 0 || len(verifier.uploaded) != 4 || agent.downloadLimits[0] != candidateLimit {
		t.Fatal("agent workspace received private inputs or unbounded candidate transfer")
	}
}

func TestVerifierRejectsDisagreementAndIncompleteEvidence(t *testing.T) {
	for _, tc := range []struct {
		name       string
		exit       int
		report     []byte
		wantError  bool
		wantStatus string
	}{
		{"candidate_fails", 1, junit(1), false, core.StatusFailed},
		{"zero_exit_with_failures", 0, junit(1), true, core.StatusFailed},
		{"failure_exit_with_pass_report", 1, junit(0), true, core.StatusFailed},
		{"missing_report", 0, nil, true, core.StatusFailed},
		{"no_tests", 0, []byte(`<testsuites><testsuite tests="0"/></testsuites>`), true, core.StatusFailed},
		{"pytest_broken", 4, junit(0), true, core.StatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, task := benchmarkTask(t)
			verifier := newFakeSandbox()
			verifier.exitCode = tc.exit
			if tc.report != nil {
				verifier.files[verifierDir+"/report.xml"] = tc.report
			}
			result, err := b.Evaluate(context.Background(), task, candidate(t), &freshFactory{sandbox: verifier})
			if (err != nil) != tc.wantError || result.Status != tc.wantStatus || result.Reward != 0 || result.Artifact == nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestCandidateFailureCannotStartVerifierOrInventReadiness(t *testing.T) {
	for _, content := range [][]byte{nil, make([]byte, candidateLimit+1)} {
		b, task := benchmarkTask(t)
		agent := newFakeSandbox()
		if content != nil {
			agent.files[Workdir+"/intervals.py"] = content
		}
		factory := &freshFactory{}
		result, err := b.Evaluate(context.Background(), task, agent, factory)
		if err == nil || result.Artifact != nil || factory.calls != 0 || result.VerifierStatus != core.StatusNotRun {
			t.Fatalf("failure disappeared: %+v %v starts=%d", result, err, factory.calls)
		}
	}
}

func TestFreshFailureRetainsAlreadySealedArtifact(t *testing.T) {
	b, task := benchmarkTask(t)
	factory := &freshFactory{err: errors.New("provider unavailable")}
	result, err := b.Evaluate(context.Background(), task, candidate(t), factory)
	if err == nil || result.Artifact == nil || result.VerifierStatus != core.StatusNotRun {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestOccurrencesAndConcurrentEvidenceNeverOverwrite(t *testing.T) {
	b, task := benchmarkTask(t)
	child, err := b.ForTask(task.ID, task.ID+"--occ-2")
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := child.Tasks(context.Background())
	if err != nil || tasks[0].ID == task.ID {
		t.Fatal("logical and occurrence IDs were conflated")
	}
	paths := make(chan string, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			agent := newFakeSandbox()
			agent.files[Workdir+"/intervals.py"] = []byte(buggySource)
			fresh := newFakeSandbox()
			fresh.exitCode = 1
			fresh.files[verifierDir+"/report.xml"] = junit(1)
			result, err := b.Evaluate(context.Background(), task, agent, &freshFactory{sandbox: fresh})
			failures <- err
			if result.Artifact != nil {
				paths <- result.Artifact.Path
			}
		}()
	}
	wg.Wait()
	close(paths)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	for path := range paths {
		if seen[path] {
			t.Fatal("concurrent candidates overwrote one another")
		}
		seen[path] = true
	}
	if len(seen) != 2 {
		t.Fatal("concurrent candidate disappeared")
	}
}

func TestCandidateEvidenceRejectsSymlinkAndCanceledDownload(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.py")
	link := filepath.Join(dir, "link.py")
	if err := os.WriteFile(target, []byte("x=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedRegular(link, candidateLimit); err == nil {
		t.Fatal("symlink candidate accepted")
	}
	b, task := benchmarkTask(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	factory := &freshFactory{}
	result, err := b.Evaluate(ctx, task, candidate(t), factory)
	if !errors.Is(err, context.Canceled) || result.Artifact != nil || factory.calls != 0 {
		t.Fatalf("cancellation: %+v %v", result, err)
	}
}
