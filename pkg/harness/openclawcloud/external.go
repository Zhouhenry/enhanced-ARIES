package openclawcloud

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"path/filepath"
	"sync"
	"time"

	"github.com/hyscale-lab/aries/pkg/bridge/ssh"
	openclawssh "github.com/hyscale-lab/aries/pkg/bridge/ssh/openclaw"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/sandboxprocess"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
	gossh "golang.org/x/crypto/ssh"
)

type ExternalBridgeOptions struct {
	RunID, TaskID, SandboxID string
	Workdir, OutputDir       string
	// CommandEnv supplies the declared task environment; native per-call values win.
	CommandEnv map[string]string
	// InitialDirectoryMtimeUnixNS applies only after all bridge filesystem setup.
	InitialDirectoryMtimeUnixNS map[string]int64
	// BindHost must be a loopback/private interface reachable by the external
	// harness. The existing unauthenticated SSH dialect must never bind publicly.
	BindHost, AdvertiseHost            string
	ClientSourceFile                   string
	CleanupTimeout                     time.Duration
	IdentityScopeID, IdentitySessionID string
}

type ExternalBridge struct {
	options ExternalBridgeOptions
	mu      sync.Mutex
	used    bool
	group   *sandboxprocess.Group
	server  *openclawssh.Server
}

var _ runner.ToolBridge = (*ExternalBridge)(nil)

func NewExternalBridge(options ExternalBridgeOptions) (*ExternalBridge, error) {
	if options.BindHost == "" {
		options.BindHost = "127.0.0.1"
	}
	bind := net.ParseIP(options.BindHost)
	if bind == nil || (!bind.IsLoopback() && !bind.IsPrivate()) || bind.IsUnspecified() {
		return nil, errors.New("external OpenClaw SSH bridge requires an explicit private or loopback bind IP")
	}
	if options.AdvertiseHost != "" && !core.ValidEndpointHost(options.AdvertiseHost) {
		return nil, errors.New("invalid external bridge advertised host")
	}
	if options.RunID == "" || options.TaskID == "" || options.OutputDir == "" || !filepath.IsAbs(options.ClientSourceFile) {
		return nil, errors.New("external bridge requires occurrence identity, evidence directory and Linux SSH-client executable")
	}
	if options.IdentityScopeID != "" || options.IdentitySessionID != "" {
		identity := performance.NativeToolIdentity{SchemaVersion: 1, ScopeID: options.IdentityScopeID, AgentSessionID: options.IdentitySessionID, ToolCallID: "validation", NativeInvocationID: "validation"}
		if err := identity.ValidateNative(); err != nil {
			return nil, fmt.Errorf("external bridge native identity: %w", err)
		}
		if options.IdentityScopeID != options.RunID+"/"+options.TaskID {
			return nil, errors.New("external bridge native identity differs from its occurrence")
		}
	}
	if options.CleanupTimeout <= 0 {
		options.CleanupTimeout = 20 * time.Second
	}
	options.CommandEnv = maps.Clone(options.CommandEnv)
	options.InitialDirectoryMtimeUnixNS = maps.Clone(options.InitialDirectoryMtimeUnixNS)
	return &ExternalBridge{options: options}, nil
}

func (b *ExternalBridge) Start(ctx context.Context, sandbox runner.Sandbox) (core.ToolEndpoint, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.used {
		return core.ToolEndpoint{}, errors.New("external cloud bridge is single use")
	}
	b.used = true
	identity, ok := sandbox.(interface{ ID() string })
	if !ok || identity.ID() == "" {
		return core.ToolEndpoint{}, errors.New("external cloud bridge requires the allocated sandbox identity")
	}
	if b.options.SandboxID != "" && b.options.SandboxID != identity.ID() {
		return core.ToolEndpoint{}, errors.New("external cloud bridge sandbox identity mismatch")
	}
	b.options.SandboxID = identity.ID()
	g, err := sandboxprocess.New(ctx, sandbox, b.options.Workdir)
	b.group = g
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	b.server, err = openclawssh.New(ssh.Options{HostSigner: signer, SandboxID: b.options.SandboxID, OutputDir: b.options.OutputDir, CleanupTimeout: b.options.CleanupTimeout,
		IdentityScopeID: b.options.IdentityScopeID, IdentitySessionID: b.options.IdentitySessionID,
		ResolveListen: func(context.Context) (core.BridgeListen, error) {
			return core.BridgeListen{BindHost: b.options.BindHost}, nil
		},
	})
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	target := &cloudTarget{group: g, options: b.options, allowSetup: true}
	endpoint, err := b.server.StartTarget(ctx, target)
	target.mu.Lock()
	target.allowSetup = false
	target.mu.Unlock()
	if err != nil {
		return core.ToolEndpoint{}, err
	}
	if b.options.AdvertiseHost != "" {
		_, port, err := net.SplitHostPort(endpoint.Address)
		if err != nil {
			return core.ToolEndpoint{}, err
		}
		endpoint.Address = net.JoinHostPort(b.options.AdvertiseHost, port)
	}
	endpoint.ClientCommand = "/opt/aries/bin/aries-ssh-client"
	endpoint.ClientSourceFile = b.options.ClientSourceFile
	if err := prepareDirectoryMtimes(ctx, sandbox, b.options.InitialDirectoryMtimeUnixNS, b.options.OutputDir); err != nil {
		return core.ToolEndpoint{}, err
	}
	return endpoint, nil
}

func (b *ExternalBridge) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.used = true
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), b.options.CleanupTimeout)
	defer cancel()
	var errs []error
	if b.server != nil {
		errs = append(errs, b.server.Stop(cleanupCtx))
	}
	if b.group != nil {
		errs = append(errs, b.group.Stop(cleanupCtx))
	}
	return errors.Join(errs...)
}

type cloudTarget struct {
	group      *sandboxprocess.Group
	options    ExternalBridgeOptions
	mu         sync.Mutex
	allowSetup bool
}

func (t *cloudTarget) ContainerID() string   { return t.options.SandboxID }
func (t *cloudTarget) ContainerName() string { return t.options.SandboxID }
func (t *cloudTarget) RunID() string         { return t.options.RunID }
func (t *cloudTarget) TaskID() string        { return t.options.TaskID }
func (t *cloudTarget) Workdir() string       { return t.options.Workdir }
func (t *cloudTarget) ExecStream(ctx context.Context, c core.Command, in io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	if c.User == "0:0" {
		t.mu.Lock()
		allowed := t.allowSetup
		t.allowSetup = false
		t.mu.Unlock()
		if !allowed {
			return core.CommandResult{ExitCode: -1}, fmt.Errorf("unprivileged cloud tool cannot request root")
		}
		// Only the native server's one-time workspace alias preparation uses
		// root; agent SSH requests cannot select Command.User.
		c.User = "root"
		return t.group.Sandbox().ExecStream(ctx, c, in, out, errout)
	}
	if len(t.options.CommandEnv) != 0 {
		env := maps.Clone(t.options.CommandEnv)
		maps.Copy(env, c.Env)
		c.Env = env
	}
	return t.group.ExecStream(ctx, c, in, out, errout)
}
