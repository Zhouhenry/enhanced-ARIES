# Jiuwen 原生远端工具与观测验证（2026-10-27）

本页保留 release19→21 的工程修复、夹具与失败证据。后续 [release23 四组真实云验收](jiuwen-release23-cloud-validation-20261027.md)已完成双云 A/B 原评分各 12/12、命令进程计时及四组各自快照的 Perfetto/Jaeger/Prometheus 查询；全局 coverage 仍 unknown，不回填本页历史样本。

最新 release27 已完成 ECS 五门禁；image8、4 vCPU / 8192 MiB 的华为 A/B PPT 均通过独立结构评分并取回原件。A 的三项观测后端对账通过；B 的公开端口误脱敏使本轮观测失败，修复另验。见 [PPT 当前状态与逐次证据](jiuwen-pptx-capacity-20261027.md)。这些新任务不能继承旧编码任务或下列夹具的通过结论。

## 接入方式

固定 JiuwenSwarm `e2c9d519498080e249e34f16a80c8c97f0cd1962`、agent-core `3460150aa5f319b0b8f84a4ed10eec0a08b0feb1`。原生已提供 SysOperation SANDBOX 和 Provider 注册接口；ARIES 注册 `aries` 文件/命令 Provider，把原生 Flash 工具接到 Runner 已创建的 Sandbox。Agent Loop、工具定义与原生结果保留。

release21 的显式原生补丁包括：ToolCall 上下文在调用、流式迭代和取消后准确恢复；Glob 从远端文件接口读取 `.gitignore`；原生 session 取消等待对应远端调用取消后再 ACK。最终隔离仍需确认 Agent、guest 进程组终止并撤销访问，ACK 不代替该证明。

## 本页历史冻结版本：release21

release21 固定 843 个源文件，在 release19 上叠加本轮 47 个文件。build / test / test-race / lint / integration 分别为 11.211 / 31.880 / 82.665 / 3.431 / 322.465 秒，全部 PASS；源码、基底、原始 fixture 与 Docker 库存核验通过。源码清单 SHA `924deaddf3953bddcb0a03fdb48b177320e6b1307803dd9a1e6d2a745c49c039`，冻结回执 SHA `2fbdbafa778a87487a051f3d97cfb61a419d784f82b89ea9e885e10228eac719`。镜像仍为已发布 image5，不推广未证明必要的 image6。release20 的五门禁 PASS 保留，但其目录解析问题使其未用于新云任务。

## release21 工程验证

| 范围 | 实际结果 | 不证明什么 |
| --- | --- | --- |
| image5 | ECS 构建、UID60000 校验、9 项 core 生命周期测试、14 项最终 Provider 测试、bootstrap 正常与篡改拒绝通过 | 不代表完整云 Agent 任务通过 |
| 远端原生语义 | 正式 image5 的 9 项双容器回归通过，含 guest `.gitignore`、后台父子进程、session 取消、其他 session 保活和取消失败 | 活跃 round 的受控协作者不等于真实模型任务 |
| E2B/Huawei 新模板 | 各 1 个真实 Sandbox；2 CPU/4096 MB；来源与工作区、UID/cgroup、停止和 revoke 检查通过 | 不代表云 Agent A/B 或评分出网隔离通过 |
| 完整 ECS A 夹具 attempt6 | 真实 Jiuwen Agent 容器 + 独立 Linux guest worker + 2 次受控模型请求，40.801210999 秒，通过 | 无真实 LLM、独立 Benchmark 评分或云性能排名 |
| image5 + Go 接收修复 attempt7/8 | 正常真实 Agent 链路 40.598 秒；真实远端 Tool 后 HTTP500 负例 41.455 秒，Harness failed，原生 chat.error 留存；清理无错误 | 不代表真实云评分或完整观测通过 |
| 生产目录组合 attempt9 | 真实 Agent/guest 45.808 秒，精确身份与 1+1 请求区间通过，清理无错误；包含 app/cloud、performance、jiuwenswarm 三包 unit/race/vet | 不把旧错误路径夹具当成生产目录验收 |
| Go 请求观测 | performance、jiuwenswarm、runner 三包 unit/race/vet 通过 | 不是新组合的完整发布五门禁 |

两个模板探测 Sandbox 均删除并确认 NotFound；ECS 夹具的 Agent 容器、网络、guest 后台进程、listener 和临时 UID60000 账号均已清理。

## 一次真实 ToolCall 看到了什么

受控模型发出 `call_external_native_probe`，原生工具写入真实 guest 文件并启动真实后台进程。验收核对该 ID 同时存在于原生 Tool span、Bridge 调用与 worker 起止记录；确认文件不在 Agent 容器内，并在 Stop 后确认后台 PID 消失。

| 测点 | 实测 | 边界 |
| --- | ---: | --- |
| `jiuwen.bridge.forwarding` | 8.809062 ms | 控制端完成鉴权和解析后，至转发响应 `io.Copy` 返回 |
| `jiuwen.worker.native_operation_request` | 5.726820 ms | `invocation_start` 单调采样（response.prepare 前）至 `invocation_terminal` 采样；正常路径在最后 end 帧 await write 返回后 |
| 原生 Span | 10 条，其中有准确 ToolCall ID 的 Tool span | 原生调用范围与原始 Unix 时间，不能替代 guest 进程单调计时 |

worker 区间排除之前的鉴权、JSON/参数解码与 init_cwd，也不含终点后的账本落盘；写入返回不代表客户端已收到。

这些是**同一次功能夹具的观测样本**，不是云厂商性能样本。两层请求时间重叠，不能相加；差值也不能称为网络延迟。worker 返回后台启动结果不代表后台进程结束。spawn、首输出、进程退出和 pipe drain 在这批历史夹具中未补采。

Bridge 使用 Runner 的单调时钟。worker 使用独立单调时钟及本地 Unix 锚点；原生 span 保留原始 wall clock。三个域分别展示，精确 ID 只证明关联，不证明时钟对齐。原生 Tool 到 Bridge 的完整身份边离线验收另见下节。

## 同一快照与身份关联

attempt6 原始分析器为 `aries-analysis-v1.13.0-jiuwen-remote-operations`；一个 Bridge 区间、一个 worker 区间及它们的精确 invocation 边进入同一 Snapshot，生成控制端/worker Perfetto 和 OTLP。`integrity=complete`，`coverage=unknown`。

后续分析器在该不可变原始归档上增加原生 Tool→Bridge 的身份边：使用 Attempt、Session、Request、ToolCall 四元组唯一匹配；一个 Tool 可以对应多个 Provider 调用。缺字段或歧义不能靠命令/时间猜测补齐。真实离线验收确认 1 条 Tool→Bridge、1 条 Bridge→worker；OTLP Link 保留原生 trace/span ID，Perfetto 展示查询关系，不绘制跨域时间 flow。重载 Snapshot 后实际 Perfetto JSON 相同，OTLP 同值；未重新运行 Agent。

新请求 Histogram 与阶段 Histogram 分开；请求返回成功可能包含命令非零退出码，不能据此计算任务成功率。独立 Verifier 和原失败分母保持不变。这里的历史夹具未验收实际后端；后续 release21 E2B A 真云快照已另行通过 TraceProcessor/Jaeger/Prometheus，不回填夹具结论。

## 历史失败记录与验收边界

| 记录 | 根因与处置 | 该次结论 |
| --- | --- | --- |
| 完整夹具 attempt4 | 夹具产物根与生产解析目录不一致，且缺 Harness 类型声明；身份/停止断言已到达，最终 Snapshot 拒绝 | 原 FAIL 保留 |
| 完整夹具 attempt5 | 修夹具时把私有产物目录设成 guest 工作目录的共同父目录，0700 阻断 UID60000；已改为私有 artifacts 与可达 work 并列，attempt6 重跑通过 | 原 FAIL 保留，不冒充通过 |
| 模型错误接收与留证 | attempt5 被误认为原生吞错；后续 image5 完整 SSE 对照确认已有 `chat.error`，旧 ARIES 白名单未留存、Client 未消费，最后 succeeded 覆盖了错误 | **修复及真实正负例通过**；attempt7 正常、attempt8 实际 Tool 后模型失败，3 次模型请求含重试，Harness failed，Stop/revoke 确认 |
| 生产 Bridge 目录 | 云装配写入 occurrence/bridge，新增解析器与夹具误用 occurrence/harness；已修正解析器和夹具，保持原生 span 的 harness 目录不变 | attempt9 按生产目录真实重跑通过：1 Bridge + 1 worker、精确身份，三包 unit/race/vet 通过；release21 两云 A 各 11 Bridge + 11 worker、11 条原生 Tool 身份边通过 |
| 离线投影首轮 | Go 内存值的 int/json.Number 类型差异导致 DeepEqual 拒绝；改为核对实际序列化输出，原始数据和匹配标准不变 | 第二轮真实离线对账 PASS，首轮 FAIL 保留 |

仅修复 ARIES 对明确 `chat.error` 的识别和留存；任意 Tool 非零退出仍可由 Agent 恢复。未证明必要的 image6 原生补丁已撤回，该次发布保留 image5。反证保存在 `/opt/aries-artifacts/jiuwen-image6-1/review-closeout.json`（SHA `88171ee07630aaeb3a45ee6416fa2f3867be1ff5029abda7eb76236c54c27170`）；原先错误预期导致的测试 FAIL 保留，不改写成 PASS。

release23 已补充 guest 命令创建、首输出和实际退出计时，不能继续将它们列为全局未实现；自然 EOF、完整后代树计时、Agent→Bridge 提交段计时、全局覆盖分母和跨机时钟校准仍未验收。`bridge_prepare` 阶段字段已通过成功/失败/取消测试，以及 release21 四组真实云阶段/Attempt 对账；剩余差额明确单列。历史八字段不回填、不改变数值。

## release21 及前置夹具证据索引

| 证据 | ECS 路径 | SHA-256 |
| --- | --- | --- |
| release21 五门禁 | `/opt/aries-artifacts/release21/evidence.json` | `4d6367bac05c98a718d1b2de34464db6bf36f4cc49e04689a53eece49705a431` |
| image5 构建验收 | `/opt/aries-artifacts/jiuwen-image5-1/build-2/evidence.json` | `f981193570bbf2404f6eb0b4aacebd349839414e9ad093f3e2c83217d6513cb8` |
| 最终 Provider 14 项 | `/opt/aries-artifacts/jiuwen-image5-1/provider-final-1/evidence.json` | `85b87a0d4c0289fba002a38906ce165da45126a039153093f3a83d4bbf73b0c7` |
| 远端语义 9 项 | `/opt/aries-artifacts/jiuwen-native-remote-gaps-1/image5/evidence.json` | `cfef799e3077700cbdd995ec28a88c7d4de9d3f591c3a4112b7b19860e730aa7` |
| 完整 ECS A 夹具 | `/opt/aries-artifacts/jiuwen-external-native-6/evidence.json` | `4c5a22f82665fe1062aeda6ad6b3c2029a7d1fe7ab79f95a6bda5c66dd56a339` |
| image5 + Go 正常全链 | `/opt/aries-artifacts/jiuwen-external-native-7/evidence.json` | `73c6f55edad6adba82695ac3d2d8f7b9ac7de7a8abe4255116a3d675e56fbea6` |
| image5 + Go 模型失败全链 | `/opt/aries-artifacts/jiuwen-external-native-8/evidence.json` | `031328c597066af6ee54e13d8017b81cb37593cf200846616bf1ce0ee1dd1796` |
| 生产目录完整夹具 | `/opt/aries-artifacts/jiuwen-external-native-9/evidence.json` | `836bb3c899ddf87f0d582162b31fa04bf39ec2ea7719345fe5038da27172bde7` |
| 观测三包 unit/race/vet | `/opt/aries-artifacts/jiuwen-remote-observation-1/candidate-4/evidence.json` | `6204ade7c3d694fcc7219c7ff9a007e45e86875b3f3e340afc75dc957ba9e34e` |
| 身份边三包 unit/race/vet | `/opt/aries-artifacts/jiuwen-native-link-observation-1/candidate-1/evidence.json` | `90a515b740e708ccda6e22d1f810c886cec6dcb008dc4e44bea21c8b1250637b` |
| 身份边真实归档离线对账 | `/opt/aries-artifacts/jiuwen-native-link-observation-1/offline-fixture6-2/evidence.json` | `7cb5253168d2347a131574db4242fbd82d4ca728ebe14ad7ef7d1a7a5ef77157` |

本机保存 `.cache/jiuwen-external-native-6/receipts.tar.gz`（86,188 字节，SHA `148369a5aee202376d20ab675fa3717df2293f1c38b0cd9d4c229f5bf4ed8436`）与 `.cache/jiuwen-image5-cloud-1/evidence.tar.gz`（19,893 字节，SHA `529a20269150ebecb1dca64b37e446b67e16e0ef73f63c64bd2b1c199f5836f9`）。完整运行原件和私有原生 payload 留在 ECS，文档仅引用必要回执。
