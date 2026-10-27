package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Delivery is a transport receipt. Even a full success still needs a backend
// query; timeouts do not establish whether the receiver stored the request.
type delivery struct {
	Status        string          `json:"status"`
	SnapshotID    string          `json:"snapshot_id"`
	ExportSHA256  string          `json:"export_file_sha256"`
	RequestSHA256 string          `json:"request_sha256"`
	HTTPStatus    int             `json:"http_status,omitempty"`
	Attempts      int             `json:"http_attempts"`
	Response      json.RawMessage `json:"response,omitempty"`
	QueryVerified bool            `json:"query_verified"`
}

func sendExport(args []string, metrics bool, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "usage: aries-perf <--otlp-send | --metrics-send> <export.json> <http(s)://signal-endpoint>")
		return 2
	}
	content, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	var export struct {
		Request  json.RawMessage `json:"request"`
		Manifest struct {
			SnapshotID string `json:"snapshot_id"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(content, &export); err != nil || export.Manifest.SnapshotID == "" || len(export.Request) == 0 {
		fmt.Fprintln(stderr, "invalid OTLP export envelope")
		return 1
	}
	var request struct {
		ResourceSpans   []json.RawMessage `json:"resourceSpans"`
		ResourceMetrics []json.RawMessage `json:"resourceMetrics"`
	}
	if err := json.Unmarshal(export.Request, &request); err != nil || (metrics && request.ResourceMetrics == nil) || (!metrics && request.ResourceSpans == nil) {
		fmt.Fprintln(stderr, "invalid OTLP request for selected signal")
		return 1
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result, err := deliver(client, args[1], export.Request, metrics)
	result.SnapshotID, result.ExportSHA256 = export.Manifest.SnapshotID, digest(content)
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if writeErr := encoder.Encode(result); writeErr != nil {
		fmt.Fprintln(stderr, writeErr)
		return 1
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func deliver(client *http.Client, endpoint string, payload []byte, metrics bool) (delivery, error) {
	result := delivery{Status: "not_sent", RequestSHA256: digest(payload)}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return result, errors.New("OTLP endpoint must be an http(s) URL without userinfo, query or fragment")
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, errors.New("invalid OTLP endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	result.Attempts = 1
	response, err := client.Do(req)
	if err != nil {
		result.Status = "delivery_unknown"
		return result, errors.New("OTLP transport failed; delivery is unknown; no automatic retry")
	}
	defer response.Body.Close()
	result.HTTPStatus = response.StatusCode
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		result.Status = "delivery_unknown"
		return result, errors.New("OTLP response could not be read completely")
	}
	if json.Valid(body) {
		result.Response = body
	}
	if response.StatusCode != http.StatusOK {
		result.Status = "http_error"
		return result, fmt.Errorf("OTLP HTTP status %d; no automatic retry", response.StatusCode)
	}
	// Prometheus' native OTLP receiver returns an empty body on full success.
	if metrics && len(bytes.TrimSpace(body)) == 0 {
		result.Status = "accepted_unverified"
		return result, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		result.Status = "delivery_unknown"
		return result, errors.New("invalid OTLP success response")
	}
	partial, exists := fields["partialSuccess"]
	if !exists {
		partial, exists = fields["partial_success"]
	}
	if exists && !bytes.Equal(bytes.TrimSpace(partial), []byte("null")) {
		var detail struct {
			Rejected            json.Number `json:"rejectedSpans"`
			RejectedSnake       json.Number `json:"rejected_spans"`
			RejectedPoints      json.Number `json:"rejectedDataPoints"`
			RejectedPointsSnake json.Number `json:"rejected_data_points"`
			Warning             string      `json:"errorMessage"`
			WarningSnake        string      `json:"error_message"`
		}
		if err := json.Unmarshal(partial, &detail); err != nil {
			result.Status = "delivery_unknown"
			return result, errors.New("invalid OTLP partial success response")
		}
		if metrics {
			detail.Rejected, detail.RejectedSnake = detail.RejectedPoints, detail.RejectedPointsSnake
		}
		// Empty/default partial_success is a full success under OTLP. Warnings,
		// even with zero rejected spans, are preserved and must never be retried.
		if (detail.Rejected != "" && detail.Rejected != "0") || (detail.RejectedSnake != "" && detail.RejectedSnake != "0") || detail.Warning != "" || detail.WarningSnake != "" {
			result.Status = "partial_success"
			return result, errors.New("OTLP partial success; inspect response and query backend; no retry")
		}
	}
	result.Status = "accepted_unverified"
	return result, nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
