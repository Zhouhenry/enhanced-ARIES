# 大文本与 SQLite：华为任务环境准备归档（2026-10-26）

**两个 Huawei 模板均已 ready，可用于原题或获批的独立指令变体实测。** 本轮完成 ECS 镜像构建、SWR 不可变引用发布、原文件核验、公开依赖断网 smoke 和模板资源回读。没有创建云 Sandbox、没有调用模型、没有执行 Agent 或原私有 Verifier；因此本页没有新增 Agent 成功率或 Sandbox 性能结果。

| 原题 | Agent / Verifier 原预算 | 模板配置与实际回读 | 模板准备状态 |
| --- | --- | --- | --- |
| large-scale-text-editing | 1200 / 1200 秒 | 1 CPU / 2048 MiB；build.status=ready | READY；真实任务 UNRUN |
| sqlite-with-gcov | 900 / 900 秒 | 1 CPU / 2048 MiB；build.status=ready | READY；真实任务 UNRUN |

任务绑定仍使用原 `task_id`，变体通过已测试的 `benchmark.variant` 入口追加公开指令，不改原 task.toml、预算或私有评分。工作目录 `/app`；SQLite 继承原 E2B 声明的通用编译工具，准备时不编译 SQLite 或预制答案。私有测试、参考解和模型密钥不进入任务镜像。

## 实际准备耗时与失败

以下为准备工作，不计入任务 Runtime 或任务 Sandbox 创建耗时。镜像行表示各回执 started_at→finished_at 的墙钟窗口，包含构建、smoke、推送等步骤，未拆出未采集的细项；模板行使用原 build_duration_ns。此时 ECS 可能有其他主线任务，数值不能解释为独占环境性能。

| 准备步骤 | 实际耗时（秒） | 结果 |
| --- | --- | --- |
| 大文本 image-1 | 38.956 | FAIL：通用缓存未通过本镜像的离线 uvx 公开 smoke；未推送、未建模板 |
| 大文本 image-2 | 104.218 | 构建及公开离线 smoke 通过；FAIL：SWR 层提交等待响应头超时 |
| 大文本 image-3 发布回执 | 3.255 | PASS：复用 image-2 同一 Docker image 重发 SWR，没有再次构建 |
| 大文本 template-1 | 26.873 | PASS：ready、1 CPU / 2048 MiB 回读一致 |
| SQLite image-1 | 231.494 | PASS：构建、公开离线 smoke、SWR 发布 |
| SQLite template-1 | 33.517 | PASS：ready、1 CPU / 2048 MiB 回读一致 |

大文本首轮不是 uv 安装失败：官方本地资产安装成功，uvx 尝试访问 PyPI 获取 pytest 时，断网验证按预期拒绝。镜像内实际 uv 选择 managed Python 3.13.9，不能把失败草率归为系统 Python 版本差异。修复是在每个具体镜像构建阶段，以原测试使用的精确 uvx / pytest 版本参数预热环境，再用相同参数进行 network-none 公开 smoke；未改原 test.sh、Python 选择规则或 Agent PATH。旧通用缓存为何未命中该镜像的具体缓存键尚未独立追溯，本页不捏造原因。

公开依赖缓存包含官方 uv 0.9.5、root 私有 Python / pytest 环境；绑定新增 `UV_DOWNLOAD_URL=file:///opt/aries-verifier-deps/uv/0.9.5`。断网 smoke 为公开算术测试 1/1，只证明安装/执行依赖路径可用，不替代原大文本 5 项或 SQLite 原私有测试。原测试仍可能包含其他网络准备，不能把此 smoke 说成完整 Verifier 禁网通过。

## 初始任务文件核验

- SQLite 原 E2B fixture spec 有 1 个条目，派生镜像沿用同 SHA spec/check 并实际通过检查。原源码压缩包和原测试不改。
- 大文本原 E2B fixture spec 为空，旧“fixture smoke PASS”仅说明准备环境检查通过，不能宣称 CSV 逐文件已校验。为补齐这一缺口，另在两个 network-none Docker 容器中对原固定 source image 与成功 Huawei image 的初始 CSV 逐文件计算 SHA；两文件完全一致。没有重写旧 spec，也没有运行解法。

| 大文本初始文件 | 原镜像与派生镜像的共同 SHA-256 |
| --- | --- |
| `/app/expected.csv` | `833cb9527dad306efe9f8f870497f6f519e3978e9c27f093bc56eb2935f41f97` |
| `/app/input.csv` | `5b4a07a1568b7b11f4602729cea8a8db0aba7093e6a450bbbeb08c128f0628c0` |

大文本文件核验原件：ECS `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-fixture-compare-1/evidence.json`，SHA `cf9e7a7f92c67a6827a43a847696ac26eca6728f550059c841d63cdd8dfdbce6`。两个容器均已清理。源 Dockerfile 原本就包含的 expected.csv 保持原状，独立 Verifier 仍按原逻辑重新生成并校验；没有增加参考答案。

## 固定镜像与绑定

原基准固定为 Terminal-Bench commit `7131e4375048a0e408a8fb404b5f499d726b695b`。同 CPU / 内存配置可以开展后续云间比较；底层磁盘差异单独记录，不作为阻断条件。模板配置回读不等于证明所有运行时强制配额一致。

| 原题 | 原 source image digest | Huawei 发布 digest |
| --- | --- | --- |
| large-scale-text-editing | `719adca3f1388220546ce6a155eee56eff3c4fe318183100320606a210f6b59c` | `7991a296d4b081c31ae83b0e257a6145921d031895c1996d7fb1302f3e654e89` |
| sqlite-with-gcov | `f986820b00c74ce75db7288719d364df313fde7c7917bea6eb6352a08431a89f` | `dbb6a43f4813658ffefa0c4017d2b0f25f834800ed9bfd8eb0a32a3d61896646` |

完整不可变引用：

- `swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-tb-large-scale-text-editing:20261026-2@sha256:7991a296d4b081c31ae83b0e257a6145921d031895c1996d7fb1302f3e654e89`
- `swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-tb-sqlite-with-gcov:20261026-1@sha256:dbb6a43f4813658ffefa0c4017d2b0f25f834800ed9bfd8eb0a32a3d61896646`

| 原题 | ECS binding 路径 | binding SHA-256 |
| --- | --- | --- |
| large-scale-text-editing | `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-template-1/binding.json` | `28833e4edd1161150aab0c3ee1a06683a4f43ac2d1b0633dc57fbc133be5cd0a` |
| sqlite-with-gcov | `/opt/aries-artifacts/tb-huawei-sqlite-with-gcov-template-1/binding.json` | `cdbd3517f68179fcae942072aa5d625748b2b9c69f2e3b0155c05a06af4caa42` |

## 已准备的 Huawei 变体运行输入

从 release7 冻结的 E2B profile 复制并校验 source manifest，仅改变 provider、token_env、binding_file 与附录绝对路径四项。原 variant ID、task ID、预算、terminal-result-v1 与私有评分不变，没有启用 assume_network_isolation。

这些是现有实验 driver 的输入；driver 在实际录制 / 回放时固定版本、路径、输出目录与真实 Tape。当前未选择或伪造新 Tape，也未因此启动 Agent。

| 原题 | ECS 输入 profile | 新 profile SHA-256 |
| --- | --- | --- |
| large-scale-text-editing | `/opt/aries-artifacts/tb-repro-agent-inputs-1/huawei-large-scale-text-editing-terminal-result-v1.json` | `0a51cb26ce4514e5391108463a82b7dba3973a23d096d99ca4ad665eaa213c00` |
| sqlite-with-gcov | `/opt/aries-artifacts/tb-repro-agent-inputs-1/huawei-sqlite-with-gcov-terminal-result-v1.json` | `bbd628bf196d5c1644bf67d00a4d3d68348a88fbb3a4d696da89b9cfce45786b` |

原 / 新 profile SHA 与完整四字段差异保存在 `/opt/aries-artifacts/tb-repro-agent-inputs-1/huawei-terminal-result-profile-evidence.json`，SHA `829b6158d1be12aace6b509f4c05414f9f38c1c5dcd7a58b628dff37b517cb30`。

## 保存与清理

未生成新的 docker save 大包。成功 Docker image、SWR digest、准备脚本、原文件哈希、平台请求/回执及小型结果保留；公开依赖归档用 hardlink 共享，不产生多份物理副本。

大文本 image-1 在确认无容器引用、从未发布且成功模板使用另一 image 后，删除其独占旧 tag 与两个失败上下文中的共享缓存引用。失败日志、配方和 SHA 保留；image-2 虽有发布失败记录，但其字节正是 image-3 发布成功的镜像，继续保留。没有全局 image / volume prune，也没有改 VPC、网关或模型入口。

| 证据 | ECS 路径 / 文件 | SHA-256 |
| --- | --- | --- |
| large-scale-text-editing 镜像 | `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-image-3/evidence.json` | `e59480b662bd6bb8cadbb21c92ad3e27cc8c96f9b25b2f2f4a31d8f0ab34a282` |
| large-scale-text-editing 模板 | `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-template-1/evidence.json` | `5acfe19b4992a5245a00de78d0b55bd644ef61b9571d716713ed0f3e1e8e1d65` |
| sqlite-with-gcov 镜像 | `/opt/aries-artifacts/tb-huawei-sqlite-with-gcov-image-1/evidence.json` | `0bc4614c7ab0002da17bbcc350897725648137fb19382935ea825790b40047b9` |
| sqlite-with-gcov 模板 | `/opt/aries-artifacts/tb-huawei-sqlite-with-gcov-template-1/evidence.json` | `f1394cc77534021f2b91843ef20b8942650f979b627f4399eea1f8d623691ae6` |
| 大文本原失败 | `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-image-1/evidence.json` | `865dc7d1e9e4880523d423a92fc1f0ab880ad708082f86b6980d69782df0ca4e` |
| 大文本原失败 | `/opt/aries-artifacts/tb-huawei-large-scale-text-editing-image-2/evidence.json` | `68d4152d59d728b348c1cbd3c49d4ac0521e3bcdcdf5e3c1305652558a37666a` |
| 两题收口与失败候选清理 | `/opt/aries-artifacts/tb-huawei-variants-preparation-final-1/evidence.json` | `2372d2309a5b1b1739582a24e897e1851dbd0c5ec18cdf20dcb3c8658e404bdf` |

本地对应小型副本为 `.cache/tb-huawei-variants-2/final-evidence.json`、`large-fixture-evidence.json`；准备1/2与重新发布3的失败/成功回执分别保存，不用最终成功覆盖历史失败。后续真实变体录制、同 Tape Strict Replay、原私有评分、停止撤权和云资源清理必须按自己的运行证据验收。

公开小回执已通过 SCP 取回 `.cache/tb-huawei-variants-2/public-receipts/`：13 个 JSON，共享归档仅 10,695 字节，SHA `02e4eddc02bdc77a935c102fafd3374e2b073ec21257d50391c6ae5799a2aabd`，包含绑定、构建回读、镜像结果、CSV 对照、失败清理及运行输入，不含凭据、镜像层或工具正文。
