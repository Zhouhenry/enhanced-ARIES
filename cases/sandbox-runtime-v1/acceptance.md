# ASB 辅助负载验收卡 v1

**定位：辅助测量，非真实 Agent 主用例。** 主线六例自主 Agent 任务见 [agent-cases](agent-cases/README.md)。本文保留 [cases.json](cases.json) 的 10 个 ASB 设计，参数和来源哈希不变。设计状态仍为 `DESIGNED_UNRUN`；另有 exec-short 构造材料的 ECS 本机功能检查，见其 [证据说明](exec-short/README.md)，不计为 Agent/Sandbox 性能样本。没有运行参考 suite 或集成这些负载。

## 共同约定

公开输入只包含任务、固定参数、种子 `20261024` 和运行所需源码/数据；私有断言、预期值与参考解不放入 Agent 工作区。每例使用新工作目录。以下“负例”是待构造的独立候选副本，不能修改正常运行的产物来凑通过。

工具调用指 Agent 发出的真实 native 调用。程序内部的循环、进程、SQL 和 HTTP attempt 均不自动算作 ToolCall。除 `exec-short` 明确规定 N 次顺序提交外，其他用例允许一次 native 调用启动固定负载；实际调用、辅助命令和原生 process 轮询按观测分别记账，不预造调用数。保留后台、自动 yield、process 与 PTY 能力。

验收分开记录两件事：独立 Verifier 是否确认产物正确，以及既有观测实际覆盖了哪些执行过程。固定代码/输入、真实调用与终态、独立功能校验及一致环境条件，可支持程序级耗时比较；不要求每个循环或 syscall 新增测点。自报次数、耗时、RSS 或同权限服务日志仅供一致性核对，不能单独证明执行历史。缺测细节标记 `UNMEASURED`，不补零，也不据此否定已成立的程序级测量。

所有私有验证在 Harness 停止和 Bridge 撤权被确认后进行。ASB 候选由可信侧封存；需要执行候选二进制时使用独立验证 Sandbox 的受限用户，报告由可信侧写入。Prepare、Run、Verifier、Cleanup 分开；最后保存候选和日志、回收所属进程及 Sandbox。原始 TB 保留其原评估方法。

跨云比较固定 case、档位、代码/输入哈希、镜像/工具版本、架构和资源限制；A/B placement 分开，真实模型和 Strict Replay 分开。未对齐的时钟域不相减。未来回放只使用真实录制并严格匹配的 Tape；更改参数不能沿用另一工作量的 Tape。下面是验收设计；除了 exec-short 文档明确列出的 ECS 本机功能检查，其他验收与真实 Agent/Sandbox 测量仍未执行。

## 已细化的 5 个辅助负载（保留原设计优先级）

### asb-exec-short-v1

- **公开输入：** small/medium/large 的 `calls=24/96/384`，空 `steps/`、单步 fixture 与逐条计划。第 0 步读 `START`，其后读取前一步文件，按 catalog 的 SHA-256 链公式写 `steps/{i:06d}.txt`，独占创建且无换行。
- **真实调用：** N 次有前后依赖的 native exec，各执行一步。A 要有 N 条精确 native→SSH→Bridge→SDK→guest 身份及终态；B 要有 N 组实际 ToolCall/run/session 和 native/process 身份及终态，不强加 A 的 invocation ID。准备和轮询另计；一次 shell 内循环 N 步不能冒充 N 次提交。
- **候选产物：** 恰好 N 个步骤文件、实际退出码和 stdout、既有原始调用证据。参考 `exec-events.json`/`result.json` 仅为回执。
- **私有验证：** 从可信 seed/N 独立重算完整链，核对精确路径集合、逐文件内容、退出 0 和 stdout 的 index/digest；不导入候选单步实现。另将预登记 N 与实际调用身份/终态逐项对账。
- **负例：** ① 删除或修改中间步骤，功能拒绝。② 单次 shell 循环产出完整链，功能可能正确，但 N 次调用要求失败。③ 文件正确却少一条身份或终态，观测不完整，不能补关联。
- **可比较范围：** 同 placement、同 N 的实际工具往返、已有 guest/process 时间及完整任务耗时。A 的 Bridge 往返与 B 的原生工具时长分别展示；不从跨时钟差值推算网络开销，不声称未观测进程树均已退出。

### asb-small-files-v1

- **公开输入：** `(files,bytes,fanout)` 为 `(128,1024,8)` / `(1024,4096,32)` / `(4096,4096,64)`；空 `files/`。字节模式使用 catalog 的 SHAKE256(seed,index,revision)，公开运行代码，私有预期不随包分发。
- **真实调用：** native 执行固定负载：创建目录和 `.bin` 文件，完整读回 revision 0，改写 revision 1，再改名 `.done`。四类文件操作各 `files` 次是程序工作量，不是 4×files 个 ToolCall；读字节为 files×bytes，写字节为 2×files×bytes，无逐文件 fsync。
- **候选产物：** `files/d{i%fanout:03d}/f{i:06d}.done` 全集合及回执。
- **私有验证：** 核对精确路径集合、每文件全部 revision 1 内容、固定参数及字节量回执；不得只抽样文件数或 hash 回执。
- **负例：** ① 删文件或改一个字节，功能拒绝。② 多文件或残留 `.bin`，功能拒绝。③ 只生成最终 `.done`，不能据正确终态声称四类历史操作均发生。
- **可比较范围：** 同档位固定程序的批量文件工作耗时、已有资源读数。新目录不等于冷缓存；不报告逐 syscall 时延、物理写入量或删除吞吐，删除属于 Cleanup。

### asb-xxhash-build-v1

- **公开输入：** xxHash `v0.8.3`、commit `e626a72bc2321cd320e953a0ccf1584cad60f363`，catalog 固定的 C/H 哈希和许可证、调用标准公开向量的 `probe.c`；`rounds=1/2/4`。预备固定 gcc/libc/架构，不在 Run 联网安装。
- **真实调用：** 每轮真实编译 C 到新 `.o`、链接新 probe、执行 probe。工作量为 2×rounds 次编译/链接与 rounds 次探针进程，不据此杜撰相同数量的 ToolCall；禁用构建缓存复用以保持此用例定义。
- **候选产物：** `source/xxhash.{c,h}`、`probe.c`、每轮 `xxhash-r.o` 和 `probe-r`、`build-probes.json` 及版本/轮次回执。
- **私有验证：** 核对源哈希、每轮文件存在且为 ELF、回执与公开标准向量一致；在隔离验证 Sandbox 中实际运行封存 probe，由可信侧比较输出。候选不能在 controller 或 root 身份下执行。
- **负例：** ① 改源、缺一轮对象或提交非 ELF，功能拒绝。② 探针错误向量或运行失败，功能拒绝。③ 复制旧 binary 可以形成正确终态，但不能证明本轮实际编译；超时保留错误，不能跳过该轮。
- **可比较范围：** 固定编译链的整体构建/工具时间及已有 CPU/内存观测。未采逐子进程时只报整体；不称 xxHash 哈希吞吐，不以 ELF 字节相等作为跨云必需条件，不推断缓存命中或编译器内部阶段。

### asb-sqlite-etl-v1

- **公开输入：** `(rows,batch)` 为 `(2000,200)` / `(20000,1000)` / `(100000,1000)`；CSV 表头 `id,grp,value`，第 i 行 `(i,i%97,(i*17+seed)%100003)`；起始无数据库/WAL/SHM。
- **真实调用：** native 执行固定程序，WAL/FULL 下分批 INSERT/commit，建 `idx_group`，更新 `id%3=0` 的值并 commit，再分组聚合、checkpoint、close。10/20/100 是插入批次数，不包括后续 commit，也不是 ToolCall 数。
- **候选产物：** `input.csv`、一致的 `data.sqlite` 与实际必要 sidecar、`aggregates.json` 和参数/版本/事务回执。
- **私有验证：** 只读重开库，要求完整性检查通过；逐行核对输入公式及更新结果，核对精确行数、按组独立聚合和索引存在，参数从可信 catalog 读取。
- **负例：** ① 删行、改值或漏更新，功能拒绝。② 改聚合结果或缺索引，功能拒绝。③ 使用 OFF 等模式构造相同终态，不能据最终库证明 WAL/FULL 或每批 commit 的历史；缺证据项保持未测。
- **可比较范围：** 固定 ETL 全流程和实际工具时间；同时固定 SQLite/文件系统条件。不同档位改变 rows 和 batch，不能仅归因于行数；不声称逐 SQL/fsync 时间、物理持久化次数或崩溃耐久性。

### asb-http-retry-v1

- **公开输入：** `(requests,payload_bytes,concurrency,fail_first,max_attempts)` 为 `(8,4096,2,1,3)` / `(24,16384,4,2,3)` / `(48,65536,8,2,4)`。独立服务进程仅监听 loopback，永久失败 ID=requests−1，每次清空计数；Prepare 只准备文件/配置。
- **真实调用：** native 启动固定负载并管理服务启动、就绪和停止；普通 ID 真实经历 fail_first 次 503 后 200，最后 ID 达到上限仍 503。每次 HTTP attempt 新 TCP、无退避；预期 attempt 总数 17/72/145，HTTP attempt 不是 ToolCall。服务管理计入 Run，request phase 单列。
- **候选产物：** `http-events.json` 完整尝试序列、`server-events.jsonl`、ready 回执与 result；503 正文为公开的 `retry`，200 正文按 catalog 的 SHAKE256(seed,id) 公式生成。
- **私有验证：** 精确 ID 集合、连续 attempt、状态序列、字节/hash 和 outcome；核对双端多重集合及总数。成功请求为 7/23/47，耗尽为 1；用例功能可通过，但不能把耗尽请求计为成功请求吞吐。
- **负例：** ① 最后 ID 提前停止、伪 200 或删除一次 503，功能拒绝。② 普通 ID 失败、内容损坏或双端不一致，功能拒绝。③ 服务不可达/未知状态是执行或基础设施错误，不能当作预期耗尽。
- **可比较范围：** 同档位的 loopback 重试程序耗时、实际工具和进程边界。双方同 Agent 权限的日志仅作一致性核对；不宣称独立可信逐请求历史、跨主机带宽、DNS/TLS、连接复用或退避性能。

## 其余 5 个用例

### asb-cpu-primes-v1（P0）

- **输入/调用：** `(limit,rounds,workers)` 为 `(12000,2,1)` / `(40000,4,2)` / `(100000,8,4)`；native 启动固定试除程序，进程池分发 rounds 轮，包含进程池启动。候选整数数 rounds×(limit−1) 不等于实际除法数或 ToolCall 数。
- **产物/私有验证：** `answers.json` 恰为 rounds 组 count/sum；私有侧用独立埃氏筛重算，核对固定参数及候选数回执。
- **负例：** ① 修改 count/sum，拒绝。② 少一轮，拒绝。③ 复制正确答案不能证明各轮实际计算。
- **比较边界：** 同档位完整固定程序的 CPU 工作耗时与已有资源采样；不是 sysbench 分数，不报告未采的各 worker CPU/试除次数。三个档位同时改变工作量和 worker 数，不能当固定工作量强扩展实验。

### asb-memory-touch-v1（P0）

- **输入/调用：** `(mib,passes)` 为 `(8,3)` / `(32,5)` / `(128,8)`；native 真正分配 bytearray，每轮按 `(seed+r)%251` 全量填充并 CRC 扫描，最终保存前 4096 字节后释放。内部填充/扫描次数不算 ToolCall。
- **产物/私有验证：** `checksums.json`、`memory-sample.bin`、参数与资源回执；私有侧分块增量重算各轮 CRC，核对末轮样本和字节量。自报 RSS 阈值只作回执一致性，后端采样另列。
- **负例：** ① 错 CRC 或少一轮，拒绝。② 样本使用上一轮字节，拒绝。③ 伪造正确 CRC/样本/RSS 不能证明实际分配触达。
- **比较边界：** 固定内存程序耗时及已采 CPU/内存；不称 STREAM/内存带宽，不推断逐页触达、NUMA、分配历史或未采峰值；self/children maxrss 不能直接相加。

### asb-sequential-io-v1（P1）

- **输入/调用：** `(mib,block_kib)` 为 `(8,256)` / `(32,1024)` / `(128,1024)`，起始无文件；native 完整 buffered 写入→flush/fsync→读回 SHA-256，读写各 size 字节。
- **产物/私有验证：** `sequential.bin` 与回执；按公开块公式重算全部内容、精确长度及整体哈希，不只相信候选 hash。
- **负例：** ① 截断或追加字节，拒绝。② 修改任一块或回执 hash，拒绝。正确终态本身不证明实际 fsync/读回。
- **比较边界：** 同档位程序级 write/read-after-write 工作耗时；guest 自报分段明确标注。没有冷缓存、裸盘/物理带宽或独立 fsync 时延结论，不操作宿主缓存或设备。

### asb-random-io-v1（P1）

- **输入/调用：** `(blocks,block_bytes,passes)` 为 `(256,4096,2)` / `(2048,4096,3)` / `(8192,4096,4)`；Prepare 写满 revision 0 并 fsync，已预热。native 每轮对上一轮排列按 seed+r 原地 shuffle，逐块读校验并写下一 revision，轮末 fsync；QD1、OS buffered。
- **产物/私有验证：** `random.bin`、逐轮排列摘要和回执；独立核对全部最终块 revision=passes、精确长度、按同一原地排列规则重算摘要。读写各 blocks×passes 次是程序工作量。
- **负例：** ① 旧 revision 或截断，拒绝。② 每轮重置排列造成摘要错误或缺一轮回执，拒绝。
- **比较边界：** 同档位、同 Python/缓存条件的固定程序时间；无独立块级观测则不报块级 IOPS/时延或物理 IO。Python unbuffered 不等于 O_DIRECT。

### asb-http-transfer-v1（P1）

- **输入/调用：** `(requests,payload_bytes,concurrency)` 为 `(12,32768,2)` / `(48,131072,4)` / `(128,262144,8)`；loopback 独立服务进程，fail_first=0、无永久失败。native 管理完整负载；每 ID 恰一新 TCP 请求，HTTP 请求数不是 ToolCall 数。
- **产物/私有验证：** 客户端完整内容回执、server/ready/result；逐 ID 验 200、succeeded、公开公式生成的字节/hash，核对双端多重集合和 attempts=successes=requests。
- **负例：** ① 缺 ID 或损坏正文，拒绝。② 多 attempt、错状态或双端不一致，拒绝。一致伪日志不证明真实传输历史。
- **比较边界：** loopback 固定传输程序耗时；Run 包含服务管理，request phase 不含。无公网/跨 host、DNS/TLS、keep-alive 或链路带宽结论；跨 host 另建用例变体。

## 真实 Agent 主线材料

原始 `fix-git`、`cancel-async-tasks`、`db-wal-recovery`，revision 2 的 `sqlite-with-gcov`、`large-scale-text-editing`，以及 revision 3 的 `custom-memory-heap-crash` 已整理为 [独立材料目录](agent-cases/README.md)，状态 `CONSTRUCTED_UNRUN`。六例使用原题目、原环境输入和原 2 / 6 / 7 / 3 / 5 / 6 个测试函数，共 29 个；Agent 自主分析与用工具，不采用 ASB 的固定负载计划。机器可读来源/哈希、公开与私有边界、负例设计和原评分限制见 [index.json](agent-cases/index.json)。六例不并入 ASB 计数，不代表已运行或官方成绩。

## 来源与交接状态

ASB 验收条件依据既有 catalog，以及参考副本 `lib/exec_step.py`、`lib/workloads.py`、`lib/verifier.py` 的只读核对。参考路径相对 `aries-sandbox-suite-1.0/`；完整性证据见 [README](README.md)。原始 TB 当前共 46 个来源文件（含共享许可证），已单独下载/复制与核验，没有复制参考解或 suite 执行器。

本文件仍为 10 张辅助验收卡、30 档参数和每例至少两项负例设计。exec-short 的有限 ECS 本机功能结果不替代其中的真实调用要求；其余 ASB 及六例自主 Agent 任务都没有本项目执行结果。后续用例工作以真实 Agent 任务为主，受控负载保留为辅助定位工具。
