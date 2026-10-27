package openclaw

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
)

func (manager *Manager) verifyNativeVersion(ctx context.Context, active *session) error {
	probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	evidence := struct {
		Status          string    `json:"status"`
		ObservedAt      time.Time `json:"observed_at"`
		ExpectedVersion string    `json:"expected_version"`
		Executable      string    `json:"executable"`
		OpenClaw        string    `json:"openclaw,omitempty"`
		Node            string    `json:"node,omitempty"`
		ErrorClass      string    `json:"error_class,omitempty"`
	}{Status: "FAIL", ExpectedVersion: manager.options.ExpectedVersion, Executable: "/usr/local/bin/openclaw"}
	var failure error
	for _, probe := range []struct {
		path  string
		value *string
	}{
		{"/usr/local/bin/openclaw", &evidence.OpenClaw}, {"/usr/local/bin/node", &evidence.Node},
	} {
		result, err := manager.runtime.Options.Deployment.Exec(probeCtx, active.ID, core.Command{Path: probe.path, Args: []string{"--version"}, Dir: "/app", Timeout: 15 * time.Second, OutputLimitBytes: 4096})
		if err != nil {
			failure = active.Credentials.RedactErr(err)
			evidence.ErrorClass = "probe_error"
			break
		}
		if result.ExitCode != 0 || len(result.Stdout) > 4096 {
			failure = errors.New("OpenClaw native version probe failed")
			evidence.ErrorClass = "probe_failed"
			break
		}
		*probe.value = strings.TrimSpace(result.Stdout)
	}
	if failure == nil {
		match := regexp.MustCompile(`^(?:OpenClaw )?` + regexp.QuoteMeta(manager.options.ExpectedVersion) + `(?: \([0-9a-f]+\))?$`)
		if !match.MatchString(evidence.OpenClaw) || !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(evidence.Node) {
			failure = errors.New("OpenClaw native version differs from the pin or Node version is unavailable")
			evidence.ErrorClass = "version_mismatch"
		} else {
			evidence.Status = "PASS"
		}
	}
	evidence.ObservedAt = time.Now().UTC()
	content, err := json.MarshalIndent(evidence, "", "  ")
	if err == nil {
		// Startup rollback removes staged harness artifacts; retain the probe at attempt scope.
		path := filepath.Join(manager.runtime.Options.OutputDir, active.TaskID, "native-version.json")
		err = privatefiles.WriteArtifact(path, active.Credentials.Redact(content))
		if err == nil {
			active.Artifacts.Add(path)
		}
	}
	return errors.Join(failure, err)
}
