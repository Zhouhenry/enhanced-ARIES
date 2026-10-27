package openclaw

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/hyscale-lab/aries/pkg/core"
	"github.com/hyscale-lab/aries/pkg/deployment"
	"github.com/hyscale-lab/aries/pkg/harness"
	"github.com/hyscale-lab/aries/pkg/runner"
)

const producerIdentity = `{"schema_version":1,"scope_id":"run-1/fix-git","agent_session_id":"agent:main:aries-workload","tool_call_id":"tool-1","native_invocation_id":"invocation-1"}`
const producerStatus = `{"schema_version":1,"scope_id":"run-1/fix-git","agent_session_id":"agent:main:aries-workload","producer_instance_id":"producer-1","attempted":1,"written":1,"write_failed":0,"status_write_failed":0,"failed_latched":false}`

func nativeScope() *NativeIdentityScope {
	return &NativeIdentityScope{ScopeID: "run-1/fix-git", AgentSessionID: "agent:main:aries-workload"}
}

func TestNativeIdentityScopeMountMatchesOccurrenceAndRetainsFailedStopOwnership(t *testing.T) {
	fake := newFakeDeployment()
	manager := newTestManager(t, fake, []byte("model-secret"))
	manager.options.NativeIdentity = nativeScope()
	manager.options.WorkloadInputs = &WorkloadInputs{SessionKey: nativeScope().AgentSessionID}
	request := core.HarnessRequest{RunID: "run-1", TaskID: "fix-git", Endpoint: endpointFiles(t), Model: testModel(), Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "test-network"}}}
	if err := manager.Start(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(fake.created.Mounts) != 1 {
		t.Fatalf("mounts=%v", fake.created.Mounts)
	}
	mount := fake.created.Mounts[0]
	if mount.Target != nativeIdentityScopePath || !mount.ReadOnly || len(fake.validatedRequest.Mounts) != 1 || fake.validatedRequest.Mounts[0] != mount {
		t.Fatal("read-only scope bind was not declared and validated")
	}
	info, err := os.Stat(mount.Source)
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatalf("scope mode: %v", err)
	}
	data, err := os.ReadFile(mount.Source)
	if err != nil {
		t.Fatal(err)
	}
	var scope map[string]string
	if err := json.Unmarshal(data, &scope); err != nil || len(scope) != 2 || scope["scope_id"] != request.RunID+"/"+request.TaskID || scope["agent_session_id"] != nativeScope().AgentSessionID {
		t.Fatalf("scope=%v err=%v", scope, err)
	}
	for _, env := range fake.created.Env {
		if strings.Contains(env, "run-1/fix-git") {
			t.Fatal("identity entered process environment")
		}
	}
	archive := tar.NewReader(bytes.NewReader(fake.archive))
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == strings.TrimPrefix(nativeIdentityScopePath, "/") {
			t.Fatal("archive overwrites read-only scope bind")
		}
	}
	fake.stopErr = errors.New("absence unconfirmed")
	if err := manager.Stop(context.Background()); err == nil || manager.active == nil || manager.active.ID == "" {
		t.Fatal("unconfirmed stop released runtime ownership")
	}
	if _, err := os.Stat(mount.Source); err != nil {
		t.Fatal("unconfirmed stop removed mounted source")
	}
	fake.stopErr = nil
	if err := manager.Stop(context.Background()); err != nil || manager.active != nil {
		t.Fatalf("confirmed stop failed: %v", err)
	}
}

func TestNativeIdentityRejectsWrongOccurrenceBeforeCreate(t *testing.T) {
	fake := newFakeDeployment()
	manager := newTestManager(t, fake, []byte("model-secret"))
	manager.options.NativeIdentity = nativeScope()
	err := manager.Start(context.Background(), core.HarnessRequest{RunID: "run-2", TaskID: "fix-git"})
	if err == nil || fake.createCalls != 0 {
		t.Fatal("foreign occurrence reached deployment")
	}
}

func TestNativeIdentityRequiresExplicitMatchingWorkloadSession(t *testing.T) {
	for _, sessionKey := range []string{"", "foreign", nativeScope().AgentSessionID} {
		opts := Options{Runtime: harness.RuntimeOptions{Deployment: newFakeDeployment(), Image: testOpenClawImage, OutputDir: t.TempDir()}, NativeIdentity: nativeScope()}
		if sessionKey != "" {
			opts.WorkloadInputs = &WorkloadInputs{SessionKey: sessionKey}
		}
		_, err := New(opts)
		if (err == nil) != (sessionKey == nativeScope().AgentSessionID) {
			t.Fatalf("session=%q err=%v", sessionKey, err)
		}
	}
}

type identityArchiveDeployment struct {
	*fakeDeployment
	content   []byte
	sourceErr error
	mode      int64
	status    []byte
	statusErr error
}

func (d *identityArchiveDeployment) DownloadArchive(ctx context.Context, id, path string) (io.ReadCloser, deployment.FileInfo, error) {
	if path != nativeIdentityProducerPath && path != nativeIdentityStatusPath {
		return d.fakeDeployment.DownloadArchive(ctx, id, path)
	}
	content, mode, sourceErr := d.content, d.mode, d.sourceErr
	if path == nativeIdentityStatusPath {
		content, mode, sourceErr = d.status, 0600, d.statusErr
		if content == nil {
			content = []byte(producerStatus)
		}
	}
	if sourceErr != nil {
		return nil, deployment.FileInfo{}, sourceErr
	}
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	_ = writer.WriteHeader(&tar.Header{Name: filepath.Base(path), Mode: mode, Size: int64(len(content))})
	_, _ = writer.Write(content)
	_ = writer.Close()
	return io.NopCloser(bytes.NewReader(archive.Bytes())), deployment.FileInfo{}, nil
}

func TestNativeIdentityCollectorNeverInventsCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		sourceErr     error
		mode          int64
		valid         bool
	}{
		{"valid", producerIdentity + "\n", nil, 0600, true},
		{"missing", "", runner.ErrNotFound, 0600, false},
		{"empty", "", nil, 0600, false},
		{"partial-line", producerIdentity, nil, 0600, false},
		{"duplicate-invocation", producerIdentity + "\n" + producerIdentity + "\n", nil, 0600, false},
		{"foreign-scope", strings.Replace(producerIdentity, "run-1/fix-git", "foreign", 1) + "\n", nil, 0600, false},
		{"duplicate-json-key", strings.Replace(producerIdentity, `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1) + "\n", nil, 0600, false},
		{"ssh-id-forbidden", strings.TrimSuffix(producerIdentity, "}") + `,"ssh_request_id":""}` + "\n", nil, 0600, false},
		{"extra-sequence", strings.TrimSuffix(producerIdentity, "}") + `,"sequence":1}` + "\n", nil, 0600, false},
		{"public-file", producerIdentity + "\n", nil, 0644, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDeployment()
			manager := newTestManager(t, fake, []byte("model-secret"))
			manager.options.NativeIdentity = nativeScope()
			manager.runtime.Options.Deployment = &identityArchiveDeployment{fakeDeployment: fake, content: []byte(tc.content), sourceErr: tc.sourceErr, mode: tc.mode}
			dir := t.TempDir()
			active := &session{Occurrence: &harness.Occurrence{ID: "owned", ArtifactDir: dir, Credentials: harness.NewCredentials("test"), Artifacts: &harness.Artifacts{Directory: dir}}}
			err := manager.collectNativeIdentities(context.Background(), active)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			data, readErr := os.ReadFile(filepath.Join(dir, "native-tool-identities.index.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var index nativeIdentityIndex
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
			if (index.FileIntegrity == "valid") != tc.valid || index.ExpectedNativeCalls != nil || index.Coverage != "producer_only_independent_denominator_required" {
				t.Fatalf("false coverage claim: %+v", index)
			}
			if tc.valid && index.Events != 1 {
				t.Fatal("producer count missing")
			}
		})
	}
}

func TestNativeIdentityDisabledDoesNotCollectProducer(t *testing.T) {
	fake := newFakeDeployment()
	manager := newTestManager(t, fake, []byte("model-secret"))
	manager.runtime.Options.Deployment = &identityArchiveDeployment{fakeDeployment: fake, sourceErr: errors.New("must not read")}
	if err := manager.collectNativeIdentities(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if report, err := manager.NativeIdentityObservation(); err != nil || report.Status != core.StatusNotEnabled {
		t.Fatalf("disabled observer: %+v %v", report, err)
	}
}

func TestNativeIdentityCollectorCountsInvocationAndRejectionRecords(t *testing.T) {
	invocation := strings.TrimSuffix(strings.Replace(producerIdentity, `"schema_version":1`, `"schema_version":2`, 1), "}") + `,"phase":"invocation","outcome":"started","dispatched":null,"reason":""}`
	rejection := strings.TrimSuffix(strings.Replace(producerIdentity, `"schema_version":1`, `"schema_version":2`, 1), "}") + `,"phase":"pre_dispatch","outcome":"rejected","dispatched":false,"reason":"shell_preflight_rejected"}`
	fake := newFakeDeployment()
	manager := newTestManager(t, fake, []byte("model-secret"))
	manager.options.NativeIdentity = nativeScope()
	health := strings.Replace(strings.Replace(producerStatus, `"attempted":1`, `"attempted":2`, 1), `"written":1`, `"written":2`, 1)
	manager.runtime.Options.Deployment = &identityArchiveDeployment{fakeDeployment: fake, content: []byte(invocation + "\n" + rejection + "\n"), mode: 0600, status: []byte(health)}
	dir := t.TempDir()
	active := &session{Occurrence: &harness.Occurrence{ID: "owned", ArtifactDir: dir, Credentials: harness.NewCredentials("test"), Artifacts: &harness.Artifacts{Directory: dir}}}
	if err := manager.collectNativeIdentities(context.Background(), active); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "native-tool-identities.index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index nativeIdentityIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	if index.Events != 2 || index.FileIntegrity != "valid" || index.ExpectedNativeCalls != nil || index.ProducerHealth.Written != 2 {
		t.Fatalf("incorrect record denominator: %+v", index)
	}
}

func TestNativeProducerHealthRejectsMissingFailedAndMismatchedEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		sourceErr    error
	}{
		{"missing", producerStatus, runner.ErrNotFound},
		{"empty", "", nil},
		{"failed-latch", strings.Replace(producerStatus, `"failed_latched":false`, `"failed_latched":true`, 1), nil},
		{"lost-pair", strings.Replace(producerStatus, `"attempted":1`, `"attempted":2`, 1), nil},
		{"write-failed", strings.Replace(strings.Replace(producerStatus, `"attempted":1`, `"attempted":2`, 1), `"write_failed":0`, `"write_failed":1`, 1), nil},
		{"status-write-failed", strings.Replace(producerStatus, `"status_write_failed":0`, `"status_write_failed":1`, 1), nil},
		{"wrong-count", strings.Replace(strings.Replace(producerStatus, `"attempted":1`, `"attempted":2`, 1), `"written":1`, `"written":2`, 1), nil},
		{"foreign", strings.Replace(producerStatus, "run-1/fix-git", "foreign", 1), nil},
		{"missing-field", strings.Replace(producerStatus, `,"failed_latched":false`, "", 1), nil},
		{"duplicate-field", strings.Replace(producerStatus, `"attempted":1`, `"attempted":1,"attempted":1`, 1), nil},
		{"null", strings.Replace(producerStatus, `"attempted":1`, `"attempted":null`, 1), nil},
		{"negative", strings.Replace(producerStatus, `"attempted":1`, `"attempted":-1`, 1), nil},
		{"unsafe-integer", strings.Replace(producerStatus, `"attempted":1`, `"attempted":9007199254740992`, 1), nil},
		{"trailing", producerStatus + "{}", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeDeployment()
			manager := newTestManager(t, fake, []byte("model-secret"))
			manager.options.NativeIdentity = nativeScope()
			manager.runtime.Options.Deployment = &identityArchiveDeployment{fakeDeployment: fake, content: []byte(producerIdentity + "\n"), mode: 0600, status: []byte(tc.status), statusErr: tc.sourceErr}
			dir := t.TempDir()
			active := &session{Occurrence: &harness.Occurrence{ID: "owned", ArtifactDir: dir, Credentials: harness.NewCredentials("test"), Artifacts: &harness.Artifacts{Directory: dir}}}
			if err := manager.collectNativeIdentities(context.Background(), active); err == nil {
				t.Fatal("invalid health passed")
			}
			report, err := manager.NativeIdentityObservation()
			if err == nil || report.Status != core.StatusFailed {
				t.Fatalf("lost independent failure: %+v %v", report, err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "native-tool-identities.index.json"))
			if err != nil {
				t.Fatal(err)
			}
			var index nativeIdentityIndex
			if err := json.Unmarshal(data, &index); err != nil {
				t.Fatal(err)
			}
			if index.FileIntegrity != "incomplete" || index.ExpectedNativeCalls != nil {
				t.Fatalf("false complete: %+v", index)
			}
		})
	}
}

func TestNativeObservationFailurePreservesRunAndStop(t *testing.T) {
	for _, name := range []string{"missing", "source-enospc", "producer-write-failed", "producer-over-limit", "artifact-write-failed", "index-write-failed"} {
		t.Run(name, func(t *testing.T) {
			fake := newFakeDeployment()
			manager := newTestManager(t, fake, []byte("model-secret"))
			manager.options.NativeIdentity = nativeScope()
			manager.options.WorkloadInputs = &WorkloadInputs{SessionKey: nativeScope().AgentSessionID}
			request := core.HarnessRequest{RunID: "run-1", TaskID: "fix-git", Endpoint: endpointFiles(t), Model: testModel(), Connectivity: core.HarnessConnectivity{Placement: core.RuntimePlacement{AttachmentID: "test-network"}}}
			if err := manager.Start(context.Background(), request); err != nil {
				t.Fatal(err)
			}
			d := &identityArchiveDeployment{fakeDeployment: fake, content: []byte(producerIdentity + "\n"), mode: 0600}
			switch name {
			case "missing":
				d.sourceErr = runner.ErrNotFound
			case "source-enospc":
				d.sourceErr = syscall.ENOSPC
			case "producer-write-failed":
				d.status = []byte(strings.Replace(producerStatus, `"failed_latched":false`, `"failed_latched":true`, 1))
			case "producer-over-limit":
				d.content = make([]byte, maxNativeIdentityBytes+1)
			case "artifact-write-failed":
				if err := os.Mkdir(filepath.Join(manager.active.ArtifactDir, "native-tool-identities.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			case "index-write-failed":
				if err := os.Mkdir(filepath.Join(manager.active.ArtifactDir, "native-tool-identities.index.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			manager.runtime.Options.Deployment = d
			result, err := manager.Run(context.Background(), "repair git")
			if err != nil || result.Status != core.StatusSucceeded || result.FinalResponse != "task complete" {
				t.Fatalf("observer changed Agent: %+v %v", result, err)
			}
			report, err := manager.NativeIdentityObservation()
			if err == nil || report.Status != core.StatusFailed {
				t.Fatalf("observer falsely succeeded: %+v %v", report, err)
			}
			fake.stopErr = errors.New("absence unconfirmed")
			if err := manager.Stop(context.Background()); err == nil || manager.active == nil {
				t.Fatal("observer bypassed isolation failure")
			}
			fake.stopErr = nil
			if err := manager.Stop(context.Background()); err != nil || manager.active != nil {
				t.Fatalf("observer contaminated Stop: %v", err)
			}
			if report, err := manager.NativeIdentityObservation(); err == nil || report.Status != core.StatusFailed {
				t.Fatal("Stop erased observer failure")
			}
		})
	}
}
