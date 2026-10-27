# Jiuwen PPT 工作量回放：本轮性能归档（2026-10-27）

本页主表选用 **A29 双云、B30 双云** 的同 placement / 同 Tape zero 样本。四种组合均已完成 Workload 回放和独立 PPT 评分；另完成两云 A 各一次保留模型响应节奏的回放。**新增 E2B B5 已用 release31 完成下载与采集预算修复的真实回归**：大 Trace、Producer 审计、80 个 Tool 的精确关联及四项离线投影通过，PPT 评分和清理通过。B5 单列，不混入 B30 主表；B2/B3 下载失败、B4 审计超时的三次观测失败保留。本轮共 9 次 zero、2 次 recorded，即 11 次回放尝试、10 次功能成功、1 次首轮失败。所有回放均为 Workload，不是 Strict Replay；后端查询和全链覆盖状态分别记录。

汇报视图已交付：[最小分享 ZIP（7.12 MB）](dashboard/jiuwen-pptx-dashboard-share.zip)，解压后双击 `index.html`，无需安装开发环境；[本机 Dashboard](dashboard/jiuwen-pptx-huawei-a/index.html)和[完整归档 ZIP](dashboard/jiuwen-pptx-huawei-a/jiuwen-pptx-huawei-a-dashboard-v3.zip)也保留。首屏依次展示全局八阶段对比和 Sandbox 工具耗时；随后用可点击高亮的时间范围图解释包含关系，再按组查看模型、工具动作、CPU / 内存和 PPT。完整 Trace 专区固定展示第 03 组华为去等待回放。六组均已完成，样本与口径如下。

分享包仅缩减字体、去除开发附件和重复资产，保留六组真实最终 PPT 与五份 Trace 原 gzip；看板可离线查看，Perfetto 官方查看器需联网。包大小 7,118,800 字节，SHA-256 `14484c6fb202163ac8e8216fb820d315c98b32400b2ab3e96f711b409d0847ce`。ECS 已验证离线打开、六组切换、时间图、中文字体和下载。

v3 已在 ECS 完成桌面及手机浏览器检查，并与六份原始摘要和 Worker 账本核对 6,798 项、零差异。最终 ZIP 共 22,864,526 字节，SHA-256 `ac59398d790f83acff955f16410fc704de232aaeb4c8e5dabd3dbeb47e4ee089`；归档及内部资产安全扫描无凭据命中。原 v1/v2 保留；[交付验收说明](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-pptx-dashboard-v3)记录验证范围。

## 六组总览：先区分模型来源，再比较全局与 Sandbox

以下均为 A / 外置 Agent，Agent 在 ECS，工具在 4 vCPU / 8 GiB 云 Sandbox 内真实执行。真实模型两次使用 release27、各自生成决策；四组回放使用 release29 和同一华为 A Tape。两云在各自相同回放时序内对照，真实模型仅作端到端参考，不把不同工作量或版本的差额归因于云性能。

[时间口径图](dashboard/jiuwen-pptx-huawei-a/time-boundaries.svg)说明 Attempt、八阶段、Runtime、模型入口、Agent Tool、Sandbox Worker 与命令进程之间的包含关系；它是边界示意，不是按比例绘制的跨主机时间线。Worker 从请求校验后的处理起点计到终态，包含结果封装和发送；前台命令进程是其中一层，后台进程可能晚于工具返回结束。

| 组别 | 本次模型如何工作 | 云 | 完整任务秒 | Agent 运行秒 | ToolCall 数 | 独立 PPT 评分 |
| --- | --- | --- | ---: | ---: | ---: | --- |
| 实时模型 H | DeepSeek 实时生成决策 | 华为 | 1086.403 | 1032.478 | 78 | 通过 |
| 实时模型 E | DeepSeek 实时生成决策 | E2B | 1495.148 | 1422.830 | 89 | 通过 |
| 去等待回放 H | MockLLM 返回已录决策，不按录制节奏等待（zero） | 华为 | 169.414 | 108.915 | 78 | 通过 |
| 去等待回放 E | 同一 Tape，不按录制节奏等待（zero） | E2B | 196.489 | 130.566 | 78 | 通过 |
| 保留节奏回放 H | MockLLM 按录制的响应节奏返回决策（recorded） | 华为 | 1065.757 | 1007.439 | 78 | 通过 |
| 保留节奏回放 E | 同一 Tape，按同一录制响应节奏返回（recorded） | E2B | 1105.656 | 1034.892 | 78 | 通过 |

| 对应组别 | Sandbox 创建秒 | 已计时命令数 | 命令累计秒 | 命令覆盖说明 |
| --- | ---: | ---: | ---: | --- |
| 实时模型 H | 0.806 | 32 | 37.968 | 33 次 bash 中 1 次无进程区间 |
| 实时模型 E | 1.065 | 50 | 30.593 | 48 个 bash 命令和 2 个 grep 命令；另 1 次 bash 无进程区间 |
| 去等待回放 H | 2.447 | 32 | 11.802 | 33 次 bash 中 1 次无进程区间 |
| 去等待回放 E | 1.609 | 32 | 19.206 | 同一 Tape 对应 32 个命令，返回码一致 |
| 保留节奏回放 H | 0.604 | 32 | 11.876 | 同一 Tape 对应 32 个命令 |
| 保留节奏回放 E | 0.945 | 32 | 28.555 | 同 Tape 的 32 条命令逐 ID 配对，退出码一致 |

命令累计是工具内部区间，不与全局耗时相加；缺失区间不填 0。真实模型 ToolCall 总数与命令数不同，不以次数反推耗时。实时模型原件见[真实录制归档](jiuwen-pptx-capacity-20261027.md)，华为 / E2B summary SHA 分别为 `0e745240b94692de92a73b4bc6db8b721e4bf80dc693f36344714e155ad6f91f` / `878a7ed021cf2e960696b39e3798715baaebc27eb44494b55a6783488ac6c653`。

Sandbox 内也有 **Worker 服务处理**计时，不只测到了命令进程。Worker 收到并校验请求后，执行文件或命令操作，直到结果发送完成或异常终态；包含封装与发送，不是纯 CPU 计算。下面先列可精确关联 ToolCall 的请求，再列整个采集窗口，避免把辅助请求误算到某次工具上。

| 对应组别 | 可关联请求数 | 可关联服务累计秒 | 采集窗口全部请求数 / 累计秒 | 无 ToolCall 身份的请求数 |
| --- | ---: | ---: | ---: | ---: |
| 实时模型 H | 110 | 38.236 | 112 / 38.238 | 2 |
| 实时模型 E | 136 | 31.370 | 138 / 31.373 | 2 |
| 去等待回放 H | 110 | 12.067 | 112 / 12.068 | 2 |
| 去等待回放 E | 110 | 19.796 | 112 / 19.799 | 2 |
| 保留节奏回放 H | 110 | 12.155 | 112 / 12.157 | 2 |
| 保留节奏回放 E | 110 | 29.044 | 112 / 29.049 | 2 |

一次 ToolCall 可对应多次 Worker 请求，命令进程通常又位于其中，所以这些数量和耗时不能互加。文件读取/写入内部的系统调用尚未细分；后台命令也可能晚于工具返回结束。Dashboard 同时保留 Agent 侧 Tool、guest Worker、命令进程三个边界，不用任一层替代另一层。

## 1. 这个 case 测了什么

华为 A 的 `timing=recorded` 对照已完成：仍使用同一 A Tape、release29、image8 和 4u8g，仅恢复录制的模型帧时序；不重录、不调用真实上游模型。Attempt 实测 **1065.757 秒**，Runtime **1007.439 秒**，64 轮 / 72,102 帧全部消费，独立评分通过，两个 Sandbox 已确认删除。该运行单列，不混入 zero 跨云对照，详细口径见下节。

用户任务：**“幻灯片 生成一份关于大模型发展的pptx材料，写5页。”**

Jiuwen 读取技能和项目材料，查看目录，读写和修改文件，执行 PPT 生成、处理及转换命令，最后交付 PPT。模型决策来自真实录制的 Tape；这些文件操作和命令仍在本轮 Sandbox 中真实执行，没有直接复制录制时的工具结果来充当执行。

独立 Verifier 在 Agent 停止、工具入口撤权后读取导出的产物，检查：PPTX 可解析、**恰好 5 页、每页正文非空**。至少一份导出 PPT 满足这三项即可得到 score=1。它没有评分事实准确性、排版质量或是否完整走完 `ppt-creation` 技能。原生技能调用记录存在，但本轮不能据此宣称技能全流程成功。中间的 3 页 PPT 和空白模板继续保留为不合格文件。

| 位置 | Agent 在哪里 | 真实工具在哪里执行 | 读数范围 |
| --- | --- | --- | --- |
| A / external | 远程 ECS 的 Agent 容器 | 云厂商任务 Sandbox；经过 Provider / Bridge | Runtime 包含 Agent、转发及真实工具的全过程 |
| B / embedded | 云厂商任务 Sandbox | 同一个任务 Sandbox | Runtime 包含 Agent 与其工具；ECS 负责控制、MockLLM、独立评分调度 |

Benchmark、AgentHarness、ToolSandbox、ToolBridge 四角色保持不变。两种位置都在另外创建的评分 Sandbox 中验证产物。

## 2. 固定了什么，怎样比较

A 主表取 release29 的两云 replay1，B 主表取 release30 的两云 replay2；同一对照组内源码冻结一致，不把 B1 与 B2 当同版本样本混用。两云均使用 **4 vCPU / 8 GiB** 任务 Sandbox、同一 Jiuwen 原生版本及 image8、L1 观测、串行单任务、`replay_policy=workload`、`timing=zero`。磁盘、云平台底层硬件与网络路径可能不同，作为环境差异说明，不据此否定任务耗时对照。

| 对照组 | 共同 Tape 来源 | 模型轮次 / ToolCall | 跨云复用方式 |
| --- | --- | ---: | --- |
| A | 华为 A 原始 record3 | 64 / 78 | 华为 A 与 E2B A 读取同一份原件，未重新录制 |
| B | 华为 B 原始 record3 | 61 / 80 | 华为 B 与 E2B B 读取同一份原件，未重新录制 |

**比较 A 内两云、B 内两云。A 与 B 的 Tape 和工作步骤不同，不能把它们耗时接近或不同解释为 placement 的性能优劣。**

Workload 保持录制的模型决策、ToolCall ID、工具名与参数、顺序及分支因果检查；允许真实工具正文变化，以及明确定义的 Jiuwen 私有运行目录、提示日期和用户时间戳变化；release30 另限原生 Environment 块的 Linux 内核版本元数据。原始任务、附件和其余静态内容仍检查一致。Strict 路径没有放宽。本轮按同 Tape 的实际工具身份、命令返回码对账及独立结构评分确认成功，不额外追加未约定的语义等价门。

`zero` 取消模型录制时的等待延迟；HTTP 访问、认证、解析、完整历史比较、返回响应和 Agent 调度仍真实耗时。它不等于模型入口耗时为零，也不是 DeepSeek 的实时性能测试。

### 华为 A recorded：先看整项任务，再定位耗时环节

本次完整任务约 **17 分 46 秒**；其中 Runtime 约 **16 分 47 秒**，占 Attempt 约 **94.5%**。Runtime 内模型 HTTP 请求累计 **961.625 秒**，主要包含恢复录制帧时序的等待；这不是此次实时调用 DeepSeek 的推理耗时。Sandbox 的 32 条直接子进程区间累计 **11.876 秒**，是工具内部的局部时间，不能再加到 Runtime 或 Attempt 上。

| 完整任务中的阶段 | recorded 实测秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 0.604 |
| 工作区准备 | 0.777 |
| Bridge 准备 | 3.284 |
| Harness 启动 | 41.884 |
| Runtime 执行 | 1007.439 |
| 停止与撤权 | 2.744 |
| 独立验证 | 1.992 |
| 清理 | 0.712 |
| 八阶段合计 | 1059.435 |
| 未归属差额 | 6.321 |
| **Attempt 合计** | **1065.757** |

排队另计 **14.387 μs**，driver 外层 **1100.011 秒**。八阶段与差额在原始纳秒下精确相等，显示值独立四舍五入。78 个原生 Tool 区间累计 **24.232 秒**，其中 33 次 bash 对应 32 条命令计时，另一次原生错误缺少进程区间；缺失不补零。模型、Tool、cmd 是嵌套或可能重叠的层，不能加成新的任务总时间。

64 个请求的末帧目标时刻合计 **961.574674 秒**、实发合计 **961.611429 秒**；72,102 个帧目标均符合 recorded 时序契约，缓存重试及请求错误均为 0。实际发送有调度延迟：单次运行内帧延迟 p95 **1.067 ms**、最大 **14.121 ms**。这里统计的是响应窗口与帧调度误差，没有独立测出纯 sleep 耗时，不能把它解释为 0 延迟或逐纳秒一致。

ECS Agent 容器有 1051 次采样，首末 CPU 增量 **166.949 CPU 秒**、内存采样最大值 **607.898 MiB**；共享 Controller CPU 增量 **34.678 CPU 秒**、生命周期峰值 RSS **488.578 MiB**。华为任务 Sandbox 的 213 个资源回执返回 CPU 最大值 **13.71%**、内存最大值 **7698 MiB**，平台采样时间与缓存口径仍未核实；不当作本任务实际内存峰值。任务 I/O / Network、逐工具 CPU / 内存仍未采集。

该样本期间共享 ECS 与 Dashboard 构建/浏览器验证有重叠，已在 sample-conditions 中保留；没有并行大型工程门禁。与 zero 的 Runtime 差 **898.524 秒**不能直接命名为纯模型等待，也不是经过随机化重复的因果估计。新样本四项离线投影通过，但 Perfetto / Jaeger / Prometheus 实际后端查询仍 **UNRUN**，不借用 zero 的 PASS；global coverage 仍 unknown。

交付：[本次 5 页 final.pptx](../runs/jiuwen-pptx-20261027/huawei-a-workload-recorded-replay-1/decks/projects/llm-development/output/final.pptx)、[原始 summary](../runs/jiuwen-pptx-20261027/huawei-a-workload-recorded-replay-1/evidence/acceptance-summary.json)、[时序及资源对照](../runs/jiuwen-pptx-20261027/huawei-a-workload-recorded-replay-1/evidence/zero-recorded-comparison.json)、[清理确认](../runs/jiuwen-pptx-20261027/huawei-a-workload-recorded-replay-1/evidence/postflight.json)。summary SHA `312109003ab64d3a75cada1780b5b7ed820144be679df743bb39f6fb0f1e7844`；对照 SHA `7d50e5ce21ce85bf2ce4500a7940e161f987b532802e84b5947e76a56f0299c3`。

### 两云 recorded 对照已完成

E2B A 同一 Tape、release29、image8、4u8g 回放也已完成：64 请求 / 72,102 帧、78 ToolCall、32 条命令，独立五页非空评分及清理通过。32 个命令 ID 一一对应、退出码差异为 0。两云都没有真实上游模型调用。

| 同时序全局阶段，秒 | 华为 A | E2B A |
| --- | ---: | ---: |
| Sandbox 创建 | 0.604 | 0.945 |
| 工作区准备 | 0.777 | 1.775 |
| Bridge 准备 | 3.284 | 11.267 |
| Harness 启动 | 41.884 | 37.221 |
| Runtime 执行 | 1007.439 | 1034.892 |
| 停止与撤权 | 2.744 | 3.877 |
| 独立验证 | 1.992 | 7.194 |
| 清理 | 0.712 | 2.230 |
| 未归属差额 | 6.321 | 6.255 |
| **Attempt 总计** | **1065.757** | **1105.656** |
| 其中：Sandbox 命令累计（不可加到总计） | 11.876 | 28.555 |

这一次 E2B 的完整任务多 **39.899 秒**，Runtime 多 **27.453 秒**；其他阶段分别实测，不把差额直接命名为网络或纯 Sandbox 开销。两云每组仅一次，保留共享 ECS 条件差异，不推导总体排名。

E2B 原始 [final.pptx](../runs/jiuwen-pptx-20261027/e2b-a-workload-recorded-replay-1/decks/projects/llm-development/output/final.pptx) 已复制本机；[summary](../runs/jiuwen-pptx-20261027/e2b-a-workload-recorded-replay-1/evidence/acceptance-summary.json) SHA `2dc0d114ac53b3d3e0f418c5296dd3e3c8538ac60d6314bdffdd11bf42c9f5fe`，[两云时序与命令对账](../runs/jiuwen-pptx-20261027/e2b-a-workload-recorded-replay-1/evidence/recorded-crosscloud-comparison.json) SHA `6e301e7be54ad69803d5f8693d0db8f11681a33aa1faf985b0b81f62d4f54b91`。两云本次四投影 PASS；实际后端查询均 UNRUN，不继承 zero 的后端 PASS。两组均保留 global coverage unknown。

## 3. 完成情况和交付文件

本节至第 7 节主表列依次为华为 A29 / E2B A29 / 华为 B30-2 / E2B B30-2。E2B B2 的阶段及进程原始数据照录，native 缺失保持未知，不能以功能评分替代观测验收。

| 项目 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| Agent 状态 | succeeded | succeeded | succeeded | succeeded |
| 独立评分 | 1 | 1 | 1 | 1 |
| 录制模型轮次全部消费 | 64 / 64 | 64 / 64 | 61 / 61 | 61 / 61 |
| 精确关联真实 ToolCall | 78 | 78 | 80 | 未知；native 文件缺失 |
| 可配对命令进程 | 32 | 32 | 39 | 39（仅 process 证据） |
| 停止 / 撤权 | 已确认 | 已确认 | 已确认 | 已确认 |
| 云资源删除 / 不存在确认 | 已确认 | 已确认 | 已确认 | 已确认 |

| 组别 | 可直接交付的合格 PPT | 同次保留的中间产物 |
| --- | --- | --- |
| 华为 A | [final.pptx](../runs/jiuwen-pptx-20261027/huawei-a-workload-replay-1/decks/projects/llm-development/output/final.pptx)；5 页、各页非空 | content.pptx（3 页，未通过） |
| E2B A | [final.pptx](../runs/jiuwen-pptx-20261027/e2b-a-workload-replay-1/decks/projects/llm-development/output/final.pptx)；5 页、各页非空 | content.pptx（3 页，未通过） |
| 华为 B | [final.pptx](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/decks/projects/llm-development/output/final.pptx)、[大模型发展.pptx](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/decks/projects/llm-development/output/大模型发展.pptx)；5 页、各页非空 | content.pptx（3 页，未通过）、template-clean.pptx（5 页，未通过） |
| E2B B | [final.pptx](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-2/decks/projects/llm-development/output/final.pptx)、[大模型发展.pptx](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-2/decks/projects/llm-development/output/大模型发展.pptx)；5 页、各页非空 | content.pptx（3 页，未通过）、template-clean.pptx（5 页，正文为空，未通过） |

每一份文件都保留原件和 SHA 校验；不同运行生成的 ZIP 元数据可以不同，PPT 字节相同不是本模式的验收条件。华为评分的“禁止出网”仍为显式假设，未实测确认；这与已经确认的 Agent 停止、撤权、独立验证及清理分别记录。

## 4. 从完整任务下钻：Attempt → 八阶段 → Runtime 内部

单位：**秒**。本页编号 1–8 是 Attempt 内的八项实测阶段，**排队单列、没有混入这八项**。名称与原始 `source_field` 一一对应；Bridge 准备也有单独一行。历史总表的编号如有不同，以这里的字段为准。

| 阶段与字段 | 做什么 | 华为 A | E2B A | 华为 B | E2B B |
| --- | --- | ---: | ---: | ---: | ---: |
| 1. 任务 Sandbox 创建<br>`sandbox_create_duration` | 申请本次工具运行环境 | 2.447 | 1.609 | 0.781 | 0.975 |
| 2. 工作区准备<br>`sandbox_prepare_duration` | 放入题目输入并准备工作目录 | 0.951 | 1.194 | 0.897 | 0.899 |
| 3. Bridge 准备<br>`bridge_prepare_duration` | 建立或绑定工具访问通道 | 3.279 | 7.804 | 0.128 | 0.391 |
| 4. Harness 启动<br>`harness_start_duration` | 准备并启动 Jiuwen Agent | 42.264 | 36.406 | 22.368 | 37.177 |
| 5. Runtime 执行<br>`runtime_execution_duration` | 交付任务后运行 Agent 循环、模型入口和真实工具 | 108.915 | 130.566 | 108.636 | 186.679 |
| 6. 停止与撤权<br>`stop_revoke_duration` | 停止 Agent 并撤销工具访问，确认可独立评分 | 2.446 | 4.332 | 3.262 | 6.417 |
| 7. 独立验证<br>`verifier_duration` | 评分环境中检查候选 PPT | 1.844 | 6.264 | 2.177 | 6.810 |
| 8. 清理<br>`cleanup_duration` | 回收 Runner 管理的运行资源 | 0.772 | 2.036 | 0.736 | 1.323 |

| 另列的时间 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| 排队，单位 μs（Attempt 外） | 23.555 | 11.767 | 17.781 | 20.318 |
| 八项实测阶段之和，秒 | 162.920 | 190.211 | 138.984 | 240.671 |
| Attempt 内未归到上述八项的差额，秒 | 6.493 | 6.278 | 7.983 | 7.956 |
| Attempt 总耗时，秒 | 169.414 | 196.489 | 146.966 | 248.626 |
| driver 外层总耗时，秒 | 201.684 | 229.518 | 184.102 | 285.756 |

这些数的关系：

- **八阶段之和 + 表中未归属差额 = Attempt**。差额反映阶段计时之外的调度、交接等区间，本页不把它臆测成网络或 Agent Loop 的纯开销。
- **排队在 Attempt 之外**；提交到完成还包含调度交接，不能拿 driver 与 Attempt 的差直接称为排队。
- **driver 包住整个执行脚本**，还包含前置检查、服务准备、离线 Snapshot / Trace / Metrics 导出、归档和额外清理确认。因此 driver 不代表 Sandbox 的纯执行速度。
- **Runtime 内已经包含模型与工具**。下面的命令时间是 Runtime 中部分真实进程的局部区间，不能再与 Runtime 相加。存在嵌套、等待及重叠，也不能用 Runtime 减去命令累计值就宣称得到了纯 Agent Loop 开销。
- 八阶段是 Controller 上的同口径时长；guest 命令使用各自单调时钟。只比较各域测得的 duration，不跨主机相减绝对时间戳。

## 5. 工具到底执行了什么、耗时多久

ToolCall 不全是 shell 命令。读取文件、编辑文件、技能调用也各算一次 ToolCall；一次工具又可能产生多次 Provider / Bridge 调用。A 两云都是 78 个 ToolCall 和 112 次 Bridge/Worker 调用，这两种数量不能相加或互换。

| 工具类型 | A 的共同 Tape | B 的共同 Tape | 含义 |
| --- | ---: | ---: | --- |
| bash | 33 | 40 | 查看目录、生成/处理 PPT、转换/渲染、脚本执行等 |
| read_file | 26 | 24 | 读取技能、项目文件等 |
| edit_file | 12 | 4 | 修改内容或程序 |
| write_file | 4 | 5 | 写入内容或程序 |
| skill_tool | 1 | 1 | 请求 PPT 技能；不等于已完整完成该技能 |
| todo | 1 | 4 | Agent 任务记录 |
| web_flash | 0 | 1 | 原始 B 轨迹包含的原生检索工具 |
| send_file_to_user | 1 | 1 | 交付生成文件 |
| 合计 | 78 | 80 | Tape 决策数；E2B B2 native 关联缺失单列 |

命令区间是从原生进程连接建立，到**直接子进程退出回调**的墙钟时长；进程创建另记。它包含进程调度和 I/O 等待，不是 CPU 时间，也不保证所有后代进程、自然管道 EOF 都已结束。原始完整事件和结束状态保留，未采到的区间不补零。

| 命令进程指标 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| 配对进程数 | 32 | 32 | 39 | 39（仅 process） |
| 累计直接子进程时长，秒 | 11.802 | 19.206 | 12.258 | 14.921 |
| 最短 / 最长，毫秒 | 1.592 / 1825.252 | 4.443 / 3461.869 | 0.128 / 2030.360 | 1.077 / 3438.696 |
| 返回码分布 | 29 个 exit=0；3 个 exit=2 | 29 个 exit=0；3 个 exit=2 | 33 个 exit=0；6 个 exit=1 | 33 个 exit=0；6 个 exit=1 |
| 对源 / 同 Tape 前一云返回码差异 | 0 | 0 | 0 | 0（仅 process） |

A 两组均为 33 次 bash、32 条命令进程区间；华为 B2 为 40 次 bash、39 条命令进程区间。它们另一次标为“原生 Tool 报错、无对应进程计时”；E2B B2 有 40 个 bash 请求及 39 条进程记录，但原生 Span 文件缺失，不能借华为结果补定额外请求的原生终态。不能仅凭缺少进程记录推断没有启动进程。SDK 的 `native_status_code=2` 是 OTel ERROR，**不是进程退出码 2**。上表 exit 来自真实命令结束证据，二者分开处理。原始轨迹中的非零退出码仍保留，随后成功生成合格产物；没有删掉失败步骤来提高成功率。

A 及华为 B 的配对使用同一 Attempt 内唯一、精确的 ToolCall ID 及来源绑定，再跨运行按共同 Tape 的 ID 比较。两 B2 的 39 条进程仅按 process marker 中唯一 ToolCall ID 配对，返回码一致，但 E2B 的 native_association=not_observed；这不修复整份原生观测缺失。不用命令哈希或时间近邻猜测。附录列出每次 bash 的动作类别和两云命令时长；完整参数留在原始私有证据中。

### 华为自身耗时分解：模型入口、原生工具与命令三层

以下从原已校验 Snapshot 在 ECS 只读提取：A29 与 B30-2 各自解释本次工作量，不能据不同 Tape 比较 A/B 优劣。工具仅取 `authoritative_tool=true`，按当前 Attempt 的唯一 ToolCall ID 和 operation_id 对照，排除重复 wrapper；包含真实报错 Span，没有删掉失败步骤。历史 B1 的分析原件仍保留，主表已换成 B30。

| 模型入口：各自 Controller 单调时钟 | 华为 A29 | 华为 B30-2 |
| --- | ---: | ---: |
| 真实 HTTP 请求数 | 64 | 61 |
| HTTP handler 累计 / 同域去重覆盖，秒 | 5.359 | 24.415 |
| 最长单次 HTTP handler，秒 | 0.279 | 0.783 |
| 读取请求体累计，秒 | 0.733 | 19.696 |
| JSON 解析累计，秒 | 0.375 | 0.397 |
| 历史/决策匹配累计，秒 | 1.006 | 1.103 |

`model.http_request` 从代理收到请求到 handler 结束，包含接收、解析、匹配、响应发送和收尾；子阶段不能再加到 HTTP 总数上。本次各列 HTTP 区间互不重叠，累计与同域 union 相等。这些是真实入口时间，说明 `zero` 仍有实际成本；请求体读取包含传输、读取与调度等因素，不等于纯网络延迟，不能将 B 的 19.696 秒全部归因于网络。

| 原生 Tool 类型 | A 次数 | A 累计秒 | A 最长秒 | B30 次数 | B30 累计秒 | B30 最长秒 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| bash | 33 | 14.265 | 1.900 | 40 | 12.750 | 2.038 |
| edit_file | 12 | 2.455 | 0.470 | 4 | 0.256 | 0.190 |
| read_file | 26 | 4.591 | 0.567 | 24 | 1.115 | 0.129 |
| send_file_to_user | 1 | 0.200 | 0.200 | 1 | 0.011 | 0.011 |
| skill_tool | 1 | 0.207 | 0.207 | 1 | 0.051 | 0.051 |
| todo | 1 | 0.142 | 0.142 | 4 | 0.317 | 0.125 |
| web_flash | 0 | 不适用 | 不适用 | 1 | 0.013 | 0.013 |
| write_file | 4 | 0.360 | 0.134 | 5 | 0.145 | 0.071 |

| 同一层的范围 | 华为 A29 | 华为 B30-2 |
| --- | ---: | ---: |
| 全部 authoritative Tool 累计，秒 | 22.219 | 14.658 |
| 全部 authoritative Tool 同域 union，秒 | 20.243 | 13.811 |
| 命令直接子进程累计，秒 | 11.802 | 12.258 |

原生 Tool 是 Agent 端完整调用的 wall-clock Span：A 包含访问远端工具服务的过程，B 在 guest 内调用；`bash` Span 还包含命令进程之外的校验、封装等动作。32 / 39 条 cmd 只测直接子进程区间，不能替代全部 78 / 80 个 ToolCall。

工具有并发和嵌套：A 的 26 个 `read_file` 累计 4.591 秒，但同域覆盖只有 2.855 秒；B30 的 24 个读文件累计 1.115 秒、同域覆盖 0.679 秒。union 只合并同一 clock 的重叠区间，原生 SDK 仍使用 wall clock，不成为跨主机单调时间线。模型、原生 Tool 和 cmd 处于不同时间域及嵌套层级，**三层不能相加或从 Runtime 相减来命名“纯 Agent Loop”“网络”或关键路径**。这里只定位已测动作耗时，剩余未归因不伪造。

安全分析原件：[A29 与历史 B1](../runs/jiuwen-pptx-20261027/analysis/runtime-breakdown-release29.json)、[B30-2](../runs/jiuwen-pptx-20261027/analysis/runtime-breakdown-huawei-b-release30.json)，记录输入 Summary/Snapshot SHA、同域区间数量、累计/最长/union 和精确身份核对；无原始提示或工具正文，原 Snapshot 未改。

## 6. CPU、内存和 I/O：按谁消耗来读

CPU 秒是累计消耗的 CPU 时间，可在多核下与墙钟秒不同。内存“采样最大值”可能漏掉采样间峰值；进程/组生命周期峰值也不等于 Runtime 单一阶段峰值。

| 同口径资源 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| ECS Controller CPU 增量，CPU 秒（含共享 MockLLM） | 20.619 | 20.457 | 24.868 | 23.397 |
| ECS Controller 生命周期峰值 RSS，MiB | 585.98 | 536.61 | 739.82 | 541.16 |
| A：ECS Agent 容器采样首末 CPU 增量，CPU 秒 | 131.265 | 129.353 | 不适用 | 不适用 |
| A：ECS Agent 容器采样内存最大值，MiB | 657.09 | 465.38 | 不适用 | 不适用 |
| B：guest Agent 与后代进程组 CPU 增量，CPU 秒 | 不适用 | 不适用 | 76.815 | 120.531 |
| B：guest Agent 与后代进程组生命周期内存峰值，MiB | 不适用 | 不适用 | 831.54 | 945.68 |

Controller 是执行器/MockLLM 所在的共享进程；其边界采样包含本轮服务工作，不是独立测得的模型成本。A Agent 容器的读数不包含远端 Sandbox 工具；B guest cgroup 包含 Agent 及后代工具。因此 A 容器与 B 进程组不能直接当同一范围比较。各层内存也不能简单求和。

华为 A / E2B A 的 Agent 容器分别有 153 / 168 个采样；华为 B 的 guest 进程组是 4 个生命周期边界采样，不能画成连续 CPU 曲线。

| 云端整体 Sandbox 返回值 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| 任务 Sandbox：云端返回内存最大值，MiB | 7737.00 | 534.99 | 7788.00 | 834.58 |
| 任务 Sandbox：云端返回 CPU 百分比最大值 | 13.87% | 14.06% | 25.82% | 26.98% |
| 任务 Sandbox：返回值时间状态 | unverified | not_collected / verified | unverified | not_collected / verified |

这些云端数字只表示平台返回值。华为的采样时间语义未核实，内存值也没有证明与 E2B 的 cache/used 定义相同，不能把 7,737 MiB 对 535 MiB 解读为 PPT 工作负载内存相差十几倍。本轮 CPU/内存分析优先用上表同范围的 ECS 或 guest/cgroup 数据；云端口径缺口不否定已经量出的任务时延。

E2B A 返回了 703 条带值记录，但有 666 条相同时间戳和值的重复回执，不能声称取得了 703 个独立采样点。E2B B2 的 1,176 条带值回执中也有 1,128 条相同时间戳和值的重复。两次 E2B 评分 Sandbox 的查询都没有返回资源读数，应记“未采集”，不是零用量。华为评分 Sandbox 虽有返回值，仍有时间语义未核实的限制。

华为 B2 guest cgroup 还返回块设备 I/O：`254:0` 的读取计数从 167,284,736 到 429,449,216 字节，写入计数为 0；另一个设备也有独立回执。它们是进程组设备计数，不能把可能重叠的设备求和，也不能把写计数 0 当成没有写 PPT（页缓存等会影响口径）。本轮没有每 Tool 的 CPU/内存或任务级 Network 字节；`disk_used` 是空间占用，不能冒充读写 I/O 字节。

## 7. 模型、Trace 和观测完整性

| 可核对的观测项 | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| 进程事件写入 / 尝试 | 225 / 225 | 225 / 225 | 275 / 275 | 275 / 275 |
| 进程事件丢失 / 写入失败 | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| 原生 SDK Span 精确集合对账 | PASS | PASS | PASS | FAIL（文件缺失） |
| 同 Snapshot 四项离线投影 | PASS（同源离线） | PASS（同源离线） | PASS（同源离线） | FAIL（四项 exit=1） |
| 本次 Perfetto / Jaeger / Prometheus 实际查询验收 | PASS（本次原 Snapshot） | UNRUN（本次未查询） | UNRUN（本次未查询） | UNRUN（本次未查询） |
| 已声明来源的事件完整性 | 完整 | 完整 | 完整 | 不完整 |
| 全系统观测覆盖率 | 未知 | 未知 | 未知 | 不完整 |

离线交付为同一版本化 Snapshot，以及从它导出的 **Perfetto / Chrome Trace、OTLP Trace、OTel Metrics**。大文件保留 ECS，避免反复复制上百 MB 的重复导出。A 两组及华为 B2 的四投影成功代表同源导出成功；E2B B2 四项均返回 exit=1，已有不完整产物保留，不计投影 PASS。华为 A 的本次原 Snapshot 已另行完成实际 Perfetto SQL、Jaeger 及 Prometheus 导入查询；其余组本次后端仍 UNRUN，不沿用旧样本背书。

华为 A 后端验收：Perfetto 按各自时钟域对账，Controller 777 区间 / 272 flow、native 450 区间 / 449 父子关系；Jaeger 1,547 Span、18,575 项核对通过，v3 起止/时长转换误差 0。Prometheus 对任务、Controller、Agent 三个有值流逐一首发及重复导入，实际读回数量和值一致；毫秒时间精度转换误差小于 1 ms，不能称为纳秒无损。华为 provider 两条资源流时间语义未验证，仍不导成可信时间序列。临时后端、端口已回收，原 Snapshot SHA 不变。[验收依据](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-workload-a29-backends)。

SDK 原生 Span、Controller、guest 进程各有自己的时钟泳道。guest cmd 有单调时钟区间，不意味着跨主机时钟已对齐。`integrity=complete` 是已声明事件来源的计数/关联完整，不是“全系统所有时间和资源都测到了”；后代进程终态、自然 pipe EOF、跨主机对齐以及每 Tool 资源仍是已标注缺口。不得把它们填成零或用伪时间线拼接。

| Workload 差异回执（不是新工具数量） | 华为 A | E2B A | 华为 B | E2B B |
| --- | ---: | ---: | ---: | ---: |
| 成功模型请求，不含缓存重试 | 64 | 64 | 61 | 61 |
| 缓存重试回执 | 0 | 0 | 0 | 0 |
| 工具正文差异回执 | 254 | 361 | 285 | 308 |

历史会在后续请求中再次提交，因此同一次 Tool 正文差异可能被多轮重复记录，不能将这些回执数当作不同 ToolCall 数。提示上下文差异另列类别、位置、长度和摘要，不改写原始工具正文。华为模型 HTTP 入口的真实区间汇总见第 5 节；它不包含跨域推算的 Agent 等待，也不能用 `zero` 把这一栏补为 0。

## 8. 本轮结论及保留的失败

### 九次 zero 尝试：功能和观测分开

| 样本 | 版本 | PPT 功能 | 原生采集 / 离线投影 | 在主表中的用途 |
| --- | --- | --- | --- | --- |
| 华为 A1 | release29 | PASS | 已测范围 PASS | A 对照 |
| E2B A1 | release29 | PASS | 已测范围 PASS | A 对照 |
| [华为 B1](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-1/evidence/acceptance-summary.json) | release29 | PASS | 已测范围 PASS | 历史成功保留，不与 B30 混作同版本样本 |
| [E2B B1](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-1/evidence/acceptance-summary.json) | release29 | FAIL，首轮偏离 | 0 Tool，失败 Trace 保留 | 历史失败分母 |
| 华为 B2 | release30 | PASS | 已测范围 PASS | B30 实测数据 |
| E2B B2 | release30 | PASS | **FAIL：native 下载失败、四投影 exit=1** | B30 阶段/process 实测数据，完整性能观测未验收 |
| [E2B B3](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-3/evidence/acceptance-summary.json) | release30 | PASS | **FAIL：native 下载再次失败、四投影 exit=1** | 同版本同 Tape 复测；保留失败，不替换主表 |
| [E2B B4](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-4/evidence/acceptance-summary.json) | release30 + HTTP/1.1 配置候选 | PASS | **大 Trace 完整；Producer 审计采集超时，四投影仍 FAIL** | 下载修复真实回归；新采集预算问题单列，不替换主表 |
| [E2B B5](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-5/evidence/acceptance-summary.json) | release31 | PASS | **大 Trace、Producer、80 Tool 精确关联及四投影 PASS；global coverage unknown** | 下载及采集预算修复回归，版本不同于 B30，单列 |

选定四组合功能已齐；9 次 zero 尝试是 8 功能 PASS / 1 功能 FAIL，另有两云 A 各 1 次 recorded 功能 PASS，分开统计。E2B B2、B3、B4 三次整体观测 FAIL；B5 修复了这两项已定位采集故障，但不代表全链观测或生产能力全部验收。E2B B2 的 18,489,919 字节原生 Span 文件下载失败，332 个 SDK Span 已 start/end，但 retained exported=0，80 个原生 Tool 的精确关联未知。39 命令 / 275 事件 / drop=0 的实际 process 数据仍保留，不用 B5 或华为 B2 的完整原生数据回填旧缺口。

原 B2 下载失败约 1.95 秒结束，之后小文件仍可下载；它不是后续 B4 的 5 秒采集截止问题。同 release30 / 同 B Tape 的 E2B B3 再次完成 61/61 轮、PPT score=1、停止及清理，但 18,489,969 字节 native 文件下载失败，80 个 native Tool 关联未知、四投影失败。其 Runtime 为 191.772 秒、Attempt 为 258.548 秒；39 条真实 process 与华为 B2 同 ID 返回码一致，不能替代 native 观测。后续同文件交替实验已确认公开 HTTP/1.1 连接配置能够完整下载，B4 也真实通过大文件回收；底层传输实现的具体缺陷未证明。B2 的阶段时间为真实记录，Runtime 华为 108.636 秒、E2B 186.679 秒；cmd 累计 12.258 / 14.921 秒仅表示已保留进程区间，不用于补齐完整 native 性能解释。

### 下载修复后的 B4：大文件已完整，审计收尾仍超时

SDK 公开连接配置改为 HTTP/1.1 后，同 B Tape 的 B4 完成 61/61 模型轮、独立 score=1、停止和清理；18,489,986 字节 native Trace 已完整下载，源与保留文件 SHA 一致。但后续 Producer 审计文件在共享采集窗口的第 5 秒超时，文件未保留，整体观测及四项投影仍判失败。此前 B2/B3 的 `ReadError` 与这次明确的 context deadline 是两个已区分的问题，不能用“大文件已下载”替代完整采集验收。后续 release31 使用既有 CleanupTimeout 的剩余预算，保留私有目录清理时间；实际回归见下节。

B4 Runtime 187.994 秒、Attempt 268.165 秒，39 条真实命令的 ToolCall ID / 返回码与华为 B2 一致。原 PPT 和 15 份小回执已复制本机，[摘要](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-4/evidence/acceptance-summary.json) SHA `838ce3a8fb62e89dca5b2505d23fda5be41d39c67b3299fe0caa986c6699f77a`，[下载时序](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-4/evidence/download-timing-evidence.json)保留每个真实起止。B2、B3、B4 三次整体观测失败均计入记录，主表继续保留 B2 历史对照。

### B5：大 Trace 与 Producer 审计已共同通过真实回归

release31 在 ECS 通过五项发布检查及冻结后，使用原华为 B Tape、image8、4u8g 完成新的 E2B B 回放：61/61 模型轮、80 个权威 Tool 精确关联、39 个命令 / 275 条事件、drop=0；命令逐 ID 对华为 B2 的退出码差异为 0。18,489,897 字节 native Trace 完整，Producer started / ended / exported 均为 332、missing=0，四项同源离线投影通过。实际后端查询仍 UNRUN，全链 coverage unknown；不能用“观测修复通过”代替未验证范围。

| B5 全局阶段 | 秒 |
| --- | ---: |
| Sandbox 创建 | 1.689 |
| 工作区准备 | 2.402 |
| Bridge 准备 | 0.403 |
| Harness 启动 | 44.119 |
| Runtime 执行 | 197.843 |
| 停止与撤权 | 6.496 |
| 独立验证 | 7.202 |
| 清理 | 2.813 |
| 八阶段合计 | 262.967 |
| 未归属差额 | 8.500 |
| **Attempt 总计** | **271.467** |

排队为 19.370 μs，在 Attempt 外；driver 309.804 秒；39 条命令累计 17.834 秒是内部局部区间，不加入上述总计。B5 是 release31 修复验收样本，华为 B2 是 release30，不伪装成同冻结版本的性能实验。

独立 PPT 评分通过：`final.pptx` 和 `大模型发展.pptx` 均 5 页非空，二者同 SHA；3 页 `content.pptx` 及含 2 张空白页的模板仍记不合格。四份原件、17 份小回执已复制本机，停止撤权、两个 Sandbox provider NotFound、运行资源与锁清理确认。

[B5 原始摘要](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-5/evidence/acceptance-summary.json) SHA `048aa9cd1bab6053eea36d89cdf9b6a0fee5fdc1cdb1f1fd746706648744e0bc`；[逐命令对账](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-5/evidence/command-returncode-comparison.json) SHA `765228d32a71050fd56583f7905af023c416d3eaa1e082686640f396bedeb533`；[复制回执](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-5/copy-receipt.json) SHA `d778fd6247769dfd3523d3898e7d5bb5a2899ee3ea40210f94edb3d95260cb08`。[工程及真实回归证据](observability/VALIDATION_AND_REPRODUCE.md#e2b-native-trace-download)保留修复和底层协议仍未知的边界。

### 历史 E2B B1 首轮失败：内核元数据问题已修复并真实通过模型回放

下载故障的[独立真实复现](observability/VALIDATION_AND_REPRODUCE.md#e2b-native-trace-download)已捕获 `httpx.ReadError` / `pyqwest.ReadError`，具体传输根因待确认；不是仅凭文件大小推断失败。该诊断与下面已修复的首请求问题分别留证。

实际首个模型请求在 `/messages/0/content` 被拒。原生 Jiuwen 把 `platform.system()` 与 `platform.release()` 写入系统提示：录制云为 `Linux 6.6.0-72.0.0.76.r1561_118.hce3.x86_64`，E2B 为 `Linux 6.1.158+`。在私有诊断中，仅替换已批准的运行根路径与这一条 OS 行，重建完整 system 摘要与实际拒绝请求完全相等；运行模式附件相同。因此本次根因已确认是尚未纳入 Workload 例外的 Linux 内核元数据，不是 MockLLM 换了工具顺序或模式附件变化。

本次 0 ToolCall、0 命令、0 PPT、独立 score=0；6 个原生 SDK Span 完整，不能称为丢采样。停止/撤权、两个 Sandbox 删除及 provider 不存在确认均成功，cleanup_errors=[]。下列时间只用于失败诊断，**不进入成功性能对照**：

| 失败 Attempt 阶段 | 秒 |
| --- | ---: |
| 任务 Sandbox 创建 | 0.980 |
| 工作区准备 | 2.172 |
| Bridge 准备 | 0.990 |
| Harness 启动 | 51.328 |
| Runtime 执行 | 3.913 |
| 停止与撤权 | 4.401 |
| 独立验证 | 5.028 |
| 清理 | 1.350 |
| Attempt | 77.833 |
| driver | 89.756 |

release30 修复只在显式 Workload、固定 Jiuwen 中/英文 Environment 块放行 Linux 内核版本差异；平台、Shell 及其他文字仍精确，Strict、真实环境、原生提示、命令与旧 Tape 不变。复用 `jiuwen_system_context` 摘要，不创建新变体。**release30 工程检查与冻结 PASS，两 B2 模型 61/61 和独立 PPT 评分通过**；E2B B2 另有下载观测失败，不能把功能成功升级为完整观测成功。旧 B1 失败保留。[原始差异证明](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-1/evidence/first-request-system-difference.json)。

### 成功样本的当前结论

A 两组满足同 Tape 的实际 ToolCall 贯通、命令返回码对账和独立结构验证，可用于本轮同 placement 的单次跨云对照。华为 B2 同样完成上述已测范围；E2B B2 功能通过但 native 缺失，B30 当前只展示已有实测区间，不宣称完整性能证据闭环。当前 A 组华为 Runtime 108.915 秒、E2B 130.566 秒；命令区间累计分别为 11.802 与 19.206 秒。它们展示本次环境下真实测到的差别，但 Runtime 差额还包含转发、文件工具、Agent 与模型入口等因素，不能全部归因于 guest 命令。

当前华为 A29 与 B30-2 的 Runtime 接近 109 秒，并不能说明两种 placement 同样快，因为 A/B 使用不同 Tape。A 两云和华为 B 的主样本各取 1 次，E2B B2/B3/B4 三次功能通过但整体观测失败，B5 修复通过单列；不推导 P95、容量、稳定性或普遍云厂商排名，也不把这一轮称为严格回放或完整观测生产验收。

上述 9 次 zero 与两云 A recorded 均各创建任务与评分两个 Sandbox，逐项确认删除且 provider 查询不存在；Agent、模型监听及运行进程按所属位置清理，无清理错误。回放总计 11 次尝试、10 次功能通过、1 次首请求失败，三次 B 采集失败仍保留。源华为 B record3 的历史观测失败仍保留原状，本轮回放成功没有回写升级历史记录。

## 9. 原始证据入口

B3 的[验收摘要](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-3/evidence/acceptance-summary.json) SHA 为 `af934e9f3e605c49c01b29b7a4bcceba243f99b714996a34089bcff7c4f0d493`；[39 条 process 对账](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-3/evidence/command-returncode-comparison.json)与[复制回执](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-3/copy-receipt.json)单独保留。它未替换下面 B2 主表证据。

每组本地目录包含 PPT 原件、独立评分、源 Tape 元数据、逐 Tool 身份、命令返回码配对、资源边界、云资源不存在证明和 SCP 哈希回执。Tape / Snapshot / Trace 大文件继续保留 ECS。下面链接均指向实际文件；E2B B 同样保留失败终态、观测失败回执和清理证据，不从分母删除。

| 组别 | 验收与时间/资源摘要 | 命令逐 ID 对账 | 本机复制校验 |
| --- | --- | --- | --- |
| 华为 A | [acceptance-summary.json](../runs/jiuwen-pptx-20261027/huawei-a-workload-replay-1/evidence/acceptance-summary.json) | [command-returncode-comparison.json](../runs/jiuwen-pptx-20261027/huawei-a-workload-replay-1/evidence/command-returncode-comparison.json) | [copy-receipt.json](../runs/jiuwen-pptx-20261027/huawei-a-workload-replay-1/copy-receipt.json) |
| E2B A | [acceptance-summary.json](../runs/jiuwen-pptx-20261027/e2b-a-workload-replay-1/evidence/acceptance-summary.json) | [command-returncode-comparison.json](../runs/jiuwen-pptx-20261027/e2b-a-workload-replay-1/evidence/command-returncode-comparison.json) | [copy-receipt.json](../runs/jiuwen-pptx-20261027/e2b-a-workload-replay-1/copy-receipt.json) |
| 华为 B | [acceptance-summary.json](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/evidence/acceptance-summary.json) | [command-returncode-comparison.json](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/evidence/command-returncode-comparison.json) | [copy-receipt.json](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/copy-receipt.json) |
| E2B B | [acceptance-summary.json](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-2/evidence/acceptance-summary.json) | [两 B2 的 process 对账](../runs/jiuwen-pptx-20261027/huawei-b-workload-replay-2/evidence/command-returncode-comparison.json)，native 关联仍缺失 | [copy-receipt.json](../runs/jiuwen-pptx-20261027/e2b-b-workload-replay-2/copy-receipt.json) |

ECS 主样本为 `/opt/aries-artifacts/jiuwen-{huawei|e2b}-{external|embedded}-pptx-workload-replay-{1|2}/`：A 取 1、B 取 2；旧 B1 保留，其中 `snapshot.json`、`perfetto.json`、`otlp.json`、`metrics.json` 的原始 SHA 和大小见每组摘要 `observation.exports`。源 Tape 身份见 `tape.source`，不与当前执行 Attempt 混淆。

## 附录：每次 bash 的动作和命令时间

单位为毫秒；顺序按 Tape 中“模型轮 / 本轮工具序号”（从 1 开始）排列。动作是脱敏分类，未打印命令体。只有同一 ToolCall ID 的进程才能配对；“未采集”不等于 0。返回码采用当前真实进程证据，源/本轮及同 Tape 跨云对账结果见上表。

<details>
<summary>A：共同 64 轮、78 个 ToolCall 的 Tape：展开逐次 bash</summary>

| 轮 / 工具 | 动作 | ToolCall ID | 进程 exit | 华为 ms | E2B ms |
| --- | --- | --- | ---: | ---: | ---: |
| 3 / 1 | 查看目录与查找文件 | `call_00_73itPvoaj1RU5lozbtzg8475` | 2 | 9.565 | 49.609 |
| 4 / 1 | 查看目录与查找文件 | `call_00_tUX2gxu69i77F8WBzUqp7439` | 2 | 2.657 | 5.606 |
| 5 / 1 | 查看目录与查找文件 · SKILL.md · SKILL.md | `call_00_QcmJoZFgX1GxVRSt3DQ74812` | 0 | 375.818 | 793.059 |
| 6 / 1 | 查看目录与查找文件 | `call_00_lWlwj0Ocn4STYF4pYP8d5593` | 0 | 133.530 | 129.690 |
| 7 / 1 | 查看目录与查找文件 | `call_00_NHCj49rCEBZGyOrMNLvb2224` | 0 | 2.566 | 4.443 |
| 10 / 2 | 查看目录与查找文件 | `call_01_ifJq2pLRj7hIUvxPv5J48679` | 0 | 1122.062 | 3026.197 |
| 13 / 1 | 运行 Node 脚本处理演示项目 | `call_00_WvSyRTVPDXePHRUsssnl2124` | 0 | 640.640 | 2440.499 |
| 14 / 1 | 运行 Node 脚本处理演示项目 | `call_00_VB4M1K29FCBkCF4DB3BA4350` | 0 | 64.085 | 123.402 |
| 15 / 1 | 复制项目文件并检查目标 · design-spec-reference.md · design-spec-reference.md | `call_00_8c2kb41qa3hgm0T0i8pF5227` | 0 | 18.768 | 7.164 |
| 18 / 1 | 运行 Node 脚本处理演示项目 | `call_00_har4nmN6RD3NX7oMpdPh5935` | 0 | 136.420 | 248.170 |
| 22 / 1 | 运行 Node 脚本处理演示项目 | `call_00_RqcCYNLN3PmhJpv6kevG7297` | 0 | 64.963 | 107.635 |
| 23 / 1 | 读取文件末尾 · components.js · components.js | `call_00_PUsFRtcuXx7aPuHgV5729435` | 0 | 4.467 | 30.608 |
| 26 / 1 | 查看目录与查找文件 · slides.js · slides.js | `call_00_nP158cJUItRfcwcZn9YI7005` | 2 | 140.978 | 283.203 |
| 27 / 1 | 建立输出目录并检查文件 | `call_00_dV1heOGFm7PMRpWpcq6V6566` | 未知 | 未采集 | 未采集 |
| 28 / 1 | 建立输出目录并检查文件 | `call_00_Fx6nIZeTtoVfxZHKLUfp9812` | 0 | 2.356 | 5.983 |
| 33 / 1 | 查看目录与查找文件 · slides.js · slides.js | `call_00_bxLHRVB2tWdsNLca69dA0850` | 0 | 128.364 | 215.815 |
| 34 / 1 | 运行演示稿定稿脚本 · finalize_deck.py · finalize_deck.py | `call_00_haEonERhQV7ckFsaP3Zc6421` | 0 | 75.439 | 55.573 |
| 35 / 1 | 运行演示稿定稿脚本 · finalize_deck.py · finalize_deck.py, content.pptx, final.pptx | `call_00_EvQSo23Cm9cacLp8n6wX1009` | 0 | 764.348 | 903.793 |
| 36 / 1 | 运行演示稿定稿脚本 · finalize_deck.py · finalize_deck.py, content.pptx, final.pptx | `call_00_96WtFHNUDx0AH2y8XJZt5452` | 0 | 720.103 | 646.968 |
| 39 / 1 | 运行演示稿定稿脚本 · slides.js / finalize_deck.py · slides.js, finalize_deck.py, content.pptx | `call_00_Ea0AaaSdw5kYHZV6HIcD4040` | 0 | 907.432 | 1060.482 |
| 40 / 1 | 运行 Node 脚本处理演示项目 · final.pptx · final.pptx | `call_00_YbbpbZXMFj3bmUwuqz304279` | 0 | 329.598 | 402.405 |
| 41 / 1 | 运行 Python 检查或处理演示文件 · final.pptx · final.pptx | `call_00_W080fWgDz65EgtCCkLLl4800` | 0 | 93.196 | 139.046 |
| 42 / 1 | 运行 PPT 生成或处理脚本 | `call_00_t2pP0wZD2M2la2ls5fTk9085` | 0 | 1.592 | 4.995 |
| 44 / 1 | 运行 Python 检查或处理演示文件 | `call_00_X6IWba4SABXpKqburV0E5125` | 0 | 39.984 | 42.866 |
| 45 / 1 | 运行 Python 检查或处理演示文件 | `call_00_i3eetxEWebaXccFos4gs0263` | 0 | 38.509 | 50.450 |
| 46 / 1 | 运行 PPT 生成或处理脚本 | `call_00_6zs7dEYqgh32naMEoxzk7234` | 0 | 3.935 | 5.721 |
| 47 / 1 | 运行 Python 检查或处理演示文件 | `call_00_JBqrc1RF3WAoIlCPiH993004` | 0 | 41.131 | 49.533 |
| 50 / 1 | 转换 PPT 为 PDF，并提取文本检查 · final.pptx / final.pdf · final.pptx, final.pdf | `call_00_hVClbHVUTYrNs5E12G0k6338` | 0 | 1825.252 | 3461.869 |
| 52 / 1 | 运行演示稿定稿脚本 · slides.js / finalize_deck.py · slides.js, finalize_deck.py, content.pptx | `call_00_jbw9lbgKsOAi6ZVlVUD70374` | 0 | 904.248 | 1025.914 |
| 56 / 1 | 运行演示稿定稿脚本 · slides.js / finalize_deck.py · slides.js, finalize_deck.py, content.pptx | `call_00_TrMWdmLDxFFMaf9IsyF27688` | 0 | 945.384 | 1056.915 |
| 60 / 1 | 运行演示稿定稿脚本 · slides.js / finalize_deck.py · slides.js, finalize_deck.py, content.pptx | `call_00_MeY4cCsB9E8FBPGxn8HV9569` | 0 | 957.487 | 1148.235 |
| 61 / 1 | 运行 Python 检查或处理演示文件 · final.pptx · final.pptx | `call_00_I9HZjczpYxkQQ8XwA6Wu4919` | 0 | 300.994 | 257.605 |
| 62 / 1 | 转换 PPT 为 PDF，并提取文本检查 · final.pdf / final.pptx · final.pdf, final.pptx | `call_00_RljKOhZlSzy0waIT6VoF7479` | 0 | 1005.718 | 1422.385 |

</details>

<details>
<summary>B30：共同 61 轮、80 个 ToolCall 的 Tape，39 条 process marker 对账</summary>

E2B B2 原生 Span 缺失，下列 process 配对不代表完整 native 关联。

| 轮 / 工具 | 动作 | ToolCall ID | 进程 exit | 华为 ms | E2B ms |
| --- | --- | --- | ---: | ---: | ---: |
| 3 / 1 | 运行 PPT 生成或处理脚本 | `call_00_CLXHMD8XwNYPIwTcOvC07886` | 0 | 208.838 | 517.185 |
| 4 / 1 | 检查目录或查找产物 | `call_00_voWVH66HKc4aZZJYtXDr5820` | 1 | 3.645 | 21.555 |
| 5 / 1 | 运行 PPT 生成或处理脚本 | `call_00_ZnXU6vsV4gvKasc1X1nt3240` | 0 | 712.069 | 646.904 |
| 8 / 1 | 运行 PPT 生成或处理脚本 | `call_00_YgMalFMpBqh016NkAfoJ1844` | 0 | 3.930 | 1.811 |
| 9 / 1 | 检查目录或查找产物 | `call_00_WMn8tGVObVXGEz6Zfjmn9942` | 0 | 0.128 | 5.271 |
| 11 / 1 | 转换或渲染演示文档 | `call_00_3LtROSd6ZoX4ndPjaM9K7165` | 0 | 1069.090 | 2674.615 |
| 14 / 2 | 运行 PPT 生成或处理脚本 | `call_01_AzB47q3ehCAsGuxjEddb6676` | 1 | 177.211 | 341.756 |
| 17 / 1 | 运行 PPT 生成或处理脚本 | `call_00_jR7VTDMEWm6fN0DLS8dp5133` | 0 | 137.974 | 190.828 |
| 18 / 1 | 运行 PPT 生成或处理脚本 | `call_00_i9k4ina1OtR9IApaSfGK5871` | 0 | 130.701 | 225.636 |
| 20 / 1 | 执行 JavaScript 脚本 | `call_00_v4md3Seq8VUnyyGBfVMb0036` | 0 | 141.176 | 210.607 |
| 21 / 1 | 运行 PPT 生成或处理脚本 | `call_00_u29jLE2xrkEA7cM8UmQK6790` | 1 | 730.690 | 580.677 |
| 22 / 1 | 运行 PPT 生成或处理脚本 | `call_00_IfZpu4BIXIJ4i5rFqoPp1181` | 0 | 708.828 | 564.497 |
| 24 / 1 | 运行 PPT 生成或处理脚本 | `call_00_TDzpdAXKTL3fB4JkMdYV8124` | 未知 | 未采集 | 未采集 |
| 25 / 1 | 运行 PPT 生成或处理脚本 | `call_00_LrQXGoidmwBtl7JdmkY52375` | 0 | 882.319 | 866.023 |
| 26 / 2 | 运行 PPT 生成或处理脚本 | `call_01_xn45i8ACskBAHPXakgTs4577` | 1 | 194.183 | 146.261 |
| 27 / 1 | 运行 PPT 生成或处理脚本 | `call_00_AsvO3PW1hF9lst16RO5c8076` | 0 | 133.460 | 142.674 |
| 29 / 1 | 运行 PPT 生成或处理脚本 | `call_00_4TN230NHvhUDnMG2LqCa5229` | 0 | 135.792 | 156.928 |
| 30 / 1 | 运行 PPT 生成或处理脚本 | `call_00_YaBJo5iZhCSnAVESKzUh1612` | 0 | 1.300 | 1.077 |
| 32 / 1 | 执行 Shell 命令 | `call_00_KwJiEXrq07UiUtsxDlb77791` | 0 | 3.115 | 5.253 |
| 33 / 1 | 运行 PPT 生成或处理脚本 | `call_00_DYuDH8Q69nmlmCtecqnq5403` | 0 | 1.426 | 3.527 |
| 35 / 1 | 运行 PPT 生成或处理脚本 | `call_00_wVXxlPO2gW2JZzszqs8q5300` | 0 | 3.075 | 4.448 |
| 38 / 1 | 执行 Shell 命令 | `call_00_7QdY8byTdUs2fxDTplIp7563` | 0 | 3.279 | 5.311 |
| 39 / 1 | 运行 PPT 生成或处理脚本 | `call_00_PhsNoYLdYouWGkFSEnlQ2433` | 0 | 160.188 | 144.047 |
| 40 / 1 | 运行 PPT 生成或处理脚本 | `call_00_oizxPyss844D3SuNha1o7631` | 0 | 775.807 | 649.173 |
| 41 / 1 | 运行 PPT 生成或处理脚本 | `call_00_Hh5VJEwxOunPvd4Oon8Z3804` | 1 | 195.506 | 136.088 |
| 42 / 1 | 读取或检查文件 | `call_00_MBuwACjnHohQ8rUjXNpY7008` | 0 | 2.637 | 4.791 |
| 43 / 1 | 运行 PPT 生成或处理脚本 | `call_00_qVgmIIllzvrzkN94OHxG0482` | 0 | 41.795 | 39.483 |
| 44 / 1 | 运行 PPT 生成或处理脚本 | `call_00_1TeWO8UsYHYoaLzxt1WM4617` | 0 | 39.439 | 38.164 |
| 46 / 1 | 运行 PPT 生成或处理脚本 | `call_00_I2bYLAQHnKNvgOCZUYp45596` | 0 | 813.760 | 740.073 |
| 47 / 1 | 运行 PPT 生成或处理脚本 | `call_00_96rLJWWUEKNAKZbbmbwJ8904` | 1 | 190.052 | 148.173 |
| 48 / 1 | 运行 PPT 生成或处理脚本 | `call_00_PjFvSRRWKtVm0nrMqpbb8182` | 0 | 38.694 | 36.572 |
| 50 / 1 | 运行 PPT 生成或处理脚本 | `call_00_CWbPN29LggdjHPwgj20u5030` | 0 | 862.317 | 730.202 |
| 51 / 1 | 运行 PPT 生成或处理脚本 | `call_00_XmFHZVcwbYAQnUirAjQv3877` | 0 | 196.098 | 151.654 |
| 53 / 1 | 运行 PPT 生成或处理脚本 | `call_00_fdygoJqO07eQPimAkIrn0390` | 0 | 1154.237 | 970.401 |
| 54 / 1 | 运行 PPT 生成或处理脚本 | `call_00_des6RMnlvIJpjxu7ZJhq7049` | 0 | 198.279 | 133.952 |
| 55 / 1 | 转换或渲染演示文档 | `call_00_JFex3rtyvnB6guiWtZzc9758` | 0 | 2030.360 | 3438.696 |
| 56 / 1 | 读取或检查文件 | `call_00_ub36yqSavpXV979lnpuT0882` | 0 | 17.887 | 23.319 |
| 57 / 1 | 执行 Shell 命令 | `call_00_L6ayvu2iJyMgO8KYADTP6798` | 0 | 20.134 | 36.896 |
| 58 / 1 | 运行 PPT 生成或处理脚本 | `call_00_oIqEjVxlHollWYLwoDX71245` | 0 | 131.995 | 181.616 |
| 59 / 2 | 运行 PPT 生成或处理脚本 | `call_01_dof0AYhTuGHbtfqMIySG6211` | 0 | 6.287 | 4.761 |

</details>
