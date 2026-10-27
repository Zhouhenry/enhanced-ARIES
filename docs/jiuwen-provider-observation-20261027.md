# Jiuwen A 客户端请求观测：时间含义与验收

本页说明 Agent 侧原生 Sandbox Provider 的请求区间。它连接已有 ToolCall、Bridge、Sandbox worker 证据，**该区间本身不代表命令进程的运行时间**；release23 已用独立协议回调采集命令进程时间。A 的 Agent 在 ECS 容器，B 的 Agent 在 Sandbox；B 不经过这个远端 Provider，不能为 B 填造这些数据。

## 当前状态

候选在 ECS 的真实 Jiuwen Agent、独立 worker、受控模型联调通过，实际远端文件写入与原生 ToolCall 身份一致，容器/进程组/监听已清理。Python Provider、Go unit/race/vet 均通过。随后补了两项发布前修正：使用 Bridge 的真实接纳数核对 Provider 缺失；将四个客户端时间点精确导出到 OTLP span events。

历史 release22 的 845 文件组合相对 release21 改动 15 个文件，ECS 五项发布检查通过，但未独立启动云运行。最终合并命令进程计时的 release23 冻结 861 文件并通过五门禁，随后 image7 双云 A/B 四次真实模型任务各原评分 12/12、8 个 Sandbox 独立确认删除。两组 A 各 11 次 Provider 请求、44 个生命周期事件和 22 个身份链接通过对账；15 个命令进程的独立计时另列，不混成 Provider 耗时。

四组各自冻结快照的 Perfetto、Jaeger、Prometheus 实际查询均限定 PASS；完整数据见 [release23 四组报告](jiuwen-release23-cloud-validation-20261027.md)。新 image7 两云模板均为 2 CPU / 4096 MB。四份独立真实模型轨迹不能作性能排名，整体 coverage 仍 unknown，华为独立评分出网仍 assumed；不借旧版本结果或功能评分宣称生产化全部完成。

## 每个时间点是什么意思

| 观测点 | 实际记录位置 | 不能解释成什么 |
| --- | --- | --- |
| 请求开始 | 创建该调用的观测状态之后、建立 HTTP ClientSession 之前 | Agent 开始整个 Tool 的时刻 |
| `http_submit` | 调用 HTTP POST 前 | 请求字节已经写完，或 Bridge 已接纳 |
| `response_headers` | 客户端取得 HTTP 响应头 | 命令已经开始或已经完成 |
| `first_frame` | 收到并解析第一个完整协议帧，核对 invocation ID 后 | TCP 首字节、命令首输出或模型首 token |
| `terminal_frame` | 收到该协议调用的 end 帧 | 后台进程真实退出 |
| 请求结束 | 原有传输上下文和必要的取消收尾退出后 | 纯网络时间或纯 Sandbox 命令耗时 |

总请求耗时用同一个 Agent 进程的单调时钟计算。它包括传输、远端执行、流式结果消费者的停顿和收尾。四个时间点是该区间内的里程碑，不应逐个当成耗时相加。错误或取消可只有部分里程碑；保留真实部分，不补零。请求正常返回只表示 Provider 协议完成，原生工具非零退出码、任务结果和独立 Verifier 继续各自记录。

## 三层时间与两种 Trace

| 层 | 计量范围 | 时钟与关系 |
| --- | --- | --- |
| Agent Provider | 请求开始至传输/取消收尾结束 | Agent 单调时钟及本地 UTC 采样锚 |
| ECS Bridge | 已接纳请求的转发和响应 | Runner 单调时钟 |
| Sandbox worker | 响应准备前至原生结果末帧写出后的终态记录 | worker 单调时钟及本地 UTC 采样锚 |

三层按真实 invocation、Attempt、Session、Request、ToolCall 和操作类型关联。ToolCall 可以包含多次 Provider 操作，不能把操作数当工具数。UTC 锚用于导出 epoch；它不证明三台/三个进程的时钟已校准。各层有包含或重叠关系，不相加，也不把差值命名为网络耗时。

同一份版本化 Snapshot 同时提供：

- Perfetto/Chrome Trace：Provider 独立时钟泳道、四个时间点与可查询身份关系；跨时钟关系不画成已对齐 flow。
- OpenTelemetry/OTLP：保留原生 span ID 和父关系，Provider 单调区间另作 span；四个时间点按准确父区间 ID 导出为 span events，跨层关系用 Link。
- Metrics：`aries.jiuwen.remote.duration` 按 Provider、Bridge、worker 的 scope 分开，保留成员、count、sum、桶；不并入阶段总时间。

release22 的分析器为 `aries-analysis-v1.15.0-jiuwen-provider-requests`、OTLP exporter 为 `aries-otlp-http-json-v1.2-provider-events`、event policy 为 `jiuwen-chat-and-provider-lifecycle-v2`。当前 release23 使用 `aries-analysis-v1.16.0-jiuwen-process-timing`、`aries-otlp-http-json-v1.3-process-events` 和 `jiuwen-chat-provider-process-lifecycle-v3`，保留上述 Provider 语义并加入独立进程源。旧冻结 Snapshot 原件不改写。

## 完整性与失败证据

Provider 使用现有原生 SDK 及其 producer/export 审计，不新增一条包含正文的日志。只保存受限身份、操作类型、状态和时间，原生参数、输出与异常正文不进入公共导出。SDK span 不跨异步生成器 yield 绑定当前上下文，保留原生调用树和并行行为。

对每个 Bridge 已接纳调用，要求找到唯一且身份一致的 Provider 观测。整个 Provider span 丢失也必须报不完整；Bridge 时钟记录缺失时保留真实接纳数，不能用零分母通过。辅助操作没有 ToolCall 时保留 unavailable，不编造 ID。尚未到达 Bridge 就失败的客户端请求不属于这个接纳分母；全局覆盖仍为 unknown。

## 命令进程计时

release23 已在固定 native LOCAL `ShellOperation._create_subprocess` 接入进程协议回调，并覆盖 A worker 和 B 原生 extension，四组真实运行共 15 个进程、105 条生命周期事件通过来源内完整性对账。详见 [独立的进程时间说明与当前验收状态](jiuwen-process-observation-20261027.md)。Provider 的上述范围保持原样，不能把它改名为命令时间。

进程管道关闭仅证明 callback 被观察到，不承诺自然 EOF；直接子进程退出不代表完整后代树退出。逐 Tool CPU/内存和跨机时钟校准仍需独立证据。

## 证据入口

- 前置候选：ECS `/opt/aries-artifacts/jiuwen-provider-observation-1/evidence.json`，SHA-256 `0ee231fb156072eae5719e7c35c0f1e4939ce449dcf47d7d7e7690c4932402e7`。联调为真实 Agent 与工具、受控模型，没有云端任务评分。
- 历史 release22 源码清单：SHA-256 `de0d83bdf0fe4dd7d9233059a27fd4fa028ebc2104873e5281524b786c988b9f`；源码包 `4f579825c14ace16524a455de5719e121442429e874f8238e720582abf6c48b2`。仅证明该组合门禁，不替代 release23 实际云验收。
- release23 冻结回执：`/opt/aries-artifacts/release23/freeze-manifest.json`，SHA `3cfe77ec01640dbde5c80eb3e039ad7bd2500098c69c3d1e193bc103bbd759fd`；四组实际后端汇总 SHA `02a2ccb76506be07f2b1fad2a165b96b8b46f1f9de0f7e6423149880c09f5f1b`，原件与字段对应见 [本轮报告](jiuwen-release23-cloud-validation-20261027.md)。
- [上一正式版本四组云结果](jiuwen-release21-cloud-validation-20261027.md)；[完整问题台账](observability/OPEN_ISSUES_AND_RESOLUTIONS.md)。R01–R18 其余范围保持，不因此宣称 P0 或生产化全部完成。
