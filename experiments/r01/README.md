# R01 原生工具观测实验

这些补丁针对固定 OpenClaw 编译产物；所有构建、原生探针与镜像发布均在授权 ECS 执行。`native_patch.py` 校验原件 SHA 和唯一替换位置，不修改原镜像。完整 Agent、跨云与覆盖率结论需各自的真实运行证据，不能由局部探针推定。

## 脚本预检拒绝收尾（2026-10-26）

DB Strict record-2 的 ToolCall `call_00_tysNw8rvzsuuudhI6bBz4835` 在原生 `validateScriptFileForShellBleed` 拒绝复杂解释器命令，只有 invocation/started，没有 pre_dispatch/rejected；尚未进入 `runExecProcess` 或 Bridge。原错误和 ToolResult 已存在，缺口仅在观测事件。该录制另有上游模型流错误和业务失败，此修复不把它们改写为成功。

`native_patch.py:patch_script_pre_dispatch_rejection` 使用一个局部标志覆盖整个 validator 的 `await`。成功返回后、进入 `runExecProcess` 前清除标志；既有 catch 仅在标志仍为 true 时调用既有 `recordToolRejection(..., "shell_preflight_rejected")`，随后保留原清理和 `throw error`。不按错误文本分类，不生成 Bridge 身份，不将后续原生执行错误归为未派发。

候选基于 `e040654efde8adde910583291b547fb9d5fc94e565b2f4dc8bad29abcc423041`，只 COPY `bash-tools-D7H1rgMh.js`，实际编译模块净增 8 行。工具 schema、renderer、权限判定、Runtime、Bridge、后台/PTY 和 terminal-result-v1 模块保持原实现。

| ECS 定向检查 | 原 e040 | 新候选 |
| --- | --- | --- |
| 复杂解释器、脚本 shell 变量、JS 开头 NODE 三类拒绝 | 6 次 invocation，0 条拒绝，复现缺口 | 6 次 invocation，3 条拒绝，共 9 条记录 |
| 相同三类拒绝，terminal-result-v1 开启 | 本轮未单独运行 | 同上；3 个原异常对象和消息均保留 |
| validator 成功后的 backend 异常 | 原异常对象保留，无未派发记录 | 原异常对象保留，无未派发记录 |
| 正常前台、后台与 PTY | 通过 | 默认及 terminal-result-v1 均通过 |
| 既有 SSH parser/elevated/host 拒绝探针 | 6 invocation / 5 拒绝 / 11 记录 | 同分母、原错误相同，无重复拒绝 |

第一轮全部通过，没有失败重跑。探针使用隔离的真实原生模块与本地子进程，network=none，无模型、云 Sandbox 或 SSH/Bridge 执行；共享 ECS 的结果仅作功能验收。已有 16-job 协议矩阵沿用未变化模块的历史证据，本轮未重跑。其他未覆盖的 pre-dispatch 路径仍为 unknown，整 Agent 使用新镜像的运行仍待独立验收。

独立发布状态为 `PASS_PUBLISHED_CANDIDATE_ONLY`，完整引用：

```text
swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-openclaw:2026.7.1-terminal-result-v1-preflight-39964c39357e@sha256:39964c39357eee32a1c5fff7c5562aebfca2495fcdeb7a7871dd06fed73134ea
```

发布复用现有 SWR 认证流程，核验 registry manifest 原字节摘要与读回镜像身份；认证临时目录已删除，既有服务身份未变，cleanup_errors=[]。候选全部临时容器均确认 absent。旧 e040 与默认配置保持不变，新录制必须显式选择新 digest。

| ECS 证据 | SHA-256 |
| --- | --- |
| `/opt/aries-artifacts/r01-script-preflight-rejection-1/evidence.json` | `b7e4e0aeffc0ccc559bb955b39d7ca4bf37dabbcc5dc8bc6eb50d43f1b4c22ee` |
| 同目录 `candidate.patch` | `5dd58ad8521abc4f464cd9aa5a470b643e2fd849f1a31fc2abbebdc07c15a98e` |
| 同目录 `image/bash-tools-D7H1rgMh.js` | `15be27c4bcc3db087d991ff0e593bc215aa7c5b03f65f117bd06e5488967bbce` |
| `/opt/aries-artifacts/r01-script-preflight-publish-1/evidence.json` | `b6551b9959eaf494c9c77e6a100505cfca02945a137068b6a2e323f467512951` |
| 同目录 `source-manifest.json` | `59dba0700e63265787d47b194cbf3708532cee32ee5ada91be4e1407bf7d9d4f` |

发布目录 `recipe/` 保存补丁源、原样既有探针、新 `native_script_preflight_probe.mjs`、构建脚本、Dockerfile、唯一变更模块、实际 diff 和五份原生检查 JSON。定向检查入口为 `/root/aries-setup/r01-script-preflight-rejection-1/check.py`；输出目录不可覆盖。旧错误与成功探针完整结果均保留在原件目录，小回执位于仓库忽略目录 `.cache/r01-script-preflight-rejection-1/`。
