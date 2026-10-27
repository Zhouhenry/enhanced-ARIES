// aries-model-replay serves one explicitly bound model session per listener.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hyscale-lab/aries/internal/modelreplay"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		logrus.WithError(err).Error("model replay stopped")
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("aries-model-replay", flag.ContinueOnError)
	mode := flags.String("mode", "", "record or replay (required)")
	listen := flags.String("listen", "127.0.0.1:8000", "listener for this attempt/session; remote access requires authenticated HTTPS ingress")
	output := flags.String("output", "", "new private output directory (required; parent must exist)")
	tapePath := flags.String("tape", "", "immutable tape.json for replay")
	model := flags.String("model", "", "served model ID; record defaults to DEEPSEEK_MODEL")
	upstream := flags.String("upstream", "", "upstream API root; record defaults to DEEPSEEK_BASE_URL")
	upstreamKeyEnv := flags.String("upstream-key-env", "DEEPSEEK_API_KEY", "record upstream key environment name")
	accessEnv := flags.String("access-token-env", "ARIES_REPLAY_ACCESS_TOKEN", "session access token environment name; minimum 16 bytes")
	runID := flags.String("run-id", "", "run identity (required)")
	attemptID := flags.String("attempt-id", "", "execution attempt identity (required)")
	sessionID := flags.String("session-id", "", "agent session identity (required)")
	placement := flags.String("placement", "", "external or embedded (required)")
	harnessVersion := flags.String("harness-version", "", "exact harness version/image pin (required)")
	idDialect := flags.String("tool-call-id-dialect", "", "recorded causal ID dialect: empty is exact; openclaw-2026.7.1 requires that harness version")
	timing := flags.String("timing", "recorded", "replay: zero, recorded or synthetic")
	initial := flags.Duration("initial-delay", 0, "synthetic first-frame delay")
	interval := flags.Duration("frame-interval", 0, "synthetic delay between SSE frames (not tokens)")
	timeout := flags.Duration("request-timeout", 5*time.Minute, "bounded model request duration")
	turns := flags.Int("max-turns", 128, "recording turn bound")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *mode == "" || *output == "" {
		return errors.New("usage: aries-model-replay --mode record|replay --output NEW_DIR --run-id ID --attempt-id ID --session-id ID --placement external|embedded --harness-version PIN")
	}
	host, _, err := net.SplitHostPort(*listen)
	if err != nil {
		return errors.New("listen must be host:port")
	}
	// TLS termination belongs to ingress; direct CLI listeners stay on loopback.
	if host != "localhost" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return errors.New("listen must be loopback; use authenticated HTTPS ingress for remote clients")
		}
	}
	o := modelreplay.Options{Mode: *mode, Identity: modelreplay.Identity{RunID: *runID, AttemptID: *attemptID, SessionID: *sessionID, Placement: *placement, HarnessVersion: *harnessVersion}, Model: *model, AccessToken: []byte(os.Getenv(*accessEnv)), OutputDir: *output, Timeout: *timeout, MaxTurns: *turns, Timing: modelreplay.Timing{Mode: *timing, InitialDelay: *initial, FrameInterval: *interval}}
	o.ToolCallIDDialect = *idDialect
	if err := modelreplay.ValidateToolCallIDDialect(o.ToolCallIDDialect, o.Identity.HarnessVersion); err != nil {
		return err
	}
	if *mode == "record" {
		if modelreplay.ValidateHistoryDialect(modelreplay.JiuwenHistoryDialect, o.Identity.HarnessVersion) == nil {
			o.HistoryDialect = modelreplay.JiuwenHistoryDialect
		}
		if *tapePath != "" {
			return errors.New("record mode does not accept a tape")
		}
		if o.Model == "" {
			o.Model = os.Getenv("DEEPSEEK_MODEL")
		}
		o.UpstreamURL = *upstream
		if o.UpstreamURL == "" {
			o.UpstreamURL = os.Getenv("DEEPSEEK_BASE_URL")
		}
		if o.UpstreamURL == "" {
			o.UpstreamURL = "https://api.deepseek.com"
		}
		o.UpstreamKey = []byte(os.Getenv(*upstreamKeyEnv))
	} else {
		if *upstream != "" {
			return errors.New("replay mode forbids upstream configuration")
		}
		if *tapePath == "" {
			return errors.New("replay mode requires --tape")
		}
		o.Tape, err = modelreplay.Load(*tapePath)
		if err != nil {
			return err
		}
		o.HistoryDialect = o.Tape.HistoryDialect
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer listener.Close()
	modelServer, err := modelreplay.New(o)
	clear(o.AccessToken)
	clear(o.UpstreamKey)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := &http.Server{Handler: modelServer, ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if server.Shutdown(shutdown) != nil {
			_ = server.Close()
		}
	}()
	logrus.WithFields(logrus.Fields{"listen": listener.Addr().String(), "mode": *mode, "placement": *placement}).Info("serving explicitly bound model session")
	err = server.Serve(listener)
	stop()
	<-stopped
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if closeErr := modelServer.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("finalize model session: %w", closeErr))
	}
	if observationErr := modelServer.ObservationError(); observationErr != nil {
		logrus.WithError(observationErr).WithField("observer_status", "failed").Error("model observation incomplete")
	}
	return err
}
