# 六项真实 Agent 任务：阶段性能与失败归档（2026-10-26）

当前是 **E2B A（External）、并发1的真实单任务验收**。六题中五题至少一次通过原独立Verifier；其中git、async、db已有同Tape完整Strict回放。大文本和SQLite新录制评分通过、Tape完整，但新回放仍失败；C++因账户运行时限未执行Agent。**六题闭环未完成，不宣称生产可用。**

累计保留19个实验目录：18条原ARIES Attempt、17次实际Agent执行；另1次Tape路径预检失败没有Attempt。原账本12次validated、9次succeeded；原私有driver有7次PASS，另外2次录制因验收脚本字段错误记FAIL，新增旁路复核通过，原FAIL没有改写。它们是已有运行的纠错复核，不增加Attempt。工程检查、创建诊断、离线分析不算Agent测试。

历史总表按原整体PASS口径为此前80+本组六例7=**87次**；另列上述2次纠错合格录制，不悄悄改成89或清除原失败。次数包含重复和回放，不是不同任务数，也不是自主Agent成功率。

先看结果总表，再看每题八阶段与逐工具表。模型/工具步骤数量不同的运行不能按总时长排名；失败回放很短不意味着Sandbox更快。

## 统计与测量口径

- 保留全部失败、重试、超时和未执行。录制/回放、原Verifier、ARIES账本、私有验收和旁路复核分开表达。没有独立候选封存的运行不宣称Artifact Goodput合格。
- 各任务模板声明1vCPU/2048MiB；未完成逐实例配额、存储、冷热、地区与主机争用的可比性证明。A的Harness在ECS，真实工具在E2B。
- 停止Harness并撤权确认后，向原任务Sandbox注入原私有测试进行评分；不是新禁网评分实例。实际17个已知任务Sandbox均有删除和独立NotFound证据；C++无ID，不能把列表未发现写成NotFound。
- 八阶段来自同一Attempt原字段，顺序与[阶段说明](performance-stages.md)一致；单位秒。阶段存在未覆盖区间，不机械求和替代E2E。
- SDK等待为控制器侧exec.wait，guest整体为受管执行回执；不同钟不相减当网络耗时，重叠区间不相加当墙钟。明确拒绝、未知、未采集均不填0。
- `integrity=complete`是已采来源契约完整，`coverage=unknown`仍不知道全局漏采分母；Trace通过不能覆盖业务失败。

## 结果总表

“录/放”的数字与真实目录后缀一致；不存在的db-放2、text-放1、sqlite-放1是计划后未运行，另行列出，不补造结果。

| 运行 | 执行Agent | 原评分 | 原Attempt | 原driver | 纠错复核 | 完整性/覆盖 |
| --- | --- | --- | --- | --- | --- | --- |
| git-录1 | 是 | 2/2 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| git-放1 | 是 | 2/2 | failed | FAIL | — | complete/unknown |
| git-录2 | 是 | 2/2 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| git-放2 | 是 | 2/2 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| async-录1 | 是 | 5/6 | failed | FAIL | — | complete/unknown |
| async-录2 | 是 | 6/6 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| async-放1 | 无Attempt | 未执行 | 预检失败 | 未进入driver验收 | — | 未采集/未采集 |
| async-放2 | 是 | 6/6 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| db-录1 | 是 | 7/7 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| db-放1 | 是 | 0/7 | failed | FAIL | — | complete/unknown |
| db-录2 | 是 | 0/7 | timeout | FAIL | — | incomplete/incomplete |
| db-放3 | 是 | 7/7 | succeeded | PASS_ORIGINAL_TB_SINGLE_RUN | — | complete/unknown |
| text-录1 | 是 | 5/5 | failed | FAIL | — | incomplete/incomplete |
| text-录2 | 是 | 5/5 | succeeded | FAIL | 旁路复核通过 | complete/unknown |
| text-放2 | 是 | 0/5 | failed | FAIL | — | complete/unknown |
| cpp-录1 | 否 | 未执行 | failed | FAIL | — | incomplete/incomplete |
| sqlite-录1 | 是 | 3/3 | failed | FAIL | — | incomplete/incomplete |
| sqlite-录2 | 是 | 3/3 | succeeded | FAIL | 旁路复核通过 | complete/unknown |
| sqlite-放2 | 是 | 0/3 | failed | FAIL | — | complete/unknown |

## 失败原因、已采取措施与剩余工作

| 运行 | 证据与当前处置 |
| --- | --- |
| git-放1 | 真实Git提交ID不同；固定新提交日期后，重新录2/放2通过。原失败保留。 |
| async-录1 | Agent取消逻辑未满足一项原断言；同配置重新自主解题及回放通过，不删首次失败。 |
| async-放1 | 操作者传入错误Tape路径，预检失败，没有ARIES attempt或云资源。 |
| db-放1 | 首工具根目录mtime不同，Strict拒绝；仅执行原11条中的1条。 |
| db-录2 | 固定初始mtime的新录制运行达到原900秒预算，75轮模型/74次exec、评分0/7；真实Agent未完成，Tape不完整。 |
| db-放3 | 复用录1原Tape，显式准备原可见分钟的根mtime后12轮/11工具严格通过、7/7；是选定历史样本修复验收，不代表录2成功。 |
| text-录1 | 原5/5，但Run返回后迟到模型请求污染Tape；原生解析拒绝缺少派发前证据，整体失败。 |
| text-录2 | 系统修复后Tape完整、原5/5；私有driver误读coverage.status造成KeyError。原FAIL保留，新旁路复核通过全部原验收。 |
| text-放2 | 首工具根目录日期18:34→18:48；Strict实际拒绝，仅执行1/12条，原评分0/5。 |
| cpp-录1 | 原allocation_unconfirmed且无ID；新4200秒真实诊断明确HTTP400/项目1小时上限。完整Agent等待足够时限的项目，不缩原预算。 |
| sqlite-录1 | 原3/3，但迟到请求污染Tape；here-string解析拒绝与权限拒绝的旧身份记录不完整。 |
| sqlite-录2 | 新Tape完整、原3/3；28条明确执行+1条明确派发前拒绝，另6次process。driver字段错误旁路复核通过，旧FAIL保留。 |
| sqlite-放2 | 第三工具展示运行中新建目录，日期18:37→18:49；Strict实际拒绝，仅执行3/35条，评分0/3。 |

私有验收器错误是读取 `coverage[0]['status']`，实际字段为 `coverage_status`。只修这一字段；ECS正例及15项负例通过。对text/SQLite录2先复现旧KeyError，再运行完整原验收，原件SHA与Snapshot不变。随后确实创建了新Sandbox进行回放，分别暴露上表真实差异；没有用离线复核代替必要真实测试。

SQLite旧here-string `<<<`被原解析器误报未终止here-doc，不能归因Agent忘记闭合；权限拒绝为另一分支。新版本补齐派发前拒绝证据，保持原拒绝行为，不造SDK/guest执行区间。text/SQLite录2分别5/6次迟到请求被准确记为ADMISSION_CLOSED；只允许同clock封口且未接纳的这类请求，不放过会话中真正错误。

DB放3显式初始root mtime为2026-10-25T16:28:00Z，读回确认并保留atime。分钟来自录1可见结果，原始秒/纳秒和原时区未知，不能称完整环境恢复。11/11真实决策与结果相同、12轮Strict完成、原评分7/7；录2超时仍单列为失败，不以选中的历史成功抹除选择偏差。

后台随机句柄/PID/idle及更晚新文件时间在text/SQLite录2中确实存在，但本次回放较早失败；它们是已观察到的后续风险，不是全部已重现的首因。原case有效，问题在跨实例逐字回放的实验条件。用户批准先审阅[变体设计](design/reproducible-agent-workloads.md)，尚未批准实施；Strict、真实工具、后台/PTY、原题和私测均未放宽。

本轮新增6个真实Sandbox均确认删除；round1的3个后续回放计划因录制超时或验收异常未执行，保持UNRUN。round2的两个失败回放与一个成功DB回放均留档。C++不缩短原Agent1800秒+Verifier1800秒预算，也不把创建错误当Agent失败。

## 八阶段耗时

单位秒；同一行始终是同一个阶段，列名与上表运行ID相同。全部取原 `stage_duration_ns`，不从其他运行回填。排队是app接纳等待，不是云创建内部队列；Runtime不是单独LLM或工具耗时。

### fix-git

| 阶段 | git-录1 | git-放1 | git-录2 | git-放2 |
| --- | --- | --- | --- | --- |
| 排队 | 0.000011 | 0.000014 | 0.000016 | 0.000013 |
| 任务 Sandbox 创建 | 1.443368 | 0.919595 | 1.400219 | 0.857961 |
| 工作区准备 | 2.655866 | 1.471914 | 1.361275 | 1.687674 |
| Harness 启动 | 9.026265 | 7.620358 | 7.277924 | 7.297785 |
| Runtime 执行 | 29.747619 | 27.894157 | 29.588969 | 30.277142 |
| 停止与撤权 | 5.650240 | 5.906410 | 5.571381 | 5.579772 |
| 独立验证 | 8.934452 | 9.764097 | 8.080144 | 8.734893 |
| 清理 | 0.763945 | 0.649236 | 1.906126 | 0.715608 |

### cancel-async-tasks

| 阶段 | async-录1 | async-录2 | async-放1 | async-放2 |
| --- | --- | --- | --- | --- |
| 排队 | 0.000015 | 0.000015 | 未采集 | 0.000015 |
| 任务 Sandbox 创建 | 1.673262 | 1.518144 | 未采集 | 0.786462 |
| 工作区准备 | 2.460744 | 2.300800 | 未采集 | 2.137998 |
| Harness 启动 | 9.185049 | 7.214013 | 未采集 | 7.164884 |
| Runtime 执行 | 12.578048 | 11.426669 | 未采集 | 12.609468 |
| 停止与撤权 | 5.685071 | 5.533693 | 未采集 | 5.568886 |
| 独立验证 | 21.706980 | 22.301675 | 未采集 | 23.342831 |
| 清理 | 1.349927 | 0.662324 | 未采集 | 1.506018 |

### db-wal-recovery

| 阶段 | db-录1 | db-放1 | db-录2 | db-放3 |
| --- | --- | --- | --- | --- |
| 排队 | 0.000010 | 0.000014 | 0.000017 | 0.000014 |
| 任务 Sandbox 创建 | 0.881640 | 1.460532 | 2.106196 | 1.498665 |
| 工作区准备 | 0.820240 | 1.125498 | 1.587330 | 1.393233 |
| Harness 启动 | 9.301970 | 7.549569 | 7.412102 | 7.259524 |
| Runtime 执行 | 53.016780 | 5.436948 | 899.847700 | 53.620164 |
| 停止与撤权 | 5.774516 | 5.576371 | 5.595336 | 5.551201 |
| 独立验证 | 13.989670 | 10.910786 | 10.273236 | 12.905302 |
| 清理 | 0.615227 | 1.364734 | 0.757151 | 1.597080 |

### large-scale-text-editing

| 阶段 | text-录1 | text-录2 | text-放2 |
| --- | --- | --- | --- |
| 排队 | 0.000022 | 0.000015 | 0.000015 |
| 任务 Sandbox 创建 | 0.806089 | 1.574061 | 1.507975 |
| 工作区准备 | 1.970630 | 2.712335 | 1.705927 |
| Harness 启动 | 7.204072 | 10.610703 | 9.428875 |
| Runtime 执行 | 519.499781 | 77.178976 | 6.008553 |
| 停止与撤权 | 5.679270 | 5.556017 | 5.552742 |
| 独立验证 | 47.764074 | 56.943372 | 11.598667 |
| 清理 | 0.802299 | 0.712425 | 1.300638 |

### custom-memory-heap-crash

| 阶段 | cpp-录1 |
| --- | --- |
| 排队 | 0.000010 |
| 任务 Sandbox 创建 | 0.317512 |
| 工作区准备 | 未采集 |
| Harness 启动 | 未采集 |
| Runtime 执行 | 未采集 |
| 停止与撤权 | 未采集 |
| 独立验证 | 未采集 |
| 清理 | 未采集 |

### sqlite-with-gcov

| 阶段 | sqlite-录1 | sqlite-录2 | sqlite-放2 |
| --- | --- | --- | --- |
| 排队 | 0.000013 | 0.000015 | 0.000027 |
| 任务 Sandbox 创建 | 1.262606 | 1.871654 | 0.947005 |
| 工作区准备 | 0.743427 | 0.745154 | 1.760605 |
| Harness 启动 | 7.229989 | 7.323080 | 7.142628 |
| Runtime 执行 | 233.002784 | 254.967549 | 12.721733 |
| 停止与撤权 | 5.588669 | 5.555073 | 5.587970 |
| 独立验证 | 11.465665 | 12.750182 | 11.731878 |
| 清理 | 0.621449 | 0.778872 | 0.655939 |

## 模型与工具工作量

| 运行 | 完成模型轮 | SSE帧 | 原生总调用 | exec | 其他工具 |
| --- | --- | --- | --- | --- | --- |
| git-录1 | 8 | 1017 | 7 | 7 | 0 |
| git-放1 | 7 | 695 | 7 | 7 | 0 |
| git-录2 | 9 | 1120 | 8 | 8 | 0 |
| git-放2 | 9 | 1120 | 8 | 8 | 0 |
| async-录1 | 3 | 718 | 2 | 2 | 0 |
| async-录2 | 3 | 575 | 2 | 2 | 0 |
| async-放1 | 未采集 | 未采集 | 未采集 | 未采集 | 未采集 |
| async-放2 | 3 | 575 | 2 | 2 | 0 |
| db-录1 | 12 | 3300 | 11 | 11 | 0 |
| db-放1 | 1 | 46 | 1 | 1 | 0 |
| db-录2 | 75 | 61292 | 74 | 74 | 0 |
| db-放3 | 12 | 3300 | 11 | 11 | 0 |
| text-录1 | 38 | 6789 | 37 | 19 | 18 |
| text-录2 | 13 | 2169 | 12 | 11 | 1 |
| text-放2 | 1 | 63 | 1 | 1 | 0 |
| cpp-录1 | 0 | 0 | 未采集 | 未采集 | 未采集 |
| sqlite-录1 | 42 | 4115 | 41 | 33 | 8 |
| sqlite-录2 | 36 | 5477 | 35 | 29 | 6 |
| sqlite-放2 | 3 | 321 | 3 | 3 | 0 |

完成轮数不等于HTTP请求数，迟到拒绝/重试单独留证；exec包含明确派发前拒绝，不等于guest执行次数。回放失败只执行原Tape前缀，不能当固定工作量完成。

## 已完成的离线 Trace 对账

新增6次实际运行各自先形成Snapshot，再生成Perfetto与OTLP。下表都实际导入TraceProcessor，起止/时长转换最大误差0ns；不是跨域同步精度。本轮后端发送、Jaeger/Prometheus查询及UI均UNRUN，不借此前结果冒充本轮验收。

| 运行 | 对账项 | 区间/Span | flow/parent | 完整性/覆盖 | 范围 |
| --- | --- | --- | --- | --- | --- |
| db-录2 | 85005 | 21239 | 387 | incomplete/incomplete | 不完整来源投影通过 |
| text-录2 | 3856 | 951 | 77 | complete/unknown | 限定投影通过 |
| sqlite-录2 | 9200 | 2287 | 192 | complete/unknown | 限定投影通过 |
| text-放2 | 764 | 178 | 20 | complete/unknown | 限定投影通过 |
| sqlite-放2 | 1372 | 330 | 28 | complete/unknown | 限定投影通过 |
| db-放3 | 4832 | 1195 | 60 | complete/unknown | 限定投影通过 |

旧git首次录制2424项、DB首次录制4840项对账保留；旧text/SQLite的21012/9064项是不完整来源投影，不能把旧失败提升为通过。本文新表不是全历史分析样本的重新验收。

## 逐条工具耗时

229条原生exec请求中220条有SDK等待、214条有guest时长。差额包含拒绝与缺测；不能解释成全部执行或全部成功。下表按归档真实顺序，模型轮从1开始、精确ToolCall ID查询；毫秒保留3位，原始纳秒与来源行在summary.json。process等其他工具未混入exec表。

<details>
<summary>fix-git：全部已归档exec</summary>

| 运行 | exec | 模型轮 | ToolCall ID | SDK ms | guest ms | exit | 关联/缺测 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| git-录1 | 1 | 1 | call_00_uXSUqKT4nfamf9mlfwNC1425 | 3042.680 | 2769.669 | 0 | exact |
| git-录1 | 2 | 2 | call_00_6ETWsKShlzJvqGNmMqhe0145 | 411.850 | 33.632 | 0 | exact |
| git-录1 | 3 | 3 | call_00_kY7iDL6X0dKprZVpeZB64807 | 531.154 | 40.129 | 0 | exact |
| git-录1 | 4 | 4 | call_00_zGOosK4EgcSo76kd4Bjq4660 | 552.106 | 289.925 | 1 | exact |
| git-录1 | 5 | 5 | call_00_pAKO0uoKjNcA94PsFTxK5415 | 876.767 | 48.685 | 0 | exact |
| git-录1 | 6 | 6 | call_00_mw1n40qVDQFFF9Ni1hKb5727 | 291.963 | 30.718 | 0 | exact |
| git-录1 | 7 | 7 | call_00_TCxyeryVKO45Llnpfj9m2683 | 318.503 | 42.481 | 0 | exact |
| git-放1 | 1 | 1 | call_00_uXSUqKT4nfamf9mlfwNC1425 | 3477.996 | 2604.197 | 0 | exact |
| git-放1 | 2 | 2 | call_00_6ETWsKShlzJvqGNmMqhe0145 | 317.478 | 43.217 | 0 | exact |
| git-放1 | 3 | 3 | call_00_kY7iDL6X0dKprZVpeZB64807 | 323.679 | 43.259 | 0 | exact |
| git-放1 | 4 | 4 | call_00_zGOosK4EgcSo76kd4Bjq4660 | 682.375 | 414.333 | 1 | exact |
| git-放1 | 5 | 5 | call_00_pAKO0uoKjNcA94PsFTxK5415 | 303.645 | 36.001 | 0 | exact |
| git-放1 | 6 | 6 | call_00_mw1n40qVDQFFF9Ni1hKb5727 | 305.476 | 33.249 | 0 | exact |
| git-放1 | 7 | 7 | call_00_TCxyeryVKO45Llnpfj9m2683 | 317.696 | 45.487 | 0 | exact |
| git-录2 | 1 | 1 | call_00_RnErxekYFhAVCFnHoH0J6395 | 2455.983 | 2185.361 | 0 | exact |
| git-录2 | 2 | 2 | call_00_E4lQIZpVVbB6ukiHdH3O9082 | 311.849 | 39.398 | 0 | exact |
| git-录2 | 3 | 3 | call_00_BVrLtFaTtA4HbIGkHz4s4535 | 1010.677 | 194.678 | 0 | exact |
| git-录2 | 4 | 4 | call_00_7CTkMtiYa14kTtsQDL1f5625 | 318.320 | 39.965 | 0 | exact |
| git-录2 | 5 | 5 | call_00_2sKRgvCnWRnaCHBZ6OV85297 | 553.945 | 281.674 | 1 | exact |
| git-录2 | 6 | 6 | call_00_yZebOIu6jgWUl6j22Lfu2616 | 311.773 | 35.863 | 0 | exact |
| git-录2 | 7 | 7 | call_00_0Xxvwv4YeLlPh4VxqNMU3955 | 299.925 | 35.834 | 0 | exact |
| git-录2 | 8 | 8 | call_00_MRpU9KFf2eZuBWq4nTZ39167 | 437.894 | 41.891 | 0 | exact |
| git-放2 | 1 | 1 | call_00_RnErxekYFhAVCFnHoH0J6395 | 2655.073 | 2378.010 | 0 | exact |
| git-放2 | 2 | 2 | call_00_E4lQIZpVVbB6ukiHdH3O9082 | 311.562 | 35.830 | 0 | exact |
| git-放2 | 3 | 3 | call_00_BVrLtFaTtA4HbIGkHz4s4535 | 379.510 | 101.438 | 0 | exact |
| git-放2 | 4 | 4 | call_00_7CTkMtiYa14kTtsQDL1f5625 | 517.351 | 34.360 | 0 | exact |
| git-放2 | 5 | 5 | call_00_2sKRgvCnWRnaCHBZ6OV85297 | 662.734 | 391.337 | 1 | exact |
| git-放2 | 6 | 6 | call_00_yZebOIu6jgWUl6j22Lfu2616 | 304.443 | 35.623 | 0 | exact |
| git-放2 | 7 | 7 | call_00_0Xxvwv4YeLlPh4VxqNMU3955 | 311.248 | 30.016 | 0 | exact |
| git-放2 | 8 | 8 | call_00_MRpU9KFf2eZuBWq4nTZ39167 | 317.845 | 39.954 | 0 | exact |

</details>

<details>
<summary>cancel-async-tasks：全部已归档exec</summary>

| 运行 | exec | 模型轮 | ToolCall ID | SDK ms | guest ms | exit | 关联/缺测 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| async-录1 | 1 | 1 | call_00_YuVUf9mcpLHaMdbmlthI1881 | 324.920 | 63.040 | 0 | exact |
| async-录1 | 2 | 2 | call_00_zjet9yrJoWTWxPEzEajM5606 | 812.540 | 532.195 | 0 | exact |
| async-录2 | 1 | 1 | call_00_YvqNjFOAQTLlDKruex9D3693 | 257.746 | 26.014 | 0 | exact |
| async-录2 | 2 | 2 | call_00_Apa3Tw2vOyqMsGDxqKPn9871 | 731.222 | 505.167 | 0 | exact |
| async-放2 | 1 | 1 | call_00_YvqNjFOAQTLlDKruex9D3693 | 789.802 | 43.879 | 0 | exact |
| async-放2 | 2 | 2 | call_00_Apa3Tw2vOyqMsGDxqKPn9871 | 816.735 | 580.488 | 0 | exact |

</details>

<details>
<summary>db-wal-recovery：全部已归档exec</summary>

| 运行 | exec | 模型轮 | ToolCall ID | SDK ms | guest ms | exit | 关联/缺测 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| db-录1 | 1 | 1 | call_00_DLnd8zd5HUPuv8PrlPin4213 | 630.947 | 106.855 | 127 | exact |
| db-录1 | 2 | 2 | call_00_WcJ3LlafkYJ7OXe3aQmK8790 | 487.211 | 242.506 | 0 | exact |
| db-录1 | 3 | 3 | call_00_W4Oj0MJLY0nqZS7QFEsD7953 | 287.708 | 33.257 | 0 | exact |
| db-录1 | 4 | 4 | call_00_QNvZyO175e2HoYoAJwrO5007 | 285.562 | 42.066 | 0 | exact |
| db-录1 | 5 | 5 | call_00_OG2Q2fdNuvC27tAYTz1v5266 | 610.882 | 362.225 | 0 | exact |
| db-录1 | 6 | 6 | call_00_3NZXrn3RVZvc2O6dJXXn1797 | 292.421 | 41.659 | 0 | exact |
| db-录1 | 7 | 7 | call_00_hlAqOfzlN7FaEGkJlnFA4488 | 302.054 | 48.795 | 0 | exact |
| db-录1 | 8 | 8 | call_00_IMFpCumxW3U0slJADw4x3621 | 298.135 | 45.747 | 0 | exact |
| db-录1 | 9 | 9 | call_00_nvSAdTYmisQUB4WwaQT82651 | 290.395 | 39.866 | 0 | exact |
| db-录1 | 10 | 10 | call_00_uF8uz6NL9PeWs4btPqaM6418 | 279.633 | 31.595 | 0 | exact |
| db-录1 | 11 | 11 | call_00_mrxm4gnB15ok1Tzn1Zps1863 | 300.139 | 54.379 | 0 | exact |
| db-放1 | 1 | 1 | call_00_DLnd8zd5HUPuv8PrlPin4213 | 358.762 | 50.162 | 127 | exact |
| db-录2 | 1 | 1 | call_00_WxIkQdIwMCgIQvWt7hUE3129 | 1146.049 | 174.304 | 127 | exact |
| db-录2 | 2 | 2 | call_00_jN3oQtdnLYrr684bFzg55335 | 366.850 | 37.658 | 2 | exact |
| db-录2 | 3 | 3 | call_00_CD9JfQ96PFINszxaMp4S0543 | 743.579 | 267.105 | 0 | exact |
| db-录2 | 4 | 4 | call_00_67K5DhNNc21lhqxFlpoJ2503 | 332.828 | 38.868 | 0 | exact |
| db-录2 | 5 | 5 | call_00_acugnfaGNHzANnWNFr6H2064 | 1169.848 | 886.297 | 0 | exact |
| db-录2 | 6 | 6 | call_00_i76vjbH1EqC8kVWEpNoy7874 | 324.620 | 35.975 | 0 | exact |
| db-录2 | 7 | 7 | call_00_qzi3ZrrvDAsrQewO4F5w5370 | 349.618 | 49.371 | 1 | exact |
| db-录2 | 8 | 8 | call_00_LSNO8wTds9jaIVrbWSMC4722 | 344.922 | 52.413 | 1 | exact |
| db-录2 | 9 | 9 | call_00_IbE4G6FcfwlJ1JDKUrwD4371 | 431.645 | 114.653 | 1 | exact |
| db-录2 | 10 | 10 | call_00_eOj58nD6Of8oGSgnT8Hp6885 | 362.900 | 38.575 | 0 | exact |
| db-录2 | 11 | 11 | call_00_cIZT6NJYP0c9AC8jkA2K4239 | 1571.289 | 1272.081 | 1 | exact |
| db-录2 | 12 | 12 | call_00_fsjTliRU3K0C5nfyRIJC9921 | 478.437 | 191.102 | 0 | exact |
| db-录2 | 13 | 13 | call_00_SDCaEaHSEuCzZDkSiXDR9771 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| db-录2 | 14 | 14 | call_00_yX3s63HrWxHdrSpAMnjy8874 | 319.043 | 42.732 | 2 | exact |
| db-录2 | 15 | 15 | call_00_AsV5c651n16d6sfBV4qT5475 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| db-录2 | 16 | 16 | call_00_T4etA7DgTFCka4UZq2VZ2564 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| db-录2 | 17 | 17 | call_00_2YPrIqHMxyWhnhyKEVzx5781 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| db-录2 | 18 | 18 | call_00_MGVvrLol4g6UTbOOmxel0056 | 338.198 | 41.106 | 0 | exact |
| db-录2 | 19 | 19 | call_00_Zjl3t8K4sJislkbiLjmg2712 | 331.117 | 38.642 | 0 | exact |
| db-录2 | 20 | 20 | call_00_oaoSnlV8iLQHXaPq3IhF3682 | 452.775 | 132.919 | 0 | exact |
| db-录2 | 21 | 21 | call_00_gdgKJhho2i4BGe9UwLfl6553 | 340.151 | 37.359 | 0 | exact |
| db-录2 | 22 | 22 | call_00_xC1MJU23pXIGuUinJDJF2848 | 653.225 | 245.066 | 0 | exact |
| db-录2 | 23 | 23 | call_00_7VySL1GTmAe804GgkVhc4335 | 313.678 | 39.878 | 0 | exact |
| db-录2 | 24 | 24 | call_00_aqxcEWjfErup18BdbLIK0891 | 342.359 | 42.484 | 0 | exact |
| db-录2 | 25 | 25 | call_00_9QCEDbsyQcgLtoJEbx225410 | 344.559 | 39.352 | 0 | exact |
| db-录2 | 26 | 26 | call_00_Z7dTyBom7alCkcezttpS5930 | 424.021 | 42.690 | 0 | exact |
| db-录2 | 27 | 27 | call_00_9QCRufSJQn9IicTH159e8635 | 336.688 | 43.550 | 0 | exact |
| db-录2 | 28 | 28 | call_00_SmQtVzHRSp6MduCXB1Tm8445 | 403.019 | 55.951 | 0 | exact |
| db-录2 | 29 | 29 | call_00_ckpqP40vgwShDtalqFqZ6562 | 423.113 | 124.684 | 0 | exact |
| db-录2 | 30 | 30 | call_00_PHw9epcbxEUzpxXLVMIW4063 | 1174.114 | 877.757 | 0 | exact |
| db-录2 | 31 | 31 | call_00_bAJ9g6pomEfVxuzBt4o81736 | 834.256 | 334.515 | 0 | exact |
| db-录2 | 32 | 32 | call_00_wmf2Hh5IZV2Jhxksefxd5890 | 356.194 | 43.701 | 2 | exact |
| db-录2 | 33 | 33 | call_00_ARVNk8rY17DInE0w7Pz48855 | 383.759 | 108.154 | 0 | exact |
| db-录2 | 34 | 34 | call_00_FQy9QhBZAyi9vNGXmwBb6322 | 355.051 | 33.881 | 0 | exact |
| db-录2 | 35 | 35 | call_00_fjLBewgIDyVbgFLxnXNx9687 | 331.118 | 33.191 | 0 | exact |
| db-录2 | 36 | 36 | call_00_wCXCHi8hYMPCo0IqH3Pt4985 | 437.373 | 118.332 | 0 | exact |
| db-录2 | 37 | 37 | call_00_P0LkbeLJ18bpK4h52xn91929 | 361.509 | 82.651 | 0 | exact |
| db-录2 | 38 | 38 | call_00_yqOH6HWDwwApaOv8XZQL3486 | 324.836 | 36.825 | 1 | exact |
| db-录2 | 39 | 39 | call_00_20m80lqOrJ6Xwwu3DObW5538 | 701.937 | 408.698 | 0 | exact |
| db-录2 | 40 | 40 | call_00_Kfv6W4ulvI1HTxsavp9Z1669 | 436.129 | 57.664 | 0 | exact |
| db-录2 | 41 | 41 | call_00_84XrJSPydhHrzRfjjAaN3670 | 338.795 | 49.667 | 0 | exact |
| db-录2 | 42 | 42 | call_00_j9Nff2c9ELXalse19Uyd3058 | 379.898 | 65.375 | 1 | exact |
| db-录2 | 43 | 43 | call_00_OUlgHUj0jJ5lLpgcalD50660 | 363.379 | 60.570 | 1 | exact |
| db-录2 | 44 | 44 | call_00_gc2H5aFNEKtFoeaEXDnH5921 | 345.575 | 68.911 | 0 | exact |
| db-录2 | 45 | 45 | call_00_LSStgIyh3YvZy5x1yMB74765 | 364.678 | 70.286 | 0 | exact |
| db-录2 | 46 | 46 | call_00_C739pYxFZGBAG8cLpqk99818 | 556.562 | 259.146 | 0 | exact |
| db-录2 | 47 | 47 | call_00_88yoQhkcvuvjATahPZuc8867 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| db-录2 | 48 | 48 | call_00_g5KfyMhJ3SPEG2PCLYrw8526 | 320.268 | 33.687 | 0 | exact |
| db-录2 | 49 | 49 | call_00_pv7pRUEepIlrU9w2Wl1h9551 | 341.689 | 50.023 | 0 | exact |
| db-录2 | 50 | 50 | call_00_tedL8J57tvpz62fdZdZ22531 | 334.210 | 44.911 | 1 | exact |
| db-录2 | 51 | 51 | call_00_G6sxAiGFfmML24P3rpiN5642 | 343.645 | 50.635 | 0 | exact |
| db-录2 | 52 | 52 | call_00_CrDiqdPOb21weXzT8xBE1293 | 340.945 | 52.891 | 0 | exact |
| db-录2 | 53 | 53 | call_00_A9nfBBn5xBsQrJGIRziW5501 | 351.259 | 48.816 | 0 | exact |
| db-录2 | 54 | 54 | call_00_8UAbIkNkkLLrV0xB5pOJ6389 | 349.892 | 54.812 | 0 | exact |
| db-录2 | 55 | 55 | call_00_wWZrq8r0v1ZsoHtOZLf61033 | 329.396 | 41.484 | 0 | exact |
| db-录2 | 56 | 56 | call_00_ikkjMNvmsvc8iDVeaJjO1691 | 1765.130 | 1456.593 | 0 | exact |
| db-录2 | 57 | 57 | call_00_9fYvOG41M7juYpyt9h1e2044 | 454.165 | 182.305 | 0 | exact |
| db-录2 | 58 | 58 | call_00_3Ym8r3Zqnq1H1xV07ml74297 | 407.232 | 104.639 | 0 | exact |
| db-录2 | 59 | 59 | call_00_uSEpDLWvBJVqHvdeide80333 | 354.254 | 37.670 | 0 | exact |
| db-录2 | 60 | 60 | call_00_m3temOjvEewQcdAayBZS0541 | 326.407 | 32.103 | 0 | exact |
| db-录2 | 61 | 61 | call_00_VRWFazOp76Yh6XAtmyhW8596 | 340.421 | 41.177 | 0 | exact |
| db-录2 | 62 | 62 | call_00_hqdYnUaojVwlSkvLFjK94896 | 334.799 | 33.472 | 0 | exact |
| db-录2 | 63 | 63 | call_00_aWzClS6d15CbTbXE4eT21820 | 317.370 | 32.877 | 0 | exact |
| db-录2 | 64 | 64 | call_00_wGmVtk4llLWACoBJKm4b3859 | 309.442 | 29.799 | 0 | exact |
| db-录2 | 65 | 65 | call_00_BHQeoUANM6WVLlFyhbcg6329 | 330.757 | 46.800 | 0 | exact |
| db-录2 | 66 | 66 | call_00_F1ITampFzaMFQ7Zqas9K0662 | 344.414 | 43.640 | 0 | exact |
| db-录2 | 67 | 67 | call_00_FrlNc4YvNM1tOEZ5eOdI7157 | 305.486 | 31.325 | 0 | exact |
| db-录2 | 68 | 68 | call_00_2iQmttYseAYdGufgLqh01833 | 365.846 | 55.636 | 0 | exact |
| db-录2 | 69 | 69 | call_00_6wr6QvseLF5uqCB8x1Og0085 | 508.634 | 222.928 | 0 | exact |
| db-录2 | 70 | 70 | call_00_CD9CD4SwBe3dIsj96yUW3859 | 542.958 | 243.100 | 0 | exact |
| db-录2 | 71 | 71 | call_00_3Mmod9uOQ41kwbzWk8O50512 | 487.245 | 171.917 | 0 | exact |
| db-录2 | 72 | 72 | call_00_5FEqsdzZ0K6g91d2bXLc3497 | 347.748 | 45.731 | 0 | exact |
| db-录2 | 73 | 73 | call_00_A32CQO2ROnCkoQp3HTTQ2426 | 316.848 | 32.492 | 0 | exact |
| db-录2 | 74 | 74 | call_00_leet6bhYw3q41Tb3nqK55473 | 328.260 | 33.063 | 0 | exact |
| db-放3 | 1 | 1 | call_00_DLnd8zd5HUPuv8PrlPin4213 | 532.934 | 91.792 | 127 | exact |
| db-放3 | 2 | 2 | call_00_WcJ3LlafkYJ7OXe3aQmK8790 | 460.366 | 182.028 | 0 | exact |
| db-放3 | 3 | 3 | call_00_W4Oj0MJLY0nqZS7QFEsD7953 | 330.859 | 32.745 | 0 | exact |
| db-放3 | 4 | 4 | call_00_QNvZyO175e2HoYoAJwrO5007 | 317.215 | 39.234 | 0 | exact |
| db-放3 | 5 | 5 | call_00_OG2Q2fdNuvC27tAYTz1v5266 | 711.081 | 240.316 | 0 | exact |
| db-放3 | 6 | 6 | call_00_3NZXrn3RVZvc2O6dJXXn1797 | 332.638 | 46.021 | 0 | exact |
| db-放3 | 7 | 7 | call_00_hlAqOfzlN7FaEGkJlnFA4488 | 316.903 | 39.297 | 0 | exact |
| db-放3 | 8 | 8 | call_00_IMFpCumxW3U0slJADw4x3621 | 433.864 | 38.334 | 0 | exact |
| db-放3 | 9 | 9 | call_00_nvSAdTYmisQUB4WwaQT82651 | 314.474 | 37.000 | 0 | exact |
| db-放3 | 10 | 10 | call_00_uF8uz6NL9PeWs4btPqaM6418 | 309.000 | 30.125 | 0 | exact |
| db-放3 | 11 | 11 | call_00_mrxm4gnB15ok1Tzn1Zps1863 | 323.369 | 46.062 | 0 | exact |

</details>

<details>
<summary>large-scale-text-editing：全部已归档exec</summary>

| 运行 | exec | 模型轮 | ToolCall ID | SDK ms | guest ms | exit | 关联/缺测 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| text-录1 | 1 | 1 | call_00_8qaJeXifc8fxSOQWsoFz4287 | 2357.342 | 2086.792 | 0 | exact |
| text-录1 | 2 | 2 | call_00_SMvhdLyCN52cKmppKFBo8325 | 388.822 | 121.699 | 0 | exact |
| text-录1 | 3 | 3 | call_00_3VxW84cnclsNf0mhgF8V0025 | 959.146 | 148.473 | 0 | exact |
| text-录1 | 4 | 4 | call_00_ABYeleWW1N7qL7ivyWFU0311 | 436.561 | 27.854 | 2 | exact |
| text-录1 | 5 | 5 | call_00_4yyX3vC8NhOGaGkZwqcu3898 | 5122.504 | 4855.105 | 0 | exact |
| text-录1 | 6 | 6 | call_00_NSn6hKp8OT3fxeI3aXwm0664 | 338.632 | 85.267 | 0 | exact |
| text-录1 | 7 | 7 | call_00_JYMWjgIjeCluFxbb9VDh9317 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| text-录1 | 8 | 8 | call_00_G90ikbzHVJ5W0k5OFhUC1442 | 325.667 | 61.639 | 0 | exact |
| text-录1 | 9 | 9 | call_00_5dQxy6zCLYmRJptLV8938718 | 363.486 | 95.771 | 0 | exact |
| text-录1 | 10 | 10 | call_00_YnSQsxSIvJXvVOmXiWBo8100 | 28516.487 | 28244.667 | 0 | exact |
| text-录1 | 11 | 12 | call_00_qEfvPfZfLMWtrienN4gM6155 | 75484.468 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| text-录1 | 12 | 17 | call_00_7ojMYvOQtysGD0fcgLGm1111 | 27566.777 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| text-录1 | 13 | 20 | call_00_nwKjZZbsjJyU7jwLWTb49326 | 76983.364 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| text-录1 | 14 | 24 | call_00_AOl8njmpg6W5rWoENzV42964 | 1527.896 | 81.548 | 0 | exact |
| text-录1 | 15 | 25 | call_00_b9zPM2EoCSwdRltDLbzh6078 | 1507.528 | 89.194 | 0 | exact |
| text-录1 | 16 | 26 | call_00_o9zMSUXSKkaZehcfsprA6481 | 48211.415 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| text-录1 | 17 | 29 | call_00_2IhCs8XsAh78qogmneWf3855 | 82197.909 | 81866.985 | 0 | exact |
| text-录1 | 18 | 33 | call_00_OKRqzL5ACt6LBOiOjp5y5598 | 82258.134 | 81923.177 | 0 | exact |
| text-录1 | 19 | 37 | call_00_KHWBDTTEsL5ckBhxrYDc1589 | 440.885 | 92.202 | 0 | exact |
| text-录2 | 1 | 1 | call_00_Nwuyg4tC8Px7Qddj64J40317 | 737.508 | 340.486 | 0 | exact |
| text-录2 | 2 | 2 | call_00_XivTgOMzF4jgcciVTWse4302 | 2663.687 | 2423.801 | 0 | exact |
| text-录2 | 3 | 3 | call_00_TJUNqesV9rcY2Xt6eJcS4343 | 275.488 | 37.374 | 0 | exact |
| text-录2 | 4 | 4 | call_00_7wFPVfhkehk8cRJsi4KW5528 | 607.107 | 363.707 | 0 | exact |
| text-录2 | 5 | 5 | call_00_Lz4WXHkf6kcvqJzwR9dM6452 | 1796.243 | 1557.520 | 0 | exact |
| text-录2 | 6 | 6 | call_00_z4lm098pm5VR9mlCghMy7936 | 665.785 | 38.531 | 0 | exact |
| text-录2 | 7 | 7 | call_00_1kHb70qWihASP6b00WtB1735 | 273.438 | 35.129 | 0 | exact |
| text-录2 | 8 | 8 | call_00_86VJon4CWAFtEl1SXIQm1272 | 280.620 | 45.547 | 0 | exact |
| text-录2 | 9 | 9 | call_00_OABOet5h1DRuyJYoRo9m7017 | 35847.419 | 35608.582 | 0 | exact |
| text-录2 | 10 | 11 | call_00_93ge1JlLt1NYrVYJ79Ld2447 | 304.214 | 57.512 | 0 | exact |
| text-录2 | 11 | 12 | call_00_C6qVzYvY3IFtaPjh44vy5897 | 280.188 | 41.203 | 1 | exact |
| text-放2 | 1 | 1 | call_00_Nwuyg4tC8Px7Qddj64J40317 | 643.880 | 403.523 | 0 | exact |

</details>

<details>
<summary>custom-memory-heap-crash：全部已归档exec</summary>

Agent未运行，没有工具样本。

</details>

<details>
<summary>sqlite-with-gcov：全部已归档exec</summary>

| 运行 | exec | 模型轮 | ToolCall ID | SDK ms | guest ms | exit | 关联/缺测 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| sqlite-录1 | 1 | 1 | call_00_11wRFsWTkv7g8Ur2fp3K9501 | 656.520 | 276.299 | 0 | exact |
| sqlite-录1 | 2 | 2 | call_00_uNfHt6TEU0JN4r6ATHvJ5173 | 1690.856 | 1415.029 | 0 | exact |
| sqlite-录1 | 3 | 3 | call_00_ividmwzHRU5mBaLxBN8d0106 | 311.359 | 37.503 | 0 | exact |
| sqlite-录1 | 4 | 4 | call_00_PWII3d5AxUvWrQTGOYwH6280 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| sqlite-录1 | 5 | 5 | call_00_ECoPB715Ly8jskEreKEG5824 | 17269.632 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| sqlite-录1 | 6 | 8 | call_00_VGA8YaE6tinntMxe11vO7035 | 15703.570 | 未采集 | 未采集 | guest_duration_ns_not_collected；exit_code_not_collected |
| sqlite-录1 | 7 | 11 | call_00_7R6oufqalFhRm6VQsIRi5231 | 310.312 | 37.308 | 0 | exact |
| sqlite-录1 | 8 | 12 | call_00_EBB3JRUnEJ3hn4MWdYAD1133 | 945.858 | 657.764 | 0 | exact |
| sqlite-录1 | 9 | 13 | call_00_rkFzKYOapt9Bk4axhn6F3629 | 302.255 | 32.179 | 0 | exact |
| sqlite-录1 | 10 | 14 | call_00_lvHR1iVX6gmi8mHIvCsx7761 | 475.954 | 34.534 | 0 | exact |
| sqlite-录1 | 11 | 15 | call_00_HhTLnxap1NTJIZKq1DrQ3593 | 4403.224 | 4105.410 | 0 | exact |
| sqlite-录1 | 12 | 16 | call_00_iVzYCxAoaqSumLlSGZ8m9896 | 102593.873 | 102295.267 | 0 | exact |
| sqlite-录1 | 13 | 20 | call_00_EhwXVNFJEFVrrex8Fzjc2589 | 634.624 | 244.319 | 0 | exact |
| sqlite-录1 | 14 | 22 | call_00_ZxxmJ52pYbo84xZDiOVS6484 | 387.237 | 86.294 | 0 | exact |
| sqlite-录1 | 15 | 23 | call_00_JQXfwRyj0NOz9M9BkimY2333 | 314.506 | 39.583 | 2 | exact |
| sqlite-录1 | 16 | 24 | call_00_4qMTnUdTGMjrVfO8bLLX8981 | 322.406 | 49.944 | 0 | exact |
| sqlite-录1 | 17 | 25 | call_00_gkYmWUJ3kNu6YMw6q9kT2411 | 334.201 | 36.686 | 0 | exact |
| sqlite-录1 | 18 | 26 | call_00_BLp6VfhVDVrLBaR62YD18932 | 302.937 | 34.485 | 0 | exact |
| sqlite-录1 | 19 | 27 | call_00_D8MwQfAUTi9zcOJw6WTM7692 | 未采集 | 未采集 | 未采集 | exact_mapping_missing_or_duplicate |
| sqlite-录1 | 20 | 28 | call_00_MDuYkeRTUctDjfIF8Kkd7444 | 317.600 | 33.231 | 0 | exact |
| sqlite-录1 | 21 | 29 | call_00_NrV1RK6USQzacpOFZWMN8120 | 310.982 | 38.018 | 2 | exact |
| sqlite-录1 | 22 | 30 | call_00_dw9K3BgceEEom0Jt6uaD3007 | 304.358 | 36.261 | 0 | exact |
| sqlite-录1 | 23 | 31 | call_00_9MuOYrJ7zM2SCoaWpD1x7069 | 313.659 | 30.687 | 0 | exact |
| sqlite-录1 | 24 | 32 | call_00_naO13CZI5ukGEQ8VoWaH3595 | 677.600 | 236.071 | 0 | exact |
| sqlite-录1 | 25 | 33 | call_00_RPXs5Tkkb6M05ioOslYm1706 | 305.839 | 32.997 | 0 | exact |
| sqlite-录1 | 26 | 34 | call_00_RAVVQsFWqKId9vEnvrrO5156 | 406.043 | 98.625 | 1 | exact |
| sqlite-录1 | 27 | 35 | call_00_OaGhFLQIqa6dUM0KHmV63490 | 353.366 | 43.887 | 0 | exact |
| sqlite-录1 | 28 | 36 | call_00_recDS9Lx54YfD3l1XwHb3529 | 353.964 | 62.651 | 0 | exact |
| sqlite-录1 | 29 | 37 | call_00_VE7CLjGUr4qayOqp4n420809 | 339.236 | 33.348 | 0 | exact |
| sqlite-录1 | 30 | 38 | call_00_LXvzYih4NtnGRXB18tNr5786 | 344.838 | 41.617 | 0 | exact |
| sqlite-录1 | 31 | 39 | call_00_pZoLUVIhn1ahrCDinSpa1655 | 334.472 | 48.998 | 0 | exact |
| sqlite-录1 | 32 | 40 | call_00_tKLk3gg7TBrYz12SKwCZ7397 | 535.520 | 251.274 | 0 | exact |
| sqlite-录1 | 33 | 41 | call_00_kKXaEaH5bHJ7ch5jYoQU2122 | 434.036 | 44.542 | 0 | exact |
| sqlite-录2 | 1 | 1 | call_00_0L6DuECXCDLsFSOojChe9317 | 1130.819 | 687.271 | 0 | exact |
| sqlite-录2 | 2 | 2 | call_00_hvAj455jTEkIBdJGRZ610657 | 1093.900 | 804.345 | 0 | exact |
| sqlite-录2 | 3 | 3 | call_00_VsW6JJrWydHFmv4Y4vAe5153 | 860.269 | 580.827 | 0 | exact |
| sqlite-录2 | 4 | 4 | call_00_Mq71A7NvhhCkUoD0ZpI96314 | 331.463 | 31.471 | 0 | exact |
| sqlite-录2 | 5 | 5 | call_00_8to0ZIZRcqxSnax9OI6g7142 | 321.341 | 35.520 | 0 | exact |
| sqlite-录2 | 6 | 6 | call_00_LBwQtvOI17UAQYmDpGgh4413 | 763.615 | 474.534 | 0 | exact |
| sqlite-录2 | 7 | 7 | call_00_qvwoczeHzo7nFC3GNgpr0857 | 307.643 | 39.747 | 0 | exact |
| sqlite-录2 | 8 | 8 | call_00_Emzy80vncWOvy75IPxjY3098 | 3594.977 | 3324.816 | 0 | exact |
| sqlite-录2 | 9 | 9 | call_00_uwdogTWrl8htFj1mxhzn0609 | 82950.402 | 82664.316 | 0 | exact |
| sqlite-录2 | 10 | 13 | call_00_vaM44hsTMaEcSvwUVnTT3447 | 503.660 | 40.997 | 0 | exact |
| sqlite-录2 | 11 | 14 | call_00_cCV6YBbwr8YCAhiTOujn0575 | 304.180 | 28.554 | 0 | exact |
| sqlite-录2 | 12 | 15 | call_00_RqosP5XkKSE0uTpFTJHh3599 | 319.974 | 28.367 | 0 | exact |
| sqlite-录2 | 13 | 16 | call_00_PlsfeNBT3GAYiRa7ic199673 | 未采集 | 未采集 | 未采集 | 未派发 / elevated_policy_rejected |
| sqlite-录2 | 14 | 17 | call_00_GQXjU0UMeRoxvH3WQkbR9975 | 326.682 | 37.055 | 2 | exact |
| sqlite-录2 | 15 | 18 | call_00_FM65jkvrPCBBPekUDzoZ1622 | 323.889 | 36.477 | 2 | exact |
| sqlite-录2 | 16 | 19 | call_00_aUXZCBNhL1rQrt78skoN9870 | 77552.122 | 77089.916 | 2 | exact |
| sqlite-录2 | 17 | 23 | call_00_zzgsqGIwRrGM3b890gJE2881 | 309.068 | 27.000 | 0 | exact |
| sqlite-录2 | 18 | 24 | call_00_LZuwvIza6wytKn65OjM75848 | 311.172 | 30.784 | 0 | exact |
| sqlite-录2 | 19 | 25 | call_00_FwaNsNnXtYAzeKzwA3GM2651 | 305.472 | 30.801 | 0 | exact |
| sqlite-录2 | 20 | 26 | call_00_UsRKvFt8PTiODAaoRjyj5305 | 577.826 | 299.200 | 0 | exact |
| sqlite-录2 | 21 | 27 | call_00_Jg3QZ1fVhR4CiZosDT6Y3053 | 310.924 | 33.954 | 0 | exact |
| sqlite-录2 | 22 | 28 | call_00_JG0tilj9hC4v9hBn299P6454 | 297.642 | 27.172 | 2 | exact |
| sqlite-录2 | 23 | 29 | call_00_n0gowSKJj5aEEqDvgmbQ3046 | 312.162 | 26.498 | 0 | exact |
| sqlite-录2 | 24 | 30 | call_00_JqWHo0Pa2EMhfY8bQ7tG7162 | 418.230 | 128.892 | 0 | exact |
| sqlite-录2 | 25 | 31 | call_00_vlXteC38paLoMSmdkqgZ5607 | 322.029 | 33.774 | 0 | exact |
| sqlite-录2 | 26 | 32 | call_00_SUjXSPj3XvEbWEPkKqor0965 | 478.977 | 190.122 | 0 | exact |
| sqlite-录2 | 27 | 33 | call_00_hJsK308ajv8z1azdu2LT3165 | 306.568 | 30.014 | 0 | exact |
| sqlite-录2 | 28 | 34 | call_00_LFF4qGm1qisLEl7FzM0J6680 | 334.199 | 43.797 | 1 | exact |
| sqlite-录2 | 29 | 35 | call_00_d3rHLnQG6oCkk1QrJpBq1299 | 349.327 | 40.022 | 0 | exact |
| sqlite-放2 | 1 | 1 | call_00_0L6DuECXCDLsFSOojChe9317 | 447.690 | 132.270 | 0 | exact |
| sqlite-放2 | 2 | 2 | call_00_hvAj455jTEkIBdJGRZ610657 | 895.510 | 625.695 | 0 | exact |
| sqlite-放2 | 3 | 3 | call_00_VsW6JJrWydHFmv4Y4vAe5153 | 931.672 | 655.932 | 0 | exact |

</details>

## 证据位置与复核边界

当前汇总：ECS `/opt/aries-artifacts/tb-six-summary-round-3/summary.json`，SHA `e22aa1f3ba19e2f9a13e04bad4773fd71d2552253d9c8b86c3e1c3bbbdf4f737`；这是安全字段归档，包含逐原件SHA、来源行、原始纳秒，未包含私有ToolResult/命令/凭据。汇总不替代原run-result、CTRF、Tape和清理回执。

| 运行 | ECS目录 | 原evidence SHA-256 |
| --- | --- | --- |
| git-录1 | /opt/aries-artifacts/tb-six-fix-git-record-1 | e90d19131441426abec977cb8497052321320a29fa48e631b0216cf6cfa8e7a3 |
| git-放1 | /opt/aries-artifacts/tb-six-fix-git-replay-1 | e7dfb7b9a6ae66b2da724ccadaa3e0afc34e002def3bab80c5fbf91a82cd6bb6 |
| git-录2 | /opt/aries-artifacts/tb-six-fix-git-record-2 | 247c1181e8cc3dbad1046303050674482a82379f58571d4c05ccace9dbfcf5a3 |
| git-放2 | /opt/aries-artifacts/tb-six-fix-git-replay-2 | d7d4778946bd38c55f4517170f52e4238446216576942de0302eb1cc7521f413 |
| async-录1 | /opt/aries-artifacts/tb-six-cancel-async-tasks-record-1 | 9f04f279ee6180842280b7a5fcbeceede5440b987f39ba43784da41ea825fb42 |
| async-录2 | /opt/aries-artifacts/tb-six-cancel-async-tasks-record-2 | bb96a07fb38cddfa46fc2ca5718492dd20b9817ec292f4d06399c20ac22d1820 |
| async-放1 | /opt/aries-artifacts/tb-six-cancel-async-tasks-replay-1 | 无原evidence；预检目录保留 |
| async-放2 | /opt/aries-artifacts/tb-six-cancel-async-tasks-replay-2 | 103a40e3cff7db4a02e1c154c4f7023453edf196019551ee9dc4a2a9dc994b2e |
| db-录1 | /opt/aries-artifacts/tb-six-db-wal-recovery-record-1 | 21681f75094155e81372277bc9d73c88bfd20eda9e60c2d67590c95a54ee8f0b |
| db-放1 | /opt/aries-artifacts/tb-six-db-wal-recovery-replay-1 | 517917768096843b3aff361a0bad30da7de00e2f40fa8616d4cdc51cff282caa |
| db-录2 | /opt/aries-artifacts/tb-six-db-wal-recovery-record-2 | d95f42c8c4ce81373db25b1e7ee0663700c56c5f57b31d30d3ed80c57269e6d5 |
| db-放3 | /opt/aries-artifacts/tb-six-db-wal-recovery-replay-3 | 8e68b0cf7c69ae2fce6b759e17502be5087a29e71b1b3332224f022005c7090c |
| text-录1 | /opt/aries-artifacts/tb-six-large-scale-text-editing-record-1 | 5b81a2319bbd7b16708d97ea45f2b3a674045e39523a85eca2c2283de845bd92 |
| text-录2 | /opt/aries-artifacts/tb-six-large-scale-text-editing-record-2 | ada4403bbee5b53ad9ca1b322feab411774122d219d41232e2eeb8fa5a70b852 |
| text-放2 | /opt/aries-artifacts/tb-six-large-scale-text-editing-replay-2 | 3fa0ab8e87c62ff6123c397a9811c35b3f2ff28bcfb73bdf81de9075f04a157d |
| cpp-录1 | /opt/aries-artifacts/tb-six-custom-memory-heap-crash-record-1 | 21958b13b5026320e496f51c5c9a1fef374875de5d037d82ad4e7f4755b9e10a |
| sqlite-录1 | /opt/aries-artifacts/tb-six-sqlite-with-gcov-record-1 | 7f517c9af6b426f428cd39eb4d00b5499d03719ca09cd70fb50e6835f0012cac |
| sqlite-录2 | /opt/aries-artifacts/tb-six-sqlite-with-gcov-record-2 | f3ba233418e9393d3d1a215dcd5cd151dc646712ee55c26bf1e076655f4ec4ec |
| sqlite-放2 | /opt/aries-artifacts/tb-six-sqlite-with-gcov-replay-2 | cdf5ec6b3cf6f58982ed63abe9f8d3984b54b91e2e31a704b52cb773d3609c85 |

新增两份 `acceptance-review-2/review.json` 位于text-录2与sqlite-录2目录，SHA分别为 `495c195c6e5ed3a7e13d134606cba6c77f4a0fa9f330db1a134b29c3a0a5e1bf`、`ace6712905f821ec098708ec36760c023148d6b8efbe37417e0a642997428edc`。新对账 `/opt/aries-artifacts/tb-six-replay-review-3/evidence.json` 的43项检查通过，SHA `9bd8bebfefdfa6ff8ea8540983aebe8d01c141e90e5d89930bfa81f80420501f`；这是归档复核，不增加云样本。

本轮新增6次运行固定release-4，605文件manifest `d61c2a1245ded2f561843143276a177e1d45540250b10f8e922012a41e318710`；原生发布证据 `9046876419563e64ed6accd257964e430386d7d50b86f27cf7d9845776fcf0d1`，SWR digest `sha256:4f519985234cc93f653314ea672bcf25c0abcc9d0009597ef82f066c08d02d2d`。原生-1/-2失败、早期release及旧Tape均保留。完整工程、原生、离线和复核版本见[验证归档](observability/VALIDATION_AND_REPRODUCE.md#tb-six-current)。

这不是跨云性能排名或六题全部验收完成。未通过的case继续保留；系统问题修复后要真实重测，不通过改断言、删失败分母、参考解或宽松matcher换取通过。R05 I/O支线仍按用户要求暂停，多任务待单任务阶段交付后再推进。
