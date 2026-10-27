# C++ 堆损坏调试：terminal-result-v2 独立变体

状态：材料仅完成静态核对，真实 v2 Agent 录制、回放与跨云比较均 `UNRUN`；预期 release10 的二进制及 pins 仍待验证。原生任务及 [metadata-portable-v1](../metadata-portable-v1/README.md) 的成绩、失败和 Tape 全部保留。

独立身份为 `custom-memory-heap-crash-terminal-result-v2`，仍选择同名原题。Terminal-Bench 2.1 固定提交 `7131e4375048a0e408a8fb404b5f499d726b695b`（Apache-2.0）的[原题](../../public/instruction.md)、[task.toml](../../task.toml)、环境源码及私有测试不变；这是 ARIES 公开交互变体，不是未修改的官方成绩。

- [instruction-appendix.md](instruction-appendix.md)：与 Text/SQLite v2 完全相同的 1902 字节公开附录，SHA `0a7859d75d23a4497ce17a894c2b7b1fe5a3fff968cb1db931c0ea6cdf619c05`。只指导 names-only 发现、相关属性读取及有限后台工作的显式启动/真实终态等待，没有调试步骤、补丁或参考答案。
- [effective-instruction.txt](effective-instruction.txt)：上游去首尾空白的原指令＋两个换行＋附录原字节。
- [prompt.diff](prompt.diff)：原题至新公开文本差异，原题内容未改。
- [variant.json](variant.json)：来源、SHA、原始预算及精简归因证据，属于控制端元数据，不能整体分发给 Agent。

依据已有 record-2 的真实 Tape：未启用新协议的 exec 自动 yield 后返回原生随机会话句柄与 PID；随后同一会话出现 poll、log、kill，poll/log 还输出闲置秒数。因此仅稳定初始根目录时间不足以处理这一类模型可见差异。v2 开启既有 `terminal-result-v1` 协议，复用已发布 e040 镜像；工作负载名称 v2 不代表新增 native 协议。

[公开 record profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-huawei-a-custom-memory-heap-crash-terminal-result-v2-record.json) 保留原 `task_id`、1800 秒 Agent＋1800 秒 Verifier、4200 秒 Sandbox 窗口，使用既有华为固定根 mtime binding。原六项测试仍由停止和 Bridge 撤权后才注入的私有 Verifier 评分；评分中的五次编译不算 Agent 已执行的工作。原 E2B 项目一小时限制仍是独立未解决项，华为通过不能替代 E2B 通过。

旧录制中的 poll/log/kill 可能是必要故障诊断，新指导只避开无关动态状态，不禁止这些操作或削弱自主解题能力。原生 poll/log/kill/PTY、真正失败和取消均保留。有限工作指导省略可选单次 wait 超时，但原任务截止时间仍适用；真正卡住的工作不能伪装完成。本变体不保证 Agent 必然遵循指导或永不 timeout，也不保证所有未来输出相同，必须重新真实录制，并以原评分、完整 Tape、真实工具和完整 Strict 回放验收，不改旧 Tape 或匹配规则。

本次仅构造材料与运行输入，没有执行 Agent、模型、云任务或本地测试；不修改原题、私测、共享索引或产品源码。
