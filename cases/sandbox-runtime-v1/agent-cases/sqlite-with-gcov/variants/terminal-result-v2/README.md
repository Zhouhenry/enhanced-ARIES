# SQLite 编译：terminal-result-v2 公开指令变体

状态：`CONSTRUCTED_STATIC_CHECKED`；本 v2 的真实 Agent 录制、Strict Replay 和跨云比较均为 `UNRUN`。原生任务及 [v1 材料](../terminal-result-v1/README.md)、Tape 与失败记录全部保留；既有 native 协议验证不等于本工作负载已经跑通。

独立逻辑身份为 `sqlite-with-gcov-terminal-result-v2`，仍选择原 `task_id=sqlite-with-gcov`。来源为 Terminal-Bench 2.1 固定提交 `7131e4375048a0e408a8fb404b5f499d726b695b`（Apache-2.0）；[原题](../../public/instruction.md)、[task.toml](../../task.toml)、环境 fixture、私有 Verifier 和预算保持原字节。这是获准的 ARIES 公开交互变体，不是未经修改的官方基准成绩。

- [instruction-appendix.md](instruction-appendix.md)：相对 v1 修改版本标题和元数据指导段，并补充有限后台工作省略可选单次等待超时的说明。目录发现使用一个固定 locale 的 names-only 例子；需要属性时只检查相关字段。没有解题步骤、编译参数、宏或参考答案。
- [effective-instruction.txt](effective-instruction.txt)：上游去首尾空白后的原任务说明＋两个换行＋附录原字节，附录不 trim。
- [prompt.diff](prompt.diff)：原题到最终公开文本的差异；原题段落未改。
- [variant.json](variant.json)：源 pin、原文件/附录/最终 prompt SHA、v1 来源与保留约束。这份控制端元数据引用私有评分位置，不能整体分发给 Agent。

v1 的真实 SQLite 回放先在解包后的目录长列表处偏离：父目录时间由运行期间创建内容改变。仅声明初始 mtime 不足以消除此类输出；v2 改为明确要求目录发现只输出名称，并选择任务需要的属性。同一完整录制还有两次 30000ms wait 超时，后续 wait 才完成，因此同一 v2 明确有限工作使用无单次超时的终态等待。它不改系统时钟、运行中新建文件时间或真实输出，不屏蔽错误，也不限制必要调试和原生工具能力。

`terminal-result-v2` 是公开指令版本；Harness 协议仍为 **`terminal-result-v1`**。显式 background→wait 段只新增一句：有限工作省略可选单次 wait 超时，等待真实终态；原任务截止时间与取消仍适用，不把终态等待变成反复短超时轮询。原生 poll/log/PTY 保留段与 v1 原字节相同。复用 [现有固定版本目录](../../../../../../configs/versions-terminal-result-v1.json) 的 `e040654e…` native 镜像，本材料不要求重新构建 Go 或 native 镜像。

原 Agent 和 Verifier 预算各 900 秒，原评分仍为 3 项。沿用已声明的通用工具环境适配，不预编译 SQLite；覆盖文件存在不能单独证明 Agent 完成了构建。 CPU 1、内存 2048 MB 不变。独立私有评分仍在确认 Agent 停止、Bridge 撤权后才注入原 Sandbox；不把参考答案录成模型轨迹。

[独立 record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-e2b-a-sqlite-with-gcov-terminal-result-v2-record.json) 保留原模板绑定身份，使用独立 `benchmark.variant.id` 与附录 SHA。生产接口已支持显式附录；实际运行需将其定位到确切文件并记录最终 prompt SHA，不改冻结的上游 checkout。新公开文本必须重新真实录制，不能以 v1 Tape 冒充 v2 验收。

本次改动不保证所有结果跨运行逐字相同。名字本身也可能由平台动态生成；原生有限等待的 timeout/completed 仍取决于真实完成速度，v2 指导比较工作负载省略该可选参数，但不伪造状态或强制修改 Agent 参数；poll/log/PTY 及子进程主动输出的时间/PID 仍可能偏离。Strict matcher 不放宽，也不冻结真实状态。是否形成可比较样本，仍需原独立评分、完整新 Tape、真实工具重跑与清理证据；相同 CPU/内存、placement、变体及 Tape 对比，磁盘差异另行报告。

设计背景见[可复现工作负载设计](../../../../../../docs/design/reproducible-agent-workloads.md)；历史实际结果见[变体性能归档](../../../../../../docs/repro-variants-performance-20261026.md)。本目录只构造公开材料，没有执行工作负载、编译、测试或云调用。
