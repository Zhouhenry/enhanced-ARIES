# db-wal-recovery：metadata-portable-v1 独立变体

状态：材料已构造并静态核对；此变体的真实录制、独立评分、Strict Replay 和双云对照均为 `UNRUN`。不是原生不变的官方成绩。

原始 DB Tape 在 E2B 通过，在 Huawei 首次目录检查的 ToolResult 中出现真实文件系统差异（目录大小、链接数、块总数和日期），第二个模型请求的完整历史因此 Strict 拒绝。失败 Tape 和运行归档保留；只固定日期不能消除全部差异。

本变体只在原题后追加公开的元数据读取建议：读取任务需要的信息，避免无关时间、耗时和进程标识；不得伪造元数据、改系统时钟、隐藏错误或替换真实输出。附录取自已批准的 terminal-result-v1 元数据段，不引入该版本的后台 start/wait 协议，也不修改 Native ToolResult。Agent 仍自主选择真实调查和修复步骤。

- 原题选择仍为 `db-wal-recovery`；Attempt 及统计身份独立为 `db-wal-recovery-metadata-portable-v1`。
- 保留原题、原始加密 fixture、私有 Verifier、Agent / Verifier 各 900 秒、1 CPU / 2048 MB、原始模板绑定。只读环境适配仍按既有声明归档。
- 复用已测试的 `benchmark.variant` 入口：上游加载后的原指令 + 两个 LF + 附录原字节。文件及 SHA 见 [variant.json](variant.json)、[prompt.diff](prompt.diff)。
- 私有测试留在控制端，停止与撤权确认后才注入评分；参考解不进入 Agent 提示或任务镜像。

入口：[E2B 首次真实录制 profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-e2b-a-db-wal-recovery-metadata-portable-v1-record.json)。它继承原 DB profile，不隐式启用新后台协议；运行时固定实际版本目录、原题 binding、观测配置和工具输出契约。Huawei 对照只切换明确的 provider / 原题 binding，不复用旧 Tape 充当新变体。

验收需新真实 Agent 录制通过原独立评分，并用同一份新 Tape 在 E2B、Huawei 执行真实 Tool、完整 Strict 校验、原私有评分和确认清理。所有失败保留，必要动态输出若仍偏离就如实失败；提示建议本身不保证跨云通过。CPU / 内存配置一致即可开展比较，磁盘等差异单列说明；当前没有此变体的性能结果。
