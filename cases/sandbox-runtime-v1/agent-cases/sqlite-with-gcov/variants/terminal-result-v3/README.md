# SQLite 编译：terminal-result-v3 公开指令变体

状态为构造时快照：`CONSTRUCTED_STATIC_CHECKED`；本 v3 真实录制、Strict Replay、跨云比较均为 `UNRUN`。原任务及 [v1](../terminal-result-v1/README.md)、[v2](../terminal-result-v2/README.md) 的材料、Tape 和失败原件不变。实际运行状态以[变体性能归档](../../../../../../docs/repro-variants-performance-20261026.md)为准。

独立身份为 `sqlite-with-gcov-terminal-result-v3`，原任务仍为 `sqlite-with-gcov`。原题固定于 Terminal-Bench 2.1 提交 `7131e4375048a0e408a8fb404b5f499d726b695b`，许可证 Apache-2.0。这是独立公开交互变体，不是未经修改的官方成绩。

本次只将附录标题 v2 改为 v3，并在元数据段后添加一个经确认的通用段落：第三方 build/install 等命令自动输出的无关元数据也适用既有规则；不需要在对话中展开的完整 stdout/stderr 保留为工作区日志，保留原命令真实退出码，日志读取是 Agent 的真实工具操作；失败必须报告并检查真实诊断。没有 SQLite 安装步骤、编译参数、参考解或私测内容，也没有命令、参数、环境的硬限制。

本轮 v2 首次回放在第 24 个请求 / turn 23，`call_00_jGCNvYpQlozkAHTkxeoT0465` 的 `make install` 自动长列表发生偏离：三项 libsqlite3 产物仅时间 `06:46` → `06:57` 六字节不同。这不是 Agent 自己执行 `ls`；v2 目录发现指导没有覆盖这类成功日志。v3 不修改实际 mtime、系统时钟、原生输出或 Strict matcher，也不对模型可见 ToolResult 做后处理、过滤或替换。

- [instruction-appendix.md](instruction-appendix.md)：新公开附录；除标题和新增段落外与 v2 字节一致。
- [effective-instruction.txt](effective-instruction.txt)：上游 trim 后原任务文本＋两个 LF＋附录原字节；附录不 trim。
- [prompt.diff](prompt.diff)：原题到最终公开文本的差异。
- [variant.json](variant.json)：原题 pin、保留文件、附录/最终文本 SHA 和修订依据。其私测位置/哈希仅供控制端核验，不整体分发给 Agent。
- [E2B record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-e2b-a-sqlite-with-gcov-terminal-result-v3-record.json)：原 binding 身份与独立变体 ID；真实运行定位到已冻结的外部附录。

Harness 后台协议仍为 `terminal-result-v1`，复用[固定版本目录](../../../../../../configs/versions-terminal-result-v1.json)的 `e040654e…` 镜像。真实后台等待、取消、poll/log/kill/PTY、原生 ToolResult 和全部工具能力不变；不是后台协议升级。本材料不需要 Go/native 构建。

[原题](../../public/instruction.md)、[task.toml](../../task.toml)、环境 fixture、私有 Verifier 都保持原字节。原 Agent / Verifier 预算各 900 秒，评分仍为原 3 项；CPU 1、内存 2048 MB、TTL 2400 秒不变。私测只在确认 Stop 与 Bridge 撤权后注入原 Sandbox，沿用通用构建工具准备，不预编译 SQLite 答案。

公开指导不能保证 Agent 遵从，也不能使任务必需诊断变为确定输出。日志重定向及按需读取会改变真实 I/O 和模型上下文，必须独立统计，不能用 v2 Tape 代替 v3；先获得一份合格的新真实录制，再在多云重复使用同一 Tape。工作区日志不等于已经导出归档：当前没有自动归档整个 workspace 或所有日志的新增能力。功能通过仍需完整 Tape、原独立评分及实际工具和清理证据才能形成对应比较样本；CPU/内存、placement、变体和 Tape 固定，磁盘差异另行说明。

本目录只构造材料并核对字节、来源和链接；没有执行工作负载、编译、模型或云调用。
