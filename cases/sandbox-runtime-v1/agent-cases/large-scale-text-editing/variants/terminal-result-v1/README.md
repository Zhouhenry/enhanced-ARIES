# 大文本编辑：terminal-result-v1 独立变体

状态：材料已构造并静态核对；新协议、真实 Agent、新 Tape、Strict Replay 和跨云对照均尚未运行。原生任务的历史成绩与失败保留，不能由本目录替换。

变体身份为 `large-scale-text-editing-terminal-result-v1`，仍选择原 `task_id=large-scale-text-editing`。来源是 Terminal-Bench 2.1 固定提交 `7131e4375048a0e408a8fb404b5f499d726b695b`；[原题](../../public/instruction.md)、[原 task.toml](../../task.toml)、初始 CSV 与原私有评分保持不变。这是获准的 ARIES 交互变体，不是未修改的官方基准成绩。

- [instruction-appendix.md](instruction-appendix.md)：唯一新增的 Agent 公开说明。只说明必要元数据读取及显式 background→wait；不提供 Vim 宏、转化算法或成功命令序列。
- [effective-instruction.txt](effective-instruction.txt)：按“上游已去首尾空白的原任务说明＋两个换行＋附录文件原字节”组成的任务文本；附录及其结尾换行不再trim。
- [prompt.diff](prompt.diff)：公开文本的逐行差异；原段落未改。
- [variant.json](variant.json)：独立身份、原文件与附录/最终文本 SHA、保留预算/资源和验收边界；它是控制端材料，不能整体放进 Agent 工作目录。

原 Agent 和 Verifier 预算各 1200 秒，CPU 1、内存 2048 MB；原模板与环境适配继续单独冻结。原评分仍执行原 5 项测试，私有 tests 只在确认 Agent 停止、Bridge 撤权后注入原 Sandbox，不随本公开附录分发。Verifier 重新生成大文件、执行宏、比较结果的工作量属于评分阶段，不能当作 Agent 先前已经执行相同工作量的证据。

[独立 record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-e2b-a-large-scale-text-editing-terminal-result-v1-record.json) 保留原 `benchmark.task_id`，使用已确定的 `benchmark.variant={id,instruction_appendix_file,appendix_sha256}` 接口；变体 ID 作为 Attempt 逻辑任务身份，和原生样本分开。候选 native 镜像已通过 ECS 定向验证并发布；`versions_file` 指向仓库内 `configs/versions-terminal-result-v1.json`，只覆盖已验证镜像的固定 digest。完整工程发布和本变体真实 Agent 录制/回放仍待验收，当前文件存在不等于端到端通过。固定 checkout 仍须保持干净；不得覆盖其中的 `instruction.md` 来假装未改官方任务。

真实录制必须由 Agent 自主完成，原评分通过且 Tape 完整；之后至少两个新 Sandbox 的同 Tape 严格回放，再进行相同 placement、CPU/内存规格下的 E2B/华为比较。磁盘等差异只报告说明，不因差异本身判样本失格。原生 poll/log/PTY 仍可用；若 Agent 输出动态时间、PID或实时前缀，Strict 仍逐字检查，不能修 Tape 或删输出来通过。

总体协议、等待时间与进程时间的重叠关系见[变体设计](../../../../../../docs/design/reproducible-agent-workloads.md)。本材料没有修改原题、私有测试、Strict matcher 或原生默认工具路径。
