# custom-memory-heap-crash：metadata-portable-v1 独立变体

状态：**材料已构造并静态核对；未纳入当前冻结 release7；本变体真实录制、独立评分、Strict Replay 与跨云对照均为 UNRUN。** 本次没有构建、测试或云调用，也不为这些声明材料触发新的 Go 发布。

已有原题 Huawei record-2 通过原评分，随后同模板 replay-1 在第二个模型请求被 Strict 拒绝：首工具 `ls -la /app` 展示的 `..`（根目录 `/`）mtime 为 03:55 与 04:10，其余工具正文一致。原始成绩、Tape 和失败不改写，详情见[原 C++ 性能归档](../../../../../../docs/tb-huawei-cpp-performance-20261026.md)。这只定位了已执行前缀的差异，未证明后续 C++ 输出都确定。

此变体复用已经批准的 DB 元数据附录原字节，只建议读取任务需要的名称、类型、权限、大小或内容，避免无关时间和进程标识；禁止伪造元数据、改时钟、隐藏错误或回填录制输出。它不提供解题方法、答案、编译步骤或私有测试，不强制后台启动/wait，不改 Native ToolResult。Agent 仍自主调查、修改和调用真实工具。

- 原题选择仍为 `custom-memory-heap-crash`；Attempt 与统计身份为 `custom-memory-heap-crash-metadata-portable-v1`。
- 原题仅允许修改 `/app/user.cpp` 的要求不变；10 个原题/环境/私测文件逐 SHA-256、官方 Git blob 与字节数核对，原有其余 36 个来源文件也未改变。
- 保留原 6 个测试函数，Agent / Verifier 各 1800 秒、1 CPU / 2048 MB，以及现有原题环境绑定。评分中的编译和 Valgrind 成本归 Verifier，不能算作 Agent 工作量。
- 附录接线复用 `benchmark.variant`：加载后的原指令 + 两个 LF + 附录原字节。SHA、来源、限制见 [variant.json](variant.json)，文字差异见 [prompt.diff](prompt.diff)。
- 私有测试只在控制端保存，确认停止和 Bridge 撤权后才评分；不进入公开附录、Agent 初始工作区或参考轨迹。

[Huawei 首次录制 profile](../../../../../../profiles/cloud/terminalbench/variants/openclaw-huawei-a-custom-memory-heap-crash-metadata-portable-v1-record.json) 继承原题配置，保留 4200 秒 Sandbox 窗口和真实 task.toml 预算，无隔离假设和后台协议。它仍需在后续明确的材料冻结与运行步骤中使用；当前 profile 文件存在不代表已纳入 release7 或已经运行。E2B 原项目的一小时时限问题单列，不缩短原题预算，也不将 Huawei 结果计作 E2B 成绩。

验收须重新真实录制，由原独立 Verifier 通过，再用同一新 Tape 严格回放、执行真实 Tool 并确认所属资源删除。CPU / 内存一致是跨云资源比较条件，磁盘等差异如实报告。编译日志、程序自己输出的 PID/时间、并行次序及其他动态内容仍可能不一致；附录是交互建议，不能保证所有 C++ 轨迹可回放，也不支持放宽 matcher、改写旧 Tape 或删除失败样本。
