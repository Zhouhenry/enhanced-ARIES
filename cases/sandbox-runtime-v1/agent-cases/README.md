# 真实 Agent 任务材料 · revision 3

**material_revision=3；CONSTRUCTED_UNRUN / NOT_EVALUATED 是材料构造时的快照，不是当前运行状态。** 现有六例要求 Agent 自主解决问题、调用真实 Sandbox 工具；不预设工具计划或参考解轨迹。第三版新增小型 C++ 项目调试任务 `custom-memory-heap-crash`，原五例定义和 36 个源文件保持不变。本文未运行的设计负例及历史准备清单也按构造时记录保留。

**后续真实执行请看 [E2B 六题逐次结果与阶段性能](../../../docs/tb-six-performance-20261026.md) 和 [华为 C++ 准备与运行归档](../../../docs/tb-huawei-cpp-performance-20261026.md)。** 功能评分、完整 Strict 回放、在途任务和可比较性能分别判断，不从构造状态推断全部未运行，也不从环境准备通过推断 Agent 成功。

[index.json](index.json) 是本目录清单：6 个 `original_benchmark`，0 个 `custom_agent`。来源是 [Terminal-Bench 2.1](https://github.com/harbor-framework/terminal-bench-2-1/tree/7131e4375048a0e408a8fb404b5f499d726b695b) 的固定 commit `7131e4375048a0e408a8fb404b5f499d726b695b`，许可见 [LICENSE](LICENSE)。这是原始任务选集，不能将六例成绩称为完整官方基准成绩。第二版来源保留在 [revision-2-source-manifest.json](revision-2-source-manifest.json)；本次新增与旧 36 文件未变证明见 [revision-3-source-manifest.json](revision-3-source-manifest.json)。

## 文件怎样交接

| 路径 | 用途 | Agent 是否可见 |
| --- | --- | --- |
| `<task>/public/instruction.md` | 原始题目，逐字节保留 | 是 |
| `<task>/environment/` | 原 Dockerfile 与最小构建输入 | **只用于构建**；仅其产生的原运行环境可见，不能整个目录上传工作区 |
| `<task>/task.toml` | 原 schema 1.1、镜像 tag、资源、超时和作者信息 | 控制端元数据，不作为额外提示 |
| `<task>/private/tests/` | 原独立评分入口和断言 | **执行期间不可见**；仅在确认 Harness 停止、Bridge 撤权后按既有方法注入 `/tests` |
| `index.json` | 每个文件的来源/hash、接入限制及验收设计 | 研究/控制端归档 |

没有复制原 `solution/solve.sh`、透露解法的原 README、suite 执行器或历史测试结果。这里使用分离目录而非 loader 标准任务布局，**不能直接作为 ARIES Profile 加载**。现行 `terminalbench2` 云入口已支持 E2B / Huawei 的 External 单任务运行，使用固定原始 checkout、独立环境 binding 和原任务预算，详见 [云运行指南](../../../profiles/cloud/terminalbench/README.md)。只准备公开运行环境，确认停止与撤权后才注入原私有评分，不能把整个材料目录同步给 Agent。

## 六例任务与原评分

### fix-git

Agent 获得[原题目](fix-git/public/instruction.md)和 `/app/personal-site` 仓库，自行调查丢失的更改并恢复。可以使用原生 Git、文件和 shell 工具；不规定命令数、步骤顺序或最后一条命令。

原构建在准备期 clone 站点仓库，切到指定历史状态，生成 detached-HEAD 更改后返回 master。`environment/resources/patch_files/` 是构建所需 fixture；原 Dockerfile 随后删除 `/app/resources` 和 `setup.sh`，不得将这些构建输入额外提供给 Agent。候选保存完整工作仓库和实际工具记录。

原 Verifier 有 **2 个文件内容断言**，产出 reward/CTRF。它验证两份目标文件的最终内容，不独立证明合并 ancestry 或命令历史，不新增“最后必须 pytest”的条件。设计负例为漏恢复一个更改、目标内容错误；尚未执行。它适合观察 Agent 的 Git 调查/修复工作流，不能充当固定 Git 命令或文件系统微基准。

### cancel-async-tasks

Agent 根据[原题目](cancel-async-tasks/public/instruction.md)自主实现 `/app/run.py` 的 `run_tasks`，处理并发限制和取消后的清理。保留代码编写、调试和真实测试过程，不提供实现骨架答案或固定解法。

原 Verifier 有 **6 个测试函数**，覆盖文件存在、并发执行、最大并发和 3 种 SIGINT 清理情况。`private/tests/test.py` 是评分 fixture，原入口仅在评分时复制到 `/app/test.py`。设计负例为不限制并发、取消后不完成清理；尚未执行。

这些断言测的是候选 Python 函数行为，**不是 ARIES Runner 的停止或云 Sandbox 销毁能力**。原测试含真实 sleep、子进程超时和 elapsed-time 条件；其耗时属于 Verifier，不能合入 Agent 工具耗时或当纯调度性能分数。

### db-wal-recovery

Agent 获得[原题目](db-wal-recovery/public/instruction.md)、原始数据库和待调查 WAL，自行检查并生成 `/app/recovered.json`。原 Dockerfile 将 `environment/main.db` 放到 `/app/main.db`，将 `main.db-wal.encrypted` 原字节放到 `/app/main.db-wal`；Prepare 不解密、不提前生成恢复结果。

原 Verifier 有 **7 个测试函数**，检查 JSON 结构、排序、ID/条数及选定数据值。保留原断言，预期值只在私有 tests 中；原检查并非对每个字段进行完整数据库等值比较。设计负例为只恢复基础记录、遗漏所检查 WAL 更新或重复 ID；尚未执行。

功能通过证明原断言成立，不独立证明采用了何种解密方法或发生了哪些 IO。真实工具与进程证据另行记录。该任务适合观察 Agent 数据恢复工作流；小体量输入与自主命令不能直接形成磁盘带宽、IOPS 或崩溃耐久性结论。

### sqlite-with-gcov（revision 2 新增）

Agent 根据[原题目](sqlite-with-gcov/public/instruction.md)自主配置和编译 SQLite，要求在 `/app/sqlite` 使用 gcov 并使 `sqlite3` 可从 PATH 调用。保留原 Ubuntu 基础与预置源码，不提前提供构建方案或编好的答案。原环境没有预装完整编译链；后续 ARIES 接入已将通用构建工具作为显式适配预置，Agent 仍须从原 SQLite 源码配置、编译和安装。该环境差异与准备耗时独立归档，不能称原封不动的官方环境，也不把预置工具的成本计作 Agent 编译工作量。

固定原 SQLite 源码包 **12,640,606 bytes**，SHA-256 `7c02b6cc04dddc2f90e63d43ab8900cb694ba4665f4e4ac877ec724733125229`；已与同 commit 的原 sidecar 和 Git blob 核对。只读包内元数据为 SQLite **3.50.4**，Fossil manifest `4d8adfb30e03f9cf27f800a2c1ba3c48fb4ca1b08b0f5ed59a4d5ecbf45e20a3`。源码及其 Public Domain / 构建工具 BSD 类许可完整保留在原压缩包内，没有运行或构建；不能将包内全部第三方文件笼统标成 Apache-2.0。

原 Verifier 有 **3 个测试函数**：运行 SQLite 查询、检查 PATH，以及检查 `/app/sqlite` 下 `.gcda` / `.gcno` 存在。原评分不完整核实源码版本、编译 flags 或覆盖文件与可执行文件的对应关系；**这些文件存在不等于证明真实编译**。保存既有真实编译工具/guest 进程与源文件/二进制身份后，才能解释构建阶段的 CPU、进程和文件 IO。Verifier 的查询和文件扫描单列。设计负例为 PATH 找不到可用 SQLite、缺任一种覆盖文件，均未执行。

### large-scale-text-editing（revision 2 新增）

Agent 根据[原题目](large-scale-text-editing/public/instruction.md)自主制作和调试 `/app/apply_macros.vim`，用三个受限 Vim 宏处理一百万行 CSV。**`input.csv` 与 `expected.csv` 都是原任务公开输入**；原 Dockerfile 在构建时生成二者并移除生成器，不应误把公开示例目标删掉。保留原宏约束，不能用 pandas 代替而仍宣称是原题。

原 Verifier 有 **5 个测试函数**：检查脚本/允许命令，实际运行 headless Vim，检查全文件 SHA-256，并验证三个不同且非空的宏、总按键数小于 200。原入口先删除候选 CSV、重建干净输入；运行候选宏后再生成评分预期。因此大文件重建、规定的完整 Vim 变换与哈希比较属于 **Verifier 阶段**，不能用它们证明 Agent 阶段做过相同工作或算入 Agent Tool 时长。

它增加自主编辑/调试与大文件处理覆盖，但仍需真实调用观测确认 Agent 实际执行量；不等同于磁盘吞吐或 IOPS。设计负例为缺失/非法宏、变换结果错误、按键数超限，均未执行。原入口在 `cd /app` 前清理 CSV，未来接入应保持并核对原 `/app` 评分工作目录；本轮不改原脚本。

### custom-memory-heap-crash（revision 3 新增）

Agent 根据原题自主修复小型 C++ 项目的 release 崩溃，保留原程序、双配置编译和文件修改约束；不是生产仓库 issue 或内存微基准。详见 [独立用例说明](custom-memory-heap-crash/README.md)。新增 10 个原文件共 23,096 bytes，没有下载 GCC/镜像或构建两套 libstdc++。

原 Verifier 的 **6 个测试函数**包含两份受保护文件检查、debug/release 编译运行与 Valgrind。源码中的 **5 次编译属于评分**，不能冒充 Agent 工作量；原评分不完整检查应用输出、所有系统文件或全部泄漏类别。保留原断言，真实工具/guest 进程、环境构建与评分各自观测；后续执行状态见文首实际运行归档。

## 真实性能样本如何形成

六例都没有固定 ToolCall 数，记录 Agent 实际选择的每次 native 调用、后台/process 轮询、进程终态和失败。复用现有 P0 身份、时钟和版本化分析快照；不要按命令文本或时间近邻猜关联，也不为内部每个 syscall 新建硬门槛。

主结果为 Agent E2E、各阶段耗时、有独立验证的任务吞吐；模型等待、工具/进程耗时和资源观测按实际覆盖补充。环境准备、执行、停止隔离、评分、清理分别保留。功能评分通过、Harness 成功和可用于比较的性能样本是不同结论；缺测与隔离假设如实标注，沿用现有 Goodput 口径。

实时模型比较须固定模型配置、任务/镜像/资源、云区域与运行端位置、冷热条件和 placement，保留所有尝试并报告任务质量和工作量变化。Strict Replay 仅从真实自主 Agent 多轮轨迹录制；同 Tape 在其他云仍严格检查完整历史和 ToolResult，不由参考解生成 Tape。A/B 分组，不能把不同执行路径直接解释为云厂商差异。

六例只是初批真实工作流，不能代表 CPU、IO、网络性能全谱或生产仓库修复覆盖。`custom-memory-heap-crash` 的双 libstdc++ 环境在材料构造时尚未构建，后续准备结果见实际运行归档；SWE-bench Pro 仍缺完整 task row，不据 smoke ID 伪造材料。

## 构造时的接入检查清单（历史）

以下保留材料构造时的缺口，不代表当前全部待办。后续完成情况与失败保存在文首两份运行归档，现行接入方法见云运行指南。

1. **冻结环境。** 六例原声明均为 1 CPU、2048 MiB 内存、10240 MiB 存储，build 超时 600 s；`large-scale-text-editing` 的 Agent/Verifier 超时为 1200 s，`custom-memory-heap-crash` 为 1800 s，其余为 900 s。这些不代表云端已经兑现。原任务镜像和基础镜像只有 tag，digest 未确定。`fix-git` 还需冻结 clone 的实际仓库状态、生成提交元数据及最终镜像，避免构建日期影响 Strict Replay。
2. **准备离线评分依赖。** 六例原 `test.sh` 现场 apt 安装 curl，再下载 uv 0.9.5、Python 3.13、pytest 8.4.1 与 pytest-json-ctrf 0.3.5。当前只保留原文件；将来须在评分前固定可用依赖，才能在禁止出网条件下保留原评分语义，不可把联网失败记为候选失败或声称隔离已验证。
3. **接入和真实执行。** 公开输入按原路径准备，私有 tests 在现有停止/撤权门后注入；分别保存候选、reward、CTRF、真实调用/进程和清理证据。当前没有执行这些步骤，没有成绩、Tape 或性能样本。

共有 **46 个原始来源文件（含共享许可证）和 29 个原测试函数**。revision 1 的 22 个文件、revision 2 的 14 个文件仍匹配原 hash；revision 3 新增 10 个白名单文件直接来自同官方 commit，大小与 Git blob 全匹配，并记录 SHA-256。SQLite 源包另匹配原 checksum sidecar。未下载参考解或原解法 README；Vim 环境中未被 Dockerfile 引用的重复 tests 没有复制。canary、原内容和包内许可证保留；静态核对不代替构建和运行。
