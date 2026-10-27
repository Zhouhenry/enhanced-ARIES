# ARIES 性能报告

**用户 PPT 任务优先按非 Strict 完成端到端交付。四种组合已有结构合格的原件，但观测验收分别列出，不以文件生成掩盖观测失败。** 两云 Sandbox 统一 4 vCPU / 8192 MiB，使用 image8、原生技能及独立五页非空评分。

| 组合及实际运行 | 本机合格原件 | 本次观测结论 |
| --- | --- | --- |
| E2B A，release27 `record-3` | `e2b-a-record-3/.../output/final.pptx`，5 页非空 | 89 个精确 ToolCall，563 个 sampled SDK Span，50 命令/348 事件；四离线投影通过，后端本次 UNRUN |
| E2B B，release25 `record-2` | `e2b-b-record-2/.../output/content.pptx`，5 页非空；其 `final.pptx` 是 7 页，不合格 | 原生 Trace 下载失败，四离线投影失败；历史失败保留 |
| 华为 A，release27 `record-3` | `huawei-a-record-3/.../output/final.pptx`，5 页非空 | 78 个精确 ToolCall，450 个 sampled SDK Span，32 命令/225 事件；四投影及 Perfetto/Jaeger/Prometheus 实际查询通过 |
| 华为 B，release28 `record-4` | `huawei-b-record-4/.../output/final.pptx` 与 `大模型发展脉络与趋势.pptx`，同一五页非空成品的两个原文件 | 73 个精确 ToolCall，264 个 sampled SDK Span，36 命令/252 事件；四离线投影通过，后端本次 UNRUN；旧 `record-3` 观测失败保留 |

所有原件在本机 `runs/jiuwen-pptx-20261027/`，包括不合格中间稿和模板，逐项 SHA 校验。[逐文件结果、八阶段、资源与缺口](jiuwen-pptx-capacity-20261027.md)保存详细证据；[E2B A 最新完整录制](jiuwen-pptx-capacity-20261027.md#release27-e2b-appt完整录制与离线观测通过)已通过，原 57 轮收尾失败另存。以上观测通过范围的 global coverage 仍 unknown，华为评分出网仍 assumed，不升级为全隔离验证的 Goodput。

旧 E2B A 的 Tape 收尾失败、E2B B 的七页 final 与观测缺失、旧华为 A 的 JSON 拒绝均保留在[历史结果](jiuwen-pptx-capacity-20261027.md#release25-真实复测与后续修复)。[华为 B 新复测](jiuwen-pptx-capacity-20261027.md#huawei-b-record4)完成公开路由端口修复的真实闭环，旧失败不改判。这些是独立真实模型录制，仍未完成 PPT 同 Tape 跨云 MockLLM 对照；三组 release27 和新 B `record-4` 均未触发子 Agent/JSON，不能用功能通过证明 JSON 子轨迹云端通过。

**最新 Jiuwen 命令进程计时：[release23 双云 A/B 与后端验收](jiuwen-release23-cloud-validation-20261027.md)已完成。** 861 文件冻结组合通过五门禁，四次独立真实模型任务各原评分 12/12；15 个命令进程、105 条事件精确关联并完整收尾，8 个 Sandbox 独立确认删除。报告列出九阶段、逐命令墙钟时间/首输出/退出码与 ToolCall、分对象 CPU/内存；四组各自冻结快照的 Perfetto/Jaeger/Prometheus 实际查询均限定 PASS。[每段时间的定义与请求时间的区别](jiuwen-process-observation-20261027.md)单独说明。整体 integrity=complete / coverage=unknown，华为评分出网仍 assumed；独立模型轨迹不是同 Tape 对照，不作云性能排名，也不新增六例 Strict 成功数。

**历史 Jiuwen 结果：[release21 双云 A/B 验收](jiuwen-release21-cloud-validation-20261027.md)。四组真实 DeepSeek 解题均原 12/12、完整 Tape，8 个 Sandbox 全部回收；两组 A 各 8 个 ToolCall、11 次 Provider 调用均精确关联原生→Bridge→worker。** 该历史版本未采集命令进程生命周期。报告统一列出九阶段及总时间关系、分对象 CPU/内存/块 I/O、缺测与单位边界。E2B A 的同一快照已通过 Perfetto、Jaeger、Prometheus 实际查询，其他三组未借用其后端结论。四份独立录制属于功能验收，不计同 Tape 性能对照，整体 coverage 仍 unknown。

[工程修复与失败记录](jiuwen-native-runtime-validation-20261027.md)保留 image5 语义测试、真实模型错误负例、生产 bridge/ 目录修复及 release21 五门禁。以下 release18/19/21 是历史记录，不能把当时的 ToolCall unavailable、进程未采集或后端 UNRUN 当成当前 release23 状态。


**Strict 跨云通过：3/6；另外3/6已有 Workload 跨云功能对照。DB、SQLite、C++ 的 Workload 成功不计入 Strict。** [DB](workload-db-performance-20261026.md)、[SQLite](workload-sqlite-performance-20261026.md)、[C++](workload-cpp-performance-20261026.md)分别复用各自同一Tape完成E2B/华为对照，原评分分别7/7、3/3、6/6。C++整体观测仍incomplete，其他成功样本也未证明所有内部环节均覆盖，详见下方观测状态说明。[新的失败](remaining-strict-cases-20261026.md)和[SQLite v4失败](sqlite-v4-performance-20261026.md)单列保留；不以功能评分掩盖Strict或观测问题。

大文本同Tape跨云结果可直接打开[离线 Dashboard](dashboard/large-scale-text-editing/index.html)：E2B两次、华为一次，含八阶段、逐Tool、模型/SSE、双Trace和分对象CPU/内存。资源采样最大值与未知/unsupported分开表达，华为时间戳冲突保留；[来源与边界](dashboard/large-scale-text-editing/SOURCES.md)。


新增[release14采集修复真实回归](native-tool-retention-performance-20261026.md)：大文本同Tape Strict 23/23、原5/5、64条原生工具记录完整，阶段与20次exec逐项归档。旧样本及跨云用例计数不变。

另列 [JiuwenSwarm 接入验收与性能](jiuwen-cloud-validation-20261027.md#release18-native-audit-cloud)：**release18五门禁冻结后，E2B record7与华为record2均完成原生真实模型解题，独立Verifier各12/12、完整Tape分别9/7轮，Stop/revoke及各两个Sandbox独立删除确认。** 两轮决策和工具工作量不同，不计为同Tape性能对照，也不改变六个主用例统计。sampled SDK started/ended/exported分别44/44/44与35/35/35、缺失/额外0；仅此专项integrity/coverage complete，global coverage仍unknown。四出口投影通过，本轮未借用record5的TP/Jaeger/Prom后端结论；旧后端PASS与首轮Metrics FAIL均保留。控制器、cgroup边界与provider资源归属分别报告，生命周期高水位不是Runtime峰值。E2B Strict首请求偏离与旧失败保留；校验模式讨论延后，固定提示变体暂停，尚未宣称生产化完成。

另列 [Jiuwen A两云真实验收](jiuwen-external-cloud-validation-20261027.md)：release19下E2B/Huawei均原12/12、5轮完整Tape及回收确认；各4次真实Bridge调用，原生ToolCall关联仍4/4 unavailable。报告单列八阶段、漏入阶段字段的Bridge注册时间、SDK审计与分对象资源来源。两份独立真实轨迹不计同Tape性能对照，不改变六例统计。

## 如何理解观测完整性

**目前没有一个用例已证明“所有内部环节和全部资源指标都覆盖”。但不能因此把所有运行写成“工具数据丢失”。** 功能评分、工具执行对账、已采集数据完整性和全链覆盖是四个独立结论。

| 报告状态 | 实际含义 | 当前例子 |
| --- | --- | --- |
| `integrity=complete / coverage=unknown` | 已声明的来源通过完整性检查；尚未证明所有内部边界均有观测。已有精确工具计量可在其范围内使用 | fix-git、async 的成功 Strict；大文本、DB 和 SQLite 的成功对照仍须看各自声明范围 |
| `incomplete`，但实际同步工具终态完整 | 存在明确缺失的来源，不能给整体完整结论；不等于实际工具终态丢失 | C++ Workload 的34个同步exec终态全部对账；未触达后台协议，但其观测文件未采集 |
| `incomplete`，且实际数据确有缺失 | 只能展示保留下来的证据，不能作为完整任务工具覆盖 | DB record3 的原生计时collector超限；SQLite v4失败运行缺原生transcript和后台终态 |
| `unsupported / not_collected / 未对齐` | 指标或时钟关系没有取得证据；不能填0、猜时间或合成跨域因果 | 任务级I/O/Network归属；未对齐的原生后台区间；部分资源采样的时间戳语义 |

CPU/内存已有采样的，只能报告采样值及采样最大值，不能保证抓到真实瞬时峰值或精确归属于单Tool。大文本 Dashboard 的三次运行已有按对象展示的CPU/内存；这不能外推为所有历史任务均已补齐。同快照 Perfetto/OTLP 对账通过，证明已有数据投影一致，不证明来源无遗漏。阶段定义见[统一阶段说明](performance-stages.md)，各次采集事实以链接的专项报告为准。


## 跨云用例交付进度

**六个选定 Agent 主用例的 Strict 跨云进度为3/6：fix-git、cancel-async-tasks 两个原始任务，以及大文本 terminal-result-v2 独立变体已完成同版本、同 Tape 的 E2B / 华为云功能对照。原始任务本身仍为2/6，不能把变体成功回填为原生成功。** 下表按不同题目计数，不把录制、回放、工程检查或多次重复算成新用例。Strict列要求同一Tape、同一placement、两边完整Strict执行及原独立评分通过；Workload列另要求固定决策、真实工具终态与原评分通过，正文差异留证。性能表保留每次实际版本、八阶段和逐工具耗时，不借其他运行填数。

**Workload 对照：** [DB](workload-db-performance-20261026.md)各12轮、11次真实工具、原评分7/7；[SQLite v3](workload-sqlite-performance-20261026.md)各41轮、38次exec与2次wait、原评分3/3；[C++ v2](workload-cpp-performance-20261026.md)各35轮、34次exec、原评分6/6。三组均固定各自Tape的决策与参数，实际终态对账、独立评分和删除确认通过，正文差异留证。其中C++的后台协议未触达且producer文件未采集，整体观测incomplete，不能宣称全链观测通过。三题是工作量功能成功，不是Strict成功或新的自主解题成绩。

六个题目已各有至少一种模式的跨云功能对照；这只表示题目覆盖，不能称“Strict 6/6”、无偏Agent成功率或全观测闭环。C++本次E2B按3600秒存活期实际完成，全程288.328秒；旧4200秒创建被拒绝仅证明项目配置上限，不能推断任务实际需要超过一小时。

每个工作负载版本只需一份选定的合格真实 Tape，各云和重复实验共同复用；重复回放继续执行真实工具和原独立评分。录制通过与可复现资格分开，不能为不同云各录一份不同决策轨迹后直接比较；详见[录制复用规则](design/reproducible-agent-workloads.md#1-先区分两个测量问题)。

| 选定 Agent 用例 | 已有真实原评分 | Strict 跨云对照 | Workload 跨云对照 | 当前结论 |
| --- | --- | --- | --- | --- |
| fix-git | 两云 2/2 | 同版、同 Tape 通过 | 未另跑 | 原任务完成；[阶段及工具](tb-crosscloud-performance-20261026.md) |
| cancel-async-tasks | 两云 6/6 | 同版、同 Tape 通过 | 未另跑 | 原任务完成；欠费中断另保留 |
| db-wal-recovery | 原 Tape 两云 Workload 7/7 | E2B通过；华为目录元数据偏离；新变体录制失败另存 | 同 Tape两云通过，11工具终态一致 | 固定工作量完成；[新对照](workload-db-performance-20261026.md)，不计Strict成功 |
| large-scale-text-editing | v2 两云 5/5 | v2同Tape两次E2B、一次华为通过 | 未另跑 | 独立变体完成；[Dashboard](dashboard/large-scale-text-editing/index.html)，原生失败另存 |
| sqlite-with-gcov | v3 两云 Workload 3/3 | v3 E2B因安装日志mtime偏离；v4新录制2/3、Tape不完整，未回放 | 同Tape两云通过，38exec+2wait终态一致 | 独立变体固定工作量完成；[新对照与shell环境适配](workload-sqlite-performance-20261026.md) |
| custom-memory-heap-crash | v2同Tape两云Workload均6/6；新E2B Strict为5/6 | E2B3600秒创建成功，第6请求因GDB地址差异失败；华为旧顺序差异保留 | 同Tape两云功能通过，各34终态一致；整体观测incomplete | E2B全程288.328秒；[工作量对照](workload-cpp-performance-20261026.md)、[Strict失败与时限](remaining-strict-cases-20261026.md) |

上表不隐藏[六题历史失败与纠错复核](tb-six-performance-20261026.md)。另有 **1 个基础编码工作负载 `merge-ranges-portable-v1`** 已完成 E2B A／华为 A 同 Tape 对照，见[阶段与逐工具耗时](#p0-mainline-l1)；多次回归仍只算一个用例。两云 B 模式的现有 Tape 不同，不计入同工作量跨云对照。现有单次对照没有统计排名资格，华为评分出网假设继续保留。

**最新增量（2026-10-26）：** 大文本v2同一Tape的两次E2B与一次华为Strict回放均原5/5，各23轮、20/20 exec精确、2job/2wait通过且删除确认；[三次实际耗时](repro-variants-performance-20261026.md#text-v2-huawei-replay-1)已对齐，华为评分长尾为原APT获取约486秒，与Agent Runtime分开。SQLite v3 [首次录制](repro-variants-performance-20261026.md#sqlite-v3-e2b-record-1)原3/3、41轮完整Tape、38exec+2wait通过并删除确认，同Tape [Strict自回放已FAIL](repro-variants-performance-20261026.md#sqlite-v3-e2b-replay-1)：turn24/request25因make install日志被tail读回的3处mtime偏离；22次exec精确、2次wait已执行，原评分0/3，清理确认。fix/async [原题双云证据](tb-crosscloud-performance-20261026.md)、DB华为元数据失败、async欠费中断及C++回放失败均保留；没有放宽Strict或更改历史分母。

华为 C++ 原生任务的首次评分依赖失败、新录制通过、随后 Strict 偏离分别保留在[专项归档](tb-huawei-cpp-performance-20261026.md)。下文19目录、18条Attempt、87次PASS及release4结论均为此前封存样本；新增结果不回填历史分母。E2B旧4200秒创建请求被1小时项目上限拒绝；新3600秒实验已实际完成C++ Workload全流程。六题的Strict与全链观测闭环仍未完成，P0仍PARTIAL；R05暂停、R13暂缓，P1/P2完整保留。

独立变体的 release8 已在 ECS 通过全部五项工程检查，host 拒绝身份与原生持久化修复已合并；旧失败保留。随后两次真实运行又暴露了动态元数据与 ExecStream 收尾竞态，不能把工程通过算作任务闭环。下表保留四题逐版本的历史尝试，均与原生原题分开统计；最新去重交付状态以上表为准。

| 独立变体 | 该版本实际结果 | 后续结果与边界 |
| --- | --- | --- |
| DB `metadata-portable-v1` | 原评分 5/7，68 轮 Tape 完整；两次 host 策略拒绝缺身份，整体 FAIL、清理确认 | host/node身份及SSH收尾修复已合并；record2因模型SSE失败、原0/7且另有1次preflight缺身份，见[新录制归档](remaining-strict-cases-20261026.md)。原题旧Tape的Workload双云通过另列 |
| 大文本 `terminal-result-v1` | 原评分 5/5，但模型生成重复 JSON 键，Tape 不完整；整体 FAIL、清理确认 | 保持严格 JSON 校验；v2结果另列，原失败保留 |
| 大文本 `terminal-result-v2` | release10真实录制及同Tape两次E2B、一次华为Strict回放均原5/5，各23轮/20exec/2job/2wait通过；删除确认 | 同Tape跨云闭环通过；[三次八阶段 / 工具对比](repro-variants-performance-20261026.md#text-v2-huawei-replay-1)，不据单组样本给云排名，coverage unknown及华为评分出网边界保留 |
| SQLite `terminal-result-v1` | 原评分 3/3、37 轮 Tape 完整；归档截断丢失三个小字段，观测验收 FAIL、清理确认 | release8 同 Tape 真实回放在第六请求失败：运行中 `/tmp` 时间变化；未到达后台 wait。保留原失败及Tape，v2结果另列 |
| SQLite `terminal-result-v2` | release10新E2B录制原3/3、35轮完整Tape、36/36 exec精确，1个真实后台作业和1次wait映射通过 | 同Tape E2B回放第24请求失败：make install自动输出3个安装mtime；已完成24exec+1wait，原0/3、删除确认。旧截断字段修复已真实覆盖；不改Tape，公开日志规则补强待验 |
| SQLite `terminal-result-v3` | [新E2B录制](repro-variants-performance-20261026.md#sqlite-v3-e2b-record-1)原3/3、41轮完整Tape、38/38 exec及2job/2wait通过，删除确认 | 同Tape [Strict自回放FAIL](repro-variants-performance-20261026.md#sqlite-v3-e2b-replay-1)：turn24/request25日志回读mtime偏离，22exact exec+2wait、原0/3、清理确认；不计Strict跨云PASS。后续[同Tape Workload双云已通过](workload-sqlite-performance-20261026.md)，单独计数 |
| C++ `metadata-portable-v1` | 华为录制原评分 6/6、46 轮、42 次 exec；同 Tape 回放首工具后的根目录日期偏离，原评分 5/6，两次清理确认 | 补齐根目录初始值后录制2原6/6、63/64 exec精确，整体FAIL。辅助检查成功被自身收尾取消误报SSH255的竞态已复现修复；它与首个native调用之间缺直接身份边，旧unknown保持 |
| C++ `terminal-result-v2` | release10新华为录制原6/6、35轮完整Tape、34/34 exec精确；同Tape回放前5轮/5exec精确，第6请求Strict偏离、原5/6，两次清理确认 | gdb命令内部合流后的两行线程库提示位置变化，Strict拒绝；不重排输出或放宽匹配。两次均未触达后台协议，全链仍incomplete |
| C++ `terminal-result-v3` | 华为真实录制原5/6、27exact exec；第28请求上游HTTP 402、Tape不完整，Strict未执行 | 原1800+1800秒及4200秒TTL保持；未耗尽预算，停止撤权及删除确认；[失败原件及双Trace](remaining-strict-cases-20261026.md)保留，不猜测未记录的402正文 |

大文本和 SQLite 的华为模板均已就绪；v2只新增公开元数据/终态等待说明，仍使用 terminal-result-v1 原生协议和已发布镜像。两次新失败的 Sandbox 均已确认删除。定向工程检查、环境准备与真实 Agent 成功次数分别统计。

新增材料：[独立变体真实运行及八阶段](repro-variants-performance-20261026.md) · [大文本 / SQLite 华为准备与失败修复](tb-huawei-variant-preparation-20261026.md)。

### 三个跨云主用例的资源规格对齐

`fix-git`、`cancel-async-tasks`、`db-wal-recovery` 的原题均要求 **1 核 CPU / 2048 MiB 内存**。E2B 构建显式请求相同规格；三份既有 guest 探测均看到 1 个 CPU、`MemTotal=2030620 KiB`。华为创建请求同为 `cpuCount=1`、`memoryMB=2048`，构建回执及模板内对应 `buildID` 均回显相同值，因此当前的 1U2G 是显式选择，没有发现请求被忽略的证据。

| 对齐项 | E2B 三题已有证据 | 华为三题已有证据 | 能得出的结论 |
| --- | --- | --- | --- |
| CPU / 内存 | 构建请求 1 / 2048；guest 可见 1 CPU、2030620 KiB | 创建请求及两种控制面回执均为 1 / 2048 | 配置规格一致；guest 可见值与控制面配置口径分别保留 |
| 磁盘 | 原题请求 10240 MiB；guest 文件系统容量分别为 11403038720、11324395520、11623280640 字节 | 构建及模板回执 `diskSizeMB=30720` | 保留容量差异提示；文件系统容量与配置值口径不同，不据此阻断跨云对比 |
| 强制配额与硬件性能 | binding 的 `storage_mb=null`；guest 探测明确不证明每任务强制配额 | 准备证据的运行时配额验证为 UNRUN | 配置相同不等于物理 CPU 性能、共享程度或全部强制配额相同 |

**跨云对比的资源条件按用户确认的口径执行：Sandbox CPU、内存配置规格一致即可，磁盘等平台差异如实提示，不作为对比阻断条件或样本失格理由。** 性能结果反映该规格下各平台的实际表现；对磁盘相关用例可结合容量等已知差异解释，不要求所有底层硬件一致。上述资源核查本身不新增任务成功或性能对照结果；同 Tape、真实工具、独立评分及清理的原验收要求继续保留。

证据：本地 [fix-git 华为回执](../.cache/tb-huawei-three-1/receipts/tb-huawei-fix-git-template-2/build.json)、[async 华为回执](../.cache/tb-huawei-three-1/receipts/tb-huawei-cancel-async-tasks-template-1/build.json)、[DB 华为回执](../.cache/tb-huawei-three-1/receipts/tb-huawei-db-wal-recovery-template-1/build.json)，三文件的构建 CPU/内存字段在第 7/15 行、创建请求在第 42/44 行、模板内对应构建在第 72/76 行；[三题准备汇总](../.cache/tb-huawei-three-1/receipts/tb-huawei-three-preparation-1/summary.json)保留完整 SHA。ECS `/opt/aries-artifacts/tb21-e2b-{task}-prepare-1/preparation/` 下 `tb2-e2b-prepare.py:179` 是 E2B 显式资源请求，`build.json`／`binding.json` 保留配置，`guest-check.stdout.json:1` 保留实际探测；原题 `/opt/aries-agent-bench/.cache/terminal-bench-2-1/tasks/{task}/task.toml:30` 起记录 CPU、内存与存储。E2B `build.json.template_config` 是脚本记录的请求值，不能写成独立控制面回执。

原封存统计按整体PASS口径归档 **87次成功且独立验证通过的运行**：此前80次、六例批次7次；本批新增已确认成功6次（华为fix-git/async/C++、E2B fix-git/async/DB），按增量单列。另有2次新录制原评分通过、Tape完整，但私有验收器字段错留下FAIL；修正后新增完整旁路复核通过，原FAIL不改写，也不增加Attempt。运行次数不是不同任务数或自主模型成功率；async首批欠费中断不计成功，新批完整通过单列。

**主线历史入口：** [阶段报告](stage-report-20261025.md) → [六例封存19目录、八阶段与229条exec](tb-six-performance-20261026.md)。该批18条Attempt中17次执行Agent、12次原评分通过；3/6题已有同Tape完整Strict回放。大文本/SQLite原回放的动态日期失败保留，E2B C++时限阻断与后续华为评分依赖失败分别统计。该批新6个Sandbox均确认删除，安全汇总SHA `e22aa1f3ba19e2f9a13e04bad4773fd71d2552253d9c8b86c3e1c3bbbdf4f737`；[已批准的变体设计](design/reproducible-agent-workloads.md)与真实验收结果分开。

**此前任务执行进展：** E2B B原CLI取消故障验收通过：计划2个Attempt，实际1个canceled、1个not_started，61项原件复核及双时钟Perfetto导入通过，1个Sandbox确认删除；没有执行独立评分，成功任务总数仍为78。R01同Tape两个完整A任务并发、当前568文件B baseline/L0四对8/8及此前565文件四格L1回归均已通过。真实工具、独立Verifier和停止/撤权门保持不变。**P0全链尚未完成**；E2B B／华为 B 的首输出与直接子进程回调均已实测通过，华为评分环境出网限制仍为显式假设。

第一次阅读：[阶段说明](performance-stages.md) → [统一阶段定义](#阶段与指标定义) → [A模式同Tape并发](#r01-native-concurrent) → [当前 B 观测开销实测](#r15-b-native-paired) → [E2B B 首输出与退出分段](#e2b-b-first-output) → [华为 B 首输出与退出分段](#huawei-b-first-output) → [上一版工具细分](#huawei-b-native-process) → [当前四对观测成本](#r15-cleanup-paired) → [四组合阶段与工具耗时](#p0-mainline-l1)。

历史对照：[R01录制/回放](#r01-current-agent) · [release3回归](#observability-p0) · [旧R15四对](#r15-current-paired) · [旧R15六次](#r15-观测扰动) · [portable-v1跨云](#portable-crosscloud) · [逐工具旧基线](#工具到底耗时多久)。
失败与可靠性：[当前B原CLI取消与未启动账本](#r04-cli-cancel) · [B直接Runner取消与评分前停止](#b-native-cancel) · [修复前华为A失败](#huawei-current-failure) · [停止隔离/观测挂起](#current-fault-validation) · [工程验证](#增量工程验证不计入任务分母)。完整证据见[复现档案](observability/VALIDATION_AND_REPRODUCE.md)；测量边界见[契约](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md)。

<a id="r01-real-multicall"></a>

## 真实模型同轮多工具：自主修复、独立评分与严格回放

录制和回放各7轮模型/758帧/7次真实exec，原独立Verifier均12/12。首轮两次独立读取；中间两次公开测试 exit1 都是原真实调试过程，最终复测 exit0 后独立评分通过。Strict整历史、工具结果与候选SHA一致，4个Sandbox确认删除；旧受控协议score0不改。

| 阶段 | 真实模型录制（秒） | 同Tape严格回放（秒） |
| --- | ---: | ---: |
| 排队 | 0.000011 | 0.000011 |
| 任务 Sandbox 创建 | 1.180658 | 0.609765 |
| 工作区准备 | 2.876132 | 1.645454 |
| Harness 启动 | 7.340088 | 7.384710 |
| Runtime 执行 | 20.778185 | 19.852145 |
| 停止与撤权 | 5.555628 | 5.595328 |
| 独立验证 | 6.197812 | 5.328375 |
| 清理 | 1.889842 | 1.676308 |

原Attempt观测窗口分别 46.591625s / 42.971547s，不用阶段求和替代。录制只有一次，不能由窗口倒数估计容量或p99。

| 调用 | 模型轮 | 退出码（两次） | 录制 SDK 等待（秒） | 回放 SDK 等待（秒） | 录制 guest 整体（秒） | 回放 guest 整体（秒） |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| T1 | 1 | 0 | 0.314011 | 0.295385 | 0.045266 | 0.035723 |
| T2 | 1 | 0 | 0.302560 | 0.298959 | 0.028960 | 0.038595 |
| T3 | 2 | 1 | 0.552960 | 0.534115 | 0.286263 | 0.256791 |
| T4 | 3 | 0 | 0.312264 | 0.302976 | 0.044164 | 0.037767 |
| T5 | 4 | 1 | 0.678279 | 0.471956 | 0.219445 | 0.205673 |
| T6 | 5 | 0 | 0.339937 | 0.329389 | 0.058723 | 0.043560 |
| T7 | 6 | 0 | 1.086233 | 0.472154 | 0.262822 | 0.206428 |

SDK 等待包含通信与控制器路径；guest 整体是受管 wrapper 的本地时长，两者差值不命名为纯网络耗时。首轮两调用SDK等待重叠0.302560s/0.255772s，guest跨域并行未测。两份Perfetto实际导入469/454区间、50/43 flow，起止/时长误差0ns；同快照OTLP分别469/454Span、50/43parent。完整性complete，全局coverage仍unknown；当前样本后端发送/UI未验。[完整版本、SHA与复现](observability/VALIDATION_AND_REPRODUCE.md#r01-real-multicall)。

<a id="r01-multicall-app"></a>

## 同轮多工具的应用级回放：真实账本与双 Trace

这次把此前受控录制的原Tape送入真实应用调度，**不计自主Agent任务成功**。1条计划、1条实际执行并完成、1条独立验证失败、0条验证成功；原Verifier12项中11失败，score0。模型3轮/9帧、三个真实exec严格完成。后续真实模型自主修复成功另见上节，本历史协议样本保持score0。

| 阶段 | 应用级回放耗时（秒） |
| --- | ---: |
| 排队 | 0.000017 |
| 任务 Sandbox 创建 | 1.990747 |
| 工作区准备 | 2.976116 |
| Harness 启动 | 7.277605 |
| Runtime 执行 | 8.473110 |
| 停止与撤权 | 5.598754 |
| 独立验证 | 4.300355 |
| 清理 | 2.592051 |

上述为原Attempt的八阶段字段；观测窗口33.983398秒，不能用阶段求和替代端到端测量。`completed=1`只表示执行完成，验证成功吞吐为0。此为一次功能验收，不能由0.029426任务/秒推算容量。

| 工具调用 | 模型轮 | SDK 在途等待（秒） | guest 受管执行整体（秒） |
| --- | ---: | ---: | ---: |
| T1 | 1 | 2.938402 | 2.047305 |
| T2 | 1 | 2.323604 | 2.048337 |
| T3 | 2 | 2.317044 | 2.044225 |

T1/T2同控制器SDK等待重叠2.315237332秒；两路原生调用身份独立，真实ToolResult与录制一致。guest时长不拼成跨域并行时间线，SDK减guest不叫网络耗时。

1010项原件/身份/账本/导出检查通过。同一Snapshot导出221个Perfetto区间/27条flow、221个OTLP Span/27个parent；TraceProcessor起止/时长误差0ns，完整性complete、全局coverage仍unknown。本次后端发送、UI及观测扰动未验。2个Sandbox独立NotFound，无补杀、临时服务已清理。使用私有应用API夹具恢复录制的OpenAI请求方言，不宣称原CLI无需适配。[版本与复现证据](observability/VALIDATION_AND_REPRODUCE.md#r01-multicall-app)。

<a id="r01-multicall"></a>

## R01同轮多个真实工具：受控录制与严格回放

这是**协议验收，不是自主Agent解题成绩或Sandbox性能排名**。同一固定OpenClaw A镜像，在两个新任务Sandbox中分别录制、严格回放：每次3轮模型/9帧、3个真实exec；前两次来自同一轮，第三次是下一轮的同命令新调用。模型由本地受控服务给出；未向Agent提供修复解法，原独立Verifier各12项中11失败、score0，成功任务总数仍78。

### 八阶段原值（秒）

| 阶段 | 受控录制 | 严格回放 |
| --- | ---: | ---: |
| 排队 | 未采集：直接Runner无app admission | 未采集：直接Runner无app admission |
| 任务 Sandbox 创建 | 0.969562 | 0.721326 |
| 工作区准备 | 3.443461 | 1.630757 |
| Harness 启动 | 8.392704 | 7.216278 |
| Runtime 执行 | 8.486204 | 7.687951 |
| 停止与撤权 | 5.590331 | 5.607986 |
| 独立验证 | 5.148170 | 4.687415 |
| 清理 | 0.866173 | 0.839704 |

独立验证包含本探针额外的评分前cgroup/UID/listener检查、候选捕获与原Verifier执行，不能与无此探针的评分阶段直接比快慢。八阶段有未覆盖区间，不能相加替代端到端时长。模型请求累计分别为8.214ms、9.547ms，是受控模型/回放代理成本，不能解释成真实LLM延迟。

### 逐工具原值（秒）

| 运行 | 调用 | 所属模型轮 | SDK在途等待 | guest受管执行整体 | 退出码 | stdout/stderr字节 |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| 受控录制 | T1 | 1 | 2.291449 | 2.039789 | 0 | 15/0 |
| 受控录制 | T2 | 1 | 2.325819 | 2.045710 | 0 | 15/0 |
| 受控录制 | T3 | 2 | 2.309581 | 2.054910 | 0 | 15/0 |
| 严格回放 | T1 | 1 | 2.288001 | 2.037025 | 0 | 15/0 |
| 严格回放 | T2 | 1 | 2.327794 | 2.039658 | 0 | 15/0 |
| 严格回放 | T3 | 2 | 2.303554 | 2.038023 | 0 | 15/0 |

T1/T2使用不同ToolCall/Invocation/SSH/Execution/Process身份；即使命令相同也不合并。两次运行的T1/T2 SDK等待分别重叠1.640077205秒、2.260018555秒，说明同Agent多工具同时在途；guest整体时长各有其本地依据，**不能将两条guest时钟拼成并行执行时间线**，也不能将SDK减guest机械命名为网络耗时。

125项复核通过，6/6身份链精确贯通，全部ToolResult正文与退出/字节证据对账；停止撤权与评分前正向隔离确认成功。4个Sandbox独立NotFound、无需补杀，云资源已回收。前三次准备/启动失败均0创建、0实际Runner，原件保留；计数误填和修正见[复现归档](observability/VALIDATION_AND_REPRODUCE.md#r01-multicall)。该直接Runner原件未验完整Snapshot/双出口；后续应用级运行另见上节。UI、华为/B多调用、真实LLM多调用、新代码扰动仍未验；本例不升级P0整体状态。

<a id="p1c-resource-prometheus"></a>

## 最新资源观测：控制器、Harness 与 Sandbox 分别看

这次重读**7组已有运行**，没有增加Agent成功次数。资源流是“谁的资源读数”，不同于前文八个任务阶段：controller是共享Go进程；A模式Harness是独立容器；provider读数覆盖整个任务或评分Sandbox。它们不能相加为单个Tool消耗，也不能把共享controller拆分给多个任务。

| 归档环境 | controller指标点 | A Harness指标点 | Sandbox指标点 | 保留的缺测 |
| --- | ---: | ---: | ---: | --- |
| 四格E2B A | 16 | 93 | 任务56、评分7 | 无新增缺测流；全链覆盖仍unknown |
| 四格E2B B | 16 | — | 任务42 | 评分Sandbox无读数 |
| 四格华为A | 16 | 90 | 未投影 | provider采样时间单位未确认 |
| 四格华为B | 16 | — | 未投影 | provider采样时间单位未确认 |
| A两任务并发 | 16（共享） | 未采 | 未采 | L0原运行未启provider采样 |
| B首输出 | 16 | — | 任务56 | 评分Sandbox无读数 |
| B CLI取消 | 16 | — | 未采 | L0原运行未启provider采样 |

“指标点”是带时间的单个指标值，**不是独立采样次数或ToolCall次数**；“—”表示该placement没有A外置Harness容器。16个controller点来自两次采样×8个值，其中RSS为进程生命周期峰值。E2B重复返回的同时间同值保留原回执、只投影一次；首次monitor CPU百分比0为初始化占位，不当空闲。

13条可导出资源流进入各自独立Prometheus，首发与重发后均为90条owner内序列、456个历史点；26次查询/2108项对账通过、数值误差0，时间按毫秒截断不足1ms。另6条缺测流不补零、不发送。华为controller/Harness资源可看，但其Sandbox采样仍仅保留原值；IO/Network、每Tool CPU与短任务峰值未测。这里没有新增耗时排名或云厂商优劣结论。解析器与后端测试脚本的两次失败原因、原件及后续验证均见[完整证据](observability/VALIDATION_AND_REPRODUCE.md#p1c-resource-prometheus)，[指标解释与命令](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#p1c-resource-prometheus)单列。

<a id="p1b-offline-prometheus"></a>

## 最新分析能力：阶段指标进入 Prometheus

这是**既有运行的离线分析验证**，不新增成功任务或性能样本。A并发、B首输出、B取消三份原Snapshot分别进入独立Prometheus3.15.1：每份6种指标、29个OTLP点，查回117条业务序列和117个历史样本；再次发送相同数据，仍是117个样本。原任务计数、吞吐和阶段Histogram的count/sum/每个桶均逐值一致。

A含2任务的16个阶段测量；B含1任务的8个；取消样本含8个已测阶段，另一个未启动任务的8项保留缺测。失败不会被滤掉，取消时的Evaluate入口耗时也不称作评分执行。Prometheus按毫秒保存时间，本次截断不足1ms，原ns留在Snapshot/manifest。未采的资源Gauge、未配置SLO的Goodput不补零；当前不能据此绘制资源实时曲线或用单末点推导rate()。

实际查询、重复/乱序/reset/冲突协议检查、源码版本与全部临时后端清理见[验收归档](observability/VALIDATION_AND_REPRODUCE.md#p1b-offline-prometheus)；指标含义及命令见[测量契约](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#p1b-offline-prometheus)。两种Trace投影和原任务结果保持不变，P0仍PARTIAL。

<a id="p1a-offline-jaeger"></a>

## 最新分析能力：同一快照的 Perfetto 与 Jaeger 对账

本轮是**已有结果的离线导出验收**，没有新增 Sandbox、模型调用或任务成功数。`aries-perf` 读取冻结 Snapshot，校验原内容 ID 后生成标准 OTLP JSON；在 ECS 隔离 Jaeger 2.22.0 中实际发送并查询。原 Perfetto 投影逐值保持一致，原始快照不修改。

| 冻结归档 | Snapshot 区间数 | 实际导出 / 查回唯一 Span | 明确排除的无 UTC 锚点区间 | 精确 parent | 来源文件 |
| --- | ---: | ---: | ---: | ---: | ---: |
| E2B A 两任务并发 | 845 | 845 | 0 | 86 | 38 |
| E2B B 首输出 | 802 | 795 | 7 | 31 | 12 |
| E2B B CLI 取消 | 787 | 784 | 3 | 18 | 12 |

Jaeger v3 查询的起点、终点和持续时间与 OTLP 纳秒值误差均为 **0 ns**。legacy UI 查询按微秒表达，实测起点/持续时间误差最低 −999 ns，由分别截断的起点与持续时间相加得到的终点误差最低 −1983 ns；未改写原始时间。无锚点原生区间继续留在独立 Perfetto 时钟域中。瞬时事件和只有时长的记录仍保留在 Snapshot，未凭近邻时间挂入 Span；完整性 complete、全局覆盖 unknown 不变。

**重复发送确实会重复入库。** 实际再次发送相同 payload 后，Jaeger 返回行数为1690/1590/1568，唯一 Span ID 仍为845/795/784，内容相同。稳定 ID 不等于 exactly-once；重复导出不能增加业务成功数或吞吐分母。三份真实归档的非 parent Links 都为0；另用明确标注的4 Span协议夹具验证同/跨 Trace Link，不算真实执行证据。

工程定向 unit/race 各254项通过（含子测试），vet/build通过；HTTP部分拒绝、零拒绝但有警告、超时等另有测试。候选1曾因 SDK 回执读回后的 JSON 字段顺序改变而未通过原快照哈希；失败保留，候选2恢复原类型后原 ID 校验通过，没有重签旧数据。详见[版本、查询、重发与清理证据](observability/VALIDATION_AND_REPRODUCE.md#p1a-offline-jaeger)。P1b账本/阶段指标已另见上节；资源指标限定范围见[P1c](#p1c-resource-prometheus)，P2未验，P0仍部分完成。

测试用例另见[目录](../cases/sandbox-runtime-v1/README.md)、[逐例验收卡](../cases/sandbox-runtime-v1/acceptance.md)及[来源与测量依据](../cases/sandbox-runtime-v1/authority-and-measurement.md)。用例主线为6个[真实Agent任务](../cases/sandbox-runtime-v1/agent-cases/README.md)：Agent自主解决问题、独立评分，随后才录制/回放。10个自建场景及5个标准工具变体保留为辅助测量设计；exec-short仅新增ECS本机子进程功能检查，不是Agent/ToolBridge/Sandbox性能样本，均不增加78个成功任务。

## 运行环境与测试类型

Runner（测试控制器）与模型录制/回放入口均在远程 ECS。A/B 表示 Agent 放在哪里，和云厂商是两个独立维度。

| 运行环境 | Agent 在哪里 | 工具和工作目录在哪里 | 已有端到端结果 |
| --- | --- | --- | --- |
| E2B A（外置） | ECS 的 Harness 容器 | E2B Sandbox，经 SSH/Bridge 调用 | 真实模型录制、recorded 回放、顺序重复、并发2的精确身份验收 |
| E2B B（内嵌） | E2B Sandbox | 与 Agent 同一 Sandbox，本地执行 | 真实模型录制、recorded/zero 回放、顺序重复、并发 2 |
| 华为 A（外置） | ECS 的 Harness 容器 | 华为 Sandbox，经 SSH/Bridge 调用 | 真实录制、同 E2B portable-v1 Tape 的 recorded 回放；旧 Tape 元数据偏离失败保留 |
| 华为 B（内嵌） | AgentSphere Sandbox | 与 Agent 同一 Sandbox，本地执行 | 真实模型录制、recorded 回放；评分出网隔离采用假设 |
| 阿里云、腾讯云 | 尚未验收 | 尚未验收 | 尚无对应性能结果 |

| 模型模式 | 本文表中写法 | 实际行为 |
| --- | --- | --- |
| 真实模型并录制 | record | 调用 DeepSeek，保存请求与响应 |
| 按录制时序严格回放 | recorded | 返回录制响应并按录制时序等待，严格校验请求历史和工具结果 |
| 不主动等待的严格回放 | zero | 返回相同录制响应，但不主动等待录制时序 |

recorded 和 zero 只替代模型响应，Agent、工具和 Sandbox 仍真实运行。
镜像构建、ECS 本机协议检查、Docker 集成测试的耗时另存附录，不算云端任务阶段耗时。

## 阶段与指标定义

### 阶段耗时：一次任务的各段操作

全文阶段名称、阶段表顺序统一如下。流程中的中文名与表格第一列完全一致：

**排队 → 任务 Sandbox 创建 → 工作区准备 → Harness 启动 → Runtime 执行 → 停止与撤权 → 独立验证 → 清理**

| 阶段 | 做什么 | 计时边界及包含范围 |
| --- | --- | --- |
| [排队](performance-stages.md#排队) | 等待调度器分配执行名额 | 计划到达 → 调度接纳；不是 Sandbox 内部排队 |
| [任务 Sandbox 创建](performance-stages.md#任务-sandbox-创建) | 创建做题的沙箱 | 调用任务沙箱创建接口 → 返回；含 Adapter/SDK/网络等待，不是纯虚拟机启动时间 |
| [工作区准备](performance-stages.md#工作区准备) | 上传题目源码、初始化目录 | 调用工作区准备 → 返回；含文件传输和初始化命令 |
| [Harness 启动](performance-stages.md#harness-启动) | 启动 Agent 的运行外壳与 Gateway | 调用 Harness.Start → 返回；A/B 的连接归属不同，见下文；不含此前的 Bridge 注册 |
| [Runtime 执行](performance-stages.md#runtime-执行) | Agent 接收任务、调用模型与工具 | 调用 Gateway Agent 请求 → 返回；含模型等待、工具调用和通信，比整个 Harness.Run 范围窄 |
| [停止与撤权](performance-stages.md#停止与撤权) | 停止 Agent，撤销工具访问权限 | 依次调用 Harness.Stop、Bridge.Stop 的整段时间；确认成功才允许独立验证 |
| [独立验证](performance-stages.md#独立验证) | 封存答案，在新沙箱运行独立测试 | 调用 Benchmark.Evaluate → 返回；含答案捕获、评分沙箱创建/准备、测试执行、报告回收 |
| [清理](performance-stages.md#清理) | 回收本次任务仍持有的资源 | Runner 最终清理开始 → 结束；含任务/评分沙箱删除，必要时再次停止/撤权；不含 attempt 和 run 级收尾 |

**这些是已采集的阶段，不是无缝覆盖全流程的时间分桶。** Bridge 注册、连接和证据收集等还有其他开销。
A 的任务 WebSocket 连接在 Harness.Run 内、Runtime 执行之前；B 的连接在 Harness 启动内，所以 A/B 启动耗时的操作范围也不同。
Trace 的 `agent.run` 覆盖整个 Harness.Run，范围比表中 Runtime 执行更大；两者不能当作同一个数。
不要用八个阶段相加代替端到端延迟，也不要用“总时间 − 模型 − 工具”当作准确的 Harness 开销。

### 其他指标：累计延迟、内部活动与整个运行窗口

以下指标有各自的含义，**不作为额外阶段加入阶段耗时表**。

| 指标 | 类型 | 从哪里计时 / 如何统计 |
| --- | --- | --- |
| 产物延迟 | 单任务累计延迟 | 计划到达 → 答案被稳定封存；包含排队、停止与撤权，以及独立验证开始后的答案捕获 |
| 验证延迟 | 单任务累计延迟 | 计划到达 → 独立验证完成；包含独立验证，不含后续清理 |
| 模型请求累计 | Runtime 内部活动 | 每轮模型请求从代理接纳到结束的耗时求和；含解析、校验、上游/回放等待与流式转发，不是纯模型计算时间 |
| 原生工具耗时 | Runtime 内部活动 | OpenClaw 对一次原生工具调用报告的墙钟耗时；A 含 SSH/Bridge，B 在 Sandbox 内本地计时 |
| 原生工具累计 | Runtime 内部活动 | 本次任务各次原生工具耗时之和，包含错误调用；并发调用的时长可能重叠，不等于关键路径 |
| 完整观测窗口 | 调度批次的统计窗口 | 调度开始 → 所有 attempt 返回；含排队、任务清理与 attempt 收尾，用于吞吐分母；不含 run 级共享准备/清理和最终 Trace 落盘 |

下文时间表均标明单位。`<0.001` 表示非零但不足 1 毫秒；`—` 表示没有可用测量，不能解释成零。
工具耗时是经过的实际时间，不是 CPU 使用时间；后台进程是否全部结束还需独立确认。
原始结果中的 duration 字段以纳秒保存，阶段表按表头换算为秒或毫秒，逐工具表换算为毫秒；各表保留所标示的精度。

阶段原始字段、A/B 的边界差异及三层清理范围，见[阶段说明](performance-stages.md)。


<a id="r01-native-concurrent"></a>

## E2B A：同 Tape 两任务并发的阶段与工具耗时

固定568文件运行时、同A镜像/模板、原portable Tape、recorded时序、L0；原CLI设置attempts=2/concurrency=2。两个任务各7轮/735帧、6次真实exec与12项独立Verifier全部通过，原Stop/revoke先于评分，4个Sandbox均独立NotFound。没有修改产品源码或Strict标准，0上游模型调用。212项原件核对通过，新增2个整体成功任务。

001和002是同一批次中的两个任务，不是两个云厂商，也不是baseline/L0对照。阶段单位为秒，阶段名称与上文完全一致。

| 阶段 | 任务001 | 任务002 |
| --- | ---: | ---: |
| 排队 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.414 | 1.500 |
| 工作区准备 | 1.626 | 2.415 |
| Harness 启动 | 9.826 | 10.057 |
| Runtime 执行 | 19.400 | 19.391 |
| 停止与撤权 | 5.584 | 5.552 |
| 独立验证 | 5.545 | 4.753 |
| 清理 | 0.892 | 2.068 |

| 另行统计的累计量（秒，不是额外阶段） | 任务001 | 任务002 |
| --- | ---: | ---: |
| 产物延迟 | 40.025 | 40.240 |
| 验证延迟 | 44.256 | 44.485 |
| Attempt持续时间 | 45.162 | 46.574 |

六次工具顺序及ToolCall ID在两任务中相同，身份链通过各自Attempt scope区分，12条均精确贯通native→SSH→Bridge→SDK→guest。下表按同一个ToolCall逐项对齐，单位毫秒；exit两次一致，T2/T4的失败是原录制轨迹的一部分，不能删除。

| 工具（ID末8位） | 原生工具001 | 原生工具002 | SDK等待001 | SDK等待002 | guest wrapper总时长001 | guest wrapper总时长002 | exit |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| T1（w52C3459） | 340.000 | 361.000 | 280.955 | 299.189 | 42.260 | 48.294 | 0 |
| T2（hare0388） | 649.000 | 637.000 | 520.967 | 499.656 | 282.104 | 235.833 | 1 |
| T3（Fwxt1521） | 356.000 | 344.000 | 298.240 | 292.312 | 43.668 | 38.484 | 0 |
| T4（3wUS4367） | 516.000 | 585.000 | 462.611 | 535.145 | 212.985 | 291.327 | 1 |
| T5（WqaJ0202） | 375.000 | 378.000 | 307.397 | 325.005 | 37.590 | 58.593 | 0 |
| T6（CES65411） | 636.000 | 725.000 | 585.473 | 663.457 | 318.369 | 230.589 | 0 |

原生工具来自OpenClaw实报墙钟，A包含SSH/Bridge；SDK等待是控制器`exec.wait`区间；guest wrapper总时长来自`guest_duration_ns`，包含guest启动、执行与收尾，不是CPU时间。三列范围不同且有包含关系，不能相加，也不能把差值直接当作网络延迟。原始`guest_boundaries`另保留Popen返回、首输出、观察退出及pipe EOF；无输出时首输出缺失，不填零。

同controller的`agent.run`区间重叠18.540939532秒，证明两个Harness.Run调用同时在运行；该区间比上表Runtime执行范围宽。本次SDK等待区间重叠对数为0，guest时钟未对齐，不能声称工具同时执行。完整观测窗口46.574544889秒，原账本2个已验证成功任务的吞吐为0.0429419118任务/秒；这是单批观察值，不是稳态容量或SLO Goodput结论。

同Snapshot的实际Perfetto TP通过845区间/86flow/38来源，起止与时长导入误差0ns；完整性complete、覆盖unknown，没有合成guest泳道，本次UI未执行。运行证据`observability-r01-native-concurrent-1/evidence.json` SHA `02242069258cfd48aeaa0a486f550cc89860d2427f8008b81e32744121280400`，复核`c160e55b…`、TP `4dfd3eb3…`；[完整复现与SHA](observability/VALIDATION_AND_REPRODUCE.md#r01-native-concurrent)。本次跨任务并发范围验收完成，单Agent内部多工具并行/重试另列未验。

<a id="r04-cli-cancel"></a>

## E2B B：原 CLI 取消与未启动 Attempt 的真实账本

固定568文件运行时、schema2 E2B模板、原B Tape `73fafa3d…`、recorded时序和v1.4分析器，原CLI计划2个Attempt、并发1。在第2个模型请求（turn 1）严格匹配成功后发送一次SIGINT，原CLI退出码为1。实际接纳并执行1个Attempt，终态为**canceled**；另1个保持**not_started**，没有创建其运行环境或模型会话。完成数、独立验证成功数均为0，本次故障不增加累计78个成功任务。

下表单位为**毫秒**，原始纳秒值换算后保留6位小数。未启动的002全部为`not_collected`，不是耗时0，也没有把它等候至取消的时间补作排队耗时。

| 阶段 | 001：canceled（毫秒） | 002：not_started | 本次实际范围 |
| --- | ---: | --- | --- |
| 排队 | 0.023175 | 未采集（not_collected） | 001计划到达到调度接纳 |
| 任务 Sandbox 创建 | 1503.198156 | 未采集（not_collected） | 仅创建001的任务Sandbox |
| 工作区准备 | 2656.265325 | 未采集（not_collected） | 上传并准备原题目工作区 |
| Harness 启动 | 18195.579195 | 未采集（not_collected） | 启动内嵌Gateway并连接 |
| Runtime 执行 | 5799.510149 | 未采集（not_collected） | Agent请求开始至取消返回 |
| 停止与撤权 | 364.228646 | 未采集（not_collected） | 原Harness.Stop与Bridge.Stop |
| 独立验证 | 0.124309 | 未采集（not_collected） | 仅Evaluate因context取消而返回，未执行Verifier |
| 清理 | 441.141243 | 未采集（not_collected） | 回收001任务Sandbox |

**独立验证这一行不表示评分执行。** 原值`verifier_duration=124309ns`只计量Evaluate调用至取消返回；原结果为`evaluation.status=canceled`、`verifier_status=not_run`，没有创建评分Sandbox，没有独立测试结果，原`score=0`不能解释为测得的评分。

Strict实际匹配2个请求、完成1轮、发送48帧；第一个真实工具已完成，消费结果的已归档字段与原录制一致，未见Strict偏离或重试。整段模型会话仍因取消以`incomplete`结束，应用账本模型状态为failed，不能称完整Tape回放成功。该工具保留10条精确身份的原生回调，以下三个区间均只在同一Node单调时钟内相减：

| T1（ToolCall ID末8位：9ZVU3443）的区间 | 实测耗时（毫秒） | 边界 |
| --- | ---: | --- |
| 启动返回→stdout首输出 | 56.833176 | `child_spawn_return→stdout_first_nonempty` |
| 启动返回→直接子进程退出回调 | 82.884209 | `child_spawn_return→child_exit` |
| 退出回调→最后一路自然EOF | 15.266023 | `child_exit→stdout_end`；stderr已先到达EOF |

未观测到stderr首个非空输出回调，该值不填零。上述区间不与控制器Runtime相加，也不跨时钟计算网络或取消延迟；第一工具完成不能证明任意后台后代全部退出。原Stop/revoke确认成功，1个任务Sandbox由provider独立确认NotFound，无fallback、无清理错误，0上游LLM。

61项原件复核通过。同一Snapshot保留两个Attempt及八阶段原值，完整性**complete**、覆盖**unknown**；模型会话未完成与记录完整性是独立结果。实际TraceProcessor分别导入控制器784区间/18flow和原生3区间/0flow，12个来源SHA、787区间并集、起止/时长/outcome均精确对账，导入误差0ns、无processor errors；无guest对齐锚点，两个域分别显示，本次UI未执行。

运行证据`observability-r04-cli-cancel-1/evidence.json` SHA `107f9d0975dfebadc217752032bb38a037b83720b5aeffb929dcf351d6118829`，原件复核`40d77d4b…`；TP证据`observability-r04-cli-cancel-perfetto-1/evidence.json` SHA `692218816456403e825327628f90a4069fc41c2e720987d0928fc77adcc28d59`。[完整复现与SHA](observability/VALIDATION_AND_REPRODUCE.md#r04-cli-cancel)。本例补的是原应用层取消/未启动账本及其真实源导出；不覆盖下方直接Runner故障的缺失exit/EOF或辅助导出FAIL，旧原件全部保留。

<a id="b-native-cancel"></a>

## B 模式主动取消：阶段耗时与评分前停止验收

这是当前568文件/schema2 E2B B版本的一次**故障测试**，使用脚本模型启动真实工具，确认父进程和脱离会话的子进程活动后主动取消。没有上游模型调用，也不是Tape回放或成功任务；不计入成功任务或供应商性能排名。

| 阶段 | 实测耗时（秒） | 本次发生什么 |
| --- | ---: | --- |
| 排队 | 未采集 | 直接Runner夹具，没有应用层admission |
| 任务 Sandbox 创建 | 0.996397 | 创建真实E2B任务环境 |
| 工作区准备 | 3.500353 | 放入原始未修复候选和故障工作负载 |
| Harness 启动 | 19.695399 | 启动原生OpenClaw Gateway并确认就绪 |
| Runtime 执行 | 3.374608 | 原生Agent启动工具，确认心跳后取消Run |
| 停止与撤权 | 0.319896 | 原Stop、cgroup停止、Bridge撤权与通道join |
| 独立验证 | 5.113368 | 含评分前追加隔离探针、候选捕获及原始Verifier |
| 清理 | 0.763820 | 删除评分和任务Sandbox |

整项任务wall为36.013593秒；八阶段不是穷尽分解，另含Bridge注册约0.240384秒与Run返回前观测收尾约2.009225秒，不机械相加解释为Sandbox耗时。该故障夹具的评分前探针增加了独立验证阶段耗时，不能与正常回放的评分耗时直接排名。

已知父/子PID+start身份消失、UID60000进程0、cgroup empty、两次心跳与mtime不再变化，guest18789/18790拒绝连接后才评分。独立12测试中11项失败、得分0，符合未修复候选；Harness canceled、隔离confirmed、清理succeeded。25项原件复核通过，2个Sandbox均独立NotFound、无fallback。生命周期证据 `fd90659a…`，详情见[复现档案](observability/VALIDATION_AND_REPRODUCE.md#r07-b-native-cancel)。

工具只采到spawn与stdout首输出，间隔**26.008256ms**；exit/EOF缺失，终态和排空耗时为**未知**。原生collector仍failed/incomplete。直接Runner结果缺应用层Attempt账本，该夹具辅助导出只含控制器记录；该夹具的全链Trace验收未通过，原FAIL保留，不能把停止确认画成精确退出时间线。后续[原CLI取消验收](#r04-cli-cancel)是另一份真实运行证据，不回填或覆盖本例缺失。

<a id="r15-b-native-paired"></a>

## E2B B：新增原生观测的四对开销实测（最新）

固定同一个schema2模板、568文件运行时、原B Tape `73fafa3d…` 与recorded模型时序，仅切换baseline/L0。执行顺序为 **baseline→L0、L0→baseline、L0→baseline、baseline→L0**。8/8严格回放及每次12项独立验证通过，16个Sandbox均确认删除，0上游LLM；完整原件与配对算术479项复核通过。

baseline保留真实工具、Strict完整会话、独立验证和停止/清理，只关闭可选观测；L0打开控制端、模型、SDK和原生进程/首输出观测。两组都不开L1资源轮询。因此测的是这套L0观测的整体增量；不能拆成单独logger成本。

### 八阶段：每次运行的实测时间（秒）

下表列序为实际执行顺序，“基线”表示baseline；L0表示观测开启。每行阶段名与全文定义一致。

| 阶段 | 对1 基线 | 对1 L0 | 对2 L0 | 对2 基线 | 对3 L0 | 对3 基线 | 对4 基线 | 对4 L0 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 排队 | 0.000013 | 0.000026 | 0.000015 | 0.000014 | 0.000019 | 0.000014 | 0.000013 | 0.000028 |
| 任务 Sandbox 创建 | 0.984456 | 1.406852 | 0.853294 | 0.897184 | 1.402785 | 1.187108 | 0.839665 | 1.468888 |
| 工作区准备 | 2.526399 | 3.231388 | 2.866944 | 2.509774 | 3.743187 | 1.734750 | 2.603025 | 2.166091 |
| Harness 启动 | 23.972576 | 23.102829 | 13.992286 | 21.033924 | 16.959403 | 12.333106 | 20.484350 | 18.759542 |
| Runtime 执行 | 16.636038 | 16.500795 | 15.857461 | 15.978462 | 15.432784 | 15.409273 | 16.286163 | 16.111829 |
| 停止与撤权 | 0.325758 | 0.378157 | 0.333509 | 0.304648 | 0.310668 | 0.339925 | 0.417976 | 0.335078 |
| 独立验证 | 5.154768 | 4.556738 | 4.039533 | 4.531741 | 4.807688 | 3.999663 | 4.947841 | 5.976058 |
| 清理 | 0.881824 | 1.614164 | 0.798452 | 0.799413 | 1.532677 | 1.793148 | 0.770176 | 1.460456 |

### 成对差值：L0 减 baseline（毫秒）

| 配对 | Runtime 执行差值 | 整个 Attempt 差值 | 独立验证差值 | Harness 启动差值 |
| --- | ---: | ---: | ---: | ---: |
| 1 | -135.243 | +327.975 | -598.030 | -869.747 |
| 2 | -121.001 | -7251.935 | -492.208 | -7041.637 |
| 3 | +23.511 | +7378.659 | +808.025 | +4626.298 |
| 4 | -174.334 | -91.433 | +1028.217 | -1724.808 |

Runtime配对差值中位数 **−128.122ms**，范围 **−174.334～+23.511ms**；整个Attempt中位数 **+118.271ms**，范围 **−7.252～+7.379秒**。启动、工作区准备与评分等阶段有明显波动；四对小样本不足以将负差值解释为“负开销”，也不足以判定p99、稳态容量或性能达标。

### 共享控制器资源与观测写入

| 实际执行项 | 控制器CPU（毫秒） | 进程生命周期峰值RSS（MiB） | 四类JSONL返回写入字节 |
| --- | ---: | ---: | ---: |
| 对1 baseline | 726.325 | 29.336 | —（未启用） |
| 对1 l0 | 779.017 | 32.766 | 1966696 |
| 对2 l0 | 602.487 | 34.148 | 1610542 |
| 对2 baseline | 664.850 | 29.207 | —（未启用） |
| 对3 l0 | 660.943 | 33.340 | 1717418 |
| 对3 baseline | 526.410 | 28.727 | —（未启用） |
| 对4 baseline | 653.738 | 29.375 | —（未启用） |
| 对4 l0 | 698.291 | 33.117 | 1732619 |

CPU是同一Go进程内Runner、ModelProxy与collector的合计；RSS是进程生命周期峰值，不能拆出Sandbox/Tool资源。峰值RSS的配对差值中位数 **+4.178MiB**，范围 **+3.430～+4.941MiB**。写入字节是4类明确JSONL文件的Write返回计数，非物理磁盘I/O，不含guest logger、Adapter及所有索引文件。GC/分配、写入/Sync明细和GNU time另在原件中保留。

四次L0各实测4轮/573帧、3个exec和30个原生回调；baseline这些观测为null，不能拿Tape预期值替代。L0的Runner与模型边界队列均报告drop=0/write_failed=0；没有独立producer分母的native logger全局丢失仍unknown。四份Trace分别导入controller1120/753/860/877区间，各另有7个native区间、31条flow、12来源，全部0ns导入误差；coverage仍unknown，UI沿用同版已验能力，本次UNRUN。

此前首轮矩阵单列保留：1条baseline通过，7条在准备阶段失败、未启动Runner或创建Sandbox；独立Linux实验复现无SO_REUSEADDR的冗余bind检查会拒绝TIME_WAIT端口，结合原失败位置判断是该预检查误拒绝（原OSError未保存errno）。仅删除实验驱动的该检查后重新预登记并完整执行本节8次；**旧baseline不混入本节配对**。首轮2个Sandbox也已删除；两批合计18个，不遗漏旧失败分母。

当前B版本的这组baseline/L0功能与声明计量验收完成；性能合格阈值未设定，L1/L2和独立guest/Adapter成本仍未验。证据与复现见[完整档案](observability/VALIDATION_AND_REPRODUCE.md#r15-b-native-paired)。

<a id="e2b-b-first-output"></a>

## E2B B：首输出、直接子进程退出与排空

复用华为 B 已验收的同一组schema2原生观测模块与v1.4分析器，从固定原E2B模板派生新构建。原E2B B Tape `73fafa3d…` 的一次recorded严格回放通过：**4轮/573帧、3个真实工具、12/12独立验证**；Stop/revoke确认，0上游LLM。正式运行的2个Sandbox及先前1个启动探针均确认删除，模板保留复现。

### 八阶段耗时（秒）

| 阶段 | 本次 E2B B |
| --- | ---: |
| 排队 | 0.000023 |
| 任务 Sandbox 创建 | 0.932837 |
| 工作区准备 | 2.625814 |
| Harness 启动 | 13.941225 |
| Runtime 执行 | 15.829574 |
| 停止与撤权 | 0.359630 |
| 独立验证 | 4.864940 |
| 清理 | 1.209090 |

### 同一工具的分段观测（毫秒）

与下节华为 B 使用完全相同的列和计时边界；两次使用不同的原Tape，不能据此作云厂商快慢排名。

| ToolCall顺序 | OpenClaw自报 | 控制器start→result | spawn→首输出 | spawn→直接子进程exit | exit→最后自然EOF | exit code |
| --- | ---: | ---: | ---: | ---: | --- | ---: |
| T1（9ZVU3443） | 66.000 | 315.787 | 7.588 | 35.683 | 4.438 | 0 |
| T2（sr1K0418） | 221.000 | 240.147 | 152.823 | 205.451 | 不生成：EOF先于exit | 1 |
| T3（LCm15952） | 208.000 | 224.146 | 161.493 | 195.719 | 不生成：EOF先于exit | 0 |

30条回调按真实ToolCall/run/session身份关联。三次首输出均为stdout；stderr没有非空chunk观察值，不能填0。T2仍为原轨迹中修复前测试失败（exit=1），最终任务由独立Verifier判为成功。首输出与直接子进程区间均从spawn返回开始，不能相加；不同单调时钟未对齐，不能用列间差值推导通信开销。

TraceProcessor实际导入控制器795区间/31flow与native7区间/0flow，12来源SHA、802区间并集及结果对账通过，导入误差0ns。快照完整性complete、全局覆盖unknown。本次没有重新跑UI，投影未改，沿用上一轮同版华为B的UI能力验收；不声称本次E2B Trace已做UI点选。详见[原件、模板与复现](observability/VALIDATION_AND_REPRODUCE.md#r07-e2b-first-output)。全进程树、PTY终态、该观测版本的扰动成本仍未验收。

<a id="huawei-b-first-output"></a>

## 华为 B：首输出、直接子进程退出与排空

新镜像 `ba1c0c46…` 只在原生已有数据回调增加首个非空 stdout/stderr 测点；没有改工具命令、解码、ToolResult、后台/poll/PTY或Strict规则。使用原华为B Tape `3fc9751b…` 的 recorded 回放，**4轮/693帧、3个真实工具、12/12独立验证通过**。本次任务来自 `observability-r07-first-output-huawei-b-1`；2个Sandbox和1个临时模板确认删除，0上游LLM调用。

### 八阶段耗时（秒）

| 阶段 | 本次华为 B |
| --- | ---: |
| 排队 | 0.000016 |
| 任务 Sandbox 创建 | 0.594050 |
| 工作区准备 | 1.254168 |
| Harness 启动 | 23.498267 |
| Runtime 执行 | 13.725760 |
| 停止与撤权 | 0.214608 |
| 独立验证 | 2.391645 |
| 清理 | 0.703200 |

### 同一工具的分段观测（毫秒）

“首输出”从native spawn返回算起，终点是原生回调第一次拿到非空原始数据块，早于解码与模型消费；不是程序首次write的内核时刻。

| ToolCall顺序 | OpenClaw自报 | 控制器start→result | spawn→首输出 | spawn→直接子进程exit | exit→最后自然EOF | exit code |
| --- | ---: | ---: | ---: | ---: | --- | ---: |
| T1（4N3N6498） | 38.000 | 266.784 | 3.408 | 17.373 | 2.797 | 0 |
| T2（zcXC8208） | 479.000 | 514.418 | 438.164 | 468.947 | 不生成：EOF先于exit | 1 |
| T3（wisF6708） | 315.000 | 356.778 | 280.955 | 302.793 | 不生成：EOF先于exit | 0 |

30条原生回调通过真实身份关联：27条既有生命周期、3条stdout首输出。三次均未观察到非空stderr，因此stderr首输出没有时间值；不能写成0。T2是原Tape中修复前的失败测试，仍保留exit=1；最终成功由独立Verifier判定。

首输出与退出都是从spawn开始的累计区间，**不能相加**。T1此次实际测到exit后的2.797ms自然排空；上一版未出现该顺序的记录保持原样。控制器与Sandbox单调时钟没有对齐，不把两列差值算成网络或Harness成本。单次结果不构成观测开销比较或云厂商排名。

真实回放证据SHA `9fd94c87a04401e12e71e4d547eb23facdd3c5507a6f4b1d0c512950a46e957e`；原件对账 `c6ec81105b453278c90a61934ed4f9164febe23ed93ef5c8466dd381f6ab62e6`。同一Snapshot的控制器779区间/31flow、native7区间/0flow与12来源均通过TraceProcessor对账，导入误差0ns；实际Perfetto UI也完成T2首输出438.164376ms的查询和点选，身份、来源及作用范围一致，3张截图归档。完整性complete、全局覆盖unknown。详见[首输出复现档案](observability/VALIDATION_AND_REPRODUCE.md#r07-b-first-output)。

<a id="huawei-b-native-process"></a>

## 华为 B：上一版进程时间（schema 1）

`observability-r07-native-huawei-b-1` 使用568文件运行时 `r07-native-release-1`、候选镜像 `6470521e…`，开启 `aries-native-process-v1`，保持原 ToolResult 和原华为 B Tape `3fc9751b…`。一次 recorded Strict Replay：**4轮、693帧、3个真实exec、12/12独立验证通过**；2个Sandbox及1个临时模板均确认删除，0上游模型调用。原生后台/poll/PTY功能未禁用，本次真实任务走的是3次前台非PTY调用。

### 八阶段耗时（秒）

| 阶段 | 本次华为 B |
| --- | ---: |
| 排队 | 0.000014 |
| 任务 Sandbox 创建 | 0.581323 |
| 工作区准备 | 1.201875 |
| Harness 启动 | 29.766537 |
| Runtime 执行 | 13.707187 |
| 停止与撤权 | 0.216731 |
| 独立验证 | 2.835474 |
| 清理 | 0.816542 |

### 每个工具的三个观测口径（毫秒）

工具顺序与原Tape完全一致。T2的exit=1是轨迹中修复前的测试失败；最终任务由独立Verifier判定成功，不能把中间失败删除或改为成功。

| 同一 ToolCall | OpenClaw自报工具时长 | 控制器收到start→result | Sandbox内spawn返回→直接子进程exit | exit code | 退出后的pipe drain |
| --- | ---: | ---: | ---: | ---: | --- |
| T1（4N3N6498） | 44.000 | 166.408 | 22.511 | 0 | 不生成：双流EOF均早于exit |
| T2（zcXC8208） | 405.000 | 434.300 | 394.435 | 1 | 不生成：双流EOF均早于exit |
| T3（wisF6708） | 315.000 | 346.927 | 303.746 | 0 | 不生成：双流EOF均早于exit |

三列都是真实值，但边界不同，**不能相加，也不能把差值直接命名为网络或Agent Loop开销**。新增27条回调逐一绑定真实run/session/ToolCall；3个spawn→exit区间在独立Node单调时钟中计算。没有强行拼接控制器时钟；双流先EOF的原始顺序保留，不填0、负耗时或虚构排空区间。本节旧schema1没有首输出；新schema2结果见上节。完整后代树、PTY drain、独立预期进程分母及logger增量成本仍未知。

运行证据SHA `5f1b029959862aebe75ad94d1e5b2042c5a2e63b6938486660107183dd6e66e2`；原件逐项对账SHA `efa0d2514ad2e6bfc597dcce3d27f947c48602d22f3719583d07a85512535def`。初版分析错误要求B提供A的SSH身份文件，原incomplete快照保留；修订后的分析和Trace状态见[本次复现证据](observability/VALIDATION_AND_REPRODUCE.md#r07-huawei-b-native)。修订后的同一Snapshot为integrity complete / coverage unknown；Perfetto主视图878区间/31flow、原生进程视图3区间/0flow，12来源及完整区间并集对账通过，导入误差0ns；[实际UI](observability/VALIDATION_AND_REPRODUCE.md#r07-huawei-b-ui)也已核对T2的434.299929ms控制器区间和394.434524ms原生区间、身份及来源，两个时钟保持未对齐。这是一个B轨迹的实测，不能直接与A或另一份E2B B Tape作供应商排名。

<a id="native-agent-cancel"></a>

## 故障验收：A原生Agent执行中取消

固定568文件运行时/原A镜像，脚本模型只给1条持续运行exec；父子及变化心跳确认后取消。**这是故障用例，不是Strict Replay，不计入成功任务数。** Harness保持canceled；停止撤权与评分前PID/cgroup/心跳检查通过，独立Verifier对未修候选12项中11失败、score0；2个Sandbox确认删除，0上游LLM。

| 阶段 | 本次秒数 |
| --- | ---: |
| 排队 | 未采集：直接Runner夹具没有app admission |
| 任务 Sandbox 创建 | 1.294114 |
| 工作区准备 | 2.755511 |
| Harness 启动 | 7.191033 |
| Runtime 执行 | 4.440958 |
| 停止与撤权 | 5.532607 |
| 独立验证 | 7.514844 |
| 清理 | 1.441600 |

Verifier总段包含本例额外的评分前进程/心跳检查，不能与普通任务的Verifier耗时作性能比较；Runtime是原生Agent请求区间，取消后的guest退出时刻未采，不补零。模型→native→SSH→Bridge→SDK→guest receipt已按精确身份复核。原检查器错误要求Bridge ID为UUID而FAIL；纠正为代码实际的不透明ID后，仅对原件离线复核PASS，旧FAIL保留。完整源SHA、复现、JUnit、回收及原始计时来源见[取消验收档案](observability/VALIDATION_AND_REPRODUCE.md#r01-native-cancel)。

<a id="r15-cleanup-paired"></a>

## 当前版本：baseline／L0 四对真实观测成本

`observability-r15-paired-6`，固定565文件 `cleanup-release-1`、同一E2B A镜像/模板、同一Tape `d716431e…`、并发1、recorded时序。预登记8次按表中顺序执行，8次均Strict成功、6个真实exec、独立Verifier 12/12、停止/撤权确认。16个Sandbox全部provider NotFound，0上游LLM调用。另有0创建的预检，未计入8次任务。411项原件/分母/算术对账通过。

### 八阶段原值（秒）

| 次序／配对 | 观测级别 | 排队 | Sandbox创建 | 工作区准备 | Harness启动 | Runtime执行 | 停止与撤权 | 独立验证 | 清理 |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 / 1 | baseline | 0.000009 | 0.915 | 2.528 | 7.503 | 18.697 | 5.526 | 5.969 | 0.764 |
| 2 / 1 | l0 | 0.000021 | 0.999 | 1.918 | 7.315 | 18.936 | 5.516 | 4.165 | 0.760 |
| 3 / 2 | l0 | 0.000011 | 0.812 | 1.965 | 7.615 | 19.066 | 5.509 | 4.467 | 0.766 |
| 4 / 2 | baseline | 0.000027 | 0.702 | 2.016 | 7.668 | 19.092 | 5.507 | 4.676 | 0.751 |
| 5 / 3 | l0 | 0.000037 | 0.890 | 1.819 | 7.388 | 19.318 | 5.527 | 4.405 | 0.763 |
| 6 / 3 | baseline | 0.000013 | 0.749 | 1.932 | 7.280 | 19.296 | 5.544 | 7.876 | 0.758 |
| 7 / 4 | baseline | 0.000009 | 0.951 | 1.861 | 7.388 | 18.798 | 5.500 | 4.176 | 0.762 |
| 8 / 4 | l0 | 0.000018 | 0.903 | 1.943 | 7.339 | 19.263 | 5.534 | 3.659 | 0.758 |

### 开启L0增加了多少（每对 L0 − baseline）

| 指标 | 四对差值中位数 | 最小～最大差值 | 口径 |
| --- | ---: | ---: | --- |
| Runtime执行 | +130.106 ms | −25.134～+464.468 ms | 含真实模型回放等待、Agent与Tool，不是纯观测函数开销 |
| 控制器用户态CPU | +25.121 ms | +15.438～+29.231 ms | 共享Go进程，不能拆成Runner/ModelProxy/collector |
| 控制器内核态CPU | +22.488 ms | −10.629～+33.131 ms | 同上 |
| Go累计分配 | +4.293 MiB | +3.762～+4.964 MiB | 累计分配量，非常驻内存 |
| 全进程生命周期峰值RSS | +2.836 MiB | +1.422～+19.387 MiB | 比较生命周期峰值，非某阶段独占峰值 |
| 独立验证 | −1.161 s | −3.471～−0.209 s | 云侧验证延迟存在波动 |
| 整体Attempt | −1.246 s | −3.298～+0.028 s | 不能据此宣称观测“负开销”；该样本中验证耗时波动更大 |

四对只能描述这组固定条件，未设性能合格阈值，不外推p99、容量或云厂商排名。两级都开启原生身份协议，故测的是可选L0增量，**不包含身份协议自身的绝对成本**。baseline关闭的模型轮/帧与文件I/O为null；L0四次实测均7轮/735帧，四类JSONL写入/Sync计数留原值。L1/L2成对成本尚未测；四组L1功能回归不能代替它。

四份L0 Trace均通过实际TraceProcessor：416/431/422/434区间，各43条flow/21来源，起点、时长和终点误差0ns。整体coverage仍unknown；baseline关闭的观测不伪造成完整时间线，本次UI未验证。

历史 `paired-4` 的4成功/1失败/3未运行原件保持不变；本组是提前固定版本和顺序的新实验，不替换失败样本，也未修复旧额外请求的未知来源。证据及复现见[当前成对验收](observability/VALIDATION_AND_REPRODUCE.md#r15-cleanup-paired)。

<a id="p0-mainline-l1"></a>

## 最新主线：E2B／华为 A、B 四组 L1 回归

2026-10-25，当前565文件冻结版本 `r05-cleanup-release-1`，并发1、recorded Strict Replay、真实 Sandbox Tool、独立 Verifier，四组各完成1次任务，均12/12验证通过。L1同时开启资源采样；没有上游模型调用。总计8个Sandbox、2个华为临时模板已回收。

A两组使用同一份Tape `d716431e…`（7轮/735帧/6工具）；E2B B为 `73fafa3d…`（4/573/3），华为B为 `3fc9751b…`（4/693/3）。**B的两份轨迹不同，不能据此比较供应商；A也只有各1次样本，仅展示本次实值，不给性能排名。** 华为禁止出网仍按既有 `assumed-unverified` 前提执行，不宣称完成完全隔离验收。

### 八阶段：名称与上文一致，单位秒

| 阶段 | E2B A | 华为 A | E2B B | 华为 B |
| --- | ---: | ---: | ---: | ---: |
| 排队 | <0.001 | <0.001 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.131 | 0.567 | 0.788 | 0.556 |
| 工作区准备 | 2.153 | 1.438 | 1.616 | 1.434 |
| Harness 启动 | 7.344 | 7.301 | 13.937 | 24.198 |
| Runtime 执行 | 19.521 | 18.027 | 15.964 | 14.053 |
| 停止与撤权 | 5.526 | 5.393 | 0.316 | 0.211 |
| 独立验证 | 5.521 | 2.130 | 5.364 | 3.083 |
| 清理 | 1.409 | 1.185 | 1.314 | 0.679 |

各次 **attempt全程** 分别为 E2B A 43.420秒、华为 A 36.545秒、E2B B 39.650秒、华为 B 44.410秒。它包含阶段间及attempt收尾开销，不是八阶段相加；不是吞吐容量或p99结论。

### A同一Tape：逐工具三种耗时（毫秒）

以下T1–T6按Tape顺序，以同一ToolCall ID及已有execution/Process映射对齐。消费＝ECS收到原生start至result；客户端＝提交至Wait返回；guest＝沙箱内wrapper全过程。三者边界不同、部分重叠，不能相加或相减推算网络时间。

| 工具（ToolCall ID后缀） | E2B消费 | 华为消费 | E2B客户端 | 华为客户端 | E2B guest | 华为guest | 退出码 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| T1（w52C3459） | 1173.408 | 762.774 | 288.357 | 159.183 | 45.412 | 61.607 | 0 |
| T2（hare0388） | 1216.260 | 970.639 | 790.460 | 648.235 | 314.137 | 469.845 | 1 |
| T3（Fwxt1521） | 844.204 | 490.950 | 464.099 | 221.486 | 46.281 | 55.919 | 0 |
| T4（3wUS4367） | 810.491 | 787.214 | 422.065 | 481.056 | 177.181 | 334.032 | 1 |
| T5（WqaJ0202） | 810.033 | 458.941 | 335.058 | 152.520 | 41.649 | 56.474 | 0 |
| T6（CES65411） | 830.426 | 777.775 | 436.284 | 476.031 | 174.132 | 324.041 | 0 |

T2/T4的exit1是同一录制轨迹中的真实失败测试，随后修复复测成功；两云失败位置一致，未删除失败工具。guest时长只有本域duration，没有伪造与ECS对齐的起点。

### B原生工具：仅列实际采到的消费区间（毫秒）

| 环境／独立Tape | T1 | T2 | T3 | 逐工具guest退出/EOF |
| --- | ---: | ---: | ---: | --- |
| E2B B | 371.054 | 206.330 | 373.762 | —（未接入） |
| 华为 B | 301.428 | 513.386 | 353.461 | —（未接入） |

两个B表行不构成同任务性能对照；ECS私有B进程PoC的exit/EOF测点尚未随本轮云运行发布，不能拿PoC数据填这里。

### L1与Trace是否采齐

| 组合 | 任务/评分采样RPC行数 | Observer | Perfetto区间/flow/来源 |
| --- | ---: | --- | --- |
| E2B A | 10/2 | succeeded | 435/43/21 |
| 华为 A | 9/2 | succeeded | 428/43/21 |
| E2B B | 9/2 | succeeded | 833/31/11 |
| 华为 B | 10/2 | succeeded | 809/31/11 |

四组均核对同一控制器时钟下Metrics返回早于对应kill开始，资源文件Write计数/字节及一次Sync完成；该结果验证正常L1收尾，不证明永久文件挂起已解决。E2B返回滚动历史，任务的36/27条provider读数分别只含8/6种timestamp；华为本次每RPC返回1条读数。**RPC行数、provider读数、独立样本数不是同一指标**；E2B B评分沙箱虽然RPC成功，却没有provider读数，资源量为未采到，不是CPU/内存为零。

四份Trace均实际导入TraceProcessor，起点/时长/终点导入误差0ns、无processor errors；integrity=complete、coverage=unknown。UI本轮未另执行，未声称全工具零漏采。

本轮保留3次启动失败：B镜像配置校验失败1次、华为端口预检OSError各1次，均在业务attempt前、0云创建；原两项NOT_RUN及后续续跑分别留档。最终4个真实业务attempt均成功，没有挑选成功样本替换失败。原OSError未记录errno，不能事后认定为TIME_WAIT。

离线统一对账：`observability-r05-mainline-analysis-3/evidence.json`，SHA `86f235c4e8bb00d2e2e7373750db58ce385a9a7423ea7aa0c885097dba7f8bd3`；全部输入SHA未变。四组原件、复现入口和清理证据见[完整归档](observability/VALIDATION_AND_REPRODUCE.md#p0-mainline-l1)。这组L1功能/测量回归不补历史R15失败矩阵，也不是baseline/L1成对成本实验。

<a id="p0-single-diagnostic"></a>

## 新诊断版本：E2B A 单次严格回放

2026-10-25，`p0-diagnostic-release-1`，564文件 source manifest `7909603e…`。同原 A Tape `d716431e…`、native 镜像与模板，L0、recorded、并发1；新增外部停止日志跟随。一次执行成功，7轮/735帧/6个真实exec，独立Verifier12/12，2个Sandbox确认删除。属于诊断样本，不能与旧版本拼接成R15配对或用于性能排名。

| 阶段 | 实测耗时（秒） |
| --- | ---: |
| 排队 | 0.000035701 |
| 任务 Sandbox 创建 | 1.794651724 |
| 工作区准备 | 2.642138718 |
| Harness 启动 | 7.420430162 |
| Runtime 执行 | 19.411319308 |
| 停止与撤权 | 5.546744018 |
| 独立验证 | 4.207550562 |
| 清理 | 1.558197251 |

Attempt 时长43.414638322秒，验证延迟41.838611057秒；八阶段不是无缝时间分桶，不相加替代端到端值。6个原生消费区间与152个SDK客户端区间完成对账，整体coverage仍unknown。follower保留7,188字节，流正常关闭，不能据此声称producer无丢失。本次未触发历史额外模型请求，不代表已修复。

原始证据 `/opt/aries-artifacts/observability-p0-e2b-diagnostic-1/evidence.json`，SHA `ea68ad81bdac4e2f1e31e0869e298865a772a08c7f37277f1f96c44f10ad4530`；固定run与验证范围见[复现档案](observability/VALIDATION_AND_REPRODUCE.md#p0-single-diagnostic)。R05文件worker的8例新PoC属于工程验证，不计入上述成功数；父索引等I/O尚未隔离，生产R05仍BLOCKED，见[PoC范围](observability/VALIDATION_AND_REPRODUCE.md#r05-process-file-poc)。

<a id="p0-runtime-current"></a>

## 历史合并版本：华为 A、B 严格回放

2026-10-25，统一Runner/分析器源码manifest `68a6d81f…`（562文件）。A使用既有R01录制的A Tape `d716431e…`（本轮未重录），B使用既有B Tape `3fc9751b…`；均按recorded时序、并发1、真实工具执行。**两份Tape不同，这张表展示各自阶段，不比较A/B快慢。** 任务各1/1通过，独立Verifier各12/12；共4个Sandbox及2个临时模板确认删除，0上游模型调用。

### 八阶段（秒）

阶段及顺序与上文一致；排队保留9位小数。阶段不是无缝分桶，不能相加替代端到端延迟。

| 阶段 | 华为 A / A Tape | 华为 B / B Tape |
| --- | ---: | ---: |
| 排队 | 0.000018074 | 0.000011127 |
| 任务 Sandbox 创建 | 0.589 | 0.533 |
| 工作区准备 | 1.213 | 1.374 |
| Harness 启动 | 7.467 | 23.909 |
| Runtime 执行 | 17.829 | 13.748 |
| 停止与撤权 | 5.376 | 0.194 |
| 独立验证 | 2.894 | 2.953 |
| 清理 | 0.559 | 0.540 |

### 此次具体采到了什么

| 观测对象 | 华为 A | 华为 B | 含义 |
| --- | ---: | ---: | --- |
| 严格匹配模型轮 / recorded SSE帧 | 7 / 735 | 4 / 693 | 真实回放计数，不是上游模型推理耗时 |
| Agent原生exec调用 | 6 | 3 | 含轨迹中预期失败的工具 |
| 原生start→result消费区间 | 6 | 3 | ECS接收事件的经过时间，包含传输/消费调度 |
| 客户端操作区间 | 145 | 519 | 环境、工具、产物、验证与清理的RPC/Wait；不是Agent工具数量 |
| 客户端成功 / 失败 / 取消 | 125 / 4 / 16 | 451 / 47 / 21 | 原始状态保留，不能用最终任务通过改写 |
| Python SDK duration-only属性 | 47 | 117 | 有本地时长、无跨域锚点，不画嵌套slice |
| 归档exec→SSH→guest精确映射 | 6/6 | 不适用 | B本地执行，不伪造A链路 |
| Snapshot完整性 / 全链覆盖 | complete / unknown | complete / unknown | 文件完整不等于全部调用均被采集 |

A的guest wrapper计时可按精确身份另行查看；B目前没有原生每工具的独立进程退出/排空边界，明确缺测。两种计时都不能称工具CPU时间，也不能把消费者区间减去guest时长命名为网络延迟。

### 华为 A：同一次工具调用的三种耗时（毫秒）

T1–T6按固定Tape顺序；用ToolCall→execution→Process精确身份核对，不按时间猜关联。三列有不同边界，不能相加，也不能相减得到网络耗时。

| 工具 | 原生事件消费区间 | 客户端提交→Wait返回 | guest wrapper全过程 | 退出码 |
| --- | ---: | ---: | ---: | ---: |
| T1 | 726.899 | 134.003 | 41.938 | 0 |
| T2 | 946.353 | 581.879 | 450.031 | 1 |
| T3 | 445.538 | 151.532 | 57.079 | 0 |
| T4 | 714.794 | 424.830 | 332.963 | 1 |
| T5 | 461.996 | 157.879 | 65.413 | 0 |
| T6 | 754.076 | 476.625 | 331.416 | 0 |

消费区间是ECS收到start至result；客户端区间包含提交、通信和等待；guest全过程从wrapper计时起点到终态返回。T2/T4的exit1是录制轨迹中的真实失败，仍严格回放并保留。

下面四列是**同一guest起点到观察时刻的偏移量**，不是四段可相加的耗时。直接子进程退出指轮询观察到终态，不是内核实际退出时间；pipe EOF可能早于这次轮询观察，不能倒置或强制修正。

| 工具 | Popen返回 | 首次非空输出 | 观察到直接子进程退出 | 双流pipe EOF |
| --- | ---: | ---: | ---: | ---: |
| T1 | 0.234 | 41.601 | 41.889 | 41.930 |
| T2 | 0.337 | 427.728 | 449.968 | 450.023 |
| T3 | 0.277 | — | 57.010 | 56.991 |
| T4 | 0.288 | 312.648 | 332.901 | 332.954 |
| T5 | 0.252 | — | 65.350 | 65.408 |
| T6 | 0.278 | 317.408 | 331.336 | 331.406 |

T3/T5未观察到非空输出，首输出用“—”，不补零。原始ns、Clock/scope、SHA与源行号保存在`observability-p0-runtime-huawei-tool-timings-1/summary.json`，SHA `c67b8fe709d9dfb09b82de02e78888f87ca72ac3a933f0b6034a5b42407ef984`；全部输入未变，证据SHA `6494d349da232bc48acf52c0e026d5d3b3a2a23ee591b3165b02f02a9a44f2fe`。

华为B原Tape三次工具的“原生消费区间 / 原生自报耗时”分别为 **278.123/26、434.327/392、351.293/301 ms**。消费区间来自ECS单调时钟，原生自报值来自OpenClaw；无B逐工具guest四边界，不能把A的SDK/guest数值填到B列。

A真实TraceProcessor验收408个区间/43条flow/21源，B为740/31/11，起点/时长/终点误差均0ns。新A实际Perfetto UI已查询并点选native_tool与sandbox_client两类独立区间，身份/时长/scope与Snapshot一致；不是同一Tool的父子示例，也不证明R15通过。UI证据SHA `778119f1170b06ca90e88326a60dc10b291e2b1b105bf6932195972139077144`，10个浏览器进程已确认回收。

A证据 `observability-p0-runtime-huawei-replay-1/evidence.json` SHA `77439a39cfb9d4385b87587866c53e1e795a79370ad619e29014217f06ef2ebc`。
B原driver误用字符串比较UTC与+08:00，导致checker失败；实际Verifier创建晚于答案封存25,649ns。原FAIL保留，新整型时间解析离线复核PASS，无第二次云运行：`observability-p0-runtime-huawei-b-review-2/evidence.json` SHA `4c4f4e11100d36efcfd54948a8d63590e1e30ebb6e23ee36f8b14070f142e1a6`。完整source/run/reconciliation及原失败SHA见[复现档案](observability/VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates)。

<a id="p0-runtime-r15"></a>

## 历史合并版本：E2B A 配对测试未全部通过

同release-2、固定A Tape与原生镜像，并发1。预登记4对交错顺序；计划8次，实际5次：**4次整体成功、1次Strict会话失败、3次NOT_RUN**。独立Verifier实际5次均12/12通过，不能把它等同5次完整Benchmark成功。按预登记规则在第一次失败后停止，没有替换或重试样本；10个Sandbox均独立NotFound，自有资源已清理，0上游模型调用。

第5次已完整回放7轮/735帧，6次工具决策和完整结果与Tape一致，最终assistant也正常结束；随后在Harness.Run返回后的Stop窗口收到额外模型请求，Strict拒绝并将session标为失败。它不是题目测试失败，也不是已证实的双流偏离。额外调用方仍在调查，不能归因给云厂商或观测开销。

### 五次实际运行的八阶段（秒）

| 序号 / pair / 条件 / 整体结果 | 排队 | 任务 Sandbox 创建 | 工作区准备 | Harness 启动 | Runtime 执行 | 停止与撤权 | 独立验证 | 清理 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 / 1 / baseline / PASS | 0.000012822 | 0.979 | 1.553 | 7.313 | 20.562 | 5.546 | 6.355 | 1.309 |
| 2 / 1 / l0 / PASS | 0.000011415 | 0.746 | 2.126 | 7.320 | 19.434 | 5.549 | 5.824 | 0.771 |
| 3 / 2 / l0 / PASS | 0.000014041 | 0.724 | 2.289 | 7.336 | 19.326 | 5.568 | 3.696 | 0.774 |
| 4 / 2 / baseline / PASS | 0.000011438 | 0.687 | 2.209 | 7.427 | 19.685 | 5.553 | 4.703 | 0.872 |
| 5 / 3 / l0 / FAIL | 0.000014663 | 0.733 | 2.125 | 7.377 | 19.426 | 5.509 | 5.506 | 0.773 |
| 6–8 / 后续计划 / NOT_RUN | — | — | — | — | — | — | — | — |

八阶段仍不是无缝时间分桶。失败样本保留原始时长，但不与前两对成功样本合算扰动。

### 仅两对完整样本的差值

每格“baseline → L0（L0减baseline）”，按原始纳秒计算差值后再四舍五入，可能与展示值相减相差1ms。资源为共享Runner+ModelProxy+Go collector的SELF数据，不包含Docker Agent、Adapter或Sandbox，也不能拆分给三个组件。

| 指标 / 单位 | pair 1 | pair 2 |
| --- | ---: | ---: |
| Runtime执行 / s | 20.562 → 19.434（-1.127） | 19.685 → 19.326（-0.359） |
| 验证延迟 / s | 43.118 → 41.796（-1.321） | 41.061 → 39.741（-1.319） |
| Attempt时长 / s | 44.428 → 42.587（-1.841） | 41.933 → 40.528（-1.405） |
| 共享SELF CPU / ms | 631.681 → 703.014（+71.333） | 643.659 → 711.495（+67.836） |
| 累计分配 / MiB | 110.955 → 116.194（+5.239） | 112.681 → 116.478（+3.797） |
| GC次数 | 34 → 27（-7） | 45 → 29（-16） |
| GC暂停累计 / ms | 2.608 → 3.105（+0.497） | 3.388 → 3.426（+0.038） |
| 生命期峰值RSS / MiB | 72.375 → 64.121（-8.254） | 59.535 → 60.703（+1.168） |

Runtime差为−1.127s、−0.359s，只有两对，不能解释为负开销或性能合格；共享SELF CPU分别增加71.333ms、67.836ms。冷热/缓存/厂商调度状态未知；无批准的合格阈值、不估置信区间，不推p99或容量。原`paired-analysis.json`的`NOT_ESTIMATED_FOUR_PAIRS`是计划四对的固定旧标签，实际可计算分母是2，独立复核已明确这一点。

### L0文件写入实测（含失败样本）

每格“Write返回次数 / 接受字节数”；各项Sync started=returned=1，无返回错误/短写。baseline关闭这几类writer，值为null而不是零，不能据此作零成本相减。原生事件文件和其他进程IO不在此四类计量内。

| 文件 | 第2次L0 PASS | 第3次L0 PASS | 第5次L0 FAIL |
| --- | ---: | ---: | ---: |
| Runner events | 351 / 272,250 | 351 / 272,262 | 363 / 281,975 |
| 模型request-boundaries | 815 / 676,843 | 815 / 676,843 | 832 / 689,983 |
| 模型legacy events | 768 / 441,350 | 768 / 441,349 | 773 / 444,773 |
| SDK客户端operations | 300 / 198,085 | 298 / 195,541 | 296 / 196,104 |

三份L0 Trace均实际导入TraceProcessor：区间/flow分别413/43、412/43、428/47，各21源、误差0ns、无processor错误；第5次失败status/validated及模型拒绝原样保留。baseline可选观测关闭，不把空时间线当通过。可导出且文件完整不等于本次Benchmark成功。

主证据`observability-r15-paired-4/evidence.json` SHA `78f4a30e77d33c137f77dfe71bfd0a72718021fd6c8cfcf8dccfbd7646bf285c`；失败因果边界`observability-r15-case5-analysis-1/evidence.json` SHA `6e921883c15fc26c8c897846c694902ae4ac5d49c115a15851b0fdba39e42636`。236项原件/分母/部分配对算术独立复核通过，SHA `30e2e1cbfb0cc843a72ed6e50c18aaf7b561420343ae97c36f8777cfdaa0e447`；其含义是“失败矩阵记录可信”，不是矩阵通过。完整复现位置与后续诊断见[验证档案](observability/VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates)。

固定镜像源码复核排除了不适用的dashboard标题入口和六次前台exec的后台完成通知条件。心跳首轮可按哈希相位提前，只是候选；原件缺少晚期请求结构与Stop期间Gateway日志，无法唯一确认caller。该缺口及下一次最小取证已归档，未通过关闭心跳、删除失败或放宽Strict来“修复”。

<a id="r01-current-agent"></a>

## 历史 R01：E2B A 新录制与同 Tape 严格回放

2026-10-24，固定 `combined-release-3` 的 Runner、SSH client、Adapter 和 native 镜像。
真实 DeepSeek 录制一次、recorded Strict Replay 一次；并发 1，每次 7 轮模型、6 次真实
exec、735 个 SSE 帧，分别通过 12 项独立测试。原生后台/process/yield/PTY 能力保留，
这两次实际只运行前台非 PTY。4 个 Sandbox 已逐项确认删除。

两次均有 6/6 **归档 exec** 的 native→SSH→Bridge→SDK Process→guest 精确映射；
归档之外的全工具分母仍未知。A native 计时未采集，两份 Snapshot 保持 incomplete。
本组验证新输出契约与身份链，**不是跨云对比，也不是 baseline/L0 扰动对照**。

单位：秒；阶段名称和顺序与上文一致，不能相加代替端到端延迟。

| 阶段 | E2B A / record | E2B A / recorded |
| --- | ---: | ---: |
| 排队 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.573 | 0.657 |
| 工作区准备 | 2.319 | 1.625 |
| Harness 启动 | 7.257 | 8.826 |
| Runtime 执行 | 19.919 | 19.259 |
| 停止与撤权 | 5.535 | 5.544 |
| 独立验证 | 5.431 | 5.232 |
| 清理 | 1.408 | 1.569 |

| 另行统计的累计量（秒，不是额外阶段） | record | recorded |
| --- | ---: | ---: |
| 产物延迟 | 38.509 | 37.197 |
| 验证延迟 | 42.775 | 41.959 |
| Attempt 持续时间（attempt 开始→返回，不是整个 run） | 44.216 | 43.545 |

新 Tape SHA `d716431ea36e1b89b47aa0c8f091cd6e65700bc10e83522467414a0d3735cffb`；
候选 SHA 两次均为 `181c011a55737cb2ade8836c992ee463c45479d31312cfc6855aba0d6f62bdf1`。
主证据 `/opt/aries-artifacts/observability-r01-full-agent-2/evidence.json`，SHA
`ddf450782ad18f5f074ab3c815302b7f363273f18ff607d787a26553a6797b1b`。
record/replay 的 run-result SHA 分别为
`aba05e54d0d9081ef8baf3e6aaefb1a85447efdec47460767fcc289673436a69`、
`4b8a6615886e3b92e04e13adbc3e3d1e5cdbedfbfacafacdbd50a964b8e20f17`。
逐任务路径、实际版本与清理证明见[复现档案](observability/VALIDATION_AND_REPRODUCE.md)。

新 Trace 的原生 TraceProcessor 实际导入/SQL 对账通过：record 62 区间/50 flow，
replay 55/43；同 Snapshot 的起点、时长和 flow 端点一致，导入误差 0 ns。
各 19 个来源 SHA 核验，原始产物与导出未变；新 UI 交互未执行。
证据 `observability-r01-full-agent-perfetto-1/evidence.json`，SHA
`3ac0470bbcc0038bf1c05c1027ec12405366c7296bc4a0c1d45d7e9ba9ff7b3e`。

独立原始证据复核69项通过，SHA `c20e935b5202e293ddb3bbc1225e125cc0be3a9ab1b59548a74b6bae4c8a7fa2`。
首次复核器错误要求非零退出也标成功、遗漏既有 ID 方言；失败留档，修正只影响检查器，
实际失败工具仍计入轨迹，完整文本仍严格比较。详见复现档案。

前次 `full-agent-1` 因缺发布 pins 在 preflight 失败，两次任务均 NOT_RUN、0 创建；
失败保留，不计入这两个已执行任务，也不从全部尝试记录中删除。
此后合并版本已用该Tape验收华为A，见本轮结果；华为B使用不同Tape。原录制/回放证据及原时钟缺测状态不变。

<a id="observability-p0"></a>

## 历史 P0：release3 新埋点的真实回放回归

2026-10-24，Runner `observability-p0-v3`。预登记 3 个组合，实际各执行 1 次，**3/3 完成且每项通过 12 个独立测试**；没有新录制、重试、Strict divergence 或环境失败。本组新建的 6 个沙箱均确认删除，华为临时模板也已删除。原始任务证据、评分与本节数字不随 Trace 导出器修复改变。

E2B A 与华为 A 使用同一 portable-v1 Tape：8 轮模型、7 次真实工具、675 个 recorded SSE 帧，调用顺序、完整工具结果及候选 SHA 一致。E2B B 使用另一份原 merge-ranges Tape：4 轮模型、3 次工具、573 帧，**B 这一列是独立兼容回归，不与 A 做速度排名**。均为并发1、单样本；本轮未测观测扰动、稳态容量或p99。

本轮 P0 表没有新增华为 B 行，不表示华为 B 从未测试：历史 `agentsphere-embedded-record-3` 与 `agentsphere-embedded-replay-1` 已完成华为 B 原始 `merge-ranges` 的真实录制和严格回放（4轮、3工具、693帧、12项验证通过）；对应 record evidence SHA 为 `3b7b71cac4904df80a977db39afce278f65ccf39190f466841ae6cf8b630d61e`，replay evidence SHA 为 `70b2523cd27a5cb304779c3a707b1553bc354dd574c9f70d3bdcb04a61d2cf29`。那组运行早于release3新观测字段，且工作负载与本轮矩阵不同，因此保留在历史基线，不冒充本轮 P0 观测验收；补齐华为 B 的新观测需要使用最终冻结的候选 Runner/Adapter/native 组合重新执行。

### 八阶段：与上文相同的定义

单位：秒。八阶段不是无缝分桶，不能相加替代端到端延迟。

| 阶段 | E2B A | 华为 A（同 A Tape） | E2B B（不同 Tape） |
| --- | ---: | ---: | ---: |
| 排队 | <0.001 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.322 | 0.602 | 0.962 |
| 工作区准备 | 4.007 | 1.064 | 1.889 |
| Harness 启动 | 7.471 | 7.382 | 21.656 |
| Runtime 执行 | 21.823 | 18.712 | 15.968 |
| 停止与撤权 | 5.534 | 5.347 | 0.279 |
| 独立验证 | 5.521 | 2.999 | 4.717 |
| 清理 | 1.373 | 0.681 | 1.396 |

| 另行统计的累计量（秒，不是额外阶段） | E2B A | 华为 A | E2B B |
| --- | ---: | ---: | ---: |
| 产物延迟 | 41.483 | 33.860 | 42.161 |
| 验证延迟 | 46.445 | 36.571 | 45.777 |
| 完整观测窗口 | 47.833 | 37.268 | 47.193 |
| 模型请求累计（保持原接纳口径） | 12.864 | 12.865 | 10.997 |

### 新细分：独立验证内部

这些子区间均包含在“独立验证”内，单位为秒。评分 Sandbox 创建由已有真实 `sandbox.start` 且 `purpose=evaluation` 的跨度提供；其余四项是新埋点。没有把子区间再加到总阶段上。

| 子步骤 / 原始 operation | E2B A | 华为 A | E2B B |
| --- | ---: | ---: | ---: |
| 捕获候选 `candidate.capture` | 0.559 | 0.288 | 1.101 |
| 创建评分 Sandbox `sandbox.start` | 1.175 | 0.570 | 0.866 |
| 准备评分文件 `verifier.prepare` | 1.712 | 1.320 | 1.402 |
| 执行独立测试 `verifier.exec` | 1.232 | 0.743 | 1.109 |
| 读取评分报告 `verifier.report` | 0.843 | 0.078 | 0.239 |

### 新细分：模型入口和 Gateway

下表是同一任务内对应区间的**累计毫秒**，不是额外阶段，也不是模型纯计算耗时。模型HTTP与内部读取/解析/strict匹配互相包含，不能相加。

| 观测范围 | E2B A | 华为 A | E2B B |
| --- | ---: | ---: | ---: |
| HTTP收到→处理返回 `model.http_request` | 12865.134 | 12866.409 | 10997.194 |
| 实际读取请求体 `model.body_read` | 0.626 | 0.526 | 613.480 |
| 解析请求 `model.parse` | 4.827 | 4.687 | 5.827 |
| Strict匹配 `model.match` | 11.117 | 10.604 | 12.046 |
| Gateway收包→入队尝试 | 0.229 | 0.217 | 2.851 |
| Gateway入队尝试→消费 | 0.019 | 0.021 | 2.456 |

A只消费accepted和终态两个响应；B还消费原生事件，共89次delivery，因此Gateway累计数不是相同负载下的A/B性能对比。“入队尝试”埋在非阻塞send前，不能解释成精确队列commit或网络往返。B的请求体读取区间包含数据到达等待，613.480毫秒不是JSON解析耗时，更不能据此归因Sandbox CPU慢。

### 同 A Tape：每一步工具耗时

单位：毫秒。以下仍是原生工具报告的墙钟耗时，**包括Harness/SSH/Bridge及guest链路**。本轮新增身份只准确连通Bridge接受点→SDK Process；不能把下面第n个原生工具硬配到第n个Bridge执行。

| 顺序 | 实际操作 | exit code（两云一致） | E2B A | 华为 A |
| --- | --- | ---: | ---: | ---: |
| 1 | 读取源码与公开测试 | 0 | 341 | 290 |
| 2 | 首次pytest | 1 | 753 | 693 |
| 3 | 写入第一次修复 | 0 | 335 | 176 |
| 4 | 第一次复测 | 1 | 537 | 492 |
| 5 | 修正相邻区间合并 | 0 | 352 | 189 |
| 6 | 第二次复测 | 0 | 1083 | 444 |
| 7 | 读取最终源码 | 0 | 348 | 174 |

两次失败复测是Tape中的实际工具步骤，全部保留，不能从性能统计删除。7个原生调用之外，Bridge还执行启动/检查等操作；两云各有15个带execution_id的Bridge执行，都按精确ID绑定到SDK Process，不靠命令/顺序/时间猜测。guest总duration仍包含包装与排空边界，不是原命令CPU时间。

B的三个工具在控制器端“消费start→消费result”区间为431.231、215.601、197.903毫秒；这是 `native_event_consumer_interval`，与上表原生duration不是同一口径，故不混排。B的后台进程终态与纯exit/drain分离仍unknown。

### 可信度和证据位置

- R15 的 model/request-boundaries 与 performance writer 在有索引的样本中 `dropped=0`；E2B operation/native-tool 索引没有 drop 字段，不能把它们写成零丢失。完整模型轮/帧已与Tape逐项核对。A的原生start/result源未采集，Snapshot明确`incomplete`；B已收文件`complete`，但缺独立预期采集分母，整体coverage仍`unknown`。这些都不等于全链100%覆盖。
- 三项复核：`/opt/aries-artifacts/observability-p0-final-review-1/evidence.json`，SHA `edbf9d2a613bfcb1ac45c0787cd37628806026020e6d458b2f4014baed68d561`；数值来源`metrics.json`，SHA `265ba949ec3e093365ab8876eee278cbe33095b7f6377056dff72855bd3c754c`。144个被读取的源文件哈希保持不变。
- 修正版 Trace 的最终 Perfetto 证据：`/opt/aries-artifacts/observability-p0-perfetto-3/final-evidence.json`，SHA `b87f277816c8a2f6e751029c6ef15bc2528e8658fc6757aeece94f5ec8a2bb42`。A/B 的原生 SQL 与实际 UI 均通过；A 查询为 60 slices / 47 flows / 0 errors，B 为 217 / 31 / 0，选中的 `model.http_request` 时长分别为 1.837496333 秒和 2.517551987 秒。截图、UI 状态快照、Trace SHA 与 Snapshot SHA均在该私有归档中；浏览器残留为0。
- 真实Trace首轮导入发现flow ID格式错误，失败证据保留；任务评分通过不代表图已正确导入。最终导出/UI结果统一记录在[验证归档](observability/VALIDATION_AND_REPRODUCE.md#离线导出与perfetto)。
- 华为评分环境禁止出网仍为`assumed/unverified`；不能计成已实证网络隔离的Goodput。

### 本轮工程验证尝试（不计入任务分母）

| 尝试 | 结果与归档 |
| --- | --- |
| release-1 | build通过，test因旧schema负例失败；`observability-p0-release-1`保留原FAIL |
| release-2 | build/test/race/lint/integration五项通过；`observability-p0-release-2` |
| release-3 | A ResponseOnly观测选择修正后五项重新通过；`observability-p0-release-3`，本节真云Runner |
| Perfetto工具链smoke | 实际导入样例/SQL/点击通过，仅证明工具可用；初次浏览器下载失败也保留在`perfetto-ui-environment-1` |
| 真实Trace导入第1/2次 | FAIL，保留`observability-p0-perfetto-1`与`-2`；第1次另有checker空ID选择问题，第2次已修checker但产品flow ID仍被拒绝 |

完整版本、命令、退出码、导出器修复及后续UI证据见[验证与复现归档](observability/VALIDATION_AND_REPRODUCE.md)。没有因测试工具出错重新录制或删掉已有失败。


<a id="r15-current-paired"></a>

## 历史 R15：旧计量版本 E2B A baseline/L0 四对

固定559文件源码、同一 E2B A placement、同一 Tape、并发1，按 baseline→L0、L0→baseline、L0→baseline、baseline→L0 四组顺序完成8次真实 Strict Replay。每次6条exec、12项独立Verifier通过，Stop/revoke confirmed；计划/接纳/执行/完成/验证成功均8，失败与未运行均0。实际创建16个Sandbox，逐项provider NotFound，cleanup_errors为空；未调用上游模型。执行时间为2026-10-24 21:48–21:54（北京时间）。

仅比较双方均开启原生身份协议时的可选L0增量；不含身份机制绝对成本。baseline模型轮/帧原始观测为null，不能把Tape预期7轮/735帧填成实测；L0四次均实际观察7轮/735帧。共享Go进程CPU/GC/分配不能拆成Runner、ModelProxy、collector独立开销。四类L0 JSONL计数为实际Write/Sync调用，baseline关闭时值为null，第五类L1资源writer未启用。区域、provider底层缓存与冷热状态unknown；四对样本无预先批准阈值，不宣布扰动合格、负开销、p99或供应商排名。共享控制器计量两边同开；这些数字不能与扩盘前旧B样本合并。

### 八阶段原值（秒）

表中阶段名称与[阶段定义](#阶段与指标定义)完全一致；排队保留到纳秒换算的9位小数，避免四舍五入成“零”。各阶段不能相加代替Attempt总时长。

| 执行序号 / pair / 条件 | 排队 | 任务 Sandbox 创建 | 工作区准备 | Harness 启动 | Runtime 执行 | 停止与撤权 | 独立验证 | 清理 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 / 1 / baseline | 0.000009044 | 1.147 | 2.250 | 7.308 | 19.927 | 5.588 | 5.295 | 0.771 |
| 2 / 1 / l0 | 0.000009466 | 1.014 | 2.288 | 7.424 | 19.504 | 5.601 | 4.141 | 0.770 |
| 3 / 2 / l0 | 0.000010610 | 1.092 | 1.687 | 7.223 | 19.470 | 5.574 | 4.131 | 0.766 |
| 4 / 2 / baseline | 0.000013732 | 0.602 | 1.971 | 7.453 | 19.590 | 5.560 | 4.850 | 0.824 |
| 5 / 3 / l0 | 0.000011260 | 0.868 | 2.169 | 7.361 | 19.485 | 5.546 | 4.906 | 1.278 |
| 6 / 3 / baseline | 0.000011433 | 0.643 | 1.710 | 7.365 | 19.411 | 5.550 | 4.958 | 0.762 |
| 7 / 4 / baseline | 0.000009790 | 1.044 | 2.083 | 7.513 | 19.912 | 5.570 | 4.571 | 0.767 |
| 8 / 4 / l0 | 0.000013928 | 0.969 | 2.120 | 7.250 | 19.835 | 5.587 | 4.771 | 0.758 |

### 四对差值：L0 减 baseline

每格为“baseline → L0（差值）”，单位秒。差值由未舍入的原值计算，因此可能与表中已舍入数字直接相减不同。验证延迟从计划到达计至独立验证完成；Attempt时长还包含清理/收尾，二者不是Runtime执行阶段。

| 指标 | pair 1 | pair 2 | pair 3 | pair 4 |
| --- | ---: | ---: | ---: | ---: |
| Runtime 执行 | 19.927 → 19.504（-0.423） | 19.590 → 19.470（-0.120） | 19.411 → 19.485（+0.075） | 19.912 → 19.835（-0.076） |
| 验证延迟 | 42.379 → 40.834（-1.545） | 40.800 → 39.991（-0.809） | 40.450 → 41.110（+0.660） | 41.530 → 41.416（-0.114） |
| Attempt 时长 | 43.151 → 41.617（-1.534） | 41.624 → 40.771（-0.853） | 41.213 → 42.405（+1.192） | 42.297 → 42.187（-0.110） |

Runtime差中位数−0.098秒（范围−0.423至+0.075）；验证延迟差中位数−0.461秒（−1.545至+0.660）；Attempt差中位数−0.482秒（−1.534至+1.192）。仅四对，未估置信区间；负差值不能解释为“观测使系统变快”，没有合格阈值也不能宣布成本合格。

### 共享进程 CPU、分配与 GC

每格仍为“baseline → L0（差值）”。CPU是控制器SELF user+system区间累计，Go堆/GC也是Runner+ModelProxy+Go采集器共享值；不含独立Docker Harness、Adapter或Sandbox，不拆分各part。MiB=1,048,576字节；RSS是各次进程生命期峰值之间的比较，绝非同次终值减起值的区间峰值。

| 指标 / 单位 | pair 1 | pair 2 | pair 3 | pair 4 |
| --- | ---: | ---: | ---: | ---: |
| SELF CPU / ms | 646.943 → 703.022（+56.079） | 651.752 → 717.179（+65.427） | 644.606 → 664.211（+19.605） | 658.481 → 703.191（+44.710） |
| 累计分配 / MiB | 111.607 → 115.401（+3.794） | 113.044 → 115.534（+2.491） | 110.972 → 115.786（+4.813） | 112.274 → 115.571（+3.297） |
| 分配次数 | 984063 → 989857（+5794） | 984097 → 987441（+3344） | 984255 → 989102（+4847） | 984681 → 989156（+4475） |
| GC次数 | 36 → 28（-8） | 44 → 25（-19） | 37 → 27（-10） | 42 → 27（-15） |
| GC暂停累计 / ms | 4.894 → 3.376（-1.518） | 3.107 → 1.955（-1.152） | 4.836 → 3.235（-1.601） | 5.170 → 13.769（+8.599） |
| 生命期最大RSS / MiB | 73.422 → 62.156（-11.266） | 59.523 → 78.465（+18.941） | 68.398 → 63.273（-5.125） | 59.348 → 72.145（+12.797） |

SELF CPU差为+19.605至+65.427ms，中位数+50.395ms；累计分配多2,611,584至5,046,920字节。GC次数减少不等于观测改善：采样为进程总量，调度/GC时机和环境变动仍在。完整user/system、堆起止及外层GNU time分别保留，最终资源报告写盘不包含在SELF终点内。

### 四类 JSONL 实际文件调用

下面只列L0已启用文件；每格为“Write返回次数 / 实际返回字节”。四个writer各次Sync均started=returned=1，Write started=returned，未见短写/错误/非法返回；这不是物理磁盘写入次数。baseline全部为not_enabled/null，故文件I/O的成对差值也是null，不用0相减。第五类Sandbox metrics属于L1，本轮双方关闭。

| 文件 / 计量范围 | pair 1 L0 | pair 2 L0 | pair 3 L0 | pair 4 L0 |
| --- | ---: | ---: | ---: | ---: |
| Runner events | 48 / 33,253 | 48 / 33,252 | 48 / 33,253 | 48 / 33,245 |
| Model request-boundaries | 815 / 676,843 | 815 / 676,843 | 815 / 676,843 | 815 / 676,843 |
| Model旧events | 768 / 441,348 | 768 / 441,354 | 768 / 441,348 | 768 / 441,349 |
| E2B operations | 300 / 187,359 | 298 / 185,558 | 296 / 184,043 | 300 / 187,459 |

上述范围不含index/manifest/late、native身份文件、Bridge审计、其它进程I/O或报告自身。只有Runner与model-boundary显式drop字段为0，不能推广为全链零丢失。全native调用分母仍unknown，A工具原生时长未采；baseline缺源和L0 native缺测的Snapshot/Trace均保持incomplete、exit1。机器级proc/PSI背景采样已归档，区域/缓存未知不改写为同冷热。

证据 `/opt/aries-artifacts/observability-r15-paired-2/evidence.json`，SHA `3fb5a096b2ee3253757ef93e05a1691d28a32fed37d4272faea1c78ea90df37a`；同目录 `paired-analysis.json`，SHA `031ae0159d92c4eb0511bf2ff1b5541f4f561b665876ecfed0b5bddb59999c0d`。绑定559文件manifest
`f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`、固定Adapter/native和原Tape；
执行driver SHA `7a9377b0a03e3a79707fc70ae2d6310ee03e199a2d1075fa53fe82c73b29eb28`。
独立394项原件/算术对账：`/opt/aries-artifacts/observability-r15-paired-review-1/evidence.json`，SHA `d182fba9e138b69529ffdff91d12138fa5c822f6045844d9e351d7acfee47286`；它没有新增运行或模型调用。
逐run ID、二进制/image SHA、脚本fixture和全部尝试见[验证档案](observability/VALIDATION_AND_REPRODUCE.md#r15-paired-validation)。

四条L0 Trace已实际导入原生TraceProcessor，每条55个区间/43条flow/19个来源，operation起点、时长、终点及flow端点逐项一致，导入误差0ns、processor errors为0。四条baseline因可选观测关闭而SKIPPED，不把空导出算时间线通过。原始文件SHA未变，native A计时缺测与incomplete保留；新UI交互UNRUN。证据 `/opt/aries-artifacts/observability-r15-paired-perfetto-1/evidence.json`，SHA `1b4b992f912783f05b9927b4134676f80a1e27cf5b3b30c7e70a44b05e4c826b`，仅为 `PASS_FOUR_L0_OFFLINE_PERFETTO_ONLY`，无新增云/模型调用。

<a id="r15-观测扰动"></a>

## R15：历史冻结源码的观测级别扰动

这六次使用同一 E2B B、同一 Strict Replay Tape、`merge-ranges`、并发 1、单 attempt 与 recorded 时序。源码是 `current-source-validation-1` 的归档版本，**不覆盖后来的 R01/R05/R07 或原生输出修订**。六个任务的真实工具、独立 12 项测试、Stop/revoke 和正常清理均有原始结果；不能与华为 B 历史基线拼成供应商对比。

| 执行顺序 / 级别 | 任务总耗时（秒） | Runtime 执行（秒） | 独立验证（秒） | Runner CPU（秒） | 最大 RSS（KiB） | SDK 资源快照 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 1 / baseline | 45.048 | 15.554 | 4.353 | 0.53 | 31216 | not_run |
| 2 / L0 | 43.978 | 15.844 | 5.028 | 0.65 | 34340 | not_run |
| 3 / baseline | 40.072 | 15.543 | 3.539 | 0.53 | 30772 | not_run |
| 4 / L0 | 39.214 | 15.866 | 5.300 | 0.57 | 35084 | not_run |
| 5 / L1 | 43.617 | 15.943 | 4.956 | 0.66 | 32712 | 12 次 SDK 快照 |
| 6 / L2 | 45.242 | 16.141 | 4.646 | 0.65 | 35108 | 12 次 SDK 快照 |

Runner CPU 是 `/usr/bin/time` 的 **user + system**；此前本页只填 user 却标作 CPU 总时间，现已纠正。RSS 和 CPU 都是控制器进程口径，不是 guest 工具资源。SDK 快照数不表示厂商底层有相同数量的独立采样。Runtime 和独立验证只是八阶段中的两段，不能相加替代任务总耗时。

原驱动有错误的 run-result 查找路径且未强制检查退出码；最终 evidence 是后补 seal 对账，不是原驱动完整验收。本次再次只读核验六份结果、model session、Verifier XML 与候选 SHA。baseline 没有原始模型事件；`provider_resources_expected_absent=true` 是 seal 常量，不能当作云 API 删除确认。详细复现与限制见[六条实际记录](observability/VALIDATION_AND_REPRODUCE.md#r15-历史冻结组合的六条实际记录)。

L0/L1/L2 中有 drop 字段的 model/performance 索引为 0；E2B operation/native 索引没有该字段，保持 unknown。baseline 不创建可选观测文件，不能说 drop=0。L2 实际与 L1 相同，未启用 profiler。样本少、冷热未控制、没有预设扰动阈值，**不能据此宣称观测成本合格、p99 或性能排名**。后续新组合的E2B A四对已在上节单列，不能回填为本组旧B的验证。

此组六次 evidence：`/opt/aries-artifacts/observability-r15-current-1/evidence.json`，SHA `bd321f265c829ca34d2b7bf458d20570003cb54f4086c853fa84a693929dec1a`。更早 release3 的六次耗时为 baseline 46.218/47.546s、L0 47.192/46.027s、L1 48.823s、L2 39.745s；保留在 `observability-r15-1`（SHA `f4485eb559560491bb70413059b31f3bac345792e16763c585692e377ce44adb`），初次 baseline panic 失败也保留，不能与上表合并为同版本样本。

<a id="huawei-current-failure"></a>

## 修复前华为 A：stdin 关闭错误导致回放失败

当时 R05 gate2 Runner、固定 native 镜像与新 Tape `d716431e…` 的首次华为 A 回放：
计划/接纳/执行/完成均1，失败1、验证成功0。模型只完成1轮/29帧；首个题目命令未执行。
工作目录校验实际 exit0，但 SDK 关闭 stdin 失败使 Bridge 返回255，随后 Strict 拒绝真实
ToolResult 偏离。没有改目录或放宽 matcher；[原件与定位](observability/VALIDATION_AND_REPRODUCE.md#r01-huawei-current-failure)单独保留。

单位：秒。仅为失败诊断，不能据较短 Runtime 得出华为更快的结论。

| 阶段 | 华为 A / recorded / FAIL |
| --- | ---: |
| 排队 | <0.001 |
| 任务 Sandbox 创建 | 0.638 |
| 工作区准备 | 1.170 |
| Harness 启动 | 7.462 |
| Runtime 执行 | 4.881 |
| 停止与撤权 | 5.417 |
| 独立验证 | 3.402 |
| 清理 | 0.535 |

验证实际12项，11失败、1通过、score0；停止/撤权confirmed。两个Sandbox和临时模板
均已确认删除，清理无错误。评分出网隔离仍采用假设；完整观测窗口24.009秒，验证成功
吞吐为0。原运行未保存SDK异常类别；后续独立探针已复现进程先退出、关闭stdin返回NotFound的机制，并完成最小修复的两SDK定向回归，完整云复测另列。通用“workdir unavailable”不能当作目录不存在的证据。

<a id="current-fault-validation"></a>

## 当前停止隔离与观测挂起实验

这些是故障验收，不能作为正常 Strict Replay 的性能样本。A断连用例没有完整Agent loop；
B使用Scripted Mock触发停止故障；R05两例最终被实验owner中断，缺少完整run-result。

| 实验 | 已证实的行为 | 资源与结果口径 |
| --- | --- | --- |
| R07 A：SSH断连 | 断连后父子进程仍活；已有Bridge.Stop清空cgroup、进程消失、心跳停止、SSH监听撤销 | 1个Sandbox已NotFound；组件验收PASS，无Verifier，不算完整任务 |
| R07 B：Stop不可用 | 4次定向Stop故障；Runner将隔离/评分标为blocked_isolation，Verifier调用0次 | 1个Sandbox最终NotFound；原任务Cleanup失败保留，不能用最终删除改写为成功 |
| R05：model boundary fsync挂起 | 两个Sandbox及Harness已清理，尚有1个run network，由实验owner回收 | workload中断、Observer不完整；仅故障收容PASS |
| R05：Recorder fsync挂起 | Sandbox、Harness和run network均已清理 | workload中断、Observer不完整；仅故障收容PASS |

R07 B可用的阶段原值如下；排队未在该组件fixture采集、Verifier被阻止，均写“—”，
不把结果文件中默认的验证duration=0当作执行耗时。单位：秒。

| 阶段 | E2B B / Stop故障 |
| --- | ---: |
| 排队 | — |
| 任务 Sandbox 创建 | 1.308 |
| 工作区准备 | 1.435 |
| Harness 启动 | 14.949 |
| Runtime 执行 | 45.001 |
| 停止与撤权 | <0.001（注入立即失败） |
| 独立验证 | —（被阻止） |
| 清理 | 1.017（原任务状态failed） |

R05两例各有7次Strict请求、735帧及12项独立测试原始证据，但最终index/run-result缺失，
不补造八阶段。TERM后超过5秒仍停在精确目标fsync，实验随后中断；不是生产Close有界
或内核D-state已解决。R05共4个Sandbox全部独立NotFound。来源与退出条件见
[R07验收](observability/VALIDATION_AND_REPRODUCE.md#r07-current-isolation)和
[R05验收](observability/VALIDATION_AND_REPRODUCE.md#r05-fsync-containment)。

## 增量工程验证（不计入任务分母）

以下验证解决身份、可靠性或输出语义问题，没有完整执行 Agent→模型→真实工具→独立 Verifier 全流程，故不提供虚构的八阶段耗时，也不计入开头的整体成功任务数。

| 验证对象 / ECS 归档 | 已执行结果 | 验收边界 |
| --- | --- | --- |
| 当前L1清理/结果分离：`observability-r05-cleanup-release-1` | 五文件修复后的同源五项Go门禁PASS；先取消采样并保存final RPC，再删除Sandbox，最终真实join和写盘另收尾；observer错误不再污染业务结果 | 工程验收0云/模型，永久文件I/O仍BLOCKED；[复现与SHA](observability/VALIDATION_AND_REPRODUCE.md#r05-cleanup-current)，不替代实际L1云回归 |
| 当前R01协议：`observability-r01-protocol-cloud-review-4` | 5次真实SSH请求：取消/重试两次接受，旧invocation/错误scope/session三次拒绝，SDK/guest身份精确对应 | 两轮共2个Sandbox已回收；首轮fixture FAIL保留；无LLM/Verifier，不证明完整Agent取消或进程树终态；[证据](observability/VALIDATION_AND_REPRODUCE.md#r01-current-protocol) |
| B运行时与离线scope修复 | 568文件运行时五门禁PASS；分析器scope修复unit/race各209项、vet/build PASS | 全量runtime gate与后续离线分析器源SHA分别保留；真实任务只计上方1次；[复现](observability/VALIDATION_AND_REPRODUCE.md#r07-huawei-b-native) |
| B真实factory/logger：`observability-r07-native-gateway-sink-run-1` | 原5组对照PASS，27条callback日志精确一致；此组exit→最后自然EOF 313.661219ms | 两容器已清理，0云/LLM；未运行完整Agent，日志成本未作配对；[证据](observability/VALIDATION_AND_REPRODUCE.md#r07-native-process-snapshot) |
| B统一Snapshot：`observability-r07-native-snapshot-1` | performance包unit/race各58顶层/207含子项、vet和分析器build PASS；3生命周期/1排空区间，保留独立时钟 | 用真实27条metadata及明确模拟的native start解析fixture；不是云B新测点验收，不增加任务数；[版本与边界](observability/VALIDATION_AND_REPRODUCE.md#r07-native-process-snapshot) |
| 当前B原生观测：`observability-r07-native-observer-poc-run-1` | 原版/候选5对业务对照一致；27个真实回调；直接子进程退出至最后pipe EOF实测311.383578ms | ECS本地两容器已回收、0云/LLM；该原始PoC仅私有候选，后续logger/Snapshot进展见上两行；[范围](observability/VALIDATION_AND_REPRODUCE.md#r07-native-observer-poc) |
| Strict 拒绝摘要：`observability-model-rejection-diagnostic-2`、`model-rejection-consumers-1` | modelreplay、performance、app 的 unit/race 通过，模型包 vet 通过；首轮测试哨兵误报 FAIL 保留 | 只增加私有结构诊断；未部署或新云回放，额外 caller 仍未知 |
| Stop 日志：`observability-p0-stop-log-follow-2` | ECS 本地 Docker 正常停止、observer 断流、16MiB 上限三例通过；后两例采集仍 partial | 只证明外部取证管线；真实 OpenClaw/云 driver 接入仍 UNRUN，0 Sandbox/模型调用 |
| R01 协议：`observability-r01-protocol-1`、`protocol-review-1`、`cloud-binding-1`、`python-binding-1` | 元数据校验、session隔离、重复/冲突与进程身份定向测试通过 | 分层测试，不能代替完整 Agent |
| R01 真实 E2B：`observability-r01-native-cloud-1` | 9/9 native→SSH→Bridge→Process→guest 身份精确映射；1 个沙箱 TTL 300 秒，已确认删除 | 无 LLM/独立 Verifier；同命令 guest 实际并发区间未证明 |
| R01 后台重叠：`observability-r01-overlap-1` | 相同命令真实同时存活、反序退出；2/2完整身份链、双流字节/退出码一致 | 1个Sandbox已回收；0模型，无独立Verifier，不算后台Strict回放；8次辅助调用保持unlinked |
| 原生返回差异：`observability-r01-native-poc-3` | 9/9 身份与双流/退出码通过，最终结果仅 8/9 一致，**FAIL 保留** | 原版 stdout/stderr 合并顺序不确定 |
| 根因复现：`observability-r01-native-stream-order-1` | 未修改 OpenClaw 镜像四次为 outerr/errout/outerr/outerr；双流和 exit 均相同 | 证明存在差异，不估计概率，不改 Strict matcher |
| 输出候选：`observability-r01-output-order-6` | 5 组 helper、B 原生 4 项、A 固定 26 项：25 项预期行为通过，1 项原版生命周期 KNOWN_LIMIT | 保留后台/process/yield/PTY；只修实际前台非 PTY final。该隔离证据不含新 Tape；后续 full-agent-2 单列通过 |
| 撤回工具限制：`observability-r01-native-tool-policy-1` | A/B renderer 开关前后完整原生配置逐字节相同，UT/race 通过 | 不等于真实 native 模型 tools schema 已导出对账 |
| R15 新计量：`observability-r15-metering-1` | ECS 四 package unit/race/vet通过；共享进程资源起止和两个JSONL实际Write/Sync计数 | 无真实模型/云调用，不计入成功任务数；后续完整门禁另见r15-release-1，其它I/O仍未覆盖，真实配对另列 |
| R15 计量补齐：`observability-r15-metering-2` | ECS 六 package unit/race/vet通过；增加模型旧事件、SDK操作、L1资源三类JSONL计量，并验证新旧索引兼容 | 无真实模型/云调用；累计五类writer有明确scope，仍不等于全部I/O或成对性能结果，不计入成功任务数 |
| R15 同源完整门禁：`observability-r15-release-1` | 559文件源码的build/test/test-race/lint/integration均PASS；源码/fixture前后未变、进程组为空、容器/网络盘点一致 | 仅Go工程门禁，云创建/付费模型调用0、生产未部署；后续真实paired-2单列，不把本次门禁计入成功任务数 |
| 门禁候选中断验证：`observability-r15-driver-signals-1` | TERM/HUP各1场景：真实抽取函数处理受控异常、保存FAIL、四个自有PID/PGID消失 | 未执行候选完整main/Go门禁/Docker，不证明复杂后代、重复信号或文件I/O有界退出 |
| R05：`observability-r05-reliability-2` | 22 项故障矩阵：17 项限定断言 PASS、4 BLOCKED、1 UNRUN；当时源码五项门禁通过 | 包含阻塞反例，普通 Write/Sync 有界停止、崩溃持久性仍 BLOCKED |
| R05 错误分离：`observability-r05-observer-separation-2`、`r05-cli-observer-1` | 同源五项门禁通过；实际 CLI 正常/观测文件冲突各7轮735帧一致、退出0；历史偏离返回409并退出1 | 首轮临时路径过长失败保留。注入文件错误与 CLI 发布冲突分开计证据；零云/模型调用，不计入成功任务数，不表示有界I/O或新云组合通过 |
| R07：`observability-r07-semantic-1`、`python-binding-1` | 31 项定向测试、20 次原版/候选真实进程语义对照；后续身份版本再次对照 | 退出观察与排空分开；不代表 kernel 时间、全部后代终态或 B 工具 |
| R07 真实SDK：`observability-r07-sdk-real-1` | 20次执行/10组旧新语义对照PASS；真实timeout/kill后child存活反例保留 | 1个Sandbox最终独立NotFound；只验wrapper语义和边界，不代表全树停止或R14通过 |
| 历史组合门禁：`observability-r01-combined-release-1` | build/test/race 通过，lint 因 3 处测试 struct literal 失败 | keyed literal 后续修正；该次FAIL保留，不代表下行R01冻结组合状态 |
| R01 冻结组合：`observability-r01-combined-release-3/reconciliation.json` | 同源五项 Go 门禁、新 Adapter/SDK、5 组 helper、4 项 B 原生与26项 A 矩阵通过各自断言；25项应走SSH的身份逐项核对 | 1项原版生命周期缺陷仍 KNOWN_LIMIT；不是完整 Agent 回放或扰动实测；构建/fixture原FAIL保留 |
| 身份 producer 故障：`observability-r01-producer-failures-2` | 21 项 helper 用例、旧 helper ENOSPC 反例、私有 tmpfs 真实满盘用例均符合预登记断言 | 新实现写日志失败仍保留真实工具双流与 exit7；三容器已回收。普通文件阻塞、存储全失败留下旧状态仍未解决；不是完整 Agent 回放 |
| 身份采集故障隔离：`observability-r01-native-observer-2` | Go 定向 unit/race/vet 通过 | 新增身份采集错误只计入 Observer；业务与 Stop/revoke 结果独立。仅覆盖该冻结源码，不代表最新合并组合 |
| 身份快照：`observability-r01-snapshot-identity-3` | 定向 unit/race/vet/build 通过；缺失/冲突/重试与旧快照兼容负例保留 | 只投影可信身份和归档 exec 分母，不增加时间线；缺失计数为 null，A native 计时仍未采集 |
| 最新分析器重导：`observability-r01-snapshot-reexport-1` | 原E2B A/B、华为A实际TraceProcessor导入，区间60/217/60、flow47/31/47，端点与单位一致；旧文件SHA不变 | 只有离线回归；A incomplete、B coverage unknown保留；未新增浏览器UI验收或任务运行 |

Stop 日志 fixture 的真实工程计时如下，使用 ECS owner 单调时钟，单位毫秒；不是 Agent/Sandbox 八阶段，也不用于云厂商比较。原始证据、失败检查器及清理记录见[停止日志取证](observability/VALIDATION_AND_REPRODUCE.md#p0-stop-log-follow)。

| 工程用例 | Stop 调用 | Stop 返回→Remove 返回 | 日志字节 | 采集状态 |
| --- | ---: | ---: | ---: | --- |
| 正常 SIGTERM | 230.205 | 47.258 | 334 | 固定标记齐全、流正常关闭 |
| observer 提前断流 | 246.116 | 47.043 | 123 | partial |
| 达到 16MiB 上限 | 245.249 | 47.947 | 16,777,216 | partial |

第二个区间包含 CLI 调度，不是引擎内部删除耗时。两轮合计 6 个本地容器和 6 个 follower 均已回收，首轮大小写误判的 FAIL 原件保留。

历史R07后代持管道场景的候选计时如下，均从同一guest wrapper起点测量，单位毫秒。只用于解释边界，不加入八阶段或供应商性能比较。

| 观察点 | 起点后时间（ms） |
| --- | ---: |
| Popen返回 | 0.361508 |
| 观察到直接子进程退出 | 407.731501 |
| 首次非空输出 | 1408.569995 |
| 双流pipe EOF | 1409.275010 |
| 生成终态帧前（guest_duration_ns，含排空） | 1409.285299 |

子进程退出观察至EOF相隔 **1001.543509ms**，即后代仍持管道约1秒。timeout/kill四次取证均发现child存活，因此取消后完整进程终态仍BLOCKED；最终Sandbox已删除。R01重叠与R07 SDK的SHA、原始字段、执行命令集中在[真实重叠](observability/VALIDATION_AND_REPRODUCE.md#r01-live-overlap)和[真实SDK对照](observability/VALIDATION_AND_REPRODUCE.md#r07-real-sdk)，两者均不增加完整任务成功数。

旧 native-poc-1 的 patch anchor 失败、output-order-1 的 probe 语法失败、output-order-2 的退出字段差异、output-order-5 的独立 Node probe 提前退出均保留；output-order-4 是旧强制前台配置的局部证据，不能覆盖保留原生能力后的版本。每个归档的 SHA、源版本与退出条件统一列在[验证档案](observability/VALIDATION_AND_REPRODUCE.md)，不将所有局部 PASS 拼成 P0 全链通过。

R01 冻结组合汇总 SHA `8812dd96d58c8fdf4313c112d5ea47e72f9eef46e34ec6d16017ee4358c96117`；
同源 manifest SHA `01660084b9bfaa900f8236429a7407472e2d20cac36213b8a8adfa2c48a595c6`。
重导证据 SHA `c7d3c4d23e55a935dee7cafbc02ac4b6159bc709647e677864a112d47a611e2b`。
该组合的原始 integration 失败（缺固定数据集）、禁网构建缺缓存及 native 文件权限
失败均单独留档；恢复后源码哈希未变，成功汇总没有覆盖失败原件。

最新 `observability-r15-release-1/evidence.json` SHA
`b0260a73807e0771217010fffd6cfe983a25f8cf6999eaca21394496eb69f2db`；source manifest SHA
`f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`。
五项工程检查分别耗时10.20/26.81/71.61/3.13/375.99秒（build/test/test-race/lint/integration），
**不是云任务阶段耗时或新性能样本**。首次SCP kex失败、远端脚本不存在且未启动检查的记录仍保留。
信号候选独立证据 SHA `42c0aaf0f177597497a055608ac9e45c4779aece831a9f0a5168a03b2cd5836f`，
仅验证抽取的step等待期间TERM/HUP，不借原驱动成功声称候选完整脚本通过。
二进制/驱动/日志SHA及传输失败原件统一见[验证档案](observability/VALIDATION_AND_REPRODUCE.md#r15-metering-release)。

2026-10-24 ECS 系统盘扩容后，根文件系统由 40G 扩至 99G，可用 58G、使用率 39%；服务未重启。证据 `ecs-rootfs-expand-20261024T100351Z/evidence.json`，SHA `d518f6fdd8704a1526c9b2dbf9a4f3c21eecee67dc9d39a0118f203fd2bcae0e`。这是实验环境变化，之后的成对实验须重新冻结环境，不能把扩盘前后结果冒充同条件重复。

上述 producer 故障证据 SHA 为 `19025d9fbe750e1d1e81696af2ae124df363be121e2574f218371ef90f9f5f74`，Go Observer 证据 SHA 为 `eccdc505ef2168bb9d5cdeb7085f93b59de33a606335fe259d4a69e57d6526c1`；两者均位于对应 ECS 目录的 `evidence.json`。故障测试的通过只证明所列失败被正确处理，不能填入任务八阶段耗时或算成吞吐样本。

身份快照证据 SHA 为 `d1ac7e1116a59c9a0c1a42cc8de6faf1a8e872589d6fde1de3855f4f4f2078b4`。新的 `native_tool_identities` 只说明哪些已归档调用能精确关联，不能据此补造工具执行耗时或跨域网络延迟。

<a id="portable-crosscloud"></a>

## 同一 Tape：E2B A 与华为 A

本组固定 `merge-ranges-portable-v1`、外置 A、OpenClaw 2026.7.1、2 vCPU / 4096 MB、并发 1。先真实录制一次，再分别在 E2B、华为真实 Sandbox 回放同一份 Tape：**8 轮模型交互、7 次工具调用、12 项独立测试**；两次回放均采用 recorded 时序。Tape SHA-256：`07be09fa95cb3678c94c43accdc4b96ae079d6ae2352505091eeff134e6065bd`。

portable-v1 在执行前明确题目指令：读取源码/测试、运行原生 pytest、修复并复测，避免与题目无关的文件系统探测。只改变题目指令与 README；初始源码、公开/隐藏测试、独立评分语义不变。**没有修改旧 Tape、过滤工具输出或放宽完整请求匹配。** 旧 `ls -la` Tape 仍保留失败状态；新结果证明这条共同工作负载可跨云复现，不代表任意旧轨迹都可移植。

下表 record 是来源参考；性能对照应读两列 recorded。每列只有一次真实任务，区域、云网络与底层实现不同，不能用它们估计稳定收益、容量或 p99。

| 阶段 | E2B A portable record（秒） | E2B A portable recorded（秒） | 华为 A portable recorded（秒） |
| --- | ---: | ---: | ---: |
| 排队 | <0.001 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.536 | 0.985 | 0.608 |
| 工作区准备 | 2.894 | 2.678 | 1.313 |
| Harness 启动 | 7.327 | 7.349 | 7.072 |
| Runtime 执行 | 21.173 | 20.567 | 18.635 |
| 停止与撤权 | 5.554 | 5.489 | 5.353 |
| 独立验证 | 6.527 | 4.465 | 2.248 |
| 清理 | 1.776 | 1.696 | 0.858 |

| 指标 | E2B A portable record（秒） | E2B A portable recorded（秒） | 华为 A portable recorded（秒） |
| --- | ---: | ---: | ---: |
| 产物延迟 | 39.890 | 38.192 | 33.732 |
| 验证延迟 | 45.782 | 42.138 | 35.687 |
| 完整观测窗口 | 47.588 | 43.847 | 36.555 |
| 模型请求累计 | 12.866 | 12.862 | 12.864 |
| 原生工具累计 | 3.253 | 3.100 | 2.383 |

下表 T1–T7 由相同 Tape 的 ToolCall 身份与参数对应，**两次失败测试 T2、T4 完整保留**。这里的 A 原生工具耗时仍包含 ECS Harness、SSH/Bridge、Sandbox 执行和结果回收，不能称为纯 Sandbox 执行耗时。

| 工具调用（同一 Tape） | 原生结果 | E2B A portable record（毫秒） | E2B A portable recorded（毫秒） | 华为 A portable recorded（毫秒） |
| --- | ---: | ---: | ---: | ---: |
| T1 读取源码与测试 | 成功，退出码 0 | 353 | 323 | 187 |
| T2 首次 pytest | 错误，退出码 1 | 662 | 778 | 641 |
| T3 第一次修复 | 成功，退出码 0 | 359 | 318 | 196 |
| T4 pytest 复测 | 错误，退出码 1 | 585 | 481 | 528 |
| T5 第二次修复 | 成功，退出码 0 | 343 | 324 | 176 |
| T6 pytest 复测 | 成功，退出码 0 | 551 | 514 | 469 |
| T7 只读检查源码 | 成功，退出码 0 | 400 | 362 | 186 |

验收要求最后一次指定 pytest 成功；随后仅允许成功的原样 `cat intervals.py` 只读检查。最初自编实验脚本错误地要求 pytest 必须为最后一次工具调用，原失败记录仍保留。这是看到结果后的验收脚本修正，不声称全部判据都在实验前冻结。修复这个检查只纠正题目未要求的限制：**所有 7 次调用、完整 Tool Result、错误状态和退出语义、整段历史、675 个 SSE 帧的录制时序、最终产物和独立验证仍逐项检查**。复用原录制只是复用未改动证据，不能替代重新验收；也不重复计算一次真实录制。另直接检查了实际命令：T3/T5 仅写 `intervals.py`，未改测试、未过滤输出；T5 写入内容与最终候选字节一致（SHA-256：`928cc235158526876c2ea1b7b6ea9817ef8d38549f01129403bd53af649ec1f8`）。

三次任务均确认停止、撤权、独立评分与资源回收；华为评分禁止出网依然采用显式假设。已有分析器各导出 741 个 Trace 事件，覆盖受支持的 Runner/模型来源；本组 A 原生工具明细来自单独核对的原生产物，**尚未合入通用 Trace**。完整导出不代表所有部件已采集或时钟已对齐。

ECS 汇总：`/opt/aries-artifacts/portable-crosscloud-review-2/`，含 `metrics.json`、`evidence.json`、输入哈希及三组 `summary.json` / `trace.json`。复核证据 SHA-256：`57c4daaa328fc7b69dc91ff94136f5bfbb4007d2fca9b9f9cf9950f556999176`。真实来源为 `/opt/aries-artifacts/portable-e2b-1/record/`；接受的只读复制与 E2B 回放在 `/opt/aries-artifacts/portable-e2b-3/`，华为回放在 `/opt/aries-artifacts/portable-huawei-replay-2/`。

<details>
<summary>构建验证、检查器失败与修复记录（不计入云端任务耗时）</summary>

- `portable-workload-release-1` 失败于测试临时 Unix socket 路径过长，以及旧文件元数据测试中符号链接自身 atime 的不稳定断言。保留证据 SHA-256：`050eb1e44f9b84672c687dc662e5c76f0985cc3fbe5e6039020ac7122d60d806`；三次诊断表明无实际修改也会改变该链接自身 atime。修正测试后仍校验目标文件/目录元数据与内容、链接不被改指；产品处理代码未借此改动。
- `/opt/aries-artifacts/portable-workload-release-2/` 在 ECS 完成 build、test、race、lint、真实 Docker integration，分别约 8.650、23.866、66.741、2.981、359.143 秒，全部通过。证据 SHA-256：`f70d254b15543dba8303cadc52c2cdb329638741f81ca616a237d65410c44636`。本组 Runner 镜像固定为 `sha256:10d53023dac38f0761197e6c5d84fa744d96bdfb16bb528d35fc7a3c557ba026`。
- `portable-e2b-1` 的真实录制与独立评分成功，但自编验收脚本末次命令断言失败；原证据 SHA-256：`86ae57cdc2942bcc62a59108e17617ab0e7a0a877e54746f1257bfb1f74879a0`。后续只读复制保留逐文件哈希及来源。
- `portable-e2b-2` 离线导出误用不支持 `--trace` 的较旧 CLI，未产生新的云端任务或模型调用。随后统一采用已验收分析器 `/opt/aries-artifacts/trace-export-check-1/aries-perf`，SHA-256：`d35964c25ccc134652cdac720fcc024cc44339063f0d1ebdc1a166ff0db7d18c`。不得把它的导出能力记作本轮 Runner 新增功能。
- `portable-huawei-replay-1` 在创建任何模板/沙箱前因 OSError 终止，原 errno 未记录，原因未证实。后续预检采用与服务一致的地址复用行为；独立临时端口实验验证 TIME_WAIT 可复用、真实占用端口仍拒绝，随后才完成一次华为实测。未以重试隐藏已执行的失败任务。
- `portable-crosscloud-review-1` 的 E2B 生命周期字段读取错误使复核失败；保留原文件，在 `portable-crosscloud-review-2` 按实际证据结构复核。原始运行、Tape、工具结果未改动。

</details>

## 华为 A：真实录制通过

本轮 `agentsphere-external-record-1` 在远程 ECS 运行 Agent，在真实华为 Sandbox 执行工具，使用 DeepSeek 完成 **7 轮模型交互、6 次工具调用、12 项独立测试通过**。并发 1、单任务；7 项离线复核均通过。评分禁止出网仍为显式假设。

这是**新录制，不能当作复现了 E2B A 的旧轨迹**。新 Tape SHA-256：`902af9452e96e0b074d92088bee02ced661d154ce4e4fc3948d7639ea5d64085`。该 Tape 尚未在另一次华为 A 运行中回放，也未完成跨云对照。

| 阶段 | 华为 A record（秒） |
| --- | ---: |
| 排队 | <0.001 |
| 任务 Sandbox 创建 | 0.565 |
| 工作区准备 | 0.948 |
| Harness 启动 | 7.614 |
| Runtime 执行 | 19.826 |
| 停止与撤权 | 5.391 |
| 独立验证 | 2.385 |
| 清理 | 0.891 |

| 指标 | 华为 A record（秒） |
| --- | ---: |
| 产物延迟 | 35.176 |
| 验证延迟 | 37.133 |
| 完整观测窗口 | 38.053 |
| 模型请求累计 | 14.769 |
| 原生工具累计 | 1.991 |

下面 T1–T6 仅是这次新轨迹的调用编号。原生工具耗时包含 SSH/Bridge，不能称为纯 Sandbox 内执行或 CPU 时间。

| 工具调用（本次编号） | 原生耗时（毫秒） | 原生结果 |
| --- | ---: | --- |
| T1 | 245 | 错误，退出码 2 |
| T2 | 228 | 成功，退出码 0 |
| T3 | 200 | 成功，退出码 0 |
| T4 | 660 | 错误，退出码 1 |
| T5 | 150 | 运行时错误，退出码未知（null） |
| T6 | 508 | 成功，退出码 0 |

六次均为 `exec`；每次真实结果与下一轮模型请求逐字一致。T5 原生状态为 `failed/runtime-error`，不存在可用进程退出码，不能补成 0；底层原因尚未定位，记为遗留。Agent 保留错误结果后继续处理，最终通过验证。首次离线复核误要求所有工具都有整数退出码，其失败记录保留在 `analysis-1`；修正检查器后在新的 `analysis-2` 重新核验，未修改原始事件、Tape 或严格匹配规则。

停止与撤权已确认；任务与独立评分两只 Sandbox 均经 GET 404 确认删除，临时模板、ECS 容器、运行网络与临时凭据已清理。已有导出包含 **1036 个 Trace 事件**，支持来源完整；A 原生工具明细仍来自私有原生产物复核，通用 Trace 尚未合入其工具跨度，不代表新设计已实施。

ECS 归档：`/opt/aries-artifacts/agentsphere-external-record-1/`。`analysis-2/` 含 `review.json`、`metrics.json`、`summary.json`、`trace.json`、`evidence.json`；原始运行、镜像引用及执行脚本均保留。复核 SHA-256：`a000132cf63dcdff57d07afd816819a4ca74ec68bc836ba527e948fead96e74c`；分析 SHA-256：`3f4024721d8b743c08df75f067bf93de0423fda7b843c2161aec6ba6d89ec730`。本次复核和分析均未再创建云资源或调用模型。

## 华为 A：跨云严格回放失败

本轮 `agentsphere-external-replay-1` 原样加载 E2B A 的 Tape（SHA-256：`01d761e39ae8b4a20e491d821c3dbf35c5af16431ba407cdc89b024cca6fd3d3`），Agent 在 ECS，工具在真实华为 Sandbox 中执行。它是**失败样本，不能纳入完整轨迹的性能比较**。

前两轮模型响应和两次工具调用已执行。T1 与录制一致，返回预期错误；T2 的 `ls -la` 退出码为 0，但真实输出不同：E2B 为 `total 12`、目录大小 128/60，华为为 `total 20`、目录大小 4096/4096。第三轮模型请求因此触发 `REPLAY_DIVERGED`；后续四个工具未执行，不能记作耗时 0。

这证明当前 Tape 暴露了不同文件系统的元数据，不能直接跨云复现；不能修改输出或放宽匹配来制造通过。命令执行成功与回放结果匹配是两个不同条件。

| 阶段 | 华为 A recorded 失败样本（秒） |
| --- | ---: |
| 排队 | <0.001 |
| 任务 Sandbox 创建 | 0.693 |
| 工作区准备 | 1.105 |
| Harness 启动 | 7.959 |
| Runtime 执行 | 7.108 |
| 停止与撤权 | 5.423 |
| 独立验证 | 3.101 |
| 清理 | 0.654 |

原生工具耗时：T1 为 **252 ms（预期退出 2）**，T2 为 **334 ms（退出 0，结果偏离）**。产物延迟 23.292 秒，验证延迟 26.110 秒，完整观测窗口 26.775 秒；均是提前结束路径的测量，不能当作比成功任务更快。

独立评分实际执行且未通过，score=0；停止与撤权已确认，任务及评分两只 Sandbox 均经 GET 404 确认删除，临时模板、ECS Harness/Runner/Adapter 与运行网络均已回收。华为禁止出网仍为显式假设。现有导出产生 150 个 Trace 事件，表示所支持来源的导出完整，不代表新设计中的细分观测已实现。

ECS 归档：`/opt/aries-artifacts/agentsphere-external-replay-1/`，含原始运行、脚本、`analysis-1/review.json`、`summary.json`、`trace.json` 和 `evidence.json`。复核 SHA-256 为 `eb34a396dcd3afd299ff609b822fc82f303000d0d9fb60f8eccba5c2a45fe1b8`；分析 SHA-256 为 `b8a5e0fb931d7f390b1ceeab2a16f8f1bf0c58c7e1bf545cad3b5f1565133b74`。原始产物未修改。

<a id="最近四次的阶段耗时"></a>

## 四项既有成功基线的阶段耗时

本节保留新增华为 A 测试之前的四项成功基线。这四次使用同一版 Runner，均为**并发 1、单任务、12 项独立测试通过**。
下面的运行名称也用于后面的逐工具表；华为的两个结果均带上述出网隔离假设。
四列不是统一工作负载下的跨云对照；同题、同版本 Runner 并不保证工具命令、次数和错误路径一致，轨迹关系见下节。

### 阶段耗时（秒）

| 阶段 | E2B A recorded | E2B B recorded | 华为 B record | 华为 B recorded |
| --- | ---: | ---: | ---: | ---: |
| 排队 | <0.001 | <0.001 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.177 | 0.880 | 0.553 | 0.580 |
| 工作区准备 | 2.752 | 2.046 | 0.971 | 1.301 |
| Harness 启动 | 7.521 | 21.373 | 22.624 | 23.203 |
| Runtime 执行 | 20.826 | 15.733 | 13.876 | 13.733 |
| 停止与撤权 | 5.560 | 0.280 | 0.177 | 0.166 |
| 独立验证 | 5.217 | 4.998 | 3.158 | 3.090 |
| 清理 | 1.370 | 1.341 | 0.731 | 0.717 |

### 累计延迟（秒）

| 指标 | E2B A recorded | E2B B recorded | 华为 B record | 华为 B recorded |
| --- | ---: | ---: | ---: | ---: |
| 产物延迟 | 39.080 | 41.257 | 38.617 | 39.459 |
| 验证延迟 | 43.794 | 45.637 | 41.484 | 42.254 |

两项累计延迟均从计划到达开始，不能再相加。它们的终点分别位于独立验证内部和独立验证结束。

## 工具到底耗时多久

**以下是三组轨迹的运行诊断，不能合成云厂商性能对照表。** T1、T2……是工具调用的先后编号，不是任务生命周期的阶段，也不是跨轨迹通用的步骤 ID。
使用同一份录制轨迹时，模型返回的工具调用决策应固定；工具仍在目标 Sandbox 中真实执行。
如果工具结果使后续请求偏离录制，严格回放应报错，不能重新调用真实模型生成另一条路径后仍声称是同轨迹比较。

### 轨迹一：E2B A，外置模式

源录制包含 6 次工具调用，T1、T4 返回错误。这组只用于观察外置链路；华为 A 的同轨迹尝试在 T2 结果处偏离，尚无完整成功结果，见上文失败记录。

| 运行 | 原生工具耗时（毫秒） | 原生工具累计（秒） | 模型请求数 | 模型请求累计（秒） |
| --- | --- | ---: | ---: | ---: |
| E2B A recorded | T1：314（错误）、T2：340、T3：318、T4：765（错误）、T5：482、T6：338 | 2.557 | 7 | 13.764 |

### 轨迹二：E2B B，内嵌模式

另一份源录制包含 3 次工具调用，T2 返回错误。它与轨迹一、轨迹三的工具负载不相同。

| 运行 | 原生工具耗时（毫秒） | 原生工具累计（秒） | 模型请求数 | 模型请求累计（秒） |
| --- | --- | ---: | ---: | ---: |
| E2B B recorded | T1：34、T2：198（错误）、T3：182 | 0.414 | 4 | 10.996 |

### 轨迹三：华为 B，同一录制与回放

这两行使用同一条轨迹，3 次工具调用逐项对应，T2 均返回错误（退出码 1）。已有复核确认回放的命令、完整请求历史和工具结果匹配，最终产物一致。
它们能用于检查华为环境中的轨迹复现及观察单次耗时差异；每项只有一个样本，模型时序来源也不同，不能据此判断跨云性能或稳定性能提升。

| 运行 | 原生工具耗时（毫秒） | 原生工具累计（秒） | 模型请求数 | 模型请求累计（秒） |
| --- | --- | ---: | ---: | ---: |
| 华为 B record | T1：22、T2：467（错误）、T3：310 | 0.799 | 4 | 10.890 |
| 华为 B recorded | T1：39、T2：407（错误）、T3：308 | 0.754 | 4 | 10.887 |

模型请求累计、原生工具累计均属于 Runtime 内部活动，不能再加到 Runtime 执行上。
“错误”表示该次工具返回错误；Agent 根据错误继续修复，最终独立验证仍通过。
**错误编号不同，是上述录制轨迹不同的表现，不能拿来归因于 Sandbox 性能。** 即使两条轨迹都在 T2 报错，也不证明它们执行了相同命令。

### 跨云公平对照的要求

每个工作负载、每种模式选定一份录制作为基准，固定 Tape 的 SHA-256，再把同一份录制用于各云厂商的 Sandbox。
A、B 的工具协议和运行位置不同，应分别建立轨迹组：同一条 A 轨迹跨厂商比较，同一条 B 轨迹跨厂商比较。

- 固定题目初始文件、工作目录、相关环境、Agent/Harness/工具版本、镜像内容、资源规格、并发和模型回放时序；记录无法统一的区域与网络条件。
- 逐步核对工具类型、参数、调用决策与因果依赖、结果和预期错误路径；录制里存在的错误也必须保留。工具和结果不能 mock；未预期偏离要保留失败证据并计入失败统计。
- 不同 Sandbox 的主机名或环境信息可能使严格匹配失败。这表示尚未满足该轨迹的复现条件，不能自动归因于平台故障，也不能靠放宽匹配隐藏差异；应先统一可控条件，再制作共同基准。
- 可比步骤使用稳定标识关联，计时范围一致后再比较；不能按各自的“第 N 次调用”硬对齐。A/B 两种模式的总工具耗时也不能直接混排。
- 统一独立验证与隔离口径，在对照条件内重复运行；区分正常步骤、预期错误步骤和未预期失败，保留样本数与原始 Trace。

这些要求已在[portable-v1 A 的单次跨云组](#portable-crosscloud)执行；**尚未完成多工作负载、B 模式和重复/并发矩阵**。华为同源回放的现有证据见[回放验收记录](performance-evidence.md#huawei-真实-sandbox-的-recorded-严格回放)。

### 已有工具计时的范围

| 原生工具耗时的计时位置 | 包含范围 |
| --- | --- |
| A 外置：ECS 上的 Agent | 原生工具准备、SSH/Bridge 通信、Sandbox 执行与结果回收 |
| B 内嵌：Sandbox 内的 Agent | 本地工具准备、进程启动、执行等待与输出处理 |

历史A外置运行未采集完整原生身份，不能回填对应关系；新R01限定归档exec已可精确关联guest；本轮合并版新增native消费区间与SDK客户端区间，但仍不能当作每个part都已测齐。
工具 CPU 时间、磁盘/网络 I/O 尚未采集。历史 12 个任务的旧汇总未提供独立逐工具耗时，不能拿 SDK 请求耗时代填。
上述三组均保留原始数据，不通过删除错误调用或重新编号伪装成同一负载。

## 历史五项基线

以下均为 **E2B、并发 1、单任务**，使用较早版本，不能与四项既有成功基线合并成同版本重复实验。

### 阶段耗时（秒）

| 阶段 | A record | A recorded | B record | B recorded | B zero |
| --- | ---: | ---: | ---: | ---: | ---: |
| 排队 | <0.001 | <0.001 | <0.001 | <0.001 | <0.001 |
| 任务 Sandbox 创建 | 1.875 | 1.394 | 1.354 | 1.855 | 1.129 |
| 工作区准备 | 3.070 | 2.986 | 3.073 | 2.734 | 1.442 |
| Harness 启动 | 7.410 | 8.730 | 22.731 | 21.459 | 21.296 |
| Runtime 执行 | 21.159 | 21.057 | 15.909 | 15.716 | 6.299 |
| 停止与撤权 | 5.704 | 5.690 | 0.303 | 0.323 | 0.345 |
| 独立验证 | 6.032 | 7.299 | 6.070 | 4.323 | 5.830 |
| 清理 | 2.094 | 1.928 | 2.296 | 1.448 | 1.397 |

### 累计延迟与模型请求累计（秒）

前两行是累计延迟，最后一行是 Runtime 内部活动；三行均不是额外阶段。

| 指标 | A record | A recorded | B record | B recorded | B zero |
| --- | ---: | ---: | ---: | ---: | ---: |
| 产物延迟 | 40.633 | 42.101 | 44.809 | 42.946 | 31.874 |
| 验证延迟 | 45.974 | 48.025 | 49.695 | 46.736 | 36.689 |
| 模型请求累计 | 13.768 | 13.765 | 10.998 | 10.997 | 0.656 |

zero 取消模型主动等待后的结果仅代表该次运行，单个样本不能证明固定比例的性能提升。

## 重复与并发测试

以下三批次均为 **E2B、recorded 回放**，每批使用各自固定轨迹，共 7 个任务。
“并发 2，共 3 个任务”表示最多同时运行两个任务，第三个等待执行名额。
各行保留原任务 ID；同一逻辑题目会在不同批次重复出现。

### 阶段耗时（秒）

| 条件 / 任务 | 排队 | 任务 Sandbox 创建 | 工作区准备 | Harness 启动 | Runtime 执行 | 停止与撤权 | 独立验证 | 清理 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| E2B A 并发1，共2个 / merge-ranges-001 | <0.001 | 1.240 | 5.040 | 8.712 | 22.236 | 5.897 | 5.079 | 1.239 |
| E2B A 并发1，共2个 / merge-ranges-002 | 50.290 | 0.738 | 1.584 | 7.333 | 20.971 | 5.537 | 5.681 | 1.250 |
| E2B B 并发1，共2个 / merge-ranges-001 | <0.001 | 0.767 | 2.514 | 19.356 | 15.498 | 0.323 | 4.620 | 1.251 |
| E2B B 并发1，共2个 / merge-ranges-002 | 44.676 | 2.452 | 2.111 | 20.497 | 15.841 | 0.349 | 5.901 | 1.260 |
| E2B B 并发2，共3个 / merge-ranges-001 | <0.001 | 0.844 | 2.330 | 20.795 | 15.847 | 0.347 | 5.544 | 1.262 |
| E2B B 并发2，共3个 / merge-ranges-002 | <0.001 | 0.722 | 1.640 | 14.470 | 16.066 | 0.352 | 5.470 | 1.242 |
| E2B B 并发2，共3个 / merge-ranges-003 | 40.308 | 0.856 | 1.591 | 12.719 | 15.791 | 0.325 | 5.057 | 1.270 |

### 累计延迟与模型请求累计（秒）

前两项指标是累计延迟，最后一项是 Runtime 内部活动。

| 条件 / 任务 | 产物延迟 | 验证延迟 | 模型请求累计 |
| --- | ---: | ---: | ---: |
| E2B A 并发1，共2个 / merge-ranges-001 | 45.158 | 49.040 | 13.765 |
| E2B A 并发1，共2个 / merge-ranges-002 | 88.353 | 93.534 | 13.763 |
| E2B B 并发1，共2个 / merge-ranges-001 | 39.340 | 43.411 | 10.995 |
| E2B B 并发1，共2个 / merge-ranges-002 | 87.334 | 92.165 | 10.996 |
| E2B B 并发2，共3个 / merge-ranges-001 | 41.592 | 46.052 | 10.996 |
| E2B B 并发2，共3个 / merge-ranges-002 | 34.079 | 39.057 | 10.997 |
| E2B B 并发2，共3个 / merge-ranges-003 | 72.852 | 76.979 | 10.996 |

这些批次的任务集中到达。并发 1 时后一个任务、并发 2 时第三个任务，都要等待名额释放。
排队已经计入产物延迟和验证延迟，因此后续任务累计延迟更长，不代表 Sandbox 执行更慢。
当前样本量只能描述本次调度行为，不能给出最大容量或可信的 p95/p99（95%/99% 分位延迟）。

## 如何判断瓶颈

先看阶段耗时，找出较长的阶段；再看 Runtime 内的模型请求和原生工具活动。
例如华为 B recorded 的 Runtime 执行为 13.733 秒，原生工具累计为 0.754 秒，不能把整个 Runtime 归为工具耗时。
这些观测存在嵌套和覆盖缺口，差额的来源还需精确关联后确认。

Trace（调用时间线）已可导出，但 Runner、模型、工具使用的时间基准尚未全部对齐。
独立泳道的水平位置不能直接比较，不同机器时间戳不能直接相减；不能把 RPC 与 guest 耗时差额叫作纯网络延迟。

这 45 个任务是**已接受的成功结果集合**，不是全部历史尝试，因此不能据此计算总体成功率。
吞吐的完整窗口、历史失败及测量局限保留在[性能证据附录](performance-evidence.md)。
Goodput 指满足正确性及明确时限要求的有效吞吐；当前未配置该时限，故未计算，不填零。

## 仍待完成的观测能力

本轮合并版本华为A/B严格回放与细分观测通过，E2B A新baseline/L0矩阵中途失败，仅2对完整；旧四对结果仍只覆盖旧版本。P0保持部分完成，额外模型请求来源、生产有界IO、部分进程终态及L1/L2仍需收口，没有性能合格结论。完整边界与退出条件见[实施报告](observability/OBSERVABILITY_IMPLEMENTATION_REPORT.md)、[测量契约](observability/OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md)和[问题台账](observability/OPEN_ISSUES_AND_RESOLUTIONS.md)。

| 所属阶段 / 指标 | 待补的观测点 | 当前缺口 |
| --- | --- | --- |
| P0 Runtime/Model/Gateway/Verifier | 请求、帧、Gateway delivery、Verifier 子区间与同一 Snapshot | 华为A/B新组合通过；E2B配对第5次主流程后出现额外模型请求，Strict失败保留，caller待定位 |
| 原生工具耗时：A | SSH 建连/握手/收流；Bridge execution ID → Adapter process ID → guest 命令 | 归档exec精确贯通，新增native消费与SDK客户端区间；独立SSH细分/全工具分母仍未齐，覆盖unknown |
| 原生工具耗时：B | Gateway 收包与消费、native consumer interval | 已记录；独立 expected-tool 分母和真实进程终态仍 unknown |
| 原生工具耗时：Sandbox 进程 | 进程启动、首输出、direct-child退出观察、输出排空；guest 单调时钟耗时 | 本轮华为A六次可按身份查看四边界；CPU/I/O、B原生guest边界及部分取消/后台终态仍unknown / R07未完整关闭 |
| P1 Trace | OTLP→Jaeger、Span 查询和 partial_success 对账 | PLANNED/UNRUN |
| P1 Metrics | OTLP Metrics→Prometheus 历史批次、乱序、重发、Histogram 对账 | PLANNED/UNRUN |
| P2 Profiling | procfs/cgroup/perf/eBPF/应用 profiler 权限和热点 | PLANNED/UNRUN；不加载特权探针 |

P0 已有同一 Snapshot 的控制端区间与离线显示；不能将其推广成完整原生工具身份/计时覆盖。P1/P2 仍需独立实施和真实后端/权限验收。同命令真实重叠和反序完成现已通过限定PoC；重连/取消等完整身份矩阵仍未齐，本轮顺序严格回放不表示已支持所有并行工具。
缺少终态标未知；legacy model/events.jsonl 已返回的存储错误现已与模型业务结果分开，Strict/Tape 错误仍失败；[五项门禁与实际 CLI 验收](observability/VALIDATION_AND_REPRODUCE.md#r05-observer-separation)不代表有界落盘或新组合云回归。R05两例最终fsync延迟已完成实验收容，生产有界关闭仍未解决；R07当前A断连清理/B Stop故障拒评通过，详见[故障结果与剩余边界](observability/OPEN_ISSUES_AND_RESOLUTIONS.md#r05-r07-fault-exits)。
新观测只用于新运行，不能倒填为历史已经测到的数据。Perfetto 修复只改变显示层 flow ID，未修改历史 Snapshot 或原始证据。

## 数据来源与详细证据

既有 16 个成功任务的文档整改只读取原始证据；随后执行华为 A 独立录制与 portable-v1 的三次真实任务；离线复制、复核和导出不重复计数，各自原始记录与分析目录见上文。
既有 16 个任务的完整数值、纳秒原值、逐任务来源及输入哈希保存在 ECS：
`/opt/aries-artifacts/performance-readable-stages-1/` 的 `data.json`、`tables.md`、`evidence.json`。

| 要查什么 | 入口 |
| --- | --- |
| 历史吞吐、故障、协议检查、资源采样、135 份基础证据索引 | [性能证据附录](performance-evidence.md) |
| 最近 E2B A/B 回放的原始记录 | [E2B A/B 回归证据](performance-evidence.md#新-runner-的-e2b-ab-严格回放回归) |
| 华为真实录制与严格回放的原始记录 | [华为录制证据](performance-evidence.md#huawei-embedded-真实录制条件性通过)、[华为回放证据](performance-evidence.md#huawei-真实-sandbox-的-recorded-严格回放) |
| 每个阶段的详细说明与原始字段 | [阶段说明](performance-stages.md) |
| 结果字段与 Trace 使用方法 | [运行结果指南](run-results.md) |
| 各组件职责 | [架构指南](architecture-guide.md) |
