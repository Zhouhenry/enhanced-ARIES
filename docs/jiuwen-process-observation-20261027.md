# Jiuwen A/B 命令进程观测

本次增加 Sandbox 内原生命令进程的计时。已有 Tool、Provider、Bridge 和 worker 请求时间保留，各自回答不同的问题；不能把请求耗时改名为命令执行时间。

release23 已完成 ECS 五项工程门和 E2B/华为 A/B 四组真实 DeepSeek 任务验收。四组原独立 Verifier 各 12/12；15 个命令进程、105 条事件完整，身份、实际退出和收尾均对账；8 个 Sandbox 已删除并独立确认不存在。四组各自冻结快照的 Perfetto、Jaeger、Prometheus 实际查询均限定 PASS，后端临时资源已清理；整体保持 integrity=complete / coverage=unknown。[本轮阶段、逐命令、资源与证据报告](jiuwen-release23-cloud-validation-20261027.md)。四次独立模型轨迹不作性能排名，旧记录不补造进程数据。

## 每段时间是什么

| 展示项 / 分析名称 | 起点 → 终点 | 能说明什么 |
| --- | --- | --- |
| 进程创建 / `jiuwen.process.creation` | `spawn_requested` 取样 → subprocess protocol `connection_made` 回调入口 | 含起点后的同步观测写入、原生创建及事件循环响应；不含取样前的身份快照初始化 |
| 命令进程存活 / `jiuwen.process.command` | `connection_made` → `process_exited` 回调 | Sandbox 内直接子进程的观测墙钟时长，含等待与调度；不是 CPU time |
| 首输出标记 / `stdout_first`、`stderr_first` | 各输出流首次非空 protocol 回调 | 输出何时到达原生进程协议层；不等于模型、Provider 或 UI 收到输出 |
| 退出后管道关闭 / `jiuwen.process.exit_to_pipe_closed` | 直接子进程退出 → 最后一个输出管道关闭 | 只在实际关闭晚于退出时生成；不是完整消费耗时，也不保证自然 EOF |
| 事件容器 / `jiuwen.process.observation` | 创建请求 → 最后一个实际观测事件 | 在双 Trace 中承载上述标记；与子区间重叠，禁止相加或当作新阶段 |

以上边界都在同一进程观测器的单调时钟域内计算。回调时间包含用户态事件循环的响应延迟，并非内核 exec/exit 时间戳。没有跨主机时钟对齐证据时，仅凭 ID 关联，不相减推算网络时延。

`Process.wait()` 在某些管道状态下会等到管道关闭才返回，因此不把它当作直接子进程退出时间。命令也可能先关闭 stdout/stderr，之后才退出；这种情况下不生成负数或伪造的零耗时“退出后关闭”区间。允许输出或退出回调早于 `spawn_return`。回调入口先取样，调用原生回调后再写事件；以入口时间解释事件，JSONL `sequence` 仅表示写入尝试顺序，不保证时间戳严格递增。

上述区间包含插桩时已发生的观测扰动，不能直接称为未插桩的内核创建成本；本批功能验收不代替独立 L0/L1 开销对照。

后台 Tool 返回“启动成功”后，进程区间仍可继续。后台任务可能与后续 Tool 重叠，因此 Tool/命令区间总和不能替代 Agent 墙钟总耗时。既有任务阶段边界和历史统计保持不变。

## 采集与关联

固定 openJiuwen core `3460150aa5f319b0b8f84a4ed10eec0a08b0feb1`，只在原生 `ShellOperation._create_subprocess` 的 shell/exec 创建入口接入观察器。前台、流式、后台共享该入口。协议回调继承原生实现，不添加输出读取者，不改命令、双流正文、退出码、工具渲染和调度规则。

- A：Provider 的原始 run/task/session/request/ToolCall 和 invocation 随请求到达 worker，在创建进程时复制。后台退出继续使用这份身份，不读取已结束调用的 ContextVar。
- B：在创建进程时复制原生 ToolCall、request、shell session 上下文及本次 run/task。缺失身份明确保留，不能通过命令哈希、时间近邻补配。
- 每次创建有独立 process ID；同一 Tool 发起多个命令时分别展示，不强制一对一。
- JSONL 记录时钟、事件、身份、PID、退出码和异常类型。该观测源不记录命令正文、输出字节或凭据。

原始产物：A 位于 occurrence 的 `bridge/jiuwen-process-{events.jsonl,index.json}`；B 位于 `harness/jiuwen-process-{events.jsonl,index.json}`。index 提供独立尝试/启动/退出计数、事件计数、丢失/写失败/迟到计数、未结束进程数、账本 SHA-256 和时钟锚点。外层 index 另声明是否启用，避免整对文件丢失被当作“没有运行命令”。旧镜像缺少原生 hook 时保留实际 unsupported；请求开启观测不等于成功采集。当前 image7 的 patch manifest、模块与原生入口必须吻合，安装损坏不能降级为正常 unsupported。

分析器为 `aries-analysis-v1.16.0-jiuwen-process-timing`，OTLP exporter 为 `aries-otlp-http-json-v1.3-process-events`，事件策略为 `jiuwen-chat-provider-process-lifecycle-v3`。同一份校验后的分析快照供 Perfetto 和 OTLP 使用。生命周期标记按明确的 observation ID 放入对应 OTLP span；跨域身份链接只证明属于同一调用，不证明时间对齐。Prometheus 分别聚合创建、命令和退出后关闭三个口径，排除事件容器。

## 停止与完整性

结束时先撤销新调用，再通过原生取消机制让进程退出，短暂等待已有回调完成并关闭观察器。最后仍必须通过既有 cgroup 停止与空组确认，之后才收集产物并允许独立评分。观察器关闭成功、worker `/quiesce` 成功、直接子进程退出都不能替代隔离确认。

观察收尾失败不阻止实际隔离清理；其原始证据、错误和不完整状态必须保留。缺少退出回调时，保留已知创建/首输出事件，不凭 Tool 完成或清理时间补终点。整棵后代树的退出仍由隔离门证明，不能冒充单条命令的进程生命周期。

## 验证状态

| 检查 | 当前状态 | 证据 |
| --- | --- | --- |
| 固定原生依赖上的 14 项进程行为测试 | PASS | ECS `/opt/aries-artifacts/jiuwen-process-native-1/test-1/evidence.json`；含原始双流/exit 7、背景 grace、取消、后代持管道、身份、写失败 |
| Go 快照、双 Trace、指标、收集与隔离回归 | PASS，最终兼容组合 | ECS `/opt/aries-artifacts/jiuwen-process-go-3/evidence.json`；unit 8.610s、race 16.614s、vet 1.067s，仅工程检查耗时 |
| 新镜像及真实 Remote worker/原生 extension | PASS，限定本地集成 | `/opt/aries-artifacts/jiuwen-process-native-1/build-1/`、`integration-3/evidence.json`；A 17 项，B 原生 ExtensionManager+FlashBash，前台 exit 7、后台实际退出与身份对账 |
| image7 E2B/华为模板与 guest 探测 | PASS，限定能力探测 | ECS `/opt/aries-artifacts/jiuwen-image7-cloud-1/closeout.json`；两侧均 2 CPU / 4096 MB，各一个 TTL 300s 实例，已删除并独立确认 NotFound |
| 最终新旧镜像 runtime 兼容 | PASS | `/opt/aries-artifacts/jiuwen-process-native-1/final-compat-1/closeout.json`；image7 remote 18 PASS+1旧镜像用例skip，image5 remote 17 PASS，readiness 3 PASS；两镜像真实 staging/worker、B工具与扩展发现通过；9个容器均清理 |
| release23 完整工程门 | PASS | `/opt/aries-artifacts/release23/freeze-manifest.json`；861 文件，build/test/test-race/lint/integration 均通过并冻结 |
| E2B A/B、华为 A/B | PASS，真实任务与进程证据 | 原独立评分均 12/12；E2B A/华为 A/E2B B 各 4 进程、28 事件，华为 B 为 3/21；exact 身份与实际退出完整，drop/write/late/identity failure 为 0，8 个 Sandbox 均独立确认删除 |
| 实际 Perfetto/Jaeger/Prometheus 对账 | PASS，四组各自同快照的实际查询 | 2,035 个 Jaeger span / 24,464 项对账；15 进程、105 markers、31 个进程 histogram 样本；18 个有值 metrics 流首次与重复 payload 查询通过，5 流 NOT_COLLECTED；临时容器/listener/cgroup 清理确认 |

本轮使用新的 release23 冻结源码和 image7，不借用旧 release21/22 的结果。华为云评分网络隔离仍按既有授权假设标注，不能升级为已验证隔离或 Goodput。

本机后端汇总为 `.cache/jiuwen-release23-backend-evidence/jiuwen-release23-backends-summary.json`，SHA `02a2ccb76506be07f2b1fad2a165b96b8b46f1f9de0f7e6423149880c09f5f1b`；ECS 逐组目录为 `/opt/aries-artifacts/jiuwen-release23-backends-1` 至 `-4`。进程相对时间与 v3 OTLP 纳秒投影逐项对账，不证明跨主机校准。相同 payload 的两次查询稳定也不证明生产 exactly-once 或持久存储。

保留首轮 Go 候选超时记录：新增测试中的模拟超时 handler 等待未消费请求的 context 结束，测试服务器收尾无法完成；修正夹具为先读取请求并设置有限等待后，候选2通过。B 夹具前两次分别未提供原生 ExtensionRegistry 构造依赖、未对齐原生 LOCAL allowlist 配置，修正夹具后 integration-3 通过，未改生产工具行为。以上失败不覆盖、不改写为通过。
