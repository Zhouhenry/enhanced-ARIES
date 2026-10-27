# SQLite v3 Workload Replay 真实验收（2026-10-26）
表中“控制端”沿用原 `sdk_wait_duration_ns` / `exec.wait` 字段，实际从ARIES客户端提交进程前计到Wait返回，含提交、执行、输出传输与排空；不是单独Wait方法或官方SDK内部耗时。guest列是其独立wrapper窗口，两者不相减推断网络时间。


本页单独记录 `replay_policy=workload` 的真实工具回放。它固定原 Tape 的模型决策、工具名称、参数、调用顺序及非工具历史，允许真实文本 ToolResult 与录制值不同；差异按 UTF-8 字节数和 SHA-256 归档。原任务、私有 Verifier、真实工具和清理要求保留。Strict 主线仍使用 release10，本页不将 Workload 成功计为 Strict 闭环。

E2B 第二轮与公开 profile 环境对齐后的 Huawei 第二轮均完整通过同 Tape、release12 的 Workload 验收：各 41/41 模型轮、原 Verifier 3/3、40 个工具终态与原录制相同，差异审查和双 Trace 对账通过。两云此固定工作量已闭环；Strict 闭环状态不变。四轮各只创建一个 Sandbox，均确认删除及 provider 不存在，cleanup 无错误；本路最终共享部署锁、模型入口和临时容器已释放。

E2B 首轮因冻结 Snapshot 重载错误而整体验收 FAIL；Huawei 首轮因真实读取缺失的 `/etc/profile.d/shell.sh`，exec 29 从原录制 exit 0 变为 exit 1 而整体验收 FAIL。两轮原 Verifier 均为 3/3，历史失败与原件全部保留。Snapshot 修复进入 release12；Huawei 环境对齐只增加该公开 shell 配置文件。

## 固定输入与验收范围

- release11：`/opt/aries-artifacts/workload-replay-release-11`，工程门禁状态 `PASS_REPLAY_POLICY_FIVE_GO_GATES_ONLY`。
- release evidence SHA：`4207eea46954a605084fa02ef04ce522daa7ae4b8c1ed9b4f39363c4e249561f`。
- source manifest SHA：`409a911602b3ae6aea944faf3fc1e987cff29e91965665c6697cf580ca339851`。
- 复用真实成功的 `sqlite-with-gcov-terminal-result-v3` 原录制：41 轮、38 次 exec、2 次 wait、2 个后台 job、原 Verifier 3/3；Tape SHA `a73e482ea845c44b370df656d5358b8231a5c161e1acdbd5e7da1d8edda72d46`。
- 每次独立新 Sandbox、concurrency=1、attempts=1、`timing=recorded`，无上游模型请求；E2B 与华为共享部署锁并顺序执行。
- 原公开任务预算保留；本批有效 Sandbox TTL 为 2400 秒，systemd 最大运行窗口 3600 秒，`KillMode=mixed`、停止清理宽限 600 秒。

ECS 准备目录 `/root/aries-setup/workload-replay-cloud-1` 保存源 driver receipt、静态 diff、完整启动参数与审查脚本。`validation.py` 原字节复用，SHA `8b3bea0af741bbf40c2461f04126de242b63a3cff3849cf97c6e20dcea06256d`。新 driver SHA `6ee922365f7e0f43d4dc22fc821eef41cc928be15e41cbdbf796ab49cb46d5cc`；独立差异审查器 SHA `ae7d5e626d7bd9c2a480de7b8f167e0c17cf77fa13f95272631ab522842ef3cd`。

启动前只读核对原真实 transcript 的 40 个 ToolResult 与 Tape 中 820 处历史工具正文，UTF-8 字节数及 SHA 全部相等。工具 ID 仅按已支持的 `openclaw-2026.7.1` 子集映射：长度 1–40，ASCII 字母数字或下划线/连字符，仅移除下划线和连字符；未知形状或映射碰撞拒绝，不按相似内容配对。另有四项合成审查通过：历史差异核对、篡改边界 hash 拒绝、遗漏 Snapshot 差异拒绝、原评分非 3/3 拒绝。这些审查自身不创建 Sandbox、不调用模型。

## 原 Strict 失败与本页的区别

同一 SQLite v3 原 Tape 的 Strict 自回放，在第 25 个请求因安装日志中的三个真实时间戳不同停止。公开附录已建议把成功的冗长日志完整保存在工作区，但该次模型决策随后又读取日志尾部，将时间戳带回历史。不能改写旧 Tape、替换 ToolResult 或放宽 Strict 来消除这次失败。

原 exec 22中的make install实际返回2，权限错误与退出状态仍在日志正文；分号后的tail成功，使整个shell/ToolResult退出为0。这是原真实命令的结果构造，不能据0称安装成功；Workload逐终态一致与原Verifier通过仍按完整轨迹分别验收。[精确根因](sqlite-v3-mtime-rootcause-20261026.md)说明安装脚本自身的ls输出及日期显示候选。

本页显式使用独立 Workload 模式，在保留真实正文和原评分的前提下继续执行既定决策。结果只支持此固定工作量的功能与性能验收；不是新自主解题成功、Strict 可重复性、容量或云厂商稳定排名。

## 首轮实际暴露的系统问题

`/opt/aries-artifacts/tb-workload-sqlite-v3-e2b-replay-1` 使用 release11，完成所有 41 个模型请求，未访问上游模型；边界事件保留 46 处历史工具正文差异。原运行返回 0、评分 3/3，停止撤权、原 cleanup 和 provider 不存在确认均通过。

失败发生在后续共享 Snapshot 出口：`analysis-snapshot.json` 为有效 JSON，20,333,695 字节，integrity=complete、diagnostics=[]；其 SHA 为 `ab212e198924b03ebe08df4e72706fb1ba6d3c6da2cdae57362f4d48486b7949`。`aries-perf --trace --from-snapshot` 返回 1，标准输出为空，标准错误为 `invalid analysis snapshot`。旧 driver 随后对空输出解析 JSON，表现为 `JSONDecodeError`；该异常不能覆盖原业务成功，也不能让整体验收算通过。

生产者在 attributes 中保存了有固定字段顺序的 typed `tool_content_differences`。重载时该字段成为按键排序的通用 map，重新序列化的顺序改变，原 Snapshot 内容身份校验因此拒绝。现有 loader 已对 SDK receipt 恢复类型，但遗漏新增的差异字段。最小修复为重载时恢复该字段类型，沿用原哈希算法、原 Snapshot ID 和未知字段拒绝；新增真正落盘/重载后的双出口测试及 digest 篡改、未知字段拒绝负例。修复进入独立 release12，旧 release11 和本轮证据不变。

首轮 evidence SHA：`f93f8c36dfb6db2280f9d00044e7e6e4543db980142d3b041f5907c62db71697`。失败投影的独立复现保存在 `/opt/aries-artifacts/tb-workload-sqlite-v3-e2b-projection-diagnosis-1`。新 driver 单独归档出口 stderr，并在非零返回时报告明确的 projection 失败，不再以空 JSON 解析错误掩盖原因。

修复前的新 round-trip 回归准确失败，修复后落盘/重载、原整数/SDK receipt 保真及两个篡改负例通过。原失败轮的同一 Snapshot SHA/ID 保持不变，使用修复 loader 重导与 Trace Processor 深对账通过：主时钟 2592 区间、178 flows，导入误差 0 ns；OTLP 2592 spans、176 parents、2 links。4 个未对齐的原生后台区间仍明确排除，coverage=unknown。修复后审查保存在 `/opt/aries-artifacts/workload-snapshot-roundtrip-fix-2`，属于新离线诊断，未改变旧 driver FAIL。

## release12 复测补齐的独立验收

release12 五门禁均通过；evidence SHA `4aae132eae541198a1743b72bcf8078eac39ed9946fba100acc648dfafbe9823`，source manifest SHA `77f2b087bb4328d4866ecb920e3864dabe0f22ed8b91a23fffd0d7e5cc5f4ff6`，`aries-perf` SHA `6c24c1bd81aaeb40834a81e90ca89fa5fb7f9f23486f192e63d3216ac6c64d11`。它在 release11 上只追加 frozen Snapshot 重载修复与回归测试，未修改 Strict 匹配或 Workload 正文策略。

专用复测 driver SHA `6572e99f59cda5c65c8d3b5b508e804edaf01055d895628e82ab2b11e6c7a6d0`，审查器 SHA `e7d80d79355ee18338080de48474ef3768be006382fca48258ee8215d8fcf1a9`。除原工具计数、派发身份、后台协议、原评分及清理外，还按精确原生 ToolCall ID 对比原 record 与实际 replay 的终态、退出码、退出信号和错误标记；accepted exec 读取真实协议 terminal，wait 继续与本轮协议终态独立核对。缺关键字段记 unknown，不能通过；正文不用于猜退出码。

首轮真实原件上的补充对账为 40/40 相同，unknown=0、different=0，原录制 5 个非零退出均保留。改变非零退出码、缺失退出码、改变 wait 退出码的负例均被发现。证据为 `workload-snapshot-roundtrip-fix-2/terminal-outcomes-review-1/evidence.json`；这份补充诊断不能把首轮历史整体验收改为成功。

## 四轮真实运行与性能原件

四轮均完成 41/41 模型轮、38 次 exec、2 次 wait、2 个后台 job；原 Verifier 3/3、reward 1，未访问上游模型，各创建并删除 1 个 Sandbox，provider 独立不存在确认及原 cleanup 通过。终态对账包含原录制的 5 个非零退出，不能把非零统一改成失败，也不能遗漏 Huawei 新增的非零退出。

| 运行 | release | 业务 / 整体验收 | 工具终态相同 | Driver 总用时（秒） | Attempt 用时（秒） | 历史正文差异 |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| E2B replay-1 | 11 | 3/3；Snapshot 重载 FAIL | 40/40（后补独立审查） | 344.475 | 337.656 | 46 |
| E2B replay-2 | 12 | 3/3；完整 PASS | 40/40 | 311.438 | 302.446 | 46 |
| Huawei replay-1 | 12 | 3/3；终态等价 FAIL | 39/40 | 274.887 | 266.629 | 152 |
| Huawei replay-2 | 12 | 3/3；完整 PASS（公开 profile 对齐） | 40/40 | 269.697 | 261.582 | 152 |

Driver 用时取其 started_at 至 finished_at，包含部署和事后审查；Attempt 用时直接取原运行记录，不含排队和接纳后的派发延迟。它们与八阶段简单相加的范围不同，不混为 Agent 运行时间。Verifier 包含自身准备、原测试脚本执行及报告读取；pytest等内部耗时已经包含其中，不能再加一次。Huawei 失败轮仅用于诊断，不能拿其较短用时作为已验收的等工作量加速结论。所有数字为单次样本，不代表容量或稳定排名。

八阶段名称与边界沿用[性能阶段说明](performance-stages.md)，单位为秒，保留原纳秒数据于摘要；排队时间虽很小仍单独保留。Runtime 执行直接读取 `attempts[].timing.runtime_execution_duration`，未使用更宽的 Trace `agent.run` 区间。

| 阶段 | E2B replay-1 | E2B replay-2 | Huawei replay-1 | Huawei replay-2 |
| --- | ---: | ---: | ---: | ---: |
| 排队 | 0.000010982 | 0.000010272 | 0.000014824 | 0.000012760 |
| 任务 Sandbox 创建 | 1.023806 | 1.453239 | 0.699887 | 0.570550 |
| 工作区准备 | 2.011302 | 0.624430 | 1.103922 | 0.474410 |
| Harness 启动 | 7.438772 | 7.142290 | 7.362674 | 7.284710 |
| Runtime 执行 | 302.262324 | 270.898284 | 232.407064 | 226.916525 |
| 停止与撤权 | 5.588676 | 5.542315 | 5.367956 | 5.406556 |
| 独立验证 | 16.129998 | 14.923538 | 17.889769 | 19.315534 |
| 清理 | 1.119212 | 0.623685 | 0.661899 | 0.751984 |

38 个 exec 的下表数值均为 `控制端 / guest` 秒，按完整原生 ToolCall ID 精确对应，再展示 exec 序号与模型轮。两列来自各自计时来源，不作跨时钟相减；后台作业的实际终态由原协议与 wait 单独验证。完整 ToolCall ID、execution ID、来源文件与行号、退出码、原始纳秒值保存在下述摘要。未公开工具 shell 正文。

| exec / 模型轮 | E2B replay-1 | E2B replay-2 | Huawei replay-1 | Huawei replay-2 |
| --- | ---: | ---: | ---: | ---: |
| 1 / 1 | 1.311286 / 0.363689 | 0.585730 / 0.203480 | 0.586662 / 0.414938 | 0.225065 / 0.112903 |
| 2 / 2 | 1.074556 / 0.740419 | 1.005585 / 0.740710 | 0.721042 / 0.630201 | 0.585610 / 0.492359 |
| 3 / 3 | 0.346834 / 0.049531 | 0.288257 / 0.041961 | 0.152919 / 0.061737 | 0.143427 / 0.054037 |
| 4 / 4 | 1.021902 / 0.724860 | 0.870473 / 0.627022 | 0.752173 / 0.664584 | 0.794720 / 0.645212 |
| 5 / 5 | 0.377604 / 0.045364 | 0.366981 / 0.044699 | 0.154251 / 0.024190 | 0.143755 / 0.053140 |
| 6 / 6 | 0.342125 / 0.044068 | 0.268305 / 0.030622 | 0.125412 / 0.039714 | 0.137833 / 0.047354 |
| 7 / 7 | 0.315996 / 0.033237 | 0.269983 / 0.027978 | 0.137814 / 0.046715 | 0.129540 / 0.041269 |
| 8 / 8 | 0.317471 / 0.033831 | 0.292801 / 0.039323 | 0.144410 / 0.048833 | 0.153492 / 0.046444 |
| 9 / 9 | 0.342235 / 0.045860 | 0.281730 / 0.032189 | 0.151254 / 0.032026 | 0.135961 / 0.047488 |
| 10 / 10 | 0.324020 / 0.042669 | 0.283531 / 0.038642 | 0.153422 / 0.044139 | 0.148746 / 0.050164 |
| 11 / 11 | 3.900131 / 3.608531 | 3.209833 / 2.970696 | 5.386510 / 5.299804 | 1.668908 / 1.576320 |
| 12 / 12 | 0.319613 / 0.037399 | 0.275850 / 0.029806 | 0.133334 / 0.039643 | 0.129716 / 0.041239 |
| 13 / 13 | 102.063220 / 101.766577 | 87.028741 / 86.786072 | 63.489371 / 63.390895 | 62.547320 / 62.452717 |
| 14 / 15 | 0.315106 / 0.032851 | 0.285681 / 0.037871 | 0.153139 / 0.045362 | 0.150827 / 0.049729 |
| 15 / 16 | 0.316553 / 0.037356 | 0.269797 / 0.031954 | 0.141897 / 0.054600 | 0.135880 / 0.044574 |
| 16 / 17 | 0.493240 / 0.042201 | 0.266541 / 0.029585 | 0.134954 / 0.041788 | 0.131767 / 0.041052 |
| 17 / 18 | 0.461671 / 0.052127 | 0.275115 / 0.035663 | 0.138246 / 0.050849 | 0.141398 / 0.052505 |
| 18 / 19 | 0.331632 / 0.040174 | 0.275472 / 0.035326 | 0.153697 / 0.050308 | 0.133939 / 0.031562 |
| 19 / 20 | 0.832985 / 0.351978 | 0.600389 / 0.199330 | 0.136848 / 0.045847 | 0.127352 / 0.038850 |
| 20 / 21 | 0.318150 / 0.036920 | 0.264123 / 0.028738 | 0.151918 / 0.047376 | 0.152511 / 0.041404 |
| 21 / 22 | 1.535069 / 1.208840 | 1.175643 / 0.941412 | 0.974588 / 0.887026 | 0.944442 / 0.853839 |
| 22 / 23 | 43.971675 / 43.685523 | 35.291669 / 35.047979 | 26.421086 / 26.327144 | 26.346339 / 26.250029 |
| 23 / 25 | 0.365990 / 0.049647 | 0.367147 / 0.029774 | 0.147597 / 0.046237 | 0.151074 / 0.047889 |
| 24 / 26 | 0.334513 / 0.038427 | 0.277686 / 0.033922 | 0.314771 / 0.222086 | 0.283946 / 0.183887 |
| 25 / 27 | 0.349377 / 0.050794 | 0.277361 / 0.040533 | 0.146546 / 0.050719 | 0.156958 / 0.051114 |
| 26 / 28 | 0.348939 / 0.041544 | 0.269915 / 0.032821 | 0.134880 / 0.043315 | 0.134442 / 0.040827 |
| 27 / 29 | 0.550454 / 0.250649 | 0.464080 / 0.217704 | 0.487243 / 0.395052 | 0.341133 / 0.248945 |
| 28 / 30 | 0.333206 / 0.043255 | 0.265845 / 0.030770 | 0.142504 / 0.050210 | 0.134057 / 0.040711 |
| 29 / 31 | 0.339870 / 0.037994 | 0.264384 / 0.026353 | 0.154259 / 0.043275 | 0.129821 / 0.039378 |
| 30 / 32 | 0.326093 / 0.041326 | 0.276288 / 0.033221 | 0.211917 / 0.119763 | 0.151832 / 0.053755 |
| 31 / 33 | 0.340144 / 0.051933 | 0.274193 / 0.033321 | 0.142714 / 0.047340 | 0.148489 / 0.041801 |
| 32 / 34 | 0.431697 / 0.119890 | 0.343219 / 0.100156 | 0.161895 / 0.045594 | 0.152787 / 0.046144 |
| 33 / 35 | 0.339166 / 0.035976 | 0.285164 / 0.030609 | 0.133666 / 0.039602 | 0.116642 / 0.027862 |
| 34 / 36 | 0.408191 / 0.065479 | 0.287572 / 0.030197 | 0.131386 / 0.045346 | 0.139882 / 0.048423 |
| 35 / 37 | 0.321755 / 0.033941 | 0.274273 / 0.034262 | 0.151955 / 0.043304 | 0.130860 / 0.040949 |
| 36 / 38 | 0.343955 / 0.049040 | 0.279784 / 0.038419 | 0.149918 / 0.043371 | 0.153875 / 0.044480 |
| 37 / 39 | 0.332675 / 0.047142 | 0.290475 / 0.042894 | 0.152236 / 0.047005 | 0.140685 / 0.049718 |
| 38 / 40 | 0.329304 / 0.043561 | 0.289943 / 0.044491 | 0.145842 / 0.052300 | 0.148822 / 0.045319 |

原录制、两次 E2B 及新 Huawei replay-2 的非零退出为 exec 2、18、25、30 的 exit 1 和 exec 34 的 exit 127。Huawei 首轮另有 exec 29 / 模型轮 31 / `call_00_J6oaweikPdcyxkm70mmb5907` 的 exit 1，原录制为 exit 0；该差异保留为整体验收失败，其余 39 个结果相同、unknown=0。

四轮统一摘要位于 ECS `/opt/aries-artifacts/workload-sqlite-performance-archive-2/summary.json`，SHA `d882837441fb4a0269dd6663bb9d8f1d7a0d6254f5e5badcd288280fef0a7dc7`；其中逐项保存原 evidence、run-result、评分、原生 transcript、操作记录及 Snapshot 的 SHA。原件根目录分别为 `/opt/aries-artifacts/tb-workload-sqlite-v3-e2b-replay-1`、`…-e2b-replay-2`、`…-huawei-replay-1`、`…-huawei-replay-2`；此前三轮 archive-1 摘要仍保留。

E2B 第二轮 evidence SHA `73b50096aa6e316a8588b496f4cf7c9b6639788949b233ec6e08a7f7f7407023`；独立完整验收收据位于其 `performance-review-1/acceptance.json`。双 Trace 对账为 2568 主时钟区间、178 flows，OTLP 2568 spans、176 parents、2 links，导入误差 0 ns；TP 收据 SHA `51cd60de1d059d666eaa84a19d739831a0387887cae0e3f650655891df69343e`。

Huawei 首轮 evidence SHA `68fc871d981027d6d3bfe4e29cf745fb8f05b9f7c0bb242208f6403c219c2c5d`。其原审查在终态不等价处拒绝；后续独立诊断在 `performance-review-1` 完整核对 152 处历史正文差异的原始 UTF-8 字节/hash，同时保留 `FAIL_TERMINAL_EQUIVALENCE_WITH_COMPLETED_DIFFERENCE_HASH_AUDIT`，未改旧 driver 或旧结果。诊断 SHA `99e112fee9c02947af2cdd505feb12de28062a2b5febf891941e80023eb3ad1d`；双 Trace 为 2628 主时钟区间、178 flows，OTLP 2628 spans、176 parents、2 links，误差 0 ns；TP 收据 SHA `4bf955f22f71260f7935c95a6f137f8b11838bb0ef77993634ad313c0bd3e9a7`。每轮双 Trace 均排除 4 个无法对齐的原生后台区间，coverage=unknown，不声称这些区间已获主时钟覆盖。

## Huawei 公开 shell 环境对齐与同 Tape 复测

唯一终态差异来自实际读取 `/etc/profile.d/shell.sh`：E2B 原录制可读到公开配置 `export SHELL='/bin/bash'`，Huawei 返回文件不存在。该配置行含末尾换行的 SHA 为 `0a43d87c557a6b3111daea9159e3e5f5976ccdee4a6cd6d3e291ea4d5c35e4be`。无网络只读探针确认原任务镜像 `f986820b…` 和旧 Huawei 派生镜像 `dbb6a43f…` 都没有此文件，二者均有 `/bin/bash`；已审查 E2B 准备脚本未创建此文件。证据支持运行环境附加配置差异，上游具体注入实现尚未定位，不据此宣称产品系统 bug。

新 Huawei 镜像直接继承原已固定派生镜像，只以两行 Dockerfile 增加 root:root、0644 的这份真实配置文件。镜像 Config 完全相同，原层完整保留，仅新增一个 COPY 层；没有重新安装依赖、修改任务、Tape、工具参数/正文或原评分。只读探针核对文件 hash/权限、原 fixture，并验证直接非登录 `/bin/sh -c` 不自动加载它，显式 source 后 SHELL 为 `/bin/bash`。现有桥接器固定使用 `/bin/sh -c`，因此该适配不会把 Agent 命令解释器改为 Bash；登录或显式加载 profile 的程序可以观察到这项环境声明。

新镜像 digest `sha256:14059041f9c15b642d15deabf8f8183ce9b6216bd234b6b27de50911aa454be5`；image evidence 为 `/opt/aries-artifacts/tb-huawei-sqlite-with-gcov-image-profile-1/evidence.json`，SHA `c56cb734a44df753311961701686e1a735e51c487326c495cf57d270cabb86f4`。新正式模板固定 1 CPU / 2048 MiB，保留供同 Tape 复测；binding 为 `/opt/aries-artifacts/tb-huawei-sqlite-with-gcov-template-profile-1/binding.json`，SHA `89a4fe2962aef8b99e8de01d08f09e0b1dc5c9594afa09531317231d915752d2`，build SHA `0da569eed0849f20ca18db4db4584e7a478747dcac6cc28210acc1d4d2094d76`，template evidence SHA `f0438237750c22d6c9735a81131789310f66ea22d5c348860b64296d12dde2f1`。

本次镜像/模板准备未创建云 Sandbox、未调用模型，临时 Docker 探针和认证副本均已删除，cleanup=[]。随后 Huawei replay-2 使用该正式模板、原 Tape 和 release12，完成单个新 Sandbox 的真实运行；原评分 3/3、40 个精确工具终态全部等价、152 处历史正文差异审查、身份、清理及双 Trace 对账均通过。旧失败和旧镜像/binding 全部保留，正式新模板及发布证据继续保留供复用。


新 Huawei 原 run 为 `20261026T110345.744172232Z-tb-sqlite-with-gcov-terminal-result-v3-workload-replay`，evidence SHA `06ee1cefa5f9fdac5695a97168a6a4cea0766d811662d81d03883f84d3dd2815`，正文差异/终态审查 SHA `cfbdb89dfc131697905b80a9e8b5d65f7ce7cf6a2336f76e0ae294769a6de46d`。完整验收收据位于该轮 `performance-review-1/acceptance.json`，状态 `PASS_COMPLETE_WORKLOAD_ACCEPTANCE`；共享入口释放证明位于 `release-receipt.json`，确认 systemd inactive/dead、MainPID=0，部署锁可获取后释放，8082 无 listener，本轮 Adapter 容器为空，provider/ledger/runner process group 均不存在。

新 Huawei 的 Snapshot SHA `87ae2a127bf8777de2018ce001749389f5c1846a10ec68f225dfaa54c4fff6ed`；主 Trace SHA `ba8cc4ebe9072fe1990e696525a78c30b0ba0eef7ce67e8efb90cf960daa7365`，OTLP SHA `e443a7ba6eed340fb198ad661d327d0d2a6448fbff4400beacad66b02dc4e5b2`。Trace Processor 独立对账为 2628 主区间、178 flows，OTLP 2628 spans、176 parents、2 links，起止导入误差均为 0 ns；TP evidence SHA `695a51bc4b9cf83dbf0a3ffa58063c36fb767059d60918416c015f9ece3b5bae`。仍有 4 个原生后台区间因时钟未对齐而排除，coverage=unknown。

这对通过验收的单次样本中，E2B / Huawei 的 Runtime 执行为 270.898284 / 226.916525 秒，Attempt 为 302.446432 / 261.582241 秒；其中环境差异和缓存状态均按实际固定输入保留。该结果只描述本次已固定 Tape 的工作量执行，不将单次耗时差解释为云厂商的普遍性能优势，也不替代 Strict 可重复性验收。
