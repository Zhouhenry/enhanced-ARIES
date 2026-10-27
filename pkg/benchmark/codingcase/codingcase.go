// Package codingcase supplies versioned workloads for sandbox A/B checks.
package codingcase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const Workdir = "/workspace/aries-case"
const WorkloadLegacy = "merge-ranges"
const WorkloadPortable = "merge-ranges-portable-v1"
const WorkloadMultiCall = "merge-ranges-multicall-v1"
const verifierDir = "/run/aries-verifier"
const candidateLimit = 128 << 10
const reportLimit = 1 << 20
const expectedTests = 12

type Options struct {
	Workload        string
	TaskID          string
	Template        string
	OutputDir       string
	TaskTimeout     time.Duration
	VerifierTimeout time.Duration
}

type Benchmark struct{ options Options }

var _ runner.Benchmark = (*Benchmark)(nil)
var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,159}$`)

func New(options Options) (*Benchmark, error) {
	if options.Workload == "" {
		options.Workload = WorkloadLegacy
	}
	if options.Workload != WorkloadLegacy && options.Workload != WorkloadPortable && options.Workload != WorkloadMultiCall && options.Workload != WorkloadPPTX {
		return nil, fmt.Errorf("unknown codingcase workload %q", options.Workload)
	}
	if options.TaskID == "" {
		options.TaskID = options.Workload
	}
	if !safeID.MatchString(options.TaskID) || strings.TrimSpace(options.Template) == "" || options.OutputDir == "" {
		return nil, errors.New("codingcase requires a safe task ID, explicit template selection, and private output directory")
	}
	if options.TaskTimeout < 0 || options.VerifierTimeout < 0 {
		return nil, errors.New("codingcase timeouts must not be negative")
	}
	if options.TaskTimeout == 0 {
		options.TaskTimeout = 5 * time.Minute
	}
	if options.VerifierTimeout == 0 {
		options.VerifierTimeout = time.Minute
	}
	return &Benchmark{options: options}, nil
}

func (b *Benchmark) Tasks(ctx context.Context) ([]core.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []core.Task{{ID: b.options.TaskID, Instruction: b.instruction(), Timeout: b.options.TaskTimeout, Environment: core.Environment{Image: b.options.Template, Workdir: Workdir, ExecUser: "root", AllowNetwork: true}}}, nil
}

func (b *Benchmark) instruction() string {
	if b.options.Workload == WorkloadPPTX {
		return pptxInstruction
	}
	if b.options.Workload == WorkloadMultiCall {
		return multiCallInstruction
	}
	if b.options.Workload == WorkloadPortable {
		return portableInstruction
	}
	return instruction
}

// ForTask lets the app retain the logical catalog and give each occurrence a
// distinct artifact directory, without adding scheduling state to the benchmark.
func (b *Benchmark) ForTask(logicalID, occurrenceID string) (runner.Benchmark, error) {
	if logicalID != b.options.TaskID || !safeID.MatchString(occurrenceID) {
		return nil, errors.New("codingcase occurrence does not match its logical task")
	}
	options := b.options
	options.TaskID = occurrenceID
	return New(options)
}

func (b *Benchmark) validateTask(task core.Task) error {
	if task.ID != b.options.TaskID || task.Instruction != b.instruction() || task.Environment.Image != b.options.Template || task.Environment.Workdir != Workdir {
		return errors.New("codingcase task does not match the selected occurrence")
	}
	return nil
}

func (b *Benchmark) PrepareSandbox(ctx context.Context, task core.Task, sandbox runner.Sandbox) error {
	if err := b.validateTask(task); err != nil {
		return err
	}
	if sandbox == nil {
		return errors.New("codingcase requires a task sandbox")
	}
	if err := execOK(ctx, sandbox, core.Command{Path: "python3", Args: []string{"-I", "-c", prepareScript}, Dir: "/", User: "root"}); err != nil {
		return err
	}
	if b.options.Workload == WorkloadPPTX {
		if err := uploadText(ctx, sandbox, Workdir+"/README.md", b.instruction()); err != nil {
			return err
		}
		return execOK(ctx, sandbox, core.Command{Path: "/bin/chown", Args: []string{"-R", "60000:60000", Workdir}, Dir: "/", User: "root"})
	}
	files := map[string]string{"intervals.py": buggySource, "test_public.py": publicTests, "README.md": b.instruction()}
	for _, name := range []string{"intervals.py", "test_public.py", "README.md"} {
		if err := uploadText(ctx, sandbox, Workdir+"/"+name, files[name]); err != nil {
			return err
		}
	}
	if err := execOK(ctx, sandbox, core.Command{Path: "/bin/chown", Args: []string{"-R", "60000:60000", Workdir}, Dir: "/", User: "root"}); err != nil {
		return err
	}
	return execOK(ctx, sandbox, core.Command{Path: "python3", Args: []string{"-I", "-c", finalizeScript}, Dir: "/", User: "root"})
}

func (b *Benchmark) Evaluate(ctx context.Context, task core.Task, sandbox runner.Sandbox, sandboxes runner.EvaluationSandboxes) (evaluation core.Evaluation, err error) {
	if b.options.Workload == WorkloadPPTX {
		return b.evaluatePPTX(ctx, task, sandbox, sandboxes)
	}
	started := time.Now()
	evaluation.Status = core.StatusFailed
	evaluation.VerifierStatus = core.StatusNotRun
	defer func() {
		evaluation.Duration = time.Since(started)
		if err != nil {
			evaluation.Error = err.Error()
		}
	}()
	if err = b.validateTask(task); err != nil {
		return
	}
	downloader, ok := sandbox.(runner.LimitedDownloader)
	if !ok || sandbox == nil || sandboxes == nil {
		err = errors.New("codingcase requires bounded candidate download and fresh evaluation sandboxes")
		return
	}
	dir, dirErr := b.artifactDirectory(task.ID)
	if dirErr != nil {
		err = dirErr
		return
	}
	phaseCtx, phase := performance.Begin(ctx, "candidate.capture")
	defer func() { phase.End(err) }()
	candidate := filepath.Join(dir, "intervals.py")
	if err = execOK(phaseCtx, sandbox, core.Command{Path: "python3", Args: []string{"-I", "-c", candidateValidateScript}, Dir: "/", User: "root", OutputLimitBytes: 4096}); err != nil {
		return
	}
	if err = downloader.DownloadLimit(phaseCtx, Workdir+"/intervals.py", candidate, candidateLimit); err != nil {
		return
	}
	var source []byte
	if source, err = readBoundedRegular(candidate, candidateLimit); err != nil {
		return
	}
	if err = os.Chmod(candidate, 0o400); err != nil {
		return
	}
	digest := sha256.Sum256(source)
	evaluation.Artifact = &core.ArtifactEvidence{ObservedAt: time.Now(), Path: candidate, SHA256: hex.EncodeToString(digest[:])}
	evaluation.LogPaths = append(evaluation.LogPaths, candidate)
	phase.End(nil)
	// Only this immutable, bounded single-file candidate crosses into the clean
	// verifier environment. No agent test, conftest, config, or Git metadata does.
	environment := task.Environment
	environment.AllowNetwork = false
	fresh, startErr := sandboxes.Start(ctx, environment)
	if startErr != nil {
		err = fmt.Errorf("start fresh codingcase verifier: %w", startErr)
		return
	}
	if fresh == nil {
		err = errors.New("fresh codingcase verifier returned nil sandbox")
		return
	}
	reportDownloader, ok := fresh.(runner.LimitedDownloader)
	if !ok {
		err = errors.New("codingcase verifier requires bounded report download")
		return
	}
	phaseCtx, phase = performance.Begin(ctx, "verifier.prepare")
	if err = execOK(phaseCtx, fresh, core.Command{Path: "python3", Args: []string{"-I", "-c", verifierPrepareScript}, Dir: "/", User: "root"}); err != nil {
		return
	}
	if err = fresh.Upload(phaseCtx, candidate, Workdir+"/intervals.py"); err != nil {
		return
	}
	for _, file := range []struct{ name, content string }{{"conftest.py", verifierHelper}, {"test_public.py", verifierPublicTests}, {"test_hidden.py", hiddenTests}} {
		if err = uploadText(phaseCtx, fresh, verifierDir+"/"+file.name, file.content); err != nil {
			return
		}
	}
	if err = execOK(phaseCtx, fresh, core.Command{Path: "/bin/chmod", Args: []string{"0444", Workdir + "/intervals.py"}, Dir: "/", User: "root"}); err != nil {
		return
	}
	phase.End(nil)
	evaluation.VerifierStatus = core.StatusFailed
	phaseCtx, phase = performance.Begin(ctx, "verifier.exec")
	verifyCtx, cancel := context.WithTimeout(phaseCtx, b.options.VerifierTimeout)
	result, runErr := fresh.Exec(verifyCtx, core.Command{Path: "python3", Args: []string{"-I", "-m", "pytest", "-q", "-p", "no:cacheprovider", "--junitxml=" + verifierDir + "/report.xml", verifierDir + "/test_public.py", verifierDir + "/test_hidden.py"}, Dir: verifierDir, User: "root", Env: map[string]string{"PYTEST_DISABLE_PLUGIN_AUTOLOAD": "1"}, Timeout: b.options.VerifierTimeout, OutputLimitBytes: 1 << 20})
	cancel()
	phase.End(runErr)
	phaseCtx, phase = performance.Begin(ctx, "verifier.report")
	for _, file := range []struct{ name, content string }{{"stdout.log", result.Stdout}, {"stderr.log", result.Stderr}} {
		p := filepath.Join(dir, file.name)
		if writeErr := writeExclusive(p, []byte(file.content), 0o600); writeErr != nil {
			err = errors.Join(err, writeErr)
		} else {
			evaluation.LogPaths = append(evaluation.LogPaths, p)
		}
	}
	if runErr != nil {
		err = errors.Join(err, fmt.Errorf("run codingcase verifier: %w", runErr))
		return
	}
	if result.ExitCode != 0 && result.ExitCode != 1 {
		err = errors.Join(err, fmt.Errorf("pytest infrastructure failed with exit %d", result.ExitCode))
		return
	}
	reportPath := filepath.Join(dir, "report.xml")
	if downloadErr := reportDownloader.DownloadLimit(phaseCtx, verifierDir+"/report.xml", reportPath, reportLimit); downloadErr != nil {
		err = errors.Join(err, downloadErr)
		return
	}
	report, readErr := readBoundedRegular(reportPath, reportLimit)
	if readErr != nil {
		err = errors.Join(err, readErr)
		return
	}
	if modeErr := os.Chmod(reportPath, 0o600); modeErr != nil {
		err = errors.Join(err, modeErr)
		return
	}
	evaluation.LogPaths = append(evaluation.LogPaths, reportPath)
	passed, parseErr := verifyReport(report, result.ExitCode)
	if parseErr != nil {
		err = errors.Join(err, parseErr)
		return
	}
	if err != nil {
		return
	}
	if passed {
		evaluation.Status = core.StatusSucceeded
		evaluation.VerifierStatus = core.StatusSucceeded
		evaluation.Score = 1
		evaluation.Reward = 1
	}
	return
}

func execOK(ctx context.Context, sandbox runner.Sandbox, command core.Command) error {
	result, err := sandbox.Exec(ctx, command)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("codingcase setup command failed with exit %d", result.ExitCode)
	}
	return nil
}

func uploadText(ctx context.Context, sandbox runner.Sandbox, destination, content string) error {
	file, err := os.CreateTemp("", "aries-codingcase-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err = file.WriteString(content); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return sandbox.Upload(ctx, name, destination)
}

func (b *Benchmark) artifactDirectory(id string) (string, error) {
	parent := filepath.Join(b.options.OutputDir, id, "evaluation")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	for p := parent; ; p = filepath.Dir(p) {
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("codingcase artifact parent must be a real directory")
		}
		if filepath.Dir(p) == p || p == "." {
			break
		}
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(parent, "candidate-")
}

func readBoundedRegular(name string, limit int64) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > limit {
		return nil, errors.New("codingcase evidence must be a bounded nonempty regular file")
	}
	return os.ReadFile(name)
}

func writeExclusive(name string, content []byte, mode os.FileMode) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(content)
	return errors.Join(writeErr, f.Close())
}

type junitSuite struct {
	Tests    int `xml:"tests,attr"`
	Failures int `xml:"failures,attr"`
	Errors   int `xml:"errors,attr"`
	Skipped  int `xml:"skipped,attr"`
	Cases    []struct {
		Failure *struct{} `xml:"failure"`
		Error   *struct{} `xml:"error"`
		Skipped *struct{} `xml:"skipped"`
	} `xml:"testcase"`
}

func verifyReport(data []byte, exitCode int) (bool, error) {
	var report struct {
		XMLName xml.Name
		Suites  []junitSuite `xml:"testsuite"`
	}
	if err := xml.Unmarshal(data, &report); err != nil {
		return false, fmt.Errorf("invalid codingcase pytest report: %w", err)
	}
	if report.XMLName.Local != "testsuites" || len(report.Suites) != 1 {
		return false, errors.New("unexpected codingcase pytest report shape")
	}
	s := report.Suites[0]
	if s.Tests != expectedTests || len(s.Cases) != expectedTests || s.Failures < 0 || s.Errors != 0 || s.Skipped != 0 {
		return false, errors.New("codingcase verifier did not execute the complete test set")
	}
	failures := 0
	for _, c := range s.Cases {
		if c.Error != nil || c.Skipped != nil {
			return false, errors.New("codingcase verifier skipped or errored")
		}
		if c.Failure != nil {
			failures++
		}
	}
	if failures != s.Failures || (exitCode == 0) != (failures == 0) {
		return false, errors.New("codingcase pytest report disagrees with process exit")
	}
	return failures == 0, nil
}
