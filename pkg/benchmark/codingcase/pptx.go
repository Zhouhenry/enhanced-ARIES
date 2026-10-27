package codingcase

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const WorkloadPPTX = "llm-development-pptx-v1"
const pptxInstruction = "幻灯片 生成一份关于大模型发展的pptx材料，写5页。"
const pptxFileLimit = 64 << 20
const pptxTotalLimit = 512 << 20
const pptxCountLimit = 128

//go:embed pptx_capture.py
var pptxCaptureScript string

//go:embed pptx_verifier.py
var pptxVerifierScript string

type pptxFile struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	LocalPath string `json:"local_path,omitempty"`
}

type pptxManifest struct {
	SchemaVersion int        `json:"schema_version"`
	Complete      bool       `json:"complete"`
	Files         []pptxFile `json:"files"`
	Error         string     `json:"error"`
}

type pptxReportFile struct {
	pptxFile
	Parseable  bool `json:"parseable"`
	SlideCount int  `json:"slide_count"`
	Slides     []struct {
		Part         string   `json:"part"`
		Nonempty     bool     `json:"nonempty"`
		ContentKinds []string `json:"content_kinds"`
	} `json:"slides"`
	Passed bool   `json:"passed"`
	Error  string `json:"error"`
}

type pptxReport struct {
	SchemaVersion int              `json:"schema_version"`
	Passed        bool             `json:"passed"`
	Files         []pptxReportFile `json:"files"`
}

func validatePPTXManifest(manifest pptxManifest) error {
	if manifest.SchemaVersion != 1 || !manifest.Complete || manifest.Error != "" || manifest.Files == nil || len(manifest.Files) > pptxCountLimit {
		return fmt.Errorf("incomplete or invalid PPTX inventory: %s", manifest.Error)
	}
	seen := make(map[string]bool, len(manifest.Files))
	var total int64
	for _, file := range manifest.Files {
		digest, err := hex.DecodeString(file.SHA256)
		if file.Path == "." || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path || strings.HasPrefix(file.Path, "../") || strings.ContainsAny(file.Path, "\\\x00") || !strings.HasSuffix(strings.ToLower(file.Path), ".pptx") || seen[file.Path] || file.SizeBytes < 0 || file.SizeBytes > pptxFileLimit || err != nil || len(digest) != sha256.Size || file.SHA256 != strings.ToLower(file.SHA256) {
			return errors.New("invalid, duplicate, or oversized PPTX artifact inventory entry")
		}
		seen[file.Path] = true
		total += file.SizeBytes
	}
	if total > pptxTotalLimit {
		return errors.New("PPTX artifact total size exceeds capture limit")
	}
	return nil
}

func (b *Benchmark) evaluatePPTX(ctx context.Context, task core.Task, sandbox runner.Sandbox, sandboxes runner.EvaluationSandboxes) (evaluation core.Evaluation, err error) {
	started := time.Now()
	evaluation.Status, evaluation.VerifierStatus = core.StatusFailed, core.StatusNotRun
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
		err = errors.New("PPTX task requires bounded artifact download and fresh evaluation sandboxes")
		return
	}
	dir, directoryErr := b.artifactDirectory(task.ID)
	if directoryErr != nil {
		err = directoryErr
		return
	}
	phaseCtx, phase := performance.Begin(ctx, "candidate.capture")
	defer func() { phase.End(err) }()
	result, captureErr := sandbox.Exec(phaseCtx, core.Command{Path: "python3", Args: []string{"-I", "-c", pptxCaptureScript, Workdir, verifierDir + "/pptx-capture.json"}, Dir: "/", User: "root", OutputLimitBytes: 4096})
	if captureErr != nil {
		err = fmt.Errorf("inventory PPTX artifacts: %w", captureErr)
		return
	}
	capturePath := filepath.Join(dir, "capture.json")
	if err = downloader.DownloadLimit(phaseCtx, verifierDir+"/pptx-capture.json", capturePath, reportLimit); err != nil {
		return
	}
	evaluation.LogPaths = append(evaluation.LogPaths, capturePath)
	capture, readErr := readBoundedRegular(capturePath, reportLimit)
	if readErr != nil {
		err = readErr
		return
	}
	var manifest pptxManifest
	if err = json.Unmarshal(capture, &manifest); err != nil {
		return
	}
	if err = validatePPTXManifest(manifest); err != nil {
		return
	}
	if result.ExitCode != 0 {
		err = fmt.Errorf("PPTX inventory exited %d despite a complete manifest", result.ExitCode)
		return
	}
	for index := range manifest.Files {
		file := &manifest.Files[index]
		file.LocalPath = filepath.Join(dir, "decks", filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(file.LocalPath), 0o700); err != nil {
			return
		}
		if err = downloader.DownloadLimit(phaseCtx, Workdir+"/"+file.Path, file.LocalPath, max(file.SizeBytes, 1)); err != nil {
			return
		}
		evaluation.LogPaths = append(evaluation.LogPaths, file.LocalPath)
		info, statErr := os.Lstat(file.LocalPath)
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != file.SizeBytes {
			err = errors.Join(errors.New("downloaded PPTX size or type differs from captured inventory"), statErr)
			return
		}
		data, fileErr := os.ReadFile(file.LocalPath)
		if fileErr != nil {
			err = fileErr
			return
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			err = errors.New("downloaded PPTX SHA-256 differs from captured inventory")
			return
		}
		if err = os.Chmod(file.LocalPath, 0o400); err != nil {
			return
		}
	}
	manifestData, marshalErr := json.MarshalIndent(manifest, "", "  ")
	if marshalErr != nil {
		err = marshalErr
		return
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	if err = writeExclusive(manifestPath, manifestData, 0o400); err != nil {
		return
	}
	digest := sha256.Sum256(manifestData)
	evaluation.Artifact = &core.ArtifactEvidence{ObservedAt: time.Now(), Path: manifestPath, SHA256: hex.EncodeToString(digest[:])}
	evaluation.LogPaths = append(evaluation.LogPaths, manifestPath)
	phase.End(nil)

	environment := task.Environment
	environment.AllowNetwork = false
	fresh, startErr := sandboxes.Start(ctx, environment)
	if startErr != nil {
		err = fmt.Errorf("start fresh PPTX verifier: %w", startErr)
		return
	}
	reports, ok := fresh.(runner.LimitedDownloader)
	if !ok || fresh == nil {
		err = errors.New("fresh PPTX verifier requires bounded report download")
		return
	}
	phaseCtx, phase = performance.Begin(ctx, "verifier.prepare")
	if err = execOK(phaseCtx, fresh, core.Command{Path: "python3", Args: []string{"-I", "-c", verifierPrepareScript}, Dir: "/", User: "root"}); err != nil {
		return
	}
	if err = fresh.Upload(phaseCtx, manifestPath, verifierDir+"/manifest.json"); err != nil {
		return
	}
	if err = uploadText(phaseCtx, fresh, verifierDir+"/verify_pptx.py", pptxVerifierScript); err != nil {
		return
	}
	if err = execOK(phaseCtx, fresh, core.Command{Path: "python3", Args: []string{"-I", verifierDir + "/verify_pptx.py", verifierDir, "--prepare"}, Dir: verifierDir, User: "root"}); err != nil {
		return
	}
	for _, file := range manifest.Files {
		if err = fresh.Upload(phaseCtx, file.LocalPath, verifierDir+"/decks/"+file.Path); err != nil {
			return
		}
	}
	phase.End(nil)
	evaluation.VerifierStatus = core.StatusFailed
	phaseCtx, phase = performance.Begin(ctx, "verifier.exec")
	verifyCtx, cancel := context.WithTimeout(phaseCtx, b.options.VerifierTimeout)
	result, runErr := fresh.Exec(verifyCtx, core.Command{Path: "python3", Args: []string{"-I", verifierDir + "/verify_pptx.py", verifierDir}, Dir: verifierDir, User: "root", Timeout: b.options.VerifierTimeout, OutputLimitBytes: reportLimit})
	cancel()
	phase.End(runErr)
	phaseCtx, phase = performance.Begin(ctx, "verifier.report")
	for _, output := range []struct{ name, content string }{{"stdout.log", result.Stdout}, {"stderr.log", result.Stderr}} {
		filePath := filepath.Join(dir, output.name)
		if writeErr := writeExclusive(filePath, []byte(output.content), 0o600); writeErr != nil {
			err = errors.Join(err, writeErr)
		} else {
			evaluation.LogPaths = append(evaluation.LogPaths, filePath)
		}
	}
	if runErr != nil || (result.ExitCode != 0 && result.ExitCode != 1) {
		err = errors.Join(err, fmt.Errorf("PPTX verifier infrastructure failed, exit=%d", result.ExitCode), runErr)
		return
	}
	reportPath := filepath.Join(dir, "pptx-report.json")
	if downloadErr := reports.DownloadLimit(phaseCtx, verifierDir+"/pptx-report.json", reportPath, reportLimit); downloadErr != nil {
		err = errors.Join(err, downloadErr)
		return
	}
	evaluation.LogPaths = append(evaluation.LogPaths, reportPath)
	report, reportErr := readBoundedRegular(reportPath, reportLimit)
	if reportErr != nil {
		err = errors.Join(err, reportErr)
		return
	}
	passed, reportErr := verifyPPTXReport(report, manifest, result.ExitCode)
	err = errors.Join(err, reportErr)
	if err == nil && passed {
		evaluation.Status, evaluation.VerifierStatus = core.StatusSucceeded, core.StatusSucceeded
		evaluation.Score, evaluation.Reward = 1, 1
	}
	return
}

func verifyPPTXReport(data []byte, manifest pptxManifest, exitCode int) (bool, error) {
	var report pptxReport
	if err := json.Unmarshal(data, &report); err != nil {
		return false, fmt.Errorf("invalid PPTX verifier report: %w", err)
	}
	if report.SchemaVersion != 1 || report.Files == nil || len(report.Files) != len(manifest.Files) {
		return false, errors.New("PPTX verifier report did not cover the complete artifact inventory")
	}
	passed := false
	for index, actual := range report.Files {
		expected := manifest.Files[index]
		if actual.Path != expected.Path || actual.SHA256 != expected.SHA256 || actual.SizeBytes != expected.SizeBytes || actual.SlideCount != len(actual.Slides) {
			return false, errors.New("PPTX verifier report disagrees with captured artifact identity or slide inventory")
		}
		filePass := actual.Parseable && actual.SlideCount == 5
		for _, slide := range actual.Slides {
			if slide.Part == "" || slide.Nonempty != (len(slide.ContentKinds) > 0) {
				return false, errors.New("PPTX verifier returned incomplete slide content evidence")
			}
			for _, kind := range slide.ContentKinds {
				if kind != "text" && kind != "image" && kind != "chart" {
					return false, errors.New("PPTX verifier returned unknown slide content evidence")
				}
			}
			filePass = filePass && slide.Nonempty
		}
		if filePass != actual.Passed || (!actual.Passed && actual.Error == "") {
			return false, errors.New("PPTX verifier per-file outcome disagrees with its checks")
		}
		passed = passed || filePass
	}
	if report.Passed != passed || (exitCode == 0) != passed {
		return false, errors.New("PPTX verifier report disagrees with process exit or per-file checks")
	}
	return passed, nil
}
