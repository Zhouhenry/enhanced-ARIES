# C++ 堆损坏调试：terminal-result-v3 独立变体

构造时状态：`CONSTRUCTED_STATIC_CHECKED`；本 v3 真实录制、Strict Replay 与跨云比较均 `UNRUN`。沿用现有 release10，不构建新 Go/native；原任务及 [metadata-portable-v1](../metadata-portable-v1/README.md)、[terminal-result-v2](../terminal-result-v2/README.md) 的材料、Tape 与全部结果保留。实际运行状态见[变体性能归档](../../../../../../docs/repro-variants-performance-20261026.md)。

独立身份为 `custom-memory-heap-crash-terminal-result-v3`，仍选择原 `task_id=custom-memory-heap-crash`。原题固定于 Terminal-Bench 2.1 提交 `7131e4375048a0e408a8fb404b5f499d726b695b`，Apache-2.0 许可。[原题](../../public/instruction.md)、[task.toml](../../task.toml)、环境源码和私测共 10 文件保持原字节；这是独立公开交互变体，不是未经修改的官方成绩。

- [instruction-appendix.md](instruction-appendix.md)：原样复用 [SQLite v3](../../../sqlite-with-gcov/variants/terminal-result-v3/instruction-appendix.md) 的 2664 字节通用附录，SHA `7fae98659041edf2600c81f7e78d3a2036cae7ff3b536d4f59f9d3102789cccc`。相对 v2 只有标题和一个获批段落变化；没有 gdb 专用指令、调试步骤、补丁或答案。
- [effective-instruction.txt](effective-instruction.txt)：上游 trim 后原任务文本＋两个 LF＋附录原字节，附录不 trim。
- [prompt.diff](prompt.diff)：原题到最终公开文本的差异，原题内容不改。
- [variant.json](variant.json)：源 pin、SHA、预算及修订依据，仅供控制端核验，不整体分发给 Agent。
- [Huawei record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-huawei-a-custom-memory-heap-crash-terminal-result-v3-record.json)：独立身份和附录 pin，沿用原题模板绑定。

旧 v2 同 Tape 回放在第 6 个模型请求 / turn 5 的 `call_00_wMYrddcSIOQ0iohF67eF1088` 失败：两条相同的 libthread_db 通知在已经由 guest 命令内部合流的 stdout 中重排。它不是 native renderer 双流重排、初始 mtime 或身份缺失；旧失败不回写。新增段落只指导按实际任务需要把完整 stdout/stderr 留在工作区日志，并保留底层命令真实退出状态；日志读取仍是 Agent 的真实工具调用。

这不是 I/O 排序修复。实际崩溃、堆损坏和错误诊断不能当作无关信息隐藏；必要动态诊断仍须原样读取，Strict 可以继续因真实偏离失败。日志查看或管道末端成功不能替代底层命令状态。没有输出排序、过滤、ToolResult 后处理、时间伪造或固定结果替代；也没有新增自动归档整个 workspace 或所有日志的能力。

原 Agent / Verifier 预算为 1800＋1800 秒，Sandbox TTL 4200 秒；CPU 1、内存 2048 MB、既有华为固定根 mtime binding 不变。私有评分仍为原 6 项，在确认 Stop/Bridge 撤权后注入原 Sandbox；其中 5 次评分编译不算 Agent 工作量。E2B 项目一小时限制仍独立保留，华为通过不能替代 E2B 通过。

公开工作负载版本 v3 不升级 Harness 协议：仍为 `terminal-result-v1` 和 e040 固定镜像。原生 poll/log/kill/PTY、取消、真实失败、原 Task 截止时间与全部工具能力保留。公开指导不保证 Agent 遵从或输出总是确定；新版本会改变模型上下文和真实日志 I/O，必须独立统计，先录得一份合格的新真实 Tape，再复用它完成 Strict 回放，不能修改旧 Tape。相同 CPU/内存、placement、变体和 Tape 用于比较，磁盘差异另行说明。

本次只有材料构造和静态来源核验，没有执行工作负载、编译、模型或云操作。
