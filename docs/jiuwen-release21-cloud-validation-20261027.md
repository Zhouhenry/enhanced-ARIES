# Jiuwen release21：双云 A/B 与观测后端验收（2026-10-27）

**四组真实 DeepSeek 解题均通过原独立 Verifier 12/12、score=1；本轮 8 个 Sandbox 均确认删除。A 模式原生 ToolCall→Bridge→worker 已精确贯通；E2B A 的同一冻结快照通过 Perfetto、Jaeger、Prometheus 实际查询对账。**

本轮优先完成非 Strict 接入。四份 Tape 是独立真实模型决策，不能按这张耗时表给云厂商排名；既有六例 Strict 3/6 与 Workload 结果保持原口径。全部样本 `integrity=complete / coverage=unknown`；这代表已声明来源的完整性，尚不代表完整内部观测或生产化全部完成。华为独立评分采用已批准的出网隔离假设，不计作已证明隔离的 Goodput。

## 任务、版本与真实执行范围

任务为 `merge-ranges-portable-v1`：Agent 在真实工作区读取和修复区间合并代码，运行公开检查，再由单独评分 Sandbox 执行原来的 12 项私有断言。原生 Flash 六个文件/Shell 工具、后台能力、工具结果与独立评分保持；不使用参考解冒充 Agent。A 的 Agent 在 ECS 容器，工具经官方 SysOperation Provider 到任务 Sandbox；B 的 Agent 和 LOCAL 工具都在任务 Sandbox。

当前固定 release21（843 文件、47 个增量文件），ECS build/test/test-race/lint/integration 全部通过。JiuwenSwarm `e2c9d519`、agent-core `3460150a`、image5 digest `sha256:b4078957d488e1cb52e9cc85ef5555208f0281a545c580a740a619f52127c6b1`。镜像含显式身份/远端 Glob/session-cancel 补丁，不能称纯上游未修改镜像。两云任务模板均 **2 CPU / 4096 MB**；A 的外置 Agent 另外使用 ECS 资源。磁盘等平台差异说明保留，不作为对比阻断条件。

| 本轮运行 | 原评分 | 完整模型轮数 | 原生 ToolCall 数 | SDK started/ended/exported | 清理 |
| --- | --- | ---: | ---: | --- | --- |
| E2B A | 12/12 | 9 | 8 | 44/44/44 | 2 Sandbox 删除确认，错误 0 |
| 华为 A | 12/12 | 9 | 8 | 43/43/43 | 2 Sandbox 删除确认，错误 0 |
| E2B B | 12/12 | 7 | 6 | 37/37/37 | 2 Sandbox 删除确认，错误 0 |
| 华为 B | 12/12 | 7 | 6 | 35/35/35 | 2 Sandbox 删除确认，错误 0 |

四组 SDK 导出对账 missing/unexpected 均 0。它的分母仅为已采样 SDK Span，不含未插桩工作。两组 A 还确认外置容器、网络已回收；所有 Runner 进程组、模型 listener 与任务/评分资源均结束。

## 每段时间的含义与关系

以下全部为本轮真实字段，单位秒。阶段定义见[统一阶段说明](performance-stages.md)。新增 `Bridge 准备` 位于工作区准备之后、Harness 启动之前；原八字段保留原义，历史记录没有这一字段时仍为未采集。B 的 Bridge 准备是其嵌入式访问绑定工作，不是 A 的远端转发。

| 阶段 | E2B A | 华为 A | E2B B | 华为 B |
| --- | ---: | ---: | ---: | ---: |
| 排队 | 0.000024 | 0.000018 | 0.000013 | 0.000014 |
| 任务 Sandbox 创建 | 1.341 | 0.782 | 1.089 | 0.553 |
| 工作区准备 | 3.692 | 1.174 | 2.557 | 1.147 |
| Bridge 准备 | 13.616 | 3.271 | 0.520 | 0.118 |
| Harness 启动 | 40.011 | 35.762 | 53.583 | 22.743 |
| Runtime 执行 | 40.150 | 31.986 | 31.934 | 28.574 |
| 停止与撤权 | 2.205 | 1.627 | 4.151 | 1.688 |
| 独立验证 | 7.117 | 2.425 | 5.820 | 2.183 |
| 清理 | 1.417 | 0.716 | 1.280 | 0.779 |
| **Attempt 总时间（不含排队）** | 109.641 | 77.816 | 101.518 | 58.075 |
| 未归入上述阶段的剩余时间 | 0.090348 | 0.072824 | 0.585130 | 0.289536 |

关系是：`提交至结束 = 排队 + Attempt`；`Attempt = 排队以外的已记录阶段 + 剩余时间`。剩余时间按原字段相减保留，没有按猜测摊入工具或网络。模型等待、原生 Tool、Bridge 和 worker 请求都是 Runtime 内的子区间；存在嵌套或重叠，不能再次加到 Attempt，也不能把所有 Tool 耗时之和称作关键路径。driver 的总时间还包含启动控制与归档，不等于 Attempt。

## A 的工具身份与请求计时

两云 A 均为 **8 个原生 ToolCall、11 次 Provider/Bridge 调用、11 对 worker 区间**。一个文件工具可调用多个 Provider 方法，11 次不能误报为 11 个模型 ToolCall。每次均按 Attempt/Session/Request/ToolCall 四元组唯一关联原生 Tool；Bridge→worker 再核对 invocation 身份和操作。两组均 11 条原生身份边，未用命令哈希或时间近邻。

| 来源 | 两云 A 各自实际对账 | 时间边界 |
| --- | --- | --- |
| 原生 Tool span | 8 个有准确 ToolCall ID 的 span | 原生 Ability 调用，保留原始 Unix 时间 |
| Bridge forwarding | 11 个接受调用、11 个完成区间/记录 | 鉴权和 JSON 解析后，至转发响应 `io.Copy` 返回；不含随后 Body.Close |
| worker native operation | 11 个接纳调用、33/33 预期事件、11 对区间；dropped/write_failed=0 | 单调起点在 response.prepare 前，鉴权/参数解码/init_cwd 后；终点为 invocation_terminal 采样 |
| 精确身份关系 | 原生 Tool→Bridge 11 条；Bridge→worker 11 条 | 身份关联不证明跨机器时钟同步 |

worker 正常终点在最后 end 帧 await write 返回后、终态账本写入前，不能等同客户端收到、进程退出或 pipe drain。Bridge 尚无独立 drop/write-failed 计数，因此这些字段仍 null，不用成对记录宣称全局零丢失。两层时间重叠，差值不能直接当网络 RTT。B 不走 RemoteBridge，相关记录不适用；B 原生 Tool span 也不能冒充进程细分计时。

## CPU、内存、I/O 与 Network

资源按拥有者展示，不能把整台 Sandbox 的内存或外置 Agent 的 CPU 归到单个 Tool。CPU 累计秒是消耗的 CPU 时间，墙钟秒是等待了多久；两者不同。下表的采样最大值仅覆盖实际返回窗口，不能保证捕捉瞬时峰值。

| 对象与指标 | E2B A | 华为 A | E2B B | 华为 B |
| --- | ---: | ---: | ---: | ---: |
| 控制器生命周期累计 CPU（user+system，末值，秒） | 1.334 | 1.086 | 1.148 | 1.257 |
| 控制器进程生命周期 RSS 高水位（MiB） | 48.11 | 45.89 | 45.54 | 42.77 |
| 外置 Agent 容器累计 CPU（末值，秒） | 42.007 | 40.437 | 不适用 | 不适用 |
| 外置 Agent 容器内存采样最大值（MiB） | 518.50 | 421.08 | 不适用 | 不适用 |
| B Agent 进程组累计 CPU（末边界，秒） | 不适用 | 不适用 | 36.769 | 23.138 |
| B Agent 进程组生命周期内存高水位（MiB） | 不适用 | 不适用 | 601.35 | 608.06 |
| 任务 Sandbox provider CPU 返回最大值（%） | 24.49 | 27.58 | 51.40 | 51.35 |
| 任务 Sandbox provider 内存返回最大值（各自声明单位） | 372.90 MiB（bytes换算） | 3607 MiB（原字段） | 723.95 MiB（bytes换算） | 3705 MiB（原字段） |
| 任务 Sandbox 有值时间点数量 | 21 | 17 | 19 | 13 |
| 独立评分 Sandbox provider 读数 | 1 个有值时间点 | 2 个时间点 | 未返回有效读数 | 2 个时间点 |
| 单 Tool 的 CPU / 内存 / I/O / Network | 未采集 | 未采集 | 未采集 | 未采集 |

B 的进程组资源在 start/runtime_start/runtime_end/stop 四个边界采集，生命周期 CPU/内存含启动，不是纯 Runtime 数值。E2B B 的设备块 I/O 末值为 read 173,752,320 B、write 1,683,456 B；华为 B 分两个设备，末值合计 read 180,461,568 B、write 1,683,456 B。它们是内核设备归属的累计计数，不是文件内容字节数，也不是从首次有值边界起的增量。起始 io.stat 为空时保留缺测，不能补零。A 没有本轮逐进程块 I/O；四组任务级 Network 字节未采集，独立模型代理 CPU 也未从控制器进程拆出。

E2B task API 多次返回同一批历史读数：A 有 233 个回执但仅 21 个有值时间点，B 为 200/19；不按回执条数增加样本量。外置 Agent 容器各 81/68 个采样。华为原始 mem_used/disk 字段的单位仍标 unknown，仅显式 mem_used_mib 按 MiB 展示，provider 时间戳状态仍 unverified，不能强行对齐到原生 Tool。华为 mem_used_mib 显著高于 Agent 进程组内存，二者范围不同；不能据此断言 Agent 内存泄漏或云性能差距。磁盘规格差异单独保留在模板/原读数，不阻断后续同工作量对照。

## 同一快照的双 Trace 与 Metrics 后端对账

本轮选用 **E2B A record2** 的不可变 Snapshot，分析器 `aries-analysis-v1.14.1-jiuwen-native-tool-links`；其他三组已导出其各自快照和 Trace/Metrics，但未冒用本组后端查询结论。对账没有重新执行 Agent、Sandbox 或付费模型调用。

| 验收项 | 实际结果 |
| --- | --- |
| Perfetto TraceProcessor | 217 个控制端区间、61 条同域 flow；44 个原生 span、43 个精确 parent；11 个 worker 区间。时长/相对时间导入误差 0 ns |
| OTLP→Jaeger | 实际发送、实际查询 272 个 span，3275 项核对通过；ID、parent、Link、事件与时间同快照，start/end/duration 误差 0 ns |
| OTel Metrics→Prometheus | 主指标和 4 个资源流分别使用独立 TSDB；每组首次发送后明确重复发送。两次各查得 578 个样本、2732 项总核对，数值一致无重复增长 |
| Prometheus 时间精度 | 实测截断误差 -992,324 至 0 ns，符合后端毫秒时间戳；未声称纳秒无损 |
| 生命周期 | 所有临时后端容器/listener 回收，原始源 SHA 未变，共享锁释放 |

控制端单调时钟、worker 单调时钟、原生 Unix 时间分开；OTLP 的 Link 保留精确关系，不伪造父节点或对齐后的时间线。22 个远端请求（11 Bridge + 11 worker）进入独立请求 Histogram，不重复计入阶段 Histogram。此结论限离线可重现导出与真实后端查询；实时持续上报、UI 验收、持久化恢复和全局覆盖仍未获得本轮证明。

## 本轮证据与复现入口

所有编译、测试、部署和云运行均在 ECS。根目录 `/opt/aries-artifacts/`，以下各目录保存原始运行、Tape、评分、Snapshot、投影和清理回执；本机只回收小型无正文摘要。不要把原录制/私有工具或评分文件发布到公共文档包。

| 运行目录 | acceptance-summary.json SHA-256 |
| --- | --- |
| `jiuwen-e2b-external-record-2` | `7256296cfd059c5652b29be718fe62ca047219e96a61ea112a1af04b146f2414` |
| `jiuwen-huawei-external-record-2` | `1a0a16b578e83f28f3366f7eec598ef6bcba1e6af40e89427808907de0163799` |
| `jiuwen-e2b-record-8` | `a44f3feb978f6bab0446690e21fdb047256c240749343a3a54aa5b8e97bf2d91` |
| `jiuwen-huawei-record-3` | `7d0c563bcbee83249f95758aa270e7039c5378fdb0aac12060c1baacc3499be8` |

发布冻结回执：`release21/freeze-manifest.json`，SHA `2fbdbafa778a87487a051f3d97cfb61a419d784f82b89ea9e885e10228eac719`。五门禁回执 SHA `4d6367bac05c98a718d1b2de34464db6bf36f4cc49e04689a53eece49705a431`。

后端目录 `jiuwen-release21-backends-1`：`evidence.json` SHA `8854a4f8a13a6c2b59e41d9302b3f6e84ab75e29651c6302402e794028c8444b`，`metrics-evidence.json` SHA `a7b3ece3d8212674fd7cc49b567e303ae524746300a1a26565eb6c954deb9ea6`。目录内保存本次实际 `validate.py`、导出 JSON、SQL、查询回执及清理证据，复验应使用新输出目录，不能覆盖原件。

实际 public profiles 为 `profiles/cloud/jiuwenswarm-{e2b,huawei}-external-record.json` 与 `jiuwenswarm-{e2b,huawei}-embedded-record.json`；record 使用真实模型，不是 Strict Replay。ECS 执行脚本与输入映射归档于 `/root/aries-setup/jiuwen-final-cloud-1`，私有 input 文件不进入报告。

## 已关闭的本轮问题与仍需推进的范围

本轮已实测闭合：A 自动身份缺口、Bridge 准备漏入阶段、生产 bridge/ 目录漏读、最终 image5/release21 双云 A/B 功能回归，以及新身份/请求指标的实际后端对账。模型错误处理的实际 HTTP500 负例与远端 Glob/后台取消专项证据见[工程验证](jiuwen-native-runtime-validation-20261027.md)，不会把正常云任务冒充取消负例。

仍未完成：Agent→Bridge 提交段计时、guest spawn/首输出/exit/pipe-drain、未插桩行为的全局覆盖分母、跨机器时钟校准、单 Tool 资源归属、观测扰动和长期后端运行。四组均为单次功能验收，未进行容量或生产长期稳定性声明。R01–R18 的历史证据、未验范围和退出条件继续保存在[问题台账](observability/OPEN_ISSUES_AND_RESOLUTIONS.md)；不重开已封存的 I/O 支线，不放宽 Strict matcher。
