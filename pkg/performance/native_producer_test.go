package performance

import (
	"strings"
	"testing"
)

const nativeProducerV1 = `{"schema_version":1,"scope_id":"run/occ","agent_session_id":"session-key","tool_call_id":"tool-one","native_invocation_id":"inv-a"}`
const nativeProducerV2 = `{"schema_version":2,"scope_id":"run/occ","agent_session_id":"session-key","tool_call_id":"tool-one","native_invocation_id":"inv-a","phase":"invocation","outcome":"started","dispatched":null,"reason":""}`
const nativeProducerRejected = `{"schema_version":2,"scope_id":"run/occ","agent_session_id":"session-key","tool_call_id":"tool-one","native_invocation_id":"inv-a","phase":"pre_dispatch","outcome":"rejected","dispatched":false,"reason":"shell_preflight_rejected"}`

func TestNativeProducerRecordVersionsAndRejection(t *testing.T) {
	for _, content := range []string{nativeProducerV1 + "\n", nativeProducerV2 + "\n", nativeProducerV2 + "\n" + nativeProducerRejected + "\n",
		nativeProducerV2 + "\n" + strings.Replace(nativeProducerRejected, "shell_preflight_rejected", "elevated_policy_rejected", 1) + "\n",
		nativeProducerV2 + "\n" + strings.Replace(nativeProducerRejected, "shell_preflight_rejected", "host_policy_rejected", 1) + "\n"} {
		records, err := DecodeNativeProducerRecords([]byte(content))
		if err != nil || len(records) != strings.Count(content, "\n") {
			t.Fatalf("records=%+v err=%v", records, err)
		}
		if records[0].Identity.SchemaVersion != 1 || records[0].Identity.SSHRequestID != "" || records[0].Dispatched != nil {
			t.Fatal("producer changed wire identity or invented dispatch")
		}
		if len(records) == 2 && (records[1].Identity != records[0].Identity || records[1].Dispatched == nil || *records[1].Dispatched || records[1].Outcome != "rejected") {
			t.Fatal("rejection lost exact invocation")
		}
		if len(records) == 2 {
			wantReason := "shell_preflight_rejected"
			if strings.Contains(content, "elevated_policy_rejected") {
				wantReason = "elevated_policy_rejected"
			}
			if strings.Contains(content, "host_policy_rejected") {
				wantReason = "host_policy_rejected"
			}
			if records[1].Reason != wantReason {
				t.Fatal("rejection reason was rewritten")
			}
		}
	}
}

func TestNativeProducerRejectsUnsupportedTransitions(t *testing.T) {
	for name, content := range map[string]string{
		"orphan-rejection":     nativeProducerRejected + "\n",
		"duplicate-invocation": nativeProducerV2 + "\n" + nativeProducerV2 + "\n",
		"duplicate-rejection":  nativeProducerV2 + "\n" + nativeProducerRejected + "\n" + nativeProducerRejected + "\n",
		"foreign-rejection":    nativeProducerV2 + "\n" + strings.Replace(nativeProducerRejected, "tool-one", "tool-other", 1) + "\n",
		"mixed-version":        nativeProducerV1 + "\n" + nativeProducerRejected + "\n",
		"missing-dispatch":     strings.Replace(nativeProducerV2, `,"dispatched":null`, "", 1) + "\n",
		"claimed-dispatch":     strings.Replace(nativeProducerV2, `"dispatched":null`, `"dispatched":true`, 1) + "\n",
		"rejection-unknown":    nativeProducerV2 + "\n" + strings.Replace(nativeProducerRejected, `"dispatched":false`, `"dispatched":null`, 1) + "\n",
		"free-text-reason":     nativeProducerV2 + "\n" + strings.Replace(nativeProducerRejected, "shell_preflight_rejected", "arbitrary error text", 1) + "\n",
		"null-reason":          strings.Replace(nativeProducerV2, `"reason":""`, `"reason":null`, 1) + "\n",
		"duplicate-key":        strings.Replace(nativeProducerV2, `"phase":"invocation"`, `"phase":"invocation","phase":"invocation"`, 1) + "\n",
		"ssh-id":               strings.TrimSuffix(nativeProducerV2, "}") + `,"ssh_request_id":"ssh"}` + "\n",
		"partial-record":       nativeProducerV2,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeNativeProducerRecords([]byte(content)); err == nil {
				t.Fatal("invalid producer accepted")
			}
		})
	}
}
