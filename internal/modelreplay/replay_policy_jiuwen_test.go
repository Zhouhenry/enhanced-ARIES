package modelreplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const recordedJiuwenRoot = "/run/aries-jiuwen-0123456789abcdef"
const actualJiuwenRoot = "/run/aries-jiuwen-fedcba9876543210"

// Excerpts retain the pinned native formatter's literal lines. Static content
// around the eligible lines is deliberately present to catch broad filtering.
const workloadSystemFixture = "SECRET_STATIC_SYSTEM\n## JiuwenSwarm 启动配置\n\n" +
	"| 路径 | 用途 |\n|------|------|\n" +
	"| `{root}/home/data/config/config.yaml` | JiuwenSwarm 主配置 |\n" +
	"| `{root}/home/data/config/.env` | JiuwenSwarm 环境变量 |\n\n" +
	"任务中出现相对路径 `config/` 时，不要自动解释为 `{root}/home/data/config`。只有用户明确要求修改 JiuwenSwarm 自身配置时，才修改上述文件。修改后需要重启 JiuwenSwarm 服务才能保证配置生效。\n" +
	"# 运行时目录上下文\n\n- 当前工作目录（cwd，也是 Bash 默认执行目录）：/workspace\n" +
	"- Agent 内部数据目录：{root}/home/data/service_default/agent_default/agent/jiuwenclaw_workspace\n" +
	"- Agent 技能目录：{root}/home/data/service_default/agent_default/agent/jiuwenclaw_workspace/skills\n" +
	"- Agent 记忆目录：{root}/home/data/service_default/agent_default/agent/jiuwenclaw_workspace/memory\n" +
	"- Agent 待办目录：{root}/home/data/service_default/agent_default/agent/jiuwenclaw_workspace/todo\n" +
	"- JiuwenSwarm 启动配置目录：{root}/home/data/config\n\n" +
	"# 时间说明\n\n当前日期：2026-10-27（周二）\n本周日期范围：2026-10-26 至 2026-11-01\n" +
	"- 当用户询问“最新、当前、今年、本年、实时、近期”等信息并需要搜索时，搜索 query 必须优先使用当前年份或日期\n" +
	"Reference date in static instructions: 2026-10-27\n"

func jiuwenWorkloadRequest(t *testing.T, raw []byte, actual bool) []byte {
	t.Helper()
	v, err := decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	req := v.(map[string]any)
	ms := req["messages"].([]any)
	root, timestamp := recordedJiuwenRoot, "2026-10-27 10:11:12"
	system := workloadSystemFixture
	if actual {
		root, timestamp = actualJiuwenRoot, "2026-11-02 12:13:14"
		system = strings.Replace(system, "当前日期：2026-10-27（周二）", "当前日期：2026-11-02（周一）", 1)
		system = strings.Replace(system, "本周日期范围：2026-10-26 至 2026-11-01", "本周日期范围：2026-11-02 至 2026-11-08", 1)
	}
	ms[0].(map[string]any)["content"] = strings.ReplaceAll(system, "{root}", root)
	ms[1].(map[string]any)["content"] = "你收到一条消息：\n" + fmt.Sprintf(`{"source": "web", "timezone": "Asia/Shanghai", "timestamp": %q, "preferred_response_language": "zh", "content": "幻灯片 生成一份关于大模型发展的pptx材料，写5页。", "files_updated_by_user": "{}", "type": "user input"}`, timestamp)
	if actual {
		for _, item := range ms {
			m := item.(map[string]any)
			if m["role"] == "tool" {
				m["content"] = "SECRET_ACTUAL_TOOL_RESULT: exit=127 " + m["tool_call_id"].(string)
			}
		}
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func jiuwenWorkloadTape(t *testing.T, parallel bool) *Tape {
	t.Helper()
	tape := requestTailTape(t)
	if parallel {
		tape = multiTape(t, false)
		tape.Identity.HarnessVersion, tape.HistoryDialect = jiuwenFixedTailHarnessVersion, JiuwenHistoryDialect
		for i := range tape.Turns {
			v, _ := decode(tape.Turns[i].Request)
			req := v.(map[string]any)
			req["messages"] = append(req["messages"].([]any), map[string]any{"role": "user", "content": jiuwenAttachmentPrefix + "runtime\">\nCurrent mode: agent\n</prompt-attachment>\n</system-reminder>"})
			tape.Turns[i].Request, _ = json.Marshal(req)
		}
	}
	for i := range tape.Turns {
		tape.Turns[i].Request = jiuwenWorkloadRequest(t, tape.Turns[i].Request, false)
		tape.Turns[i].SHA256 = turnDigest(tape.Turns[i])
	}
	tape.SHA256 = tapeDigest(*tape)
	if err := validateTape(*tape); err != nil {
		t.Fatal(err)
	}
	return tape
}

func jiuwenWorkloadServer(t *testing.T, tape *Tape, policy string) *Server {
	t.Helper()
	s, err := New(Options{Mode: "replay", ReplayPolicy: policy, Identity: tape.Identity, HistoryDialect: tape.HistoryDialect,
		AccessToken: []byte(testToken), Tape: tape, OutputDir: filepath.Join(t.TempDir(), "replay"), Timing: Timing{Mode: "zero"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestJiuwenWorkloadContextHistoryEvidenceAndExactRetry(t *testing.T) {
	tape := jiuwenWorkloadTape(t, false)
	original, _ := json.Marshal(tape)
	s := jiuwenWorkloadServer(t, tape, ReplayPolicyWorkload)
	for i, turn := range tape.Turns {
		raw := jiuwenWorkloadRequest(t, turn.Request, true)
		unchanged := bytes.Clone(raw)
		w := post(s, raw)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), bytes.Join(framesAt(i), nil)) || !bytes.Equal(raw, unchanged) {
			t.Fatalf("turn %d did not replay original response without altering input", i)
		}
		if i == 1 {
			if retry := post(s, raw); retry.Code != http.StatusOK || !bytes.Equal(retry.Body.Bytes(), w.Body.Bytes()) || s.position != 2 {
				t.Fatal("actual exact retry lost its response or advanced the cursor")
			}
		}
	}
	if s.Close() != nil || s.position != 3 {
		t.Fatal("workload did not finish")
	}
	after, _ := json.Marshal(tape)
	internal, _ := json.Marshal(s.tape)
	if !bytes.Equal(original, after) || !bytes.Equal(original, internal) {
		t.Fatal("old Tape was changed")
	}
	events, _ := readBoundaries(t, s)
	var matches []boundaryEvent
	for _, e := range events {
		if e.Event == "request_matched" && e.Status == "succeeded" {
			matches = append(matches, e)
		}
	}
	if len(matches) != 4 || !matches[2].CachedRetry || !reflect.DeepEqual(matches[1].PromptContextDifferences, matches[2].PromptContextDifferences) {
		t.Fatal("missing exact retry context evidence")
	}
	for n, e := range matches {
		if len(e.PromptContextDifferences) != 2 || len(e.ToolContentDifferences) != []int{0, 1, 1, 2}[n] {
			t.Fatal("context/body evidence or full old history was lost")
		}
		for i, category := range []string{"jiuwen_system_context", "jiuwen_user_timestamp"} {
			d := e.PromptContextDifferences[i]
			want, _ := decode(tape.Turns[0].Request)
			got, _ := decode(jiuwenWorkloadRequest(t, tape.Turns[0].Request, true))
			wantText := want.(map[string]any)["messages"].([]any)[i].(map[string]any)["content"].(string)
			gotText := got.(map[string]any)["messages"].([]any)[i].(map[string]any)["content"].(string)
			if d.SchemaVersion != 1 || d.Category != category || d.Path != fmt.Sprintf("/messages/%d/content", i) || d.Encoding != "utf-8" || d.Recorded != textDiagnostic(wantText) || d.Actual != textDiagnostic(gotText) {
				t.Fatal("context evidence did not hash complete original content")
			}
		}
		raw, _ := json.Marshal(e.PromptContextDifferences)
		if bytes.Contains(raw, []byte("SECRET_")) || bytes.Contains(raw, []byte(actualJiuwenRoot)) || bytes.Contains(raw, []byte("幻灯片")) {
			t.Fatal("prompt evidence leaked content")
		}
	}
}

func TestJiuwenWorkloadRejectsStaticHistoryAndCausalityChanges(t *testing.T) {
	for _, field := range []string{"old_task", "attachments", "extra_user_field", "user_format", "invalid_timestamp", "system_instruction", "static_date", "invalid_weekday", "invalid_week", "mixed_runtime_root", "private_path_placeholder", "private_path_suffix", "project_path", "mode_attachment", "old_tool_id", "old_tool_arguments", "missing_tool", "stream"} {
		t.Run(field, func(t *testing.T) {
			tape := jiuwenWorkloadTape(t, false)
			s := jiuwenWorkloadServer(t, tape, ReplayPolicyWorkload)
			for i := 0; i < 2; i++ {
				if post(s, jiuwenWorkloadRequest(t, tape.Turns[i].Request, true)).Code != http.StatusOK {
					t.Fatal("setup")
				}
			}
			v, _ := decode(jiuwenWorkloadRequest(t, tape.Turns[2].Request, true))
			req := v.(map[string]any)
			ms := req["messages"].([]any)
			replace := func(index int, old, new string) {
				m := ms[index].(map[string]any)
				text := m["content"].(string)
				if !strings.Contains(text, old) {
					t.Fatal("negative fixture did not change content")
				}
				m["content"] = strings.Replace(text, old, new, 1)
			}
			switch field {
			case "old_task":
				replace(1, "写5页", "写6页")
			case "attachments":
				replace(1, `"files_updated_by_user": "{}"`, `"files_updated_by_user": "[]"`)
			case "extra_user_field":
				replace(1, `"type": "user input"`, `"type": "user input", "override": true`)
			case "user_format":
				replace(1, `"source": "web"`, `"source":"web"`)
			case "invalid_timestamp":
				replace(1, "2026-11-02 12:13:14", "2026-11-32 12:13:14")
			case "system_instruction":
				replace(0, "SECRET_STATIC_SYSTEM", "changed instruction")
			case "static_date":
				replace(0, "Reference date in static instructions: 2026-10-27", "Reference date in static instructions: 2026-11-02")
			case "invalid_weekday":
				replace(0, "2026-11-02（周一）", "2026-11-02（周二）")
			case "invalid_week":
				replace(0, "2026-11-02 至 2026-11-08", "2026-11-03 至 2026-11-09")
			case "mixed_runtime_root":
				replace(0, actualJiuwenRoot, recordedJiuwenRoot)
			case "private_path_placeholder":
				replace(0, actualJiuwenRoot, "{runtime}")
			case "private_path_suffix":
				replace(0, "/home/data/config/config.yaml", "/home/data/config/other.yaml")
			case "project_path":
				replace(0, "/workspace", actualJiuwenRoot+"/home/data")
			case "mode_attachment":
				replace(len(ms)-1, "智能体模式", "agent")
			case "old_tool_id":
				ms[3].(map[string]any)["tool_call_id"] = "other-call"
			case "old_tool_arguments":
				ms[2].(map[string]any)["tool_calls"].([]any)[0].(map[string]any)["function"].(map[string]any)["arguments"] = `{"command":"different command"}`
			case "missing_tool":
				req["messages"] = append(ms[:3], ms[4:]...)
			case "stream":
				req["stream"] = false
			}
			raw, _ := json.Marshal(req)
			if post(s, raw).Code != http.StatusConflict || s.position != 2 || s.Close() == nil {
				t.Fatal("workload accepted a non-context or causal change")
			}
			events, _ := readBoundaries(t, s)
			for _, e := range events {
				if e.Turn != nil && *e.Turn == 2 && (len(e.PromptContextDifferences) != 0 || len(e.ToolContentDifferences) != 0) {
					t.Fatal("rejected differences were labeled accepted")
				}
			}
		})
	}
}

func TestJiuwenWorkloadContextDoesNotRelaxStrictOtherDialectsOrRetryIdentity(t *testing.T) {
	tape := jiuwenWorkloadTape(t, false)
	want, _ := decode(tape.Turns[0].Request)
	got, _ := decode(jiuwenWorkloadRequest(t, tape.Turns[0].Request, true))
	for _, dialect := range []string{"", jiuwenFixedTailHistoryDialect, jiuwenRequestTailHistoryDialect, JiuwenSubagentHistoryDialect, JiuwenHistoryDialect} {
		for _, policy := range []string{"", ReplayPolicyStrict, ReplayPolicyWorkload} {
			path, tool, prompt := replayRequestDifference(want, got, policy, dialect)
			eligible := policy == ReplayPolicyWorkload && dialect == JiuwenHistoryDialect
			if (path == "") != eligible || !eligible && (len(tool) != 0 || len(prompt) != 0) {
				t.Fatal("context relaxation escaped the explicit main Jiuwen workload contract")
			}
		}
	}
	s := jiuwenWorkloadServer(t, tape, ReplayPolicyWorkload)
	raw := jiuwenWorkloadRequest(t, tape.Turns[0].Request, true)
	if post(s, raw).Code != http.StatusOK {
		t.Fatal("first request")
	}
	changed := bytes.Replace(raw, []byte("12:13:14"), []byte("12:13:15"), 1)
	if post(s, changed).Code != http.StatusConflict || s.position != 1 || s.Close() == nil {
		t.Fatal("workload-equivalent but nonidentical request became cached retry")
	}
}

func TestJiuwenWorkloadParallelToolsKeepOrderAndIDs(t *testing.T) {
	for _, reordered := range []bool{false, true} {
		tape := jiuwenWorkloadTape(t, true)
		s := jiuwenWorkloadServer(t, tape, ReplayPolicyWorkload)
		if post(s, jiuwenWorkloadRequest(t, tape.Turns[0].Request, true)).Code != http.StatusOK {
			t.Fatal("parallel setup")
		}
		raw := jiuwenWorkloadRequest(t, tape.Turns[1].Request, true)
		if reordered {
			v, _ := decode(raw)
			ms := v.(map[string]any)["messages"].([]any)
			ms[3], ms[4] = ms[4], ms[3]
			raw, _ = json.Marshal(v)
		}
		w := post(s, raw)
		if (w.Code == http.StatusOK) == reordered {
			t.Fatal("parallel result order/ID association changed")
		}
		if !reordered && (post(s, jiuwenWorkloadRequest(t, tape.Turns[2].Request, true)).Code != http.StatusOK || s.Close() != nil) {
			t.Fatal("parallel workload did not finish")
		}
	}
}

func TestJiuwenWorkloadEnglishNativeContext(t *testing.T) {
	want := "unchanged\n| `" + recordedJiuwenRoot + "/home/data/config/config.yaml` | JiuwenSwarm main configuration |\n" +
		"# Time Description\n\nCurrent date: 2026-10-27 (Tuesday)\nThis week: 2026-10-26 to 2026-11-01\nstatic instructions\n"
	got := strings.ReplaceAll(want, recordedJiuwenRoot, actualJiuwenRoot)
	got = strings.Replace(got, "2026-10-27 (Tuesday)", "2026-11-02 (Monday)", 1)
	got = strings.Replace(got, "2026-10-26 to 2026-11-01", "2026-11-02 to 2026-11-08", 1)
	if !jiuwenSystemContextEqual(want, got) || jiuwenSystemContextEqual(want, strings.Replace(got, "static instructions", "new instructions", 1)) {
		t.Fatal("English formatter context scope changed")
	}
	want = `You receive a new message:` + "\n" + `{"content": "five pages", "timestamp": "2026-10-27 00:00:00", "files_updated_by_user": "{}"}`
	got = strings.Replace(want, "2026-10-27 00:00:00", "2026-11-02 23:59:59", 1)
	if !jiuwenUserTimestampEqual(want, got) || jiuwenUserTimestampEqual(want, strings.Replace(got, "five pages", "six pages", 1)) {
		t.Fatal("English user task binding changed")
	}
}
