# ARIES 性能证据附录

本页保存实验取证与历史记录。先阅读[性能报告](performance-archive.md)、[独立阶段指南](performance-stages.md)及[指标定义](performance-archive.md#阶段与指标定义)；本页不是另一套阶段定义。

本页保存已进行的性能实验、失败运行、功能探针、准备及发布检查，便于追溯每个结论。
基础清单是 **2026-10-23 12:15:21 UTC 的 135 份证据文件快照**，不是 135 次实验。
同一次运行的 `evidence.json`、`run-result.json`、性能分析和复核报告可能分别占一项。
后续补充列在增量记录中，不覆盖历史失败或把正在进行的工作写成成功。

所有构建、测试、实际运行及离线性能检查均在 ECS 执行。本页整理已有结果，不发起云资源或模型请求。
详细字段定义见[运行结果指南](run-results.md)，实验形态见[架构指南](architecture-guide.md)。

## 读表口径

- A 为 Agent/Harness 外置、工具在云 Sandbox；B 为 Agent/Harness 与工具内嵌在同一个云 Sandbox。
  A/B 描述放置位置，E2B 或 Huawei 描述供应商，二者不是同一维度。
  `record` 使用真实模型并录制；`recorded` 按录制的代理时序回放；`zero` 不主动等待，但代理、网络和 Agent 仍有实际开销。
- “完成”表示 Agent 执行完成；“验证”要求独立 Verifier 和停止/撤权证据，不只看 Harness 或进程退出码。
  清理另行确认。作业耗时包含准备、构建、启动和收尾，不能直接作为 Sandbox 创建或 Agent 延迟。
- 已验证吞吐为独立验证通过的任务数除以完整观测窗口。`AttemptSummary.observation_duration` 的边界是
  `runProfile` 开始至所有任务尝试（attempt）返回，包含排队、任务执行、任务清理及 attempt 收尾；
  不包含此前的 run 级共享准备、此后的共享清理及最终 Trace 落盘，不能等同整个实验作业耗时。
  本页已列性能条件均未配置 SLO，**Goodput 未定义**，不能填为零。
- 总览延迟换算为秒；详细事件另列原有纳秒或毫秒单位。表中中位数直接取原分析的 `median_descriptive`，不是重新合并样本计算。
  单个或几个样本不能证明最大容量、稳态吞吐、可靠 p95/p99、厂商排名或因果性能提升。
- A/B 使用不同真实轨迹，早期基线和后续批次的 Runner/Adapter 版本也有变化；不把八个条件混成同条件重复实验。
  `complete`/`READ_COMPLETE` 只说明对应分析或记录检查完成，不替代任务正确性、网络隔离或整个实验验收。
- 对 Huawei 显式假设模式，现有计数器不会自动从 validated throughput/Goodput 中剔除结果。
  报告必须按 `infrastructure.network_policy` 分组，把
  `assumed-huawei-verifier-egress-unverified` 写成条件性结果，不称为禁止出网已实测通过。
  该假设不豁免停止、撤权、独立评分及清理；旧失败不因此改写。

阶段统一使用：**排队、任务 Sandbox 创建、工作区准备、Harness 启动、Runtime 执行、停止与撤权、独立验证、清理**。
产物延迟、验证延迟是从计划到达开始累计的指标；完整观测窗口是上述调度器任务执行范围，三者不是额外阶段。
模型请求累计与原生工具耗时是内部活动，也不能再加到阶段耗时上。各边界沿用[主报告定义](performance-archive.md#阶段与指标定义)。
本页保留原摘要的显示精度；主报告统一显示三位小数，不表示两份报告采用不同计时口径。

| 文中术语 | 含义 |
| --- | --- |
| W2 / W3 / P0 | 项目中的故障验证工作项 / 性能测量工作项 / 本期优先范围，不是运行阶段 |
| SLO / Goodput | 约定的延迟或截止时间目标 / 在目标内完成且独立验证通过的任务吞吐；未约定目标时不计算 |
| p95 / p99 | 第 95 / 99 百分位延迟；本页的小样本不能给出可靠估计 |
| SSE / TTFT | 模型流式响应事件 / 首 token 延迟；代理首 SSE、首内容观测不等于模型内部 TTFT |
| Trace / UNALIGNED | 事件及区间轨迹 / 时钟域尚未对齐，不能按泳道位置推断跨进程先后 |
| Tape / flush | 模型录制轨迹文件 / 代理的本地写出刷新；不是 Agent 实际收包时刻 |
| RSS / CPU-seconds / I/O | 常驻内存 / 累计 CPU 时间 / 输入输出；都不同于任务墙钟耗时 |
| bytes / MiB / ns | 字节 / 1,048,576 字节 / 纳秒；毫秒为千分之一秒，纳秒为十亿分之一秒 |
| VPC / VPCEP / SWR / OBS | 私有网络 / VPC 终端节点 / 镜像仓库 / 对象存储服务 |
| ENVD / SDK / RPC | Sandbox 环境服务进程 / 云接口客户端库 / 远程调用 |
| cgroup / UID / PID | Linux 进程资源组 / 用户标识 / 进程标识，用于停止和隔离取证 |

## W3：八个条件的性能结果

来源：ECS `/opt/aries-artifacts/runtime-performance-summary-1/analysis.json`，
SHA-256 `999dcec423948c1d4f18524973362a20dc44e5e41e5d9c01347bb63dbdcad282`。
逐任务 CSV 为同目录 `attempts.csv`，SHA-256
`c5977c3ca583b7f3a4afad360c18e7174e0122868a8b7ba845d9e19b1cd3ed4a`。
此历史 E2B 集合包含 **8 个条件、12 个已接受任务**；它是明确选择的成功集合，不是全部历史任务的成功率分母。

| 条件 | 并发 | 完成 / 计划（任务数） | 独立验证通过（任务数） | 完整观测窗口（秒） | 产物延迟中位数（秒） | 验证延迟中位数（秒） | 已验证吞吐（任务/秒） |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| A 真实录制 | 1 | 1 / 1 | 1 | 48.099 | 40.633 | 45.974 | 0.02079 |
| A recorded 回放 | 1 | 1 / 1 | 1 | 49.966 | 42.101 | 48.025 | 0.02001 |
| B 真实录制 | 1 | 1 / 1 | 1 | 52.019 | 44.809 | 49.695 | 0.01922 |
| B recorded 回放 | 1 | 1 / 1 | 1 | 48.197 | 42.946 | 46.736 | 0.02075 |
| B zero 回放 | 1 | 1 / 1 | 1 | 38.097 | 31.874 | 36.689 | 0.02625 |
| A recorded，2 次顺序 | 1 | 2 / 2 | 2 | 94.796 | 66.756 | 71.287 | 0.02110 |
| B recorded，2 次顺序 | 1 | 2 / 2 | 2 | 93.436 | 63.337 | 67.788 | 0.02141 |
| B recorded，并发 2、共 3 个任务 | 2 | 3 / 3 | 3 | 78.258 | 41.592 | 46.052 | 0.03833 |

“产物延迟”是计划到达至稳定候选产物，“验证延迟”是计划到达至独立验证完成，均包含排队。
八个条件的失败和清理失败计数均为零，但仅对该选定集合成立。并发度 2 的三个任务确实有两个 Agent
同时运行、第三个等待容量释放；这证明本次调度行为，不证明最大并发容量。
源分析没有把跨进程时钟相减拼成全局关键路径，也没有把 SDK 重复读数视为独立资源样本。

### 代理开销与计时边界校准

`replay-cost-calibration-1/evidence.json` 的范围是 ECS 本地回放服务进程，
状态 `MEASURED_LOCAL_REPLAY_PROCESS_ONLY`。两种时序各 5 次，以下为描述性中位数：

| 时序 | 服务进程 CPU 时间（秒） | 从启动到回收的墙钟时间（秒） | 峰值 RSS（bytes） |
| --- | ---: | ---: | ---: |
| zero | 0.156275 | 0.169568 | 35,430,400 |
| recorded | 0.214611 | 11.124875 | 35,561,472 |

这组检查不含真实 Harness、Tool 或云 Sandbox，不能替代端到端开销校准或据此扣除所有观测开销。
原证据 SHA-256 为 `57dd809bbef719f3e812251db310f3d6b1253daf4d787fcbb2fd2b75b0ff86e4`。
`five-baseline-boundary-calibration-1/analysis.json` 检查了五项单次基线，各有 28 条 Runner 事件、12 对操作；
对应 SHA-256 为 `412e0d23daf4cb3f43ff6321f53e11ebd8a358a8b31fcbf4c6a9c733059e12fc`。
完整观测开销、跨机器时钟精度和资源采样误差尚未全面验收，缺失测量不写成零。

<a id="当前三次已接受运行的-w3-离线增量"></a>

### 2026-10-23 三份已接受运行的 W3 离线增量

2026-10-23 13:11:25 UTC，ECS `/opt/aries-artifacts/runtime-w3-current-runs-1/`
完成 `PASS_READ_ONLY_CURRENT_RUN_W3_SUMMARY`。输入为新 Runner 的 E2B A/B recorded
回归及 Huawei record3；24 个输入文件前后 SHA-256 不变，无云、模型、Docker 或编译调用。
这是对已有运行的补充分析，不是三次新增实验，也不改变原有评分、隔离或失败结论。
本节范围不含稍后完成的 Huawei recorded 回放；最近四次运行的合并展示见主报告。

| 文件（上述 ECS 目录） | SHA-256 |
| --- | --- |
| `summary.json` | `31a353038cf25afe4a51732c15820af49df60189a442979024ba118d104347f8` |
| `evidence.json` | `02014e3a4e173196c0c6704f95b0e9dfa39c9cf4e05863506770eccf2b6b65fa` |

模型时序逐轮保存 `first_sse`、`first_content`、`first_tool_call_delta` 和完整请求耗时，
单位为纳秒。A 的 7 轮中，首 SSE 覆盖 7 轮、首内容 5 轮、首工具增量 6 轮；
E2B B 与 Huawei 各 4 轮，分别覆盖 4、4、3 轮。三者均未观察到 reasoning 增量，
缺失类别保持空值，不记作零延迟。录制保存代理收到上游的相对时刻，recorded 回放保存
录制目标和实际本地 flush 时刻；两者都不是模型内部 TTFT、Agent 收到响应时间或实际 token 计数。

`request_admission_to_finished_ns` 使用请求接纳至结束的既有计时字段；此前 Trace 的模型区间
从校验后的 `request_started` 事件开始，边界不同，不能互相覆盖。以 Huawei 四轮为例，
前者为 2.689244、2.308623、2.918764、2.973601 秒；此前 Trace 的四个区间仍按原口径保留。

| 运行 | 原生工具调用数 | 原生工具耗时，按调用顺序（`durationMs`，毫秒） | 错误工具调用数 |
| --- | ---: | --- | ---: |
| E2B A recorded | 6 | 314、340、318、765、482、338 | 2 |
| E2B B recorded | 3 | 34、198、182 | 1 |
| Huawei B record3 | 3 | 22、467、310 | 1 |

原生耗时来自 OpenClaw 的墙钟差值，包含原生 exec 的准备、启动和等待；A 还包含 SSH/Bridge
链路，不能解释为纯 Sandbox 执行时间。B 同时保留控制端收到工具 start/result 的单调时间区间，
不与 guest 墙钟相减。工具结果不独立证明进程及后代全部退出，`process_completion` 继续为 unknown。
A 另外有 25 次 Bridge 请求，均 completed，耗时范围 0–712 毫秒；尚未建立与 6 次原生工具的
逐项对应，两个层级的数量和耗时不能相加。

| 运行 | 任务资源查询次数 / 返回读数条数 / 唯一时间戳数 | 重复读数条数 | 评分资源查询次数 / 返回读数条数 | 任务 CPU 采样最大值 | 任务内存采样最大值 |
| --- | --- | ---: | --- | ---: | --- |
| E2B A recorded | 10 / 44 / 8 | 36 | 2 / 0 | 23.23% | `mem_used` 319,066,112 bytes |
| E2B B recorded | 11 / 50 / 9 | 41 | 2 / 0 | 62.4% | `mem_used` 1,080,852,480 bytes |
| Huawei B record3 | 10 / 10 / 10 | 0 | 2 / 2 | 62.6% | `mem_used_mib` 3832 MiB |

这些是各次采样范围，不是真实瞬时峰值或可直接排名的资源效率。E2B 的单位由已固定 SDK
字段说明核对，时间戳是 `timestamp`，报告名义周期 5 秒；累计返回的重复时间戳不算新样本。
Huawei 时间戳字段为 `ts`，单位和供应商更新周期未知，不能从控制端 5 秒轮询设置推断。
仅显式带单位的 Huawei 字段用于换算，其余 `mem_*`、`disk_*` 保留原数值和 unknown 单位。
E2B 评分环境两次查询均无读数，表示未覆盖，不能填写零消耗。
CPU 百分比沿用各后端读数，本节没有将不同供应商的读数校准为统一容量基准。

本增量明确了已有时序和资源字段的来源、单位、重复与缺测。CPU-seconds、磁盘/网络 I/O
及 Agent/Tool 分进程资源归因仍未采集；后者是当前采集实现的缺口，不等于供应商永远不支持。
跨域关键路径、真实瞬时峰值、完整观测开销和更深资源归因留作精度提升，不新增为本期 P0
退出条件；未知字段和 Huawei 出网假设仍必须随结果披露。

## 失败、准备失败与预期故障

### 已实际进入任务的失败

| 记录 | 计划 / 完成 / 独立验证通过（任务数） | 已知结果与保留范围 |
| --- | --- | --- |
| `e2b-formal-external-record-2` | 1 / 1 / 0 | 模型仅一轮，后续请求拒绝，Tape 不完整；隐藏测试失败、评分 0。隔离 confirmed，任务和共享清理 succeeded。完整性能事件保留，不能作为成功回放材料。 |
| `e2b-runtime-batch-a-c1-2-1` | 2 / 1 / 1 | 旧批次存在失败及原始证据丢失；不补造，不并入 W3 成功集合。后续独立 `-2` 批次通过不改变它。 |
| `agentsphere-embedded-record-1` | 1 / 0 / 0 | Gateway 的三个 core library manifest 检查失败，模型零轮；Verifier 断网要求另被拒绝。停止/撤权及清理有记录，不能当完整 Huawei Agent 验收。 |
| `agentsphere-embedded-record-2` | 1 / 0 / 0 | 插件启动错误已消失，但连接没有 `operator.write`，模型零轮；Verifier 仍被 422 拒绝。任务与共享清理成功。 |

上述失败当前保留的 `evidence.json`、`run-result.json` 见下方 135 项索引；旧批次已丢失的原始证据不在其中，不宣称全部原件齐全。
`e2b-formal-external-record-1` 则在提交 Runner 前因结果目录缺失失败，前后创建计数均为零，未产生正式 Benchmark 任务结果。

### 网络、协议与准备失败的解释边界

| 记录 | 可以确认的事实 | 不能据此声称 |
| --- | --- | --- |
| `agentsphere-network-metrics-1` | 公网模板显式请求禁止出网后，仍两次连接自有 ECS 公网 443；该检查 FAIL。指标仅一份 ENVD 快照。 | 禁止出网通过；固定采样周期已知。 |
| `agentsphere-explicit-network-1`、`agentsphere-vpc-egress-1` | 阳性对照创建遇到 snapshot not found，未到达私网出站检查；后续对账单列。 | 已测得私网阻断效果。 |
| `agentsphere-vpc-egress-2` | 延迟后的同一公共模板创建成功、文件/命令和公网 TCP/TLS 正对照通过；private 模板构建 error，private Sandbox 数为零。 | 平台内部根因已定位；安全组或私网隔离通过。 |
| `agentsphere-vpc-egress-3` | SWR/OBS VPCEP readback 为 accepted，同 VPC 有路由；pull/flatten 时对 SWR 的 DNS 查询 `169.254.169.253:53` 超时，终态日志已保留。private Sandbox 数为零，临时模板 DELETE 204 / GET 404。 | DNS 超时已归因于安全组、VPCEP、路由或平台；禁止出网已验收。 |
| `gateway-device-auth-probe-1` | 配对后原生返回 read+write，原 PoC 错误期待仅 write，断言失败；负例当时未执行。独立 probe-2 后续通过。 | 第一次完整鉴权 PoC 成功。 |
| `agentsphere-runtime-release-1` 首轮 | `make integration` 因缺 fixture 失败；失败证据保留，独立重试另列。 | 首轮完整发布已通过。 |

VPC、子网、安全组与用户购买的 VPCEP 是保留的网络资源，不与临时 Sandbox、临时模板混为清理失败。
快照中 `huawei-vpcep-readback-1` 的 `PENDING_USER_PURCHASE` 是该作业保留的旧状态；后续 accepted
回读事实单列，不把旧状态解释为当前未购买。第四次私网重测的终态见增量记录。
其余失败只按原记录的证据类型归类；没有直接核实的原因不做推断。

### 原字段为 FAIL 的快照记录

以下单列基础快照内原 `status` 字段为 FAIL 的 34 份文件，完整 SHA-256 在末尾索引按编号查阅。
这不是 34 次终态失败实验：E050 在 12:15 快照时尚无 `finished_at`，FAIL 是运行中作业的默认值，
不能据此判断当时已失败。其后真正首轮失败的终态证据与 hash 在增量记录中单列，原快照不改写。
构建、依赖、测试断言、发布和文档检查失败不是正式 Benchmark 任务失败；不能混用分母。
一个失败实验的不同文件仍可能占多项。缺少顶层 `status` 的 `run-result.json` 不被自动当成 PASS，
实际任务失败已按上表的 attempt 字段说明。

| 索引 | 原字段为 FAIL 的文件 | 证据类别（不是运行阶段） |
| --- | --- | --- |
| E013 | `artifacts/agentsphere-adapter-candidate-1/evidence.json` | 单测、SDK 与发布检查 |
| E014 | `artifacts/agentsphere-adapter-candidate-2/evidence.json` | 单测、SDK 与发布检查 |
| E017 | `artifacts/agentsphere-adapter-live-1/evidence.json` | 协议与功能检查 |
| E018 | `artifacts/agentsphere-adapter-live-2/evidence.json` | 协议与功能检查 |
| E019 | `artifacts/agentsphere-adapter-live-3/evidence.json` | 协议与功能检查 |
| E021 | `artifacts/agentsphere-compatibility-1/evidence.json` | 协议与功能检查 |
| E022 | `artifacts/agentsphere-compatibility-2/evidence.json` | 协议与功能检查 |
| E023 | `artifacts/agentsphere-compatibility-3/evidence.json` | 协议与功能检查 |
| E024 | `artifacts/agentsphere-compatibility-4/evidence.json` | 协议与功能检查 |
| E026 | `artifacts/agentsphere-embedded-record-1/evidence.json` | 运行结果与实验复核 |
| E028 | `artifacts/agentsphere-embedded-record-2/evidence.json` | 运行结果与实验复核 |
| E030 | `artifacts/agentsphere-explicit-network-1/evidence.json` | 生命周期与网络检查 |
| E033 | `artifacts/agentsphere-image-1/evidence.json` | 镜像、部署、文档等配套证据 |
| E037 | `artifacts/agentsphere-native-discovery-preflight-1/evidence.json` | 协议与功能检查 |
| E041 | `artifacts/agentsphere-network-metrics-1/evidence.json` | 生命周期与网络检查 |
| E050 | `artifacts/agentsphere-runtime-release-1/evidence.json` | 发布检查；快照时未终态，FAIL 为默认值 |
| E051 | `artifacts/agentsphere-vpc-egress-1/evidence.json` | 生命周期与网络检查 |
| E053 | `artifacts/agentsphere-vpc-egress-2/evidence.json` | 生命周期与网络检查 |
| E054 | `artifacts/agentsphere-vpc-egress-3/evidence.json` | 生命周期与网络检查 |
| E056 | `artifacts/baseline-matrix-docs-1/evidence.json` | 镜像、部署、文档等配套证据 |
| E069 | `artifacts/e2b-formal-external-record-1/evidence.json` | 运行结果与实验复核 |
| E070 | `artifacts/e2b-formal-external-record-2/evidence.json` | 运行结果与实验复核 |
| E077 | `artifacts/e2b-runtime-batch-a-c1-2-1/evidence.json` | 运行结果与实验复核 |
| E086 | `artifacts/e2b-template-cloud-20261022-v1/explicit-build-verification-2/job-evidence.json` | 协议与功能检查 |
| E087 | `artifacts/e2b-template-cloud-20261022-v1/job-evidence.json` | 协议与功能检查 |
| E090 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/diagnostic-1/evidence.json` | 协议与功能检查 |
| E091 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/diagnostic-2/evidence.json` | 协议与功能检查 |
| E092 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/evidence.json` | 协议与功能检查 |
| E108 | `artifacts/gateway-device-auth-probe-1/evidence.json` | 协议与功能检查 |
| E110 | `artifacts/gateway-go-device-auth-probe-1/evidence.json` | 协议与功能检查 |
| E115 | `artifacts/huawei-verifier-network-1/evidence.json` | 生命周期与网络检查 |
| E119 | `artifacts/native-dialect-roundtrip-1/evidence.json` | 协议与功能检查 |
| E121 | `artifacts/native-id-dialect-validation-1/evidence.json` | 协议与功能检查 |
| E125 | `artifacts/openclaw-2026.7.1-linux-amd64/apt-https-probe/evidence.json` | 协议与功能检查 |

### 预期故障检查

`e2b-w2-faults-1/evidence.json` 状态为 `PASS_COMPONENT_FAULT_PROBES`，SHA-256
`405c1e1e1ebefb79a9bdbea2bee44e19ebd4ffc083f6eb78dafed08353c2ba63`。
三项为 SSH 工具连接中断、内嵌 Agent 超时、精确注入停止 RPC 不可用。
停止不可确认时，评分被阻止并保留清理失败状态，最终删除整只沙箱。
这类 PASS 表示预期故障处理符合检查，不表示任务执行成功；后两项使用脚本模型，
不计入真实模型或严格回放成功分母。检查后当时 33 条 E2B 资源记录均确认 absent。

## 本轮功能、定向验证和 Trace 增量

本节“本轮”指 2026-10-23 的归档批次，各子项按各自时间与版本保留，不代表新的实验批次。

### 原生鉴权与定向测试

| 检查 | 结果 | 范围与证据 |
| --- | --- | --- |
| `agentsphere-device-assumption-check-1` | PASS_TARGETED_CHECKS | ECS 隔离源码目录：6 个 Go 包的并发数据竞争检查（race）通过；Huawei 15 项及 E2B 24 项单元测试（包含已安装 SDK）通过，无跳过。没有云/模型/Docker 调用，该证据本身不等于正式 Huawei 任务通过。 |
| `gateway-go-device-auth-probe-1` | FAIL | 第一次准备/执行失败保留，不改写成重试结果。 |
| `gateway-go-device-auth-probe-1/retry-1` | PASS_GO_NATIVE_GATEWAY_DEVICE_AUTH | 真实 Go Client 与固定原生 OpenClaw Gateway：识别专用配对错误类型 → 按请求 ID、设备、公钥及权限精确批准与回读 → 同一设备身份重连，返回 operator.read+operator.write。检查容器已删除；该证据本身不等于 Huawei 全链路验收。 |

定向测试 evidence SHA-256 为 `a4b8c4dac5ae55c18fdac4156f6ac56e59e93ad732c144702efd88a8552d9280`；
Go 原生互通重试 SHA-256 为 `1699a2d72aa461cf67ff3ebcb251d6ed57529f6aba4127eae8c1b6f4528290af`。
这两份已包含在基础清单，不重复计为新实验。

### 可查看的离线 Trace

2026-10-23 12:17:05 UTC，ECS `/opt/aries-artifacts/trace-export-check-1/` 完成，
状态 `PASS_TRACE_EXPORT_CHECKS`。`go test -race ./pkg/performance ./cmd/aries-perf` 与独立 analyzer build 均通过。
随后用两份既有真实结果执行导出；无新云、模型或 Docker 调用，生产源码未改写。

| 输入结果 | Trace 事件数 | Runner 区间数 | 模型请求观测区间数 | 原生工具观测区间数 | 原始证据 |
| --- | ---: | ---: | ---: | ---: | --- |
| 已验收 E2B B recorded replay | 636 | 12 | 4 | 3 | 20 个源文件前后 SHA-256 不变 |
| Huawei 失败 record2 | 21 | 10 | 0 | 0 | 15 个源文件前后 SHA-256 不变；失败阶段保留 |

第二行的零表示该次确实未进入模型/工具执行，不是把缺失耗时填零。
模型与工具使用明确的 `UNALIGNED` 独立时钟域，每条泳道的零点不同，不能据水平位置推断同时发生。
工具 start/result 是控制端收到的原生事件区间，区别于 guest 内原生 `durationMs`；`process_completion` 为 unknown，没有凭命令匹配猜测父关系，
也没有用统计差值或已有 tape 创建时间拼接跨域关键路径。导出不包含命令、对话、模型载荷或原始错误正文。
Chrome Trace / Perfetto JSON 产物已生成；本轮未验证浏览器 UI 导入。操作步骤见[离线 Trace](run-results.md#viewable-offline-trace)。

| 增量文件（ECS 路径前缀 `/opt/aries-artifacts/trace-export-check-1/`） | SHA-256 |
| --- | --- |
| `evidence.json` | `bf4f4d854e17318f9099deaedcb06578adcfcce84da45eaec197b0b727fb63fc` |
| `aries-perf` | `d35964c25ccc134652cdac720fcc024cc44339063f0d1ebdc1a166ff0db7d18c` |
| `formatted-go.tar.gz` | `4ee3c59394fa8e34ab0a32a58721d87d8d32a4ce87c57adb6e1fdb066593fd20` |
| `analysis/b-replay/trace.json` | `963c2a7eae24397839a95c625ace456602cd54d6ca2a3958364f3f5a5e97878a` |
| `analysis/huawei-failed-record2/trace.json` | `fc9d10ca762f2b22c220eaf378edf6f20cc1d03e69a07458259c595b8294c533` |

每份派生 Trace 旁保留 `source-hashes.json`；测试日志、545 文件源码前后清单和输入/格式化包留在检查目录。
该导出验证不新增性能实验样本，也不把 record2 改写成成功。

### 后续准备与中止记录

| 记录（ECS `/opt/aries-artifacts/`） | 结果与边界 | evidence SHA-256 |
| --- | --- | --- |
| `agentsphere-runtime-release-1/evidence.json`（首轮终态） | 首轮真正终态为 FAIL：`make integration` 缺 fixture。此 hash 对应终态文件，与 E050 的运行中快照 hash 区分保存。 | `3241dae72017979a3f451ea34b35fbedc6ed8b9b1014a3cf440077ec90ec10e9` |
| `huawei-vpcep-readback-2/evidence.json` | 只读前置核验 PASS：专用网络及 SWR/OBS 检查通过，当时安全组出方向规则数为零，临时输入删除。不是 Sandbox 内部禁止出网测试。 | `c346e34595a17f624a4a5d1b83d48f2dd269fed68c8bbe3a73473c8799118ea0` |
| `agentsphere-runtime-release-1/retry-1/evidence.json` | Git 全局配置覆盖使 fixture 保持 LFS pointer；用户要求优先私网复测后，该重试已取消并清理。没有完整发布 PASS。 | `54be362f22d45ae31980459264386fed9f801ecc76938dd88376066bd5cc8b62` |
| `agentsphere-runtime-release-1/retry-2/evidence.json` | 恢复原 ENV、补齐 fixture 后 integration 通过；随后离线全模块元数据枚举失败，整份作业仍记 FAIL。没有重复执行已通过的前四项 make。 | `399db3206a881ac940c7b4fe10f64006a9cee3d4c6a79f3ad05beb195aed5bf5` |
| `agentsphere-runtime-release-1/release-images-1/evidence.json` | PASS_RUNTIME_RELEASE_CANDIDATE：绑定相同源码及前述实际检查，补齐镜像 build/save/load、实际二进制依赖元数据及归档。该检查没有切换生产入口；同一 Docker 服务实例中恢复不等于新机器恢复。 | `bccd5044de268533707ed0c74cb59f9c618f0b40f2c53b08bf164bcfee72e4d4` |
| `agentsphere-record3-cleanup-check-1/evidence.json` | PASS：ECS 5 个确定性清理检查，覆盖已知 Sandbox 与未知分配同时存在时先清理已知资源。零云调用，不当作真实云清理验收。 | `638e3456c51a2481df1d3eb4ecf4058356a77142250ceb3c94c1c5851f8e24ae` |
| `e2b-gateway-disconnect-compile-1/evidence.json` | PASS_COMPILE_ONLY_NOT_EXECUTED：真实 Gateway 断连实验的私有测试已在 ECS gofmt、race 编译通过；这份检查没有执行测试函数或云实验，后续真实断连结果见下文。 | `12f191a464d5912450972d9b7a8982547e169e1aa76abef21852c81a97ed1093` |
| `agentsphere-vpc-egress-4/evidence.json` | 12:25:00.627–12:27:48.862 UTC，FAIL：私网模板构建仍在解析 SWR 时访问 `169.254.169.253:53` 超时，`NA.95004`。0 个 Sandbox、1 个临时模板；模板删除后 GET 404，输入已删，借用网络零变更。 | `6ff73f5f602d9cf8de8f7db5fbbdc7ebd2bc7787eaa13223f0c92fea01d7c360` |
| `agentsphere-vpc-egress-4/review.json` | 核对 60 份 HTTP 归档及删除回执；没有额外云请求，原件未改。E2B 账本保持不变。 | `7e8610414c0053c209130d2cf7ef0f5b23651a1d02963beeed49978bfa05a598` |

第四次私网测试使用既有专用网络、固定镜像 digest、2 CPU / 4096 MB。构建提交至终态状态采集
为 **167.411 秒**，整个脚本含清理 **168.235 秒**，仅 1 次失败样本。这是客户端观测时间，包含
构建轮询等待，不能作为 Sandbox 创建延迟或平台内部构建耗时。没有进入 Sandbox 文件、命令或
出站路径测试；底层 DNS 超时的原因仍未确定。VPC、子网、安全组与用户购买的 VPCEP 均保留。
按用户最新要求，后续复用现有配置直接运行脚本，不每次重复独立网络/VPCEP 查询；保留脚本必需的
构建等待、结果采集及清理确认。

用户另报告全放通可构建成功，并提出先 build 后收紧出方向。这是下一轮验证线索，尚不是我们
完成的受控对照。收紧后应新建评分 Sandbox，检查冷启动、命令和文件通路，以及被禁止目的地的新连接；
已有出站长连接可能沿用旧规则。DNS、SWR、OBS 是否仍是运行依赖须实测，不能由构建完成推断。
任务 Sandbox 访问模型与评分 Sandbox 的禁止访问要求应分别配置；本轮没有据此修改安全组。

### Huawei Embedded 真实录制：条件性通过

`agentsphere-embedded-record-3` 于 2026-10-23 12:42:34.924–12:44:37.579 UTC 执行。
Runner 使用发布候选 `4d8a240d…b33c`，Huawei Adapter 为 `052dbf29…3231`，
Sandbox 镜像固定为 `d33b073f…1afc`，OpenClaw 2026.7.1，2 CPU / 4096 MB。
这是公共模板上的单次真实 DeepSeek 录制，不是私网验收或 MockLLM 回放。

独立复核 `PASS_SINGLE_HUAWEI_RECORD_ASSUMED_EGRESS`，7 项检查通过：

- 计划、准入、执行、完成、验证通过均为 1；失败、阻断及清理失败为 0。
- 4 轮真实模型调用、693 个观测 SSE 帧；3 次原生 exec、9 条工具事件。
  其中一次工具 exit 1 后 Agent 继续修复，真实失败没有从轨迹中删除。
- 候选产物由独立 Verifier 执行 12 项测试，4 项公开、8 项隐藏，全部通过，score 1。
  Harness 停止和 Bridge 撤权确认先于评分；不是只根据退出码认定成功。
- 创建 2 个 Sandbox（任务和新建评分环境），均确认 absent 并有独立 GET 404；
  临时模板、Runner、Adapter 及凭据输入已清理，E2B 原账本未变。

`network_policy=assumed-huawei-verifier-egress-unverified` 仍是明确假设。
评分与吞吐只能作为该条件下的结果；本次没有证明禁止出网，不能算完整隔离验收。
原生工具证据包含命令及结果元数据，尚无逐个工具的完整独立结果正文/PID 快照；
严格回放和 Tape loader 校验也不能由这份录制复核代替；后续 loader 与协议检查已通过，
见下方独立的九项协议预检；随后真实 Huawei 回放也已通过，详见后面的回放验收记录。

| 文件（ECS 前缀 `/opt/aries-artifacts/agentsphere-embedded-record-3/`） | SHA-256 |
| --- | --- |
| `evidence.json` | `3b7b71cac4904df80a977db39afce278f65ccf39190f466841ae6cf8b630d61e` |
| `review.json` | `ba0c632d69b39b058ff41017d5cd96e47a6e46f43bd8b96329001e2f4940e01f` |
| 该 run 下 `merge-ranges-001/model/tape.json` | `3fc9751b0ef20c16707c6d968f7aa46a65b70b7d4302a075e9e27c8b8fd233f5` |
| `analysis-1/evidence.json` | `ff09cf80a8ea1d93ca59209ea000f2714b7021c28ece6d2b29fe99c9da8147a5` |
| `analysis-1/summary.json` | `925b3c153f7fa5523c56175a658728a5d5862fd2cd39295cbe0046c7a881a34a` |
| `analysis-1/trace.json` | `32f5be63b01c33de411a11be6fadf637cce00d346180375f6294ce1b45323419` |
| `analysis-1/adapter-status-review.json` | `d3320fc53abaf9f69e198e70bfb11141ee04f79a303d58095ba68de9cdf9565e` |

离线分析 `PASS_RECORD3_OFFLINE_PERFORMANCE_AND_TRACE`，26 个输入文件前后 hash 不变。
以下均为单次描述值，不与先前 E2B 轨迹做厂商性能排名：

累计指标与完整观测窗口：

| 指标 | 单次观测值（秒） |
| --- | ---: |
| 完整观测窗口 | 42.241566 |
| 产物延迟 | 38.617314 |
| 验证延迟 | 41.484173 |

本段原摘要列出的阶段耗时如下；排队未在该摘要中列出，不补为零。主报告从原始运行结果另行提取了排队字段。

| 阶段 | 单次观测值（秒） |
| --- | ---: |
| 任务 Sandbox 创建 | 0.553092 |
| 工作区准备 | 0.970961 |
| Harness 启动 | 22.623844 |
| Runtime 执行 | 13.876201 |
| 停止与撤权 | 0.177438 |
| 独立验证 | 3.157645 |
| 清理 | 0.730677 |

创建是控制端观测的操作耗时，不是供应商内部启动时间；模板构建和脚本准备不在此任务窗口内。
累计指标与阶段有包含关系，阶段之间也有未单列的开销，不能把表内数值简单相加。
条件性已验证吞吐为 **0.023673 任务/秒**，未配置 SLO，Goodput 未定义。
4 次模型 Trace 区间为 2.609683、2.212284、2.811392、2.866606 秒，边界是校验后
`request_started` 至请求结束；它们不是主报告用于“模型请求累计”的请求接纳至结束耗时。
两种观测均包含代理/传输，不等于模型计算时间，也不是额外运行阶段。

Trace 共 756 个事件，含 12 个 Runner、4 个模型、3 个原生工具区间，diagnostics 为空；
模型和工具泳道仍为独立时钟零点，不能据此绘制跨域关键路径。559 对 Adapter 操作中有
45 个 failed 和 41 个 canceled；早期 review 的 `sdk_operation_failures=86` 实际合计所有非成功状态。
这些子操作不等于 86 个失败任务，也不能因最终任务成功而忽略。`exec.wait` 中 39 次 exit 1、
1 次 exit -9、5 次 canceled；缺乏完整命令关联，具体原因尚未归因。原证据保留，不解释成全是预期探测。

### 新 Runner 的 E2B A/B 严格回放回归

`e2b-runner-regression-ab-1` 于 2026-10-23 12:55:01 UTC 完成，
`PASS_E2B_NEW_RUNNER_AB_RECORDED`；随后根任务独立复核
`PASS_ROOT_REVIEW_NEW_RUNNER_AB`。使用同一新 Runner `4d8a240d…b33c`，
原 E2B Adapter `69ffe664…a11` 和已接受的固定模板，A、B 各一次、顺序执行，真实模型调用为零。
完整请求历史和工具结果由 strict replay 核对，没有拒绝、重试或真实模型兜底。

| 条件 | 模型轮数 / SSE 帧数 | 独立测试通过数 / 总数 | 完整观测窗口（秒） | 产物延迟（秒） | 验证延迟（秒） | 已验证吞吐（任务/秒） | Trace 事件数 |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| A recorded | 7 / 1175 | 12 / 12 | 45.177484 | 39.079510 | 43.794432 | 0.022135 | 1237 |
| B recorded | 4 / 573 | 12 / 12 | 46.986314 | 41.256585 | 45.637235 | 0.021283 | 636 |

两项均确认停止与撤权、候选哈希一致、独立验证通过、任务及共享清理成功。
新增 4 个 Sandbox 全部 absent，当时正式账本累计 37 条均 absent；后续断连故障检查增至
39 条、全部 absent，见下节。该次收尾后仅原 Adapter/Caddy 保持运行。
两条轨迹不同、每项仅一次，表格用于兼容性回归，不用于 A/B 因果性能排名。
未配置 SLO，Goodput 不计算；Trace 独立时钟域限制同前。

| 文件（ECS 前缀 `/opt/aries-artifacts/e2b-runner-regression-ab-1/`） | SHA-256 |
| --- | --- |
| `evidence.json` | `4089c5082840e243e7c844bc0e1cf03b05173160fe9de829e5d2a70ab795da9b` |
| `root-review.json` | `b59e998ea88de5c9f818804b160bc48b43b33b283c92e2c91806a293179dcae4` |
| `a/evidence.json` | `71aa33117ba4b1fb75b461493f45b94c2a3765a973ba230c0691a7bb5f705414` |
| `b/evidence.json` | `a2be0e30c0e44f3795fa3a28896d72f0639cf9d03b943d322da5ae45f7517b85` |
| `a/performance-analysis.json` | `d1febe2c31fb2dccd8092aa507f0e7b3be852d5d8b9c35f03b5cfbffa18d15e3` |
| `b/performance-analysis.json` | `7e2ed269ff94c99141951abefce92c7357b97ecbe912104914d914421e7f2048` |
| `a/trace.json` | `364cd41ecaa36764b1712f06bd51110504bf43868f0d32b5ca038610186083b4` |
| `b/trace.json` | `dc8787260ab4b30689f5e58295e4acdf7f97a898b179ab891fddb2eff5563ec2` |

### 内嵌 Gateway 真实断连故障

`e2b-w2-gateway-disconnect-1` 于 2026-10-23 13:06:10 UTC 终态
`PASS_EMBEDDED_GATEWAY_DISCONNECT`，只运行新增的单项故障测试，测试作业墙钟耗时 29.75 秒，
不是 Runtime 执行阶段耗时。实际使用当时已编译的内嵌实现、真实 E2B 与 OpenClaw Gateway，模型为脚本场景。
确认父/脱离会话的子进程及持续变化的心跳后，控制端关闭真实 WebSocket；没有由注入器
直接调用 Harness.Stop 或取消任务上下文。Runner 保留“已接受但执行结果未知”的失败，
随后正常停止与撤权；评分前两次独立观察确认 cgroup 为空、UID 进程为零、心跳停止。

新建独立 Verifier 对原始未修复产物评分：12 项测试中 11 项失败、0 error/skip，score 0，
这是预期故障结果，不能算 Benchmark 成功样本。任务/评分两个 Sandbox 清理确认，
正式账本从 37 增至 39 条、全部 absent。控制端子进程和模型监听也已退出。

根任务独立复核 `PASS_ROOT_GATEWAY_DISCONNECT_REVIEW`；26 条 Runner 事件及派生 Trace
均完整，原始文件 hash 不变。该 Trace 的 17 个事件覆盖 Runner 阶段与注入边界，
不能替代断连后缺失的原生工具终态。此测试证明控制端主动关闭 WebSocket 的路径，
未测试服务端崩溃、网络丢包或模型流断连的全部组合。

| 文件（ECS 前缀 `/opt/aries-artifacts/e2b-w2-gateway-disconnect-1/`） | SHA-256 |
| --- | --- |
| `evidence.json` | `619378578647c2602ef9b503ad78794b5b4783ebd9b629ca06518592b5ee37c7` |
| `analysis-1/review.json` | `db6cf80bf43ee5c16fc20f9f18bf831a3551b8cde953e99df7e5f523faeb9e4e` |
| `analysis-1/performance-summary.json` | `863e7dea225a727e9662533788b9e9242fb83925ba07b907f55d782a83ab8c7f` |
| `analysis-1/trace.json` | `e418d0e645528db95067201f753e4d073fb4c15ba0fcdcf9a61a4a20c16f9029` |

### Huawei record3 的本机协议预检

`agentsphere-record3-protocol-check-1` 于 2026-10-23 13:03:34.412–13:03:46.135 UTC
完成 `PASS_HUAWEI_RECORD3_MODEL_PROTOCOL_ONLY`，复用已接受的 Go 检查器，9/9 项通过：
完整 Tape 校验、损坏校验和拒绝、zero/recorded 顺序回放、旧历史/旧 Tool Result/schema 偏离拒绝、
两个并发会话。负例先接受 3 轮，再返回 409；恢复正确请求也保持 SESSION_FAILED，不能继续蒙混通过。
并发每会话 8 个请求含 4 个幂等重试，各发送 1386 帧，未串用会话。

recorded 正例 4 个请求、693 帧，墙钟 10.898012 秒；录制末帧等待总和 10.884580 秒，
最大本机 flush 延后 1.250 毫秒。zero 正例为 57.227 毫秒。这些是 ECS 本机回环（loopback）协议耗时，
没有真实 Harness、Tool、Sandbox 或模型上游调用，不能称为云端严格回放性能。
原 Tape 与输入未改、进程组已退出，没有占用公网模型入口或部署锁。

原件位于 ECS `/opt/aries-artifacts/agentsphere-record3-protocol-check-1/`，
`evidence.json` SHA-256 `077974f21296e0d7bd6ad37e3e7e3d5f4646a9b78ef2f53e77e24b5869e8d038`；
同目录 `results/summary.json` SHA-256
`603301ba86d2537d60f9e70eece88815d5bc3ce4621228e48bdeb69cdacef42f`。

### Huawei 真实 Sandbox 的 recorded 严格回放

`agentsphere-embedded-replay-1` 于 2026-10-23 13:19:16.964–13:21:21.579 UTC
完成，随后独立复核为 `PASS_SINGLE_HUAWEI_REPLAY_ASSUMED_EGRESS`，7/7 项通过。
使用与 record3 相同的 Runner、Adapter 和 Sandbox 镜像，重新创建模板及任务、评分
Sandbox；只读加载 record3 的真实 Tape，未注入 DeepSeek Key，也未调用真实模型。

真实 OpenClaw 完成 4 轮全历史严格匹配、693 个 recorded SSE 帧、3 次原生 exec，
没有请求拒绝、重试或回退。一次工具 exit 1 与录制一致，后续修复产物的 SHA-256
与 record3 相同，独立 Verifier 的 12 项测试全通过，score 1。
本次新 Sandbox 的 hostname 与录制一致，没有归一化请求；这不是所有后续实例稳定性的保证。
Agent 停止与工具撤权先于候选捕获和独立验证；两只 Sandbox 均独立 GET 404，模板、
Runner、Adapter 及临时凭据已删除。E2B 39 条 absent 账本及既有服务保持不变。

累计指标、完整观测窗口与吞吐：

| 指标 | 单次观测值 | 单位 |
| --- | ---: | --- |
| 产物延迟 | 39.459349 | 秒 |
| 验证延迟 | 42.253704 | 秒 |
| 完整观测窗口 | 42.982427 | 秒 |
| 条件性已验证吞吐 | 0.023265 | 任务/秒 |

本段原摘要列出的阶段耗时如下；排队未在该摘要中列出，不补为零，完整阶段表见主报告。

| 阶段 | 单次观测值（秒） |
| --- | ---: |
| 任务 Sandbox 创建 | 0.579852 |
| 工作区准备 | 1.300502 |
| Harness 启动 | 23.202644 |
| Runtime 执行 | 13.733015 |
| 停止与撤权 | 0.166327 |
| 独立验证 | 3.090271 |
| 清理 | 0.716504 |

内部活动观测，不能作为额外阶段累加：

| 观测项 | 数值（毫秒） |
| --- | ---: |
| 第 1 次原生工具耗时（exec） | 39 |
| 第 2 次原生工具耗时（exec，exit 1） | 407 |
| 第 3 次原生工具耗时（exec） | 308 |
| recorded 目标到本地 flush 最大延后 | 1.127180 |

这些计时沿用既有边界；原生工具耗时是 guest 内墙钟时间，模型 flush 延后是代理本地观测，不能跨域相减或与阶段简单相加。
35 个输入文件在离线分析前后 hash 不变；
28 条 Runner 事件完整，Trace 756 个事件包含 12 个 Runner、4 个模型、3 个工具区间，
无诊断错误，跨时钟域仍为 UNALIGNED。帧长度、数量及相对时序与 Tape 核对通过，
没有独立抓取发送线上的原始字节。512 次 Adapter 操作中有 77 次非成功结果，
分别为 exec.wait 43、stdin 31、events 3，包含取消；具体原因未逐项归因，不能隐藏或当作
77 个失败任务。原生工具结果正文通过后续完整模型请求的严格匹配核验，未额外独立归档全部正文。

| 文件（ECS 前缀 `/opt/aries-artifacts/agentsphere-embedded-replay-1/`） | SHA-256 |
| --- | --- |
| `evidence.json` | `70b2523cd27a5cb304779c3a707b1553bc354dd574c9f70d3bdcb04a61d2cf29` |
| `analysis-1/review.json` | `70589019704c6434bab82187fbb1c5f66be7f15ca6511dd763536c58f8cb32cf` |
| `analysis-1/evidence.json` | `fef5ecfc0c65bccd8b021ff684d20659eaeeef222fda474ea73d1189f6ecb4f0` |
| `analysis-1/summary.json` | `4fc77cb374d0d8efbf69e987bf81de7c99f90e886d37ef6eafd477de07606b34` |
| `analysis-1/trace.json` | `fe7fa67be4b478e8f1a734fdd3879bdaf06cb60a0b429d0e9e3bcaeb49cd56d9` |

### 保留的遗留与后续验证

Huawei 录制和回放均在显式假设 Verifier 出网隔离的条件下通过，不能宣称真实禁网通过。
未配置 SLO，因此不计算 Goodput。私网先构建、再收紧出方向、新建评分 Sandbox 的方案
尚未实跑；运行时的 DNS/SWR/OBS 依赖仍须实测。ENVD 版本差异按用户要求保留为遗留。

## 清单来源与复核方法

ECS `/opt/aries-artifacts/performance-archive-1/` 保存 2026-10-23 的基础快照只读提取结果。提取状态
`PASS_READONLY_PERFORMANCE_ARCHIVE`，读取 135 份 allowlist 文件，提取 10,426 个数值字段，
没有修改输入；未读取私有凭据文件或模型载荷。仓库本页展示可读摘要，完整字段与单位保存在以下清单：

| 文件 | SHA-256 |
| --- | --- |
| `inventory.json` | `631b066c13c1fd4cfb670f554eadc76dcac84fa1873771d5aa3532193c29f001` |
| `metrics.csv` | `e9969e8147536035b9e2425ef68847ebe1284bbed0685d104c8ab0467eee7c25` |
| `candidate.md`（初始材料，保留原样） | `c2dd63943751008c021281d57672c67fd6c2d2761037cb47872f7371d997a4f0` |
| `evidence.json` | `eb322c671ebf214a0059c4f74b3352f10ecbc62bdea3494a9f57c0436ff9844e` |

下方索引完整内嵌这 135 项的文件路径、快照状态及 SHA-256，因此不依赖工作站 `.cache` 才能查阅。
路径前缀 `artifacts/` 指 ECS `/opt/aries-artifacts/`；`runs/` 指 ECS `/opt/aries-agent-bench/runs/`。
例如表中 `artifacts/agentsphere-runtime-release-1/evidence.json` 的实际绝对路径是
`/opt/aries-artifacts/agentsphere-runtime-release-1/evidence.json`，不是仓库内的 `artifacts/` 目录。
这些是 ECS 上的归档文件，不是工作站同名路径或公共下载链接。需要复核时在 ECS 按路径读取，
核对 SHA-256，再查看对应 run、独立 review 及清理证据；不要仅由目录名或 job 退出码推断成功。
源状态 `UNSPECIFIED` 表示原 JSON 没有顶层 status；`READ_COMPLETE`、`complete`、
`COMPLETED_REQUIRES_EVIDENCE_REVIEW` 均按原样保存，不统一改成 PASS。
对快照后有终态变化的作业，以单列增量证据说明；原快照 hash 仍代表当时读取内容。

## 完整证据索引（基础快照 135 文件）

### 运行结果与实验复核（34 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E001 | `runs/20261022T172300.393697352Z-openclaw-e2b-external-record/run-result.json` | `UNSPECIFIED` | `0d8a00c9d02dceab586b7bc67b847f7b437879496971d01c20a65e9f78fb68a3` |
| E002 | `runs/20261022T185352.745645061Z-openclaw-e2b-external-record/run-result.json` | `UNSPECIFIED` | `7652ad29d512ad8c304ee0a370e36607249f760eff944a5c668ddc621be0303c` |
| E003 | `runs/20261023T015907.662466731Z-openclaw-e2b-external-replay/run-result.json` | `UNSPECIFIED` | `36ccd8f9c8dc2d9db2aed5ec5609a5085b0bf5d5a592f40002aa8f1f4d1f5603` |
| E004 | `runs/20261023T022605.576605763Z-openclaw-e2b-embedded-record/run-result.json` | `UNSPECIFIED` | `677099498155dc2e85250bafaf844eef332e2dd866aa474afeda690da2f679bf` |
| E005 | `runs/20261023T023947.417753785Z-openclaw-e2b-embedded-replay/run-result.json` | `UNSPECIFIED` | `9570102c3577a4810170110a935d4ffc25a4e2b5dc2244169aed59567aef3063` |
| E006 | `runs/20261023T024511.324592751Z-openclaw-e2b-embedded-replay-zero/run-result.json` | `UNSPECIFIED` | `0b096b089c73d5c2598c02b55ce0097cda0d70d0411f57d519c62d174a90f7d4` |
| E007 | `runs/batches/a-c1-2-2/20261023T055659.807155663Z-openclaw-e2b-external-replay/run-result.json` | `UNSPECIFIED` | `e7b74ba714536e5b24b08cae93ed8212e4709fc9842bb65f7ed2680a5a7255d5` |
| E008 | `runs/batches/a-c1-2/20261023T035909.067350683Z-openclaw-e2b-external-replay/run-result.json` | `UNSPECIFIED` | `ff3d02fa5c57cf9c2937b5b50b4c2071c0ee25f0cf2f3cad611acc2d074f39f6` |
| E009 | `runs/batches/b-c1-2-2/20261023T060024.243252307Z-openclaw-e2b-embedded-replay/run-result.json` | `UNSPECIFIED` | `f5fb20f6dee8d11bfe1413618d6ab9bf7d8df5eac3cb86133bc465ffb1555b6e` |
| E010 | `runs/batches/b-c2-3-2/20261023T060555.509149280Z-openclaw-e2b-embedded-replay/run-result.json` | `UNSPECIFIED` | `b712a64b9e44a42647a41c70449993ffb95c2d867727c4b7b4e6f5d4a167b959` |
| E026 | `artifacts/agentsphere-embedded-record-1/evidence.json` | `FAIL` | `aaa60872c37ecb25fcddb0c2b34498f9d0fe1bb6638391330291dae83fe146a4` |
| E027 | `artifacts/agentsphere-embedded-record-1/runs/20261023T094827.134992253Z-openclaw-agentsphere-embedded-record/run-result.json` | `UNSPECIFIED` | `184dbf233623be624075a171cebcff431d522d377ddf578cdc8fd2ee1a1b556c` |
| E028 | `artifacts/agentsphere-embedded-record-2/evidence.json` | `FAIL` | `47cf77e3e6fe7fa72d7aa1a509447a9f7fbbae8d14b5b6e8be64ceccb839670b` |
| E029 | `artifacts/agentsphere-embedded-record-2/runs/20261023T101904.133578168Z-openclaw-agentsphere-embedded-record/run-result.json` | `UNSPECIFIED` | `060b4c3e72c7ea83a6809d94dd08de9560283015b2e698efe9a5020f0cb92b1d` |
| E063 | `artifacts/e2b-formal-embedded-record-1/evidence.json` | `COMPLETED_REQUIRES_EVIDENCE_REVIEW` | `75a21f604dc91aee339975580e472ae698d92d69f7d29e8995ccdbdc57c8d25a` |
| E064 | `artifacts/e2b-formal-embedded-record-1/performance-analysis.json` | `complete` | `837ae29fd01c23ad51e4e0ad1f28f15c2ec4f1bcd789d9ab39f498300ac935f1` |
| E065 | `artifacts/e2b-formal-embedded-replay-1/evidence.json` | `COMPLETED_REQUIRES_EVIDENCE_REVIEW` | `500f40f4fd147fb038497f31dcdf9ee60be35c6f8450ad432b808cda2a36d233` |
| E066 | `artifacts/e2b-formal-embedded-replay-1/performance-analysis.json` | `complete` | `2ba5ebd8f92b0629591aace9f71c8ec10a0ebae42ef11929ba65cb948a25420f` |
| E067 | `artifacts/e2b-formal-embedded-replay-zero-1/evidence.json` | `COMPLETED_REQUIRES_EVIDENCE_REVIEW` | `80078a0254364ea1b8f5902fc92585e5a8cc76af0d3efe591ae6fbe7feb920e3` |
| E068 | `artifacts/e2b-formal-embedded-replay-zero-1/performance-analysis.json` | `complete` | `168bd1868551fed4ff89b9937ceac5fb416ebe1210e79c8b58eec7f180f5d1ce` |
| E069 | `artifacts/e2b-formal-external-record-1/evidence.json` | `FAIL` | `9baa1e9ea1c42eefbdb2388ecdcebdc219311c6f70fe5384458c307800c74d2e` |
| E070 | `artifacts/e2b-formal-external-record-2/evidence.json` | `FAIL` | `e72282b730e7872bb81f5547f7aee7a140c9f8d215fd5a4c4a523cb96e8c209a` |
| E071 | `artifacts/e2b-formal-external-record-3/evidence.json` | `COMPLETED_REQUIRES_EVIDENCE_REVIEW` | `0bcf4261145a93cf4533a197a6790220b9d4b8f36468c74dfee2b3cd63b53f4c` |
| E072 | `artifacts/e2b-formal-external-replay-1/evidence.json` | `COMPLETED_REQUIRES_EVIDENCE_REVIEW` | `01e6bf0cba1afd25ba7837acc23b64fd337af483e2a5987e75a4582f34664c42` |
| E073 | `artifacts/e2b-formal-external-replay-1/performance-analysis.json` | `complete` | `93cb66f3d0c93c734d761f64b082799247a3183c0ac69300932f12c12bc97703` |
| E077 | `artifacts/e2b-runtime-batch-a-c1-2-1/evidence.json` | `FAIL` | `964049d66000b72c6923463107d1a7ca358fdfbb218cdf14c46bf770895281cb` |
| E078 | `artifacts/e2b-runtime-batch-a-c1-2-2/evidence.json` | `PASS_BATCH_RECORDED` | `02ee5140d4fab1336d92d86d2faea0922ca6ff2e2980c31594db261fea9f7b51` |
| E079 | `artifacts/e2b-runtime-batch-a-c1-2-2/performance-analysis.json` | `complete` | `49c1ea9018556290d3012becdeabc6cccc2111ee083ebdd46ebff77d12ee4771` |
| E080 | `artifacts/e2b-runtime-batch-b-c1-2-2/evidence.json` | `PASS_BATCH_RECORDED` | `44820884f4c2a8df8a29c1d8aecf62f353a81efb6b932792b3104317237f89a4` |
| E081 | `artifacts/e2b-runtime-batch-b-c1-2-2/performance-analysis.json` | `complete` | `3627b25d01aa3f3db0e88d6af4c73045b11bddaddbff92443b8ccfd43162ed49` |
| E082 | `artifacts/e2b-runtime-batch-b-c2-3-2/evidence.json` | `PASS_BATCH_RECORDED` | `2c8ccb5a6939d5c67c84fe923807186c29af1cfe81ba712fdc7f38fbdbbb86fe` |
| E083 | `artifacts/e2b-runtime-batch-b-c2-3-2/performance-analysis.json` | `complete` | `4c3a55478804cc6adcdf534c7738f6c2dfc31e478bbd793e9242ab794e5625f9` |
| E094 | `artifacts/e2b-w2-faults-1/cases/b-stop-unavailable/run-result.json` | `UNSPECIFIED` | `a71451376e5d196dbff15310c5f612e063537ceb82b35b1ba3f8acb47768a0de` |
| E095 | `artifacts/e2b-w2-faults-1/cases/b-timeout/run-result.json` | `UNSPECIFIED` | `0a1d8555c8785a87bcf481aac3911f77a7ddeabe611cb310ae22f0a9656a8bf3` |

### 生命周期与网络检查（17 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E012 | `artifacts/adapter-delete-concurrency-check-1/evidence.json` | `PASS` | `82912df27b505b89a67268638fe3c027f82ecb3e05ae5cbb206465126eab305f` |
| E030 | `artifacts/agentsphere-explicit-network-1/evidence.json` | `FAIL` | `409cd8512d77cf01465221539c17afbe48ae80173a4b212c5b9aa8b4c59ef5d2` |
| E031 | `artifacts/agentsphere-explicit-network-1/reconciliation-1/evidence.json` | `PASS_NO_OWNED_RUNTIME_OBSERVED_TEMPLATE_REMOVED` | `b9f53af27117268fe304c84ba7a8fcfec9ae1cf3787bd79fea267c7b8dd13bf5` |
| E040 | `artifacts/agentsphere-network-inventory-1/evidence.json` | `PASS_READONLY_INVENTORY` | `62a7819ccf5b917398e8c5093831ead5e490accf9bb2e7a94d85d1d5b17fe360` |
| E041 | `artifacts/agentsphere-network-metrics-1/evidence.json` | `FAIL` | `1a0b6b079e6af6bea3626d5f692ebf158faa3bbd5039f6c80f04b8be5d65af01` |
| E051 | `artifacts/agentsphere-vpc-egress-1/evidence.json` | `FAIL` | `baa8a031dd88caacf2aa50733dc11345d0fb0bf44b6404701849395b97eddc69` |
| E052 | `artifacts/agentsphere-vpc-egress-1/reconciliation-1/evidence.json` | `PASS_NO_OWNED_RUNTIME_OBSERVED_TEMPLATE_RETAINED` | `d94f5600a5a30f2efa21a987009e2ef8e6bb9d607de05203b3b2d12f10c8917e` |
| E053 | `artifacts/agentsphere-vpc-egress-2/evidence.json` | `FAIL` | `1628bddd90fdc1db64c566ca428b4ec72a79d31975d4649ef2fcaea00cf375f9` |
| E054 | `artifacts/agentsphere-vpc-egress-3/evidence.json` | `FAIL` | `35739779ef81b8ecfe2d1b8479c8a9c1069545cc3f6dc29d851179f67d0fb413` |
| E061 | `artifacts/e2b-delete-concurrency-1/evidence.json` | `PASS` | `1c356b31cade8412488befc4c136fde5289ee5ab055b1c0a4eddd2f22de85fbe` |
| E096 | `artifacts/e2b-w2-faults-1/evidence.json` | `PASS_COMPONENT_FAULT_PROBES` | `405c1e1e1ebefb79a9bdbea2bee44e19ebd4ffc083f6eb78dafed08353c2ba63` |
| E113 | `artifacts/huawei-network-readonly-1/evidence.json` | `PASS_READONLY_INVENTORY` | `b35970f522f516c62d4c9a931735eb65ee6a1d0edcf148e1c962db291832814d` |
| E114 | `artifacts/huawei-network-sdk-1/evidence.json` | `PASS_PINNED_SDK_PREPARATION` | `1ce3187a117bf8b7356ab9bb65c0dff5e0f5334f4bbd51a5f598b6f20de1286b` |
| E115 | `artifacts/huawei-verifier-network-1/evidence.json` | `FAIL` | `afceacd66bdb195a6372a9ceb114cf6a61870264d317f38c05908dea34f3b378` |
| E116 | `artifacts/huawei-verifier-network-resume-1/evidence.json` | `PASS_NETWORK_RESOURCES_PREPARED_NOT_BOUND` | `86b5fe0ca353f3e39be0ded7fa9f5b3a4eaa075fae2c43cd8e8bff3b8667cbe9` |
| E117 | `artifacts/huawei-vpcep-readback-1/evidence.json` | `PENDING_USER_PURCHASE` | `48a4fe74c5d792c4a7d9a0b16b8a2e57f8145be7ebe010760aa433f7ad956a7e` |
| E118 | `artifacts/huawei-vpcep-sdk-1/evidence.json` | `PASS_PINNED_VPCEP_SDK_PREPARATION` | `335d037013edac1a54e4683b2e7f8047b9b9dae09cf0d2a6ea55e8cf3e3de176` |

### 协议与功能检查（43 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E017 | `artifacts/agentsphere-adapter-live-1/evidence.json` | `FAIL` | `e43b74280c34565cc613dd415367b9ff44e8c40cfbe1bf008f4da120485a526b` |
| E018 | `artifacts/agentsphere-adapter-live-2/evidence.json` | `FAIL` | `8b21128a779ae6090b3a3cdaca0ed61a4abcbf6a0f88132598ad012b208b8ad0` |
| E019 | `artifacts/agentsphere-adapter-live-3/evidence.json` | `FAIL` | `e46e696c2f6f8eae2938116377d8d4f876eaffc229916663e0d7702cbee9163c` |
| E020 | `artifacts/agentsphere-adapter-live-4/evidence.json` | `PASS_ADAPTER_PROTOCOL_SMOKE` | `c548840be8e7158cb9f49891020fcafa4560940be6e09c948a751587ec4d9627` |
| E021 | `artifacts/agentsphere-compatibility-1/evidence.json` | `FAIL` | `b9a39b11d9a1fb0ec64b468103531cc847804558306ae7b221a9f994eb9831a1` |
| E022 | `artifacts/agentsphere-compatibility-2/evidence.json` | `FAIL` | `ffb6fe4d29755671e19dbfc26a10483d314c490ba811ed66b70f9a343e0f96ab` |
| E023 | `artifacts/agentsphere-compatibility-3/evidence.json` | `FAIL` | `aa0ec4352edaa474988b37307875e98e605e6b6e526cc8147e4b0967c89a8fa4` |
| E024 | `artifacts/agentsphere-compatibility-4/evidence.json` | `FAIL` | `4c69f6fa1e41410a2a1f2038bacc8bc52f1a9debda41f397afb4a8a987749c55` |
| E036 | `artifacts/agentsphere-live-source-1/evidence.json` | `PASS_SOURCE_ARCHIVE` | `acf8d884c542f77d3a4557ebfe816d8bff5dc8b19614167247cbe9e60bd7f774` |
| E037 | `artifacts/agentsphere-native-discovery-preflight-1/evidence.json` | `FAIL` | `4f0ac09901704f852a982d656172be7185c1e088f890f3da7a981e96f2bfb8ea` |
| E038 | `artifacts/agentsphere-native-discovery-preflight-2/evidence.json` | `PASS_NATIVE_DISCOVERY_PREFLIGHT` | `10cba4d3b74046383bf14c1c804c054d4675fd95877fadb7cfdbd2b9cff8f379` |
| E039 | `artifacts/agentsphere-native-overlay-1/evidence.json` | `PASS_NATIVE_GUEST_OVERLAY_DIAGNOSIS` | `94facad2cdc2ffc8536ea02b21cc8c38a8a75a3450186b10945ab54b032d2216` |
| E043 | `artifacts/agentsphere-overlay-probe-1/evidence.json` | `PASS_NATIVE_OVERLAY_REPRODUCTION` | `aa2921e96afa23768b68bac34cd57d31e24f23cb02bab11a2cbec424e5b5b052` |
| E044 | `artifacts/agentsphere-plugin-inspect-1/evidence.json` | `PASS_READONLY_INSPECTION` | `d07813b1a046107970e45fcccdfd6e0cd57cd1ca2e1353752d25ad36f2379f10` |
| E048 | `artifacts/agentsphere-readonly-1/evidence.json` | `PASS` | `2778904cecf9968c081d3898489ce88fa3296fd4ee8ee2ef63c60438c0185c57` |
| E055 | `artifacts/aries-e2b-adapter-refresh-1/evidence.json` | `PASS` | `b604f6df5efe7d88b650b78ad2107d8ae2c781403cd1b0a0f9149c20bec5a7fc` |
| E057 | `artifacts/bridge-evidence-adoption-1/evidence.json` | `PASS_RUNNER_ADOPTION` | `2cb5fdd0d8059d9e8f7b9bb8f0ee7598a9da29246a36fd4b22009add91dc9c92` |
| E076 | `artifacts/e2b-remove-create-cap-1/evidence.json` | `PASS` | `678b4d79a02ad733b14db4e44fd72e7eb23ed36005fa7ecef0abe45d0ddd9a17` |
| E085 | `artifacts/e2b-template-cloud-20261022-v1/allocation-reconciliation-1/evidence.json` | `PASS` | `dfc0f0f197cdc967df480f56d0e35eb5e5fd864aad0a8bfbdecb429ad5e2e95b` |
| E086 | `artifacts/e2b-template-cloud-20261022-v1/explicit-build-verification-2/job-evidence.json` | `FAIL` | `4168cf323146e1eca10764e384160118c9e07aa39118d531f6130298e480c67d` |
| E087 | `artifacts/e2b-template-cloud-20261022-v1/job-evidence.json` | `FAIL` | `db05cb804d8ce27ffa1663a04030b9f2f519b2100fb449ef0fe40095516d93c2` |
| E088 | `artifacts/e2b-template-cloud-20261023-v2/job-evidence.json` | `PASS` | `878d641ba9cc7b00aca9d5dcdba5a0d0ad317f3187e247b7626fffd55c705617` |
| E089 | `artifacts/e2b-template-filesystem-preflight-2026.7.1-fixed1/evidence.json` | `PASS` | `843f7eeb0de0f1f66013e04a849dfa74d148df521682489b32d0a858e3786cbc` |
| E090 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/diagnostic-1/evidence.json` | `FAIL` | `7e095cac0a0df16dbe64bb2fe7ee2c5761e024f053f4a642b4f92d832f6dc4cf` |
| E091 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/diagnostic-2/evidence.json` | `FAIL` | `b2e722de65b266580314e216641927548a9905b16f584df6d7b353955f5c36a5` |
| E092 | `artifacts/e2b-template-filesystem-preflight-2026.7.1/evidence.json` | `FAIL` | `4cd45d88c55c9722e9a896d305c8c6219c7696c327473106fd805e7aae442be9` |
| E093 | `artifacts/e2b-template-finalize-preflight-1/evidence.json` | `PASS` | `ed923b12bb5fc05e8c6185a3494c736b5df55f9b2e7e98c31dbc84d2b6bfaae0` |
| E099 | `artifacts/formal-a3-evidence-review-1/evidence.json` | `PASS` | `7cf27e8307e65fa2b202b10e4a542c47d33aec04a4b51bd042abf862d79bcbc1` |
| E100 | `artifacts/formal-a3-evidence-review-1/formal-summary-current.json` | `RECORDED_REQUIRES_REVIEW` | `fda888835519bd076f5df7658a7af6dcd70935c143547fb58ee1d8982bd52f73` |
| E108 | `artifacts/gateway-device-auth-probe-1/evidence.json` | `FAIL` | `e0f2a6ad415349ca20f2d7161eb9ebf6f556434794d924c6f450a739eb803eed` |
| E109 | `artifacts/gateway-device-auth-probe-2/evidence.json` | `PASS_NATIVE_GATEWAY_DEVICE_AUTH` | `e051af2a78c69768003d3d2fe7d1830bd7ae1051718b2d04ba223dbd6c958ab4` |
| E110 | `artifacts/gateway-go-device-auth-probe-1/evidence.json` | `FAIL` | `58d4c1ff475c0b0c93f81f4976ae0a4ccf11a9802a2196a75f2c04a7d44cd719` |
| E111 | `artifacts/gateway-go-device-auth-probe-1/retry-1/evidence.json` | `PASS_GO_NATIVE_GATEWAY_DEVICE_AUTH` | `1699a2d72aa461cf67ff3ebcb251d6ed57529f6aba4127eae8c1b6f4528290af` |
| E119 | `artifacts/native-dialect-roundtrip-1/evidence.json` | `FAIL` | `34d7890fe2468d291b6c23a7a8b3c7eaef9eb49043a53b8c3e3012025ff3f953` |
| E120 | `artifacts/native-dialect-roundtrip-2/evidence.json` | `PASS` | `a3263138a5c62dfc273f57a614bc7af2b95204b106cfdcbec847d2f16c6f4840` |
| E121 | `artifacts/native-id-dialect-validation-1/evidence.json` | `FAIL` | `4ac6372eb8db11841f4c555d784efdf7130f7288b2781ba48108c257841cc9fa` |
| E122 | `artifacts/native-id-dialect-validation-2/evidence.json` | `PASS` | `b93f27df8d4aea4863f1aeff24aa24686d43148389a381757fdd81124bf4d758` |
| E124 | `artifacts/native-protocol-diagnostic-1/evidence.json` | `PASS` | `20116aabeac056fa66a5afeaa786a7ee67d638060f8c5e2ea1ecff14edd37953` |
| E125 | `artifacts/openclaw-2026.7.1-linux-amd64/apt-https-probe/evidence.json` | `FAIL` | `d63000514d7e503894a9bc41880db2f52b768f96951afad151fba7572ae7aca8` |
| E126 | `artifacts/openclaw-2026.7.1-linux-amd64/dangling-link-probe/evidence.json` | `PASS` | `adef1494446a05ba3f914ab48997b9c158fbe7da26d13dc9012c25a891e79c0e` |
| E127 | `artifacts/openclaw-2026.7.1-linux-amd64/native-probe/evidence.json` | `PASS` | `25f32bd692c9eee9560fec4cf3b0a9bbc26a209b209fed14ec3ba450548a3590` |
| E128 | `artifacts/openclaw-native-config-validation-1/evidence.json` | `PASS` | `e039e26c130189e3beb357bce8f9e48b4651a698110faf2d270753383ef9b49b` |
| E132 | `artifacts/ssh-diagnostic-20261023-1/evidence.json` | `PASS_SSH_ACCESS_VIA_EXISTING_SOCKS` | `552f7650c6d77237d34654ab7883a962c1ff26d08f291f20b7572c180ae439b8` |

### 单测、SDK 与发布检查（17 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E013 | `artifacts/agentsphere-adapter-candidate-1/evidence.json` | `FAIL` | `d7e2a9ef16c0f4f86592aaa14a8ecdac40c00d076b8f4297e98e2803b77d7dce` |
| E014 | `artifacts/agentsphere-adapter-candidate-2/evidence.json` | `FAIL` | `09e86cdaa02a56a005160ef796dffb6feeab16ce16b4c8a1de216a18c3b1e400` |
| E015 | `artifacts/agentsphere-adapter-candidate-3/evidence.json` | `PASS_OFFLINE_CANDIDATE_IMAGES` | `6e822efd401de49fe493575f2bd628989ba30de71872044ce7194baf450f1392` |
| E016 | `artifacts/agentsphere-adapter-candidate-4/evidence.json` | `PASS_OFFLINE_CANDIDATE_IMAGES` | `fe10488f4dddd665dcf5500216b5f2bf19703129f6d47372b63e068ac312f970` |
| E025 | `artifacts/agentsphere-device-assumption-check-1/evidence.json` | `PASS_TARGETED_CHECKS` | `a4b8c4dac5ae55c18fdac4156f6ac56e59e93ad732c144702efd88a8552d9280` |
| E032 | `artifacts/agentsphere-go-check-1/evidence.json` | `PASS_TARGETED_GO_CHECKS` | `cbbb6cd7b3056530d1fd18b7b80939ee47a779b527879013e2cfa251b50122d6` |
| E049 | `artifacts/agentsphere-runner-candidate-5/evidence.json` | `PASS_RUNNER_RELEASE_CANDIDATE` | `0a539bd6cbe3d503af596b99a522e650cca063e4abefefc681e5cc6c77342d4d` |
| E050 | `artifacts/agentsphere-runtime-release-1/evidence.json` | `FAIL` | `1ce45fd0ca93d2ee76f2c70c436673560194fe765348e4c0d6a846319b6e7b53` |
| E058 | `artifacts/bridge-evidence-regression-1/evidence.json` | `PASS` | `f4ee8edae555bc10185846f5f9273dd734574a54733224c12f7145a115569dde` |
| E059 | `artifacts/bridge-evidence-release-1/evidence.json` | `PASS` | `7b9d56505ab8f299def8f580899d8b346f49f1921bec7dfe785684b4edaa9499` |
| E084 | `artifacts/e2b-sdk-smoke-20261022-1/job-evidence.json` | `PASS` | `bae90c5b11b06f0b357ad4b4a2c1b1d88a8db30a15a60e9490c1311db937f16e` |
| E101 | `artifacts/formal-a3-model-protocol-check-1/evidence.json` | `PASS` | `30b1a9b08e3943381120e70d58249ce83702332ab521cc483262bd56a058b9b3` |
| E102 | `artifacts/formal-a3-model-protocol-check-1/formal-summary.json` | `RECORDED_REQUIRES_REVIEW` | `fda888835519bd076f5df7658a7af6dcd70935c143547fb58ee1d8982bd52f73` |
| E103 | `artifacts/formal-a3-model-protocol-check-1/results/summary.json` | `PASS` | `16e858fdc7e6131b4838235804ffcdf3ed4043ce350f3146cc636707d664da5a` |
| E106 | `artifacts/formal-b1-model-protocol-check-1/evidence.json` | `PASS` | `547721ee73e9a6e0e85779c1d06488f932c3f21375149047db3d8c0b3b0d74b6` |
| E107 | `artifacts/formal-b1-model-protocol-check-1/results/summary.json` | `PASS` | `b42472a2f5f229071103a0a71968c4e930a4270ec415ef45b75466dbc0635ecc` |
| E123 | `artifacts/native-input-release-1/evidence.json` | `PASS` | `23b0a73485d1956006d1859101fc80bd858ee79d085dae44d20fe98eb319a451` |

### 离线性能分析（7 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E097 | `artifacts/five-baseline-boundary-calibration-1/analysis.json` | `READ_COMPLETE` | `412e0d23daf4cb3f43ff6321f53e11ebd8a358a8b31fcbf4c6a9c733059e12fc` |
| E098 | `artifacts/formal-a3-causal-audit-1/evidence.json` | `PASS` | `4dd7345dc80cbdad2233e7cff7b85c6057eda36002776f154ebb5c97f0ffd7ac` |
| E104 | `artifacts/formal-a3-performance-analysis-1/analysis.json` | `READ_COMPLETE` | `5c0c9cca645f579a5ab63e3d016c4b5089123ebe9c005cd860a7bc6ee67662a4` |
| E105 | `artifacts/formal-a3-performance-analysis-1/aries-perf.json` | `complete` | `315a25a5386220b7ed55742c551a6973979fa4de4f90d89d2c2f4755e75f8f18` |
| E129 | `artifacts/replay-cost-calibration-1/evidence.json` | `MEASURED_LOCAL_REPLAY_PROCESS_ONLY` | `57dd809bbef719f3e812251db310f3d6b1253daf4d787fcbb2fd2b75b0ff86e4` |
| E130 | `artifacts/replay-cost-calibration-1/release-evidence.json` | `PASS` | `7b9d56505ab8f299def8f580899d8b346f49f1921bec7dfe785684b4edaa9499` |
| E131 | `artifacts/runtime-performance-summary-1/analysis.json` | `READ_COMPLETE` | `999dcec423948c1d4f18524973362a20dc44e5e41e5d9c01347bb63dbdcad282` |

### 镜像、部署、文档等配套证据（17 份文件）

| 索引 | ECS 文件路径 | 源状态（快照） | SHA-256 |
| --- | --- | --- | --- |
| E011 | `artifacts/a3-docs-update-1/evidence.json` | `PASS` | `addfcbbd92743dd185c5bdfea025564fb396b13fbb789ae6865827e8086d6ae0` |
| E033 | `artifacts/agentsphere-image-1/evidence.json` | `FAIL` | `29bfd96a29eab847a24ae24197cbce61d689f8da17da5fc26e2d7514f9bb7695` |
| E034 | `artifacts/agentsphere-image-push-2/evidence.json` | `PASS` | `0910e1bdb5676596af2f9445f75702f7a9bed46eb8bf37b9cd080904f7ab3f02` |
| E035 | `artifacts/agentsphere-live-docs-1/evidence.json` | `PASS_DOC_UPDATE` | `13d37cc5ad723f35de2657183faf6834084dfe6715d9dc210d730c9bef899a7a` |
| E042 | `artifacts/agentsphere-offline-docs-1/evidence.json` | `PASS_DOC_UPDATE` | `046f08485eaff42bad0ab2de1e6fecd1bf6dfe039cca4d6c2c673eaf212d164a` |
| E045 | `artifacts/agentsphere-progress-docs-1/evidence.json` | `PASS_DOC_UPDATE` | `ac0f78d9a1e424915abf548522b19fe324452ba6654736a0e3676f9fe2c5d17a` |
| E046 | `artifacts/agentsphere-progress-docs-2/evidence.json` | `PASS_DOC_UPDATE` | `45fd54b4720dea8da27e0ecf7fa2c18a4f97f5e2e02ae7e744dd74542add13f4` |
| E047 | `artifacts/agentsphere-progress-docs-3/evidence.json` | `PASS_DOC_UPDATE` | `0c4170099090398757aa2cec34bb08536ebf302f7139b0ed32f412b6ffd4cf87` |
| E056 | `artifacts/baseline-matrix-docs-1/evidence.json` | `FAIL` | `6d6734bbe95d11df3fbeaf152ea7a061977443ce2ce2384b1ccc8d528b32926a` |
| E060 | `artifacts/bridge-evidence-runtime-activation-1/evidence.json` | `PASS_RUNTIME_ACTIVATION` | `84c26dddb1ad126097c27d2a06c01e2d75f07265856cf37470c7062053105dac` |
| E062 | `artifacts/e2b-delivery-docs-1/evidence.json` | `PASS_DOC_UPDATE` | `b9d624a4881743f7259f2c51a0bbee41c8693adbc2733abeaad00698fe1939c1` |
| E074 | `artifacts/e2b-platform-deployment-1/attempt-9bcfd87826a4/evidence.json` | `PASS` | `f15b317c91190c9081708078750b59eabfa13a45dba0f174097197be35b4e4d4` |
| E075 | `artifacts/e2b-platform-deployment-1/evidence.json` | `PASS` | `f15b317c91190c9081708078750b59eabfa13a45dba0f174097197be35b4e4d4` |
| E112 | `artifacts/huawei-config-preparation-1/evidence.json` | `PASS` | `f8583fd130dbc192ee0cabea71ef058ed7519444d1c0e4015d841bba8078c51c` |
| E133 | `artifacts/swr-config-preparation-1/evidence.json` | `PASS` | `6bff1343d2c111999f7fd2c5456758cc3b2d38c30456ef924c34801a00100b83` |
| E134 | `artifacts/swr-config-preparation-2/evidence.json` | `PASS` | `f2c6ccb31dc6dccbaa20e7efc8b208643c0597560aa9e12c4e661126cb56e55b` |
| E135 | `artifacts/swr-login-1/evidence.json` | `PASS` | `638b5b9fc555a91b7b23fae5d1329bf5fc9b162bfa25e7186124d8fc7f00270a` |
