# DB Workload Replay 真实验收（2026-10-26）
表中“控制端”沿用原 `sdk_wait_duration_ns` / `exec.wait` 字段，实际从ARIES客户端提交进程前计到Wait返回，含提交、执行、输出传输与排空；不是单独Wait方法或官方SDK内部耗时。guest列是其独立wrapper窗口，两者不相减推断网络时间。


本页单独记录 `db-wal-recovery` 原合格 Tape 的两云 Workload 回放。两次运行显式采用 `model.replay_policy=workload`、`timing=recorded`，固定模型决策、工具名、参数、调用顺序及非工具历史，只允许对应文本 ToolResult 正文不同。真实工具、原私有 Verifier、停止撤权与清理要求均保留；本页结果不计为 Strict 成功或新的自主解题成绩。

两云本次 Workload 验收均通过：各完成 12/12 模型轮、11/11 工具决策、派发身份和原终态对账，原 Verifier 7/7、reward 1。E2B 正文差异为 0；Huawei 有 21 处包含重复历史的正文差异，逐项 UTF-8 字节数/SHA 对账通过。两轮同快照双 Trace 对账及清理均通过，原 Snapshot 的整体 coverage 仍为 `unknown`。这是此固定工作量的两云单次闭环，不替代 [Strict 独立链](remaining-strict-cases-20261026.md)。

运行先 E2B 后 Huawei，每云仅一次，无自动重试。启动沿用共享部署锁，systemd 设置 `Type=exec`、`UMask=0077`、`KillMode=mixed`、`RuntimeMaxSec=3600`、`TimeoutStopSec=600`。

## 原件与版本

原 Tape 来自 `/opt/aries-artifacts/tb-six-db-wal-recovery-record-1/runs/20261025T162843.830338160Z-tb-db-wal-recovery-record/db-wal-recovery-001/model/tape.json`，SHA `180abc61b0333ff38aa5ebcf91c727210da85311dc35253552fa197c7f3801b4`。原评分 7/7，12 次模型决策、11 个同步 `exec`、无其他工具调用；工具 schema 中虽然包含 `process`，该 Tape 没有实际触达它，也没有 `terminal-result-v1` 协议。

原 native transcript SHA `8df391207454f07a8e372702ab4eb35e6ceac80ed539857681883ea0125798cf`；全部结果的终态字段在扁平 `details` 中，没有 `details.exec`。原结果是一次 exit 127、十次 exit 0。验收以完整原生 ToolCall ID 对应，逐项比较 `isError`、status、exitCode、exitSignal、exitReason、failureKind、超时与输出截断/renderer 字段。缺关键终态字段或出现无协议的 accepted 结果为 UNKNOWN，不能通过；原非零退出原样保留。

原录制使用 native `cc7b2c8270be5ca0f5169b7b9f0b6574fd56f6f559cb778b9cbfc467b7b8e664`，publication SHA `29f377daf7144298f2c852b1db184ddabd0934145a430427d2c1f86b19f05cfc`。本页两云统一使用已有同 Tape 完整 Strict E2B 7/7 证明的 native `4f519985234cc93f653314ea672bcf25c0abcc9d0009597ef82f066c08d02d2d`，publication SHA `9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1`。两份 publication 实际状态均为 `PASS_PUBLISHED_CANDIDATE_ONLY`；不将镜像发布状态当作新业务验收。

12 轮原 request.tools 的规范 JSON SHA 均为 `e17f506f909cd38b895b243fe77e246f78430c5b91762c70191ab15f730086b1`，未改工具 schema。既有 Strict E2B 成功证据位于 `/opt/aries-artifacts/tb-crosscloud-db-wal-recovery-e2b-replay-1`，evidence SHA `036eb620340f678cfab0d614681a1da90958fbf9968bd7232b1c3c1ed8bfdb23`。旧 Huawei Strict 在首个 `ls` 后因目录元数据正文差异拒绝，记录独立保留；这里显式选择 Workload，没有自动降级。

控制器固定 release12：`/opt/aries-artifacts/workload-replay-release-12`，五门禁 evidence SHA `4aae132eae541198a1743b72bcf8078eac39ed9946fba100acc648dfafbe9823`，source manifest SHA `77f2b087bb4328d4866ecb920e3864dabe0f22ed8b91a23fffd0d7e5cc5f4ff6`，`aries-perf` SHA `6c24c1bd81aaeb40834a81e90ca89fa5fb7f9f23486f192e63d3216ac6c64d11`。

专用入口 `/root/aries-setup/db-workload-cloud-1` 从已锁定的 SQLite cloud2 通用 driver 最小适配，原清理和锁逻辑不变。DB driver SHA `653f3bb59a68312bcde2798845cacab4a69cd6b53ebd71ac03d9e45392e4485d`；正文差异审查器 SHA `98b446312c2e60e44c73497de6325b70c59773d1e7a57f1f643df37475e9a7b8`；DB 原件/终态审查器 SHA `ad24d9bd9f9afd7c3c9d3a4d47fe5fec68a7c399f77565252a5ba34c143f67c1`。准备回执 SHA `ee32eccc53a50ff19258062cdb90193e976a7becf20198a32eadc941a49f572c`，源码复核 SHA `5585a32a9e8130702af4e3332676dc83c25536b19276b6237f63654c9cb2c683`。

两云沿用旧历史回放的 profile/binding，包含人为选定的根目录分钟起点 mtime。该设置保留既有局限，不代表完整恢复原文件系统元数据、存储配额强制执行或跨云环境完全一致。原记录 native 与本页 native 不同，必须按上述版本谱系解释。

## 验收与测量范围

通过必须同时满足原评分 7/7、reward 1、12/12 模型轮、11/11 原生工具决策和派发身份、11/11 工具终态等价、无上游模型请求、正文差异逐项 UTF-8 字节数/SHA 与 Tape、native transcript、request boundary、Snapshot 对账，以及完整停止撤权、原清理和独立 provider 不存在确认。评分通过不能覆盖终态或证据失败。

历史正文在后续请求中重复出现的差异逐次保留；若某轮没有正文差异，则明确 `difference_branch_exercised=false`，不能宣称已覆盖差异分支。双 Trace 由同一个冻结 Snapshot 导出并独立交给 Trace Processor 对账；仅导出成功不算 Trace 验收，coverage 的 unknown 不能改写为完整覆盖。

八阶段直接取原 Attempt 字段，逐工具 SDK 与 guest 耗时按完整身份从原操作记录读取。不同计时来源不相减推断网络耗时，阶段不保证相加等于端到端；失败或缺测不会删去。单次样本不支持稳定性能排名、容量或 Goodput 结论。

## 两云实际结果

| 运行 | 原评分 / reward | 模型轮 / 工具终态等价 | Driver 秒 | Attempt 秒 | 历史正文差异 |
| --- | --- | --- | ---: | ---: | ---: |
| E2B replay-1 | 7/7 / 1 | 12/12 / 11/11 | 89.955334 | 83.186493 | 0 |
| Huawei replay-1 | 7/7 / 1 | 12/12 / 11/11 | 109.404457 | 103.455323 | 21 |

两轮均为 `PASS_REAL_DB_WORKLOAD_SCOPED_ACCEPTANCE`，无上游模型请求。E2B 的 `difference_branch_exercised=false`；Huawei 为 true。Huawei 差异来自原生 `call_00_DLnd8zd5HUPuv8PrlPin4213` 的正文在 11 个后续请求中的重复出现，以及 `call_00_WcJ3LlafkYJ7OXe3aQmK8790` 的正文在 10 个后续请求中的重复出现；不是 21 个不同工具调用，也未改写真实正文。

Driver 秒取其 started_at 至 finished_at，包含部署和事后审查；Attempt 秒取原 `attempts[].duration`，不含排队和接纳后的派发延迟，二者不等同 Runtime。Verifier 包含自身准备、原测试脚本执行及报告读取；pytest等内部耗时已经包含其中，不能再加一次。两轮全部工具终态都与录制一致，unknown=0、different=0；第一条原失败的 exit 127 保留，其余十条 exit 0。第 12 次模型决策没有新增工具调用。

八阶段名称与边界沿用[性能阶段说明](performance-stages.md)。单位为秒；排队保留九位小数，原纳秒值均保存在摘要。Runtime 执行严格读取 `runtime_execution_duration`，没有用较宽的 `agent.run` 区间替代。

| 阶段 | E2B replay-1 | Huawei replay-1 |
| --- | ---: | ---: |
| 排队 | 0.000019748 | 0.000019556 |
| 任务 Sandbox 创建 | 1.195724 | 0.695518 |
| 工作区准备 | 0.772663 | 0.874156 |
| Harness 启动 | 7.659113 | 7.404940 |
| Runtime 执行 | 52.845825 | 49.302566 |
| 停止与撤权 | 5.588856 | 5.373585 |
| 独立验证 | 12.438304 | 38.424295 |
| 清理 | 0.714300 | 0.593158 |

本次 Huawei 的 Driver 与 Attempt 时间更长，独立验证阶段也明显更长；其 Runtime 读数稍短。这些是单次观察，不能据此断定云平台、网络或验证器内部的具体瓶颈。

## 逐工具实际读数

下面分别列出 SDK 客户端 `exec.wait` 与 guest 回执耗时，单位为秒。全部 11 条均按完整原生 ToolCall ID、execution ID、process ID 与原操作记录精确关联，诊断列表为空。表中序号与模型轮恰好同为 1–11；原始 ID、来源文件/行号、原纳秒值保存在摘要，未公开 shell 正文。失败调用同样保留。

| exec / 模型轮 | E2B SDK | E2B guest | Huawei SDK | Huawei guest | 原录制 / 两云 exit |
| --- | ---: | ---: | ---: | ---: | --- |
| 1 / 1 | 0.552498 | 0.204078 | 0.130179 | 0.041822 | 127 / 127 / 127 |
| 2 / 2 | 0.345899 | 0.092910 | 0.433631 | 0.344716 | 0 / 0 / 0 |
| 3 / 3 | 0.288198 | 0.032915 | 0.132645 | 0.041364 | 0 / 0 / 0 |
| 4 / 4 | 0.309760 | 0.053304 | 0.142432 | 0.048087 | 0 / 0 / 0 |
| 5 / 5 | 0.728687 | 0.315605 | 0.515914 | 0.429793 | 0 / 0 / 0 |
| 6 / 6 | 0.789415 | 0.042538 | 0.149375 | 0.046931 | 0 / 0 / 0 |
| 7 / 7 | 0.294216 | 0.048095 | 0.140474 | 0.053489 | 0 / 0 / 0 |
| 8 / 8 | 0.289773 | 0.041253 | 0.143398 | 0.054609 | 0 / 0 / 0 |
| 9 / 9 | 0.288756 | 0.038827 | 0.131424 | 0.045828 | 0 / 0 / 0 |
| 10 / 10 | 0.275486 | 0.033139 | 0.133566 | 0.044951 | 0 / 0 / 0 |
| 11 / 11 | 0.299553 | 0.051433 | 0.131018 | 0.042558 | 0 / 0 / 0 |

## 双 Trace、清理与证据位置

两轮 Snapshot integrity 均为 complete，coverage 均为 unknown。每轮 Chrome Trace 与 OTLP 从该轮同一冻结 Snapshot 导出；Trace Processor 导入无 error，原 interval/flow 与 OTLP parent/link 分母逐项对账通过。没有伪造跨域时间线，也未把 unknown coverage 改为完整覆盖。

| 运行 | Chrome / TP 区间 | TP flows | OTLP spans / parents / links | 最大导入误差 | 排除区间 |
| --- | ---: | ---: | --- | ---: | ---: |
| E2B replay-1 | 1196 | 60 | 1196 / 60 / 0 | 0 ns | 0 |
| Huawei replay-1 | 1195 | 60 | 1195 / 60 / 0 | 0 ns | 0 |

E2B 独立投影检查 4836 项，Huawei 4832 项，全部通过。本次没有 OTLP 后端发送或新浏览器 UI 验收；这里的双 Trace 结论限于原始 Snapshot、标准导出和 Trace Processor 对账。

两次均创建且删除一个 Sandbox，原任务与共享基础设施 cleanup=succeeded，driver cleanup_errors=[]，独立 provider 查询确认不存在，ledger 全部 absent、runner 进程组不存在。Huawei unit 已 inactive、MainPID=0，两个临时 adapter 均已移除；共享锁及 8082 无监听状态在交接时核验。业务窗口已交给后续 SQLite 复测，本次没有再次启动云任务。

| 证据 | E2B replay-1 | Huawei replay-1 |
| --- | --- | --- |
| 原件根目录 | `/opt/aries-artifacts/tb-workload-db-e2b-replay-1` | `/opt/aries-artifacts/tb-workload-db-huawei-replay-1` |
| run ID | `20261026T105624.782666230Z-tb-db-wal-recovery-workload-replay` | `20261026T105940.168272829Z-tb-db-wal-recovery-workload-replay` |
| evidence SHA | `33956d3a9f735709ff206b2795c7c001e4ebde63b616efbc2683ec319e155a8e` | `bdf404d490d7f06cec20a9559c658c29242c4f4dda9f3c61e6aa068251502488` |
| 正文差异审查 SHA | `d95f64f636aa94f9689a6322393061ee9bbe07a325f22ccb654e97d7528cf96b` | `5e75dce1ecd03d2c866aff5e56911c2d7678a022b88a1f1f0a6574874ef934c6` |
| 双 Trace TP evidence SHA | `d0e433054010acbbe6673d39258dd823d6114c51f4000eb70534f120b4445d5e` | `fc9de98dd7cab2e56ffb9f673dd5eb3840874635143d371e0d2f14908d84c7e7` |
| 完整验收 SHA | `1533f8c728c2e8f532f596a2f59c749689110b5df25d6a7a7a4d879b92e74e00` | `e5f3cecf2458b30e304389b45d007a3af8da173741abb8cadcf52c6b3688ad6d` |

各轮独立投影与最终验收在原件根目录下 `performance-review-1/{evidence,acceptance}.json`。联合摘要 `/opt/aries-artifacts/workload-db-performance-archive-1/summary.json` SHA `54f8d3c0c1903f0938c9d7869f29f8e2c307ca99bd0e0ab5f4a995509f9c3ada`；联合验收 `acceptance.json` SHA `4c085cb8de8cd506be6fa26b0efa3de29fae71ed4933879c0f77c7c2384af72c`。摘要按原始文件保存来源 SHA、八阶段纳秒和逐工具身份/读数；本地仅取回小回执，原始运行文件留在 ECS。
