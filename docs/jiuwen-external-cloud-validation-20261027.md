# Jiuwen Flash A：两云真实验收与观测（2026-10-27）

**E2B、华为云 A 模式均完成真实DeepSeek解题：原独立Verifier各12/12、score=1，各录得5轮完整Tape；停止撤权、两个Sandbox删除与外置Agent容器/网络回收均确认。** 这是两次独立真实轨迹的功能验收，不是Strict回放或同Tape性能排名。华为评分Sandbox的禁止出网仍是显式假设，不能计作完整隔离验证的Goodput。历史六例Strict 3/6不变。

## 测试对象与实际接入

任务为自建 `merge-ranges-portable-v1`：Agent阅读区间合并代码与公开测试、执行测试定位缺陷、修改代码并复测；覆盖真实文件读取/修改、Shell执行、模型多轮交互及独立候选验证，不作为CPU或磁盘极限压测。沿用原任务和12项私有Verifier，没有提供参考解、改工具正文或降低评分。

A的实际路径为：**ECS Docker中的原生AgentServer/Flash → 官方SysOperation SANDBOX → `aries` FS/Shell provider → RemoteBridge → Runner创建的云任务Sandbox**。Jiuwen原生已允许注册自定义Provider；我们适配其官方入口，没有重写Agent循环或原生六工具。准确源码与本地辅助I/O边界见[源码审查](design/jiuwenswarm-flash-review-20261026.md#为什么当前只有-b)。原生 `sandbox.startup_mode=external` 仅关闭内置JiuwenBox自动启动，与ARIES的A placement不是同一设置。

两云均为2 CPU / 4096 MiB、image4固定digest `sha256:f0a82b0e258a0e763defa922c7a01410a40698b92967d03d7279e7976fdf6489`；模板见[锁文件](../deploy/jiuwenswarm/templates.lock.json)。Agent位于同一ECS，任务预算5m、Verifier 1m、Sandbox存活期900s，单并发各一次。磁盘及平台实现差异保留为环境说明，不作为拒绝比较的理由；本次未作性能排名的原因是两份模型轨迹并非同一Tape，且独立重复不足。

使用冻结release19的827文件源码组合，build / test / test-race / lint / integration五项ECS工程检查全部通过；原生受控Docker夹具独立通过。门禁与受控夹具不替代本页真实云验收，冻结SHA及前置证据见[release19实施记录](implementation/jiuwenswarm-flash-progress-20261026.md#jiuwen-release19-a)。

| 项目 | E2B external-record1 | 华为 external-record1 |
| --- | --- | --- |
| Harness / 原Verifier | succeeded；12/12，score 1 | succeeded；12/12，score 1 |
| 真实录制 | 5轮、complete | 5轮、complete |
| Bridge真实调用 | 4次唯一invocation；worker 4 start / 4 returned；HTTP均200 | 4次唯一invocation；worker 4 start / 4 returned；HTTP均200 |
| 原生ToolCall → Bridge身份 | 4/4 unavailable，不能按数量相同猜关联 | 4/4 unavailable，不能按数量相同猜关联 |
| 评分前隔离 | Harness stopped、Bridge revoked确认 | Harness stopped、Bridge revoked确认；Verifier出网为assumed |
| 最终回收 | 2个Sandbox独立provider absence；Agent容器、run网络、模型监听及Runner进程均不存在；cleanup=[] | 同左 |

`invocation_returned`表示远端接口返回，可能只是后台启动回执，不能改名为进程退出。此处4次调用对应本次实际工作量，不等于所有工具类型、后台/取消分支均已实测。

## 八阶段与总时间的关系

单位为秒。八阶段名称沿用[统一阶段说明](performance-stages.md)，逐值读取本次 `run-result.json`；保留原字段，不把新增发现事后塞进旧字段。

| 原阶段 / 边界 | E2B | 华为 |
| --- | ---: | ---: |
| 排队（不计入Attempt） | 0.000014362 | 0.000014698 |
| 任务Sandbox创建 | 1.066651010 | 0.757076474 |
| 工作区准备 | 1.825085146 | 1.206430643 |
| Harness启动 | 39.997729868 | 36.113656102 |
| Runtime执行 | 25.924043097 | 22.253257436 |
| 停止与撤权 | 2.627992245 | 1.497605434 |
| 独立验证 | 6.647059736 | 2.363427498 |
| 清理 | 3.176197659 | 0.737936098 |
| **上述7个执行阶段之和（排除排队）** | **81.264758761** | **64.929389685** |
| **Attempt总时间（不含排队）** | **94.144342886** | **68.244441443** |
| Driver总窗口 | 99.863081000 | 72.563371000 |

**本次确认了R04真实缺项：`bridge.register`有原始Trace区间，但没有写入八阶段的TaskTiming字段。** A需要启动远端worker并建立连接，这段准备不能当作0或藏到Runtime。它在工作区准备之后、Harness启动之前；下表单列对账，不改变上表历史值。

| Attempt与7阶段之和的差额分解 | E2B | 华为 |
| --- | ---: | ---: |
| `bridge.register`原Trace区间 | 12.820719372 | 3.257819619 |
| Attempt减去`task.e2e`（外层窗口） | 0.053828389 | 0.052261552 |
| `agent.run`减去原生Runtime（Harness调用外层） | 0.004973922 | 0.004889983 |
| 剩余未分配边界时间 | 0.000062442 | 0.000080604 |
| **合计差额** | **12.879584125** | **3.315051758** |

因此本批满足：**Attempt = 7阶段之和 + Bridge注册 + 上述外层窗口 + 未分配边界时间**。排队另算；Driver还含实验启动、结果处理和删除确认，不与Attempt相加。Verifier内已含候选采集（两云分别0.485403321 / 0.174418080s）及评分准备/执行，不能再加一次。模型等待与工具调用属于Runtime内部并可能重叠，也不能在Runtime之外重复累加。R04尚需在正式汇总契约中表达Bridge准备，再以真实运行核对；本报告仅如实补充已有Trace，未伪造阶段。

## Trace、身份与资源

| 观测项目 | E2B | 华为 | 正确解释 |
| --- | --- | --- | --- |
| sampled SDK started / ended / exported | 27 / 27 / 27 | 25 / 25 / 25 | 独立ID及终态集合对账，missing / unexpected均0，仅此专项complete |
| 原生authoritative Tool Span | 4 | 4 | 原生Span内精确身份存在；与Bridge的ToolCall贯通仍未完成 |
| Snapshot完整性 / 全局覆盖 | complete / unknown | complete / unknown | 已声明数据完整，不证明所有实际行为均采集 |
| 四出口 | Snapshot、Perfetto、OTLP、Metrics均通过同快照投影 | 同左 | 本轮TP导入、Jaeger查询和Prometheus接收仍UNRUN，不借用旧样本后端结果 |
| 原生时钟 | 未对齐Unix纳秒 | 未对齐Unix纳秒 | guest进程单调计时及跨机对齐仍unsupported，不猜跨域因果或用网络差值当纯执行时间 |

原生SDK审计不证明未采样/未插桩行为或flush时长有界。Bridge有独立invocation ID，可以核对控制端与worker的真实收发；它不能代替原生ToolCall ID。个人版官方事件回调携带ToolCall ID的候选入口已经定位，尚待实际验证正常、并行及取消后的身份不串号，见[问题台账](observability/OPEN_ISSUES_AND_RESOLUTIONS.md#jiuwen-a-native-gaps)。

| 资源归属与来源 | E2B本轮证据 | 华为本轮证据 | 尚不能声称 |
| --- | --- | --- | --- |
| ECS控制器共享进程，rusage/MemStats | 起止2次CPU、RSS/堆等读数 | 同左 | proxy未单独拆分；生命周期RSS高水位不是Runtime峰值 |
| ECS Agent容器，monitor | 67条CPU累计量/内存读数 | 59条CPU累计量/内存读数 | 不是云Sandbox资源；原cpu_percent未获出口资格，不自行积分或推算逐Tool用量 |
| 整台任务Sandbox，provider | 177条标准化receipt，1条空；176条有值，158条重复时间和值 | 15条有值，时间语义unverified；明确MiB字段与未知单位原字段分开 | receipt数不是独立采样数；provider返回最大值不是Runtime真实峰值；未对齐时间不能精确裁切Runtime |
| 独立评分Sandbox，provider | 2条均空，未采集到CPU/内存 | 2条有值，时间语义unverified | 不借任务Sandbox数据填充，不把缺测写0 |
| 任务/逐Tool I/O、Network | 未采集到可归属数据 | 未采集到可归属数据 | `disk_used/total`是占用/容量，不是I/O字节；观察器写文件字节不是任务I/O |

## 原件与复现定位

ECS根目录分别为 `/opt/aries-artifacts/jiuwen-e2b-external-record-1`、`/opt/aries-artifacts/jiuwen-huawei-external-record-1`。根下保留 `evidence.json`、`acceptance-summary.json`、`raw-artifact-manifest.json`、同快照四出口，以及 `runs/<run-id>/` 的真实原件；私有模型/工具/Verifier正文不复制进本文。本地 `.cache/` 仅取回小回执。

| SHA-256 | E2B | 华为 |
| --- | --- | --- |
| acceptance-summary.json | `796f600cdf0c9a225bbc6f23d3f6fc866b86f2291ee69caaef3a860f1e15a69f` | `8b6fdd0d92ee929a0800da369a1be8c9b05e67470fdfd64871c43b8bad2db139` |
| 原evidence.json | `c48d193a46713042e4ec4b4a58b67ae1e9510cbb3adbf96d0501469f02bbc932` | `524fc19feac4354583a33c9e2182e036a5252e8cf2579088ec105d65fab697dd` |
| raw-artifact-manifest.json | `8176fbe54f4233eab2b2108a61fd59a5d46f188c5aa4a03e3e608cce9b444d14` | `57711f40a91861608cd727c053fc39740d384c8d90fd5c964457dc8c0eaff457` |
| runner-events.jsonl（阶段补充来源） | `4e6920d17b2507779e5ad0a30ed394071729f3b59286e54e92f507334067e6d6` | `7c0d750d8cf13e7c6e15d2c1ac1e6bce676550ffddf070c6730d64b189a01bf3` |

run-id分别为 `20261027T034631.729288072Z-jiuwen-e2b-external-record-1`、`20261027T034839.057292163Z-jiuwen-huawei-external-record-1`；阶段原件位于对应 `runs/<run-id>/performance/runner-events.jsonl`。两云阶段补充只读对账回执 `.cache/jiuwen-external-cloud-1/dual-cloud-handoff.json` SHA `7d4ccc5f52c1a802aefab67374209abe2f700c9185db663889af339535c475f4`。使用对应冻结输入和原件可复核功能、清理、阶段与投影，不将本轮有限验收宣称为P0全部完成或生产就绪。
