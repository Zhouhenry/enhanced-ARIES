package jiuwenswarm

import (
	"context"
	"errors"
	"path"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// Collect only after the owning runtime has confirmed process-group absence.
// The snapshot validates producer counts, exact identities and original clocks.
func collectNativeProcessFiles(ctx context.Context, sandbox runner.Sandbox, directory, output string) (paths []string, result error) {
	downloader, ok := sandbox.(runner.LimitedDownloader)
	if !ok {
		return nil, errors.New("Jiuwen process evidence requires bounded downloads")
	}
	if err := privatefiles.EnsureDirectory(output); err != nil {
		return nil, err
	}
	for _, item := range []struct {
		source, target string
		limit          int64
	}{
		{"process-index.json", "jiuwen-process-index.json", 1 << 20},
		{"process-events.jsonl", "jiuwen-process-events.jsonl", 64 << 20},
	} {
		target := filepath.Join(output, item.target)
		if err := downloader.DownloadLimit(ctx, path.Join(directory, item.source), target, item.limit); err != nil {
			result = errors.Join(result, errors.New("Jiuwen process evidence download failed: "+item.source))
		} else {
			paths = append(paths, target)
		}
	}
	return paths, result
}
