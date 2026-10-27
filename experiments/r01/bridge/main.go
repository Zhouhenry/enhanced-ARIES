// bridge is an ECS-only native-protocol experiment, never production wiring.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	bridgessh "github.com/hyscale-lab/aries/pkg/bridge/ssh"
	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
	"github.com/hyscale-lab/aries/pkg/bridge/ssh/openclaw"
	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment/docker"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/sandbox/e2b"
	"golang.org/x/crypto/ssh"
)

const taskID = "r01-native-poc"

type cloudConfig struct {
	Endpoint       string `json:"endpoint"`
	Token          string `json:"token"`
	Template       string `json:"template"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

func loadCloudConfig(path string) (cloudConfig, error) {
	var config cloudConfig
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return config, errors.New("cloud config must be an absolute clean private path")
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return config, errors.New("cannot open private cloud config")
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return config, errors.New("cannot inspect private cloud config")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Geteuid() || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 16384 {
		return config, errors.New("cloud config must be an owned private regular file <=16KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, errors.New("invalid cloud config JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return config, errors.New("trailing cloud config data")
	}
	if config.Endpoint == "" || len(config.Token) < 32 || config.Template == "" {
		return config, errors.New("cloud endpoint, token and template are required")
	}
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 300
	}
	if config.TimeoutSeconds != 300 && config.TimeoutSeconds != 600 {
		return config, errors.New("cloud timeout_seconds must be 300 or 600")
	}
	return config, nil
}

type target struct {
	exec                                             func(context.Context, core.Command, io.Reader, io.Writer, io.Writer) (core.CommandResult, error)
	id, scope, output, defaultUser, measurementScope string
	cloud                                            bool
	stop                                             context.CancelFunc
	mu                                               sync.Mutex
}

func (t *target) ContainerID() string   { return t.id }
func (t *target) ContainerName() string { return "r01-poc-guest" }
func (t *target) RunID() string         { return t.scope }
func (t *target) TaskID() string        { return taskID }
func (t *target) Workdir() string       { return "/work" }
func (t *target) ExecStream(ctx context.Context, command core.Command, in io.Reader, out, errout io.Writer) (core.CommandResult, error) {
	if command.User == "" {
		command.User = t.defaultUser
	}
	if t.cloud && command.User == "0:0" {
		command.User = "root"
	}
	stdout, stderr := sha256.New(), sha256.New()
	identity, linked := execidentity.FromContext(ctx)
	result, err := t.exec(ctx, command, in, io.MultiWriter(out, stdout), io.MultiWriter(errout, stderr))
	record := map[string]any{"native_identity": identity, "linked": linked,
		"execution_id": performance.ExecutionID(ctx), "command": command,
		"exit_code": result.ExitCode, "transport_error": err != nil,
		"stdout_sha256": hex.EncodeToString(stdout.Sum(nil)), "stderr_sha256": hex.EncodeToString(stderr.Sum(nil)),
		"measurement_scope": t.measurementScope}
	if t.cloud {
		record["process_binding_source"] = taskID + "/e2b-operations.jsonl"
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	file, writeErr := os.OpenFile(filepath.Join(t.output, "target-events.jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if writeErr == nil {
		writeErr = errors.Join(json.NewEncoder(file).Encode(record), file.Close())
	}
	if writeErr != nil {
		t.stop()
	}
	return result, errors.Join(err, writeErr)
}

func writeJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	return errors.Join(json.NewEncoder(file).Encode(value), file.Sync(), file.Close())
}

func run(container, cloudPath, output, scope, session string) (runErr error) {
	if (container == "") == (cloudPath == "") || output == "" || scope == "" || session == "" || !filepath.IsAbs(output) {
		return errors.New("require --output/--scope/--session and exactly one of --container or --cloud-config")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(output)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("output must be a private directory")
	}
	for _, name := range []string{"ready.json", "cleanup.json", "stopped", "target-events.jsonl", "tool-calls.jsonl", taskID} {
		if _, err := os.Lstat(filepath.Join(output, name)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("output already contains experiment artifacts")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	var server *openclaw.Server
	var deployment *docker.Manager
	var manager *e2b.Manager
	var live *e2b.Sandbox
	cloudStarted := false
	bridgeStarted := false
	defer func() {
		report := map[string]any{"schema_version": 1, "cloud": cloudPath != "", "scope_id": scope, "bridge_started": bridgeStarted, "cloud_start_attempted": cloudStarted}
		var cleanupErr error
		if server != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			cleanupErr = server.Stop(cleanup)
			cancel()
		}
		report["bridge_stop_confirmed"] = cleanupErr == nil
		if manager != nil {
			if live != nil {
				cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				stopErr := manager.Stop(cleanup, live)
				cancel()
				report["sandbox_handle"] = live.ID()
				report["sandbox_absence_confirmed"] = stopErr == nil
				cleanupErr = errors.Join(cleanupErr, stopErr)
			} else {
				report["sandbox_not_allocated"] = true
			}
			observation, observeErr := manager.FinishObservation()
			report["observation"] = observation
			cleanupErr = errors.Join(cleanupErr, observeErr)
		}
		if deployment != nil {
			cleanupErr = errors.Join(cleanupErr, deployment.Close())
		}
		report["cleanup_succeeded"] = cleanupErr == nil
		if cleanupErr != nil {
			report["error"] = cleanupErr.Error()
		}
		if runErr != nil {
			report["run_error"] = runErr.Error()
		}
		cleanupErr = errors.Join(cleanupErr, writeJSON(filepath.Join(output, "cleanup.json"), report))
		if cleanupErr == nil && (cloudPath == "" || live != nil) {
			cleanupErr = os.WriteFile(filepath.Join(output, "stopped"), []byte("confirmed\n"), 0600)
		}
		runErr = errors.Join(runErr, cleanupErr)
	}()
	t := &target{id: container, scope: scope, output: output, defaultUser: "1000:1000", measurementScope: "docker_exec_return", stop: stop}
	if cloudPath == "" {
		deployment, err = docker.New(docker.Options{})
		if err != nil {
			return err
		}
		t.exec = func(ctx context.Context, c core.Command, in io.Reader, out, errout io.Writer) (core.CommandResult, error) {
			return deployment.ExecStream(ctx, container, c, in, out, errout)
		}
	} else {
		config, err := loadCloudConfig(cloudPath)
		if err != nil {
			return err
		}
		manager, err = e2b.New(e2b.Options{Provider: "e2b-cloud", Endpoint: config.Endpoint, Token: config.Token, Template: config.Template, OutputDir: output, TimeoutSeconds: config.TimeoutSeconds, ObservationLevel: performance.ObservationL1})
		if err != nil {
			return err
		}
		var deadlineStop context.CancelFunc
		ctx, deadlineStop = context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds-30)*time.Second)
		defer deadlineStop()
		if err := manager.CheckProvider(ctx); err != nil {
			return err
		}
		cloudStarted = true
		allocated, startErr := manager.Start(ctx, core.SandboxRequest{RunID: scope, TaskID: taskID, Purpose: "r01-native-protocol-poc", Environment: core.Environment{Workdir: "/work", ExecUser: "aries-agent", AllowNetwork: true}})
		if allocated != nil {
			live = allocated.(*e2b.Sandbox)
		}
		if live != nil {
			if err := writeJSON(filepath.Join(output, "cloud-allocation.json"), map[string]any{"schema_version": 1, "sandbox_handle": live.ID(), "timeout_seconds": config.TimeoutSeconds, "creation_response_confirmed": startErr == nil}); err != nil {
				return errors.Join(startErr, err)
			}
		}
		if startErr != nil {
			return startErr
		}
		if live == nil {
			return errors.New("cloud creation returned no owned handle")
		}
		prepared, err := live.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", "import os,pwd; a=pwd.getpwnam('aries-agent'); assert (a.pw_uid,a.pw_gid)==(60000,60000); os.mkdir('/work',0o755); os.chown('/work',a.pw_uid,a.pw_gid)"}, Dir: "/", User: "root", Timeout: 30 * time.Second, OutputLimitBytes: 4096})
		if evidenceErr := writeJSON(filepath.Join(output, "cloud-preparation.json"), map[string]any{"result": prepared, "transport_error": err != nil}); evidenceErr != nil {
			return errors.Join(err, evidenceErr)
		}
		if err != nil {
			return err
		}
		if prepared.ExitCode != 0 {
			return errors.New("cloud /work preparation failed; inspect operation evidence")
		}
		t.id, t.cloud, t.defaultUser, t.measurementScope = live.ID(), true, "aries-agent", "e2b_exec_return"
		t.exec = live.ExecStream
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return err
	}
	server, err = openclaw.New(bridgessh.Options{HostSigner: signer, SandboxID: t.id, OutputDir: output, IdentityScopeID: scope, IdentitySessionID: session, ResolveListen: func(context.Context) (core.BridgeListen, error) { return core.BridgeListen{BindHost: "127.0.0.1"}, nil }})
	if err != nil {
		return err
	}
	endpoint, err := server.StartTarget(ctx, t)
	if err != nil {
		return err
	}
	bridgeStarted = true
	if err := writeJSON(filepath.Join(output, "ready.json"), map[string]any{"address": endpoint.Address, "agent_session_id": session, "scope_id": scope}); err != nil {
		return err
	}
	<-ctx.Done()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return errors.New("fixture lifetime reached cleanup deadline")
	}
	return nil
}

func main() {
	container := flag.String("container", "", "borrowed experiment guest Docker container")
	cloud := flag.String("cloud-config", "", "absolute private JSON path with endpoint/token/template")
	output := flag.String("output", "", "private fresh experiment evidence directory")
	scope := flag.String("scope", "", "attempt identity scope")
	session := flag.String("session", "", "native agent session")
	flag.Parse()
	if err := run(*container, *cloud, *output, *scope, *session); err != nil {
		fmt.Fprintln(os.Stderr, "r01 bridge:", err)
		os.Exit(1)
	}
}
