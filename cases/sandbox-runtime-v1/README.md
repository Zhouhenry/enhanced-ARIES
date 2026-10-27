# Agent Sandbox 用例材料 v1

**主线是自主 Agent 任务：Agent 阅读题目、自行分析和修改、调用真实工具，再由独立 Verifier 评分。** material_revision=3 共六例：`fix-git`、`cancel-async-tasks`、`db-wal-recovery`、`sqlite-with-gcov`、`large-scale-text-editing`、`custom-memory-heap-crash`，均来自固定版 Terminal-Bench 2.1。材料索引中的 **CONSTRUCTED_UNRUN** 保留构造归档时的状态，不代表这些任务目前全部未运行。

**实际运行状态请看 [E2B 六题逐次结果与阶段性能](../../docs/tb-six-performance-20261026.md) 和 [华为 C++ 准备与运行归档](../../docs/tb-huawei-cpp-performance-20261026.md)。** 两者区分原评分、Strict Replay、失败与在途任务；本目录保存来源和任务定义，不以材料状态代替运行结论。

从 [真实 Agent 用例指南](agent-cases/README.md) 开始阅读；[index.json](agent-cases/index.json) 提供机器可读的来源、原始资源/超时、公开与私有文件清单、候选目标、原评分、负例设计和测量边界。六例保留原题目及 2 / 6 / 7 / 3 / 5 / 6 个原测试函数，共 29 个；没有复制参考解，也没有预定 Agent 的命令顺序或调用次数。

| 主用例 | Agent 要解决的问题 | 可研究的真实工作流 |
| --- | --- | --- |
| `terminal-bench/fix-git` | 找回个人站点丢失的更改并合入 master | Git 调查、文件修改、复核和真实工具往返 |
| `terminal-bench/cancel-async-tasks` | 实现限制并发且取消后正确清理的异步函数 | 代码编写、运行、调试及测试循环 |
| `terminal-bench/db-wal-recovery` | 调查数据库及 WAL，恢复要求的数据 | 数据检查、恢复程序、文件及数据库工具 |
| `terminal-bench/sqlite-with-gcov` | 自主将预置 SQLite 源码编译为带 gcov 的可用工具 | 依赖配置、真实编译、安装；源/进程证据另验 |
| `terminal-bench/large-scale-text-editing` | 自主设计 Vim 宏完成百万行 CSV 变换 | 编辑、调试、大文件处理；重型评分独立计时 |
| `terminal-bench/custom-memory-heap-crash` | 自主调查并修复小型 C++ 项目的 release 崩溃 | 源码调查、编译与调试；Verifier 编译另计，不称纯内存性能 |

这些任务用于 Agent Runtime E2E、模型等待、真实工具执行、阶段开销和有独立验证的任务吞吐。自主 Agent 每次可能采取不同步骤，不能把实时模型实验的工具总耗时直接解释为云厂商性能差异；需要固定轨迹时，应从真实 Agent 运行录制 Strict Replay，再严格跨环境回放。题目、参考解或预设计划都不能替代真实录制。

## 辅助测量材料

以下材料用于补充定位具体工作负载的耗时，**不作为真实 Agent 任务的替代或主用例**。原定义和文件均保留，统计与原始 TB 分开。

| 材料 | 数量与状态 | 用途 |
| --- | --- | --- |
| [ASB catalog](cases.json) | 10 个自建受控 workload、30 档固定参数；catalog 原字节保留，`DESIGNED_UNRUN` | 固定 exec、CPU、内存、文件、编译、SQL、HTTP 工作量；不是官方基准 |
| [ASB 验收卡](acceptance.md) | 10 张设计卡，含负例和测量边界 | 将程序功能与实际执行证据分开 |
| [标准工具变体规格](standard-variants.json) | 5 项 fio/sysbench/iperf3 变体、16 组合；`DESIGNED_UNRUN` | 官方工具语义下的辅助性能实验，工具版本/digest 仍待冻结 |
| [exec-short 构造材料](exec-short/README.md) | `CONSTRUCTED_FUNCTIONAL_CHECKED`，仅 ECS 本机子进程功能检查 | 24/96/384 条单步计划及独立文件/回执检查；未验证真实 ToolBridge、Sandbox 或 Agent 性能 |

[来源与性能测量依据](authority-and-measurement.md) 区分原始 Agent 基准、标准工具和自建负载，说明可以比较什么、需要哪些真实执行证据。数量口径为 **6 个原始 Agent 主用例 + 10 个 ASB 辅助 catalog 用例 + 5 项标准工具变体规格**；不能合并成已运行案例数或官方基准成绩。`cases.json` 中旧的三任务参考索引保留来源用途，最新材料状态以 `agent-cases/index.json` 为准。

## 接入边界

本目录是交接材料，**不是可直接加载的 Profile 或标准 task checkout**；`public/`、`environment/`、`private/` 按公开输入与私有评分分开保存。现行 [cloud 配置](../../pkg/config/cloud.go) 已支持 `benchmark.type=terminalbench2`，在 E2B 或华为 AgentSphere 上以 External placement 执行单任务、单 attempt；读取固定原始 checkout 和对应 provider 的环境 binding，保留原 `task.toml` 预算。实际入口、准备要求与环境适配见 [Terminal-Bench 云运行指南](../../profiles/cloud/terminalbench/README.md)，不能将材料目录整体上传给 Agent。镜像 digest、模板与评分依赖以具体运行的 binding 和证据为准。

实施时沿用 [Runner 四角色与生命周期](../../pkg/runner/interfaces.go)：只给 Agent 原题目和原运行环境；Harness 停止且 Bridge 撤权确认后才暴露私有 tests。原生后台、自动 yield、process 轮询和 PTY 保持可用。封存候选、保存评分/CTRF 和原始观测、回收所属资源分别记录，不靠产物通过来证明工具调用历史；未采集项保持未知。

## 来源核验

原始 TB 来源为 [terminal-bench-2-1 固定 commit](https://github.com/harbor-framework/terminal-bench-2-1/tree/7131e4375048a0e408a8fb404b5f499d726b695b)，Apache-2.0。原三例的 22 个原始文件（含共享许可证）保持原字节；revision 2 新增 14 个官方固定 commit 文件，当时共 36 个；revision 3 再增 10 个，当前共 46 个原始来源文件。新增文件的 Git blob/大小均匹配官方树，SHA-256 与旧文件未变证明分别见 [revision 2 来源清单](agent-cases/revision-2-source-manifest.json) 和 [revision 3 来源清单](agent-cases/revision-3-source-manifest.json)；SQLite 源包另匹配原 checksum sidecar。原题目、测试和 canary 未改，参考解未复制。

实际 `归档.zip` 为 975144 bytes，SHA-256 `d2a57502b9efe12a3de99bf94da570007226a8e586bc5ba3bf073633aa7563ef`，CRC 通过。包内是已展开 suite，没有交接说明所称的内层 ZIP，因此其指定哈希未核验。suite 的 MANIFEST **386/386** 匹配，总计 **387** 文件；Research 与 REPORT.md 逐字节一致。来源的历史 local 结果不算本项目真实 Agent、Sandbox 或性能结果。
