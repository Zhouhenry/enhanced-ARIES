# 观测 Schema 与测量口径

**Jiuwen process schema 1（release23）：** 原生进程使用独立单调时钟、创建时身份快照和 producer 分母，已通过双云 A/B 真实任务。`creation` 从 spawn_requested 到 connection_made，含起点后的同步观测写入；`command` 从 connection_made 到直接子进程退出回调；`exit_to_pipe_closed` 只在管道晚于退出关闭时存在。`observation` 是重叠事件容器，禁止加总。不是 CPU time、内核 exec/exit 或完整后代树时长；缺失边界不补零。分析器 v1.16.0、OTLP v1.3-process-events/事件策略v3，双出口使用同一快照。[完整口径与证据](../jiuwen-process-observation-20261027.md)。

四组各自冻结 Snapshot 的 Perfetto、Jaeger、Prometheus 实际查询已限定 PASS：15 个进程、105 个 markers、46 个含容器区间，Metrics 排除 15 个容器后只聚合 31 个区间样本。进程相对时间与 v3 epoch 投影误差 0 ns，旧版微秒投影和 Prometheus 毫秒截断按各自精度对账；身份链接不证明跨时钟校准。18 个有值 metrics 流通过首次/重复 payload 查询，5 流保持 NOT_COLLECTED。整体 integrity=complete / coverage=unknown，华为评分出网 assumed，独立模型轨迹不作性能排名。[四组数值、资源归属和原件 SHA](../jiuwen-release23-cloud-validation-20261027.md)。

## Agent Provider 请求层（release22 实现，release23 合并验收）

[具体边界与验收状态](../jiuwen-provider-observation-20261027.md)：新增原生 Provider 请求单调区间和 `http_submit` / `response_headers` / `first_frame` / `terminal_frame` 四点；只有 A 远端模式适用。与 Bridge、worker 用精确身份关联，各层时钟不相减。请求时长包含消费者和收尾，不是 guest 进程时间。

OTLP 新策略 `jiuwen-chat-and-provider-lifecycle-v2` 保留原五个 chat 点并加入四个 Provider 点；后者以明确 `parent_analysis_operation_id`、相同 clock、完整身份和区间范围校验，使用 span events；跨层使用 Link。独立 SDK 审计加上 Bridge 实际接纳分母检测丢失，缺 clock/整对 Provider 事件不能成为零分母成功。三个请求层 Histogram 独立于阶段汇总。候选联调已通过，最终发布/云端与实际后端状态以链接报告为准。

## release21：原生身份与两层远端请求

[真实组合证据及边界](../jiuwen-native-runtime-validation-20261027.md)：Bridge 使用 Runner 单调域，worker 使用独立单调域及本地 Unix 锚点；两层请求的实际起止进入同一 Snapshot。原生 Tool→Bridge 只按 Attempt/Session/Request/ToolCall 唯一匹配，Bridge→worker 按 invocation 和完整身份/操作匹配。OTLP 使用 Link，Perfetto 保留查询关系；未对齐的域不画时间 flow。请求返回成功不是命令成功或独立评分成功。

新增可选 `TaskTiming.bridge_prepare_duration`，紧贴 `Bridge.Start`；`bridge_prepare_started_at` 表示实际进入，只有成功才有 `bridge_ready_at`，失败/取消仍保留实际 duration。Snapshot 将 `bridge_prepare` 放在 workspace_prepare 与 harness_start 之间；原八阶段字段含义与数值不变，历史缺失仍为 not_collected。两层请求 Histogram `aries.jiuwen.remote.duration` 与阶段 Histogram 分开，不能相加计总耗时。[四组新云阶段对账与 E2B A 实际后端验收](../jiuwen-release21-cloud-validation-20261027.md)已 PASS；其余三组不借用该后端结果。原生、worker、控制器仍是不同时间域，完整覆盖仍 unknown。

## Jiuwen A 的实际观测边界（release19）

[两云 A 真实结果](../jiuwen-external-cloud-validation-20261027.md)沿用同一分析快照及现有 Schema。每轮的 4 个 Bridge invocation ID 可精确对齐 controller/worker；它们不是模型 ToolCall ID，后者在该 release19 历史样本中仍 unavailable；release21 新样本另列，旧结果不回填。不得按相同数量、命令或时间邻近建立跨层父子关系。SDK 导出专项 complete 不代表全局 coverage complete。

`bridge.register` 是 Runner 原始事件中的真实区间，该 release19 样本尚未进入八阶段 duration 字段；报告将它单列并列出总时间剩余差额，不篡改历史阶段或把八阶段宣称为无缝分割。Agent 容器资源、任务 Sandbox、评分 Sandbox 与控制器分别归属；E2B 评分环境本轮未取得 provider 读数，保留缺测。华为原始单位不明的内存字段不导出为 bytes，显式 MiB 字段保持其原单位。

## Jiuwen官方Span与producer审计范围（2026-10-27）

[release18五门禁冻结](VALIDATION_AND_REPRODUCE.md#jiuwen-release18)后，E2B record7与华为record2均通过原12/12、score1，完整Tape分别9/7轮；每轮Stop/revoke、两个Sandbox独立删除及cleanup=[]均确认。两份真实模型轨迹不同，仅作双云功能验收，不是同Tape性能对照；华为评分出网仍为显式假设。Strict replay3与record1–5旧失败保留；校验模式讨论延后，固定提示变体暂停，历史六例Strict仍3/6。

原生采集器保存私有`harness/native-spans/traces-*.jsonl`及无正文索引`native-spans-index.json`。索引保留官方trace/span/parent、session/request、ToolCall身份、原始Unix纳秒和来源SHA；完整性及匹配计数显式核对。仅有准确session/request或原生parent链的Span进入本次Attempt。release17真实样本的独立producer分母、exporter写失败计数仍未知，原生root complete不代表零丢失。

历史分析器`aries-analysis-v1.9.0-native-wall-spans`新增可选`native_spans`，release17的v1.10另加入cgroup边界；均不向原有单调`events/intervals`塞入epoch时间。核对Attempt会话、索引计数、私有源SHA及字段后生成同一快照，老冻结快照缺可选字段时内容身份保持兼容。

**release18的v1.11 producer审计：** native fresh进程的extension记录sampled SDK Span start/end身份与终态，Stop后按实际导出ID集合对账。Snapshot验证可选`producer_audit`的来源SHA/字节、绑定、状态和计数，仅单列`jiuwen_sampled_sdk_span_exports`。真实E2B record7与华为record2的sampled SDK Span的started/ended/exported分别44/44/44与35/35/35，missing/unexpected均0；`jiuwen_sampled_sdk_span_exports`专项integrity/coverage complete，global coverage仍unknown。未采样/未插桩行为、guest进程计时、跨机时钟与有界flush不在该证明内。 无audit的旧样本保持unknown；缺导出、未结束、重复、坏来源或不完整终态不能计完整。官方exporter吞掉文件错误并返回SUCCESS的反例已在定向测试复现，因此不以shutdown/force_flush返回值证明零丢失。[前置定向证据](VALIDATION_AND_REPRODUCE.md#jiuwen-producer-audit-candidate)与[新云验收](VALIDATION_AND_REPRODUCE.md#jiuwen-release18)分别保留。

| 展示/出口 | 时间和关联口径 |
| --- | --- |
| Perfetto默认控制端视图 | 保持Runner单调域；显式列出被排除的原生域 |
| `--trace-clock jiuwen-wall/<attempt-id>` | 独立原生wall-clock视图，先用整数减去明确的Unix原点再转微秒；原始ns与`wall_duration_ns`保留。它不是单调时钟，不与Runner相减 |
| 原生Tool Span | 官方Ability调用范围，实际ToolCall ID；包含该原生调用内的开销，不是guest纯进程执行/退出/pipe drain |
| OTLP | 从同一Snapshot保留原生trace/span/parent IDs及Unix时间；独立service/trace，不嫁接伪controller父节点 |
| Prometheus及八阶段 | 沿用既有账本和单调阶段计时；不把原生父子Span时间再次相加或并入原有吞吐分母 |

公共出口不复制原始prompt、参数、ToolResult、异常正文或任意原生Span name；只投影身份、受限操作/工具名称、状态、时间和证据引用。L1/L2启用详细原生Span；L0保留已有轻量SSE观测，详细Span显式`not_enabled`。缺文件、截断、重复身份或计数不符仍是观测失败，与原Agent结果分开。

record5显式L1采到11个官方Span，全部精确关联本Attempt，含1个authoritative Tool Span与1个complete root；invalid=0、unassociated=0。Snapshot integrity complete / coverage unknown，四出口exit0仅证明投影；本轮TP/Jaeger与Prometheus独立补验均通过，首轮Metrics脚本失败保留。原生索引coverage partial、flush未确认、producer分母未知，SIGTERM clean exit不能证明exporter零丢失。5个控制端chat生命周期event与这11个原生Span是不同计数，不相加为工具数。

资源按归属及窗口分别解释：record5控制器共享进程CPU首末增量0.369278秒、末次进程生命周期RSS高水位35,520,512 bytes；任务整台Sandbox 15次receipt、2空、93条返回读数中13个唯一读数、80重复、0冲突，返回CPU最大51.71%、内存最大755,273,728 bytes。provider时间覆盖17:36:40–17:37:40 UTC，而Runtime仅8.136秒，不能把全窗口最大值称为Runtime峰值或将CPU百分比积分成时间。评分Sandbox两次receipt均空，应为未知而非0。`disk_io`、`network_io`和Agent/工具资源拆分不支持；磁盘占用不是IO吞吐，独立cgroup资源候选不在release16。[窗口与原件SHA](../jiuwen-cloud-validation-20261027.md)保留可核查依据。

以下历史版本与已验数据保留原口径；最新冻结及实际结果见[Jiuwen实施报告](../implementation/jiuwenswarm-flash-progress-20261026.md)。以上不关闭R01–R18的全链覆盖退出条件。

release17新样本的35/43个Span与6/8个authoritative Tool是不同计数；新增cgroup四边界按Agent进程组及后代计量，块设备I/O保持设备身份，不能叠成逐工具消耗。cgroup生命周期`memory.peak`不归为Runtime峰值；provider整台Sandbox采样、控制器进程增量和cgroup资源不能混加。

**Jiuwen原生入口计时：** `jiuwen.chat`为HTTP/SSE控制端消费区间；request_started、response_headers、first_sse、terminal_received、stream_drained属于同一controller clock。first_sse是首个完整SSE记录到达，不是首字节或LLM首token。request/session身份保留，native sequence不作为丢失分母；未安装进程 hook 的历史原件保持进程计时 unsupported，当前 release23 的独立进程源见本页首节，cgroup资源仍只覆盖其声明边界。[历史实测和出口差异](../implementation/jiuwenswarm-flash-progress-20261026.md)。 历史 OTLP 的 `jiuwen-chat-lifecycle-v1` 只接受上述五点，以原父operation ID、相同clock和task/attempt身份关联，必须落在已验证父区间内；缺失/歧义/无锚/重复均显式计数。只读Exporter不重新推算工具身份或指标。

**原生采集上限口径（release14）：** `NativeToolCollector` 的4096条及4 MiB限制针对保留的工具记录，不是ToolCall数、模型token数或云资源配额。旧版误将非工具Agent流计入条数，已修复并通过ECS定向与五项发布检查；全流序号、错误和终态检查保留。真正超限仍标记incomplete；真实同Tape回归已通过，64条工具记录完整；1371输入未触达4096边界，不能借用最后序号声称超限压测。旧DB数据不补造。通用Recorder的4096槽队列是另一机制，本次未修改。[证据与真实回归状态](VALIDATION_AND_REPRODUCE.md#native-tool-retention-release14)。

**最新测量范围（2026-10-26）：** 当前六个选定主用例 Strict 跨云完成 **3/6：fix-git、async两个原始任务＋大文本terminal-result-v2独立变体**；原始任务本身仍2/6，旧失败和分母保留。 大文本两次E2B/一次华为同Tape均PASS；SQLite v3录制PASS、自回放因安装日志回读mtime偏离而FAIL。此前release5同版E2B复测与跨云阶段/工具/Trace明细已取回。fix-git、cancel-async-tasks已有同题同Tape双云成功；三题7次真实尝试均保留八阶段及同快照Perfetto/OTLP、TraceProcessor对账。async中断样本仍为incomplete/incomplete；华为DB首工具后Strict失败，不拿部分工作量与完整E2B轨迹算速度比。导入误差0ns仅证明投影数值一致，coverage仍unknown。详见[双云测量与来源](../tb-crosscloud-performance-20261026.md)。

CPU/内存同规格是资源比较条件，磁盘等差异随结果说明；SDK等待与guest耗时不跨时钟相减。华为C++新录制原6/6，首轮回放因目录mtime偏离而原5/6，业务/模型/评分/清理分别记录，见[C++归档](../tb-huawei-cpp-performance-20261026.md)。release7的wait解析使用自身调用ID、精确job关联和独立原生单调时钟；无UTC锚的原生区间不造OTLP epoch，保留producer终态/覆盖未知。工程、原生探针和driver离线验收已通过，DB变体首轮实际FAIL、原评分5/7；65次exec中62精确执行、1明确预检拒绝、2无producer身份，快照保持incomplete。双Trace对账仅通过不完整来源投影，未做新后端验收。见[新变体测量归档](../repro-variants-performance-20261026.md)。[版本证据](VALIDATION_AND_REPRODUCE.md#replay-variants-release7)。P0仍PARTIAL、R05暂停、R13暂缓，P1/P2完整台账保留。

SQLite独立变体录制原3/3、Tape完整，但旧原生持久化的8192B截断丢失终态小字段。release8已合并两处既有白名单的`protocol`、`outputRenderer`、`outputTruncated`保留，不改ToolResult、正文、阈值和Strict。实际回放在第六请求即因运行中新建目录日期偏离失败，未到达后台wait，故全链修复仍待验。旧录制的5个无UTC锚原生区间不进入主Trace/OTLP；2,025个共同区间的精确投影不代表全部时钟对齐。

CPP旧录制中的SDK进程exit0与Bridge最终255必须分别记录。精确execution ID可证明同一次辅助检查在wait成功后收到stdin取消；该辅助检查与首个native ToolCall之间只有准备流程推断，不能补造身份或Trace边。ExecStream内部取消与实际执行错误的混淆已修复，定向unit/race与release10五项发布检查通过。CPP v2新华为录制原6/6、34/34 exec精确，同Tape回放前5轮/5exec精确、第6请求因gdb内部合流输出的两行提示位置变化而Strict偏离、原5/6；两次删除确认。后台协议均未触达，原始材料和覆盖仍incomplete；前台身份完整不代表全链完整。新v2附录仅为公开工作负载数据，显式SHA固定实际源路径/组合文本，不改时钟、正文、Strict或旧Tape。SQLite v2新E2B录制原3/3，35完整模型轮、36/36 exec精确及1次后台wait映射通过；40个观测请求中的5个在已证实的封口之后拒绝，独立保留，不混入Tape的35轮。Snapshot integrity complete、coverage unknown，artifact_proven=false不改；SQLite同Tape回放在第24请求因安装命令自动输出mtime而失败：23模型轮、24exec+1wait已执行，原0/3，其余未执行不算缺身份。两个真实wait均覆盖8193B持久化截断，三字段保留正确。大文本v2真实录制、两次E2B及一次华为同Tape Strict回放均原5/5，各23完整轮、20/20 exec与2job/2wait通过。三次回放均23轮匹配、0上游请求；recorded时序与真实工具耗时分开。原生4区间/6边无UTC锚点，主时间轴与OTLP仍排除，coverage unknown不改。[修复与真实回归](VALIDATION_AND_REPRODUCE.md#execstream-teardown-fix)。

SQLite v3新E2B录制原3/3、41轮完整Tape、38/38 exec及2job/2wait通过，唯一Sandbox删除确认；5条ADMISSION_CLOSED均在同域关闭时点之后，不计入模型41轮。原生结果没有本次8192B截断标志，不能借v2结果宣称本次覆盖。录制双Trace2,663区间/Span、219Flow（217parent+2link），0ns投影误差；integrity complete / coverage unknown和artifact_proven=false保留。同Tape自回放在turn24/request25因make install日志被tail读回的3处mtime偏离而FAIL，原0/3；22exact exec+2wait已执行，剩余16次exec未执行，清理确认。失败样本双Trace为1355区间/Span、114Flow（112parent+2link），5479项对账0失败；4原生区间/6边无UTC锚点而未导出，coverage仍unknown。投影通过不改变任务FAIL，部分工作量不与完整录制直接算性能比；见[回放阶段、工具与首差异](../repro-variants-performance-20261026.md#sqlite-v3-e2b-replay-1)。

华为大文本评分519.266396秒与Runtime191.408839秒分别计量：原评分exec517.824025秒，原CTRF测试25.035945秒；APT日志报告获取9395kB耗8min6s，不能把这段依赖准备归入Agent Tool或伪造精确网络Span。具体DNS/连接/传输未拆分；同CPU/内存允许比较，磁盘差异单列，评分禁止出网未被证明。[三次对照及APT来源](../repro-variants-performance-20261026.md#text-v2-huawei-replay-1)保留这些边界。

**R01新缺口已局部修复：** DB record2中脚本预检拒绝漏记终态，已在原生validator边界补齐；原异常、前台/后台/PTY语义保持，ECS原版/候选探针对照通过，独立新镜像已发布。DB record3已使用新镜像，但唯一拒绝是既有elevated分支，新增validator拒绝仍只有定向探针覆盖；旧缺口不回填、R01全范围未关闭；见[修复与发布证据](VALIDATION_AND_REPRODUCE.md#r01-script-preflight-rejection)。

**R03诊断闭环：** [release13](VALIDATION_AND_REPRODUCE.md#record-failure-release13)已通过最终4Go组合的五项ECS门禁。模型record失败的安全原因分类贯通原边界事件、同一Snapshot与Perfetto/OTLP；未知任意文本不透传，Tape失败语义、匹配与时序保持。旧DB末帧精确首因仍unknown，C++旧HTTP 402有独立原始header证据；新录制结果单列。

**Workload Replay 真实增量（与 Strict 分开）：** 显式 `model.replay_policy=workload` 已通过 [release12 五项 ECS 工程门禁](VALIDATION_AND_REPRODUCE.md#workload-replay-release12)，完成 [DB](../workload-db-performance-20261026.md)、[SQLite v3](../workload-sqlite-performance-20261026.md)和[C++ v2](../workload-cpp-performance-20261026.md)同Tape跨云功能对照：分别各12轮/11工具/原7/7、41轮/38exec+2wait/原3/3、35轮/34exec/原6/6。真实终态、停止撤权、独立评分与删除确认通过，正文差异留证。**Strict仍3/6，另有3/6完成Workload功能对照；不能计成Strict 6/6。** C++整体观测incomplete；其他成功样本的全链coverage仍unknown。双Trace对账只证明保留数据的投影一致，不将功能成功升级为全观测通过。C++新E2B共享TTL3600秒，实际全程288.328秒；旧4200创建拒绝与新Strict的GDB地址差异失败均保留。SQLite首轮Snapshot重载FAIL、华为缺profile文件FAIL修复后新轮通过；[SQLite v4新录制](../sqlite-v4-performance-20261026.md)原2/3、Tape不完整单列，不覆盖v3。全部历史失败保留，未改旧Tape或Strict matcher，P0仍PARTIAL。

## 六例真实任务暴露的边界（release4封存批次）

release-4固定605文件，manifest SHA `d61c2a1245ded2f561843143276a177e1d45540250b10f8e922012a41e318710`；ECS五项Go门禁证据 `e3f38350de19c89c40d43a0a45e4db73c7e573e05c1133c8cfb801d3336b4ec0`。原生-3四错误等价、两套26场景及4个B并行限定检查通过，原finalize/yield限制保留；原生-1 guard选择错误、-2矩阵JS权限EACCES失败未删除。native publish-3已成功，证据 `9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1`；新6次云运行均使用此版本。

- **身份**：native producer v2、SSH身份v1、parser1.6.1。明确pre_dispatch/rejected/dispatched=false与已派发、未知分别表达。SQLite录2有28个精确执行和1个明确拒绝；拒绝的SDK/guest/exit为null，不造span。旧here-string误解析与权限拒绝缺口不回填。
- **模型封口**：Harness.Run返回即关闭新模型接纳；已接纳请求的失败仍保留。仅同clock关闭之后且证明未接纳的ADMISSION_CLOSED允许作为收尾事件。text/SQLite录2真实5/6次符合该条件，Tape完整，不将其他拒绝抹掉。
- **字段**：覆盖对象为coverage_status，不能读status或以缺字段默认通过。验收器正例/15项负例在ECS通过；旧真实KeyError可复现，新旁路复核原数据和Snapshot不变。
- **初始mtime**：准备阶段设置已声明路径并实际读回、保持atime；不固定系统时间或改变性能时钟。DB放3采用历史结果可见分钟，未知原秒/纳秒/时区，不声称完整恢复。运行中创建文件的mtime不由此固定。
- **失败口径**：18原Attempt中9succeeded、8failed、1timeout；12次validated。原driver7PASS，另2次原FAIL的归档复核通过，不增加Attempt或改原统计。
- **Trace**：每次同一快照供两出口。6新运行均实际TP对账，0ns只是投影数值误差；DB超时源incomplete保留，其余coverage仍unknown。新后端/UI未验。

原始case可有效测试真实Agent，不能假设它天然满足跨实例逐字回放。text/SQLite新回放分别实证初始目录和运行中新建目录日期偏离；随机句柄/PID/idle在录制中存在，但尚未执行到该后续偏离。该历史批次当时仅提交[变体设计](../design/reproducible-agent-workloads.md)审阅；随后用户批准独立实现。新候选工程验证与真实验收分别报告，Strict 和原生默认后台/PTY 保留。[本轮真实证据](VALIDATION_AND_REPRODUCE.md#tb-six-current)。

日期：2026-10-25。本页描述本轮代码契约；是否实测通过以 [验证归档](VALIDATION_AND_REPRODUCE.md) 为准。P1a离线OTLP/Jaeger与P1b账本/阶段Metrics已限定通过；P1c资源流解析与独立Metrics出口已通过ECS工程验证，真实后端按所述范围对账通过。P2仍为计划，P0仍PARTIAL。

<a id="r01-real-multicall"></a>

新版真实模型用例的首轮含两个独立调用，后续公开测试的失败与重试均按原模型决策执行。工具 exit 1 是任务内部调试事实，不自动等于最终任务失败；最终成功依据原独立 Verifier。录制与回放各自的真实时长保持差异，决策/ToolResult/候选必须严格一致。每次运行只产生一份校验 Snapshot，两种 Trace 共用它；本轮控制器同域 SDK 等待重叠不证明 guest 跨域并行。[完整验收](VALIDATION_AND_REPRODUCE.md#r01-real-multicall)。

<a id="r01-multicall-app"></a>

应用级回放沿用原`runProfile`生成的计划、调度、完成和验证分母。`RunCloud`返回nil、模型session成功、工具退出0都不能覆盖独立评分失败：本例1 completed / 1 failed / 0 verified_succeeded，验证成功吞吐0。Provider在私有wiring入口显式恢复为录制时的OpenAI方言，Strict整请求匹配不变；这证明应用API接入，不证明未经适配的CLI可直接消费该受控Tape。统一Snapshot只生成一次，两个Trace从它投影；SDK等待重叠2.315237332秒仅为控制器同域在途重叠，guest时长不跨域对齐。[真实证据](VALIDATION_AND_REPRODUCE.md#r01-multicall-app)。

<a id="r01-multicall"></a>

## R01：同轮多个 ToolCall 的严格契约

新录制 Tape v2 用 `Turn.Calls/tool_calls` 保留按 SSE `index` 排列的全部调用；交错分片按各自索引聚合，不能把两个同名/同参数调用合并。旧 v1 的 `Turn.Call/tool_call` 仍按原字节形状与校验和读取，禁止同时出现两种元数据。原 SSE 帧及 offset 不重写。

录制要求完整旧历史、一个含完整调用数组的 assistant 消息，以及每个确切调用 ID 对应且只出现一次的结果。结果可按实际到达顺序记录；Strict Replay 比较整个原录制请求，包括数组顺序和每个旧结果，**不按 ID 重排请求来消除偏离**。ID 方言逐调用应用，新增同轮映射碰撞拒绝。模型首工具增量事件包含任意索引，仍是请求级 first-delta，不代表某个 Tool 已开始执行。

原生调度由固定 OpenClaw 决定。其 `executeToolCalls` 支持顺序和并行；并行路径通过 `Promise.all` 取得按原调用顺序排列的结果，完成事件可以交错。协议“同轮多个调用”、控制器 SDK 在途重叠和 guest 同时执行是三项不同证据；没有共同 guest 时钟不能推算并行执行时长。当前工程与真实链路状态见[复现](VALIDATION_AND_REPRODUCE.md#r01-multicall)。

当前R15新增[568文件B schema 2四对实测](VALIDATION_AND_REPRODUCE.md#r15-b-native-paired)：8/8通过、16个Sandbox确认不存在，479项独立对账及四条L0双域TP通过。只算每对L0−baseline，关闭来源用null，控制器共享资源不拆分组件；不与[此前565文件A四对](VALIDATION_AND_REPRODUCE.md#r15-cleanup-paired)混池。负差值不能归因观测提速；该固定B配对范围完成，独立guest logger成本、L1/L2及全链成本仍未知。

<a id="r04-cli-cancel"></a>

## B 原 CLI 取消：真实分母与观测完整性

[本次实际验收](VALIDATION_AND_REPRODUCE.md#r04-cli-cancel)保留原应用层两条计划、concurrency=1。首工具返回后，原 Strict `request_matched/turn=1/succeeded` 触发一次 SIGINT；真实账本为planned=2、admitted/started/executed=1、canceled=1、not_started=1、completed/verified_succeeded/succeeded=0。未启动任务没有模型会话或Sandbox，八阶段均保留null/not_collected，不用0或合成跨度代替。活动任务的Evaluate调用为124309ns、status=canceled、Verifier=not_run；这是评分入口提前取消的耗时，不是测试执行耗时或验证成功。

原模型严格匹配2轮请求、完成1轮响应、发送48帧后session incomplete，应用账本model session=failed。首工具3条consumer事件与10条schema2 callback精确关联；B没有独立native transcript归档，模型可见首工具结果由原Strict第二轮完整请求匹配证明，不声称离线逐字核对整个ToolResult对象。原Stop/revoke成功先于Evaluate，1个Sandbox独立NotFound；本例没有在途工具取消或精确全树终态测量。

任务取消不等于观测失败：本次native index确实complete/terminal_observed，controller与model request-boundaries writer分别89/69条、已记录drop=0；Snapshot为complete/unknown。实际TP导入控制器784区间/18flow与独立原生域3区间/0flow，12来源、两视图并集及业务结局一致，起止/时长误差0ns。原生域不跨域对齐；全局漏采仍unknown，本次UI为UNRUN。四角色、原产品源码与停止门均未改，复用568文件运行时和v1.4分析器，无新构建；旧直接Runner夹具的缺Attempt导出FAIL保留，不由此回填为通过。

## A 模式跨任务并发：身份与计时边界

[当前两任务验收](VALIDATION_AND_REPRODUCE.md#r01-native-concurrent)使用原应用层调度、同一Tape及两个真实Attempt。Tape预登记每个任务6次exec，离线重新拼接录制SSE核对决策；同一ToolCall ID在不同Attempt中重复是预期行为，必须与scope/Agent session一起使用，不能把ToolCall ID单独作为全局主键。12条native invocation→SSH request→Bridge execution→SDK Process→guest receipt全部精确关联；两个独立模型会话分别通过7轮/735帧Strict。真实ToolResult正文和原业务字段严格比较，测量字段durationMs不要求相等；没有降低Strict匹配标准。

同一controller时钟的两个`agent.run`区间重叠18.540939532秒，证明完整Harness.Run调用并发；这不是表中较窄的Runtime执行计时。本次SDK `exec.wait`区间实际没有重叠，不能据此声称两个guest工具同时执行；guest内相对耗时只在各自时钟中解释。单Agent内部并行工具、自动重试和后台/PTY矩阵没有新增覆盖。

Snapshot保留两个Attempt与12条精确映射，845区间/86flow/38来源的实际TraceProcessor核对通过，起止与时长导入误差0ns。预登记分母证明本次12条决策均被追踪，不等于所有原生事件的全局零丢失；故完整性complete、覆盖unknown保持不变。没有新增guest时钟锚点或合成guest进程时间线，本次UI为UNRUN。

## 当前 B 显式取消的证据边界

[一次真实故障验收](VALIDATION_AND_REPRODUCE.md#r07-b-native-cancel)使用固定脚本预先确定一个ToolCall ID，原生schema2回调逐字绑定该ID与请求的Agent session；独立进程探针以PID/start ticks、cgroup和心跳证明评分前已停止，不用命令或时间近邻关联。已知父子与所在受管组的停止证明，不等于任意后台/PTY进程树已全量验收。

原始回调仅有`child_spawn_return`与`stdout_first_nonempty`，同Node时钟差26.008256ms；exit、natural EOF及全局drop仍unknown，不能从Stop成功反推退出时间。原native index的`complete=false`、`terminal_observed=false`、`incomplete_tool_pair`保留，collector为failed。业务取消、观测失败、独立评分失败、清理成功是四个独立结果。

本夹具直接调用Runner，原结果没有应用层`attempts`。`BuildSnapshot`仅逐已有Attempt读取辅助来源，所以本次导出只含Runner/Gateway记录，`integrity=complete/coverage=unknown`只指已读取的控制器来源；不能当作已检查native缺失。未填造Attempt、队列耗时或native终态；该旧夹具完整故障Trace验收仍UNRUN，辅助导出验收的scope错误保留为FAIL。后续[原CLI取消验收](#r04-cli-cancel)已按真实Attempt账本验证另一条取消轨迹的双域投影；它在首工具返回后的模型请求阶段取消，不补验本夹具的在途工具终态或缺失来源。

## B native process与首输出（schema 2真实华为B与E2B B限定通过）

<a id="r07-native-first-output"></a>

`aries-analysis-v1.4.0-native-first-output` 引入同一Snapshot读取Stop后归档的 `harness/gateway.log`，v1.5资源解析器保留此行为。兼容原生观测schema 1/2，同一来源不能混用版本。接受无时间前缀的 `[aries/native-process] ARIES_NATIVE_PROCESS `，以及实际Gateway CLI验证的毫秒ISO数字时区前缀，例如 `2026-10-25T07:32:24.660+00:00 `，随后是同一subsystem/marker和metadata。时间前缀只校验日志包络，不能转成进程时钟锚点。无marker的旧归档维持原投影，未验证的其它包络不猜解析。

通过已有 `harness.native_observation_protocol=aries-native-process-v1` 显式启用，仅限embedded；该协议名与事件schema版本分别管理。默认空值不改变旧运行。Go将启用选择与既有ObservationLevel合并：baseline关闭producer，L0/L1/L2经staging写入 `launch.env` 的单个 `ARIES_NATIVE_PROCESS_OBSERVATION=1`。Gateway环境会重建，不依赖镜像ENV；候选factory读取后删除开关，再由原生exec继承工具环境。原生ToolResult协议、后台/poll/PTY和停止隔离门不变。

既有原生日志配置合法style为compact/json/pretty，非TTY默认info/compact。实际CLI会打开timestamp前缀；旧fixture设置的`plain`实际回退compact，不是新支持的style。当前证据验证compact格式与既有同步logger路径，没有新增采集队列。

- ToolCall/run/sessionKey必须与已校验native tool.start相同；native_agent_session_id与agent-result.requested_native_session_id相同。进程由processSession/supervisorRun/PID绑定，拒绝命令/时间猜配。
- `clock_origin_monotonic_ns`、`observed_monotonic_ns`来自同一Node hrtime；仅在本域相减，独立泳道无UTC/控制器锚点。序号缺口、倒退或身份冲突使来源incomplete。
- `process.spawn_to_exit` 是spawn返回到真实child exit callback；`process.exit_to_pipe_eof` 仅在双流natural end齐备且最后EOF晚于exit时生成。EOF更早保留原序；close、wait settle、force-kill fallback不是退出或自然EOF证明。
- schema 2新增 `stdout_first_nonempty` / `stderr_first_nonempty`：在既有data回调中，原decoder处理之前记录每路第一个非空原始chunk；不新增reader/listener，不改输出正文或解码。两事件保留为instant，`process.spawn_to_first_output` 取同一进程、同一clock的spawn返回→两路最早首输出，时间相同按sequence排序。后代持有管道时，首输出可以晚于直接子进程exit；对应自然EOF之后的首输出拒绝解析。PTY不补造此测点。
- `first_output`：schema 1为`not_collected`；schema 2实见首输出为`observed`；未见首输出且双流自然EOF齐备、无已知emit失败时为`no_nonempty_chunk_observed`；未见首输出的其余情况为`unknown`。没有首输出时不生成区间、不填0；事件JSON时间字段是`monotonic_offset_ns`，区间使用`start_offset_ns/end_offset_ns/duration_ns`。
- 原 `native_event_consumer_interval` 保留；不与进程/SDK区间相加计算Sandbox时间。child exit=7仍为exited，任务成败来自独立业务结果。
- 完整observed记录只证明已采到的范围。A wrapper与B原生首输出分别采集，不能相互回填。独立预期进程数、全后代终态、PTY双流与全局drop仍未知；缺exit/EOF不得伪造complete。既有同步logger成本属于被测路径；当前B baseline/L0配对包含整个可选观测路径的变化，未独立分离guest logger成本。

实现位于 `experiments/r07/first_output_patch.py` 与 `pkg/performance/snapshot_native_process.go`。ECS原版/候选9组语义对照PASS，73个factory/logger回调含10个首输出，两容器已回收、0云/模型调用；证据SHA `029983f6dd47dd85b62a32d5d76822ac5698c7e4fa1255f65594b82b2c8666db`。分析器定向unit/race各225项（含子测试）、vet/build通过，证据SHA `f4e39e055aa4e57641ee18aa7182564c4f27a21e02a0bb26bbb3201193b30df4`，568文件分析器清单SHA `7e462d86330d149cf4c986df2eaa9029a4829523ee6830476fcae14b4bab4761`；不是新完整五门禁。候选镜像以SWR digest `sha256:ba1c0c46219c2de635109ffe16e672da1db83aefc950b003df85a7755bdd8554` 固定归档。

[schema 2真实华为B验收](VALIDATION_AND_REPRODUCE.md#r07-b-first-output)同原Tape完成4轮/693帧/3次exec/12项独立Verifier：30回调=27生命周期+3次stdout首输出；spawn→首输出分别3.407878、438.164376、280.955009ms，stderr首输出值为null。第一工具exit→最后自然EOF为2.796507ms，另两例EOF早于exit，不生成drain区间。Snapshot为complete/unknown、无诊断；2个Sandbox和1个模板均404，临时凭据回收、生产服务未改。云证据SHA `9fd94c87a04401e12e71e4d547eb23facdd3c5507a6f4b1d0c512950a46e957e`，离线对账SHA `c6ec81105b453278c90a61934ed4f9164febe23ed93ef5c8466dd381f6ab62e6`。新Trace的双域TP已PASS：controller 779区间/31flow、native 7区间/0flow（3首输出、3直接子进程生命周期、1排空区间），12来源与786区间并集精确对账，TP起止/时长误差0ns、processor errors为空，原件未变；证据SHA `823007b6c65a7503abc1e98d53764940382d754573f8cd68b6c5abae3ff90139`。新数据的原生首输出UI已实际PASS：Perfetto v59.1-1e9c61e98内置WASM中对T2执行SQL并点选timeline，438164376ns及operation/ToolCall/source/scope一致，泳道为UNALIGNED；本次未重复验证控制器域UI。证据SHA `75ded0256b8d22ace8f94b04b9a8f60acc7e0b8b57b93d3021ab4052c8762189`，3张截图已目视核对、10个自有浏览器PID/starttick确认消失，原件未改、无新增云/LLM调用。4条console error与9条warning及初次缺libatk启动失败均保留；加载既有public tools.env后成功，无新增安装。华为Verifier出网假设与全链覆盖缺口保留。

[schema 2真实E2B B验收](VALIDATION_AND_REPRODUCE.md#r07-e2b-first-output)复用相同四模块与v1.4分析器，从原E2B固定模板派生，使用其原Tape完成4轮/573帧/3次exec/12项独立Verifier。30回调含3个stdout首输出，spawn→首输出为7.587584、152.823030、161.493016ms，stderr首输出为null；T1 exit→最后自然EOF为4.437940ms，T2/T3 EOF早于exit，不生成drain区间。Snapshot complete/unknown、无诊断；两个Sandbox均独立NotFound，cleanup_errors为空，源码、Tape和生产服务未改，临时凭据已清理；派生模板保留用于复现，无运行中Sandbox保留。云证据SHA `be619aff4016477c861826d84867d09651358daa0dd31aff050380afc94186c2`，对账SHA `dd9897d474490c5f402ba8eb09185471979ca6a049e2dd875f155335bc956efa`；分阶段数据见[性能归档](../performance-archive.md#e2b-b-first-output)。E2B的573帧与华为的693帧来自不同Tape，本次只证明测点可用，不能据此做跨云排名；全树终态、PTY与guest logger独立成本仍未闭合。

E2B新Trace的TP验收已PASS：同一Snapshot SHA `cc85e91f00adab6498d62a416f14557c89e033acca368e938d4706ee2e4ed06c`导出控制器795区间/31flow及原生7区间/0flow，12来源、802区间并集与outcome精确对账，TP起止/时长误差0ns、processor errors为空、原件未改。证据SHA `77b751fbca06fd6efc1d38a344878c26e5ae7f08a796ff07279df42f9f91e771`。本次E2B UI为**UNRUN**；此前华为B实际UI仅作为同版本投影验证，不冒充本次E2B数据的UI验收。

历史schema 1的[真实factory、实际CLI格式、定向测试与完整门禁证据](VALIDATION_AND_REPRODUCE.md#r07-native-process-snapshot)：snapshot-1历史结果保留，snapshot-2的测试比较错误留FAIL，snapshot-3定向验证通过；568文件 `r07-native-release-1` 五门禁已PASS。随后[真实华为B验收](VALIDATION_AND_REPRODUCE.md#r07-huawei-b-native)使用原Tape完成4轮/693帧/3次exec/12项独立验证及停止回收；27条callback与3个直接子进程区间精确对账，分别22.511373、394.434524、303.746110ms。该次三例的两流EOF均早于exit，`post_exit_drain_ns=null`，不生成drain区间，不表示0ms或没有管道生命周期；旧原件没有首输出，不回填为schema 2。

旧v1.3误把B非空协议当成A身份源启用，故原review/TP1保留incomplete。v1.3.1只修离线scope：已验证embedded+`aries-native-process-v1`且A producer/index均不存在时跳过A专用身份解析；存在意外A文件、未知协议或不合法placement时仍保留原检查。ECS performance定向unit/race/vet/build通过后，仅重读同一真实原件；八阶段、业务结果、consumer/process区间均未变。新Snapshot integrity=complete、coverage=unknown，独立预期分母仍null，不把scope修复称为采集零丢失。

历史schema 1的TP2分别导入主控制域878区间/31flow和native单调域3区间/0flow，共用同一Snapshot及12来源；两视图并集精确、start/duration/end误差0ns，无processor错误，保留跨域未对齐。[该次实际UI](VALIDATION_AND_REPRODUCE.md#r07-huawei-b-ui)已验证同一ToolCall在两个独立域中的SQL数值、点选时长、身份和来源，原生泳道明确UNALIGNED；该检查不生成跨域差值。全树终态、后台/PTY云上证据和同步logger独立成本未因此闭合，华为Verifier出网仍assumed-unverified。此前R15 A四对使用565文件及旧冻结分析器，不回填为568文件B成本；新B schema 2四对单独归档于[本页已测范围](#r15-b-native-paired)。

新增[完整A取消样例](VALIDATION_AND_REPRODUCE.md#r01-native-cancel)只为1条预登记脚本ToolCall提供独立分母；初始native response/producer使用原始ToolCall ID，不误用后续模型history的方言变换。Bridge execution ID来自既有 `rand.Text()`，是不透明非空字符串，不要求UUID；SDK Process ID与guest receipt仍逐项精确匹配。取消后guest receipt可存在而退出边界缺失，两者分别表达：身份链已证实，guest terminal时间仍unknown。评分前独立PID/start/cgroup/心跳检查证明该fixture已停止，不回填未采集的退出时间，也不宣称全局零漏采。该故障实验的Verifier区间含额外隔离证明检查，不能与普通Strict回放作耗时对比。

## 单一事实源与版本

原始文件 → 一次解析/校验/身份关联 → `AnalysisSnapshot` → 各出口。Exporter不能重新读日志找相邻命令、另算duration或成功率。快照是可保存的不可变分析结果，`snapshot_id`标识其内容；投影时重新校验该标识。旧原始文件不被改写。

| 数据 | 版本与含义 | 兼容/缺失处理 |
| --- | --- | --- |
| performance manifest/index/runner-events | 新原始schema **2**；新增`observation`瞬时事件和可选身份属性 | 原schema1仍读；manifest/index/event版本必须一致；v1不接受v2 observation |
| model/events.jsonl 与 tape.json | 事件保留原格式及可选`model_request_instance_id`；新录制Tape v2支持完整多ToolCall数组，旧Tape v1保持原形读取 | 旧`first_*`仍是原观测点，不静默当新口径 |
| model/request-boundaries.jsonl | 独立schema1；每个handler HTTP invocation一request ID；首个Strict偏离可附加私有`request_diagnostic` | 无文件时明确旧源未采集；旧源无摘要不回填；摘要不投影到公共Snapshot/Trace |
| model/request-boundaries-index.json | producer计数`attempted/written/dropped/write_failed`、HTTP进入/终结数`requests_received/requests_finished`、clock和session origin offset | HTTP计数包括busy/鉴权拒绝，不是已接纳的模型轮数；缺index/坏文件/producer不一致→incomplete |
| harness/native-tool-events.jsonl / native-tool-events.index.json | 保留schema1与原字段；可选`clock`和`host_origin_offset_ns` | 旧consumer offset独立分域，不能仅凭UTC合域 |
| harness/gateway.log中的ARIES_NATIVE_PROCESS metadata | 可选schema 1/2；v2增加两路首个非空chunk，保留直接子进程callback、精确ToolCall/run/session与Node单调时钟 | 来源不可混版；v1首输出not_collected，无输出不造零区间；进程域独立，日志前缀不是锚点，未知全局分母不写100% |
| harness/native-tool-identities.jsonl / index / status | A显式启用；旧producer v1为五字段身份，当前v2增加invocation及明确pre_dispatch拒绝；SSH身份仍v1。status记录producer计数/失败，index固定原件SHA | 只有身份，无计时；缺失/损坏/计数冲突单列观测失败，不改 Agent 结果 |
| e2b-operations.jsonl / e2b-operations-index.json | 保留schema1，新增可选`observed_offset_ns`及index的`clock` | 无共享单调锚点的旧文件只保留结果和duration，不补造区间 |
| AnalysisSnapshot | **analysis_schema_version=1**；当前parser=`aries-analysis-v1.6.1-native-pre-dispatch`，历史资源流版为`aries-analysis-v1.5.0-resource-streams` | 新增可选`resources`，保留原事件/区间/身份；旧冻结快照缺该字段仍按原ID校验，重新解析的资源快照使用新ID |
| Chrome Trace metadata | **schema3**，`exporter_version=aries-chrome-trace-v3`；引用同Snapshot ID与原source SHA | 旧`--trace`入口保留；默认仅主Recorder域，其余域列出并可单独导出；schema2导入失败与修正版分开归档 |
| P1a OTLP manifest | schema1，`aries-otlp-http-json-v1`；固定Snapshot ID/parser/source SHA与operation→Span映射 | 仅可信本域UTC anchor生成epoch；未导出区间/边、instant/untimed计数显式保留；[离线Jaeger限定验收](VALIDATION_AND_REPRODUCE.md#p1a-offline-jaeger) |
| P1b Metrics manifest | schema1，`aries-otlp-metrics-http-json-v1`；离线账本/阶段指标限定PASS | 同冻结Snapshot的原窗口/账本、直方图成员与原ns；资源和缺失Goodput不补零；[测量契约](#p1b-offline-prometheus) |
| P1c资源Metrics manifest | schema1，`aries-otlp-resource-metrics-http-json-v1`；工程验证通过，后端限定对账通过 | 一次选择一个资源owner；保留完整流、源SHA和每个指标点的原sample IDs；[测量契约](#p1c-resource-prometheus) |

事件身份、source line、原始ns、源文件SHA留在快照。source SHA用于证据完整性，不是隐藏正文的通用办法。公共导出不包含prompt、Tool Result、命令正文、环境、token、原始异常。私有Tape只用于严格验证。

重复分析同一组已终态、未变化的原始文件应得到同一个`snapshot_id`；旧CLI的`--snapshot`与按run导出的`--trace`各自解析一次，再按该ID核对。`--trace`/`--trace-clock`/`--otlp`/`--metrics`及`--resource-metrics <stream-id>`的`--from-snapshot`入口直接读取同一冻结JSON并校验原ID，不重读原日志。更换parser版本或原始文件会改变内容身份。快照不是采集期的原子多文件事务，不能一边写原始证据一边把不同导出的ID差异当作分析器错误。

### Strict 拒绝请求的私有诊断

在既有 `request_matched/failed/REPLAY_DIVERGED` 事件中附加摘要，事件种类、数量、Strict 匹配和 cached retry 不变。仅摘要已通过原始及解码后凭据检查的请求：消息数、角色顺序、正文类型、文本及工具参数的 UTF-8 字节数/SHA、ToolCall/ToolResult ID。正文、reasoning、参数文本、任意 metadata 不复制。

`supplied_identifiers` 只保存请求顶层实际提供的 `native_run_id/session_id/session_key`；这些值是调用方声明，**不是已验证的身份映射**。缺失为 null，不能补用模型 session ID 或时间邻近推断。未解析正文的 `SESSION_FAILED` 等早期拒绝没有摘要；非文本内容不冒充已读取文本。此功能不是通用秘密识别器，私有原件仍按凭据规则保管。

匹配终点在生成摘要前采样，拒绝请求的 HTTP 总耗时如实包含诊断成本；baseline 不生成摘要。Snapshot 保留原事件及来源引用，白名单投影不会把新摘要带入公共 Trace，也不会据此新增 flow。工程验证与真实复现分开：见[拒绝诊断验收](VALIDATION_AND_REPRODUCE.md#p0-rejection-diagnostics)，原第八次请求的调用方仍未确认。

## 身份与关联

- 任务：run/attempt/occurrence/logical task/arrival/purpose。attempt必须属于run，不能凭工具名跨任务合并。
- 模型：model session、独立HTTP request instance、turn、frame index。相同turn的缓存重送是两个请求，不覆盖、不多算真实上游；鉴权/busy等拒绝可没有turn。接收帧和成功flush各按请求校验从0连续、唯一的`frame_index`；失败flush与first标记必须指向其实际帧。
- Gateway：client私有随机身份+逐frame序号形成delivery ID。accepted与terminal可能属于同request，但各有不同delivery。receive→enqueue_offer→consumed只按同delivery关联。
- A/B原生工具：native session/run/ToolCall ID/native sequence。共用Gateway collector；consumer看到result不证明进程退出，`command_output.end`也不证明所有后台写入者停止。A新增接线已通过release-2华为A回放及E2B限定样本，完整覆盖与进程终态仍未证明。
- A受管层：Bridge实际接受时新execution_id，经context固定到SDK Process UUID；重试产生新Process。独立候选已有9/9 PoC；full-agent-2 整 Agent 录制/回放各6/6归档exec精确贯通 native→SSH→Bridge→Process→guest。后续合并版本华为A同Tape回放仍6/6，并新增原生消费区间；仅证明该归档分母内身份，全工具应采覆盖仍unknown，不回填历史运行。
- 正式关联只允许`exact_id`与`verified_mapping`。命令文本/哈希、时间邻近、同退出码、单并发顺序均不构成可信身份。

存在映射不等于覆盖完整。完整性索引的“应写事件数”和“应有工具操作数”是不同分母；只有可独立得到冻结Tape/producer预期的情况下才报告应采覆盖率。未知分母用null/unknown，不写100%。

真实 `overlap-1` 进一步覆盖同命令后台重叠：两次调用分别携带自己的 invocation/SSH/execution/Process 身份，guest以PID+启动身份互认屏障，逆序完成仍2/2精确关联。八次准备/控制调用保持unlinked；这不是基于命令相同或完成顺序配对，也未新增native计时。证据见[重叠验收](VALIDATION_AND_REPRODUCE.md#r01-live-overlap)。

`r01-protocol-cloud-review-4`进一步核对真实SSH/Bridge/SDK/guest协议：5次身份请求中，READY后取消和同ToolCall的新invocation重试两次被接受并精确关联；重连复用旧invocation、错误scope、错误session三次被拒绝。取消的guest退出码/时长缺失，不补终态。证据SHA `2af8c8fa8d1a77b0f2e8b5e4cc6c498b7bfffea44c030293bd7181951fd14806`；原检查失败及原始输入保留。这是协议调用方的限定实测，不含native producer、Gateway、LLM、Verifier或进程树停止证明，不扩大归档ToolCall覆盖率。

### A 身份证据投影：不增加假时间线

`native_tool_identities.attempts[]` 保存每个 Attempt 的身份映射。解析入口是显式启用
`aries-ssh-identity-v1` 的 A profile；旧运行没有该配置和身份文件时维持原有缺测口径。
已启用A身份协议却缺文件时报告 incomplete，不能回退成未启用。已验证的embedded B进程协议不要求不存在的A producer/index；若意外出现A文件，仍保留placement/protocol无效诊断，不能宽泛跳过未知协议。

原生分母只统计**归档 assistant 消息中的 exec ToolCall**：校验 `agent-result.json`
请求的 native session UUID、telemetry index、session header，以及 `sessions.json`
中的 session key→UUID 绑定。它不证明 transcript 采集零丢失，也不包括 `process.poll/log`
等其他工具。`archived_exec_calls` 与 `covered_exec_calls` 均按 ToolCall 计数；
`producer_invocations` 另计，重试不能扩大 ToolCall 分母。相同 ToolCall 的每次 invocation
单独列出，只有其所有已知 invocation 均精确关联时才计入 covered。

每条 mapping 逐段校验 producer 五字段、Bridge 的 SSH request/execution ID、
SDK operation 中的 Process UUID，以及 `exec.wait` 留存的 guest 原始 receipt。
`evidence_ref + source_line + json_pointer` 指向具体原始记录；每个 evidence_ref 可回查
源 SHA。缺失、重复、冲突或越过 Attempt/Session 的身份均为 unlinked；辅助 Bridge
调用单列，不能按序号补齐。guest identity receipt 在 Popen 前生成，证明接纳的执行身份；
另列 `guest_completion`，不能由 receipt 推断成功或后代已停止。

该扩展没有 event、clock、duration 或新 flow。即使达到
`exact_for_archived_exec_observations`，也只表示该归档分母内身份吻合；
历史 A 原件的 native 工具计时仍 `not_collected`，全工具覆盖率仍 unknown；新采集接线单列验收，不回填旧数据。
旧 exporter 若丢弃新扩展字段，内容哈希校验会拒绝该新快照，不能静默导出不完整版本。

### A/B 原生消费区间与 SDK 客户端区间

`openclaw.NativeToolCollector`由两种placement共用。A的baseline不开启可选原生事件，L0及以上通过既有Gateway能力订阅；以实际确认的native run ID筛选。原始start/result按session、run、ToolCall ID配对，`native_event_consumer_interval`只表示ECS消费者两次接收之间的时间，包含传输和调度，不能命名为guest纯执行。native墙钟保留源值，不能与控制端单调时钟相减。采集超限继续消费到终态或排空期限，报告incomplete；观察失败独立进入`cloud-observation.json.native_tools`，不改Agent结果或Stop含义。

SDK Manager从attempt保存实际共享Clock，使清理及SSH后台context中的操作仍使用同一控制端锚点。操作start/end的`observed_offset_ns`与`client_duration_ns`来自同一对单调时刻；`exec.wait`起点是Process提交前，终点为Wait返回，包含提交、传输、执行和排空，**不是进入Wait才开始计时**。其他`adapter_rpc.*`使用各自RPC起止；按同run/occurrence/原始source/operation ID配对，允许反序完成。成功wait与失败stdin独立保留，不合并状态。

分析器只读一份SDK原件供区间与身份解析复用。已有共享锚点时生成`sandbox_client`区间；旧源进入`untimed_operations`，保留outcome、duration及source行号。Python `sdk_calls_duration_only`和guest duration仍是属性，不能画成虚构嵌套区间。该补丁不新增Bridge区间，也不凭时间或命令补造parent/flow。SDK producer计数证明文件内配对，独立应采分母仍unknown。

上述候选通过ECS定向unit/race，`p0-runtime-release-2`合并五项门禁通过。该版本华为A同Tape回放、12项独立验证及实际TraceProcessor对账通过：6个原生消费区间、145个客户端操作区间，失败/取消原样保留；完整性complete、全链覆盖unknown。旧失败运行的81个SDK终态仅投影为duration-only，与原始end行分母逐项对账，未补造旧时钟。分版本证据见[验证记录](VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates)。

### stdout/stderr 合并顺序：根因与已批准修复契约

未修改的固定 OpenClaw 镜像经同一真实 Bridge/Docker 目标，四次预登记执行
`printf out; printf err >&2; exit 7`，最终文本依次为 `outerr`、`errout`、`outerr`、`outerr`。
每次 stdout=`out`、stderr=`err`、exitCode=7 均相同。证据为 ECS
`/opt/aries-artifacts/observability-r01-native-stream-order-1/evidence.json`，SHA
`14a7404c8bc4c1ca8dbfdbd4a76d8ba8bc1750d60b0c2a6940c925eb3b9ab24d`；
对应源码/分流分析 `analysis.json` SHA
`550e38ab6471817b7c24047931568a049cc28c15bf95696615bfc9d736be6d66`。
这证明身份补丁不是该现象的必要条件；不证明补丁没有任何时序扰动。

根因是把两个独立输出流的**回调到达顺序**当成可复现文本顺序：
Docker `stdcopy` 顺序转发其收到的帧，但 `x/crypto/ssh v0.57.0`
分别缓冲 stdout/stderr 并以两个 `io.Copy` goroutine 写本机 FD；OpenClaw
`bash-tools.exec-runtime-CHHVz-P3.js` 的两路回调再分别调用
`appendOutput`，`bash-process-registry-17q1dHVV.js` 将 chunk 直接追加到
`session.aggregated`。云 guest 的双 PIPE/selector 也只能提供采集顺序。
现有接口没有保留跨两个 FD 的全局业务写入顺序，给 writer 加锁或改用
`CombinedOutput` 无法恢复它。尚未对每次实验做内核级追踪，不能指定某个
具体调度切换是某次 `errout` 的唯一发生点。

原 native PoC 第 3 次对照的原始返回一致性仍是 **FAIL**，不可豁免：
`observability-r01-native-poc-3/reconciliation-v2.json` 分别记录 9/9 精确身份及分流
IO/退出码通过、8/9 最终文本一致、1/9 文本不同；SHA
`749c0b69de73e0ece72749207d2450dd8559f5b59a178d7b3a9080c2a7b42fa7`。
分流相同不能代替 Strict Replay 的完整 ToolResult 匹配。

用户已批准**显式新版本的结果构造契约**、重新录 Tape 和严格回归，并进一步要求保留 ARIES 原生工具能力。曾以禁用 `process` 强制前台的配置已撤回；`tool_output_renderer` 只标记候选版本，不改变工具列表、后台权限、yield 阈值或 exec 参数。A/B 配置逐字节相同的 UT/race 证据为 `observability-r01-native-tool-policy-1/evidence.json`（SHA `772ad5cdbbf88901c11259d654d039759a4b5d517396206b82bdaafd3ed2d6d4`），该配置检查不等于证明所有模型 tools schema 分支；full-agent-2 已实际保留原生 policy 并完成前台 exec 的真实录制/回放，未运行后台/poll/PTY 回放。当前契约：

- 对实际以前台完成的非 PTY exec 分别累计 stdout/stderr，以固定格式与流顺序构造最终
  模型可见结果；保持每路内部顺序、退出码和信号，不改业务命令或重排字节。
- 新累计必须独立于会被 `process.poll` 清空的 pending 缓冲；沿用有界内存。
  截断规则必须按固定流结构定义，不能再按两个流的到达顺序截取。
- 原生层本来会解码、过滤部分字符。需单独验证跨 chunk UTF-8/ANSI 边界，
  不得把原始字节证据与模型可见文本混为一谈。
- 原始双流、流式观测与失败证据完整保留。后台任务、poll 的时间相关增量
  另立验收边界，不承诺此修改让所有异步工作负载自动可严格回放。
- 输出快照在原生 wait/error 边界冻结，只在外层真正选择前台返回后转换最终结果。
  不提前改变 registry、通知或原有 outcome；不能用 wait 时刻的 backgrounded 状态
  猜测最终分支，因为 finalizer 尚未返回时仍可能触发自动 yield。
- 新 A/B 录制与回放使用同一新镜像及契约，重新录 Tape；旧 Tape 与正式镜像
  保留。Strict matcher 不排序、不忽略 stderr、不归一化差异。

验收须覆盖反序到达、多种 chunk 分割、双流与单流、非零退出/信号、空输出、
超限截断、取消和并发；固定来源的新 Tape 再进行真实 Tool 与独立 Verifier 回归。
这会改变模型可见 ToolResult，不能作为“只增加观测、不改变原生返回”的修补
静默启用。`observability-r01-output-order-6/evidence.json`（SHA
`c661bd005bfb2ac7e54ce65dfc06e14a3a961ef8478063627f5e4825d9ba10a6`）记录保留原能力的候选：
5 组 helper、4 项 B 并发、26 项 A 固定矩阵。其中 25 项预期行为通过，另 1 项
finalizer 延迟负例在原版和候选均复现 `running` 后 `No session`，标为 KNOWN_LIMIT，
不能视为正常后台生命周期通过。此前 `output-order-4` 只覆盖旧强制前台配置，
`output-order-5` 的独立 Node probe 提前退出失败仍保留。其后 full-agent-2 用同一组合新录 Tape 并严格回放通过：两次各 6 条实际前台 exec，真实结果/模型历史及 12 项独立测试均核对。此证明不延伸到后台/poll/PTY。

原版前台 final、已结束 poll 与 log 使用按回调到达顺序构造的 aggregate；运行中 poll
虽然将当次 drain 的 stdout 放在 stderr 前，但会消费 pending，读取窗口由调用时机决定。
因此还需分别考虑跨流顺序、poll 分块窗口、自动 yield 前台/后台分支三类差异。
Strict Replay 对真实工具结果引起的请求历史变化必须继续拒绝，不能靠禁用工具、排序历史
或替换输出使其通过。内部版本是否另有优化仍待直接核验。

## 时钟与精度

Recorder拥有单个进程的实际monotonic anchor。`ClockFromContext`返回同进程能力，`Clock.Offset(time.Now())`对该anchor求ns；UTC仅作为本域epoch锚点。Clock的monotonic部分不序列化，JSON解码后的UTC不能用来重建跨进程单调时钟。

新Model sidecar的`monotonic_offset_ns`已经直接来自所声明Clock；index还保留`session_started_offset_ns`，不应再把它加到这些新事件上。Model writer的`emitter_id`独立于Clock的`emitter_id`，共享时钟不等于共享事件序列。B的`host_origin_offset_ns`才用于将旧形式collector offset换算到共同Recorder域。当前解析器对只有legacy模型文件的输入仍保留独立域，不用Tape.StartedAt冒充精确anchor。

记录序号是writer入队顺序，多个goroutine事先捕获的时间戳可能交错；不强制用sequence排序假定时间单调。模型请求内部与同一Gateway delivery的真实起止顺序仍须一致。

跨ECS/guest/Adapter没有校准窗口、漂移和误差上界时为`clock_unaligned`。UTC/NTP接近不改变此结论。只有duration没有start的旧SDK/Bridge结果保留为duration-only，不用结束后audit时间倒推开始。

E2B `exec.wait` 的可选 `guest_boundaries` 使用 guest 自己的 `time.monotonic_ns()`，相对一次 Popen 前锚点记录 `popen_return_ns`、可空的 `first_output_ns`、direct-child 单次 wait watcher 的 `child_exit_observed_ns`、两个管道 EOF 的 `pipe_eof_ns`；`guest_duration_ns` 保持原有含排空语义。旧的两字段 terminal 仍兼容。它们不能跨主机换算成 ECS 时间、kernel 时间或 CPU 时间；R07 closure-4 及后续 semantic/identity 回归只证明 wrapper 边界，既有 A/B Trace 不回填这些历史缺失字段。

B native私有PoC使用固定华为B镜像在ECS本地容器运行，原版/候选五对业务字段一致：前台双流exit7、后台持pipe及poll/log、process.kill、观测sink抛错、PTY。候选27个真实callback事件的ToolCall/session/run/PID精确对应；直接子进程退出到最后自然EOF间隔311.383578ms，属于同一Node单调时钟，期间已见父PID消失、后代仍活、poll仍running。它不是kernel时间或全树终态。PTY双流EOF为NOT_COLLECTED；模拟fallback无exit/end为UNKNOWN，不能说真实SIGKILL失败。

此三模块PoC证据SHA `5e98d6a5084e2824309ce58e4ccc82591e754575f74213ddd73a9d2c1c2ae164`，0云/LLM，两容器及已知fixture进程确认回收。该次局部业务比较排除易变时间/生成ID/PID文本，不是Strict Replay；当时尚未接入Gateway sink或Snapshot。后续四模块factory接线、实际CLI日志包络与Go来源解析已分别验证并进入完整工程门禁，见本页顶部；不升级这份旧PoC的范围，也不回填既有B Trace。随后schema 1真实华为B同Tape回放及双域TP已限定通过，见[历史B验收](VALIDATION_AND_REPRODUCE.md#r07-huawei-b-native)；该旧原件未采首输出。schema 2首输出的新实测见[本页契约](#r07-native-first-output)，完整后台终态与PTY drain仍未闭合。

Chrome JSON的`ts/dur`单位为**微秒**，`displayTimeUnit=ms`只影响显示。快照保留整数ns；导出记录浮点投影的最大数值误差，不能把ns存储精度说成ns测量准确性。默认Trace只含主Recorder同域；`--trace-clock <domain>`导出独立视图。不同lane不解决跨域同时性的未知问题。

有起止且同域的区间才画slice；未终结不延到deadline。并行操作独立lane，不能把重叠slice总和当wall time。跨域关系可以保留证据关联，不能画有时间长度的伪网络flow。当前不推导全局关键路径、网络单程耗时、模型思考或CPU时间。

Chrome flow的`id`是显示用的UInt64十六进制字符串，取Snapshot原始边序号加1；不同clock视图不重新编号，也不截断哈希。完整稳定因果身份保留在`args.causal_edge_id`，并带`from_operation_id/to_operation_id`；结束事件`bp=e`绑定对应slice。部分Perfetto版本不将s/f的所有args存入flow表，导入验收须用两端slice的operation ID回查原Trace/Snapshot，不能假定UI能直接显示所有flow args。修复仅改变显示格式，三份真实Snapshot逐字节未变；实际导入状态见[显示层修复记录](OBSERVABILITY_IMPLEMENTATION_REPORT.md#perfetto显示层修复与独立导出器)。

新单次E2B诊断回放的离线Trace Processor验收已通过：`p0-diagnostic-perfetto-1/evidence.json` SHA `aa552f48c74af4761022b88aa320ef132f302ac92211c1f636dff851005ec413`，431区间、43条flow、21个来源SHA核对，operation/outcome/flow端点一致，导入误差0ns、processor errors为空，原件未变。Snapshot integrity=complete、coverage=unknown；这份Trace的UI仍UNRUN，不借旧UI结果宣称已浏览新Trace。

## 八阶段：保留原值，子区间只解释测得范围

| 阶段 | 原始字段 | 实际边界与限制 |
| --- | --- | --- |
| 排队 | `attempts[].queue_duration` | `scheduled_at→admitted_at`；不含随后`dispatch_delay`，也不是云平台内部排队 |
| 任务 Sandbox 创建 | `timing.sandbox_create_duration` | 任务`ToolSandbox.Start`调用；不含提前构建模板或评分Sandbox创建 |
| 工作区准备 | `timing.sandbox_prepare_duration` | `Benchmark.PrepareSandbox`调用，含真实上传与准备执行；不含Bridge注册 |
| Harness 启动 | `timing.harness_start_duration` | `Harness.Start`调用；OpenClaw B包括Gateway连接认证，A的Connect在后续Run内；Jiuwen A/B 为原生服务启动/就绪检查 |
| Runtime 执行 | `timing.runtime_execution_duration` | OpenClaw A/B紧贴`Client.Agent`前后；Jiuwen A/B围绕HTTP/SSE `Client.Run`，`agent_started_at→agent_finished_at`；不是整个`Harness.Run` |
| 停止与撤权 | `timing.stop_revoke_duration` | 顺序调用`Harness.Stop`和`Bridge.Stop`；返回成功与正向停止证据需同时核对 |
| 独立验证 | `timing.verifier_duration` | `Benchmark.Evaluate`整段，包括候选捕获、新环境、执行与报告 |
| 清理 | `timing.cleanup_duration` | Runner任务/评分环境清理；不含attempt收尾和run级共享清理 |

以上JSON duration单位均为纳秒，`attempts[].timing`是对应任务计时的副本，不能与`tasks[].timing`重复相加。快照阶段表照抄原ns并标duration-only，**不从阶段列表先后关系拼时间线**。Trace的`agent.run`覆盖整个`Harness.Run`，因此通常比Runtime字段更宽；两者不能互换。历史数据中Bridge注册、attempt构造/收尾等仍在八阶段之外。新版本另采`timing.bridge_prepare_duration`，排在工作区准备与Harness启动之间；不回填历史八字段，未到达记缺失。更详细的阶段定义见 [performance-stages.md](../performance-stages.md)。

## 新增边界的字典

| 观测 | Producer/计时点 | 可以解释什么 | 不可以解释什么 |
| --- | --- | --- | --- |
| HTTP received/finished | model handler进入/真实handler结束 | 单次HTTP请求从进入到返回，包含本handler内鉴权及失败/busy/reject | cloud mux拒绝的未知token、上游服务端内部排队 |
| body read / JSON parse / strict decision | Proxy实际操作前后 | 控制器读体、解析与严格比较成本 | GPU prefill/decode |
| upstream call/headers/first完整SSE | 真请求发出/HTTP返回/解析到完整帧 | 代理观察到的等待；包含传输和服务端未知成分 | 纯模型计算或纯网络 |
| content/reasoning/tool delta | 实际解析到对应首个非空语义delta | 真实协议类型的首响应点 | token个数、相同类型不存在时的零延迟 |
| frame receive / flush | Proxy真实收完整帧、写/Flush返回 | 发送端流式进度；可与recorded目标计算迟到 | Agent已消费、网络对端已收到 |
| recorded / zero / synthetic / cached_retry | 原Tape时序或明确模拟策略，逐请求保留实际值 | 计划发送与实际发送对照 | zero零成本、模拟时序等于录制时序 |
| gateway.frame.received | ECS Transport.Receive返回立即捕获time，解析后记录 | controller收到完整frame | guest发送时刻或网络耗时 |
| gateway.*.enqueue_offer | 非阻塞本地channel发送前 | 实际入队尝试时刻，和同delivery消费可对照 | 精确channel commit时刻；offer不保证成功 |
| gateway.*.consumed | 从本地队列取出且有效性检查后 | 包含排队和本地调度的消费边界 | 原生工具执行终态 |
| gateway.agent.send_started/returned | Transport.Send真实调用两侧 | 本地send调用范围 | 请求已经被Agent接受 |
| gateway.agent.accepted | 接受合法非空runId响应后 | 远端承认该运行 | 工具已经执行或完成 |
| followup_received / terminal_validated / returned | 相关响应收到 / 合法最终结果解析后 / 本地Agent调用返回 | 畸形/重复accepted不会伪造terminal；本地失败/取消单列 | 远端进程已退出；`returned`成功不等于Verifier成功 |
| candidate.capture | 候选校验、bounded download、只读落盘与SHA完成 | 稳定候选导出耗时；产物ObservedAt仍原时点 | Verifier开始时间、任务执行deadline |
| verifier.prepare | 新评分环境启动后安装评分输入 | 上传候选和私有测试及准备成本 | 环境创建（已有独立span） |
| verifier.exec | 实际pytest调用/返回 | 评分进程调用耗时；返回transport错误如实标记 | 测试全通过（需报告解析） |
| verifier.report | stdout/stderr落盘、结果检查/下载/解析 | 评分证据处理成本 | 模型或原工具运行耗时 |

终帧收尾修复：record 的 `[DONE]` 与合法尾注释等待上游完整 EOF 校验后原样 Flush，其他帧照常流式发送。`upstream_frame_received`/Tape `OffsetNS` 仍是实际接收时点，`frame_flushed` 是实际发送时点；二者之间可能包含 EOF 等待。全部帧 Flush 成功后客户端正常关闭不再使请求反向失败；提前取消、读失败及写入/Flush 失败仍保留。新真实云结果单独验收，旧失败不重写；细节见[模型收尾契约](../implementation/model-replay.md#终帧交付与正常断开)。

非流式 JSON 使用 `upstream_json_received`（完整 HTTP body 已读完）与 `json_flushed`（原始 JSON 写入并 Flush 成功）；没有 SSE 首帧、首 token 或逐 chunk 时序，相关值保持未采集。录制时序只保存真实完整响应偏移，synthetic 仅模拟首次完整交付延迟。`model_branch` 标记精确原生 `task_tool` 因果路径；同一 Attempt 共享 HTTP 请求分母和单调时钟，子分支历史与游标独立。入口路由、等待父请求提交的时间包含在 HTTP handler 区间内，不伪装成上游模型等待。详见[子会话协议](../implementation/model-replay.md#jiuwen-子-agent-与非流式-json)。

Gateway只输出受限opaque ID和受控status，auth/challenge/token/正文不进入事件。B原生host offset属于consumer观测，旧字段名/旧注释不能改变真实边界。

`upstream_first_*`只表示真实上游完整帧接收；`downstream_first_*`在真实写入和Flush成功后记录，record/replay共用此发送侧口径。旧`events.jsonl`中的`first_*`在record与replay的原始位置不同，不能混在一起作同口径首响应比较。A的ResponseOnly客户端有意不选择native事件，此处没有消费观测不是丢失已选择的事件；A原生工具源仍单独标缺测。

## 完整性与正确性是两条轴

`integrity_status`用于记录文件/索引/丢失是否可信，`coverage_status`表示是否采到目标范围。not_executed、not_collected、unsupported、unlinked、clock_unaligned、incomplete、dropped不能变成0。

请求生命周期配对完整，不等于每个内部阶段都已闭合。例如上游HTTP调用失败时有`upstream_request_started`但没有headers：快照保留真实start instant，标`phase_coverage_status=incomplete`和`missing_boundary=terminal`，输出coverage/diagnostic，不拿handler终点补造headers。此时文件记录完整性仍可为complete；HTTP失败仍是HTTP失败。收到headers但返回503时则保留真实headers区间及HTTP状态，不由区间存在推断上游成功。

整个A native源缺失会产生`not_collected`和整体incomplete；这是采集范围缺口，不等于已有模型或Runner文件损坏。本轮E2B A严格回放成功而快照仍incomplete，B严格回放成功且integrity complete、coverage unknown，正是这两条轴的实际例子，见[真实结果](OBSERVABILITY_IMPLEMENTATION_REPORT.md#历史-release3-真实回放结果)。请求/事件的producer分母不能替代独立的应有ToolCall分母，也不能证明模型到工具再到guest的完整因果链。

Recorder与模型事件队列容量4096、非阻塞入队。Overflow/write失败由producer计数揭示，包含“整对开始/结束都丢掉”。关闭状态链为：禁止新入队 → queue drain → file Sync/Close → 原始 JSONL `events_sha256`/`events_bytes` → 临时 index Sync+Rename 发布；`finalized_at_utc` 和 `flush_status` 只描述进程内观察到的步骤。最终index缺失意味着未完成关闭。关闭后写入单独的`late-after-close.json`或`request-boundaries-late-after-close.json`使原索引失效，不覆盖历史index。新字段由Snapshot可选校验，旧证据仍兼容。

剩余限制：文件系统写/Sync尚无可取消的有界退出证明；late marker自身写失败或进程崩溃仍可能无法持久化失效信号。对此保留R05 BLOCKED，不能只凭完整JSONL说完整。boundary、计量sidecar与legacy `events.jsonl`的已返回文件错误均通过`ObservationError()`独立上报；cloud挂到Observer，CLI记录`observer_status=failed`。`Close()`仍返回Strict/会话未完成与必需Tape错误；业务成功不等于证据完整。

L0 的两例真实 fsync 延迟已验证关闭顺序：模型 boundary 最终 Sync 时，任务 Sandbox 与 Harness 已清理，共享 run network 仍待 `cloud.Close`；Recorder 最终 Sync 在共享云资源清理之后。独立 owner 中断挂起进程并收回剩余 network，四个 Sandbox 均独立确认 NotFound。两例只记 `PASS_FAULT_CONTAINMENT_ONLY`，任务为 interrupted、Observer 为缺少最终 index 的 incomplete，不计完整回放成功。注入属于可中断的 ptrace 延迟，不证明内核 D-state 或生产 Close 有界；该历史L0试验未覆盖L1；当前L1文件锁与资源删除的修复及剩余边界见下文。原始证据见[两例 fsync 收容验收](VALIDATION_AND_REPRODUCE.md#r05-fsync-containment)。

后续事件文件进程代理的8例 PoC 保留父 producer/clock，实际终止并等待 worker 后原 Close 返回错误，故障 index 保持 incomplete。该实现仍是 test overlay；父 hash/read/index 与其它同步 writer 未隔离，未覆盖实际 Runner 的生产关闭或内核 D-state，R05 状态不变。见[进程文件代理证据](VALIDATION_AND_REPRODUCE.md#r05-process-file-poc)。

### L1 资源采样与业务结果分离

当前实现把采样生命周期锁与文件锁分开。`Stop` 取消定时采样、等当前 Metrics RPC 返回，并把每个 sampler 唯一一次删除前 final snapshot 暂存内存，随后调用 `manager.Stop`；不等待资源文件 Write。attempt 收尾依次停止采样、`manager.Close`、`finishObservation`；最后才真实 join 采样 goroutine、写 final record、Sync/Close。禁止删除后新发 Metrics。原 JSON schema、RPC 起止含义和 `sandbox_metrics_jsonl_only` FileIO 口径不变；Observer.Duration 仍是 started→finish，fallback 清理路径现包含删除时间，不能当作纯采样耗时。

`experiment.Observation()` 已返回的错误只标记 `tasks[].observer.status/error`，不再写进 Task.Error 或业务错误链。模型 observer 的后续错误追加到该报告；真实任务、清理、Strict/会话未完成与必需 Tape 错误继续失败。CLI 的完整 RunResult JSON 保留观测失败，不以业务 exit 0 表示观测成功。

`metrics-cleanup-1` 的 cloud 包 unit/race 各21项顶层、32项含子测试及 vet 通过；在 Write/Sync 门闩释放前确认 owner 已删除 Sandbox，释放后才验证真实 join、文件返回计数及错误。`cloud-observer-outcome-1` 先在旧代码复现错误污染，再通过 app 包 unit/race 各95项顶层、169项含子测试及 vet。均为ECS本地验证，0云/LLM；[精确证据与风险](OPEN_ISSUES_AND_RESOLUTIONS.md#r05-r07-fault-exits)另列。五文件组合`observability-r05-cleanup-release-1`已通过build/test/race/lint/integration五项门禁；随后同一组合E2B/华为A/B的4次正常L1真实回归均通过；各次Metrics返回早于对应kill开始、原始Write字节及Sync完整，Observer均succeeded，见[真实复现](VALIDATION_AND_REPRODUCE.md#p0-mainline-l1)。这不是云端文件故障或成对成本实验。普通文件永久挂起仍会阻塞 finish，SDK operation 日志也仍可能阻塞 Metrics RPC；这些边界继续 **BLOCKED**，没有将取消请求或门闩释放当成 writer 有界退出。


A 候选身份 producer 的 JSONL 上限与采集端统一为 4 MiB。单独的私有 status 文件使用
临时文件→rename 发布九字段：`schema_version`、`scope_id`、`agent_session_id`、
`producer_instance_id`、`attempted`、`written`、`write_failed`、`status_write_failed`、
`failed_latched`。这是本 producer 实例的累计计数，不是所有工具的应采分母，也不是
断电耐久承诺。不同实例冲突、同实例旧状态回退或任一次写入故障都会保留失败状态；
后续写盘恢复不抹去它。status 自身一直无法落盘时，不能宣称失败标记已持久化。

纯日志 open/write/close 或 status 发布失败不改变业务命令、stdout/stderr、exitCode；
scope/session 冲突、私有 SSH 身份协议错误仍拒绝执行。Harness 用
`NativeIdentityObservation()` 暴露采集结果，cloud 的 `native_identity` 子报告独立记录
失败，不把它加入 Harness Run/Stop 或 Cleanup 的业务错误。真实 Stop/revoke 错误仍
照常阻止评分。普通文件同步写入可能阻塞的限制仍在，不能因捕获异常便声称有界退出。

## 吞吐、Goodput、资源与P1

保持原Attempt账本。Validated要求独立Verifier成功与现有Stop/revoke确认；不自动要求cleanup成功或Huawei出网实测。Goodput还要求ArtifactProven及产物ready满足显式deadline，分母为包含admission/drain的实际观测窗口；不是评分完成deadline。失败、未启动、取消、divergence都保留。

<a id="p1b-offline-prometheus"></a>

### P1b离线Metrics实现契约（账本/阶段指标限定PASS）

`ProjectMetrics(AnalysisSnapshot)`复用原Snapshot身份校验，返回标准OTLP/HTTP JSON `request.resourceMetrics`与独立`manifest`；不读raw日志、Jaeger或文件名补充上下文。`--metrics --from-snapshot`读取与两种Trace相同的冻结JSON；`--metrics-send`只负责一次传输。ECS定向unit/race各274项及vet/build通过，三份真实冻结快照的首发/重复查询均逐值对账通过；资源与未配置Goodput未冒充实测，证据见[验证归档](VALIDATION_AND_REPRODUCE.md#p1b-offline-prometheus)。

| OTLP指标名 | 类型/单位 | 数据来源与含义 |
| --- | --- | --- |
| `aries.attempt.lifecycle` | cumulative Sum；计数无单位 | 原planned/admitted/started/executed/completed/verified_succeeded，标签`phase`；阶段计数重叠，不能相加作总任务数 |
| `aries.attempt.outcomes` | cumulative Sum；计数无单位 | 原succeeded/failed/blocked/canceled/timed_out/unknown/not_started/loadgen_dropped，标签`outcome` |
| `aries.attempt.cleanup_failed` | cumulative Sum；计数无单位 | 原cleanup_failed，独立于终态计数且可能重叠 |
| `aries.attempt.throughput` | Gauge；`1/s` | 原started/executed/completed/verified_succeeded各速率，标签`phase`；原指针缺失就不发该点 |
| `aries.attempt.goodput_count` / `aries.attempt.goodput` / `aries.attempt.goodput_deadline` | Sum计数 / Gauge `1/s` / Gauge `s` | 分别复制原GoodputCount、Goodput、产物ready deadline；缺失不补0，不重新筛选成员 |
| `aries.window.duration` | Gauge；`s` | 原ObservationDuration/DrainDuration，标签`scope=observation/drain`；不以序列化UTC差替换单调时长 |
| `aries.stage.duration` | explicit Histogram；`s`，cumulative | 同Snapshot八阶段已采duration，按`phase/outcome/scope`分组；outcome是Attempt终态，不是阶段成功判定 |

Sum与Histogram的`startTimeUnixNano/timeUnixNano`固定为原AttemptSummary的WindowStartedAt/WindowFinishedAt，Gauge在原窗口末点表达该次实验值；不用当前时间移动历史，也不重算原成功分子、吞吐或Goodput。一个末态累计点不能用`rate()/increase()`补造吞吐。缺AttemptSummary返回`NOT_COLLECTED`与空指标；非法窗口报错。阶段只纳入`measurement_status=duration_only`且duration非null的原值，真实0保留，缺测列入manifest，不产生0样本。

Histogram固定边界为`[0.001,0.01,0.1,1,5,10,30,60,300]`秒，OTLP `bucketCounts`是各桶计数，最后一桶覆盖+Inf；Prometheus可将它映射为累计`le`桶。manifest保存每组成员的Attempt/phase/outcome/scope/source引用、原始duration_ns、整数sum_ns/count/bounds_ns/bucket_counts。秒数是浮点投影，不能承诺Metrics纳秒无损。取消样本中的`verifier`时长可能只是Evaluate入口提前取消，例如124309ns且Verifier未运行；标签与原成员必须保留，不能称独立测试耗时。

原P1b验收的三份v1.4冻结Snapshot没有资源采样值，`resource_gauges`保持`NOT_COLLECTED`；v1.5可选资源流使用下节的独立owner出口，不混入账本批次。原Goodput要求已验证候选及产物deadline；Snapshot没有逐Attempt的ArtifactLatency，不能重新推导资格，也不把Validated自动升级为整体成功或cleanup成功。manifest保留原AttemptSummary、window、source SHA与完整性/coverage；complete/unknown仍是两条独立质量轴。

实际标签仅`phase/outcome/scope`，resource仅低基数`service.name=aries-offline-metrics`；run/attempt/snapshot等证据身份只在manifest，Tape不进入timeseries。backend/placement/mode不在当前Snapshot中就不猜。每个不可变批次使用独立TSDB或明确隔离的序列空间，避免同标签历史批次被误接成一条持续Counter；原窗口查询需核对count/sum/buckets、重发、乱序与reset。

Metrics发送器为单次HTTP、30s超时、无自动重试。Prometheus的HTTP200空body只记`accepted_unverified`，必须另查原历史时间窗口；partial_success拒收数/警告保留，超时为delivery_unknown。人工重复发送是独立验收动作，不能据HTTP成功或稳定payload宣称exactly-once；未采/未知不因接收成功而变完整。本次每份117条业务序列/117历史样本，重发不增样本，值误差0；时间按毫秒截断，原ns不改。实时导出、资源Gauge与生产容量/耐久不在本实现的完成声明中。

<a id="p1c-resource-prometheus"></a>

### P1c资源流与独立owner Metrics

`BuildSnapshot`仅在run-result身份确认后读取已有controller、A外置monitor及Sandbox provider资源归档，写入可选`resources.streams`；每条流固定owner、Attempt、scope、来源、时间依据和源SHA，保留重复、空读数和失败记录。资源时钟留在流内，不向原Trace添加空泳道或合成Span。无`resources`的旧快照仍按原ID严格装载；三份旧快照的Trace、OTLP、账本Metrics投影全等已实测通过。候选2的ECS unit/race/vet/build通过（证据SHA `2ee63b2270f77d36af14512c82a4d7194f1aa59fe5641291de992419297eced9`）；首版误把monitor零基sequence按一基解析的真实FAIL保留，修正后重新验证。资源后端限定对账通过，统一证据见[验收归档](VALIDATION_AND_REPRODUCE.md#p1c-resource-prometheus)。

| 资源来源 / scope | 时间与数值口径 | 保留边界 |
| --- | --- | --- |
| `controller-resources.start/end.json` / `controller_shared_process_self` | 独立`anchor_utc`加采样完成offset；保留采样开始/完成bracket、CPU与GC/分配累计读数、heap及原delta | MemStats与Rusage顺序读取，完成边界不是原子采样时刻；RSS是进程生命周期峰值，delta不成为区间峰值或新时间序列；不能拆成Runner/Proxy/collector |
| `<occurrence>/monitor/resources.jsonl`及index / `container_resource` | 原`time`、CPU累计ns、内存usage/limit；按runtime owner分流，零基sequence与index计数核对 | `cpu_percent`原值保留但不导出，避免首次baseline占位0被解读为空闲；不从累计值补造进程启动epoch |
| `sandbox-metrics.jsonl` / `sandbox_provider`，E2B | 使用原RFC3339 provider timestamp和固定SDK声明的count/percent/bytes；保留每个RPC返回的全部reading | 轮询可反复返回历史值，5秒nominal interval不保证独立采样；空Verifier读数不是资源为0 |
| 同文件，Huawei | 保留原`ts`、requested-at与RPC duration；仅显式count/percent/MiB字段具备已证单位 | `ts`单位未证，不能用请求时间代替CPU采样epoch；原值保留，当前不生成资源指标点；无后缀mem/disk单位仍unknown |

`ProjectResourceMetrics(snapshot, streamID)`只投影所选流，CLI为`aries-perf --resource-metrics STREAM --from-snapshot FILE`；原`--metrics`仍只导出账本/阶段。每个owner使用独立冻结payload和独立TSDB，与账本批次也分开；标签仅scope、可选purpose/component，resource仅`service.name=aries-offline-resources`。run/Attempt/Sandbox身份留在manifest，不做高基数标签；异步owner不相加、不插值、不丢弃某owner来消除序列冲突。

所有值均为Gauge；累计读数的指标名带`_observed`，表示实际观察到的累计值，不声明Counter epoch或rate。ns转`s`，bytes及已证MiB转`By`，百分比用`%`且不除以100，count无单位。输出为float64，manifest同时保存原精确数值、单位、export value及sample IDs，不能称整数/纳秒无损。同owner、同指标、同timestamp且原数值相等时合并一个输出点，保留全部receipt IDs；不同时间的相同值仍保留，同时间冲突返回错误，不取最后值。合并指标点不等于证明独立provider样本数。

仅已验证时间与单位的值可导出；无可导出点为`NOT_COLLECTED`，来源不完整仍报`INCOMPLETE`。IO/Network缺测不补零，disk usage不是disk IO；Sandbox资源不归因单Tool，采样值不证明短任务真实峰值。HTTP接受与实际历史查询分开验收，已完成的真实查询只覆盖所列资源流，不等于R08整体完成。

## 观测级别与 R15 扰动实验

观测级别只控制可选证据写入和采样，不改变 Runner、Strict Replay、Tool、Verifier、Stop/revoke 或 Cleanup。`baseline` 不创建 performance、model boundary、E2B operation 和 B native 时序观测写入，仍保留任务结果和独立验证；`L0` 开启控制面、模型边界和 native 事件，关闭 provider 资源采样；`L1` 在 L0 上开启 provider 资源采样；`L2` 预留给授权 profiler，本期实现与 L1 相同，不得写成已经启用 perf/eBPF。无采集写成 `not_run`/`unsupported`，不写 0。

新 A 身份协议独立于观测级别：显式启用 `native_observation_protocol` 后，baseline
仍保留逐调用身份、producer JSONL/status 和归档。新 A 的 baseline/L0 配对只能测量
可选观测的增量成本，不能声称测出了身份机制本身的绝对开销；两个条件的原生工具能力、
renderer 和协议必须相同。此定义不反向改写历史 B 的已执行条件。

当前 cloud Runner、HTTP ModelProxy 与 Go collector 共用一个进程；模型事件里的
`ReadMemStats/Getrusage(RUSAGE_SELF)` 是全进程计量，不能分别归因或重复相加。
组件独立 CPU/RSS/GC/分配仍 unknown。最终文件大小、事件数和 producer `written`
不等于文件 Write/Sync 调用次数；Node 的 `writeSync` 也不等于 `fsync`。
新成对实验需要明确全进程起止与实际文件 I/O 的采集范围；尚未实现的计量不可填零。

新增 cloud 资源文件 `controller-resources.start.json` / `controller-resources.end.json`
为 schema1，scope=`controller_shared_process_self`；它们是成对实验原始成本证据，
现已进入[独立资源流](#p1c-resource-prometheus)，不生成Trace Span。入口使用本进程 monotonic anchor，UTC只留锚点；
每次样本记录采样前/后 offset，MemStats 与 Rusage 顺序读取，不宣称原子同时采样。
起点先于 profile 解析；有合法输出目录后才落盘。终点位于所有 cloud cleanup 和模型
HTTP server 关闭之后；早于最终计量文件自身的编码/写盘。外层 wait4/time 另计全进程
退出开销。初始文件 `incomplete/end=null/delta=null` 不被修改；最终文件记录初始落盘
是否成功。配置在目录创建前失败时没有账本，须依 CLI 失败记录，不能填零。

CPU 使用 RUSAGE_SELF 的 user/system 累计值及差值；Maxrss 换算字节，仅表示生命期峰值。
TotalAlloc/Mallocs/NumGC/PauseTotalNs 分别保留累计起止及差值，Alloc 仅作起止 gauge；
不强制 GC。CPU 采集失败为 null/unavailable，累计倒退拒绝 delta，缺终值为 incomplete。
这些值不能独立归因 Runner/Proxy/collector，也不包含远端 Sandbox 或其它进程资源。
计量文件持久化仍受普通 Write/Sync 不可取消的 R05 边界影响；初始写入在创建云资源前，
最终写入在清理后，不借“计数已增加”证明物理持久化。

新增两个索引的可选 `file_io` 对应 `runner_event_jsonl_only`、
`model_request_boundary_jsonl_only`。记录 Write started/returned、请求/实际返回字节、
error、short、无效 n，以及 Sync started/returned/error。既有调用只计数不增加重试、
Sync 或缓冲；错误锁存后跳过的事件不是新的 Write。返回字节是文件 API 返回值，
不是物理磁盘落盘量。旧索引无此字段仍为未采集；baseline 未创建 writer 为 not_enabled。
后续 metering-2 复用同一计数定义，新增三个明确范围：

| 证据位置 | measurement_scope | 条件与边界 |
| --- | --- | --- |
| `model/events-file-io.json.file_io` | `model_legacy_events_jsonl_only` | schema1，带 session identity、events_file 与 complete/incomplete；不包含侧车自身写入 |
| `e2b-operations-index.json.file_io` | `e2b_operation_jsonl_only` | 可选字段；旧 index 缺失表示未采集，完整性和身份仍按原证据判断 |
| `cloud-observation.json.sandbox_metrics_file_io` | `sandbox_metrics_jsonl_only` | 只在资源文件已创建且 finish 后输出；baseline/L0 不创建资源 writer，不伪补零 |

模型侧车及 legacy 事件文件错误由 ObservationError 上报，HTTP/SSE 与 Strict 匹配不变。
legacy 的 nil-error short write 在 Close 时锁存为 io.ErrShortWrite，非法 n 也显式报错；
计量 status 为 incomplete，成功模型会话仍成功，必需 Tape 错误仍失败。
SDK index 出现 file_io 时严格检查字段、类型、
scope 与重复键；接受新格式不放宽未知字段，也不从计数推导 ToolCall 或时间。
三个 writer 保持既有 Write/Sync 次序；重复 finish/Close 不重复计量。
这些计数仍不包含 manifest/index/late 文件、native artifacts、Bridge 审计及其它进程
文件 I/O，也不包含计量报告自身；不能汇成全链总量。metering-1/2 的 ECS 定向
unit/race/vet 已通过，同一 metering-2 的559文件源码也已通过
`observability-r15-release-1` 的 build/test/test-race/lint/integration，证据 SHA
`b0260a73807e0771217010fffd6cfe983a25f8cf6999eaca21394496eb69f2db`，
source manifest SHA `f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`。
该次只有工程门禁，没有真实云/模型调用或生产部署；后续同源真实baseline/L0四对另行验收，见下方。

门禁后另有 TERM/HUP 候选函数验证，证据
`observability-r15-driver-signals-1/evidence.json` SHA
`42c0aaf0f177597497a055608ac9e45c4779aece831a9f0a5168a03b2cd5836f`。
它只证明自有sleep进程在中断后退出并保存FAIL证据，不证明候选完整main、Docker清理、
重复信号或不可取消文件I/O安全；不能用于关闭R05或R15。执行驱动与候选SHA及首次SCP
失败原件见[验证归档](VALIDATION_AND_REPRODUCE.md#r15-metering-release)。

历史R15在同一 E2B B placement/backend、同一 Strict Replay Tape、同一 workload/spec、并发1和单 attempt 下交错执行 baseline×2、L0×2、L1×1、L2×1。对应归档源码组合的六次回放每个样本记录八阶段原值、Runner `/usr/bin/time` CPU/RSS、产物字节数、各**有该字段的** writer attempted/written/dropped/write_failed、Verifier/Stop/Cleanup 状态和 run-result SHA；E2B operation/native-tool 索引没有 `dropped` 字段，不得推断零丢失。该历史证据见 `/opt/aries-artifacts/observability-r15-current-1/evidence.json`（SHA `bd321f265c829ca34d2b7bf458d20570003cb54f4086c853fa84a693929dec1a`），旧 release3 矩阵与失败记录另存；这些历史实测不覆盖后续R05/R07/R01改动；后续E2B A新配对单列，不能回填旧B结果。六次均有功能/验证/隔离/正常清理结果；原驱动弱门及后补 seal 的限制见验证归档。这六个历史样本的 GC/分配、独立 collector CPU、普通文件 Write/Sync 调用次数未采集，不能从 events 数量或 Runner CPU 猜出这些值。样本量与冷热条件不足以给出扰动阈值、p99 或供应商排名。

<a id="r15-b-native-paired"></a>

### 当前568文件B schema 2配对的已测范围

`observability-r15-b-native-2` 固定568文件运行时、同一E2B B派生模板、原Tape及v1.4分析器，仅切换baseline/L0。按baseline→L0、L0→baseline、L0→baseline、baseline→L0预登记顺序完成四对8次；计划/接纳/执行/完成/独立验证成功均8，失败及未运行均0。每次Strict完整会话、同候选SHA、12项独立Verifier、Stop/revoke及清理通过；16个Sandbox逐项确认不存在，0上游模型调用。未修改产品源码、Tape或工具行为。矩阵SHA `6b7aeaab7bbfb838e4eccb08fe95ed3d905b96bec5f5eae57921497d1bc89128`；配对分析SHA `d83825d06574ef6973697c6e50aff9017e3ac7f2bf09f4d7f6257921f2fd4d8a`。复现与逐阶段值见[验证归档](VALIDATION_AND_REPRODUCE.md#r15-b-native-paired)和[性能归档](../performance-archive.md#r15-b-native-paired)。

baseline关闭native producer、模型/Runner/SDK可选JSONL，模型轮/帧、native计数/时间及四类writer计数均为null；Strict完成由原会话终结校验证明，不以Tape预期值补观测。L0四次分别实见4轮/573帧/30个schema 2 callback，并保留原生消费、首输出、直接子进程与EOF的独立clock/scope。八阶段、Attempt duration、dispatch delay与verified E2E均取原记录；verified E2E含调度至独立Verifier，不能当作纯Agent运行时间。

控制端SELF CPU、共享堆分配/GC与生命期峰值RSS、外层GNU time及四类JSONL实际Write/Sync计数均已采集。baseline关闭writer不填零；L0只对有明确drop字段的来源报告该值，其它来源仍unknown。guest logger、Adapter等独立进程CPU/I/O未拆分，L1资源writer未开启。配对L0−baseline的Runtime中位数为−128.121778ms，范围−174.333993至+23.510723ms；Attempt中位数+118.2710945ms，范围−7251.934591至+7378.659286ms。四对仅作描述性差值，无预先批准阈值，不声明扰动合格、负开销、p99、容量或云厂商排名。

479项独立原件/算术对账PASS，证据SHA `539556c94c41f85be0d78efb2f15ce777d3fd73d83969d8b580c1d6741258496`。四条L0分别将controller域1120/753/860/877区间及各31flow、native域各7区间/0flow导入TP；各自同一Snapshot的12来源与双视图并集精确，导入起止/时长误差0ns、processor错误为空，原件未改。TP证据SHA `7e091369438bef922bb2c07f0e21c127200337c416c08462be86fda5c1d0917a`；baseline因观测关闭而SKIPPED，本轮UI为UNRUN，不能把两域叠成统一guest时间线。

首轮`r15-b-native-1`原件保留：1次baseline通过且2个Sandbox回收，随后7次在Runner/分配前失败、各0create，未凑成配对；矩阵SHA `25fe996ef6a607bfe7c7c545a6d1cbb4a4464d56b983784e17cbd9dc77754769`。原OSError未记录errno；结合分配前标记与同Linux bind反例，推断冗余裸socket预检把TIME_WAIT当占用，不能声称直接取得原七次errno；只移除实验driver的该预检后另行预登记完整矩阵，业务门槛不变，旧单样本不混入新四对。这关闭本次固定B baseline/L0范围，不关闭P0、R15其它级别或全链成本。

<a id="r15-paired-scope"></a>

### 历史559文件版本配对的已测范围

固定559文件源码、同一 E2B A placement、同一 Tape、并发1，按 baseline→L0、L0→baseline、L0→baseline、baseline→L0 四组顺序完成8次真实 Strict Replay。每次6条exec、12项独立Verifier通过，Stop/revoke confirmed；计划/接纳/执行/完成/验证成功均8，失败与未运行均0。实际创建16个Sandbox，逐项provider NotFound，cleanup_errors为空；未调用上游模型。

仅比较双方均开启原生身份协议时的可选L0增量；不含身份机制绝对成本。baseline模型轮/帧原始观测为null，不能把Tape预期7轮/735帧填成实测；L0四次均实际观察7轮/735帧。共享Go进程CPU/GC/分配不能拆成Runner、ModelProxy、collector独立开销。四类L0 JSONL计数为实际Write/Sync调用，baseline关闭时值为null，第五类L1资源writer未启用。区域、provider底层缓存与冷热状态unknown；四对样本无预先批准阈值，不宣布扰动合格、负开销、p99或供应商排名。CPU为SELF累计起止差；RSS表比较各进程生命期峰值，不能用同进程Maxrss终值减起值伪造区间峰值。外层GNU time含wait记账与最终报告落盘，和SELF口径分开。每次机器级/proc/stat、diskstats、PSI起止原件及SHA只用于背景干扰分析，不能归到本工作负载。

baseline的Snapshot因可选来源未启用而incomplete，模型帧/缺失drop为null；L0的native A计时仍not_collected，Snapshot/Trace同样incomplete、退出1。L0只有Runner与model-boundary显式drop字段可报告0，其余没有该字段的来源保持unknown。实际FileIO错误/短写计数0只覆盖这四类文件，不推成全链零丢失。

证据 `/opt/aries-artifacts/observability-r15-paired-2/evidence.json`，SHA `3fb5a096b2ee3253757ef93e05a1691d28a32fed37d4272faea1c78ea90df37a`；同目录 `paired-analysis.json`，SHA `031ae0159d92c4eb0511bf2ff1b5541f4f561b665876ecfed0b5bddb59999c0d`。独立394项原件/算术对账见 `/opt/aries-artifacts/observability-r15-paired-review-1/evidence.json`，SHA `d182fba9e138b69529ffdff91d12138fa5c822f6045844d9e351d7acfee47286`，不是新性能运行。

四条L0 Trace已实际导入原生TraceProcessor，每条55个区间/43条flow/19个来源，operation起点、时长、终点及flow端点逐项一致，导入误差0ns、processor errors为0。四条baseline因可选观测关闭而SKIPPED，不把空导出算时间线通过。原始文件SHA未变，native A计时缺测与incomplete保留；新UI交互UNRUN。证据 `/opt/aries-artifacts/observability-r15-paired-perfetto-1/evidence.json`，SHA `1b4b992f912783f05b9927b4134676f80a1e27cf5b3b30c7e70a44b05e4c826b`，仅为 `PASS_FOUR_L0_OFFLINE_PERFETTO_ONLY`，无新增云/模型调用。

### 合并562文件版本：失败矩阵与测量分母

`runtime-release-2`新增A原生消费与SDK客户端时钟，因此重新预登记四对，不能复用旧版本扰动值。实际5次执行：4次整体成功、1次Strict会话失败，剩余3次NOT_RUN；独立Verifier5次均通过。两对完整样本只能作描述性比较，第三对失败不补样本，第四对未启动；不声称四对已完成或成本合格。baseline可选模型轮/帧和关闭writer仍为null，L0原件含实际成功/拒绝请求；第五次主流程7轮后出现额外请求，被Strict拒绝，失败保留。

三份L0的完整性均complete、coverage unknown，包括失败任务；这两条质量轴不代替业务成功。实际TP核验所有operation起止/outcome和Attempt分母，失败保持failed、validated保持true。CPU/GC/分配是共享Go进程值，四类FileIO只覆盖各自JSONL，未计入原生事件文件的独立I/O；未知或未启用仍不填零。10个Sandbox最终NotFound，0上游模型调用。

新A/B及这三份L0同版本TP通过；新A实际UI也核对了native_tool与sandbox_client两个独立示例，不能把两者当成同一个Tool的层级。版本、失败分析、原始分母、236项独立复核与UI证据见[合并验收](VALIDATION_AND_REPRODUCE.md#p0-runtime-candidates)，五次实值与两对差值见[性能报告](../performance-archive.md#p0-runtime-r15)。

R05 的 legacy `model/events.jsonl` 错误分离已实现并通过 ECS 验收：Write/Sync/Close、短写与非法返回由 `ObservationError()` 锁存，成功模型会话不再因此失败；Strict 偏离、会话未完成及必需 Tape 写入错误仍由 `Close()` 返回。实际 CLI 单独报告 `observer_status=failed`；[源码、故障注入与三案例 CLI 证据](VALIDATION_AND_REPRODUCE.md#r05-observer-separation)分别归档。上述两例云故障收容也不代表当前组合的完整成功云回归。

R07真实SDK旧新对照现已执行：20次执行、10组配对的双流/退出/异常语义一致；后代持管道时，直接子进程退出观察与EOF相隔1001.543509ms。timeout/cancel的四次取证均发现wrapper已消失、同PID/启动身份的child仍存活，明确保留全树终态BLOCKED；最后整个Sandbox已独立确认NotFound。[实际SDK证据](VALIDATION_AND_REPRODUCE.md#r07-real-sdk)本身不覆盖完整Bridge Stop/revoke或Verifier门。后续[取消/Stop故障验收](VALIDATION_AND_REPRODUCE.md#r07-current-isolation)与[完整A取消验收](VALIDATION_AND_REPRODUCE.md#r01-native-cancel)已分别验证：隔离确认失败不得评分，取消后隔离确认成功可以独立评分，并核对资源回收。未验范围仍为B后台/PTY的全树终态；不靠killpg修改原生行为来消除SDK反例。

<a id="p1a-offline-jaeger"></a>

### P1a离线OTLP契约与已验范围

`LoadSnapshot`使用精确JSON数字并严格恢复已知`AnalysisSDKCall`属性类型；`ValidateSnapshot`继续验证原内容身份。候选1读取真实A时因struct字段顺序变map排序导致FAIL；候选2恢复原类型后原ID通过，未更改历史文件、parser版本或重签。`ProjectOTLP`只投影该Snapshot，输出标准OTLP/HTTP JSON `request`及独立`manifest`。Trace/Span ID由版本、Snapshot ID及operation ID稳定生成；int64纳秒使用十进制字符串，保留operation/Attempt/ToolCall、scope、outcome、原source SHA及起止事件身份。

仅`local_monotonic_anchor`按本域UTC origin加原单调offset生成epoch；无锚区间列入`excluded_intervals`，不存在跨域相减或合成根Span。精确同域parent边才设parentSpanId，其余可信边使用带kind/correlation/source引用的Link；失败/取消保留ERROR，未知/observed不推断成功。instant与duration-only操作保留Snapshot和manifest计数，本版不伪造Span或按时间近邻附着SSE事件。完整性与coverage独立，complete/unknown不升级为全量覆盖。

发送器一次HTTP请求、30s超时，无自动重试；成功回执仍为`accepted_unverified`，须查后端。拒收数/警告/非空partial_success与超时测试通过；超时为delivery_unknown。固定Jaeger2.22、skew=0s实查A/B/取消样本845/795/784个Span及86/31/18个parent，v3原始时间误差0ns；B/取消的7/3个原生区间无anchor而排除。重发实际产生重复存储行，稳定ID不保证exactly-once，不能重计业务成功。真实样本Links均0；显式synthetic协议夹具另验同trace及跨trace Link，不替代真实业务关联。详细版本、legacy微秒显示误差与证据见[复现](VALIDATION_AND_REPRODUCE.md#p1a-offline-jaeger)。P1b账本/阶段指标另见[实际验收](#p1b-offline-prometheus)；P1c资源出口[工程已验、后端限定对账通过](#p1c-resource-prometheus)。P2、实时/SSE事件投影、大批量、认证及生产耐久仍未验。

资源边界：Runner/Proxy、Sandbox/VM cgroup、Agent、Tool、宿主分别定义；秒级/5秒是采集轮询频率，非云指标更新保证。首次CPU占位0、reset/重启、同值不同样本与缺测必须显式分组。采样峰值不是瞬时真实峰值。

## P2能力探测与边界

| 能力 | E2B Cloud | Huawei Sandbox | ECS控制器 | 只读探测/退出条件 |
| --- | --- | --- | --- | --- |
| procfs/进程启动身份 | UNRUN | UNRUN | UNRUN | `/proc/self/status`、`/proc/<pid>/stat`、boot_id、namespace；读取范围和权限归档 |
| cgroup CPU/memory/io | UNRUN | UNRUN | UNRUN | cgroup版本、挂载、cpu.stat/memory.current/io.stat；解释实际归属，不探未授权宿主 |
| perf events/CPU stacks | UNRUN | UNRUN | UNRUN | `perf_event_paranoid`、内核版本、实际capabilities、perf工具是否存在；先不加载采样 |
| eBPF/sched/io热点 | UNRUN | UNRUN | UNRUN | 只读核对capabilities、BPF/sysfs可见性；不自动提权、不加载程序 |
| 应用级profiler | UNRUN | UNRUN | UNRUN | 对被控Runner/Proxy或支持的Agent运行时确认版本和可配置入口，单独有界PoC |

任何不可用字段为unsupported或BLOCKED；不能默认云产品提供宿主信息。真实采样前固定短时间窗、CPU/IO上限和敏感栈处理；通过Attempt+execution/ToolCall+进程启动身份+clock建立关联，不凭可重用PID。应用栈、调度、IO证据不存在时不宣称定位到函数。


### 大文本 Dashboard 已有证据展示补齐（2026-10-26）

[展示与测量说明](../dashboard/large-scale-text-editing/SOURCES.md#完整展示修订测试动作采样与原生-trace)：同三份冻结Snapshot补出9个owner资源采样流、20次精确ToolCall动作与6个独立时钟域Trace视图。资源缺失原因由“页面未提取”与原接口unsupported区分；采样极值、累计CPU与生命周期RSS不混用。主Trace原SHA不变，native另投影，TP核验20,361项通过；Coverage仍unknown、后端验收未升级。R08/R09/R18仅新增展示证据；R01–R18原处理状态、R05暂停及P0未完成均保持。ECS构建、浏览器与原始视图核验路径和SHA见链接，不新增真实任务分母。
