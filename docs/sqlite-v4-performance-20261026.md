# SQLite v4：真实录制失败与原生恢复中止（2026-10-26）

本轮 `terminal-result-v4` 仅执行一次 E2B 真实录制，原私测 **2/3，reward=0**，Tape 不完整，整体 **FAIL**。没有启动 E2B Strict 或 Huawei Strict，没有自动重试。它与[旧 v3 的 mtime 归因](sqlite-v3-mtime-rootcause-20261026.md)、旧 v3 的 3/3 Workload 结果分别统计，不覆盖旧结果。

UTC 2026-10-26 12:16:19.775848 至 12:26:33.299506，driver 用时 613.524 秒；实际 Runtime 执行 571.106 秒。唯一 Sandbox 已删除，provider、ledger、runner process group 均确认不存在，cleanup_errors 为空；共享锁实测可获即放、8082 无监听、owned adapter 为 0、systemd MainPID 为 0，已交给下一项 C++ 实验。

本次准备与执行沿用冻结 release 13、native `39964c39357eee32a1c5fff7c5562aebfca2495fcdeb7a7871dd06fed73134ea`。三阶段 launcher 原样复用已冻结通用 driver，未加产品补丁、环境探针 hook 或新模板。原题、私测、900 秒 Agent/Verifier 预算、1 CPU/2048 MiB 不变。公开附录和 `TIME_STYLE="+"` 范围见 [v4 材料](../cases/sandbox-runtime-v1/agent-cases/sqlite-with-gcov/variants/terminal-result-v4/README.md)。

## 失败的直接证据

模型服务没有在本轮返回 HTTP 402 或其他非 200：45 次请求全部完成 HTTP 200，request-boundaries 索引完整、45 received/45 finished、0 dropped/0 write_failed，没有失败或拒绝的安全 reason。45 个录制 turn 的 finish_reason 全为 `tool_calls`，没有最终 `stop`；随后模型 session 以 incomplete 结束。不能把上游 HTTP 成功说成 Agent 已完成。

原生 gateway.log 第 121–125 行记录了主动恢复中止：12:26:01.751，`stuck session recovery` 以 `age=567s`、`action=abort_embedded_run`、`aborted=true` 取消原生运行；两秒后的 abort settle 超时与 `OPENCLAW_DIRECT_ABORT` 被返回给 Harness。driver 最终 `tape_incomplete` 是收尾检查结果；不是一次 Strict matcher 分歧，也不是本轮上游服务欠费。

最后两次模型决策是 turn 44 的 exec `df -h /app; tclsh -e 'puts [info patchlevel]'`（`call_00_FdvoAZ8vmY1GF7Y7ZgAT9659`）和 turn 45 的 `process.wait`（`call_00_NfC0jYmFqyiAVEm96g9R5317`）。该 exec 被原生后台协议 accepted，后续 wait 最终为 interrupted；这个 job 没有归档到 terminal_result。按同一个 native 单调时钟，accepted 至 interrupted wait 为 **376.973516523 秒**，没有跨 host/guest 时钟相减。不能把最后的命令或 wait 记为成功。

原生恢复条件已做有界只读核对。`diagnostic-BHCP1NDL.js:1373` 的 age 是 `now - state.lastActivity`，与独立的 `lastProgressAgeMs` 不同；active tool 的恢复条件要求 **activeToolAgeMs 和 lastProgressAgeMs 都达到 abort 阈值**（990–993 行）。默认 warn 为 120 秒，默认 abort 是 `max(300秒, 3×warn)=360秒`；本轮归档配置没有覆盖这两个值。实际日志第 111–120 行显示 `blocked_tool_call`、activeTool=process：最后进展从 135 秒持续增到 345 秒时 recovery=none，375 秒时才 recovery=checking；此时显示的 age 为 567 秒。因此本轮证据不支持“仅因固定 session 总年龄、仍有进展却被误杀”的归因。

可以确认原生所观察的 process 工具持续没有进展，并由既有恢复机制中断；不能由这些日志进一步证明 tclsh 在等待哪一个内核资源或永远无法完成。本轮没有据此修改恢复阈值、额外添加 watchdog 或改变 Agent 命令；原失败保持。源码只从既有本地 image layer 读取两个编译模块，没有创建容器、snapshot 或 mount，也没有新云/模型探针。

原始私测结果为：

| 原始测试 | 结果 | 原件解释 |
| --- | --- | --- |
| test_sqlite_compiled | PASS | 编译产物检查通过 |
| test_sqlite_in_path | PASS | PATH 中 SQLite 检查通过 |
| test_gcov_enabled | FAIL | 验证器运行 `sqlite3 :memory: 'SELECT 1;'` 成功，但遍历 `/app/sqlite` 未找到任何 `.gcda` 文件 |

这说明原任务未合格；不把“没有找到 .gcda”扩大解释为已完整证明所有编译参数。验证仍在 Harness stop 与 Bridge 撤权确认后独立执行。

## 环境和观测的实际范围

两云冻结 binding 均声明 `command_env.TIME_STYLE="+"`；既有 DefaultEnvironment→每调用覆盖→group payload→真实 guest execvpe 路径，以及两个固定任务镜像里的 GNU `ls`/错误/文件时间不变探针已通过。当前冻结版本没有配置级 Agent 前环境回显 hook，因此本轮**未直接回显 guest 的 TIME_STYLE 值**，不把配置或源码当成读回值，也未为此修改 adapter。已保留的最后请求历史没有匹配到旧 v3 的三项 libsqlite3 长格式列表，本轮不能宣称已用真实 Strict 验收消除了那三处 mtime 分歧。

原生事件 index 有 150 events、terminal_observed=true，但 complete=false，问题为 incomplete_tool_pair；telemetry.index.json 的 paths 为 null，没有归档到原生 assistant/ToolResult transcript。身份 producer 自身 50 attempted/50 written、0 write_failed，不能替代独立原生调用分母。Snapshot 的 integrity 和 coverage 都是 incomplete，诊断包括 native index、exec identity 和后台 accepted binding 不完整。没有发现本轮 collector 4096 上限诊断；不能用其他实验的上限问题替代本轮事实。

下表以实际 Tape 的 45 turn、49 exec 和 2 process 为模型决策分母，将原始 ToolCall ID 精确连接到已有 producer、Bridge 和 exec.wait 记录；它只补充失败样本的原始计量呈现，不把缺失的原生 transcript 补成完整身份/ToolResult 验收。一个 exec 在派发前拒绝，未伪造 guest 时间。

本轮同快照 Chrome Trace/OTLP 验证结果为 **PASS_PROJECTION_OF_INCOMPLETE_SOURCE**：13,734 项投影检查、3,422 个 interval/span、237 个 flow/parent；Trace Processor 导入错误为 0，时长投影误差为 0。这个结果只证明保留的证据能一致投影，不证明源完整或业务通过；未声称 OTLP 后端交付或本轮浏览器验收。

运行期间并行发生一个短本地 DB 离线 probe，完成并确认删除于 12:25:57.139295 UTC。其无网络、只读根文件系统、UID 60000、timeout 60 秒；没有记录精确开始时刻/CPU/内存测量，也未指定 CPU/内存上限。因此本次时长保留为有干扰的单次失败样本，不作为隔离稳态排名。

## 八阶段与逐工具原始读数

阶段采用 [统一八阶段](performance-stages.md)，Runtime 取 `runtime_execution_duration`。阶段覆盖范围不同，不强求相加等于 attempt 或 driver 总时长。SDK 客户端列保留适配器调用方 `exec.wait.client_duration_ns`，并非某一次 SDK `commands.run` 的耗时；guest 列取 `guest_duration_ns`。两者计量边界不同，保持分列，不相减推断开销；native wait 另列且时钟未对齐。`—` 表示不存在或未采集，绝不是零。

| 阶段 | 原始秒数 |
| --- | ---: |
| 排队 | 0.000034 |
| 任务 Sandbox 创建 | 1.794976 |
| 工作区准备 | 0.651806 |
| Harness 启动 | 7.467870 |
| Runtime 执行 | 571.105773 |
| 停止与撤权 | 5.568298 |
| 独立验证 | 13.690925 |
| 清理 | 0.709484 |

| Turn | 工具 / 原始 ToolCall ID | SDK 客户端秒 | Guest 秒 | Native wait 秒 | exit | 原始状态 |
| ---: | --- | ---: | ---: | ---: | ---: | --- |
| 1 | exec `call_00_18f1TPITqN5xeWhnqFVT3209` | 0.601443 | 0.130235 | — | 0 | succeeded |
| 1 | exec `call_01_WR6iAxmDDUX5jC2TKOev1727` | 0.610928 | 0.191452 | — | 0 | succeeded |
| 2 | exec `call_00_9r3exuGE6lLWgxJCpEiD0823` | 0.345119 | 0.073794 | — | 0 | succeeded |
| 2 | exec `call_01_z4NZFLavmJh0OQWQVlcc9237` | 1.148553 | 0.879705 | — | 0 | succeeded |
| 3 | exec `call_00_XVIacR25oAolQVU3nAsr0997` | 0.892315 | 0.623277 | — | 0 | succeeded |
| 4 | exec `call_00_2pwB9j3PddaYQaFkv1aP4771` | 0.283490 | 0.028418 | — | 0 | succeeded |
| 4 | exec `call_01_CFPfEiPz7EUGUqqaK4Re9610` | 0.337409 | 0.029168 | — | 0 | succeeded |
| 5 | exec `call_00_ToA6IG6MChRsvIS3kd1I7646` | 0.317542 | 0.040769 | — | 0 | succeeded |
| 5 | exec `call_01_HdbsmiidV958V13LYmj12906` | 0.387011 | 0.034352 | — | 0 | succeeded |
| 6 | exec `call_00_KlrHoiLupLaYThp6ZvWG4284` | 0.300619 | 0.025227 | — | 0 | succeeded |
| 7 | exec `call_00_Sdp2U2A1mtponsZWQqji3901` | 0.434321 | 0.039174 | — | 0 | succeeded |
| 7 | exec `call_01_TOMrohO9dQ7aXEurzM3p8695` | 0.325263 | 0.028686 | — | 0 | succeeded |
| 8 | exec `call_00_2oySmAPBGrUjkZTbOtpU9779` | 0.349893 | 0.031044 | — | 0 | succeeded |
| 9 | exec `call_00_NCCkWsBhsuhCLbaiOS862499` | 0.317716 | 0.032535 | — | 0 | succeeded |
| 10 | exec `call_00_l9581Tc50pURz8JSIgkH2328` | 0.301645 | 0.032407 | — | 0 | succeeded |
| 11 | exec `call_00_tzdhxEnqx2boBO2uDCGx5253` | 0.299502 | 0.030316 | — | 0 | succeeded |
| 12 | exec `call_00_oOQaMHs3LZr3Ydri9gEC2661` | 0.309636 | 0.036658 | — | 0 | succeeded |
| 13 | exec `call_00_C9d8JQra2Wqv3lEBpoRl2117` | 0.291709 | 0.030951 | — | 0 | succeeded |
| 14 | exec `call_00_qgNWxHGquuIYZ69No1rA8808` | 0.289579 | 0.027980 | — | 0 | succeeded |
| 15 | exec `call_00_DgZ6EXxdgqfrx0SPDNHZ1892` | 0.333381 | 0.026450 | — | 0 | succeeded |
| 16 | exec `call_00_hkV358tRXX0JA4RTPd5J4376` | 0.301867 | 0.027198 | — | 0 | succeeded |
| 17 | exec `call_00_CradAiEHppn8tFbl2hlB7950` | 0.328902 | 0.041281 | — | 0 | succeeded |
| 17 | exec `call_01_OlbK3MJmVUpkDk0qgqPF5975` | 0.343557 | 0.031782 | — | 0 | succeeded |
| 18 | exec `call_00_uVa1jgh2hkHucUdRxumW3218` | 0.298719 | 0.031923 | — | 0 | succeeded |
| 19 | exec `call_00_doOEgrKJhvJRn4fGhcmE6344` | 0.307694 | 0.038171 | — | 0 | succeeded |
| 20 | exec `call_00_G98UtFAaliz3GUBGuyg40831` | 3.718807 | 3.461499 | — | 0 | succeeded |
| 21 | exec `call_00_lH1inS27GfZIINuhqjNE9670` | 0.317741 | 0.033825 | — | 0 | succeeded |
| 22 | exec `call_00_t5UnB0pdOpV5usQ1XoAL8436` | 15.818521 | 15.546119 | — | 0 | succeeded |
| 23 | process `call_00_qDymkfcp25OJOz1iD2qy3654` | — | — | 13.928377 | — | returned |
| 24 | exec `call_00_KqAgSvKanfrs8j1hJT9U3501` | 0.332918 | 0.027758 | — | 127 | failed |
| 25 | exec `call_00_Rjg64js3zrjBpX0EpBPh4257` | 0.318092 | 0.048283 | — | 0 | succeeded |
| 26 | exec `call_00_5RYbdrKAlG0G3tAuSVUJ8921` | 0.309763 | 0.040658 | — | 1 | failed |
| 27 | exec `call_00_brQYU1yZivT555rXVDDi3827` | 0.304380 | 0.036868 | — | 0 | succeeded |
| 28 | exec `call_00_j06FhHalbIVkU4kkgLYf1346` | 0.321507 | 0.035278 | — | 0 | succeeded |
| 29 | exec `call_00_izRzgCTG7Y7J0pT8dF4Q5390` | 0.311584 | 0.031635 | — | 2 | failed |
| 30 | exec `call_00_4wqP8G947fyEea2XFOoK1577` | 0.302894 | 0.038022 | — | 0 | succeeded |
| 31 | exec `call_00_CtmfphkYkBEmKyIDdpbt7966` | 0.568742 | 0.284132 | — | 0 | succeeded |
| 32 | exec `call_00_wFtXMs2AwGfoSXQcIuXo3602` | 0.342249 | 0.035200 | — | 0 | succeeded |
| 33 | exec `call_00_v8q9rFryneiOm9LATgp57735` | 0.312279 | 0.034416 | — | 0 | succeeded |
| 34 | exec `call_00_zgVI3Mnqj1wXmvUHu1Hv3203` | 0.332610 | 0.032916 | — | 0 | succeeded |
| 35 | exec `call_00_qOuWDpSjiLwDWgYQwstg4082` | 0.297082 | 0.030393 | — | 0 | succeeded |
| 36 | exec `call_00_MpAiD96huok5OacfSNr70543` | 0.313313 | 0.031070 | — | 0 | succeeded |
| 37 | exec `call_00_Mx4OkQGoBnpmg6rjBz7c0042` | 0.302905 | 0.033046 | — | 0 | succeeded |
| 38 | exec `call_00_TBR0ZWot3Fk1AqhqWTFP0195` | 0.328580 | 0.033198 | — | 0 | succeeded |
| 39 | exec `call_00_o1DJTCjRVVjInJHFJEMf8937` | 0.291808 | 0.027378 | — | 0 | succeeded |
| 40 | exec `call_00_vaWp4dk9E7SPzpeZsR9g5792` | 0.321541 | 0.039909 | — | 0 | succeeded |
| 41 | exec `call_00_jImp1y6MZa4RlA3Vg1216111` | 0.332549 | 0.031609 | — | 0 | succeeded |
| 42 | exec `call_00_U2s6rN0sThHYsu0fkp7f4982` | 0.319531 | 0.037382 | — | 0 | succeeded |
| 43 | exec `call_00_tMk5J5yGAEb51cMuxPUI0146` | — | — | — | — | rejected_before_dispatch |
| 44 | exec `call_00_FdvoAZ8vmY1GF7Y7ZgAT9659` | 394.244759 | — | — | — | canceled |
| 45 | process `call_00_NfC0jYmFqyiAVEm96g9R5317` | — | — | 375.008677 | — | interrupted |

## 可复核原件

- 真实运行根：`/opt/aries-artifacts/tb-repro-sqlite-v4-e2b-record-1`。
- Run：`20261026T121621.375099804Z-tb-sqlite-with-gcov-terminal-result-v4-record`；occurrence：`sqlite-with-gcov-terminal-result-v4-001`。
- 失败只读归档：`/opt/aries-artifacts/tb-repro-sqlite-v4-e2b-record-1-review-1`；audit.json SHA `c3598452edabf2ee5569c2b9141146df0194ba04d7465a9af87fc45216f4285e`，按路径保存所有输入 SHA 和读取后未变检查。
- 清理交窗回执：同目录 release-receipt.json，SHA `7814fc75b004ffd89dc2106f8aa1d5d373ccfa7a3a4be035dfa2eb672af76751`。
- 双投影回执：运行根 performance-review-1/evidence.json，SHA `760661a5dacd01cf87ccced9c80814288da36f76a35955836f317877673e1e4a`。
- 原生恢复源码与实际配置/日志：失败归档 native-source/evidence.json，SHA `f2013643f321edd3aee681dc18d45d7d50c44aa55420ee1bc18bfd5e32045513`；diagnostic-BHCP1NDL.js SHA `e87f208ebcb9fe488cf223aec61e468b5698704c5271be620652a1a9856efa48`，recovery runtime 模块 SHA `c21f1f3d0ff62a73b6ae9e4ddc500ac3551583d2923da2e0945f25c6eed96b1a`。
- 材料冻结：`/opt/aries-artifacts/tb-repro-sqlite-workload-v4-1/evidence.json`，SHA `2c0bbe3f67f7a08a4d551363dd7a60d57579002b6a169ee07bca22664b278a91`；启动核验 ready.json SHA `df2a6bfab74f03645e973fe5115f2e88dd35d3ee339ce0d96bcc3e77f8ad1b7e`。
- 并发负载原件：`/opt/aries-artifacts/tb-repro-db-e2b-record-3-review-1/input-loss-probe-1/evidence.json`，SHA `ad59371ea85f67e39a028f8ef90afa538c56ebfc16157c1fbaae47c4888115d7`。
