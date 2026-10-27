# 用例来源与 Sandbox 性能测量依据

**资料核对日期：2026-10-25。** 真实 Agent 主线为 [六例原始 TB 材料](agent-cases/README.md)，material_revision=3，`CONSTRUCTED_UNRUN`；[ASB catalog](cases.json)、[验收卡](acceptance.md) 和标准工具规格是辅助测量设计。exec-short 另完成 ECS 本机子进程功能检查，尚无 Agent、ToolBridge 或 Sandbox 性能结果。材料构造/功能检查与真实运行/性能资格分别报告，没有新增可加载 Profile，也没有接入参考 suite。

## 权威性来自哪里

来源权威与测量有效是两件事。采用上游工具可以减少自制测量误差，但仍须真实执行、固定配置、核对结果；公开基准中的任务也不能自动给出纯 Sandbox 性能。

| 来源类别 | 可以沿用的依据 | 本项目的声明范围 |
| --- | --- | --- |
| 原始 Agent 任务基准 | 固定版 Terminal-Bench 的任务、环境和原始独立评分 | `fix-git` 等六例用于任务质量及 Agent E2E，保留原身份与评分；六例选集的结果不能当完整榜单成绩，更不能直接当 Sandbox 硬件分数。 |
| 成熟性能工具 | fio、sysbench、iperf3 的原始实现、参数语义和输出 | 后续按固定版本构造独立变体，注明我们的实验配置；工具官方并不等于实验得到官方认证。当前尚未实现或运行。 |
| 自建受控负载 | 当前 10 个 ASB 的固定输入、明确工作量和独立功能校验 | 用于定位 Sandbox 中工具链与程序工作流的耗时；属于项目自建案例，没有行业认证或官方基准分数。参考包的 local 结果不是本系统结果。 |

固定 commit 的 [Terminal-Bench 2.1 README](https://raw.githubusercontent.com/harbor-framework/terminal-bench-2-1/7131e4375048a0e408a8fb404b5f499d726b695b/README.md) 将目标定义为 Agent 在容器中完成复杂任务的能力。因此本项目以原 TB 自主任务作为 Agent Runtime 质量与性能主线，ASB 固定工作量仅作辅助归因；这一分层是我们的实验设计。原任务文件哈希和许可证见 [Agent 清单](agent-cases/index.json)；revision 1 的本地参考包证据保留在 catalog，后续新增官方源文件与旧文件未变证明分别单列在 [revision 2 来源清单](agent-cases/revision-2-source-manifest.json) 和 [revision 3 来源清单](agent-cases/revision-3-source-manifest.json)。

## 主线：真实 Agent 任务怎样测到 Sandbox

六例原任务由 Agent 自主调查、修改和调试，覆盖 Git、Python、数据库恢复、SQLite 构建、Vim 大文件处理与小型 C++ 项目调试；实际执行工作量以真实调用为准，独立评分验证任务结果。它们测试 Sandbox 承载真实 Agent 工作流的表现；任务 E2E 同时包含模型与 Harness，不能直接当 Sandbox 单独得分。C++ 原评分中预设的五次编译属于 Verifier 工作，不是 Agent 调用数，也不能用于计算 Agent 编译吞吐。

沿用既有阶段与身份观测，分别报告排队、环境准备、Runtime、模型等待、真实工具/guest 进程、评分与清理。工具内部缺测只限制相应归因，不能从总时间相减猜出网络或进程耗时。先录制真实成功 Agent 轨迹，再以同 Tape Strict Replay、真实工具和独立 Verifier 对照，可固定模型决策；不硬编码探索顺序或每任务调用数。

成功轨迹回放衡量选定轨迹下的 Runtime 表现，其通过率不是模型自主解题成功率。录制尝试和回放尝试分别保留全部成功、失败及对应分母，不能只挑回放成功者比较。

比较固定任务/镜像 digest/工具版本、CPU/内存/架构、云区域与运行端位置、冷热条件、A/B placement、模型模式；真实模型与回放分别统计，失败和重试留在原分母。六例只是初批，**不覆盖 CPU、IO、网络性能全谱**。后续可筛选真实仓库修复/编译任务扩充，不在本轮执行；需要解释单一资源瓶颈时再结合下面的辅助负载和官方工具变体。

## 怎样确认测到真实执行

一个基本性能样本应能串起：**固定输入/实际运行代码 → 真实 native 工具提交 → 对应 Sandbox 进程及终态 → 封存产物 → 停止撤权后独立验证**。复用已有身份、进程事件、Runner 阶段和版本化 AnalysisSnapshot，不创建第二套计时或匹配机制。

- `T_tool` 使用同一 native 调用的实际开始/完成边界；A 包含已定义的 SSH/Bridge 往返，B 使用其原生边界，二者分组展示。
- `T_process` 仅在已有 guest/native process 事件实际覆盖且同一时钟域时报告。spawn、直接子进程 exit、最后 pipe EOF 分列；不能把 ToolResult 返回等同于全部后代退出。
- `T_run` 使用现有 Runner 的执行阶段；环境准备、Verifier、Cleanup 独立列出。缺失内部阶段不从总时间扣出一个“估计值”。
- 固定程序可以一次 ToolCall 执行大量内部操作；记录实际启动命令、代码/二进制哈希、调用身份与终态，并独立核对产物。无需把每次内部循环或 syscall 变成新的硬门槛。细分测点缺失，只限制该细分指标或归因。

反之，仅有候选 `valid=true`、预填 `result.json`、自报耗时或文件存在，不能证明负载运行过。对编译、SQL、文件访问等历史，固定实际程序和真实调用支持程序级测量；若缺乏内部事件，不能进一步声称已直接观测到每次编译、commit 或 syscall 的时长。

## 10 个辅助 ASB 负载的测量对象

以下“必须证据”建立在上一节的公共链条上；列出的内部操作数是定义的程序工作量，不冒充已采集的事件数。来源中的 `lib/*` 路径指参考包，精确哈希见 catalog，未复制到执行环境。

| 用例与来源 | 实际被测对象 | 首要指标 | 必须保留的真实执行证据与独立验证 | 不能据此归因 |
| --- | --- | --- | --- | --- |
| `asb-exec-short-v1`；自建 `lib/exec_step.py` 状态链 | N 次真实调用、每次进程启动与跨调用状态；N=24/96/384 | 每次 `T_tool` 分布、N 次总时间；已有 `T_process` 单列 | 预登记 N 与 N 条真实 native 身份/终态对账；A 精确关联 Bridge/SDK/guest，B 关联 native/process；私有重算全部链文件。一条命令内循环 N 次不满足调用数要求。 | 不能叫纯 Sandbox 启动延迟；含 Python 启动和链文件工作。A/B 口径不同不能混比，缺钟对齐不能相减求网络开销。 |
| `asb-cpu-primes-v1`；自建 `lib/workloads.py#cpu_primes` | Python 试除、解释器、进程池启动和调度组成的 CPU 工作流 | 固定档位程序时间，已有 CPU 消耗；派生候选数/秒明确注明公式 | 固定 Python/源码/limit/rounds/workers 的实际进程执行，私有埃氏筛核对每轮 count/sum | 不是纯 CPU 核心/指令性能，也不是 sysbench 分数；缺 worker 观测不能分配各 worker CPU。 |
| `asb-memory-touch-v1`；自建 `memory_touch` | 分配/初始化、填充、CRC 扫描和释放的混合流程 | 固定字节工作量程序时间；实际可用的内存/CPU 采样 | 固定程序实际分配路径和终态；独立增量 CRC 与末轮样本检查，自报 RSS 与外部采样分开 | 不是纯 DRAM 带宽；分配器、Python 循环、CRC、缓存均可能主导。短任务采样峰值不足不能称真实峰值。 |
| `asb-small-files-v1`；自建 `small_files` | 批量文件创建、读校验、改写、改名，含数据生成 | 固定文件数/大小的程序时间；已有 tool/process 时间 | 固定程序与真实调用，精确 `.done` 文件集合和逐文件内容；回执计数仅作一致性核对 | 无逐文件测点不能报各 syscall 时延；buffered、无逐文件 fsync，不推断物理落盘量或删除性能。 |
| `asb-sequential-io-v1`；自建 `sequential_io` | buffered 写→fsync→读回哈希，含生成/校验开销 | 固定字节程序时间；guest 自报分段另标来源 | 实际固定程序/参数/进程终态、全文件内容与哈希校验；明确 read-after-write 和文件系统条件 | 读可能命中 page cache；不是裸盘/冷缓存带宽。fsync 返回与断电恢复验证不同，不能宣称已测崩溃耐久性。 |
| `asb-random-io-v1`；自建 `random_io` | QD1 buffered 随机读改写、模式生成与验证，Prepare 已预热 | 固定 blocks/passes 的程序时间；公式工作量与实测计数区分 | 固定程序、种子与原地 shuffle 规则，实际终态；独立核对最终每块及排列摘要 | Python 无缓冲文件对象不等于 O_DIRECT；不是独立块设备 IOPS、队列扩展能力或冷读实验。 |
| `asb-xxhash-build-v1`；[上游 v0.8.3](https://raw.githubusercontent.com/Cyan4973/xxHash/v0.8.3/README.md) + 自建构建流程 | 真实 C 编译、链接、运行探针 | 固定 rounds 的总构建程序时间；逐子进程时间仅在已采时报告 | 固定 C/H 与编译器版本，实际构建程序及新输出；独立验证源哈希、ELF 和标准向量 | 采用官方源码不等于运行官方 hash benchmark；本例不测 XXH64/XXH3 吞吐。旧 binary 的正确输出不能证明本轮编译。 |
| `asb-sqlite-etl-v1`；自建 ETL + [SQLite PRAGMA](https://sqlite.org/pragma.html#pragma_synchronous) | CSV 解析、WAL/FULL 批事务、索引、更新、聚合与 checkpoint | 同 rows/batch 的 ETL 程序时间；已有资源读数 | 固定 SQLite/SQL/输入；运行连接查询实际 journal_mode/synchronous 并归档；停止后独立只读查询全部行、聚合、索引和完整性。连接设置回执仍不是逐事务历史。 | 最终库正确不证明过去每次 commit/fsync；不能称数据库通用 TPS、物理存储带宽或断电耐久性。 |
| `asb-http-transfer-v1`；自建 `http_work/http_fixture` | 同 Sandbox 的 loopback HTTP、独立服务进程、每请求新 TCP 和正文校验 | 固定请求/字节的程序时间；request phase 自报另列 | 真实客户端与服务进程、就绪与终止，实际请求对应内容校验、双端回执一致；同权限日志注明来源 | loopback 不走跨主机链路，不能代表云厂商公网/内网带宽；不测 TLS/DNS/连接复用。 |
| `asb-http-retry-v1`；自建故障 fixture | 可控 503→成功及永久 503 耗尽的真实 HTTP 流程 | 固定失败模型的程序时间，成功请求数、耗尽数与全部 attempt | 客户端/服务真实进程；17/72/145 次尝试回执、完整 503 序列与内容验证；与实际 ToolCall 计数分开 | 同权限回执不等于独立网络采集；没有退避，不能评价退避策略；预期耗尽不计成功请求吞吐。 |

SQLite 官方说明 WAL 的读、写、checkpoint 是不同工作，自动 checkpoint 配置会改变执行路径；FULL 与 NORMAL 的同步语义也不同。因此后续实例化 ETL 时应记录实际 SQLite 版本、`page_size`、`wal_autocheckpoint` 和连接 PRAGMA，再将这些条件固定用于比较；这不表示当前 catalog 已采集这些数据。[SQLite WAL](https://sqlite.org/wal.html)、[synchronous 定义](https://sqlite.org/pragma.html#pragma_synchronous)

## 最小比较规则

1. **固定比较单元。** 同 case/档位/种子/初始数据、镜像 digest、架构、工具版本、CPU/内存配额及存储条件；受控负载或同 Tape 对照另固定实际工作代码。真实模型实验允许 Agent 自主生成不同代码和探索步骤，记录工作量差异，它们仍是有效 E2E 结果，不能因此排除。无法相同的配置明确分层，不把 vCPU 型号、地域、文件系统和资源大小差异统称为厂商性能差异。
2. **固定访问方式与模型条件。** 同 placement 比同 placement；真实 LLM 与 Strict Replay 分开。跨云使用同一真实录制 Tape 时保留完整匹配、真实 Tool 执行和独立 Verifier；回放失败仍进入结果，不反复筛到成功才统计。
3. **先写测量口径。** `T_tool`、`T_process`、`T_run` 和服务自报时间分别列出；分母使用对应真实完成量。默认不把环境准备/Verifier/Cleanup 混入 Run，也不丢弃它们的阶段时间。Goodput 沿用既有独立验证、隔离与显式 deadline 定义。
4. **独立重复且保留全部尝试。** 首轮建议每个固定组合至少 5 个独立 occurrence，按预先记录的随机/交替云顺序，列出逐次值、中位数和范围；这是本项目起步配置，不是工具官方要求。失败、超时、取消和未启动分别留在总分母。5 次不足以稳定解释 p99，不用少量样本制造尾延迟排名；重复目标升级时预先登记。
5. **冷暖条件与观测成本相同。** 新 Sandbox、进程重启、page cache 命中、构建缓存各自说明；不把新目录当冷缓存。固定 L0/L1 等既有观测等级，沿用采集开销对照；掉事件影响覆盖范围。CPU/内存/IO/网络指标只按实际归属和采样周期解释，不为缺测项补零。

功能通过仍不能算目标性能样本的典型情况：没有真实调用/进程终态；拿旧产物或旧回执代替执行；工作量、参数或已固定的运行代码改变；关键计时边界缺失或跨未校准时钟拼接；私有验证/停止撤权未成立；`exec-short` 只用一次调用模拟 N 次。此时保留功能结果、失败原因和分母，不计入合格吞吐。

另一方面，某个内部 syscall 或子步骤未采，并不自动排除完整程序样本。已有证据足以支持整体耗时时，就报告整体；仅将缺测的细分指标标为未知。若实际代码/输入固定却没有足够外部观测支持某种硬件归因，应收窄结论，而不是添加新的采集框架。

## 后续 5 项标准工具变体（仅定义）

以下均为 **DESIGNED_UNRUN**，没有加入当前 catalog 或 cloud Profile。实施前再冻结官方 release/commit、构建选项、二进制 SHA-256 和镜像 digest，以该版本 `--help`/文档核对参数。工具原始输出与 ARIES 的真实调用/终态、独立验证同时保留；工具报告不能替代实际启动证据。主线优先完善真实 Agent 任务材料，以下仅用于后续辅助归因，不替代自主 Agent 用例。

| 建议变体 | 初始固定配置 | 指标与边界 |
| --- | --- | --- |
| `asb-sysbench-cpu-v1` | `cpu-max-prime=20000`，固定 `events=500`、`time=0`；threads=1/2 各自独立组合，同总工作量，不混在一份结果 | 固定事件总时间、events/s、工具 latency；配合 ARIES 进程时间和实际 CPU 配额。对照当前 Python primes，减少解释器混杂，但仍不是 CPU 全面排名。 |
| `asb-sysbench-memory-v1` | 顺序 write/read 分开；`memory-scope=local`，threads=1、time=0；block=1 KiB/64 MiB 各成组合，total=1 GiB，hugetlb=off | 工具 MiB/s 与总进程时间；记录实际缓存/架构信息。1 KiB 很可能主要反映缓存，64 MiB 也不能未经机器条件核实就称 DRAM 带宽。 |
| `asb-fio-sequential-v1` | Sandbox 工作目录的单一普通文件，size=512 MiB、bs=1 MiB、ioengine=sync、iodepth=1、numjobs=1；write/read 分离，direct=0/1 分离，write 使用 end_fsync=1；校验另列 | fio BW、完成时延与整体进程时间；明确缓存/同步口径。direct 不支持时记 unsupported，不静默退回 buffered，不使用宿主裸设备。 |
| `asb-fio-random-v1` | 已完整填充的 512 MiB 文件，bs=4 KiB，ioengine=sync、iodepth=1、numjobs=1、direct=1、固定 seed、number_ios=8192；randread/randwrite 分开，数据验证在计时段外 | 固定 QD1 下 IOPS、完成时延分布和实际字节量；本版不称存储最大能力。缓存持久性和更高队列深度另设变体，不能用提交深度冒充实际深度。 |
| `asb-iperf3-controlled-v1` | 独立受控主机作为服务端；固定地域/路由/IP 族/MTU，TCP，P=1/4 各组合；每方向独立测量 15 秒，另加 omit=3 秒预热；保留两端 JSON 和 CPU，默认不启用 zerocopy | 接收/发送吞吐、TCP 重传、双端统计及完整实际进程。可测该 Sandbox 到该受控端点的路径，不能单独归因 Sandbox 内核；端点负载或链路饱和需标注。跨时钟无需相减，两端各按自己的区间统计。 |

这些参数是项目拟定的初始实验，不是上游推荐配置。sysbench 官方提供 CPU/内存测试以及事件和时延统计；CPU 测试本身仍是特定素数算法。参数依据：[官方 README](https://github.com/akopytov/sysbench/blob/master/README.md)、[CPU 实现](https://raw.githubusercontent.com/akopytov/sysbench/master/src/tests/cpu/sb_cpu.c)、[内存实现](https://raw.githubusercontent.com/akopytov/sysbench/master/src/tests/memory/sb_memory.c)。

fio 官方明确 buffered/direct 的区别，并提醒同步 engine 的深度不能仅靠增大 iodepth 提高；异步实验也应核对实际 depth 分布。后续使用它报告 IO 模式下的测量，而不把本例 Python 开销称为磁盘极限。[fio 官方参数和结果说明](https://fio.readthedocs.io/en/latest/fio_doc.html)

iperf3 由 ESnet 开发，用于主动网络测量；支持方向、并发流、JSON 和省略起始预热段。其双端输出分别来自各自进程，客户端的 server output 仅在测试完成后可取得；中断测试仍需保留已获得的部分证据。[iperf3 官方介绍](https://software.es.net/iperf/)、[参数与输出语义](https://software.es.net/iperf/invoking.html)

## 本轮交付边界

当前交付分两轴：六例原 TB 的公开题目、环境输入、私有原评分已构造，真实 Agent/Sandbox 执行仍为 UNRUN；ASB 与标准工具是辅助设计，exec-short 单独有 ECS 本机功能检查，真实调用与性能资格仍未验证。官方资料核对不改变固定来源，也不构成官方认证。后续先完善真实 Agent 用例和冻结环境，不因材料齐全就宣称已有 Sandbox 性能结论。
