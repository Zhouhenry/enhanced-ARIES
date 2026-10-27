# 验证与复现归档

<a id="e2b-provider-reading-references"></a>

## E2B 累计资源历史重复：release32 工程与真实回归 PASS

根因来自真实采集路径：`adapters/e2b/server.py:metrics_snapshot` 调用固定 E2B SDK 2.58.0 的 `get_metrics()` 时未传 `start`；该 SDK 默认从 Sandbox 创建时返回历史。`observedSandboxes.Start` 的查询仍为 5 秒 ticker。以下是两份历史任务 Sandbox 的真实计数，另各有 2 次评分 Sandbox 查询；不能把返回读数或展开记录数当成查询数或独立样本数。

| 原始回放 | 任务查询次数 | 返回有值读数 | 相同时间戳及完整内容的重复项 | 不同读数 |
| --- | ---: | ---: | ---: | ---: |
| E2B A 去等待 | 39 | 703 | 666 | 37 |
| E2B A 保留录制节奏 | 221 | 24,090 | 23,871 | 219 |

ECS 上使用两份未修改原件进行真实 writer / parser 夹具验收：包含评分流的原 41 / 223 条 RPC 逐行保留；666 / 23,871 个引用展开后，响应内容、数组次序、请求字段、空数组和 null 均与原件一致。新原始文件分别由 151,449 / 4,633,771 字节降至 54,329 / 1,183,742 字节；这是本地归档大小，不是上游网络节省量。任务新分析样本分别为 37 / 219 个不同读数加 2 条真实空响应；legacy 分析的 705 / 24,092 条原样本字段和顺序不变。实际生成的 Prometheus payload 分别含 259 / 1,533 个指标点，所有时间戳与指标值保持一致，指标点数不等于采样时刻数。

首次候选夹具发现 `[]` 被写成 `null`，已修复并重测上述两份原件；原失败保留在 ECS `release32-attempt-1/`，evidence SHA `43e16ad01a65e09590ad978bbc38326861e549ba39e8dda7fcf0cce11fb45d3f`。独立审阅另补齐 schema 2 的 null 读数占位必须携带有效引用，否则标记证据不完整；不得默认为成功的空指标。candidate2 的工程通过记录独立保留于 `release32-attempt-2/`，未用它替代最终源码验收。

最终 914 文件 release32 已在 ECS 完成两 package 定向 / race、两份真实历史 writer / parser 夹具以及五项发布检查。build / test / test-race / lint / integration 分别 7.079 / 37.955 / 115.757 / 3.221 / 344.347 秒，全部 PASS；测试后容器盘点复原、源码和原始输入不变。manifest SHA `228cd89a2f86c5cf70943ce9aa7e797745c0a769d51fd6363f341f9d575325e6`；evidence SHA `7d77871a6712bcbb4eb2b44990a0ac17b906b8e05e27afb30310605c873650f3`；freeze SHA `926fc64cc7e83cfb8aa2e954898e6d017ee4d024c295a47cd101566d0b86ee55`。CLI `aries` SHA `db6f588df9d002f934265bd570f038d1460df3436c35404ad14092c8be2642a4`，分析器 SHA `7720bc99fb87f3da3fec9cd1f7a12b936d39bac2a06b242eb40566c5dd85c3d7`；本机 6 个 Go 文件与最终清单逐 SHA 相等。最终夹具输出的 zero / recorded metrics SHA 为 `d1cb29f343d95357e3710750f93f9aaa129a05efd01eef2d36c39a871f734da8` / `795557775ff9e3df0545e4ff7d162b7a4511393120ea5b750df5960485869348`。

新 E2B A 同 Tape 回放于 2026-10-27 19:05:45 UTC 启动并完成：64 轮 / 78 Tool / 32 命令，PPT score=1、四离线投影、停止隔离和清理通过；两个 provider NotFound、ledger absent、unit PID=0、锁实际取得释放。任务资源 44 次查询 / 904 条返回 / 862 条重复 / 42 个不同读数；评分资源 3 / 4 / 2 / 2，均无 unkeyed、缺引用或时间戳冲突。47 条 schema 2 回执共 66,932 字节，raw SHA `67ef861651c7a78253a3e34631e3fb3edaf4737309ae50972d6612e9829ec545`；新分析不再重复展开。完整耗时、CPU / 内存、PPT 与回执见[性能归档](../jiuwen-pptx-workload-performance-20261027.md#e2b-a32-resource-regression)。summary SHA `f6669ea1e3162bbd374d9f49a8f989a633017ef06148bc82ea541e14fe29ac27`，资源对账 SHA `e7119c8a65e4019b71d76ca855edc27735ab4d34c398895ebc0bfbfd2193a550`。全局 coverage 仍 unknown，本次后端查询 UNRUN；历史 payload 无损对账不冒充这次后端实测。

新 3 个 Dashboard 源文件单独验证，不纳入 Go 冻结包的覆盖声明。ECS 浏览器于 19:11:33.985–19:11:36.406 UTC 实测六组旧数据回退和绑定新真实摘要的内存字段夹具：五类计数正确，CPU / 内存与 Agent 采样不变，无 console/page errors，临时浏览器与服务均关闭。回执 `.cache/release32-dashboard-ui-1/evidence.json` SHA `58a207fe0a873e70adad7b97272acfab233b68258ee07a7c1f97fda339507747`，三截图保留并目视核验。该验证没有重新生成已交付 v3 / 分享 ZIP，也没有改变其六组原始数据。[测量契约](OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#e2b-provider-reading-references)。

<a id="jiuwen-workload-b5-backends"></a>

## E2B B5：同冻结 Snapshot 的实际后端验收 PASS

仅处理已完成真实运行的原 Snapshot SHA `7823c4220f01694d8d94d38533c4a8eaf510216627b06c8fb35138916806c5be`，使用 release31 `aries-perf`，没有新建 Sandbox 或调用模型。

- Perfetto：Controller 2,905 区间 / 260 flows，Native 332 spans / 80 Tool，39 个命令、121 区间、275 markers；时间对账误差 0 ns。
- Jaeger：3,358 spans、40,307 项查询检查通过；v3 起止与时长误差 0 ns。
- Prometheus：Attempt、Controller、任务 E2B、guest cgroup、guest 块设备 5 个流的首次及重复导入实际查询通过，数值误差 0；时间按毫秒截断，误差不超过 0.997 ms。任务 provider 的 1,379 条源读数包含重复；实际导出的 357 个 gauge 点分属 7 条指标序列，不能当作 357 个独立采样时刻。任务另有 2 条时间未验证，评分 Sandbox 无可验证时间的读数仍 NOT_COLLECTED。

首轮临时 Jaeger 接收端拒绝 29,487,006 字节 OTLP 请求（HTTP 400，body too large），失败原件保留。只在新的后验目录将临时 receiver 请求体上限设为 64 MiB，再进行一次明确的新验收；不是发送器自动重试，未改产品、Snapshot 或 Tape。后端临时容器、端口、进程清理确认，锁实际取得并释放，原 Snapshot SHA 不变。

47 个小回执已复制校验到 `.cache/jiuwen-pptx-e2b-b5-workload-backends-2/`。evidence SHA `801ec68668f30833263029721576dd32b2e41c8c02ddf431f1dec8af29dd382d`；metrics SHA `3f4fba3d83cc8d8ea8fb4f64e00ebc2e03525cd31fc66661e630d3dd7615c0e5`；postflight SHA `b90819ad9888959c5a24883a23e644445045ec3f99b61f66d7911196646d9b20`；copy SHA `2578c0a9e73e64b951464d0eec12a51cdfcfd7f3249cdd5979274dd5d97e5954`。原 B5 摘要中的 UNRUN 保持原时点记录，本节为新增独立证据；全局 coverage unknown，不能外推实时、持久化或容量验收。

<a id="jiuwen-pptx-dashboard-v3"></a>

## Jiuwen PPT 六组 Dashboard v3：交付验收 PASS

[看板](../dashboard/jiuwen-pptx-huawei-a/index.html)与[离线包](../dashboard/jiuwen-pptx-huawei-a/jiuwen-pptx-huawei-a-dashboard-v3.zip)已复制本机。使用两云各一次真实模型录制、同华为 A Tape 的两云去等待回放及两云保留响应节奏回放，共六组；首屏展示八阶段及 Sandbox Worker / 命令分层，时间范围图可按指标高亮，详情始终切换到该组原始数据和 PPT。

供转发的[最小分享包](../dashboard/jiuwen-pptx-dashboard-share.zip)另列，7,118,800 字节，SHA `14484c6fb202163ac8e8216fb820d315c98b32400b2ab3e96f711b409d0847ce`。36 个成员，保留六份 final PPT 与五份完整 Trace 原 gzip；字体子集 216,872 字节，保留许可证，去除开发附件及重复资产。ECS 对新包验证离线入口、六组切换、时间图、下载、字体加载及手机视图，通过后 SCP 逐 SHA 核对；完整 v3 不修改。看板无需开发环境，只有打开 Perfetto 官方网页时需要联网。

ECS 浏览器在 2026-10-27 17:54:40–17:54:55 UTC 完成桌面/手机交互、六组切换、矩阵守恒、grep 实际工具选择、缺失计时及 Perfetto 导入检查，浏览器与临时服务已关闭。此前用文本匹配 grep 的检查误选 bash 命令，修正测试选择方式后通过；原失败记录保留。浏览器后 data 中仅修正第 01 组真实模型的后验 backend_note / receipt / receipt_sha256 / status 四字段，同步调整后验链接和 Trace 专区措辞；逐项静态复核，未宣称再次浏览器验证。

独立只读核对六份原摘要、六份实际 Worker 账本与 PPT，6,798 项零差异。ZIP 的 57 个成员逐 SHA 与交付文件一致；展开 Trace/PPT 内部成员后共 707 项安全扫描，已知凭据及编码变体、通用敏感模式均无命中。最终 ZIP 22,864,526 字节，SHA `ac59398d790f83acff955f16410fc704de232aaeb4c8e5dabd3dbeb47e4ee089`；data SHA `826f39a89e7ed2d0b98f037e6ffb7a10c1819f764b30007c7b435c31c9b7ba6d`。最终审阅回执 `.cache/jiuwen-huawei-a-dashboard-review-1/v3-final-review.json` SHA `3bce1eb2e310641b6f1a21ecfcd93311105f371412a6b113fa244f6048f6286f`。

第 01 / 03 组分别使用各自已完成的实际后端证据，其余组保持 UNRUN；完整 Trace 展示区仅属于第 03 组华为去等待回放。每组 n=1、全链覆盖 unknown、华为出网隔离仍为假设；不把看板验收等同于生产观测全能力验收。复现入口为目录内 `build.py`、`groups.py`、`verify.cjs`，只在 ECS 读取原归档并进行构建与浏览器验证，不触发新的模型或 Sandbox 运行。

<a id="jiuwen-workload-a29-recorded"></a>

## 华为 A29 recorded：同 Tape 时序对照限定 PASS

在 ECS 完成 `jiuwen-huawei-external-pptx-workload-replay-2`，仅将原 A29 zero 的时序改为 recorded，保持同一 Tape、release29、image8、4u8g 和 Workload 策略。64 请求 / 72,102 帧，帧目标与契约无偏离，请求错误及缓存重试均 0；实际发帧调度误差保留。78 个原生 Tool、32 条命令，独立 PPT score=1、停止撤权、两个 Sandbox 不存在及运行资源清理均通过。

本机原件目录 `runs/jiuwen-pptx-20261027/huawei-a-workload-recorded-replay-1/`；summary SHA `312109003ab64d3a75cada1780b5b7ed820144be679df743bb39f6fb0f1e7844`，zero/recorded 对照 SHA `7d50e5ce21ce85bf2ce4500a7940e161f987b532802e84b5947e76a56f0299c3`，postflight SHA `17a9e305c4a756a756cf13f2e39645ddd3660c8e0685484e6191921c93236cbf`。四离线投影 PASS，实际后端查询 UNRUN；不移用以下 zero 的后端证据。期间共享 ECS 有 Dashboard 工作重叠，条件文件保留；这不是随机化重复实验。阶段/模型窗口、资源及 PPT 见[分层性能归档](../jiuwen-pptx-workload-performance-20261027.md)。

对应 E2B A recorded 也已完成，保持同 A Tape、release29、image8 和 4u8g；64 请求 / 72,102 帧、78 ToolCall、32 条命令，独立评分、停止及资源清理均通过，四离线投影 PASS，实际后端查询 UNRUN。本机 `runs/jiuwen-pptx-20261027/e2b-a-workload-recorded-replay-1/`，summary SHA `2dc0d114ac53b3d3e0f418c5296dd3e3c8538ac60d6314bdffdd11bf42c9f5fe`；跨云对照 SHA `6e301e7be54ad69803d5f8693d0db8f11681a33aa1faf985b0b81f62d4f54b91`。命令 ID 一一对应、退出码差异 0，global coverage unknown；不把单次差值推导为云厂商总体排名。

<a id="e2b-native-trace-download"></a>

## E2B 原生 Trace 下载故障：连接配置与采集预算修复

**最新 release31 工程与 B5 真实回归已通过已测范围，旧失败保留。** E2B 连接明确配置公共 `http_version="1.1"`，Embedded 观测采集去除独立 5 秒上限，沿用总 CleanupTimeout 并保留私有目录清理时间。没有增加重试、放宽 Strict 或改动 Tool 输出。旧 5 秒代码在新 6 秒下载夹具中按预期失败，候选在有足够总预算时完整采集；总预算不足仍明确观测失败，停止及清理行为保留。

ECS 上 build / test / test-race / lint / integration 分别 17.299 / 90.156 / 352.742 / 9.387 / 361.404 秒，五项通过。914 文件冻结，source manifest SHA `7a19fb5c672cec6c4d9c8323105a4bc940632a3a36a7783f9e6285286ee06edb`，工程 evidence SHA `04c9724caaefcd8dda943c35064f2124bf398d66f34314771fdccce157a39a11`，freeze SHA `05d81c82e9ce2c829f9eb8c549b57d05317ba2cb5602a95c7b6f4ee76c025e51`；基线、夹具和容器/网络库存核对、清理与锁释放通过。

同原华为 B Tape 的 E2B B5 完成 61/61、独立 PPT score=1；18,489,897 字节 native Trace 完整，SHA `9d5a1b0ce1cb8caf1242e6dfae6ec5761c615a5b03453831f05329125111abb9`。Producer started / ended / exported 为 332 / 332 / 332，missing=0，80 个权威 Tool 精确关联；39 条命令、275 事件、drop=0，按 39 个唯一 ToolCall ID 与华为 B2 对账，退出码差异 0。四项离线投影通过，两个 Sandbox provider NotFound、台账无残留、停止撤权及清理确认。摘要 SHA `048aa9cd1bab6053eea36d89cdf9b6a0fee5fdc1cdb1f1fd746706648744e0bc`。此次后端查询仍 UNRUN，全链 coverage unknown；release31 与华为 B30 的版本差异单列，不回写 B2/B3/B4 历史观测失败。底层传输库的具体协议缺陷仍未证明，已验证的是公开连接配置的实际修复效果。

以下为定位过程与各次原始证据。

E2B B2/B3 原始失败保留。专项 `e2b-native-download-diagnostic-3` 在一个新建、180 秒 TTL 的真实 E2B Sandbox 内，使用相同 SDK 2.58.0、64 MiB 文件限额、96 MiB 响应限额与 30 秒请求超时复现：同一运行目录的合成 JSONL 18,489,919 字节下载成功；改用华为 B2 原始 Trace 内容 18,489,873 字节后，在读取 11,984,896 字节、1.819 秒时发生 `httpx.ReadError`，底层为 `pyqwest.ReadError`。原件及 guest 文件前后 SHA/大小一致，响应均为 `application/octet-stream`、无 Content-Encoding。这次证据排除该次失败由固定文件限额、文件增长、路径扩展名或请求超时触发；尚未证明具体传输根因，也没有恢复此前已删除 Sandbox 的 E2B 原 Trace。

回执：`.cache/e2b-native-download-diagnostic-3/receipt.json` SHA `9e42086ae5d8fa17a6728ce0f78046af9839aecb3bce5c46e54b40ad632c6f84`；详细小证据 SHA `ba0857d117f5c94a99b286701f27c497ee6f1a4a9491f2a558346753e31527aa`。全过程 15.058 秒、无模型调用，Sandbox 删除后 provider NotFound，临时容器、凭据及台账均清理。真实 Trace 内容只用于 ECS 与私有 Sandbox 间的诊断，没有写入公开报告。

下载错误元数据候选只记录异常类型及状态码，保留原 HTTP 错误和单次 SDK 调用；不记录正文、URL、凭据或异常消息，不增加重试。ECS 上 E2B 47 项、华为 17 项适配器测试通过，测试回执 `.cache/e2b-download-error-observation-1/receipt-2.json` SHA `21f7945d37671c303d2727dc648351c59a09c88469360eb07944413bfcc994f2`。它是 release30 冻结后的独立候选，未部署到 B3；日志可诊断不等于下载根因已修复。首轮测试因错误导入镜像内旧模块而失败的证据保留，复验核对实际 import 源码 SHA。退出条件：确定真实读取失败原因、验证最小修复，再用同 B Tape 完成真实回放及完整原生 Trace 对账。

后续 `e2b-native-download-diagnostic-4` 在同一个短生命周期 Sandbox 中，对同一原 Trace 交替进行四次独立读取：SDK 默认配置两次分别在 10,153,984 / 12,562,432 字节中断，显式公共 `http_version="1.1"` 配置两次完整返回 18,489,873 字节且 SHA 相同，耗时 2.087 / 2.043 秒；没有自动重试。回执 SHA `f108b29dd2ffd25719af45bd7940e61752242cae0203e8834b4bb0e788f86967`。只读源码确认 Pyqwest 未填 HTTPX response 的版本扩展，HTTPX 属性的 `HTTP/1.1` 是缺字段时的默认值，**不能据此声称实测了协商协议，也未证明上游 HTTP/2 实现的具体缺陷**。解释回执 SHA `d97cad3e168406b29a588d95a0da05861e6e136013db66e73e8648e97c1a0482`，源码证据 SHA `da6b3fb6fff2ea45b9e672c4f1c372ae52596828250aac1dff9aa0713c5abd67`；所有资源已清理。

最小产品候选对 E2B 创建的 Sandbox 连接明确采用该公开配置，覆盖该连接的文件/命令 API；华为连接及 E2B 类级清理调用保持原实现。`sandbox_connection_http_version` 仅报告配置。新候选 ECS E2B 48 项、华为 17 项测试 PASS，最终回执 SHA `245ba426c755463d663462574240edd2285372131b250431b6fb8b523f79e6b9`；首轮 provider 字符串错误导致的测试失败另存。B4 使用 release30 加单文件候选 overlay 已完成 61/61、score=1 和清理。18,489,986 字节原生 Trace 完整保留，源/保留 SHA 相等；随后 Producer 下载在共享采集窗口第 5 秒被 context deadline 截断，因此整体观测和四投影仍 FAIL。原冻结、旧失败与原 Tape 保留。摘要 SHA `838ce3a8fb62e89dca5b2505d23fda5be41d39c67b3299fe0caa986c6699f77a`，本机 `runs/jiuwen-pptx-20261027/e2b-b-workload-replay-4/evidence/download-timing-evidence.json` 保留原始时序；下一项修复仅去除任意 5 秒上限，沿用现有总 CleanupTimeout 及清理预留。

<a id="jiuwen-workload-a29-backends"></a>

## 华为 A29 Workload：本次 Snapshot 的实际后端验收 PASS

对已有 `jiuwen-huawei-external-pptx-workload-replay-1` 原 Snapshot SHA `d7f7655a68ff68c440a13fc26720143c3a57744fa7a8873d6a4c68b79efd2fe3` 运行离线验证，没有重跑 Sandbox 或模型，没有改写原业务评分和冻结摘要。ECS 原件目录 `/opt/aries-artifacts/jiuwen-pptx-huawei-a-workload-backends-1/`，30 份小回执已 SCP 并逐 SHA 校验到本机同名 `.cache/` 目录。

- Perfetto：Controller 777 区间、272 flow；native 450 区间、449 父子关系、78 个 Tool 精确核对，各域真实时长转换误差 0。64 个模型请求、128 条提示差异及 254 条工具正文差异的 SQL 叶子值共 2,802 项对账通过；不据此拼接未对齐的时钟域。
- Jaeger：1,547 Span、18,575 项查询检查通过，v3 epoch 起止及 duration 转换误差 0。与 Perfetto 使用同一个已校验 Snapshot。
- Prometheus：任务、Controller、Agent 三个流分别首次及重复导入后查询，201 / 16 / 459 个读回样本均与各流预期一致；数据值误差 0，时间按毫秒转换的误差绝对值小于 1 ms。两个华为 provider 流因采样时间语义未验证保留未导出状态，未伪造资源点。这个结果不证明线上乱序、持久化或容量。
- 独立清理检查确认 unit inactive、MainPID=0、自有容器与 14319 / 16688 / 19095 端口不存在，共享锁可重新获取。原 Snapshot SHA 未变。

`evidence.json` SHA `5ba72bf48657dfb09f483e336c2e060e82c08a7d1ad515a20c0ef154e7b7126a`；`metrics-evidence.json` SHA `fc53062ada3129317e06fe638b98df9bfc6c23aebf92a38b06b4e8ccd2a4457d`；独立 `postflight.json` SHA `945701aa1d91c8694529d7470e365ac1fe65c48f3bffd1114e921c5d607caa09`。这次 PASS 只更新华为 A29 的后端验证状态，其余 A/B 样本保持各自状态，global coverage 仍 unknown。

<a id="jiuwen-workload-release29"></a>

## release29：Jiuwen PPT 显式 Workload 接入

工程验证在 ECS 完成。显式 `model.replay_policy=workload` 使用固定 Jiuwen 原生 formatter 的有限提示上下文例外；Strict、原 Tape、工具参数、真实工具、独立 Verifier 和停止隔离门保持原行为。[具体契约](../implementation/model-replay.md#jiuwen-原生-ppt-的非-strict-接入)与[差异 Schema](OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md)分别定义功能和测量边界。

相关 modelreplay/performance/config/app 四包、私有真实首请求 fixture、构建通过。fixture 使用原 Huawei A record3 和 E2B A record3 的实际首请求：Workload 匹配且记录两个提示差异，Strict 仍拒绝，原 Tape 前后 SHA 不变；它没有执行真实工具。首次 `make test` 因 wiring 的旧“拒绝 Workload”负例失败；只删除这条过期负例，保留 `attempt-1` 原源码、失败 evidence 和日志。续跑 wiring 定向、全量 test、race、lint、integration 均通过；生产源码未变，原已通过构建的二进制按 SHA 复用，测试源码修正与来源关系单列。最终 ECS 容器/网络、fixture、源码核对通过，临时缓存清理，锁释放。

冻结来源：`/opt/aries-artifacts/release29/`；source manifest SHA `932384701b543d4531476c7a7dbcb20131b45446983639eecdb4b3e9bcbe38f6`，工程 evidence SHA `cd623c814537f73b304045cc93838babd584dc898e5c4d744dd7364a43a339a1`，freeze manifest SHA `71fe8c5a0b493d0d22eef0c7bf7b7c09454cfdf73f5fe13b9c2c255f50bbbeac`。本节及本轮新增结果文档在冻结后整理，不冒称属于编译输入。

真实回放计划按华为 A → 华为 B → E2B A → E2B B 串行执行，`timing=zero`、image8、4 vCPU / 8192 MiB；每轮重新执行真实工具、PPT 评分及清理。A 原 Tape SHA `6a15976b408e008a238c34e60e417006a989963b42e4f4db1ea042bd13df9c17`，B 原 Tape SHA `f31d9799e43b28fafbdb4167234bc65244674aac53121aea2ebcd98a745338f7`，均来自华为 record3，不为不同云替换 Tape。实际完成为 **3 PASS / 1 FAIL**：华为 A / E2B A 均 64 轮、78 ToolCall / 32 命令；华为 B 61 轮、80 / 39。三组返回码对账一致、独立 score=1、停止撤权和删除确认，按同 placement / 同 Tape 作本轮工作量对照。E2B B 在首轮系统提示匹配时失败，0 Tool / 0 命令、score=0，停止及清理成功。四组均已生成同源离线投影；华为 A29 原 Snapshot 的实际后端查询已另行 PASS（见上节），其余本次后端 UNRUN。[阶段、资源和原件入口](../jiuwen-pptx-workload-performance-20261027.md)。不改历史 Strict 成功数或全局 coverage。

<a id="jiuwen-workload-release30"></a>

### release30：跨云原生 Linux 环境元数据修复（工程/双云 B 功能 PASS，E2B B2 / B3 观测 FAIL）

失败原件：本机 `runs/jiuwen-pptx-20261027/e2b-b-workload-replay-1/evidence/acceptance-summary.json` SHA `491b45e0bbfb73a3eba9ddb12ae79955ab9cf1cfb16206756928a406bc4803a7`；同目录 `first-request-system-difference.json` SHA `364ecb1bd6e30773157779f780422d97812e179329ba9e3415cd4d5476eada0d`。诊断只在私有内存中将原 Tape system 的已批准运行根路径及唯一 OS 行替换成实际日志值，重建 39,996 字节的完整 SHA 与被拒请求一致；第三条 user 模式附件相同。故确认未允许的差异只有 `/messages/0/content` 中的原生 Linux 内核版本。实际是 0 Tool / 0 命令而非漏采；6 个 SDK Span 完整，独立 score=0，两个 Sandbox 确认删除、cleanup_errors=[]。旧失败原件不改。

修复限定固定中/英文 Environment 块的 Linux 内核版本行，平台/Shell/其他文字继续匹配，沿用 `jiuwen_system_context` 摘要。Strict、默认模式、原 Tape、真实 OS 和原生提示不动，无新配置或框架。当前 **工程验证与冻结 PASS / 两 B2 功能 PASS / E2B B2 观测 FAIL**。ECS build/test/test-race/lint/integration 五项实际耗时分别为 15.646 / 84.201 / 344.019 / 8.832 / 348.598 秒，全部 PASS；914 文件、二进制与源码冻结，容器库存恢复、临时缓存清理、锁释放，本机 Go 文件与冻结 SHA 一致。私有对照 PASS 覆盖原 A 实捕首请求及 E2B B 实际 system 摘要重建；失败的完整 HTTP envelope 没有捕获，不能称其为完整实际请求或端到端验收。

冻结目录 `/opt/aries-artifacts/release30/`：source manifest SHA `00009a103be1ce709c23888a8dd17ca260c72e8f73c49495121628ad9a337c4f`，工程 evidence SHA `0a2b6eeb47d70112cb549d806416f7fae58337b5151fc9cb97d77ca2e97eaaac`，freeze manifest SHA `cff81c3715ef4de5a60708a4c5f1e0ca18349a202ceac0602ff142bf36e26ddb`；本机 `.cache/release30/completion-receipt.json` 保留传回与源码核对。冻结文档中的 PENDING 是封包时状态；本段为冻结后的实际工程回执更新，不回写源码包。后续真实两 B2 均 61/61 模型终态、独立 PPT score=1、停止和清理 PASS。华为 B2 摘要 SHA `fde6345291776bfe3e0bca2cf796f27463b112712de81724198b1170971b82b0`：80 精确 Tool、332 SDK、39 命令 / 275 事件 / drop=0、四项投影通过；E2B B2 摘要 SHA `2076b26ac2c3a7120d4261f9c16e1a0063a42d7e44303cac5e2560f4bb36e03a`：原生 18,489,919 字节文件下载失败，332 Span 已 start/end 但 retained exported=0，80 个 native Tool 关联未知、四投影 exit=1。39 条真实 process 仍有终态，跨云同 ToolCall ID 返回码一致；此小回执不升级 E2B 的观测失败。两组本机目录为 `runs/jiuwen-pptx-20261027/{huawei,e2b}-b-workload-replay-2/`，四份原 PPT 和失败中间文件均保存，后端查询本轮 UNRUN。

本批八次尝试为七次功能 PASS / 一次首轮 FAIL；A29 和 B30 按各自版本对照，不混用华为 B1。E2B B2 的下载底层原因仍在诊断，约 1.95 秒失败后小文件可下载，不能定为 5 秒收集窗口超时。同 release30 / 同 B Tape 的 E2B B3 已完成 61/61、独立 score=1、停止及清理 PASS，但 18,489,969 字节原生文件下载再次失败、80 native Tool 关联未知、四投影失败。39 条 process 与华为 B2 按唯一 ToolCall ID 返回码全部一致，仅证明保留的进程范围；不升级观测。摘要位于 `runs/jiuwen-pptx-20261027/e2b-b-workload-replay-3/evidence/acceptance-summary.json`，SHA `af934e9f3e605c49c01b29b7a4bcceba243f99b714996a34089bcff7c4f0d493`；同目录对账 SHA `61c27e9c9f5101b9d80ca1fe2130a59a68d53fef23380f3c408811957d9ca14d`，复制回执 SHA `f01ebcfa1bf3304e5f431dd5aa211a8fe9e1849022e941220b1b0fb8ff4bfc17`。两次 E2B B 观测失败均保留；底层原因尚未确认，日志元数据增强候选不构成根因修复证据。

**Jiuwen process（ECS release23）限定 PASS：** 861文件冻结组合通过 build/test/test-race/lint/integration；最终新旧镜像 native/worker/bootstrap 回归通过。E2B A/B、华为 A/B 四次真实DeepSeek任务各原12/12，15进程/105事件完整且exact关联，8个Sandbox已独立确认删除。四组各自冻结快照的 Perfetto/Jaeger/Prometheus 实际查询全部通过，后端资源清理确认；整体 integrity=complete / coverage=unknown。华为评分出网仍 assumed，四份独立模型轨迹不作性能排名。原Go夹具超时、B夹具失败及历史记录保留。[逐阶段、命令、资源与原件SHA](../jiuwen-release23-cloud-validation-20261027.md)、[测量定义与原生测试](../jiuwen-process-observation-20261027.md)。

<a id="jiuwen-release23"></a>

## release23 双云 A/B 命令进程与四组实际后端

冻结回执 `/opt/aries-artifacts/release23/freeze-manifest.json` SHA 为 `3cfe77ec01640dbde5c80eb3e039ad7bd2500098c69c3d1e193bc103bbd759fd`。四个实际运行目录依次为 `jiuwen-e2b-external-record-3`、`jiuwen-huawei-external-record-3`、`jiuwen-e2b-record-9`、`jiuwen-huawei-record-4`；各自 `acceptance-summary.json` 绑定该版与原 Snapshot，不能跨组替换证据。

对应后端目录为 ECS `/opt/aries-artifacts/jiuwen-release23-backends-1` 至 `-4`；本机汇总 `.cache/jiuwen-release23-backend-evidence/jiuwen-release23-backends-summary.json` SHA `02a2ccb76506be07f2b1fad2a165b96b8b46f1f9de0f7e6423149880c09f5f1b`。实际 TraceProcessor 逐组核对进程 12/12/13/9 个区间与 28/28/28/21 个 markers；Jaeger 共 2,035 个 Span、24,464 项对账，v3精确epoch误差0ns；18个有值 Metrics 流首发与相同payload重发后均实际查询通过，5流NOT_COLLECTED。进程直方图31个样本不含15个重叠容器。

原Snapshot SHA前后不变；四组后端临时容器、listener、cgroup均无残留，共享锁可获取并释放。查询回执、legacy精度、单位/时间缺测及逐组来源完整保存在[本轮报告](../jiuwen-release23-cloud-validation-20261027.md)。这是离线固定快照限定验收，不能外推为全链覆盖、实时上报、生产exactly-once、耐久或UI通过；历史UNRUN不回填。

<a id="jiuwen-release21"></a>

## release21 双云 A/B 与后端验收

[最终四组结果与原件 SHA](../jiuwen-release21-cloud-validation-20261027.md)：843 文件冻结、五门禁 PASS，E2B/Huawei A/B 原评分各 12/12；两云 A 各 11 次调用准确关联原生 ToolCall，九阶段与资源归属逐项保留。8 个 Sandbox 独立删除确认。E2B A 同快照实际 TP 导入、272 个 Jaeger span 查询及 5 个 Prometheus 导出流首次/重复发送全部对账，数值一致，原件不变且临时后端回收。后端回执 SHA `8854a4f8a13a6c2b59e41d9302b3f6e84ab75e29651c6302402e794028c8444b`。其余三组后端 UNRUN，四组全局覆盖 unknown；华为评分出网保留假设。

## release19 后的原生工具与观测组合验收

[完整范围、失败记录与 SHA](../jiuwen-native-runtime-validation-20261027.md)：image5 的 9 项 core、14 项 Provider、9 项远端语义检查通过；ECS 完整 Agent attempt6 的实际身份与两 Trace 通过；三包 unit/race/vet 和真实归档的原生身份边离线对账通过。两云模板探测实例均删除。attempt7/8 已验证 Go 接收模型错误的正负全链；attempt9 按真实 bridge/ 目录完成 Tool/Bridge/worker 与投影回归，三包 unit/race/vet 通过，清理无错误。release20 五门禁通过但未投放云测试，最终目录修正进入 release21；这些夹具自身不代替发布和真实云验收；最终 release21 五门禁、四组云与后端结论见本页上节，不回填旧样本。

<a id="jiuwen-release19"></a>

## release19 与 Jiuwen A 两云验证（2026-10-27）

827 份冻结源码的 build/test/test-race/lint/integration 均通过；后续 E2B、华为各一次真实 A 模式录制均原评分 12/12、完整 Tape 5 轮，停止撤权及每轮两个 Sandbox 删除确认。实际配置、逐阶段时间、Bridge/SDK 证据、原件 SHA 和明确缺口集中在 [A 模式验收归档](../jiuwen-external-cloud-validation-20261027.md)。两次独立模型轨迹不作同 Tape 性能对比；本轮未重新验收 Jaeger/Prometheus 后端，原生 ToolCall 到 Bridge 的身份仍 unavailable。下列旧版本验收维持原范围。

<a id="jiuwen-release18"></a>

## release18正式工程冻结（2026-10-27）

`/opt/aries-artifacts/release18`基于未改的release17，仅叠加14个Jiuwen Harness/Performance生产与测试文件（7新增、7修改），冻结812份源码；本次人类文档更新另行归档。ECS build/test/test-race/lint/integration分别11.860017 / 31.341643 / 79.686411 / 3.325290 / 329.213115秒，全部PASS，正式状态`PASS_RELEASE_18_FROZEN`。源清单、基底与fixture复核通过，Docker库存恢复、unit inactive/MainPID0、共享锁已实际释放。门禁自身0云/付费模型调用，不冒充真实Agent验收。

| 原件 | SHA-256 |
| --- | --- |
| evidence | `6c49efbd53443206bd84d2bca11d95f2b155a4594453b3d0a296b5863d14d990` |
| source manifest | `eaf3f83b11e37684200f9422cb4e62b3a0a8da42f93c5dc99cd291921fcccd40` |
| freeze manifest | `f413dfa01a56f96442b470160d475e1651d48fc0f6fadd79eb009bd744b68b24` |
| aries | `f5b2cfc2dd1b0756a02a6578fd8cd4ccd13cd08d756d0f2d4c122a27520a249e` |
| aries-perf | `071a419de470452c8e188b35aa4f2e4b43c029efbdc9dd1bfbb17847efa4f8fe` |

原Go/Python定向回执保持其原范围。发布包含sampled SDK Span producer审计、Stop后实际导出集合对账和Snapshot范围限定；该分母不覆盖未采样/未插桩行为、guest进程退出/pipe-drain计时或跨机时钟对齐，也不把flush返回当零丢失或有界关闭证明。旧release17双云原生12/12和Strict失败均不改写。

E2B record7与华为record2均通过原12/12、score1，完整Tape分别9/7轮；每轮Stop/revoke、两个Sandbox独立删除及cleanup=[]均确认。两份真实模型轨迹不同，仅作双云功能验收，不是同Tape性能对照；华为评分出网仍为显式假设。

sampled SDK Span的started/ended/exported分别44/44/44与35/35/35，missing/unexpected均0；`jiuwen_sampled_sdk_span_exports`专项integrity/coverage complete，global coverage仍unknown。未采样/未插桩行为、guest进程计时、跨机时钟与有界flush不在该证明内。 两云均显式L1，44/35个原生Span中分别8/6个authoritative Tool Span；四出口投影PASS，仅证明已有数据可投影，未将旧record5后端结论移用到本轮。Strict replay3与record1–5旧失败保留；校验模式讨论延后，固定提示变体暂停，历史六例Strict仍3/6。

| release18真实运行 | evidence SHA-256 | sampled SDK专项回执 SHA-256 |
| --- | --- | --- |
| E2B record7 | `e3b3dd82828264f9d43d89c57447b635986ef3a1ff026054ffd6112c3b4ffe58` | `74c8f74e737fb2ddb1380efe2e72c6dfe3592b38089074de9c5f22cc70679b3e` |
| 华为 record2 | `60030d3189861034fee1693dfcc0c978b926604da013f2d05121a72c5fb27bee` | `71c8c5a6db0e6d1ae128b2500533bcf3ee239d826217c8f2073c97a61895e7a4` |

本地小回执分别位于`.cache/jiuwen-e2b-record-7`与`.cache/jiuwen-huawei-record-2`，逐SHA核验；[新运行八阶段及来源](../jiuwen-cloud-validation-20261027.md#release18-native-audit-cloud)单列。旧定向Go/Python检查仍只表示其原测试范围，不改写为云验收。

<a id="jiuwen-release17"></a>

## 历史release17工程冻结与双云原生功能验收（2026-10-27）

`/opt/aries-artifacts/release17`基于未改的release16，仅叠加12个文件：2个`call-goal-v2`适配文件与10个四边界cgroup资源观察文件，冻结805文件。ECS build/test/test-race/lint/integration分别 **11.056427 / 30.757987 / 80.681490 / 3.387995 / 325.117679秒**，全部PASS；状态`PASS_RELEASE_17_FIVE_GO_GATES_ONLY`与`PASS_RELEASE_17_FROZEN`。基底、源码、fixture未变，Docker库存恢复、unit inactive/MainPID0，锁实际释放；门禁自身0云/模型调用。随后E2B record6与华为record1均完成原生解题及原12/12独立评分；不回写release16 record5 FAIL或历史Strict3/6。

| 冻结原件 | SHA-256 |
| --- | --- |
| `evidence.json` | `9f10f07ba410da1eae655cb89ae2e9e8b9e0b066758a1e82ea94fab9dedc3eef` |
| `source-manifest.json` | `9ee984fe07d7c68e4de68ddfefe96c153a10f3b63bcdab6adb84c288754e40a7` |
| `freeze-manifest.json` | `89c82872ce7a769580b79d9702abd2441d6a6178fe905f758b6aae5d4f7c329f` |
| `source/bin/aries` | `91d63453bea548545688b437388243e7da2ec5773ed9a6dbfd617993cd7d6b66` |
| `source/bin/aries-perf` | `b60484cbf246540beef6809b9eaf032dcddd5586e7959f852089b5869863fdd5` |

| release17真实运行 | 功能与清理 | 原 evidence SHA-256 |
| --- | --- | --- |
| E2B record6 | 原12/12、score1，7轮完整Tape；35官方Span/6 authoritative Tool | `fffd48e7cf7932c7a7112d0c8fa0f4b9770617774950f3e6a5bea9aede8afbbe` |
| 华为 record1 | 原12/12、score1，9轮完整Tape；43官方Span/8 authoritative Tool；评分出网为显式假设 | `9e8d6c8ce41bd7abd1a5305245941ba8f3513a994201c55272fcaae0ee562dc5` |
| E2B Strict replay3 | record6同Tape首请求turn0 `REPLAY_DIVERGED`，0模型帧/0 Agent工具，原1/12、score0 | `aac26355793a8a368090e11da63973ce11c71951c1795f8bf7229dbd93bf04c9` |

三次均确认Stop/revoke、各两个Sandbox独立删除、cleanup=[]及控制端收尾。E2B录制Tape SHA `71355f718833b48455d35784baa5ff353cf344fc79b88a74d7ff497c9d0f2319`；华为Tape SHA `133e1376ab4cfa6d4940c33b7bb9e3e82fa063a631552242b4addf660a55789c`，是不同真实模型轨迹，不能作同Tape工作量或云性能比较。华为`acceptance-summary.json` SHA `552a2b60b44bdcf597e0bf270a4b9f4ca8567c6f8309548baefc63479767086c`核实原XML共12项、失败/跳过均0；本地证据根分别为`.cache/jiuwen-e2b-record-6`、`.cache/jiuwen-huawei-record-1`与`.cache/jiuwen-e2b-replay-3`。[八阶段及历史失败](../jiuwen-cloud-validation-20261027.md)另列。校验模式讨论延后，固定提示变体暂停，没有扩展Workload Replay或放宽Strict。

四边界cgroup数据与provider整台Sandbox采样分开，块I/O保持设备归属；进程组计数不是逐Tool资源，生命周期内存高水位也不是Runtime峰值。在这批release17样本中，官方Span producer覆盖与flush仍未证明；后续release18专项结论不回填旧样本。实时后端/持久化仍未闭合，原生解题成功不等于全链生产验收。

<a id="jiuwen-producer-audit-candidate"></a>

### producer审计的前置定向ECS验证（历史证据）

审计通过原生fresh进程extension记录sampled SDK Span计数、ID和终态，不复制prompt、工具参数或结果正文；Stop后对照实际导出集合。以下为发布前的定向证据，保持各自原范围；后续已合入上方release18并完成两云专项验收，未改写旧release17运行。

| 定向检查 | 实际范围 / 证据SHA-256 |
| --- | --- |
| Go collector与Snapshot | `/opt/aries-artifacts/jiuwen-producer-go-1/evidence.json`，`PASS_PRODUCER_GO_TARGETED_ONLY`；两包unit/race/vet全PASS，分别4.778/11.491/0.516秒；SHA `5fa209079d269121e8456f14c4d1e8663fc4fe20fc83aa3ce3a1f1e5e162aabb` |
| Python官方SDK/原生入口 | `/opt/aries-artifacts/jiuwen-native-audit-1/attempt-3/evidence.json`，12项定向检查PASS；SHA `3bfb7126ea13aad286021b0dc17a0fb2e532a37ff86864903d5c013eeb62c6cc`；真实固定app_agentserver完成extension加载与SIGTERM关闭，但未提交Agent任务 |
| 实际bootstrap配置链 | `/opt/aries-artifacts/jiuwen-native-audit-1/bootstrap-config-1/evidence.json`，L1发现extension、L0不启用；SHA `57bb00a5f9b0c0a6b14871009d98641af8eeed523421122d44e40ca2e0cc7565`。stage/run/config/native discovery实际执行，run UID gate与最终exec为mock，不冒充进程containment或云验收 |

Go源manifest SHA `1ed4f9472b413b4c5d986d05c6c221b464eea7ee4859366a7f93118170af4e34`；两份本地handoff为`.cache/jiuwen-producer-go-1/handoff.json`（SHA `d7d50140834229d2d700ece0aae630f751dce7dba6d98dc4f653a7ef11b359c2`）和`.cache/jiuwen-native-audit-1/handoff.json`（SHA `06b80f4fa4251ce7216dd43cd1e6e824772edf9c2aff818dbf7e0fb635712f1d`）。全部检查0云/付费模型；Python临时进程、端口与进程组清理确认，未创建容器。

正例40个ended ID与40个exported ID相等；真实FileExporter目录错误反例为ledger2/export0，即使原exporter返回SUCCESS，仍必须判缺失。Go负例覆盖整对start/end丢失、缺导出、重复、损坏ledger、未结束Span、错误终态/身份及来源SHA/状态/计数。新parser v1.11只将`jiuwen_sampled_sdk_span_exports`作为独立范围；原生全覆盖仍unknown，未插桩/未采样操作、exporter错误计数、guest进程退出/pipe-drain及跨机时钟缺口保留。旧无audit样本不回填，强杀无终态保持incomplete；不把force_flush/shutdown返回当零丢失或有界退出证明。

<a id="jiuwen-release16-record5"></a>

## Jiuwen release16冻结与record5真实失败（2026-10-27）

ECS `/opt/aries-artifacts/release16`冻结798个源文件；`evidence.status=PASS_RELEASE_16_FIVE_GO_GATES_ONLY`，`freeze-manifest.status=PASS_RELEASE_16_FROZEN`。基于release15组合原生请求附件适配、官方Span采集和共享Snapshot出口；独立cgroup资源候选未纳入。以下均为ECS实际结果，未在工作站执行编译或测试。

| 工程门禁 | 秒 | 结果 |
| --- | ---: | --- |
| make build | 12.972033 | PASS |
| make test | 30.692945 | PASS |
| make test-race | 80.383315 | PASS |
| make lint | 3.526171 | PASS |
| make integration | 335.217100 | PASS |

源码/基底/固定fixture未变，Docker库存恢复，冻结时共享锁实际获取并释放；unit inactive/dead、MainPID0。门禁自身0云调用、0付费模型调用，不把工程通过算作真实解题成功。

| 冻结原件 | SHA-256 |
| --- | --- |
| `evidence.json` | `56c233278e82bdea5f39d7726badd8b8b88ae82e74c58f3185a21ad4a5de9dff` |
| `source-manifest.json` | `149035e8e20b08d2cae8bad4bdb55e4979396cbbc609be7d23a0f3fea7b670dc` |
| `freeze-manifest.json` | `267c5bec954df686c788d130ced891e53334671094e09e0c515f9505c6d393ce` |
| `source/bin/aries` | `bc7faa7e092e74dad392894f1558ee271ef7b97e62d990ba633de71359099bfb` |
| `source/bin/aries-perf` | `f5ca4a9dc026d0d1bc3b97a7d11b38b22f42413024c28d6b810e746dc0326c0f` |

本地`.cache/release16`回执已逐SHA核对；工作树后续修改不自动属于这个冻结版本。release15及全部历史失败保留。

随后真实E2B `jiuwen-e2b-record-5`使用同image4与原`merge-ranges-portable-v1`，显式L1。首轮真实生成61个SSE帧并调用工具，第二请求本地`UNSUPPORTED` / HTTP409；独立Verifier **1/12、score0**，Tape一轮且不完整，整体FAIL。Harness协议succeeded不覆盖模型会话或评分失败。Stop/revoke、两个Sandbox独立NotFound、cleanup=[]、共享锁及8082释放均确认；八阶段与资源窗口见[逐轮性能归档](../jiuwen-cloud-validation-20261027.md)。原evidence SHA `0a737add17a01f87633f6966aec12f04483072ab0a1090115bb8f1c3dcd08725`；acceptance-summary SHA `f686d116d36d94b7a03935beed1ea104c156233483af02cd8c97ea0896644900`。

真实SSE与第二请求日志的离线复现确认`tool call causality mismatch`：原生只移除工具参数顶层`call_goal`，命令、description、工具ID/name、结果关联、前缀/临时附件、正文/reasoning均相同。`/opt/aries-artifacts/jiuwen-record5-diagnosis-1/native-contract.json` SHA `c82efdd1ea2c237f3830fdafa8d327b9d4119c16826f5b3b73815ef93166e6ba`记录15个源码证据点；这是release16的录制适配缺陷，不是record4附件更新问题，也不是Strict matcher已经放宽。未触达的动态system、compaction、path repair及synthetic tools仍unsupported；不宣称覆盖完整native grammar。后续两云真实录制成功见上方，Strict仍未通过，固定提示变体暂停。

**后续适配，已并入release17：** 显式`jiuwenswarm-e2c9d519-request-tail-call-goal-v2`在record/load因果校验中复现固定native移除顶层`call_goal`和`callGoal`的规则；两键按native规则移除，不据值类型猜测。原始SSE、每轮完整请求、Tape、执行参数及Strict比较不变，默认/OpenClaw与旧两方言语义不变。原定向证据`/opt/aries-artifacts/jiuwen-call-goal-1/evidence.json`状态`PASS_CALL_GOAL_CAUSAL_TARGETED_ONLY`，SHA `5629ba36e06770631285062420dc45df7624152a6db054b788ad1fe8622b6f46`：unit/race/vet、真实record5旧拒绝复现及新候选离线因果/HTTP record/load/Strict正负例通过，0云/模型调用；其中`full_five_gates=false`只描述这份较早定向回执。`causal-mapping.json` SHA `b3d917e1bc496c764d36dfbab47d039b7f6179cd741b9a09e82f2ef385e3892b`。上方release17完整门禁与两云原生解题另行通过，不回写record5 FAIL。

同次失败真实采到11个官方Span（1个authoritative Tool Span、1个complete root），全部精确关联，invalid=0、unassociated=0。Snapshot integrity complete / coverage unknown，Snapshot/Perfetto/OTLP/Metrics出口均exit0，**本轮TraceProcessor/Jaeger已通过，Prometheus独立补验也已通过；首轮backend1的Metrics失败保留**。原生索引coverage partial、flush未确认、producer分母未知；Span使用未对齐Unix ns。索引SHA `667c9ad3616deaeaaa56a54e199da82dbe1e013d4450cec2721e130abdd46051`，原Snapshot SHA `a529b98cdd18e225d1de158f80df504ffe0c4ab27cf861f29193b7f84770e03b`。旧宿主后端PASS不移作云端证据。

实际云原件的独立后端回执位于`/opt/aries-artifacts/jiuwen-cloud-backends-1/evidence.json`，SHA `ce90c348bbecae2536dc1f99f05d53825c58d53f2dcd68442a6a0f5b6ac199c2`：TraceProcessor导入437个controller区间/25条flow与11个native区间/10条parent flow，时间误差0ns；Jaeger查询448个Span，5387项身份/parent/时间/ToolCall检查通过，各Span起止/时长误差0ns，原数据SHA不变、Jaeger清理通过。首轮Metrics验收脚本exit1：继承的`umask0077`使临时公共`prometheus.yml`为root0600，非root Prometheus无法读取，尚未发生send/query，owned清理通过。该首轮整体FAIL保留，Metrics原回执SHA `0ab9bf8536a251ef462037fd1636fa54bff401522ffbf967d304cce169e3d196`。

仅将该公共静态文件设为0644后，`jiuwen-cloud-backends-2`对相同冻结导出独立补验PASS；没有重跑Agent、TP或Jaeger。3份非空导出各首发+repeat：attempt 117 series/117 samples、controller 8/16、任务provider 7/91；共6次真实查询、1184项检查、0失败、值误差0。时间差分别为attempt −63,171ns、controller −381,273至−327,199ns、provider 0，均为原ns到Prometheus毫秒截断。评分provider两次原receipt均空，`NOT_COLLECTED`，0 backend/0 send，不补零。3个owned容器、19095监听、unit及共享锁均清理/释放。Metrics回执SHA `5bdbc833c106a4e31949d04cd1442ab5be8baba80ad0d22441b2a870d7828311`；合并对账`combined-reconciliation.json` SHA `1a9f93f57986ebf9e87630d1096ea2da32b4f6b02dc2b0bdc8786b60b310f86b`，状态`PASS_OFFLINE_BACKENDS_OF_FAILED_REAL_RUN`。这证明同一失败云运行保留数据的三出口离线后端一致性，不证明业务成功、全链coverage、实时或持久化能力。

资源仍为控制器共享进程及整台Sandbox，非Agent或ToolCall独占：任务15 receipts中2空、93返回读数含13唯一/80重复/0冲突；CPU返回最大51.71%、内存755,273,728 bytes，不能称8.136秒Runtime峰值。评分Sandbox两次均空；IO/network与Agent/工具资源拆分unsupported。只读资源回执SHA `1a4b606ddd261b07784652f8411b5a7a5ea0e17ab7273acced2a054a2b5e3779`。历史Strict仍3/6，R01–R18退出条件及P0 PARTIAL不变。

### 之前的Jiuwen限定验收与失败

**Jiuwen并行验证：** [接入报告](../implementation/jiuwenswarm-flash-progress-20261026.md)记录宿主真实工具与隔离、历史release15五门禁、image4权限修复及两云模板初始化验收。record2/record3真实任务均失败，独立Verifier均1/12、score0，模型上游请求均0；各自两个Sandbox删除已确认，不计成解题成功。原控制端5/5事件的Jaeger接收与查询21项通过、宿主11区间实际TraceProcessor误差0ns；这些旧样本不回填为新原生OTel采集的验收。

record4使用定向fixed-tail候选，首轮真实上游生成43帧并调用工具，第二请求因合法临时附件更新而拒绝；独立评分1/12，Tape incomplete，两个Sandbox独立NotFound。原件、哈希及八阶段见[云逐轮验收](../jiuwen-cloud-validation-20261027.md)。request-tail修复已通过ECS定向unit/race/vet及真实请求离线检查，证据`jiuwen-request-tail-2/evidence.json` SHA `903a5fefc41355f2c0efc32ebd0675488a2341afe1c4e2fb76e1b9d2132f93a9`；合成末轮仅用于离线Tape收尾，未新增真实模型或Agent成功样本。默认方言仍拒绝初始多user，两个显式Jiuwen声明分别保留自己的语义。首次foreign-Harness负例fixture错配默认grammar的FAIL保留，修正后仍验证云分配前版本拒绝。后续release16五门禁已通过，record5新失败另列于上方，不覆盖本段原失败。

新Native Span分析器最终定向工程证据为`/opt/aries-artifacts/jiuwen-native-projection-2/evidence.json`：performance包unit/race/vet/build通过，覆盖原始epoch/parent身份、独立时钟视图、私有正文不导出、坏来源/身份/计数拒绝、旧SSE-only与新声明漏采区分、冻结Snapshot重载及非authoritative ToolCallID校验。原候选1单独保留。

真实官方采集证据`jiuwen-native-observability-2`包含L1 10span、L0详细span未启用、两轮真实工具及停止清理。随后**未重跑原生**，在明确标注的单Runner观测fixture副本中从真实task/session/Timing建立绑定；没有app调度分母或Metrics验收。原始RunResult、Span与采集索引保持原SHA。冻结Snapshot为`snapshot-da4623dadf72ee677341aec4a96d77863ad792bd06a5bccd7ca31868a3c43994`。

`/opt/aries-artifacts/jiuwen-native-backends-2/evidence.json` SHA `c1d036ca545015dccb898a822387fafbf2726c1576805bad0a47f2eba84c5af6`：Perfetto TraceProcessor实际导入10原生Span/9精确父flow，0ns误差、0导入错误；Jaeger2.22实际接收查询21Span（11控制端+10原生），263项ID/parent/ns/ToolCall/状态对账通过，19父关系、0跨域链接。临时容器与端口清理。首次验收脚本误查未带`aries.`前缀属性的FAIL保留，修正后仍使用同一Snapshot。完整性complete、覆盖unknown；未宣称云采集、实时或全进程观测通过。

<a id="native-tool-retention-release14"></a>

## 原生工具采集误计数：release14 修复与验证

DB `tb-repro-db-e2b-record-3` 的 `event_limit_exceeded` 来自 ARIES 的 `NativeToolCollector.observe`：旧实现先把所有本会话 `agent` 事件计入4096条限额，再筛选工具事件。模型文本增量也占用额度，因此只保留148条工具相关记录就停止保存后续记录。这不是云厂商配额，也不是4096次ToolCall；一次工具调用本身会产生开始、输出结束、结果等多条事件。

修复仅修改 `pkg/harness/openclaw/native_tools.go` 及对应测试：额度按实际保存的工具记录计算；继续检查所有事件的序号、错误和生命周期终态。4096条工具记录及4 MiB字节上限保留，真正超限仍报告不完整并继续消费连接。没有改变ToolResult、Strict matcher、执行顺序或原Verifier；通用Recorder的4096槽队列和R05文件I/O遗留不在本修复范围内。

| 验证 | 实际结果 |
| --- | --- |
| 20,000条非工具事件混合两次工具调用 | 旧实现复现 `event_limit_exceeded`，仅保留1条；修复后6条工具记录完整、终态可见并排空。输入来自测试receiver，不能称真实模型运行 |
| 真实记录/字节上限、乱序和原生错误 | 定向测试通过；超限后仍显式标记，不把丢失改写为成功 |
| Harness定向单元、race及现有A/B调用链fixture | 全部通过；其中fixture不是实际云Sandbox |
| ECS完整发布检查 | build 12.110秒、test 30.210秒、test-race 77.486秒、lint 3.428秒、integration 308.239秒，单轮全部通过 |

定向证据：ECS `/opt/aries-artifacts/native-tool-retention-1/evidence.json`，SHA `cc6ac77fbabe77e9ba96a524a410d7fa75ba56087a32bbeb1a3cd43d9ec1f624`。发布证据：`/opt/aries-artifacts/workload-replay-release-14/evidence.json`，SHA `96667b598ce3c087d39f859b8416feaa10a22c3dc31e05bfd69c8b1312a4c140`。658文件源码清单SHA `04aea9b397e408ed206e0067f2726fa924ff3c9fb2df94f373d5f1dd637a61fa`；冻结清单SHA `0f1bea40bd025dbf695b1b0e317cb4d204bc4fa99e53c9b11609f828894d2d1c`；Runner SHA `07f5827031372303794be2d11eebbf576dd589fc91e544f9902fb7e5c896fcc9`。release13及旧实验原件不变。

上述检查于2026-10-26 13:22:37 UTC完成，已核验容器/网络恢复、锁释放及可重建缓存清理；它们没有云或付费模型调用。release14真实大文本同Tape回归已通过：23/23轮、20exec+2wait、原Verifier5/5，1个Sandbox确认删除且cleanup为空。原生1371输入保留64条工具记录完整，未达到4096输入边界；20k边界仍只由定向receiver测试覆盖。双Trace13179项对账通过，4个无可信UTC锚的原生后台区间继续排除，coverage仍unknown。回执SHA `cd9c2ccb059fc5a16c893e1090ea5f680a01287db8078049f957e1d98af5d5cd`；[本轮阶段和逐工具性能归档](../native-tool-retention-performance-20261026.md)。旧DB观测仍incomplete，原业务5/7也不因采集修复升级；P0仍PARTIAL。

<a id="huawei-cpp-current"></a>

**最新真实范围（2026-10-26）：** 当前六个选定主用例 Strict 跨云完成 **3/6：fix-git、async两个原始任务＋大文本terminal-result-v2独立变体**；原始任务本身仍2/6，旧失败和分母保留。 大文本v2两次E2B/一次华为同Tape均PASS；SQLite v3录制原3/3，Strict自回放因安装日志回读mtime偏离而FAIL、原0/3，清理确认。新增完整SHA见下方[当日闭环证据](#text-huawei-sqlite-v3-current)。此前release5 / native publication3 的同版三题双云7次尝试及全部阶段/工具/离线双Trace取证已完成：fix-git、cancel-async-tasks成对成功，华为DB首工具后Strict偏离，async原中断另保留。每次原evidence、summary与TraceProcessor/OTLP对账SHA见[双云性能与证据索引](../tb-crosscloud-performance-20261026.md#复核位置与固定身份)。其中中断源保持incomplete，不将投影通过或后续恢复清理改写为原任务成功；本批新Jaeger/Prometheus发送查询和UI仍UNRUN。

华为C++ record-2原6/6、reward=1，evidence SHA `8aa8f8c2ae70b552d8376aea5b091c5c0cd75b9ab84ec5fcd269e5c4982b548f`；replay-1首工具217行中仅第3行 `..` mtime由03:55变04:10，在第2模型请求Strict失败，原5/6、reward=0，evidence SHA `9a73ff20538b17e1838be14ab846ed3bb20049cc5d53ce5d538b3778317a19b6`。两次均确认删除，原record-1无CTRF/reward的评分流程FAIL保留。原件、逐阶段和差异来源见[C++专项归档](../tb-huawei-cpp-performance-20261026.md)。async原中断的恢复清理回执SHA `2f785f5977d47aed25afef9650d777a1d01fb4cfd29d05839f52ea0fa71f9725`保留。

下文19目录/18Attempt及87次PASS均为此前封存分母，不加入新尝试冒充同一批结果。P0 PARTIAL、R05暂停、R13暂缓及R01–R18、P1/P2退出条件不变。磁盘清理仅删除可重建缓存/合并字节相同投影，原始证据与失败保留；[归档保留规则](../../deploy/e2b/README.md#归档保留与磁盘清理)。

**Workload Replay 真实增量（与 Strict 分开）：** 显式 `model.replay_policy=workload` 已通过 [release12 五项 ECS 工程门禁](VALIDATION_AND_REPRODUCE.md#workload-replay-release12)，完成 [DB](../workload-db-performance-20261026.md)、[SQLite v3](../workload-sqlite-performance-20261026.md)和[C++ v2](../workload-cpp-performance-20261026.md)同Tape跨云功能对照：分别各12轮/11工具/原7/7、41轮/38exec+2wait/原3/3、35轮/34exec/原6/6。真实终态、停止撤权、独立评分与删除确认通过，正文差异留证。**Strict仍3/6，另有3/6完成Workload功能对照；不能计成Strict 6/6。** C++整体观测incomplete；其他成功样本的全链coverage仍unknown。双Trace对账只证明保留数据的投影一致，不将功能成功升级为全观测通过。C++新E2B共享TTL3600秒，实际全程288.328秒；旧4200创建拒绝与新Strict的GDB地址差异失败均保留。SQLite首轮Snapshot重载FAIL、华为缺profile文件FAIL修复后新轮通过；[SQLite v4新录制](../sqlite-v4-performance-20261026.md)原2/3、Tape不完整单列，不覆盖v3。全部历史失败保留，未改旧Tape或Strict matcher，P0仍PARTIAL。

<a id="workload-replay-release11"></a>

## 显式Workload Replay：release11工程限定PASS

ECS `/opt/aries-artifacts/workload-replay-release-11` 于2026-10-26 08:28:33.070849 UTC结束，状态 `PASS_REPLAY_POLICY_FIVE_GO_GATES_ONLY`，冻结656个源文件。基于未修改的release10副本验证显式策略和同Snapshot脱敏证据接线；0次云调用、0次付费模型调用，未部署到当前主线。默认Strict与原Tape不变；此工程检查完成时，真实Workload云回归、相同实际工具工作量及原独立评分验收为 **UNRUN**，不能把工程通过记成业务或性能样本通过。后续真实结果见本页顶部的逐次归档，原工程收据不改写。

| 工程检查 | 秒 | 结果 |
| --- | ---: | --- |
| make build | 11.365717 | PASS |
| make test | 28.235735 | PASS |
| make test-race | 76.505733 | PASS |
| make lint | 3.275771 | PASS |
| make integration | 311.774642 | PASS |

原release10源码、候选源码、固定TB fixture均逐项未变；Docker容器与网络前后相同，遗留step进程组为0，仅删除本轮472,449,024字节可重建缓存。integration在已结束且清理确认的SQLite v3 Strict回放之后执行。以上是工程检查自身的资源审计，不替代未来Workload任务的停止、隔离和清理门。

| 证据 | SHA-256 |
| --- | --- |
| ECS `evidence.json` | `4207eea46954a605084fa02ef04ce522daa7ae4b8c1ed9b4f39363c4e249561f` |
| ECS `source-manifest.json` | `409a911602b3ae6aea944faf3fc1e987cff29e91965665c6697cf580ca339851` |
| ECS `formatted-overlay.tar.gz` | `2a2e70adb602f73e241387f8c75d116dd84b07c28981d50e7e189b705d4adc34` |
| 本地 `go-sync-receipt.json` | `7c452f4754e3a3ca8b03b250c2decf5eac0b07f32bd562ae4fb3dd4aafa2f028` |

本地小型收据位于 `.cache/workload-replay-release-11/receipts/`。13个Go文件在同步前再次核对仍与上传的格式化前字节一致，随后与ECS冻结manifest逐SHA对齐；只同步Go，没有覆盖之后更新的docs或`.agents`，未在本地运行格式化、编译或测试。五项原日志的SHA和精确时间保存在ECS evidence内。

<a id="replay-variants-release7"></a>

## 独立变体：release7工程与driver限定验收

release6的 `make test` 真实FAIL：两份terminal-result profile引用未归档的 `.cache/repro-workload-1/versions.json`。改为仓库内 `configs/versions-terminal-result-v1.json`，只固定已实测/发布的OpenClaw镜像，默认catalog、Go、原题与Verifier不改；原FAIL保留。release7冻结641文件，在ECS完成build/test/test-race/lint/integration，分别10.765/28.399/94.724/8.765/313.168秒。manifest SHA `a3f8b2fae6314725f9259d4017927c014fb796b38213182ba688400aa0967cdc`；旧源码、原2容器/5网络未变，无遗留进程，仅清理本轮472,444,928字节可重建缓存。

| 证据 | SHA-256 / 限定结论 |
| --- | --- |
| ECS `tb-six-release-6/evidence.json` | `5eadf83631c3ea6184776f5badf6e15d0d386d0a705b926f3cb671160ad45fd1`；配置引用FAIL保留 |
| ECS `tb-six-release-7/evidence.json` | `7242b827dc0a32f9fd70e5d90b1b3f7df900d5186a6fe484b10c1e4a42c5272b`；五门PASS |
| ECS `repro-workload-go-2/evidence.json` | `53ed77517df19ce59cc6c486100dc2e8969763d31f20dbf45011f4f5b8740fa6`；七包unit/race各1205项通过 |
| ECS `repro-workload-2/evidence.json` | `aa51406b7dce9efd214b9d0a7d5f55299e84540ba47aa1bada3714c9e0412839`；限定原生探针16/16实际Bridge身份 |
| ECS `repro-workload-native-publish-1/evidence.json` | `9b9da7d04c32d346c4f455df0aa56584800eda370ce4fb98e890c1a7c20a7c1e`；相同已测镜像发布，digest `39c688b7c67c279e803bb01cdfa651d7a146f12436a8f47b11f79195167b12ed` |
| 本地只读 `.cache/repro-agent-run-1/checks/summary.json` | `b16aa1c02eb2df61b7b17c5b7e4700f723546998fcd820b82d2734f6af3496fb`；driver 10项合成检查及旧原件离线兼容PASS，0云/模型 |
| ECS `tb-repro-driver-legacy-review-1/evidence.json` | `bd8e27da2ba5ba7ce6c457841bd3b06e96bbaae672b45174c4e9a56b3ae89aea`；实际旧原件复核，不新增Agent尝试 |
| ECS `tb-repro-agent-inputs-1/pins-evidence.json` | `7a3c020b92b967c0b4025cca374186ed0dccaec03697e3a35a693378bb9cf682`；源码/6二进制及native四helper逐SHA对账，准备通过 |

ECS相对路径以上均位于 `/opt/aries-artifacts/`。driver合成日志SHA `e00da7e465fbb1b007f5d736308609b32bb1a35656e71b4274a4472930943589`；复用的pins SHA `10f7435bdb8da12fabaaaaddefcde1790dbfa70c058cb828f99fc0e2e15c3d57`。旧Go attempt1中断、native首轮接纳后未派发的取消覆盖缺口均保留。新wait独立时钟/身份解析的工程和合成验收，不等于真实Agent双Trace完整覆盖；保留unknown及无锚区间排除。

新DB `metadata-portable-v1` 首轮已FAIL，原评分5/7，68轮完整Tape，Sandbox删除确认；65次exec中2条host政策拒绝缺少producer身份，不能补造旧观测。原evidence SHA `67e440ef5d3a698078ca0465bf923c74b167a6d54cd0c520922c1b1c76477fee`；失败样本的八阶段、65条exec与不完整来源双Trace对账见[新变体归档](../repro-variants-performance-20261026.md)。独立变体仍需新真实录制、同Tape Strict回放、原独立Verifier及清理闭环；原始任务与变体分开统计。[变体设计](../design/reproducible-agent-workloads.md)。

本批后续：大文本原5/5但第15个模型请求的工具参数重复JSON键，Tape不完整，整体FAIL；SQLite原3/3、37完整轮及36次工具调用，观测验收因归档丢字段FAIL；C++华为独立变体录制原6/6、46轮/42exec及3次其他工具；同Tape回放仅执行首工具，因根目录初始mtime偏离而Strict失败、原5/6。原binding无初始mtime声明，后续需通过既有准备接口固定新初始值并重新录制，不从旧分钟推造秒/ns。以上已结束的录制均确认实例回收，原失败不回写。八阶段、逐工具与原件SHA集中在[变体性能归档](../repro-variants-performance-20261026.md)，不复制原始大文件。

| 定向修复 | ECS实际证据 | 限定结果与下一步 |
| --- | --- | --- |
| R01 host政策拒绝身份 | `r01-host-policy-rejection-1/evidence.json`，SHA `a9004f246efa16297e51dc3a6cd17b294f359629a361ea0d11cbca9dc8d3f582` | 实际原生三路8项错误相同，6身份/5拒绝/11记录、0SSH；performance unit/race各328项通过。保持原host政策和错误；合并版本后真实Agent回归待执行 |
| 原生终态小字段归档 | `r01-persisted-details-1/evidence.json`，SHA `a6f849539845e7f3b1220e5e13170ddb0235d0690e748194f8e2ad273bda3456` | 原生SessionManager→guard→JSONL的8组通过，覆盖8191/8192/8193B、二级fallback、false与缺字段；仅两白名单各加3字段，正文/原对象/8192B阈值不变。合并镜像后的SQLite原Tape真实回放待执行 |

两项检查均为0云/模型调用，定向容器已删除；不替代真实回放。SQLite旧记录保留`protocol_wait_result_differs_from_terminal`，不能从现场结果补齐旧遥测后改判成功。大文本的模型重复键仍按原严格JSON规则拒绝，不作宽松修复。

### release8：合并修复与真实回放入口

host拒绝身份与长结果归档字段合并为一次镜像构建，固定image/digest `e040654efde8adde910583291b547fb9d5fc94e565b2f4dc8bad29abcc423041`。默认及terminal-result两套真实host探针、8组真实持久化场景和6个协议helper检查均通过。私有checker两次误判分别来自fixture sessionKey与旧基线不同、Node默认spec输出被按TAP文本判断；原FAIL保留，最后复核读取已执行的原日志、退出码及逐案例结果，没有新增测试或修改产品错误。

| 产物 | 固定证据 / 范围 |
| --- | --- |
| 原生合并复核 | `r01-native-combined-3/evidence.json` SHA `02837bea4c4d365badd2eb7b73b1e2e0276698ae91e2908e10db8be6a56c2019`；仅复核，实际检查位于combined-2、唯一构建位于combined-1 |
| 发布 | `r01-native-combined-publish-1/evidence.json` SHA `4ecd30d312e13f6aa1429ea6af1c5aa8fd5ec83a68a77d21b68a801d600e5352`；相同已测镜像，临时认证已移除 |
| release8五门 | `tb-six-release-8/evidence.json` SHA `53b6f730301613920e4407a4691f6306a2ec537403f27c9ccce83d0c93c0d974`；build/test/race/lint/integration全部PASS，分别12.114/29.143/75.567/3.281/313.200秒 |
| release8源码 | manifest SHA `89d8b62871afc87fbb32c9cb9bef1b040a394a9e1af88d3aaca0f0e4d3ffe9b2`，649文件；原release7未改，当前版本自身临时数据集缓存472,449,024字节已清除 |
| 新driver检查 | `tb-repro-driver-2-checks/evidence.json` SHA `3ecb173194041ac447ac4ae278b59c924e1159e9a51ec3ef1f7c480fcbfc85a9`；10项合成及旧async原件只读复核，0云/模型 |
| 新运行pins | `tb-repro-agent-inputs-2/pins.json` SHA `b15bf6a150e9cc83dad61a432c3be992449d2d7df7eb94f07bef126c9f08fb67`；绑定release8和新发布，不修改旧pins |

<a id="replay-variants-release8-results"></a>

### release8 后的真实失败与下一步输入

| 运行 / 证据 | 实际结论与后续 |
| --- | --- |
| `tb-repro-sqlite-e2b-replay-1/evidence.json` SHA `bc7f4716610dbbf7a1ebd1cdd3200b74c62644b34db7bfcee942fb23b0ccdb36` | 前5请求严格匹配，第6请求`$.messages[11].content`偏离；解压后`/tmp/sqlite/..`即`/tmp`日期改变。原0/3；未到达process/wait。Sandbox删除确认，清理错误为空。 |
| `tb-repro-cpp-huawei-record-2/evidence.json` SHA `5ab1b4351b15c8ba902b00a180e5bad9b9960e44ad2762b22d107cc4e1eeb903` | 新录制原6/6、63完整模型请求、64exec+3process；63exec精确、1未知，整体FAIL。`/`初始mtime实际读回1704067200000000000；Sandbox删除确认。 |
| CPP `operator-review-1.json` SHA `83c237929d8b040c17f6ba1b8d5245e304eb6b1852323569596be191e03813d3` | 同一辅助execution ID的SDK wait exit0，随后stdin canceled、Bridge255。`ExecStream`先cancel再读inputDone会把自身收尾错误并入成功结果；native调用→辅助检查仅推断，不补身份。新系统修复及真实回归待验。 |
| `tb-repro-driver-3-checks/evidence.json` SHA `bf3f56e98d635fd8c92446c40f6ad19be5ce52a05fac87d7bfdda19bd00eb5b9` | 14项合成检查和原async离线兼容通过，0云/模型。只让公开附录独立SHA固定；source内部附录仍须匹配manifest。真实driver、Strict、原题/评分/清理语义不变。 |
| `tb-repro-workloads-v2-1/verification-receipt.json` SHA `f63e14b24ce382c406b108f53321ad977f04dc211d08017749d359b969474374` | text/sqlite v2共12文件39,077B，SCP完成并逐SHA核对。manifest `458ce59a47875b8b45b3affb3889f99899b609f43aba638ccb45ffe2447bc32c`。只是静态材料准备，真实录制/回放UNRUN。 |

v2统一公开附录SHA `0a7859d75d23a4497ce17a894c2b7b1fe5a3fff968cb1db931c0ea6cdf619c05`，补强names-only目录查看与有限后台任务省略可选单次wait timeout、等待真实终态的说明。原生后台/PTY保留；协议仍terminal-result-v1，镜像仍e040。模型是否遵从说明须真实验收，不能从静态准备推定回放成功。原题、私测与预算不变。

旧CPP成功评分不消除基础设施255；旧SQLite完整Tape不消除动态元数据偏离。两条Tape和所有FAIL均保留，系统修复后重新真实录制，不改写结果或过滤工具正文。逐阶段、工具和离线Trace见[变体性能档案](../repro-variants-performance-20261026.md)。


<a id="execstream-teardown-fix"></a>

### ExecStream 收尾取消误报：真实缺口与定向修复

`pkg/sandbox/e2b/client.go:ExecStream`将自身收尾`cancel()`移到非阻塞收集`inputDone`之后。真实输入失败仍先发布错误再取消等待；父context取消和命令timeout仍由Wait传播。没有忽略所有Canceled，没有修改Bridge退出码规则或停止证明。

ECS `/opt/aries-artifacts/execstream-teardown-fix-1/evidence.json` SHA `2f50d9dde33ec48a068e9bd514772f5b14004023b25a157784164ffb53a2b230`，状态`PASS_OLD_RACE_REPRODUCED_AND_CANDIDATE_PACKAGE_CHECKS`。旧版20次受控重复中3次复现成功进程被内部取消污染；这是压力夹具结果，不是生产发生率。候选完整e2b包unit/race均通过；覆盖真实输入读失败、adapter错误、父取消、命令超时，以及退出码0/7与原始双流字节。候选源码SHA `00299412678742edee4a97e262bf1663f4c83b54bd4a9db79edc4b0010b27a18`，新测试SHA `4b3507542bd35007de748f384c83b3319ee63dffd4d9fa2826a73c2b507646ad`。0云/模型调用。

release9的build/test通过，完整race检查在既有`TestRunProfileConcurrentDuplicatesPreserveDispatchOrder`失败（peak=4，原断言5）。测试的两次独立`peak.Load()`允许CAS将5覆写为4；另一个测试存在相同写法。只修两处测试CAS为同一观察值比较并交换，断言与调度实现不变。release9失败归档保留；release10包含这项测试修复，100次相关fixture回归及build/test/test-race/lint/integration全部通过。工程证据SHA `19c1c695ccfa51cd32cf5c6b9784d6f4537ae0e9d57a8fc7ad01635e87432405`，650文件source manifest SHA `0863298927cd663b5a91cca47e49c59e120e45fd90d4fcb83b137f5c9bb7d797`；五门分别10.706/28.183/72.111/3.385/314.774秒，临时数据集缓存472,444,928B已清理。

release10绑定回执 `tb-repro-agent-inputs-4/pins-evidence.json` SHA `0d555b28486e126da29bd9695c65c5099a5b7836859be50c8a36d6669614fd22`；pins SHA `e4f590ff108f429b0f915c055777291f4e71ac59a0e465779a7496be0b91f5ea`。复用e040原生镜像，driver run-5相对已验run-3仅更新release路径及错误提示，validation逐字节相同。小型工程收据包已SCP取回并校验，118,488B、SHA `db635df518fb4e83892642e35720a4b6c188626e3232fbbb267d6ca220b73921`，不复制完整源码和镜像。

CPP v2 新录制 `tb-repro-cpp-v2-huawei-record-1/evidence.json` SHA `b6c52d186cb1709cacc571a77d5cc57c71a4c4ca87e8afe2a603b8d06fc88423`：原6/6、35完整模型轮、34/34 exec精确，限定单次实录PASS。Tape SHA `ea1a8d1b0f1147e3108b7fd21a965c890d07f2e206e8db6de770d6b686a867df`。同Tape真实回放 `tb-repro-cpp-v2-huawei-replay-1/evidence.json` SHA `2c14911287019e3d7d6586d5a2f29a7b688e37ecdc0fd26a7e991ce6b8ac022a`：前5模型轮/5exec精确，第6请求Strict偏离，随后2次SESSION_FAILED；原5/6、reward0、整体FAIL。首偏离精确对应第5个exec：`gdb --batch … 2>&1 | tail …`在命令内部合流，两行线程库提示位置变化；39行/1925B及两行块字节相同，没有PID、地址、日期差异。诊断SHA `6221a9d23d1dcfbca19f0d3a6b0fccd87cd96ea24399b61022be52ca0060495d`；不重排输出、不放宽Strict，不重开已暂停的R05。没有fallback，两次停止/撤权及Sandbox删除均确认。后台协议两次均未触达，incomplete保留；新录制身份通过不替代完整回放。逐阶段与工具证据见[独立变体归档](../repro-variants-performance-20261026.md#cpp-v2-huawei-record-1)。

SQLite v2新录制 `/opt/aries-artifacts/tb-repro-sqlite-v2-e2b-record-1` 已终态PASS，runner0、原3/3、35完整模型轮、36/36 exec精确及1次真实后台wait映射通过；evidence SHA `6c0d043c7428fb4fb3b96b4cea37df74ff3a08faaba101944feb1e80461b224c`。unit `aries-tb-repro-sqlite-v2-e2b-record-1.service`、invocation `657cfd9c97484e90b4ce51d8e1b63437`，终态MainPID0/inactive，完成于06:47:43.767383 UTC。使用同release10/pins与profile SHA `925af54676008bfa4faad1ee63e5404d182d334393911d74fcb4ea20a77ab497`，原900+900秒预算及2400秒Sandbox TTL不变。Tape SHA `71aa297888b2eb2ee287ed0f401abbbeb0928b0a8242a41c3b7b2794d4418850` 保留，真实回放结论见下文；本次录制成功不替代回放资格。Snapshot integrity complete、coverage unknown；5个封口后ADMISSION_CLOSED与35轮Tape分别计量。

SQLite v2同Tape E2B replay-1为真实FAIL：23个模型轮匹配，24/24 exec精确及1次wait已执行；第24次请求在 `$.messages[49].content` 被Strict拒绝。真实 `make install` 输出的3个产物时间由06:46变06:57，其余字节一致；原评分0/3、reward0，剩余12次exec未执行。后台wait再次实际触达8193B归档截断，三个小字段与terminal一致；独立离线协议范围验收通过，不改写原driver或Strict失败。八阶段、24条exec、首差异和SHA见[SQLite v2回放归档](../repro-variants-performance-20261026.md#sqlite-v2-e2b-replay-1)。

大文本v2 E2B record-1为真实PASS：原评分5/5、reward1，23个完整模型轮、20/20 exec精确、2个job/2次wait映射通过。Tape SHA `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893`；23次上游请求、cached retry为0、停收后拒绝为0，本次没有触达8192B归档截断。此处仅记实录，不能替代同Tape回放或跨云资格。阶段、20条exec与后台等待见[大文本v2录制归档](../repro-variants-performance-20261026.md#text-v2-e2b-record-1)。两次均各创建1个Sandbox，停止/撤权及provider/ledger不存在确认完成，cleanup errors为空。

以下原件均位于 `/opt/aries-artifacts/` 对应实验目录；summary为 `performance-review-1/summary.json`，audit为 `offline-analysis-1/evidence.json`，各只分析一次。

| 当日实验目录 / 原结论 | evidence SHA-256 | summary SHA-256 | audit SHA-256 |
| --- | --- | --- | --- |
| `tb-repro-sqlite-v2-e2b-replay-1` / Strict FAIL | `4e7e82f7f738da66163b7ed43fd00f048a9e0a4d4f0ef8865e50daae8ca2f05a` | `9cb637359e0a17b8a6ae704f1915ebd437c9e187a9a2a79802f26ebd87318be8` | `9b9529ca8ace233d6cbde30f98a6817d7e949976fce7e71f53ffcfd481a83219` |
| `tb-repro-text-v2-e2b-record-1` / 实录 PASS | `55b398db414b164486593041a6119fd317e9b4594e70d74e0615aeaa35a75bf6` | `0a75f70e5573bbd590a46a495e8e434d7751b159f481be43d63631eede1bec9b` | `5cea60dbd42f20fe345a46dad53c26887f65b146bc707213e96f227ff8c98e00` |
| `tb-repro-text-v2-e2b-replay-1` / Strict PASS | `ea5229721d285a48d6d32e0aedc5384ba5e48d42b5a44458b8f44f116cde3dbd` | `57eb695b864c4803c90f02276de906676e857039bd0ba24d40a03a303ff22628` | `5bab2cf7552ef15add648672dc9c52deff3d8c5ce0c875445b61d4743f2956fc` |
| `tb-repro-text-v2-e2b-replay-2` / Strict PASS | `3beff58ef8b3273783834d034e08fe79b8c4781c5e877fa7ee899d014c94a0b8` | `d48cbe56fc0fd75ae1e8180754dd744902152582bd60e4a9ad6c45ec657ec5f9` | `729bc86482589eb5dc328ceac0671c0900d108db069fa2a058a730eb02efb239` |
| `tb-repro-text-v2-huawei-replay-1` / 跨云Strict PASS | `934a78c818f6f63166105ec7be47966f19f246a93d7bd43824b3277fb832185b` | `ef49355b97f3657b7ebf3144c7d7d2d9345b22da2ab118b78f9683de82575817` | `dfbbc383a37d6eec8c121ce6e6a7b2ec3716135aedcb321a93e2e91c8df04e58` |
| `tb-repro-sqlite-v3-e2b-record-1` / 真实录制 PASS | `82e293d5ad38b0b166fff530d40454eab95346335ad8fd6cb3849d5fe8870893` | `8ac86210766c88272221f39899a96b0ad045433098c12e63ae28d8b2c3e73faf` | `2caecf51d62e6db8d2eecbafb91a0af0fa2607374c5f4e40d5e65e8c46067cf4` |
| `tb-repro-sqlite-v3-e2b-replay-1` / Strict FAIL | `831d5552cef6814f39429a30ac02871fc8a76c860c021c9fe75cd2309af207e4` | `65872b9d2360fed59aa586fe7524525369e0b43fb7f489aa9a0fdd02e5d33a17` | `15d99f60575ba639f7552c956fba32ff09e3b1f03f7edf3114594aef340b4bb5` |

大文本 v2 第一次同 Tape E2B 回放于07:23:05.162205 UTC完成，unit `aries-tb-repro-text-v2-e2b-replay-1.service` 实际MainPID0、inactive/dead、ExecMainStatus0。原5/5、23轮Strict匹配、0上游请求/缓存重发/拒绝，20/20 exec精确及2job/2wait映射通过，停止/撤权与唯一Sandbox删除确认。13,283项离线对账0失败，3,306 Span、106 flow（104 parent、2 link），4个原生区间及6条边无UTC锚点而排除，integrity complete / coverage unknown保留。八阶段和20工具表见[首回放归档](../repro-variants-performance-20261026.md#text-v2-e2b-replay-1)。Tape原件SHA未变；该次结束时尚不能替代第二次独立复现或华为对照，后续两次结果另列。

第二次独立新Sandbox回放已于07:30:26.855846 UTC完成，unit `aries-tb-repro-text-v2-e2b-replay-2.service` MainPID0、inactive/dead、ExecMainStatus0。仍为原5/5、23完整轮、20exec精确、2job/2wait映射通过，0上游请求/缓存重发/迟到拒绝。停止/撤权、provider与ledger不存在确认均完成；13,231项离线对账0失败，3,293 Span、106 flow（104 parent、2 link），无UTC锚点范围及complete/unknown不变。该次结束时所选Tape已有两次同环境独立复现；后续华为同Tape通过另列于下方，不回写该次原件。详见[第二次回放](../repro-variants-performance-20261026.md#text-v2-e2b-replay-2)。

SQLite回放 / 大文本录制的离线双Trace分别完成6,153 / 13,391项检查、0失败，结论均为 `PASS_SCOPED_REAL_RUN_DUAL_PROJECTIONS`；对应1,524 / 3,333个Span，最大时间误差0ns。两者Snapshot均为integrity complete、coverage unknown；SQLite的2个原生区间/3条边及大文本的4个原生区间/6条边缺UTC锚点，保留但不导入主时间轴/OTLP。未新增Jaeger/Prometheus查询或UI验收，各3份相同投影已硬链接去重。投影对账通过不改变原Strict FAIL，也不增加跨云成功计数。

SQLite v3的历史静态准备收据（不是当前运行状态）：本地收据 `.cache/tb-repro-sqlite-workload-v3-1/evidence.json` SHA `2d373763c4469bb672c877cdbb51f85732c387043755a552616f6c7ddd735ce1`，状态 `PASS_STATIC_MATERIALS_AND_PROFILES_ONLY_REAL_V3_UNRUN`。静态收据核对6项材料、原题/私测来源及E2B录制/E2B回放/华为回放3份profile；manifest SHA `c8ab326c57474ef713acbb70cf475f81fec557bba67159586381a2214ec7f50a`，公开附录 SHA `7fae98659041edf2600c81f7e78d3a2036cae7ff3b536d4f59f9d3102789cccc`。除版本标题与在已批准变体范围内补充的一段公开日志说明外，v2正文不变；原来源、旧材料、native协议及binding保留。该次静态检查没有构建或执行工作负载/模型/云调用，当时真实录制、Strict回放、跨云比较均为UNRUN；原收据不改写。后续录制PASS、自回放FAIL见下方；工作区日志自动归档仍为NOT_AUTOMATIC。


<a id="text-huawei-sqlite-v3-current"></a>

### 大文本同Tape跨云闭环、SQLite v3录制通过与回放失败

华为大文本 `tb-repro-text-v2-huawei-replay-1` 已于07:44:37.726631 UTC完成，unit inactive/dead、MainPID0、runner0。原5/5、reward1；23轮Strict匹配、0上游请求/缓存重发/拒绝，20/20 exec精确、2job/2wait映射通过。Tape仍为 `d41842ecac2175adbeaee203413f8abd97affd56a8a779f790ef95f07033b893`，没有换轨迹。13,675项离线对账0失败，3,404区间/Span、106Flow（104parent+2link），4原生区间/6边因缺UTC锚未导入主Trace/OTLP；integrity complete / coverage unknown保留。唯一Sandbox停止/撤权、provider与ledger不存在均确认，cleanup errors为空。结合前两次E2B回放，当前证明同Tape两次E2B与一次华为的完整功能对照；八阶段、20exec及资源差异见[三次对照](../repro-variants-performance-20261026.md#text-v2-huawei-replay-1)。

评分长尾有直接原件：该run的 `evaluation/stdout.log` 第7行报告APT获取9395kB耗8min6s、19.3kB/s，整文件SHA `94db2f489a961b4a094b359147548d18331dc61f3f6ef8a39cce45b7b950c82a`。`runner-events.jsonl` 第4133–4138行对应prepare0.331727501秒、exec517.824024672秒、report1.099812189秒；CTRF原测试25.0359447秒。完整评分519.266395855秒不是Agent Tool耗时；APT取整日志不构造成精确网络Span。只读诊断 `.cache/tb-repro-text-v2-huawei-verifier-diagnosis-1/conclusion.json` SHA `f33da732e760205cb3c9ea5c424cebde865755ffda3dbf2b1f6d90b3c163ac77` 保留原路径/行号，DNS/连接/传输归因仍未知。实际profile/adapter `assume_network_isolation=false`，原题允许出网；停止撤权通过不证明评分禁网，不能记为完整出网隔离的Goodput。

SQLite v3 `tb-repro-sqlite-v3-e2b-record-1` 已于08:05:20.432092 UTC完成，unit inactive/dead、MainPID0、runner0，原3/3、reward1。41轮真实上游录制，Tape complete=true且最后stop；38/38 exec精确、2job/2wait映射通过，40次原生调用/结果保留。5条ADMISSION_CLOSED均在同域关闭时点之后，不进入41轮Tape；0cached retry。33次exec退出0、4次退出1、1次退出127原样保留；原评分成功不掩盖中间命令错误。本次没有大于8192B details截断覆盖。唯一Sandbox停止/撤权、provider与ledger不存在均确认，cleanup errors为空。

SQLite v3完整Tape SHA `a73e482ea845c44b370df656d5358b8231a5c161e1acdbd5e7da1d8edda72d46`，绝对路径 `/opt/aries-artifacts/tb-repro-sqlite-v3-e2b-record-1/runs/20261026T075942.346881697Z-tb-sqlite-with-gcov-terminal-result-v3-record/sqlite-with-gcov-terminal-result-v3-001/model/tape.json`。该Tape随后已用于Strict自回放，失败详情另列下段；原Tape保持完整且未改，不计回放PASS，华为对照未运行。10,711项离线对账0失败，2,663区间/Span、219Flow（217parent+2link），4原生区间/6边缺可信UTC锚，integrity complete / coverage unknown及artifact_proven=false保持。详见[录制阶段与38exec](../repro-variants-performance-20261026.md#sqlite-v3-e2b-record-1)。

SQLite v3 `tb-repro-sqlite-v3-e2b-replay-1` 已于08:15:10.747238 UTC结束，runner1、原评分0/3、reward0。第25请求（零起点turn24）的 `$.messages[49].content` 首次 `REPLAY_DIVERGED`；wait `call_00_2VvLMEuEjsdfLQNTpV247411` 精确关联exec 22 `call_00_00rjJN3XgiTZ656qW2hn8439`。make install输出已存工作区日志，但随后tail回读了3条产物mtime，08:03→08:14，仅6字节变化；两边正文均1983字节。24轮此前匹配、22exact exec+2wait均为原Tape的精确前缀，剩余16次exec未执行；0上游调用、首次拒绝后5次SESSION_FAILED保留。停止/撤权、唯一Sandbox的provider与ledger不存在均确认，cleanup errors为空。详见[完整失败归档](../repro-variants-performance-20261026.md#sqlite-v3-e2b-replay-1)。

本次失败样本仅离线汇总/分析一次，5479项检查0失败，1355区间/Span、114Flow（112parent+2link），最大投影误差0ns；4原生区间/6边无UTC锚点保留在Snapshot，integrity complete / coverage unknown，不增加任务或跨云PASS。协议范围独立复核为2job/2wait通过，receipt SHA `a0fcf8884c28eb733193d7071bcd7a262c15bb1b8edfb0e218cf956a62502a04`；源命令与wait映射补充receipt `/opt/aries-artifacts/tb-repro-sqlite-v3-replay1-diagnosis-1/source-context.json` SHA `cc4ecc3f62053d71d628484336e0e153e376ec763544c936ded4f5bed267a7b8`。原driver FAIL、Tape及原私测不改；无新云/模型调用或后端查询。

上述华为大文本回放与SQLite v3录制各仅执行一次summary和离线审计，原evidence/Tape未改；3份投影各在SHA相同后硬链接共享，分别释放73,310,208 / 70,438,912字节。去重收据SHA分别 `5336588e04dce2d4562f71a8066184e5fd13bdd2947713a7a30b0a8c61779ff8` / `cd5c0edc87cdb447f5ba00ea779b8745cda8983eae2abc29cce7585f69ac0b2d`。本轮离线分析无新云/模型调用，未新增Jaeger/Prometheus查询或UI验收，完整evidence / summary / audit SHA已列在上方当日表。新结果不覆盖旧失败、未运行分母或R01–R18退出条件。

以下为C++ v3的历史静态准备记录；其后真实录制已发生HTTP 402失败、原5/6、Tape不完整，Strict仍未执行，见[实际结果](../remaining-strict-cases-20261026.md)。准备时证据：ECS `/opt/aries-artifacts/tb-repro-cpp-workload-v3-1`，receipt SHA `b244a1af2397de281d255175047ede190bd284bb55824891a740f2caa7feb7d6`，manifest SHA `8fc4a2c2fb2bebe41f39921e80fca629d0355e16608b452b5b11a882eb32ff1e`。原样复用SQLite v3通用附录，最终Prompt SHA `4d651f0a71de8618062cd0c2ba9cd3cbdc819b73b0d50266a65642b1abdbec12`。原10文件/6项私测、1800+1800秒、4200秒TTL、固定根mtime binding、release10/e040/pins4及旧材料未变；Huawei record/replay profile只改独立身份与附录路径/SHA。它不修复底层合流顺序，也不允许隐藏必要崩溃诊断或错误退出。新材料没有新增构建、云调用或模型运行。

旧CPP首个ToolCall的原生事件已核对：同ID的tool/start、tool/result（is_error=true）和command_output/end均在；process_completion unknown，exit/duration缺测。native index为198事件、complete=true、issues为空；index SHA `5c55317acdb96ab64ba0abb284e1ca0b5a48384f0ffa5f928ce4230796ba0331`，events SHA `7ceac86f5f6564716d97e1b7979d6f0c7b85b8160d195e9174ee5cb021a72de4`。缺口是ToolCall到辅助初始化检查的因果边，不能描述为原生ToolResult丢失。ensureRuntime/validateWorkdir使用较早的独立SSH session，正式工具session的身份文件在初始化后才写入；辅助检查维持环境准备scope，不能并入真实工具执行分母。

本轮小范围清理在SQLite录制结束后、回放开始前执行：仅删除可恢复的 `execstream-teardown-fix-1/source` 副本和未部署的 `tb-six-release-9/source/bin`。保留base8源码、修复输入、source manifest及原失败日志，删除前后保留文件SHA不变；目标无进程/FD引用。实测文件系统空闲净增98,762,752B，清理后可用37,505,089,536B；receipt `tb-disk-cleanup-20261026-3/evidence.json` SHA `44faea2bd83aab1bfd7c9bcb4325adbb20fe53443d77e5ed4645b40f5e7f7788`。没有全局prune或修改当前release10/镜像。

真实CPP的辅助检查与首个ToolCall之间仍缺直接身份边。该次初始化失败原因由SDK/Bridge同一execution ID直接支持，native调用关联仅有源码顺序支持；不能把这条旧unknown升级为exact。退出条件：新版本真实任务逐调用对账、完整Strict回放、原评分和清理通过；其他初始化失败的原生身份覆盖仍需独立证据。


<a id="tb-six-current"></a>

## 2026-10-26 六个原始 Agent 任务：release4封存批次

新增6次真实运行已完成，固定release-4与native publish-3，6个Sandbox均确认删除。合计19个实验目录、18条原Attempt、17次执行Agent、12次原评分通过；原driver有7次PASS，另2次录制的验收脚本错误已旁路复核通过，旧FAIL保留且不增加Attempt。git/async/db已有选定同Tape完整Strict回放；text/SQLite新录制评分通过，但新回放实际失败；C++当时的4200秒创建请求被项目上限拒绝，Agent未启动；该记录不能证明任务在3600秒内跑不完。详见[逐次结果、八阶段与工具耗时](../tb-six-performance-20261026.md)。

release-4固定605文件，manifest SHA `d61c2a1245ded2f561843143276a177e1d45540250b10f8e922012a41e318710`；ECS五项Go门禁证据 `e3f38350de19c89c40d43a0a45e4db73c7e573e05c1133c8cfb801d3336b4ec0`。原生-3四错误等价、两套26场景及4个B并行限定检查通过，原finalize/yield限制保留；原生-1 guard选择错误、-2矩阵JS权限EACCES失败未删除。native publish-3已成功，证据 `9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1`；新6次云运行均使用此版本。

### 冻结版本与工程验证

所有编译、测试、镜像发布和真实运行均在ECS。HEAD `e686735fe0196aacfb799c8d528848dcad026d83`本身不能代表脏工作区，以下manifest才固定实际源码。

| 项目 | ECS路径 / SHA-256 | 范围 |
| --- | --- | --- |
| 该批源码 | `/opt/aries-artifacts/tb-six-release-4`；manifest `d61c2a1245ded2f561843143276a177e1d45540250b10f8e922012a41e318710` | 605文件，build/test/race/lint/integration全部PASS，分别11.164/28.086/74.527/3.783/373.923秒 |
| 该批aries / perf | `479eb181df799ae78f48a668a1c068ede6c9fbeac9d14b0eee0d5822a6bb8c92` / `5afdfeb7ac645564b1f0ff19723642a852673180accf9a3734f30b2bd99ecbe6` | 新6次运行与离线分析 |
| 该批ssh / replay | `3c56212bb3fbf6af3a81be6d8b63e837630165cf412308384c99d9688926c300` / `7daf5815a437140f72c492fedf95db990e4fd3c075269c9edb503d8c256b24eb` | 相同冻结候选 |
| 原生限定验收 | `/opt/aries-artifacts/tb-six-native-rejection-3`；`4e07884d7d66ec8778db3318f91b4f72d41371a1b4680f43fcbde70bc7bd61b5` | 四错误等价、两套26场景、4个B并行；finalize/yield限制保留 |
| 镜像发布 | `/opt/aries-artifacts/tb-six-native-rejection-publish-3`；`9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1` | SWR digest `sha256:4f519985234cc93f653314ea672bcf25c0abcc9d0009597ef82f066c08d02d2d`；临时认证已清理 |
| 旧release-2/3 | 工程证据 `11c072351251d13b49eff7fc9d13e847e5413fe8339f85b3d6aac818012d9007` / `c1c518d4fd54fcbc7e462093562750b84d8e6b254d959fb18f1592407cb69bf9` | 五项门禁原结果保留；release-2的E2B/Huawei适配器测试另通过 |
| 原生-1/-2失败 | `8d6f9e1b0f08494fe8ef781d7970e0470f5d6a0407572d026cbce2f23cafd045` / `a3dfd68f595a6463976176f2ea9d31f85653029ca2658922dd5c3df8113b4c1c` | guard选错、矩阵JS权限EACCES，失败保留；只修矩阵子进程umask022，私有父目录0700不变 |

### 实际运行与验收器纠错

原始任务固定Terminal-Bench 2.1 commit `7131e4375048a0e408a8fb404b5f499d726b695b`；原题/私测/时间预算不降级。六次新运行目录与原evidence SHA见[全部19目录清单](../tb-six-performance-20261026.md#证据位置与复核边界)。

1. `/opt/aries-artifacts/tb-six-retest-round-1`：db/text/sqlite录2实际执行；DB原900秒预算超时，text/SQLite原5/5、3/3与Tape完整，但driver字段错触发KeyError。三条随后计划的回放UNRUN，不能造attempt。
2. 原driver `d802a844df7f1755b4456d424a87cb9f2c95ee71629f58734f1b3badcf66e8a6`保留。新 `/root/aries-setup/tb-six-retest-2/driver.py` 仅status→coverage_status，SHA `5675480837a4cbb20819f3b4acd8f63c8571001ac2976bc0cf49dd96b765b262`；正例/15负例脚本 `4edf8c1f6b1d74c423e71ff568400e5c4ccdb202a7acce32026d6b37a18930e6`，实际ECS日志 `01b6d2089ed9dd743436dd2134aae0246ab86063d20968a0f2cf3253b89107aa`。
3. text/sqlite录2各新增`acceptance-review-2/review.json`，SHA `495c195c6e5ed3a7e13d134606cba6c77f4a0fa9f330db1a134b29c3a0a5e1bf` / `ace6712905f821ec098708ec36760c023148d6b8efbe37417e0a642997428edc`。先复现旧KeyError，再执行全部原验收、原runner0与NotFound核验；新Snapshot与旧完全一致，原件不变，0新Agent。复核脚本SHA `9da07656cdd84d13a15206147c3d98aad65671215ba30a336a7331352729dc54`。
4. `/opt/aries-artifacts/tb-six-retest-round-2/evidence.json`：3个新Sandbox回放真实结束，状态COMPLETE_WITH_FAILURES，SHA `c7458ec32c5e20c3781b9e30c85cb9b6decd077a151f27945f6533ec168fc611`。text/SQLite放2分别在首/第三工具后的严格请求失败；db放3完整PASS。脚本SHA `d936a97e7c415fe375bd8395c4139096cccf578d9621c12bf067e0fcdc6d1766`，不是仅离线重判。
5. `/opt/aries-artifacts/tb-six-replay-review-3/evidence.json`，SHA `9bd8bebfefdfa6ff8ea8540983aebe8d01c141e90e5d89930bfa81f80420501f`：43项原件复核，准确定位两条首次日期差异；DB11/11实际决策/结果一致、原7/7、12轮Strict完成。保留全部失败、来源SHA不变，0新增Agent/模型/云调用。

DB历史Tape SHA `180abc61b0333ff38aa5ebcf91c727210da85311dc35253552fa197c7f3801b4`；声明历史分钟mtime的准备证据 `/opt/aries-artifacts/tb-six-db-history-mtime-1` SHA `77ad3ece065b1b5d0545bbf9ed9e8566bd0c56fb7da50c79b302e733e24669e1`。实际读回1792945680000000000ns且保留atime；原秒/纳秒/时区未知。新00:00初始mtime录2依然超时，不能被历史放3成功替代；选定Tape验收不是无偏成功率。

C++ `/opt/aries-artifacts/tb-cpp-create-diagnostic-1` 独立实际4200秒请求收到HTTP400/1小时上限；原分配无ID，限定列表未发现实例不叫NotFound。这份4200秒请求保留BLOCKED_ACCOUNT_RUNTIME_LIMIT，不能推断3600秒内实际运行失败。独立3600秒实测已完成：Strict因GDB地址偏离失败，另一次Workload全程288.328秒、原6/6；Agent/Verifier各1800秒预算未改，二者共享平台存活期。后续Huawei记录与新E2B结果单列，不回填旧失败。

### 同快照双Trace与展示归档

| 新运行 | 检查数 / 区间Span / flow parent | 离线evidence SHA-256 |
| --- | --- | --- |
| db录2 | 85005 / 21239 / 387，来源incomplete | `d38e00ea0419f634b6874810fe7356d3e55e1455fb07054074c8c71d5cb75294` |
| text录2 | 3856 / 951 / 77 | `462deda01eaa4e54d838ffdda95e2c40b6a5b5306016bdd48b98cb01ac292cea` |
| sqlite录2 | 9200 / 2287 / 192 | `c67c04618f8684dc7f429d27c2e79835a9fb0ed822ddfb81135eb8fe3c18114d` |
| text放2 | 764 / 178 / 20 | `c57e6598ae6801d37f5f19faba1cb23b011c9f400c16ace278b1ea1c16ba7149` |
| sqlite放2 | 1372 / 330 / 28 | `86a090298448606e6eb8ad7d7cc0be80f7a282182c35917d1fdafd059a8a99ad` |
| db放3 | 4832 / 1195 / 60 | `c0410144ebe0ae44c91c4e2adfb673f3a2f34e300d777974cd67780a85fc9155` |

每份位于其运行目录`offline-analysis-1/evidence.json`。TP固定SHA `58042408e6cc861fb1a731c26bb082dc222285561eaa4e12a48a8b2b90dca7b9`，实际导入起止/时长转换误差0ns；不推导跨域同步精度，coverage仍unknown（DB超时incomplete）。round1/2离线汇总SHA分别 `347e93fd2d06429487ee1e6ee52ec5624d40f10629c49beb02aa6ec1e8b04aea` / `18c8ee9f27bdc9b8b7f88d84e8a9b7c241cf95d7640899085eccf0b9519268b9`。新运行Jaeger/Prometheus发送查询和UI均UNRUN。

当前安全汇总 `/opt/aries-artifacts/tb-six-summary-round-3/summary.json` SHA `e22aa1f3ba19e2f9a13e04bad4773fd71d2552253d9c8b86c3e1c3bbbdf4f737`，输入清单 `15be0f41a7d3e9a821ec0344de7bb27432610bbe1a21e86b55238400731f3732`，生成器 `2e4e27475c96e0f1056fb76b99460abfde1abdd6b2d9ee62d056f5fd92ce963c`。包含原纳秒、来源行、全部19目录和229exec；未包含私有命令/结果正文。

复核先核对上述原件SHA与版本，再按归档脚本读取，输出到新目录；已终态round服务不要直接重启覆盖。真实重测应注册新Attempt、使用已冻结配置/真实Tape、原预算与独立Verifier，确认Stop/revoke及独立删除回执。系统问题修复后须真实验证；费用不替代该要求。[可复现变体](../design/reproducible-agent-workloads.md)随后已获批准；工程、driver与真实运行的最新状态见[release7验收](#replay-variants-release7)，现有原生Strict失败继续保留。

<a id="r01-real-multicall"></a>

## R01 真实模型多 ToolCall 自主解题与严格回放

**真实 DeepSeek 同轮多工具自主修复与 Strict Replay 已通过。** 新任务 `merge-ranges-multicall-v1` 只改变题目流程说明，原坏代码、公开/隐藏测试与独立 Verifier 不变。真实模型首轮发出两条独立读取命令，随后自主修复：第一次公开测试失败、中间一次复测仍失败、最后复测成功，失败工具结果均完整保留。录制和严格回放各 7 轮/758 帧/7 次真实 exec，独立 Verifier 均 12/12；14/14 归档调用精确贯通原生→SSH→Bridge→SDK→guest，ToolResult 与候选哈希一致。4 个 Sandbox 均独立 NotFound，无补杀或清理错误。旧受控协议样本的 score 0 保持原值。

两次实际 Perfetto 导入分别 469 区间/50 flow、454 区间/43 flow，起止/时长误差 0ns；对应 OTLP 为 469/454 Span、50/43 parent，均从同一份各自 Snapshot 投影。两份离线对账分别 1924/1864 项通过；完整性 complete、全局 coverage unknown。当前运行的后端发送/UI、华为/B 同 Tape 与观测扰动未验；不外推 P0 完成或生产容量。

本轮使用真实 `aries cloud` CLI、DeepSeek 方言和原四角色 Runner，未替换 Provider 或返回受控答案。唯一新版任务说明要求首轮两个独立读取调用，再完成原测试/修复；旧 workload 和旧 Tape 均不改。后台、PTY、process 工具保留，本次实际七次 exec 均前台完成，不能据此宣称其他分支新验收。录制一次、回放一次、各 attempts=1/concurrency=1、Sandbox TTL600s；回放不传上游密钥且无上游调用。

源码清单 `ff02d056c86228ab4b5e13077f84e687200b6cf4a50cb52a7a97300df98ec009`；Runner `c22987f087e4b8afe2a2cf85084b7b80461e492cc5085ccc263cad7c663c0fe2`；分析器 `4ad358dc859abd44745d8bafef797ebc90f24cdd02d9c26e67db9570c7fb7a23`。ECS 五门禁 build/test/test-race/lint/integration 均通过，不能将后续 TerminalBench 接线借用此门禁。Driver `9ae2c39e9030b46f0ff573db9eb9e98236903bc2f1a657c09619a746860a62f0`。Adapter 从冻结 manifest 复制可读公共源码，未修改原发布源码或现有服务。

| ECS 证据（相对 `/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-r01-real-multicall-release-1/evidence.json` | `f718df91636cb7c8d3414af3a42bee617ad27322e6f61d6dc866f68de869db1b` |
| `observability-r01-real-multicall-1/evidence.json` | `7371ea0bded6d3bef0f1e6b5d22df694c69c6825cee9bd35be88ecbc81002187` |
| `observability-r01-real-multicall-1/summary.json` | `21a02ce3f1b8dd3884881d133670e645e3967377e6ba5f4bb5ba1fc3906d2956` |
| `observability-r01-real-multicall-1/offline-analysis/record/evidence.json` | `c68f484c3de739410038ec509728c07b3cb44d7a747e1e44993e1a1d97b9bb74` |
| `observability-r01-real-multicall-1/offline-analysis/record/analysis-snapshot.json` | `9512f9505dfd6e4aa7d22f470f7b755e12a239ea33329219e416b70b1c6d0214` |
| `observability-r01-real-multicall-1/offline-analysis/record/trace.json` | `46a378ab5c678448dad3818193f0a54a2d177988fd49d60b53b70bd046dab21b` |
| `observability-r01-real-multicall-1/offline-analysis/record/otlp-export.json` | `2357d6056a48b881625240b07c253d3d596dda9b1e2f377ec99d8fd85dc8eb26` |
| `observability-r01-real-multicall-1/offline-analysis/replay/evidence.json` | `736254fff487b89d48ceb12ce37e2b270377b62ebce340cea4b6fb3638120c75` |
| `observability-r01-real-multicall-1/offline-analysis/replay/analysis-snapshot.json` | `8ab234689d61ba577d481238865f5f35e7d9a7437e5b662f0736eb2df8071ea7` |
| `observability-r01-real-multicall-1/offline-analysis/replay/trace.json` | `438c950784d479f49ae7a0cb55092867961acb528e5ce36ccf1c1d2374b8451f` |
| `observability-r01-real-multicall-1/offline-analysis/replay/otlp-export.json` | `6bdf1d10ce07ded213d19c99302c0f141c53d449cc850f32f320c179ef0b0ed8` |

新 Tape SHA `e28fdf63abf6a06cb2ed5fc34ba9bcd25605dc3ffb57da82b93af3100f3201ca`；候选 SHA `9381739882721e9f8d8a17797975ffcd42a6e654f006ebadc34bfbef42f5dda2`。录制实际 RunID `20261025T155504.047371245Z-openclaw-e2b-external-r01-record`，回放 `20261025T155553.826553322Z-openclaw-e2b-external-r01-replay`。两次模型均7轮/758帧，Strict整历史和结果顺序不放宽，录制时序与实际发送时序保留原语义。SDK前两调用等待重叠分别 `302559574` / `255771626` ns，仅证明控制器在途重叠，guest并行时长仍unknown。

复现：ECS `/root/aries-setup/ecs-r01-real-multicall-1.py --pins PINS --input PRIVATE_SETTINGS --output NEW_EXPERIMENT`；pins 对应上表精确发布，须使用新实验目录。离线 `/root/aries-setup/ecs-r01-real-multicall-analyze.py --run RUN --binary BUILT_ARIES_PERF --output NEW_ANALYSIS`，不访问模型/云、不改原件。临时凭据不得放进分享包。完整阶段与每次工具原值见[性能表](../performance-archive.md#r01-real-multicall)。

<a id="r01-multicall-app"></a>

## R01 应用调度、真实 Attempt 与同源双 Trace

**限定PASS；受控模型协议样本，不是自主解题成功。** 原`81f1af39…` Tape在真实`app.RunCloud`→原`runProfile`→原四角色Runner重新执行：1 planned/admitted/started/executed/completed，1 failed/verification_failed、0 verified_succeeded，原独立Verifier12项中11失败、score0。模型3轮/9帧、3次真实exec无偏离；拒绝把RunCloud的nil返回、模型成功或工具exit0当任务成功。原直接Runner记录不补造Attempt，新增应用记录有独立RunID。

唯一实验接线是将真实`cloudwiring.Prepare`返回的`NewAttempt`入参Provider从固定DeepSeek恢复为原录制的OpenAI方言；没有更改Strict matcher、Prompt、Sandbox Tool、Runner、停止门、评分或清理。属于应用API验收，不是原CLI兼容性宣称。镜像引用来自原发布证据`29f377da…`，未拼装/猜测digest；任务为merge-ranges，真实occurrence=merge-ranges-001、L0，预登记1次replay、并发1、最多2创建、TTL240秒、task45秒/verifier30秒、0上游LLM。

1010项检查通过：Tape原决策独立分母3，实际native决策/结果及producer→SSH→Bridge→SDK process→guest receipt均3/3精确对应；整个ToolResult内容与原录制相同。同一应用clock内的SDK wait重叠2.315237332秒，仅证明在途等待重叠。真实八阶段/失败账本保持原值；Snapshot仅构建一次，Perfetto与OTLP都从它生成。221区间/27 flow的真实TraceProcessor导入起点、终点、时长误差0ns；OTLP221 Span/27 parent、0 Link，无伪造跨域epoch。Snapshot complete/coverage unknown，不宣称全局零漏采。2个Sandbox均独立NotFound，无补杀，原服务不变、临时资源及凭据副本已清理。

同一581文件源码manifest `821eefe81a45f89db206f8eb15d216933141e1fe61666c48643733ac70c3115d`在ECS独立副本完成build/test/test-race/lint/integration五门禁，使用固定TB2.1 fixture，原源码/fixture与Docker既有资源均未变；0云/LLM。完整构建的Runner `6d05bc493b7f3d9f26752ebaeec9b763b7898422004de72c9ef5a6d55df027fc`、Replay `c0b7303881d2c3c49a2e46a6380776547ad4cdd7f9cc3874c20e90c1678a6c30`、分析器 `a64695de41b4c139c2c12c24d7a3b7326380e539e1f9db999f96ae02438b9800`；不同编译目录的二进制另存，不覆盖旧构建证据。后续未验候选不借本门禁宣称通过。

| ECS证据（相对`/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-r01-multicall-release-1/evidence.json` | `70686b1306710a2121611596308d50c81d0cda897a7687cab06182743a813369` |
| `observability-r01-multicall-app-1/analysis/analysis-snapshot.json` | `43ddbbfe82787dd4c8e69ec26f3f96961912bacc79549c48a8723e77ebdf479b` |
| `observability-r01-multicall-app-1/analysis/evidence.json` | `7f32da7a6c9acdcc826c454d59cd2ba4ff9d7b2af9ade0e9706704cb4351503f` |
| `observability-r01-multicall-app-1/analysis/otlp-export.json` | `7d889235b0c69f19cfe8c558a86efc95abca9dff1f121e5f6abd7c74167a11c6` |
| `observability-r01-multicall-app-1/analysis/trace.json` | `5af3f60cd0ae62388887c68961886589c30cd623f136390ab38b894ed5434cc2` |
| `observability-r01-multicall-app-1/app-receipt.json` | `30d48f23166949f3183bf1ed21bc73cc131f458093223f09746253323f97ede5` |
| `observability-r01-multicall-app-1/evidence.json` | `d0f60f88703c80afe9cd4fd1e59c8d3c39fb76db8b668ef6bc31c2777d62e69b` |
| `observability-r01-multicall-app-1/prepared.json` | `bacafd849b3da137170bfe91b910a87a2876eaa47f3e5d7bcd80687df90542c0` |
| `observability-r01-multicall-app-1/provider-cleanup.json` | `e3ffdf7087148e8da2bd6a874e438839bae20cd77c0d6cfed33d618b28d170a1` |
| 实际运行`20261025T151940.634592649Z-openclaw-e2b-external-r01-multicall-app/run-result.json` | `fe7dc5fc1339a6be12f693e25f1dc3d213417c1092464126e4ba03393e4516fa` |

复现：`/root/aries-setup/uploads/r01-multicall-app-1/driver.py --output NEW --inputs INPUTS`先准备/gofmt/编译，`--output NEW --execute-paid`执行一次并回收；新目录必须不存在。`analyze.py --root NEW --binary BUILT_ARIES_PERF`在新的analysis目录核对原件并导出，不访问云或模型。原运行/原分析文件不覆盖，付费执行需按当次预登记范围运行。分阶段原值见[性能表](../performance-archive.md#r01-multicall-app)。

本次Jaeger/Prometheus后端重发、UI、当前版本观测扰动、华为/B多调用及真实模型自主修复成功均UNRUN。后者已列为下一项必要验收，不能用原协议score0替代。R05文件I/O支线仍暂停；完整生产与全局coverage结论不升级。

<a id="r01-multicall"></a>

## R01：原生同轮多调用 → Tape v2 录制与严格回放

**E2B A受控协议范围PASS，P0仍PARTIAL。** 固定OpenClaw 2026.7.1的实际bundle支持同轮多个ToolCall，`proxy-BzhBz8iM.js:429`选择顺序/并行路径，`:517`用Promise.all保留原调用顺序，`:862`默认parallel；`sessions-DrsnOdf0.js:12395`附近创建Agent而不覆盖该默认。源码直接从既有OCI归档提取，未联网替换版本：image index `6a31d44b…`、layer `2d1d5819…`；实际A候选镜像仍为 `cc7b2c82…`，未改原生调度或工具语义。

生产增量仅7个Go文件：`internal/modelreplay/stream.go`按索引聚合多调用，`tape.go`提供v2数组与v1兼容、完整调用/结果因果校验；`server.go/boundaries.go`将所有索引纳入首工具delta及敏感内容检查；3个测试文件覆盖新的正负例。SSE原帧、recorded/zero/synthetic与缓存retry语义不改，Strict仍逐轮比较完整JSON历史与数组顺序。候选在ECS对modelreplay/app/replay CLI的unit/race各309个passed test事件（含子测试）、vet及两个CLI编译通过；该定向记录本身不是完整五门禁。同源码后续五门禁及应用接入见上节，原件不回填。

**真实执行**：`observability-r01-native-multicall-4`直接使用原四角色Runner、固定A镜像、SSH client、E2B SDK与模板。预登记record/replay各1次，单Agent并发1、至多4次Sandbox创建、TTL240秒、0上游LLM。受控模型第一轮给出2条相同exec，第二轮再给1条同命令的新调用；不提供修复代码或参考解。录制与回放各3轮/9帧/3真实工具，Tape v2第二次在真实新Sandbox严格完成。125项离线检查通过，6条模型→原生producer→SSH request→Bridge execution→SDK process→guest receipt全为精确ID映射；模型输出分母事先固定，不从producer条数反推。

两次同轮SDK `exec.wait`区间分别重叠1.640077205秒、2.260018555秒；这些是同控制器时钟内的在途等待重叠，**不是guest同时执行时长**。真实ToolResult正文、退出码、字节数逐项对账；原生调度、IO、后台/PTY开关均未改。停止/撤权完成后额外确认cgroup空、UID60000进程0及listener关闭，再运行原独立Verifier：未修复候选各12项测试中11失败、0error/skip、score0。验证失败原样保留，不计成功任务。4个Sandbox均独立NotFound且无需补杀，临时Adapter、网络、模型监听、凭据副本已清理，既有服务不变。

原真实v1 Tape `d716431e…`另在新代码下按原字节执行7轮/735帧本地HTTP边界回放，Load及session完成通过，原件未改；这不是重新运行其Sandbox工作负载。首次准备引用分析副本中不存在的SSH binary而失败；随后Adapter就绪失败（源目录700、日志为空，独立只读代码暂存后进入下一阶段），再因夹具未先创建performance父目录而失败；三份原FAIL均保留，均0创建/0实际Runner调用。第三份旧driver预填`runner_invocations=2`属于计数错误，不能当执行证据；后续改为实际Runner入口文件计数，本轮实际2次。

| ECS证据（相对`/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-r01-dispatch-source-3/evidence.json` | `6e3f2409de274cc9461fb2d2bb693f0120f14736f64f092c957cb99d58d60047` |
| 同目录 `source/app/dist/proxy-BzhBz8iM.js` | `e206d9858cf8071ba0986cfff201cf9ddd720d7156b337cf8874fc04058ac73e` |
| `observability-r01-multicall-1/evidence.json`（工程） | `470308d27fcb7756cbb979caa78a1a35f33487ea7b13a632e6a112e4d5905572` |
| 同目录 `source-manifest.json` / `aries` / `aries-model-replay` | manifest `821eefe81a45f89db206f8eb15d216933141e1fe61666c48643733ac70c3115d`；Runner `59a7d4f45f3eef04d6bdb6f02abe693e3e30d18c3c0716fece281eb5521ec3ea`；Replay `2a62394f2c9022717465dab40475a6a1eb3e780372a511a9f68881884178669c` |
| `observability-r01-native-multicall-4/prepared.json` | `fa6b922f6311b142eb9e664ba311e0fa9c5d34919bbe045ba9714bbb96107378` |
| 同目录 `evidence.json` | `6d922062ee47dea9a81c10d8e9e855f81ec930211a0c6c007e67cceb9862361c` |
| 同目录 `identity-review.json` | `ffe5c97fd252a7a7d98f32f4c8d9aa2e3fe293ff2bb46e4048c06222a3f37452` |
| 同目录 `provider-cleanup.json` | `6cf6a5bbd9e53d05fbbb70a24f11702db23b88ca40e24012c10fb10c76ef8623` |
| 同目录 `cases/record/model/tape.json`（私有原件） | `81f1af392d8c96043dda72d0234a32005cda3dd8f4916425211ecd66973377f0` |
| 同目录 `legacy-v1.log` | `c49837d18c29533eacbece9ff4f05e31426655fdf2be18e237c5acff4296f5c8` |
| `observability-r01-native-multicall-1/prepare-failure.json` | `61890d99fc6963a8deecbf74309068122e55406aa4e6a4225c249bc51384b8df` |
| `observability-r01-native-multicall-2/evidence.json`（就绪原FAIL） | `1732b3fd4f26d9fa63f6b8b3fc232fb5180edcfe1959cd97f7707c87211bb37d` |
| `observability-r01-native-multicall-3/evidence.json`（目录原FAIL） | `60cd1a11a7e9b81d9af1ab3bb32d8a5bb94f1eea30e558f2802a29c9910306d8` |

复现入口：ECS `/root/aries-setup/uploads/r01-native-multicall-4/driver.py`；对新绝对目录先`--output NEW --inputs INPUTS`准备，再`--output NEW --execute-paid`，原目录禁止重用。离线`review.py ROOT`只读原件、按完整ID复核，不重新创建资源。性能原值见[独立阶段表](../performance-archive.md#r01-multicall)。

**仍UNRUN/unknown**：此为受控模型的真实协议验收，不是自主Agent解题成功率；同命令新调用不等于失败后的原生自动重试。直接Runner没有app Attempt/排队账本，未伪造Attempt给Snapshot，也未宣称本次新轨迹完整Perfetto/OTLP/UI已验。华为/B的多调用Tape、重型异步工具/后台/PTY多调用、真实LLM同轮多调用与新代码观测扰动尚需各自证据；旧限定PASS仍封存。R05有界文件IO保持BLOCKED并暂停，R01全局producer覆盖仍unknown。

<a id="p1c-resource-prometheus"></a>

## P1c：真实资源原件 → 同一 Snapshot → Prometheus 查询

**已声明资源流的离线链路限定PASS；R08和P0整体未完成。** 重读四格L1与A并发/B首输出/B取消共7组既有运行，不新增云任务。parser `aries-analysis-v1.5.0-resource-streams`保留独立owner、scope、原值、来源SHA与真实时间依据。所有原业务事件、区间、边、时钟、阶段和Attempt账本与P1b解析结果一致；3份旧冻结Snapshot的原ID/hash和Trace/OTLP/账本Metrics共9项投影全文不变。新资源快照另获内容ID，不重签旧快照。

| 既有运行 | 全部资源流 | 实际导出流 / 缺测流 | 去重后Gauge点 | 观测来源 |
| --- | ---: | ---: | ---: | --- |
| 四格E2B A | 4 | 4 / 0 | 172 | controller 16、Harness 93、任务Sandbox 56、评分Sandbox 7 |
| 四格E2B B | 3 | 2 / 1 | 58 | controller 16、任务Sandbox 42；评分无读数 |
| 四格华为A | 4 | 2 / 2 | 106 | controller 16、Harness 90；两条provider流时间未证 |
| 四格华为B | 3 | 1 / 2 | 16 | controller；两条provider流时间未证 |
| A两任务并发 | 1 | 1 / 0 | 16 | 共享controller，不能拆成两个任务消耗 |
| B首输出 | 3 | 2 / 1 | 72 | controller 16、任务Sandbox 56；评分无读数 |
| B CLI取消 | 1 | 1 / 0 | 16 | controller；取消和未启动账本不变 |
| 合计 | 19 | 13 / 6 | 456 | 不将异步owner求和成全栈资源消耗 |

同固定Prometheus3.15.1镜像、OTLP receiver与单位翻译，**主验收real-results-2**为13个独立TSDB分别首发、重复发送后按原时间查询：26次查询、2108项检查PASS；每轮合计90条owner内序列/456个历史点，数值误差0。时间差为−998796～0ns，来自毫秒截断，原ns仍留在Snapshot/manifest。重复不增加历史点，也不证明分布式exactly-once。6条NOT_COLLECTED不发送、不启动后端；HTTP200只记accepted_unverified，PASS来自实际查询。

E2B返回的历史读数按同owner/指标/timestamp和精确数值合并，保留所有sample IDs；不同时间的相同值不合并。四格E2B A任务36条reading→8组时间点、B的27条→6组，不能将RPC轮询次数当独立采样数。controller用自身UTC锚点加采样完成offset，保留完整采样bracket和原delta；累计读数以`*_observed` Gauge表达，不造Counter进程起点。RSS是进程生命周期峰值。华为原`ts`单位未证，4条provider流只保留原件，不以请求时间代替采样时间。IO/Network、每Tool资源、短任务峰值、跨主机时间校准仍未采或未验。

候选2在ECS对performance/aries-perf的unit/race各300个passed test事件（含子测试）、vet/build通过；本地8个增量Go文件逐SHA等于ECS格式化源码。首版单测夹具错误沿用一基sequence，真实A监控零基数据揭示解析错误；修正解析与夹具并补重复序号拒绝后，7组真实原件通过。旧FAIL保留，不改原件或放宽计数。运行时/Agent镜像/Strict/Verifier/隔离门未改，此为分析器定向验收，不是新运行时五门禁。

| ECS证据（相对`/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-p1c-export-1/projection-evidence.json`（零基序号原FAIL） | `74c9e5312f10ecdb4f57d0eae9f1fea8ee54c89ad10ec71bbb8fff15ed2847d4` |
| `observability-p1c-export-2/evidence.json` | `2ee63b2270f77d36af14512c82a4d7194f1aa59fe5641291de992419297eced9` |
| 同目录`source-manifest.json` / `aries-perf` | manifest `7af0e601ddeede1b0147732edd7f2fac9fa0eb02fde1e40ae358903e465ab3c5`；binary `a4a4c0f6a74d1ac52cc09c1fcf2332098045143d6cf864d075cdf96302671328` |
| 同目录`projection-evidence.json` | `6bd8164ecf4778bf121eaad7fdd0bcde2fc9c632a38313d2655bf75dc41b2d91` |
| `observability-p1c-resource-prometheus-1/real-results-1/evidence.json`（端口预检原FAIL） | `58fff6a6322927c9e6e0e58a1012177bc4faef232dc510669414ee1c0f264eb5` |
| 同backend目录`real-results-2/evidence.json`（主验收） | `3668a1523adb36b9866d23ece9912174ce7b3413f91fa41b39294e754bf8949e` |
| 同backend目录`real-results-3/evidence.json`（脚本简化后补充） | `aacb1be7a8fd91e0fadaa090f4a631f719c3dbdda72d8e8af372bb1f591631e3` |
| 同backend目录`references/manifest.json`（固定官方translator与E2B SDK） | `ce47cfc11c9bf3b30a0e2253151e8906dfe29dfec2e18abaf42c72fed20ba19a` |
| 同backend目录`summary.json`（本地`.cache/ecs-p1c-summary.json`） | `5e076fd00cec7cb4da80572b0b7d3c06ad52023526b98cc1824e561c31184f8d` |
| 同backend目录`final-cleanup.json` | `469b780d9701def8ea1bb813fa455fa527dd8653650d6532648f00e44f0e7e71` |

后端首轮第1流查询通过，后12流因冗余bind预检将TIME_WAIT误判占用而未启动；FAIL完整保留。control2允许TIME_WAIT后主验收通过；随后control3删除冗余bind预检，保留Docker真实绑定、owner/readiness/cleanup检查，已发生的13流补充复验也归档，不增加性能样本、不替换主验收。实际创建/删除27个临时Prometheus容器（首轮1+主验收13+补充13），39次cleanup脚本含12次未创建检查，不能混称39个容器。端口仅ECS loopback19095，TSDB/WAL及配置保留，0新Sandbox/0模型调用，既有服务不变。

复现：使用候选2的`exports/<fixture>/analysis-snapshot.json`及`exports/resource-fixtures.json`，运行`aries-perf --resource-metrics STREAM_ID --from-snapshot FILE`；每个owner使用新的隔离TSDB，调用`--metrics-send`后运行归档`query.py`查原绝对时间。主验收实际使用control2/validate2，简化后的control3/validate3只对应补充记录，禁止混标来源。剩余退出条件：华为时间字段证据、IO/Network及各组件独立归属、provider reset/短峰值与采集成本；真实Goodput须显式SLO和资格证明，生产实时/并发批次/认证/耐久及P2仍PLANNED/UNRUN。R05保持BLOCKED并暂停，成功任务总数仍78。

<a id="p1b-offline-prometheus"></a>

## P1b：冻结 Snapshot → Metrics → Prometheus 实际查询

**离线账本和阶段指标限定PASS；资源指标、实时出口和P0整体未完成。** `ProjectMetrics`只读取校验后的Snapshot，原AttemptSummary计数、吞吐和历史窗口不重算；八阶段直方图按phase、Attempt outcome和scope聚合，manifest保留每个成员的原始ns及来源。不存在的Goodput和资源样本不补零。ECS定向unit/race各274个passed test事件（含子测试）、vet/build通过；6个增量Go文件的格式化源码与本地逐SHA一致，不是新运行时五项门禁。两种Trace的原投影全等、原快照和成功任务数未变。

| 原冻结样本 | Metrics / OTLP点 | 阶段组 / 实际成员 | 缺测阶段 | 首发 / 重发后历史样本 | Prometheus时间减原ns |
| --- | ---: | ---: | ---: | ---: | ---: |
| E2B A两任务并发 | 6 / 29 | 8 / 16 | 0 | 117 / 117 | −727789 ns |
| E2B B首输出 | 6 / 29 | 8 / 8 | 0 | 117 / 117 | −108332 ns |
| E2B B原CLI取消 | 6 / 29 | 8 / 8 | 8 | 117 / 117 | −992207 ns |

每次查询的117条业务序列逐项对账：Counter、原窗口吞吐Gauge、原窗口长度、Histogram count/sum/累加le桶值的实际误差均0，共6次查询/2130项检查。时间按Prometheus毫秒精度截断，原OTLP与manifest的ns保持不变。取消样本仍planned=2、canceled=1、not_started=1、成功0；未启动的8阶段不出Histogram。活动任务的verifier阶段124309ns只是已取消的Evaluate入口耗时，不证明执行了评分。三个样本没有配置Goodput，也没有Snapshot资源样本，故这些指标的真实后端验证仍UNRUN；不得用协议Gauge夹具补作资源监测证明。

固定`quay.io/prometheus/prometheus:v3.15.1`，digest `sha256:fa3302279710be2f21a8301ec17a8b4f2301fd829bd113839b3b6b02b6f191a2`；开启`--web.enable-otlp-receiver`，入口`/api/v1/otlp/v1/metrics`。每份快照独立TSDB、无scrape；固定`UnderscoreEscapingWithSuffixes`、classic Histogram、关闭scope metadata提升，乱序窗口24h仅作实验配置。JSON receiver支持由固定版本官方源码与真实HTTP确认。6份发送回执均为HTTP200空body、`accepted_unverified`，后端查询才确立验收；不把发送成功当作计数正确。

另一个独立TSDB中的**合成协议夹具**验证历史点、完全重复、24h内乱序、同标签同时间不同值冲突、累计epoch reset、窗口外拒绝，223项复核通过。历史、重复、窗内乱序及reset返回200；冲突和窗口外返回400，既有值保留。Counter reset实际查询为1。首次检查器误将unit=1 Gauge映射为无后缀名称，留下FAIL；依据官方translator的`_ratio`规则修正期望后，只重读原HTTP/query原件，未重发或改负载。这不是改业务验收门槛。单末点不提供`rate()`趋势；完全相同重复不增样本也不等于分布式exactly-once，多批次同标签不可直接混写。

| ECS证据（相对`/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-p1b-export-1/evidence.json` | `5318dc2ac687db4de70ef8f971a42bf4cf72582facd91bcf6454faa5a5a8672f` |
| 同目录 `source-manifest.json` / `aries-perf` | manifest `e4918c91deaf98c774b60c95d9d9fcfda6866f4476501982639b2dd075a3e7fe`；binary `6722ecf6ca1fb970e058f12f01349d918faa41bdce01a8b71193661138014276` |
| 同目录 `projection-evidence.json` | `4b8c77d0631e14e3f9ba1820ab2e4f004ee0a5057f3a2334af1ccc68abb8ef14` |
| `observability-p1b-prometheus-backend-1/real-results-1/evidence.json` | `386710c8dddb69eec0efbe712f338c1e4b0dacfcce82f0d70f719f7b54b5f1e7` |
| 同backend目录 `protocol-1/protocol-evidence.json`（原检查器FAIL） | `4ea19145e79cd3efb994f0bd218ab521fd18a4f7ce2699f0d27cabfb1acb9962` |
| 同backend目录 `protocol-1/protocol-review-2.json`（原件复核PASS） | `e2c5143acf8cd34c18f86eb5d0c1bfe35180cb278d867bd30701cfd9c7bf8635` |
| 同backend目录 `a-1/cleanup.json` | `953773f649747e3ab7a8244ff1168c362a1aa0ffe986654067e20bed07f927e5` |
| 同backend目录 `b-1/cleanup.json` | `9fd6a822bdfdf24b809d33018e2f8b8171e41db4f783489ad7c9b61b636630f9` |
| 同backend目录 `cancel-1/cleanup.json` | `912daeae40ffd1821128cbefeed6c5229516c9cfcec0378e98740ff091f7b2bf` |
| 同backend目录 `protocol-1/cleanup.json` | `3c8e6a32057f7a2d7530e7dbf4163cfaf5bb5e8db46c1341f2903cbb2e985954` |

复现先使用冻结`exports/{a,b,cancel}/analysis-snapshot.json`运行`aries-perf --metrics --from-snapshot FILE`；原件SHA见同目录projection证据。独立空TSDB就绪后执行`--metrics-send EXPORT http://127.0.0.1:PORT/api/v1/otlp/v1/metrics`，用归档`query.py --export EXPORT --url BASE --out NEW_DIR --label LABEL`按原绝对时间检查，再人工重复同payload并再次查值/样本数。backend目录保留control/query/validate/protocol脚本、完整响应、配置、TSDB/WAL和固定镜像；重新执行必须使用新目录，不覆盖旧结果。局部文件SHA与全部回执另汇总于本地`.cache/ecs-p1b-summary.json`，SHA `025d0ceccbe411310ff84144692874b1d1640cf1e87d924ddd97f5197d3ce746`。

本次0新Sandbox/0模型调用，4个自有Prometheus容器均已删除、19091–19094无监听，既有服务未动。后续资源Gauge限定验收见[P1c](#p1c-resource-prometheus)；剩余退出条件：Goodput需有显式SLO与独立资格证据；生产并发批次、认证、耐久/重启、UI及实时出口仍PLANNED/UNRUN。原Validated/Goodput口径不因导出而升级为已确认网络隔离或全链覆盖；P2仍先核实权限，R05支线维持BLOCKED并暂停。


<a id="p1a-offline-jaeger"></a>

## P1a：冻结 Snapshot → OTLP → Jaeger 实际查询

**限定PASS，未宣称P0/P1整体完成。** 本次只使用既有A并发、B首输出及B取消的三份冻结Snapshot，没有新建Sandbox或调用模型；四角色、Strict、真实工具、Verifier及停止门不变。候选2新增离线载入/投影/发送能力，在ECS执行`pkg/performance`与`cmd/aries-perf`的unit/race各254个passed test事件（含子测试），vet及分析器build通过，不称完整运行时五项门禁。parser仍为`aries-analysis-v1.4.0-native-first-output`，exporter为`aries-otlp-http-json-v1`。

| 冻结真实样本 | Snapshot区间 | OTLP Span / parent / source | 无锚区间排除 | 首次 / 重发后存储行 | 唯一Span ID |
| --- | ---: | --- | ---: | --- | ---: |
| A两任务并发 | 845 | 845 / 86 / 38 | 0 | 845 / 1690 | 845 |
| E2B B首输出 | 802 | 795 / 31 / 12 | 7 | 795 / 1590 | 795 |
| B原CLI取消 | 787 | 784 / 18 / 12 | 3 | 784 / 1568 | 784 |

三者原Snapshot ID/source SHA不变，identity/integrity校验通过、coverage仍unknown；原Perfetto JSON投影全等、同Snapshot重复OTLP导出字节相同。Jaeger v3查询逐项对账Span身份、属性、parent、scope、来源及时间，start/end/duration误差均0ns；legacy微秒接口的起点/时长误差绝对值最大999ns，推导终点最大1983ns，不能称legacy纳秒无损。取消仍为原失败/未启动分母，不因观测complete而变成功。三份真实样本共2424个唯一Span、Links均0；重发实际产生重复存储行，稳定ID不保证exactly-once，也不新增成功任务。

另以显式`synthetic_protocol_fixture`、`real_execution_evidence=false`验证Link协议：4个Span/2个Trace、1个parent、同trace与跨trace Link各1，55项对账通过；v3时间误差0ns，legacy FOLLOWS_FROM精确。它证明后端协议能力，不属于真实运行/任务分母，也不证明真实业务跨域因果关联。

固定后端`cr.jaegertracing.io/jaegertracing/jaeger:2.22.0`，镜像digest `sha256:836b967b5abcf1f7758066aa6b348619267a2f1459f72d472840570f9bd88a17`，memory存储，`extensions.jaeger_query.max_clock_skew_adjust=0s`；仅ECS loopback端口14318/16687。真实样本首次/重复6次及协议夹具1次发送均只返回`accepted_unverified`，通过由独立query确认。partial_success/拒收/警告/无效响应/超时已在受控HTTP测试覆盖；不声称实际Jaeger发生过这些故障。发送器一次30s请求、无自动重试，超时为delivery_unknown。实验后容器已删除、两个监听端口不存在。

| ECS证据（相对`/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-p1a-export-2/evidence.json` | `68a246e9e41ca0dcd8cde0153474b24f8bbb332e0f5cd874df9399f1f50e2dd3` |
| 同目录 `source-manifest.json` / `aries-perf` | manifest `5915320cc91b0c2b1c83b7ca981551efbf009e634cdbf857f4abf7910213ccf7`；binary `6240fd06bbdca4957427d2c8b35beea796aebf83f721a4c7d3c25e60e7ec162e` |
| 同目录 `projection-evidence.json` | `76bdfa6d12729a6eaedd1525d33ff217e5aa1e1da2f578be05ca5f6af6bfaf79` |
| `observability-p1a-jaeger-backend-1/results-1/evidence.json` | `3d5072a8dd6131a45c70835d3155b5357d9168e659356b526f0b8bf09fc648b0` |
| 同backend目录 `synthetic-links-1/evidence.json` | `f452055154e968b24ad9aedb16d0c9486f1fd99d7915bf0043f10756e9ebda0a` |
| 同backend目录 `cleanup.json` | `785513f97d2072545ff5523b33815e8579db3a466bca7c52dcc45f64b17ab405` |

本地汇总`.cache/ecs-p1a-backend-summary.json` SHA `d83a1d1244d6e987643df9275cd2febd5525ce21e316189ab187f40c0e91146b`保留backend配置、每次发送/查询、协议夹具与清理证据。候选1的真实A载入FAIL仍在`observability-p1a-export-1`：`sdk_calls_duration_only`原为`[]AnalysisSDKCall`，读回泛型map后字段序改变，原内容哈希不再相同。候选2仅严格恢复该已知类型再验证原ID；未重签Snapshot、改raw或松校验。后端最初用legacy services路径得到404的readiness失败也保留，实际`/api/v3/services`通过后才执行发送。

复现只在ECS进行：工程驱动`.cache/p1a-export-2/driver.py`及投影驱动`project.py`绑定已验基线和源码SHA，固定输出目录排他创建，不可直接覆盖本次归档。日常导出可使用已验二进制和既有冻结文件，在新派生目录执行：

```sh
perf_bin=/opt/aries-artifacts/observability-p1a-export-2/aries-perf
snapshot=/opt/aries-artifacts/observability-p1a-export-2/exports/a/analysis-snapshot.json
"$perf_bin" --otlp --from-snapshot "$snapshot" > otlp-export.json
"$perf_bin" --trace --from-snapshot "$snapshot" > trace.json
"$perf_bin" --trace-clock '<snapshot中的clock_domain>' --from-snapshot "$snapshot" > trace-domain.json
# 仅向独立准备的接收器发送；本次临时后端已清理。
"$perf_bin" --otlp-send otlp-export.json http://127.0.0.1:14318/v1/traces > delivery.json
```

发送回执不代替查询；使用归档独立query脚本核对export中的trace/span ID、属性和原时间窗口。无锚原生区间、instant/SSE事件与duration-only数据仍留在Snapshot，不造绝对Span。UI渲染、大批量、认证、持久存储/生产耐久和实时导出未验；P1b账本/阶段指标见[另项验收](#p1b-offline-prometheus)，资源Gauge、真实Goodput与P2仍PLANNED/UNRUN。已通过离线范围封存，以上边界不外推为生产全量能力。


<a id="r04-cli-cancel"></a>

## R04：原 B CLI 取消、未启动分母与双域 Trace

原`aries cloud`一次运行，planned=2、concurrency=1、L0、最多2个Sandbox/TTL600、0上游LLM。实时读取第一任务的`model/request-boundaries.jsonl`，首条`request_matched/turn=1/succeeded`（非cached retry）触发对原CLI PID的一次SIGINT，原行与身份/SHA独立保存。该点已完成首个真实工具，并严格匹配含其结果的下一轮请求；不声称取消发生在工具执行中。

固定568文件`observability-r07-native-release-1` manifest `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`、E2B SDK2.58.0、B schema2模板/build `bd59dosz98xd33glqt1q:6d193918-762d-4ce2-a7d4-6513cc3875f8`、原B Tape `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb`；分析器仍为`aries-analysis-v1.4.0-native-first-output`，binary SHA `86de3551cdae736f0a3154a2e38dd09a18dc2eb614678de9ff27ee10297e8d25`。四角色、原工具策略、停止/评分逻辑及产品源码未改，未新建运行时或重复构建。

| ECS归档（相对`/opt/aries-artifacts/`） | 实际结果与SHA-256 |
| --- | --- |
| `observability-r04-cli-cancel-1/preregistration.json` | 运行前固定两计划/单并发/一次SIGINT；`07fbc50ace99f60347813e49e30d162ccc3bfa106a92e4e8c19d1aaf58158036` |
| 同目录 `evidence.json` | PASS_EXPECTED_CLI_CANCEL_AND_CLEANUP_ONLY，原CLI exit=1保留；`107f9d0975dfebadc217752032bb38a037b83720b5aeffb929dcf351d6118829` |
| 同目录 `trigger.json` | 原Strict匹配行、request/Attempt/session/PID及单SIGINT；`097e066d880475a46adb31e4253cd55a918509fa84520e6897683f1635519946` |
| 同目录 `reconciliation.json` | 61项独立对账PASS；`40d77d4bd4cefbe0a25ab88c4275abe95e2bb63558d1c0e8a6b5f111582191a7` |
| `observability-r04-cli-cancel-perfetto-1/evidence.json` | 双域实际TP通过，业务结局/原件/单位精度对账；`692218816456403e825327628f90a4069fc41c2e720987d0928fc77adcc28d59` |
| 同目录 `exports/run/analysis-snapshot.json` | complete/unknown，无诊断，两真实Attempt均保留；`640939d6f3e59b94926eb82460ba88ba5743455729a99dbe2975ce79e4bcf67a` |

真实run为`20261025T112714.650437350Z-openclaw-e2b-embedded-r04-cli-cancel`。planned=2，admitted/started/executed=1，canceled=1、not_started=1，completed/verified_succeeded/succeeded=0。第二任务没有admission、模型session、Sandbox或运行阶段观测，八阶段null保持；原模型匹配2轮、完成1轮、发送48帧后incomplete，应用model session为failed，没有REPLAY_DIVERGED或上游请求。首工具3条consumer事件与旧B录制稳定字段一致，10条原生callback精确属于该ToolCall/run/session；B未采独立native transcript，模型可见结果以原Strict第二轮完整请求匹配为证，不声称整ToolResult离线字节比较。

原Stop/revoke成功且先于Evaluate。Evaluate status=canceled、Verifier=not_run，124309ns只代表已取消的评分入口调用，不是验证运行耗时；本例没有成功验证，不增加成功任务分子。只创建1个Sandbox，原清理成功后独立provider NotFound，未补杀；临时凭据已删除，源码、Tape和生产服务未变。性能分项见[本次归档](../performance-archive.md#r04-cli-cancel)。

本次观测原件确实完整：native index complete/terminal_observed、controller与model request-boundaries writer分别89/69条且记录drop=0；这与业务取消并不矛盾。实际TP对同一Snapshot导出控制器784区间/18flow、独立native域3区间/0flow，12来源与787区间并集、业务结局及时间端点全部一致，导入起止/时长误差0ns、processor errors为空。跨域仍不对齐，全局覆盖unknown，UI **UNRUN**；不外推全进程树终态。旧[直接Runner取消夹具](#r07-b-native-cancel)缺应用账本的导出FAIL仍保留，该旧case未因本次成功而补验。

复现输入为`/opt/aries-artifacts/observability-r04-cli-cancel-inputs-1/`：driver SHA `ab46fe86461cc29bef2e56cadb2bfc32f0c759c012926f27f531012350f38d75`，review `c4083437a441cb5f1843ad3f0312e4e20959019e754de5ee7fee44b5b4eb1d25`，`perfetto.py` SHA `c580f1c472b637fe7a12f64f348baced7b9a5d38ee0618b850e34f324e1d2eaf`。在ECS既有SDK2.58虚拟环境运行`driver.py`，终态清理后运行`review.py ROOT --recording RECORDING --tape RECORDING/merge-ranges-001/model/tape.json`，再运行`perfetto.py`；`RECORDING`为`/opt/aries-agent-bench/runs/20261023T022605.576605763Z-openclaw-e2b-embedded-record`。运行根为`/opt/aries-artifacts/observability-r04-cli-cancel-1`，TP根为`/opt/aries-artifacts/observability-r04-cli-cancel-perfetto-1`；复现须使用新目录并调整固定ROOT/证据pin，不能覆盖本次原件。检查器仅写新reconciliation，无helper依赖；本限定取消分母与投影范围封存。

<a id="r01-native-concurrent"></a>

## R01：同 Tape、两个完整 A 任务并发验收

本次直接运行原`aries cloud`，没有修改产品源码、部署服务或隔离/评分逻辑。固定568文件运行时`observability-r07-native-release-1`（manifest `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`）、既有A镜像`sha256:cc7b2c8270be5ca0f5169b7b9f0b6574fd56f6f559cb778b9cbfc467b7b8e664`、E2B模板/build `3887f1lyrzka29unnzgj:ea001e75-54b9-407b-890c-f20c96e036c2`、原A Tape `d716431ea36e1b89b47aa0c8f091cd6e65700bc10e83522467414a0d3735cffb`。原生工具策略与录制一致，后台/process/poll/PTY能力保留，本轨迹实际为6次前台exec。

执行前固定attempts=2、concurrency=2、L0、recorded时序、最多4次创建及TTL600秒，无自动补样本。预登记从Tape列出每个任务6次决策；事后独立拼接原SSE校验ID/name/参数，参数遵循原JSON语义，不要求录制SSE的空白与canonical metadata字节相同。原始run为`20261025T105702.213484419Z-openclaw-e2b-external-r01-concurrent`，两个任务后缀001/002均进入实际调度账本。

| ECS归档（相对`/opt/aries-artifacts/`） | 实际结果与SHA-256 |
| --- | --- |
| `observability-r01-native-concurrent-1/preregistration.json` | 执行前两个任务/12次调用分母；`cb17d95e6a8c685855968e2d630dea0b944d40b8601e654670feecc15d3ec944` |
| 同目录 `evidence.json` | PASS_EXECUTION_AND_CLEANUP_ONLY；`02242069258cfd48aeaa0a486f550cc89860d2427f8008b81e32744121280400` |
| 同目录 `reconciliation.json` | 212项原件复核一次通过，两真实Attempt及12条精确身份链；`c160e55bca80d202830b71db2084d843deb51c745aae00f9507bd2cf72a981dc` |
| 同目录 `provider-cleanup.json` | 4创建/4独立NotFound，0恢复删除；`568459f8a683757c9f6b26ab7de5df52be4efece6f9f4926b0627e5cb1aec1d7` |
| `observability-r01-native-concurrent-perfetto-1/evidence.json` | 实际TP：845区间、86flow、2995事件、38来源，导入起止/时长误差0ns；`4dfd3eb3ecf0a7edfe4d6244df0c49ec7b33720783c7531d13c21bba8b307a7f` |
| 同目录 `exports/run/analysis-snapshot.json` | complete/unknown、无诊断，两个Attempt和12个身份映射；`0b8c3a7b61fd1d31ed3ee2cb9c1d908882e1c8b2ddcee34f10f1d7d019434be4` |
| 同目录 `exports/run/trace.json` | 与Snapshot同内容身份，未造guest泳道；`46ebd2ec79a8093956d32607e187a5009921f97cd6383a5ab4d6d0289cdb580c` |

planned/admitted/started/executed/completed/verified_succeeded/succeeded均2，失败/未启动/清理错误均0；各7轮/735帧Strict、6次真实exec及12项独立Verifier通过。两个候选均与录制SHA `181c011a55737cb2ade8836c992ee463c45479d31312cfc6855aba0d6f62bdf1`相等。每个任务的原Stop/revoke均先于独立评分，4个Sandbox独立NotFound，临时Adapter/凭据/容器与网络正常回收，源码、Tape及既有服务未改，0上游LLM。

两个任务重复使用相同六个ToolCall ID，仍按Attempt scope绑定不同native invocation/SSH request/Bridge execution/SDK process；未采用命令或时间匹配。两个模型session实测独立，token原值未采集。ToolResult正文和原业务字段逐项一致，测量durationMs除外；失败工具exit=1保留在原轨迹中，不变成成功。controller `agent.run`重叠18.540939532秒，本次SDK wait重叠对数为0；仅验收跨任务身份隔离，不声称guest同时执行、单Agent并行工具或重试矩阵闭合。

复现输入为`observability-r01-native-concurrent-inputs-1/`：driver SHA `b8652220cf3bbc91e5d8f15651a5795d4fea1c312a58f24c1bdeb097ba076785`，review `9e052b820e371514552726c370bc03d5838d3893ce2231b80649aa9053d1a94c`，固定helper `020b26c4003a6986cc547152973a372094a93fed7ebb9c055f4f855fe45f2ea7`，Perfetto脚本 `dc5914784439bb7d0b26ffadbcdbe2c42d51d8a1efd2825a2fc2168f66d5a885`。ECS先完成语法/预登记检查，再由既有SDK2.58虚拟环境运行driver；清理完成后运行review，再运行perfetto.py。复现须另设输出目录、更新对应证据pin，不能覆盖本次归档。未改产品，因此未重复运行原568文件工程门禁。

本次新增2个整体成功任务，累计78；八阶段及逐工具数据见[性能归档](../performance-archive.md#r01-native-concurrent)。本限定范围封存。P0仍PARTIAL；全局漏采分母、单Agent并行/重试、后台/PTY以及P1/P2按原台账保留。本次UI未执行，不以TP成功替代UI。

<a id="r07-b-native-cancel"></a>

## 当前 B 显式取消：真实 Agent、已知后代停止与独立评分

复用568文件运行时manifest `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`、schema2 E2B模板/build `bd59dosz98xd33glqt1q:6d193918-762d-4ce2-a7d4-6513cc3875f8`及SDK镜像 `sha256:69ffe66403a83d5956a887e838138b1b9fd50f1e3040313d076aefba0a257a11`。只在私有source副本增加两个测试文件，没有修改产品源码或部署服务。ECS编译及既有Runner/Embedded/ExternalBridge三项定向隔离测试退出0。

| ECS归档（相对`/opt/aries-artifacts/`） | 实际结果与SHA-256 |
| --- | --- |
| `observability-r07-b-native-cancel-1/prepared.json` | PASS_PREPARATION_ZERO_CLOUD；`0b6ef112d6e0f8873d5537b26755f2a2e0ae4628a09fbd113b18bd840bd5a266` |
| 同目录 `evidence.json` | PASS_NATIVE_B_CANCEL_IDENTITY_CONTAINMENT_VERIFIER_SCOPED；`fd90659aa86551cb403d0af1b828eaf03ffedc1584153a8093474398608fd8c1` |
| 同目录 `identity-review.json` | 25项原件独立检查、输入未变；`df9fb09110c7dfdcfba0ed0b30532ae3c36c97f15b73ece3a66cb2bb44a49801` |
| 同目录 `provider-cleanup.json` | 2创建/2独立NotFound，无fallback；`69b632b0b0d78672260d596f124055b598d79de500c7b48440db782ae007e12b` |
| 同目录 `cases/b-native-cancel/run-result.json` | Harness canceled、isolation confirmed、Verifier failed/score0、cleanup succeeded；`ff9153b0eef4b6be46276c5ac046b527269a6a48ea0cb19d1e16b2dee20adf27` |
| 同目录 `cases/b-native-cancel/native-tool-observation.json` | failed/incomplete原样保留；`962971ea97ce88b3237febe24b0fb40c4a03a9bc73f8544a7055b1a4ddd41baf` |
| `observability-r07-b-native-cancel-analysis-1/evidence.json` | **FAIL_DIAGNOSTIC_EXPORT**：辅助脚本错误预期直接Runner结果会导入native来源；输出只含控制器完整记录，不能称全链Trace通过；`bfdc885db7aa9eb9d388ca87bd48ba401f555ac7e38b363f6a35c8adcdb241a0` |

预登记1次、并发1、最多2个Sandbox、TTL240秒；实际Runner调用1、模型HTTP请求1、预期ToolCall1、评分调用1、0上游LLM，原始候选SHA `dc417e41228d55bcc6a1a7895adbc17fa47a3699edae23fd6f19a1de7fed6cd4`。这是一条Scripted Scenario故障夹具，**不是Strict Replay或成功任务**。原JUnit为12测试/11失败/0error/0skip，SHA `d4206d2856e0ae836757f8dbd6124df20a628fe604afebb601ea161a6189ca7a`。原失败候选不因为生命周期验收PASS而升为成功；该次结束时累计成功任务仍为76，之后新增任务另计。

原`w2AwaitActive`确认父/setsid子进程PID+start、UID60000/cgroup与两次变化心跳；仅取消Harness.Run子context，Runner主context有效。原`Embedded.Stop`/`embeddedBridge.Stop`先停止/撤权，再在Evaluate入口复用`w2ConfirmStopped`的两次探针：已知PID/start消失、cgroup empty、UID60000为0、心跳及mtime不再变，早于120秒自然结束；同时runDone/processDone闭合、guest18789/18790均ECONNREFUSED，然后调用原codingcase Verifier。运行后两个Sandbox独立NotFound，临时Adapter/凭据/测试进程均无残留，原568源码及生产Adapter/Caddy未变。

模型fixture SHA `82f03254a9f226808c6516d846dc9fe7ed0f6abaa7ba9ad89faa68d312b583d8`，原生日志第23/24行回调与预登记ToolCall/Agent session精确相等；PID1917属于取消前实见受管进程组。spawn→stdout首输出26.008256ms；日志无exit或EOF，不伪造排空/终态时间。native index实见1个start、无terminal，issues为`native_terminal_not_observed`、`incomplete_tool_pair`；这项观测失败与正向停止隔离分别报告。

辅助导出未通过的原因已按源码确认：`pkg/performance/snapshot.go:BuildSnapshot`逐`run.attempts`读取native等辅助来源，直接Runner夹具没有应用层Attempt账本。导出实际只有4来源、63控制器事件、16成对operation及9条Gateway delivery，complete/unknown、attempts空；这不是native证据完整。原FAIL、snapshot/trace和输入SHA保留；该旧case的TP/UI尚未执行，未补造Attempt或修改验收使其通过。后续[原CLI取消验收](#r04-cli-cancel)已独立验证实际账本及双域投影，但触发发生在首工具返回后的模型请求阶段，不补验本夹具的在途工具缺失来源或终态。

复现输入 `/opt/aries-artifacts/observability-r07-b-native-cancel-inputs-1/`：`driver.py` SHA `783b866a421a3cc4a75b151e477574c54a6385d576b833fe243e7dae5c9c1ff0`，`review.py` SHA `d00b80ba09914b1899b6e33ea15af72b9845d9edcdcca6e96dda730fb429165f`；ECS格式化后的native fixture SHA `82f7944729eba9e5cf93a0ab512f70073d74ea4562a8dda684feb57c23b55393`、原helper `2f4a641cbe2c48e53bba6f11c1cb957b6fdc97782a189f7003760711e96e0d25`，测试binary `6ca61953a998ac759b641256889b75d96817efefb13229653c7e5c6abaed0269`。准备命令为该driver的`--output ... --inputs ...`，真实执行加`--execute-paid`，selector `TestR07NativeEmbeddedCancel`；重现须使用新输出目录并同步RunID，不覆盖本次证据。

旧`e2b-w2-faults-1`已有B超时后的正向停止/独立评分证据（`405c1e1e1ebefb79a9bdbea2bee44e19ebd4ffc083f6eb78dafed08353c2ba63`，见performance-evidence E095/E096），本次不是首次证明B能停止，只补当前568/schema2组合的显式取消与原生身份。通用后台/PTY树终态、精确kill时刻和全局采集分母仍未验证。

<a id="r15-b-native-paired"></a>

## 当前 B 观测版本：四对 baseline/L0 实际验收

新增实验不改变产品源码。固定运行时 `observability-r07-native-release-1`（568文件manifest `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`），既有schema2四模块、v1.4分析器，模板/build `bd59dosz98xd33glqt1q:6d193918-762d-4ce2-a7d4-6513cc3875f8`（2vCPU/4096MB），原B Tape `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb`。两条件同配置/工具/Strict/停止/评分，只改ARIES_OBSERVATION_LEVEL；基线也保留严格会话完整性、真实工具、Verifier和清理。

| ECS归档（相对`/opt/aries-artifacts/`） | 结果与SHA-256 |
| --- | --- |
| `observability-r15-b-native-1/evidence.json` | 保留PARTIAL_OR_FAILED_MATRIX：1baseline业务/计量通过，7个准备失败、未调用Runner/创建Sandbox；2Sandbox确认删除。`25fe996ef6a607bfe7c7c545a6d1cbb4a4464d56b983784e17cbd9dc77754769` |
| `observability-r15-b-port-probe-1/evidence.json` | PASS：独立loopback TCP反例，无SO_REUSEADDR的冗余bind在TIME_WAIT返回errno98，加该选项可绑定；原失败标记均停在复制driver前。原OSError只有class，没有errno，故原次根因依据控制流与复现推断，未伪称捕获原errno。0云。`34f008b4cc5312af53801ee000652bc4d6a75a1d9a866f00f0fbeec572c05076` |
| `observability-r15-b-native-2/evidence.json` | PASS_EIGHT_B_REPLAYS_AND_DECLARED_MEASUREMENT_ONLY：新完整8次，16创建全部独立NotFound，0模型，源码/Tape/生产服务未变。`6b7aeaab7bbfb838e4eccb08fe95ed3d905b96bec5f5eae57921497d1bc89128` |
| 同目录 `paired-analysis.json` | 四对原数、L0−baseline、相对差值及median/min/max，缺值null。`d83825d06574ef6973697c6e50aff9017e3ac7f2bf09f4d7f6257921f2fd4d8a` |
| `observability-r15-b-native-review-2/evidence.json` | PASS：479项原件SHA、真实分母、八阶段、CPU/GC/分配/写入字节和独立配对算术复核，不调用原paired_analysis；`539556c94c41f85be0d78efb2f15ce777d3fd73d83969d8b580c1d6741258496` |
| `observability-r15-b-native-2/perfetto/evidence.json` | PASS_FOUR_B_L0_TP_AUDITS：4份L0实际双域TP导入，原始来源和snapshot区间并集/outcome核对，误差0ns；`7e091369438bef922bb2c07f0e21c127200337c416c08462be86fda5c1d0917a` |

新矩阵预登记顺序baseline→L0、L0→baseline、L0→baseline、baseline→L0；并发1，每次最多任务/评分2个Sandbox，各TTL600秒，总创建上限16。新8次planned/admitted/started/executed/completed/verified/succeeded均8，失败/未启动/清理错误0；旧矩阵1成功+7准备失败在自己的分母里保留，不被新8次覆盖，也未将旧单样本纳入配对。两矩阵共18个Sandbox均确认消失，临时adapter/凭据/Runner进程无残留；复用既有模板，不新增build。实际账单未知，未把无上游模型请求写成零云费用。

baseline模型轮帧、native回调、Snapshot、可选writer计数均null；L0四次实测各4轮/573帧/3exec/30回调，Strict/候选SHA/12项独立Verifier通过。Runner与request-boundaries的实际producer计数均drop=0/write_failed=0；原生logger总分母仍unknown。四类JSONL只度量Write接受字节与调用，不等于物理I/O。控制器SELF包含Runner+ModelProxy+collector，GNU time另列；没有拆出guest/native logger/Adapter进程成本。L1云资源轮询在两条件均关闭。

四份L0的controller区间分别1120/753/860/877，各有7个独立native区间、31flow、12来源；时间单位/起止/结果/源SHA实际TP对账均通过。两时钟UNALIGNED、integrity=complete/coverage=unknown。baseline无时间线，不算Trace通过；本次UI UNRUN，复用同版已验UI投影能力。

Runtime配对差值中位数−128.121778ms，范围−174.333993～+23.510723ms；整个Attempt中位数+118.2710945ms，范围−7251.934591～+7378.659286ms。峰值RSS差值中位数4380672bytes（4.177734375MiB）。四对小样本仅描述差值，不估p99/容量、不称负开销，未设性能合格阈值。region/云宿主/cache冷热未知；完整八阶段、每次资源和差值见[性能归档](../performance-archive.md#r15-b-native-paired)。

复现入口 `/root/aries-setup/uploads/r15-b-native-2/`：PLAN.md、matrix.py、driver.py、review.py、measurement.py、review-matrix.py及trace-matrix.py/perfetto.py；上传包SHA `aefb003299bd4ef52e9b7a28206b389f2eb676c48b4397834187b54b14b322e4`，额外独立复核器SHA `aedc07a5ab4057f83e8877cc087d10df26764dcb1291de33681959f6ef34d641`。driver `be6e01336e51cba930aaf7bd103bf9e112027cfc4dbe9de192483d48a068958b`、review `d8c145e28f5e12238d9c9488c9eff6f1faa6f2b96714a721aacd1ecd8d7a497a`、measurement `0da7b22aa26a9c50b228bbea46079b7d72d309a59e0ae3534e49767ad6afcb74` 固定。ECS预先执行语法与小型配对算术/缺失样本检查；仅删除bind预检查的第二批复用了未变计量/复核器的验证。不改Strict标准，不重跑产品工程门禁；新复现必须分配新输出目录。

<a id="r07-e2b-first-output"></a>

## 同版 E2B B：原 Tape 回放与首输出实测

本次只派生E2B模板并做实际回放，不改变产品源码、Runner或离线分析器。运行时继续固定 `observability-r07-native-release-1` 的568文件manifest `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`，完整工程门禁证据 `bd4acb1c0080857ba5ac54615f2d158f9c80f2be06958082a23fda2997b96bda`。分析器仍为下节v1.4.0、二进制 `86de3551cdae736f0a3154a2e38dd09a18dc2eb614678de9ff27ee10297e8d25`，四模块仍用 `c972e462…` manifest；未因新增实验而重复宣称工程门禁。

| ECS归档（相对`/opt/aries-artifacts/`） | 实际结果与证据SHA-256 |
| --- | --- |
| `observability-r07-first-output-e2b-template-1/evidence.json` | 保留FAIL：构建已提交，实验脚本误把SDK dataclass状态当成具有to_dict/model_dump的方法对象，尚未创建Sandbox；`04a92bf8846bc7a115fa883e3fc24e35f1e24d523d82b8598aec97188274add4` |
| `observability-r07-first-output-e2b-template-resume-1/evidence.json` | PASS：对同一已提交构建修正状态序列化，新增build请求0；四模块SHA、启动/运行环境及cgroup探针通过，探针Sandbox确认删除；`adc3dbe56fe83033f57bb1696c04125b5a12b61ab094c5bafcb19706c07022f2` |
| `observability-r07-first-output-e2b-b-1/evidence.json` | PASS：一次原Tape严格回放、12项独立Verifier、Stop/revoke、2/2 provider NotFound、无临时凭据/容器/Runner残留；`be619aff4016477c861826d84867d09651358daa0dd31aff050380afc94186c2` |
| 同目录 `reconciliation.json` | PASS：4轮/573帧/3exec、30回调、八阶段与直接进程/首输出/EOF精确对账；`dd9897d474490c5f402ba8eb09185471979ca6a049e2dd875f155335bc956efa` |
| `observability-r07-first-output-e2b-perfetto-1/evidence.json` | PASS：controller795区间/31flow及native7区间/0flow，12来源、802区间并集、状态及时间一致，导入误差0ns；`77b751fbca06fd6efc1d38a344878c26e5ae7f08a796ff07279df42f9f91e771` |

原模板/build `3887f1lyrzka29unnzgj:ea001e75-54b9-407b-890c-f20c96e036c2`；新模板/build `bd59dosz98xd33glqt1q:6d193918-762d-4ce2-a7d4-6513cc3875f8` 保留以便复现。构建先验证原模块及start/check脚本，再复制四模块；未更改启动脚本和运行规格（2vCPU/4096MB）。初次脚本失败原件不覆盖，也未重复付费提交构建。

真实运行目录 `observability-r07-first-output-e2b-b-1/runs/20261025T093437.305074792Z-openclaw-e2b-embedded-r07-first-output-replay`。原E2B B Tape SHA `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb`；只增加native观察协议和候选模板，不改Strict/工具结果/停止/验证规则。预登记一次build、一次TTL180秒启动探针、一次并发1回放（任务/Verifier各TTL600秒，最多2Sandbox），不自动重试。实际总共3个Sandbox均确认消失、0上游LLM，生产服务及源/Tape未改。模板保留不等于Sandbox仍在运行；实际账单未测量，不能写成零费用。

3个ToolCall首输出分别7.587584/152.823030/161.493016ms，直接子进程区间35.683295/205.450768/195.718501ms；T1 exit→最后自然EOF为4.437940ms，T2/T3 EOF在exit前，不补造排空区间。原轨迹的T2 exit1保留。两云B使用相同观测模块/分析器但不同原Tape，不能凭这两次时长作云性能排名；表格见[性能归档](../performance-archive.md#e2b-b-first-output)。

Snapshot文件SHA `cc85e91f00adab6498d62a416f14557c89e033acca368e938d4706ee2e4ed06c`；controller/native视图来自同一快照，独立单调时钟保持UNALIGNED，integrity=complete、coverage=unknown。新Trace未跑UI（UNRUN），投影与已验v1.4华为B UI相同，仅复用版本能力验收，不宣称新E2B原件已点选。全进程树、PTY终态、独立覆盖分母与新版本成对扰动仍未完成，不因此重跑已通过的普通前台工具样例。

复现脚本归档 `/root/aries-setup/uploads/r07-first-output-e2b-1/`：`PLAN.md`预登记、`template.py`/`template-resume.py`构建及探针、`driver.py`回放、`review.py`离线核对、`perfetto.py`实际导入。driver上传包SHA `c609cbb78a104e629ac162feb806742a34ecfaf199996b3f1c33d66e706858c6`；review SHA `21c809b7ed9e3e6e789838ca5b181453c6f219ea659f36076f6c8e4526476f5d`。原执行脚本分别在各输出目录留存；复现必须使用新输出目录，不覆盖旧FAIL或PASS。

<a id="r07-b-first-output"></a>

## B首输出：schema2、真实华为回放与Trace验收

本轮仅补R07首输出缺口。`experiments/r07/first_output_patch.py`以固定schema1四模块manifest为输入，只改child adapter的schema版本和原stdout/stderr data回调中的一次性观测；没有增加读流器/listener或改decoder、ToolResult、argv/env、取消、后台/poll/PTY。`pkg/performance/snapshot_native_process.go`与测试、`snapshot.go`升为parser `aries-analysis-v1.4.0-native-first-output`；协议配置仍为`aries-native-process-v1`。v1原件首输出not_collected，新v2缺测unknown，无输出不补零区间。

| ECS归档（相对`/opt/aries-artifacts/`） | 实际验收与证据SHA-256 |
| --- | --- |
| `observability-r07-first-output-run-1/evidence.json` | PASS：原版/候选各9组，73个工厂日志回调含10个首输出，业务返回/退出/原生能力对照一致；两个容器回收。`029983f6dd47dd85b62a32d5d76822ac5698c7e4fa1255f65594b82b2c8666db` |
| `observability-r07-first-output-analysis-1/evidence.json` | PASS：performance unit/race各225项（含子项）、vet、aries-perf构建。`f4e39e055aa4e57641ee18aa7182564c4f27a21e02a0bb26bbb3201193b30df4` |
| `observability-r07-first-output-image-1/evidence.json` | PASS：新镜像构建、版本/四模块SHA及配置一致；两个只读探针容器回收。`2daf60da567dd9f1c9eabab1c9196c964de45b48e1f51cd7c699efc4259dc8f3` |
| `observability-r07-first-output-publish-1/evidence.json` | PASS：SWR实际push、OCI index/root/platform核对、临时认证清除。`c9a10fc0a60f98d37b6f2ef5e273942cf329a98b365b034954c85fbf1c44ecf0` |
| `observability-r07-first-output-huawei-b-1/evidence.json` | PASS：一次同Tape Strict回放、真实工具、独立Verifier、Stop/revoke与清理。`9fd94c87a04401e12e71e4d547eb23facdd3c5507a6f4b1d0c512950a46e957e` |
| 同目录 `reconciliation.json` | PASS：4轮/693帧/3exec、12测试、30回调、首输出/exit/EOF与同Snapshot逐项对账。`c6ec81105b453278c90a61934ed4f9164febe23ed93ef5c8466dd381f6ab62e6` |
| `observability-r07-first-output-perfetto-1/evidence.json` | PASS：controller779区间/31flow与native7区间/0flow、12来源、并集786精确；起点/时长/终点导入误差0ns，processor errors空。`823007b6c65a7503abc1e98d53764940382d754573f8cd68b6c5abae3ff90139` |

实际UI也已通过：`observability-r07-first-output-ui-1/evidence.json` 状态 `PASS_ACTUAL_B_FIRST_OUTPUT_UI`，SHA `75ded0256b8d22ace8f94b04b9a8f60acc7e0b8b57b93d3021ab4052c8762189`。在ECS Chromium中用Perfetto v59.1-1e9c61e98内置WASM打开本次native视图，实际SQL与timeline点选T2：首输出438164376ns，ToolCall/operation/scope/source与Snapshot一致，泳道UNALIGNED；只声明本次原生视图UI验收，两域完整对账由上表TP证据覆盖。3张截图取回目视，10个自有浏览器进程按PID/start确认消失，原始证据未改。4条控制台错误、9条警告保留；首次未加载既有tools.env而缺libatk的启动失败另存摘要，随后加载正确环境成功，未安装依赖或增加云/模型调用。

镜像/root digest `sha256:ba1c0c46219c2de635109ffe16e672da1db83aefc950b003df85a7755bdd8554`；四模块manifest `c972e462d49e708b24fedf019c23e6181fa75a2e2b126ac6484d809b5c9d6209`，只有supervisor相对上一版发生内容改变。完整构建输入归档在image目录，SHA `599747bbefbf5be9e6d9ca4ba3c700b524b24afac902dba0effb624e523c1108`。分析器568文件manifest `7e462d86330d149cf4c986df2eaa9029a4829523ee6830476fcae14b4bab4761`、二进制 `86de3551cdae736f0a3154a2e38dd09a18dc2eb614678de9ff27ee10297e8d25`；实际Runner仍为已验收的`r07-native-release-1`（manifest `3b6450a8…`），未把离线解析器定向测试写成重新执行五项门禁。

ECS容器矩阵包含无输出、双流延迟、NUL、262KiB输出、前台非零退出、父退后子持pipe与poll/log、kill、sink异常、真实PTY。新数据回调捕获raw非空chunk，业务比较仍比较native解码后返回，不能冒充完整原始字节录制；原始fixture结果均保留。解析矩阵涵盖旧schema兼容、双流顺序、无输出、exit后首输出、EOF后异常、重复/混版及观测失败，不猜跨域时间。

真实run目录为 `observability-r07-first-output-huawei-b-1/runs/20261025T085835.964652343Z-openclaw-huawei-embedded-p0-runtime-replay`。冻结原B Tape SHA `3fc9751b0ef20c16707c6d968f7aa46a65b70b7d4302a075e9e27c8b8fd233f5`；预登记1次、并发1、TTL600秒、最多2Sandbox和1临时public模板，无自动重试、0上游模型请求。2/2Sandbox独立404、模板404、adapter/Runner进程组无残留、临时凭据删除、既有服务与Tape/源码未改。华为评分出网仍`assumed-unverified`。

3个ToolCall首次stdout相对spawn为3.407878/438.164376/280.955009ms；stderr未观测到非空chunk，时间为空。T1直接子进程exit后最后自然EOF晚2.796507ms；T2/T3的EOF均早于exit，不倒置也不造零排空。三条直接子进程区间17.372505/468.946705/302.793355ms，T2 exit1保持为预期修复前失败，最终Verifier12/12成功。7个native区间=3首输出+3直接子进程+1排空，独立Node时钟保持UNALIGNED，coverage仍unknown。

复现入口：`/opt/aries-artifacts/observability-r07-first-output-1/fixture/driver.py`（原生对照）；`/root/aries-setup/uploads/r07-first-output-analysis-1.py`（Go验证）；`/root/aries-setup/uploads/r07-first-output-cloud-1/driver.py`（真实回放、参数和配额见归档METHOD）；`/root/aries-setup/uploads/r07-first-output-perfetto-1.py`（离线导入）。原件保留；每次复现用新输出目录，不能覆盖已有证据。完整进程树、PTY EOF、独立应采分母、logger增量成本仍未验收；这不关闭整个R07或P0。

<a id="r07-native-process-snapshot"></a>

## B原生进程：真实工厂日志与统一Snapshot接入

以下工程步骤分别在ECS验收，0云/LLM。未修改Stop、四角色或原生工具行为，也未把本地fixture算成真实Agent任务。完整工程门禁通过后，真实云B同Tape回放及新进程泳道TP已有独立证据，见[华为B原生观测验收](#r07-huawei-b-native)；本节工程步骤本身不包含云验收；后续[实际UI验收](#r07-huawei-b-ui)单独留证。

1. `observability-r07-native-gateway-sink-run-1`：在原B镜像 `d33b073f…` 中，原版/四模块候选各执行5组相同用例。前台双流/exit7、后台后代持pipe与poll/log、process.kill、真实PTY及异常sink对照保持原行为；实际factory提供run/session身份，27条回调经既有logger到stdout逐字一致，候选启用开关在工具继承环境前被消费。两容器与已知fixture进程均回收。证据SHA `b6628f73f9fc6629af27fbf653cb400061599e8d7ce7ba49969b4ef31b21f398`，4模块manifest SHA `712eedb551156cbd49c066ae0ce595ac7d66920d0f662575db80b93cec32853d`。该次factory接线仅在私有候选内验证，未构建或发布镜像。
2. `observability-r07-native-snapshot-1`：基于冻结565文件，只新增解析器/测试/纯metadata fixture并接入 `snapshot.go`；解析器版本 `aries-analysis-v1.3-native-process`。performance包unit/race各58顶层、含子项207测试通过，vet及分析器build通过。证据SHA `0967f852502fde2530a589e83b420f434fa3a18e5c430100c40cce3082e412bd`；`aries-perf` SHA `c765f950757e49eeb55d55e09426e243da2b99c462ca947a230be6c619b6130d`。本地仅编辑/打包，ECS格式化后的四文件已回传。

解析测试使用真实27条callback原件，**native start/result与run-result为明确构造的解析fixture**，不宣称完整Gateway/Agent因果贯通。已投影3个spawn→exit区间、1个exit→最后自然EOF区间（313661219ns），保留3个consumer区间；原模拟业务FAIL未改变。exit7仅记exited。另两例的双流EOF早于exit，记录原序、不造0耗时或倒置区间。ID/会话/序号/时钟/PID偏离、缺exit/仅close/合成fallback/缺EOF均验证，不通过匹配命令或时间近邻关联。

| snapshot-1历史源码/fixture | 该次ECS格式化后SHA-256 |
| --- | --- |
| `pkg/performance/snapshot.go` | `03f575ee3624dbbb67a10c6594571292acc70546fe3bd1667a8360d35c508a0d` |
| `pkg/performance/snapshot_native_process.go` | `2a933d34bc08b0a99392d774e81a3ba6e7afbe77743f18561e10771522572cfd` |
| `pkg/performance/snapshot_native_process_test.go` | `d6c80f93262b5bdfd44b9d11df886c98d1fdb64350ec5db0d0cd5162f75408b0` |
| `pkg/performance/testdata/native-process-gateway.txt` | `e8254c9a2df9b1bb9110f1405e518e42285e314897b8fce003a6ed8a48b10e8d` |

snapshot-1复现入口 `/root/aries-setup/uploads/ecs-r07-native-snapshot-1.py`，输入overlay SHA `99608a484695d478157a88f490c017285d224a22c2e79f53fc7e2e24a2abbda5`，原565文件与其它候选源逐项未改。该次证据只覆盖无时间前缀的factory日志；后续真实CLI格式、修复和完整门禁分别如下，不能回写为snapshot-1已验证。

### 实际Gateway CLI包络与定向修复

固定B镜像的Gateway CLI会调用 `setConsoleTimestampPrefix(true)`。`observability-r07-gateway-log-envelope-2` 实际启动原CLI，通过同一subsystem logger记录纯metadata探针，确认格式为 `2026-10-25T07:32:24.660+00:00 [aries/native-process] ARIES_NATIVE_PROCESS {metadata}`。使用既有 `logging.level=info`、`consoleLevel=info`、`consoleStyle=compact`，network=none，0 Agent/模型/云请求。这验证日志包络，不验证Agent执行或Gateway完整就绪。之前fixture的`plain`不是原生合法style，而是非TTY下回退到compact。

| 归档（相对`/opt/aries-artifacts/`） | 结果与SHA-256 |
| --- | --- |
| `observability-r07-gateway-log-envelope-1/evidence.json` | 保留FAIL：私有fixture目录受umask变成0700，非root CLI无法读取preload；`fbf467359235055e03501ab982c3bc0629cf66e2f396606e08bc26ce0095f667` |
| `observability-r07-gateway-log-envelope-2/evidence.json` | 仅修目录0755后PASS；`23c6fb481ceeeb27268a9cb6e11d1463fdac8aa941dccd4dfa82d29b49316596` |
| `observability-r07-native-snapshot-2/evidence.json` | 保留unit FAIL：新测试误要求加前缀后的source SHA/EvidenceRef不变；`86034ff52c59d6bfc024bfa5e9e1bd511f9bc98cbd37abf433cff6e08bcbee1a` |
| `observability-r07-native-snapshot-3/evidence.json` | 比较真正clock属性与区间后PASS：unit/race各59顶层、含子项208测试，vet/build通过；`e0cee306bbd15579e4fe4beb83db58053ed5010de994aba6455bcb62439f7b63` |

两次CLI探针的自有容器均删除、原init PID消失；成功例Stop耗时1,502,818,618ns，stop与容器退出码均0。包络解析只新增实测毫秒ISO时间与数字时区前缀，不作clock锚点。测试给原27条callback加该前缀属于明确合成的组合fixture：身份、单调offset、duration及coverage必须不变；source SHA正常变化。snapshot-2到3只修测试比较范围，生产解析未变。

snapshot-3 manifest SHA `7c07c346d824b4ec0931ef479be3a31e4dae4b14672de0af3d582652e41bba87`；分析器SHA `74f51bdfef9134e2cd2a3d9717e01bd0b4e9d217072805b6d3edb9a5125e455a`。该次解析源/测试SHA分别为 `fd7387176261654025c6199de28bdb5d44d491121e162fa14d266d2319b6b0b3` / `87c123bea76b0afd48ede34e97acdf359c4b1d4be8312a5b602b89d293327da2`；当次snapshot入口及原metadata fixture未改。复现驱动 `/root/aries-setup/uploads/ecs-r07-native-snapshot-3.py` 与CLI探针 `/root/aries-setup/uploads/r07-gateway-log-envelope-2.py` 绑定各自归档，已有输出不可覆盖。

### 显式启用与完整工程门禁

`harness.native_observation_protocol=aries-native-process-v1` 仅接受embedded，DecodeCloud及Prepare均在副作用前校验；空值默认不启用。Go将协议选择与 `ObservesNative(level)` 相与后写入既有staging/launch.env：baseline不注入，L0/L1/L2注入 `ARIES_NATIVE_PROCESS_OBSERVATION=1`。Gateway启动器会重建环境，因此不依赖镜像ENV；候选factory导入时读取并删除这一个开关，不让工具继承。没有新增Tool wrapper、队列或采集生命周期，也不改变ToolResult、原生后台/poll/PTY、Stop/Verifier门。

`observability-r07-native-release-1` 已完成 `make build/test/test-race/lint/integration` 五项门禁，状态 `PASS_R07_NATIVE_COMBINED_FIVE_GO_GATES_ONLY`。证据SHA `bd4acb1c0080857ba5ac54615f2d158f9c80f2be06958082a23fda2997b96bda`；568文件manifest SHA `3b6450a8050691e5c98f888d516845e93d9e67ff97b3097be65c422275c0dcc5`；底层cleanup-release的565文件原件未变。该冻结版本 `aries-perf` SHA `485cb60c46da908b53ca6bc1af09396eaf6f9403e6db27ee9db956f727500045`。

门禁完成时，新候选云回放及进程视图TP/UI尚未执行；后续真实回放和TP结果单列如下，[实际UI验收](#r07-huawei-b-ui)亦另行留证。全进程树、PTY drain、独立事件分母/最终seal及logger同步I/O增量成本仍unknown/未验。Perfetto主控制域与native单调域分别导出，不伪造跨域对齐。

<a id="r07-huawei-b-native"></a>

## 华为B：真实原生进程观测、同Tape回放与双域Perfetto

### 固定版本、镜像与真实执行

使用上述568文件 `r07-native-release-1`（门禁SHA `bd4acb1c…`、manifest `3b6450a8…`）及原B Tape `3fc9751b0ef20c16707c6d968f7aa46a65b70b7d4302a075e9e27c8b8fd233f5`。候选镜像仅把已验证的4个原生模块加入固定B基底，原版本和环境不变；运行显式启用 `aries-native-process-v1`，没有改Strict、工具执行、Stop/revoke或独立Verifier。

镜像完整引用：`swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-openclaw:2026.7.1-r07-6470521e690f021ab260c23ba2c6087900c3194c171281c8e88d7b323902876e@sha256:6470521e690f021ab260c23ba2c6087900c3194c171281c8e88d7b323902876e`。该digest是OCI index根描述符，不是config digest；实际平台leaf为 `cbe22b1762e943554a15b806ab95542f6136e1399bf64e51b9816f03a4d50b83`，publish-3按真实描述符层级核对。

| 归档（以下路径均相对ECS `/opt/aries-artifacts/`） | 结果与SHA-256 |
| --- | --- |
| `observability-r07-native-image-2/evidence.json` | 缓存基底及4模块构建核验通过；`49196cd93a07b5eb27697c4a57931e52958248f2a2583fb7cd8581f0ffbe9423` |
| `observability-r07-native-publish-3/evidence.json` | 最终远端镜像身份与清理验证通过；`5d7c81f4205279a85a701cf51f88c9ec8b94ff08cc591ab1370af06e7038f214` |
| `observability-r07-native-huawei-b-1/evidence.json` | 单次真实Strict回放、独立评分、停止撤权及回收通过；`5f1b029959862aebe75ad94d1e5b2042c5a2e63b6938486660107183dd6e66e2` |

`native-image-1`把裸镜像ID解释成Docker Hub仓库而构建失败，原件保留；其预设 `registry_calls=0` 被build日志中的真实Docker Hub元数据请求反证，不能据此声称零网络调用，image-2已标 `not_instrumented`。`native-publish-1/2`的OCI index/config digest误判及buildx缓存清理问题也保留。publish-3对已经上传的同一镜像完成验证与收尾，没有再次推送或替换镜像来掩盖失败。

真实run为 `observability-r07-native-huawei-b-1/runs/20261025T075455.544487451Z-openclaw-huawei-embedded-p0-runtime-replay`：4轮、693帧、3次真实exec、12/12独立Verifier通过，原Tape与源码未改；Stop/revoke通过。2个Sandbox及1个临时模板均确认删除，cleanup_errors为空、临时凭据清除，0上游LLM调用。华为Verifier出网仍为 `assumed-unverified`，不算已证明网络隔离的Goodput。

27条原生callback与ToolCall/native run/session身份逐项对账，3个直接子进程区间如下；序号仅表示本Tape中的步骤，不能用来替代身份匹配。

| 真实工具步骤 | spawn返回→child exit callback（ms） | exit code | 双流natural EOF与exit关系 | exit后drain区间 |
| --- | ---: | ---: | --- | --- |
| T1 | 22.511373 | 0 | 两流EOF均早于exit | 不生成；null |
| T2 | 394.434524 | 1 | 两流EOF均早于exit | 不生成；null |
| T3 | 303.746110 | 0 | 两流EOF均早于exit | 不生成；null |

非零exit表示该命令已退出，不替代整体任务结果。以上是Node原生callback观察区间，不是kernel精确起止、全后代终态或consumer/SDK耗时之和；本次没有观测到exit后的pipe等待，不能补成0ms。 每次9条callback覆盖spawn/exit/EOF/close/wait等，未新增B的stdout/stderr首输出测点；该值保持未采集，不借用A wrapper的首输出证据。

### 离线scope修正与原件复核

原分析器把任何非空 `native_observation_protocol` 都送入A专用身份解析，因此B没有 `native-tool-identities.jsonl/index` 被错误标为incomplete。原 `observability-r07-native-huawei-b-1/reconciliation.json` SHA `efa0d2514ad2e6bfc597dcce3d27f947c48602d22f3719583d07a85512535def`、原快照及 `observability-r07-native-huawei-b-perfetto-1/evidence.json` SHA `f2491fdced71317580197ac75c82292ee6856613345dfced7247d263335885f4` 保留；TP1只证明已有区间投影一致，没有把incomplete改判通过。

`observability-r07-analysis-scope-1`仅更改离线解析scope、parser版本和回归fixture：已验证embedded+`aries-native-process-v1`且A的producer/index均不存在时不要求A身份源；未知协议或意外A文件仍按原检查报错。先在旧源码复现错误，再在ECS通过performance包unit/race各60顶层、209含子项及vet/分析器build。parser为 `aries-analysis-v1.3.1-native-process`；没有重新部署Runtime、构建云镜像或重跑任务。

| 离线归档 | SHA-256 |
| --- | --- |
| `observability-r07-analysis-scope-1/evidence.json` | `0a9eadd713dc28a4c35f661ea00624c75c79451a4bfa0e0f39c22fab8197394b` |
| 同目录568文件 `source-manifest.json` | `69acfb1013e3e1ef8fdbc9cf45c61d375c40492f31d80cc335c768c740cece1c` |
| 同目录 `aries-perf` | `1efda744f1beff0d089d9de0af7df651c3ba38110c79953fde4d553d42791ea5` |
| `observability-r07-native-huawei-b-reanalysis-1/evidence.json` | `5fd4a1bdb2032c846df04cf640e6fc5eb10abe63078e931855556b60d332f177` |
| 同目录 `reconciliation.json` | `377f5f9869a779f63f4b51e78bb0f6ee2da5234d04c4d8e80de6c01ab4537335` |
| `observability-r07-native-huawei-b-perfetto-2/evidence.json` | `a36a133e507fa186feb89b06bc5bce5a2d8b443445db2bce0ac83d0f5b545d78` |

新Snapshot ID为 `snapshot-aff85934a3401ca4c437bdfbae6fff765d0ac40fc8219087e25768f377e60666`：integrity=complete、coverage=unknown、diagnostics为空。独立复核确认真实业务结果、模型轮帧、八阶段、consumer和process区间等字段与旧review精确一致；旧原件未改，变化只来自解析器scope及快照身份。

TP2从同一Snapshot分别导出主控制域878区间/31flow、native单调域3区间/0flow，两视图共用12个来源；区间集合互不重叠、并集恰好覆盖快照。实际TraceProcessor按operation ID核对start/duration/end误差均0ns、processor_error_stats为空，原始来源SHA不变。两域保持不对齐，未制造跨域父子关系或拼接工具总耗时。导出位于该TP2目录的 `exports/run/` 与 `exports/native-process/`；复核读取上列固定原run和新分析器，输出须使用独立目录，不覆盖TP1或原review。

**本次新增真实云B限定验收通过；同源双域实际UI亦已通过，见下节。** 它不替代E2B B同测点验证、后台/PTY云上终态、全进程树、独立丢事件分母、logger增量成本或P1/P2；565文件R15四对也不因此升级为新568文件B版本的观测成本证据。

<a id="r07-huawei-b-ui"></a>

### 华为B双域Trace：实际Perfetto UI验收

`/opt/aries-artifacts/observability-r07-huawei-b-ui-1/evidence.json` 状态 **PASS_ACTUAL_B_UI_SAME_TOOL_TWO_UNALIGNED_CLOCKS**，SHA `c143bb0cda1471dd430ca96268ebaa0cdaa3eed41b7f31b5c3aadac262d2d075`；`artifact-manifest.json` SHA `186e573eb833d42c973c23575aefee73f00e01e3f030213b57fafd2dae12a59c`。只使用上节TP2的两个原导出和同一Snapshot，没有改源码、重跑云任务或新增模型调用。

ECS现有Chromium/Playwright打开两个本地Trace，实际使用Perfetto UI `v59.1-1e9c61e98` 内置WASM；Query(SQL)按精确operation ID查询，再通过时间线搜索及下一项按钮选中对应slice。选择的是同一原生ToolCall `call_00_DAVxIjtRInSDaQXozcXC8208`，同时核对run/session/attempt身份，不按顺序猜关联。

| 视图 | UI SQL slice / start offset（ns） | UI及Snapshot时长 | 测量边界 |
| --- | --- | --- | --- |
| controller | slice[2155] / 38182591718 | 434299929ns，即434ms 299µs 929ns | native_event_consumer_interval |
| native-process | slice[10] / 5992780577 | 394434524ns，即394ms 434µs 524ns | native_spawn_return_to_child_exit_callback |

查询行与详情中的operation、ToolCall、clock、duration、scope、evidence_ref均对上Snapshot及原始source SHA。原生进程泳道显示 **UNALIGNED**，全后代终态仍unknown；UI的Start time相对本视图起点，不能与另一域相减。T2原有exit=1、consumer failed与进程exited原样保留，不因最终任务通过而改写中间失败。双流EOF早于exit，未新增虚假drain区间。

6张查询/选中/详情截图已拉回并实际查看，保存在该目录 `ui/{native,run}-{query,selected,detail}.png`；对应可访问性快照保留。浏览器控制台5条错误、14条警告完整留档，涉及外部加速器探测、扩展manifest CORS、WASM mprotect与GPU ReadPixels；实际导入/查询/点选成功，不宣称控制台零错。仅关闭本次 `r07-huawei-b` 会话，10个所属PID/start身份均确认消失；原run所有输入及两份Trace SHA再次核对未变。

复现使用归档 `prepare.py` 的固定TP2 SHA与 `prepared.json`（SHA `33a98303e3b48c0be2d9185c0263b15d7eba4a40f3c93f58ae50ebd3e55c4cc8`），本地文件导入步骤见 `METHOD.md`；`finalize.py`核对实际UI产物及清理记录。重验须新建输出目录和浏览器session，不覆盖本次证据。P0仍PARTIAL，UI通过不代替全树终态、观测覆盖率或成本验收。

<a id="r01-native-cancel"></a>

## 完整A原生Agent：执行中取消、身份链与评分前停止

本次是单次scripted fault，**不是Strict Replay，也不是成功编码任务**。在ECS上复用568文件 `r07-native-release-1`（gate `bd4acb1c…`、manifest `3b6450a8…`）、原A镜像 `cc7b2c82…`、SDK镜像 `abd55576…`、原E2B模板；生产源未改，只在独立副本追加两个测试夹具。准备阶段gofmt、编译与既有3项隔离测试通过；最多2次Sandbox Start、TTL240、1次Runner、1条脚本ToolCall、0上游LLM，未启动其它W2用例。

| 原始证据（相对ECS `/opt/aries-artifacts/`） | SHA-256 / 结论 |
| --- | --- |
| `observability-r01-native-cancel-1/prepared.json` | `9f585a890b1a6093a0ea9615539cb676c08b86cc822b5b4fec0eb8441f67860a`；准备PASS/0云 |
| 同目录 `evidence.json` | `d33e51ba169e571f210edee541bc27aede795d21820338db5311dab2629f9227`；Go实际用例PASS，最终保留FAIL_IDENTITY_RECONCILIATION |
| 同目录 `cases/a-native-cancel/run-result.json` | `c570f3674a2ff4a75429ebd2b219620e3bef136a800dc4a83253e3009532518a`；Harness canceled、isolation confirmed、Verifier failed、cleanup succeeded |
| 同目录 `provider-cleanup.json` | `d3594552aa82102fdd906a87e21c7e32f7c34943c4d8440e6f2c9c29d97d9c98`；2/2独立NotFound，无fallback |
| `observability-r01-native-cancel-review-2/identity-review.json` | `f9ff7f8b53782c54530848bae426c8f2f3ad1cb2934ccddb7e0b1b6b8520414d`；PASS_ONE_NATIVE_A_CANCEL_IDENTITY_CONTAINMENT_VERIFIER |

真实工具父子、setsid后代、非空cgroup及两次变化心跳先被观测，然后只取消实际 `openclaw.Manager.Run` 子context；四角色Runner保留评分context。真实Harness Stop和Bridge revoke之后，Evaluate入口先确认同PID/start已消失、cgroup空、UID60000进程0、两次心跳与mtime不变、原SSH listener拒绝连接，再调用未改的codingcase独立Verifier。未修代码12测试中11失败、0error/0skip，score0；JUnit SHA `f201cdf59cbd214a4919e7c9265ba1beec334542ae146aa05bf0d18be6ac1937`。不得把隔离验收通过写成任务成功。

原生transcript、producer/index、同channel SSH metadata、Bridge execution ID、SDK Process及guest before_popen receipt逐段exact。原生模型ID为 `aries_mock_f1418ed29720_0`，producer分母1/写入1/错误0；只覆盖这个独立脚本指令。取消后的guest terminal仍unknown，不从进程最终消失反造退出时刻。

首版检查器错误调用 `UUID(execution_id)`，但冻结 `pkg/bridge/ssh/execute.go` 实际用 `rand.Text()`；原ValueError和运行证据不改。review-2只纠正无依据的格式要求，保留全链精确一致和全部生命周期断言；绑定原review SHA `719ba19aa4f6e5ae1025cfa00a58bb94fa1d56f81e744881589da48335f12d6d`、原evidence及execute.go SHA，输出新目录，不重跑云。修正reviewer SHA `7f8d4d824c39feea9df08c66e9d9ab55113fc8553e6c3c331a7d98e76df014c4`；全部已读输入SHA复核未变。

复现入口 `/root/aries-setup/uploads/r01-native-cancel-1/driver.py`，先 `--output NEW_ROOT --inputs INPUT_DIR` 准备，再 `--output NEW_ROOT --execute-paid` 单次执行；本次driver SHA `940d1bf920df8c3a21ce70165f820b9666994defe6c3e98f1ce4890b3f2b25c1`。原目录有evidence后拒绝重跑。修正复核入口 `observability-r01-native-cancel-review-2/reviewer.py`，原始运行目录与新输出目录两个参数；重复复核须改为新的归档目录并记录新driver SHA。API凭据由既有私有配置提供，未写入公开文档；专用Adapter/清理容器、原生Harness与run network均已回收，生产服务与冻结源码未改，cleanup_errors为空。

[性能表](../performance-archive.md#native-agent-cancel)保留真实计时。直接Runner夹具没有app admission排队值；Verifier总段包含额外评分前隔离检查。该证据关闭本次单工具取消场景，不外推完整并发/重试、其它云、B后台/PTY、内核退出或全局观测覆盖。

<a id="r15-cleanup-paired"></a>

## 当前冻结版本：R15 baseline/L0 四对完成

`observability-r15-paired-6` 使用 `observability-r05-cleanup-release-1`，565文件manifest `1d3c7c5a889a90d13730b35eaea2773ced906d4d799b83b23f4aaf94135d4c5f`，五项门禁SHA `a7c09290bd6126f843d525084034991d1b84d93c659b54982227d943d18c3cdc`。原生A镜像、E2B模板及 `d716431e…` Tape与前述真实四组合相同；并发1、recorded Strict Replay、真实工具、独立评分和原停止门。

预登记顺序为 baseline→L0、L0→baseline、L0→baseline、baseline→L0，8次/最多16创建/同时2存活/TTL600秒/0上游LLM。仅修改实验源pins和调度停止规则：独立业务失败仍留分母，清理确认后继续预登记样本；清理不确定则停止接纳，无重试/补样本。验收断言及Strict未放宽。0创建的 `paired-7` 预检通过后，`paired-6` 一次执行8/8通过，每次6个exec及12/12独立测试、Stop/revoke confirmed，16个Sandbox各有provider NotFound；cleanups为空，原源/Tape/服务未变。

| 原件（均在ECS `/opt/aries-artifacts`） | SHA-256 |
| --- | --- |
| `observability-r15-paired-6/evidence.json` | `c7b56f52732f3a4670f3eda4eedba7ad42bba07090348a1ee8df056cb1e37695` |
| 同目录 `paired-analysis.json` | `de81f4c51398a02c1ee3d2a5a7adbae7abc598e1f1da5b2a5a5b2c816c6d4826` |
| `observability-r15-paired-review-3/evidence.json`：411项只读复核 | `1f3ca0e0f02498de2aefc95bb78b630198e204111f4029819e08baee4ee0ce4e` |
| 原driver | `2de0663f795819344d8475d4bb372295ea7cacf116f8cfe208321acbd2b4e5b3` |
| 输入pins | `325b130b3c77d06506f7139a45ae064c380523ab96cf36de6ce3faccc78786ed` |
| 只读复核driver | `76b8495ea8938a71ce715e6f9a3588238fffe75fc1043a3bae365adbd7b6a593` |

执行入口归档于 `/root/aries-setup/uploads/r15-current-1/driver.py`；参数为该目录 `--pins pins.json --input input.private.json --output /opt/aries-artifacts/observability-r15-paired-6 --execute-reviewed`。临时私有输入按已归档cloud-profile中的两项连接字段重建，凭据仍来自授权现有Adapter，不输出到终端。复现应使用新的独立输出目录，不能覆盖现有原件。

四个L0导出随后全部实际导入同一固定TraceProcessor：case2/3/5/8分别416/431/422/434区间，每份43条flow、21个来源；起点/时长/终点导入误差均0ns，processor errors为空，来源SHA保持不变。统一证据 `observability-r15-current-perfetto/evidence.json` SHA `41f213d3d3819922beab41041f6b99c531c60d42302f9a5e1b9be39979dea61a`。baseline观测关闭的四份不算时间线通过，UI本次UNRUN。最终本地owner与临时输入清理回执 `observability-r15-current-final-cleanup.json` SHA `66fa522b9d2d5815d81a7fa342cee2635148a1431d66758bf3295969f7a92296`；复用已确认16个资源缺席原件，没有重复云查询。

八阶段、CPU/分配/RSS及配对差值见[性能表](../performance-archive.md#r15-cleanup-paired)。baseline关闭的数据保持null；四个L0实测7轮/735帧。只有控制器共享进程及声明四类writer计量，L1/L2、全链资源和原生身份协议绝对成本仍UNRUN。无批准阈值，不称扰动合格。当前版本四对完成后不再因历史失败补跑；旧 `paired-4` FAIL及额外请求调用方UNKNOWN原样保留。

<a id="p0-mainline-l1"></a>

## 当前主线：E2B／华为 A、B 四组真实 L1 回归

同一 `observability-r05-cleanup-release-1` 冻结565文件，manifest `1d3c7c5a889a90d13730b35eaea2773ced906d4d799b83b23f4aaf94135d4c5f`；门禁证据 `a7c09290bd6126f843d525084034991d1b84d93c659b54982227d943d18c3cdc`。均为并发1、recorded Strict Replay、真实Sandbox工具、独立Verifier，L1资源采样。没有修改产品源码、Tape、matcher或原生工具策略；没有调用上游模型。

| 实际业务运行 | 轮/帧/工具/独立测试 | 原件 evidence SHA-256 |
| --- | --- | --- |
| `observability-r05-mainline-e2b-a-1` | 7/735/6/12，PASS | `4c144ac780cba5040f98ce59676e1bc97679326e5d0b25d8e69871c51101d46f` |
| `observability-r05-mainline-e2b-b-2` | 4/573/3/12，PASS | `6a2fe496be5dbeb63ce9b3450b138ac8a28be31f41ba6f927ef45e2ed319c75d` |
| `observability-r05-mainline-huawei-a-3` | 7/735/6/12，PASS | `539d70b24ab2b5165b2d7b7e33d32124785fe87e4aaa2512fc2a4ffa80030579` |
| `observability-r05-mainline-huawei-b-3` | 4/693/3/12，PASS | `2227d61c010f1e1fd2bee7a030db150d1d981c2fa050386de108df8d9f899f8a` |

路径均相对 `/opt/aries-artifacts/`，SHA对应各目录 `evidence.json`。各1个真实业务attempt，Strict/独立验证/停止撤权/清理/Observer分别通过。共8个Sandbox逐一有provider缺席确认、2个华为临时模板删除；云资源回收正常，无fallback恢复。华为出网隔离仍为 `assumed-unverified`，不升级成完全隔离的Goodput验收。

A共用 `d716431ea36e1b89b47aa0c8f091cd6e65700bc10e83522467414a0d3735cffb`；E2B B用 `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb`，华为B用 `3fc9751b0ef20c16707c6d968f7aa46a65b70b7d4302a075e9e27c8b8fd233f5`。B两云及A/B不同轨迹不作性能排名；各1次样本也不估p99或容量。

### 所有启动失败与续跑均保留

- `mainline-1`：E2B A成功，B的测试脚本错误继承A镜像到versions配置，`LoadCloud`在Prepare/create之前失败；华为两项NOT_RUN。原B ledger缺失被owner记为UNKNOWN/cleanup错误，原件不改。独立源码/日志/资源核对证明该次0云创建、0业务attempt：`observability-r05-mainline-b-setup-review-1/evidence.json` SHA `87ffbe5d82a465a14e10f765e0205b1d664df8f54ee3eefe2e5ac9c1d94b9415`。
- `mainline-2`：只删除B脚本错误的image覆盖，原B配置/镜像保留；E2B B真实成功。华为A/B均在socket.bind预检发生OSError，尚未创建模板或调用Runner，各0云创建。原件未记录errno，不能认定是TIME_WAIT；事后无LISTEN且bind成功只能证明当时复核可用。
- `mainline-3`：仅华为预检bind增加SO_REUSEADDR并保留errno诊断，再执行此前未进入业务的两格。两格真实成功；没有重跑已成功的E2B A/B，没有付费重录。

因此本轮为**4个真实业务attempt成功，另3次启动失败、2条历史NOT_RUN记录**。NOT_RUN后来执行的条目不重复计任务；启动失败不是Strict业务失败，也不能从历史中删除。静态检查中首次误加固定hostname的FAIL另留存，恢复原E2B B的cgroup/UID/current-runtime契约，Strict全文匹配未放宽。

| 调度原件 | SHA-256 |
| --- | --- |
| `observability-r05-mainline-1/evidence.json`（原FAIL_OR_INCOMPLETE） | `ad554e0baac43bd573d987d3d431fe98586c5f7f090b6e009ee3efe0be0f7fd7` |
| `observability-r05-mainline-2/evidence.json`（原FAIL_OR_INCOMPLETE） | `62d85ac5aefc5c79903bb899ea470433d330afdefb81c0ea6c188c18c93cdec0` |
| `observability-r05-mainline-3/evidence.json` | `a795780c2d1b5118b87ce3f016779e504ff35b9ffd980f4360c975c3c8fd0478` |

### 测量和真实 TraceProcessor 对账

E2B A/B分别保存12/11行采样RPC，华为A/B为11/12行；相同sandbox handle、operation ID、控制器单调时钟证明每次Metrics返回均早于对应kill开始，所有Write计数/字节及一次Sync完成。Provider更新周期与RPC周期分开：E2B有重复历史timestamp；E2B B的评分RPC返回没有provider读数，因此缺资源测量，不填零。

四个Trace的实际TP区间/flow/来源分别为435/43/21、833/31/11、428/43/21、809/31/11，起点/时长/终点导入误差0ns、processor errors为空；原业务状态与八阶段逐项一致，输入SHA未改。integrity=complete、coverage=unknown；本轮UI未重复执行。B仍只有原生事件消费区间，私有B进程PoC没有混入这次运行。

统一离线报告 `observability-r05-mainline-analysis-3/evidence.json` SHA `86f235c4e8bb00d2e2e7373750db58ce385a9a7423ea7aa0c885097dba7f8bd3`，保存全部三份调度原件、4个run的状态/八阶段/精确ToolCall投影、采样语义及TP审计。分析脚本在 `/root/aries-setup/uploads/r05-mainline-analysis-3/driver.py`，SHA `7e1d03ca49251c11f726343fa32cad7cf2475d1c0da2796a22791527a2de60ae`，调用固定 `aries-perf` 与原TP auditor；只有输入目录适配，没有另算身份或时间线。参数为 `--matrix …/observability-r05-mainline-1 --resume …/observability-r05-mainline-2 --huawei …/observability-r05-mainline-3 --output …/observability-r05-mainline-analysis-3`；现有输出不可覆盖，复现需新归档目录。

最终本地进程/临时输入清理回执 `observability-r05-mainline-final-cleanup-1.json` SHA `319023b14b292ba6391d6014ae5015021fc8f8f5970bf3d0892c41a544e59c1e`：三个driver进程均消失，私有输入已删除；复用每格独立provider缺席证据，不重复查询云。八阶段及逐工具数值见[性能报告](../performance-archive.md#p0-mainline-l1)。这次正常L1回归不证明永久I/O故障有界，也不替代R15 baseline/L0成对成本实验。

<a id="p0-rejection-diagnostics"></a>

## P0 后续取证：首个 Strict 拒绝请求摘要

`observability-model-rejection-diagnostic-2` 在冻结的 `p0-runtime-release-2` 基础上只改四个文件：`internal/modelreplay/server.go`、`boundaries.go`，新增 `request_diagnostic.go` 与测试。首个历史偏离或 Tape 末尾之外的请求，仅在原失败事件附加私有结构摘要；不改变 Strict、cached retry、响应、轮数或事件分母。失败匹配终点在构造摘要前采样；baseline 不生成摘要。字段及隐私边界见[测量契约](OBSERVABILITY_SCHEMA_AND_MEASUREMENT.md#strict-拒绝请求的私有诊断)。

ECS unit 与 race 均通过 41 项顶层测试、106 项含子测试，vet 通过；覆盖历史偏离、Tape 后额外请求、相同请求缓存重送、拒绝后的 SESSION_FAILED、凭据早拒绝、未知内容形状和正文不泄露。Strict 拒绝仍为 9 个边界事件，SESSION_FAILED 仍为 2 个；后者未读取正文，没有伪造诊断。生产服务、原生镜像、Tape、原失败证据均未修改，Sandbox/上游模型调用为 0。

| ECS 相对 `/opt/aries-artifacts/` 的证据 | SHA-256 |
| --- | --- |
| `observability-model-rejection-diagnostic-2/evidence.json` | `5000d4f5b6395379615cc5b4e4951858a24fb4e7bdd4ec63fbaaa6a32b8a2e2f` |
| 同目录 `formatted-overlay.tar.gz` | `d9403e4607b22d609c91f30bf655f4e1c213287def0b8cfe7d369db17168b016` |
| `observability-model-rejection-consumers-1/evidence.json`（分析器/调度层兼容） | `0a3e3c5de06c66559ec8bb3f894b1136af06f94fd799a74ccce0488b4d2f7bc0` |
| `observability-model-rejection-diagnostic-1/evidence.json`（首轮测试断言误报，保留） | `d4cd7e24733a9e9349e6ed286cbf2a9983d270f5f9cf91c8caa6a5813a043139` |

首轮隐私断言用 `reasoning` 搜索整个事件文件，撞上既有 `downstream_first_reasoning` 事件名；改成唯一正文哨兵后继续检查整个文件。没有删除隐私要求或修改 matcher。第二轮四文件均已按 ECS gofmt 结果同步，本地/远程 SHA 一致。

随后在独立目录复用同一冻结候选验证消费者：`pkg/performance` unit/race 各 54 个顶层、190 个含子测试通过；`internal/app` 各 94 个顶层、165 个含子测试通过。四文件和第二轮 evidence 的 SHA 未变。独立源码审查确认基线不生成摘要，成功/重试分支未改，公共 Snapshot/Trace 白名单不会复制摘要；未创建 Docker、Sandbox 或调用模型。

这是定向工程验证，**不是新云回放、性能样本或额外请求根因修复**。原 `paired-4` 仍为 4 PASS / 1 FAIL / 3 NOT_RUN，调用方仍 `EXTRA_CALLER_UNRESOLVED`。后续四文件已单独完成下述新五项门禁与真实诊断；历史失败及其调用方未解决状态不变。

<a id="p0-stop-log-follow"></a>

## P0 后续取证：覆盖 Stop 窗口的外部日志跟随

当前 `OpenClaw.Run` 在返回前采集 `gateway.log`，Docker Stop 随后会停止并删除容器；因此 Stop 后再取日志已经太晚。本次没有扩展生产 Deployment API，而是在 ECS 实验 owner 中对核验后的精确容器 ID 使用 `docker logs --follow --timestamps`，保留启动历史并持续到停止窗口；Runner 的 Stop/Remove 不等待该 observer。16MiB 文件上限、独立退出码与缺测状态均留证。

`observability-p0-stop-log-follow-2` 只用已缓存镜像中的 Python 日志 fixture，三项真实本地 Docker 验证通过：正常 SIGTERM 期间的全部固定输出在立即 Remove 后仍可读；observer 断流、超出 16MiB 两例都标 partial，仍正常删除容器。正常例 `closed_without_detected_transport_error` 只证明流关闭且固定标记齐全，**不证明任意 producer 零丢失**。Docker 时间戳是源墙钟，不构成跨域校准或 ToolCall 身份。

| 用例 | Stop 调用（ms） | Stop 返回→Remove 返回（ms） | 原始采集字节 | follower exit | 采集结果 |
| --- | ---: | ---: | ---: | ---: | --- |
| 正常停止 | 230.205 | 47.258 | 334 | 0 | 7 个固定启动/停止标记齐全 |
| observer 提前断流 | 246.116 | 47.043 | 123 | 143 | partial |
| 16MiB 上限 | 245.249 | 47.947 | 16,777,216 | 1 | partial |

以上时间来自同一 owner 单调时钟，按原始纳秒四舍五入；Stop 返回→Remove 返回区间含两次 CLI 调用间调度，不能当作 Docker 引擎内部纯删除时间。follower 启动→owner 末次 wait 返回的观察窗口分别为 350,403,886 / 367,875,277 / 933,954,543 ns；特别是断流例已提前 wait 结束，这不是 follower 的精确执行时长。

| ECS 相对 `/opt/aries-artifacts/` 的证据 | SHA-256 |
| --- | --- |
| `observability-p0-stop-log-follow-2/evidence.json` | `be67a34c9e75e0db270e8e7dee1c0dc47a78e6845cddc18528d3f87d9553e820` |
| 同目录 `artifact-manifest.json` | `1e01275f9514fb76483d411ba4ef596d236a4532e12ffb6ce2f51abba825b586` |
| 同目录 `owned-cleanup-final.json` | `a358c92bf22025df70a744a062dde62e8a0bf5ca726f03e8d06d5c3556cb3459` |
| `observability-p0-stop-log-follow-1/evidence.json`（首轮检查器 FAIL） | `ff5112db11fe67324378ae5dd311ad2f1af5e1c55df1e62d42d6e16423338b19` |

首轮仅把 Docker 返回的小写 `error: no such object` 误判为未确认消失；保留原 FAIL，第二轮只修大小写判定。最终独立只读核对两轮精确 6 个容器与 6 个 PID/启动身份全部消失，私有原始日志脱敏后删除。镜像 SHA `abd55576ec4baeef6cb47622e83b55c2b6182bf7cf793c6f1ce2c235fdb79951`，无镜像下载、Sandbox 或模型调用。

真实 driver 接入已由下述单次诊断验证；本地 fixture 不进入任务成功数、R15 配对样本或八阶段性能表。仅凭日志流正常关闭或时间相近不能确认 caller。

<a id="p0-single-diagnostic"></a>

## 新诊断版本：一次真实 E2B A 回放

`observability-p0-diagnostic-release-1` 冻结 564 文件，仅在 release-2 上增加上述四文件诊断。ECS build/test/race/lint/integration 全通过；证据 SHA `6386314c5ad36740da425d3cd8ff389681dd806c8e756eb34563041a0b694e5b`，source manifest SHA `7909603e39f1a606c864641da81d6ab8e74d07678b69882148a898356d7dad27`。工程测试无云/真实模型调用，既有 Docker 资源未变。

随后 `observability-p0-e2b-diagnostic-1` 只执行预登记的 1 次 L0、并发1、recorded Strict Replay：原 A Tape、原 native 镜像与 E2B 模板均不变，不重录、不自动重试、不补 R15 缺失样本。实际 7 轮/735 SSE 帧、6 个真实 exec 与原决策/结果逐项一致，独立 Verifier 12/12；停止撤权确认，任务与评分两个 Sandbox 已独立确认删除，cleanup_errors 为空。计划/接纳/执行/完成/验证成功均1，无失败或未启动。新源码下的一次成功不能证明旧额外请求已修复。

独立只读复核 `observability-p0-e2b-diagnostic-review-2` 的 evidence SHA `3fcdf351d0b0bb376034ebc22a0a17510094cb204aba804ff6130926efab68bd`、summary SHA `cb2d9b39b2463b5c9efd72157d41c982d4059f13787681157ed422fe3042898b`；66个输入SHA未变。本次 `NOT_REPRODUCED`：只有7个HTTP200，未出现 REPLAY_DIVERGED/SESSION_FAILED，新拒绝摘要未触发，旧 caller 仍 UNKNOWN。首个离线 review-1 的摘要脚本错误假设 runner 有 attempt_id，且直接比较 Docker short ID 与 full ID 的hash，原件保留；review-2 使用实际 run/occurrence 字段，后者明确 NOT_COMPARABLE，仅核对已归档标签/image和driver的同次inspect验证，不伪造full ID关联。这一只读修正不改变原始云结果，也没有重跑云。

精确归属校验后，外部 follower 保存 7,188 字节，正常关闭，无已检测传输错误或 partial 原因，私有原件已删除、follower 进程确认消失。日志为 Docker 墙钟、未跨域校准；流关闭不证明 producer 零丢失，也不提供不存在的额外 caller 身份。云清理及 provider 缺席确认先于 observer 收尾，日志不阻塞付费资源回收。

Snapshot integrity=complete、coverage=unknown；6 个归档 exec 身份映射与消费区间、152 个客户端区间（129 succeeded/2 failed/21 canceled）、40 个 SDK duration-only 属性完成原件对账。失败/取消不涂改；guest 时长不补造跨域时间线。随后同版分析器在 ECS 实际导入 TraceProcessor：431 个区间、43 条 flow、21 个来源，起点/时长/终点导入误差均 0 ns，业务 outcome 与原件一致、processor errors 为空。证据 `observability-p0-diagnostic-perfetto-1/evidence.json` SHA `aa552f48c74af4761022b88aa320ef132f302ac92211c1f636dff851005ec413`；原件未改，云/模型调用0。该次 UI 仍 UNRUN，不借此前 UI PASS 冒充此次执行。

| ECS 相对 `/opt/aries-artifacts/` 的证据 | SHA-256 |
| --- | --- |
| `observability-p0-e2b-diagnostic-1/evidence.json` | `ea68ad81bdac4e2f1e31e0869e298865a772a08c7f37277f1f96c44f10ad4530` |
| `observability-p0-e2b-diagnostic-preflight-1/evidence.json` | `6bb0977465aa5b5593aae846ad9234095d63da142758bf4c32b900bf0dc30eda` |
| `observability-p0-e2b-diagnostic-selfcheck-1/evidence.json` | `7924e91c2f6119838e840a30376201cc4ee26bf12c2b8bccf3470d1244300009` |
| `observability-p0-e2b-diagnostic-ordering-1/evidence.json` | `7d9bf329d5dc2b76fe47d2855f3c0b1d504095afa318e1b57b51b430d4d4ef39` |

Run：`20261025T055816.636249455Z-openclaw-e2b-external-diagnostic-01-diagnostic-l0`，位于新实验 `01-diagnostic-l0/runs/`。八阶段实值单列在[性能归档](../performance-archive.md#p0-single-diagnostic)，不混入旧 R15 配对。旧矩阵仍为 4 PASS / 1 FAIL / 3 NOT_RUN，旧 caller 仍 UNKNOWN。

<a id="r05-process-file-poc"></a>

## R05：父进程保留 producer、子进程代理事件文件的 PoC

`observability-r05-process-io-1` 仅为 ECS test overlay，生产代码未接入。真实 Recorder 和 model boundary 的 producer、clock、queue、index 留在父进程，单个 worker 只接收原事件文件 Write/Sync/Close。两类 writer 各执行正常、Write 门闩、Sync 门闩、worker 异常退出，共8例通过。正常原始字节一致，每个 writer 为2 Write/1 Sync/1 Close；故障均保留2个事件的生产分母、原 Close 返回错误、index incomplete。

四个挂起案例先释放本地 listener 和模拟 runtime，再实际 kill/join 文件 worker，使 IPC 错误解除原 writer 的等待；8个 worker/runtime、父原文件 FD 与 IPC FD 均确认回收。提前完成的业务 fixture 成功文件 SHA 未变。这证明接口隔离可行，**不是完整 Runner、真实 Sandbox 或 Verifier 故障验收**。

证据 `/opt/aries-artifacts/observability-r05-process-io-1/evidence.json`，SHA `6259168eff566febe4746627a31e05ea5f9cdaeb3a38e58d2030ee5cd29b2425`。release-2/source 副本562文件未变，云/真实模型/Docker调用0。

生产接线、启动失败回滚、关闭归属及截止时间尚未实现；父进程 hash/read/index/late marker，以及 legacy、SDK 同步 I/O 仍未隔离。当时 L1 的 `observedSandboxes.Stop` 在删除 Sandbox 前等待同步采样写盘；后续五文件修复已分开这两个步骤，见下一节。门闩位于系统调用前，不是内核 D-state；Probe 手动 kill 后的 Wait 不能推广为所有磁盘故障均能有界退出。完整有界 I/O **仍 BLOCKED**；该框架支线暂不扩展，继续真实回放与性能主线，不新增任务成功数或 R15 样本。

<a id="r05-cleanup-current"></a>

## 当前修复：L1 清理顺序与 Observer 结果分离

在原诊断版上仅调整五个 Go 文件。L1 停止采样后把唯一最终 Metrics RPC 的结果留在内存，先允许 manager 删除 Sandbox；最终写盘、Sync/Close 与真正的采样 goroutine join 仍在 observation 收尾完成。它解决文件锁挡住删除的具体问题，不声称 SDK RPC/日志或永久文件 I/O 均有界。JSON schema 与 FileIO scope 不变；Observer.Duration 是观察器启动至最终 finish 的跨度，包含收尾，不能当纯采样时长。

`runCloudAttempt` 的 cloud observer 错误改为进入 `tasks[].observer.status/error`，不再追加到任务业务错误；任务、清理、Strict/Tape 错误仍按原路径失败。新测试先在原实现实际复现“仅 observer 错误污染业务成功”，再验证修复；不是独立 Verifier 或真实云故障验收。

| ECS 归档（相对 `/opt/aries-artifacts/`） | 结果与 SHA-256 |
| --- | --- |
| `observability-r05-metrics-cleanup-1/evidence.json` | unit/race 各21顶层、含子项32，vet PASS；Write/Sync 门闩未释放时 manager 已删除，释放后才确认实际 join。`7f31f4870878940e01df12b76c23e75eee43467dae8877a2607b81eb4b2da987` |
| `observability-r05-cloud-observer-outcome-1/evidence.json` | 原实现预期 FAIL 留存；修复后 app unit/race 各169项（含子项）、vet PASS。`a024f04e8369a77c5572a1c6da1a5b4c5898d36bd5d3cb10420423fa75f5d44f` |
| `observability-r05-cleanup-release-1/evidence.json` | 同一565文件源码 build/test/race/lint/integration 全 PASS。`a7c09290bd6126f843d525084034991d1b84d93c659b54982227d943d18c3cdc` |

新 source manifest `1d3c7c5a889a90d13730b35eaea2773ced906d4d799b83b23f4aaf94135d4c5f`；五门禁耗时分别 9.762/26.370/71.376/3.128/367.054 秒。上述工程测试云/模型调用均0，既有 Docker 清单和源码未变。四格真实 L1 回归另记，不用工程 PASS 代替云验收。

<a id="r01-current-protocol"></a>

## R01：真实 SSH 取消、重试及身份拒绝

同冻结诊断版先通过当前真实 SSH race 验证（3包、10顶层/21含子项）；再在独立 E2B Sandbox 验证5次协议请求：收到首输出后取消、旧 invocation 重连拒绝、wrong scope 拒绝、wrong session 拒绝、同 ToolCall 新 invocation 重试成功。两次接受调用各有精确 Bridge/SDK Process/guest receipt 身份；重试 Process ID 与执行 ID 均不同。取消没有观测到 guest exit/时长，不能视作完整进程树已停。另1次固定 workspace 准备调用保持 unlinked，不用命令或时间猜 ToolCall。

第一轮取消探针因 Go 嵌入 `bytes.Buffer` 的 `ReadFrom` 绕过自定义 Write，未触发 READY 回调，1次执行而余4项 NOT_RUN；原 FAIL 保留。零云 before/after race 复现探针问题后仅修 fixture，第二轮5次真实协议成功。离线 checker 首次还误把辅助准备调用计入业务分母，并误读审计转义；修正实际字段解析后重对同一原件，不改变云结果。两轮各1个 Sandbox 均独立 NotFound，0 LLM，无 Gateway/独立 Verifier，不计完整任务成功。

| ECS 归档（相对 `/opt/aries-artifacts/`） | SHA-256 |
| --- | --- |
| `observability-r01-protocol-current-1/evidence.json` | `430f60f7020c5de3dc2ec8f8cd007d6e650fd0f023c6e9a3a7a71ff5de4b0f5b` |
| `observability-r01-protocol-cloud-1/evidence.json`（首轮 FAIL） | `a06c00b4624c8cec4fda14b443f6790e54565e2cbfa41ab0c062225b370a69e6` |
| `observability-r01-ready-fixture-1/evidence.json` | `741a3f1502fe7138b629f8378a0d5518773ea1c661ee679efbb2102e2d1c1eec` |
| `observability-r01-protocol-cloud-2/evidence.json`（原件） | `122c2d44619c8e3a72375ec5e3636794c6460aaf4ab47ca8d4ccce5e042a3d1d` |
| `observability-r01-protocol-cloud-review-4/evidence.json` | `2af8c8fa8d1a77b0f2e8b5e4cc6c498b7bfffea44c030293bd7181951fd14806` |
| 同目录 `reconciliation.json` | `34440bbd1446f010883496102d3ea490f09f0bbf3216470ceacdb64a05b11ab9` |

通过范围仅真实 SSH/Bridge/SDK/guest 协议，不包含 native producer 或整 Agent 取消。检查器修订历史另存 SHA `eca12e55688e8a96a84bcf96f7af13fb96aa1d665ef5f4b05c43c5665e686c29`；两次中间离线尝试未持久化终端日志，脚本保留，不宣称该部分日志完整。

<a id="r07-native-observer-poc"></a>

## R07：B 原生进程退出与管道排空的私有 PoC

从真实华为 B 镜像 `d33b073f…` 提取原生模块，只在私有副本中修改 main、exec-runtime、supervisor 三文件，PTY adapter 原文不变。ToolCall ID 经原生调用参数关联 process sessionId/supervisor runId/PID；时间来自同一 Node 进程的单调时钟，含义是回调被观察到的时刻，不是内核精确退出时刻。

两个 ECS 本地容器按原版/候选各执行5个原生案例：前台、直接子进程退出但后代持管道的后台/poll、原生 kill、观测 sink 抛错、PTY。业务对照一致（易变 PID/时长不作 Strict Tape 比较），候选采到3次 exec 共27个真实生命周期事件。后代用例中父进程已经不存在、后代仍活着且 poll=running；实际 child exit 到最后自然 pipe EOF 相差311.383578 ms，证明两者必须分开。sink 连续抛错9次仍返回原工具结果；PTY 双流 EOF 明确未采集。

另外只用 adapter-body 测试替身验证 fallback 与 close≠EOF 分类，不宣称真实 SIGKILL 失败；fallback 缺实际 exit/EOF 时保持 UNKNOWN。容器均退出0、精确 owner 容器及已知 fixture PID确认消失，cleanup为空，0云/LLM，无生产文件修改。

`/opt/aries-artifacts/observability-r07-native-observer-poc-run-1/evidence.json` SHA `5e98d6a5084e2824309ce58e4ccc82591e754575f74213ddd73a9d2c1c2ae164`，`comparison.json` SHA `881dba9b53b15cfca7bbeab66e89a9108ce626f5324987b3149e697eaf6feb10`。同目录保存候选/还原 manifest 与 `RESULT.md`。在该历史PoC时，生产日志跨Run→Stop、Snapshot导入、云端回放均UNRUN；后续接线及[真实华为B验收](#r07-huawei-b-native)另有绑定版本的证据。本PoC本身不等于R02/R07全部完成，也不计完整任务或性能配对样本。

<a id="p0-runtime-candidates"></a>

## P0 并行增量：A 原生工具、SDK 调用计时与华为 EOF 修复

三个分支复用已有组件；分支定向结果与随后合并门禁分开记录，真实回放仍须使用同一冻结版本验收。

| 候选 / ECS evidence.json | 已通过范围 | SHA-256 |
| --- | --- | --- |
| `observability-r07-native-tools-1` | A/B共用collector；4包unit/race、integration tag仅编译。实际WebSocket客户端→产物→Snapshot→Trace fixture；baseline、缺result、墙钟倒退、超限继续drain、B观察落盘错误不改业务结果 | `d6446941ced232f843f61f99cb6eb23aa718ea5fabf770bfb55b0142e1f872fe` |
| `observability-sdk-operation-clock-2` | 4包unit/race；同Clock客户端区间、反序完成、独立成功/失败、旧源duration-only及错误时钟拒绝 | `52676c7ec661889abf806eaf68d757e8129f854453ebf5bf82d81232c39ae7e6` |
| `observability-r01-stdin-fix-1` | 两个固定SDK版本的EOF NotFound分类，E2B42/42、华为16/16，无skip | `81f2343a99124155244476a13469ce9d7b0c4a418130c38e8ec945c1df761bab` |

三项均无云/上游模型调用，源码与日志独立归档。SDK第一轮测试误把旧源“文件完整、计时缺测”当成必须导出报错；保留失败证据，修正该测试断言以符合既有integrity/coverage双轴契约，未修改Strict或原始运行结果。新组合云回放与成本对照分别留证，不借用旧R15四对成本证明新A collector的扰动。

合并候选 `observability-p0-runtime-release-2` 已通过全部五项工程门禁：

| 命令 | 结果 | 耗时（秒） |
| --- | --- | ---: |
| `make build` | PASS | 10.15 |
| `make test` | PASS | 26.27 |
| `make test-race` | PASS | 71.46 |
| `make lint` | PASS | 3.23 |
| `make integration` | PASS | 368.26 |

562个源文件；相对R05 gate2为26个覆盖文件、3个旧collector文件删除。8个Python文件与既有42/16测试输入逐哈希相同，复用其证据。来源/fixture未变，门禁进程组为空、容器/网络盘点恢复，无云或模型调用，未部署生产。

- `evidence.json` SHA：`d893954c0a56ef640733aa762a549daca3780a90497a00ce3949c20bd54369fd`。
- `source-manifest.json` SHA：`68a6d81f3564992ceb5b0d0c5f97e214fb498751f3f3da25736f7deeefc09528`。
- Runner SHA：`9baad2320e05cdc8811e9aa1824fe64f8fc339303c221d4e46a6cc57de8fd882`；分析器 SHA：`74b302785516e66308a76b51e05a9b1cc834d3f25cfa5bc1d1890ca4ed448a5c`。

首轮 `runtime-release-1` 因驱动再次设置过长TMPDIR导致Unix socket测试失败，原件SHA `deb7a47d6bfc49abc7b4c1f50b148d09af88eab74c5622654f37251acc097a67` 保留。第二轮仅撤销TMPDIR覆盖、换新输出目录；源码和测试断言不变，未跳过失败测试。上述PASS只证明合并工程门禁，不证明P0全链或真实云回放成功。

### 合并版本：华为 A 同 Tape 严格回放

`observability-p0-runtime-huawei-replay-1`绑定上述release-2、原生镜像`cc7b2c82…`及原Tape`d716431e…`，未重新录制或重试。实际1次任务，7轮/735帧、6次真实exec、12项独立Verifier通过；候选SHA仍`181c011a…`。停止/撤权confirmed，2个Sandbox独立GET404，临时模板与自有ECS容器/进程均确认回收，cleanup_errors为空；0上游模型调用。华为Verifier出网隔离仍是显式假设。

6/6归档exec精确关联；6个原生消费区间和145个客户端操作区间已逐项对账。客户端125 succeeded、4 failed、16 canceled；47条SDK duration-only属性为43 succeeded、4 failed。失败工具及EOF调用错误保持原值，不因最终任务通过而涂改。Snapshot integrity=complete、coverage=unknown，diagnostics为空；这不证明原生采集零丢失或全部后台进程终态。

| ECS相对`/opt/aries-artifacts/`的证据 | SHA-256 |
| --- | --- |
| `observability-p0-runtime-huawei-replay-1/evidence.json` | `77439a39cfb9d4385b87587866c53e1e795a79370ad619e29014217f06ef2ebc` |
| 同目录`reconciliation.json` | `e518048042a1d6bc199f194ee65616819ed97a4fb56d90cb19307125ae1d5a6d` |
| `observability-p0-runtime-perfetto-3/evidence.json` | `ce41cf3be129a403dc099872c6ba3b01866c4f59a17ab18163a1e476304d78ec` |

真实run：`observability-p0-runtime-huawei-replay-1/runs/20261025T045256.655160140Z-openclaw-huawei-external-p0-runtime-replay`。离线导出使用同release分析器，真实TraceProcessor导入1474事件、408区间、43条flow，21个源SHA核对，起点/时长/终点误差0ns，无processor错误。原件未改；native消费、SDK客户端、guest属性仍分别标明scope，不能相减伪造网络耗时。

### 合并版本：华为 B 原 Tape 严格回放

`observability-p0-runtime-huawei-b-replay-1`使用同release-2、原B镜像`d33b073f…`、原`record-3` Tape SHA `3fc9751b0ef20c16707c6d968f7aa46a65b70b7d4302a075e9e27c8b8fd233f5`。仅一次实际运行：4轮/693帧、3个真实exec、独立Verifier12/12；3个工具退出码0/1/0与原轨迹一致。Stop/revoke confirmed，2个Sandbox GET404，临时模板/自有进程/容器/私有输入已回收，production/source/tape未变，cleanup_errors为空。华为Verifier出网仍假设未验证。

B使用原生工具与原输出规则，没有强加A身份协议。原生消费区间3个，客户端操作519对（451 succeeded、47 failed、21 canceled），SDK duration-only属性117条（113 succeeded、4 failed）；integrity complete、coverage unknown。客户端操作包含环境/产物/评分等受管操作，**不是519次Agent工具调用**；B原生3个工具没有A链路的guest进程计时，不拼接假关联。B Tape与A不同，只是兼容回归，不比较两列性能高低。

原driver的Runner exit0、reviewer exit1，状态FAIL保留：检查器用字符串直接比较UTC与+08:00时间，错误拒绝“评分沙箱创建晚于答案封存”。新checker按整型UTC纳秒比较，实际创建晚25,649ns；跨时区同瞬间与1ns差别均有真实断言。仅离线重新核验，没有第二次云运行、没有修改业务/Strict/隔离标准。

| ECS相对`/opt/aries-artifacts/`的证据 | SHA-256 |
| --- | --- |
| `observability-p0-runtime-huawei-b-replay-1/evidence.json`（原检查器FAIL） | `4dfeb12e143f0b3d02173bd63572f9b8f6664e62d8a926f78b476b30ff5e490a` |
| `observability-p0-runtime-huawei-b-review-2/evidence.json`（离线PASS） | `4c4f4e11100d36efcfd54948a8d63590e1e30ebb6e23ee36f8b14070f142e1a6` |
| 同目录`analysis/reconciliation.json` | `d4ed90fc3506d1ce062f86d3900bd7c712572db32321a9879f0f504424f2eb12` |

真实run：`observability-p0-runtime-huawei-b-replay-1/runs/20261025T045710.048220160Z-openclaw-huawei-embedded-p0-runtime-replay`。没有独立归档完整B ToolResult正文；成功Strict请求历史与原生工具metadata、答案SHA、独立Verifier共同验证本次重放，不能宣称另做了完整wire文本对比。

### 合并分析器：旧失败结果保留与检查器修正

旧华为A失败原件使用新分析器离线重导，业务FAIL及缺测状态不变。81条旧SDK end记录与`untimed_operations`逐项核对，包括成功wait和失败stdin；没有Clock锚点，只保留duration/outcome，不生成slice。实际TraceProcessor仍102事件、32区间、23flow、19来源，误差0ns；两个导出exit1、完整性incomplete均保留。

- `observability-p0-runtime-perfetto-2/evidence.json`：`03fd9b2187d9e1b8106570a402d3c03df2b18b2dfff29c9b759a358939a1d6e6`。
- `observability-p0-runtime-legacy-audit-check-1/evidence.json`：`89b7d2821ece5ef3c862dbff7469dea2ed7d007076be08e7876c0cd10c4c587b`。ECS真实81行原件通过；删掉所有结果、把failed改成succeeded两个负例均拒绝。
- 先前`runtime-perfetto-1`仅核对已导出的untimed行，缺少独立end行分母，证据`b4404e1f28b054e94d0563dd6ee6e2b6aec007973f01f3873ea5d27b84f87842`保留，不能作为分母验收。后续只补检查器分母，未改源码或云结果。
- 共享replay reviewer此前把Go `omitempty`省略的正常`problems`字段当必填，修正为可选读取；非空问题仍拒绝。实际Go SDK fixture重现旧KeyError并通过新检查，证据`observability-r01-review-sdk-check-1/evidence.json` SHA `178430664061dd71b96610a4f5dcd27d7a7aef1cf4fc996457db72fde5c508a0`。Strict、业务及隔离断言不变。

### 合并版本：R15四对计划中途失败

零云预检`observability-r15-paired-3` PASS，证据SHA `801068e4f0daa0ce9aa15cf5c8b9f6d9453cd05397471d802ad83c86beac9732`，实际创建0。执行使用`observability-r15-inputs-2/pins.json`（SHA `079a58648497c8709e237a038508e811f2886c04b08d9757a9a2ac1ee28f71a5`）、固定driver SHA `9d5dfa16aae7653f08ef439272ee4252ee2639bfffb16ecbb6b2456a3a55fd01`及共享reviewer `8c5d9066…`；native镜像/Tape不变，Adapter使用同release的只读Python覆盖。预登记4对交错、8次、最多16创建/同时2存活、TTL600、0真实LLM；首次失败即止，不替换样本。

实际`observability-r15-paired-4`为**FAIL**：第1–4次PASS，第5次FAIL，第6–8次NOT_RUN。创建10个Sandbox均独立NotFound，自有runner/adapter/container/network消失，cleanup_errors为空，源码/二进制/Tape/生产服务未变。原始Runner分母为5 planned/admitted/started/executed/completed、5 verified_succeeded、4 succeeded、1 failed；未启动的3个driver计划不伪造为Runner零attempt。第五次独立Verifier12/12通过，但模型session失败，因此不能把5个验证成功写成5个完整回放成功。

| case / 原预登记顺序 | 真实run ID（位于同case的`runs/`） | 整体结果 |
| --- | --- | --- |
| `01-pair-1-baseline` | `20261025T050102.884880973Z-openclaw-e2b-external-r15-01-pair-1-baseline` | PASS |
| `02-pair-1-l0` | `20261025T050150.036001881Z-openclaw-e2b-external-r15-02-pair-1-l0` | PASS |
| `03-pair-2-l0` | `20261025T050235.477728046Z-openclaw-e2b-external-r15-03-pair-2-l0` | PASS |
| `04-pair-2-baseline` | `20261025T050319.016020836Z-openclaw-e2b-external-r15-04-pair-2-baseline` | PASS |
| `05-pair-3-l0` | `20261025T050403.476570781Z-openclaw-e2b-external-r15-05-pair-3-l0` | FAIL：额外模型请求 |
| `06-pair-3-baseline`、`07-pair-4-baseline`、`08-pair-4-l0` | 无实际run | NOT_RUN |

case2/3/5原始transcript的6个工具决策与完整结果、最终assistant内容SHA均与录制一致；无compaction/timeout/abort。case5前7个请求、735帧、HTTP200正常结束，随后在Harness.Run返回后的Stop窗口出现第8请求：1次REPLAY_DIVERGED，之后4次SESSION_FAILED。Strict拒绝保留；caller仍待确认，不能称工具输出偏离或Verifier失败。原来的`task_not_validated`只是driver泛化错误标签。

后续固定镜像源码只读审计状态`EXTRA_CALLER_UNRESOLVED`：`observability-r15-native-followup-audit-1/evidence.json` SHA `f6563131780e6c7b295b6d6961794b220b0ecb2ac8d933c69cc95ea9f76ec750`，8个源码文件及行号、5个运行输入SHA未变。dashboard标题入口要求`dashboard:`会话，本次不适用；exec完成唤醒要求backgrounded，六次实际foreground completed不满足。周期heartbeat默认30m但首轮按device哈希相位调度，缺失HEARTBEAT.md不阻止，是可疑机制而非本次根因证明。

原件没有第8请求的结构/调用身份，也没有Run返回后到Stop完成之间的Gateway日志，无法恢复已删除容器的这些数据。最小后续取证是保留拒绝请求的去内容结构摘要及实际调用身份（不存在则unknown），并覆盖Stop窗口的Gateway/native日志，再做单独复现；不新建框架、不关闭原生功能、不放宽Strict、不把后3次补跑拼回原失败矩阵。

两对完整样本单独计算，失败/未配对不补零、不用新成功替换。共享SELF CPU的L0增量为71.333ms、67.836ms；Runtime差−1.127s、−0.359s不能称负开销。样本量不足、无预批阈值，性能合格未判定。原分析器的`NOT_ESTIMATED_FOUR_PAIRS`固定文本不代表四对已完成；独立复核明确planned=4/complete=2/CI未估。阶段与文件IO实值见[性能报告](../performance-archive.md#p0-runtime-r15)。

| ECS证据 | SHA-256 |
| --- | --- |
| `observability-r15-paired-4/evidence.json` | `78f4a30e77d33c137f77dfe71bfd0a72718021fd6c8cfcf8dccfbd7646bf285c` |
| `observability-r15-case5-analysis-1/evidence.json` | `6e921883c15fc26c8c897846c694902ae4ac5d49c115a15851b0fdba39e42636` |
| `observability-r15-paired-review-2/evidence.json`：236项独立原件/分母/部分算术复核 | `30e2e1cbfb0cc843a72ed6e50c18aaf7b561420343ae97c36f8777cfdaa0e447` |

### 合并版本：新增Trace的真实TP与UI验收

各导出使用同release-2分析器、原始数据只读。实际TraceProcessor按每个operation核对source SHA、起止/duration/outcome和flow端点，所有条目导入误差0ns，无processor错误；原始Attempt分母、失败/validated状态不改写。以下证据目录位于`/opt/aries-artifacts/`，文件均为`evidence.json`。

| 目录 / 输入 | 区间 / flow / 来源数 | SHA-256 |
| --- | ---: | --- |
| `observability-p0-runtime-perfetto-3`：华为A | 408 / 43 / 21 | `ce41cf3be129a403dc099872c6ba3b01866c4f59a17ab18163a1e476304d78ec` |
| `observability-p0-runtime-perfetto-4`：华为B | 740 / 31 / 11 | `d6df71ad7988dcc0e2b641b061dcb3ef903ad8b7cd1cad38a63dde44ecb010de` |
| `observability-p0-runtime-perfetto-5`：R15 case2 | 413 / 43 / 21 | `c299a36c046fbdf68b4e7a4f204480a56e8908f642e683b9b55d1b72c314fe55` |
| `observability-p0-runtime-perfetto-6`：R15 case3 | 412 / 43 / 21 | `3a39408622b44e1e72149c7e644865456486c67cd63480204f1359c5d7b14a81` |
| `observability-p0-runtime-perfetto-7`：R15 case5失败 | 428 / 47 / 21 | `70c63306886c06d8901923eb592b93f20431008e3c86920ba557eced76e9c827` |

五份Snapshot均integrity complete / coverage unknown。失败样本的Trace完整只表示原始事实正确投影，不能转成业务PASS。baseline不执行可选Trace验收。新版离线audit driver SHA `4462a091c5313c710b7a67247ba31160e88c27586161604afd89fe53ce3eaf6a`，原生TP SHA `58042408e6cc861fb1a731c26bb082dc222285561eaa4e12a48a8b2b90dca7b9`。

新A实际Perfetto UI v59.1验证：SQL后真实搜索并点选native_tool slice168（726899118ns）和sandbox_client exec.wait slice9（774430960ns），operation_id/scope/clock/evidence_ref与Snapshot一致。它们是两类区间的**独立示例**，不是同一Tool的分层；A T1对应的client_wait实际为134003324ns。6张截图、操作/查询日志与METHOD归档于`observability-p0-runtime-ui-1`；证据SHA `778119f1170b06ca90e88326a60dc10b291e2b1b105bf6932195972139077144`，65件manifest SHA `6f6302d97057b48d3c141d56a1f3d97020a94bad3c03a91236a0cefb1536746f`。UI初始化的4错误/9警告（accelerator/CORS/WASM）原样保留，实际查询/点选成功不等于控制台零错误。

专属browser close exit0，10个已记录进程按PID+启动tick全部消失；原run及Trace/Snapshot SHA未变，0云/模型调用。该UI结果不代替B实际UI，也不改变R15失败矩阵的状态。

<a id="r01-huawei-current-failure"></a>

## R01：修复前华为 A 同 Tape 回放失败（2026-10-25）

`/opt/aries-artifacts/observability-r01-huawei-replay-1` 使用当时 R05 gate2 源码
`7352037b…`、Runner `434a3905…`、已发布 native 镜像 `cc7b2c82…` 与原 Tape `d716431e…`。
计划/接纳/启动/执行/完成均1，失败1、验证成功0；共4个模型HTTP请求，完成1轮、29帧，
随后1次 REPLAY_DIVERGED、2次 SESSION_FAILED。只有1次 native exec 被调用，题目命令
尚未真正执行，不能用失败样本的短 Runtime 与成功回放比较性能。

根因定位到 transport：同一 execution 的工作目录验证已得到 guest exit0、stdout22字节，
`exec.wait` 成功且退出/排空边界齐全；并发的 SDK `commands.close_stdin` 失败，被合并为
Bridge exit255，OpenClaw 再包装成“workdir unavailable”。不是目录缺失的证据，也不是
双流排序或 Strict matcher 错误；原归档没有 SDK 异常类别，具体原因仍需短探针确认。
Strict 拒绝的是该真实 ToolResult 与 Tape 内容的差异，未做宽松匹配。

独立 Verifier 仍实际执行12项：11失败、1通过、score0。停止/撤权 confirmed，任务与评分
Sandbox 均独立 GET404，临时模板确认删除，cleanup errors为空，生产实例未改；上游模型
调用0，华为评分出网隔离仍显式假设。八阶段见[性能报告](../performance-archive.md#huawei-current-failure)。

| 证据（上述 ECS 目录下） | SHA-256 |
| --- | --- |
| `evidence.json`：原运行FAIL | `2769d2a8c3cb74bdc1e9313749f6661584961d382989a1295d796a8a881c4db1` |
| `failure-analysis-2.json`：原件定位/分母/八阶段 | `8ceff4caecfb35b75efc6d709be6b97235b55f66ba39b5e52363add64a7d5df4` |

首份 analysis 原件保留；新版通过 supersedes 明确纠正了HTTP计数和目录误判，不改运行结果。
执行件为归档 `driver.py`，SHA `7429f91d077d0092719a00d999a037182d5da31409c239d7accd7249f763c8ab`；
只有一次完整回放，没有付费重录。后续 SDK 探针是独立实验，不混入这次任务的通过分母。

随后同SDK短探针给出直接证据：已退出短命令关闭stdin返回 `NotFoundException/NOT_FOUND`，
等待EOF的存活命令关闭成功、输出 `ready\neof\n`、真实exit0；排除协议整体不支持。
该证据证明同类机制，不能恢复原运行未保存的异常类别。两次探针各1个Sandbox及临时模板，
TTL300秒、均独立404；第一次漏设生产路径已有的cwd，提交即INVALID_ARGUMENT，FAIL保留。
第二次直接复用生产Adapter.start参数与wrapper，无宽松匹配或执行重试。

| 独立证据 | SHA-256 |
| --- | --- |
| `observability-r01-huawei-stdin-probe-1/evidence.json`：输入错误FAIL | `1da286e73d9d3dbc19f1fc33a7a4440722ec75cd459ffc623bea65a96a5ad6f2` |
| `observability-r01-huawei-stdin-probe-2/evidence.json`：真实短/长进程 | `6d644e5ecc6ae2cb5f56f07b58e13de707af5cc3f22918a515aac6bd65d2ee45` |
| `observability-r01-stdin-fix-1/evidence.json`：两版本离线回归 | `81f2343a99124155244476a13469ce9d7b0c4a418130c38e8ec945c1df761bab` |

候选仅把EOF close的明确SDK NotFound转为既有stdin_closed409；Go仍等待真实完整终态。
超时、权限和发送数据错误不吞掉；SDK调用原始failed状态仍可观测，不冒充一次成功RPC。
8个Python文件含所有构造调用与测试，ECS固定SDK镜像、禁网容器验证E2B42/42、华为16/16，
无skip、容器确认删除、0云调用。修改后的同Tape完整云复测见上方合并版本记录；原回放FAIL不改写。

同一失败样本另经当前gate2分析器导出、实际导入TraceProcessor：102事件、32区间、23flow、
19个来源；起点/时长/终点导入误差0ns，无processor错误。失败attempt、validated=false及三次
模型拒绝在原件→Snapshot→Trace→TP一致。SDK原始成功exec.wait/失败stdin仅绑定来源SHA，
当时尚未投影结果/区间，不能称全链可见。原native缺测与identity incomplete保留，两个导出退出1；新分析器的duration-only结果保留另见上方记录。
`observability-huawei-failure-perfetto-1/evidence.json`，SHA
`a76c25b3fe5ade2aa410615edd80859d5f48e968c855bc3bcc887dad00f05372`；状态
PARTIAL_NATIVE_IMPORT_PASS_SDK_OUTCOME_PROJECTION_BLOCKED。原件未改、无新云/模型/UI实验。

<a id="r05-fsync-containment"></a>

## R05：两个最终 fsync 挂起的真实资源清理验证

`/opt/aries-artifacts/observability-r05-fsync-containment-1` 使用当前冻结源码
`7352037b…`、Runner `434a3905…` 与同一 Tape `d716431e…`，执行两个 L0 场景。
生产代码未改；每例只对自己的 Runner、指定 JSONL 的 fsync 注入60秒 ptrace 延迟。
实际通过 PID/TID、FD 对应路径、TracerPid 和 syscall 74 确认命中；发送 TERM 后超过5秒
仍在同一调用，随后结束自有 Runner/tracer并清理所属资源。没有等待完整60秒，也没有
注入内核 D-state；被中断的 strace 没有完成行的 DELAYED 注记，原值 false 保留。

| 延迟目标 | 故障期间的实际资源 | 实验 owner 补充清理 | 结果 |
| --- | --- | --- | --- |
| `model/request-boundaries.jsonl` | 两个 Sandbox 已独立 NotFound、Harness 已不存在；仍有1个 run network | 删除该网络 | PASS_FAULT_CONTAINMENT_ONLY |
| `performance/runner-events.jsonl` | 两个 Sandbox、Harness、run network 均已清理 | 无剩余云资源 | PASS_FAULT_CONTAINMENT_ONLY |

每例均留下7次成功 Strict 请求、735帧与12项独立 Verifier 原始证据，候选 SHA 与录制一致；
但最终 index 和 run-result 缺失。因此两个 workload 均为 `INTERRUPTED_BY_FAULT_OWNER`，
Observer 为 `INCOMPLETE_MISSING_FINAL_INDEX`，不计入完整回放成功分母，也不伪造八阶段汇总。
共4个 Sandbox，TTL600秒，最终逐项独立 NotFound；两个 Runner/tracer 进程组消失，
cleanup errors为空，8082无监听，生产实例未变，真实模型调用0。

| 归档证据 / 执行件 | SHA-256 |
| --- | --- |
| `evidence.json` | `ed245bf72c90ad8d9941a254a9bba9bf8370e6602743d918ef6664e7f9b7b3ef` |
| `provider-absence-final.json` | `762f4d20f3b3a2db445bd0cf91ffd42288f610472ba486965b40da77f5418476` |
| `driver.py` | `30e4e176caf572b73587d95c5a3e60a03fd9eac3edeabcf86b224ddf4221244a` |
| 零云 attach 探针 `observability-r05-strace-attach-1/evidence.json` | `d63d3006c0752fae63b60d5d45cd8f3d35b07478ffb36420bc37daaaf603a718` |

复现仅在 ECS 使用归档 driver，传入原有私有 settings 的 `--input`、新的 `--output` 和
`--execute-reviewed`；它复用既有 owner/ledger 清理，须与其它8082实验串行。
源码顺序与实测相符：`runCloudAttempt` 先关闭 experiment、后关闭 model proxy，
run级 `cloud.Close` 要等 attempt 返回；`runWithPerformance` 则在共享清理后关闭 Recorder。
本次证明受控延迟下的资源状态和实验收容，**不证明生产 Close 有界**。普通 Write/Sync
不可取消、late marker/断电耐久仍 BLOCKED；L1同步采样写盘挂起不在本次覆盖范围。

<a id="r05-observer-separation"></a>

## R05 已返回文件错误分离与实际 CLI 验收（2026-10-25）

此次仅在 `r15-release-1` 冻结源码上修改模型服务、对应测试和 CLI 三文件；
源码 manifest SHA `7352037b62b7c3bc2f8c92ca7a8d289c4db3a9749a6c67585466f97622a4ee5e`。
`Server.Close()` 保留 Strict/未完成会话和必需 Tape 写入错误；legacy 事件文件的已返回
Write/Sync/Close、短写、非法 n 由 `ObservationError()` 锁存，CLI 独立记录 observer 失败。
HTTP/SSE、Strict matcher、Tape 格式、工具、Verifier 与隔离语义未改。

| 最终 ECS 源文件 | SHA-256 |
| --- | --- |
| `internal/modelreplay/server.go` | `349fb586a5d8416d563fefa7fe1b225192bf1685b57e185015487c63de13c530` |
| `internal/modelreplay/legacy_file_io_test.go` | `2674b65b2d56acbec127dfeac2aff3a6385c3f57451806d437b40d4b7d024e62` |
| `cmd/aries-model-replay/main.go` | `9e8a3a6237cb67963fe56b22ef980dfd4f88923b25c4d1b0f56affa53340c611` |

首轮 `/opt/aries-artifacts/observability-r05-observer-separation-1/evidence.json` 为 **FAIL**，
SHA `c689e9be4189f9fa042e5eed3318e87059450c07c6324f715c9b35a7a24e8c74`：定向测试和编译通过，
完整测试因驱动设置过长 TMPDIR、Unix socket 路径超限失败。保留原件；第二轮仅移除该
TMPDIR 覆盖，源码与验收断言不变。`observability-r05-observer-separation-2/evidence.json`
SHA `1346011f67f399147d683e1ad9ed956a9e489810f2a64335083104acf47bb6cc`，定向测试及
`make build/test/test-race/lint/integration` 五项均 **PASS**；源/fixture 未变，测试进程组为空，
容器与网络前后盘点一致，未部署生产、无云或上游模型调用。

包级测试使用真实临时文件包装器注入 ENOSPC、nil-error 短写、Sync/Close 错误与非法 n；
验证独立计数、incomplete 状态、重复 Close 不重写与错误锁存。另有 Strict 偏离/未完成
负例，以及本机 HTTP fixture 的正常/ENOSPC × Tape 正常/冲突四案例：请求和 SSE 保持
原值、cached retry 不重复请求上游、有效 Tape 可严格回放，错误 Tape 仍失败且不覆盖。
这些是可控故障注入，不是生产磁盘耗尽或真实模型录制。

实际 CLI 使用上述构建的 `source/bin/aries-model-replay`，SHA
`f2fb19b2290dff90b279a3123f949e7293cfbfcac0d80acfb6d35dc206bee7fa`，复用 `full-agent-2`
Tape `d716431ea36e1b89b47aa0c8f091cd6e65700bc10e83522467414a0d3735cffb`，loopback、zero timing：

| CLI 案例 | 实际结果 |
| --- | --- |
| 正常 | 7轮/735帧逐字节一致，退出0，无observer失败 |
| `events-file-io.json` 发布冲突 | 同样7轮/735帧一致、退出0，记录`observer_status=failed`，原文件不覆盖 |
| Tool历史偏离并叠加发布冲突 | 首轮正常；修改下一轮已有Tool Result后409 `REPLAY_DIVERGED`，正确重试仍409 `SESSION_FAILED`；退出1，业务与observer错误均保留 |

CLI 证据 `/opt/aries-artifacts/observability-r05-cli-observer-1/evidence.json`，SHA
`39ffc6bf9887c4e24d86111463a56cfbda4cab1bed2f554c60a570ae69444646`。三进程组与监听均确认消失，
`cleanup_errors=[]`，binary/Tape 未变。此 CLI 故障是侧车发布冲突，**没有注入 legacy Write 故障**；
不把包级注入与实际 CLI 案例混为同一种端到端故障。没有启动 Agent/Sandbox，不增加性能任务分母。

复现限 ECS：使用 `/root/aries-setup/uploads/ecs-r05-observer-separation-2.py`
（SHA `85597b92fe1735bf6f6e75ca9d744195ecf3dcc987213cf0919b51ea814de6c5`），传入归档
`r05-observer-separation-1.overlay.tar.gz`（SHA `8149adb910f4a9503a99dba5401b189eff817593b1c8096bc29fb2c00fc40010`）
作为 `--overlay/--sha256`，`--output` 必须使用新目录。CLI 驱动同上传目录的
`ecs-r05-cli-observer-1.py`（SHA `a6bafb97c98acc65d1b2fc8c6a977bbe9ae4a76b94b77c8c2a0384628fa5f6d1`），
参数为 `--release-evidence-sha256` 和 `--binary-sha256`，取值对应上方 gate2 和 binary。
重复 CLI 实验须复制驱动、仅改 ROOT 为新目录并记录新驱动 SHA，不能覆盖此次归档。
普通文件 Write/Sync 挂起、late marker/断电耐久仍 **BLOCKED**；当前三文件组合真实云回归
**UNRUN**，既有 R15 云结果不回填为新源码通过。

<a id="r01-live-overlap"></a>

## R01：真实同命令后台重叠与反序完成

`/opt/aries-artifacts/observability-r01-overlap-1`，1个E2B Sandbox、TTL600秒、2条同命令后台exec、0模型调用。复用combined-release-3固定源码/二进制/镜像，未改变native工具策略。

- `evidence.json` SHA `d675e3bc28aef02ee21f739beee95f551c343e10f01301d3dcfd4c8a6ccbcbcf`，原执行状态保留为EXECUTED_PENDING_RECONCILIATION。
- `reconciliation.json` SHA `2facd2eb1c3ca0f8f7c2469f2215f48920bd66f06bd19d59835393a8aceed058`，PASS_SAME_COMMAND_LIVE_GUEST_OVERLAP_IDENTITY_POC_ONLY。
- 两个guest以PID+start_ticks及nonce互认实际存活，第二个exit7先终态，再释放第一个exit0；native→SSH→Bridge→SDK Process→guest receipt精确关联2/2。双流字节/哈希/退出码逐段一致；运行中的process.log实际读到各自READY，未规范化跨流合并结果。
- 八次辅助准备/控制调用单列unlinked。producer attempted/written均2，不能外推全工具覆盖。Bridge/Adapter退出0、provider确认后的ledger为absent、cleanup errors为空，自有容器和临时凭据消失，生产实例未改。
- 此PoC没有LLM、Gateway或独立Verifier，不算完整任务/后台Strict Replay；native计时、华为、新建SSH连接与取消/错误metadata矩阵仍未由此覆盖。

输入位于`/opt/aries-artifacts/r01-overlap-inputs-3`。driver SHA `413eba72d74d371bd9a9d35689887e976501f4e4fcbf8c6aee3d66bb8c8d6936`，checker SHA `8fc7fdc404437feb8d370aac7454c7d7aca212f027b08ec01cd2642052eb0c14`。ECS已有31项fixture通过和固定native镜像Node语法检查；旧inputs2的既有容器清理归属反例FAIL保留，修正仅约束清理本次尝试创建的资源。实际执行命令如下；复现时必须使用新的output目录，旧证据不覆盖。

```sh
python3 /opt/aries-artifacts/r01-overlap-inputs-3/ecs-r01-overlap-1.py --pins /opt/aries-artifacts/r01-overlap-inputs-3/r01-overlap-1-pins.json --output /opt/aries-artifacts/observability-r01-overlap-1 --execute-reviewed
```

<a id="r07-real-sdk"></a>

## R07：真实 E2B SDK 退出／排空与取消对照

`/opt/aries-artifacts/observability-r07-sdk-real-1`：1个Sandbox、TTL300秒、20个预定执行槽全部启动、10组旧/新配对PASS、0模型调用。仅对冻结wrapper做真实SDK对照，未修改生产代码；SDK2.58.0，镜像`sha256:abd55576ec4baeef6cb47622e83b55c2b6182bf7cf793c6f1ce2c235fdb79951`。

| 证据/输入 | SHA256 |
| --- | --- |
| evidence.json：PASS_REAL_SDK_WRAPPER_SEMANTICS_ONLY | `2b4bad882386f9f48c8b5034a1b94567a794e8b97d9e8931c7ac268ee1c862ab` |
| worker.json：20槽、双流/终态/进程取证 | `ff5925b4960e32f661fd5df50960c9f28e9da1928d3aca28df588dcfcc98189c` |
| baseline guest_exec.py，原p0-release-3 | `9681c6ed5089e56d320c4661a74ae18b914c8deb8647b929adc06a2c5488977e` |
| current guest_exec.py，r15-release-1 | `c7f8ac5257ae7aa0d78fb83541b997a839632640dae28337beee8f8eb515993c` |
| 实验driver | `c7e8a94a2399717852b541d102888f4351008360c46fb1a9ab77278110983bfc` |

覆盖无输出、双流二进制/NUL、延迟输出、每流512KiB、后代持管道、信号、异常、Popen失败、真实SDK deadline及commands.kill。旧新分别比较双流字节/哈希、退出码、SDK异常和执行上下文；不要求两个独立流的到达顺序相同。此前19项ECS零云断言和固定SDK预检通过；探针仅修正后代读取已退出父进程/proc的竞争，未改变受测wrapper。

后代持管道的候选原始值均来自同一guest单调时钟：direct-child退出观察407731501ns，pipe EOF1409275010ns，相差1001543509ns。这证明约1秒的排空等待可以发生在直接子进程退出之后，不能全部归为用户命令执行时间；不是kernel精确退出时刻，也不跨主机相减。

**真实取消反例保留：** timeout两版均TimeoutException；kill两版均返回true并收到CommandExitException(exit=-1)。四次取证都看到managed PID不在列表、wrapper已消失，而同PID/start_ticks的child仍为S状态。20次各自取证共38个已知进程样本：34 absent、4 sleeping、0 observed zombie；不是同一时刻的系统快照，不能推断未跟踪后代。两次Popen失败没有child身份和guest terminal，保持缺测。

最终worker删除Sandbox并取得provider NotFound；独立cleanup再次NotFound。容器、凭据已清理，cleanup_errors为空，输入SHA未变；144个归档文件SHA已复核。最终整体删除不等于取消时全树已停止。此SDK结果不关闭R14评分门、B原生工具或完整Bridge Stop；后续正式停止路径的限定证据见下一节。

实际命令（仅ECS；复现使用新的output目录）：

```sh
python3 /root/aries-setup/uploads/ecs-r07-real-sdk-1.py --pins /root/aries-setup/uploads/r07-real-sdk-1.pins.json --output /opt/aries-artifacts/observability-r07-sdk-real-1 --execute-paid
```

<a id="r07-current-isolation"></a>

## R07/R14：当前组合的 A 撤权与 B 拒绝评分

`/opt/aries-artifacts/observability-r07-current-faults-3`：状态 **PASS_CURRENT_A_CONTAINMENT_B_VERIFIER_GATE_ONLY**。仅执行既有W2的`a-ssh-disconnect`、`b-stop-unavailable`，2个Sandbox、逐个运行、TTL240秒、0付费模型调用。生产源码不变：复用`r05-observer-separation-2/source`的559文件，manifest SHA `7352037b62b7c3bc2f8c92ca7a8d289c4db3a9749a6c67585466f97622a4ee5e`；沿用prepare-2已通过的Runner隔离失败、Embedded Stop重试、ExternalBridge撤权三项定向Go测试及同一已编译fixture。此处不是新增全量release门禁。

- **A：** 真实SSH断开后、调用`ExternalBridge.Stop`前，已知父/子进程仍活着且cgroup populated；Stop之后两次取证均为这两个PID/start身份不再存活、group为空、该UID进程数0，心跳停在同一值，listener已撤权。fixture只补了Stop前存活断言，证明故障确实触发。此例不含完整外置Agent Loop或Verifier；直接SDK取消的child存活反例不等于正式A隔离门失效。
- **B：** 真实Embedded Gateway、脚本模型和真实Tool；仅对本次group Stop的客户端传输注入失败，命中4次。实际Runner记录harness canceled、isolation/evaluation blocked_isolation、cleanup failed；Verifier调用0、无候选产物导出、无评分Sandbox。保持这次预期失败，不将其计为任务成功；此例不是Strict Replay，也不代表平台真实cgroup kill失败。
- **回收：** 两个Sandbox均由正常测试清理删除，独立SDK复查均NotFound，无补救删除；自有容器、凭据和8082监听已释放，cleanup_errors为空。整个Sandbox删除不回填为B已取得正向隔离证据。

| 关键证据/输入 | SHA256 |
| --- | --- |
| evidence.json；原始取证文件哈希见其artifact索引 | `6e8238a58eb7278890032596c85b8fa2fed51522ce0480e7f995c07446686fee` |
| cases/a-ssh-disconnect/case.json | `d7b782a67d875185c658c02d5e37ac82a99f0e1f8c778143af701e41da2b9216` |
| cases/b-stop-unavailable/case.json | `5ebfe5777830313ad9d0943a2e8e65d89e9cf69500e5113bc021da907dcb3ac9` |
| provider-cleanup.json | `84ddcad00fe049000c8bd51e54c74420e0387f4e322ce5a7047d9b266e00108e` |
| ecs-r07-current-faults-3.py | `f51fa44585c85324aabc4cf87763274e9f40b668c131dacec1476cf04c5b0ff2` |
| fixture输入 / ECS gofmt后 | `6cb1c940f1fc3ef01471d341837b85fa8c8424168d7cd4506b1aabb24ba55ed8` / `aaeb7ab5d1a7c6eee455ecd245587055cbdd2b10b0e7ba77a3ae7adce8221d9e` |
| 复用test binary | `8c9f9afd7b8f08574f80e950335da9be5ed738c7a22c3b702c965c3126860cf6` |

旧`observability-r07-current-faults-2/evidence.json`的 **FAIL_CLEANUP_UNCONFIRMED** 原样保留，SHA `7262d9b3eafc7996c21212610ce7c8e79d1771ae438c66dfe525ca91faaff452`：fixture在首次Start前因空ledger尚未创建而停止，0次云创建；cleanup helper的root进程移除全部capability后无法读取私有state。修正仅按Adapter原生schema初始化空ledger，并恢复该helper既有root读取能力；未改生产代码、重编译或扩测。新`prepared.json` SHA `fdc4184bb48720404aaf3f704bd0685d09db3d5dd60fff528c745df5200da4d9`关联原prepare-2、失败证据及复用binary。

实际命令（仅ECS；复现使用新的output目录）：

```sh
python3 /root/aries-setup/uploads/ecs-r07-current-faults-3.py --output /opt/aries-artifacts/observability-r07-current-faults-3 --execute-paid
```

## R15 新计量验证（无云调用）

`/opt/aries-artifacts/observability-r15-metering-1/evidence.json` SHA
`141e8dbb10f3a2d2ffeaba53396a9a62b066c72e92ba7822d6517b5842a13cdf`，
**PASS_SCOPED_METERING_TESTS_ONLY**。558 文件源码包 SHA
`c120237e1299341446cb0dc766bbddbf69b8c503a75bc9edbe61ca9d500e68d3`，
ECS gofmt 后 manifest SHA
`811c34f87623eaac4140c78977655c6b93dda98b9ce30cdc93159e8414b3ef4e`；
格式化后的8个文件已逐哈希/仅空白变化核对并带回工作区。原 combined-release-3 未改。

远程执行 `go test -count=1 -timeout=180s`、相同 packages 的 `go test -race` 和 `go vet`：
`./pkg/performance ./internal/modelreplay ./internal/app ./cmd/aries`，均退出0。
测试分别约8.61s、16.05s、1.58s，包含原有writer故障矩阵；实际文件包装计数与上报计数
独立对照，错误锁存后的事件不冒充Write，短写n和Sync错误保留。
新账本测试覆盖真实分配量、保留初始证据、拒绝重复最终文件、CPU缺值与累计倒退、
baseline/L0下同一预检失败仍保留计量，以及起始证据冲突保持incomplete。
此处的预检用测试 HTTP doer，不访问真实模型；未创建 Docker/云资源，未部署新版本。

四 package 测试不等于完整 release 门禁或真实 R15 成对实验。该次证据仅覆盖两个
JSONL 的 I/O 边界，后续三类补采另见下方 metering-2，不能回填原 metering-1 结果。
RunCloud 启动前错误无输出目录时没有资源文件；panic/kill/挂起没有最终样本即不完整。
该次没有执行R15云矩阵；后续baseline/L0真实结果单列，不把此处工程PASS计为扰动合格。

### metering-2：模型旧事件、SDK 操作、L1 资源与解析兼容

ECS `/opt/aries-artifacts/observability-r15-metering-2/evidence.json` SHA
`6f030dc9b61ceca25aca556b1752ceb6db397414e62c5312c11cf5008732fad9`，
状态 **PASS_SCOPED_METERING_TESTS_ONLY**。559 文件输入包 SHA
`e2c80bb396d93cefa768fe78e80b19fda8366db72b73f6dc92b8cf9e08a28339`；
gofmt 后 source manifest SHA
`f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`；
驱动 SHA `04b07b1bc2375aa8416e806c0b2912d3f393041670a6a39c4d93f5682a78a6ac`。
10 个格式化文件经源哈希与仅空白差异核对回传；旧源码、证据不覆盖。

命令为 `go test -count=1 -timeout=180s`、同参数 `go test -race` 和 `go vet`，packages：
`./pkg/performance ./internal/modelreplay ./pkg/sandbox/e2b ./internal/app/wiring/cloud ./internal/app ./cmd/aries`。
三项分别 9.37s、18.01s、1.42s，均退出0。gofmt 后源文件测试前后哈希一致。

- legacy JSONL：正常、注入 ENOSPC/短写/Sync/Close 错误、并发、重复关闭、baseline
  无 writer、侧车已存在冲突；真实 HTTP fixture SSE 字节保持一致，计数与独立 wrapper 对账。
- 模型侧车失败：ObservationError 保留错误，缺 boundary recorder 不吞错；与 late 错误并存，
  当时版本不改 Close 原返回，也不覆盖已有文件；后续 legacy 错误分离见本页专项验收。
- SDK JSONL：实际字节/Write/Sync 和关闭文件失败，重复 Finish 不增加调用。
- L1 metrics：成功写入字节、实际调用和最终 cloud-observation 对账；真实已关闭临时文件
  的 Write/Sync 失败仍保持模拟 SDK 的删除确认；baseline/L0 字段缺省，非测得零值。
- Snapshot：旧 index 与新增 file_io 的映射/事件/区间/边/clock/coverage 相同；新增字段
  不带来虚构时间；null、负数、溢出、重复、别名及未知字段继续拒绝。

后置核验 `metering-2/postcheck.json` SHA
`37b9581d3be7da01736eb3e26c89b3f9a755c40faf95572c175fa31791f049a5`：
全部源 SHA 不变，已启动测试进程组均不存在；不使用这一检查冒充云资源查询。

这里的 HTTP/SDK 是进程内 fixture，不是实际供应商调用。云/模型调用0、Docker创建0、
生产部署false。元数据/native/Bridge及其它进程I/O、普通文件不可取消仍未闭合；
同源统一门禁与真实配对随后完成，另节记录。本次工程回归不计入独立验证任务，
不宣称扰动合格。

<a id="r15-metering-release"></a>

### metering-2 同源完整 Go 门禁与独立信号验证

ECS `/opt/aries-artifacts/observability-r15-release-1/evidence.json` SHA
`b0260a73807e0771217010fffd6cfe983a25f8cf6999eaca21394496eb69f2db`，
状态 **PASS_FIVE_GO_GATES_ONLY**。输入为上节 metering-2 的559个文件，source manifest
SHA仍为 `f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`；
输入 evidence SHA为 `6f030dc9b61ceca25aca556b1752ceb6db397414e62c5312c11cf5008732fad9`。
实际驱动 `/root/aries-setup/uploads/ecs-r15-release-1.py` SHA
`f34ea5de84ad1a70f5f268bd8fb31c7b83a0bff3dca4af2bbd9fd1f1a74ebcfc`。
先按manifest逐文件核验/复制到本次独立 `source/`，测试后输入和副本源SHA均未变。

terminal-bench fixture从已有干净cache以 `git clone --no-hardlinks --no-checkout`
复制到本次 `source/.cache/terminal-bench-2-1`，detached checkout固定commit
`7131e4375048a0e408a8fb404b5f499d726b695b`，连接性检查通过；借用cache前后HEAD与
工作区状态一致。Go使用已有远程缓存、GOPROXY/GOSUMDB=off、GOTOOLCHAIN=local，
`GOMAXPROCS=2`、`GOFLAGS=-p=2`；下列为实际make命令耗时，非云任务八阶段。

| 门禁 | 状态 / 退出码 | 耗时（秒） | 日志 SHA256（本次根目录下） |
| --- | --- | ---: | --- |
| `make build` | PASS / 0 | 10.204758 | `54ed41a09eb9d734c82fc964db7363ce5511034c4a223ac01794de79e4a31495` |
| `make test` | PASS / 0 | 26.813224 | `f5e30fbc29e99ef2d6f185c35a35b1133d58ac34f97f8a6d28feddf7f2b1a0e0` |
| `make test-race` | PASS / 0 | 71.607739 | `1fa5e9d5505e5d884be6b535a1f7f6ea9232ee3f44f6903594a38c3263d7988a` |
| `make lint` | PASS / 0 | 3.133365 | `b03d6d56dc4685bc800059cf1ecdf1e3e70fdaeeacecd9b39ca1687f6d7c527d` |
| `make integration` | PASS / 0 | 375.988612 | `9fbbd04df8074920694a70573dd74c34fe6339c0df1f9d93364d9e9195d6a898` |

日志名分别为 `make-build.log`、`make-test.log`、`make-test-race.log`、`make-lint.log`、
`make-integration.log`。integration使用真实Docker与确定性模型fixture，不调用付费模型
或供应商Sandbox。状态记录 `cloud_creates=0`、`paid_model_calls=0`、`production_deployed=false`；
所有门禁进程组消失，新残留container/network列表为空，已有容器/网络完整盘点前后一致。
该结果证明本次正常结束的资源状态，不代表驱动已证明所有超时清理分支。

| 二进制（本次 `source/bin/`） | SHA256 |
| --- | --- |
| `aries` | `eda0146ce84662b288b6e894e461cff293319536a6d14422a7190d5c0da0f1b0` |
| `aries-bridge` | `c56bc28b77c02955d54075dd75d2ce59bfcdd2b1eae389356ca631259e754fb9` |
| `aries-mock-model` | `b88535ff8219094290b8509e63583a6cdcb8d969cb516a9d6fd1f793c2c73fb8` |
| `aries-model-replay` | `33093348ec1d9d7f4dbbe97e4a5a779175aa6d4f989f4d26689eefb9cfe25c02` |
| `aries-perf` | `1cbff04aca2b22240d3cb180cfc8e60404d25d878d0cce87ee5851a74e7719da` |
| `aries-ssh-client` | `3713f9c455ac0d09045a4d05d8dc88b73c6d2badf807ecc526d8d575cfd4ea5f` |

初次传输失败独立保留在本地 `.cache/r15-release-1-transfer-failure.json`，SHA
`c0e5f3e9c1300ba086a5643173c6188b09897a0509a31807c54684bc15778b6c`，状态
**FAIL_TRANSFER_BEFORE_EXECUTION**：SCP退出1、kex连接关闭；随后远端执行退出2且明确脚本
不存在，`test_driver_started=false`、cloud_created=0。修复传输后才开始上述实际门禁，
没有把传输失败计为测试执行，也没有抹去原失败。

运行中的驱动未被覆盖。另存候选 `ecs-r15-release-1-interrupt-safe.py`，SHA
`189ae6ba5eb060e450ea91e8ed85decebeae10bf473b93e415d1d29d94a9d3a7`，增加TERM/HUP处理。
ECS `/opt/aries-artifacts/observability-r15-driver-signals-1/evidence.json` SHA
`42c0aaf0f177597497a055608ac9e45c4779aece831a9f0a5168a03b2cd5836f`，状态
**PASS_ISOLATED_DRIVER_SIGNALS_ONLY**。测试驱动SHA
`c445567ef22592a76096553331f678dac72a4a71389de135fe64eefe2671fd18`；按候选SHA提取真实
`sha/save/step/interrupted`函数和两条signal注册，未执行完整main。两个独立sleep场景在
RUNNING+PGID证据就绪后分别发TERM/HUP，均验证InterruptedError、step及最终FAIL落盘、
worker退出1、所有自有PID/PGID不存在；另一次只读复核同样确认无残留。
此测试无Docker/云/模型调用，不覆盖重复信号、step前后窗口、复杂后代树、SIGKILL升级
或Docker资源清理，不能宣称候选完整脚本重新通过五项门禁，也不能解决R05普通文件I/O阻塞。

本节两个限定工程PASS不增加任务样本，不证明扰动合格；后续paired-2真实8次单列，
完整任务总数由37增至45。旧镜像发布、combined-release-3及其云/Perfetto证据仍只覆盖各自版本。

<a id="r15-paired-validation"></a>

## R15 当前冻结组合：四对八次真实回放

固定559文件源码、同一 E2B A placement、同一 Tape、并发1，按 baseline→L0、L0→baseline、L0→baseline、baseline→L0 四组顺序完成8次真实 Strict Replay。每次6条exec、12项独立Verifier通过，Stop/revoke confirmed；计划/接纳/执行/完成/验证成功均8，失败与未运行均0。实际创建16个Sandbox，逐项provider NotFound，cleanup_errors为空；未调用上游模型。

`/opt/aries-artifacts/observability-r15-paired-2/evidence.json`，SHA `3fb5a096b2ee3253757ef93e05a1691d28a32fed37d4272faea1c78ea90df37a`，状态 `PASS_EIGHT_REPLAYS_AND_DECLARED_MEASUREMENT_ONLY`；
同目录 `paired-analysis.json`，SHA `031ae0159d92c4eb0511bf2ff1b5541f4f561b665876ecfed0b5bddb59999c0d`。时间为2026-10-24 13:48:16–13:54:19 UTC。
运行驱动为paired-4，SHA `7a9377b0a03e3a79707fc70ae2d6310ee03e199a2d1075fa53fe82c73b29eb28`；
35项脚本fixture证据SHA `0ea74c40535989d1f9deada9517bd6a1caa61f42b9e4dc354992f0e8c0e0d71e`。
预登记SHA `b9bfd739c5cbb402fc697446be75446e783b2b2cad289b71eba571278ba9f8d8`；
source manifest `f26d2dcd051ad6ed822e381e69f2a3781b825dd72262e5d15d57675db27990e2`。
Runner/aries-perf/ssh-client使用上节r15-release-1 SHA；Adapter image ID
`sha256:abd55576ec4baeef6cb47622e83b55c2b6182bf7cf793c6f1ce2c235fdb79951`，native真实manifest
`sha256:cc7b2c8270be5ca0f5169b7b9f0b6574fd56f6f559cb778b9cbfc467b7b8e664`；
镜像沿用combined-release-3/publish-4来源，不能说它们由559文件重新构建。所有pins前后核验未变。

| 原执行序号 / 条件 | pair | run ID（各目录 `runs/` 下） | 模型原始轮/帧 | 结果 |
| --- | ---: | --- | --- | --- |
| 1 / baseline | 1 | `20261024T134818.341467862Z-openclaw-e2b-external-r15-01-pair-1-baseline` | null / null | Strict/12 tests/隔离/清理 PASS |
| 2 / l0 | 1 | `20261024T134903.433948698Z-openclaw-e2b-external-r15-02-pair-1-l0` | 7 / 735 | Strict/12 tests/隔离/清理 PASS |
| 3 / l0 | 2 | `20261024T134947.938843630Z-openclaw-e2b-external-r15-03-pair-2-l0` | 7 / 735 | Strict/12 tests/隔离/清理 PASS |
| 4 / baseline | 2 | `20261024T135030.814552093Z-openclaw-e2b-external-r15-04-pair-2-baseline` | null / null | Strict/12 tests/隔离/清理 PASS |
| 5 / l0 | 3 | `20261024T135114.852873821Z-openclaw-e2b-external-r15-05-pair-3-l0` | 7 / 735 | Strict/12 tests/隔离/清理 PASS |
| 6 / baseline | 3 | `20261024T135200.080586444Z-openclaw-e2b-external-r15-06-pair-3-baseline` | null / null | Strict/12 tests/隔离/清理 PASS |
| 7 / baseline | 4 | `20261024T135243.047671284Z-openclaw-e2b-external-r15-07-pair-4-baseline` | null / null | Strict/12 tests/隔离/清理 PASS |
| 8 / l0 | 4 | `20261024T135327.722165752Z-openclaw-e2b-external-r15-08-pair-4-l0` | 7 / 735 | Strict/12 tests/隔离/清理 PASS |

仅比较双方均开启原生身份协议时的可选L0增量；不含身份机制绝对成本。baseline模型轮/帧原始观测为null，不能把Tape预期7轮/735帧填成实测；L0四次均实际观察7轮/735帧。共享Go进程CPU/GC/分配不能拆成Runner、ModelProxy、collector独立开销。四类L0 JSONL计数为实际Write/Sync调用，baseline关闭时值为null，第五类L1资源writer未启用。区域、provider底层缓存与冷热状态unknown；四对样本无预先批准阈值，不宣布扰动合格、负开销、p99或供应商排名。全部8次为实际前台非PTY、无process/poll，不能扩大到后台Strict。
baseline归档exec→producer→Bridge为6/6，SDK/guest观测关闭；L0为已归档exec精确链6/6。
完整native漏采分母unknown，原生A计时未采；全部Snapshot/Trace exit1，baseline保留缺源诊断，
L0保留native source/index not collected。不存在为了完整图而补零或伪时间线。

完整原值在 `attempts[].review.eight_stage_ns`、`measurement.controller`、`measurement.file_io`；
四对差值及median/range在analysis文件。`absolute_l0_minus_baseline=L0-baseline`；
baseline缺值的FileIO差值为null，绝不把关闭writer填零。八阶段与资源表见
[性能归档](../performance-archive.md#r15-current-paired)。外层GNU time与SELF资源口径独立，
机器背景proc/PSI原件不归到Agent。配额为8attempt/16create、TTL600秒；实际计费时长/金额
仍null，controller窗口×已知分配数只是sandbox-seconds上界，不冒充账单。

独立原件/算术复核 `/opt/aries-artifacts/observability-r15-paired-review-1/evidence.json`，SHA `d182fba9e138b69529ffdff91d12138fa5c822f6045844d9e351d7acfee47286`，394项PASS，
复核脚本SHA `e7b67f7142e2c6e02f4ec78d2e76baee4c123ecc991db1d94ae97f3527b8b37c`。
核各源SHA、raw八阶段、SELF delta、候选/12tests、已留provider absence与四对差值/median/range；
源文件前后不变，0云/模型请求，不重建HTTP请求；该算术复核未执行TP/UI。

四条L0 Trace已实际导入原生TraceProcessor，每条55个区间/43条flow/19个来源，operation起点、时长、终点及flow端点逐项一致，导入误差0ns、processor errors为0。四条baseline因可选观测关闭而SKIPPED，不把空导出算时间线通过。原始文件SHA未变，native A计时缺测与incomplete保留；新UI交互UNRUN。证据 `/opt/aries-artifacts/observability-r15-paired-perfetto-1/evidence.json`，SHA `1b4b992f912783f05b9927b4134676f80a1e27cf5b3b30c7e70a44b05e4c826b`，仅为 `PASS_FOUR_L0_OFFLINE_PERFETTO_ONLY`，无新增云/模型调用。原始JSON浮点投影最大误差约3.64e-6ns，
不冒称浮点运算误差也为0；Chrome ts/dur为微秒，TP为纳秒，结束点单独核对。

### 本轮预检与失败记录（不重复计任务）

paired-1预检原件的本地只读副本 `.cache/r15-paired-1.preflight.evidence.json`，SHA
`22cc7f97af5e67d0367812e6aa48d4ff7c62844d3693032cae24f78e3cd2b152`
为 **PASS_PREFLIGHT_ONLY_NO_ATTEMPTS**：candidate3、8个NOT_RUN、0create；它不是失败、
不是第一次付费回放，也没有增加45个成功任务。

前置 `observability-r15-template-1` 只读GET把TemplateID:BuildID当路径而HTTP404，0create；
`template-2` 改用TemplateID后确认指定Build ready、2CPU/4096MB。定价/pins证据
`observability-r15-pricing-1/evidence.json` SHA
`502efd389d5025cf4ab952d8070bc9d563815b76a33dfd42b8048d3a0f966842`
仅 **PASS_PINNED_BUILD_AND_ESTIMATE_ONLY**，0cloud creates/0model calls；估算不是实际账单。

旧candidate3在同fixture下33PASS/2FAIL：中断后仍可能开始下一attempt、versions未seal；
修正candidate4的35项fixture通过（SHA见本节），保留旧失败。更早fixture遗漏
`preflight_only` 参数导致的两项检查错误也保留，不算受测生产代码缺陷。
执行真实八次的是candidate4，没有自动重试；这些预检/离线检查不改真实attempt分母。

## 执行环境与冻结输入

全部编译、gofmt、测试、容器与云实验在SSH端口8228的授权ECS运行。本地只编辑、打包、哈希与SCP；不source/eval `.env`，凭据不写进命令、文档、公共Trace。

| 对象 | 归档/身份 |
| --- | --- |
| 上游HEAD | `e686735fe0196aacfb799c8d528848dcad026d83`，branch main；工作区dirty源另行冻结 |
| 开工前基线 | 556文件；manifest SHA `828db09416de8a029f9ea516b6b72043b2e2dcb406f75aecd277cefa418ce89d`；source archive SHA `2f4d64659ce94242382560b90212f9d1bf6ca263a103121c09ac45d3e472ce7a` |
| P0首轮输入 | 573文件；manifest SHA `e7cf863349d562cfdd55d0cf1f118b972ec64f8587e73e89229265719409fc53`；archive SHA `5eb0aa85bf77c94e22512c85f0ed9003b9ae70de1deee93e3c501a03b2cb28d8` |
| 首轮release job | `/root/aries-setup/uploads/obs-p0-release-1/run.py`，SHA `c5d1bf656c4ab9d46c5df2645c577958ae54dce834001d8b8d22b503d6efa08a` |
| 首轮隔离release根 | `/opt/aries-artifacts/observability-p0-release-1`；生产repo/tag不替换 |
| 历史release3候选输入 | 575文件；manifest SHA `d970bc6230fbe9f6fde8f2e603a04e063b4cfa2b9378eb4173451985396f8e82`；archive SHA `a866294236bbe3004ee7745f4d4f7e3c369b0e96f44fed9f1654eb15148b21c9` |
| 历史release3证据 | `/opt/aries-artifacts/observability-p0-release-3/evidence.json`；SHA `91ce19c32a948bb98dacb19109800ee1283beada25e1192f211f5dc86c71e82f`；2026-10-24T05:07:33Z终态PASS，不代表后续组合 |
| ECS格式化后的源码 | `observability-p0-release-3/source-manifest.json`；SHA `d81d1d962d49c86402529b2f73b5fd1860dd55257a2a48ef4a4ed52ca3b2879f`；格式化差异已按前后SHA核对后同步本地 |
| 新Runner候选tag / image | `aries-e2b-runner:observability-p0-v3` / `sha256:6b3157f051cf2995875adacf4a5ad534010f17d970cbfd9f23c0761078b4bb4b` |
| 首次分析器 | release-3/source/bin/aries-perf；SHA `bd259299355fff6f5e160cd77a70dacbffa319bc0d5a2cf1be5c17cffe69175b`；其Trace v2存在后述真实导入缺陷 |
| 修正后的离线分析器 | `/opt/aries-artifacts/observability-p0-exporter-1/aries-perf`；SHA `e2447a1e65c12b1fac8ee5abc64f4fb3bc3a90f095a416899ac16ea53f813085`；Trace metadata v3 / `aries-chrome-trace-v3` |

release job按实际manifest校验每个成员，只在独立目录gofmt，归档格式化后source manifest、binaries与image。保留失败尝试，不覆盖或伪称首轮通过。

release-1：build通过，test被原schema负例阻断（旧测试将已合法的v2当成未知版本）。原FAIL保留；负例改为当前版本+1，没有放宽生产解析器。release-2五项门禁全部通过。随后修正A ResponseOnly主动丢弃事件的观测范围，release-3重新运行五项门禁并全部通过。首轮及中间候选不是最终云实验镜像。

## 远程回归

在隔离release `source/` 内，实际job采用已有Go离线缓存、固定terminal-bench fixture、短TMPDIR；这些是远程测试环境，不是本地执行命令。

```sh
make build
make test
make test-race
make lint
make integration
```

有具体失败时才定向重跑，先保留原日志。主要新增断言：

| 断言 | 文件/命令覆盖 | 当前结果 |
| --- | --- | --- |
| v1/v2兼容、source SHA、单次解析、快照不可变、不同clock不拼接、ns→µs、缺index/截断/late/整对缺失 | `pkg/performance`、`cmd/aries-perf` | PASS，release-3全量test/race包含这些包 |
| 模型整段历史偏离、busy/retry/auth/断流/flush失败、独立session与request ID、write/drop不改变Tape功能结论 | `internal/modelreplay` | PASS，release-3全量test/race |
| Gateway每delivery独立，auth正文不泄漏，重复accepted不算terminal，本地Send失败/取消，B共有anchor，A ResponseOnly丢弃范围 | `pkg/harness/openclaw/gateway`、`pkg/harness/openclawcloud` | PASS，release-3全量test/race |
| 相同命令/重试/跨session/反序/错误ctx不串SDK Process，无terminal不伪造退出 | `pkg/bridge/ssh`、`pkg/sandbox/e2b` | PASS，release-3全量test/race；不包含阻塞的上游native身份 |
| 候选失败/评分环境创建失败/评分调用失败保留真实子区间与原结果 | `pkg/benchmark/codingcase` | PASS，release-3全量test/race |
| 原Runner stop/revoke/取消/cleanup/分母/Goodput与真实容器行为 | full test/race/integration | PASS，release-3，生产资源不变、测试容器/网络已回收 |
| R05 producer/关闭/落盘故障矩阵 | `observability-r05-reliability-2`在current-source-validation-1基础上叠加七个R05文件；定向go test/race及五项make门禁全部通过。包含整对丢失、HTTP handler join、index发布冲突、ENOSPC/EROFS/短写/Sync/Close注入、异常进程退出。完整证据见下节。 | 22项中17项限定断言PASS、4 BLOCKED、1 UNRUN；不包含同期R01/R07最新修改。释放阻塞门闩后通过不等于生产有界I/O通过 |
| R05 文件系统flush可取消超时、崩溃耐久 | 尚未实现可证实的退出机制 | BLOCKED，不能用普通Close通过代替 |
| R07 guest wrapper 边界与语义 | 历史 `r07-semantic-1` 为10场景/20次ECS本地进程，`r07-shared-compat-1`为离线SDK；新增[真实SDK对照](#r07-real-sdk)完成20次执行/10组配对，[当前停止门两例](#r07-current-isolation)覆盖A实际撤权与B拒绝评分 | 语义及退出/排空边界限定PASS；直接SDK取消后child存活反例保留。后续B已获3个直接子进程区间，[完整A取消](#r01-native-cancel)已单独通过一条原生调用的身份与评分前隔离；B全后代/PTY及其它负例仍未验，不用最终Sandbox删除代替取消时终态 |
| R15 观测扰动 | 当时冻结manifest `39dff992bfe7d5fb01121dd63bcc0b6c7c46e36fc539ae44370ccfdefca435f0`，同一E2B B Tape/spec/concurrency；baseline×2、L0×2、L1×1、L2×1，保存time/run-result/阶段/index | 历史组合部分PASS，六次功能/Verifier/隔离/清理成功。`observability-r15-current-1/evidence.json` SHA `bd321f265c829ca34d2b7bf458d20570003cb54f4086c853fa84a693929dec1a`；不覆盖后续R05/R01/R07或新renderer。仅有drop字段的writer可断言0；L2无profiler |

## R01 combined-release-3：统一门禁与独立原生矩阵

本节根目录为 `/opt/aries-artifacts/observability-r01-combined-release-3/`，与前述历史 `observability-p0-release-3` 是不同版本。冻结source manifest SHA `01660084b9bfaa900f8236429a7407472e2d20cac36213b8a8adfa2c48a595c6`，archive SHA `f5f8ada17d0421c65068dfb55d13ddf247d6ba6d28f6f293ebb21bb907a8c993`。恢复过程核对源文件未变；修复的是fixture、构建依赖可达性和生成产物权限，不是降低断言或修改受测源码。

| 证据（相对本节根） | SHA256 | 实际结果与范围 |
| --- | --- | --- |
| `evidence.json` | `d4b33b5a7617810def867e170ea04053e412e46e5ff528853907fe3a0188d62f` | **FAIL保留**：build/test/test-race/lint退出0；integration退出2，隔离source缺固定terminal-bench fixture。后续Adapter/native步骤未执行 |
| `integration-recovery-1/evidence.json` | `7188d261a76f615c5a7f631240adda58cc9455ee93339f6f5c66ed2808e4bb3a` | **FAIL保留**：准备脚本假定clone有alternates文件，实际不存在；未视为integration成功 |
| `integration-recovery-2/evidence.json` | `00480882b5f7e8bf564552c96079f0bdb0d6e6641107c44d2efb0438cbe782dc` | **PASS_ENVIRONMENT_RECOVERY_AND_INTEGRATION_ONLY**：独立本地对象包与来源SHA相等，git连接性检查及`make integration`退出0；源manifest、原FAIL不变，无新增残留容器/网络 |
| `remaining-gates-1/evidence.json` | `f7478526bc7b17b35a6ba0dea57b30a8a0f5681f317b45f6ef07c6e5433cc7d0` | **FAIL保留**：离线SDK测试通过，Adapter构建`--network=none`没有可用依赖缓存，pip解析/下载失败；日志不是业务SDK兼容失败证据 |
| `remaining-gates-2/evidence.json` | `fc9b63d26463db25d1d24218d1a39b957f140003bd2b21261ef82d1293a37cdf` | **FAIL保留**：联网构建新Adapter、依赖清单/`pip check`与断网SDK测试通过；native镜像构建通过但helper首项EACCES。生成环境umask077使COPY代码0600，非root运行无法读取，后续native矩阵未执行 |
| `remaining-gates-3/evidence.json` | `4f90850aff766a78dc0fd19d7651f2f0d6369222205cca56e34f843601d94a87` | **PASS_REMAINING_GATES_ONLY**：仅native代码生成与嵌套B镜像构建显式umask022，归档目录仍私有；native完整重跑通过。复用-2已通过且SHA未变的同一Adapter镜像/依赖/SDK测试，未冒充重新执行 |
| `remaining-gates-3/native-regression/evidence.json` | `b5f06a1db51f9925c24e2270c654cd86d973f8001e011655e8a1fc2629c0793f` | **限定PoC PASS**：helper、B原生4并发、A固定26场景；25项预期行为和1项原版finalize/yield无会话反例，后者`normal_lifecycle_pass=false`。26次实际native exec中25次身份发布/25个精确target映射，分母来自实际调用与ALS绑定发布，非固定猜数 |
| `reconciliation.json` | `8812dd96d58c8fdf4313c112d5ea47e72f9eef46e34ec6d16017ee4358c96117` | **PASS_COMBINED_GATES_AND_ISOLATED_NATIVE_ONLY**：引用原FAIL、恢复与remaining证据，不覆盖原件；此归档仅包含门禁与原生PoC，当时整Agent/新Tape/云验收UNRUN，后续真实证据另见下节 |

该 R01 组合的 `aries` SHA `af42a84827fa714ed1ecb75be2e43dbdb5926f70da91009673aec3f1f326c3db`，`aries-ssh-client` SHA `c5184da972acb80d9929588951636bd6da2fc3438aae06a330c8ffd08bb18447`；Adapter image ID `sha256:abd55576ec4baeef6cb47622e83b55c2b6182bf7cf793c6f1ce2c235fdb79951`，组合native image ID `sha256:cc7b2c8270be5ca0f5169b7b9f0b6574fd56f6f559cb778b9cbfc467b7b8e664`。仓库manifest身份须独立核验，不能仅由image ID推定；本次后续实查见publish-4。所有门禁均记录cloud/model调用0、production_deployed=false；本次容器确认absent，已有容器和网络不变。producer健康为attempted=written=25、write/status失败0，但只证明这个原生探针，不能外推整Agent覆盖率或R05阻塞问题已消失。

完整Agent驱动另有 `/opt/aries-artifacts/observability-r01-full-agent-review-2/evidence.json`，SHA `d08867821f8cd8c7e4e3b56f8780814a7af6b426525ad9c782520812cc982103`：**仅16项离线AST逻辑/语法检查PASS**。修复Runner尚未调用时attempt应终结为FAIL_SETUP、已调用失败为FAIL，保留未启动/已完成分母；checkpoint写失败或中断不能跳过下一项cleanup，失败在内存保留。旧驱动负例同时保留，新驱动SHA `f6de6b09c15b41cd6995ae1a976277c1195214391071420d7b1bf31813285948`。该检查未导入driver、未执行main，Docker命令/云调用均0；不证明真实录制、回放、Verifier或云资源清理已运行。

## 最新真实 E2B A：新录制与同 Tape Strict Replay

`/opt/aries-artifacts/observability-r01-full-agent-2/evidence.json`，SHA `ddf450782ad18f5f074ab3c815302b7f363273f18ff607d787a26553a6797b1b`，终态 **PASS_TWO_ATTEMPT_A_RECORD_STRICT_REPLAY_SCOPED_IDENTITIES**。绑定上述combined-release-3/source与实际发布native镜像；预登记SHA `eef9b8e1b1c372518c240dd657b381d9cea638a9ac3210dbca33404032392a5a`，驱动SHA `f6de6b09c15b41cd6995ae1a976277c1195214391071420d7b1bf31813285948`。C1顺序执行record/replay两个attempt，Agent在ECS Docker，Tool在真实E2B Sandbox；使用独立当前Adapter，保留原生工具policy、后台/process/PTY能力和未改的Strict matcher。

| 实际attempt | 模型/工具与验证 | 原始结果SHA / 快照 |
| --- | --- | --- |
| `record/runs/20261024T113242.936006762Z-openclaw-e2b-external-r01-record` | 真实DeepSeek live录制；7轮/735 SSE帧/6 exec，6个producer invocation；12项独立测试、失败/错误/跳过均0 | run-result `aba05e54d0d9081ef8baf3e6aaefb1a85447efdec47460767fcc289673436a69`；Snapshot `snapshot-eb6009d56e9c2ae7894fa32aa97c081f40268525e4fbb8a737f92d0d7970c305` |
| `replay/runs/20261024T113329.498639783Z-openclaw-e2b-external-r01-replay` | 同Tape recorded时序；7轮/735帧/6 exec，replay Runner未获真实上游key；12项独立测试、失败/错误/跳过均0 | run-result `4b8a6615886e3b92e04e13adbc3e3d1e5cdbedfbfacafacdbd50a964b8e20f17`；Snapshot `snapshot-e1c7d5c250be49173d73b5b10efd9df6d069c1d9df4d9b1b686ec6d423d88589` |

新Tape文件SHA `d716431ea36e1b89b47aa0c8f091cd6e65700bc10e83522467414a0d3735cffb`；record/replay候选文件SHA同为 `181c011a55737cb2ade8836c992ee463c45479d31312cfc6855aba0d6f62bdf1`。两次producer均attempted=written=6、write_failed/status_write_failed=0、failed_latched=false；collector index的独立分母仍null，随后用归档exec逐ID核验6/6，而不是把producer自报当独立全量分母。12次实际工具调用均是前台非PTY、使用`stdout-stderr-v1`，没有process/poll；不能声称本次验证了后台Strict或所有原生分支。

两次Runner退出0，model session、Harness与Verifier成功；harness_stopped/bridge_revoked=true、isolation=confirmed，task/infra cleanup succeeded。每次task+Verifier各1个Sandbox，共4次实际create；逐资源SDK `NotFound`确认absent，不仅是ledger状态。两份provider-absence证据SHA为 `1af23b0df4819a6139a96adada0ca88e97808cbc19eff42dd5debd47903c0e7b`、`32140870e5f1a308e72f15f22ee6a0ebceddaf1e1894e14cb41444acceec0f7d`；总体与每attempt cleanup_errors为空，本次Runner进程组消失，fallback前无残留Docker资源。

限制保留：exact只覆盖已归档exec，native transcript是否整体漏采仍unknown；回放请求正文没有另存，整请求严格性依据固定matcher执行成功与request-boundaries，不冒充独立正文重建。身份扩展没有补native A计时，两个Snapshot integrity/coverage仍incomplete、snapshot与trace命令退出1，原因保留`native tool source/index not collected`；不得为了6/6关联把缺测时间线涂成complete。本次是功能与限定身份验收，不是R15成对扰动、容量或P0全链完成。

真实导出的Perfetto另经 `/opt/aries-artifacts/observability-r01-full-agent-perfetto-1/evidence.json` 核验，SHA `3ac0470bbcc0038bf1c05c1027ec12405366c7296bc4a0c1d45d7e9ba9ff7b3e`，**PASS_OFFLINE_PERFETTO_ONLY**。record为1629 events/62 intervals/50 flows，replay为863/55/43；每份19个来源逐文件SHA核对。原生Trace Processor实际导入并按operation ID核对起点、duration与flow两端，误差0ns、无processor错误；Chrome ts/dur使用微秒，TP输出纳秒，displayTimeUnit仅影响显示。原始投影浮点误差分别约`3.34e-6 ns`、`3.64e-6 ns`，不冒充原始浮点运算完全无误差。full-agent-2原始树、Snapshot/Trace哈希未变，native缺计时与incomplete诊断原样保留。**新Trace的UI交互UNRUN**；投影代码SHA与旧UI验证相同，只能复用旧UI能力证据，不能称已浏览这两条新Trace。

独立只读复核原始 XML、模型/原生正文、身份链、冻结源码/二进制与清理响应另存
`full-agent-2/independent-review-root-2.json`，SHA
`c20e935b5202e293ddb3bbc1225e125cc0be3a9ab1b59548a74b6bae4c8a7fa2`，69项断言通过。
原检查 `independent-review-r07.json` SHA
`7dfaaa8bfaef4ae5aebec9b4bed2e8e589957ad7a29b31ea58e7ce81631cc4ce`
有5项不符，保留 FINDINGS：两次运行各2条非零退出被检查器误要求 `exec.wait.status=succeeded`；
模型正文关联遗漏已有 `openclaw-2026.7.1` ID 方言，因查不到 key 而未进行文本比较。
源码 `operationSpan.end` 明确非零 exit→failed；`requestToolCallID` 已有固定 ASCII
下划线/连字符去除规则（长度≤40、无截断、拒绝碰撞）。修正的仅是复核器：退出码仍逐层相等，
文本不归一化、不忽略；新增修改正文和错ID两项负例仍拒绝，所有被读取原件 SHA 不变。
诊断脚本第一版也因遗漏方言 KeyError 退出；第二版逐字段表 SHA
`eb813f7735d9127bf670aa494b4e35fedf492b357267dac5ab21682dda17a178`，存于
`/opt/aries-artifacts/observability-r01-full-agent-review-findings-2.json`。
该诊断临时采用的32字符上限未改变本次未触及该上限的ID；最终复核严格使用源码≤40/无截断规则。
没有新增模型或云调用，没有修改 Strict matcher、Tape、原测量或验收驱动。

此前 `observability-r01-full-agent-1/evidence.json` SHA `ed5889e47d50e993593b89229318df0201a456c2a12550133d1a9f7da4fdd671` 为 **FAIL保留**：发布pins尚未生成即启动，preflight FileNotFound；record/replay均NOT_RUN、actual_creates=0。full-agent-2才是本计划首次真实模型录制，不是失败后付费重录；不删除前次setup失败或把它计为已执行模型attempt。

发布记录均位于 `/opt/aries-artifacts/observability-r01-native-publish-{1,2,3,4}/evidence.json`：

| 发布核验 | SHA256 | 结果 |
| --- | --- | --- |
| 1 | `ae2ea0145658ebafe388a1a53e5f2f13ab67520d66e7f48872c79d075e2579a8` | FAIL保留：旧登录凭据push失败 |
| 2 | `7670e47efdb8ec9ac7b9c5d61c3a6aa38e3bfa3eba6617d30af163a980be291f` | FAIL保留：AK/SK派生临时认证后push实际退出0，但错误要求config digest等于本机image ID而核验失败 |
| 3 | `871f8eea22d7cd4ee36c0f51a98e711d4608cb67df86ec45338f447ea523030e` | FAIL保留：raw OCI manifest SHA、RepoDigest与候选身份核验成功；buildx留下元数据使临时认证目录清理失败，无再次push |
| 4 | `29f377daf7144298f2c852b1db184ddabd0934145a430427d2c1f86b19f05cfc` | PASS_PUBLISHED_CANDIDATE_ONLY：清理本次目录后复核发布身份通过，临时认证移除；无新push/Sandbox |

本机Docker采用OCI manifest身份：本次raw manifest SHA与RepoDigest均为 `sha256:cc7b2c8270be5ca0f5169b7b9f0b6574fd56f6f559cb778b9cbfc467b7b8e664`，config digest独立为 `sha256:644a8b686ace7661ae3a28a0b500b3e71edbdf97893c499a04cf6ea28e0416ce`。真实profile固定仓库tag@manifest digest；不是把未发布image ID伪造成digest。

## 后续增量验收：证据与复现范围

以下目录均位于授权ECS的 `/opt/aries-artifacts/`，名称中的 `current` 只表示实验执行时的冻结版本。复现必须使用该目录的源码manifest、固定image ID、原始日志与输入；不能从本地最新文件推定旧证据自动覆盖。各次失败原件保留。

| 实验/证据文件（相对上述根） | SHA256 | 已验证范围与边界 |
| --- | --- | --- |
| `observability-r01-protocol-review-1/evidence.json` | `cb5534e45a51740265345cb6ca9b6db8e6e5bc0d74b764d18a752791b6fa340d` | 修复前3项缺陷复现FAIL保留；修复后SSH包unit/race PASS，含同channel身份、继承context清除、opaque ID编码。非整Agent验收 |
| `observability-r01-python-binding-1/evidence.json` | `e482d1c3c05b0b3d11a4be32c85027e21dae4920becf36145c1fa987969d895e` | 离线E2B/华为测试、guest identity probe、身份修改后的R07语义重跑均PASS；无云调用 |
| `observability-r01-native-cloud-1/reconciliation.json` | `94f89f9673995755dc303374780670d7ab14e35d94b1604f52a369d657a53e9c` | 真实固定native createExecTool→SSH→Bridge→SDK→E2B guest 9/9精确映射；1个Sandbox确认absent。非LLM/Gateway/Verifier工作流 |
| `observability-r01-output-order-4/evidence.json` | `59d35e5711719149a5e7efea0ca077fbb43f3fd098404044b4a226ac05dbce2b` | 历史旧版：5组helper、18个A真实SSH/Bridge场景、4个B真实本地原生工具并发PASS；15个前台以allowBackground=false运行，后台/poll/真实PTY走旧旁路。未验证默认允许后台时的前台renderer，也非完整Agent或云回放 |
| `observability-r01-output-order-5/evidence.json` | `839f832705065dcd347d1e0ceed3735bf6ba8e3de8aa363516426dfc67ac51ad` | 保留后台能力的新版本首轮FAIL：helper与4个B前台并发通过，baseline A native退出13，candidate A未执行。独立probe缺常驻harness句柄，原生poll使用unref等待；失败原件保留，Bridge与本次两个容器已确认回收，随后另开-6回归 |
| `observability-r01-output-order-6/evidence.json` | `c661bd005bfb2ac7e54ce65dfc06e14a3a961ef8478063627f5e4825d9ba10a6` | 默认允许后台：5组helper、4个B前台并发、A固定26场景限定PASS；25项预期行为，1项原版生命周期缺陷单列且normal_lifecycle_pass=false。17个前台分流IO/命令/环境对账、7项后台完成后poll/log、真实PTY；Bridge与4容器确认回收，无云/LLM |
| `observability-r01-native-observer-2/evidence.json` | `eccdc505ef2168bb9d5cdeb7085f93b59de33a606335fe259d4a69e57d6526c1` | Go native证据隔离unit/race/vet PASS；14项health负例及6种Run/Stop/Observer对照，cloud子报告独立于cleanup。base为combined-release-1/source加5个Go文件，manifest `ff0b1502e49d0b6fedb9893067c21e99e125c65e57316709231ba69f0e53648c`；非新JS/Snapshot/整Agent组合 |
| `observability-r01-producer-failures-2/evidence.json` | `19025d9fbe750e1d1e81696af2ae124df363be121e2574f218371ef90f9f5f74` | JS helper的21案例、旧ENOSPC阻断业务负例及独占tmpfs真实ENOSPC正例PASS；三个测试容器确认absent，无云/LLM。真实子进程stdout/stderr/exit保持；同步延迟无界、旧干净status残留反例保留，非完整native Agent |
| `observability-r01-snapshot-identity-3/evidence.json` | `d1ac7e1116a59c9a0c1a42cc8de6faf1a8e872589d6fde1de3855f4f4f2078b4` | 身份Snapshot定向unit/race/vet/build PASS；manifest `bf09b4a6cecde5c785b812fc2ff66de917f50420f47fd1224a3a6b3437ee3a16`。精确来源指针、同exec分母、无时钟/flow；缺失或部分无效来源计数为null，旧无health来源仍unknown；不是整Agent验收 |
| `observability-r01-snapshot-reexport-1/evidence.json` | `c7d3c4d23e55a935dee7cafbc02ac4b6159bc709647e677864a112d47a611e2b` | 三个历史run离线重导出与Trace Processor PASS；原始/旧导出不变，Snapshot仅parser与ID变、Trace仅snapshot ID变；A/华为A原incomplete和B coverage unknown保留，无新云/LLM/UI实验 |
| `observability-r01-real-a-wiring-1/evidence.json` | `37cd8aa91b9f480494983a312556d0fa7a9e01c42b282ef4fadbe400dc5c097c` | opt-in A接线format/unit/race PASS；scope明确无部署、无云。source manifest `0881c48ba68acdebfa20c64220dc2327978c26ec495f12c85c6c99e034255164` |
| `observability-r05-reliability-2/evidence.json` | `7564fc55b9f2d30ace3fe928c7be150c59c7c2f7d6440bebe7f0e6aa038bacf7` | 两writer定向/race及build/test/test-race/lint/integration PASS；manifest `13c43e3005ab3f3f552378b4e2fb2317c5cb75984e6ddd165bbdf9cd7789999b`，不含并行R01/R07最新组合 |
| `observability-r05-reliability-2/fault-matrix.json` | `d223df291c9709aa8eb6908df37e699d0954a1890f262c51ffa77a7e45d26071` | 22项逐条状态；17项PASS中包含Write/Sync阻塞反例，并非有界关闭通过。4 BLOCKED：生产Write阻塞、Sync阻塞、late marker持久化、断电/父目录耐久；legacy/provider writer为UNRUN |
| `observability-r07-semantic-1/evidence.json` | `09c0e980a9c401fd1d7b911d88b4f685b413989a67a124e76444a4644ff4f9ab` | wrapper语义10场景/20次真实ECS本地进程，owned process已回收；不是云guest终态 |
| `observability-r07-shared-compat-1/evidence.json` | `3d1f36e284709037715b7035f4634a97fe3e729afa4340b0f04eb3aafc8f21e9` | network=none的固定E2B/华为adapter镜像兼容测试PASS；未重新请求华为云 |
| `observability-r01-python-binding-1/probe/r07/evidence.json` | `e12dac6c437a4d6d81080ac8bd5c73d670579957a8eb218e285e6e17977e97c3` | guest身份修改后再次跑R07语义；绑定guest_exec SHA `c7f8ac5257ae7aa0d78fb83541b997a839632640dae28337beee8f8eb515993c`，不能拿较旧closure-4替代 |

R01真实云的执行原件 `native-cloud-1/evidence.json` 保持 `EXECUTED_PENDING_RECONCILIATION`；PASS来自上表独立对账。九次调用包括同命令、相同ToolCall重试、反序完成和非零退出；native producer witness与target按精确ID关联，辅助调用保持unlinked。slow/fast的controller wait观察到重叠，同命令pair未观察到；guest单调时间缺共同保留原点，不能推导guest并发。原始guest输出帧未全量归档，分流摘要在ExecStream返回时核验；这些限制均保留。

`native-poc-3/reconciliation-v2.json`（SHA `749c0b69de73e0ece72749207d2450dd8559f5b59a178d7b3a9080c2a7b42fa7`）仍为 `FAIL_NATIVE_RESULT_EQUIVALENCE`：九条身份与分流IO/exit匹配，但混合stdout/stderr的native最终文本不同。output-order-1的fixture语法失败和output-order-2的timedOut字段差异也保留；修复实现后output-order-4完整重跑，没有放宽matcher或用旧结果代替新源码。新renderer只规定stdout后接stderr、不插分隔符、UTF16安全截断与原有trim/exit格式；真实内容变化仍应strict fail，成本尚需重新测量。

复现历史输出PoC时，在ECS使用该次已归档的六文件输入与独立空输出目录运行 `run_output_order.py --output <新目录>`；其脚本会检查固定native输入SHA、实际base image内main/runtime的SHA，编译独立镜像并验证清理。不能拿当前改写后的脚本替代旧输入再声称复现同一版本。

最新用户约束是保留原有工具能力：不因启用renderer改变工具policy，不禁用 `process`、后台、自动yield或PTY。新版本在原生控制流真正决定返回前台结果之后才应用确定性格式，输出以首次native wait/error边界冻结；后台registry、poll/log、通知与live update保持原路。默认 `allowBackground=true` 的新矩阵预登记26场景，双方结果数量、唯一性和名称集合已对账：25项覆盖实际前台、4次瞬时后台、自动yield、yieldMs=0、取消、finalize错误及真实PTY，另1项诊断退出后finalize延迟跨越yield的原生生命周期边界。原版和候选该例都出现poll无会话，仅记语义一致和原生缺陷，不计生命周期PASS。首轮-5退出13保留；-6才是此次修订的限定验收，旧-4不能替代。

独立probe修订增加与真实常驻Agent等价的保活句柄，快速后台采用双方相同的固定等待后检查已完成poll/log；这不验证在途poll等待的全部行为。collector首次wait冻结后，即使finalize稍后抛错也复用该视图，不声称这是独立pipe-drain证明。原生registry的`markExited`先移除运行会话，仅当已backgrounded时保存finished；稍后才yield的无会话路径已在-6双方复现，状态为`KNOWN_ORIGINAL_LIFECYCLE_DEFECT_REPRODUCED`。

-6的六文件输入与当前审阅版本一致，patch SHA `3aeeca7c9028f256f55820d3783539d53600b8d898311e52e6b0b3832a4fdd59`，helper SHA `5951ff9baecbf121d9d21639c52fa120651b953e29d025914483442560e2d406`。A候选image ID为`sha256:91c0b2bad69fadee686482b867de4a410d5052a1a5e3127e6b39933c4f5b5264`，原版B叠加renderer的image ID为`sha256:dbfe0198f66800f47e5332769adf4becb59b4d571e317d070936b68c98288268`；只读复核38份归档文件SHA、双方26条原始结果和后台/PTY边界一致。不包含新producer持久化、当前完整接线源码、新Tape或整Agent。

新producer/collector故障须分开验证协议无效时拒绝SSH与观测故障时业务不变、证据无效。Go侧`native-observer-2`已证明native采集错误不再加入Harness/Stop错误，而独立进入Observer：健康status严格9字段，身份匹配、attempted=written+write_failed、written=保留事件数且失败计数/latch为零才称文件valid；缺失仍失败，独立调用分母仍unknown。14项health负例、6种缺日志/来源ENOSPC/失败latch/4MiB超限/产物或index写冲突均通过，缺陷注入未耗尽生产磁盘；原有legacy artifact失败语义没有改。cloud composition同时检查失败Observer不改变cleanup，非opt-in不创建native报告字段。

复现Go修订使用`/opt/aries-artifacts/observability-r01-native-observer-2/source`与目录内manifest：`go test -count=1 -v ./pkg/harness/openclaw ./internal/app/wiring/cloud`、同两包`go test -race -count=1`、`go vet`，全部在ECS执行。5文件为`pkg/harness/openclaw/{harness.go,native_identity.go,native_identity_test.go}`及`internal/app/wiring/cloud/{cloud.go,cloud_test.go}`；collector SHA `90ba4532fffa8d1522e9e46e3322b6d23527b27907f8e280b588426e007e48a8`。较早`native-observer-1/evidence.json`为 **FAIL_SETUP**（SHA `e3b45483096739f64b91d73cb9c44e580f2401faa1df8240734f6b8a5e8b6958`），因不存在的base路径而未启动编译/测试，保留原件。

上述Go验收不覆盖新JS producer、Snapshot扩展或完整Agent镜像。JS独立证据`producer-failures-2`绑定helper SHA `6140aef076578afbe0c3324db40007a84e344e68bb3ce1c2c416105edb62eba0`、test SHA `e01d30fe3dd85ac22e7b5ff2074b2dc097a503fcdbd4250c195a8a13f4bc6221`；21案例包含短写/close/rename故障、FIFO/软硬链接、4MiB上限、producer冲突、同实例计数回退、scope/session拒绝及旧干净状态反例。另保留旧helper ENOSPC导致真实业务未执行的负例，以及独占tmpfs真实ENOSPC下新helper仍保持子进程字节与exit=7的正例；未填满ECS根盘。只读复核9份归档SHA和21条结果，非整Agent验收。

Snapshot扩展有上表独立`identity-3`证据，`pkg/performance/snapshot_identity.go` SHA `ae0d137607bf287fb7f239b876e08622a3657c1f3909c236aa30e6e27f2943d3`。expected/observed/started/paired统一计数归档exec调用，producer invocation与辅助Bridge记录另计且仅在完整有效来源后赋值；missing、合法前缀后坏行均保留null。来源须为静止归档普通文件，FIFO拒绝，producer实例限定256字节ASCII，与collector一致；不声明普通文件读取可取消或消除了TOCTOU。原始SHA/JSON位置与精确ID保持可追溯，不新增虚构时钟、跨度或因果边；该范围不替代完整Agent请求、Verifier和清理对账。

`snapshot-reexport-1`另核对原E2B A、B和华为A三组：A/华为A各60个interval和47条flow，导出退出1/incomplete符合原有缺测；B为217个interval和31条flow、退出0但coverage仍unknown。新旧JSON在排除parser版本/内容ID后保持相等，Trace仅snapshot ID变化；Trace Processor导入时长误差0ns。原始数据和旧导出SHA不变，不改写历史PASS/FAIL。UI依据旧投影已验证且本次内容相等，没有新UI交互证据；该重导出不包含后续full-agent-2真实录制/回放。

普通文件同步I/O有界退出、所有日志/status持久化都失败后旧干净status无法失效仍BLOCKED：实际业务执行2次而磁盘健康状态仍为attempted=written=1且latch=false的反例已实测。不能据旧干净记录推断无丢失，须有独立Tool调用分母；status只覆盖其producer实例，不作多进程汇总。output-order-6继承旧identity helper，不能覆盖这些新变更；JS、Go和新Snapshot的统一源码/镜像门禁见本页combined-release-3，完整Agent的E2B A限定验收见full-agent-2，华为新链路首次回放FAIL，修复后待验。

R01完整录制还必须固定最终镜像/源码与原生工具policy，核验实际前台/后台分支、实际下一轮模型请求、新Tape、独立Verifier和隔离/清理；ToolResult中的renderer元数据与模型可见content分别验证，不假定provider会把details发送给LLM。**现有PoC及驱动离线检查不表示这些步骤已执行**。combined-release-3通过范围只限统一门禁和独立native；后续full-agent-2另有E2B A实际证据，不能回写旧PoC为整AgentPASS。

旧 `observability-r05-closure-1/evidence.json` 因literal `\\n`不是合法JSON，仍保留但不作为机器证据；`current-source-validation-1`（SHA `595b0694701da34c90015feaae1c171e10af58bd51840d5812aa0abc8de5d225`）也只覆盖当时manifest。R05的index发布冲突不等于真实磁盘耗尽；文件故障由真实临时文件包装器注入，未耗尽或重挂载生产磁盘。

### 完整指令的十组矩阵对应

下表避免把“全量测试命令退出0”误写成所有工程能力已经验收。历史UT来自release3的`make test`和`make test-race`；后续R01/R05/R07分别使用上节注明的源码与门禁，不互相替代。未实现能力没有用替代断言充数。

| 组 | 已验证范围 | 未验证/阻断范围 |
| --- | --- | --- |
| 1 身份与并发 | 协议unit/race、Python绑定、真实九调用PoC；full-agent-2各6/6归档exec关联；overlap-1同命令guest重叠/反序完成2/2精确关联 | 全量漏采分母仍unknown；后续云协议重连/取消/身份拒绝、华为A同Tape和[完整A单调用取消](#r01-native-cancel)均另有限定证据，不能外推所有组合 |
| 2 Strict与模型 | 原严格历史、旧轮Tool Result偏离、busy/retry/断流/时序与新请求边界UT通过 | 原生Harness压缩、所有并行Tool形态的完整真实回放不在本轮证据范围 |
| 3 工具与停止 | wrapper分别观察direct-child退出与pipe EOF；真实SDK20次/10对兼容并保留取消后child存活反例；当前组合A真实SSH断开后Stop确认group为空，B真实Runner Stop失败时Verifier调用0，2个Sandbox独立NotFound | A此例不含完整Agent Loop；B原生工具精确终态、更多持续写/Kill超时及其他云负例UNRUN；不以SDK managed PID消失推断全树终态 |
| 4 丢失与落盘 | R05整对掉失/producer分母、join、index冲突、文件故障与阻塞反例已有矩阵；legacy已返回错误分离、Strict/Tape负例及实际CLI三案例通过 | 生产有界Write/Sync、late marker持久化与断电耐久BLOCKED；legacy/provider完整可靠性故障矩阵仍UNRUN |
| 5 时钟 | 共享anchor、无anchor分域、重叠、ns→µs投影UT通过 | 真实UTC跳变/重启、跨主机校准与漂移上界UNRUN；不跨域相减 |
| 6 源与资源 | native源缺测、B不走Adapter、下游execution/Process身份的代码与回归证据 | 既有资源流、重复/缺测与真实出口见[P1c限定验收](#p1c-resource-prometheus)；provider reset、华为时间字段、短任务峰值仍UNRUN |
| 7 双Trace | 统一Snapshot与Perfetto纯投影回归；后续[P1a离线验收](#p1a-offline-jaeger)已补真实A/B/取消的Jaeger查询、重复入库、Link协议夹具及受控partial success/timeout测试 | 真实业务跨域Links、UI、实时事件、认证/大批量/生产持久性未验；重发不保证exactly-once |
| 8 Metrics | 同快照原账本与阶段Histogram已完成3真实样本首发/重发查询；历史/乱序/同时间冲突/reset/窗外另有协议验证，见[P1b验收](#p1b-offline-prometheus) | 资源Gauge限定范围已另通过[P1c](#p1c-resource-prometheus)；真实Goodput、跨批次生产并发与实时/耐久UNRUN；单快照隔离TSDB，不造rate() |
| 9 真实云 | 历史release3三组合、E2B native PoC与full-agent-2；新增overlap-1及sdk-real-1各1个Sandbox，所有尝试和回收分别归档 | 新renderer/最新组合华为同Tape已FAIL（关闭stdin错误）；修复后回放待验；华为网络隔离仍assumed/unverified |
| 10 观测成本 | R15历史六次另存；r15-release-1同源baseline/L0四对八次真实回放、阶段/共享资源/四类writer计量和算术核验已完成 | 不覆盖任意后续源码；无预设合格阈值、无p99/容量结论，新L1/L2与完整成本未验，L2无profiler |

## 已重新核验的历史真实材料

这些只证明原版本成功，**不计为新埋点验收**。本轮已在ECS只读检查具体文件及SHA，原FAIL没有删除。

| 事实 | 路径与SHA |
| --- | --- |
| portable A同Tape跨云旧复核16/16 | `/opt/aries-artifacts/portable-crosscloud-review-2/evidence.json`；SHA `57c4daaa328fc7b69dc91ff94136f5bfbb4007d2fca9b9f9cf9950f556999176` |
| 原末步checker失败仍保留 | `/opt/aries-artifacts/portable-e2b-1/record/evidence.json`；SHA `86ae57cdc2942bcc62a59108e17617ab0e7a0a877e54746f1257bfb1f74879a0` |
| 旧review1失败仍保留 | `/opt/aries-artifacts/portable-crosscloud-review-1/evidence.json`；SHA `3f59981408dfaa0decc6a51aaeebaef8c8b842060c7ccdca83ff7ab9c64d8c51` |
| 原B成功回放 | `/opt/aries-agent-bench/runs/regressions/agentsphere-runner-ab-1/b/20261023T125413.718075155Z-openclaw-e2b-embedded-replay`；run-result SHA `c57f06ae3e990ca2b5ef4facf4c94fefc40f1aeb835ccaa67be6e7c00cac0b48` |
| 原B接受证据 | `/opt/aries-artifacts/e2b-runner-regression-ab-1/b/evidence.json`；SHA `a2be0e30c0e44f3795fa3a28896d72f0639cf9d03b943d322da5ae45f7517b85`；review SHA `500d8982346ea0c01c48e9e2882955bc1ef077d98a06e23f5ced1c6637ec6982` |
| 原B native index/events | 上述run下`merge-ranges-001/harness/`；index SHA `ea4658dc82b300d0bf39b77f398346f844f2c91595f867e5763e93d1b5a9c074`；events SHA `3d273b7e66dc912072a642f6ea8dee0c6a50622760f5cbd7ba898a40e07e9321` |
| R01真实固定镜像源码核查 | `/opt/aries-artifacts/observability-r01-native-source-1/evidence.json`；SHA `ea2a38c6ce5febbf10f62f2886ed47ce56eec8e58a9c5e5675124bbc94d3997d` |

原B实查4 model/3 Tool/573 SSE、native start/result/end各3、12项测试全通过、stop/revoke confirmed、cleanup succeeded。原portable A为8 model/7 Tool/675帧，保留两次预期pytest失败以及成功复测后的cat。验收标准事后修订是已披露的选择偏差，不称全部事先冻结。

## 历史 release3 真实云实验登记

先冻结表，再运行；release失败不得进入云实验。每项C1×1，顺序执行，使用真实工具与独立Verifier；无付费模型录制。每次最多任务+Verifier两个Sandbox，有界TTL和job watchdog；只回收本次owner新增资源。不得改VPC、SG、VPCEP或全局累积创建配额。

| 组合 | Tape SHA256 | 计划/执行状态 | 范围 |
| --- | --- | --- | --- |
| E2B / A external / recorded replay | `07be09fa95cb3678c94c43accdc4b96ae079d6ae2352505091eeff134e6065bd` | PASS（功能及限定观测） | 8轮/675帧/7工具/12测试；15个Bridge→Process映射；该旧运行未采native ToolCall上游关联 |
| E2B / B embedded / recorded replay | `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb` | PASS（功能及限定观测） | 4轮/573帧/3工具/12测试；89个Gateway delivery；coverage仍unknown |
| Huawei / A external / recorded replay | 同E2B A | PASS（功能及限定观测，egress假设） | 同8轮/675帧/7工具/12测试；15个Bridge→Process映射；禁止出网仍assumed/unverified |

华为B并非未测试：历史 `agentsphere-embedded-record-3` / `agentsphere-embedded-replay-1` 已完成原始 `merge-ranges` 工作负载的真实录制与严格回放（4轮、3工具、693帧、12项独立验证），但该结果早于release3新观测字段，且不在此三组合矩阵中。后续补测须固定新的候选Runner/原生镜像/Tape，并单独验收native coverage、进程终态和出网状态；不能用历史release3或本地B输出PoC代替。

A/B的Tape/工作负载不同，只用于兼容回归，不能比较供应商或placement性能优劣。正式同placement性能比较还要求相同版本/spec/region/冷热条件/观测深度与独立验证标准。

兼容率分母是实际进行兼容性检查的预登记组合，另列计划未跑、环境失败、Strict Diverged、retry及所有尝试完成率。本轮预登记3组合、执行3、通过3，功能兼容率和全部尝试完成率均为 **3/3**；计划未跑0、环境失败0、Strict Diverged 0、retry 0。没有因此宣称任意工作负载100%跨云兼容。新付费模型调用0；每项任务+Verifier两个Sandbox，共6个全部absent，临时Huawei模板删除。没有修改VPC/SG/VPCEP。

每次冻结候选image/source manifest/输入Tape SHA。验收原严格历史与完整工具结果、完整模型轮/帧及recorded时序、候选SHA、12项独立验证、Stop/revoke、正常cleanup及资源absent；追加observer counts/clock/IDs/sourceSHA检查。缺失身份不能因功能通过而宣称全链性能有效。

| 本轮证据 | SHA256 |
| --- | --- |
| `/opt/aries-artifacts/observability-p0-replay-1/evidence.json`（E2B A/B总作业） | `134752d89b2d77efd7147e5dfa8bf8786213d0c7a41b4e0786f004c6e423dc3b` |
| 上述根下`a/evidence.json` | `e76f5f69a2536e5515e3b4a1cb2c009b1b55d95b7f15c444e140b9fe78799b4f` |
| 上述根下`b/evidence.json` | `790493b483c781cff7ab4a17a9cf93614f334753460f5371ad727cb9af6b1a56` |
| `/opt/aries-artifacts/observability-p0-huawei-replay-1/evidence.json`（真实执行记录） | `3e52b7a5710f9b7001ae6a7b827779f7a19933f0cc0f4d0b5e90e11bf7525418` |
| `/opt/aries-artifacts/observability-p0-final-review-1/evidence.json`（三组合离线复核全部PASS） | `edbf9d2a613bfcb1ac45c0787cd37628806026020e6d458b2f4014baed68d561` |
| final-review-1/metrics.json（本轮阶段与细分原数值） | `265ba949ec3e093365ab8876eee278cbe33095b7f6377056dff72855bd3c754c` |
| `/opt/aries-artifacts/observability-p0-perfetto-3/final-evidence.json`（修正版 Trace 原生TP+实际UI） | `b87f277816c8a2f6e751029c6ef15bc2528e8658fc6757aeece94f5ec8a2bb42` |

最终复核只读原件，按字节复制新run后分析；144个输入文件SHA首尾不变，不复制旧Tape、不新增云/模型调用。对A核对相同工具决策、完整结果正文与完成语义；B按原生结构事件与严格模型历史核对。评分候选均为独立12项全通过。缺少独立SSE线缆抓包，帧证据是代理成功flush记录及客户端完成/strict请求，不把它夸大为网络逐字节抓包。

## 离线导出与Perfetto

以下在ECS使用修复后的`/opt/aries-artifacts/observability-p0-exporter-1/aries-perf`。`RUN_ROOT`指具体私有run，输出文件独立创建；部分快照可能以非零退出保留诊断，必须检查原因，不能统一忽略exit code。

```sh
/opt/aries-artifacts/observability-p0-exporter-1/aries-perf --snapshot "$RUN_ROOT" > analysis-snapshot.json
/opt/aries-artifacts/observability-p0-exporter-1/aries-perf --trace "$RUN_ROOT" > trace-primary.json
/opt/aries-artifacts/observability-p0-exporter-1/aries-perf --trace-clock "$CLOCK_DOMAIN" "$RUN_ROOT" > trace-domain.json
```

当前CLI每次调用生成相同内容身份的Snapshot；出口内部只消费已验证快照，不能各自猜关联。保存Snapshot文件SHA、snapshot_id、Trace manifest及源SHA用于核对。

本轮实际导入暴露并修复了产品问题，不能将此前JSON/UT通过当成导入成功：

1. `observability-p0-perfetto-1/native-sql-evidence.json` FAIL，SHA `ee5be7fe8f081e939bdad7b3e9ae81b7b4d8d0767913d105e4e4883c4f30c6cb`。checker选择了空operation ID，且产品flow ID格式有缺陷。
2. `observability-p0-perfetto-2/native-sql-evidence.json` FAIL，SHA `c32f2fb958f0fb33b0224aa9bf0c2932223be5fa9591169f1a8f7060f29c7bf3`。修正checker后，A/B的60/217个真实operation及ns精度通过，但flow均为0，`flow_invalid_id`为94/62。实际UI v59.1也复现A的94个错误；截图位于该根下`a/ui/actual-ui-error-stats.png`。
3. 独立修复仅涉及`trace.go`、`trace_test.go`：显示层flow ID采用可解析的UInt64十六进制编号，完整稳定因果身份仍在args，终点使用`bp=e`绑定对应slice。依据[Perfetto官方解析器](https://github.com/google/perfetto/blob/main/src/trace_processor/importers/json/json_trace_parser.cc)。未更改snapshot/原始事件/时长，metadata升为v3。
4. `/opt/aries-artifacts/observability-p0-exporter-1/evidence.json`，SHA `b26006918a980ffef299f906ccbba9374b7e3c9b7502340853a4f37e4ec9544c`：ECS `go test -race ./pkg/performance ./cmd/aries-perf`、`go vet`同包、独立`go build`全部通过。三份snapshot字节完全相同，旧Trace与失败原件未覆盖。该补丁是离线导出器，真实云任务仍使用release-3 Runner，无新增云实验。

新导出路径为`observability-p0-exporter-1/outputs/{e2b_a,e2b_b,huawei_a}/trace.json`，SHA依次为`057628b715db86532b46a968dc6067f34d50aa33776a028ee93a58eda5aac4d0`、`5a64862365880c76c6e1ef741490a0d128e2d7363d13cc9a1caa7b1d3825d502`、`77f3465e42bfef881a9395698a5c887700aad463013c0ee6cc538a63ed7485ce`。原始schema仍为v2，analysis schema仍为v1；只有显示出口版本变化。

修复后原生TraceProcessor的A/B实际导入和flow两端ID对账已通过，`observability-p0-perfetto-3/native-sql-evidence.json` SHA `4aece1c2e64e014fbcdc8cd28d06b452e52c8a1d15c2a0b4bdaa24bbb449e018`；实际UI也已通过，最终证据 `/opt/aries-artifacts/observability-p0-perfetto-3/final-evidence.json` SHA `b87f277816c8a2f6e751029c6ef15bc2528e8658fc6757aeece94f5ec8a2bb42`。A为60/47/0，B为217/31/0（operations/flows/errors），两端flow operation ID精确匹配；选中model.http_request的UI时长与原始ns一致，浏览器已关闭。不能用SQL通过替代UI；本次两者均有证据。相关官方说明：[TraceProcessor](https://perfetto.dev/docs/analysis/trace-processor)、[UI导入](https://perfetto.dev/docs/visualization/deep-linking-to-perfetto-ui)。

## R15 历史冻结组合的六条实际记录

使用同一 E2B B Strict Replay Tape（SHA `73fafa3d2ea3a2634dff8c6276fab4309d97d7e9671d26d9dfaa669004e1f5eb`）、embedded placement/backend、`merge-ranges`、并发1、单attempt和recorded timing。Runner是 `/opt/aries-artifacts/observability-p0-current-source-validation-1/source/bin/aries`，SHA `0c24ff932e4cca8b6674ad6ffeb78861a00e92c5714f7cc5c9c3fd816ec203d8`，源码manifest `39dff992bfe7d5fb01121dd63bcc0b6c7c46e36fc539ae44370ccfdefca435f0`。这些是实验当时的版本，**不覆盖后续R05 reliability、R01/R07或新renderer组合**。

证据 `/opt/aries-artifacts/observability-r15-current-1/evidence.json`，SHA `bd321f265c829ca34d2b7bf458d20570003cb54f4086c853fa84a693929dec1a`。下表按 `samples` 原执行顺序列出，六次均Strict Replay、真实工具、独立评分、停止隔离与清理成功，无模型录制调用。这里修正文档先前误放的旧矩阵数值，没有改写任何原始证据。

这份最终evidence来自**运行结束后的离线seal补充对账**，不是原驱动直接完成完整验收。归档驱动 `/root/aries-setup/uploads/ecs-r15-current-source-replay.py`（SHA `14180de77730f8a54abcf870981be965df185fe0939966b8c9a7d8bba4693df8`）存在两项缺陷：没有以子命令非零退出拒绝PASS，且在错误层级查找run-result；它的 `PASS_REPLAY_COMMANDS_COMPLETED` 不足以证明功能通过。后补 `ecs-r15-current-source-seal.py`（同目录，SHA `4e418844d226c6862fd548621b827e1aaff82fa636cc88b6da733496a10e579a`）改从各sample递归读取唯一run-result，检查Harness/评分/隔离/清理四状态，并补写同一evidence文件；未找到另存的独立reconciliation文件，不将它描述成原驱动已有的验收门。

本次只读复核再次核对六份run-result的SHA、`model_sessions`均为replay/recorded/succeeded、停止和撤权标志、task及infrastructure cleanup，并直接解析六份Verifier XML：每份12项、失败/错误/跳过均0，候选文件SHA均匹配。baseline两条model目录按设计为空，所以没有可供独立重建每个Strict请求的原始模型事件。seal中的 `provider_resources_expected_absent=true` 是脚本常量；本节不将它当成云API确认absence，也没有为文档复核重新查询或创建云资源。

| 顺序/样本 | 任务耗时（秒） | Runtime（秒） | Verifier（秒） | Runner CPU（秒） | 最大 RSS（KiB） | 归档产物（字节） | 资源采样状态 / SDK快照数 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 / baseline-1 | 45.048 | 15.554 | 4.353 | 0.53 | 31216 | 20129 | not_run / 0 |
| 2 / L0-1 | 43.978 | 15.844 | 5.028 | 0.65 | 34340 | 1707411 | not_run / 0 |
| 3 / baseline-2 | 40.072 | 15.543 | 3.539 | 0.53 | 30772 | 20137 | not_run / 0 |
| 4 / L0-2 | 39.214 | 15.866 | 5.300 | 0.57 | 35084 | 1575676 | not_run / 0 |
| 5 / L1-1 | 43.617 | 15.943 | 4.956 | 0.66 | 32712 | 1813230 | succeeded / 12 |
| 6 / L2-1 | 45.242 | 16.141 | 4.646 | 0.65 | 35108 | 1774269 | succeeded / 12 |

取数口径：任务耗时为 `samples[].task_duration_ns / 1e9`；Runtime/Verifier分别为 `stages_ns.runtime_execution_duration` / `verifier_duration`。任务还包含准备、启动、停止和清理等其他阶段，不能仅将这两列相加。Runner CPU是同次 `/usr/bin/time -v` 的user+system，RSS也是该Runner进程口径，不是guest工具CPU/内存。SDK快照数从该sample指定的run-result中 `tasks[0].observer` 读取，并与 `sandbox-metrics.jsonl` 行数核对；它不代表12个独立provider采样点。

baseline未创建可选观测writer，不能据此声称drop=0。L0/L1/L2有drop字段的model request-boundaries与performance writer均为0；E2B operation/native-tool索引没有该字段，仍为unknown。L2与L1代码路径相同，没有P2 profiler。样本少、没有预注册扰动阈值或冷热控制，不能给出成本合格、p99、因果开销差或供应商排名；后续E2B A新组合已另行预登记实测四对，不能将其回填到这组旧B样本。

后续测试环境已变化：ECS根文件系统由40G扩至99G，扩容后约37G已用、58G可用、使用率39%，服务未重启。证据`/opt/aries-artifacts/ecs-rootfs-expand-20261024T100351Z/evidence.json`，SHA `d518f6fdd8704a1526c9b2dbf9a4f3c21eecee67dc9d39a0118f203fd2bcae0e`。新的扰动矩阵必须记录扩容后环境和缓存状态，不能把上述六条旧样本视作同环境对照。

旧 `observability-r15-1/evidence.json`（SHA `f4485eb559560491bb70413059b31f3bac345792e16763c585692e377ce44adb`）及初次baseline关闭观测路径后访问operation log的panic（exit 2）独立保留。不能把旧矩阵与此六条记录合并成一个源码版本的结果。

## P1/P2验收状态与剩余退出条件

- Jaeger：本期离线限定范围已[实际通过](#p1a-offline-jaeger)：固定版本/skew0s、真实A/B/取消首次及重发query、显式Link协议夹具和清理。真实业务Links、UI、大批量、认证与生产持久性仍未验；不把受控HTTP故障测试说成后端真实故障，不把稳定ID说成exactly-once。
- Prometheus：固定版本原生OTLP接收器，隔离实验TSDB；HTTP 2xx之外查询真实历史window的sum/histogram count/sum/buckets。覆盖乱序/重发/reset/同labels并发。导入不可靠时保留原快照并归档失败，采用批次隔离/离线表，不改时间或成功计数。
- P2：按schema文档只读权限矩阵逐后端探测；无授权/权限不加载perf/eBPF。应用级与内核热点证据分别报告，包含observer扰动与稳定进程身份。
- baseline/L0/L1/L2：历史 R15 小样本矩阵与原CPU/RSS/产物字节/资源采样/drop保留；新计量源码五项Go门禁及baseline/L0四对实测完成；L1/L2新组合仍UNRUN。后续预登记同条件交错样本和变异处理；性能合格阈值须经用户确认，没有阈值只能作描述性结果，不能把工程门禁写成成本合格。

## R05/R07 故障验收结果与剩余边界

R05 的 legacy `model/events.jsonl` 错误分离已实现并通过 ECS 验收：Write/Sync/Close、短写与非法返回由 `ObservationError()` 锁存，成功模型会话不再因此失败；Strict 偏离、会话未完成及必需 Tape 写入错误仍由 `Close()` 返回。实际 CLI 单独报告 `observer_status=failed`；[源码、故障注入与三案例 CLI 证据](VALIDATION_AND_REPRODUCE.md#r05-observer-separation)分别归档，不代表当前组合已做云回归。

R05两个最终fsync受控延迟已实际验收：任务Sandbox均已独立确认不存在；model关闭仍挡住1个共享network，由实验owner回收，Recorder关闭前共享资源已清。两个workload中断、Observer不完整，不计完整任务成功。详见[收容证据](VALIDATION_AND_REPRODUCE.md#r05-fsync-containment)。生产有界I/O、L1同步采样挂起及耐久性仍BLOCKED，不新增监管框架。

R07[真实SDK对照](#r07-real-sdk)仍保留managed PID消失而child存活的反例；[当前组合停止门验收](#r07-current-isolation)已补齐A实际SSH断开后group Stop/撤权及B实际Runner拒绝评分的两项负例，并独立确认2个Sandbox已删除。后续[完整A取消](#r01-native-cancel)已完成一条原生调用的身份与评分前停止验收；当前B已有3个直接子进程区间。剩余UNRUN：B全后代/PTY终态、更多持续写/Kill超时负例及其他云覆盖；不以SDK托管进程消失、Stop返回nil或最终整箱删除代替对应终态证据，不新增killpg监管层改变原生行为。

## 清理与归档要求

release检查生产source/tag/services及E2B ledger前后不变，测试containers/networks/temp按owner回收。云实验分别保存所有新增handles的最终absent证据；cleanup失败与任务成功分开，不隐藏剩余资源。性能数字及每次失败将在 [performance-archive.md](../performance-archive.md) 保留简明索引，原始证据留ECS私有归档。


### 大文本 Dashboard 已有证据展示补齐（2026-10-26）

[展示与测量说明](../dashboard/large-scale-text-editing/SOURCES.md#完整展示修订测试动作采样与原生-trace)：同三份冻结Snapshot补出9个owner资源采样流、20次精确ToolCall动作与6个独立时钟域Trace视图。资源缺失原因由“页面未提取”与原接口unsupported区分；采样极值、累计CPU与生命周期RSS不混用。主Trace原SHA不变，native另投影，TP核验20,361项通过；Coverage仍unknown、后端验收未升级。R08/R09/R18仅新增展示证据；R01–R18原处理状态、R05暂停及P0未完成均保持。ECS构建、浏览器与原始视图核验路径和SHA见链接，不新增真实任务分母。


<a id="workload-replay-release12"></a>

### release12：冻结快照往返修复与实际五门禁

ECS `/opt/aries-artifacts/workload-replay-release-12` 继承不可变release11，仅修改 `pkg/performance/snapshot_io.go` 与 `snapshot_model_policy_test.go`。保留原Snapshot schema、ID与哈希算法，重载时恢复typed `tool_content_differences`；真正保存/重载后验证双出口，并拒绝digest篡改与未知字段。修复前回归复现FAIL，修复后同一真实首轮Snapshot无需改字节便可重载；原失败driver不覆盖。业务与性能回归见[单独归档](../workload-sqlite-performance-20261026.md)。

五门禁build/test/test-race/lint/integration全部PASS，integration约309.3秒。656源码及6二进制冻结，Docker容器/网络前后相同、进程与临时cache清理完成、部署锁释放；没有改旧release11或基准checkout。小回执在 `.cache/workload-replay-release-12/receipts/`，本地两个源码与ECS格式化后SHA一致，不执行本地编译/测试。

- evidence SHA `4aae132eae541198a1743b72bcf8078eac39ed9946fba100acc648dfafbe9823`。
- source-manifest SHA `77f2b087bb4328d4866ecb920e3864dabe0f22ed8b91a23fffd0d7e5cc5f4ff6`。
- freeze-manifest SHA `14451b7f8b27c065599e7be6036991971f27585052525001bfc3ce96f69ab62c`。

工程门禁和旧原件重导不替代release12新Sandbox真实验收；R12/R18记录本次实际缺陷，P0状态与历史Strict分母不升级。


<a id="r01-script-preflight-rejection"></a>

### R01：脚本预检拒绝身份与独立镜像发布

DB record2 的复杂解释器命令被原生 `validateScriptFileForShellBleed` 拒绝，原错误结果存在，但只记录 invocation/started。候选仅用一个局部标志覆盖该 validator 的整个 await，在既有 catch 补 `pre_dispatch/rejected/dispatched=false`，随后继续原清理并抛出同一错误对象。成功校验后清除标志，后续 backend 错误不伪装为未派发。实际 compiled diff 净增8行，复用既有事件协议。

ECS 的真实原生模块探针复现原版3类拒绝漏记；新候选在默认与 terminal-result-v1 两种配置各记录3个唯一拒绝，原错误对象/消息、正常前台/后台/PTY均保持。旧SSH parser/elevated/host探针仍为6 invocation、5拒绝、11记录，没有重复记录。只有exec模块变化；既有16-job矩阵是未变化模块的历史证据，本轮未重跑。全部临时容器已删除，0模型/云调用。

独立镜像 `sha256:39964c39357eee32a1c5fff7c5562aebfca2495fcdeb7a7871dd06fed73134ea` 已发布，状态 `PASS_PUBLISHED_CANDIDATE_ONLY`。registry摘要与读回身份已核对，临时认证已清理；旧e040和默认配置保持，后续真实录制必须显式选择新版本。此局部修复不把旧DB的评分/SSE失败改为成功，也不关闭R01全部分支或补写旧缺失身份。DB record3已实际使用该镜像，唯一拒绝为既有elevated分支；新增validator拒绝分支尚未被整Agent任务触达。53次exec身份完整对账与原生计时collector的event_limit_exceeded分开：后者使整体观测仍incomplete。

- 定向证据：ECS `/opt/aries-artifacts/r01-script-preflight-rejection-1/evidence.json`，SHA `b7e4e0aeffc0ccc559bb955b39d7ca4bf37dabbcc5dc8bc6eb50d43f1b4c22ee`。
- 发布证据：ECS `/opt/aries-artifacts/r01-script-preflight-publish-1/evidence.json`，SHA `b6551b9959eaf494c9c77e6a100505cfca02945a137068b6a2e323f467512951`。
- 完整范围、构建与探针入口：[R01实验说明](../../experiments/r01/README.md)。


<a id="record-failure-release13"></a>

### release13：模型失败分类贯通原事件与双 Trace

ECS `/opt/aries-artifacts/workload-replay-release-13` 继承不可变release12，仅叠加4个Go文件：record错误分类及真实HTTP回归、Snapshot既有原因白名单及未知文本投影负例。原有请求边界与legacy事件记录固定安全分类；同一Snapshot投影到Perfetto/OTLP时保留该分类，任意未知错误文本仍转成 `unrecognized_reason`。未新增原始错误正文、重试或宽松接受规则，Tape/session仍按原语义失败，不为旧DB丢失的末帧补定根因。

7个真实HTTP/parser失败场景覆盖不支持的finish reason、读取不完整、缺DONE、帧界限、上游stream错误、未知JSON与下游flush失败，并贯通真实Recorder/共享clock→Snapshot→两出口。检查失败Tape仍不完整、游标不推进、错误私有正文和凭据不进入投影；未知任意文本有独立负例。代码仅增加固定原因值，不改变时序、Strict、Workload或工具契约。

最终4文件组合的 `make build`、`make test`、`make test-race`、`make lint`、`make integration` 首轮全部通过，耗时分别13.366/29.145/77.586/3.325/315.542秒，合计441.176秒；658份源码已冻结。状态 `PASS_RELEASE_13_FROZEN`；Docker容器/网络恢复原清单，基底/fixture/候选源码均保持，临时缓存与步骤进程组清理完成，部署锁释放。0付费模型或云Sandbox调用。此前2文件准备材料仅为未执行的准备版本，已独立保留；不能把它与最终组合混用。

- evidence SHA `13c28e90f0b5b1eda5737c49084142b30f0febfdce2d2f45250acab996c33987`。
- source-manifest SHA `d54b8a9380fa81ebc54d0331238e5872d773fc500c1f15286af40a1cac0075d7`。
- freeze-manifest SHA `bed805677e917fca806f0786164ed9af3b14740a57e84493b86f049e9b4611eb`。

release13控制器与R01独立native镜像为两个分别固定的产物；后续driver须同时记录两者，不将整个当前工作区等同该release。工程门禁不增加Agent或Strict成功数，新真实运行见[DB/C++专项归档](../remaining-strict-cases-20261026.md)。
