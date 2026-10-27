# DB / C++：Strict 实验结果与失败边界
表中“控制端”沿用原 `sdk_wait_duration_ns` / `exec.wait` 字段，实际从ARIES客户端提交进程前计到Wait返回，含提交、执行、输出传输与排空；不是单独Wait方法或官方SDK内部耗时。guest列是其独立wrapper窗口，两者不相减推断网络时间。


日期：2026-10-26。状态：**DB record2 原评分 0/7；C++ v3 华为 record 原评分 5/6，第 28 请求收到上游 HTTP 402。两份 Tape 均不完整，后续 Strict 未执行；两次 Sandbox 均已清理。上游恢复后 DB record3 已完成原 5/7，完整 Tape 未达回放门槛；C++ v2 E2B 3600 秒实验已实际创建：Strict 在第 6 请求地址差异处失败；独立 Workload 完成原 6/6，原两阶段预算不变。** DB 保留已批准的 `metadata-portable-v1`；C++ 历史新录制使用 `terminal-result-v3`，当前一次 Strict 复验复用 `terminal-result-v2` 合格 Tape。原题、初始数据/缺陷、时间预算、私有评分、Strict 完整历史匹配和真实工具执行保持。变体与原生任务分别统计；不将新录制或 Workload Replay 替代 Strict 成功。

## DB：先验证真实解题，再复用同一份合格 Tape

此前独立变体录制 `tb-repro-db-e2b-record-1` 的原评分为 **5/7**，68 轮 Tape 完整，Agent Runtime 368.380 秒，尚未耗尽 900 秒预算。最终导出包含 11 行，但 WAL 更新后的值不正确；已归档的最终写入来自字面数据列表。没有证据表明必需 `/app` 输入或目标输出写入被系统阻止。两次原生 host 策略拒绝缺少身份是另外的观测问题，不能用其修复推断解题通过。

原始证据位于 ECS `/opt/aries-artifacts/tb-repro-db-e2b-record-1/`；driver evidence SHA `67e440ef5d3a698078ca0465bf923c74b167a6d54cd0c520922c1b1c76477fee`。独立归因 `tb-repro-db-functional-review-1/conclusion.json` SHA `2aa16cd739e863699f717d822bae475bb7dad14a0e4ba7a37739ec9176fb5d1c`。[原变体归档](repro-variants-performance-20261026.md)保留失败分母。

更早的原生 Tape 曾在 E2B 通过原 7/7，但华为第 2 请求因目录 block 数、链接数、目录大小和日期差异被 Strict 拒绝。该失败不能仅由设置初始 mtime 解决，亦不能拿旧 E2B 成功替代新变体的功能验收，见[跨云原件](tb-crosscloud-performance-20261026.md)。

本次进一步复核同 ToolCall `call_00_DLnd8zd5HUPuv8PrlPin4213` 的原始遥测。两边首命令完全相同：`ls -la /app/ && echo "---" && file /app/* 2>/dev/null`。仅列该目录输出的实际元数据：

| 字段 | 已成功 E2B Strict | 失败 Huawei Strict |
| --- | --- | --- |
| `total` | 32 | 48 |
| `/app/.` 链接数 / 大小 | 3 / 60 B | 1 / 4096 B |
| `/app/..` 链接数 / 大小 | 24 / 4096 B | 1 / 4096 B |
| `/app/bin` 链接数 / 大小 | 2 / 60 B | 1 / 4096 B |
| `/app/.`、`bin` 日期 | Oct 25 16:07 | Jan 1 1970 |
| 两份输入数据日期 | Oct 24 2025 | Jan 1 1970 |
| 父目录日期 | Oct 25 16:28 | Oct 25 16:28 |

两份 binding 都明确请求 `/` mtime `1792945680000000000`，各自 `bridge/initial-directory-mtimes.json` 的命令 exit 0，`after_mtime_unix_ns` 都等于请求值。时间配置已生效；继续补目录时间仍无法设置真实目录 inode 的链接数、大小和分配块数。现有 binding 只提供初始目录 mtime，不包含一个已验证、可复现的底层目录布局对齐方案。因此不再以同一真实布局重放这份已知首条失败的旧 Tape。这是**完整历史文本契约**的可复现性差异，并不说明磁盘规格不同就不能进行性能比较。[只读原件复核](../.cache/db-cpp-strict-next-1/db-old-tape-metadata-review.json)列出每个源文件及 SHA；ECS 同名文件位于本页准备目录，SHA `8cddab6a78f1db018458a21e47278f3a8eaa10e60b11f20f2b30e68ff243df3d`。

record2 使用 release10 已修复的观测/传输边界和现成 DB profile，保持公开元数据附录不变，但未取得合格 Tape。用户确认上游恢复后，独立 record3 使用 release13 和新预检观测镜像，仍保持同一题目与附录。只有原评分 7/7、Tape 完整、身份与清理验收通过后，才能冻结该 Tape 并依次执行 E2B Strict 自回放、华为同 Tape Strict 回放；本轮没有自动重录循环。

### DB record2：第 52 请求失败，继发压缩请求被拒绝

`tb-repro-db-e2b-record-2` 于 **10:34:24–10:49:03 UTC** 执行。原评分 **0/7**；51 个模型请求成功，第 52 请求失败，Tape `complete=false`、failure `DELIVERY_OR_UPSTREAM_FAILED`。Agent Runtime **804.316848 秒**，低于原 900 秒预算，不能归因为题目超时。

第 52 请求获得 HTTP 200 SSE 后，运行 91.829301 秒失败。原边界记录收到 8,192 个完整上游帧，但 Tape 和下游各只有 8,191 个；最后收到的 466 B 帧在保存/发送前失败，没有 `frame_flush_failed`。已保存尾帧只有 content delta，未保存 finish reason、`[DONE]` 或 usage。累计帧数与字节数低于现有界限；原错误分类没有保留被拒绝帧的具体解析/验证错误。**精确首因未知，不能断言网络断流、上游超时或 `length` 终态。** 8,192 个帧与 `max_completion_tokens=8192` 只是线索。

随后有 7 次 `SESSION_FAILED` 和 8 次 `ADMISSION_CLOSED` 拒绝。Harness 最终报告 CLI transcript compaction 的摘要请求收到 409；它发生在首个失败请求之后，属于继发错误。此 Tape 不用于任何回放，不补造末帧或替代上游请求。

实际原生调用共 **51 次 exec、51 个 ToolResult、零其他工具**，其中 **50 exact、1 unlinked**。ToolCall `call_00_tysNw8rvzsuuudhI6bBz4835` 被原生 exec preflight 的复杂解释器调用检查拒绝，尚未派发到 Bridge；observer 只留下 invocation/started，缺少拒绝终态。它不是 host/node 拒绝，且发生后 Agent 仍继续调用工具；不能将该身份缺口写成最后 SSE 请求失败的原因。未知执行身份保持未知，独立修复原生外层异常观测。

Harness 停止与 Bridge 撤权均 confirmed；原题同 Sandbox 独立评分后 cleanup succeeded。driver 确认只创建 1 个 Sandbox、provider 不存在、ledger 全 absent、runner 进程组不存在，`cleanup_errors=[]`。systemd MainPID 0、8082 无监听、专属 adapter 不存在，共享锁可获取后已交还下一项实验。

| 原八阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000013976 |
| 任务 Sandbox 创建 | 1.048807811 |
| 工作区准备 | 1.113255549 |
| Harness 启动 | 7.389851382 |
| Runtime 执行 | 804.316848349 |
| 停止与撤权 | 5.597746851 |
| 独立验证 | 12.417130002 |
| 清理 | 0.794432557 |

双 Trace 使用同一真实 Snapshot，Trace Processor 与 OTLP 共 **70,660 项**对账通过；Perfetto 17,652 intervals / 272 flows，OTLP 17,652 spans / 272 parents，导入及结束时间误差均 0 ns。由于上述原生身份缺口，Snapshot integrity/coverage 均为 **incomplete**，结论仅为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`，原业务和 driver 仍 FAIL。未进行本轮 Jaeger 后端或浏览器验收。

原 driver evidence SHA `b9556611ed058c44c050e509248390f1210c95c3e8bc1c23444a7dd8f2a41984`；不完整 Tape SHA `33763b01aefa6314d7eda05b7968c6e8efbc4473169208cc5e5a2cc538ab3965`。[只读归因](../.cache/db-cpp-strict-next-1/db-record2-conclusion.json) SHA `2b86da0cc6b39981e6caa35f9bc760b19d52996c5f671773e40f2b0777ec9e49`，[八阶段及逐 exec 摘要](../.cache/db-cpp-strict-next-1/db-record2-summary.json) SHA `369243869e3c272f03ec7c6d1de2e814c975d55c67433e23ce41a2ec0ed0d3ae`，[双投影回执](../.cache/db-cpp-strict-next-1/db-record2-projection-receipt.json)指向 ECS `/opt/aries-artifacts/tb-repro-db-e2b-record-2-review-1/` 的原件。离线复核未新增云或模型调用。

针对这次暴露的诊断缺口，独立候选在既有 record 事件的 `Reason` 中增加固定白名单分类，区分 finish reason 不支持、读取不完整、界限超出、下游写入/flush 失败等；Tape/session 的失败语义、帧接受规则和 Strict 匹配保持。它不记录任意错误正文、不重试、不为旧轮补定根因。ECS 定向 `go test ./internal/modelreplay` 与 `go test -race ./internal/modelreplay` 均通过，包含 7 个真实 HTTP/parser 路径及私有错误文本不泄漏检查；随后补齐 Snapshot/双 Trace 白名单并通过 release13 全五门禁，DB record3 显式使用该版本。C++ v2 复验继续用其冻结 release10。[候选证据](../.cache/db-model-failure-diagnostic-1/evidence.json) SHA `10b42d20fe933c9fcd4ef1560003d11dd5e901abe63a50cf9e906b86c96a2131`，ECS 目录 `/opt/aries-artifacts/db-model-failure-diagnostic-1/`。首轮测试目录设置错误及后续更正也保留在证据中。

### DB record3：完整 60 轮，但原恢复值仍未通过

`tb-repro-db-e2b-record-3` 于 **12:00:34–12:14:15 UTC** 执行，release13 / native39964，原评分 **5/7**。60 个模型请求均成功，Tape 完整，SHA `e349b18f3ed273647ec27c5dc182463c93ac7898212b97ab9285f7f60686aed1`；runner exit 0，driver 因 `original_task_or_verifier_failed` 失败。原数据完整性和 WAL 解密两项验证未通过，不进入自回放/跨云 Strict，不自动追加录制。

旧 7/7 与本轮使用同一 E2B template、原 source image digest、benchmark revision、PATH 和工作目录。首目录记录均为 UID/GID60000 对应的 aries-agent、/app 0755、两份数据 0644；初始 DB/WAL 的已观察十六进制片段逐字相同。第 3 工具读取 WAL 成功，第 4 工具直接用 SQLite 打开原数据库，紧接第 5 工具读取 WAL 已找不到文件。稍后的 `/dev/vda` 权限拒绝及 elevated 请求拒绝发生在该文件消失之后，不能倒写成原始输入不可读。

在原镜像、相同 UID/GID 和权限下，用原输入副本执行第 4 个原命令，已离线复现 exit 0、DB SHA 不变而 WAL 被 SQLite 删除；原 fixture 未改变，专属本地容器已删除。随后本轮第 56 工具成功将字面数据写入 `recovered.json`，第 59 工具仍能读取目标文件；原失败断言是 WAL 更新后的值不符，不是输出写入受阻。离线副本证据位于 ECS `tb-repro-db-e2b-record-3-review-1/input-loss-probe-1/evidence.json`，SHA `ad59371ea85f67e39a028f8ef90afa538c56ebfc16157c1fbaae47c4888115d7`。这个短本地 probe 与随后 SQLite 任务有时间重叠，不能将样本称为隔离稳态性能基线。

实际为 **53 exec + 5 process + 1 sessions_history**，exec 为 **52 exact + 1 rejected_before_dispatch，unknown 0**。唯一拒绝是既有 `elevated_policy_rejected`，本轮未触达新增复杂解释器 script-preflight 修复。identity producer 的 54 条记录全部写入、无失败；另一条 native tool timing collector 因原生 agent 事件计数超过 4096 而保留 `event_limit_exceeded`，Snapshot integrity/coverage 仍 incomplete。该观测缺口与原业务 5/7 分开记录，未将观测改成完整。

只创建 1 个 Sandbox，provider/ledger/runner 进程组均 absent、停止撤权 confirmed、`cleanup_errors=[]`。12:15:46 UTC 实测共享锁可获取释放、8082 无监听、专属 adapter 为空，窗口已交 SQLite。原 evidence SHA `4b841e320068027e61b8ebd288acddfa10c16668f86a426684590a2356fd1237`；收尾回执位于同独立 review 目录，SHA `ec1dc0a4b3f7d4ef02a05820533217c36c68e9e04f951571d3b4c41b21ade8da`。双投影共 **46,082 项**对账通过：Perfetto / OTLP 均为 **11,508 intervals / spans、312 flows / parents**，导入和结束时间误差 0 ns；范围仍为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`，无后端或浏览器验收。

| 原八阶段 | 秒 |
| --- | ---: |
| 清理 | 1.108122161 |
| Harness 启动 | 7.385868811 |
| 排队 | 0.000017501 |
| Runtime 执行 | 754.607700362 |
| 任务 Sandbox 创建 | 0.988041849 |
| 停止与撤权 | 5.771735526 |
| 独立验证 | 11.487413366 |
| 工作区准备 | 1.453488210 |

[完整归因回执](../.cache/db-cpp-strict-next-2/db-record3-receipts/conclusion.json) SHA `4a2dca66aa2a25537523794be59eeb92ab853b9422fd5ea8a5f759ddbf6d3e87`、[八阶段与每次 exec 摘要](../.cache/db-cpp-strict-next-2/db-record3-receipts/performance-review-1/summary.json) SHA `bc58cb8a34c759711cafdd2dd179e287dcae56245407f7a035e2c11122273232`、[原数据副本复现](../.cache/db-cpp-strict-next-2/db-record3-receipts/input-loss-probe-1/evidence.json)及[窗口释放记录](../.cache/db-cpp-strict-next-2/db-record3-receipts/release-receipt.json)仅保存结构化证据，不含私有工具正文。双投影 evidence SHA `91c845f8696a9222b267f977dca343a2550fab0e5671854493121ee397bcae9f`。

## C++：历史通知顺序与本次地址差异分开归档

`tb-repro-cpp-v2-huawei-record-1` 原评分 **6/6**、Tape 完整、34 次 exec 身份精确；同 Tape `tb-repro-cpp-v2-huawei-replay-1` 在第 6 请求的 `$.messages[11].content` 真实 Strict 偏离，已执行 5/5 exec 身份精确，原评分 5/6。两次 ToolResult 都是 39 行、1,925 字节；两行 libthread_db 启动通知从第 21–22 行移到第 16–17 行，所有行及重复次数不变。

按同一 ToolCall `call_00_wMYrddcSIOQ0iohF67eF1088`，两次 `bridge/tool-calls.jsonl` 第 21 行均为 **stdout 1,926 B、stderr 0 B、completed / exit 0**。命令在 guest 内已经使用 `gdb --batch … 2>&1 | tail …` 合流，因此可观察到的两行均在合流后的 stdout。Bridge 没有保留该管道合流前 GDB 与 inferior 各自的原始双流，不能从现有原件还原它们各自的写入顺序，也不能仅凭行移动断言具体缓冲或调度机制。已证实的边界是内容顺序变化进入 native renderer 前就已存在，并非其双流呈现重排。

原 record Bridge 文件 SHA `34c4263df3df22125b702c01e594a52c2cb1ad3cc7b31e0ce88dfc212b99ff4d`；replay Bridge SHA `e0eb735580b9ecfef121af45fe4904d1dd486a36d3d1e22d2d8b4e8bbd51de63`。完整身份、首差异及失败结果见 [C++ v2 回放归档](repro-variants-performance-20261026.md#cpp-v2-huawei-replay-1)。

### 已排除的 GDB 假修复

官方文档中，`set print thread-events` 控制新线程与退出提示；`set suppress-cli-notifications` 控制上下文切换及程序停止通知，不能据此推定会关闭 libthread_db 的两行启动提示。[线程开关](https://sourceware.org/gdb/current/onlinedocs/gdb.html/Threads.html)、[CLI 通知开关](https://sourceware.org/gdb/current/onlinedocs/gdb.html/Other-Misc-Settings.html)。

已在 ECS 现有 C++ image-2（`sha256:8b9876c4520f40a29bec79b4986318b983119ddecc9b71f15dfbe08e4e8af5ef`）用原 GDB 15.0.50、UID/GID 60000、`/bin/true` 做三次无网络小复现：默认、关闭 thread-events、再关闭 CLI notifications。三者 stdout SHA 相同，两行提示始终存在；真实 stderr warning 也保留。该结果排除了这两个选项作为当前修复，不证明任意 GDB 设置都不可能改变表现。

ECS 证据：`/opt/aries-artifacts/cpp-gdb-notice-probe-1/evidence.json`，SHA `856c462909f96413e8f370de64c7f651409f7d8b97e564e6349ab3b5cd785504`。三个本地容器均已删除，0 云调用、0 模型调用。该小复现没有重放任务或修改镜像，也没有隐藏调试信息、关闭线程调试或安装 gdbinit。

v3 的公开日志说明已经冻结，材料证据 `tb-repro-cpp-workload-v3-1/evidence.json` SHA `b244a1af2397de281d255175047ede190bd284bb55824891a740f2caa7feb7d6`；它只提供交互约束，**不保证** Agent 会避开动态输出。本次按既有授权执行了一次华为真实录制，未取得可供 Strict 自回放的完整 Tape。另行进行的 Workload 同决策验收须独立标注，不增加 Strict 成功数。

### C++ v3 record1：上游 HTTP 402 终止录制

`tb-repro-cpp-v3-huawei-record-1` 于 **11:13:34–11:17:09 UTC** 执行，沿用 release10、native e040、原 Agent/Verifier 1800/1800 秒及 TTL 4200 秒。原评分 **5/6**；唯一失败为 release 构建运行仍以 **-11** 退出。Agent Runtime **171.338262139 秒**，尚未耗尽原预算。

27 个模型请求成功，第 28 请求的 `upstream_response_headers`（sequence 19231）明确记录 **HTTP 402**。该请求 0.681011089 秒后失败，没有收到或保存任何 SSE 帧；下游收到 502。随后 5 次 `SESSION_FAILED` 与最终 transcript compaction 409 均为继发错误。这与 DB record2 的已收末帧解析/验证边界不同，也不是本轮已验证的 GDB 顺序差异。上游错误正文没有保留，因此不推断具体账户或余额原因，不自动更换 key/模型或重录。

Tape `complete=false`、failure `DELIVERY_OR_UPSTREAM_FAILED`，SHA `d936c3c034bdb161bcddd8997825bd6ec3cd5237faba982ff9a640c1fe9db6cf`；**同 Tape Strict 未执行**。已归档原生会话中 27 次 exec / 27 个 ToolResult 全部 **27 exact**，无其他工具。`terminal-result-v1` producer index 为 `not_collected / producer_artifact_missing`，因此不能将未收集的后台终态协议计作通过。

Harness 停止、Bridge 撤权、独立评分及 cleanup 均完成。只创建 1 个 Sandbox，provider/ledger absent、runner 进程组 absent、`cleanup_errors=[]`。收尾时共享锁已可获取并释放、8082 无监听、systemd MainPID 0、专属 adapter 不存在，已交还窗口。

| 原八阶段 | 秒 |
| --- | ---: |
| 排队 | 0.000015679 |
| 任务 Sandbox 创建 | 0.595072692 |
| 工作区准备 | 0.757083410 |
| Harness 启动 | 7.245402891 |
| Runtime 执行 | 171.338262139 |
| 停止与撤权 | 5.406619502 |
| 独立验证 | 16.285334840 |
| 清理 | 0.651335575 |

同一 Snapshot 的双投影完成 **14,653 项**检查：Perfetto 3,650 intervals / 152 flows，OTLP 3,650 spans / 152 parents，导入及结束时间误差均 0 ns。由于后台协议 producer 原件未收集，integrity/coverage 均为 incomplete，仅为 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`；业务与 driver 仍 FAIL，未执行本轮后端或浏览器验收。

原 driver evidence SHA `5a850543b937c5d4f62d695a789ead9e9332ba5e5f38ba9c5350c4b51eddf235`。[只读归因](../.cache/db-cpp-strict-next-1/cpp-v3-record1-conclusion.json)、[八阶段及逐 exec 摘要](../.cache/db-cpp-strict-next-1/cpp-v3-record1-summary.json)、[双投影回执](../.cache/db-cpp-strict-next-1/cpp-v3-record1-projection-receipt.json)及[窗口释放记录](../.cache/db-cpp-strict-next-1/cpp-v3-record1-release-receipt.json)对应 ECS `/opt/aries-artifacts/tb-repro-cpp-v3-huawei-record-1-review-1/`。双投影 evidence SHA `d0cbe8f08b1481f10948b62e1a70c691407264765b53c5046cd34aeec0848d80`。离线归档未新增云或模型调用。

### C++ v2 E2B 3600 秒实验：创建成功，Strict 在第 6 请求出现地址差异

`tb-repro-cpp-v2-e2b-3600-replay-1` 于 **12:32:46–12:33:57 UTC** 执行。E2B 接受 **3600 秒** TTL，实际创建并运行 1 个 Sandbox。原 Agent / Verifier **1800 / 1800 秒**不变，两阶段与准备、清理共享这 3600 秒存活期；这不保证两个阶段都能耗尽预算。专用 driver 只在本 case / E2B / replay 范围取消原 `agent + verifier + 300` 的寿命下限；公共 driver、核心、原任务、Strict matcher 均未改。冻结 release10 / native e040 / v2 附录和原合格 Tape `ea1a8d1b…` 保持。E2B binding 从已准备原模板派生，只补入原 v2 录制前已经显式声明的根目录 mtime。

本轮 **5/35 模型轮、5/34 exec** 完成，第 **6** 请求 `REPLAY_DIVERGED`，剩余 **29** 个工具未执行。首次差异对应 ToolCall `call_00_wMYrddcSIOQ0iohF67eF1088` 的 GDB ToolResult（历史消息位置 11）：两边均 39 行、1925 B，只有第 35–38 行 backtrace 中 **9 个十六进制地址 token** 不同，其余四个已执行 ToolResult 逐字一致。本次不是历史 Huawei 的两行 libthread_db 通知换序。地址比较仅用于离线归因，原正文和 Strict 匹配不做替换、过滤或归一化。

原评分 **5/6**，未通过 release 构建运行无崩溃一项；driver 后续 `native_decisions_differ_from_real_tape` 是完整 Tape 分母审查发现只执行前缀，不能替代更早的 Strict 首因。5 个 exec 身份均 exact；已停止 Harness、撤权并完成原独立评分，provider/ledger/runner 进程组 absent、cleanup 无错误。12:36:51 UTC 实测锁释放、8082 无监听、专属 adapter 为空。

| 原八阶段 | 秒 |
| --- | ---: |
| 清理 | 0.613753252 |
| Harness 启动 | 7.174657966 |
| 排队 | 0.000017722 |
| Runtime 执行 | 30.340886711 |
| 任务 Sandbox 创建 | 1.061035675 |
| 停止与撤权 | 5.551500762 |
| 独立验证 | 17.045061101 |
| 工作区准备 | 0.791445605 |

| exec 序号 | 模型轮 | SDK 秒 | guest 秒 | exit |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 0.504746160 | 0.183235205 | 0 |
| 2 | 2 | 0.371084886 | 0.035139838 | 0 |
| 3 | 3 | 9.098558540 | 8.845161334 | 0 |
| 4 | 4 | 0.841642032 | 0.591194907 | 0 |
| 5 | 5 | 3.702941965 | 3.454037280 | 0 |

未执行工具的耗时不填零；此 30.340886711 秒 Runtime 是失败前缀，不能用来估计完整 C++ 工作量。后续独立 E2B Workload 另开 Attempt，不改判本次 Strict。

双投影 **2,005 项**对账通过，488 intervals / spans、36 flows / parents，误差 0 ns；源观测仍 incomplete，范围仅 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`，无后端或浏览器验收。[归因回执](../.cache/cpp-e2b-3600-strict-1/receipts/conclusion.json) SHA `a95cce65b7b3e5a69ae55f11919a0238fbd960c89e4d1d23ceff32762fd2587b`、[八阶段和 exec 摘要](../.cache/cpp-e2b-3600-strict-1/receipts/performance-review-1/summary.json)、[首差异元数据](../.cache/cpp-e2b-3600-strict-1/receipts/first-difference.json)与[收尾记录](../.cache/cpp-e2b-3600-strict-1/receipts/release-receipt.json)对应 ECS 同名 review 目录。原 evidence SHA `e479d2589f419de7fba2fa0f18ca075ae24d3d74331e5e6b4af73399a38718b7`；双投影 evidence SHA `bfab929234d6e96d9dd0fedba1f8a7746f82db34a9455210931637af07d6629b`。

## 执行顺序、时限与观测口径

| 路径 | 原 Agent / Verifier 预算 | Sandbox TTL | 当前状态 |
| --- | --- | --- | --- |
| DB：E2B record → E2B Strict → 华为同 Tape Strict | 900 / 900 秒 | 2400 秒 | record2 FAIL；record3 原 5/7、Tape 完整、业务 FAIL，后续 Strict 未执行 |
| C++ v3：华为 record → 华为 Strict | 1800 / 1800 秒 | 4200 秒 | record FAIL，原 5/6、上游 HTTP 402、Tape 不完整；Strict 未执行 |
| C++ E2B 历史 4200 秒请求 | 1800 / 1800 秒 | 历史 profile 4200 秒 | 平台曾拒绝创建；不能据此断言实际任务跑不完 |
| C++ v2 E2B 本次存活期实验 | 1800 / 1800 秒 | 共享生命周期 3600 秒，含准备与清理 | 已实际创建运行；Strict 第 6 请求地址偏离，独立 Workload 35/35、34/34、原 6/6 通过 |

历史 DB record2 与 C++ v3 record1 沿用 `/root/aries-setup/repro-agent-run-5/driver.py` 与 `validation.py`、冻结 release10、`tb-repro-agent-inputs-4/pins.json`。现有 driver 对所有 provider 使用同一 `/opt/aries-artifacts/e2b-platform-deployment-1/deployment.lock`，模型代理监听 `127.0.0.1:8082`。因此材料可并行准备，DB、C++ 与 SQLite 真实运行必须等待前一项结束；不移除锁或抢占已有服务。

本次已使用两个逐次调用的 launcher，分别由现有 driver 管理一次 Attempt、一次 Sandbox、评分及清理，没有自动重试或新调度器。DB record2 和 C++ v3 record1 均已执行、失败并清理，未满足后续 Strict 的 Tape 前置条件。

DB Strict 路径结束后已交还共享窗口，完成原 7/7 DB Tape 的独立 Workload 双云对照以及 SQLite 新模板复测后，才启动 C++ v3。两条 DB 验收路径分别保留原始分母和匹配语义，Workload 不替代 Strict。

ECS 材料位于 `/opt/aries-artifacts/tb-db-cpp-strict-preparation-1/`；[静态 evidence](../.cache/db-cpp-strict-next-1/evidence.json) SHA `c1fd8fd538dd521d2d8ee8748d8b2fe5ffc416ed539c097a7f67f81d4473d10a`。650 份 release10 源文件逐 SHA 一致；原 DB instruction、task、Dockerfile、两份 fixture 与两份独立评分文件共 7 份同时符合原模板准备 manifest 和 Git HEAD。E2B binding 的模板、源镜像 digest、build evidence 与原件一致；这项准备没有重新测量云 guest 文件系统。启动时现有 driver 仍验证固定 revision、干净 checkout、source manifest、binding 和 native image。

固定 native image `e040654efde8adde910583291b547fb9d5fc94e565b2f4dc8bad29abcc423041` 包含 R01 host/node 拒绝身份修复；原生合并证据 `r01-native-combined-3/evidence.json` 保留 5 次拒绝身份与 8 次 error body 不变的检查，不将修复当成 DB 解题通过。两个 launcher 已在 ECS 通过 `bash -n`；该批静态准备为 0 云调用、0 模型调用、0 Attempt。

已执行的本地 [DB launcher](../.cache/db-cpp-strict-next-1/launch-db.sh)、[C++ launcher](../.cache/db-cpp-strict-next-1/launch-cpp-v3.sh)及[systemd 启动和 wait 命令](../.cache/db-cpp-strict-next-1/commands.md)保留供审阅。直接入口分别为 `bash /opt/aries-artifacts/tb-db-cpp-strict-preparation-1/launch-db.sh record` 和 `bash /opt/aries-artifacts/tb-db-cpp-strict-preparation-1/launch-cpp-v3.sh record`；原输出目录保留，不再次提交。ECS 同时发生的工程检查按实际负载条件披露，样本只用于功能验收及性能留档，不推导厂商稳态排名。

两次结果均已保留八阶段原始纳秒值、原评分分母、Tape 身份、逐 exec 精确映射及后台协议实际触达情况。缺失/未执行不填零，未触达后台协议不算通过；失败前缀不作为完整工作量的性能比较。独立 C++ v2 Huawei 和后续 E2B Workload 均已完成原 6/6、35/35 模型轮、34/34 精确工具终态，0 上游模型请求，各自 Sandbox 已清理。Huawei 轮历史正文差异为 0；E2B 轮有 8 个 ToolResult 差异、128 处重复历史，差异允许分支实际触达。两轮计为同 Tape 跨云工作量功能通过，整体观测仍 incomplete，不追加 Strict 成功。其单独入口位于 ECS `/root/aries-setup/cpp-workload-cloud-1/`，[准备证据](../.cache/cpp-workload-cloud-1/preparation-receipt.json) SHA `ac93e4aa697ae0b0a3bd85749f78379c7dff8418a4545aa73276cc07ffb3d4c6`。只接受原 v2 的 35 轮、34 个同步 exec、原 6/6 评分和精确终态；原后台协议未触达且 producer 文件缺失仍保留整体观测 incomplete，局部出口仅对该已知缺口验证投影，不更改 SQLite 或全局验收。

用户确认模型上游已恢复后，已准备 DB metadata-v1 record3 的新独立 Strict 路径；不会续写旧 Tape。C++ 原计划的追加 Huawei v2 Strict 已取消未执行，v3 record2 也未执行。用户后续授权复用旧 v2 合格 Tape，已完成一次 E2B 3600 秒共享存活期 Strict 失败和另一独立 Workload 成功。release13 安全错误分类及其 Trace 白名单的五门禁已全部通过，新预检观测镜像 `39964c39357eee32a1c5fff7c5562aebfca2495fcdeb7a7871dd06fed73134ea` 发布证据已核验。新 pins SHA `fe3fcf287a9093891913ef581493291e8d986484b45c75adbc70d6cb9d9869b8`，658 份 source manifest 文件逐 SHA 一致；DB record3 已完成原 5/7，业务 FAIL，未进入 Strict。原题、附录、评分及时间窗口保持，只有新的合格完整 Tape 才进入自回放或跨云回放，不自动循环录制。

C++ v2 Workload 的两云完整分项结果、并排八阶段与 34 次 exec、18,317 / 18,417 项双投影对账及观测限制见[独立跨云归档](workload-cpp-performance-20261026.md)。

第二批[冻结回执](../.cache/db-cpp-strict-next-2/ready-receipt-2.json)对应 ECS `/opt/aries-artifacts/tb-db-cpp-strict-preparation-2/`。release13 evidence SHA `13c28e90f0b5b1eda5737c49084142b30f0febfdce2d2f45250acab996c33987`，source manifest SHA `d54b8a9380fa81ebc54d0331238e5872d773fc500c1f15286af40a1cac0075d7`，freeze SHA `bed805677e917fca806f0786164ed9af3b14740a57e84493b86f049e9b4611eb`；新 native publication SHA `b6551b9959eaf494c9c77e6a100505cfca02945a137068b6a2e323f467512951`。专用 Strict driver 的四行版本适配为 release13 固定路径及其诊断标签、新 native 固定 digest、实际检查状态 `PASS_SCRIPT_PREFLIGHT_REJECTION_TARGETED_ONLY`；原执行、评分、匹配、停止清理逻辑及 validation 字节不变。首个 systemd 启动因遗漏旧 release10 路径硬编码而在 588 ms 前置拒绝，0 Attempt / 0 Sandbox / 0 模型调用，原日志与[启动失败回执](../.cache/db-cpp-strict-next-2/startup-attempt-1.json)保留。一次完整复核所有版本固定值后，[ECS 静态检查](../.cache/db-cpp-strict-next-2/preparation-check-2.json)通过，随后使用新 systemd launch-2 首次真正执行 record3。C++ 原计划的追加 Huawei Strict 和 v3 record2 已取消，未执行准备材料保留。本次 E2B 3600 秒 Strict 使用 release10，独立 Workload 使用 release12；二者保持 e040 / 原 v2 Tape，详见本页实验段及跨云 Workload 专页。
