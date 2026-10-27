package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"os"
	"runtime"

	"github.com/hyscale-lab/aries/internal/app"
	cloudwiring "github.com/hyscale-lab/aries/internal/app/wiring/cloud"
	"github.com/hyscale-lab/aries/internal/envfile"
)

func runCloudCommand(ctx context.Context, args []string, stdout io.Writer, dependencies app.Dependencies) error {
	if runtime.GOOS != "linux" {
		return errors.New("cloud workloads must run on the remote Linux ECS")
	}
	flags := flag.NewFlagSet("aries cloud", flag.ContinueOnError)
	envPath := flags.String("env-file", "", "private literal environment file on ECS; omit when Compose injects named settings")
	tape := flags.String("tape", "", "recorded tape.json overriding the replay profile's tape_file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: aries cloud [--env-file PATH] [--tape FILE] PROFILE.json")
	}
	source, err := envfile.Load(*envPath)
	if err != nil {
		return err
	}
	defer source.Close()
	lookup := func(name string) ([]byte, bool) {
		if value, found := source.Lookup(name); found {
			return value, true
		}
		value, found := os.LookupEnv(name)
		return []byte(value), found
	}
	return app.RunCloud(ctx, flags.Arg(0), stdout, dependencies, app.CloudWiring{Prepare: cloudwiring.Prepare}, lookup, *tape)
}
