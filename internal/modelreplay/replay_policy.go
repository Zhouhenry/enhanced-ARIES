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

// Workload fixes recorded decisions while allowing tool text and the declared
// Jiuwen native context fields to differ. Compare a private copy: never edit the
// received request, Tape, or anything delivered to the Agent.
func replayRequestDifference(expected, actual any, policy, historyDialect string) (string, []toolContentDifference, []promptContextDifference) {
	if policy != ReplayPolicyWorkload {
		return difference(expected, actual, ""), nil, nil
	}
	want, wantOK := expected.(map[string]any)
	got, gotOK := actual.(map[string]any)
	if !wantOK || !gotOK {
		return difference(expected, actual, ""), nil, nil
	}
	wantMessages, wantOK := want["messages"].([]any)
	gotMessages, gotOK := got["messages"].([]any)
	if !wantOK || !gotOK || len(wantMessages) != len(gotMessages) {
		return difference(expected, actual, ""), nil, nil
	}
	compared := maps.Clone(want)
	messages := slices.Clone(wantMessages)
	compared["messages"] = messages
	var allowed []toolContentDifference
	var promptAllowed []promptContextDifference
	firstUser := -1
	for i := range wantMessages {
		wantMessage, wantOK := wantMessages[i].(map[string]any)
		gotMessage, gotOK := gotMessages[i].(map[string]any)
		if wantOK && wantMessage["role"] == "user" && firstUser < 0 {
			firstUser = i
		}
		if !wantOK || !gotOK || wantMessage["role"] != gotMessage["role"] {
			continue
		}
		wantText, wantOK := wantMessage["content"].(string)
		gotText, gotOK := gotMessage["content"].(string)
		if !wantOK || !gotOK || wantText == gotText {
			continue
		}
		path := "/messages/" + strconv.Itoa(i) + "/content"
		if wantMessage["role"] == "tool" {
			allowed = append(allowed, toolContentDifference{SchemaVersion: 1,
				Path: path, Encoding: "utf-8", Recorded: textDiagnostic(wantText), Actual: textDiagnostic(gotText)})
		} else {
			category := ""
			if historyDialect == JiuwenHistoryDialect {
				if i == 0 && wantMessage["role"] == "system" && jiuwenSystemContextEqual(wantText, gotText) {
					category = "jiuwen_system_context"
				} else if i == firstUser && jiuwenUserTimestampEqual(wantText, gotText) {
					category = "jiuwen_user_timestamp"
				}
			}
			if category == "" {
				continue
			}
			promptAllowed = append(promptAllowed, promptContextDifference{SchemaVersion: 1,
				Path: path, Encoding: "utf-8", Category: category, Recorded: textDiagnostic(wantText), Actual: textDiagnostic(gotText)})
		}
		message := maps.Clone(wantMessage)
		message["content"] = gotText
		messages[i] = message
	}
	if path := difference(compared, actual, ""); path != "" {
		return path, nil, nil
	}
	return "", allowed, promptAllowed
}
