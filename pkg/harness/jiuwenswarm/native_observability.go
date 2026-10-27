package jiuwenswarm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/harness/internal/privatefiles"
	"github.com/hyscale-lab/aries/pkg/performance"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const nativeSpanByteLimit = 64 << 20
const nativeSpanFileLimit = 16

var nativeSpanFilename = regexp.MustCompile(`^traces-\d{4}-\d{2}-\d{2}\.jsonl$`)
var nativeToolIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]{0,127}$`)

func nativeSpansEnabled(level performance.ObservationLevel) bool {
	return level == performance.ObservationL1 || level == performance.ObservationL2
}

type nativeSpanFile struct {
	Name           string `json:"name"`
	Size           int64  `json:"size"`
	RetainedBytes  int    `json:"retained_bytes"`
	SourceSHA256   string `json:"source_sha256,omitempty"`
	RetainedSHA256 string `json:"retained_sha256,omitempty"`
	Error          string `json:"error,omitempty"`
}

// This index contains identities and original Unix timestamps, never prompt,
// argument or result attributes. The source OTLP lines remain private evidence.
type nativeSpanIdentity struct {
	File              string                                 `json:"file"`
	Line              int                                    `json:"line"`
	TraceID           string                                 `json:"trace_id"`
	SpanID            string                                 `json:"span_id"`
	ParentSpanID      string                                 `json:"parent_span_id,omitempty"`
	Start             string                                 `json:"start_time_unix_nano"`
	End               string                                 `json:"end_time_unix_nano"`
	SessionID         string                                 `json:"native_session_id,omitempty"`
	RequestID         string                                 `json:"native_request_id,omitempty"`
	ToolCallID        string                                 `json:"tool_call_id,omitempty"`
	AuthoritativeTool bool                                   `json:"authoritative_tool"`
	Root              bool                                   `json:"root"`
	Complete          bool                                   `json:"complete"`
	ForcedClose       bool                                   `json:"forced_close"`
	Matched           bool                                   `json:"matched_attempt"`
	StatusCode        int                                    `json:"native_status_code"`
	Operation         string                                 `json:"operation,omitempty"`
	ToolName          string                                 `json:"tool_name,omitempty"`
	Provider          *performance.JiuwenProviderObservation `json:"provider_observation,omitempty"`
}

type nativeSpanIndex struct {
	SchemaVersion    int                  `json:"schema_version"`
	ProviderExpected *bool                `json:"provider_observation_expected,omitempty"`
	ProcessExpected  *bool                `json:"process_observation_expected,omitempty"`
	Enabled          bool                 `json:"enabled"`
	CollectionStatus string               `json:"collection_status"`
	Format           string               `json:"format"`
	Clock            string               `json:"clock"`
	SessionID        string               `json:"native_session_id"`
	RequestID        string               `json:"native_request_id"`
	Coverage         string               `json:"coverage"`
	Flush            string               `json:"exporter_flush"`
	ProducerCoverage string               `json:"producer_coverage"`
	ProcessTiming    string               `json:"process_monotonic_timing"`
	Files            []nativeSpanFile     `json:"files"`
	Spans            []nativeSpanIdentity `json:"spans"`
	Records          int                  `json:"records_seen"`
	Invalid          int                  `json:"invalid_records"`
	Matched          int                  `json:"matched_spans"`
	Unassociated     int                  `json:"unassociated_spans"`
	Tools            int                  `json:"matched_authoritative_tool_spans"`
	CompleteRoots    int                  `json:"matched_complete_roots"`
	Truncated        bool                 `json:"truncated"`
	Error            string               `json:"error,omitempty"`
	NativeStop       nativeStopReceipt    `json:"native_stop"`
	ProducerAudit    *nativeProducerAudit `json:"producer_audit,omitempty"`
}

type nativeStopReceipt struct {
	Attempted    bool   `json:"attempted"`
	SignalSent   bool   `json:"signal_sent"`
	ExitObserved bool   `json:"exit_observed"`
	CleanExit    bool   `json:"clean_exit"`
	Error        string `json:"error,omitempty"`
}

func newNativeSpanIndex(session, request string) nativeSpanIndex {
	return nativeSpanIndex{SchemaVersion: 1, Format: "openjiuwen.single_span_otlp_jsonl.hex_ids.v1", Clock: "native_unix_nanoseconds_unaligned",
		SessionID: session, RequestID: request, Coverage: "partial", Flush: "unconfirmed_batch_processor_may_have_pending_spans",
		ProducerCoverage: "unknown_no_independent_denominator_or_export_failure_counter", ProcessTiming: "unsupported"}
}

func (e *nativeRuntime) collectNativeSpans(ctx context.Context) (paths []string, returnErr error) {
	index := newNativeSpanIndex(e.sessionID, e.requestPrefix+"-run")
	index.NativeStop = e.nativeStop
	index.Enabled = nativeSpansEnabled(e.options.ObservationLevel)
	index.ProviderExpected = &e.providerObservationExpected
	processExpected := index.Enabled && e.processObservationSupported && !e.providerObservationExpected
	index.ProcessExpected = &processExpected
	index.CollectionStatus = "not_enabled"
	if index.Enabled {
		index.CollectionStatus = "collected_partial"
	}
	defer func() {
		if returnErr != nil {
			index.Error = e.credentials.RedactErr(returnErr).Error()
			index.CollectionStatus = "failed"
		}
		raw, err := json.MarshalIndent(index, "", "  ")
		indexPath := filepath.Join(e.artifactDir, "native-spans-index.json")
		if err == nil {
			err = privatefiles.WriteArtifact(indexPath, append(raw, '\n'))
		}
		if err == nil {
			paths = append(paths, indexPath)
		}
		returnErr = errors.Join(returnErr, err)
	}()
	if !index.Enabled {
		return nil, nil
	}
	if processExpected {
		processPaths, processErr := collectNativeProcessFiles(ctx, e.sandbox, path.Join(e.runtimeDir, "home/data/aries-process"), e.artifactDir)
		paths = append(paths, processPaths...)
		returnErr = errors.Join(returnErr, processErr)
		index.ProcessTiming = "separate_jiuwen_process_index"
	}
	downloader, ok := e.sandbox.(runner.LimitedDownloader)
	if !ok {
		return paths, errors.Join(returnErr, errors.New("Jiuwen native span collection requires bounded sandbox downloads"))
	}
	directory := path.Join(e.runtimeDir, "home/data/aries-traces")
	result, err := e.sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", nativeSpanInventoryScript, directory}, User: "root", Dir: "/", OutputLimitBytes: 16 << 10})
	var inventory struct {
		Files     []nativeSpanFile `json:"files"`
		Truncated bool             `json:"truncated"`
	}
	if err != nil || result.ExitCode != 0 || json.Unmarshal([]byte(result.Stdout), &inventory) != nil || len(inventory.Files) > nativeSpanFileLimit {
		return paths, errors.Join(returnErr, errors.New("Jiuwen native span inventory unavailable or invalid"))
	}
	index.Truncated = inventory.Truncated
	index.Files = inventory.Files
	if index.Truncated {
		returnErr = errors.Join(returnErr, errors.New("Jiuwen native span file count exceeded its bound"))
	}
	remaining := int64(nativeSpanByteLimit)
	seenFiles := map[string]bool{}
	for i := range index.Files {
		file := &index.Files[i]
		if !nativeSpanFilename.MatchString(file.Name) || seenFiles[file.Name] || file.Size <= 0 || file.Size > remaining {
			file.Error = "invalid_empty_or_over_budget_source"
			index.Truncated = index.Truncated || file.Size > remaining
			returnErr = errors.Join(returnErr, errors.New("Jiuwen native span source invalid, empty or over budget"))
			continue
		}
		seenFiles[file.Name] = true
		remaining -= file.Size
		content, err := e.downloadNativeSpan(ctx, downloader, path.Join(directory, file.Name), file.Size)
		if err != nil {
			file.Error = "bounded_download_failed"
			returnErr = errors.Join(returnErr, err)
			continue
		}
		file.SourceSHA256 = fmt.Sprintf("%x", sha256.Sum256(content))
		retained := e.credentials.Redact(content)
		if !bytes.Equal(content, retained) {
			file.Error = "credential_redacted"
			returnErr = errors.Join(returnErr, errors.New("Jiuwen native span artifact contained a credential value"))
		}
		clear(content)
		file.RetainedBytes, file.RetainedSHA256 = len(retained), fmt.Sprintf("%x", sha256.Sum256(retained))
		artifact := filepath.Join(e.artifactDir, "native-spans", file.Name)
		if err := privatefiles.WriteArtifact(artifact, retained); err != nil {
			file.Error = "private_artifact_write_failed"
			returnErr = errors.Join(returnErr, err)
		} else {
			paths = append(paths, artifact)
		}
		index.read(file.Name, retained)
		clear(retained)
	}
	index.associate()
	auditPaths, auditErr := e.collectNativeProducer(ctx, downloader, &index)
	paths = append(paths, auditPaths...)
	returnErr = errors.Join(returnErr, auditErr)
	if len(index.Spans) == 0 || index.Matched == 0 || index.Invalid > 0 {
		returnErr = errors.Join(returnErr, errors.New("Jiuwen native span evidence is empty, invalid or has no exact attempt identity"))
	}
	return paths, returnErr
}

func (e *nativeRuntime) downloadNativeSpan(ctx context.Context, downloader runner.LimitedDownloader, source string, size int64) ([]byte, error) {
	temporary, err := os.CreateTemp(e.artifactDir, ".native-span-download-")
	if err != nil {
		return nil, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	if err := downloader.DownloadLimit(ctx, source, name, size); err != nil {
		return nil, errors.New("Jiuwen native span bounded download failed")
	}
	content, err := privatefiles.Read(name, 0o600)
	if err != nil || int64(len(content)) != size {
		return nil, errors.New("Jiuwen native span download size or private file integrity differs")
	}
	return content, nil
}

func (index *nativeSpanIndex) read(file string, content []byte) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Buffer(make([]byte, 4096), nativeSpanByteLimit+1)
	line := 0
	for scanner.Scan() {
		line++
		index.Records++
		var request struct {
			ResourceSpans []struct {
				ScopeSpans []struct {
					Spans []struct {
						TraceID string `json:"traceId"`
						SpanID  string `json:"spanId"`
						Parent  string `json:"parentSpanId"`
						Start   string `json:"startTimeUnixNano"`
						End     string `json:"endTimeUnixNano"`
						Status  struct {
							Code int `json:"code"`
						} `json:"status"`
						Attributes []struct {
							Key   string `json:"key"`
							Value struct {
								String string `json:"stringValue"`
								Bool   bool   `json:"boolValue"`
							} `json:"value"`
						} `json:"attributes"`
					} `json:"spans"`
				} `json:"scopeSpans"`
			} `json:"resourceSpans"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil || len(request.ResourceSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans) != 1 || len(request.ResourceSpans[0].ScopeSpans[0].Spans) != 1 {
			index.Invalid++
			continue
		}
		span := request.ResourceSpans[0].ScopeSpans[0].Spans[0]
		start, startErr := strconv.ParseUint(span.Start, 10, 63)
		end, endErr := strconv.ParseUint(span.End, 10, 63)
		if !nativeHexID(span.TraceID, 32) || !nativeHexID(span.SpanID, 16) || (span.Parent != "" && !nativeHexID(span.Parent, 16)) || startErr != nil || endErr != nil || start == 0 || end < start || span.Status.Code < 0 || span.Status.Code > 2 {
			index.Invalid++
			continue
		}
		identity := nativeSpanIdentity{File: file, Line: line, TraceID: span.TraceID, SpanID: span.SpanID, ParentSpanID: span.Parent, Start: span.Start, End: span.End, StatusCode: span.Status.Code}
		for _, attr := range span.Attributes {
			switch attr.Key {
			case "openjiuwen.session.id":
				identity.SessionID = attr.Value.String
			case "openjiuwen.request.id":
				identity.RequestID = attr.Value.String
			case "gen_ai.tool.call.id":
				identity.ToolCallID = attr.Value.String
			case "gen_ai.tool.name":
				if nativeToolIdentifier.MatchString(attr.Value.String) {
					identity.ToolName = attr.Value.String
				}
			case "aries.provider.observation":
				var observation performance.JiuwenProviderObservation
				if json.Unmarshal([]byte(attr.Value.String), &observation) != nil || !observation.Valid() || identity.Provider != nil {
					index.Invalid++
				} else {
					identity.Provider = &observation
				}
			case "gen_ai.operation.name":
				if attr.Value.String == "execute_tool" || attr.Value.String == "chat" || attr.Value.String == "invoke_agent" || attr.Value.String == "aries_provider_request" {
					identity.Operation = attr.Value.String
				}
			case "openjiuwen.tool.authoritative":
				identity.AuthoritativeTool = attr.Value.Bool
			case "openjiuwen.trace.root":
				identity.Root = attr.Value.Bool
			case "openjiuwen.trace.complete":
				identity.Complete = attr.Value.Bool
			case "openjiuwen.trace.forced_close":
				identity.ForcedClose = attr.Value.Bool
			}
		}
		if (identity.Operation == "aries_provider_request") != (identity.Provider != nil) {
			index.Invalid++
		}
		index.Spans = append(index.Spans, identity)
	}
	if scanner.Err() != nil {
		index.Invalid++
	}
	if len(content) > 0 && content[len(content)-1] != '\n' {
		index.Truncated = true
		index.Invalid++
	}
}

func nativeHexID(value string, length int) bool {
	_, err := hex.DecodeString(value)
	return err == nil && len(value) == length && value == strings.ToLower(value) && strings.Trim(value, "0") != ""
}

func (index *nativeSpanIndex) associate() {
	byID := map[string]int{}
	for i, span := range index.Spans {
		key := span.TraceID + ":" + span.SpanID
		if _, exists := byID[key]; exists {
			index.Invalid++
		}
		byID[key] = i
	}
	for i := range index.Spans {
		span := &index.Spans[i]
		current := *span
		for steps := 0; steps <= len(index.Spans); steps++ {
			if current.SessionID != "" && current.SessionID != index.SessionID || current.RequestID != "" && current.RequestID != index.RequestID {
				break
			}
			if current.SessionID == index.SessionID && current.RequestID == index.RequestID {
				span.Matched = true
				break
			}
			parent, ok := byID[current.TraceID+":"+current.ParentSpanID]
			if !ok {
				break
			}
			current = index.Spans[parent]
		}
		if !span.Matched {
			index.Unassociated++
			continue
		}
		index.Matched++
		if span.Root && span.Complete {
			index.CompleteRoots++
		}
		if span.AuthoritativeTool {
			if span.ToolCallID == "" {
				index.Invalid++
			} else {
				index.Tools++
			}
		}
	}
}

const nativeSpanInventoryScript = `import json,os,pathlib,re,stat,sys
root=pathlib.Path(sys.argv[1])
files=[]
truncated=False
if root.exists():
 assert root.is_dir() and root.resolve()==root
 with os.scandir(root) as entries:
  for entry in entries:
   if not re.fullmatch(r'traces-\d{4}-\d{2}-\d{2}\.jsonl',entry.name): continue
   if len(files)==16: truncated=True; break
   info=entry.stat(follow_symlinks=False)
   assert stat.S_ISREG(info.st_mode)
   files.append({'name':entry.name,'size':info.st_size})
print(json.dumps({'files':sorted(files,key=lambda f:f['name']),'truncated':truncated}))
`
