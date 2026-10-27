# 六个真实 Agent 任务：云 Sandbox A 模式单任务入口

这些 profile 选择固定 Terminal-Bench 2.1 原题，Agent 自主分析并调用真实工具；没有预设解题步骤或参考解 Tape。原题、来源与原 Verifier 见 [任务材料索引](../../../cases/sandbox-runtime-v1/agent-cases/README.md)。入口限定 OpenClaw external，一次一个任务、一个 attempt，云厂商由 profile 和环境 binding 一致指定。六个 E2B 入口的实际结果见[逐次归档](../../../docs/tb-six-performance-20261026.md)；华为 C++ 的工程、模板与 4200 秒创建已通过，首次真实 Agent 完成但原评分未成功，见[独立归档](../../../docs/tb-huawei-cpp-performance-20261026.md)。文件存在不表示任务已通过。

| Profile 的任务名 | 原 Agent 预算 | 原 Verifier 预算 | Sandbox TTL |
| --- | ---: | ---: | ---: |
| fix-git | 900 s | 900 s | 2400 s |
| cancel-async-tasks | 900 s | 900 s | 2400 s |
| db-wal-recovery | 900 s | 900 s | 2400 s |
| large-scale-text-editing | 1200 s | 1200 s | 3000 s |
| custom-memory-heap-crash | 1800 s | 1800 s | 4200 s |
| sqlite-with-gcov | 900 s | 900 s | 2400 s |

每份 `openclaw-e2b-a-<任务>-record.json` 从仓库 `.cache/terminal-bench-2-1` 读取固定 checkout `7131e4375048a0e408a8fb404b5f499d726b695b`，从 `.cache/tb2-e2b/<任务>/binding.json` 读取该任务的真实环境绑定；两种路径均相对 profile 文件解析。绑定必须由 ECS 准备结果取得，包含原镜像 digest、具体 `TemplateID:BuildID`、工作目录、资源配置、环境差异和 build 证据哈希；不得用另一任务模板或全局模板变量代替。实际适配器必须允许对应 TTL。

新增 `openclaw-huawei-a-custom-memory-heap-crash-record.json` 使用独立的 `.cache/tb2-huawei/custom-memory-heap-crash/binding.json`，binding 必须明确声明 `provider: "huawei-agentsphere"`；旧 binding 省略 `provider` 仍只代表 E2B。华为凭据和适配器使用 `ARIES_HUAWEI_ADAPTER_TOKEN` 与 `ARIES_HUAWEI_ADAPTER_URL`，不复用 E2B 模板或状态。C++ 原 Agent/Verifier 预算各 1800 秒，profile 保留 4200 秒 TTL；本次华为创建已接受，E2B 当前项目仍拒绝超过 1 小时的请求，不为适配限额缩短原预算。该原题允许联网，原私有评分仍在同一 Sandbox；profile 不设置 `assume_network_isolation`，也不声称实现了新建禁网评分环境。

现有六个 E2B 环境在 ECS 准备时各有镜像归档、任务模板和短期启动探针：保留原镜像内容，只增加可信 Python 控制入口、UID 60000、可写 `/app/bin` 及必要权限适配。SQLite 预装通用构建工具，仍由 Agent 解包原 SQLite 源码、配置、编译并安装；这属于显式记录的 ARIES 环境适配。模板不包含私有 tests 或 solutions。华为环境需独立准备并留存对应证据。启动/cgroup 探针通过仅证明环境准备完成，探针结束删除 Sandbox，不计作 Agent 成功或性能样本。

实际运行前冻结独立运行目录、版本目录、Native 观测镜像及配置、模型入口和环境 binding；这些基础 profile 不复制其他任务临时的模型地址、令牌或适配器监听地址。模型凭据仍走私有环境配置。原 Agent/Verifier 时间预算直接使用 `task.toml`，没有在 profile 覆盖。确认 Harness 停止、进程组清空及 Bridge 撤销后，才在原任务 Sandbox 注入并执行原私有 Verifier，保持上游评分方法。

记录成功真实轨迹后才能构造对应 Strict Replay 入口；回放继续执行真实工具和独立评分，不修改 Tape 或匹配规则。六例先逐例运行、修复环境或系统故障并保留全部尝试；不以辅助微基准代替真实 Agent 任务，也不从单次 E2E 成绩推断纯 Sandbox 性能。

`fix-git` 的可回放环境还需固定新建 Git 提交的日期：binding 的 `command_env` 中同时设置 `GIT_AUTHOR_DATE`、`GIT_COMMITTER_DATE` 为 `2026-10-25T00:00:00+00:00`，并在 `adaptations` 记录这一环境差异。首次未固定时，真实录制和回放产生不同的新提交 ID，虽然原评分通过，Strict Replay 仍正确拒绝；后续使用新 binding 重新真实录制，并在新 Sandbox 严格回放通过。原失败、原 binding 和旧 Tape 保留，不改 ToolResult 或 Matcher。该设置仅固定 Git 元数据，不冻结进程或性能测量时钟；原任务、私有评分、镜像和模板不变。
