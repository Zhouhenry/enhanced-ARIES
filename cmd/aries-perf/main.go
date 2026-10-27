// aries-perf derives a performance report from a single run's saved evidence.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/performance"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--otlp-send" || args[0] == "--metrics-send") {
		return sendExport(args[1:], args[0] == "--metrics-send", stdout, stderr)
	}
	mode, domain, resourceStream := "summary", "", ""
	if len(args) >= 2 && args[0] == "--resource-metrics" {
		mode, resourceStream = args[0], args[1]
		args = args[2:]
	}
	if mode == "summary" && len(args) > 0 && (args[0] == "--trace" || args[0] == "--snapshot" || args[0] == "--otlp" || args[0] == "--metrics") {
		mode = args[0]
		args = args[1:]
	}
	if mode == "summary" && len(args) >= 2 && args[0] == "--trace-clock" {
		mode, domain = "--trace", args[1]
		args = args[2:]
		if domain == "" {
			args = nil
		}
	}
	fromSnapshot := len(args) > 0 && args[0] == "--from-snapshot" && (mode == "--trace" || mode == "--otlp" || mode == "--metrics" || mode == "--resource-metrics")
	if fromSnapshot {
		args = args[1:]
	}
	if len(args) != 1 || args[0] == "" || args[0][0] == '-' || ((mode == "--otlp" || mode == "--metrics" || mode == "--resource-metrics") && !fromSnapshot) || (mode == "--resource-metrics" && resourceStream == "") {
		fmt.Fprintln(stderr, "usage: aries-perf [--trace | --snapshot | --trace-clock <clock-domain>] <run-root-or-performance-directory>\n       aries-perf <--trace | --trace-clock <clock-domain> | --otlp | --metrics> --from-snapshot <snapshot.json>\n       aries-perf --otlp-send <export.json> <http(s)://endpoint/v1/traces>\n       aries-perf --metrics-send <export.json> <http(s)://endpoint/api/v1/otlp/v1/metrics>")
		fmt.Fprintln(stderr, "       aries-perf --resource-metrics <stream-id> --from-snapshot <snapshot.json>")
		return 2
	}
	directory := filepath.Clean(args[0])
	if !fromSnapshot && filepath.Base(directory) != "performance" {
		if _, err := os.Stat(filepath.Join(directory, "manifest.json")); os.IsNotExist(err) {
			directory = filepath.Join(directory, "performance")
		}
	}
	var report any
	var err error
	if mode == "--trace" || mode == "--snapshot" || mode == "--otlp" || mode == "--metrics" || mode == "--resource-metrics" {
		var snapshot performance.AnalysisSnapshot
		if fromSnapshot {
			snapshot, err = performance.LoadSnapshot(directory)
			if err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else {
			snapshot, err = performance.BuildSnapshot(directory)
		}
		report = snapshot
		if mode == "--trace" {
			var projectionErr error
			report, projectionErr = performance.ProjectTraceDomain(snapshot, domain)
			if err == nil {
				err = projectionErr
			}
		}
		if mode == "--otlp" {
			report, err = performance.ProjectOTLP(snapshot)
		}
		if mode == "--metrics" {
			report, err = performance.ProjectMetrics(snapshot)
		}
		if mode == "--resource-metrics" {
			report, err = performance.ProjectResourceMetrics(snapshot, resourceStream)
		}
	} else {
		report, err = performance.Analyze(directory)
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if writeErr := encoder.Encode(report); writeErr != nil {
		fmt.Fprintf(stderr, "write performance report: %v\n", writeErr)
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
