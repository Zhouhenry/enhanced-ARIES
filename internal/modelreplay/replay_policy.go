package modelreplay

import (
	"maps"
	"slices"
	"strconv"
)

const (
	ReplayPolicyStrict   = "strict"
	ReplayPolicyWorkload = "workload"
)

// Both summaries hash the UTF-8 text itself, not its JSON string encoding.
// Path is a JSON pointer into the complete request, including older history.
type toolContentDifference struct {
	SchemaVersion int                   `json:"schema_version"`
	Path          string                `json:"path"`
	Encoding      string                `json:"encoding"`
	Recorded      requestTextDiagnostic `json:"recorded"`
	Actual        requestTextDiagnostic `json:"actual"`
}

// Workload replay fixes the recorded decisions but permits textual tool results
// to differ. Compare a private shallow copy of the expected request: never edit
// the received request, the Tape, or anything delivered to the Agent.
func replayRequestDifference(expected, actual any, policy string) (string, []toolContentDifference) {
	if policy != ReplayPolicyWorkload {
		return difference(expected, actual, ""), nil
	}
	want, wantOK := expected.(map[string]any)
	got, gotOK := actual.(map[string]any)
	if !wantOK || !gotOK {
		return difference(expected, actual, ""), nil
	}
	wantMessages, wantOK := want["messages"].([]any)
	gotMessages, gotOK := got["messages"].([]any)
	if !wantOK || !gotOK || len(wantMessages) != len(gotMessages) {
		return difference(expected, actual, ""), nil
	}
	compared := maps.Clone(want)
	messages := slices.Clone(wantMessages)
	compared["messages"] = messages
	var allowed []toolContentDifference
	for i := range wantMessages {
		wantMessage, wantOK := wantMessages[i].(map[string]any)
		gotMessage, gotOK := gotMessages[i].(map[string]any)
		if !wantOK || !gotOK || wantMessage["role"] != "tool" || gotMessage["role"] != "tool" {
			continue
		}
		wantText, wantOK := wantMessage["content"].(string)
		gotText, gotOK := gotMessage["content"].(string)
		if !wantOK || !gotOK || wantText == gotText {
			continue
		}
		message := maps.Clone(wantMessage)
		message["content"] = gotText
		messages[i] = message
		allowed = append(allowed, toolContentDifference{SchemaVersion: 1,
			Path: "/messages/" + strconv.Itoa(i) + "/content", Encoding: "utf-8",
			Recorded: textDiagnostic(wantText), Actual: textDiagnostic(gotText)})
	}
	if path := difference(compared, actual, ""); path != "" {
		return path, nil
	}
	return "", allowed
}
