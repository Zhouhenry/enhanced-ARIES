# SQLite v3：安装日志 mtime 偏离的只读归因

**旧 v3 Tape 的 Strict 失败应保留；三个时间值有明确的原生命令来源，也存在可供新环境采用的原生显示选项。** GNU `ls` 支持用 `TIME_STYLE='+'` 省略日期显示，不修改文件 mtime。它可以针对这次已确认的输出差异，但需要新的显式环境声明和新录制，不能用于修补旧 Tape，也不保证整条任务轨迹确定。

本次只读取既有运行原件、固定 SQLite 源码和 GNU 官方随包手册，并在 ECS 对现存文档文件做只读 `ls` 格式检查；没有新 Sandbox、模型请求、环境构建、Tape、任务材料或 matcher 修改。

## 三个时间值从哪里来

原任务提供的 SQLite 3.50.4 压缩包 SHA 为 `7c02b6cc04dddc2f90e63d43ab8900cb694ba4665f4e4ac877ec724733125229`。其 `sqlite/main.mk:1565–1584` 的 `install-dll-unix-generic` 规则依次执行：

```make
$(INSTALL) $(libsqlite3.DLL) "$(install-dir.lib)"
# 随后进入安装目录，清理旧目标
mv $(libsqlite3.DLL) $(libsqlite3.DLL).$(PACKAGE_VERSION)
ln -s $(libsqlite3.DLL).$(PACKAGE_VERSION) $(libsqlite3.DLL)
ln -s $(libsqlite3.DLL).$(PACKAGE_VERSION) $(libsqlite3.DLL).0
ls -la $(libsqlite3.DLL) $(libsqlite3.DLL).[a03]*
```

这里摘录命令主体，完整原规则中的失败退出判断保留在证据中。`install` 新建安装副本，`mv` 将其改为带版本的名字，两次 `ln -s` 新建符号链接。紧接着第 1573 行硬编码的 `ls -la` 输出三项产物各自的真实 mtime；不是 Agent 自己额外选择了长列表，也不是 libtool 或 native renderer 生成日期。

| 原生 wait 结果行 | 文件 | 录制 | 自回放 |
| --- | --- | --- | --- |
| 3 | `libsqlite3.so -> libsqlite3.so.3.50.4` | Oct 26 08:03 | Oct 26 08:14 |
| 4 | `libsqlite3.so.0 -> libsqlite3.so.3.50.4` | Oct 26 08:03 | Oct 26 08:14 |
| 5 | `libsqlite3.so.3.50.4`，5,257,000 B | Oct 26 08:03 | Oct 26 08:14 |

源规则与实际输出证明这是运行中安装产物的元数据。既有初始目录 mtime 设置无法固定这些后来创建的文件；本次没有读取已删除 Sandbox 的纳秒 stat，不能从分钟显示补造精确创建时间。

## 哪次回读进入了模型历史

exec 22、模型轮 23 的精确 ToolCall 为 `call_00_00rjJN3XgiTZ656qW2hn8439`，真实命令是：

```sh
cd /app/sqlite && make install > /app/build-install.log 2>&1; echo "install exit: $?" >> /app/build-install.log; tail -25 /app/build-install.log
```

日志在同一个 exec 中立即被 `tail -25` 回读。此调用由原生自动转后台路径返回句柄；随后 `call_00_2VvLMEuEjsdfLQNTpV247411` 的 `process action=wait` 返回最终正文。回放 transcript 中调用、wait、wait 结果分别位于第 50、52、53 行。第 25 个模型请求（turn 24）的 `$.messages[49].content` 首次因此不匹配。

两侧最终正文均为 25 行、1,983 UTF-8 字节，只有第 3–5 行各两个时间数字不同，总计 6 字节。原生正文 SHA 与模型请求边界诊断精确相符；不是按时间邻近猜测关联。旧运行仍为 22 exec + 2 wait、评分 0/3，不能把此诊断升级为完整 Strict 验收。

**`make install` 真实返回 2，ToolResult 的 exit 0 来自最后的 `tail`。** 两侧正文均保留 Tcl 安装路径的权限错误、`make: ... Error 1` 和 `install exit: 2`。命令以分号继续执行，`echo` 只将退出码写入日志，没有将安装退出状态作为整个 shell 的退出状态返回。`completed / exitCode=0 / isError=false` 证明该原生 shell 调用结束，不能称为 `make install` 成功。这里没有管道，但仍存在最后一条命令覆盖整体 shell 状态的问题；不修改旧命令、结果或 Tape。

v3 [公开附录](../cases/sandbox-runtime-v1/agent-cases/sqlite-with-gcov/variants/terminal-result-v3/instruction-appendix.md) 已要求保留完整日志和底层退出状态、失败时读取真实诊断。这次真实决策保留了错误文本，却在同一命令尾部读回动态长列表，且没有向外传播安装退出 2。继续泛化“成功日志少打印”不能保证解决这个已有失败；直接丢弃整段日志还会隐藏必要的权限诊断。

## 原生格式选项的可行范围

GNU Coreutils 9.4 官方随包手册的 `Formatting file timestamps` 节说明，`TIME_STYLE` 指定 `ls --time-style` 的默认值，`+FORMAT` 控制日期格式。对应[官方网页](https://www.gnu.org/software/coreutils/manual/html_node/Formatting-file-timestamps.html)本次抓取超时，因此核验依据是 ECS 的 `/usr/share/info/coreutils.info.gz`，节点及来源 SHA 已归档。

ECS 只读检查实际调用 GNU `ls 9.4`，目标是现存的该文档文件：默认格式展示日期；`TIME_STYLE='+'` 输出空日期字段；即使此环境变量存在，显式 `--time-style=full-iso` 仍能展示真实时间。检查前后 inode、大小、mtime、ctime 完全一致。这是原生格式选择，不是冻结时钟、改写文件元数据、过滤已生成的 ToolResult 或返回伪造日期。本次未在新 guest 环境验证。

最小候选是在新的 binding 中明确声明 `command_env: {"TIME_STYLE":"+"}` 及适配说明。已有代码路径完整：

1. `pkg/sandbox/e2b/binding.go:DefaultEnvironment` 合并显式环境；binding 对环境适配要求说明。
2. `internal/app/wiring/cloud/cloud.go` 把它传给 `ExternalBridgeOptions.CommandEnv`。
3. `pkg/harness/openclawcloud/external.go:cloudTarget.ExecStream` 合并默认值，原生单调用指定值仍优先。
4. `pkg/harness/openclawcloud/group.go` 将环境写入真实 guest 的命令 payload，降权后交给 `os.execvpe`；因此 shell → make → recipe 的 `ls` 可继承。只设置控制器或 Harness 容器环境不足以覆盖这条边界。

现有 binding 默认环境也用于 Sandbox 命令，应将影响范围声明为该 binding 下所有继承此默认值的 GNU `ls`，包括相关准备/验证命令；不只限于这三行。技术上可沿用已有兼容模板、另建环境 binding 和 profile，不必为了一个环境变量重建镜像；两云均需新声明、实际 guest 读回和新录制验证。不能只依赖 Dockerfile 的 ENV，因为 guest 执行组重建环境。Agent 若显式设置其他 TIME_STYLE、使用 `--time-style` 或读取 `stat`，仍可查看真实时间；该选项不是禁止必要诊断的策略。

该设置不影响权限、文件名、链接目标、大小和实际错误文本，也不解决 shell 最终退出状态、PID、耗时、其他工具日志或输出顺序的变化。`LC_ALL`/`TZ` 只能选择格式或时区，不能让两个不同 mtime 相同；`make -s` 只抑制 recipe 回显，不能消除 `ls` 程序自身 stdout。固定初始 mtime、事后 touch、新文件扫描改时间和 ToolResult 后处理均不是本建议。

因此，**旧固定 v3 Tape 保持 Strict FAIL，用于完整固定工作量比较时采用已独立验收的 Workload；SQLite 任务本身不能仅凭这三行就被判为永远无法 Strict。** 新公开环境可以采用上述原生选项针对这个已知差异，再以新 Tape 验证。当前只提出此最小候选，没有创建环境、变体或开始录制。

## 证据

只读审查原件：`/opt/aries-artifacts/sqlite-v3-mtime-rootcause-1/audit.json`，SHA `314b0720e8bb0397b9ff1aca6295dcc9ca9ee60f91c2ada704de6a7d70b7b9d8`。它保存原 transcript 路径/行号/SHA、精确命令、两侧完整最终正文、边界摘要、三行差异、vendor 内源码 SHA/行号、GNU 手册来源和只读格式检查。

原 Tape SHA `a73e482ea845c44b370df656d5358b8231a5c161e1acdbd5e7da1d8edda72d46`。原录制最终正文 SHA `39f4581cfacb7c2c0102d42d97f65263ee63cb488c69d50f4adf8d2d1f9170ce`，自回放正文 SHA `b314e5cfeefd3d5feae6241dd2a8beb72cb42fca95bfa38426becc0514deb937`。原件根为 `/opt/aries-artifacts/tb-repro-sqlite-v3-e2b-{record,replay}-1`，旧失败和评分均不改。
