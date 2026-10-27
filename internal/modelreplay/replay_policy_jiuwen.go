package modelreplay

import (
	"fmt"
	"strings"
	"time"
)

// Digests describe original complete content strings, never normalized payloads.
type promptContextDifference struct {
	SchemaVersion int                   `json:"schema_version"`
	Path          string                `json:"path"`
	Encoding      string                `json:"encoding"`
	Category      string                `json:"category"`
	Recorded      requestTextDiagnostic `json:"recorded"`
	Actual        requestTextDiagnostic `json:"actual"`
}

// Only the pinned build_user_prompt envelope is eligible. The task, attachments
// and every other field remain equal, including the original user in old history.
func jiuwenUserTimestampEqual(want, got string) bool {
	for _, prefix := range []string{"你收到一条消息：\n", "You receive a new message:\n"} {
		if !strings.HasPrefix(want, prefix) || !strings.HasPrefix(got, prefix) {
			continue
		}
		wantValue, wantErr := decode([]byte(strings.TrimPrefix(want, prefix)))
		gotValue, gotErr := decode([]byte(strings.TrimPrefix(got, prefix)))
		wantObject, wantOK := wantValue.(map[string]any)
		gotObject, gotOK := gotValue.(map[string]any)
		if wantErr != nil || gotErr != nil || !wantOK || !gotOK {
			return false
		}
		for _, object := range []map[string]any{wantObject, gotObject} {
			content, ok := object["content"].(string)
			if !ok || content == "" {
				return false
			}
			timestamp, ok := object["timestamp"].(string)
			parsed, err := time.Parse("2006-01-02 15:04:05", timestamp)
			if !ok || err != nil || parsed.Format("2006-01-02 15:04:05") != timestamp {
				return false
			}
		}
		wantField := `"timestamp": "` + wantObject["timestamp"].(string) + `"`
		gotField := `"timestamp": "` + gotObject["timestamp"].(string) + `"`
		// The pinned Python formatter has these exact separators. Preserve all
		// other bytes, including JSON ordering, whitespace and escaped task text.
		return strings.Count(want, wantField) == 1 && strings.Count(got, gotField) == 1 &&
			strings.Replace(want, wantField, gotField, 1) == got
	}
	return false
}

const jiuwenPrivateRootPrefix = "/run/aries-jiuwen-"
const jiuwenPrivateWorkspace = "/home/data/service_default/agent_default/agent/jiuwenclaw_workspace"

// Exact lines from pinned prompt_builder.py and RuntimePromptRail. No project,
// cwd, tool argument, attachment, or arbitrary system path is replaceable.
var jiuwenPrivatePathLines = map[string]bool{
	"| `{runtime}/home/data/config/config.yaml` | JiuwenSwarm 主配置 |": true,
	"| `{runtime}/home/data/config/.env` | JiuwenSwarm 环境变量 |":       true,
	"任务中出现相对路径 `config/` 时，不要自动解释为 `{runtime}/home/data/config`。只有用户明确要求修改 JiuwenSwarm 自身配置时，才修改上述文件。修改后需要重启 JiuwenSwarm 服务才能保证配置生效。": true,
	"- Agent 内部数据目录：{runtime}" + jiuwenPrivateWorkspace:                                                                    true,
	"- Agent 技能目录：{runtime}" + jiuwenPrivateWorkspace + "/skills":                                                          true,
	"- Agent 记忆目录：{runtime}" + jiuwenPrivateWorkspace + "/memory":                                                          true,
	"- Agent 待办目录：{runtime}" + jiuwenPrivateWorkspace + "/todo":                                                            true,
	"- JiuwenSwarm 启动配置目录：{runtime}/home/data/config":                                                                      true,
	"| `{runtime}/home/data/config/config.yaml` | JiuwenSwarm main configuration |":                                        true,
	"| `{runtime}/home/data/config/.env` | JiuwenSwarm environment variables |":                                            true,
	"When a task contains the relative path `config/`, do not automatically interpret it as `{runtime}/home/data/config`.": true,
	"- Agent internal data directory: {runtime}" + jiuwenPrivateWorkspace:                                                  true,
	"- Agent skills directory: {runtime}" + jiuwenPrivateWorkspace + "/skills":                                             true,
	"- Agent memory directory: {runtime}" + jiuwenPrivateWorkspace + "/memory":                                             true,
	"- Agent todo directory: {runtime}" + jiuwenPrivateWorkspace + "/todo":                                                 true,
	"- JiuwenSwarm startup configuration directory: {runtime}/home/data/config":                                            true,
}

func jiuwenPrivatePathLine(line string) (normalized, root string) {
	start := strings.Index(line, jiuwenPrivateRootPrefix)
	end := start + len(jiuwenPrivateRootPrefix) + 16
	if start < 0 || end > len(line) {
		return line, ""
	}
	for _, c := range line[start+len(jiuwenPrivateRootPrefix) : end] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return line, ""
		}
	}
	normalized = line[:start] + "{runtime}" + line[end:]
	if !jiuwenPrivatePathLines[normalized] {
		return line, ""
	}
	return normalized, line[start:end]
}

func jiuwenSystemContextEqual(want, got string) bool {
	left, leftPaths, leftOK := jiuwenSystemContext(want)
	right, rightPaths, rightOK := jiuwenSystemContext(got)
	return leftOK && rightOK && leftPaths == rightPaths && left == right
}

func jiuwenSystemContext(text string) (string, int, bool) {
	lines := strings.Split(text, "\n")
	root := ""
	paths := 0
	for i := 0; i < len(lines); i++ {
		if lines[i] == "# 运行环境" || lines[i] == "# Environment" {
			if !jiuwenLinuxEnvironment(lines, i) {
				return "", 0, false
			}
			lines[i+4] = "{native-linux-kernel}"
		}
		if lines[i] == "# 时间说明" || lines[i] == "# Time Description" {
			if !jiuwenDateBlock(lines, i) {
				return "", 0, false
			}
			lines[i+2], lines[i+3] = "{native-current-date}", "{native-current-week}"
			i += 3
			continue
		}
		normalized, foundRoot := jiuwenPrivatePathLine(lines[i])
		if foundRoot != "" {
			if root != "" && root != foundRoot {
				return "", 0, false
			}
			root, lines[i] = foundRoot, normalized
			paths++
		}
	}
	return strings.Join(lines, "\n"), paths, true
}

// Embedded Agents see the provider's real kernel. Only its version in the
// pinned native environment block is metadata; platform and shell stay exact.
func jiuwenLinuxEnvironment(lines []string, i int) bool {
	if i+5 >= len(lines) || lines[i+1] != "" || lines[i+5] != "" {
		return false
	}
	platform, shell, os := "- Current platform: `linux`", "- Shell: ", "- OS Version: Linux "
	switch lines[i] {
	case "# 运行环境":
		platform, shell, os = "- 当前运行平台：`linux`", "- Shell：", "- OS 版本：Linux "
	case "# Environment":
	default:
		return false
	}
	kernel, ok := strings.CutPrefix(lines[i+4], os)
	if lines[i+2] != platform || !strings.HasPrefix(lines[i+3], shell) || lines[i+3] == shell || !ok || kernel == "" {
		return false
	}
	for _, c := range kernel {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || strings.ContainsRune("._+-", c)) {
			return false
		}
	}
	return true
}

// Match the actual formatter, including weekday and Monday–Sunday range; do not
// replace arbitrary dates elsewhere in the system prompt or user task.
func jiuwenDateBlock(lines []string, i int) bool {
	if i+3 >= len(lines) || lines[i+1] != "" {
		return false
	}
	prefix := "Current date: "
	if lines[i] == "# 时间说明" {
		prefix = "当前日期："
	}
	text, ok := strings.CutPrefix(lines[i+2], prefix)
	if !ok || len(text) < 10 {
		return false
	}
	d, err := time.Parse("2006-01-02", text[:10])
	if err != nil {
		return false
	}
	weekday := (int(d.Weekday()) + 6) % 7
	start, end := d.AddDate(0, 0, -weekday).Format("2006-01-02"), d.AddDate(0, 0, 6-weekday).Format("2006-01-02")
	current := fmt.Sprintf("Current date: %s (%s)", d.Format("2006-01-02"), d.Weekday())
	week := fmt.Sprintf("This week: %s to %s", start, end)
	if lines[i] == "# 时间说明" {
		current = fmt.Sprintf("当前日期：%s（%s）", d.Format("2006-01-02"), []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}[weekday])
		week = fmt.Sprintf("本周日期范围：%s 至 %s", start, end)
	}
	return lines[i+2] == current && lines[i+3] == week
}
