# Jiuwen PPT · 两云六组实际运行看板

直接打开 `index.html`。完整 ZIP 自包含字体、六组 PPT、脱敏数据、小回执与华为去等待组的完整 Trace 压缩原件。使用 localhost / HTTPS 时可点击导入 Perfetto；file:// 可下载 gzip 后解压手动导入。

本页首先用共同组号展示两张矩阵：完整任务八阶段 / 未归属差额 / Attempt，以及 Sandbox Worker 服务处理 / 直接子进程。两张表按三类模型方式成对排列：实时 DeepSeek、回放去掉模型计划等待、回放保留录制模型节奏。两云实时模型决策不同，只作端到端参考；回放只在同 Tape、同时序内作本次对照。

随后选择任一组查看该次阶段、模型 HTTP、原生工具、命令、资源、PPT 和来源摘要。Worker 的时长来自已校验 Snapshot 中同 invocation 的起止区间，包含 native handler 和结果封装发送，不叫纯 CPU 或纯工具计算。cmd 是其中直接子进程的区间。原生 Tool 在 ECS Agent 侧包含转发。所有累计不可互加；没有跨主机时钟相减或补造 Agent Loop。

六组数据都从各自原 summary 和对应 SHA 的 Snapshot 汇总。CPU / 内存均 4 vCPU / 8 GiB；磁盘等平台差异随样本说明。ECS Agent、共享 Controller 和平台资源分别列示；CPU 秒不等于墙钟时间，平台内存口径未经验证不用于直接比较内存效率。每组一次，历史失败保留在性能归档。

独立 Verifier 检查至少一份 PPTX 恰好 5 页且每页正文非空，不评价视觉质量和事实准确性。中间产物保留原状态；每组选用该次真实 final.pptx，不复用其他组文件。

末尾完整 Trace、Perfetto / Jaeger / Prometheus 后验 PASS 固定属于第 03 组华为去等待回放。它不背书其他五组。各域未对齐，不拼成虚假绝对时间线。其余组离线投影和后端状态见各自原回执。

`build.py` / `groups.py` 仅在 ECS 读取已有归档，不运行 Agent / Sandbox / 模型。`verify.cjs` 在 ECS localhost 检查六组切换、各组数据与资源/PPT、矩阵守恒、实际 grep 命令、缺失计时和 Trace 导入。`build-receipt.json`、`browser-validation.json`、`delivery-manifest.json` 记录源 SHA、验收和精确交付字节。
