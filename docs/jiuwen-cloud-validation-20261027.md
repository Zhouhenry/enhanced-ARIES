# Jiuwen Flash B 真实云验收与性能（2026-10-27）

另见 [release19 的 A 模式两云真实验收](jiuwen-external-cloud-validation-20261027.md)：Agent 在 ECS、工具在云 Sandbox，两云均原评分 12/12。本页继续保留 B 模式的独立运行和原始统计口径。

**release18的E2B record7与华为record2均完成原生解题：原Verifier各12/12、score1，完整Tape分别9/7轮。每轮Stop/revoke及两个Sandbox独立删除均确认。** 新producer审计分别对齐44/44/44和35/35/35个sampled SDK Span，缺失/额外均0，专项integrity/coverage complete；global coverage仍unknown。两份不同真实模型轨迹不能作同Tape性能比较。release17原成功、Strict replay3首请求FAIL及record1–5历史失败保留；record5后端对账不移用到新运行。

本批配置为 native Flash B、`merge-ranges-portable-v1` 原任务和原独立 12 项 Verifier，未替换答案、工具结果或评分。record-1/2/3 固定 release15，record-4 使用下述独立定向候选，record-5 使用正式 release16，record-6、华为record1与 replay-3 使用正式 release17，新E2B record7及华为record2使用正式release18；record-2 起的这些运行各创建任务与评分 Sandbox 各一个。模型请求经过 Aries proxy；没有绕过 proxy 直连上游或使用 Workload 兜底。

按最新任务顺序，继续openJiuwen接入；校验模式留待专题讨论，Strict与Workload契约均不变，固定提示变体暂停。B模式两云原生解题和sampled SDK导出专项已有真实证据，不宣称全局覆盖、实时后端或生产化完成。

## 各轮结果与失败首因

| E2B 运行 | 整体 / Harness | 原 Verifier | 模型与 Tape | 首因 | Stop / 清理 |
| --- | --- | --- | --- | --- | --- |
| record-1 | 原 driver `FAIL_CLEANUP`；Harness 未触达 | null，未运行 | null，模型预检与生成均未触达；无 Tape | profile TTL 900s 超过未显式设置环境变量时的 600s 上限，Runner 在创建运行目录前拒绝 | 0 Sandbox；无对象可做 provider absence；临时 Adapter、8082、环境副本已清理，锁可获取 |
| record-2 | FAIL / failed | 1/12；11 项失败；score 0 | 未进入模型生成；Tape 0 轮、incomplete | 旧镜像的只读模板权限由 native copytree/copy2 保留到私有工作区，创建 `AGENT.md` 被拒绝 | harness_stopped / bridge_revoked 均 true；2 Sandbox 独立 NotFound；cleanup=[] |
| record-3 | FAIL / succeeded（仅原生协议完成） | 1/12；11 项失败；score 0 | 首请求本地拒绝 `UNSUPPORTED` / HTTP 409；随后重试 `SESSION_FAILED` / 409；0 上游生成，Tape 0 轮、incomplete | native 初始角色为 `[system,user,user]`，现有 proxy 首历史规则只接受最后一条 user，拒绝中间 user | harness_stopped / bridge_revoked 均 true；2 Sandbox 独立 NotFound；cleanup=[] |
| record-4 | FAIL / succeeded（仅原生协议完成） | 1/12；11 项失败；score 0 | 1 轮真实上游生成、43 SSE 帧；第 2 请求 `UNSUPPORTED` / 409，后续重试 `SESSION_FAILED`；Tape 1 轮、incomplete | native 请求临时附件的 `runtime.setting.mode` 从 `agent` 异步更新为显示名“智能体模式”，旧候选固定尾附件规则拒绝；核心历史、reasoning、工具 ID/参数/结果关联一致 | harness_stopped / bridge_revoked 均 true；2 Sandbox 独立 NotFound；cleanup=[] |
| record-5 | FAIL / succeeded（仅原生协议完成） | 1/12；11 项失败；score 0 | 1 轮真实上游生成、61 SSE 帧；第 2 请求 `UNSUPPORTED` / 409，后续重试 `SESSION_FAILED`；Tape 1 轮、incomplete | 真实 SSE 与日志离线复现 `tool call causality mismatch`：原生移除工具参数顶层 `call_goal`；命令、description、ID/name、结果关联、前缀/附件、正文/reasoning 均一致。与 record-4 的模式更新不同 | harness_stopped / bridge_revoked 均 true；SIGTERM clean exit；2 Sandbox 独立 NotFound；cleanup=[] |
| record-6 | 功能 PASS / succeeded | 12/12；score 1 | 模型会话 succeeded；7 轮完整 Tape；6 个 authoritative tool Span | 无业务失败；完整录制不等于 Strict 可复现 | harness_stopped / bridge_revoked 均 true；SIGTERM clean exit；2 Sandbox 独立 NotFound；cleanup=[] |
| replay-3 | FAIL / succeeded（仅原生协议完成） | 1/12；score 0 | 同 record-6 Tape、strict/recorded；首请求 turn 0 拒绝，0 模型帧、0 Agent 工具 | `REPLAY_DIVERGED`；3 条初始消息的摘要均不同，其中前两条字节数相同、第三条增加 10 bytes。本页只列结构摘要，具体正文归因另由原生日志诊断留证 | harness_stopped / bridge_revoked 均 true；SIGTERM clean exit；2 Sandbox 独立 NotFound；cleanup=[] |

record-1 的原 `FAIL_CLEANUP` 不回写：旧 driver 对不存在的 ledger 做删除确认，附加了清理查询错误。独立诊断确认 `NOT_ALLOCATED` / `NOT_APPLICABLE_ZERO_OWNED`，不能写成“已查询云端并确认无对象”。record-2 起显式传入 `ARIES_SANDBOX_TIMEOUT_SECONDS=900`，实际任务预算仍为 5m，独立 Verifier 为 1m，模型 proxy 上限为 32 轮（不是 native 迭代上限）；systemd 最大窗口 1200s，KillMode=mixed，停止清理宽限 240s。

record-2 的镜像权限修复仅进入新 image4 与新模板，旧镜像和失败保留。record-3 已通过启动，失败发生在 proxy 的 `request_history_and_trajectory_validation`，请求解析成功、尚未转发 DeepSeek，不能归因为 DeepSeek 请求不兼容。原生末帧为 `e2a.complete`、`status=succeeded`、`body.result={}`，此前 `chat.final` 内容为空；没有显式 failed 字段。Harness 协议完成不等于模型会话、任务评分或整体验收通过。

## 实际时间

八阶段沿用[性能阶段说明](performance-stages.md)。单位为秒；`null` 表示未触达或无该阶段记录，不补零。Runtime 直接读取 `runtime_execution_duration`，不把失败启动或宽 `agent.run` 窗口算作 Runtime。

| 指标 | record-1 | record-2 | record-3 | record-4 | record-5 |
| --- | ---: | ---: | ---: | ---: | ---: |
| Driver UTC 窗口 | 1.985607000 | 23.134544000 | 69.671244000 | 71.933712000 | 75.938996000 |
| Attempt 执行总时间（不含排队） | null | 18.916423752 | 65.433611485 | 68.185372781 | 71.058345994 |
| 排队 | null | 0.000021363 | 0.000028094 | 0.000016589 | 0.000021289 |
| 任务 Sandbox 创建 | null | 1.048501999 | 1.255507031 | 1.754472386 | 1.337363233 |
| 工作区准备 | null | 2.214375341 | 2.212463302 | 2.838967656 | 2.046752944 |
| Harness 启动 | null | 5.919353882 | 49.913658618 | 46.527548626 | 49.184198904 |
| Runtime 执行 | null | null | 3.442958378 | 8.433153837 | 8.135940180 |
| 停止与撤权 | null | 0.540522302 | 0.566810309 | 0.571451705 | 1.931261018 |
| 独立验证 | null | 6.256608676 | 5.354494992 | 5.382094262 | 6.243383428 |
| 清理 | null | 2.419160178 | 2.024708810 | 1.582442089 | 1.716926668 |

Driver 窗口包含临时 Adapter、Runner 启动及结果/删除确认，以 UTC 时间戳相减；Attempt 不含排队与派发延迟。八阶段不是 Driver 的无缝分割。任务 Sandbox 创建一栏只计任务环境，评分 Sandbox 的准备包含在独立验证中；Verifier 已包含原测试与报告读取，不再与 pytest 内部时间相加。上表五轮均未通过，不报告其有效任务吞吐，也不据失败耗时比较云性能。

| 指标（秒） | record-6 | Strict replay-3 |
| --- | ---: | ---: |
| Driver UTC 窗口 | 100.937809000 | 64.499775000 |
| Attempt 执行总时间（不含排队） | 96.089405351 | 60.850034316 |
| 排队 | 0.000020554 | 0.000019684 |
| 任务 Sandbox 创建 | 1.049705904 | 1.845400429 |
| 工作区准备 | 2.431876842 | 1.665273740 |
| Harness 启动 | 49.649368861 | 41.823544913 |
| Runtime 执行 | 31.705385024 | 3.321824725 |
| 停止与撤权 | 3.266068609 | 2.316140102 |
| 独立验证 | 4.646524651 | 6.778566744 |
| 清理 | 1.497100271 | 2.027288426 |

replay-3 的 Runtime 包括原生会话处理拒绝与收尾，不代表发生过模型生成或工具执行。两轮 Attempt 与排队时长均读取各自 `run-result.json` 的 `attempts[0].duration / queue_duration`；没有用 `task.e2e` 或 admission/drain 窗口替代。

## 观测与资源边界

| 对象 | 本轮实际来源与状态 | 不能推导的结论 |
| --- | --- | --- |
| ECS 控制端 | record-2/3/4/5 有 `controller-resources.start/end.json`；record-3/5 数值见下方独立核查 | 控制器 CPU/RSS 不能当作 guest Agent 用量；文件存在本身不证明采样完整 |
| 模型 proxy / 上游 | record-3 原边界有解析成功、历史匹配拒绝及 HTTP 409；record-4/5 首轮有真实上游请求与 SSE。model events 的进程计数属于控制端进程 | 不能把 proxy 进程 CPU/RSS 与控制端重复相加；record-1/2/3 没有上游生成耗时。record-4/5 SSE/请求窗口也不是模型纯推理耗时；模型目录预检不是生成调用 |
| 任务 Sandbox / native Flash | record-2/3/4/5 有 `sandbox-metrics.jsonl` 原件；record-3/5 数值见下方独立核查。record-6 另有 release17 的 cgroup 边界读数，见下文 | provider 数据属于整台 Sandbox；采样最大值不等于真实峰值，不能分给 Agent/某个工具；provider 磁盘和网络 I/O 不支持 |

record-3 的历史资源只读核查没有创建云资源、调用模型或新增样本。任务与评分 Sandbox 按原 owner/purpose 分开，不能合成一条 Agent 资源曲线。下表时间均为 **2026-10-26 UTC**；CPU 百分比保持 provider 原值，不另乘 CPU 核数或积分成 CPU 时间。

| 归属 | 观察窗口 / 实际样本 | 可报告数值 | 范围限制 |
| --- | --- | --- | --- |
| ECS 控制器共享进程 | 两次采集：16:32:22.998987255 → 16:33:28.773635117 | user CPU 增量 0.184888s、system CPU 增量 0.121733s，合计 0.306621s；末次进程生命周期 RSS 高水位 34,922,496 bytes（33.305 MiB） | 是控制器进程窗口与生命周期高水位；不是 Sandbox CPU、任务独占用量或 Runtime 内存峰值；proxy 进程计数不再另加一次 |
| 整台任务 Sandbox | 14 次 receipt，2 次空；78 条返回读数对应 12 个不同 provider timestamp / 完整读数，66 条重复、冲突 0。请求窗口 16:32:24.587216954 → 16:33:28.077436114；读数时间 16:32:25 → 16:33:20 | 原生 `cpu_used_pct` 返回最大值 51.7%；`mem_used` 返回最大值 754,388,992 bytes（719.441 MiB）；`cpu_count=2` | 窗口覆盖准备、启动和 Runtime 周边；Runtime 仅 16:33:17.355551430 → 16:33:20.798509820，不能把全窗口最大值称为 Runtime 峰值 |
| 整台评分 Sandbox | 2 次 receipt，1 次空；仅 1 条读数。请求窗口 16:33:22.643042672 → 16:33:26.725244939；唯一 provider timestamp 为 16:33:20 | 唯一读数 `cpu_used_pct=94.74%`、`mem_used=295,383,040 bytes`（281.699 MiB） | provider 时间早于本次请求窗口，保留原时间；单点不能代表完整 Verifier，更不能归因到某个 pytest 测试 |

名义采集间隔 5s 不等于 provider 更新频率；重复返回的历史读数不是独立样本。上述最大值只针对实际返回读数，不能证明采样间隙内没有更高值。原件明确 `disk_io`、`network_io`、`agent_tool_resource_split` 不支持；`disk_used` / `disk_total` 是占用与容量，不是 I/O。记录写入的 21,817 bytes / 16 次 write 属于观察器 `sandbox-metrics.jsonl` 文件，不能当作任务磁盘 I/O。embedded 模式的独立 Harness 资源 monitor 不提供，缺 `monitor/resources.jsonl` 不能解释为已知丢失了本应存在的 Agent 进程样本。

核查回执 `.cache/jiuwen-record3-resources-review-1/review.json` SHA `9a0b0578f97582012b45a070efe37a56df67c115569a57ead8247e89d781dd22`；其中逐文件核验原件未变，`sandbox-metrics.jsonl` SHA `1b3cb1e1c20495179bec6007b9d8fc5aec7d507da55aff0e2f16ef2be19af910`。record-2/4 尚未在本页进行同样的资源数值核查，不借用 record-3 数值填充。

record-5 资源复核直接读取本轮原 Snapshot 与 provider JSONL，未重新运行分析器、云端采样或模型。时间均为 **2026-10-26 UTC**，归属及最大值限制与 record-3 相同。

| 归属 | record-5 采样与窗口 | 实际数值 / 空态 |
| --- | --- | --- |
| ECS 控制器共享进程 | 两点：17:36:39.724327199 → 17:37:50.937381273 | user/system CPU 增量分别 0.243826s / 0.125452s，合计 0.369278s；末次进程生命周期 RSS 高水位 35,520,512 bytes |
| 整台任务 Sandbox | 15 receipts、2 空；93 返回读数 → 13 个唯一完整读数 / timestamp，80 重复、0 冲突。请求窗口 17:36:41.208592620 → 17:37:49.779397273；provider 时间 17:36:40 → 17:37:40 | `cpu_used_pct` 返回最大值 51.71%；`mem_used` 返回最大值 755,273,728 bytes（720.285 MiB），`cpu_count=2`。仍不代表本轮 8.136s Runtime 的峰值 |
| 整台评分 Sandbox | 2 receipts 均空；请求窗口 17:37:44.940996133 → 17:37:49.187213237 | CPU / 内存为未取得读数，不能填 0，也不能借用任务 Sandbox 数值 |

record-5 仍明确不支持 `disk_io`、`network_io` 与 Agent/工具资源拆分。原件 SHA 均复核未变；独立资源回执 `.cache/jiuwen-record5-resources-review-1/review.json` SHA `1a4b606ddd261b07784652f8411b5a7a5ea0e17ab7273acced2a054a2b5e3779`。没有把 provider 百分比积分成 CPU 时间。

record-6 的原 Snapshot 分开保存控制端、任务 provider、评分 provider、Agent cgroup 与块设备资源流。控制端 user/system CPU 增量合计 0.852729s；任务 provider 的 191 条记录包含重复读数，不能称为 191 个独立采样点。新 cgroup 在 `start / runtime_start / runtime_end / stopped` 四个边界实际读取，覆盖 Agent 进程组及后代，不是周期采样或逐工具归因。`runtime_start` 至 `runtime_end` 的累计 CPU 差为 3.553583s；`runtime_end` 的 `memory.peak` 为 630,906,880 bytes，但这是 cgroup 生命周期高水位，不能命名为 Runtime 峰值。设备 `254:0` 的块 I/O 有后续三个边界读数，初始空 `io.stat` 保留缺测；网络与单工具资源仍未支持。guest 采样的单调括号、未对齐 wall clock 与控制端 RPC 时间分别保留，边界差不能冒充精确 Runtime 资源积分。


| 运行 | Snapshot integrity / coverage | 原出口结果 | 原计数 |
| --- | --- | --- | --- |
| record-1 | null / null | 未建立运行原件 | null |
| record-2 | complete / unknown | Snapshot、Perfetto、OTLP、metrics 均 exit 0；仅投影通过 | 6 metrics / 28 points；0 个 Jiuwen chat 生命周期 event（Run 未触达） |
| record-3 | incomplete / incomplete | 四出口均 exit 1，原 stderr 与有效 JSON 保留 | 6 metrics / 29 points；5 个 Jiuwen chat 生命周期 event |
| record-4 | incomplete / incomplete | 延用 release15 分析器，四出口均 exit 1，原结果保留 | 6 metrics / 29 points；5 个 Jiuwen chat 生命周期 event |
| record-5 | complete / unknown | 正式 release16 四出口均 exit 0；仅投影通过 | 6 metrics / 29 points；5 个 Jiuwen chat 生命周期 event；另有 11 个官方原生 Span |
| record-6 | complete / unknown | 正式 release17 四出口均 exit 0；本页未将 record-5 后端结论移用至本轮 | 35 个官方原生 Span，6 个 authoritative tool Span、1 个 complete root；invalid/unassociated 均 0 |
| replay-3 | 尚未独立读取本轮 Snapshot | 四出口均 exit 0；仅投影通过 | 6 个官方原生 Span，0 个 authoritative tool Span、1 个 complete root；invalid/unassociated 均 0 |

record-3 唯一原 Snapshot 诊断是 `native tool source/index not collected`：旧 release15 将新 Harness 会话走入旧原生工具解析路径。上述只读核查使用新分析器（SHA `d10aa393aa689338ee872ebc6c5514701452c24a7f08f2296808eb0a77504641`）旁路重建，exit 0、integrity=complete、diagnostics=[]，但 coverage 仍为 incomplete；新 Snapshot SHA `905c62ffb5ab415929d38cb3449faddbe5a8e23062952a4086a9ea93dc14de93`。这份补充证据不覆盖原 `FAIL_PROJECTION` 或业务 FAIL。资源点数、5 个协议生命周期事件与工具/进程覆盖是不同口径；它们不能证明所有实际行为已被观测。

## 固定输入与证据

release15：`/opt/aries-artifacts/release15`；source manifest SHA `3f4cc9713b0bc814891a51c1d3f345972c7df486035d45b6df5e749b7463cf9a`；release evidence SHA `3cb18ef381da31a5c67673967ce9c2c91bc1075207239aed4af26f408611c12a`。原 native revision `e2c9d519498080e249e34f16a80c8c97f0cd1962`，uv.lock SHA `f28fbbd9b208bcf4f6e421818969ed2b6570f0648bed25c2ed15ee5bd551e5c9`。

record-1/2 使用旧 E2B template `bagc83pexjzacqz49ek6:a7426b0e-f4b4-4c4c-8c70-0094813e99c7`。record-3/4/5/6 与 replay-3 使用 image4 digest `sha256:f0a82b0e258a0e763defa922c7a01410a40698b92967d03d7279e7976fdf6489`、template `2f4kmdvot1ncn9q26ob9:20bd6f86-1368-41e1-8136-4d8f95d90d03`；新模板与真实 UID 工作区初始化探针已独立通过并删除探针 Sandbox，这不替代任务验收。

ECS 各轮根为 `/opt/aries-artifacts/jiuwen-e2b-record-N`。本地白名单回执位于 `.cache/jiuwen-cloud-1/recordN-receipts/`；不复制模型正文、工具命令或私有断言到报告。

| 运行 | 原 evidence SHA-256 | 性能 / 诊断摘要 SHA-256 |
| --- | --- | --- |
| record-1 | `410948ae3619fb704af8017cfa03114eeb98ec9797858ee64ae765d652486991` | `23378a77b12a19a7560d378eac0c86554b8749b28d17299907117f75fb223d58` |
| record-2 | `b68f1d9605ef0970df1c07c0dd2cba5a0b58ee0bf27a12cda4feac05a9d30a31` | `f0a38f5dac705c9927d4d7e23f622a797a015a5d996d5b94a31f7cbfe59241dc` |
| record-3 | `04d30d43355021a4d2353bfbf7fb8011767e259016888d7f5319710d18596cd0` | `c9f026a58610450b2f4ab2d60a4e5f2ff7a3ec32ae3e31d8ff243a8a7e3f3945` |
| record-4 | `38703621e395ebe0d99053e72c9fd648a0283314f6b5508cfe8025d9c1651075` | `41a0676ea0a34499eafa1b1104a32402641265df577b8c9c5f9fca9032f90ba7` |
| record-5 | `0a737add17a01f87633f6966aec12f04483072ab0a1090115bb8f1c3dcd08725` | `f686d116d36d94b7a03935beed1ea104c156233483af02cd8c97ea0896644900` |
| record-6 | `fffd48e7cf7932c7a7112d0c8fa0f4b9770617774950f3e6a5bea9aede8afbbe` | Snapshot：`b7b6d52f51b87f5fd16778c610764ad92f6bb0f25e427bab9cb2627d366cf4f6` |
| replay-3 | `aac26355793a8a368090e11da63973ce11c71951c1795f8bf7229dbd93bf04c9` | native index（evidence 引用）：`79204b7e1195ef2c32003d035b0013be5043841b37352a7ad00ece404a7a3330` |

record-3 的 `initial-history-diagnosis.json` SHA 为 `81dd0c9cb9bb2029198934f19ed93695b28b8cddfd40fff81bf459ddf210d6f0`，记录角色、字段类型、原日志行号/哈希与真实终态，不含正文。`raw-artifact-manifest.json` SHA 为 `7f0320e27df3da88457c3851969b45528418536d4f27c85aa00d93825498c0e6`（64 个原件）。record-2/3/4/5 的独立 `release-receipt.json` 均为 `PASS_WINDOW_RELEASED`：实际共享锁可获取后释放，unit MainPID=0，8082 无 listener，owned Adapter / 环境副本 / Runner 进程组无残留。

record-4 候选为 `/opt/aries-artifacts/jiuwen-initial-history-1`，状态 `PASS_JIUWEN_INITIAL_FIXED_TAIL_TARGETED_ONLY`，没有冒充新的完整五门禁发布。基于 release15，仅叠加独立 Jiuwen history dialect；原生多 user 初始消息和相同尾提示移动经过因果校验，完整每轮请求的 Strict 匹配保持不变。ECS 定向 unit/race/vet/build 与两组已保存真实原生两轮请求的离线因果/Strict 验证通过。候选 evidence SHA `72bb626b5cad24e6ce12aa7d9b0ad6923b42f0719e7a0677dedf263eb4ca4577`，manifest SHA `413215e6ac96310cbadccbc00ce644c98d8183fbb2249ef2e48c13354fe7eaea`；`aries` SHA `040a9199ff531889c92435cc3b6bf0e00dd6f22483696cb87a36465ee17bd773`，`aries-perf` 保持 release15。record-4 沿用 image4、同模板、原预算与评分；离线协议验证不计为真实云 Strict 通过。

record-4 原件根 `/opt/aries-artifacts/jiuwen-e2b-record-4`；raw manifest SHA `02740a863a77df576f6ec552dd4c6975bba87d494bc64b225e6cffcecd6ad0ca`（64 个原件）。Tape SHA `ef9a9a9649228b73528335c0d44b3fc1957222451cbe3b7105457c8623f170e0`。独立诊断 `/opt/aries-artifacts/jiuwen-request-tail-2/record4-diagnosis.json` SHA `9e47fea94656613a2e3542d5762442c4f6d82db17549674aa308e8afa7dc2de4`，确认唯一差异是上述原生临时附件更新。新录制方言按原生规则接纳每轮附件，Strict 仍逐轮比较完整请求；原 candidate、Tape 和失败评分不改。

record-5 使用 `/opt/aries-artifacts/release16`，正式五门禁与冻结均 PASS；evidence SHA `56c233278e82bdea5f39d7726badd8b8b88ae82e74c58f3185a21ad4a5de9dff`，source manifest SHA `149035e8e20b08d2cae8bad4bdb55e4979396cbbc609be7d23a0f3fea7b670dc`。本轮显式 `ARIES_OBSERVATION_LEVEL=l1`，同时包含请求方言修复、官方原生 Span 采集及共享 Snapshot 导出修复；cgroup 资源采集不在此历史版本。record-5 本身不具备完整 Tape，未用它进行 Strict 回放。

record-5 的原件根 `/opt/aries-artifacts/jiuwen-e2b-record-5`，raw manifest SHA `9f6bf6076113f8a6dda6bcaa2db90aeed33ea451dad34d53db54f43655807476`（66 个原件）；Tape SHA `1e2d71c323d213bfae2f6b3e6de1d7b0348d75a9ff1e1aad471aaa6013f69293`。原 Snapshot SHA `a529b98cdd18e225d1de158f80df504ffe0c4ab27cf861f29193b7f84770e03b`；后端首验的 Metrics 脚本失败保留，后续补验与业务结果分开。

同一真实云 Snapshot 的后端首验为 `/opt/aries-artifacts/jiuwen-cloud-backends-1/evidence.json`，SHA `ce90c348bbecae2536dc1f99f05d53825c58d53f2dcd68442a6a0f5b6ac199c2`：Trace Processor 实际核验 437 个 controller 区间/25 条 flow 及 11 个 native 区间/10 条 parent flow，0ns 误差；Jaeger 448 个 Span/5387 项对账 PASS，每个 Span 起止/时长误差 0ns。原件 SHA 不变，Jaeger 清理 PASS。首轮 Metrics 脚本 exit 1 源于继承的 `umask0077` 使临时公共 `prometheus.yml` 为 root0600，非 root Prometheus 无法读取；尚未 send/query，owned 清理通过，该 FAIL 原件保留。

独立 `jiuwen-cloud-backends-2` 仅将上述公共静态文件改为 0644，沿用四份原导出 SHA，实际 Prometheus 首发与重复发送后 6 次查询、1184 项检查均通过，数值误差 0。attempt 为 117 序列/117 样本，controller 为 8/16，任务 provider 为 7/91；评分 provider 两次空读数保持 NOT_COLLECTED，未发送、未补零。时间按 Prometheus 毫秒精度截断：attempt −63,171ns、controller −381,273 至 −327,199ns、provider 0。三自有容器与端口清理确认；未重跑 TP/Jaeger、Agent 或云任务。合并回执 `.cache/jiuwen-cloud-backends-2/combined-reconciliation.json` SHA `1a9f93f57986ebf9e87630d1096ea2da32b4f6b02dc2b0bdc8786b60b310f86b`，状态 `PASS_OFFLINE_BACKENDS_OF_FAILED_REAL_RUN`；metrics evidence SHA `5bdbc833c106a4e31949d04cd1442ab5be8baba80ad0d22441b2a870d7828311`。这是 record-5 已有数据的离线后端对账，不代表实时采集、完整覆盖或业务通过。

首因诊断位于 `/opt/aries-artifacts/jiuwen-record5-diagnosis-1/{structure.json,argument-differences.json,reproduction.json,native-contract.json}`；`native-contract.json` SHA `c82efdd1ea2c237f3830fdafa8d327b9d4119c16826f5b3b73815ef93166e6ba`，包含15个固定源码证据点。实际 `agentserver.log` SHA `85ba1131bfcc7069e9009e7d95e5846661d533b50d78396e1b947beb8e90f499`；仅公开结构与哈希，不复制请求或工具正文。未触达的动态system、compaction、path repair及synthetic tools保持unsupported，不据这次定位宣称完整native grammar已适配。

`call-goal-v2` 最初的独立定向证据为 `/opt/aries-artifacts/jiuwen-call-goal-1/evidence.json`，SHA `5629ba36e06770631285062420dc45df7624152a6db054b788ad1fe8622b6f46`，原状态 `PASS_CALL_GOAL_CAUSAL_TARGETED_ONLY` 保留。后续已与 cgroup 采集合入正式 release17，并完成五门禁及 record-6 真实录制。该适配只在 record/load 因果校验中复现 native 顶层字段提取；完整请求、原 SSE、Tape 与 Strict 比较不变。record-5 原 FAIL 不回写。

本轮 `native-spans-index.json` SHA `667c9ad3616deaeaaa56a54e199da82dbe1e013d4450cec2721e130abdd46051`，原生私有 JSONL 为 195,559 bytes，SHA `7fd7b285956360fcf42575717e37038fcd3f73c04fd1b06bb4e71fb2bc964cb1`。11 个 Span 全部精确关联本 attempt，invalid=0、unassociated=0，含 1 个 authoritative tool Span 与 1 个 complete root。它们使用未跨机对齐的 native Unix 纳秒，不能并入控制端单调时钟。SIGTERM、native clean exit 与 cgroup 隔离均确认，但 exporter flush 仍为 unconfirmed，producer 总数未知、coverage=partial；不能据这 11 个 Span 宣称完整观测或 guest 进程/退出/pipe-drain 已测量。

record-6 与 replay-3 固定 `/opt/aries-artifacts/release17`：805 个源码文件、相对 release16 的 12 文件 overlay，五项 ECS `build/test/test-race/lint/integration` 均 PASS，正式冻结状态 `PASS_RELEASE_17_FROZEN`。evidence SHA `9f10f07ba410da1eae655cb89ae2e9e8b9e0b066758a1e82ea94fab9dedc3eef`，source manifest SHA `9ee984fe07d7c68e4de68ddfefe96c153a10f3b63bcdab6adb84c288754e40a7`，freeze SHA `89c82872ce7a769580b79d9702abd2441d6a6178fe905f758b6aae5d4f7c329f`。原题、image4、预算与 Verifier 保持，显式 L1；这些门禁本身没有调用云或模型。

record-6 原件根为 `/opt/aries-artifacts/jiuwen-e2b-record-6`，完整 Tape 文件 SHA `71355f718833b48455d35784baa5ff353cf344fc79b88a74d7ff497c9d0f2319`，native index SHA `b97d10470120e30b09d8a78277d3ea5ec068684cc491b16062f8088058143cba`。replay-3 根为 `/opt/aries-artifacts/jiuwen-e2b-replay-3`，输入 Tape SHA 完全相同，model session 明确 `replay_policy=strict`、`timing=recorded`。本地 `.cache/jiuwen-e2b-record-6/` 与 `.cache/jiuwen-e2b-replay-3/` 的 evidence SHA 已核验；两轮均确认 provider/ledger/Runner 进程组/模型监听清理。record-6 的 35 Span 与 replay-3 的 6 Span 均仍为 `collected_partial`，flush 未证实、producer 分母未知，不能由功能 PASS 或出口 exit 0 推导完整覆盖。

## 华为原生运行通过：与 E2B 的接入验收

两边均使用 release17、image4、2 CPU / 4096 MiB、真实 DeepSeek 与原 12 项独立 Verifier。E2B record-6 为 7 轮 / 6 个工具 Span，华为 record-1 为 9 轮 / 8 个工具 Span，均 12/12、score=1、Tape 完整。两边分别自主解题，决策与工作量不同；下表报告实际运行，不作为固定工作量性能排名。

| 阶段 / 指标（秒） | E2B record-6 | 华为 record-1 |
| --- | ---: | ---: |
| Attempt 总时间（不含排队） | 96.089405351 | 81.359950457 |
| 排队 | 0.000020554 | 0.000021741 |
| 任务 Sandbox 创建 | 1.049705904 | 0.835284269 |
| 工作区准备 | 2.431876842 | 1.452701230 |
| Harness 启动 | 49.649368861 | 39.089174293 |
| Runtime 执行 | 31.705385024 | 34.955201159 |
| 停止与撤权 | 3.266068609 | 1.482337251 |
| 独立验证 | 4.646524651 | 2.326613364 |
| 清理 | 1.497100271 | 0.805840191 |

各阶段定义沿用本文，任务 Sandbox 创建不含评分环境；独立验证包含评分环境准备、测试与报告。Attempt不含排队；七个执行阶段之外仍有阶段间处理和产物处理开销，不用差额冒充某个工具耗时。

华为实际采集 43 个原生 Span（其中 8 个 authoritative tool Span、1 个 complete root），invalid=0、unassociated=0、未截断；原生 SIGTERM clean exit、停止隔离及 Bridge 撤权均确认。Snapshot/Perfetto/OTLP/Metrics 四出口通过仅代表已有证据投影；producer 分母与 exporter flush 在此 release17 样本仍未知，后续审计候选未被算入。

华为任务与评分 Sandbox 各创建 1 个，均经独立 provider NotFound 查询确认删除；临时 Adapter、Runner 进程组、8082 监听、凭据副本及共享锁均已释放，cleanup_errors=[]。Verifier 出网隔离沿用已获授权的华为假设，未声称平台已强制禁网，不计入完全隔离验证的 Goodput。

华为原件 `/opt/aries-artifacts/jiuwen-huawei-record-1`；evidence SHA `9e8d6c8ce41bd7abd1a5305245941ba8f3513a994201c55272fcaae0ee562dc5`；白名单 [验收与八阶段摘要](../.cache/jiuwen-huawei-record-1/acceptance-summary.json) SHA `552a2b60b44bdcf597e0bf270a4b9f4ca8567c6f8309548baefc63479767086c`；75 项原件清单 SHA `7acdb94446e8f291bec0aa30a0dc81ca407600043e2dde191dc7ccbf50506145`。原始模型、工具及评分正文保持私有。


<a id="release18-native-audit-cloud"></a>

## release18两云原生解题与producer导出专项

E2B record7与华为record2均通过原12/12、score1，完整Tape分别9/7轮；每轮Stop/revoke、两个Sandbox独立删除及cleanup=[]均确认。两份真实模型轨迹不同，仅作双云功能验收，不是同Tape性能对照；华为评分出网仍为显式假设。 同image4、2 CPU / 4096 MiB、原任务和原预算，均显式L1。sampled SDK Span的started/ended/exported分别44/44/44与35/35/35，missing/unexpected均0；`jiuwen_sampled_sdk_span_exports`专项integrity/coverage complete，global coverage仍unknown。未采样/未插桩行为、guest进程计时、跨机时钟与有界flush不在该证明内。

| 阶段 / 指标（秒） | E2B record7 | 华为 record2 |
| --- | ---: | ---: |
| Driver 总时间 | 119.788467 | 71.698673 |
| Attempt 总时间（不含排队） | 115.032528357 | 66.326138767 |
| 排队 | 0.000012826 | 0.000008122 |
| 任务 Sandbox 创建 | 1.209856992 | 0.645290200 |
| 工作区准备 | 3.524439408 | 1.225674994 |
| Harness 启动 | 54.277573689 | 22.790144901 |
| Runtime 执行 | 42.758910253 | 36.367483377 |
| 停止与撤权 | 3.568973688 | 1.740093417 |
| 独立验证 | 6.359066792 | 2.353149326 |
| 清理 | 1.490920267 | 0.762763236 |

阶段沿用[统一定义](performance-stages.md)，排队取attempt.queue_duration，其余七项取task.timing；Runtime使用runtime_execution_duration。独立验证包括评分Sandbox准备与原测试。Driver包含进程和归档等额外工作，不能与Attempt或各阶段互换。两轮工具数量、模型轨迹不同，不从此表做云性能排名。

两云分别44/35个官方Span、8/6个authoritative Tool Span及各1个complete root；invalid/unassociated均0、未截断。Stop后producer ledger与实际导出ID/终态集合对账，原始Unix ns仍未对齐Runner单调域。Snapshot/Perfetto/OTLP/Metrics四出口均PASS_PROJECTION_ONLY，本轮没有执行TP、Jaeger或Prometheus后端验收。controller共享进程、Agent cgroup四边界与provider整台Sandbox资源保持不同归属；设备块I/O不等于任务/逐Tool I/O，网络仍unsupported。采样最大值及cgroup生命周期内存高水位不称Runtime峰值。

两轮均完成原生SIGTERM clean exit、隔离Stop、Bridge撤权、任务及评分Sandbox独立NotFound、ledger无残留和cleanup=[]；unit inactive/dead、MainPID0，Runner进程组、8082、Adapter及临时凭据副本清理，实际共享锁获取后释放。华为Verifier出网仍是显式assume=true，不改称平台强制禁网。

固定工程版本为[release18](observability/VALIDATION_AND_REPRODUCE.md#jiuwen-release18)，812源文件、14文件增量及五门禁PASS。各运行原件根为`/opt/aries-artifacts/jiuwen-e2b-record-7`与`/opt/aries-artifacts/jiuwen-huawei-record-2`；仅取以下白名单小回执，原模型、工具与评分正文保持私有。

| 原件 | E2B record7 SHA-256 | 华为 record2 SHA-256 |
| --- | --- | --- |
| evidence.json | `e3b3dd82828264f9d43d89c57447b635986ef3a1ff026054ffd6112c3b4ffe58` | `60030d3189861034fee1693dfcc0c978b926604da013f2d05121a72c5fb27bee` |
| acceptance-summary.json | `c27f566f09942628474513a82a65d387f08bd15ec31ec56b75a6ae6b8d509913` | `b167ba6182f7bc1e3df46fe22ef40342f741c9a80c0a14e0b5fab7666a3b43da` |
| native-producer-acceptance.json | `74c8f74e737fb2ddb1380efe2e72c6dfe3592b38089074de9c5f22cc70679b3e` | `71c8c5a6db0e6d1ae128b2500533bcf3ee239d826217c8f2073c97a61895e7a4` |
| release-receipt.json | `a55f582697d2aa2d0f87aa2a855d2980121a5b3082d9ee701036dcb7ec9221a6` | `fc84dbdc47981b293de267b4149b76b9de7e1f701e82428e1f06d0b6fe6a7798` |
| raw-artifact-manifest.json | `535ae803421595526fd5d05b1cb36371ec49c92a0e65a97a60916c975aec6fb2` | `325ef2749d9177dc29d4abbf29a73cd9bf70aaf9a8551c27cbaab2a3e3565f60` |
| 原Tape | `688d90a31e39d8b4eb791e73b7a210ea22041d1bf066f500b8639aa008422abd` | `f66193f0ba0681eec005b40c2d3428462a0adc86e8695c3a58d66c880b23c89e` |

Strict replay3与record1–5旧失败保留；校验模式讨论延后，固定提示变体暂停，历史六例Strict仍3/6。 此次真实producer专项不回填旧release17无audit样本，也不关闭全局观测、实时/持久化或R01–R18其余条件。
