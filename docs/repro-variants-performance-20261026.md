# 独立可复现变体：真实运行归档（2026-10-26）

这些任务追加了已批准的公开交互说明，与原生原题运行分开统计。保留原任务、时间预算、真实工具及原私有 Verifier；不放宽 Strict，也不将原失败 Tape 改成成功。

## DB metadata-portable-v1：E2B record-1

**本次 FAIL：原独立评分 5/7，另外两次原生 host 策略拒绝没有身份观测。** 录制完成 68 轮，Tape 完整；真实调用共 67 次（65 次 exec、2 次其他原生工具），67 个结果。Harness 完成与模型完成不等于任务解答正确。停止、Bridge 撤权、单个 Sandbox 删除及 provider 不存在确认均通过，没有清理错误。

该次使用 release7、固定原生镜像 `39c688b7…`、A / External、1 CPU / 2048 MiB；附录仅为 `metadata-portable-v1`，没有启用新的后台终态协议。ECS 同期有其他工程/镜像准备工作，此次用于功能验收和性能留档，不作为稳定容量或云厂商排名。

### 八阶段耗时

阶段名称与[统一阶段说明](performance-stages.md)一致。单位为秒；Verifier 的耗时是原失败评分流程的耗时。各窗口不能简单求和替代端到端测量。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000013 |
| 任务 Sandbox 创建 | 1.055018 |
| 工作区准备 | 1.728986 |
| Harness 启动 | 7.664350 |
| Runtime 执行 | 368.380020 |
| 停止与撤权 | 5.736511 |
| 独立验证 | 12.887366 |
| 清理 | 0.831097 |

### 工具身份与失败边界

65 次归档 exec 中，62 次有精确原生 → SSH → Bridge → SDK 关联；1 次有明确 shell 预检拒绝；2 次请求切换到 gateway/node，被原生 host 策略拒绝，但拒绝点早于身份生产，所以现有快照无法关联。缺口按 R01 系统问题修复，保留原政策与错误；不能由命令相似性或 ToolResult 文本补造旧身份。

原评分的两项失败都涉及 WAL 更新后数据值不正确；最终输出有 11 行，并非行数不足。实际终端导出使用了字面数据列表，未恢复正确值；未发现预算耗尽或公开必需输入被系统阻挡的证据。诊断证据 `tb-repro-db-functional-review-1/conclusion.json` SHA `2aa16cd739e863699f717d822bae475bb7dad14a0e4ba7a37739ec9176fb5d1c`。不能由观测修复推断解题通过，后续仍需真实重测。没有以 driver 的后续观测错误替代原评分。

<details>
<summary>逐次 exec：模型轮、SDK 等待及 guest 整体耗时</summary>

SDK 与 guest 是不同计时边界，不相减推断网络时延；未知不是 0，明确未派发没有 guest 时长。process 等其他工具不纳入 exec 表。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 783.482 | 187.308 | 0 | exact |
| 2 | 2 | 326.270 | 39.960 | 127 | exact |
| 3 | 3 | 717.234 | 425.394 | 0 | exact |
| 4 | 4 | 336.420 | 38.728 | 0 | exact |
| 5 | 5 | 310.560 | 39.259 | 0 | exact |
| 6 | 6 | 463.538 | 179.606 | 0 | exact |
| 7 | 7 | 318.662 | 33.975 | 0 | exact |
| 8 | 8 | 318.381 | 34.253 | 0 | exact |
| 9 | 9 | 1754.571 | 1482.990 | 0 | exact |
| 10 | 10 | 392.023 | 108.819 | 0 | exact |
| 11 | 11 | 342.212 | 42.689 | 0 | exact |
| 12 | 12 | 332.004 | 40.195 | 0 | exact |
| 13 | 13 | 416.725 | 142.679 | 0 | exact |
| 14 | 14 | 413.010 | 36.114 | 0 | exact |
| 15 | 15 | 328.861 | 42.988 | 0 | exact |
| 16 | 16 | 995.263 | 112.883 | 0 | exact |
| 17 | 17 | 408.120 | 46.955 | 1 | exact |
| 18 | 18 | 314.043 | 36.296 | 0 | exact |
| 19 | 19 | 333.111 | 32.333 | 0 | exact |
| 20 | 20 | 312.773 | 40.607 | 1 | exact |
| 21 | 21 | 302.872 | 34.936 | 0 | exact |
| 22 | 22 | 873.003 | 585.413 | 0 | exact |
| 23 | 23 | 359.945 | 79.595 | 0 | exact |
| 24 | 24 | 331.892 | 40.028 | 0 | exact |
| 25 | 25 | 353.283 | 71.320 | 0 | exact |
| 26 | 26 | 357.179 | 88.998 | 0 | exact |
| 27 | 27 | 318.330 | 36.877 | 0 | exact |
| 28 | 28 | 320.751 | 38.406 | 0 | exact |
| 29 | 29 | 862.923 | 362.823 | 0 | exact |
| 30 | 30 | 566.325 | 286.082 | 0 | exact |
| 31 | 31 | 381.930 | 100.184 | 0 | exact |
| 32 | 32 | 28194.061 | 未知 | 未知 | 身份或终态未知 |
| 33 | 35 | 497.268 | 100.700 | 0 | exact |
| 34 | 36 | 506.346 | 113.374 | 0 | exact |
| 35 | 37 | 500.313 | 105.527 | 0 | exact |
| 36 | 38 | 492.937 | 100.252 | 0 | exact |
| 37 | 39 | 475.884 | 97.419 | 0 | exact |
| 38 | 40 | 476.682 | 97.296 | 0 | exact |
| 39 | 41 | 未知 | 未知 | 未知 | 身份或终态未知 |
| 40 | 42 | 未知 | 未知 | 未知 | 身份或终态未知 |
| 41 | 43 | 未知 | 未知 | 未知 | 身份或终态未知 |
| 42 | 44 | 508.819 | 98.797 | 0 | exact |
| 43 | 45 | 501.863 | 92.102 | 0 | exact |
| 44 | 46 | 477.997 | 102.698 | 0 | exact |
| 45 | 47 | 542.232 | 161.461 | 0 | exact |
| 46 | 48 | 491.988 | 105.488 | 0 | exact |
| 47 | 49 | 482.204 | 97.624 | 0 | exact |
| 48 | 50 | 472.096 | 92.396 | 0 | exact |
| 49 | 51 | 488.872 | 109.945 | 0 | exact |
| 50 | 52 | 1306.622 | 883.862 | 1 | exact |
| 51 | 53 | 505.887 | 117.570 | 0 | exact |
| 52 | 54 | 493.350 | 100.007 | 2 | exact |
| 53 | 55 | 474.655 | 96.063 | 0 | exact |
| 54 | 56 | 511.960 | 107.850 | 0 | exact |
| 55 | 57 | 457.790 | 89.419 | 0 | exact |
| 56 | 58 | 505.684 | 113.062 | 2 | exact |
| 57 | 59 | 532.494 | 126.304 | 0 | exact |
| 58 | 60 | 527.858 | 140.242 | 0 | exact |
| 59 | 61 | 495.315 | 92.825 | 0 | exact |
| 60 | 62 | 534.371 | 146.188 | 0 | exact |
| 61 | 63 | 496.170 | 128.710 | 0 | exact |
| 62 | 64 | 482.858 | 92.912 | 2 | exact |
| 63 | 65 | 469.319 | 101.039 | 0 | exact |
| 64 | 66 | 491.106 | 113.174 | 0 | exact |
| 65 | 67 | 482.633 | 96.425 | 0 | exact |

</details>

### 原件、版本与双 Trace

ECS 目录 `/opt/aries-artifacts/tb-repro-db-e2b-record-1`。同一 AnalysisSnapshot 供 Perfetto 和离线 OTLP 导出，Trace Processor 对账为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`；源完整性及覆盖状态仍是 incomplete。此结论只支持不完整来源的正确投影，不支持任务成功、全链完整或新的 Jaeger/Prometheus 后端验收。

| 证据 | SHA-256 |
| --- | --- |
| 原 driver evidence | `67e440ef5d3a698078ca0465bf923c74b167a6d54cd0c520922c1b1c76477fee` |
| 原完整 Tape | `8153cd31713c9adeb0fb33bdd7d8ca3a7978c117e0f071ee0864c31feea380dc` |
| performance-review-1/summary.json | `e13a4339d26a686b50a2ef1172a8275176fa020f335eb369225efafab2522e99` |
| offline-analysis-1/evidence.json | `3fd50190245917f17a2bf5fb9d7859e3e1944300a932bfd9945e9b2809d2704e` |
| release7 source manifest | `a3f8b2fae6314725f9259d4017927c014fb796b38213182ba688400aa0967cdc` |

[工程、原生与驱动验证](observability/VALIDATION_AND_REPRODUCE.md#replay-variants-release7)与真实 Agent 结果分开；[原题双云结果](tb-crosscloud-performance-20261026.md)继续保留。

## 大文本 terminal-result-v1：E2B record-1

**本次 FAIL：原独立评分 5/5、reward 1，但第 15 个模型请求生成了含重复键的工具参数，Tape 不完整。** 前 14 个请求成功；原生归档保留 15 次调用及结果，其中 13 次 exec、2 次 process。Harness 返回 succeeded，Attempt 与 driver 仍为失败；评分通过不能替代完整录制验收。本次未进入 Strict Replay。停止、Bridge 撤权、Sandbox 删除及 provider 不存在确认通过，清理错误为 0。

使用 release7、`terminal-result-v1` 独立镜像和公开附录、A / External、1 CPU / 2048 MiB，原题与原私有评分不变。本次仍是单任务功能验收与性能留档，不是独占资源的云厂商性能排名。

### 八阶段耗时

阶段名称与[统一阶段说明](performance-stages.md)一致，单位为秒。Runtime 执行取原 `runtime_execution_duration`，不是外围 Harness.Run 的全部耗时；这些窗口不能简单相加替代端到端测量。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000010 |
| 任务 Sandbox 创建 | 0.943356 |
| 工作区准备 | 1.442819 |
| Harness 启动 | 7.662563 |
| Runtime 执行 | 133.214512 |
| 停止与撤权 | 6.347953 |
| 独立验证 | 64.138163 |
| 清理 | 0.709417 |

### 失败原因与后台协议实际覆盖

第 15 个模型请求（从 0 计为 turn 14）上游 HTTP 200，接收 30 个 SSE 帧，已向原生 Agent 转发 29 帧。已保存的工具参数共 59 字节，`process` 调用重复出现 `action` 键，且没有 `sessionId`。录制器在流结束时经 `streamResult.complete → decode` 拒绝重复 JSON 键，外层记录为 `DELIVERY_OR_UPSTREAM_FAILED`；没有 `frame_flush_failed`，也未标记取消或超时。失败在同一模型时钟的 admission 关闭之前 **1.750746780 秒**，不是 Harness 返回后迟到请求的旧问题。随后 3 个请求被已失败的会话拒绝。

原生 Agent 已从转发的流中组装该调用，并收到“未找到作业”的失败结果。不能因此把重复键参数改为可接受，也不能因原评分通过而修补旧 Tape。最后收到的 14 字节帧在校验失败前未写入 Tape，不能声称其原始字节已有完整归档；失败归因依据已保存的重复键参数、实际事件顺序及固定源码。

后台协议确实触达：2 个作业被接受，其中 1 个具有 `accepted → wait_started → terminal_result(completed, exit 0) → wait_finished(returned)` 的真实记录；另 1 个只有 accepted，未采到原生终态。两次 exec 都由原生自动 yield 转为后台，没有显式设置 `background=true`，因此本次不能证明显式后台策略及整段回放已完成验收。协议日志的 `retained` 只代表字节已保留；独立原生时钟不与模型或控制器时钟相减。

<details>
<summary>逐次 exec：模型轮、SDK 等待及 guest 整体耗时</summary>

单位为毫秒。SDK 等待与 guest 整体属于不同边界，不能相减推断网络时延；未知不是 0。第 8–10 次没有唯一精确关联；第 13 次 SDK 等待被取消，但 guest 耗时与退出码未采集，不能从此补出进程退出。2 次 process 不纳入这张 exec 表，其耗时未由该表采集。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 / 缺测 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 2434.211 | 2147.675 | 0 | exact |
| 2 | 2 | 378.049 | 86.240 | 0 | exact |
| 3 | 3 | 1303.985 | 439.832 | 1 | exact |
| 4 | 4 | 7397.197 | 7112.950 | 1 | exact |
| 5 | 5 | 6965.482 | 6706.898 | 1 | exact |
| 6 | 6 | 5858.370 | 5586.843 | 0 | exact |
| 7 | 7 | 34817.033 | 34538.943 | 0 | exact |
| 8 | 9 | 未知 | 未知 | 未采集 | 缺少唯一精确关联 |
| 9 | 10 | 未知 | 未知 | 未采集 | 缺少唯一精确关联 |
| 10 | 11 | 未知 | 未知 | 未采集 | 缺少唯一精确关联 |
| 11 | 12 | 314.473 | 30.264 | 0 | exact |
| 12 | 13 | 293.502 | 33.742 | 0 | exact |
| 13 | 14 | 18198.652 | 未采集 | 未采集 | SDK canceled，guest 终态未知 |

</details>

### 原件、版本与双 Trace

原件目录 `/opt/aries-artifacts/tb-repro-text-e2b-record-1`。同一不完整 AnalysisSnapshot 的 Perfetto / 离线 OTLP 投影完成 6,127 项核对：1,518 个区间 / Span，87 条 Flow / parent，Trace Processor 时间差 0 ns；结论为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`。源完整性与覆盖仍为 incomplete，不计新的 Jaeger / Prometheus 后端验收，也不计完整回放成功。

| 证据 | SHA-256 |
| --- | --- |
| 原 driver evidence | `02bbbe7b0e5d864d53f5c231df640494f62c6633464bb662c8194bbf729d3b56` |
| 原不完整 Tape | `5b1ba58b88d5b52bbe185c89de4f4575deb3c2cb0f83ebe6da9e7f828c6adb3e` |
| performance-review-1/summary.json | `4ba6e82b034f1333125d1b7d12b55118f1175894486014622a2171707585c3db` |
| offline-analysis-1/evidence.json | `5c07de28b4577a09253adfa958d051a458477bf1e1faca9f1d931ce1a037364c` |
| 独立只读诊断 `tb-repro-text-failure-review-1/conclusion.json` | `fffff09d6085c3c4e0f14b6768fc57da8135a318aae921c853a5caba6b46aa08` |

## SQLite terminal-result-v1：E2B record-1

**原题评分 3/3、reward 1，录制完整；driver 验收仍为 FAIL。** 原 runner 返回 0，完成 37 个模型轮，保留 36 次原生调用及结果（31 次 exec、5 次 process）。失败原因为 `protocol_wait_result_differs_from_terminal`，不能将它记为完整端到端通过或 Strict Replay 成功。停止、Bridge 撤权和清理完成，唯一 Sandbox 已确认不存在，清理错误为 0。

### 八阶段耗时

单位为秒，名称与[统一阶段说明](performance-stages.md)一致；各窗口不能简单相加替代端到端测量。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000015 |
| 任务 Sandbox 创建 | 1.132309 |
| 工作区准备 | 1.402966 |
| Harness 启动 | 7.332046 |
| Runtime 执行 | 186.988737 |
| 停止与撤权 | 5.923303 |
| 独立验证 | 13.487299 |
| 清理 | 0.703030 |

### 归档缺口与修复状态

实际失败发生在原生长结果归档：超过 8,192 字节的结果经过两处字段白名单，丢失了 `protocol`、`outputRenderer`、`outputTruncated`，导致归档结果与协议终态无法按原规则对账。实时 ToolResult 的 `completed / exit 0 / stdout-stderr-v1 / outputTruncated=false` 保持正确；不能据此回写缺字段的旧档案，也不放宽验收。

修复仅补齐这两处白名单中的字段。ECS 已通过实际 `SessionManager → JSONL` 路径的 8 组定向验证；这证明归档修复候选的行为，不是本次原失败记录变为通过。合并镜像已执行一次[真实 E2B 回放](#sqlite-e2b-replay-1)，在第 6 次模型请求因目录时间偏离而失败，尚未触达后台等待与长结果归档；不能据此宣称该次回放覆盖了此路径。后续独立 v2 实录已真实触达超过 8,192 字节的归档截断并通过三字段对账，见[SQLite v2 实录](#sqlite-v2-e2b-record-1)；原 v1 失败保持不变。本节未补造原录制尚未整理的逐次工具耗时。

### 原件与离线双 Trace

原件目录 `/opt/aries-artifacts/tb-repro-sqlite-e2b-record-1`。离线结果为 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`：8,160 项核对、0 失败；Perfetto / Trace Processor 导入 2,025 个区间、201 条 Flow，OTLP 导出 2,025 个 Span、197 个 parent、4 个 link，最大时间误差 0 ns。

另有 5 个原生独立时钟区间（1 个 job、4 个 wait）没有 UTC 对齐锚点，保留在 Snapshot，未导入主时间轴或 OTLP；对应 9 条边因端点未导出而保留为未导出。覆盖状态仍为 unknown，不能把可投影子集称为完整全链 Trace；本次未新增 Jaeger 后端验收。

| 证据 | SHA-256 |
| --- | --- |
| 原 driver evidence | `28e51ef58d3641aba6a2b8b6ca30b726cc7c46e91e8895954618176335849fe2` |
| 原完整 Tape | `fd936f3f2e95aee443b36dbbe2be177401110f0fff1230267405a598b5b845e7` |
| 性能 summary.json | `a4987fc809efd407bcce3e7f03cf9f83520277e16b4c743a5861f1110899a560` |
| 离线双 Trace evidence | `4490712041dae3d6063198f30095c2300bd1ff826306fb4c14e28898eaf85c85` |
| 长结果归档修复定向验证 evidence | `a6f849539845e7f3b1220e5e13170ddb0235d0690e748194f8e2ad273bda3456` |

<a id="sqlite-e2b-replay-1"></a>

## SQLite terminal-result-v1：E2B replay-1

**同一完整 Tape 回放 FAIL，原私有评分 0/3、reward 0。** 使用 release8 与合并后的固定原生镜像 `e040654e…`，完成前 5 个模型轮及 5 次真实 exec；第 6 次模型请求（turn 5）被 Strict 拒绝，随后 2 次请求为 `SESSION_FAILED`。原 runner 返回 1。原 record-1 的评分 3/3、Tape 完整及观测验收 FAIL 均保持原样，没有重新录制或修补原件。

运行于 2026-10-26 05:21:01.388989—05:21:55.980629 UTC。停止、Bridge 撤权、唯一 Sandbox 删除及 provider/ledger 不存在确认全部完成，清理错误为 0。**Huawei 同 Tape replay-1 保持 UNRUN**，没有把 E2B 的失败当成可跨云比较的完整样本。

### 首次严格偏离

实际拒绝请求摘要与原生 ToolResult 按固定 dialect 的精确 ToolCall ID 对齐：`call_00_TVYANjfEAFVI6e3rpzpn5498`，请求路径 `$.messages[11].content`。录制与回放均为 32 行、1,917 字节，唯一不同是第 4 行目录列表中 `..` 的时间：`Oct 26 04:46 → Oct 26 05:21`；两份内容 SHA 分别为 `7d4cd6319896df4e5c9d25fcd8c7847bcd9f725265721ed46c1b68f2656141f0`、`611b472a40662791294b468500e61fc674b2395c6943e03cc452a38a23bc7304`。

该真实命令先解压原 SQLite 源码归档到 `/tmp`，再列出 `/tmp/sqlite/`，因此这行 `..` 指向 `/tmp`。binding 没有声明初始目录时间适配；解压本身还会新建 `/tmp/sqlite` 并更新父目录，所以仅固定初始 `/tmp` 时间不足以保证此输出可复现。公开附录已有避免偶发时间戳的指导，但本次 Agent 仍输出了长目录列表中的元数据。

前 5 个工具决策与原 Tape 的前缀完全一致，5 个归档 exec 均具有精确身份关联。尚未执行 `process.wait`，不能把偏离归因于后台终态协议、双流合并或长结果观测字段，也不能据此验证这些后续步骤。driver 的 `native_decisions_differ_from_real_tape` 是未执行完完整决策序列的后续验收错误，首因是上述真实 ToolResult 差异；Strict 保持原规则。

### 八阶段与逐次工具耗时

单位为秒，阶段名称与[统一阶段说明](performance-stages.md)一致。此回放提前失败，与完整录制的工作量不同；窗口不能简单相加或用来排名云厂商。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000013 |
| 任务 Sandbox 创建 | 1.658794 |
| 工作区准备 | 1.476844 |
| Harness 启动 | 7.297617 |
| Runtime 执行 | 18.580388 |
| 停止与撤权 | 5.578016 |
| 独立验证 | 12.649267 |
| 清理 | 0.762367 |

以下为这次回放全部 5 次 exec，单位为毫秒。SDK 等待和 guest 整体使用各自计时边界，不能相减推断网络时间；第 1 次真实退出码为 1，与录制前缀一致，并非本次 Strict 首偏离。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 465.476 | 133.314 | 1 | exact |
| 2 | 2 | 678.756 | 339.560 | 0 | exact |
| 3 | 3 | 890.524 | 619.055 | 0 | exact |
| 4 | 4 | 819.559 | 516.159 | 0 | exact |
| 5 | 5 | 942.735 | 661.234 | 0 | exact |

### 原件与双 Trace

原件目录 `/opt/aries-artifacts/tb-repro-sqlite-e2b-replay-1`。一次离线 Perfetto / Trace Processor 与 OTLP 核对完成 **1,665 项、0 失败**：403 个区间 / Span、36 条 Flow / parent、0 link，最大时间误差 0 ns。结论是 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`；本次未到后台协议，未留存协议 raw，因此 Snapshot 的 integrity/coverage 仍为 incomplete，不能补成完整覆盖。未新增 Jaeger 或 Prometheus 后端验收。

新分析目录中的三份投影与 driver 原投影逐字节相等，已通过硬链接复用，减少 7,511,984 字节重复占用；源事件、Tape、评分和失败记录均未修改。

| 证据 | SHA-256 |
| --- | --- |
| 原 driver evidence | `bc7f4716610dbbf7a1ebd1cdd3200b74c62644b34db7bfcee942fb23b0ccdb36` |
| 原完整 Tape | `fd936f3f2e95aee443b36dbbe2be177401110f0fff1230267405a598b5b845e7` |
| `replay-diagnostic-1.json` | `5b28d81165ea8379e93e8c36a2d419eb096bd48a50ac9245840581fccfc0f86b` |
| `replay-path-diagnostic-1.json` | `6efd0312ed8f158654330358461bed04d15eef7ad5067f0c87c8004c7f9fab5d` |
| `performance-review-1/summary.json` | `746817e589e02a8773df9b3109eba0333ca2c67639a523f6ad27f83fe7f37c0b` |
| `offline-analysis-1/evidence.json` | `706348580ffcebed6c2fc041dacd830843a96957d9818077aff7ea4a7fe4b1ca` |

## C++ metadata-portable-v1：华为云 record-1 / replay-1

**录制 PASS（原评分 6/6），同 Tape 回放 FAIL（原评分 5/6）。** 录制完成 46 个模型轮、45 次原生调用及结果，其中 42 次 exec、3 次 process；回放只完成第 1 个模型轮和第 1 次 exec，第 2 个模型请求发生真实 `REPLAY_DIVERGED`。后续 driver 的 `native_decisions_differ_from_real_tape` 是未执行完录制决策的验收结果，不是首因。两次均停止并撤权，owned Sandbox / ledger 不存在确认通过，清理错误为 0。

两次均为华为云 A / External、冻结 release7，使用相同完整 Tape 和原私有 Verifier。`metadata-portable-v1` 只追加公开元数据使用建议，未启用 `terminal-result-v1` 后台协议。这是华为云单云功能结果，不能算跨云性能对比；此前 E2B 创建 4,200 秒 Sandbox 被项目 1 小时限制拒绝的 **BLOCKED** 记录保持不变。

### 八阶段耗时

单位为秒，名称与[统一阶段说明](performance-stages.md)一致。回放提前失败，所以两列不是相同完成工作量的性能比较；独立验证仍真实运行，失败评分耗时保留。

| 阶段 | record-1 秒 | replay-1 秒 |
| --- | ---: | ---: |
| 排队 | 0.000014 | 0.000011 |
| 任务 Sandbox 创建 | 0.752136 | 0.613139 |
| 工作区准备 | 1.071966 | 1.037410 |
| Harness 启动 | 7.610571 | 7.442089 |
| Runtime 执行 | 235.890602 | 5.101152 |
| 停止与撤权 | 5.447225 | 6.110150 |
| 独立验证 | 34.651575 | 55.828334 |
| 清理 | 0.645038 | 0.687433 |

### 首差异与最小修复边界

原 Tape 中的 `/messages/3/content` 与首个原生 ToolResult 完全对应。录制和回放的结果都是 217 行、5,307 字节，唯一差异是第 3 行长目录列表中的 `..` 修改时间：`Oct 26 04:55 → Oct 26 05:04`。实际 Bridge 工作目录和 binding 工作目录都是 `/app`，因此该 `..` 是根目录 `/`。原 HTTP 拒绝 body 未归档；上述 JSON 路径来自 Tape 与精确 ToolCall 的内容对应，实际边界事件独立确认第 2 个请求严格偏离。

两次 binding 都未声明 `initial_directory_mtime_unix_ns`，也没有 `initial-directory-mtimes.json` 准备/读回回执。公开附录没有让 Agent 避开该目录元数据查询，因此本次初始环境仍不可复现。后续可直接使用已有 ExternalBridge 准备能力：新 binding 显式固定 `/` 的初始 mtime，声明环境适配并保留实际读回，再重新录制和回放。不能由旧输出的分钟补出未知秒/纳秒，不能改 Matcher 或过滤 ToolResult；这一准备也不保证运行中产生的时间、PID、后台状态自动相同。

| 首次 ToolResult | SHA-256 |
| --- | --- |
| record-1（5,307 B） | `5e17941f385f964aae537df2d3674f5dd0e0d9f46d7573f11a323206ae995019` |
| replay-1（5,307 B） | `77468f5540f63c923556dd25309025becf9efdb06844795e5d889a75b2213916` |

<details>
<summary>逐次 exec：录制 42 次、回放 1 次</summary>

单位为毫秒，模型轮从 1 起。SDK 等待与 guest 整体使用各自边界，不相减推断网络时间。录制第 21 次 SDK 等待被取消，guest 耗时/退出码未采集；3 次 process 不在此 exec 耗时表内，不能补为 0。

| 运行 | exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 / 缺测 |
| --- | --- | --- | ---: | ---: | --- | --- |
| record-1 | 1 | 1 | 196.875 | 47.616 | 0 | exact |
| record-1 | 2 | 2 | 129.926 | 41.405 | 0 | exact |
| record-1 | 3 | 3 | 1832.133 | 1741.373 | 0 | exact |
| record-1 | 4 | 4 | 453.809 | 364.920 | 0 | exact |
| record-1 | 5 | 5 | 146.447 | 50.177 | 0 | exact |
| record-1 | 6 | 6 | 146.824 | 40.212 | 0 | exact |
| record-1 | 7 | 7 | 3694.994 | 3603.869 | 0 | exact |
| record-1 | 8 | 8 | 590.793 | 500.054 | 0 | exact |
| record-1 | 9 | 9 | 858.689 | 674.920 | 0 | exact |
| record-1 | 10 | 10 | 363.287 | 272.459 | 0 | exact |
| record-1 | 11 | 11 | 137.070 | 46.660 | 0 | exact |
| record-1 | 12 | 12 | 145.342 | 48.255 | 0 | exact |
| record-1 | 13 | 13 | 133.897 | 43.724 | 0 | exact |
| record-1 | 14 | 14 | 135.819 | 46.846 | 0 | exact |
| record-1 | 15 | 15 | 132.475 | 44.978 | 0 | exact |
| record-1 | 16 | 16 | 856.687 | 657.248 | 0 | exact |
| record-1 | 17 | 17 | 1571.407 | 1481.870 | 0 | exact |
| record-1 | 18 | 18 | 1168.169 | 1034.004 | 0 | exact |
| record-1 | 19 | 19 | 359.744 | 268.878 | 0 | exact |
| record-1 | 20 | 20 | 396.121 | 305.287 | 0 | exact |
| record-1 | 21 | 21 | 22379.443 | 未采集 | 未采集 | SDK canceled，guest 终态未知 |
| record-1 | 22 | 25 | 956.826 | 861.493 | 0 | exact |
| record-1 | 23 | 26 | 797.278 | 704.550 | 0 | exact |
| record-1 | 24 | 27 | 442.701 | 294.415 | 0 | exact |
| record-1 | 25 | 28 | 377.137 | 283.902 | 0 | exact |
| record-1 | 26 | 29 | 128.016 | 35.836 | 0 | exact |
| record-1 | 27 | 30 | 732.428 | 642.387 | 0 | exact |
| record-1 | 28 | 31 | 978.642 | 884.747 | 0 | exact |
| record-1 | 29 | 32 | 3264.009 | 3172.807 | 0 | exact |
| record-1 | 30 | 33 | 562.217 | 416.794 | 0 | exact |
| record-1 | 31 | 34 | 781.407 | 637.396 | 0 | exact |
| record-1 | 32 | 35 | 1059.453 | 968.007 | 0 | exact |
| record-1 | 33 | 36 | 736.401 | 645.087 | 0 | exact |
| record-1 | 34 | 37 | 1048.362 | 954.913 | 0 | exact |
| record-1 | 35 | 38 | 401.329 | 310.824 | 0 | exact |
| record-1 | 36 | 39 | 581.736 | 488.840 | 0 | exact |
| record-1 | 37 | 40 | 699.358 | 509.908 | 0 | exact |
| record-1 | 38 | 41 | 895.629 | 748.093 | 0 | exact |
| record-1 | 39 | 42 | 852.780 | 708.013 | 2 | exact |
| record-1 | 40 | 43 | 3896.446 | 3802.752 | 0 | exact |
| record-1 | 41 | 44 | 136.095 | 44.856 | 0 | exact |
| record-1 | 42 | 45 | 1359.669 | 1266.690 | 0 | exact |
| replay-1 | 1 | 1 | 194.149 | 45.926 | 0 | exact |

</details>

### 原件与离线双 Trace

原件目录分别为 `/opt/aries-artifacts/tb-repro-cpp-huawei-record-1`、`/opt/aries-artifacts/tb-repro-cpp-huawei-replay-1`。各只生成一次离线对账：录制 17,744 项检查、4,423 个区间 / Span、242 条 Flow / parent；回放 844 项检查、198 个区间 / Span、20 条 Flow / parent，均 0 失败、最大时间误差 0 ns。两项结论均为 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`，integrity 为 complete、coverage 为 unknown；这只证明已采证据的投影一致，不会把真实回放失败改为成功，本次未做新的 Jaeger 后端验收。

每次原件与离线目录中 3 份 SHA 相同的投影已用硬链接共享，保留所有原路径和内容，不保存重复大包。小型汇总保存在本地 `.cache/tb-repro-cpp-audit-1/`，不含命令正文、评分源码或凭据。

| 证据 | SHA-256 |
| --- | --- |
| record-1 driver evidence | `1ac7c7acf43e2157a540d5c0ba035a0c8836b9074d13f66a2d55be21228c14df` |
| replay-1 driver evidence | `9f703fc2cd3bd1bcc4349cc84cec39f1621a7e45cf0dbff52bf100f938a2e535` |
| 共同录制 Tape | `cc5a32375454d98a3b309c57d944fb897287d30165eea8b454a69f262a3c9ad2` |
| record-1 性能 summary.json | `42ceb306339590f0b5bbd2c50ce60de4efbba960900d5e9a9e6fe7915d275cdf` |
| record-1 离线双 Trace evidence | `666745d0005d565be512d66d0c12c1b62cbdc7e99bc81f5e266a49d343b8c0f2` |
| replay-1 性能 summary.json | `1358ead64299371322c621d331e3b578b43082ca6cace604d54de640f2637aac` |
| replay-1 离线双 Trace evidence | `8a358b0815d4e2124c7ef1c031b5ff1c27d96f4c02675f921f1130c2acc01321` |

<a id="cpp-huawei-record-2"></a>

## C++ metadata-portable-v1：华为云 record-2

**原独立评分 6/6、reward 1，Tape 完整；整体仍为 FAIL_OBSERVATION。** 本次使用 release8，完成 63 个模型轮和 67 次原生调用及结果（64 次 exec、3 次 process），原 runner 返回 0。验收在 `archived_exec_identity_incomplete` 失败：63 次 exec 精确关联；首次 exec 的工具开始和错误终态存在，缺的是它与辅助初始化执行之间的精确关联。不能将成功解题记为完整观测通过，replay-2 尚未启动。

运行于 2026-10-26 05:25:30.689050—05:32:52.086699 UTC，沿用华为云 A / External、1 CPU / 2048 MiB、原任务预算和原私有 Verifier。仅使用公开元数据附录，没有启用 `terminal-result-v1` 后台协议。与上一轮分开保留，不计跨云配对、容量或 Goodput 成绩。

### 初始环境与八阶段耗时

新 binding 显式将 `/app` 的父目录 `/` 初始 mtime 设为 `1704067200000000000 ns`（2024-01-01 00:00:00 UTC）。真实准备命令 exit 0，读回值完全一致，原 atime 保持 `1792992334619000820 ns`。该值是新实验事先声明的环境值，没有从旧 Tape 的分钟猜测秒或纳秒；运行中产生的目录时间、PID 和后台状态仍可能变化。

下表单位为秒，阶段名称与[统一阶段说明](performance-stages.md)一致。原 Attempt 总时长为 **421.244850 秒**；Runtime 使用原 `runtime_execution_duration`，各阶段窗口不能简单相加替代端到端测量。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000030 |
| 任务 Sandbox 创建 | 0.640310 |
| 工作区准备 | 0.877557 |
| Harness 启动 | 7.207971 |
| Runtime 执行 | 361.769517 |
| 停止与撤权 | 5.397324 |
| 独立验证 | 43.702177 |
| 清理 | 0.612384 |

### SSH 255 与身份缺口：已证事实和推断分开

首次 exec 是目录查看类命令。按精确 ToolCall ID `call_00_FXmfiO6hT2QK5MHSR2xV0924`，原生 transcript 第 6 行为调用、第 7 行为 `isError=true` 的结果，错误为 `ssh exited with code 255`；身份 producer 第 1 行只记录 invocation started。这不是已证明的策略拒绝，不能补记为未派发成功观测。

再次直接核对原 `harness/native-tool-events.jsonl`，该 ToolCall 的 `tool/start`、`tool/result`（`is_error=true`）和 `command_output/end` 均存在；它们的 `process_completion` 仍是 unknown，退出码和原生执行时长为空。事件索引为 complete、198 条、issues 为空，`terminal_observed=true` 指 Agent run 生命周期结束，不是 guest 进程退出证明。索引 SHA 为 `5c55317acdb96ab64ba0abb284e1ca0b5a48384f0ffa5f928ce4230796ba0331`，事件文件 SHA 为 `7ceac86f5f6564716d97e1b7979d6f0c7b85b8160d195e9174ee5cb021a72de4`。身份 producer 本身只记录 invocation / 显式拒绝，不能把它没有通用终态记录误报为原生 ToolResult 丢失。

另一个可独立核验的执行链是原生初始化检查 `openclaw-sandbox-check`，execution ID 为 `7QSSGCEAKYQWDHPPM5QAHSK2ZT`：Bridge 第 1 条记录耗时 117 ms、stdout 2 B、stderr 0 B，最终为 canceled / 255；同一执行 ID 的 SDK 第 61 行却已记录 `exec.wait succeeded / exit 0`，第 62 行随后记录 stdin canceled。固定 release8 的 `pkg/sandbox/e2b/client.go:665` 在 Wait 返回后主动取消 stdin，再读取输入错误；其竞态路径会把内部取消合并到已成功的执行结果，Bridge 随后返回 255。该系统边界已完成最小顺序修复，旧版复现及候选相关包unit/race通过；五项发布检查也已通过；新 CPP v2 实录已完成原评分 6/6 与 34/34 exec 身份验收；其同 Tape 回放因 gdb 输出顺序偏离失败，没有复现该 SSH 255，见[回放归档](#cpp-v2-huawei-replay-1)及[修复证据](observability/VALIDATION_AND_REPRODUCE.md#execstream-teardown-fix)。没有忽略真实输入错误、放宽Strict或改写旧结果。

原生初始化检查在实际工具身份文件写入之前。旧记录没有将该辅助检查与首次 ToolCall 连接起来，因此两段各有精确身份，但两段之间只依据源码顺序推断；不由命令哈希或时间接近补边。首次 exec 的 SDK / guest 耗时继续保留未知，辅助检查的 117 ms 也不挪用到该工具行。新 Tape 保留了真实失败与随后成功解题过程，是否能严格复现需另行实测。

<details>
<summary>逐次 exec：64 次真实调用</summary>

单位为毫秒，模型轮从 1 起；同轮多工具保留原顺序，不假定并行。SDK 等待与 guest 整体是不同计时边界，不能相减推断网络耗时。第 1 次缺少精确关联；第 21 次虽已精确关联，SDK 等待被取消，guest 终态、耗时和退出码未采集。3 次 process 不计入本表，其耗时不能补成 0。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 / 缺测 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 未知 | 未知 | 未采集 | 缺少精确 Bridge 关联 |
| 2 | 2 | 136.472 | 46.268 | 0 | exact |
| 3 | 3 | 153.993 | 46.311 | 0 | exact |
| 4 | 3 | 144.176 | 27.194 | 0 | exact |
| 5 | 4 | 1668.661 | 1560.767 | 0 | exact |
| 6 | 4 | 1697.874 | 1588.055 | 0 | exact |
| 7 | 5 | 476.328 | 369.438 | 0 | exact |
| 8 | 5 | 210.426 | 94.315 | 0 | exact |
| 9 | 6 | 675.781 | 593.529 | 0 | exact |
| 10 | 7 | 139.529 | 29.169 | 0 | exact |
| 11 | 7 | 153.596 | 43.533 | 0 | exact |
| 12 | 8 | 133.382 | 40.951 | 0 | exact |
| 13 | 9 | 156.699 | 46.614 | 0 | exact |
| 14 | 9 | 164.408 | 50.497 | 0 | exact |
| 15 | 10 | 767.872 | 613.100 | 0 | exact |
| 16 | 11 | 197.614 | 59.714 | 0 | exact |
| 17 | 12 | 347.888 | 255.355 | 0 | exact |
| 18 | 13 | 425.468 | 274.277 | 0 | exact |
| 19 | 14 | 226.567 | 137.643 | 0 | exact |
| 20 | 15 | 316.082 | 220.571 | 0 | exact |
| 21 | 16 | 29659.321 | 未知 | 未采集 | SDK canceled，guest 终态未知 |
| 22 | 20 | 149.675 | 60.295 | 0 | exact |
| 23 | 21 | 289.535 | 203.128 | 0 | exact |
| 24 | 22 | 293.820 | 203.503 | 0 | exact |
| 25 | 23 | 424.833 | 341.038 | 0 | exact |
| 26 | 24 | 418.827 | 334.839 | 0 | exact |
| 27 | 25 | 464.696 | 372.796 | 0 | exact |
| 28 | 26 | 487.101 | 396.184 | 0 | exact |
| 29 | 27 | 528.437 | 405.697 | 0 | exact |
| 30 | 28 | 120.870 | 39.448 | 0 | exact |
| 31 | 29 | 1082.242 | 991.438 | 0 | exact |
| 32 | 30 | 474.505 | 384.392 | 0 | exact |
| 33 | 31 | 509.978 | 420.062 | 0 | exact |
| 34 | 32 | 144.918 | 49.957 | 0 | exact |
| 35 | 33 | 129.037 | 40.763 | 0 | exact |
| 36 | 34 | 146.409 | 40.376 | 0 | exact |
| 37 | 35 | 549.495 | 459.747 | 0 | exact |
| 38 | 36 | 364.645 | 280.958 | 0 | exact |
| 39 | 37 | 136.149 | 46.821 | 0 | exact |
| 40 | 38 | 378.814 | 288.472 | 0 | exact |
| 41 | 39 | 419.807 | 326.922 | 0 | exact |
| 42 | 40 | 586.157 | 441.752 | 0 | exact |
| 43 | 41 | 297.548 | 206.010 | 0 | exact |
| 44 | 42 | 131.904 | 42.536 | 1 | exact |
| 45 | 43 | 237.587 | 147.855 | 0 | exact |
| 46 | 44 | 293.419 | 202.862 | 0 | exact |
| 47 | 45 | 131.367 | 42.685 | 0 | exact |
| 48 | 46 | 138.828 | 50.037 | 0 | exact |
| 49 | 47 | 144.488 | 44.229 | 0 | exact |
| 50 | 48 | 233.104 | 45.541 | 0 | exact |
| 51 | 49 | 145.440 | 55.911 | 0 | exact |
| 52 | 50 | 144.153 | 55.098 | 0 | exact |
| 53 | 51 | 430.055 | 339.771 | 0 | exact |
| 54 | 52 | 130.768 | 41.723 | 0 | exact |
| 55 | 53 | 127.348 | 37.902 | 0 | exact |
| 56 | 54 | 132.179 | 43.501 | 1 | exact |
| 57 | 55 | 131.125 | 41.682 | 0 | exact |
| 58 | 56 | 176.988 | 87.462 | 0 | exact |
| 59 | 57 | 576.026 | 484.177 | 0 | exact |
| 60 | 58 | 202.089 | 61.417 | 0 | exact |
| 61 | 59 | 840.665 | 748.443 | 0 | exact |
| 62 | 60 | 834.777 | 743.515 | 0 | exact |
| 63 | 61 | 3238.500 | 3147.241 | 0 | exact |
| 64 | 62 | 1290.704 | 1198.945 | 0 | exact |

</details>

### 原件、清理与离线双 Trace

原件目录 `/opt/aries-artifacts/tb-repro-cpp-huawei-record-2`。停止与 Bridge 撤权均 confirmed；实际创建 1 个 Sandbox，provider 与 ledger 均确认不存在，清理错误为 0，部署锁已实际验证释放。后续仅做离线分析，没有新增 Sandbox 或模型调用。

使用 release8 `aries-perf` 对原件生成一次离线对账：**29,252 项检查、0 失败**；Perfetto / Trace Processor 导入 7,300 个区间、327 条 Flow，OTLP 导出 7,300 个 Span、327 个 parent、0 link，最大导入/结束时间误差均为 0 ns。三种文件均来自同一版本化 Snapshot。结论为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`，integrity / coverage 均为 incomplete；投影一致没有修复首工具的因果缺口。

本次没有因缺少锚点而被排除的已采区间，但不代表未采的 guest / 原生时钟已完成跨域校准；仍不跨时钟相减，不补造全链时间轴。本次 Jaeger、Prometheus 后端查询和浏览器验证均 UNRUN。三份重复投影在 SHA 完全相同后共享硬链接，保留原路径和内容，回收 **232,964,096 字节**重复分配；没有重复大包或改写原失败。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `5ab1b4351b15c8ba902b00a180e5bad9b9960e44ad2762b22d107cc4e1eeb903` |
| 原完整 Tape | `5d75cbe0782eed264a1418a8e3e4220e61059366ac734348e9c6e2bb67ecd85b` |
| `operator-review-1.json` | `83c237929d8b040c17f6ba1b8d5245e304eb6b1852323569596be191e03813d3` |
| `bridge/initial-directory-mtimes.json`（run occurrence 内） | `35f397d4f801b265e181eeeca5bb066334645876c8350300a0c9e64823c7552e` |
| `performance-review-1/summary.json` | `dd9f1b871eabf31c2ad995d6786d35c2e46d458ab03c9631f226124c7e5e1454` |
| `offline-analysis-1/evidence.json` | `1bef3654e3eb5d7127b2751416bcad51b8ae8748e4d13865a503c23f5c598f35` |
| `projection-dedup-1/evidence.json` | `0954445dc88b624baa65750f7abfea65ce45e017fe958b3e568774804e90119d` |
| release8 `source-manifest.json` | `89d8b62871afc87fbb32c9cb9bef1b040a394a9e1af88d3aaca0f0e4d3ffe9b2` |

本地小型汇总为 `.cache/tb-repro-cpp-record2-audit-1/summary.json` 和 `summary.md`，仅含状态、计数、阶段、工具时长及来源哈希，不含命令正文、私有评分源码或凭据。

<a id="cpp-v2-huawei-record-1"></a>

## C++ terminal-result-v2：华为云 record-1

**本次真实录制 PASS：原独立评分 6/6、reward 1，完整 Tape 与 34/34 次 exec 精确身份对账通过。** 使用 release10 和相同 `e040654e…` 原生镜像，完成 35 个模型轮、34 次原生调用及结果，全部为 exec；没有模型请求失败、缓存重试或未知 exec 身份。原 runner 返回 0，driver 为 `PASS_TB_VARIANT_SINGLE_RUN`，身份验收为 `PASS_ARCHIVED_TOOL_IDS`。此结论限定本次实录解题、归档 exec 身份及清理，不代表严格回放或后台协议已通过。

真实运行时间为 2026-10-26 06:17:22.634387—06:22:09.530901 UTC。终态核查时 unit 已不存在、MainPID 为 0，`/proc` 未发现对应 driver；实际创建 1 个 Sandbox，停止、Bridge 撤权、provider 与 ledger 不存在确认均通过，清理错误为 0。没有因状态查询重新启动任务。

### 变体与实际协议覆盖

新版公开附录复用 text / SQLite v2 同一 SHA `0a7859d75d23a4497ce17a894c2b7b1fe5a3fff968cb1db931c0ea6cdf619c05`。旧 CPP 录制确有自动 yield 后的随机句柄 / PID，以及 poll / log 动态闲置秒数进入 Tape，因此新增独立交互变体；原题、私测、1,800 + 1,800 秒预算与 4,200 秒 Sandbox 窗口不变，poll / log / kill / PTY 继续可用。公开说明不保证 Agent 一定遵从，也不把必要故障诊断当作无关信息。

本次配置启用了 `terminal-result-v1`，但 **accepted job = 0、wait call = 0、process call = 0**，实际状态为 `NOT_EXERCISED`。协议 raw 未生成，collection index 保留 `not_collected / producer_artifact_missing`；Snapshot 的 integrity / coverage 仍为 incomplete，原 Attempt 的 `artifact_proven=false`。不能把“未触达”写成后台终态协议通过，也不能因为 34 次前台 exec 关联完整就改写全链完整性。

`/app` 的父目录 `/` 初始 mtime 继续显式固定为 `1704067200000000000 ns`，真实设置命令 exit 0、读回相同，原 atime `1792995446994446582 ns` 保留。新运行与旧 metadata-v1 分开统计，旧 Tape 和失败均保留；本次尚不构成双云性能配对或容量结论。

### 八阶段耗时

单位为秒，名称与[统一阶段说明](performance-stages.md)一致。原 Attempt 总时长 **272.746285 秒**；Runtime 取真实 `runtime_execution_duration`，各阶段不能直接求和替代端到端计时。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000011 |
| 任务 Sandbox 创建 | 0.766560 |
| 工作区准备 | 1.203679 |
| Harness 启动 | 7.181552 |
| Runtime 执行 | 220.389530 |
| 停止与撤权 | 5.385960 |
| 独立验证 | 36.381532 |
| 清理 | 0.680062 |

<details>
<summary>逐次 exec：34 次调用，全部精确关联</summary>

单位为毫秒，模型轮从 1 起。每行保留该工具的实际退出码；过程中的非零退出不等于最终任务失败，原独立评分仍为 6/6。SDK 等待与 guest 整体来自不同计时边界，不能相减推断网络时延；本次没有 process / wait 耗时样本。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 202.820 | 53.395 | 0 | exact |
| 2 | 2 | 132.725 | 44.122 | 0 | exact |
| 3 | 3 | 1842.726 | 1752.383 | 0 | exact |
| 4 | 4 | 496.270 | 365.418 | 0 | exact |
| 5 | 5 | 945.210 | 854.361 | 0 | exact |
| 6 | 6 | 147.163 | 56.587 | 0 | exact |
| 7 | 7 | 184.686 | 46.494 | 0 | exact |
| 8 | 8 | 146.386 | 45.911 | 0 | exact |
| 9 | 9 | 837.935 | 754.345 | 0 | exact |
| 10 | 10 | 134.617 | 45.064 | 0 | exact |
| 11 | 11 | 127.391 | 44.359 | 0 | exact |
| 12 | 12 | 191.326 | 42.402 | 0 | exact |
| 13 | 13 | 764.134 | 629.815 | 0 | exact |
| 14 | 14 | 350.219 | 259.043 | 0 | exact |
| 15 | 15 | 394.886 | 303.556 | 0 | exact |
| 16 | 16 | 401.981 | 309.676 | 0 | exact |
| 17 | 17 | 128.902 | 39.198 | 0 | exact |
| 18 | 18 | 401.284 | 311.014 | 0 | exact |
| 19 | 19 | 570.345 | 479.416 | 0 | exact |
| 20 | 20 | 349.252 | 258.886 | 1 | exact |
| 21 | 21 | 339.890 | 249.863 | 0 | exact |
| 22 | 22 | 333.537 | 243.494 | 0 | exact |
| 23 | 23 | 339.800 | 255.977 | 0 | exact |
| 24 | 24 | 132.366 | 43.024 | 0 | exact |
| 25 | 25 | 369.339 | 279.064 | 0 | exact |
| 26 | 26 | 386.505 | 294.629 | 0 | exact |
| 27 | 27 | 408.366 | 317.912 | 0 | exact |
| 28 | 28 | 775.982 | 644.634 | 0 | exact |
| 29 | 29 | 1062.499 | 915.969 | 0 | exact |
| 30 | 30 | 831.270 | 740.992 | 0 | exact |
| 31 | 31 | 807.359 | 716.263 | 0 | exact |
| 32 | 32 | 3294.896 | 3204.191 | 0 | exact |
| 33 | 33 | 1585.318 | 1494.036 | 0 | exact |
| 34 | 34 | 133.640 | 44.238 | 0 | exact |

</details>

### 一次离线双 Trace 与归档

原件目录 `/opt/aries-artifacts/tb-repro-cpp-v2-huawei-record-1`。使用本次固定 release10 的 `aries-perf`，一次离线核对完成 **18,345 项、0 失败**：Perfetto / Trace Processor 导入 4,573 个区间、187 条 Flow；OTLP 导出 4,573 个 Span、187 个 parent、0 link；最大导入与结束时间误差均为 0 ns。所有导出来自同一版本化 Snapshot，结论为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`，不会把协议未触达和缺失原始材料改成完整观测。

本次现有投影集合没有被排除的区间，仍不能由此声称 guest / 原生时钟已跨域校准；`guest_clock_alignment=NOT_INFERRED` 保持原样。Jaeger、Prometheus 后端查询及浏览器验证均为本次 UNRUN。分析结束后，SHA 相同的三份投影共享硬链接，保留全部路径，释放 **131,768,320 字节**重复分配；原始 Tape、事件、评分和失败历史未改动，没有新增云资源或模型调用。

材料冻结于 `tb-repro-cpp-workload-v2-1`，manifest SHA `829840968d98686a25cde71da1ce66716d320633a941138e0084f6c3aab1eb2e`，传输验证 SHA `4198d874db80d794134053bdb9c4f6fdc35ad818c8639249ddf1350568bd2398`；最终任务文本 SHA `c41c7c655eb27c9e195f7a3086075eed72c1e26f66a6307fdcf14f630ef1d082`。材料准备与真实验收分别留证。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `b6c52d186cb1709cacc571a77d5cc57c71a4c4ca87e8afe2a603b8d06fc88423` |
| `result-review.json` | `d22e9ab7b54ad49df03bc200878c2f6233ce7c1206a0b5d985f239a7d9cdfb0b` |
| 原完整 Tape | `ea1a8d1b0f1147e3108b7fd21a965c890d07f2e206e8db6de770d6b686a867df` |
| `bridge/initial-directory-mtimes.json`（run occurrence 内） | `5683a76d4d844a3a22dff66899d86760ca20ae5da7ab9bcdddbe6aa32a892ca3` |
| `performance-review-1/summary.json` | `3ad581bae42a79bedd38c93d14d9e326043a628efb47d84c7abe5f6dd481e4af` |
| `offline-analysis-1/evidence.json` | `efd5fb29962a0e8f71efa5fca5a92f96e7747aa69417468b925fcd5a5c2aa838` |
| `projection-dedup-1/evidence.json` | `568d3ece95bae16f2b2f3a7505c372f562b465edd8ef614abc2112167805a677` |
| release10 `evidence.json` | `19c1c695ccfa51cd32cf5c6b9784d6f4537ae0e9d57a8fc7ad01635e87432405` |
| release10 `source-manifest.json` | `0863298927cd663b5a91cca47e49c59e120e45fd90d4fcb83b137f5c9bb7d797` |

本地小型汇总：`.cache/tb-repro-cpp-v2-record1-audit-1/summary.json`、`summary.md`。本节只覆盖新实录，后续严格回放的失败与首差异见[独立回放归档](#cpp-v2-huawei-replay-1)。

<a id="cpp-v2-huawei-replay-1"></a>

## C++ terminal-result-v2：华为云 replay-1

**同 Tape 严格回放 FAIL：原评分 5/6、reward 0；已执行的 5/5 次 exec 身份完整。** 第 6 次模型请求（从 0 起的 turn 5）因真实 ToolResult 内容顺序变化被 Strict 拒绝，后续 2 次请求为 `SESSION_FAILED`。前 5 个模型轮成功，5 个工具决策与完整录制 Tape 的前缀完全一致；完整 Tape 预期 34 次 exec，其余 29 次未执行，不能记成缺失身份或成功样本。

本次沿用 release10、相同 `e040654e…` 原生镜像、相同 binding 与完整 Tape，模型时序为 `recorded`；实际没有上游模型请求。运行于 2026-10-26 06:24:46.247013—06:25:55.877770 UTC，unit 终态 failed / MainPID 0，原 runner 返回 1。停止、Bridge 撤权、唯一 Sandbox 删除及 provider / ledger 不存在确认全部完成，清理错误为 0，没有 fallback 残留回收。

### 首差异：命令内部合流后的 gdb 提示顺序

拒绝摘要的路径是 `$.messages[11].content`，按固定 OpenClaw dialect 的精确 ToolCall ID 对应第 5 次 exec：`call_00_wMYrddcSIOQ0iohF67eF1088`。录制与回放的模型可见结果均为 **39 行、1,925 字节**；唯一变化是同一两行 libthread_db 通知（文字合计 119 字符，不计换行），从录制第 21–22 行移到回放第 16–17 行。它们是“启用线程调试”和“使用宿主线程库”的提示，不是 warning、编译错误或 PID / 地址 / 日期变化。所有行及重复次数完全相同，内容顺序确实不同，Strict 拒绝符合原契约。

实际命令形状为 `gdb --batch … 2>&1 | tail …`。按同一精确 ToolCall ID，两次 Bridge 均记录 stdout 1,926 字节、stderr 0 字节、completed / exit 0；Native ToolResult 使用 `stdout-stderr-v1`，没有截断。因此输出在 guest 命令管道里已经合成一个 stdout，顺序变化进入 native renderer 之前就已发生，不能归因为 native 双流重排，也不是上一轮 SSH 255 / 身份缺失再次发生。本轮不修改 I/O、追加排序或过滤输出，不放宽 matcher，不重录来覆盖失败。

两次初始 `/` mtime 都真实读回 `1704067200000000000 ns`，本次 atime `1792995890870454020 ns` 保留；本次首差异与初始目录时间无关。没有 process / wait 调用，后台终态协议未触达。driver 的 `native_decisions_differ_from_real_tape` 是完整决策序列未执行完的后续验收错误，不能代替上述首因。

| 首差异证据 | SHA-256 |
| --- | --- |
| 录制 ToolResult（1,925 B） | `fd89937f53ab50401cc3f86429c4a2795db1c76e1a42ed531e386e7f8ccd1413` |
| 回放 ToolResult（1,925 B） | `0e97e6544d2d1af5f64184148aa7ee5baf77014343089b9033449231cda5b26f` |
| 移动的同一两行块（不含末尾换行） | `d88aa9c80248edab65f4098b375eedc381d4b16c021d3ef6d8989572c44ff5db` |

### 八阶段与逐次工具耗时

单位为秒，阶段名称与[统一阶段说明](performance-stages.md)一致。Attempt 总时长 **64.675089 秒**；本次只完成录制的工具前缀，不能与完整录制的耗时当作等量工作性能对比，也不能由阶段求和替代端到端计时。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000018 |
| 任务 Sandbox 创建 | 0.667461 |
| 工作区准备 | 0.757049 |
| Harness 启动 | 7.145875 |
| Runtime 执行 | 18.173315 |
| 停止与撤权 | 5.382420 |
| 独立验证 | 30.655772 |
| 清理 | 0.578361 |

以下为实际执行的全部 5 次 exec，单位为毫秒。SDK 与 guest 使用各自计时边界，不能相减推断网络时延。5 次都真实 exit 0、精确关联，gdb 调用成功执行不等于其返回内容能通过 Strict。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 154.159 | 47.436 | 0 | exact |
| 2 | 2 | 126.083 | 41.408 | 0 | exact |
| 3 | 3 | 1781.339 | 1695.226 | 0 | exact |
| 4 | 4 | 482.343 | 360.825 | 0 | exact |
| 5 | 5 | 657.490 | 569.327 | 0 | exact |

### 一次离线双 Trace 与原件

原件目录 `/opt/aries-artifacts/tb-repro-cpp-v2-huawei-replay-1`。单次离线 Perfetto / Trace Processor 与 OTLP 核对完成 **1,905 项、0 失败**：463 个区间 / Span、36 条 Flow / parent、0 link，最大导入与结束时间误差为 0 ns。两种 Trace 使用同一版本化 Snapshot，结论为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`；源 integrity / coverage 仍为 incomplete，保留协议 raw 未采集和未触达事实，不将可投影子集视为完整链路。

guest / 原生时钟未据此跨域校准；Jaeger、Prometheus 后端查询和浏览器验证均为本次 UNRUN。三份投影校验 SHA 相同后共享硬链接，保留原路径，释放 **8,822,784 字节**重复分配；原 Tape SHA 未变，原录制成功、回放失败与原评分均保留。离线分析结束后已释放 ECS 工作窗口，本分支没有启动任何重跑或其他付费任务。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `2c14911287019e3d7d6586d5a2f29a7b688e37ecdc0fd26a7e991ce6b8ac022a` |
| 原完整 Tape（record-1 目录内） | `ea1a8d1b0f1147e3108b7fd21a965c890d07f2e206e8db6de770d6b686a867df` |
| `replay-diagnostic-1.json` | `6221a9d23d1dcfbca19f0d3a6b0fccd87cd96ea24399b61022be52ca0060495d` |
| `replay-shape-diagnostic-1.json` | `b92b649614a967f9da632de8b4c88ce7037ff05fb66236117962ec22a5744843` |
| `performance-review-1/summary.json` | `bea7882c75eae39a28eb9622e863f57c1f91fd46aa230141be56dbe03611655c` |
| `offline-analysis-1/evidence.json` | `86ed9ed9ac4a4e43ea20982d72c8fd825e14333cb91fd73fa710797486ee6464` |
| `projection-dedup-1/evidence.json` | `6aa1d2e9f042cbc740abb75c62835a6bc6fe44f1efd498f8b2649c36d7846f38` |

本地小型汇总为 `.cache/tb-repro-cpp-v2-replay1-audit-1/summary.json`、`summary.md` 和 `diagnostic.json`，不含原工具正文或凭据。


<a id="sqlite-v2-e2b-record-1"></a>

## SQLite terminal-result-v2：E2B record-1

**真实录制通过：原评分 3/3、reward 1，36/36 次 exec 精确关联，后台 job / wait 映射通过，Tape 完整。** 本次为独立变体 `sqlite-with-gcov-terminal-result-v2`，使用 release10 与固定 `e040654e…` 原生镜像，仍保留原任务、私有 Verifier 和真实工具执行。共 35 个模型轮、37 次原生调用及结果（36 次 exec、1 次 process/wait），没有未解析的 exec 身份或派发前拒绝。它是单次 E2B 实录结果；后续[同 Tape E2B 严格回放](#sqlite-v2-e2b-replay-1)因安装产物时间偏离失败，华为云配对尚未完成，不将本次写成跨云性能比较成功。

运行于 2026-10-26 06:42:43.693343—06:47:43.767383 UTC。Harness、独立验证、停止及 Bridge 撤权均成功；实际仅创建 1 个 Sandbox，provider 与 ledger 均确认不存在，清理错误为 0。保留此前 SQLite v1 归档缺字段及目录时间回放失败记录，不用本次成功覆盖它们。

### 后台等待与长结果归档：这次实际覆盖了什么

后台协议原件有 4 条连续记录：accepted、wait_started、terminal_result、wait_finished 各 1 条；精确关联到同一 exec 和独立 wait 调用，验收为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。真实原生终态为 completed / exit 0，`outputRenderer=stdout-stderr-v1`、`outputTruncated=false`。这证明原生 Promise（含 finalize）与归档结果一致，不把该信号单独当作 guest 退出证书。

本次确实触达旧 **8,192 字节归档截断路径**。wait ToolCall `call_00_QA0YnizwPdeteFZIK9lV0943` 的原生 JSONL 保留 `persistedDetailsTruncated=true`、`originalDetailsBytesAtLeast=8193`；裁剪后的 details 为 494 字节，且 `protocol`、`outputRenderer`、`outputTruncated` 三个字段均完整，与协议 terminal_result 逐字段一致。模型可见文本为 8,103 字节；details 的裁剪与 `outputTruncated=false` 属于不同对象，不相互矛盾。没有提高阈值、扩大正文保留或改写旧结果。

在原生协议自己的单调时钟内，job 从 accepted 到 terminal_result 为 **121.528054 秒**，wait 从 wait_started 到 wait_finished 为 **119.482785 秒**。两者重叠，不能相加；该时钟没有可信 UTC 锚点，不能与控制器时钟相减推导网络或调度时间。此范围的映射通过，producer 总丢失数和独立调用分母仍为 unknown。

### 八阶段与逐次工具耗时

阶段单位为秒，对应[统一阶段说明](performance-stages.md)。Attempt 总时长 **291.255933 秒**；Runtime 使用实际 `runtime_execution_duration`，各阶段窗口不能简单相加替代端到端测量。本次 ECS 使用共享开发环境，不作为独占资源容量成绩。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000016 |
| 任务 Sandbox 创建 | 1.186456 |
| 工作区准备 | 1.116674 |
| Harness 启动 | 7.347952 |
| Runtime 执行 | 260.872611 |
| 停止与撤权 | 5.551868 |
| 独立验证 | 13.027706 |
| 清理 | 0.716158 |

<details>
<summary>逐次 exec：36 次真实调用</summary>

单位为毫秒，模型轮从 1 起；同轮调用保留原顺序，不据此断言并行。SDK 等待与 guest 整体来自不同计时边界，不能相减当作网络耗时。第 20 次为后台 job 对应的真实执行；其 SDK / guest 终态已经采到，process/wait 的独立计时见上节，不能再加到 job 耗时上。第 23 次真实 exit 1 被保留，不因最终任务成功而改成 0。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 473.083 | 124.773 | 0 | exact |
| 2 | 1 | 653.364 | 343.176 | 0 | exact |
| 3 | 2 | 1059.873 | 810.775 | 0 | exact |
| 4 | 2 | 334.876 | 50.912 | 0 | exact |
| 5 | 3 | 798.623 | 535.046 | 0 | exact |
| 6 | 4 | 918.449 | 665.006 | 0 | exact |
| 7 | 5 | 298.861 | 39.733 | 0 | exact |
| 8 | 6 | 440.201 | 36.926 | 0 | exact |
| 9 | 7 | 297.552 | 47.092 | 0 | exact |
| 10 | 8 | 450.935 | 41.774 | 0 | exact |
| 11 | 9 | 324.598 | 50.550 | 0 | exact |
| 12 | 10 | 295.106 | 35.019 | 0 | exact |
| 13 | 11 | 299.472 | 45.539 | 0 | exact |
| 14 | 12 | 292.808 | 38.507 | 0 | exact |
| 15 | 13 | 292.493 | 40.655 | 0 | exact |
| 16 | 14 | 307.004 | 36.853 | 0 | exact |
| 17 | 15 | 664.298 | 404.514 | 0 | exact |
| 18 | 16 | 3906.592 | 3647.516 | 0 | exact |
| 19 | 17 | 345.436 | 91.633 | 0 | exact |
| 20 | 18 | 121520.955 | 121253.208 | 0 | exact |
| 21 | 20 | 325.914 | 51.322 | 0 | exact |
| 22 | 21 | 316.642 | 55.221 | 0 | exact |
| 23 | 22 | 666.329 | 415.690 | 1 | exact |
| 24 | 23 | 345.103 | 88.781 | 0 | exact |
| 25 | 24 | 297.246 | 40.875 | 0 | exact |
| 26 | 25 | 300.711 | 47.030 | 0 | exact |
| 27 | 26 | 285.309 | 37.547 | 0 | exact |
| 28 | 27 | 286.465 | 39.007 | 0 | exact |
| 29 | 28 | 298.015 | 42.181 | 0 | exact |
| 30 | 29 | 308.877 | 47.116 | 0 | exact |
| 31 | 30 | 305.925 | 53.233 | 0 | exact |
| 32 | 31 | 318.551 | 67.338 | 0 | exact |
| 33 | 32 | 325.422 | 57.981 | 0 | exact |
| 34 | 33 | 382.093 | 85.872 | 0 | exact |
| 35 | 33 | 376.372 | 69.899 | 0 | exact |
| 36 | 34 | 523.793 | 270.973 | 0 | exact |

</details>

### 5 条停收后请求如何计量

模型原件保留 35 个已接收模型轮、35 次上游请求、0 次 cached retry，以及 **5 条 HTTP 409 / ADMISSION_CLOSED**。这 5 条请求均在同一已归档的 `model.admission_closed` 边界之后到达；各自只有 HTTP 接收与拒绝终态，未获 turn、未触发上游，也未进入 Tape。它们属于停收后的拒绝请求，单列保留在请求负载与 Trace 中，不计作新模型轮、新 Agent Attempt 或容量压测拒绝。

相对停收边界，它们分别在 1.735853、2.260850、3.215379、3.745424、4.207574 秒后到达；接收至拒绝响应耗时分别为 0.208217、0.205231、0.199596、0.194703、0.208247 毫秒，均在同一模型服务单调时钟内计算。累计处理时间 1.015994 毫秒仅表示这 5 个拒绝区间之和，不补入模型等待或吞吐分母。原 Attempt 分母仍为 1；原件没有被删去、当作 cached retry 或偷偷算成成功模型请求。

### 一次离线双 Trace 与原件

原件目录 `/opt/aries-artifacts/tb-repro-sqlite-v2-e2b-record-1`。复用现有分析器完成 **10,917 项对账、0 失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`：Perfetto / Trace Processor 导入 2,715 个区间、188 条 Flow；OTLP 导出 2,715 个 Span、187 个 parent、1 个 link，最大时间误差 0 ns。两种投影使用同一 Snapshot，integrity 为 complete、coverage 为 unknown，原 Attempt 的 `artifact_proven=false` 保持原值。

另有 2 个原生独立时钟区间（1 个 job、1 个 wait）缺少 UTC 锚点，保留在 Snapshot，未导入主时间轴或 OTLP；对应 3 条已验证因果边因端点未导出也保留为未导出。不能把可导出的子集说成完整跨域 Trace。本次没有新增 Jaeger / Prometheus 后端查询或浏览器验收。三份 SHA 相同的投影共享硬链接，保留原路径和字节，释放 **73,543,680 字节**重复分配；没有重复大包或新 Sandbox / 模型调用。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `6c0d043c7428fb4fb3b96b4cea37df74ff3a08faaba101944feb1e80461b224c` |
| 原 `result-review.json` | `3897b7e6b6365554d01ae54a2b3b5197e0d24a7e1ca5f9a647d74e6efb916a86` |
| 完整 Tape | `71aa297888b2eb2ee287ed0f401abbbeb0928b0a8242a41c3b7b2794d4418850` |
| 原生会话 transcript | `7e4a7b60356e6b31dae70b13390f9270c5ecbb958d14a0fff1063d712c03c034` |
| `harness/replay-tool-protocol.jsonl`（run / occurrence 内） | `96a0effe1dd6cb4afeaa1ae5fb63ed83f2aced8d19c01bfc20f34871cf5c1c7c` |
| `model/request-boundaries.jsonl`（run / occurrence 内） | `a428dcbd559c7df2bc898a167591b11156ddca83a8cca7e468b6f20487ce7f3d` |
| `performance-review-1/summary.json` | `b31a28f2c60a3bc258fd14d1225dbe7159cee58f3452f2524d65b8c1dc05ea95` |
| `offline-analysis-1/evidence.json` | `2879e115f59de9d5ad273640e497910e9401a4df8946ea9a4335bf3b319d1855` |
| `projection-dedup-1/evidence.json` | `f20d90ec02b9131ebc519a9f8c60bcd1fc39e0c3e5b0454b77826a94875b124d` |

本地小型汇总为 `.cache/tb-repro-sqlite-v2-record1-audit-1/summary.json`、`summary.md` 与 `protocol-timing.json`，不含原工具正文或凭据。完整 Tape 路径为上述原件目录下的 `runs/20261026T064245.000178031Z-tb-sqlite-with-gcov-terminal-result-v2-record/sqlite-with-gcov-terminal-result-v2-001/model/tape.json`。


<a id="sqlite-v2-e2b-replay-1"></a>

## SQLite terminal-result-v2：E2B replay-1

**同 Tape 严格回放 FAIL：原评分 0/3、reward 0；已执行的 24/24 次 exec 身份完整，后台等待已返回。** 第 24 次模型请求（从 0 起的 turn 23）发生 `REPLAY_DIVERGED`，随后 5 条请求为 `SESSION_FAILED`。前 23 个模型轮匹配成功，已执行 25 次调用（24 次 exec、1 次 process/wait），决策与完整录制 Tape 的前缀一致。Tape 全量预期 37 次调用，剩余 12 次 exec 未执行，不能算成身份丢失或成功工作量。

本次沿用 release10、`e040654e…` 原生镜像和完整 Tape `71aa2978…`，时序为 recorded，实际没有上游模型请求。原 runner 返回 1，Harness 与独立 Verifier 均失败；unit 于 2026-10-26 06:57:46.287984 UTC 终态，MainPID 为 0。停止及 Bridge 撤权 confirmed，唯一 Sandbox 已删除，provider 与 ledger 均确认不存在，清理错误为 0。保留原 Tape 和原失败，不重录来替换结果。

### 首差异：make install 输出的新文件时间

首个拒绝字段为 `$.messages[49].content`，精确对应 ToolCall `call_00_jGCNvYpQlozkAHTkxeoT0465`。录制与回放结果均为 **21 行、1,274 字节**，唯一变化是第 8–10 行长文件列表中的分钟 **06:46 → 06:57**，共 6 个字节替换；权限、链接数、用户/组、大小、名称及其余内容均一致。

直接读取该次真实调用和结果确认：这是 `make install` 返回的产物元数据，三项分别为 `libsqlite3.so`、`libsqlite3.so.0` 两条符号链接，以及 5,257,000 字节的 `libsqlite3.so.3.50.4` 文件。命令包含 `2>&1` 与管道，没有 `bash -lic`；Bridge 对应 exact 执行 stdout 1,275 字节、stderr 0、completed / exit 0，Native renderer 为 `stdout-stderr-v1` 且未截断。本次偏离来自运行中生成的安装时间，不是初始目录时间、后台随机句柄、身份缺失或 native 双流重排。固定初始环境本身不能固定后续安装的时间；本轮没有据此过滤 ToolResult、改 matcher 或伪造时间。

`native_decisions_differ_from_real_tape` 是全量决策序列未执行完的后续验收错误，不能代替这一首因。

| 首差异证据 | SHA-256 |
| --- | --- |
| 录制 ToolResult（1,274 B） | `c93dd7b26123ba0c12d15e691a11025ae9b9b75e679c1f137041ac7aeffd55d6` |
| 回放 ToolResult（1,274 B） | `40d4b3ee2342cc2c039a6d20ed32a4d89d0670db6e764c9af69a9f9573a95eb5` |
| 实际调用命令 | `4cf05bd28146ca547e77424a05512a83be548e38c37ee33aa6a831ec25cb2d44` |
| 原 Bridge 记录文件 | `dde475b6622a15945bd7a91b98372dadd012450224cdd0094efc0ad119c1509e` |

### 后台等待和旧归档修复已实际走过

首偏离发生前，原后台 exec 已 accepted，`process/wait` 已返回 completed / exit 0；原协议 retained 文件记录 accepted、wait_started、terminal_result、wait_finished 各 1 条。wait ToolCall `call_00_QA0YnizwPdeteFZIK9lV0943` 的真实 JSONL 再次包含 `persistedDetailsTruncated=true`、`originalDetailsBytesAtLeast=8193`，并保留 `protocol=terminal-result-v1`、`outputRenderer=stdout-stderr-v1`、`outputTruncated=false`，与原协议 terminal_result 一致。该 wait 的内容已进入后续通过 Strict 的请求历史，旧长结果字段缺失没有在这次复现。

driver 因更早的全量决策验收失败，没有走到末尾的 protocol 验收调用。随后对同一原件单独运行原有、SHA 固定的协议验收函数，结果为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`，另存 `protocol-review-1.json`；只验证已执行的 1 job / 1 wait，不修改 driver 原失败或将整次回放改成通过。原生时钟内 job 为 84.243776 秒、wait 为 82.221255 秒，两者重叠且未跨域校准；原生终态不单独等同于 guest 退出或全部观测零丢失。

### 八阶段与逐次工具耗时

Attempt 总时长为 **198.400757 秒**。以下单位为秒，阶段对应[统一阶段说明](performance-stages.md)，来自原 Attempt / `run-result.json`；各阶段不能相加替代端到端测量。实际仅执行完整 Tape 的前缀，不能与整轮录制当作等量工作性能比较。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000025 |
| 任务 Sandbox 创建 | 0.909499 |
| 工作区准备 | 1.145532 |
| Harness 启动 | 7.222160 |
| Runtime 执行 | 168.901513 |
| 停止与撤权 | 5.540910 |
| 独立验证 | 12.588135 |
| 清理 | 0.718661 |

<details>
<summary>逐次 exec：本次实际执行的 24 次调用</summary>

单位为毫秒，模型轮从 1 起。全部 24 次均精确关联；SDK 与 guest 使用各自计时边界，不能相减推导网络时延。第 20 次为后台 job，独立 process/wait 的原生计时已在上节列出；它与 job 重叠，不能重复相加。第 23 次真实 exit 1 保留。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 482.291 | 93.011 | 0 | exact |
| 2 | 1 | 965.149 | 212.590 | 0 | exact |
| 3 | 2 | 1380.137 | 878.394 | 0 | exact |
| 4 | 2 | 460.871 | 54.176 | 0 | exact |
| 5 | 3 | 740.552 | 491.216 | 0 | exact |
| 6 | 4 | 845.964 | 602.027 | 0 | exact |
| 7 | 5 | 261.616 | 27.749 | 0 | exact |
| 8 | 6 | 267.857 | 28.701 | 0 | exact |
| 9 | 7 | 290.900 | 38.767 | 0 | exact |
| 10 | 8 | 291.855 | 37.624 | 0 | exact |
| 11 | 9 | 262.173 | 27.257 | 0 | exact |
| 12 | 10 | 278.342 | 34.867 | 0 | exact |
| 13 | 11 | 287.381 | 36.525 | 0 | exact |
| 14 | 12 | 279.274 | 35.450 | 0 | exact |
| 15 | 13 | 277.615 | 32.545 | 0 | exact |
| 16 | 14 | 278.520 | 30.949 | 0 | exact |
| 17 | 15 | 559.934 | 315.268 | 0 | exact |
| 18 | 16 | 3695.364 | 3452.538 | 0 | exact |
| 19 | 17 | 330.837 | 91.457 | 0 | exact |
| 20 | 18 | 84235.095 | 83985.499 | 0 | exact |
| 21 | 20 | 277.152 | 36.066 | 0 | exact |
| 22 | 21 | 270.980 | 33.303 | 0 | exact |
| 23 | 22 | 638.432 | 222.542 | 1 | exact |
| 24 | 23 | 329.487 | 82.091 | 0 | exact |

</details>

### 一次离线双 Trace

已完成单次独立 Trace Processor / OTLP 对账：**6,153 项检查、0 失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。Perfetto 导入 1,524 个区间、109 条 Flow；OTLP 导出 1,524 个 Span、108 个 parent、1 个 link，最大时间误差 0 ns。两种投影使用同一 Snapshot；integrity 为 complete、coverage 为 unknown，原 Attempt 的 `artifact_proven=false` 保持原值，原 Strict 回放 FAIL 不变。

1 个 job 与 1 个 wait 的原生单调时钟没有可信 UTC 锚点，2 个区间和涉及它们的 3 条边仍未导入主时间轴 / OTLP；它们在 Snapshot 中保留。此次未新增 Jaeger / Prometheus 查询或浏览器验收。3 份投影 SHA 相同后共享硬链接，释放 **32,219,136 字节**重复分配，原路径和内容不变。


原件目录 `/opt/aries-artifacts/tb-repro-sqlite-v2-e2b-replay-1`；本地小型诊断及汇总位于 `.cache/tb-repro-sqlite-v2-replay1-audit-1/`，包含 `diagnostic.json`、`summary.json` 和 `summary.md`，不含工具正文或凭据。

| 证据（上述原件目录内） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `4e7e82f7f738da66163b7ed43fd00f048a9e0a4d4f0ef8865e50daae8ca2f05a` |
| 原 `result-review.json` | `df775228dc8676d3115a8eec44e74bee8b169151c9c990b7d2cd7784416b1c64` |
| 原 `run-result.json`（run 内） | `61ff4a89188542fe064a6023c2314f838505975c64598184b3223320a8fa09d9` |
| 原生会话 transcript | `f979cfa646708ebe94dc6375843909910d239a443f18dad709d33d7fedd91564` |
| 原后台协议 raw（run / occurrence 内） | `5c134674f8af80f99fb8f111f9ddece6dd85aaf643b73cbcbc79fe56656c4fd8` |
| `replay-diagnostic-1.json` | `709a0afccf06edb7841316545bebcb76b56f12eec1e7118cdb15ddf0c2b3182e` |
| `replay-shape-diagnostic-1.json` | `4ff06ae36cc429d4da207cf1d3121517c37834beae0789c7d2120b20878164a4` |
| `protocol-review-1.json`（仅已执行协议范围） | `36f04d5a62df5f3fea525af250040b1d3a68db3cad09a09a0821de5d3686155e` |
| `performance-review-1/summary.json` | `9cb637359e0a17b8a6ae704f1915ebd437c9e187a9a2a79802f26ebd87318be8` |
| `offline-analysis-1/evidence.json` | `9b9529ca8ace233d6cbde30f98a6817d7e949976fce7e71f53ffcfd481a83219` |
| `projection-dedup-1/evidence.json` | `22873d7bd298ec97e1cd45d453f12741a385260a3148f2ec17853469a88ee7a3` |


<a id="text-v2-e2b-record-1"></a>

## 大文本 terminal-result-v2：E2B record-1

**真实录制通过：原评分 5/5、reward 1，20/20 次 exec 精确关联，2 个后台 job / 2 次 wait 映射通过，Tape 完整。** 本次为独立变体 `large-scale-text-editing-terminal-result-v2`，沿用 release10 与固定 `e040654e…` 原生镜像，保留原任务、私有 Verifier、真实工具及既有预算。完成 23 个模型轮、22 次原生调用及结果（20 次 exec、2 次 process/wait），没有未知 exec 身份或派发前拒绝。它是一次真实 E2B 录制成功；同 Tape 严格回放及华为云配对需独立验收，不能先写成跨云比较已通过。

运行于 2026-10-26 07:02:44.579832—07:07:35.684807 UTC，原 runner 返回 0。Harness、独立验证、停止和 Bridge 撤权均成功；实际只创建 1 个 Sandbox，provider 与 ledger 均确认不存在，清理错误为 0。原先大文本 v1 模型断流及其他失败记录仍保留。

### 两次真实后台等待与模型分母

协议原件含 8 条事件：accepted、wait_started、terminal_result、wait_finished 各 2 条，独立逻辑 job 和 wait 身份贯通，验收 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。两项原生终态均为 completed / exit 0、`stdout-stderr-v1`、`outputTruncated=false`。这表示原生 Promise（含 finalize）的结果及归档关联通过，不单独替代 guest 退出证书或证明全部生产事件零丢失。

下表只在同一原生协议单调时钟内计算。job 和 wait 是重叠窗口，不能相加，也没有用于与控制器或云端时钟相减。

| 后台任务 | accepted → terminal 秒 | wait 开始 → 返回 秒 | 原生终态 |
| --- | ---: | ---: | --- |
| 1（exec 18） | 0.409165427 | 0.000212873 | completed / 0 |
| 2（exec 19） | 32.976924322 | 31.081658907 | completed / 0 |

第一项 job 在 Agent 发起 wait 前已完成，所以等待返回仅约 0.213 毫秒，并不表示实际任务执行只花这么久。第二项 wait 覆盖了约 31.082 秒的等待。此次没有 `persistedDetailsTruncated=true`，因此不能把它写成超过 8,192 字节归档截断的再次真实覆盖；该路径的实证来自 SQLite 的独立记录。

模型原件为 **23 个完成轮、23 次上游请求、0 次 cached retry、0 条停收后拒绝**。本次没有 `ADMISSION_CLOSED` 或其他拒绝终态；不能把 SQLite 录制的 5 条晚到请求混入本次分母。原 Attempt 分母为 1。

### 八阶段与逐次工具耗时

单位为秒，阶段与[统一阶段说明](performance-stages.md)一致；Attempt 总时长为 **280.267591 秒**。Runtime 使用实际运行窗口，Verifier 为完整独立评分阶段，不能用阶段求和替代端到端测量。这是共享 ECS 上的单次功能样本，不是容量或稳定性能统计。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000017 |
| 任务 Sandbox 创建 | 1.164969 |
| 工作区准备 | 1.631912 |
| Harness 启动 | 7.139498 |
| Runtime 执行 | 207.975861 |
| 停止与撤权 | 5.576552 |
| 独立验证 | 54.491011 |
| 清理 | 0.738269 |

<details>
<summary>逐次 exec：20 次真实调用</summary>

单位为毫秒，模型轮从 1 起，SDK 等待与 guest 整体分别保留；不同边界不能相减推导网络时延。后台 job 的真实执行也在本表中，process/wait 的原生等待已另列，不能重复累加。所有退出码保持原件。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 2591.943 | 2318.969 | 0 | exact |
| 2 | 2 | 380.590 | 111.690 | 0 | exact |
| 3 | 3 | 471.070 | 182.026 | 0 | exact |
| 4 | 4 | 819.728 | 543.893 | 1 | exact |
| 5 | 5 | 323.815 | 36.482 | 0 | exact |
| 6 | 6 | 314.595 | 41.240 | 0 | exact |
| 7 | 7 | 328.851 | 39.632 | 0 | exact |
| 8 | 8 | 296.282 | 36.846 | 0 | exact |
| 9 | 9 | 389.828 | 97.602 | 0 | exact |
| 10 | 10 | 301.826 | 37.403 | 0 | exact |
| 11 | 11 | 301.273 | 38.063 | 0 | exact |
| 12 | 12 | 316.814 | 40.035 | 0 | exact |
| 13 | 13 | 350.969 | 48.262 | 0 | exact |
| 14 | 14 | 296.784 | 36.992 | 0 | exact |
| 15 | 15 | 379.301 | 39.886 | 0 | exact |
| 16 | 16 | 313.315 | 40.636 | 0 | exact |
| 17 | 17 | 312.047 | 40.109 | 0 | exact |
| 18 | 18 | 401.720 | 120.881 | 0 | exact |
| 19 | 20 | 32968.865 | 32694.810 | 0 | exact |
| 20 | 22 | 322.246 | 53.111 | 0 | exact |

</details>

### 一次离线双 Trace 与原件

原件目录 `/opt/aries-artifacts/tb-repro-text-v2-e2b-record-1`。单次离线对账为 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`：**13,391 项检查、0 失败**；Perfetto / Trace Processor 导入 3,333 个区间、129 条 Flow；OTLP 导出 3,333 个 Span、127 个 parent、2 个 link，最大时间误差 0 ns。两种投影使用同一 Snapshot，integrity 为 complete、coverage 为 unknown，原 Attempt 的 `artifact_proven=false` 保持原值。

另外 4 个原生单调时钟区间（2 个 job、2 个 wait）没有可信 UTC 锚点，保留在 Snapshot，未导入主时间轴 / OTLP；对应 6 条因果边因端点未导出而保留为未导出。协议 producer 总丢失数及独立调用分母仍为 unknown。本次未新增 Jaeger / Prometheus 查询或浏览器验收。3 份投影校验 SHA 相同后共享硬链接，保留原路径和内容，释放 **96,743,424 字节**重复分配。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `55b398db414b164486593041a6119fd317e9b4594e70d74e0615aeaa35a75bf6` |
| 原 `result-review.json` | `1a1e2f8a96467f4c53d78181394fd26f3baa59ba86cbdd6d6f5be898d34d426e` |
| 完整 Tape | `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893` |
| 原生会话 transcript | `ac25d1f7ca2ef85f8fdef2ea8cf5adc2ce1df450ccd2f5c8dba53a49d1a0417c` |
| 后台协议 raw（run / occurrence 内） | `f1436e6d4329e63d2a0e935f59636a99dd5dcc3848db63ddda3474762269564c` |
| 模型请求边界（run / occurrence 内） | `94cdc7463af920e24605457a47045be5464b4b94a7121e8f7ea2265fa696698f` |
| `performance-review-1/summary.json` | `0a75f70e5573bbd590a46a495e8e434d7751b159f481be43d63631eede1bec9b` |
| `offline-analysis-1/evidence.json` | `5cea60dbd42f20fe345a46dad53c26887f65b146bc707213e96f227ff8c98e00` |
| `projection-dedup-1/evidence.json` | `35a5554faaed8fe201889230c10ba26e37b90514720745088d19cada2d710a50` |

本地小型汇总为 `.cache/tb-repro-text-v2-record1-audit-1/summary.json` 与 `summary.md`，不含工具正文或凭据。完整 Tape 位于上述原件目录下的 `runs/20261026T070245.932174556Z-tb-large-scale-text-editing-terminal-result-v2-record/large-scale-text-editing-terminal-result-v2-001/model/tape.json`。离线分析期间没有新建 Sandbox 或调用模型，结束后已释放 ECS 窗口供主线执行同 Tape 回放。


<a id="text-v2-e2b-replay-1"></a>

## 大文本 terminal-result-v2：E2B replay-1

**同 Tape 完整 Strict 回放通过：原评分 5/5、reward 1，23 轮请求全部匹配，20/20 次 exec 身份精确，2 个后台 job / 2 次 wait 映射通过。** 与[真实录制](#text-v2-e2b-record-1)使用相同 release10、固定 `e040654e…` 原生镜像、任务环境及原私有 Verifier；完整复用 Tape `d41842ec…`，没有重新录制、过滤工具结果或放宽 matcher。22 次原生调用及结果全部完成（20 次 exec、2 次 process/wait），不是仅前缀通过。

本次时序为 recorded；模型原件确认 **23 轮匹配、0 次上游请求、0 次 cached retry、0 条拒绝终态**，没有 ADMISSION_CLOSED 或 SESSION_FAILED。运行于 2026-10-26 07:18:16.051376—07:23:05.162205 UTC；unit `aries-tb-repro-text-v2-e2b-replay-1.service` 实际终态为 inactive / dead、MainPID 0、ExecMainStatus 0，原 runner 返回 0。仅创建 1 个 Sandbox，停止及 Bridge 撤权 confirmed，provider 和 ledger 均确认不存在，清理错误为 0。

这证明大文本 v2 在 E2B 完成一次真实录制到同 Tape 回放闭环。它没有新增华为云运行或跨云成功配对，也不替代独立重复的性能统计。

### 后台终态与覆盖范围

协议原件 retained 的 8 条记录包含 accepted、wait_started、terminal_result、wait_finished 各 2 条；原验收为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。两个 job 的原生结果均为 completed / exit 0、`outputRenderer=stdout-stderr-v1`、`outputTruncated=false`，wait 归档与终态字段一致。此次没有 details 截断标志，不能声称覆盖了大于 8,192 字节的归档路径。

下表分别计算原生协议同一单调时钟中的 job 与 wait 窗口；它们重叠，不能相加，也不能与控制器时钟相减。

| 后台任务 | accepted → terminal 秒 | wait 开始 → 返回 秒 | 原生终态 |
| --- | ---: | ---: | --- |
| 1（exec 18） | 0.383843854 | 0.000208103 | completed / 0 |
| 2（exec 19） | 34.338268514 | 32.451418052 | completed / 0 |

第一项 wait 在 job 已完成后才到达，因此返回仅约 0.208 毫秒，不能将它误读为实际任务只执行了这一点时间。原生终态范围仍是 Promise（含 finalize），不单独冒充 guest 退出证明；producer 总丢失数和独立调用分母仍为 unknown。

### 八阶段与逐次工具耗时

单位为秒，阶段对应[统一阶段说明](performance-stages.md)；Attempt 总时长 **280.268569 秒**。Runtime 使用原 `runtime_execution_duration`，Verifier 为完整独立评分阶段；各窗口不能简单相加替代端到端计时。本次为共享 ECS 的单次功能及可复现性样本，不作容量或云厂商性能优劣结论。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000012 |
| 任务 Sandbox 创建 | 0.861034 |
| 工作区准备 | 0.937629 |
| Harness 启动 | 7.229333 |
| Runtime 执行 | 209.365592 |
| 停止与撤权 | 5.578586 |
| 独立验证 | 54.272507 |
| 清理 | 0.667183 |

<details>
<summary>逐次 exec：完整回放的 20 次调用</summary>

单位为毫秒，模型轮从 1 起。SDK 等待和 guest 整体分别保留，不能相减推导网络时延。后台 job 的执行在此表中；process/wait 的原生等待在上节中单列，不能重复累加。1 次真实 exit 1 保留，其余 19 次 exit 0，单次工具错误没有被最终任务成功覆盖。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 2668.326 | 2391.825 | 0 | exact |
| 2 | 2 | 369.429 | 97.763 | 0 | exact |
| 3 | 3 | 1044.160 | 176.893 | 0 | exact |
| 4 | 4 | 1105.844 | 247.985 | 1 | exact |
| 5 | 5 | 308.958 | 37.640 | 0 | exact |
| 6 | 6 | 292.233 | 37.826 | 0 | exact |
| 7 | 7 | 311.069 | 37.767 | 0 | exact |
| 8 | 8 | 331.188 | 47.725 | 0 | exact |
| 9 | 9 | 366.434 | 99.598 | 0 | exact |
| 10 | 10 | 294.082 | 39.123 | 0 | exact |
| 11 | 11 | 308.194 | 37.525 | 0 | exact |
| 12 | 12 | 315.731 | 40.438 | 0 | exact |
| 13 | 13 | 302.133 | 37.453 | 0 | exact |
| 14 | 14 | 318.196 | 38.808 | 0 | exact |
| 15 | 15 | 316.137 | 41.614 | 0 | exact |
| 16 | 16 | 299.188 | 42.455 | 0 | exact |
| 17 | 17 | 310.635 | 39.918 | 0 | exact |
| 18 | 18 | 375.740 | 103.591 | 0 | exact |
| 19 | 20 | 34331.112 | 34052.387 | 0 | exact |
| 20 | 22 | 329.939 | 55.000 | 0 | exact |

</details>

### 一次离线双 Trace 与原件

原件目录 `/opt/aries-artifacts/tb-repro-text-v2-e2b-replay-1`。复用现有分析器各一次，完成 **13,283 项检查、0 失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。Perfetto / Trace Processor 导入 3,306 个区间、106 条 Flow；OTLP 导出 3,306 个 Span、104 个 parent、2 个 link，最大时间误差 0 ns。两种投影来自同一 Snapshot；integrity 为 complete、coverage 为 unknown，原 Attempt 的 `artifact_proven=false` 保持原值。

2 个 job 与 2 个 wait 的原生单调时钟没有可信 UTC 锚点，因此这 4 个区间和涉及它们的 6 条边仍保留在 Snapshot，未导入主时间轴 / OTLP。本次未新增 Jaeger / Prometheus 后端查询或浏览器验收。3 份投影 SHA 相同后共享硬链接，保留原路径和内容，释放 **74,604,544 字节**重复分配。分析未新建 Sandbox、未调用模型，完成后已释放 SSH 及 ECS 窗口。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `ea5229721d285a48d6d32e0aedc5384ba5e48d42b5a44458b8f44f116cde3dbd` |
| 原 `result-review.json` | `7d029e3993b9b9922ef513b2e6d7948b64843a420cbd20a1464017772e8dd758` |
| 完整 Tape（record-1 目录内） | `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893` |
| 原生会话 transcript | `55429283ef18dc3226331a657b7ad77bb2673f3444ecd6e34b42f8a2e82a063e` |
| 后台协议 raw（run / occurrence 内） | `a442b07ab47494f7a3b77c2898467af1e987f86736313617b9a28efc79b7bb88` |
| 模型请求边界（run / occurrence 内） | `02c106e856a6df6fc81b587718389e8c3ba9358c38507b06d4a924b883222f3b` |
| `performance-review-1/summary.json` | `57eb695b864c4803c90f02276de906676e857039bd0ba24d40a03a303ff22628` |
| `offline-analysis-1/evidence.json` | `5bab2cf7552ef15add648672dc9c52deff3d8c5ce0c875445b61d4743f2956fc` |
| `projection-dedup-1/evidence.json` | `ab95acc6aad5ffd98269815bb4a47db4c7e93eb3074ca2bc26fbcc66a6d02e1f` |

本地小型汇总为 `.cache/tb-repro-text-v2-replay1-audit-1/summary.json` 与 `summary.md`，不含工具正文或凭据。回放沿用的 Tape 原路径为 `/opt/aries-artifacts/tb-repro-text-v2-e2b-record-1/runs/20261026T070245.932174556Z-tb-large-scale-text-editing-terminal-result-v2-record/large-scale-text-editing-terminal-result-v2-001/model/tape.json`。


<a id="text-v2-e2b-replay-2"></a>

## 大文本 terminal-result-v2：E2B replay-2

**第二次同 Tape 完整 Strict 回放通过：原评分 5/5、reward 1，23 轮全部匹配、20/20 次 exec 身份精确、2 个 job / 2 次 wait 映射通过。** 完整22次原生调用与结果均已保留；继续使用原完整 Tape `d41842ec…`、release10、固定 `e040654e…` 原生镜像、同一任务环境与原私有 Verifier，没有重录、改 matcher 或替换工具结果。结合[第一次回放](#text-v2-e2b-replay-1)，目前证明的是 E2B 上同一 Tape 的两次完整回放通过；本节不新增跨云成功配对或容量结论。

运行于 2026-10-26 07:25:47.616946—07:30:26.855846 UTC。unit `aries-tb-repro-text-v2-e2b-replay-2.service` 真实终态为 inactive / dead、MainPID 0、ExecMainStatus 0，原 runner 返回 0；没有把运行中的初始化 FAIL 字段当作最终状态。仅创建 1 个 Sandbox，停止及 Bridge 撤权 confirmed，provider 与 ledger 均确认不存在，cleanup errors为空。

模型时序为 recorded；请求边界确认 **23 轮匹配、0 次上游调用、0 次 cached retry、0 条拒绝终态**。Tape SHA、原评分和本次单个 Attempt 分母独立保留。

### 八阶段耗时

单位为秒，对应[统一阶段说明](performance-stages.md)；原 Attempt 总时长 **270.444315 秒**。各阶段窗口不能相加替代端到端测量，也不能用两次单样本直接给出云性能优劣或统计稳定性结论。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000014 |
| 任务 Sandbox 创建 | 1.297734 |
| 工作区准备 | 1.992704 |
| Harness 启动 | 7.235322 |
| Runtime 执行 | 204.995031 |
| 停止与撤权 | 5.536798 |
| 独立验证 | 47.406967 |
| 清理 | 0.615639 |

### 后台等待与逐次工具

协议原件有8条事件，accepted、wait_started、terminal_result、wait_finished各2条；原验收为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。两个真实原生终态均为 completed / exit 0、`stdout-stderr-v1`、`outputTruncated=false`。本次没有 details 截断标志，未覆盖大于8192字节的持久化路径。

| 后台任务 | accepted → terminal 秒 | wait 开始 → 返回 秒 | 原生终态 |
| --- | ---: | ---: | --- |
| 1（exec 18） | 0.355122124 | 0.000206203 | completed / 0 |
| 2（exec 19） | 30.689176771 | 28.793775564 | completed / 0 |

以上来自原生协议自己的单调时钟：第一项 wait 到达前 job 已结束；job 与 wait 窗口不累加，不跨域相减。原生 Promise 终态不单独等同于 guest 退出证明，producer 总丢失数及独立调用分母仍为 unknown。

<details>
<summary>第二次完整回放：20 次 exec 明细</summary>

单位为毫秒，模型轮从1起。SDK等待与guest整体使用不同计时边界，不能相减当作网络耗时；process/wait的等待窗口已在上表单列，不重复累加。第4次真实exit 1保持原值，其余19次exit 0，最终评分通过没有覆盖中间失败。

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 2349.852 | 2067.875 | 0 | exact |
| 2 | 2 | 393.342 | 96.340 | 0 | exact |
| 3 | 3 | 419.692 | 144.329 | 0 | exact |
| 4 | 4 | 644.487 | 358.446 | 1 | exact |
| 5 | 5 | 313.178 | 32.738 | 0 | exact |
| 6 | 6 | 324.410 | 34.597 | 0 | exact |
| 7 | 7 | 308.808 | 31.649 | 0 | exact |
| 8 | 8 | 298.087 | 31.856 | 0 | exact |
| 9 | 9 | 344.364 | 65.497 | 0 | exact |
| 10 | 10 | 297.987 | 30.570 | 0 | exact |
| 11 | 11 | 305.858 | 37.292 | 0 | exact |
| 12 | 12 | 333.224 | 36.249 | 0 | exact |
| 13 | 13 | 309.817 | 31.501 | 0 | exact |
| 14 | 14 | 301.764 | 35.545 | 0 | exact |
| 15 | 15 | 307.506 | 29.884 | 0 | exact |
| 16 | 16 | 306.921 | 36.311 | 0 | exact |
| 17 | 17 | 311.543 | 28.884 | 0 | exact |
| 18 | 18 | 348.053 | 93.605 | 0 | exact |
| 19 | 20 | 30681.416 | 30397.648 | 0 | exact |
| 20 | 22 | 329.493 | 47.904 | 0 | exact |

</details>

### 一次离线双 Trace 与原件

原件目录 `/opt/aries-artifacts/tb-repro-text-v2-e2b-replay-2`。只对本次运行执行一次汇总和一次离线审计：**13,231 项检查、0失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。Perfetto / Trace Processor导入3,293个区间、106条Flow；OTLP导出3,293个Span、104个parent、2个link，最大时间误差0ns。两种投影使用同一Snapshot；integrity complete、coverage unknown，原Attempt的 `artifact_proven=false` 保持不变。

4个原生单调时钟区间（2个job、2个wait）及涉及它们的6条边仍缺可信UTC锚点，保留在Snapshot，未导入主时间轴/OTLP。本次没有新增Jaeger/Prometheus后端查询或浏览器验收。3份SHA相同投影硬链接共享，保留原路径和字节，释放 **74,117,120字节**重复分配；没有重跑旧分析或创建新Sandbox/模型调用。远程分析结束后已释放SSH和ECS窗口。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `3beff58ef8b3273783834d034e08fe79b8c4781c5e877fa7ee899d014c94a0b8` |
| 原 `result-review.json` | `c3e93b37ea9708b7d261e92facc97889c5d232cf36f0b8f5be216b3aa6ef47e9` |
| 完整 Tape（record-1目录内） | `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893` |
| 原生会话 transcript | `4dc3c37bfd58fc933d6e38a9859cec5a01f7a438a7e3ab35b348fdd69b5b02fc` |
| 后台协议 raw（run / occurrence内） | `66db167688669c2a835e6efc6fed8db5b7062032293d92dddc681f1b7410be7f` |
| 模型请求边界（run / occurrence内） | `c9d345efdc3cf56d4776c0cb9948458cbf6a834e0896ecf2ede966365f13c42d` |
| `performance-review-1/summary.json` | `d48cbe56fc0fd75ae1e8180754dd744902152582bd60e4a9ad6c45ec657ec5f9` |
| `offline-analysis-1/evidence.json` | `729bc86482589eb5dc328ceac0671c0900d108db069fa2a058a730eb02efb239` |
| `projection-dedup-1/evidence.json` | `263b96455902464a05747a29f5a5a69451410e66c87feb78e3913640bf15852d` |

本地小型汇总 `.cache/tb-repro-text-v2-replay2-audit-1/summary.json` 与 `summary.md` 保留原始ns、身份和来源，不含工具正文或凭据。录制Tape仍是 `/opt/aries-artifacts/tb-repro-text-v2-e2b-record-1/runs/20261026T070245.932174556Z-tb-large-scale-text-editing-terminal-result-v2-record/large-scale-text-editing-terminal-result-v2-001/model/tape.json`。


<a id="text-v2-huawei-replay-1"></a>

## 大文本 terminal-result-v2：华为 replay-1 与 E2B 同 Tape 对比

**同一完整 Tape 已在 E2B 连续两次、华为一次通过 Strict Replay：三次均原评分 5/5、reward 1，23 轮匹配、20/20 次 exec 精确关联，2 个后台 job / 2 次 wait 映射通过。** 这是 `large-scale-text-editing-terminal-result-v2` 独立变体的真实跨云结果，继续与原题统计分开。三次均使用 release10、固定 `e040654e…` 原生镜像和 Tape `d41842ec…`，原私有评分不变；没有替换真实工具、重录 Tape 或放宽 matcher。

华为本次运行于 2026-10-26 07:32:21.374169—07:44:37.726631 UTC。unit `aries-tb-repro-text-v2-huawei-replay-1.service` 终态 inactive / dead、MainPID 0、ExecMainStatus 0，原 runner 返回 0。仅创建 1 个 Sandbox，停止与 Bridge 撤权 confirmed，provider 与 ledger 均确认不存在，cleanup errors 为空。观察 SSH 曾断开，原任务持续运行并正常结束，没有据此重启任务或新增 Sandbox。

模型使用 recorded 时序，本次 **23 轮 matched、0 次上游调用、0 次 cached retry、0 条拒绝终态**；22 次原生工具调用及结果完整保留。下面以相同 ToolCall ID、模型轮和退出码对齐 [E2B replay-1](#text-v2-e2b-replay-1)、[E2B replay-2](#text-v2-e2b-replay-2) 与华为 replay-1，不用命令或时间近邻配对。

### 相同八阶段的三次实际耗时

单位为秒，对应[统一阶段说明](performance-stages.md)。端到端 Attempt 单独测量，阶段窗口不能简单累加替代它。当前是两次 E2B 和一次华为的实际样本，能展示同工作量在不同环境的耗时，尚不能据此给出稳定排名或并发容量结论。

| 阶段 | E2B replay-1 秒 | E2B replay-2 秒 | 华为 replay-1 秒 |
| --- | ---: | ---: | ---: |
| 排队 | 0.000012 | 0.000014 | 0.000018 |
| 任务 Sandbox 创建 | 0.861034 | 1.297734 | 0.667219 |
| 工作区准备 | 0.937629 | 1.992704 | 1.708633 |
| Harness 启动 | 7.229333 | 7.235322 | 7.253285 |
| Runtime 执行 | 209.365592 | 204.995031 | 191.408839 |
| 停止与撤权 | 5.578586 | 5.536798 | 5.353310 |
| 独立验证 | 54.272507 | 47.406967 | 519.266396 |
| 清理 | 0.667183 | 0.615639 | 0.942591 |
| **Attempt 端到端（另计）** | **280.268569** | **270.444315** | **727.536979** |

华为 Runtime 执行为 **191.408839 秒**，评分为 **519.266396 秒**，两者需要分开看。评分长尾已定位到原评分脚本执行 `apt-get update`：原 `evaluation/stdout.log` 第7行报告获取 **9395 kB 耗时 8min 6s，19.3 kB/s**。这是 APT 自报的取整耗时，不能伪造为有精确起止的网络 Span。

| 华为评分内部边界 | 秒 | 原始证据 |
| --- | ---: | --- |
| 私有评分文件准备 `verifier.prepare` | 0.331728 | `runner-events.jsonl` 第4133—4134行 |
| 执行原评分脚本 `verifier.exec` | 517.824025 | 同文件第4135—4136行 |
| 收取评分报告 `verifier.report` | 1.099812 | 同文件第4137—4138行 |
| 原评分进程 guest 整体 | 517.721936 | `e2b-operations.jsonl` 第1022行，exit 0 |
| 原 pytest / CTRF 的测试窗口 | 25.035945 | `evaluation/ctrf.json`，5/5 |

这些内部窗口有包含关系，不能累加；APT下载、pytest测试也不能当成 Agent 的 Tool 耗时。现有证据表明评分长尾主要发生在依赖仓库获取，具体镜像站、DNS、连接及传输的占比分解仍未知。此前小型日志摘要未识别 `8min 6s` 格式，其空数组不代表原日志没有计时；本节采用原日志行及独立诊断收据。原评分过程、日志和完成结果均保留，没有为缩短耗时补跑或改写原件。

### 相同 20 次 exec 与后台等待

表中每格为 **SDK 等待 / guest 整体，单位毫秒**。三次运行的 ToolCall ID、模型轮、退出码逐条一致，所有关联均为 exact。SDK 与 guest 使用不同边界，不能相减当作网络耗时；第4次 exit 1 是三次都真实出现的中间失败，其余19次 exit 0，最终评分通过不会抹去它。

<details>
<summary>展开同工作量的 20 次工具耗时对照</summary>

| exec | 模型轮 | E2B replay-1：SDK / guest ms | E2B replay-2：SDK / guest ms | 华为 replay-1：SDK / guest ms | exit |
| --- | --- | ---: | ---: | ---: | --- |
| 1 | 1 | 2668.326 / 2391.825 | 2349.852 / 2067.875 | 7302.460 / 7210.704 | 0 |
| 2 | 2 | 369.429 / 97.763 | 393.342 / 96.340 | 181.319 / 54.095 | 0 |
| 3 | 3 | 1044.160 / 176.893 | 419.692 / 144.329 | 246.663 / 161.276 | 0 |
| 4 | 4 | 1105.844 / 247.985 | 644.487 / 358.446 | 850.243 / 764.873 | 1 |
| 5 | 5 | 308.958 / 37.640 | 313.178 / 32.738 | 127.437 / 42.186 | 0 |
| 6 | 6 | 292.233 / 37.826 | 324.410 / 34.597 | 128.398 / 40.567 | 0 |
| 7 | 7 | 311.069 / 37.767 | 308.808 / 31.649 | 125.692 / 40.793 | 0 |
| 8 | 8 | 331.188 / 47.725 | 298.087 / 31.856 | 134.262 / 48.290 | 0 |
| 9 | 9 | 366.434 / 99.598 | 344.364 / 65.497 | 230.994 / 96.408 | 0 |
| 10 | 10 | 294.082 / 39.123 | 297.987 / 30.570 | 129.884 / 43.113 | 0 |
| 11 | 11 | 308.194 / 37.525 | 305.858 / 37.292 | 128.360 / 47.576 | 0 |
| 12 | 12 | 315.731 / 40.438 | 333.224 / 36.249 | 147.591 / 25.132 | 0 |
| 13 | 13 | 302.133 / 37.453 | 309.817 / 31.501 | 129.038 / 46.617 | 0 |
| 14 | 14 | 318.196 / 38.808 | 301.764 / 35.545 | 137.638 / 49.174 | 0 |
| 15 | 15 | 316.137 / 41.614 | 307.506 / 29.884 | 131.533 / 49.723 | 0 |
| 16 | 16 | 299.188 / 42.455 | 306.921 / 36.311 | 129.227 / 46.991 | 0 |
| 17 | 17 | 310.635 / 39.918 | 311.543 / 28.884 | 123.114 / 42.376 | 0 |
| 18 | 18 | 375.740 / 103.591 | 348.053 / 93.605 | 247.604 / 164.365 | 0 |
| 19 | 20 | 34331.112 / 34052.387 | 30681.416 / 30397.648 | 18351.412 / 18268.825 | 0 |
| 20 | 22 | 329.939 / 55.000 | 329.493 / 47.904 | 157.810 / 76.936 | 0 |

</details>

华为后台协议共8条事件，accepted、terminal_result、wait_started、wait_finished各2条，验收为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。两项真实终态均为 completed / exit 0、`stdout-stderr-v1`、`outputTruncated=false`；本次没有 details 截断标志，未覆盖大于8192字节的持久化路径。

| 后台任务 | 华为 accepted → terminal 秒 | 华为 wait 开始 → 返回 秒 | 解释 |
| --- | ---: | ---: | --- |
| 1（exec 18） | 0.255430296 | 0.000233384 | wait 到达前 job 已完成 |
| 2（exec 19） | 18.358434501 | 16.470543630 | wait 等待同一后台终态 |

以上来自原生协议自身的单调时钟，job 与 wait 不累加，不跨域相减；原生 Promise 终态本身不单独证明 guest 进程已退出。producer 总丢失数、终态计数器和独立调用分母仍为 unknown。

### 环境条件与华为评分出网边界

三次均为 A 模式（external），原任务及模板配置均为 **1 CPU / 2048 MiB**，原始任务镜像 digest 均为 `sha256:719adca3f1388220546ce6a155eee56eff3c4fe318183100320606a210f6b59c`。依据已批准口径，可以进行上述跨云性能比较；配置相同不代表物理 CPU、所有强制配额或共享宿主负载相同。本轮为共享 ECS 上的单次功能与性能样本，没有独占宿主证明。

磁盘差异单独提示：原任务请求10240 MiB，E2B准备时 guest 可见文件系统容量为 **11,459,682,304字节**；华为模板构建回执为 **30720 MiB**。前者是准备时实测、后者是模板回执，不能当作本次三次运行都重新测过的磁盘性能；差异不阻断比较，也不据此把样本判为失格。binding的 `storage_mb=null` 表示该处未填报，不表示磁盘为零。

**华为评分禁止出网未被本次证明。** 原任务 `allow_network=true`；本次 profile 和 adapter 的 `assume_network_isolation` 实际均为 false，未悄悄开启配置豁免。私有 Verifier 在确认停止及撤权后于原 Sandbox 执行，原评分确实访问依赖仓库。保留本次评分和耗时，并单列用户先前授权继续推进时的出网隔离假设；该假设不是强制隔离检查通过，不能把本次计为完整出网隔离验证的 Goodput。

资源依据为三份实际运行 binding 与准备回执：E2B binding SHA `db49124cc574eb88656f3fcac46b6af7e9f7f35c6bca77787785568c1e7d6b41`，华为 binding SHA `28833e4edd1161150aab0c3ee1a06683a4f43ac2d1b0633dc57fbc133be5cd0a`；E2B准备 guest 证据 SHA `130ce813f4d6b67a1fbbe23bdedb7e9078a9319a109beefeb6befcfa23f20a5f`，华为构建回执 SHA `afbdd73a51e61f8def3ab02b9dc07042be8f1dc66242c91820445ed34ee85922`。本地 `summary.json` 的 `comparison_environment` 保留三次实际绑定来源。

### 一次离线双 Trace 与原件

华为原件目录 `/opt/aries-artifacts/tb-repro-text-v2-huawei-replay-1`。汇总与离线审计各一次，完成 **13,675 项检查、0失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。同一 Snapshot 投影出 Perfetto / Trace Processor 的3,404个区间和106条Flow，以及 OTLP的3,404个Span、104个parent、2个link，最大时间误差0ns。integrity为complete、coverage为unknown，原Attempt的 `artifact_proven=false` 保持原值。

2个job与2个wait缺可信UTC锚点，因此4个原生区间和涉及它们的6条边仍保留在Snapshot，未导入主时间轴/OTLP；不伪造跨域时间线。本次未新增Jaeger/Prometheus后端查询或浏览器验收。3份投影先核对相同SHA后硬链接共享，保留原路径与字节，释放 **73,310,208字节**重复分配；没有新增Sandbox或模型调用。远程分析结束后已释放SSH及ECS窗口。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原 driver `evidence.json` | `934a78c818f6f63166105ec7be47966f19f246a93d7bd43824b3277fb832185b` |
| 原 `result-review.json` | `a4acc49e85dcb7521735f23f3b5c0d63f18d0103cdfdf2c0678ea813426d09ef` |
| 完整 Tape（E2B record-1目录内） | `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893` |
| 原生会话 transcript | `463cc767ac18bb1092cd43dd8a82d258c81f58b5c239407e65e3276d57058365` |
| 后台协议 raw（run / occurrence内） | `b614b0d1173de466eba4a868df54d528afb7da797f719864a225b79ac6418255` |
| 模型请求边界（run / occurrence内） | `0644ea8f9732c15bcc880bd3ebf1cc465eaee7c73614d7652fe429fe7dd682c9` |
| `performance-review-1/summary.json` | `ef49355b97f3657b7ebf3144c7d7d2d9345b22da2ab118b78f9683de82575817` |
| `offline-analysis-1/evidence.json` | `dfbbc383a37d6eec8c121ce6e6a7b2ec3716135aedcb321a93e2e91c8df04e58` |
| `projection-dedup-1/evidence.json` | `5336588e04dce2d4562f71a8066184e5fd13bdd2947713a7a30b0a8c61779ff8` |
| 原评分 `evaluation/stdout.log` | `94db2f489a961b4a094b359147548d18331dc61f3f6ef8a39cce45b7b950c82a` |
| 本地评分耗时诊断 `conclusion.json` | `f33da732e760205cb3c9ea5c424cebde865755ffda3dbf2b1f6d90b3c163ac77` |

本地小型汇总为 `.cache/tb-repro-text-v2-huawei-replay1-audit-1/summary.json` 与 `summary.md`；评分诊断为 `.cache/tb-repro-text-v2-huawei-verifier-diagnosis-1/conclusion.json`，保留原行号与计时域，不含工具正文或私有断言。Tape仍为 `/opt/aries-artifacts/tb-repro-text-v2-e2b-record-1/runs/20261026T070245.932174556Z-tb-large-scale-text-editing-terminal-result-v2-record/large-scale-text-editing-terminal-result-v2-001/model/tape.json`。


<a id="sqlite-v3-e2b-record-1"></a>

## SQLite terminal-result-v3：E2B record-1

**新变体的首次真实录制通过：原评分 3/3、reward 1；41轮模型录制完整，38/38次exec精确关联，2个后台job / 2次wait映射通过。** 本次使用release10和既有 `terminal-result-v1` 工具协议，真实模型、真实Sandbox工具、原私有评分均执行。`sqlite-with-gcov-terminal-result-v3` 独立统计，之前[v2回放的原始失败](#sqlite-v2-e2b-replay-1)保留。本节证明录制完成并具备同Tape Strict自回放输入条件，不把尚未完成的回放或跨云验证提前记为成功。

运行于2026-10-26 07:59:40.979603—08:05:20.432092 UTC。unit `aries-tb-repro-sqlite-v3-e2b-record-1.service` 终态inactive / dead、MainPID 0、ExecMainStatus 0，原runner返回0。停止与Bridge撤权confirmed；仅创建1个Sandbox，provider与ledger均确认不存在，cleanup errors为空。

完整Tape实际为 `complete=true`、41轮、最后 `finish=stop`、无failure。41次真实上游模型请求及40次原生工具调用/结果保留，其中38次exec、2次process/wait；0次cached retry、0次派发前拒绝、0次未知exec。关入口后另有5条 `ADMISSION_CLOSED`：接收和结束偏移均晚于同一时钟域的关闭时点，未调用上游、未进入Tape；模型轮数仍是41，不能把它们计成46轮模型失败。

### 八阶段耗时

单位为秒，对应[统一阶段说明](performance-stages.md)；原Attempt端到端 **331.142963秒** 单独保留，阶段窗口不能简单相加替代它。原CTRF的3项测试均通过；模型等待、工具执行与验证属于不同边界。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000015 |
| 任务 Sandbox 创建 | 1.521431 |
| 工作区准备 | 0.719133 |
| Harness 启动 | 7.124275 |
| Runtime 执行 | 301.445583 |
| 停止与撤权 | 5.596151 |
| 独立验证 | 12.956176 |
| 清理 | 0.745744 |

### 逐次工具与后台等待

38次exec均以确切ToolCall ID关联到真实执行，下面按模型轮列出SDK等待与guest整体耗时，单位毫秒。两列不能相减当作网络耗时。**33次exit 0、4次exit 1、1次exit 127**均保留；最终解题评分通过不掩盖Agent过程中的失败命令。

<details>
<summary>展开全部38次exec</summary>

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 518.772 | 201.161 | 0 | exact |
| 2 | 2 | 1060.952 | 773.941 | 1 | exact |
| 3 | 3 | 327.214 | 48.458 | 0 | exact |
| 4 | 4 | 1052.523 | 751.565 | 0 | exact |
| 5 | 5 | 347.398 | 45.662 | 0 | exact |
| 6 | 6 | 316.309 | 34.110 | 0 | exact |
| 7 | 7 | 350.472 | 51.028 | 0 | exact |
| 8 | 8 | 322.145 | 38.191 | 0 | exact |
| 9 | 9 | 333.174 | 37.659 | 0 | exact |
| 10 | 10 | 356.771 | 52.852 | 0 | exact |
| 11 | 11 | 3718.239 | 3429.146 | 0 | exact |
| 12 | 12 | 478.882 | 40.554 | 0 | exact |
| 13 | 13 | 104205.760 | 103916.344 | 0 | exact |
| 14 | 15 | 380.892 | 49.919 | 0 | exact |
| 15 | 16 | 436.935 | 39.366 | 0 | exact |
| 16 | 17 | 314.898 | 39.990 | 0 | exact |
| 17 | 18 | 354.464 | 52.756 | 0 | exact |
| 18 | 19 | 338.733 | 51.418 | 1 | exact |
| 19 | 20 | 487.266 | 194.545 | 0 | exact |
| 20 | 21 | 326.489 | 40.563 | 0 | exact |
| 21 | 22 | 1613.689 | 1311.192 | 0 | exact |
| 22 | 23 | 43072.477 | 42596.789 | 0 | exact |
| 23 | 25 | 326.457 | 40.181 | 0 | exact |
| 24 | 26 | 328.237 | 44.252 | 0 | exact |
| 25 | 27 | 327.516 | 52.158 | 1 | exact |
| 26 | 28 | 338.440 | 42.080 | 0 | exact |
| 27 | 29 | 541.833 | 253.470 | 0 | exact |
| 28 | 30 | 336.913 | 45.775 | 0 | exact |
| 29 | 31 | 434.632 | 34.817 | 0 | exact |
| 30 | 32 | 355.283 | 46.942 | 1 | exact |
| 31 | 33 | 339.617 | 46.174 | 0 | exact |
| 32 | 34 | 443.358 | 122.303 | 0 | exact |
| 33 | 35 | 302.061 | 32.459 | 0 | exact |
| 34 | 36 | 314.833 | 41.906 | 127 | exact |
| 35 | 37 | 324.573 | 37.107 | 0 | exact |
| 36 | 38 | 347.788 | 49.944 | 0 | exact |
| 37 | 39 | 330.571 | 46.442 | 0 | exact |
| 38 | 40 | 314.847 | 42.802 | 0 | exact |

</details>

后台原件8条事件，accepted、terminal_result、wait_started、wait_finished各2条；验收为 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`。两项终态均为completed / exit 0、`stdout-stderr-v1`、`outputTruncated=false`。本次原生结果没有 `persistedDetailsTruncated` 标志，**没有实际覆盖大于8192字节的details持久化路径**，不借此前其他运行的覆盖替本次下结论。

| 后台任务 | accepted → terminal 秒 | wait 开始 → 返回 秒 | 原生终态 |
| --- | ---: | ---: | --- |
| 1（exec 13） | 104.212475962 | 101.704018184 | completed / 0 |
| 2（exec 22） | 33.079918541 | 30.847867644 | completed / 0 |

这两列来自原生协议自身的单调时钟，起点分别为后台句柄接纳和wait开始；不等同于guest启动，不与上方exec整体或彼此累加。原生Promise终态也不单独证明guest退出。producer终态回执、总事件/写入失败计数及独立调用分母仍缺失，对应coverage保持unknown。

### 一次离线双Trace与回放输入

原件目录 `/opt/aries-artifacts/tb-repro-sqlite-v3-e2b-record-1`。现有汇总器与release10分析器各运行一次，完成 **10,711项检查、0失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。同一Snapshot导入Perfetto / Trace Processor的2,663个区间、219条Flow；OTLP导出2,663个Span、217个parent、2个link，最大时间误差0ns。integrity complete、coverage unknown，原Attempt的 `artifact_proven=false` 保持不变。

2个job与2个wait共4个原生区间缺可信UTC锚点，涉及的6条边也未导入主时间轴/OTLP，原件仍在Snapshot内。没有新增Jaeger/Prometheus后端查询或浏览器验收。三份投影核对SHA相同后硬链接共享，保留路径和内容，释放 **70,438,912字节**重复分配；原driver及Tape未改，无新云或模型调用。离线分析已结束并释放SSH及ECS窗口。

可直接用于下一次Strict自回放的Tape绝对路径：
`/opt/aries-artifacts/tb-repro-sqlite-v3-e2b-record-1/runs/20261026T075942.346881697Z-tb-sqlite-with-gcov-terminal-result-v3-record/sqlite-with-gcov-terminal-result-v3-001/model/tape.json`。

| 证据（上述原件目录内） | SHA-256 |
| --- | --- |
| 原driver `evidence.json` | `82e293d5ad38b0b166fff530d40454eab95346335ad8fd6cb3849d5fe8870893` |
| 原 `result-review.json` | `80f1633cf4f577169bfee5cbb4067a562eb6ef61fbe317ebcf80c9497519a611` |
| 完整 `model/tape.json` | `a73e482ea845c44b370df656d5358b8231a5c161e1acdbd5e7da1d8edda72d46` |
| 原生会话transcript | `97df092a7890fc12fcf988b130ef32b09f56f6948e73f41233292763ddd1718a` |
| 后台协议raw | `3ca9949eff27911bdc6c49c4d2db9dc057768f7ac634e94d5c41917de26ba3e7` |
| 模型请求边界 | `a1ec703db63172cb794049352931287e4085a6b82b0287217dac103ed52130eb` |
| `performance-review-1/summary.json` | `8ac86210766c88272221f39899a96b0ad045433098c12e63ae28d8b2c3e73faf` |
| `offline-analysis-1/evidence.json` | `2caecf51d62e6db8d2eecbafb91a0af0fa2607374c5f4e40d5e65e8c46067cf4` |
| `projection-dedup-1/evidence.json` | `cd5c0edc87cdb447f5ba00ea779b8745cda8983eae2abc29cce7585f69ac0b2d` |

本地小型汇总 `.cache/tb-repro-sqlite-v3-record1-audit-1/summary.json` 与 `summary.md` 保存原始ns、身份、Tape完整性及来源，不含工具正文或凭据。


<a id="sqlite-v3-e2b-replay-1"></a>

## SQLite terminal-result-v3：E2B replay-1 仍因日志中的时间偏离

**同一份完整Tape的首次Strict自回放失败：第25个模型请求发现安装日志中的三处时间不同，运行停止；原私有评分0/3、reward 0。** 已完成24轮匹配、22次真实exec和2次process/wait，所有实际调用都是原录制决策的精确前缀；原录制剩余16次exec没有执行。因此本次是保留在失败分母中的部分工作量，不能与[完整录制](#sqlite-v3-e2b-record-1)直接比较完整任务性能，也不构成跨云通过样本。

运行窗口为2026-10-26 08:10:46.690794—08:15:10.747238 UTC，原runner返回1、Harness failed。停止和Bridge撤权confirmed；仅创建1个Sandbox，provider与ledger均确认不存在，task及基础设施清理成功，cleanup errors为空。原driver的 `native_decisions_differ_from_real_tape` 是轨迹中断后未完成全部决策的验收结果；**首次失败原因是模型边界已记录的 `REPLAY_DIVERGED`**，不能据driver摘要误判成Agent先改变了调用参数。

### 首次偏离：日志保存后又被读取

第25个请求（零起点turn 24）的 `$.messages[49].content` 不同，对应wait调用 `call_00_2VvLMEuEjsdfLQNTpV247411`。两边均为1983个UTF-8字节、25行，仅第3—5行的安装库文件时间从 **08:03变为08:14**，合计6字节变化；其余正文、调用ID、参数和前序决策保持一致。24轮此前匹配成功，首次拒绝后另有5次 `SESSION_FAILED`，真实上游模型调用数为0。

原生协议的wait身份与accepted job精确关联到exec 22、模型轮23，ToolCall ID为 `call_00_00rjJN3XgiTZ656qW2hn8439`。该次真实命令先执行 `make install`，把输出重定向/追加到工作区日志，再通过 `tail` 读取日志；原始录制与回放命令参数完全相同。该命令没有管道，但以分号继续执行，最终shell exit 0来自末尾tail成功；make install的真实exit 2及Tcl权限错误保留在正文，不能把工具exit 0解释为安装成功。读取的尾部包含三条安装库文件元信息，因此变化时间重新进入了模型请求；[精确命令与原生格式归因](sqlite-v3-mtime-rootcause-20261026.md)已补齐。该命令没有显式 `background=true`，本次后台句柄来自原生自动转后台路径，不冒称已覆盖显式后台启动。

末次wait真实结果为completed / exit 0，`stdout-stderr-v1`、`outputTruncated=false`；归档ToolResult正文摘要与模型拒绝请求正文摘要完全一致。证据支持**公开附录未能避免日志回读带入无关动态信息**，没有发现本次renderer、Strict matcher或调用顺序的系统错误。保持真实日志、退出状态、原Tape及Strict规则；不以过滤结果、修改旧Tape或把未执行后续工作当成功来消除失败。

| 首次偏离正文证据 | SHA-256 |
| --- | --- |
| 原录制1983字节正文 | `39f4581cfacb7c2c0102d42d97f65263ee63cb488c69d50f4adf8d2d1f9170ce` |
| 本次真实1983字节正文 | `b314e5cfeefd3d5feae6241dd2a8beb72cb42fca95bfa38426becc0514deb937` |

### 八阶段耗时

单位为秒，名称对应[统一阶段说明](performance-stages.md)。原Attempt端到端为 **257.682796秒**；下列阶段不简单相加替代端到端。独立验证12.446562秒是完整Evaluate窗口，原CTRF自身约0.149秒，仅表示测试程序的另一计时边界；不能把两者之差直接命名为网络耗时。

| 阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000013 |
| 任务 Sandbox 创建 | 1.365993 |
| 工作区准备 | 1.269125 |
| Harness 启动 | 7.317234 |
| Runtime 执行 | 226.627944 |
| 停止与撤权 | 5.600912 |
| 独立验证 | 12.446562 |
| 清理 | 1.801883 |

### 已执行工具与尚未执行的工作

22次exec均精确关联到真实guest回执，**20次exit 0、2次exit 1**；没有把中间失败改成成功。表中SDK等待与guest整体均为毫秒，分别使用控制器和guest的边界，不跨时钟相减推算网络耗时。原录制exec 23—38未执行，不能填成0毫秒；模型等待及评分内部步骤在本小结未单独整理，保持未整理状态。

<details>
<summary>展开实际执行的22次exec</summary>

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit | 关联 |
| --- | --- | ---: | ---: | --- | --- |
| 1 | 1 | 447.531 | 134.085 | 0 | exact |
| 2 | 2 | 932.756 | 627.571 | 1 | exact |
| 3 | 3 | 339.394 | 52.561 | 0 | exact |
| 4 | 4 | 930.755 | 655.381 | 0 | exact |
| 5 | 5 | 331.670 | 47.002 | 0 | exact |
| 6 | 6 | 322.775 | 37.742 | 0 | exact |
| 7 | 7 | 326.427 | 36.414 | 0 | exact |
| 8 | 8 | 323.835 | 37.235 | 0 | exact |
| 9 | 9 | 334.470 | 36.127 | 0 | exact |
| 10 | 10 | 347.927 | 46.462 | 0 | exact |
| 11 | 11 | 2416.170 | 2131.783 | 0 | exact |
| 12 | 12 | 305.876 | 35.529 | 0 | exact |
| 13 | 13 | 100789.261 | 100512.430 | 0 | exact |
| 14 | 15 | 305.021 | 34.772 | 0 | exact |
| 15 | 16 | 319.691 | 36.588 | 0 | exact |
| 16 | 17 | 316.084 | 40.341 | 0 | exact |
| 17 | 18 | 355.513 | 52.025 | 0 | exact |
| 18 | 19 | 326.835 | 43.894 | 1 | exact |
| 19 | 20 | 397.845 | 111.968 | 0 | exact |
| 20 | 21 | 413.675 | 44.138 | 0 | exact |
| 21 | 22 | 1513.014 | 1226.820 | 0 | exact |
| 22 | 23 | 42584.992 | 42283.164 | 0 | exact |

</details>

两个process调用均为wait。单独复用既有协议验收器核对原件，得到 `PASS_ARCHIVED_NATIVE_JOB_WAIT_MAPPING`：2个accepted job、2次wait调用、2个wait invocation、8条原生协议事件；原driver和业务FAIL不变。两项真实终态均为completed / exit 0，没有details截断标志，本次不覆盖大于8192字节的details持久化路径。

| 后台任务 | accepted → terminal 秒 | wait 开始 → 返回 秒 | 原生终态 |
| --- | ---: | ---: | --- |
| 1（exec 13） | 100.797051662 | 98.293502984 | completed / 0 |
| 2（exec 22） | 32.591444909 | 30.361292346 | completed / 0 |

两列来自原生协议自身单调时钟；accepted不等同于guest启动，job和wait不与exec整体累加。原生Promise终态本身不单独证明guest退出；producer总事件、写入失败计数和独立调用分母仍为unknown。

### 一次离线双Trace与失败原件

原件目录 `/opt/aries-artifacts/tb-repro-sqlite-v3-e2b-replay-1`。现有汇总器及release10分析器各运行一次，完成 **5479项检查、0失败**，结论 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`。同一Snapshot投影出Perfetto / Trace Processor的1355个区间、114条Flow，以及OTLP的1355个Span、112个parent、2个link；最大时间误差0ns。该结论仅确认已采集证据的双出口一致，**不能把原Strict失败或原评分0/3改成通过**；integrity complete、coverage unknown。

2个job和2个wait共4个原生区间没有可信UTC锚点，相关6条边仍保留在Snapshot，未导入主时间轴/OTLP。没有新增Jaeger/Prometheus后端查询或浏览器验收。三份投影仅在SHA相同后硬链接共享，保留原路径与内容，释放29,859,840字节重复分配；本次诊断和离线分析没有新增Sandbox或模型调用。

| 证据（上述原件目录内，另有说明除外） | SHA-256 |
| --- | --- |
| 原driver `evidence.json` | `831d5552cef6814f39429a30ac02871fc8a76c860c021c9fe75cd2309af207e4` |
| 原 `result-review.json` | `cff8a5dfcfdea379fd8524b5ef8e8ed6c2ed4366fd73745f420fa00b4057d52a` |
| 完整原Tape（record-1目录内） | `a73e482ea845c44b370df656d5358b8231a5c161e1acdbd5e7da1d8edda72d46` |
| 原生会话transcript | `28b1b1702e9db55d9129bb4d893ebee4f82d9c2736519daf4c99a9593fb2d15a` |
| 后台协议raw | `3f992aca357fdcbc31191c0121ddfd8a38bdb8158f9a03590b1c9103a5dc835a` |
| 模型请求边界 | `111d64fc5605c627c9e23f9b2818a25c211372f722b10f9c8ee551b818d0fe09` |
| `performance-review-1/summary.json` | `65872b9d2360fed59aa586fe7524525369e0b43fb7f489aa9a0fdd02e5d33a17` |
| `offline-analysis-1/evidence.json` | `15d99f60575ba639f7552c956fba32ff09e3b1f03f7edf3114594aef340b4bb5` |
| `protocol-review-1.json` | `a0fcf8884c28eb733193d7071bcd7a262c15bb1b8edfb0e218cf956a62502a04` |
| `projection-dedup-1/evidence.json` | `9ba6c3f02637e33bcd9085a6a74c5f315c90db6fd28452ea4e0b52670883fc19` |
| 独立诊断目录 `diagnostic.json` | `8af4df110011aeebfb814a3ffc3b915dec033126da01971efac8b18191403aea` |
| 独立诊断目录 `source-context.json` | `cc4ecc3f62053d71d628484336e0e153e376ec763544c936ded4f5bed267a7b8` |

独立诊断目录为 `/opt/aries-artifacts/tb-repro-sqlite-v3-replay1-diagnosis-1`，补充receipt只新增，不回写原始FAIL。本地小型汇总 `.cache/tb-repro-sqlite-v3-replay1-audit-1/summary.json` 与 `summary.md` 保存精确时间、身份及去正文差异；不包含工具正文、私有断言或凭据。
