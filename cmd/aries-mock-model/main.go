// aries-mock-model serves a deterministic fixture as an external model endpoint.
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

	"github.com/hyscale-lab/aries/internal/mockmodel"
	"github.com/sirupsen/logrus"
)

func main() {
	if err := run(); err != nil {
		logrus.WithError(err).Error("mock model stopped")
		os.Exit(1)
	}
}

func run() error {
	fixturePath := flag.String("fixture", "", "JSON workload fixture (required)")
	listen := flag.String("listen", "127.0.0.1:8000", "HTTP listen address; must be reachable by ARIES and its harness")
	printModel := flag.Bool("print-model", false, "print the fixture model ID and exit")
	flag.Parse()
	if *fixturePath == "" || flag.NArg() != 0 {
		return errors.New("usage: aries-mock-model --fixture FILE [--listen ADDRESS] [--print-model]")
	}
	data, err := os.ReadFile(*fixturePath)
	if err != nil {
		return err
	}
	model, err := mockmodel.New(data)
	if err != nil {
		return err
	}
	if *printModel {
		fmt.Println(model.ModelID())
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: model, ReadHeaderTimeout: 10 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
	}()
	logrus.WithFields(logrus.Fields{"listen": listener.Addr().String(), "model": model.ModelID(), "fixture_sha256": model.FixtureSHA256()}).Info("serving scripted mock model (no authentication)")
	err = server.Serve(listener)
	stop()
	<-stopped
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
