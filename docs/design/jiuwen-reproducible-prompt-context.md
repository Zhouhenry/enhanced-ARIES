# Jiuwen Flash：固定提示上下文候选方案

状态：待审阅，尚未实现。保留原生默认模式；此方案不修改Strict matcher或工具结果。

## 已复现的问题

两次独立真实Flash启动使用相同源码、依赖、任务工作区和受控模型。20项工具定义与其他请求字段一致，但第一个模型请求的system/user正文不同，尚未执行工具就不能Strict匹配。

| 来源 | 实际差异 | 原生入口 |
| --- | --- | --- |
| 本方运行目录 | 8处配置、skills、memory、todo路径包含随机`/run/aries-jiuwen-…/home/data` | 本方`bootstrap.py`设置原生`JIUWENSWARM_DATA_DIR/CONFIG_DIR` |
| 原生system日期 | 本轮跨上海午夜，日期和星期变化 | `RuntimePromptRail.before_model_call`直接读取当前日期 |
| 原生user时间 | 同日也会变化的秒级timestamp | `interface.py:build_user_prompt`直接读取UTC+8当前时间，普通Flash经`_build_inputs`调用 |

公开固定版本的后两处没有可配置时钟参数。请求metadata中的timestamp也不是这两处的输入。这次受控bash的ToolResult正文完全一致，因此不能把失败归咎于工具、Sandbox性能或评分器。

证据：ECS `/opt/aries-artifacts/jiuwen-first-request-1/request-comparison.json`，SHA `1546becd7f75c0f43fa14134e0268db93d1617bdfa10d272df0c34bec022d323`。原始请求仅在私有目录保存；前置端口检查失败另记，不冒充第三次Agent样本。

后续record4另发现原生临时附件的模式名会因runtime state异步写入，从`agent`更新为“智能体模式”。录制入口应接受这种合法更新并完整保存，Strict仍逐轮比较；当前尚未证明该更新在不同冷启动中总发生于相同轮次。此问题不由固定时间自动解决，后续真实回放必须核对，不预先宣称本候选足以消除全部非确定性。

## 建议的最小变体

增加显式、独立命名的`fixed-prompt-context-v1`，默认关闭。其输入包含一次实验冻结的提示时间；该时间随有效配置和录制证据保存，回放使用同值。只用于声明不依赖当前日期的编码任务。

1. 每个独立Sandbox内部采用相同的私有运行路径。仍然一Sandbox一Agent；遇到残留目录拒绝复用，停止时按原机制清理。
2. 仅将上述两处**提示文字的时间输入**改为声明的冻结值。使用固定源码上的窄变体，并保存改动哈希；不全局替换Python时钟。
3. 真实进程、工具、超时、调度、模型传输、单调时钟和性能观测全部使用真实时间。工具输出中的日期、PID、耗时保持原文；如果它们偏离，Strict仍失败。
4. 使用独立Harness变体身份、配置、统计与新录制，不复用原生Tape。原生模式及失败证据保留，不能称此变体为未修改的社区原生结果。

不新增通用RuntimeDriver、通用文本规范化器或隐式回放兜底。若上游以后提供正式提示时钟参数，替换窄变体入口即可。

## 通过条件

- 同工作区两次真实冷启动，首请求全部字段严格一致；多轮请求继续完整匹配。
- 固定输入变化时，Strict确实拒绝；并发独立Sandbox不会串用提示时间或会话。
- 真实工具至少执行一次有可测时长的操作，证明观测时钟和超时未冻结；原生后台、并行工具仍可用。
- 新的真实DeepSeek录制后，在E2B/华为使用同Tape、真实工具和原12项私有Verifier完成验证；正向停止、撤权和删除确认仍必需。
- 旧原生模式、旧Tape及原评分行为不变。工具结果动态信息导致的新偏离独立记录，不由本变体掩盖。

在变体批准和验收前，继续完成原生Flash真实解题、隔离、Verifier和观测工作；原生Strict状态保持已复现阻塞。
