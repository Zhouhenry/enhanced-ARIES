package jiuwenswarm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/runner"
)

// This denominator covers sampled SDK spans, not uninstrumented native work.
type nativeProducerAudit struct {
	Status     string           `json:"status"`
	Scope      string           `json:"scope"`
	Started    int              `json:"started_spans"`
	Ended      int              `json:"ended_spans"`
	Exported   int              `json:"exported_spans"`
	Missing    int              `json:"missing_exports"`
	Unexpected int              `json:"unexpected_exports"`
	Files      []nativeSpanFile `json:"files"`
	Error      string           `json:"error,omitempty"`
}

type nativeProducerStatus struct {
	Schema        int      `json:"schema_version"`
	Protocol      string   `json:"protocol"`
	RunID         string   `json:"run_id"`
	TaskID        string   `json:"task_id"`
	PID           int      `json:"pid"`
	Before        bool     `json:"registered_before_provider"`
	Attached      bool     `json:"attached"`
	Status        string   `json:"status"`
	Started       int      `json:"started_spans"`
	Ended         int      `json:"ended_spans"`
	Records       int      `json:"written_records"`
	Bytes         int      `json:"written_bytes"`
	WriteFailures int      `json:"write_failures"`
	Dropped       int      `json:"dropped_records"`
	LimitExceeded bool     `json:"limit_exceeded"`
	Flushes       int      `json:"force_flush_callbacks"`
	Shutdown      bool     `json:"processor_shutdown_observed"`
	StopStarted   bool     `json:"extension_shutdown_started"`
	StopReturned  bool     `json:"extension_shutdown_returned"`
	SHA           string   `json:"ledger_sha256"`
	Errors        []string `json:"errors"`
}

type nativeProducerEvent struct {
	Schema   int    `json:"schema_version"`
	Sequence int    `json:"sequence"`
	Event    string `json:"event"`
	Trace    string `json:"trace_id"`
	Span     string `json:"span_id"`
	Parent   string `json:"parent_span_id"`
	Start    string `json:"start_time_unix_nano"`
	End      string `json:"end_time_unix_nano"`
	Status   int    `json:"status_code"`
	Tool     string `json:"tool_call_id"`
}

func (audit *nativeProducerAudit) reconcile(status nativeProducerStatus, events []byte, spans []nativeSpanIdentity) error {
	audit.Started, audit.Ended, audit.Exported = status.Started, status.Ended, len(spans)
	if status.Schema != 1 || status.Protocol != "jiuwen-native-span-audit-v1" || !status.Before || !status.Attached || status.Status != "complete" || !status.Shutdown || !status.StopStarted || !status.StopReturned || status.WriteFailures != 0 || status.Dropped != 0 || status.LimitExceeded || len(status.Errors) != 0 || status.Bytes != len(events) || status.SHA != fmt.Sprintf("%x", sha256.Sum256(events)) {
		return errors.New("native producer lifecycle, write counters or ledger hash incomplete")
	}
	starts, ends := map[string]nativeProducerEvent{}, map[string]nativeProducerEvent{}
	scanner := bufio.NewScanner(bytes.NewReader(events))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	sequence, flushes, shutdown := 0, 0, false
	for scanner.Scan() {
		sequence++
		var row nativeProducerEvent
		if json.Unmarshal(scanner.Bytes(), &row) != nil || row.Schema != 1 || row.Sequence != sequence {
			return errors.New("native producer ledger has invalid or missing records")
		}
		key := row.Trace + ":" + row.Span
		switch row.Event {
		case "start", "end":
			if shutdown || !nativeHexID(row.Trace, 32) || !nativeHexID(row.Span, 16) || row.Parent != "" && !nativeHexID(row.Parent, 16) {
				return errors.New("native producer span identity or lifecycle invalid")
			}
			if row.Event == "start" {
				if _, exists := starts[key]; exists {
					return errors.New("native producer span started twice")
				}
				starts[key] = row
			} else {
				start, exists := starts[key]
				_, ended := ends[key]
				if !exists || ended || start.Start != row.Start || start.Parent != row.Parent {
					return errors.New("native producer span end has no unique matching start")
				}
				ends[key] = row
			}
		case "force_flush":
			flushes++
		case "processor_shutdown":
			if shutdown {
				return errors.New("native producer shutdown duplicated")
			}
			shutdown = true
		default:
			return errors.New("unknown native producer event")
		}
	}
	if scanner.Err() != nil || len(events) == 0 || events[len(events)-1] != '\n' || sequence != status.Records || flushes != status.Flushes || !shutdown || len(starts) != status.Started || len(ends) != status.Ended || len(starts) != len(ends) {
		return errors.New("native producer ledger counters or terminal spans incomplete")
	}
	exports := map[string]bool{}
	for _, span := range spans {
		key := span.TraceID + ":" + span.SpanID
		end, exists := ends[key]
		if exports[key] {
			return errors.New("native export duplicates a producer span")
		}
		exports[key] = true
		if !exists {
			audit.Unexpected++
			continue
		}
		if end.Start != span.Start || end.End != span.End || end.Parent != span.ParentSpanID || end.Status != span.StatusCode || end.Tool != span.ToolCallID {
			return errors.New("native exported span differs from producer identity or terminal fields")
		}
	}
	for key := range ends {
		if !exports[key] {
			audit.Missing++
		}
	}
	if audit.Missing != 0 || audit.Unexpected != 0 {
		return errors.New("native producer and export span sets differ")
	}
	audit.Status = "complete"
	return nil
}

func (e *nativeRuntime) collectNativeProducer(ctx context.Context, downloader runner.LimitedDownloader, index *nativeSpanIndex) (paths []string, returnErr error) {
	audit := &nativeProducerAudit{Status: "incomplete", Scope: "sampled_sdk_spans", Files: []nativeSpanFile{}}
	index.ProducerAudit = audit
	defer func() {
		if returnErr != nil {
			audit.Error = e.credentials.RedactErr(returnErr).Error()
			return
		}
		index.ProducerCoverage = "sampled_sdk_span_ids_reconciled"
		index.Flush = "ended_span_exports_reconciled_after_stop"
	}()
	directory := path.Join(e.runtimeDir, "home/data/aries-native-audit")
	result, err := e.sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", nativeProducerInventoryScript, directory}, User: "root", Dir: "/", OutputLimitBytes: 4096})
	var files []nativeSpanFile
	if err != nil || result.ExitCode != 0 || json.Unmarshal([]byte(result.Stdout), &files) != nil || len(files) != 2 {
		return nil, errors.New("native producer audit inventory unavailable")
	}
	var status nativeProducerStatus
	var ledger []byte
	for i, name := range []string{"status.json", "events.jsonl"} {
		file := files[i]
		limit := int64(nativeSpanByteLimit)
		if i == 0 {
			limit = 64 << 10
		}
		if file.Name != name || file.Size <= 0 || file.Size > limit {
			return paths, errors.New("native producer source invalid or over budget")
		}
		content, err := e.downloadNativeSpan(ctx, downloader, path.Join(directory, name), file.Size)
		if err != nil {
			return paths, err
		}
		retained := e.credentials.Redact(content)
		if !bytes.Equal(content, retained) {
			return paths, errors.New("native producer audit contained a credential value")
		}
		file.SourceSHA256, file.RetainedSHA256 = fmt.Sprintf("%x", sha256.Sum256(content)), fmt.Sprintf("%x", sha256.Sum256(retained))
		file.RetainedBytes = len(retained)
		artifact := filepath.Join(e.artifactDir, "native-producer", name)
		if err := privatefiles.WriteArtifact(artifact, retained); err != nil {
			return paths, err
		}
		paths = append(paths, artifact)
		audit.Files = append(audit.Files, file)
		if i == 0 {
			if json.Unmarshal(content, &status) != nil {
				return paths, errors.New("native producer status invalid")
			}
		} else {
			ledger = content
		}
	}
	if status.RunID != e.runID || status.TaskID != e.taskID || status.PID != e.nativePID || !index.NativeStop.ExitObserved {
		return paths, errors.New("native producer binding or process exit unconfirmed")
	}
	return paths, audit.reconcile(status, ledger, index.Spans)
}

const nativeProducerInventoryScript = `import json,pathlib,stat,sys
root=pathlib.Path(sys.argv[1]); assert root.is_dir() and root.resolve()==root
files=[]
for name in ('status.json','events.jsonl'):
 p=root/name; info=p.lstat(); assert stat.S_ISREG(info.st_mode)
 files.append({'name':name,'size':info.st_size})
print(json.dumps(files))
`
