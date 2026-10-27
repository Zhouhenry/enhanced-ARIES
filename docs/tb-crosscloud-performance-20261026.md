# 原始 TB 同 Tape 双云对照（2026-10-26）

**本轮同版本、同题同 Tape 完整成功的双云对照有 2 题：fix-git 和 cancel-async-tasks。** db-wal-recovery 在 E2B 成功，在华为因真实目录元数据偏离停止回放，不能加入完整工作量的成功对比。三题共 7 次真实尝试：5 次成功、1 次 Strict 偏离、1 次欠费中断；失败均保留。

全部使用 release5 / native publication3、A（External）模式：OpenClaw 在 ECS、真实工具在各云 Sandbox。Strict Replay 使用原录制时序（`timing=recorded`），完整历史匹配，不模拟工具结果。Sandbox 请求均为 **1 CPU / 2048 MiB**，可以开展同配置对比；磁盘、底层实现等差异单独说明。ECS 同期可能有其他工程验证，本批是单次功能验收与性能留档，尚不足以给出稳定的云厂商排名或 p95。

C++ 新真实录制已通过原评分，单列在 [C++ 归档](tb-huawei-cpp-performance-20261026.md)，不是本页已经完成的跨云对照。

## 结果、分母与工作量

| 尝试 | Strict / Agent | 原独立 CTRF | 模型完成轮 / exec | 停止撤权 / 清理 | 是否用于成功双云对照 |
| --- | --- | --- | --- | --- | --- |
| fix 华为 1 | 通过 | 2/2 | 9 / 8 | 确认 / 已确认删除 | 是 |
| fix E2B 1 | 通过 | 2/2 | 9 / 8 | 确认 / 已确认删除 | 是 |
| async 华为 1（中断） | 回放完成；评分取消 | 缺失（取消，非 0/6） | 3 / 2 | 确认 / 原轮未确认，后续恢复清理 | 否 |
| async 华为 2 | 通过 | 6/6 | 3 / 2 | 确认 / 已确认删除 | 是 |
| async E2B 1 | 通过 | 6/6 | 3 / 2 | 确认 / 已确认删除 | 是 |
| DB 华为 1（偏离） | 第 2 个模型请求 Strict 偏离 | 0/7 | 1 / 1 | 确认 / 已确认删除 | 否 |
| DB E2B 1 | 通过 | 7/7 | 12 / 11 | 确认 / 已确认删除 | 否 |

fix 的第 5 次 exec 两云都真实退出 1，后续修复后原评分 2/2；DB 首次 exec 两云都退出 127。中间命令失败不等于任务失败，也不应从轨迹或耗时表中删除。原题独立评分、模型回放结果和执行证据分别判断。

`isolation=confirmed` 在这里指原运行确认 Harness 停止及 Bridge 撤权，不能仅由它推断云网络全面禁出网。本页不发布 isolation-verified Goodput。旧 async 华为 1 的原 cleanup 失败不覆盖为成功；恢复处理另归档于 `/opt/aries-artifacts/tb-crosscloud-cancel-async-tasks-huawei-recovery-1/`。

## 八阶段耗时（秒）

行名逐一对应 [独立阶段说明](performance-stages.md)。排队为控制器接纳时间，不是云平台内部队列；准备镜像/模板不计入 Sandbox 创建。Runtime 包含录制时序的模型等待和真实工具；Verifier 包含注入、依赖准备、执行及报告回收。不同窗口存在间隙，不能简单相加冒充端到端墙钟。

| 阶段 | fix 华为 1 | fix E2B 1 | async 华为 1（中断） | async 华为 2 | async E2B 1 | DB 华为 1（偏离） | DB E2B 1 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 排队 | 0.000017 | 0.000012 | 0.000010 | 0.000014 | 0.000011 | 0.000014 | 0.000012 |
| 任务 Sandbox 创建 | 0.585050 | 1.436937 | 0.631464 | 0.670008 | 0.993043 | 0.551843 | 1.402441 |
| 工作区准备 | 2.597038 | 1.474683 | 2.267580 | 1.406926 | 1.628996 | 0.475479 | 1.286159 |
| Harness 启动 | 10.198691 | 8.067101 | 7.272166 | 10.174032 | 7.642175 | 7.811352 | 7.191613 |
| Runtime 执行 | 27.196913 | 28.295014 | 10.797737 | 11.453977 | 11.782626 | 4.968323 | 54.549916 |
| 停止与撤权 | 5.605342 | 5.601030 | 5.393866 | 5.392054 | 5.582308 | 5.462352 | 5.633999 |
| 独立验证 | 193.061464 | 6.716091 | 164.686524 | 42.954302 | 23.936623 | 23.570430 | 13.314420 |
| 清理 | 0.603007 | 0.732652 | 0.066047 | 0.539285 | 0.608379 | 0.503669 | 0.622377 |

async 华为 1 的 164.687 秒 Verifier 是取消前窗口、0.066 秒 cleanup 是失败调用窗口，均非完成所需时间。DB 华为在首工具后偏离，4.968 秒 Runtime 不可与 E2B 完整的 54.550 秒工作量算速度比。fix 华为 Verifier 193.061 秒与 E2B 6.716 秒差异保留，不能当成 Agent Tool 耗时或直接归因为 Sandbox CPU。

## 相同 ToolCall 的实际工具耗时

以下按同 Tape 的精确 ToolCall ID 配对，以模型轮与 exec 序号展示。**控制端执行窗口**沿用原字段 `exec.wait`：从 ARIES 控制器客户端提交进程前到 Wait 返回，包含提交、执行、输出传输与排空；不是单独 Wait 方法或官方 SDK 内部耗时。**guest 整体**是 Sandbox 内 wrapper 执行回执的实际时长。两者时钟与边界不同，不相减推断网络时间，也不把 控制端执行窗口称为完整 Harness Tool 延迟。表内保留非零退出；不显示命令、私有测试或模型正文。

| 题目 / exec | 模型轮 | 华为 控制端执行窗口 ms | 华为 guest 整体 ms | E2B 控制端执行窗口 ms | E2B guest 整体 ms | 两云 exit |
| --- | --- | --- | --- | --- | --- | --- |
| fix / 1 | 1 | 3214.594 | 3115.101 | 1260.302 | 1005.234 | 0 |
| fix / 2 | 2 | 141.720 | 51.980 | 284.231 | 33.472 | 0 |
| fix / 3 | 3 | 412.222 | 315.349 | 337.647 | 82.718 | 0 |
| fix / 4 | 4 | 142.766 | 40.262 | 304.005 | 39.138 | 0 |
| fix / 5 | 5 | 166.878 | 70.764 | 965.244 | 141.504 | 1 |
| fix / 6 | 6 | 145.567 | 49.377 | 289.182 | 33.087 | 0 |
| fix / 7 | 7 | 143.699 | 47.469 | 296.036 | 34.944 | 0 |
| fix / 8 | 8 | 137.679 | 33.787 | 303.847 | 42.780 | 0 |
| async / 1 | 1 | 141.387 | 44.726 | 287.481 | 32.334 | 0 |
| async / 2 | 2 | 553.493 | 465.781 | 891.405 | 630.997 | 0 |

### DB 的未完成配对

华为只完成首次真实工具调用，完整模型历史因输出元数据不同而拒绝继续。表中「未执行」与「未采集/未知」不同，不能填 0。

| exec | 模型轮 | 华为 SDK / guest ms | E2B SDK / guest ms | exit（华为 / E2B） |
| --- | --- | --- | --- | --- |
| 1 | 1 | 281.646 / 179.997 | 967.453 / 68.892 | 127 / 127 |
| 2 | 2 | 未执行 | 381.044 / 65.611 | — / 0 |
| 3 | 3 | 未执行 | 918.857 / 41.532 | — / 0 |
| 4 | 4 | 未执行 | 378.399 / 75.183 | — / 0 |
| 5 | 5 | 未执行 | 415.402 / 128.611 | — / 0 |
| 6 | 6 | 未执行 | 345.443 / 48.759 | — / 0 |
| 7 | 7 | 未执行 | 330.697 / 42.984 | — / 0 |
| 8 | 8 | 未执行 | 340.692 / 50.428 | — / 0 |
| 9 | 9 | 未执行 | 334.304 / 45.247 | — / 0 |
| 10 | 10 | 未执行 | 878.584 / 40.990 | — / 0 |
| 11 | 11 | 未执行 | 350.588 / 66.178 | — / 0 |

### 中断样本的观测边界

async 华为 1 留有 2 个 exec 和终态回执，但 SDK 索引因 `owned_sandboxes_remaining` 被标为 incomplete。分析器因此拒绝把该索引当完整身份来源，两条工具耗时在正式表中保留未知；不凭手工匹配补成合格观测。只读核查确认原生调用、Bridge、SDK execution/process 和 guest 回执对应，未发现独立 ToolCall 透传错误。原失败与不完整索引均保留，新的 async 华为 2 重新完整通过。

## DB 偏离与下一步

第二个模型请求的 `$.messages[3].content` 与原 Tape 不同：目录块总数、链接数、目录大小和日期均有差异；原声明的初始目录 mtime 已正确设置，单改日期无法消除全部文件系统差异。Strict 按契约拒绝，未修改 matcher 或 ToolResult。1970 日期具体由哪个底层步骤引入尚未定论。

已构造独立 [metadata-portable-v1 变体](../cases/sandbox-runtime-v1/agent-cases/db-wal-recovery/variants/metadata-portable-v1/README.md)，只追加「读取任务需要的元数据、避免无关动态信息」的公开建议。原题、fixture、预算、私有评分和原失败 Tape 保留。它需要新真实录制及同新 Tape 双云验收；当前不是通过结果，也不作为原生不变的官方成绩。

## 双 Trace 与真实性范围

Perfetto / Chrome Trace 与离线 OTLP 均来自同一份校验后 Snapshot，已用 Trace Processor 与 OTLP 字段独立对账。此次未向 Jaeger 发送或查询，也未进行浏览器展示验收；不得将离线投影成功写成 P1 后端验收。所有本表导入时间与结束时间最大误差均为 0 ns、导出排除区间为 0。

| 尝试 | Snapshot integrity / coverage | Perfetto interval / flow | OTLP span / parent | 结论 |
| --- | --- | --- | --- | --- |
| fix 华为 1 | complete / unknown | 706 / 48 | 706 / 48 | 已有观测范围内对账通过 |
| fix E2B 1 | complete / unknown | 652 / 48 | 652 / 48 | 已有观测范围内对账通过 |
| async 华为 1（中断） | incomplete / incomplete | 148 / 24 | 148 / 24 | 不完整源的投影通过；任务失败 |
| async 华为 2 | complete / unknown | 262 / 24 | 262 / 24 | 已有观测范围内对账通过 |
| async E2B 1 | complete / unknown | 264 / 24 | 264 / 24 | 已有观测范围内对账通过 |
| DB 华为 1（偏离） | complete / unknown | 185 / 20 | 185 / 20 | 已有观测范围内对账通过 |
| DB E2B 1 | complete / unknown | 1207 / 60 | 1207 / 60 | 已有观测范围内对账通过 |

`integrity=complete` 表示已声明来源完整，不等于所有内部阶段都已观测；`coverage=unknown` 保留了这一边界。不同 run 的 interval 数不同不代表工作量不同，后台采样和基础操作也会产生事件。SDK / guest 可测范围之外的 Harness 内部开销不能通过时间相减虚构。

## 复核位置与固定身份

所有原件在 ECS `/opt/aries-artifacts/`。每轮的 `performance-review-1/{attempts.json,summary.json,summary.md}` 保留阶段纳秒值、ToolCall 身份、来源行和输入 SHA；`offline-analysis-1/` 保留统一快照、双投影、Trace Processor SQL / CSV 和对账证据。此轮只读取原运行生成派生文件，没有新增 Sandbox、模型调用或重复执行 Agent。 八个已归档 run（含 C++ record-2）的 24 份重复 Snapshot / Trace / OTLP 文件已逐一校验 SHA 相同后改用 hardlink，共享原件并保留全部路径、内容与哈希，释放 280,760,320 字节；小型摘要及验收证据保持独立。局部记录见 ECS `tb-crosscloud-projection-dedup-1/evidence.json`，SHA `a9b3b032a60eee23be2fb7b933e2a2c85c37aff481eb07925a9204fa44132b68`。

| 尝试 | 归档目录 | driver evidence SHA-256 | summary SHA-256 | offline evidence SHA-256 |
| --- | --- | --- | --- | --- |
| fix 华为 1 | `tb-crosscloud-fix-git-huawei-replay-1` | `54b8545d01a7185932f3691b76ed88a9fbb0682908c88859739900f7a88a2dab` | `2c05517d27da843e911d92ae97ae6561b74cd41b9016c21a004cf73d1c516030` | `8f834ac519a1a51f2b89448df3117b9ed6491287df62d96e8cac99868c104d86` |
| fix E2B 1 | `tb-crosscloud-fix-git-e2b-replay-1` | `6f771cb889b2f8c6ca3bd517caa6b2406382f32f4916ff3e75d5ca51ffcb5ef0` | `730313473ba92e4cc6c9475f4a01887f917760849ec7af210d8167a7dc992297` | `e346acacbf16b220b19ac5c0196cd95b8b954a9f600650f533401343da9e4fe5` |
| async 华为 1（中断） | `tb-crosscloud-cancel-async-tasks-huawei-replay-1` | `6ca3d8763b6d7d7d8f6aaeec0184236ad4a29a1c6e8b6fd2d2ef57a1635b3cc1` | `0ab2e1e7daab70eda2b361a56d306655215a6862c2a8a0e8aa14b9cf4f4ea506` | `219c1df020062bb87c67faa8cc59085658c81c3bed7fd34847b279c9108066b0` |
| async 华为 2 | `tb-crosscloud-cancel-async-tasks-huawei-replay-2` | `83a84fbf9315ec8d70e3675024f2ce4a103934c1478b9f4f2e9148d1cb320202` | `cd696e4c79aa59ad995a72f74efbb4ae5328f09e11c1b40ef4b0b277c2691d81` | `24cfd4b829da87c4e6dd341c2d6ce502aaa32fdec98a12faa75faf6ab1bf9988` |
| async E2B 1 | `tb-crosscloud-cancel-async-tasks-e2b-replay-1` | `8fa76184c601fbc98afcae5c536ee57f009a4d217f6ea9ce61a90ff4d0e30f4d` | `c66749fd9366d21b496909d1bfac50c90e723d3be20acc74bad22e6e4b2091d3` | `42867cb12703103a11bbb124fd4fab24bf945fd20ddc147f3df285896b26e3b2` |
| DB 华为 1（偏离） | `tb-crosscloud-db-wal-recovery-huawei-replay-1` | `5802f349c5e5960596d3a202bc5c2c53868099277dd527345e9850943a5bf4d5` | `d7646b16ce16bb6c75d02343161305f4410f5064e832ac4209e13b024382a5aa` | `4612c891854a553c55dc42b6afb0e9be4ede7cf6e6e44b97c575f0bf69c24fa7` |
| DB E2B 1 | `tb-crosscloud-db-wal-recovery-e2b-replay-1` | `036eb620340f678cfab0d614681a1da90958fbf9968bd7232b1c3c1ed8bfdb23` | `de2df7c442e6500d24a9e8a12f3ac721f314873cc7c7e144baab794c487441b3` | `80e77dd3b10477904213758f7a64edf5a087bd000fc86e4a2f92374ee82a1cd0` |

固定版本：release5 source manifest `fdb21b8ab479a1e6680582046c2255717f52efd544c4faacb8784ceb356bcd2f`；native publication3 evidence `9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1`。此次没有借用旧版本的成功样本凑双云配对。

| 原题 | 原 Tape SHA-256 |
| --- | --- |
| fix-git | `48b960ffb49186d761c64ab152a70d09f7d9e1730217a14c8d7c2471f664301b` |
| cancel-async-tasks | `69cda572dbeb841739ada65a357492847395fbcecebce312c3605e314f704054` |
| db-wal-recovery | `180abc61b0333ff38aa5ebcf91c727210da85311dc35253552fa197c7f3801b4` |

汇总只读副本：`.cache/tb-crosscloud-report-1/summary.json`，SHA-256 `4124ff9b7f864c1708a7e77d180b37836c57357c953bc495512b4e329b1c7cfb`。阶段性样本保留完整尝试分母；后续只有同配置重复样本和受控负载完成后，才讨论稳定分位数或厂商排名。
