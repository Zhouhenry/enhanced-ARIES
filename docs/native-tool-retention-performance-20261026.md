# release14 原生工具采集修复：真实回归性能归档

2026-10-26，在ECS运行E2B A模式 `large-scale-text-editing-terminal-result-v2` 同Tape Strict Replay。只有Runner采集修复升级到release14；原Tape、Agent镜像、任务、评分及预算不变。该轮是回归样本，不替换Dashboard原有三次跨云样本。

**实际结果：Strict 23/23轮、20次exec、2次后台等待、2个后台真实终态，原独立Verifier 5/5、reward=1。** 创建1个Sandbox并确认删除，账本全部absent，Runner进程组退出，cleanup errors为空。未重新录制、未调用真实模型。

## 阶段耗时

| 阶段 | 秒 |
| --- | ---: |
| 排队（不含在Attempt中） | 0.000016 |
| Sandbox创建 | 1.302081 |
| 工作区准备 | 2.782517 |
| Harness启动 | 7.303650 |
| Runtime执行 | 206.808197 |
| 停止与撤权 | 5.546196 |
| 独立验证 | 50.842723 |
| 清理 | 0.922057 |

Attempt为 **277.584145秒**；不含排队和admission后的dispatch延迟。其内七阶段合计275.507421秒，剩余2.076724秒包含阶段间编排等未细分区间，不能称纯Agent loop。Runtime是Harness.Run内部较窄边界；Verifier包含准备、评分与报告收集。定义见[阶段说明](performance-stages.md)。

## 逐工具控制端与guest计量

控制端区间是SDK提交前到Wait返回，包含RPC、执行和流收尾；guest区间为同一guest时钟下的包装器观测。两列不相加，差值也不能直接叫网络耗时。工具失败退出可能是Agent正常诊断步骤，题目正确性由最终Verifier判断。每次动作沿用大文本同Tape，具体任务步骤见[Dashboard](dashboard/large-scale-text-editing/index.html)。

| exec序号 | 模型轮次 | 控制端秒 | guest秒 | exit |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 1 | 2.800711 | 2.556694 | 0 |
| 2 | 2 | 0.494315 | 0.200157 | 0 |
| 3 | 3 | 0.452702 | 0.170663 | 0 |
| 4 | 4 | 0.712973 | 0.487984 | 1 |
| 5 | 5 | 0.319607 | 0.033527 | 0 |
| 6 | 6 | 0.274802 | 0.034633 | 0 |
| 7 | 7 | 0.248525 | 0.029519 | 0 |
| 8 | 8 | 0.253295 | 0.031984 | 0 |
| 9 | 9 | 0.303974 | 0.076036 | 0 |
| 10 | 10 | 0.276345 | 0.029880 | 0 |
| 11 | 11 | 0.320257 | 0.034889 | 0 |
| 12 | 12 | 0.283312 | 0.040513 | 0 |
| 13 | 13 | 0.336498 | 0.038392 | 0 |
| 14 | 14 | 0.253879 | 0.036327 | 0 |
| 15 | 15 | 0.303694 | 0.040313 | 0 |
| 16 | 16 | 0.277824 | 0.033777 | 0 |
| 17 | 17 | 0.429407 | 0.031664 | 0 |
| 18 | 18 | 0.335391 | 0.093916 | 0 |
| 19 | 20 | 31.632191 | 31.385614 | 0 |
| 20 | 22 | 0.270844 | 0.046424 | 0 |

## 观测证据与限制

- 本轮原生输入在frame/enqueue/consume三个边界均为1371个唯一delivery ID。保留64条：22 start、22 result、20 command_output/end；collector complete=true、issues=[]、terminal=true。
- 1371没有超过旧4096阈值。最后native sequence=5187不代表收到5187条；不能用此轮冒充真实20k边界压测。20k非工具事件的旧FAIL→新PASS是独立测试receiver验证。
- 同一快照的Perfetto与OTLP投影13179项对账、0失败；主Trace各3280区间。4个原生后台区间没有可信UTC锚，继续排除出主Trace/OTLP，不拼伪时间线。
- integrity=complete，coverage=unknown；本轮没有重新执行Jaeger服务端投递验收，标记UNRUN_THIS_RUN。CPU/内存等资源不从旧运行移植，本文摘要没有完成本轮资源归属审核，保持未审核。
- 旧DB record3依然观测incomplete、业务5/7，不能事后升级。独立Recorder队列与R05文件IO遗留不因这个修复关闭。

## 复现与固定证据

ECS目录：`/opt/aries-artifacts/tb-repro-text-v2-e2b-release14-replay-1`。真实运行时间：`2026-10-26T13:29:42.330971+00:00` 至 `2026-10-26T13:34:29.246125+00:00`。

| 证据 | SHA-256 |
| --- | --- |
| 原始运行证据 | `6efd5d933a6eed42ab33479044169180bd52fcccf3727a25dec7023b225782bd` |
| 双Trace对账 | `0bbbd320aea8e961987c9a156bdcf754458dfc7efe7a6f4f5024161e0e2ed1f7` |
| 原生collector索引 | `43abcf8ffd1f9913ea4ad10648889f91ea7571ff27cef61c6c8e4574fdb0cf1b` |
| 原生事件 | `0c8ba58a3d24d70402797f05da477c093a874e623f574dcbf8fadbd6680de83b` |
| 冻结release14 | `0f1bea40bd025dbf695b1b0e317cb4d204bc4fa99e53c9b11609f828894d2d1c` |
| 真实回归审查回执 | `cd9c2ccb059fc5a16c893e1090ea5f680a01287db8078049f957e1d98af5d5cd` |

真实回归采用已归档driver及validation，不改变Strict matcher、ToolResult或评分。完整工程检查与源码清单见[release14验证](observability/VALIDATION_AND_REPRODUCE.md#native-tool-retention-release14)。
