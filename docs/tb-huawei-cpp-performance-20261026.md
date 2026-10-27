# 华为云 C++ 任务：准备与性能归档（2026-10-26）

**第二轮真实录制已通过：原独立 Verifier 6/6，reward=1，所属 1 个 Sandbox 已确认删除。** `record-2` 使用 image-2 / template-2，真实 Agent Runtime **334.513 秒**，**63 个完成模型轮、64 次真实 exec**；原独立验证 **18.794 秒**。它不是公开依赖 smoke，也不是参考解执行。这份新 Tape 的首轮 Strict Replay 已执行并 **FAIL**：首工具读取根目录 mtime 发生偏离，原评分为 5/6；不计作完整回放或跨云成功配对。

首轮 `record-1` 的失败原件保留：Agent 完成（201.730 秒、41 轮、39 次 exec 和 1 次其他工具调用），原独立评分在依赖下载后发生 process stream 失败（1665.226 秒），没有原始 CTRF / reward，不能写成「0/6」。首轮的 1 个 Sandbox 也已确认删除。公开依赖缓存准备与原评分实测在本文分别列出。

首轮 **02:38 UTC** 的只读诊断曾观察到 `apt-get` 已安装 curl，最后一行是 `downloading uv 0.9.5 x86_64-unknown-linux-gnu`；当时尚无 pytest 或进程终态。它读取现有 Adapter 的 76 条缓存事件，没有新增 guest 进程、调用第二次 SDK wait 或修改任务，原件保存在 `/opt/aries-artifacts/tb-huawei-cpp-record-1-readonly-diagnostic-1/`。该记录用于说明当时定位到的依赖路径，不是 uv 下载的独立耗时，也不覆盖后续终态证据。

本任务是原 Terminal-Bench `custom-memory-heap-crash`：Agent 自主分析并修复小型 C++ 项目，保留原题与原私有测试。华为采用 **A（External）**：Harness / OpenClaw 在 ECS，工具在华为 Sandbox 中执行。[原任务材料与验证边界](../cases/sandbox-runtime-v1/agent-cases/custom-memory-heap-crash/README.md)和[六题原实验记录](tb-six-performance-20261026.md)分别保留。

## 已完成哪些准备

下面全部来自 ECS 归档证据。时间单位为秒；`未采集` 表示没有该项独立计时，不能解释为零耗时。

| 准备项目 | 状态 | 耗时（秒） | 结果与边界 |
| --- | --- | --- | --- |
| 编译 `make build` | PASS | 14.216 | 生成 release5 二进制 |
| 单元测试 `make test` | PASS | 28.389 | 包含华为 TB 接线、provider 绑定及原预算约束；不代表真实云验收 |
| 竞态检查 `make test-race` | PASS | 74.773 | 工程检查 |
| 静态检查 `make lint` | PASS | 3.442 | 工程检查 |
| 集成测试 `make integration` | PASS | 381.044 | ECS 集成检查；没有本任务真实 Agent 运行 |
| 原任务镜像适配、检查、归档、推送全流程 | PASS，仅准备 | 1096.486 | 根据开始/结束 UTC 时间计算的整体墙钟窗口，包含构建、检查、压缩与发布；不是单独镜像构建耗时 |
| 镜像内原始文件 smoke 检查 | PASS，仅 ECS Docker | 未采集 | 三个原始 C++ 源文件按固定清单检查；不代表云启动、Agent 修复或原评分通过 |
| release5 pins 与原生镜像复用核验 | PASS，仅准备 | 未采集 | 核验 15 个 `experiments/r01` 输入与 release4 一致，复用 publication3，不重复构建原生镜像 |
| 华为公开模板构建 | PASS，仅准备 | 105.632 | 平台报告 ready、已生成显式 Huawei binding；CPU / 内存配置为 1 CPU / 2048 MiB，准备通过不代表任务通过 |

五项工程门禁分别使用其日志记录的 `duration_s`。镜像流程窗口为 **2026-10-26 01:51:47.399698 UTC → 02:10:03.885545 UTC**；现有回执没有独立记录构建、压缩、上传各项的 duration，因此不拆出估算值。镜像准备、模板构建、版本核验属于实验前准备，不计入下文的「任务 Sandbox 创建」。

release5 冻结 **608 个文件**：在 release4 的 605 个文件上新增 `.agents/profiles.md`、`docs/quick-start.md` 和华为 C++ profile；其他本次修改覆盖既有文件。原基线、候选源码、固定任务 checkout 和既有服务检查通过。该结论是工程验收，不增加 Agent 成功次数。

## 镜像、资源与版本

| 项目 | 本次固定内容 |
| --- | --- |
| 原基准版本 | Terminal-Bench commit `7131e4375048a0e408a8fb404b5f499d726b695b` |
| 原任务镜像 | `alexgshaw/custom-memory-heap-crash:20251031@sha256:1895d7d6240d5fb2ec774d6bfdd1057d170f8d199c15c582e4585d3c63802557` |
| 新任务镜像 | SWR `benchmark-zhoutao/aries-tb-custom-memory-heap-crash:20261026-1`，固定引用见下方 |
| 原任务资源请求 | 1 CPU、2048 MiB 内存、10240 MiB 存储；允许联网 |
| Agent / Verifier 原预算 | 各 1800 秒，不缩短、不覆盖原 `task.toml` |
| Sandbox TTL | 4200 秒；首轮真实创建已接受，所属的 1 个 Sandbox 已确认删除 |
| 模式与并发 | 华为 `huawei-agentsphere`、A / External、单 attempt、并发 1 |
| 工作区与命令环境 | 继承 `/app` 和原绑定的显式 PATH；不增加禁网隔离假设 |
| 原评分方法 | 确认 Harness 停止、Bridge 撤权后，在原任务 Sandbox 注入原私有测试；不是预置评分文件，也不是新建评分 Sandbox |

已发布的不可变镜像引用：

`swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-tb-custom-memory-heap-crash:20261026-1@sha256:1b7411d3f7563d279719d9f507abecc7acc3b350955a135a23a2459c2ff17ea9`

原镜像中的两套 libstdc++ 和原始程序保留；仅沿用已归档的 Python/controller、UID/GID 60000、`/app/bin` 准备配方。没有复制私有测试或参考答案，没有安装任务 Sandbox 内的 OpenClaw。平台注入 ENVD，镜像 smoke 不能证明云中 ENVD、cgroup 或停止隔离能力。

ECS 镜像归档为 `/opt/aries-artifacts/tb-huawei-cpp-image-1/image.tar.gz`，**1,254,772,545 字节，约 1.255 GB（十进制）**；SHA-256 为 `560372b63a4c4b9a876e118bbde39b1e1d6fabe650861842f4d7b019b265ff66`。Docker 报告镜像大小为 5,987,038,556 字节，不等于压缩归档大小或云实例实际磁盘占用。临时 SWR 认证已清除，镜像准备回执没有清理错误。

镜像仓库 index digest 为 `1b7411d3f7563d279719d9f507abecc7acc3b350955a135a23a2459c2ff17ea9`，linux/amd64 manifest 为 `0f44ae543deb36f5738fa9f0130c5e25f6bb903f2e78a2174ba32edbdbf0f51e`，config digest 为 `926f4bbeaba1662bc661baa83b507c3b5cf848e3af0ff1e6dfc9750231b3ec2b`。这些身份分别保留，不混用为一种镜像哈希。

## 评分依赖修复准备：image-2 / template-2

首轮原评分下载依赖失败后，在 `image-1` 上增加小层预置 **curl、uv 0.9.5 官方压缩包、CPython 3.13.9、pytest 8.4.1 和 pytest-json-ctrf 0.3.5 的缓存**。原始三份 C++ 文件仍通过校验；原 `test.sh` 没有改写，镜像没有放入私有测试或参考解。

uv 与 Python/pytest 缓存放在原生 root 目录，Agent UID 60000 无权读取这些 root 缓存；绑定只增加 `UV_DOWNLOAD_URL=file:///opt/aries-verifier-deps/uv/0.9.5`，让原官方 installer 使用已归档的官方资产。没有改写全局 `UV_CACHE_DIR` / `UV_PYTHON_INSTALL_DIR` 或 Agent 工作目录。

| 后续准备项目 | 已验证结果 | 耗时 / 身份 |
| --- | --- | --- |
| image-2 准备、归档与发布 | PASS，仅准备 | 02:59:31.200465 → 03:06:22.141007 UTC，整体窗口 410.941 秒；不拆成构建或上传耗时 |
| 原文件检查 | PASS，仅 ECS Docker | 原三个 C++ 文件仍与固定清单一致 |
| 评分公开依赖 smoke | PASS，公开 1/1 | `--network none` 下安装官方 uv、生成环境文件，再以原 uvx 版本参数执行公开 smoke；不是原 Verifier 的 6 项测试 |
| template-2 构建 | PASS，仅准备 | 93.294 秒，平台 ready，模板保留供真实复测 |
| 模型调用 / Sandbox 创建 | 均为 0 | 以上准备步骤未运行 Agent，也没有产生新的任务成功样本 |
| 准备资源清理 | PASS | smoke 容器与临时 SWR 认证已清除，准备回执没有清理错误 |

image-2 的不可变引用：

`swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-tb-custom-memory-heap-crash:20261026-2@sha256:8b9876c4520f40a29bec79b4986318b983119ddecc9b71f15dfbe08e4e8af5ef`

新归档 `/opt/aries-artifacts/tb-huawei-cpp-image-2/image.tar.gz` 为 **1,396,021,567 字节**，SHA-256 为 `114de3e5379f50a7c699c3088d270eb01cf03704f61c63f65221fb2a45b7f760`。旧镜像、旧模板及首轮录制证据全部保留。template-2 的新 binding SHA-256 为 `3c33cd733aee3132f88e0d460b2971c0d74639d534e8d775801e5a97bf0d0836`；准备时的 `runtime_validation` 为 UNRUN，后续真实复测须按自己的终态证据统计。

## 第二轮真实录制结果

以下为新建 Sandbox 中的真实 DeepSeek 录制，与首轮不是同一轨迹。原题、原私有 Verifier 和评分方法不变；先确认停止 / 撤权，再独立评分。CPU / 内存仍为 1 CPU / 2048 MiB。模型轮 3 和 59 各有两次 exec，已在真实账本中保留；这不是只允许单工具的合成协议测试。

| 阶段 | record-2 耗时（秒） |
| --- | --- |
| 排队 | 0.000010 |
| 任务 Sandbox 创建 | 0.615656 |
| 工作区准备 | 0.981503 |
| Harness 启动 | 7.092484 |
| Runtime 执行 | 334.513245 |
| 停止与撤权 | 5.384577 |
| 独立验证 | 18.794267 |
| 清理 | 0.689828 |

统一 Snapshot 为 `integrity=complete`、`coverage=unknown`；Perfetto **6788 个区间 / 327 条 flow**，离线 OTLP **6788 个 span / 327 个 parent**，导入与结束时间最大误差均 0 ns、排除区间 0。只证明现有观测范围内对账；Jaeger 发送/查询为 UNRUN，本次没有浏览器验收。

64 次 exec 均有精确身份与 SDK / guest 时长；第 9 次原生退出码为 2，后续 Agent 修复并通过原评分。保留真实失败命令，不把最终成功解释为每条命令都成功。

<details>
<summary>record-2 的 64 次 exec 明细（毫秒）</summary>

| exec | 模型轮 | SDK 等待 ms | guest 整体 ms | exit_code |
| --- | --- | --- | --- | --- |
| 1 | 1 | 281.054 | 115.067 | 0 |
| 2 | 2 | 104.591 | 19.279 | 0 |
| 3 | 3 | 3056.648 | 2900.148 | 0 |
| 4 | 3 | 3468.022 | 3311.211 | 0 |
| 5 | 4 | 1018.117 | 924.170 | 0 |
| 6 | 5 | 1450.376 | 1352.856 | 0 |
| 7 | 6 | 146.316 | 52.303 | 0 |
| 8 | 7 | 169.029 | 49.381 | 0 |
| 9 | 8 | 228.585 | 130.364 | 2 |
| 10 | 9 | 156.319 | 62.570 | 0 |
| 11 | 10 | 164.004 | 69.600 | 0 |
| 12 | 11 | 1148.183 | 1050.149 | 0 |
| 13 | 12 | 143.047 | 45.010 | 0 |
| 14 | 13 | 764.926 | 626.107 | 0 |
| 15 | 14 | 139.642 | 45.212 | 0 |
| 16 | 15 | 380.125 | 286.445 | 0 |
| 17 | 16 | 203.342 | 107.847 | 0 |
| 18 | 17 | 349.013 | 255.078 | 0 |
| 19 | 18 | 367.492 | 270.911 | 0 |
| 20 | 19 | 377.049 | 282.203 | 0 |
| 21 | 20 | 156.797 | 46.424 | 0 |
| 22 | 21 | 137.278 | 45.354 | 0 |
| 23 | 22 | 390.666 | 294.154 | 0 |
| 24 | 23 | 281.967 | 188.231 | 0 |
| 25 | 24 | 297.019 | 201.093 | 0 |
| 26 | 25 | 271.339 | 175.707 | 0 |
| 27 | 26 | 339.550 | 243.185 | 0 |
| 28 | 27 | 337.747 | 244.067 | 0 |
| 29 | 28 | 346.829 | 250.498 | 0 |
| 30 | 29 | 331.172 | 237.251 | 0 |
| 31 | 30 | 348.756 | 252.473 | 0 |
| 32 | 31 | 152.216 | 46.378 | 0 |
| 33 | 32 | 141.711 | 46.195 | 0 |
| 34 | 33 | 837.292 | 631.485 | 0 |
| 35 | 34 | 350.435 | 254.195 | 0 |
| 36 | 35 | 426.601 | 281.906 | 0 |
| 37 | 36 | 348.595 | 254.863 | 0 |
| 38 | 37 | 377.922 | 282.005 | 0 |
| 39 | 38 | 530.099 | 433.204 | 0 |
| 40 | 39 | 328.737 | 232.460 | 0 |
| 41 | 40 | 386.129 | 291.167 | 0 |
| 42 | 41 | 321.552 | 225.426 | 0 |
| 43 | 42 | 327.046 | 233.372 | 0 |
| 44 | 43 | 139.257 | 44.795 | 0 |
| 45 | 44 | 152.164 | 45.394 | 0 |
| 46 | 45 | 387.983 | 292.429 | 0 |
| 47 | 46 | 332.267 | 238.393 | 0 |
| 48 | 47 | 352.158 | 256.107 | 0 |
| 49 | 48 | 347.754 | 251.563 | 0 |
| 50 | 49 | 390.890 | 295.212 | 0 |
| 51 | 50 | 174.858 | 79.666 | 0 |
| 52 | 51 | 143.084 | 50.705 | 0 |
| 53 | 52 | 260.541 | 165.728 | 0 |
| 54 | 53 | 254.465 | 162.413 | 0 |
| 55 | 54 | 202.280 | 77.889 | 0 |
| 56 | 55 | 141.170 | 46.521 | 0 |
| 57 | 56 | 137.755 | 45.789 | 0 |
| 58 | 57 | 146.573 | 52.347 | 0 |
| 59 | 58 | 197.121 | 41.488 | 0 |
| 60 | 59 | 1185.342 | 1049.922 | 0 |
| 61 | 59 | 1424.220 | 1280.798 | 0 |
| 62 | 60 | 797.319 | 702.715 | 0 |
| 63 | 61 | 3217.300 | 3120.511 | 0 |
| 64 | 62 | 154.731 | 44.277 | 0 |

</details>

两个时长来自不同边界和时钟，不能相减声称网络耗时；并行工具不可累加为 Runtime 墙钟。完整 ToolCall / execution 身份和来源行见原 summary JSON。

ECS 归档：`/opt/aries-artifacts/tb-huawei-cpp-record-2`。driver evidence SHA-256 `8aa8f8c2ae70b552d8376aea5b091c5c0cd75b9ab84ec5fcd269e5c4982b548f`；`performance-review-1/summary.json` SHA-256 `7221aa94878d1d31def920d5f23e9ede8a6468f43dd2831156ee3429da985832`；`offline-analysis-1/evidence.json` SHA-256 `dbde73e2be022e3f28c1d57de036e80ef07ac396757bf4df81f71f057ea5617e`。首轮 Strict Replay 的真实失败另列于下节，不由成功录制推断回放通过。

## 第二轮 Tape 的首次 Strict Replay：真实失败

`tb-huawei-cpp-replay-1` 在同一 Huawei image-2 / template-2 中真实执行，仅完成 1 次 exec 后，于**第 2 个模型请求（零基 turn=1）的 `$.messages[3].content`** 被 Strict 拒绝。记录与回放的工具内容均为 217 行、5307 字节，只有第 3 行变化：`ls -la` 在 `/app` 展示的 `..`（根目录 `/`）mtime，从 **03:55 变为 04:10**。其余 216 行一致；当前 binding 没有配置初始目录 mtime。

这是首次工具读取无关动态目录元数据造成的完整历史偏离，尚未执行到录制中的同轮多工具；此次不是双流合并、PID、后台 poll 或多工具顺序错误。请求与原生 ToolCall ID 依照已声明 `openclaw-2026.7.1` 去标点规则唯一对应，ToolResult SHA 与拒绝请求诊断完全相符，已有 1/1 exec 身份关联、producer 健康。

Driver 的末尾 `native_decisions_differ_from_real_tape` 是回放中止后，拿已执行的 1 个决策对比完整 64 个决策产生的后续验收错误；第一处根因仍是上述 mtime 偏离。原独立 Verifier 真实执行 **5/6，reward=0**，与首轮录制缺失 CTRF 的情况不同。Harness 返回成功不能覆盖模型 session 失败或原评分失败；停止 / 撤权已确认，所属 1 个 Sandbox 已确认删除，cleanup_errors 为空。

| 八阶段 | 回放耗时（秒） |
| --- | --- |
| 排队 | 0.000010603 |
| 任务 Sandbox 创建 | 0.665390 |
| 工作区准备 | 1.163288 |
| Harness 启动 | 7.114792 |
| Runtime 执行 | 5.455855 |
| 停止与撤权 | 5.441044 |
| 独立验证 | 28.065443 |
| 清理 | 0.543772 |

排队取原 Attempt.queue_duration=10603 ns，其余七阶段取原 timing.duration 字段。原统一 Snapshot 为 integrity=complete、coverage=unknown；本次诊断未重做独立双投影对账，不能借 record-2 的对账结果代替。

原 Tape、原失败运行、原评分与 matcher 全部保留。按已批准的元数据变体方向，下一步使用独立公开附录约束无关动态信息的读取，并重新真实录制与验收；不手工修改旧 Tape、不改真实 ToolResult，也不擅自调整根目录时间来凑过旧轨迹。此变体尚未录制或回放。

证据：ECS `/opt/aries-artifacts/tb-huawei-cpp-replay-1/evidence.json` SHA `9a73ff20538b17e1838be14ab846ed3bb20049cc5d53ce5d538b3778317a19b6`；record-2 Tape SHA `063be0eb73a5022f0805db4bd17e9261526a1ec9eb7ae151c45bcc21d2dfafc0`；该轮 `run-result.json` SHA `fdbbf5ce57ccb6e08626fcda8ae2e2c23a978cb0a0e99f5e1a45a09277a325bb`。本地只读差异摘要位于 `.cache/tb-cpp-replay-diagnostic-1/detail.json` 与 `path.json`，仅保存身份、计数、哈希及元数据差异，不包含工具正文。

## 首轮真实任务八阶段

阶段名称与[性能阶段说明](performance-stages.md)保持一致。以下值取自首轮终态与性能复核原件，原 duration 单位为纳秒，表中换算为秒。ECS 欠费曾中断取证，恢复后读取已有原件，没有为补表重跑 Agent。

| 阶段 | 华为 C++ 耗时（秒） | 状态 | 本任务中实际需要测量的边界 |
| --- | --- | --- | --- |
| 排队 | 0.000010682 | 已接纳 | 计划到达至 ECS 调度接纳；不是云内部队列 |
| 任务 Sandbox 创建 | 0.631251 | PASS | Adapter 创建请求至取得任务环境；不含预先构建镜像/模板 |
| 工作区准备 | 1.142995 | PASS | 原 Benchmark 的准备调用 |
| Harness 启动 | 7.179689 | PASS | ECS 上启动外置 OpenClaw 的调用窗口 |
| Runtime 执行 | 201.730376 | Agent 完成 | Agent 请求开始至结束，包含模型等待和真实工具交互 |
| 停止与撤权 | 5.387265 | PASS | Harness Stop 与 Bridge revoke，保留实际停止确认 |
| 独立验证 | 1665.226050 | FAIL，未取得原 CTRF / reward | 原私有 Verifier 的注入、执行、报告回收与解析 |
| 清理 | 0.767240 | PASS，1 个 Sandbox 已删除 | 本次所属 Sandbox 和运行资源的回收确认 |

八阶段存在未覆盖区间，不机械相加替代任务 E2E。首轮已核对的离线 attempt 计数为 planned / admitted / started / executed / completed 各 1，failed 为 1、verified_succeeded 为 0；这表示原评分没有产生已验证成功，不能把缺失的 reward 填为 0。不同进程、主机的时钟不能直接相减当作网络耗时，也不能把 SDK 等待与 guest 时间相加。

原 C++ Verifier 的 6 个测试函数及其最多 5 次评分编译，是原测试的静态结构，不能直接填作实际编译次数。Agent 自行编译属于 Runtime，评分重新编译属于独立验证，分别计时。

## 逐工具与双 Trace：限定范围对账通过

| 观察项 | 当前能确认的事实 | 尚需从本轮原件补齐 |
| --- | --- | --- |
| 录制轨迹 | 41 轮，39 次 exec、1 次其他工具调用 | 不把轮数直接当作模型网络请求次数；请求、重试和缓存命中分别核对 |
| 逐工具耗时 | 39 次 exec 已逐项取得控制器等待、guest 整体耗时与退出码，身份精确关联 | 原 JSON 保留完整 ToolCall / execution ID、源文件及行号；其他 1 次原生工具不借用 exec 耗时 |
| Perfetto / Chrome Trace | 限定范围对账 PASS：4170 个 interval、217 个 flow，时长对账误差 0、excluded 0 | integrity 为 complete，coverage 为 unknown；不能扩展为全链路覆盖已证明 |
| OTel / OTLP | 同一分析快照生成 4170 个 Span，217 个对应 parent 关联，与 Perfetto 限定范围对账 PASS | 本轮后端查询为 `UNRUN_THIS_RUN`，浏览器验证为 `UNRUN`；这是离线投影对账，不能宣称 Jaeger 查询通过 |

### 逐次 exec 工具耗时（毫秒）

按原生实际调用顺序排列；模型轮由 Tape 中的精确 ToolCall ID 查询，从 1 开始。**SDK 等待**是 ECS 控制器侧的 `exec.wait`，**guest 整体**是 Sandbox 进程回执中的时长；两者时钟域不同，不能相减认定网络耗时，也不能相加当作总耗时。下面均为身份精确关联的已完成调用，不是 CPU 用时。

| exec 编号 | 模型轮 | SDK 等待（ms） | guest 整体（ms） | 退出码 |
| --- | --- | --- | --- | --- |
| 1 | 1 | 394.052 | 173.536 | 0 |
| 2 | 2 | 147.124 | 41.210 | 0 |
| 3 | 3 | 10385.646 | 10293.038 | 0 |
| 4 | 5 | 537.453 | 397.116 | 0 |
| 5 | 6 | 200.050 | 60.166 | 0 |
| 6 | 7 | 3654.792 | 3564.628 | 0 |
| 7 | 8 | 935.719 | 846.149 | 0 |
| 8 | 9 | 165.997 | 18.276 | 0 |
| 9 | 10 | 145.973 | 44.522 | 0 |
| 10 | 11 | 185.924 | 97.447 | 0 |
| 11 | 12 | 134.825 | 45.782 | 0 |
| 12 | 13 | 806.167 | 673.582 | 0 |
| 13 | 14 | 353.209 | 263.751 | 0 |
| 14 | 15 | 784.208 | 640.792 | 0 |
| 15 | 16 | 234.091 | 145.383 | 0 |
| 16 | 17 | 384.094 | 294.555 | 0 |
| 17 | 18 | 372.086 | 282.269 | 0 |
| 18 | 19 | 373.497 | 284.026 | 0 |
| 19 | 20 | 389.301 | 297.080 | 0 |
| 20 | 21 | 414.909 | 324.647 | 0 |
| 21 | 22 | 345.040 | 254.851 | 0 |
| 22 | 23 | 329.293 | 238.935 | 0 |
| 23 | 24 | 344.326 | 254.616 | 0 |
| 24 | 25 | 330.429 | 241.092 | 0 |
| 25 | 26 | 328.104 | 237.602 | 0 |
| 26 | 27 | 387.607 | 297.498 | 0 |
| 27 | 28 | 743.313 | 612.379 | 0 |
| 28 | 29 | 358.359 | 267.233 | 0 |
| 29 | 30 | 408.482 | 318.860 | 0 |
| 30 | 31 | 141.817 | 52.777 | 0 |
| 31 | 32 | 608.917 | 478.860 | 0 |
| 32 | 33 | 832.951 | 743.228 | 0 |
| 33 | 34 | 239.466 | 150.877 | 0 |
| 34 | 35 | 1284.150 | 1194.323 | 0 |
| 35 | 36 | 3250.591 | 3160.652 | 0 |
| 36 | 37 | 132.418 | 43.803 | 0 |
| 37 | 38 | 5019.988 | 4929.807 | 0 |
| 38 | 39 | 133.582 | 44.791 | 0 |
| 39 | 40 | 5031.231 | 4940.985 | 0 |

另有 **1 次 `process` 原生工具调用**；本表没有其对应的独立 SDK / guest 耗时，记为未知，不能填零或用某次 exec 的时长代替。全部 39 次 exec 的退出码为 0，也不能代替原 C++ 私有评分成功。Bridge、工具、SDK 与 guest 的关联使用该轮原件中的精确身份，不按命令文本或时间近邻猜测。

## 保留的失败与下一步验收

E2B 原 C++ 运行因创建失败未进入 Agent；独立诊断对 **4200 秒**的实际请求得到 HTTP 400，错误为 `Timeout cannot be greater than 1 hours`。原失败、原 Attempt 分母及诊断证据保留在 `/opt/aries-artifacts/tb-cpp-create-diagnostic-1`；没有实例 ID，列表未发现不能写成 NotFound 确认。

**华为 record-2 已取得独立评分成功；E2B 原 C++ 仍未进入执行，故本题暂没有一对完整成功的跨云样本。** 华为运行不代表 E2B 时限问题已修复。按本项目当前比较口径，相同 CPU / 内存配置即可开展跨云性能比较；磁盘等差异如实标注并单独分析，不作为阻断条件。本题不从一次失败推算成功吞吐、Goodput 或分位数。

两轮都按原 1800 + 1800 秒预算运行，保留首轮评分失败与 attempt 分母。第二轮已用 image-2 / template-2 取得原 reward/CTRF；这份新 Tape 的首次 Strict Replay 已因目录 mtime 偏离而失败，后续独立元数据变体仍需新录制及验收；所有创建、Agent、评分或清理失败均保留并归因。本页不批准或实施新的可复现变体，不改变原 ToolResult、Strict matcher、后台工具或原 Verifier。

## 证据索引

以下 SHA-256 定位 ECS 归档原件。首轮终态目录为 `/opt/aries-artifacts/tb-huawei-cpp-record-1`；其中性能复核与离线 Trace 导出读取同一轮冻结原件，没有追加模型或 Sandbox 调用。

| 证据 | ECS 路径 | SHA-256 |
| --- | --- | --- |
| release5 五门禁 | `/opt/aries-artifacts/tb-six-release-5/evidence.json` | `cd75068fbff07c2d6a9c89870b25ee2b9d9e764564e7105708048dde4de567ef` |
| release5 608 文件清单 | `/opt/aries-artifacts/tb-six-release-5/source-manifest.json` | `fdb21b8ab479a1e6680582046c2255717f52efd544c4faacb8784ceb356bcd2f` |
| 镜像准备与发布 | `/opt/aries-artifacts/tb-huawei-cpp-image-1/evidence.json` | `5d181834a01028ff3c06876faf489db2ab2a4fa41c537f6125a986efe8decd0c` |
| 华为模板构建与绑定 | `/opt/aries-artifacts/tb-huawei-cpp-template-1/evidence.json` | `3efc3e72cc1a56ec875d5caed3792c683148f5b62bbc7f7fa9b60c618074d538` |
| 首轮真实录制终态 | `/opt/aries-artifacts/tb-huawei-cpp-record-1/evidence.json` | `783f4e1241e5af9679d028aaddd99d7275279bd8970cbfe8c6607b08d98b0aa7` |
| 首轮阶段与逐工具复核 | `/opt/aries-artifacts/tb-huawei-cpp-record-1/performance-review-1/summary.json` | `efd8d04bd9ede293d6754295217a0d9f12693822efb900ada0d96ddb26ad3d77` |
| 首轮性能可读摘要 | `/opt/aries-artifacts/tb-huawei-cpp-record-1/performance-review-1/summary.md` | `70fecbc3ccc7b4717877af33f410a1370f4b87c6c909279bfb242e711cd5df34` |
| 首轮离线 Trace 对账 | `/opt/aries-artifacts/tb-huawei-cpp-record-1/offline-analysis-1/evidence.json` | `ce1a847836709e73b6943b58adb4b52a8b5b9664f26b65399acd71dcce2d4efe` |
| image-2 准备、发布与公开依赖 smoke | `/opt/aries-artifacts/tb-huawei-cpp-image-2/evidence.json` | `1c5ce76d71b48e5f839cedfd69941ba69f88f7a0367cd09aabc96dfaaf56b8aa` |
| template-2 构建与绑定 | `/opt/aries-artifacts/tb-huawei-cpp-template-2/evidence.json` | `607658807c4186355c9032979dbedaf19d958ce028ec0f57b20470f59ae52dfb` |
| 版本 pins 准备 | `/opt/aries-artifacts/tb-huawei-cpp-pins-1/evidence.json` | `a48acb67f9b5fc84a2c3e83570e518a21d57082b170891f1468719f8ce835b1e` |
| 后续执行使用的 pins | `/opt/aries-artifacts/tb-huawei-cpp-pins-1/pins.json` | `d506ec3f2c7453521a6c8fe9ffe8afc0e80d064b2d0f56c5561b920d6161c9c6`（来自上述准备回执） |

pins 回执另保存源码差异表和原生复用证明，分别为 SHA `6dc7610492047c2393b8daa46c0d92436018032a65dd935e203e62eb42879fd4`、`fb41f776be6bd17a1ff236ba24f6ca23e4bef340b8f8463265ca0635701f9de7`。仅 release root、release evidence SHA、source manifest SHA 改为 release5；publication3 未改写，其旧原生矩阵验收仍属于 release4，不宣称本轮再次执行。
