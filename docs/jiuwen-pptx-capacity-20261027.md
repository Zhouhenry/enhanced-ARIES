# Jiuwen PPT 任务：容量修复与复测记录

本页记录用户五页 PPT 任务的系统修复。固定提示词、真实模型、原生工具、独立评分和 Stop/revoke 门保持不变；容量通过不等于任务通过。

本轮按用户最新要求，优先完成原生 PPT case 的端到端交付，**不以 Strict Replay 通过作为功能验收门槛**。功能验收仍要求真实工具执行、独立五页非空评分、原件回传和资源清理。MockLLM 回放及同 Tape 性能对照单独报告，不能把真实模型录制成功改称回放成功。

## 首次真实运行

E2B A 的 `jiuwen-e2b-external-pptx-record-1` 使用 release24、image8 和 4 vCPU / 8192 MiB Sandbox，运行约 19.55 分钟，结果失败。其原始记录保留，后续复测单独编号。

| 实际问题 | 证据与影响 | 处置 |
| --- | --- | --- |
| 录制 SSE 达到帧数上限 | 恰好 65,536 帧；请求加 SSE 约 49.6 MB，尚未达到原 128 MiB 字节上限；Tape 不完整 | 增大帧数及相关字节预算，完整保留原始帧 |
| PPT 下载调用与接口限额不一致 | 清单中有 5 个约 38–120 KB 的 PPTX，调用固定传入 64 MiB，而旧接口最多接受 16 MiB，导致所有下载被拒绝 | 两云共用传输限额统一为 64 MiB；按实际清单文件大小请求下载，仍核对大小和 SHA-256 |
| 原生 SDK Trace 超过收集预算 | 文件 18,317,096 字节，旧预算 16 MiB，因此未保留；producer 记录 424 个 started/ended，不能据此认定未收回的 SDK 文件也有 424 条有效 span | 收集、传输及下游分析一并验证，保留 producer 对账与超限错误 |
| A 模式发送文件找错文件系统 | 原生 `send_file_to_user` 检查 Agent 本地文件，PPT 实际位于 Sandbox；三次返回文件不存在 | 从绑定 Sandbox 的原生 FS 接口取回任务目录文件，再调用原生发送方法；接入实际 session push 订阅 |

本次 Verifier 未运行，不能把生成过 PPT 解释成评分通过。正常清理已删除该 Sandbox；由于下载失败，首次五个原始 PPT 没有保存下来，无法事后恢复。清单中的路径、大小、哈希与失败记录保留，不能用重新生成的文件冒充。

## 统一容量

MiB 为 1,048,576 字节。以下是程序容量，不是工作量、性能目标或已测峰值。

| 环节 | 原容量 | 修订容量 |
| --- | ---: | ---: |
| 模型每轮请求 / SSE 响应 | 16 MiB | 64 MiB |
| 模型单个 SSE 事件 | 1 MiB | 8 MiB |
| Tape 请求和 SSE 原始字节总量 | 128 MiB | 256 MiB |
| Tape JSON 加载 | 256 MiB | 512 MiB |
| Tape SSE 帧数 | 65,536 | 2,097,152 |
| 模型边界队列 / 控制器事件队列，各自 | 4,096 | 16,384 |
| 两云单文件上传 / 下载 | 16 MiB | 64 MiB |
| 适配器 JSON 请求 / 响应（含 base64） | 24 MiB | 96 MiB |
| PPT 数量 / 全部文件总量 | 32 / 256 MiB | 128 / 512 MiB |
| PPT 单文件 | 64 MiB，但旧下载接口不兼容 | 64 MiB，上下游一致 |
| Jiuwen HTTP 单帧 / unary 响应 | 8 MiB | 32 MiB |
| Jiuwen 最终文本 / 选定事件保留 | 8 / 16 MiB | 32 / 64 MiB |
| Jiuwen 原生 SDK Trace 收集 | 16 MiB | 64 MiB |
| Jiuwen 进程、worker 证据及 Agent 日志，各自 | 16 MiB | 64 MiB |
| 私有产物本地读取完整性检查 | 16 MiB | 64 MiB |

公开模型 HTTPS 入口也必须与后端的 64 MiB 请求容量一致；仅修改 Go 常量不能消除反向代理的旧限制。实际部署验收另存回执。

18 MiB Trace 回归另检出了本地私有文件读取层的隐藏 16 MiB 上限，已经同步修正；权限、不可变文件及哈希检查保留。原生 push 只证明已消费的通知，协议没有 chat/push 跨流排空确认，不能宣称通知全量零丢失；PPT 原件保存和评分仍依据 Stop 后的独立目录清单与下载。

事件队列仍非阻塞，满队列仍累计 dropped；容量扩大不代表保证零丢失。Strict 完整请求匹配、ToolCall 因果校验、独立 PPT 结构评分和清理确认均未放宽。25 分钟任务、96 轮模型预算及平台 TTL 是显式实验配置，本次未自动延长。

## 验证与证据位置

- 两个固定适配器镜像通过 ECS 上 8 项检查，包含各自实际 64 MiB HTTP 上传/下载及 SHA-256 对账、超限拒绝；回执位于 `.cache/adapter-capacity64-1/closeout.json`。
- A 模式原生 remote 工具 17 项、producer audit 12 项 ECS 检查通过。真实 `send_file` 的文件物化用实际 Worker/FS，fixture 的最终 push sink 不能当真实云投递成功。
- 模型超过旧 65,536 帧的录制、加载、Strict 回放，以及大请求/帧边界由 ECS Go 测试验证。所有首次测试失败记录保留，不能覆盖后只报通过。
- 最终源码五门禁、四组真实 PPT 任务、独立评分、原文件回传和云清理需分别留证。尚未完成的项不继承历史编码任务或环境探针的 PASS。

固定 image8 的两云模板与原生依赖见 [部署说明](../deploy/jiuwenswarm/presentation/README.md)。PPT 结构评分定义见 [Benchmark 说明](../pkg/benchmark/codingcase/README.md#pptx-task-and-independent-structural-verification)。

## release25 真实复测与后续修复

release25 五项 ECS 发布门禁全部通过，不能据此代替以下真实任务结果。

| 尝试 | PPT 与独立评分 | 模型与整体状态 | 清理 |
| --- | --- | --- | --- |
| E2B A `record-2` | 原件 3 个：`content.pptx` 为 3 页中间稿；`final.pptx` 与 `大模型发展概览.pptx` 字节相同，均为 5 页且每页非空；score=1 | Agent 成功，但模型第 57 轮终帧已 Flush 后被标记取消；Tape 不完整，整体 FAIL | 两个 Sandbox 删除及独立查无确认 |
| 华为 A `record-2` | 未生成 PPT，score=0 | 5 轮流式请求后，原生 `task_tool` 子 Agent 发起 `stream=false` 请求，被仅支持 SSE 的录制入口拒绝；整体 FAIL | 两个 Sandbox 删除及独立查无确认 |
| E2B B `record-2` | 导出 2 个原件：`content.pptx` 为 5 页、每页非空；`final.pptx` 为 7 页不合格。现有“至少一份合格”结构评分为 1，不能称最终文件符合要求 | Agent 成功，64 轮完整 Tape；功能状态 PASS，但原生 Trace 下载失败，整体观测不完整，四出口未通过 | 两个 Sandbox 删除及独立查无确认 |

E2B A 原件已按 SHA-256 取回本机 `runs/jiuwen-pptx-20261027/e2b-a-record-2/decks/`。其 392 个已采样 SDK span 的 started/ended/exported 对账通过，不能据此宣称全流程全量覆盖或后端查询通过。

收尾修复先在 ECS 复现旧版录制/回放/缓存重试三个误判，再验证新行为；提前取消、终帧 Flush 失败、非法尾帧继续拒绝。新发布及真实复测结果另记，旧记录与评分保留。华为子 Agent 非流式适配独立推进，不用重复尝试碰运气规避 `task_tool`。

release26 已完成并冻结五门禁，仅覆盖 SSE 收尾修复；源码清单 SHA-256 为 `11c3a9fad8f67110216328e61dc77e4b1cd39f04c05dc38a542063a88bad35d7`。新增 JSON/子会话协议的定向包测试及 image8 原生客户端 fixture 已通过，合并发布与华为真实复测另验，见[协议说明](implementation/model-replay.md#jiuwen-子-agent-与非流式-json)。

release27 已在 ECS 完成并冻结五项发布检查：build 19.715 秒、test 83.393 秒、race 344.025 秒、lint 7.129 秒、真实容器 integration 340.585 秒；另外两个 Python fixture 检查通过。源码清单 SHA-256 为 `d66934dcc3799645c7dd6b3f44770099f828cced5311367cc5f8eeb5602b0720`，证据 SHA-256 为 `c619e11ac480d48f4b85cc9cce177d9edb209f7bf9655ade5068e1244856e5da`。这组检查不调用付费模型或云 Sandbox，不能替代云端验收。

华为 A `jiuwen-huawei-external-pptx-record-3` 和华为 B `jiuwen-huawei-embedded-pptx-record-3` 使用上述冻结版本、同一 image8 及 4 vCPU / 8192 MiB 模板，均已完成真实任务和独立结构评分，原件全部取回本机。两次模型均未选择子 Agent，不能把本次成功当成非流式 JSON 子轨迹的云端验收；B 的观测失败及原始证据另列于下文。

E2B B 两个原件均已校验 SHA-256 并取回 `runs/jiuwen-pptx-20261027/e2b-b-record-2/decks/`；完整 Tape 文件 SHA-256 为 `3c68d58d4e69a7126b85b9bc107408488d29947811e47f62865d10254f323caf`，未修改。B 的 19,770,951 字节原生 Trace 未成功收回，采样覆盖不能宣称完整；该样本也未完成 MockLLM 跨云对照。

## release25 单次阶段耗时

单位为秒；数据直接来自各次 `run-result.json`。八个任务内阶段另列排队；各次真实模型决策和实际工作量不同，以下不是同 Tape 的云性能排名。华为 A 是失败样本，短耗时不能解释成性能更好。

| 阶段 | E2B A record-2 | 华为 A record-2 | E2B B record-2 |
| --- | ---: | ---: | ---: |
| 任务 Sandbox 创建 | 1.386 | 0.699 | 1.011 |
| 工作区准备 | 1.025 | 1.578 | 1.514 |
| Bridge 准备 | 13.462 | 3.291 | 1.194 |
| Harness 启动 | 40.424 | 40.563 | 50.216 |
| Runtime 执行 | 830.681 | 91.777 | 1439.387 |
| 停止与撤权 | 4.171 | 1.927 | 5.216 |
| 独立验证 | 7.451 | 1.501 | 5.315 |
| 清理 | 1.452 | 1.291 | 1.488 |
| 排队（不含在任务总耗时） | 0.000026 | 0.000035 | 0.000020 |
| 任务总耗时（不含排队） | 901.489 | 142.773 | 1508.227 |
| 总耗时与八阶段之和的未细分差额 | 1.437 | 0.147 | 2.886 |

差额保留为未细分时间，不虚构具体组件归属。Runtime 字段使用原 Agent 起止语义，不能与包含整个 `Harness.Run` 的 Trace 区间互换；定义见[阶段说明](performance-stages.md)。E2B B 的 Trace 收集失败仍保留，阶段字段可读不代表完整观测已通过。

## release27 华为 A：真实 PPT 功能通过

`record-3` 的 Agent、独立结构 Verifier 均成功，Stop/revoke 确认，任务和评分 Sandbox 共两个均已删除并由 provider 独立确认 NotFound，清理错误为空。原提示词和评分规则没有改变；华为评分出网仍沿用显式假设，不升级为已验证隔离的 Goodput。

| 原始文件 | 页数与结果 | SHA-256 |
| --- | --- | --- |
| `content.pptx` | 3 页中间稿，不合格，保留 | `b21c83c771471a01d5bd48c2d3d405e2cac1c415cfc029422917ff32e4139551` |
| `final.pptx` | 5 页、每页非空，结构通过 | `74ecae7efa700aefe05d50e1c75a4230d7d54f7c763640a0d9d907e369fcce3b` |

两个原件已 SCP 回本机 `runs/jiuwen-pptx-20261027/huawei-a-record-3/decks/projects/llm-development/output/` 并逐个校验哈希。结构通过不等于内容事实、排版或美观评分；没有替换 Agent 生成的文件。

| 阶段 | 秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 0.806 |
| 工作区准备 | 1.043 |
| Bridge 准备 | 3.251 |
| Harness 启动 | 41.387 |
| Runtime 执行 | 1032.478 |
| 停止与撤权 | 2.737 |
| 独立验证 | 1.978 |
| 清理 | 0.915 |
| 排队（不含在任务总耗时） | 0.000020430 |
| 任务总耗时（不含排队） | 1086.403 |
| 总耗时与八阶段之和的未细分差额 | 1.808448 |

64 轮 v4 Tape 完整，SHA-256 为 `6a15976b408e008a238c34e60e417006a989963b42e4f4db1ea042bd13df9c17`；78 个模型 ToolCall 决策全部通过本次 attempt 内唯一精确 ID 关联到原生 authoritative Tool Span。子轨迹和 JSON 完成数均为 0，非流式支持仍仅有前置协议/原生客户端 fixture 证据。本次为真实模型录制，尚非同 Tape 跨云回放。

观测原件收回 18,965,778 字节 SDK Trace，450 个 sampled SDK started/ended/exported 对账一致，缺失及额外均为 0。记录 32 个命令进程、225 条生命周期事件，该来源 dropped/write_failed 为 0。112 次 Bridge 调用有 112 个 worker start/terminal；其中 110 条有 ToolCall 身份、2 条 unavailable，不能说全链身份均已覆盖。命令进程区间与 Runtime、原生 Tool、Bridge 区间嵌套，不能相加为任务耗时。

控制器进程生命周期累计 CPU 末值为 70.882 秒、RSS 高水位为 506.871 MiB；A 外置 Agent 容器累计 CPU 末值为 164.283 秒、内存采样最大值为 646.457 MiB。任务 Sandbox provider 返回 CPU 最大值 14.21%、`mem_used_mib` 最大值 7733 MiB；这属于 provider 的整台 Sandbox 口径，时间戳仍未经验证，不能解释为某条 Tool 的消耗。逐 Tool CPU/内存/I/O/Network 仍未采集。

同一校验快照的四项离线投影通过，`integrity=complete / coverage=unknown`。完整阶段、78 个工具动作、32 个进程区间和分对象资源位于本机 `evidence/acceptance-summary.json`（相对于上述 `huawei-a-record-3` 目录），SHA-256 为 `0e745240b94692de92a73b4bc6db8b721e4bf80dc693f36344714e155ad6f91f`。该原归档生成时后端仍 UNRUN，保留不改；ECS 原件目录为 `/opt/aries-artifacts/jiuwen-huawei-external-pptx-record-3`。

后续已对本次相同快照完成 Perfetto、Jaeger 和 Prometheus 实际查询验收，回执为 `PASS_BACKENDS_OF_REAL_RUN`，未调用云 Sandbox 或模型，也未修改原快照。Perfetto 的 Controller、SDK、Provider/worker 与 32 个命令进程视图对账通过；Jaeger 共 1,879 spans、22,559 项检查通过，v3 时间误差为 0 ns；Prometheus 的 Attempt、Controller、Agent 三流首次投递及相同 payload 重发查询通过。华为 provider 两流仍因时间戳未验证而 NOT_COLLECTED，global coverage 仍 unknown；legacy Jaeger 微秒与 Prometheus 毫秒精度截断单列，不宣称实时或持久化系统验收。自有后端容器、cgroup、端口均无残留，共享锁已释放。本机后验回执 `.cache/jiuwen-pptx-huawei-a-backends-1/evidence.json` SHA-256 为 `faf485d6167838c1d669bd0f851366b222787c64d8dbd3685bb9d912e298fe0f`，对应原快照 SHA-256 为 `89f37420ecd667c4886543274ddaf1a577bef9aaa86b6ce0025603802ae8c154`。

## release27 华为 B：功能通过，观测失败保留

`record-3` 已收尾，systemd 为 `inactive/dead`、`Result=success`。Agent、独立结构 Verifier 均成功，驱动状态为 `PASS_NATIVE_FLASH_PPTX_RECORD_FUNCTIONAL_ONLY`；Stop/revoke 完成，任务和评分 Sandbox 共两个均删除并由 provider 独立确认不存在，清理错误为空。该状态仅代表功能通过；原生观测失败没有被覆盖。华为评分出网仍 assumed，不能升级为已证明隔离的 Goodput。

| 原始文件 | 页数与结果 | SHA-256 |
| --- | --- | --- |
| `content.pptx` | 3 页、每页非空，不满足五页要求；中间稿保留 | `d96c56ca9cf7c7173f5678409e99db4ba41d86c30441accf447c8fad0a712b14` |
| `final.pptx` | 5 页、每页非空，结构通过 | `2ca829dbe0316d40e1a3b0096b111c1d9a8e1af9ec3dbc7ac56511b7158d5c69` |
| `template-clean.pptx` | 5 页，其中 2 页为空，不合格；模板保留 | `65e90eccfc58c5aca90449e50665ad377d03021c91359eb2b1cee12f732976e4` |
| `大模型发展.pptx` | 5 页、每页非空，结构通过；与 `final.pptx` 字节相同 | `2ca829dbe0316d40e1a3b0096b111c1d9a8e1af9ec3dbc7ac56511b7158d5c69` |

四个原件和八份小元数据已 SCP 回本机 `runs/jiuwen-pptx-20261027/huawei-b-record-3/`，原件位于 `decks/projects/llm-development/output/`，逐个 SHA-256 校验完成。`copy-receipt.json` 为 `ORIGINALS_COPIED_SHA256_VERIFIED`。两份合格文件内容相同，不计为两次任务成功；结构评分不评价事实正确性或排版美观。

| 阶段 | 秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 0.643 |
| 工作区准备 | 0.837 |
| Bridge 准备 | 0.135 |
| Harness 启动 | 22.576 |
| Runtime 执行 | 1324.941 |
| 停止与撤权 | 3.753 |
| 独立验证 | 2.213 |
| 清理 | 0.804 |
| 排队（不含在任务总耗时） | 0.000029300 |
| 任务总耗时（不含排队） | 1359.209 |
| 总耗时与八阶段之和的未细分差额 | 3.307739 |

任务耗时来自本次 `run-result.json`，未细分差额按原始纳秒计算；驱动全程 1414.377 秒还包含任务外准备、导出和收尾，不能与 Runtime 等同。Tool、SDK、命令进程与 Runtime 存在嵌套，不加总为任务总耗时；两次独立模型录制的工作量不同，A/B 本表不构成同 Tape 性能排名。

61 轮 v4 Tape 完整，全部为主分支 SSE，子轨迹和 JSON 完成数均为 0；原 Tape SHA-256 为 `f31d9799e43b28fafbdb4167234bc65244674aac53121aea2ebcd98a745338f7`。本次有 80 个模型 ToolCall 决策；原生事件清单保留 243/243 条、未截断，包含 80 个唯一身份工具请求。由于原生 Span 来源被完整性检查拒绝，80 个 Tape 动作到快照的关联全部为 `unknown_native_evidence_missing`，不能宣称工具因果全量验收通过。该 Tape 未做 MockLLM 回放，也未完成同 Tape 跨云对照。

| 观测来源 | 本次事实 | 结论与缺口 |
| --- | --- | --- |
| SDK Trace 下载 | 下载源 18,791,628 字节，脱敏后保留 18,791,643 字节；文件错误 `credential_redacted` | 下载成功，不是旧版下载容量或 flush 超时；公开路由端口 `18866` 被误登记为敏感值，3 处误替换导致内容/完整性失败。在内存中仅恢复该公开值后，大小与 SHA 均等于下载时原值，未写恢复文件或覆盖原件；不是已证实的密钥泄露 |
| SDK producer 对账 | sampled started/ended/exported=334/334/334，缺失及额外 0；Stop 退出已观察 | 对账通过不能抵消 `collection_status=failed`。`native_observability.go` 的脱敏错误传入 index，`snapshot_jiuwen.go` 因 `index.Error` 拒绝该来源 |
| 原生事件 | 243/243 条、`protocol_succeeded=true`、`truncated=false` | producer coverage 仍 unknown；80 个唯一请求不等于 80 个可信工具执行区间 |
| 命令进程 | started/exited=39/39，275/275 事件，dropped/write_failed=0；observer 已关闭、pending=0 | 保存 39 个 command、39 个 creation、7 个 exit-to-pipe-close 区间；该来源仍 incomplete，不能因数量配对就判通过。缺口包括精确身份/终态审计、自然 pipe EOF、所有后代终止及跨域时钟对齐 |
| 同快照四项离线投影 | snapshot、Perfetto、OTLP、Metrics 均退出 1 | `integrity=incomplete / coverage=incomplete`；本次后端查询 UNRUN，不借用 A 或 release23 的结果 |

控制器累计 CPU 末值 109.496 秒、RSS 生命周期高水位 610.473 MiB。Sandbox 内 Agent 进程组及后代的 cgroup 累计 CPU 末值 93.579 秒，生命周期内存高水位 856.406 MiB，四边界采样的内存最大值 739.961 MiB；这些都不是单 Tool 指标。任务 Sandbox 的 273 份 provider 回执中 CPU 返回最大值 25.74%、`mem_used_mib` 最大值 7787 MiB，provider 时间戳未验证。块 I/O 仅有分设备读数，不相加堆叠设备或伪作应用读写量；任务 Network 与逐 Tool 资源未采集。

本机 `evidence/acceptance-summary.json` SHA-256 为 `37973706d7ea5602d1f205c907fcdaeb3e8d0145c12e4e007892d31ec3b704d8`，原件传输包 SHA-256 为 `72b667e054c9f3b1e3be84d8cb1a45cdfb7594e5571b0f9a09dad7a28090d311`。ECS 原件位于 `/opt/aries-artifacts/jiuwen-huawei-embedded-pptx-record-3`；该目录下 `port-redaction-root-cause.json` 的 SHA-256 为 `5ccb579830286270bcf190e7afb8fee6913ca92a791f8606b07d58003f634efe`，仅记录上述根因核验。当前报告保持原失败，修复后的新验收须另立证据。

### 公开端口误脱敏修复与下一轮

修复将三个 Header 登记点统一到 `Credentials.AddGuestHeaders`，仅排除大小写无关的公开路由字段 `e2b-sandbox-port`；API Key、访问令牌和未知 Header 的脱敏规则保持。ECS 隔离候选 `guest-header-redaction-1` 的 harness、Jiuwen、OpenClaw 三包测试 12.340 秒、race 22.962 秒均通过；回归覆盖端口数字位于 Span ID、JSON 数字及正文时不误改，真实认证/未知字段仍脱敏。本机 `.cache/guest-header-redaction-1/evidence.json` SHA 为 `03c25cc89b07b7d923b3bbc50b32dcb368fb3b4d27bcb719645c9a078827d745`。

release28 已在 ECS 完成 build 21.058 秒、test 84.250 秒、race 343.074 秒、lint 6.829 秒、integration 345.608 秒并冻结；容器/网络库存恢复、临时缓存清理、锁释放。源码清单 SHA-256 为 `88b8990bc903708140dc9385c3c146f320646b1c86374daac35552edf8d0d6f5`，工程证据为 `3a7f173eb3dc458e556b71b362b681a72f965edfd0e8e3bbeb1f27e0452e37d1`，freeze manifest 为 `868fd8e8696c33c03aa1047b0cd1b0d1658dc3735088214a0d14dd2cf5fe068a`。不含固定提示变体，冻结 release27 未修改。工程通过不改判华为 B `record-3` 的观测失败；修复后的真实任务使用新的 `record-4`。

E2B A `jiuwen-e2b-external-pptx-record-3` 使用冻结 release27，已完成下述真实复测。公开端口修复已在 release28 发布检查通过，随后串行复测华为 B，不覆盖旧失败。

## release27 E2B A：PPT、完整录制与离线观测通过

`record-3` 的 Agent、独立 Verifier 均成功，score=1；Stop/revoke 确认，两个 Sandbox 独立查无，Agent 容器、网络、模型监听器均无残留，清理错误为空。沿用 image8、4 vCPU / 8192 MiB、25 分钟和 96 轮预算，Runtime 未超时。三个原件全部取回本机 `runs/jiuwen-pptx-20261027/e2b-a-record-3/`，复制回执逐项 SHA 校验通过。

| 原始文件 | 独立评分 | SHA-256 |
| --- | --- | --- |
| `output/final.pptx` | 5 页，5/5 非空，通过 | `f5a9f770f04056d33610a71c005291ca9d1a88843e5cd8ca17e5bf6178fd6d31` |
| `output/content.pptx` | 3 页，不通过，中间稿保留 | `3822fc16562b78a8daee80107e64baf00c0cc7237427aa5ea5f8c58d3b5ba9fc` |
| `sources/template-clean.pptx` | 5 页，其中 2 页为空，不通过，模板保留 | `654b969c1ed7c0ba84f4ce3c7190e548bfede8a3a3b651a5bd98168c2a109cf0` |

| 阶段 | 秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 1.065 |
| 工作区准备 | 1.937 |
| Bridge 准备 | 12.264 |
| Harness 启动 | 41.901 |
| Runtime 执行 | 1422.830 |
| 停止与撤权 | 4.825 |
| 独立验证 | 6.298 |
| 清理 | 1.500 |
| 排队（不含在任务总耗时） | 0.000030798 |
| 任务总耗时（不含排队） | 1495.148 |
| 总耗时与八阶段之和的未细分差额 | 2.526877 |

驱动全程为 1571.367 秒，含任务外准备、导出与收尾；上表纳秒来源为本次 `acceptance-summary.json.stages`。50 个命令进程的直接子进程执行窗口累计 30.593 秒，与 Runtime、工具请求及 SDK Span 嵌套，不相加为总时间，也不等于 CPU 时间。

控制器累计 CPU 末值为 148.720 秒，进程生命周期 RSS 高水位为 618.508 MiB；A 模式 Agent 容器累计 CPU 末值 226.655 秒，1466 次采样的内存最大值 670.086 MiB。任务 Sandbox 的 provider 返回 CPU 最大值 10.86%、内存最大值 466.180 MiB。Provider 返回包含重复历史读数，不能把 44,555 条回执当作独立采样次数；上述内存最大值也不是每个 Tool 的峰值。评分 Sandbox 未取得资源数值，任务级 Network 及逐 Tool CPU/内存未采集。

83 轮主分支 SSE、89 个 ToolCall 的 v4 Tape 完整，SHA-256 为 `12a2350a25016f0d691ab5e0ff859878c084c5f03a1f193edd4b03b060b15446`。89 个 ToolCall 均按唯一精确 ID 关联原生工具 Span；563 个 sampled SDK Span 的 started/ended/exported 对账一致，缺失和额外均为 0。50 个命令进程 started/exited 对账、348 条事件保留，dropped/write_failed=0，进程专项通过。自然 pipe EOF、所有后代终止和跨主机时钟对齐仍未证明，不能把专项通过写成全链覆盖。

同一快照的 snapshot、Perfetto、OTLP、Metrics 四项离线投影均通过；`integrity=complete / coverage=unknown`。本次后端查询为 UNRUN，不借用华为 A 的后端 PASS。子轨迹与 JSON 完成数均为 0，不能据此新增 JSON 子 Agent 云端验收结论。这是新的真实录制，并非 MockLLM 回放或同 Tape 跨云性能对照。

证据：本机 `evidence/acceptance-summary.json` SHA-256 为 `878a7ed021cf2e960696b39e3798715baaebc27eb44494b55a6783488ac6c653`；`copy-receipt.json` 为 `54a4300b38553845bb144b7b2ce000068f2b5897a065b52674f4f265aef28139`；ECS 原件根目录 `/opt/aries-artifacts/jiuwen-e2b-external-pptx-record-3`。

## 本轮性能如何解读

先看每次运行自己的工作量和时间组成。E2B A `record-3` 有 83 个模型轮次、89 个 ToolCall，华为 A `record-3` 则有 64 轮、78 个 ToolCall；它们都是自主真实模型生成，不能用 1422.830 秒对 1032.478 秒直接得出某云更快。CPU/内存规格一致，磁盘差异仅作平台条件说明；当前主要的对照缺口是决策工作量不同，而非要求底层磁盘完全一致。

华为 A 的同一校验快照另提取了以下量，证据 `.cache/jiuwen-pptx-cloud-3/huawei-a-analysis.json`，SHA-256 `48fcbafb1541219be05f432a28899c70b1c51f9ee33e0bc8505b274c62d2ca2e`：

| 量 | 本次值 | 实际测量边界 |
| --- | ---: | --- |
| Runtime | 1032.478 秒 | Runner 开始 Agent 任务到任务返回；含 Agent Loop、模型请求、工具与协调 |
| 64 次模型 HTTP 请求窗口并集 | 962.124 秒 | 模型代理 handler；含请求处理、模型上游等待、传输与交付，不等于模型计算时间 |
| 78 个原生工具 Span 累计 | 50.412 秒 | 原生 SDK 工具请求范围，可能含通信、包装及错误处理 |
| 32 个命令进程窗口累计 | 37.968 秒 | guest 中直接子进程启动回调至退出回调；非 CPU 用量、非所有后代结束 |
| 模型请求读取 / 解析 / 匹配累计 | 0.691 / 0.355 / 2.452 秒 | 模型请求内部细分，已经包含在 HTTP 窗口中，不能再加一次 |
| 等待上游响应头累计 | 45.511 秒 | 只到响应头，既不是 SSE 首内容帧，也不是完整模型响应 |

本次耗时很大一部分出现在模型请求窗口，不能归因于 Sandbox 的命令运行。各来源有独立时钟域，命令、工具和 Runtime 又有嵌套，不能用“Runtime − 模型 − 工具”制造精确 Harness 开销；当前没有对这些跨域区间做全局关键路径归因。

华为 A 的 Agent 容器内存采样最大值约 646 MiB，而 provider `mem_used_mib` 最大值为 7733 MiB；两者对象和统计语义不同，不能据此宣称 Agent 用满 8 GiB 或泄漏。Provider 时间戳尚未验证，保留其原读数，但不据此积分 CPU、推导内存曲线因果或比较两云内存效率。

后续同 Tape MockLLM 对照中，每个 placement 固定一份 Tape，真实工具和独立 Verifier 继续执行。`recorded` 时序保留录制的模型等待，`zero` 时序去掉回放器的计划等待；两组分别报告，不能混在一张耗时排名中。当前 Jiuwen 原生 PPT 的此项真实回放尚未完成，不作为本轮功能交付的阻塞项。

<a id="huawei-b-record4"></a>

## release28 华为 B：新 PPT 与观测复测通过

`jiuwen-huawei-embedded-pptx-record-4` 已完成真实 DeepSeek 录制、Agent 执行、独立结构评分和清理，score=1。沿用 image8、4 vCPU / 8192 MiB、全部原生技能及 25 分钟 / 96 轮预算。两个 Sandbox 均独立查无，Stop/revoke 确认；华为评分出网隔离仍是 assumed。此结果验证公开端口误脱敏修复后的真实观测链路，**不改判旧 record-3 的失败**。

三个原始 PPT 已按 SHA 取回本机 `runs/jiuwen-pptx-20261027/huawei-b-record-4/decks/`：

| Sandbox 工作区内相对路径 | 文件级结果 | SHA-256 |
| --- | --- | --- |
| `projects/llm-development-5p/output/final.pptx` | 5 页，5/5 非空，PASS | `cab8f269e2c7ea68c0d36a7e328fdf100d171b23889b6da987e3a7667a989d01` |
| `大模型发展脉络与趋势.pptx` | 与上述 final 字节相同，PASS | 同上 |
| `projects/llm-development-5p/output/content.pptx` | 3 页中间稿，FAIL，原件保留 | `73fca4410b10ffbd96d9a1d3075d4a1005cf09d4762fce61ff02b607778826fe` |

这是一个任务产生的三个文件，其中两个是同一成品。当前独立 Verifier 检查“至少一份可读、五页且每页非空”，不把其他中间稿也算合格，不评价内容事实、排版或最后一次发送文件的选择。

| 八阶段 | 秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 0.775 |
| 工作区准备 | 0.903 |
| Bridge 准备 | 0.125 |
| Harness 启动 | 22.587 |
| Runtime 执行 | 1082.419 |
| 停止与撤权 | 3.049 |
| 独立验证 | 2.219 |
| 清理 | 0.711 |

排队另计 0.000019358 秒，Attempt 为 1115.901001303 秒且不含排队；driver 为 1169.440673 秒，还包含部署、收集和导出。Runtime 内包含模型等待与真实工具，逐工具时间不能再加到 Runtime 上；各层时钟也不能直接相减。

本次完整 Tape 为 47 个 root SSE 回合，0 child / 0 JSON。73 个 ToolCall 均按精确 ID 关联；SDK 264 个 sampled span 的 started/ended/exported 一致。36 个命令进程、252 条事件完整对账，drop/write_failed/identity_failed/pending 均为 0。Snapshot integrity=complete、global coverage=unknown；四个离线出口通过，本次 Perfetto 导入、Jaeger/Prometheus 后端查询仍 UNRUN。不能把已采集范围完整解释成全部内部环节覆盖。

CPU、内存按归属展示，CPU 秒表示实际累计用量，内存采样最大值与生命周期峰值分别列出：

| 归属与窗口 | CPU | 内存 | 测量边界 |
| --- | --- | --- | --- |
| ECS 共享控制器，start/end | 累计增量 114.215821 CPU 秒 | 进程生命周期 RSS 峰值 458.566 MiB | 包含同进程模型代理，不是 Sandbox 用量 |
| Sandbox 内 Agent cgroup 及后代，四个边界 | 最终累计 90.012348 CPU 秒 | cgroup 生命周期峰值 856.379 MiB；四次当前内存采样最大值 713.918 MiB | 包含原生 Agent 与其后代工具；不是只覆盖 Runtime 的峰值，也不是逐工具用量 |
| 华为任务 Sandbox 平台接口，224 条读数 | 返回样本最大 25.65% | 返回样本最大 7763 MiB | provider 时间语义未验证，与 guest cgroup 指标不等价 |

guest cgroup 四边界的累计 CPU 秒依次为 start=0、runtime_start=20.589707、runtime_end=89.712755、stopped=90.012348。设备 `254:0` 在 runtime_start/runtime_end/stopped 的累计读字节分别为 167284736/440537088/442900480，写字节均为 0；设备 `254:16` 仅在 runtime_end/stopped 有读 466944、写 8540160 的读数。start 的 io.stat 无设备计数；不补零，不合并设备制造“完整 Runtime I/O”，Network 仍未采集。

证据来自 ECS `/opt/aries-artifacts/jiuwen-huawei-embedded-pptx-record-4`。本机 `evidence/acceptance-summary.json` SHA 为 `0cc1d6cc2094ef861f70169026827bd30faf9718eebaca5a1572057d9b3aebf6`，`copy-receipt.json` 为 `9ec5f9006a46bd44749501c704619cbfd26381cc99209207a7f02652a7913a5f`；原 Snapshot 的资源边界提取回执 `evidence/resources-boundary-addendum.json` 为 `1e834e642ac7293fa492cd0755e94ddb24797f01eb48dbc356f3a2d9cd574d61`，无新采样。原 Tape SHA 为 `f77f9a523f04bfee38a00a30d2e097e2e6aa2872ce8243c5be1525f109e61d74`；已选定的后续 B 跨云 Workload 仍复用旧 B3 Tape，不因这次独立录制通过而换 Tape。
