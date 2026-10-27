package openclawcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// Preparation changes directory entries (notably /aries), so this runs after
// bridge setup and before returning its endpoint to the Harness.
func prepareDirectoryMtimes(ctx context.Context, sandbox runner.Sandbox, timestamps map[string]int64, outputDir string) error {
	if len(timestamps) == 0 {
		return nil
	}
	payload, err := json.Marshal(timestamps)
	if err != nil {
		return err
	}
	result, executionErr := sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", directoryMtimeScript, string(payload)}, User: "root", Dir: "/", OutputLimitBytes: 65536})
	if executionErr == nil && result.ExitCode != 0 {
		executionErr = fmt.Errorf("initial directory mtime preparation exited with code %d", result.ExitCode)
	}
	evidence := struct {
		Requested map[string]int64   `json:"requested_unix_ns"`
		Result    core.CommandResult `json:"command_result"`
		Error     string             `json:"error,omitempty"`
	}{Requested: timestamps, Result: result}
	if executionErr != nil {
		evidence.Error = executionErr.Error()
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err == nil {
		err = os.MkdirAll(outputDir, 0o700)
	}
	if err == nil {
		err = os.WriteFile(filepath.Join(outputDir, "initial-directory-mtimes.json"), append(raw, '\n'), 0o600)
	}
	return errors.Join(executionErr, err)
}

const directoryMtimeScript = `import json, os, stat, sys
requested = json.loads(sys.argv[1])
records = []
try:
    for directory, timestamp in sorted(requested.items()):
        before = os.lstat(directory)
        if not stat.S_ISDIR(before.st_mode) or os.path.realpath(directory) != directory:
            raise RuntimeError("initial mtime target must be a real directory: " + directory)
        record = {"path": directory, "before_mtime_unix_ns": before.st_mtime_ns, "before_atime_unix_ns": before.st_atime_ns, "requested_mtime_unix_ns": timestamp}
        records.append(record)
        os.utime(directory, ns=(before.st_atime_ns, timestamp), follow_symlinks=False)
        after = os.lstat(directory)
        record.update(after_mtime_unix_ns=after.st_mtime_ns, after_atime_unix_ns=after.st_atime_ns)
        if after.st_mtime_ns != timestamp or after.st_atime_ns != before.st_atime_ns:
            raise RuntimeError("initial directory mtime readback differs: " + directory)
except BaseException as error:
    print(json.dumps({"directories": records, "error": str(error)}), flush=True)
    raise
print(json.dumps({"directories": records}), flush=True)
`
