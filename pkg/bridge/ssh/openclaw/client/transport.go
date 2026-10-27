package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/hyscale-lab/aries/pkg/bridge/ssh/execidentity"
	gossh "golang.org/x/crypto/ssh"
)

// Config supplies an already validated destination.
// Invocation/config-file policy belongs to the adapter, not the SSH transport.
type Config struct {
	Address  string
	User     string
	Identity *execidentity.Metadata
}

// Run forwards remote verbatim; it never interprets or executes shell syntax.
// It owns stdin and closes it on return. Close must unblock Read, allowing an
// early remote exit or cancellation to join the input copier. Writers must not
// block indefinitely. EOF half-closes input while output continues to drain.
func Run(ctx context.Context, configuration Config, remote string, stdin io.ReadCloser, stdout, stderr io.Writer) (int, error) {
	if stdin == nil {
		return 255, errors.New("SSH stdin is required")
	}
	defer stdin.Close()
	if configuration.User == "" {
		return 255, errors.New("SSH user is required")
	}
	var identityPayload []byte
	if configuration.Identity != nil {
		if err := configuration.Identity.Validate(); err != nil {
			return 255, err
		}
		var payload bytes.Buffer
		encoder := json.NewEncoder(&payload)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(configuration.Identity); err != nil {
			return 255, err
		}
		identityPayload = payload.Bytes()
		if len(identityPayload) > execidentity.MaxPayload {
			return 255, errors.New("SSH tool identity payload exceeds limit")
		}
	}
	address := configuration.Address
	dialer := net.Dialer{Timeout: lockedConnectTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return 255, fmt.Errorf("connect %s: %w", address, err)
	}
	defer connection.Close()
	stopCancellation := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stopCancellation()
	_ = connection.SetDeadline(time.Now().Add(lockedConnectTimeout))
	clientConfiguration := &gossh.ClientConfig{
		User:            configuration.User,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         lockedConnectTimeout,
	}
	sshConnection, channels, requests, err := gossh.NewClientConn(connection, address, clientConfiguration)
	if err != nil {
		return 255, fmt.Errorf("establish SSH connection: %w", err)
	}
	_ = connection.SetDeadline(time.Time{})
	client := gossh.NewClient(sshConnection, channels, requests)
	defer client.Close()
	stopKeepalive := startKeepalive(client, connection)
	defer stopKeepalive()
	session, err := client.NewSession()
	if err != nil {
		return 255, fmt.Errorf("open SSH session: %w", err)
	}
	defer session.Close()
	if identityPayload != nil {
		// Never send stdin or exec when the same channel has not accepted identity.
		_ = connection.SetDeadline(time.Now().Add(lockedConnectTimeout))
		accepted, err := session.SendRequest(execidentity.RequestType, true, identityPayload)
		_ = connection.SetDeadline(time.Time{})
		if err != nil || !accepted {
			return 255, errors.New("SSH tool identity was not accepted")
		}
	}
	input, err := session.StdinPipe()
	if err != nil {
		return 255, fmt.Errorf("open SSH input: %w", err)
	}
	inputDone := make(chan struct{})
	var stopping atomic.Bool
	reader := &inputReader{source: stdin, stopping: &stopping}
	var inputError error
	go func() { defer close(inputDone); _, inputError = io.Copy(input, reader); _ = input.Close() }()
	session.Stdout = stdout
	session.Stderr = stderr
	err = session.Run(remote)
	stopping.Store(true)
	_ = session.Close()
	_ = stdin.Close()
	<-inputDone
	if ctx.Err() != nil {
		return 255, ctx.Err()
	}
	if reader.err != nil {
		return 255, fmt.Errorf("read SSH stdin: %w", reader.err)
	}
	// The server may finish before consuming all input. Only ordinary channel
	// closure is expected in that case; other copy failures remain visible.
	if inputError != nil && !errors.Is(inputError, io.EOF) && !errors.Is(inputError, io.ErrClosedPipe) && !errors.Is(inputError, os.ErrClosed) && !errors.Is(inputError, net.ErrClosed) && !errors.Is(inputError, syscall.EPIPE) {
		return 255, fmt.Errorf("write SSH stdin: %w", inputError)
	}
	if err == nil {
		return 0, nil
	}
	var exitError *gossh.ExitError
	if errors.As(err, &exitError) {
		status := exitError.ExitStatus()
		if status >= 0 && status <= 255 {
			return status, nil
		}
	}
	return 255, fmt.Errorf("run SSH command: %w", err)
}

// Capture source failures separately from writes rejected by an early remote
// exit. Closing our owned source after completion is not an input failure.
type inputReader struct {
	source   io.Reader
	stopping *atomic.Bool
	err      error
}

func (r *inputReader) Read(p []byte) (int, error) {
	n, err := r.source.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && !r.stopping.Load() {
		r.err = err
	}
	return n, err
}

func startKeepalive(client *gossh.Client, connection net.Conn) func() {
	done := make(chan struct{})
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		ticker := time.NewTicker(lockedKeepalive)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// Bound an unanswered request; SendRequest itself has no timeout.
				_ = connection.SetDeadline(time.Now().Add(3 * lockedKeepalive))
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					_ = client.Close()
					return
				}
				_ = connection.SetDeadline(time.Time{})
			}
		}
	}()
	return func() { close(done); _ = client.Close(); <-joined }
}
