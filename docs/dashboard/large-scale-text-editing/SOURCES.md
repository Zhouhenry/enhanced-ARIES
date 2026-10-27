# 大文本多云 Dashboard 数据来源

本目录只含从既有原件提取的展示数据，不新增运行、不复制工具正文、私有prompt、私有断言或凭据。该组为独立 `terminal-result-v2` 变体，不能改写为原生任务官方成绩。

`data.json` SHA-256：`8b1691c6f618e90842a85c47de1250a0b495a07078bdaec25494b353b88bac21`。原始耗时保留整数纳秒；请求汇总mean为明确派生的浮点均值。CTRF本身使用浮点epoch，pytest窗口保留秒及精度说明。

## 三次运行与主要原件

| 展示运行 | ECS原件目录 | driver evidence SHA | summary SHA | Trace audit SHA |
| --- | --- | --- | --- | --- |
| E2B · 回放1 | `/opt/aries-artifacts/tb-repro-text-v2-e2b-replay-1` | `ea5229721d285a48d6d32e0aedc5384ba5e48d42b5a44458b8f44f116cde3dbd` | `57eb695b864c4803c90f02276de906676e857039bd0ba24d40a03a303ff22628` | `5bab2cf7552ef15add648672dc9c52deff3d8c5ce0c875445b61d4743f2956fc` |
| E2B · 回放2 | `/opt/aries-artifacts/tb-repro-text-v2-e2b-replay-2` | `3beff58ef8b3273783834d034e08fe79b8c4781c5e877fa7ee899d014c94a0b8` | `d48cbe56fc0fd75ae1e8180754dd744902152582bd60e4a9ad6c45ec657ec5f9` | `729bc86482589eb5dc328ceac0671c0900d108db069fa2a058a730eb02efb239` |
| 华为云 · 回放1 | `/opt/aries-artifacts/tb-repro-text-v2-huawei-replay-1` | `934a78c818f6f63166105ec7be47966f19f246a93d7bd43824b3277fb832185b` | `ef49355b97f3657b7ebf3144c7d7d2d9345b22da2ab118b78f9683de82575817` | `dfbbc383a37d6eec8c121ce6e6a7b2ec3716135aedcb321a93e2e91c8df04e58` |

`summary`为各目录 `performance-review-1/summary.json`；`Trace audit`为 `offline-analysis-1/evidence.json`。原件保持不变，数据提取自本地小型汇总中的白名单内容；小型汇总与ECS原summary不是同一文件，其各自SHA分别保留。

## 字段追溯与展示规则

| Dashboard字段 | 原始来源 / 规则 |
| --- | --- |
| `runs[].stage_duration_ns`、`attempt_duration_ns` | 小摘要 `summary.tasks.large-scale-text-editing[0].attempts[0]` 的 `stage_duration_ns`、`duration`；原值复制。排队在 Attempt 外；本页其余七阶段合计，加未分段差额等于 Attempt，不能用八阶段之和替代。 |
| `tool_calls[20]` | 同一ToolCall ID、exec序号、模型轮及退出码在三次运行逐项相同；每格保留该次SDK/guest纳秒、execution_id、回执行号。第4次均exit1，其他19次exit0。 |
| `background_jobs`、`background_waits` | 原native协议的各自单调时钟、ToolCall ID、终态；不累加、不跨域相减。validator已PASS，但本地摘要未导出逐wait→job边，字段为null，禁止按序或时间猜边。 |
| `model` | 23轮matched，0上游/0cached retry/0late rejection，recorded时序。release10原session没有policy字段，Strict来自该批实际matcher契约，未伪装成原字段。 |
| `verification`、`cleanup` | 原CTRF 5/5、reward1；停止撤权confirmed，每run create1，provider/ledger不存在，cleanup errors为空。评分禁网未验证，不升级Goodput。 |
| `snapshot` | 三次各自Snapshot/Audit：complete / unknown；4个原生区间及6条边缺UTC锚而排除；不借导入成功宣称全链覆盖或后端查询通过。 |
| `version` | 保留每次run原source manifest、分析器binary/parser、共同Tape；完整native digest来自冻结release10输入，来源标注为pins，没有新运行时检查。 |
| `resources` | 同1CPU/2048MiB；E2B历史准备时文件系统容量与华为模板磁盘回执分开，不当作同一实测口径。资源采样已从同一Snapshot补提取到`resources.observation`，按Sandbox、Harness、Controller分别展示；未知仍为null，不用配置推算。 |
| `verifier_diagnosis` | 华为总评分519.266395855秒，原脚本exec517.824024672秒，CTRF约25.0359447秒；APT原stdout第7行9395kB/8min6s。约486秒是取整日志，不伪造Span。 |

仅有E2B两次与华为一次；可展示这三次真实结果，不发布稳态云排名、p95/p99或容量。未测量/未导出保留 `null` 并配说明；unsupported、unknown和UNRUN不能替换成0。有完整边界但无非空输出，以及 EOF 先于退出观察的情况，也保留 `null`，同时显示各自原始状态，不能统称漏采。控制端 `exec.wait` 是 Process 提交前到 Wait 返回的窗口；控制端−guest不是网络延迟，job+wait不是总执行时间，阶段时长不是连续Gantt。

## 小型本地来源

| 来源 | SHA-256 |
| --- | --- |
| [取回汇总：e2b-replay-1](../../../.cache/tb-repro-text-v2-replay1-audit-1/summary.json) | `a3af5838319ce4fafa729a2dd0b6ad029195ea550fd0faf52e03b9c63c031d38` |
| [取回汇总：e2b-replay-2](../../../.cache/tb-repro-text-v2-replay2-audit-1/summary.json) | `5dec56ea9bc0e7df801927b81ff8734aa0300e269e9017b7ab585379da2eda2d` |
| [取回汇总：huawei-replay-1](../../../.cache/tb-repro-text-v2-huawei-replay1-audit-1/summary.json) | `7e48b682295ad9e0d13790e3ef106c6c5ecc4ad72084e6e7b6efbbd84900908f` |
| [华为APT诊断](../../../.cache/tb-repro-text-v2-huawei-verifier-diagnosis-1/conclusion.json) | `f33da732e760205cb3c9ea5c424cebde865755ffda3dbf2b1f6d90b3c163ac77` |
| [release10固定输入](../../../.cache/tb-six-release-10/receipts/tb-repro-agent-inputs-4/pins.json) | `e4f590ff108f429b0f915c055777291f4e71ac59a0e465779a7496be0b91f5ea` |

原APT stdout SHA：`94db2f489a961b4a094b359147548d18331dc61f3f6ef8a39cce45b7b950c82a`；原CTRF SHA：`28f1d77400795a32bcdfa11e7118b178f95ad7034ea11cf8fa786e9998da6a58`。精确ECS路径及行号保留在 `data.json.sources` / `verifier_diagnosis`，不复制日志正文。

## 复核入口

- [三次同Tape八阶段与20次工具对照](../../repro-variants-performance-20261026.md#text-v2-huawei-replay-1)
- [E2B第一次回放](../../repro-variants-performance-20261026.md#text-v2-e2b-replay-1)
- [E2B第二次回放](../../repro-variants-performance-20261026.md#text-v2-e2b-replay-2)
- [阶段与单位说明](../../performance-stages.md)


模型可直接展示三次各7699个服务端 `frame_sent`，原边界中的 `frame_flushed` 数逐一对账相同；不是token数或Harness收帧证明。已从校验SHA的原Snapshot提取每次23个 `chat_completions` 请求的HTTP、admission、body_read、parse、match区间；按同一ModelRequestInstanceID+ClockDomain计算收到HTTP到首次完整SSE/content/tool delta flush，以及首末frame flush窗口。没有models路由请求，不混入其耗时。它们包含Strict录制节奏等待，不能冒充真实上游模型TTFT。

原数据整理与后续页面验证分开留证；最终单文件页面的ECS构建和浏览器检查见下方资源补齐说明与[使用说明](README.md)。

## 已采集细分计时的有界提取

本次仅一次只读SSH：先校验三份原 `analysis-snapshot.json` 与三份 `e2b-operations.jsonl` 的既有SHA；再按精确身份提取白名单。未新建Sandbox、调用模型、跑测试或重新解析/导出Trace。

提取收据：[timing-extraction.json](../../../.cache/dashboard-large-text-data-1/timing-extraction.json)，SHA `dd28d0c0a0537bb90198efd55b0f191c9daea19c23a857704824154f251a1a4d`。源Snapshot/SDK路径与SHA仍在 `data.json.sources`；每请求保留phase的operation/event ID及边界行号，每工具保留原execution_id、ToolCall ID与行号。

| 运行 | HTTP请求和 s | match和 ms | body_read和 ms | parse和 ms | 首SSE / content / tool delta事件数 | guest边界 / 首输出 | 可计算退出后pipe等待 / EOF先于退出观察 |
| --- | ---: | ---: | ---: | ---: | --- | --- | --- |
| E2B · 回放1 | 156.215627 | 40.626757 | 32.188947 | 17.519041 | 23 / 18 / 22 | 20 / 19 | 2 / 18 |
| E2B · 回放2 | 156.216689 | 42.786477 | 31.491535 | 17.998119 | 23 / 18 / 22 | 20 / 19 | 5 / 15 |
| 华为云 · 回放1 | 156.217490 | 39.797063 | 33.487401 | 17.225335 | 23 / 18 / 22 | 20 / 19 | 10 / 10 |

每个run的23个请求有同域实测HTTP、admission、body_read、parse、match各23对；HTTP与子phase重叠，不能相加。首content仅18次、首tool delta仅22次；未出现的该类delta保留null，不用首SSE替代。`request_timing.route_summaries` 同时列observed/missing数量及sum/min/max/mean，均只描述本批样本。

20次Agent exec各有Popen返回、子进程退出观察、双pipe EOF边界；各有19次非空首输出，另1次未观察到非空输出，保持null。所有值以各自guest wrapper开始为起点，不与代理或SDK时钟相减。只有EOF不早于退出观察才给 `pipe_drain_duration_ns`；相反顺序保留两个原值及null，禁止造负值或填0。该观察间隔不等于内核精确退出时间、全部I/O或CPU时间。

华为Verifier的相对边界继续限定为评分进程，不借用到Agent工具。Snapshot coverage仍为unknown，4个原生区间与6条边仍排除；本次补提取不改变原判定或制造跨域时间线。

## 资源补齐与展示纠正（2026-10-26）

首版页面只提取计时/Trace汇总，误把资源空字段概括为“未采集到本页”。三次实际都是L1运行；原Snapshot已经包含Controller、ECS Harness和Sandbox三类资源流。本次核验3个冻结Snapshot及18份资源原件的SHA，按原owner提取，不重建关联、不改Snapshot、不重跑任务。

| 指标 | E2B回放1 | E2B回放2 | 华为回放1 |
| --- | ---: | ---: | ---: |
| Sandbox CPU返回读数最大值（%） | 100 | 100 | 100 |
| Sandbox采样内存最大值（MiB） | 686.910 | 690.277 | 1815 |
| Sandbox CPU去重读数 / 返回读数 | 55 / 1562 | 53 / 1431 | 147 / 147 |
| ECS Harness观察窗口CPU（秒） | 42.493064 | 40.822625 | 42.505278 |
| ECS Harness采样内存最大值（MiB） | 994.992 | 817.660 | 794.379 |
| Controller共享进程CPU（秒） | 5.113555 | 5.040973 | 5.398092 |

这些是各owner完整采样窗口，不是单Tool或仅Runtime；同一任务Sandbox在评分等待期间仍可能被采样。内存max是采样最大值，不等于连续观测真实峰值。E2B以已确认bytes换算MiB；华为仅使用明确标注单位的`mem_used_mib`，其无后缀字段不猜单位。CPU百分比不积分为累计CPU时间；Harness累计CPU首末差只覆盖观察窗口。Controller RSS为进程生命周期高水位，不解释为独立任务内存。

华为CPU的原始时间戳`1793000669`有两次回执：第146行83.23%、第147行0%；时间戳单位仍unverified。提取首版拒绝冲突，FAIL原件保留于`/opt/aries-artifacts/text-dashboard-resource-2/failure.json`（SHA `43093014e8cefc532fa0e84f72296e55549e079e8963f0622f4b74248875a490`）。新版保留双方原值与证据，仅同时间戳同值去重；展示所有返回读数极值并标注冲突，不last-write-wins、不生成有效时间序列。正式OTLP/Prometheus导出的冲突拒绝规则未改。I/O/network在本批原回执中均明确unsupported；磁盘占用、观测文件Write计数不能替代任务I/O。

提取脚本为`extract_resources.py`，只允许ECS执行，输出到新路径。原件路径、SHA、stream/sample身份及极值行号均在页面JSON里。ECS收据目录`/opt/aries-artifacts/text-dashboard-resource-3/`：

- `resource-extraction.json`：SHA `165a7910dfeaa96ade15783e697585073f20155b5378411476b214ee9f3b8317`，9个owner流，原始冲突明确披露。
- `resource-integration-validation.json`：SHA `4f72686531542de1b7c0fd7c580b4fd62928615713b6ce0f152265cb19ce0b67`，3个Snapshot/18份原件核验，逐项极值与原Snapshot对账；0新云任务。
- `browser-validation.json`：桌面/手机交互检查，包括11行资源、6个unsupported单元格、内存单位、冲突提示、原阶段/工具筛选及离线零网络请求。截图目视及浏览器回收单独留证。

本次只补齐既有资源的Dashboard投影，不增加成功样本，不升级P0或声称这些新样本已完成后端查询验收。


## 完整展示修订：测试动作、采样与原生 Trace

ECS `/opt/aries-artifacts/text-dashboard-full-4` 构建并验证上一版，HTML SHA `a27707d9b8e25f2ade628026ece71cb0421564b1f7ecd4102ae48c6bc53da28c`。没有新增Agent/Sandbox运行，原三次结论不变；后续计时层次修订另列于下。

- `case-actions.json` 从公开任务说明与同SHA原生transcript提取。20条动作依精确ToolCall ID和模型轮接到原表，后台终态按sessionId连接。摘要描述命令行为，不声称知道模型内部意图；没有复制shell正文、宏配方、私有prompt或评分断言。
- 第18条exec外层0、内部127（缺少计时命令）保留；第19条才实际完成全量Vim，第20条比对MATCH。任务独立评分5/5不抹掉中间失败。
- `resources.observation.streams[].metrics[].points` 保留全部去重实际值、原单位、时间状态和sample/source身份。同时间戳同值去重并保留sample_ids；异值均保留。CPU与内存分对象显示散点，不插值；可信UTC仅作为本流相对横轴，华为未核验时间使用读数序号。Controller两端点不是连续采样。
- `trace_views` 内嵌六份gzip原JSON：三份历史主Trace原字节不变，三份native视图由同Snapshot指定clock导出。主域分别3306/3293/3404区间与106 flows；native各4区间、2flows。无法跨域对齐的边保持排除。
- Trace Processor在ECS核验20,361项，起点/终点/时长纳秒误差0、Flow身份正确、parser无错误；仍为scoped PASS、coverage unknown。证据 `/opt/aries-artifacts/text-dashboard-traces-1/trace-views-manifest.json` SHA `e4991a37fdeb226abfe71cbcd957006dd22e48663b3f61b488a5f1d67a4bc2f5`；压缩视图包SHA `100f4f025cd8297aaf42ae716c617bab86c7e592299eed8cb65794209ce0573c`。

离线预览使用每条真实Chrome `ts/dur`微秒值，不把耗时累计拼成时间轴；每页80条可筛选与缩放，所有事件与Flow留在原JSON。原字节SHA通过才允许显示、下载和传给Perfetto。按[Perfetto官方嵌入协议](https://perfetto.dev/docs/visualization/embedding-the-ui)，localhost/HTTPS下点击后加载原生UI；file模式下载后手动导入。默认页面零外部请求。浏览器传送握手与实际UI导入验收分开记录，不互相替代。

桌面1440×1000/手机390×844已验证基础交互、20动作/内部失败、9阶段、原值下载、六Trace切换及native4区间、分页/搜索/缩放、Trace下载原SHA、无JS异常及初始零网络请求。`browser-validation.json`保留执行回执；原生UI与资源散点专项验收位于同目录 `native-validation/`，截图目视结果另记。全部构建/浏览器执行在ECS，本机只取回静态产物。

原生UI专项：通过页面按钮向官方Perfetto v59.1-1e9c61e98传入E2B回放1主Trace，实际绘制Runner、task.e2e、agent.run、verifier.exec和模型请求轨道，截图已目视核验；不是仅PING/PONG成功。30组资源对象/指标交互检查全部通过，包括华为147个CPU/146个内存点、冲突值和2点Controller。后续额外SQL核验尝试在等待轨道时超时，保留原失败，不宣称浏览器SQL错误计数为0；解析精度仍引用六视图的独立TP证据。临时浏览器和loopback HTTP服务已关闭。

## 计时层次与统计关系修订

本次只重排展示、增加 `time-help.html` 的中文口径及依据原字段计算的两种算术量：七阶段未分段差额、模型末 flush 到 handler 结束的累计。`data.json` 仍为上文 SHA；三次原运行、六份原 Trace、计量单位与状态没有变化。

页面阅读顺序为 Attempt / 八阶段 → Runtime → 模型 / exec / 后台等待 → 资源与 Trace。关键包含关系默认可见，时间数字与“计时口径”按钮提供起点、终点、包含项、父级、统计分母、重叠及相加规则。Attempt 不含排队，Runtime 不等于 Trace `agent.run`；控制端 `exec.wait` 从 Process 提交前到 Wait 返回，不等于官方 SDK 内某一个方法的耗时。

| 算术核对 / 单位 ns | E2B 回放1 | E2B 回放2 | 华为回放1 |
| --- | ---: | ---: | ---: |
| 七个 Attempt 内阶段合计 | 278911862896 | 269080194881 | 726600273094 |
| Attempt 减上述合计 | 1356705992 | 1364119625 | 936705733 |
| 23 次 HTTP 请求累计 | 156215627409 | 156216688783 | 156217490137 |
| 接收→首 SSE flush 累计 | 7651886972 | 7652571615 | 7645133515 |
| 首→末 SSE flush 累计 | 148559947635 | 148560245011 | 148568685982 |
| 末 flush→handler 结束累计 | 3792802 | 3872157 | 3670640 |

前两行相加为 Attempt；末三行相加为 HTTP 累计。七阶段差额并非一个连续实测 Span，也不是纯网络或 Agent Loop。HTTP 等式由同请求、同域且有序的 69 组真实边界核对，不适用于均值混加。首 content 仅 18/23 次、首 tool delta 仅 22/23 次有该类事件，平均分母明确列出。

独立源审计：[static-audit.json](../../../.cache/text-dashboard-time-contract-1/static-audit.json)，SHA `b445c5da6106a465f8dbea2fcc79b4b5050fbc821f5a264251b097c950da9393`。该审计核对既有原件及算术关系；当前源码用于解释边界，不能将当前工作区版本冒称为历史 release10 执行版本。模型和控制端工具在三次样本中均有真实重叠，因此页面不把内部累计堆叠成 Runtime。

ECS 构建与浏览器回执目录：`/opt/aries-artifacts/text-dashboard-time-hierarchy-1`。最终 HTML SHA `007493e5fee56529b67c59c72f8de2d5621552c7442d81514d64464f7211af53`；`browser-validation.json` SHA `09e9013bf1eafecb37f581e1b87e152a60859717994a11a695b44a6338344f32`。Chromium 155 在 1440×1000 与 390×844 下核验 34 类口径弹窗、阶段与 HTTP 等式、工具筛选和已知空态、30 组资源指标（27 组有采样点、3 组无已核验单位的明确空态）、六份 Trace 的 SHA/下载及交互。页面异常和外部网络请求均为 0；截图另经目视检查，浏览器已关闭，没有启动 HTTP 服务或容器。

初次 UI 检查发现无输出状态应按实际 `no_nonempty_output_observed` 判断，已修正。两次后续检查因脚本的同义措辞预期与空态组合计数过窄而中止，原日志保留，修正验收脚本后完整通过。本轮没有新增云任务、模型请求或 Trace 导入；原生 Perfetto 的真实 UI 导入证据沿用上一版，未把离线回归冒称为新官网验收。
