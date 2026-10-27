# 大文本任务跨云 Dashboard

Git 仓库仅包含本 Dashboard 的构建和展示源码。运行数据 `data.json`、
`case-actions.json`、`trace_views.json` 和生成的 `index.html` 留在本地实验归档，
不随源码推送；取得对应脱敏实验数据后，在 ECS 构建即可生成下述离线页面。

打开 [index.html](index.html)。页面已内嵌全部脱敏数据，可离线使用，不需要域名、Web 服务、CDN 或模型凭据。

本页仅展示 `large-scale-text-editing-terminal-result-v2` 的三次同 Tape Strict 回放：E2B 两次、华为一次，均通过原独立评分 5/5。不同任务与历史失败不混入这三次样本；这是阶段性实测，不能据此给出云厂商排名或完整生产资格。

- 测试说明列出百万行 CSV 的任务目标、Vim 约束和原独立评分维度。
- 每次 exec 标注真实动作、目的及结果；保留内部失败与外层退出码的区别。
- 总览默认显示 Attempt 执行总时间（不含排队），可切换其中的 Runtime 与独立验证；功能成功与观测覆盖状态分开说明。
- 八阶段明确排队在 Attempt 外，其余七个阶段之和加未分段差额等于本页 Attempt。条形只比较同一阶段，不伪装连续时间线。
- Runtime 下钻到模型、exec 和后台 job/wait；模型与工具可能重叠，不能相加或相减后称为精确 Agent Loop 开销。
- 模型区分 23 次请求累计与首 SSE/content/tool delta 的样本均值。三个相邻 HTTP 子窗口的累计等式经过原边界核对。
- 逐工具切换 guest wrapper、控制端提交到 Wait 返回窗口、首输出/退出相对时点；可筛选、排序和查看原身份。EOF 在先与无非空输出各自说明。
- 点击带点状下划线的时间或“计时口径”，查看中文起止、包含项、父级、重叠/相加关系及原始字段。关键关系同时默认可见。
- 资源与 Trace 后置，CPU 秒、CPU%、内存采样最大值分别解释；未知时钟不构造全局时间线，华为CPU冲突读数明确标注。
- 证据区列出来源与 SHA；“下载数据 JSON”导出页面实际使用的数据。

[data.json](data.json) 是脱敏数据源；[SOURCES.md](SOURCES.md) 定义来源和计时口径；[完整实验归档](../../repro-variants-performance-20261026.md#text-v2-huawei-replay-1)保留详细说明。HTML 只展示这些数据，不重新匹配原始事件或修改测量结果。

页面提供六个原始 Chrome Trace 视图：三次运行各有主时钟域与后台工具时钟域。离线 Operation 泳道可搜索、缩放和查看身份；下载原字节前校验 SHA。通过 localhost/HTTPS 打开页面后，点击“用 Perfetto 打开”会按需加载官方原生查看器；file:// 下下载后在官网导入。未对齐时钟域始终分开。

最新计时层次版由 ECS `/opt/aries-artifacts/text-dashboard-time-hierarchy-1` 构建，桌面、手机、34 类口径弹窗、30 组资源组合与六份 Trace 离线交互均已验证，三次运行数据 SHA 不变。原生 Perfetto 传送逻辑未改，沿用此前实际 UI 导入证据；详见 [SOURCES.md](SOURCES.md#计时层次与统计关系修订)。

构建和浏览器验证均在授权 ECS 完成。源码为 `template.html`、`time-help.html`、Trace/资源片段与标准库 `build.py`；在对应 ECS 目录运行 `python3 build.py` 生成单文件页面。更新数据后应重新构建、检查桌面与窄屏布局，并保留输入 / 输出 SHA 和验证回执。

资源从同一冻结Snapshot提取：在ECS运行 `python3 extract_resources.py --data data.json --output 新路径.json` 可独立复核。该步骤不运行Agent；I/O和网络字节当前接口不支持，页面会保留状态。资源补齐前的原页面与提取失败均在ECS历史目录保留。
