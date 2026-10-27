# JiuwenSwarm Flash 接入：首轮实施与验证

更新至2026-10-27。**已完成的四组云与观测验收见 [release23 实际结果](../jiuwen-release23-cloud-validation-20261027.md)：双云 A/B 原评分各 12/12，15 个命令进程、105 条生命周期事件，以及四组各自快照的 Perfetto/Jaeger/Prometheus 查询均有证据。** 这是 image7、2 CPU / 4096 MB 的编码任务验收，不能替代 PPT 结果；全局 coverage 仍 unknown。

最新 release27 已完成 ECS 五门禁，image8、4 vCPU / 8192 MiB 的 PPT 复测正在推进，状态以 [PPT 修复与逐次验收](../jiuwen-pptx-capacity-20261027.md)为准。下文保留早期接入及失败记录，不回填新版本结果，也不宣称生产化全部完成。

## 历史 release18/19 云样本

**A 已复用官方 SandboxRegistry/SysOperation provider 完成接线，release19五项ECS门禁与实际原生受控夹具通过；E2B/Huawei A record1均完成真实DeepSeek解题、原Verifier12/12、5轮完整Tape及回收确认。** 这是原生Agent的Sandbox适配，不重写Flash或原生工具。两份不同轨迹仅作功能验收，Bridge ToolCall身份仍unavailable；八阶段漏列Bridge注册的真实缺项已单列，见[A云验收与时间对账](../jiuwen-external-cloud-validation-20261027.md)。

B 的release18 E2B record7与华为record2均通过原12/12、score1，完整Tape分别9/7轮；每轮Stop/revoke、两个Sandbox独立删除及cleanup=[]均确认。sampled SDK started/ended/exported分别44/44/44与35/35/35，missing/unexpected均0，专项integrity/coverage complete、global coverage unknown。两份真实模型轨迹不同，仅作双云功能验收，不是同Tape性能对照；华为评分出网仍为显式假设。Strict replay3与record1–5旧失败保留，固定提示变体暂停，历史六例Strict仍3/6。完整B结果见[云验收与八阶段归档](../jiuwen-cloud-validation-20261027.md#release18-native-audit-cloud)，不移作A或全局观测覆盖的证明。

### 历史验证范围摘要

| 验证范围 | 结果与边界 |
| --- | --- |
| 原生Flash＋真实工具＋停止隔离 | Linux宿主通过；受控模型fixture，不冒充DeepSeek解题 |
| 历史release19工程组合 | 827文件，基于release18增加A接线与生命周期；build / test / test-race / lint / integration全部通过。旧release15–18原件保留，当前工作树不自动等于冻结源码 |
| A 原生受控夹具 | 实际Docker Agent→官方aries provider→独立host worker，两次受控模型请求、真实文件与后台进程、Stop后空组及清理通过；无云Sandbox、无原题评分 |
| A 真实云验收 | 两云external-record1均原12/12、score1、5轮完整Tape；每轮4次真实Bridge调用、Stop/revoke与各2 Sandbox回收确认。不同Tape不作性能排名；华为Verifier出网仍assumed |
| 首轮云启动 | record1配置上限不一致，未分配资源；record2镜像复制权限失败，Agent未进入Runtime，两轮失败均保留 |
| 镜像权限修复 | 旧image3同一真实bootstrap复现FAIL；image4实际UID60000完整初始化与health通过，保持源码root-owned、Agent不可写 |
| 两云image4模板 | 同为2 CPU / 4096 MiB；真实工作区复制、配置重写、skills/memory/projects写入与cgroup隔离通过，两个探测Sandbox均删除 |
| 正式原生解题 | release18 E2B record7、华为record2均原12/12、score1，完整Tape分别9/7轮；每轮两个Sandbox独立删除确认。不同真实模型轨迹，仅作双云功能验收；旧release17成功及record1–5失败保留 |
| 原生Strict | E2B replay3 首请求 turn0 `REPLAY_DIVERGED`，0模型帧/0 Agent工具，原1/12；未修改matcher。校验模式讨论延后，[固定提示上下文变体](../design/jiuwen-reproducible-prompt-context.md)暂停；历史六例Strict3/6不变 |
| 原生Span采集与出口 | 新两云44/35 Span、8/6 authoritative Tool，四出口投影通过；本轮未重跑TP/Jaeger/Prom后端。record5的11 Span后端对账及首轮Metrics失败保持独立 |
| 资源 | release17控制器、provider整台Sandbox与Agent cgroup四边界分别保存，含按设备块I/O；生命周期内存高水位不是Runtime峰值，网络及逐Tool归因仍unsupported。record5的旧provider数值仅属于其原窗口 |
| producer审计 | release18实际两云sampled SDK started/ended/exported为44/44/44与35/35/35，缺失/额外0；专项complete、global unknown。前置Go/Python定向测试保留原范围，不证明所有native操作或flush有界 |

<a id="jiuwen-release19-a"></a>

### release19：A 原生接入与受控验收

A 在ECS Docker运行原生AgentServer/Flash，通过官方 `SandboxRegistry.register_provider` 注册 `aries` FS/Shell，连接Runner持有的精确任务Sandbox。原生Read/Write/Edit/Glob/Grep/Bash和模型循环保留；E2B/Huawei适配器、独立Verifier及四角色生命周期继续复用。`placement=external`须提供固定digest的 `harness.jiuwen_image`；B继续 `placement=embedded`。这与原生 `sandbox.startup_mode=external`（关闭内置JiuwenBox自动启动）是两个配置层次，详见[源码审查](../design/jiuwenswarm-flash-review-20261026.md#为什么当前只有-b)。

release19冻结827个源文件，build / test / test-race / lint / integration分别耗时13.319 / 32.090 / 82.545 / 3.540 / 333.583秒，均PASS。源码、基底和fixture核对未变，Docker库存恢复；门禁自身没有云或付费模型调用。ECS证据 `/opt/aries-artifacts/release19/evidence.json` SHA `b9ca254d5d105be9c41c027a8a3ff170a90d8fa72552db62d074b0a2f73a433a`；源码清单SHA `304354a29d3705136678a85211cca2514e8f34f240356823e008229eb1bc6441`；冻结回执SHA `f31349d9a0bf9fe9285f8ada7d2f9a1bfea06de847428ae1000159a491bf0a3c`。

`/opt/aries-artifacts/jiuwen-external-native-3/evidence.json`为 `PASS_NATIVE_EXTERNAL_FIXTURE`，SHA `3bdeee9b2d732b37aa59bcc5e12b0bf95acd43474537feacc17e1f76df3f798b`。真实原生Docker Agent完成两次受控模型请求；工具在独立host worker写入文件，Agent容器同路径文件不存在；后台真实进程在Stop前存活、Stop后group为空，Agent容器与worker监听消失、cleanup=[]、Docker库存恢复。集成夹具耗时41.899秒；它不含云Sandbox、真实DeepSeek解题或原题评分。前两轮数字UID:GID与私有下载临时文件缺陷均已修复，失败原件保留在native-1/native-2。

该夹具导出了原生Span、producer及Bridge记录，但工具总体观测仍标记 `unsupported`，没有宣称完整ToolCall或guest进程计时覆盖。当时原生ToolCall ID贯通、FlashGlob本地 `.gitignore`、空闲后台interrupt跨进程语义尚开放，后续修复与验收见[release21 工程记录](../jiuwen-native-runtime-validation-20261027.md)。随后两云A record1均通过真实解题和独立原12/12，SDK审计分别27/27/27、25/25/25；每轮4次Bridge调用的原生ToolCall身份均unavailable，观测全覆盖仍unknown。各2 Sandbox与外置容器/网络均确认回收。R04的Bridge注册计时未入八阶段字段已按原Trace单列，见[A完整归档](../jiuwen-external-cloud-validation-20261027.md)。

### 云失败、修复与回收

record1的profile为900秒，而实验driver未传入同值的`ARIES_SANDBOX_TIMEOUT_SECONDS`，仍使用600秒配置上限，配置加载即拒绝；没有创建Sandbox或调用模型。原driver因期待不存在的状态文件而附带报清理失败，原件保留，补充诊断明确`NOT_ALLOCATED`。这是本方实验接线问题，不是云厂商配额或任务失败率样本。

record2确实创建了任务Sandbox和独立评分Sandbox。镜像的`chmod -R a+rX,a-w`使原生模板目录为0555、文件0444；`prepare_workspace`复制后保留模式，UID60000不能创建`AGENT.md`或重写`.env`。Runtime未触达、模型请求0次、Tape不完整；原Verifier **1/12通过、score=0**，没有跳过评分。停止撤权、两Sandbox删除及独立NotFound均确认。原始清单在`/opt/aries-artifacts/jiuwen-e2b-record-2/raw-artifact-manifest.json`，SHA `877c74494af165fbc4d3364b2a3cc28f80c7baacb0d21ae7e9763599792c49d0`。

image4在原安装层保留owner写权限，非owner仍不可写；固定原生源码与依赖不变。先用旧镜像复现，再验证新镜像完整bootstrap，并在两个新云模板中复测实际工作区初始化。固定digest为`sha256:f0a82b0e258a0e763defa922c7a01410a40698b92967d03d7279e7976fdf6489`；[模板锁文件](../../deploy/jiuwenswarm/templates.lock.json)包含两云ID、2 CPU / 4096 MiB及逐项证据。小归档`/opt/aries-artifacts/jiuwen-image4-release-1/development-archive.tar.gz`约500KB，SHA `db0e2060cb63b459d351afd73b87466322160a9cccc7644b8abaec5be9afadaa`；不重复保存2GB镜像包。

record3启动49.914秒、Runtime3.443秒。真实Flash初始历史为`system,user,user`，原录制入口仅接受一个末尾user，首请求在本地拒绝，未进入DeepSeek；原生最后空结果报告协议succeeded，但模型会话与独立评分失败，整体保留FAIL。两组受控双轮请求中，尾部临时user附件内容相同、移到新增assistant/tool之后；据此制作的fixed-tail候选通过离线因果校验，但未覆盖真实模型运行中的附件更新。

record4启动46.528秒、Runtime8.433秒，首轮真实生成43帧。第二轮唯一变化是临时附件`runtime.setting.mode`从`agent`变为原生中文显示名“智能体模式”；历史前缀、assistant正文/reasoning、ToolCall ID/参数及结果关联均相同。固定源码的runtime state异步写入和每轮attachment渲染允许这种更新，因而固定尾内容是本方录制适配的错误假设。新request-tail方言按已核验的请求结构支持每轮临时附件，完整历史与工具因果仍校验、每轮完整请求仍入Tape，Strict仍逐字段比较。该修复与待审阅的固定提示时间变体不同，不改原生提示或时钟。旧候选、Tape及失败保留；逐轮八阶段、原评分和清理见[云验收性能表](../jiuwen-cloud-validation-20261027.md)。

record5使用正式release16，启动49.184秒、Runtime8.136秒、Attempt71.058秒（不含排队）。首轮61帧后第二请求仍本地409，真实SSE与日志离线复现`tool call causality mismatch`：原生去掉模型工具参数顶层`call_goal`，命令、description、工具ID/name、结果关联、历史前缀/临时附件、正文/reasoning均相同。它与record4的附件变化是两个缺陷。独立原评分1/12、score0，Tape一轮不完整；Harness协议完成不代表解题成功。SIGTERM clean exit、Stop/revoke、两Sandbox独立NotFound及清理均确认。诊断`native-contract.json` SHA `c82efdd1ea2c237f3830fdafa8d327b9d4119c16826f5b3b73815ef93166e6ba`；[完整原件及性能](../jiuwen-cloud-validation-20261027.md)保留失败。

后续独立`call-goal-v2`候选已通过ECS定向unit/race/vet与真实record5原件离线因果检查，证据SHA `5629ba36e06770631285062420dc45df7624152a6db054b788ad1fe8622b6f46`。它仅在record/load因果校验中复现固定native移除顶层`call_goal`/`callGoal`的行为，原始SSE、完整请求、Tape与Strict不变。随后release17完整五门禁已通过，新真实云结果已在顶部链接归档；不计为record5修复复测通过，也不覆盖尚未触达的native历史变化。[精确范围](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release16-record5)。

### 历史官方Span真实采集验收

release18云样本为E2B record7与华为record2：分别44/35个官方Span，8/6个authoritative Tool Span；sampled SDK独立审计与实际导出ID/终态集合完全对账，专项complete，global coverage仍unknown。此前release17 E2B record6与华为record1的35/43 Span没有audit，producer分母/flush未知状态不回填。历史record5的11 Span已完成TP/Jaeger与Prometheus独立补验，首轮Metrics失败保留；这些后端结果不移作新样本证据。所有原生Span仍使用未跨机对齐的Unix ns。

新producer审计通过原生extension在fresh进程注册，只保存sampled SDK Span计数/ID/终态及状态。Stop后与实际导出集合对账；即使官方FileExporter吞下`OSError`并返回SUCCESS，缺失导出仍须失败。Go测试覆盖整对start/end缺失、重复、未结束、错误终态及来源篡改；Snapshot仅新增`jiuwen_sampled_sdk_span_exports`范围，整体coverage仍unknown。定向证据和实际bootstrap测试边界见[前置验收](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-producer-audit-candidate)；后续[release18冻结与两云验收](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release18)单列，不回填旧运行。

原生AgentObservabilityRail/file exporter直接复用；L1/L2启用，L0关闭详细Span并保留原轻量SSE。Stop向准确PID及启动身份发送有界SIGTERM，让原生执行自身flush/shutdown，然后仍执行cgroup kill、确认空组和join；优雅退出不是隔离凭证。私有证据在runtime删除之前归档。

`/opt/aries-artifacts/jiuwen-native-observability-2/host-l1-1/evidence.json` SHA `cd092a4431193f5a5c839902678ae07e24c06e833f7b89deb406590297578dc8`：真实Flash两轮受控模型、真实Bash和后台进程；10个官方Span精确匹配本轮身份，1个authoritative ToolCall、1个complete root，invalid=0。原JSONL为152410字节，SHA `5842a41c743ecc32d1ba54013693edc131bb417e11bf604f9e9dcf33e1c66b41`。这证明该归档范围，producer零丢失仍unknown，原时间为未对齐Unix ns。

同目录`host-l0-1/evidence.json` SHA `e7a3cc07eb78923b7e460bb5b8f842424cb0495d392155c8fa0aa3193c0c766d`：真实模型/工具路径仍执行，删除前确认原生Trace文件数0，索引明确not_enabled。两轮均正常关闭、cgroup为空、后台PID消失、端口及私有目录清理。首轮fixture祖先目录权限失败保留，随后修为可穿越但证据仍私有；bootstrap首次native import前进入私有HOME，避免原生日志污染题目目录。

同一冻结快照`snapshot-da4623dadf72ee677341aec4a96d77863ad792bd06a5bccd7ca31868a3c43994`已完成两出口真实后端验收：Perfetto TraceProcessor导入10原生Span、9父flow，误差0ns；Jaeger2.22读回21Span（11控制端、10原生）、19父关系，263项身份/时间/ToolCall/状态检查通过，无猜测的跨域parent。证据`/opt/aries-artifacts/jiuwen-native-backends-2/evidence.json` SHA `c1d036ca545015dccb898a822387fafbf2726c1576805bad0a47f2eba84c5af6`；临时后端已清理。单Runner fixture的绑定来自实际task/session/Timing，没有app调度分母，未生成任务成功Metrics；不能替代云端解题或完整采集覆盖。首次验收脚本属性名称错误的FAIL保留，第二轮未重跑Agent或修改原始Span。

## 首轮已完成的协议验证

- 新增 `pkg/harness/jiuwenswarm/client.go`：原生健康检查、项目会话创建、HTTP/SSE运行和interrupt请求。保持普通 `mode=agent`，由已确认的Flash YAML选择实现。
- 一次提交不自动重试；不跟随重定向；检查request/session身份；错误、HITL、断流、无成功终态、截断帧均不能计为成功。原始响应只交给私有证据消费者，计时事件不复制模型和工具正文。
- 接入既有Recorder单调时钟：请求发起、响应头、首个完整SSE帧、成功终态接收、流消费结束。整个 `jiuwen.chat` 区间是控制端HTTP/SSE消费，不是guest纯执行时间。
- ECS上 `go test -race` 通过19项（含子测试），覆盖原生envelope解包、并发会话、取消、错误与身份偏离、SSE分帧和计时信息隔离；`go vet`通过，格式化也在ECS完成。
- 使用真实固定版本AgentServer和原生Flash执行两轮受控模型交互；真实bash工具在原生绑定工作区写入文件，由测试进程独立核对内容。收到8帧、1个工具调用与1个同ID结果，工具result成功。

这里的模型是明确的协议fixture，工具实际执行在ECS本地目录。它证明原生Flash、协议和实际工具互通，不是云Sandbox性能测试、DeepSeek自主解题或Strict Replay。文件核对也不冒充原题私有Verifier。

## 后续已落地的工程边界

- `embedded.go`、`bootstrap.py`：原生Flash的私有配置、项目、Session与一次任务调用；模型配置走规范 `.env`。B工具保持原生LOCAL执行、后台和并行能力，Agent与工具同处目标Sandbox。
- `harness/internal/sandboxprocess`：从OpenClaw提取既有UID60000、no_new_privs、root准入锁及cgroup停止机制；OpenClaw和Jiuwen各自保留原生协议。prepare提交失败仍持有group，Stop失败可重试，必须收到`ARIES_GROUP_EMPTY`并join调用。
- `evidence.go`：工具、最终回答、暂停及终态原帧私有保存；20,000条非工具更新不占工具预算。选择数、实际保留数、日志与回答截断分别报告。观测失败不改原Agent结果；早期样本未采集guest进程计时，状态保留`unsupported`，后续进程计时见顶部release23验收。
- 独立`agent_type=jiuwenswarm`已支持`placement=embedded/external`，当前开放codingcase；E2B和华为沿用既有adapter。A在release19通过官方provider扩展接入；TerminalBench、Workload Replay及OpenClaw专用选项仍不开放。配置通过与真实云验收分开。
- 共享模型入口改用实际Harness身份。Jiuwen Tape固定`jiuwenswarm:<源码SHA>:<lock SHA>`；OpenClaw旧值不变。受控record→Strict成功，外来Harness/source/lock均在分配云资源前拒绝。它是模型协议测试，不是Flash真实云Strict验收。

| 工程验证 | ECS证据 / 状态 |
| --- | --- |
| 共享containment及OpenClaw回归 | `/opt/aries-artifacts/jiuwen-containment-1/evidence.json`；unit/race、编译PASS；SHA `11ac964910133115d9f09622687680968be768a9bdd9a668c4d46151aa01ebd8`。内核行为的单元夹具不冒充真实隔离 |
| 新Harness行为、观察失败分离、最终回答与截断 | `/opt/aries-artifacts/jiuwen-embedded-1/unit-evidence-2.json`；unit/race、vet PASS；真实权限修复前的工程组合，保持原件 |
| config/app/wiring组合 | `/opt/aries-artifacts/jiuwen-wiring-1/phase4.json`；race、vet、CLI build PASS；SHA `f62109b23afdfc31dc1a33a6adc90cabb7e32b374a42c9a9b6630fae83169e21` |
| 完整release15发布组合 | `/opt/aries-artifacts/release15/evidence.json`；build/test/test-race/lint/integration五项PASS；SHA `3cb18ef381da31a5c67673967ce9c2c91bc1075207239aed4af26f408611c12a` |
| 历史release16发布组合 | `/opt/aries-artifacts/release16/evidence.json`；五项ECS门禁PASS；SHA `56c233278e82bdea5f39d7726badd8b8b88ae82e74c58f3185a21ad4a5de9dff`；[逐项耗时与冻结SHA](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release16-record5) |
| 历史release17发布组合 | `/opt/aries-artifacts/release17/evidence.json`；五项ECS门禁PASS；SHA `9f10f07ba410da1eae655cb89ae2e9e8b9e0b066758a1e82ea94fab9dedc3eef`；[805文件及冻结SHA](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release17) |
| 历史release18发布组合 | `/opt/aries-artifacts/release18/evidence.json`；五项ECS门禁PASS；SHA `6c49efbd53443206bd84d2bca11d95f2b155a4594453b3d0a296b5863d14d990`；[812文件及冻结SHA](../observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release18) |
| 历史release19发布组合 | `/opt/aries-artifacts/release19/evidence.json`；五项ECS门禁PASS；[827文件、冻结SHA与A受控验收](#jiuwen-release19-a) |

release16冻结798文件，source manifest SHA `149035e8e20b08d2cae8bad4bdb55e4979396cbbc609be7d23a0f3fea7b670dc`，freeze manifest SHA `267c5bec954df686c788d130ced891e53334671094e09e0c515f9505c6d393ce`。五门禁分别12.972/30.693/80.383/3.526/335.217秒；基底、源码、fixture及清理库存核验通过，门禁自身0云/付费模型调用，不替代之后record5的真实失败。新候选或当前工作树不自动属于这个冻结版本。

历史release15冻结784个源文件，清单SHA `3f4cc9713b0bc814891a51c1d3f345972c7df486035d45b6df5e749b7463cf9a`，含下面两项实际启动修复。五门禁分别12.06/30.25/79.15/3.48/321.77秒；集成使用真实容器，原有OpenClaw路径回归通过。Runner SHA `e5b4e608408a3270a10c936088ce05ed29e33144a8738d526c84034c9d922f8f`；分析器SHA `a74ec944df4b6ba376b70a34da8465f81dcb8dc05f39d691e66dc0d5bf1e717e`。release14原件未改，测试容器、网络和进程清理确认。这不是云端验收。

首轮真实宿主containment测试发现了**本方bootstrap权限缺陷**：systemd `UMask=0077`使声明的0755/0555变为0700/0500，非特权Agent无法读启动脚本。失败原件在`jiuwen-embedded-real-1`，已确认本轮进程、cgroup、runtime目录和监听清理。修复为显式`fchmod`/`chmod`，私有HOME700和spec400不变；修复后真实复测独立归档，不用放宽测试umask来通过。

第二轮随后暴露原生日志默认写入相对`./logs`，进程工作目录却是只读安装目录。将进程工作目录改为本轮私有HOME；原生项目和工具工作区仍绑定任务目录。第三轮实际通过，前两轮FAIL均保留；最终bootstrap SHA为`31c757f856a4a42ef85e38873d988ba25c8aba2ac67572c70cfe0920c342c694`。

### 真实宿主隔离验收与固定镜像

`/opt/aries-artifacts/jiuwen-embedded-real-1/attempt-3/evidence.json`：`PASS_NATIVE_EMBEDDED_HOST_CONTAINMENT`，SHA `c86cc61979356a487efe7fc8ba81abfa640ed9f371ae44d51a994ba32f9e8b3e`。执行前将4480个原生源文件逐SHA与固定归档核对，其中2435个Python文件；不是只检查锁文件或自报版本。

- 实际日志确认Flash adapter与关闭skill selection；受控模型两轮交互，真实bash写文件并启动后台进程，第二轮返回相同ToolCall ID的结果。
- 返回最终回答前，后台PID实际存活，UID为60000且属于本轮cgroup。Harness Stop、Bridge撤权后，独立probe检查文件内容与后台PID消失；cgroup、私有目录、18865/18866/18877监听及临时账户均清理。
- Runner记录18.520秒，其中启动17.172秒、Runtime 1.187秒、停止撤权0.118秒。它是单次宿主开发fixture；创建/准备由本地夹具提供，不是云创建延迟，独立probe也不是原题Verifier。不能据此做云性能排名。

历史image3的registry manifest digest为`sha256:46e170b16c2f1b227e23dbea483bc42f77884142d8c8fe1bfeea78b0e4bee42c`，发布回执`/opt/aries-artifacts/jiuwen-image-1/publication-2/evidence.json` SHA `6219a8994295b091154bb9829417c835c52fe12bde86d452fb4ebde8bfefdc41`。原导入和权限探测通过，但未覆盖原生复制后的文件模式，故不能作为完整初始化通过证据。当时由上文image4替代；旧原件及失败均保留。两版镜像均不含任务或私有Verifier。

## 实测发现并解决的两个启动问题

| 轮次 | 真实结果 / 根因 | 处置 |
| --- | --- | --- |
| native-protocol-1 | FAIL；Agent初始化到Flash，但报告模型未配置，模型请求0次。原生入口在bootstrap后再次加载规范配置目录 `.env`，覆盖了单独 `--dotenv` 的模型值 | 模型配置写入 `JIUWENSWARM_CONFIG_DIR/.env`，启动入口也指向同一文件 |
| native-protocol-2 | FAIL；两次模型交互、Agent成功终态，但工具返回JiuwenBox 409，预期文件不存在。Linux启动器在startup_mode=internal时重新启动Box并回写enabled=true | B部署显式设置 `sandbox.enabled=false` 与 `sandbox.startup_mode=external`，保持工具在Agent所在的目标Sandbox内执行 |
| native-protocol-3 | PASS；两轮模型交互、原生bash实际写入、文件字节检查通过，退出和监听清理确认 | 冻结证据，进入B生命周期集成 |

原失败和原始事件均保留。没有放宽文件检查、过滤ToolResult或把终态成功替代实际工作验证。Linux这一行为来自 `agent_ws_server.py` 的实际代码；部分上游生命周期文档仍描述旧的enabled门控，不能据旧文档覆盖实测。

## 固定版本与证据

ECS目录：`/opt/aries-artifacts/jiuwen-integration-1`。

| 项目 | 固定证据 |
| --- | --- |
| JiuwenSwarm | `e2c9d519498080e249e34f16a80c8c97f0cd1962` |
| agent-core | `3460150aa5f319b0b8f84a4ed10eec0a08b0feb1` |
| uv.lock SHA-256 | `f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9` |
| 首次真实入口失败回执 | `09c16ca65db4b7ed5bbbca737ad4737708098e3d1b717675ed6c4c4ca8a2c88e` |
| 嵌套Box失败回执 | `31cd0542598bfb2c919b41456378aff54b00db1543013b8abf241f7e910db122` |
| 第三轮成功回执 `native-validation-3.json` | `2444872ff4453dffed6435bbee8191dda84501237607a318a9a36905ead4b4f2` |
| 单元/race JSON日志 | `ccfed858ad82aa3f8f3cc0b25d2f73e971ff02a45e886cd168b35fc5915e70cd` |
| 真实原生互通测试日志 | `51dcfccfd7983a9ed769f2c47c6f655a803cacf91d62d638acecf973b1f27992` |
| 第三轮原生8帧JSONL | `95b59ddb564de581965151acdda6e1d70980c1a19df3aa7290b5c6f309c4304c` |

远程通过 `uv 0.9.5 sync --frozen --no-default-groups` 安装，锁文件与本地审查原件SHA一致。首次GitCode下载中断，改由本地已审查的同提交Git bundle提供传输镜像；没有修改依赖版本或锁文件。业务编译与运行只在ECS。未创建云Sandbox，未调用付费模型。

## 早期native-protocol-3与宿主样本的观测范围

第三轮控制端记录进入同一版本化Snapshot：`snapshot-3bfffa1d8af97d5767ed310c9365853f9b4b816fffa1b88ecfb9097c4cd58280`，含1个HTTP/SSE消费区间、5个观测点，时长969151671 ns。旧OTLP只导出区间，原件保留；新增`jiuwen-chat-lifecycle-v1`策略按原父operation ID、同clock和task/attempt身份导出五个Span events，不推测近邻关系。除events外，新旧OTLP请求内容完全一致。

| 观测点 | 本样本相对`jiuwen.chat`起点（ms） | 含义 |
| --- | ---: | --- |
| `request_started` | 0.044 | 即将发起原生Agent请求 |
| `response_headers` | 234.601 | HTTP响应头已到控制端 |
| `first_sse` | 856.171 | 首个完整SSE记录已被读取；不是LLM首token |
| `terminal_received` | 968.843 | 原生成功终态已接收；不证明工具进程停止 |
| `stream_drained` | 969.142 | HTTP流已消费完 |

这些是同一条时间轴上的位置，不能相加为总时长。总区间969.152ms包含它们；相邻位置之差只是对应控制端消费段，不解释成guest执行或模型等待。非白名单、无唯一父身份或无可信时钟锚的点分别计入排除清单，不填伪时间。

新出口已对这份**真实原生运行快照**做实际Jaeger接收与查询：1个Span、5个events、21项检查全通过，v3时间与属性精确对账；临时容器和两个监听均清理。证据`/opt/aries-artifacts/jiuwen-otlp-jaeger-1/receipt.json`，查询回执SHA `819b9eddc56c5fdfbfb6f7f478e92f8407c95f197bc12da414a082f99d46d680`。这证明离线接收查询，未做本轮UI、持久化或实时容量验收。

| 产物 | SHA-256 |
| --- | --- |
| `native-analysis-3/snapshot.json` | `dcbaea890a53dd70286388699280247d3faed942f38489b0544ba235ef1ed5dc` |
| `native-analysis-3/perfetto.json` | `7364f441cf83de6ade832c8253ccdcb199509d30eab90c42dd4a077a74d2e93d` |
| `native-analysis-3/otlp.json` | `4c1e76293661fc8a697fddbb90977b8f911e824f59d0a519133d23f84ed6cc15` |

补充对**宿主containment第三轮**的真实Perfetto TraceProcessor导入：11个控制端区间的身份、起点、终点与时长全部和同一快照精确一致，最大误差0ns。证据`/opt/aries-artifacts/jiuwen-host-perfetto-1/evidence.json`，快照`snapshot-f4f7cf2794794c1f4a71482c1c42f7a7139265644442d772daf150c64cd0dcf4`；原始快照与Trace未修改。这是实际解析器验收，尚未做本轮浏览器UI验收；与上面的native-protocol-3样本分开。

这些早期控制端样本不是完整Agent Trace：native工具调用/结果的同ID存在于原始SSE，但当时实际进程身份与终态、模型分段、CPU/Mem/IO/Network和原生OTel导入尚未贯通。后续官方Span与云资源进展见本文顶部；早期`integrity=complete / coverage=unknown`仅描述其已声明的控制端记录，不回填新数据或将来源覆盖未知写成全链通过。

原生服务与fixture均通过受控systemd单元停止；第三轮确认MainPID=0、inactive，8321/18865/18866/18867均无监听。它是本轮开发进程清理证据，不替代未来评分前的非特权cgroup隔离验收。

## 持续推进的下一阶段

1. 完成 release27 的双云 A/B PPT 真实复测，分别归档原文件、独立评分、主/子会话完整 Tape、观测和清理结果；不借用编码任务的通过结论。
2. Bridge准备阶段、原生ToolCall关联、命令进程计时和四组离线后端已有release21/23证据；新任务继续核对实际来源。全局覆盖、跨机对时、逐Tool资源及观测扰动仍按未验范围保留。
3. 固定提示变体暂停，校验模式留待专题讨论，未扩展Workload Replay或放宽Strict；不同真实Tape不作同工作量性能比较。

当前没有宣布P0或生产就绪。既有R01–R18状态继续以[问题台账](../observability/OPEN_ISSUES_AND_RESOLUTIONS.md)为准；上述协议结果不关闭尚未验收的生命周期、身份、资源或跨云项目。
