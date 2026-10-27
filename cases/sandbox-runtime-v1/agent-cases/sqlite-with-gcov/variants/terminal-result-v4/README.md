# SQLite：terminal-result-v4 原生日期显示环境变体

这是独立比较环境，原任务仍为 `sqlite-with-gcov`。2026-10-26 的一次真实 E2B 录制为 **FAIL：原评分 2/3、Tape 不完整**；未启动 Strict 自回放及跨云回放。详见[独立失败与性能记录](../../../../../../docs/sqlite-v4-performance-20261026.md)。材料与本地镜像探针不算任务通过，v1/v2/v3 的材料、Tape、评分和失败原件保持不变。

相对 [v3](../terminal-result-v3/README.md)，公开附录只改标题并增加一段环境说明：[精确差异](appendix-v3-v4.diff)。新 binding 默认设置 `command_env.TIME_STYLE="+"`，让 GNU `ls` 使用空日期格式。文件 mtime、系统时钟、真实命令、错误、ToolResult 和 Strict matcher 均不被后处理。显式单调用环境、`--time-style` 和 `stat` 仍可查看真实日期；该默认影响此 binding 下继承值的 GNU `ls`，包括准备/验证命令。

原因是原 SQLite 3.50.4 `main.mk` 安装规则自动 `ls -la` 三项新产物，旧 v3 命令又以 `tail -25` 读回日志，导致三个时间字段进入请求历史。原安装退出 2、外层 shell 因末尾 tail 成功退出 0 的区别保留，详见[只读归因](../../../../../../docs/sqlite-v3-mtime-rootcause-20261026.md)。本 v4 不提供安装步骤或修补 Agent 命令，也不保证其他动态正文确定。

原题七个文件、私测、900 秒 Agent/Verifier 预算、1 CPU/2048 MiB、原评分 3 项和两云当前模板不变。E2B 复用已通过 SQLite Workload replay-2 的 binding；Huawei 复用同轮已通过的公开 shell profile 对齐模板。新的每云 binding 只增加 TIME_STYLE 和一条适配说明，不构建新任务镜像、不预编译答案。原私测仍只在确认 stop/Bridge 撤权后进入任务 Sandbox。

原生 Harness 使用本目录 [native-versions.json](native-versions.json) 显式固定 `39964c39357e…` 脚本预检观测镜像；全局默认 catalog 保持 e040。该观测修复不改变 schema、renderer 或 terminal-result-v1 协议。其独立发布和局部检查见 [R01 实验](../../../../../../experiments/r01/README.md)。

文件说明：

- [instruction-appendix.md](instruction-appendix.md)：新增一个公开环境默认值段落，既有 v3 其他段落逐字保留。
- [effective-instruction.txt](effective-instruction.txt) 和 [prompt.diff](prompt.diff)：原题 trim 后加两个 LF，再加附录原字节。
- [variant.json](variant.json)：来源、原件 SHA、环境范围、继承契约与未运行状态。
- [E2B record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-e2b-a-sqlite-with-gcov-terminal-result-v4-record.json)：已有配置契约；本地 binding 来自忽略目录 `.cache/tb-repro-sqlite-workload-v4-1/bindings/e2b.json`。

ECS 原任务镜像 f986… 和当前 Huawei 模板镜像 140590… 的原生 GNU `ls` 检查首轮通过。两侧 UID60000，默认/空日期/显式覆盖均实测；三项文件的 inode、size、mtime、ctime 前后相同，真实安装权限错误在两种环境下文本与非零退出码一致。只读根文件系统、无网络、无模型、无云 Sandbox；临时文件仅位于容器 tmpfs，容器已删除。这不是整任务或真实云环境验收。

探针原件 `/opt/aries-artifacts/sqlite-v4-native-format-1/evidence.json`，SHA `9be39bade37f7f9813989acbb7b57a71b9da1c97962c200c0c7d1de467a4cfa1`。静态材料、两云 binding 和运行 profile 模板归档在 `/opt/aries-artifacts/tb-repro-sqlite-workload-v4-1`；最终 `evidence.json` 记录材料差异和 SHA。Replay profile 模板须在新合格录制后显式填写新 Tape，不能带入旧 v3 Tape。

本轮录制未达到原 3/3、完整 Tape 和完整原生观察门槛，因此在清理确认后停止后续回放，没有重试。失败报告保留八阶段、49 exec 与 2 process 的全部模型决策、可用原始读数及缺失项。这里的当前结果更新不改 ECS 冻结的录制前材料原件；variant.json 中 UNRUN 是该冻结材料的准备时点状态，实际结果以独立报告为准。
