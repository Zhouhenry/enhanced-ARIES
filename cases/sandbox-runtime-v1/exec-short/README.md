# exec-short：逐次真实工具提交材料

**材料状态：CONSTRUCTED_FUNCTIONAL_CHECKED；真实 Agent/Sandbox 性能状态：NOT_RUN。** 公开单步程序、24/96/384 条计划和私有验证器已在 ECS 本机完成有限功能检查；这不是 ToolBridge、Sandbox 或 Agent 测试。原 catalog 定义保持 `DESIGNED_UNRUN`，没有接入 Benchmark/Runner 或生成模型 Tape。该材料仅作辅助测量，主用例见 [真实 Agent 任务](../agent-cases/README.md)。

这是**自建受控 workload，不是官方基准**。单步源码、种子与 N=24/96/384 来自已核验的参考包，并沿用现有 catalog；参数不是任何工具或云厂商的官方标准。测得的逐调用时间会包含 Python 启动、解释器和链文件操作，不能称为纯 Sandbox 调度时间。后续比较必须固定这些条件、保留全部尝试与独立重复；少量样本不足以支持可靠的 p99 排名。

## 已有 ECS 功能证据

主线在 ECS 本机运行构造检查，**30 项检查 PASS**，small/medium/large 分别实际启动 24/96/384 个本机子进程。证据归档于 `/opt/aries-artifacts/cases-exec-short-construction-1/evidence.json`，SHA-256 `b7109ae2d06b54f0c0096751930f760514808d16b3283d2427586beb5603f7dc`。

覆盖完整链、源文件哈希/计划数量，以及 9 类负例：篡改步骤、缺步骤、多余文件、非零退出回执、错误 stdout、缺回执、重复回执行号、错误可信档位、重复步骤独占创建。30 项包含检查器不认证执行历史的断言，**不是 30 个性能样本或 30 类负例**。没有执行真实 native ToolCall、云 Sandbox 或 LLM/Replay；验证器保持 `execution_evidence=NOT_EVALUATED`、`performance_qualified=null`。

## 交接内容与边界

| 文件 | 用途 | 是否进入 Agent 工作区 |
| --- | --- | --- |
| `public/step.py` | 一次进程只完成状态链的一步，来自参考包的原单步实现 | 是 |
| `public/plans/<profile>.jsonl` | small=24、medium=96、large=384 条独立 native 提交计划 | 仅复制选定档位并命名为 `plan.jsonl` |
| `public/instruction.md` | 公开题目；Prepare 时替换可信 `{{profile}}` 和 `{{calls}}` | 是 |
| `LICENSE` | 参考单步代码的 MIT 许可 | 是，随 `step.py` 一并保留 |
| `private/verify.py` | 可信侧重算完整链并核对收集到的步骤输出 | **否，整个 private/ 不能打入 Agent 镜像或工作区** |
| `provenance.json` | 原始来源路径、哈希、改动说明 | 研究归档；不是执行入口 |

Prepare 需要新工作目录，并在其中创建空 `work/steps/`；`step.py`、选定的 `plan.jsonl` 和题目放在其上一级。计划中的 argv 使用相对路径，各条命令均从该同一工作目录执行。准备不算 N 次步骤调用，不能复用已有链文件。这里只定义准备动作，没有提供执行计划的循环脚本。

第 i 步依赖前一步成功，读取前一步 digest，以 `SHA256(previous + ':' + i + ':' + seed)` 生成新文件。第 0 步 previous 为 `START`；文件无换行且采用独占创建。**每条计划必须单独提交一次真实 native exec**；把整个计划放入一条 shell/Python 循环，虽然可能得到相同文件，也不满足 N 次调用要求。

原生后台、自动 yield、process 轮询和 PTY 能力保持可用。若步骤返回后台句柄，按原生方式等待该步骤实际完成，再提交下一步；辅助轮询不计入 N，但必须照常保留。不要修改命令正文、ToolResult 或 Strict matcher 来隐藏不同运行结果。

## 私有验证输入

验证在 Harness 停止、Bridge 撤权和候选封存后，由可信侧调用 `private/verify.py`。传入 `--candidate`（封存的 `work/`）、`--profile`（可信的档位）和 `--receipts`（可信侧从本次原始输出整理的 JSON）。这不是当前已接入的 ARIES 命令；绑定、采集和调用仍待后续实施。

验证器内部固定 seed=20261024 和三个档位的 N，不读取候选宣称的参数或 `valid`。它不导入或运行 `step.py`，而是独立重算每个摘要，检查精确文件集合与内容。候选 `result.json` 或 `exec-events.json` 不能代替可信侧提供的原始输出回执；本材料不要求 Agent 生成新的总结文件或伪计时值。

`--receipts` 是一个 JSON 数组，元素结构如下。这是**字段示意，未执行记录**，`<captured digest>` 必须来自真实收集的 stdout，不能填预计算答案：

```json
[
  {
    "index": 0,
    "exit_code": 0,
    "stdout": "{\"index\": 0, \"digest\": \"<captured digest>\"}\n"
  }
]
```

`index` 指计划行号，`stdout` 是该步骤完整原始文本；允许回执记录顺序与行号顺序不同，但不允许重复、缺失或越界。验证器比较每条 stdout 的 index/digest 和退出码，不接受自报布尔 `valid`。输入来自可信侧不等于验证器能认证它的来源：调用者必须从实际归档证据构造，不能使用候选自写回执。

## 验收输出

验证器 stdout 输出一个 JSON 对象，退出 0 表示这里的文件与回执检查通过，退出 1 表示检查失败。参数错误仍由命令行解析器报错。报告字段如下；没有预先生成“通过报告”。

| 字段 | 含义 |
| --- | --- |
| `schema` | `asb.exec-short.verification.v1` |
| `case_id`、`profile`、`seed`、`expected_calls` | 可信验证配置 |
| `functional_pass` | 完整文件链与所提供回执是否一致且满足固定工作量 |
| `checks` | 检查名称与 passed；错误只标步骤索引或错误类型 |
| `execution_evidence.status` | 固定 `NOT_EVALUATED`，本验证器不认证 N 次 ToolCall、进程身份或计时 |
| `performance_qualified` | 固定 `null`，必须交由后续真实执行证据审查，不能因 functional_pass 自动变 true |

后续 ARIES 实测必须另将 N 个计划项与真实身份、终态及 stdout/退出码核对。A 使用现有 native→SSH→Bridge→SDK→guest 链；B 使用现有 ToolCall/run/session 与 native/process 证据。不要强加 A 的字段到 B，也不要用命令哈希/时间近邻凑映射。缺身份或终态记不完整，不能让本文验证器输出冒充完整调用资格。

## 负例设计与执行范围

下表保留完整设计，已执行范围仅为上节明确列出的 9 类 ECS 本机检查。缺整个 steps/、换行、软链接等其余变体以及实际 ToolCall 资格仍未执行，不因相近负例通过而推定全部通过。负例使用隔离的候选副本，完成后恢复正常材料。

| 构造 | 文件/回执验证预期 | 真实执行证据预期 |
| --- | --- | --- |
| 删除中间步骤或缺整个 steps/ | 失败 | 保留本次失败分母 |
| 改任意文件字节、添加换行、留下多余文件/目录或用软链接替代步骤 | 失败 | 不覆盖原始产物 |
| 使用别的 seed，或以 small 候选按 medium 验证 | 失败 | 不改可信参数迁就候选 |
| 回执缺一条、重复 index、退出非 0 或 stdout digest 被改 | 失败 | 保留原始 stdout/错误 |
| 重复执行同一步 | 原单步独占创建应失败；该回执退出非 0，验证失败 | 不自动重试后篡改原调用记录 |
| 一条 shell 循环生成全部正确文件及正确回执 | 功能检查可能通过 | 必须拒绝“N 次真实调用”资格；验证器仍为 NOT_EVALUATED |
| 文件/回执正确，但缺一个 native 身份或进程终态 | 功能检查可能通过 | 观测不完整，不能判性能合格 |

私有检查不限制正常工具能力，也不自建观测协议。公开计划是固定工作量说明，不是模型响应轨迹；未来 Strict Replay 仍需真实录制，不能把预计算摘要、参考程序或此计划包装为模型 Tape。
