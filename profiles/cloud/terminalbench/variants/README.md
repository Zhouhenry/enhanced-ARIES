# 真实 Agent 的独立可复现变体入口

状态：当前冻结运行时为 **release10，五项工程门禁已通过**；真实录制与严格回放验收仍按用例分别推进。最新实际状态见[变体性能归档](../../../../docs/repro-variants-performance-20261026.md)，跨云结果见[跨云测试记录](../../../../docs/tb-crosscloud-performance-20261026.md)。各材料中的状态是构造时快照；v3 材料准备完成不等于真实录制或回放通过，工程检查也不替代端到端成绩。

| 原题选择 | 独立 Attempt 逻辑身份 / record profile | Agent / Verifier 预算 | Sandbox TTL | 材料 |
| --- | --- | --- | --- | --- |
| `large-scale-text-editing` | [large-scale-text-editing-terminal-result-v1](openclaw-e2b-a-large-scale-text-editing-terminal-result-v1-record.json) | 1200 / 1200 s | 3000 s | [v1](../../../../cases/sandbox-runtime-v1/agent-cases/large-scale-text-editing/variants/terminal-result-v1/README.md) |
| `large-scale-text-editing` | [large-scale-text-editing-terminal-result-v2](openclaw-e2b-a-large-scale-text-editing-terminal-result-v2-record.json) | 1200 / 1200 s | 3000 s | [v2](../../../../cases/sandbox-runtime-v1/agent-cases/large-scale-text-editing/variants/terminal-result-v2/README.md) |
| `sqlite-with-gcov` | [sqlite-with-gcov-terminal-result-v1](openclaw-e2b-a-sqlite-with-gcov-terminal-result-v1-record.json) | 900 / 900 s | 2400 s | [v1](../../../../cases/sandbox-runtime-v1/agent-cases/sqlite-with-gcov/variants/terminal-result-v1/README.md) |
| `sqlite-with-gcov` | [sqlite-with-gcov-terminal-result-v2](openclaw-e2b-a-sqlite-with-gcov-terminal-result-v2-record.json) | 900 / 900 s | 2400 s | [v2](../../../../cases/sandbox-runtime-v1/agent-cases/sqlite-with-gcov/variants/terminal-result-v2/README.md) |
| `sqlite-with-gcov` | [sqlite-with-gcov-terminal-result-v3](openclaw-e2b-a-sqlite-with-gcov-terminal-result-v3-record.json) | 900 / 900 s | 2400 s | [v3](../../../../cases/sandbox-runtime-v1/agent-cases/sqlite-with-gcov/variants/terminal-result-v3/README.md) |
| `db-wal-recovery` | [db-wal-recovery-metadata-portable-v1](openclaw-e2b-a-db-wal-recovery-metadata-portable-v1-record.json) | 900 / 900 s | 2400 s | [metadata v1](../../../../cases/sandbox-runtime-v1/agent-cases/db-wal-recovery/variants/metadata-portable-v1/README.md) |
| `custom-memory-heap-crash` | [custom-memory-heap-crash-metadata-portable-v1](openclaw-huawei-a-custom-memory-heap-crash-metadata-portable-v1-record.json) | 1800 / 1800 s | 4200 s | [metadata v1](../../../../cases/sandbox-runtime-v1/agent-cases/custom-memory-heap-crash/variants/metadata-portable-v1/README.md) |
| `custom-memory-heap-crash` | [custom-memory-heap-crash-terminal-result-v2](openclaw-huawei-a-custom-memory-heap-crash-terminal-result-v2-record.json) | 1800 / 1800 s | 4200 s | [v2](../../../../cases/sandbox-runtime-v1/agent-cases/custom-memory-heap-crash/variants/terminal-result-v2/README.md) |
| `custom-memory-heap-crash` | [custom-memory-heap-crash-terminal-result-v3](openclaw-huawei-a-custom-memory-heap-crash-terminal-result-v3-record.json) | 1800 / 1800 s | 4200 s | [v3](../../../../cases/sandbox-runtime-v1/agent-cases/custom-memory-heap-crash/variants/terminal-result-v3/README.md) |

`benchmark.task_id` 仍指向固定 Terminal-Bench checkout 的原题；`benchmark.variant.id` 区分变体，`instruction_appendix_file` 和 `appendix_sha256` 固定公开附录。实际任务说明只在原题后追加两个换行和附录，原文本、task.toml、环境声明、私有测试和评分算法不改；原/附录/最终任务文本 SHA 应进入运行证据。不要修改上游干净 checkout。

公开指令版本与 Harness 协议版本分开：`terminal-result-v1` 指令引入有限后台工作的显式启动与真实终态等待；v2 明确目录发现使用 names-only 输出、按需读取相关属性，并指导有限工作省略可选单次 wait 超时；SQLite/C++ v3 进一步覆盖第三方工具的冗长日志，完整 stdout/stderr 可留在工作区，必须保留底层命令退出状态并真实检查失败诊断。v3 不排序或过滤 ToolResult，不提供解法，也不增加自动工作区日志归档。

上述所有 `terminal-result-v1/v2/v3` profile 均启用同一个 **`harness.replay_tool_protocol=terminal-result-v1`**，保持 external、SSH identity、`stdout-stderr-v1` 和单任务单 Attempt。`versions_file` 指向[专用固定版本目录](../../../../configs/versions-terminal-result-v1.json)，OpenClaw 镜像为已验证并发布的 `sha256:e040654efde8adde910583291b547fb9d5fc94e565b2f4dc8bad29abcc423041`；v2/v3 没有升级 native 协议。真实驱动还须核对发布回执。绑定以各 profile 的 `sandbox.binding_file` 为准，明确区分 E2B 与 Huawei；原预算直接取 task.toml，不填超时覆盖，也不加隔离假设。

DB 与 C++ 的 `metadata-portable-v1` 只追加同一公开附录中的元数据读取建议，继承各自原 profile 的版本和工具行为，不启用 `replay_tool_protocol`，不强制后台 start/wait，也不修改 Native ToolResult。运行时仍需固定实际已验证的版本目录、原题 binding 和观测配置。

先运行独立的真实模型 record；只有原评分、Tape、停止/撤权和清理均符合要求，才能从实际 Tape 生成独立 replay 配置。旧 Tape 不迁移、不修写。后续华为同 Tape 运行需另选明确的 Huawei provider/binding，保留 CPU/内存规格一致、相同变体和 placement；磁盘等平台差异写进结果说明。入口各自明确 E2B 或 Huawei provider；实际跨云成绩以同 Tape 验收记录为准。

附录不提供答案或预定命令序列。真实 Agent 仍可使用原生 poll/log/PTY 和控制操作；这些实时结果可能不确定，Strict matcher 仍完整检查。任务运行中的日期、PID或输出若偏离，保留失败，不通过隐藏字段或模拟进程结果蒙混过关。
