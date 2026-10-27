package jiuwenswarm

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
)

// agentContainer exposes only control/evidence operations to the shared native
// collector. It is never provided to the Agent as its task sandbox.
type agentContainer struct {
	deployment deployment.Deployment
	id         string
}

func (*agentContainer) Connectivity() core.HarnessConnectivity { return core.HarnessConnectivity{} }
func (c *agentContainer) Exec(ctx context.Context, command core.Command) (core.CommandResult, error) {
	if command.User == "root" {
		command.User = "0:0"
	}
	return c.deployment.Exec(ctx, c.id, command)
}
func (*agentContainer) Upload(context.Context, string, string) error {
	return errors.New("Agent evidence transport does not upload task files")
}
func (c *agentContainer) Download(ctx context.Context, source, destination string) error {
	return c.DownloadLimit(ctx, source, destination, nativeSpanByteLimit)
}
func (c *agentContainer) DownloadLimit(ctx context.Context, source, destination string, limit int64) error {
	if limit < 0 {
		return errors.New("negative Agent evidence limit")
	}
	stream, info, err := c.deployment.DownloadArchive(ctx, c.id, source)
	if err != nil {
		return err
	}
	defer stream.Close()
	if !info.Mode.IsRegular() || info.Size < 0 || info.Size > limit {
		return errors.New("Agent evidence source is not a bounded regular file")
	}
	reader := tar.NewReader(stream)
	header, err := reader.Next()
	if err != nil {
		return err
	}
	if !header.FileInfo().Mode().IsRegular() || path.Base(header.Name) != path.Base(source) || header.Size != info.Size {
		return errors.New("Agent evidence archive differs from source")
	}
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	defer clear(raw)
	if err != nil {
		return err
	}
	if int64(len(raw)) != header.Size || int64(len(raw)) > limit {
		return errors.New("Agent evidence download exceeds declared size")
	}
	if _, err = reader.Next(); !errors.Is(err, io.EOF) {
		return errors.New("Agent evidence archive contains extra entries")
	}
	// The collector reserves a private temporary destination before calling a
	// downloader. Replace that file atomically; it is not an immutable artifact.
	if err := privatefiles.EnsureDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".agent-download-")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(raw); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), destination)
}
