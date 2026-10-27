# Jiuwen PPT 任务：容量修复与复测记录

本页记录用户五页 PPT 任务的系统修复。固定提示词、真实模型、原生工具、独立评分和 Stop/revoke 门保持不变；容量通过不等于任务通过。

## 首次真实运行

E2B A 的 `jiuwen-e2b-external-pptx-record-1` 使用 release24、image8 和 4 vCPU / 8192 MiB Sandbox，运行约 19.55 分钟，结果失败。其原始记录保留，后续复测单独编号。

| 实际问题 | 证据与影响 | 处置 |
| --- | --- | --- |
| 录制 SSE 达到帧数上限 | 恰好 65,536 帧；请求加 SSE 约 49.6 MB，尚未达到原 128 MiB 字节上限；Tape 不完整 | 增大帧数及相关字节预算，完整保留原始帧 |
| PPT 下载调用与接口限额不一致 | 清单中有 5 个约 38–120 KB 的 PPTX，调用固定传入 64 MiB，而旧接口最多接受 16 MiB，导致所有下载被拒绝 | 两云共用传输限额统一为 64 MiB；按实际清单文件大小请求下载，仍核对大小和 SHA-256 |
| 原生 SDK Trace 超过收集预算 | 文件 18,317,096 字节，旧预算 16 MiB，因此未保留；producer 记录 424 个 started/ended，不能据此认定未收回的 SDK 文件也有 424 条有效 span | 收集、传输及下游分析一并验证，保留 producer 对账与超限错误 |
| A 模式发送文件找错文件系统 | 原生 `send_file_to_user` 检查 Agent 本地文件，PPT 实际位于 Sandbox；三次返回文件不存在 | 从绑定 Sandbox 的原生 FS 接口取回任务目录文件，再调用原生发送方法；接入实际 session push 订阅 |

本次 Verifier 未运行，不能把生成过 PPT 解释成评分通过。正常清理已删除该 Sandbox；由于下载失败，首次五个原始 PPT 没有保存下来，无法事后恢复。清单中的路径、大小、哈希与失败记录保留，不能用重新生成的文件冒充。

## 统一容量

MiB 为 1,048,576 字节。以下是程序容量，不是工作量、性能目标或已测峰值。

| 环节 | 原容量 | 修订容量 |
| --- | ---: | ---: |
| 模型每轮请求 / SSE 响应 | 16 MiB | 64 MiB |
| 模型单个 SSE 事件 | 1 MiB | 8 MiB |
| Tape 请求和 SSE 原始字节总量 | 128 MiB | 256 MiB |
| Tape JSON 加载 | 256 MiB | 512 MiB |
| Tape SSE 帧数 | 65,536 | 2,097,152 |
| 模型边界队列 / 控制器事件队列，各自 | 4,096 | 16,384 |
| 两云单文件上传 / 下载 | 16 MiB | 64 MiB |
| 适配器 JSON 请求 / 响应（含 base64） | 24 MiB | 96 MiB |
| PPT 数量 / 全部文件总量 | 32 / 256 MiB | 128 / 512 MiB |
| PPT 单文件 | 64 MiB，但旧下载接口不兼容 | 64 MiB，上下游一致 |
| Jiuwen HTTP 单帧 / unary 响应 | 8 MiB | 32 MiB |
| Jiuwen 最终文本 / 选定事件保留 | 8 / 16 MiB | 32 / 64 MiB |
| Jiuwen 原生 SDK Trace 收集 | 16 MiB | 64 MiB |
| Jiuwen 进程、worker 证据及 Agent 日志，各自 | 16 MiB | 64 MiB |
| 私有产物本地读取完整性检查 | 16 MiB | 64 MiB |

公开模型 HTTPS 入口也必须与后端的 64 MiB 请求容量一致；仅修改 Go 常量不能消除反向代理的旧限制。实际部署验收另存回执。

18 MiB Trace 回归另检出了本地私有文件读取层的隐藏 16 MiB 上限，已经同步修正；权限、不可变文件及哈希检查保留。原生 push 只证明已消费的通知，协议没有 chat/push 跨流排空确认，不能宣称通知全量零丢失；PPT 原件保存和评分仍依据 Stop 后的独立目录清单与下载。

事件队列仍非阻塞，满队列仍累计 dropped；容量扩大不代表保证零丢失。Strict 完整请求匹配、ToolCall 因果校验、独立 PPT 结构评分和清理确认均未放宽。25 分钟任务、96 轮模型预算及平台 TTL 是显式实验配置，本次未自动延长。

## 验证与证据位置

- 两个固定适配器镜像通过 ECS 上 8 项检查，包含各自实际 64 MiB HTTP 上传/下载及 SHA-256 对账、超限拒绝；回执位于 `.cache/adapter-capacity64-1/closeout.json`。
- A 模式原生 remote 工具 17 项、producer audit 12 项 ECS 检查通过。真实 `send_file` 的文件物化用实际 Worker/FS，fixture 的最终 push sink 不能当真实云投递成功。
- 模型超过旧 65,536 帧的录制、加载、Strict 回放，以及大请求/帧边界由 ECS Go 测试验证。所有首次测试失败记录保留，不能覆盖后只报通过。
- 最终源码五门禁、四组真实 PPT 任务、独立评分、原文件回传和云清理需分别留证。尚未完成的项不继承历史编码任务或环境探针的 PASS。

固定 image8 的两云模板与原生依赖见 [部署说明](../deploy/jiuwenswarm/presentation/README.md)。PPT 结构评分定义见 [Benchmark 说明](../pkg/benchmark/codingcase/README.md#pptx-task-and-independent-structural-verification)。
