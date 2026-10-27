# custom-memory-heap-crash：小型 C++ 项目自主调试

**状态：CONSTRUCTED_UNRUN；引入 material_revision=3。** 本例保留固定版 Terminal-Bench 的原题、原环境源码与 6 个原测试函数，要求 Agent 自主分析和修复程序。这里只构造材料；没有下载 GCC 或镜像，没有构建、运行 Agent、执行评分或取得 Sandbox 性能结果。

来源为 [原任务固定 commit](https://github.com/harbor-framework/terminal-bench-2-1/tree/7131e4375048a0e408a8fb404b5f499d726b695b/tasks/custom-memory-heap-crash)，任务材料沿用仓库 [Apache-2.0 许可](../LICENSE)，canary 和作者信息保留。新增 10 个源文件共 **23,096 bytes**；逐文件官方 blob、大小与 SHA-256 见 [revision 3 来源清单](../revision-3-source-manifest.json)。GCC 和生成的库有各自许可，本材料没有打包或重授权这些外部依赖。

## Agent 实际要做什么

[原题目](public/instruction.md)给出一个 debug 能运行、release 会崩溃的 C++ 程序，只允许修改 `/app/user.cpp`，并提供原始编译方式。Agent 自行检查 `/app` 与原环境中的库源码、修改、编译、运行和调试；不提供答案、固定探索顺序、指定调用次数或模型 Tape。原生后台、轮询、PTY 等工具能力保持可用。

这是原基准设计的小型项目调试任务，**不是生产仓库的真实 issue 修复，也不是内存微基准**。它增加多文件阅读、双配置编译/链接、进程执行和诊断工具的工作流覆盖；Agent 是否实际执行这些步骤，以及执行多少次，都要以后续真实调用记录为准。

## 材料边界

| 路径 | 用途与可见性 |
| --- | --- |
| `public/instruction.md` | 原公开题目，给 Agent |
| `environment/program/{main.cpp,user.cpp,user.h}` | 原程序；按原 Dockerfile 准备到 `/app`，没有替换为修复答案 |
| `environment/Dockerfile`、`environment/patches/` | 原构建输入；不要把整个构建目录作为额外题目上传 Agent 工作区 |
| `task.toml` | 原控制端资源、超时、镜像 tag 和作者信息，不作为额外提示 |
| `private/tests/test.sh`、`private/tests/test_outputs.py` | 原私有评分；仅在确认 Harness 停止和 Bridge 撤权后按既有方法注入 `/tests` |
| 本 README、总索引和来源清单 | 研究者交接材料，不能替代公开题目 |

没有下载 `solution/solve.sh` 或透露解法的原 README。当前分离目录不可直接作为 ARIES Profile 加载，也没有改动 Benchmark/Runner 接口。

## 原 Verifier 验证什么

6 个测试函数检查：两份受保护文件内容、debug 编译、release 编译、debug 运行、release 运行，以及 release 程序的 Valgrind 检查。两份原程序文件的 MD5 已与原测试中固定值做静态核对，源文件未被修改。

源码显示，各相关测试体顺利执行到对应位置时，总计会发生 **5 次评分编译（debug 2 次、release 3 次）**，另有 2 次普通程序运行和 1 次 Valgrind 运行。这是**原测试的静态调用结构**，不是实测次数、Agent ToolCall 数或 Agent 的编译工作量。任何失败或未执行都应保留，不把计划值补成完成值。

保留原评分，同时明确限制：

- 受保护文件校验只覆盖 `main.cpp` 和 `user.h`，没有逐项证明系统中其他文件都未被改动。
- 编译和运行断言主要检查退出状态，不核对应用 stdout 或数值结果；通过不能独立证明所有预期计算均已发生。
- Valgrind 的泄漏错误类别设置为 `definite`，不能据此宣称所有泄漏类别都不存在。
- 原评分没有认证 Agent 此前的分析、修改、编译与调试历史；实际工具/进程观测另存，不由 reward 推导。

设计负例为修改受保护文件、候选编译失败、release 崩溃或明确泄漏；均未执行，也没有实施或验证绕过原评分的方法。原测试不在本轮增删或放宽。

## 怎样用于 Sandbox 性能分析

同一 Agent 任务的 E2E 包含模型和 Harness。把真实模型等待、Agent native 工具/guest 进程、环境准备、独立评分与清理分别计时，复用现有准确身份和时钟边界，不凭最终文件或命令文字猜测过程。

原环境构建会下载 GCC 13.2.0，打补丁并构建两套 libstdc++；这些属于环境构建成本。Agent 自主执行的编译与调试属于 Runtime。原 Verifier 再次编译、运行和 Valgrind 属于 Evaluate，不能计为 Agent 工具耗时。程序包含 10 MiB 堆与固定种子计算，也不能把该任务通过率或运行时间说成纯内存带宽、分配器吞吐、CPU 极限或 Sandbox 调度性能。

实际比较需固定原任务和初始环境、镜像 digest、g++/Valgrind 版本、架构/资源、云区域和运行端位置、冷热与观测等级；A/B placement、真实模型与 Strict Replay 分组。真实模型允许不同调试路径和代码，记录差异；同 Tape 实验必须先真实录制，再严格回放、执行真实工具并独立验证，不能用参考解构造轨迹。录制与回放各保留完整尝试分母。

## 尚待实际接入核实

原 `task.toml` 声明 1 CPU、2048 MiB 内存、10240 MiB 存储，Agent/Verifier 各 1800 s，build 600 s。原镜像 `alexgshaw/custom-memory-heap-crash:20251031` 仍仅为 tag，digest 未冻结；双 libstdc++ 环境的实际体量、构建可行性和耗时未知，不能因材料只有 23 KB 就把运行环境当轻量任务。

原评分入口还会安装 curl/uv 并解析 Python/pytest 依赖。将来需在禁止出网评分前准备固定依赖、确认停止撤权、保存候选与 reward/CTRF/真实执行证据，并清理所属资源。本轮没有执行这些步骤，也没有云资源需要回收。
