# Task 123 verification

- 修复提交：`3c51ca9603a641389363bd944bb68f7648b43d76`；独立失败测试：`1a94ca4`。
- 方案：[[archive/plans/123-runtime-output-idempotence/overview]]；方案评审：[[archive/plans/123-runtime-output-idempotence/plan-review]]；实现评审：[[archive/plans/123-runtime-output-idempotence/quality-review]]。
- 2026-10-15 在 macOS arm64 的真实 `noodle_playground` 运行状态验证；安装版本 `v0.1.5+local.3c51ca9`。

## Regression evidence

- 修复前：真实 fsnotify + runOnce 回归在 failed-only、review-waiting、active-without-output 状态快速超过 8 cycles；idle/paused 不触发此失败。
- 修复前：orders/state/pending-review/seq 的文件身份或固定旧 mtime 稳定性断言全部失败；真实 control/orders-next 的唤醒和应用正常。
- 修复后：相同回归通过；实际 `Loop.Run` 在不写输入文件的情况下处理内存 completion/merge results 并使订单完成。
- 修复后：输出删除/损坏恢复、非零 seq 推进、命令重放、flush barrier 错误路径和 orders-next crash safety 均由相关包测试覆盖。

## Checks

| 检查 | 结果 |
| --- | --- |
| `go test -race ./internal/filex ./internal/projection ./loop` | PASS |
| `pnpm check` | PASS：build、全部 Go tests、vet、arch lint、loop fixtures、fixture hashes；arch lint 保留 5 条现有体积警告 |
| `pnpm --filter noodle-ui test` | PASS：13 files / 78 tests |
| `go test -tags e2e -timeout 120s -count=1 ./e2e/ -run '^TestSmokeStartOnce'` | PASS：无需真实模型的 smoke 子集 |
| `pnpm test:smoke` 全量 | 未通过：沙箱禁止本地端口、模型 CLI 初始化失败；未扩大权限重跑真实模型调用，不宣称完整端到端模型验证通过 |

首次全套检查因 localhost 监听被沙箱拒绝失败；获本地测试权限后重跑 `pnpm check` 通过。工具链使用 Node 24、pnpm 10.30.3、Git 2.53；Go cache 放在临时可写目录。

## Live CPU and write comparison

CPU 采用同一 PID 的 `ps -o time=` 累计 CPU 时间差除以 monotonic 墙钟窗口，100% 表示一个逻辑核。文件每 10ms 轮询 inode/mtime/size，计数可能低估快速写入。不是历史平均 `%CPU`，也未混入 adapter/provider 子进程或浏览器 CPU。

| 指标 | 修复前 | 修复后 |
| --- | ---: | ---: |
| PID | 82975 | 53523 |
| 测量窗口 | 15.001 s | 15.005 s |
| CPU 时间增量 | 7.68 s | 1.34 s |
| 平均 CPU | 51.2% | 8.93% |
| orders/state/pending-review/seq 每份文件变化次数 | 97 | 0 |
| control 文件变化次数 | 0 | 0 |
| tickets 文件变化次数（后续任务） | 97 | 14 |

- CPU 降幅约 82.6%；目标四份文件窗口前后字节相同且没有替换/修改。
- 两次均为 `running` / `supervised` / max_concurrency=4；不是暂停态。重启前后 orders JSON 完全一致，仍只有 failed schedule；配置 SHA-256 与 58-session 数量一致，没有新增 session。
- `lsof` 确认新 PID 执行 `/opt/homebrew/libexec/noodle-local/noodle`；`/opt/homebrew/bin/noodle` 仍链接到该本地目标。主 checkout 已包含修复。
- 最终发布构建在合并后执行 `pnpm build`，注入 canonicalVersion；`go version -m` 确认 revision=`3c51ca9…`、vcs.modified=false。备份 `/tmp/noodle-123/noodle-before-task123`；测试输出与采样 JSON 位于 `/tmp/noodle-123/`（临时证据，关键数值已持久化本文）。
- 残余约 9% CPU 与每秒维护路径仍存在；本任务未改历史 metadata、tickets、adapter 或 WebSocket 构建，保留 124–128 等后续工作。结果只证明当前同等工作负载，不承诺所有负载的 CPU 上限。
