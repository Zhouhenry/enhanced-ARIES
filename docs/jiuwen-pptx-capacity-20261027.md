# Jiuwen PPT 任务：容量修复与复测记录

本页记录用户五页 PPT 任务的系统修复。固定提示词、真实模型、原生工具、独立评分和 Stop/revoke 门保持不变；容量通过不等于任务通过。

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

华为 A `jiuwen-huawei-external-pptx-record-3` 使用上述冻结版本、同一 image8 及 4 vCPU / 8192 MiB 模板，已完成真实任务和独立结构评分。该次模型没有选择子 Agent，不能把本次成功当成非流式 JSON 子轨迹的云端验收。华为 B `record-3` 已于 2026-10-27 20:55:38（北京时间）接续启动，结果另验。

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

同一校验快照的四项离线投影通过，`integrity=complete / coverage=unknown`；本次 Perfetto/Jaeger/Prometheus 后端查询尚未执行，不能借用 release23 的查询结论。完整阶段、78 个工具动作、32 个进程区间和分对象资源位于本机 `evidence/acceptance-summary.json`（相对于上述 `huawei-a-record-3` 目录），SHA-256 为 `0e745240b94692de92a73b4bc6db8b721e4bf80dc693f36344714e155ad6f91f`。ECS 原件目录为 `/opt/aries-artifacts/jiuwen-huawei-external-pptx-record-3`。
