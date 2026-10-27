# Jiuwen release23：双云 A/B 命令进程与后端验收（2026-10-27）

E2B 与华为 A/B 四组真实 record 均通过原独立 Verifier 12/12，合计记录 15 个命令进程、105 条生命周期事件，声明来源内无丢失或写失败。8 个 Sandbox 均已删除并由 provider 独立确认 NotFound。四组各自冻结快照的 Perfetto、Jaeger 和 Prometheus 实际查询均通过，验收所用后端容器与 listener 已清理。

四次 record 是独立真实模型决策，不能按耗时给云厂商排名，也不构成 Strict Replay 结论。四组冻结快照均为 `integrity=complete / coverage=unknown`；进程来源内完整不代表未插桩工作的全局覆盖。华为独立评分沿用已批准的出网隔离假设，不升级为已验证隔离或 Goodput。

## 固定版本与实际范围

任务为 `merge-ranges-portable-v1`：真实 Agent 修复代码并运行公开检查，停止、撤权后由独立评分 Sandbox 执行原来的 12 项私有断言。A 的 Agent 在 ECS 容器，通过原生 SysOperation Provider 和 Aries Bridge 到任务 Sandbox；B 的 Agent 和 LOCAL 工具同在任务 Sandbox。两云任务模板均为 2 CPU / 4096 MB，A 另外消耗 ECS Agent 容器资源。

release23 已冻结 861 个源文件并通过 build/test/test-race/lint/integration 五门禁。JiuwenSwarm 固定 `e2c9d519498080e249e34f16a80c8c97f0cd1962`，agent-core 固定 `3460150aa5f319b0b8f84a4ed10eec0a08b0feb1`。本轮使用 image7-process1，digest `sha256:ba27c904098a8e41c3efe297e773d16b208c2e77edf38e206b70eb0ee335ce21`；它保留 image5 补丁并追加原生进程协议观察，不能称未修改上游。

E2B 两组固定模板 `ka5ej4mq5hsk1kf91gvp:df971e4f-721e-4456-90d0-c6da803db29a`；华为两组固定模板 `7697f6d1-2b69-4090-88d8-1bb77e8adb44:db05fc28-683a-40a4-9382-d76f5c928775`。A 另显式启动上述 digest 的 ECS 容器；B 使用模板内原生运行环境。

| 运行 | 原评分 | 完整模型轮数 | 原生 ToolCall | SDK started/ended/exported | 进程 / 事件 / dropped | 清理 |
| --- | --- | ---: | ---: | --- | --- | --- |
| E2B A | 12/12，score=1 | 9 | 8 | 54/54/54 | 4/28/0 | 2 个 Sandbox 删除且确认 NotFound |
| 华为 A | 12/12，score=1 | 9 | 8 | 55/55/55 | 4/28/0 | 2 个 Sandbox 删除且确认 NotFound |
| E2B B | 12/12，score=1 | 9 | 8 | 44/44/44 | 4/28/0 | 2 个 Sandbox 删除且确认 NotFound |
| 华为 B | 12/12，score=1 | 6 | 6 | 33/33/33 | 3/21/0 | 2 个 Sandbox 删除且确认 NotFound |

四组进程 started/exited 一致，observer 已关闭，pending、identity_failed、late_events、write_failed、index_write_failed 均为 0。SDK 的导出分母仅为已采样 Span。两组 A 的容器、网络，以及各组 Runner 进程组和模型 listener 均已回收；这些清理结论各自来自本组回执。

## 阶段耗时

单位为秒，排队保留 9 位小数，阶段与总时间显示 3 位，剩余时间显示 6 位；计算使用原始纳秒字段。定义沿用[统一阶段说明](performance-stages.md)。Bridge 准备单列，B 对应嵌入式访问绑定，不能解释为远端转发。

| 阶段 | E2B A | 华为 A | E2B B | 华为 B |
| --- | ---: | ---: | ---: | ---: |
| 排队 | 0.000024270 | 0.000026491 | 0.000027690 | 0.000027473 |
| 任务 Sandbox 创建 | 1.477 | 0.607 | 1.042 | 0.568 |
| 工作区准备 | 3.153 | 1.086 | 2.288 | 1.337 |
| Bridge 准备 | 13.480 | 3.661 | 0.450 | 0.122 |
| Harness 启动 | 38.677 | 36.454 | 53.894 | 23.942 |
| Runtime 执行 | 37.514 | 36.298 | 40.130 | 28.687 |
| 停止与撤权 | 4.284 | 1.882 | 4.128 | 1.752 |
| 独立验证 | 6.551 | 2.273 | 5.634 | 2.136 |
| 清理 | 1.259 | 0.813 | 1.580 | 0.699 |
| **Attempt 总时间（不含排队）** | 106.469 | 83.149 | 109.738 | 59.551 |
| 未归入上述阶段的剩余时间 | 0.074105 | 0.075122 | 0.591751 | 0.309182 |

`提交至结束 = 排队 + Attempt`；`Attempt = 排队以外的已记录阶段 + 剩余时间`。剩余时间不猜测分摊。模型、Tool、Provider、Bridge、worker 和命令进程区间存在嵌套或重叠，不能再次加到 Attempt，也不能据其总和认定关键路径。driver 时间还包含控制与归档，不替代 Attempt。

## 逐命令进程与 ToolCall

以下单位为毫秒。“创建”是 `spawn_requested` 取样至 `connection_made` 回调入口，包含起点后的同步观测写入、创建和协议接入，不是纯 fork/exec 成本；起点前的身份快照初始化不包含在内。“命令”是 `connection_made` 至 `process_exited` 回调入口，含用户态事件循环响应延迟，不是内核 exec/exit 时间戳或 CPU time。“首 stdout”是首次非空协议回调相对 `connection_made` 的时间，不是模型/UI 收到输出的时刻。

动作按本组 Tape 的**精确 ToolCall ID** 对账，只概述公开 coding case 的动作，不公开命令正文或模型消息。process ID 以下保留前 8 位用于阅读，完整 UUID、invocation 和 ToolCall 对照保存在小摘要与原账本；不靠 UUID 前缀、命令哈希或时间近邻建立关联。所有列出的原生 Tool 均为 `bash`，其进程↔原生 Tool 关联为 `exact_id`。

### E2B A

| process ID 前缀 | Tape 轮次 / 动作 | ToolCall ID | 创建 ms | 命令 ms | 首 stdout ms | exit | 退出后关闭 ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| `cd75220d` | 1 / 读取公开检查文件 | `call_00_otFdwjmyZ38Tr1wbtdYa9408` | 1.202 | 3.408 | 2.500 | 0 | 不生成¹ |
| `2a300161` | 2 / 公开测试 1 | `call_00_mkqvkpOL0RymMroJXDwF1255` | 0.692 | 653.324 | 599.473 | 1 | 不生成¹ |
| `f7018f34` | 4 / 公开测试 2 | `call_00_ET_kIrRTbIMPApbi0EycRgY5382` | 1.084 | 316.657 | 268.053 | 1 | 不生成¹ |
| `8e86195b` | 8 / 公开测试 3 | `call_00_ET_KKYKqTPcKXWlOEs3oNvl2458` | 0.819 | 342.279 | 279.863 | 0 | 不生成¹ |

### 华为 A

| process ID 前缀 | Tape 轮次 / 动作 | ToolCall ID | 创建 ms | 命令 ms | 首 stdout ms | exit | 退出后关闭 ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| `1ba43957` | 1 / 读取公开检查文件 | `call_00_eo5pnoM5xfeJEREG3i8w3284` | 0.618 | 2.804 | 2.209 | 0 | 不生成¹ |
| `87495785` | 2 / 公开测试 1 | `call_00_UHTPLYblrJKQG2UTRsVR3934` | 0.498 | 498.660 | 465.603 | 1 | 不生成¹ |
| `e33ec47e` | 4 / 公开测试 2 | `call_00_AYECrOtL9apVs3hef1QQ0418` | 0.474 | 408.014 | 377.233 | 1 | 不生成¹ |
| `762896ca` | 8 / 公开测试 3 | `call_00_ET_s3MGsmg76Q3PWRwiWLk02549` | 0.857 | 397.593 | 376.597 | 0 | 不生成¹ |

### E2B B

| process ID 前缀 | Tape 轮次 / 动作 | ToolCall ID | 创建 ms | 命令 ms | 首 stdout ms | exit | 退出后关闭 ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| `78cbb551` | 1 / 读取公开检查文件 | `call_00_uiGc8m0kOcKsrbEQsCXX4725` | 5.695 | 1.544 | 1.347 | 0 | 1.011 |
| `9166501c` | 2 / 公开测试 1 | `call_00_elqoY9HcHqvYrOK29fUv7325` | 5.153 | 547.667 | 493.997 | 1 | 不生成¹ |
| `c99194b3` | 6 / 公开测试 2 | `call_00_ET_Z3f0qrlpkHCBlmKQvT4x4724` | 6.482 | 291.405 | 245.737 | 1 | 不生成¹ |
| `de634baa` | 8 / 公开测试 3 | `call_00_LBjuwrkR2VkxI1zEs6SR1121` | 6.267 | 257.597 | 222.059 | 0 | 不生成¹ |

### 华为 B

| process ID 前缀 | Tape 轮次 / 动作 | ToolCall ID | 创建 ms | 命令 ms | 首 stdout ms | exit | 退出后关闭 ms |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| `174ac520` | 1 / 读取公开检查文件 | `call_00_Xj0D8cpM2wjn8gtsv47H9694` | 3.353 | 5.079 | 3.796 | 0 | 不生成¹ |
| `fd3d8b44` | 1 / 公开测试 1 | `call_01_C88SHTtviKfBdBjRtWrz3816` | 2.379 | 496.656 | 463.860 | 1 | 不生成¹ |
| `5a2b35c7` | 5 / 公开测试 2 | `call_00_cXFhr6YjvuACtD7V6BBw7169` | 1.868 | 390.941 | 370.046 | 0 | 不生成¹ |

华为 B 首轮两个进程的命令回调区间重叠约 4.849 ms，同属 Tape 第 1 轮但 ToolCall ID 不同。这是本次实际并行关系，不能把两段命令时长串行相加。

¹ “不生成”表示 stdout/stderr 关闭回调均不晚于 child_exit，不能填 0 或生成负时长。E2B B 首条命令实际出现退出后最后管道关闭的正区间；它仍只证明管道关闭回调，不证明自然 EOF、消费者排空或整棵后代树退出。各条 stderr 均未观察到非空块，不将其填成 0 ms。

公开测试中的 exit 1 是该条真实命令的中间结果；不能把它改写成成功，也不能因此推翻随后原独立 Verifier 的 12/12。各组模型决策、工具调用与修复过程独立；相同测试命令也不是同一 Tape 的性能对照。

后台 Tool 的“启动成功”不代表进程退出；后台命令可延续到 Tool 返回之后并与后续调用重叠。本轮表格仅陈述这些实际命令，不以正常样本代替后台/取消专项测试。JSONL sequence 是写入尝试顺序；协议时间在回调入口取样、调用原生父方法后才发出观测。跨主机身份链接不证明时钟对齐。

## CPU、内存与资源归属

CPU 累计时间与上面的墙钟区间不同；进程观测器不采单条命令的 CPU 或内存。下表按实际拥有者展示，采样最大值只覆盖返回窗口，不保证捕捉瞬时峰值，也不是 Runtime 专属峰值。

| 对象与指标 | E2B A | 华为 A | E2B B | 华为 B |
| --- | ---: | ---: | ---: | ---: |
| 控制器进程生命周期累计 CPU（user+system 末值，秒） | 1.026 | 1.128 | 1.085 | 1.338 |
| 控制器进程生命周期 RSS 高水位（MiB） | 48.13 | 45.80 | 41.23 | 41.05 |
| A 外置 Agent 容器累计 CPU（末值，秒） | 41.228 | 41.715 | 不适用 | 不适用 |
| A 外置 Agent 容器内存采样最大值（MiB） | 488.36 | 419.67 | 不适用 | 不适用 |
| B Agent 进程组累计 CPU（stopped 边界，秒） | 不适用 | 不适用 | 38.896 | 22.940 |
| B Agent 进程组生命周期内存高水位（MiB） | 不适用 | 不适用 | 601.00 | 606.99 |
| 任务 Sandbox provider CPU 返回最大值（%） | 66.67 | 28.22 | 51.50 | 51.40 |
| 任务 Sandbox provider 内存返回最大值（MiB） | 378.28 | 3588.00 | 728.40 | 3715.00 |
| 单 Tool / 命令 CPU、内存、I/O、Network | 未采集 | 未采集 | 未采集 | 未采集 |

控制器数值是控制器进程自有生命周期资源，模型代理未单独拆分。A 的容器 CPU/内存属于外置 Agent，不属于远端某个 Tool。B 进程组资源包含 Agent、其后代及启动阶段，不能当作某条 Shell 或纯 Runtime 的消耗。任务 Sandbox provider 内存范围更大，不能与 Agent 进程组内存混为一谈；不同 provider 的原指标口径也未统一。

E2B provider 的重复历史返回不增加有效时间点数；已归档 E2B A 任务侧 231 个回执中只有 20 个有值时间点，独立评分侧 1 个；E2B B 任务侧 21 个有值时间点，评分侧仅有 2 条无值采样回执。华为 A 任务侧 18 个有值时间点、评分侧 2 个；华为 B 分别为 13 个和 2 个。华为仅显式 `mem_used_mib` 按 MiB 展示，原 `mem_used` 单位保持 unknown，provider 时间戳仍 unverified，不能强行对齐到 Tool。

两组 B 的进程组块设备累计末值分别为：E2B read 173,920,256 B、write 1,683,456 B；华为各设备流合计 read 180,613,120 B、write 1,683,456 B。它们是设备归属计数，不是文件正文大小，也不是每条命令 I/O。无读数不能补零；任务级 Network 字节与单 Tool 资源归属仍未采集。

## 同一冻结快照的后端状态

每组使用自己的不可变 Snapshot，分别完成 Perfetto TraceProcessor、OTLP→Jaeger、OTel Metrics→Prometheus 的实际导入和查询；输入 SHA 在前后保持一致。下表的进程区间包括 observation 容器，不能把区间数解释为命令条数。

| 组别 | Perfetto controller 区间 / flows | Perfetto process 区间 / markers | Jaeger spans / 校验项 | Prometheus 有值流 PASS / NOT_COLLECTED |
| --- | ---: | ---: | ---: | ---: |
| E2B A | 214 / 61 | 12 / 28 | 302 / 3,635 | 5 / 0 |
| 华为 A | 213 / 61 | 12 / 28 | 302 / 3,635 | 3 / 2 |
| E2B B | 794 / 61 | 13 / 28 | 851 / 10,223 | 5 / 1 |
| 华为 B | 538 / 46 | 9 / 21 | 580 / 6,971 | 5 / 2 |

Perfetto 的进程相对时间误差为 0 ns，保留 105 个生命周期 marker；跨 clock process flow 为 0。Jaeger 查询对账 2,035 个 Span、24,464 项检查，v3 精确 epoch 的 start/end/duration 误差均为 0 ns。旧版微秒投影另按既有容差校验，本轮 start/duration 截断误差绝对值分别最多 997/999 ns，end 最多 1,948 ns；不把这项结果称为全部投影零误差。

15 条 process→native Tool 精确身份链接和两组 A 的 8 条 process→invocation 链接均保留；B 不伪造 Provider/invocation 关联。两组 A 还各有 11 个 Provider Span、44 个生命周期事件和 22 个 Provider 身份链接。控制端、worker/process 单调域与原生 Unix 时间分开，这些身份链接不证明跨主机时钟校准。

18 个有值 Metrics 流（包括 4 个 attempt 流）均完成首次提交和再次提交相同 payload 后的实际查询；series、sample、值及桶计数对账通过，值误差为 0，查询时间戳的毫秒截断误差绝对值小于 1 ms。5 个流仍为 NOT_COLLECTED：华为两组的任务/评分 provider 流缺少已验证时间戳，E2B B 评分侧 2 条无值回执也未导出有效时间/单位的资源点；导出回执保留 `sampling_timestamp_not_verified` 与 `no_verified_time_and_unit_resource_points`。上面的原资源摘要仍保留收到的华为值，不把未导出当作没有收到，也不以零补齐。

进程直方图只有 31 个样本：15 个 creation、15 个 command、1 个 exit_to_pipe_closed。46 个进程区间中另外 15 个 observation 容器被排除，避免重复累计。重复提交结果仅证明本次固定 payload 查询稳定，不代表生产 exactly-once、实时上报、持久存储或 UI 行为。

四组后端 postflight 均确认测试容器、listener、cgroup 不残留，共享锁可获取并已释放。云清理与后端清理来自不同所有者回执，不能互相代替。自然 EOF、单条命令的全部后代完成、跨主机时钟校准与全局覆盖不在已验范围；四组快照继续保持 `integrity=complete / coverage=unknown`。

## 数据来源与 SHA-256

ECS 根目录为 `/opt/aries-artifacts/`。阶段原值来自各组 `acceptance-summary.json` 的 `stages[].duration_ns` 和 `attempt_duration_ns`；进程区间来自 `observation.process.intervals`。资源表从该组冻结 `snapshot.json` 的原 owner stream 取 cumulative 末值或采样最大值，bytes 以 2²⁰ 换算 MiB；不积分 CPU percent。动作仅从原 Tape 中同一 ToolCall ID 对应的命令概述获得，正文不进入报告。

| 运行目录 | acceptance-summary.json SHA-256 |
| --- | --- |
| `jiuwen-e2b-external-record-3` | `9b0c4b99200a4986ece9e94fe840938a7ca3c936ada84f5ebdcaaee88180fc26` |
| `jiuwen-huawei-external-record-3` | `d8bb0377acf10758ea2034b70636b3f360f186247d91fa77e2d2fe9884c3ac0f` |
| `jiuwen-e2b-record-9` | `76f9ba1be1294f17ab3a932dbc63b2db373cb9f65ed2052110ed7bcbbbdecb48` |
| `jiuwen-huawei-record-4` | `18b6f80db140a27d205f38388d8c56944a5eb90c664289bc3bbac786f2b9da22` |

| 组别 | snapshot.json SHA-256 | 原 Tape SHA-256（仅留在 ECS） |
| --- | --- | --- |
| E2B A | `3cde54a003f2db5c71ca78f4560010b44eadcd4a7c0e798cb67f76b2f6740e64` | `9b1fe9ebe9f3d484a5d096b446a08e8eea3df53a219fc14e588d0e4064571afa` |
| 华为 A | `16a31f929b265a8f6a6c0cc5b73b5144bce3042f38f694b302e036f0cb4a3213` | `692eb4020ded9367e68d31ce9f5a052f7a3ce4e752e1bf5b42c1e1c42e8723c6` |
| E2B B | `77465dfcadeea2a2b4aa20998e8640f8d28aaf601e316455c8e6f789eaad2a1a` | `dfc3dc652d928b1f828ab8a8959d49f0809e166d8fdbdc113a5dbf40723d359d` |
| 华为 B | `27019733ce3e01f15dfa5133d1b5fc72f9dd7d33b17ca35a6d621c0982340e0f` | `e7feee5a95d21afd86d5e0c7ac98a5e3f79602fd3ea15024d83edc44dff67c53` |

release23 的 `freeze-manifest.json` SHA 为 `3cfe77ec01640dbde5c80eb3e039ad7bd2500098c69c3d1e193bc103bbd759fd`，`source-manifest.json` SHA 为 `70c510c4d2a8c07ee3601a87feb3113156ae585ff19a1de3cdc34fb7a0eaecc5`，五门禁 `evidence.json` SHA 为 `bfb3ad86c6adcdb65064cc18ecf28127a9e125fa38f5d538b38882ef1f94193c`。每组摘要将这些版本、二进制和原始产物 SHA 绑定到实际运行。

本机 `.cache/jiuwen-release23-cloud/*-summary.json` 与 `*-detail.json` 仅保存小型无正文摘要；detail 保留完整 process/ToolCall/invocation 映射、Tape 来源 SHA、资源字段与证据引用。原 Tape、模型消息、工具正文和私有评分文件不加入公开文档包。后端小证据在本机 `.cache/jiuwen-release23-backend-evidence/`，其汇总 `jiuwen-release23-backends-summary.json` SHA 为 `02a2ccb76506be07f2b1fad2a165b96b8b46f1f9de0f7e6423149880c09f5f1b`，逐组绑定上表原 Snapshot SHA，并保留实际查询、reconciliation 和 cleanup 来源。

| 组别 / ECS 后端目录 | evidence.json SHA-256 | metrics-evidence.json SHA-256 |
| --- | --- | --- |
| E2B A / `jiuwen-release23-backends-1` | `dbbe5435e0119fe8098e63fe4a1edafed246a41d0a7695dfa3d33c1fc5d0ee79` | `d726a0e6c5150b3f67a748abcdea53207a6ff0300d24cdf824d755314dccde13` |
| 华为 A / `jiuwen-release23-backends-2` | `e09d7b56a3a9200d111d7aee7097ce8676b807dbae74abdc32e30961c82a60f0` | `84dc617d139e5804d65c3a7360d6c1ce44e7d08e910eb7c7bb32bfde04eb78e7` |
| E2B B / `jiuwen-release23-backends-3` | `5c1c4df556c458d37e6c166fbadf8f1298d0d1e2cbc347f236e79f312c4d6d87` | `c3aa2f5e3323267ee84342d1c005f48d69de0c52ae34cf77c13d6501f55c1ae8` |
| 华为 B / `jiuwen-release23-backends-4` | `251bedb5d23de904f075ac605cb002a17be2c57b97025587acafe731d7ede60d` | `72c80f9c99af11fbaf7ea2f2a81580369a1361a6aa3035142e9bc2f8355ccdc3` |

动作/资源小摘要的生成脚本 `report-readonly.py` SHA 为 `5961c10c9068e1aea95c8817f273837667c26e60ebac02c7000c809fd186ff66`；本机 `.cache/jiuwen-release23-cloud/source-summary-manifest.json` 列明四组摘要文件 SHA、Snapshot/Tape 身份与字段来源。该脚本只读 ECS 已有产物，本页编写未重跑模型、测试或云创建。
