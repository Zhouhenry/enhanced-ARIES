package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestOTLPDeliveryPreservesPartialSuccessAndNeverRetries(t *testing.T) {
	for _, test := range []struct {
		name, body, status string
		code               int
		fail               bool
	}{
		{"success", `{}`, "accepted_unverified", 200, false},
		{"empty_partial", `{"partialSuccess":{}}`, "accepted_unverified", 200, false},
		{"rejected", `{"partialSuccess":{"rejectedSpans":"2","errorMessage":"invalid spans"}}`, "partial_success", 200, true},
		{"warning_only", `{"partialSuccess":{"rejectedSpans":"0","errorMessage":"warning"}}`, "partial_success", 200, true},
		{"snake_fields", `{"partial_success":{"rejected_spans":"1","error_message":"partial"}}`, "partial_success", 200, true},
		{"invalid_response", `not json`, "delivery_unknown", 200, true},
		{"retryable_http", `{"message":"unavailable"}`, "http_error", 503, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			payload := []byte(`{"resourceSpans":[]}`)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || !bytes.Equal(body, payload) {
					t.Error("changed OTLP request")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.code)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			result, err := deliver(server.Client(), server.URL+"/v1/traces", payload, false)
			if (err != nil) != test.fail || result.Status != test.status || result.Attempts != 1 || calls.Load() != 1 || result.QueryVerified || result.RequestSHA256 != digest(payload) {
				t.Fatalf("receipt=%+v error=%v calls=%d", result, err, calls.Load())
			}
			if test.body != "not json" && string(result.Response) != test.body {
				t.Fatal("receiver details lost")
			}
		})
	}
}

func TestOTLPTimeoutIsUnknownDelivery(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-release
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 25 * time.Millisecond
	result, err := deliver(client, server.URL+"/v1/traces", []byte(`{"resourceSpans":[]}`), false)
	close(release)
	if err == nil || result.Status != "delivery_unknown" || result.Attempts != 1 || calls.Load() != 1 {
		t.Fatalf("receipt=%+v err=%v calls=%d", result, err, calls.Load())
	}
}

func TestMetricsDeliveryResponseAndSignalSelection(t *testing.T) {
	for _, test := range []struct {
		name, body, status string
		metrics, fail      bool
	}{
		{"prometheus_empty_success", "", "accepted_unverified", true, false},
		{"trace_empty_response_unchanged", "", "delivery_unknown", false, true},
		{"metric_rejection", `{"partialSuccess":{"rejectedDataPoints":"2"}}`, "partial_success", true, true},
		{"metric_warning", `{"partial_success":{"rejected_data_points":"0","error_message":"warning"}}`, "partial_success", true, true},
		{"metric_default_partial", `{"partialSuccess":{"rejectedDataPoints":"0"}}`, "accepted_unverified", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			result, err := deliver(server.Client(), server.URL, []byte(`{"resourceMetrics":[]}`), test.metrics)
			if (err != nil) != test.fail || result.Status != test.status || calls.Load() != 1 || result.QueryVerified {
				t.Fatalf("receipt=%+v err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := os.WriteFile(path, []byte(`{"request":{"resourceMetrics":[]},"manifest":{"snapshot_id":"snapshot-test"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output, diagnostics bytes.Buffer
	if code := run([]string{"--otlp-send", path, server.URL}, &output, &diagnostics); code != 1 || calls.Load() != 0 {
		t.Fatal("metrics envelope sent as traces")
	}
	output.Reset()
	diagnostics.Reset()
	if code := run([]string{"--metrics-send", path, server.URL}, &output, &diagnostics); code != 0 || calls.Load() != 1 || !bytes.Contains(output.Bytes(), []byte(`"snapshot_id": "snapshot-test"`)) {
		t.Fatalf("metrics delivery exit=%d output=%s diagnostics=%s", code, output.String(), diagnostics.String())
	}
}
