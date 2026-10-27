# C++ v2：E2B / Huawei 同 Tape Workload 回放
表中“控制端”沿用原 `sdk_wait_duration_ns` / `exec.wait` 字段，实际从ARIES客户端提交进程前计到Wait返回，含提交、执行、输出传输与排空；不是单独Wait方法或官方SDK内部耗时。guest列是其独立wrapper窗口，两者不相减推断网络时间。


日期：2026-10-26。**同一合格 Tape 的跨云工作量功能通过**：Huawei 与 E2B 各完成 **35/35 模型轮、34/34 精确工具终态、原 Verifier 6/6、0 上游模型请求**，各自 Sandbox 均已删除。两轮整体观测均为 **incomplete**：后台协议未触达，producer 原件未收集。功能通过、Strict 失败和观测完整性分别记录。

E2B 本轮实际接受 **3600 秒** Sandbox 存活期，driver 全流程 **288.327765 秒**完成。因此，旧 4200 秒创建请求被拒不等于本工作量无法在一小时内完成。原 Agent / Verifier 仍为 **1800 / 1800 秒**，它们与准备、清理共享 3600 秒；本次成功不保证两个阶段都耗尽预算时仍有完整窗口。

同 Tape 的历史 Huawei Strict 在 GDB 两行通知顺序处失败；本次 E2B Strict 则在第 6 请求的 GDB backtrace 地址处失败，仅执行 5/34 工具。随后这份独立 Workload 是新的 Attempt，不回写前一轮 Strict。完整分母与首差异见 [Strict 结果与边界](remaining-strict-cases-20261026.md)。

## 固定输入与两云实际差异

原 Tape：`/opt/aries-artifacts/tb-repro-cpp-v2-huawei-record-1/runs/20261026T061724.567557955Z-tb-custom-memory-heap-crash-terminal-result-v2-record/custom-memory-heap-crash-terminal-result-v2-001/model/tape.json`；SHA `ea1a8d1b0f1147e3108b7fd21a965c890d07f2e206e8db6de770d6b686a867df`。原录制 6/6，evidence SHA `b6c52d186cb1709cacc571a77d5cc57c71a4c4ca87e8afe2a603b8d06fc88423`。

| 口径 | Huawei | E2B |
| --- | --- | --- |
| 控制器 / native | release12 / e040 | 同一 release12 / e040 |
| Agent / Verifier 预算 | 1800 / 1800 秒 | 1800 / 1800 秒 |
| Sandbox TTL | 4200 秒 | 3600 秒，共享平台生命周期实验 |
| 原请求资源 | 1 CPU / 2048 MiB | 1 CPU / 2048 MiB |
| 原题 / v2 附录 / source image | 原件保持 | 同一原件保持 |
| 原评分 | 6/6 | 6/6 |
| 原生工具终态 | 34/34 相等，unknown 0 | 34/34 相等，unknown 0 |
| 工具正文差异 | 0，允许差异分支未触达 | 8 个 ToolResult；在重复历史中计 128 处，分支实际触达 |
| Verifier 依赖 | 已声明的本地 uv/Python/pytest 缓存 | 原网络安装流程，无 Huawei 专用缓存 URL |
| 执行 UTC | 11:30:53–11:35:29 | 12:37:52–12:42:40 |
| 整体观测 | incomplete | incomplete |

资源配置保留原请求值，不把配置等同已测得的执行隔离。E2B binding 从原已准备 template 派生，初始根目录 mtime 取原 v2 录制前显式声明值；未修改任务 fixture、私有评分、工具正文或进程时钟。Verifier 缓存差异和 ECS 同期离线分析负载均保留披露；以下是固定工作量的实际功能与性能记录，不作厂商稳态排名。

native 均为 `e040654efde8adde910583291b547fb9d5fc94e565b2f4dc8bad29abcc423041`。所有模型决策、工具名称/参数/顺序及非工具历史固定，只允许相应文本 ToolResult 正文不同。原会话有 34 个同步 exec，无 process 或其他工具；33 个 exit 0、1 个 exit 1。按精确 ToolCall ID 比对原生 isError、status、exitCode、exitSignal、renderer、truncation 及实际存在的可选终态字段，缺字段不补默认值，不从正文猜退出码。

公共 validation、C++ 原终态审查和原 6 项私测保持。E2B 专用 driver 仅将 provider/output 定为新实验，并显式选择共享 TTL3600；未修改公共 driver 的默认值或核心代码。原 C++ 的唯一未触达协议 producer 缺口仍由原专用审查保留为 incomplete，未放宽 SQLite 或全局规则。[Huawei 准备及负例审查](../.cache/cpp-workload-cloud-1/preparation-receipt.json)、[E2B 最小 diff](../.cache/cpp-e2b-3600-workload-1/driver.patch)和[冻结输入回执](../.cache/cpp-e2b-3600-workload-1/ready-receipt.json)可复核。

## 八阶段与清理

| 阶段 | Huawei 秒 | E2B 秒 |
| --- | ---: | ---: |
| 排队 | 0.000012822 | 0.000023279 |
| 任务 Sandbox 创建 | 0.715009647 | 1.637675012 |
| 工作区准备 | 0.998069980 | 0.854633943 |
| Harness 启动 | 7.437931087 | 7.415565135 |
| Runtime 执行 | 219.503711515 | 241.340027811 |
| 停止与撤权 | 5.351390946 | 5.618643792 |
| 独立验证 | 27.965175750 | 16.708273230 |
| 清理 | 0.610799295 | 0.747269754 |

两轮各自只创建 1 个 Sandbox；Harness 停止、Bridge 撤权后在原 Sandbox 执行原独立评分，cleanup succeeded，provider / ledger / runner 进程组均 absent，`cleanup_errors=[]`。E2B 于 12:44:23 UTC 确认部署锁可获取释放、8082 无监听、专属 adapter 为空；systemd wait 已以 0 结束。启动显式请求 `KillMode=mixed`，成功后的 transient unit 已被 systemd 回收，最终 `LoadState=not-found` 的默认属性不能反推启动配置。两次均无自动重试。

## 逐 exec 实际读数

SDK 为客户端 `exec.wait` 观测，guest 为 guest 回执耗时，分别使用自身时钟，不能跨时钟相减。两云 34 个 ToolCall ID 逐项一致，原有 exit 1 同样保留；表中三方终态经独立审查相等。完整 ID、原纳秒值与来源行号在各自摘要，未公开 shell 正文。

| exec | 模型轮 | Huawei SDK 秒 | E2B SDK 秒 | Huawei guest 秒 | E2B guest 秒 | 原 / HW / E2B exit |
| ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 1 | 1 | 0.200600 | 0.627929 | 0.049155 | 0.171895 | 0 / 0 / 0 |
| 2 | 2 | 0.141661 | 0.279361 | 0.047549 | 0.031282 | 0 / 0 / 0 |
| 3 | 3 | 1.880074 | 7.962401 | 1.787780 | 7.719985 | 0 / 0 / 0 |
| 4 | 4 | 0.495168 | 0.546528 | 0.362405 | 0.306032 | 0 / 0 / 0 |
| 5 | 5 | 0.643135 | 3.265637 | 0.554019 | 3.020795 | 0 / 0 / 0 |
| 6 | 6 | 0.143032 | 0.555785 | 0.052080 | 0.313218 | 0 / 0 / 0 |
| 7 | 7 | 0.197082 | 0.333960 | 0.044523 | 0.042811 | 0 / 0 / 0 |
| 8 | 8 | 0.130701 | 0.284493 | 0.038512 | 0.028563 | 0 / 0 / 0 |
| 9 | 9 | 0.276498 | 1.085762 | 0.188599 | 0.842813 | 0 / 0 / 0 |
| 10 | 10 | 0.128957 | 0.267656 | 0.041440 | 0.026701 | 0 / 0 / 0 |
| 11 | 11 | 0.134345 | 0.359241 | 0.042464 | 0.029557 | 0 / 0 / 0 |
| 12 | 12 | 0.133267 | 0.273617 | 0.041871 | 0.029945 | 0 / 0 / 0 |
| 13 | 13 | 0.777959 | 0.951972 | 0.649063 | 0.714892 | 0 / 0 / 0 |
| 14 | 14 | 0.365315 | 0.540421 | 0.273309 | 0.297728 | 0 / 0 / 0 |
| 15 | 15 | 0.398220 | 0.665120 | 0.306639 | 0.400356 | 0 / 0 / 0 |
| 16 | 16 | 0.456976 | 0.623445 | 0.315016 | 0.380370 | 0 / 0 / 0 |
| 17 | 17 | 0.128290 | 0.263054 | 0.040999 | 0.025291 | 0 / 0 / 0 |
| 18 | 18 | 0.403327 | 0.594433 | 0.314727 | 0.359246 | 0 / 0 / 0 |
| 19 | 19 | 0.579408 | 0.832485 | 0.486608 | 0.589935 | 0 / 0 / 0 |
| 20 | 20 | 0.353367 | 0.549874 | 0.260697 | 0.303698 | 1 / 1 / 1 |
| 21 | 21 | 0.352817 | 0.711812 | 0.261024 | 0.307496 | 0 / 0 / 0 |
| 22 | 22 | 0.320398 | 0.565730 | 0.231816 | 0.304949 | 0 / 0 / 0 |
| 23 | 23 | 0.344092 | 0.700137 | 0.252266 | 0.293152 | 0 / 0 / 0 |
| 24 | 24 | 0.139589 | 0.289487 | 0.047952 | 0.035177 | 0 / 0 / 0 |
| 25 | 25 | 0.361223 | 1.070257 | 0.269658 | 0.327003 | 0 / 0 / 0 |
| 26 | 26 | 0.395772 | 0.596032 | 0.303305 | 0.353960 | 0 / 0 / 0 |
| 27 | 27 | 0.414422 | 0.645345 | 0.325838 | 0.400180 | 0 / 0 / 0 |
| 28 | 28 | 0.842562 | 1.070400 | 0.642123 | 0.822656 | 0 / 0 / 0 |
| 29 | 29 | 1.074888 | 1.187856 | 0.928716 | 0.945670 | 0 / 0 / 0 |
| 30 | 30 | 0.817949 | 1.621533 | 0.725819 | 1.374673 | 0 / 0 / 0 |
| 31 | 31 | 0.800736 | 1.274899 | 0.711791 | 1.035999 | 0 / 0 / 0 |
| 32 | 32 | 3.295829 | 4.481495 | 3.203591 | 4.236697 | 0 / 0 / 0 |
| 33 | 33 | 1.598768 | 1.937164 | 1.507018 | 1.688570 | 0 / 0 / 0 |
| 34 | 34 | 0.135045 | 0.265210 | 0.047523 | 0.025547 | 0 / 0 / 0 |

## 双 Trace 和小证据

| 同 Snapshot 对账 | Huawei | E2B |
| --- | ---: | ---: |
| 检查项 | 18,317 | 18,417 |
| Perfetto intervals / OTLP spans | 4,566 / 4,566 | 4,591 / 4,591 |
| Perfetto flows / OTLP parents | 152 / 152 | 152 / 152 |
| links / 排除时钟域 | 0 / 0 | 0 / 0 |
| 导入 / 结束时间误差 | 0 / 0 ns | 0 / 0 ns |

两轮均仅通过 `PASS_PROJECTION_OF_INCOMPLETE_SOURCE`：`terminal-result-v1` 为 `NOT_EXERCISED`，accepted jobs / wait calls 均为 0，index 为 `not_collected / producer_artifact_missing`。未新增后端或浏览器验收，未把未触达协议计作通过，也未将缺失值填零。

Huawei 原根目录 `/opt/aries-artifacts/tb-workload-cpp-v2-huawei-replay-1`，evidence SHA `74a804cd9eead65825941758a98f8c192bc0aafcc6b09e70f51566897229ddf7`。其 review 目录双投影 evidence SHA `4c0506f799b687f875a1540a994080a34ad31ee1baef69c7a39e9538006d3385`；[分项验收](../.cache/cpp-workload-cloud-1/performance-receipts/acceptance.json)、[八阶段及 exec 摘要](../.cache/cpp-workload-cloud-1/performance-receipts/summary.json)、[窗口释放](../.cache/cpp-workload-cloud-1/performance-receipts/release-receipt.json)。

E2B 原根目录 `/opt/aries-artifacts/tb-workload-cpp-v2-e2b-3600-replay-1`，evidence SHA `087c479ec66d35dd68f5bf7709223e2bd5c03d75eee4bc39da25520b4cc1912b`。其 review 目录双投影 evidence SHA `f707031c2d0810273875a1e6cdb4ef10b8724450057433785822f431b5797ebe`；[分项验收](../.cache/cpp-e2b-3600-workload-1/receipts/acceptance.json) SHA `c552f4adcfa2b595bfa41e1e3e580fa10972846d29db1e830530e2eab6013464`、[八阶段及 exec 摘要](../.cache/cpp-e2b-3600-workload-1/receipts/performance-review-1/summary.json)、[128 处历史正文哈希与终态审查](../.cache/cpp-e2b-3600-workload-1/receipts/workload-differences.json)、[窗口释放](../.cache/cpp-e2b-3600-workload-1/receipts/release-receipt.json)。该审查只含来源哈希和结构化差异，未复制原 Tape 或私有工具正文。
