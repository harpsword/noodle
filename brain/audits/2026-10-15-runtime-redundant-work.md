# Noodle 运行时冗余操作审计（2026-10-15）

## 后续修复状态

2026-10-15：123（R1/R4）已完成，提交 `3c51ca9`。相同 failed-only 现场 CPU 51.2% → 8.93%；orders/state/pending-review/seq 每份文件 15 秒内的重复写入 97 → 0。方案、独立评审与验证已归档：[[archive/plans/123-runtime-output-idempotence/overview]]。下文保留原始调查基线；其余待办仍未实现。

调查基线为 `c52a319fa734fdf70e3baa7a7845962272a3b487`。事实来源为本地生产源码、当前进程只读观察及临时隔离实验。各项独立 CPU 占比未测量；下文明确区分实测、静态可达和条件性风险。

## 结论与范围

当前高 CPU 的首要来源是 **后端把自己的输出当成输入事件**：正常走到 cycle 尾部时，无条件持久化 projection；`orders.json` 的替换唤醒同一轮处理。历史会话全量监控、tickets 重算、backlog 脚本、重复 projection 和 WebSocket 快照构建把一次多余唤醒放大成一整套工作。

不能把所有链路都描述成独立死循环。R1 是自触发源；R2–R6/D1–D2 多为放大器；R7 会产生昂贵的多余模型调度；D3–D8 多数只在活跃输出、打开页面或生命周期变更时发生。正常完成且无订单的 idle、暂停状态会提前返回，不能从暂停测试推出 running/失败订单场景已经修好。

本次只调查、做临时实验和记录结果；没有再次修改生产源码、PATH 二进制或运行中的 `.noodle`。实验替代写入仅通过 Go overlay 作用于临时测试，不是已交付的修复。

## 实测证据

### 现场：修复版仍有相同内容的写入

- 已运行本地版本 `v0.1.5+local.c52a319`，PID 82975，实际可执行文件 `/opt/homebrew/libexec/noodle-local/noodle`。此前 10 秒窗口平均 CPU 50.3%；状态为 `running`，只有 failed schedule 订单。
- 本次 5 秒窗口：`orders.json`、`state.json`、`pending-review.json`、`tickets.json`、`last-applied-seq` **各观察到 33 次文件替换/修改，窗口前后字节完全相同**。
- `control.ndjson`、`status.json`、`mise.json`、`state.snapshot.json` 均未变化；控制文件修复确实生效。不能把 `state.snapshot.json` 与每轮改写的 `state.json` 混为一谈。
- 58 份 session meta 共观察到 1,971 次改写；抽样旧会话只有 `updated_at` 和 `idle_seconds` 变化。元数据计数是轮询观测值，不是完整 syscall 跟踪，可能漏掉采样间隔内的多次变化。
- 历史数据：58 份 canonical 日志共 253,484 bytes / 867 行；58 份 domain events 日志共 655,158 bytes / 925 行。当前规模不到 1 MB，已有显著工作放大。

### 单变量实验：只消除相同 orders 写入

临时 Go 测试使用真实 `Loop.runOnce`、真实 fsnotify 转发和默认一秒 ticker；以 fake Mise/Monitor/runtime 去掉模型、监控、adapter 的额外成本。每种状态运行 1.2 秒，先主动触发一轮，统计实际 cycle 和 brief build 数量：

| 状态 | 原代码 cycle 数 | 仅跳过内容相同的 orders 写入 |
| --- | ---: | ---: |
| 无订单 idle | 2 | 2 |
| paused | 2 | 2 |
| failed schedule，无可派发任务 | 1,209 | 3 |

3 轮包含首次 projection 变化引起的一次合法额外唤醒。这个实验确认因果关系，不代表真实 58-session 环境能跑到 1,000 轮/秒，也不是完整修复后的 CPU 基准。

命令与临时证据：`go test -overlay=/tmp/noodle-runtime-audit/baseline.json ./loop -run '^TestAuditQuiescentCycleCount$' -count=1 -v`；对照使用 `skip-orders.json`。日志分别在 `/tmp/noodle-runtime-audit/baseline.log`、`skip-orders.log`。overlay 的唯一生产逻辑差异是在 orders 输出处先比较现有字节，相同则不写；没有关闭监控之外的真实业务功能，也没有安装该实验版本。

### Backlog 子进程成本

在 `noodle_playground` 原目录只读执行现有 `adapters/backlog-sync` 五次：均值 wall 112.5 ms，整个子进程树 CPU 113.7 ms/次。这是独立于 `noodle` PID 的 CPU；不能与 50.3% 直接相加当作同一项测量，也不能用本机数值预测其他 adapter。源码保证每次 Build 都同步执行脚本，重复循环会重复付出这笔成本。

## 上游总览

| 项目 | 判定与触发 | 主要多余工作 | 优先级/成本 |
| --- | --- | --- | --- |
| R1 orders 输出反馈 | 现场 + 单变量实验确认 | 无变化也驱动下一次完整 cycle | 最高；改动小至中，须验证合法唤醒 |
| R2 历史 session 监控 | 现场 + 源码确认 | 每轮重读全部 canonical、元数据，重写全部 meta | 高；增量缓存与失效规则成本中 |
| R3 tickets 全量重建 | 现场 + 源码确认 | 再遍历全部 events，过滤/排序/重建/写相同结果 | 高；增量 reducer 成本中 |
| R4 其他固定状态输出 | 现场确认 | pending-review/seq/state 无变化也原子替换 | 高；内容/语义变化门控成本小 |
| R5 Mise/adapter 不分触发原因 | 源码 + 脚本测量确认 | idle、paused、文件噪声均全量同步 backlog | 高；独立刷新周期/失效策略成本中 |
| R6 重复 projection/发布 | 源码确认 | 正常 cycle 至少三次 Project，两次 publish；idle 状态往返 | 中；合并重复工作成本小 |
| R7 调度器反复自唤醒 | 已有现场记录 + 本轮源码复核 | 调度器自身历史变化使空结果冷却失效 | 高但独立；需设计触发语义，不能混作 CPU 热点 |
| R8 重复 degraded 事件 | 条件性源码确认，本次现场未触发 | 相同 warning 每轮生成新事件，反过来改变 Mise | 中；告警状态变化去重成本小 |

## R1：projection 文件写入反过来驱动同一个 loop

`Cycle` 在未提前返回时执行 `flushState`；`writeProjectionState -> WriteProjectionFiles` 不比较内容，每次新建临时文件、写入、chmod、rename 两份文件。runtime watcher 把 `orders.json` 的任何事件送回 cycleTrigger。单元素 channel 只合并瞬时积压，不能阻止“本轮产生下一轮事件”的持续循环。证据：[loop/loop.go:549](../../loop/loop.go#L549)、[loop/state_orders.go:145](../../loop/state_orders.go#L145)、[internal/projection/projection.go:73](../../internal/projection/projection.go#L73)、[internal/filex/filex.go:35](../../internal/filex/filex.go#L35)、[loop/loop.go:421](../../loop/loop.go#L421)。

failed schedule 的特殊性是 `hasScheduleOrder` 只看是否存在，而不是是否可运行，因而不进入 empty-backlog idle 提前返回；dispatch 没有候选仍执行尾部 flush。其他保留订单/正在执行/等待审查等正常走到 flush 的状态也需覆盖，不能只针对 failed schedule 加分支。证据：[loop/loop_cycle_pipeline.go:228](../../loop/loop_cycle_pipeline.go#L228)、[loop/loop_cycle_pipeline.go:398](../../loop/loop_cycle_pipeline.go#L398)。

**建议。** 首先给相同 projection 输出做幂等写入；同时明确文件所有权。当前 `Cycle` 以 canonical state 为输入，不会每次重新读 `orders.json`；scheduler 被明确要求写 `orders-next.json`。因此“外部输入”和“后端输出”应分别处理，核对控制/调度/恢复路径后，可从唤醒集合移除 backend-only 输出。不要只把周期改成更长，或只过滤 CHMOD：原子 rename 同样会通知。也不建议直接改通用 `WriteFileAtomic` 的所有调用，避免意外改变其他持久化/文件权限语义。证据：[loop/loop.go:456](../../loop/loop.go#L456)、[loop/state_orders.go:24](../../loop/state_orders.go#L24)、[loop/schedule.go:289](../../loop/schedule.go#L289)。

## R2：每轮重扫全部历史 session，并为时钟变化落盘

生产入口 `runCycleMaintenance` 每次直接调用 `Monitor.RunOnce`。它枚举全部 session 目录，无 active/terminal/输入变化筛选；每个 session 读取 runtime、PID/heartbeat、整个 canonical 日志、先前 meta，再重算与写回 meta。每轮成本随历史会话数和历史日志总量增长。`CanonicalClaimsReader` 从头逐行解码，而不是从上一 offset 接着读。证据：[loop/loop.go:577](../../loop/loop.go#L577)、[monitor/monitor.go:50](../../monitor/monitor.go#L50)、[monitor/claims.go:46](../../monitor/claims.go#L46)、[monitor/observer.go:81](../../monitor/observer.go#L81)。

`DeriveSessionMeta` 对已结束会话也用当前时间计算 idle 秒数，且无条件更新 UpdatedAt，所以仅在写入前比较完整 JSON 不能彻底消除这类写入。现场抽样印证只有这两个字段在变化。目录替换事件又让 WebSocket 重建快照（D1）。证据：[monitor/derive.go:21](../../monitor/derive.go#L21)、[monitor/derive.go:115](../../monitor/derive.go#L115)、[monitor/monitor.go:84](../../monitor/monitor.go#L84)。

**建议。** 分离活跃会话的存活/超时检查与历史摘要刷新；按源日志变化更新摘要，稳定终态不要因观察时钟推进反复落盘。成本是需要缓存 invalidation：日志 append、截断、替换、spawn 元数据修正都要处理；活跃 PID/heartbeat TTL 不能因日志无变化而停止检查。不能直接删除历史 session，也不能假设终态文件以后绝不会再被写入。

**排除误判。** `Monitor.Run` 另有 watcher/debounce，但 `noodle start` 生产接线调用的是 `RunOnce`，没有启动 `Monitor.Run`。修改其 debounce 并不能限制当前调用频率。证据：[loop/defaults.go:86](../../loop/defaults.go#L86)、[loop/session_meta_repair.go:11](../../loop/session_meta_repair.go#L11)。

## R3：tickets 再读另一套全部历史日志

同一 Monitor pass 处理完 canonical 后，将全部 session IDs 传给 `TicketMaterializer.Materialize`。该函数从头读每个 `events.ndjson`，JSON 解码后才筛 ticket 类型，再全局排序、重建状态、无条件写 `tickets.json`。这与 R2 不是同一文件读了两次：canonical 与 domain events 是两套日志，但一次周期内两套都被扫描。证据：[monitor/monitor.go:90](../../monitor/monitor.go#L90)、[event/tickets.go:51](../../event/tickets.go#L51)、[event/reader.go:64](../../event/reader.go#L64)、[event/tickets.go:268](../../event/tickets.go#L268)。

**建议。** 先在结果未变时不写；随后按各 session 日志 offset/版本消费新增 ticket 事件并维护聚合状态。不能只读取“活跃 session”，否则旧 session 留下的 claim/release 可能丢失；启动首次重建、事件顺序、截断后的重建是必要成本。现场 `tickets.json` 字节没有变化但被重复替换，先做写入门控收益明确；增量聚合需回归测试保护跨 session 顺序。

## R4：一轮结束后重写多个并未改变的输出

`flushState` 除 projection 外总写 pending review，并在 lastAppliedSeq 非零后每轮写相同序号。即使一个命令都没收到，只要历史处理过命令，这个序号文件以后持续被替换。它们不在 runtime loop 的触发名单里，因此主要增加系统调用、磁盘操作、WebSocket 通知，而非独立启动主循环。证据：[loop/state_orders.go:145](../../loop/state_orders.go#L145)、[loop/pending_review.go:75](../../loop/pending_review.go#L75)、[loop/control.go:366](../../loop/control.go#L366)。

**建议。** 分别按业务变化或内容变化保存；保留真实命令 seq 推进时的持久化时机，避免破坏重放去重。相较引入全局缓存，针对这些稳定输出的变化门控较小且容易验证。现场五种重复写文件不包含 canonical checkpoint，不要贸然削减真正事件边界的 checkpoint。

## R5：Mise 写入虽有去重，昂贵的前置工作仍每轮执行

`buildCycleBrief` 先 refresh adopted targets，再 `Mise.Build`，之后才检查 loop 是否 paused 等非调度状态。Build 每次同步启动 `sh -c` 执行 backlog sync，读取 tickets 与 loop events，构建并序列化 brief；最终的 `lastContent` 比较只能省掉 mise.json 写入，不能省掉以上工作。证据：[loop/loop_cycle_pipeline.go:16](../../loop/loop_cycle_pipeline.go#L16)、[mise/builder.go:40](../../mise/builder.go#L40)、[mise/builder.go:109](../../mise/builder.go#L109)、[adapter/runner.go:56](../../adapter/runner.go#L56)。

当前 playground adapter 每次含多次 shell/grep/sed 管道；五次实测均值见上。源码仓库自己的配置甚至采用 `go run adapters/main.go backlog-sync`，因此 adapter 的代价不能假定为一次廉价内存读取。每秒兜底轮询、同一轮多余事件均重复付出该代价。`readRecentEvents` 全读 loop lifecycle 日志，但正常 writer 限 200 行，故不是无限历史扫描。

**建议。** backlog 刷新与 runtime 文件事件解耦；维护有界的新鲜度周期，允许明确 backlog 操作/用户请求失效。paused 时是否继续刷新 UI/backlog 是产品语义，应先确定；即使需要刷新，也不必跟随每次 meta/订单输出通知。不能只监控本地 todos.md，因为 adapter 可能对接外部服务。成本是增加刷新时机、失败重试与显式 invalidation 的清晰约定。

## R6：同轮重复构建 projection/快照，idle 每秒做状态往返

正常执行到 cycle 尾部时：`flushState -> Project` 一次，显式 `publishState -> Project` 一次，随后 `stampStatus` 无论是否写 status 都再 `publishState -> Project` 一次。Project 生成多个视图、clone/sort，并计算 hash；这些 hash 没有用于跳过 projection 文件写入。没有语义变化时仍至少三次构建，两次发布。证据：[loop/loop.go:549](../../loop/loop.go#L549)、[loop/state_snapshot.go:42](../../loop/state_snapshot.go#L42)、[loop/stamp_status.go:34](../../loop/stamp_status.go#L34)、[internal/projection/projection.go:30](../../internal/projection/projection.go#L30)。

此外空 backlog 的 idle 每秒会在 `buildCycleBrief` 先切为 running，然后在 `bootstrapScheduleIfEmpty` 切回 idle；`setState` 每次记录日志。这个往返本身不是同等级死循环，也没有证明是主要 CPU；它是稳定空闲时可以避免的状态/日志噪声。证据：[loop/loop_cycle_pipeline.go:25](../../loop/loop_cycle_pipeline.go#L25)、[loop/loop_cycle_pipeline.go:231](../../loop/loop_cycle_pipeline.go#L231)、[loop/loop.go:131](../../loop/loop.go#L131)。

**建议。** 统一一次 cycle 的最终发布位置，复用同一 canonical 版本的投影；保留提前返回和控制命令后的及时发布。不能只把 State() 的深拷贝删掉，快照跨 goroutine 共享仍需要不可变性/隔离。先消除确定重复调用，再评估更复杂的版本缓存。

## R7：scheduler 自己的变化被当作新工作输入

已有 #120 记录了模型调度器完成空订单后、执行任务仍活跃时继续每分钟运行。源码中 Mise 内容包含 active summary、history、recent events；调度器自身生命周期会改变这些字段。`prepareOrdersForCycle` 见任何 miseChanged 都清空五分钟冷却，而 `ensureScheduleIfNeeded` 还有独立的 miseChanged 调度注入路径。证据：[mise/builder.go:96](../../mise/builder.go#L96)、[loop/loop_cycle_pipeline.go:263](../../loop/loop_cycle_pipeline.go#L263)、[loop/loop_cycle_pipeline.go:214](../../loop/loop_cycle_pipeline.go#L214)、[此前问题记录](../codebase/scheduler-self-retrigger-after-empty-orders.md)。

本轮仅复核源码，没有为了研究再调用真实模型或新造 LLM 调度实验。它不是当前 failed schedule 的每秒 CPU 回路，但会浪费模型调用、worktree 和会话初始化成本。应将“展示内容变化”与“可改变调度决定的输入变化”分离，并让所有注入路径服从统一 cooldown/eligibility。比文件写去重更需要设计和测试，适合作为独立修复。

## R8：相同 degraded warning 每轮生成新业务事件

`emitSyncWarnings` 只检查是否包含 sync script missing/failed，随后每轮日志 + `sync.degraded` 追加，没有记忆上一轮已发过相同 warning。新事件又进入下一轮 Mise 的 recent events，形成“同一问题 → 新事件 → 新 brief”的二次放大，并可能触发 R7。证据：[loop/loop_cycle_pipeline.go:365](../../loop/loop_cycle_pipeline.go#L365)、[loop/util.go:122](../../loop/util.go#L122)、[mise/builder.go:165](../../mise/builder.go#L165)。

当前现场没有这个 warning，属于条件性源码确认，不能算入这次 50% 的已测占比。建议按告警出现、内容改变、恢复状态发事件，持续故障用有界提醒频率。不要把重试/错误完全吞掉。


## 下游总览

高优先级问题是「无关文件变化仍启动全量工作」与「没有消费者仍构建数据」。随后才是每事件的小额分配和文件操作。WebSocket 的 300ms 合并与 hash 去重确实存在，但分别只限制构建频率和网络发送，不能省掉此前的目录通知、状态复制、日志读取、解析和序列化。

| 链路 | 触发条件 | 多余工作 | 影响边界 | 建议优先级 |
| --- | --- | --- | --- | --- |
| runtime 文件 → WS snapshot | Web server 开启，任意子目录文件变化 | 无客户端也构建；不相关文件变化也构建 | Noodle CPU、文件读取 | 高 |
| snapshot → active session 日志 | 每次 snapshot 构建 | 从头解析所有活跃会话事件 | 随活跃日志长度、构建频率增长 | 高 |
| broker → 无订阅会话 | provider 产生事件或 delta | 格式化、JSON 编码后才发现无接收者 | Noodle CPU；只在会话产出期间 | 中 |
| 首屏/会话切换 → HTTP + WS | 首次加载、重新订阅、重连 | 双通道可能都读取同一份完整历史 | 有限次数，非空闲忙循环 | 中低 |
| UI 实时事件 → 整份历史 | 用户正在观看的会话收到事件 | 线性查重、复制、整表排序、重新分组 | 浏览器 CPU；不等于 Noodle CPU | 中低 |
| canonical → event + sink | 每个可转换的非 delta 事件 | 同一 canonical 转换两次 | Noodle CPU；每事件小成本 | 低 |
| event.Append | 每条落盘事件 | 重复 mkdir/open/close | Noodle 系统调用；每事件成本 | 低，需先量化 |
| loop event → 保留最近 200 条 | 每个生命周期事件 | 追加后全读；超过上限后每次重写 200 条 | 有界历史、事件驱动 | 低 |

## D1：WebSocket 对整棵 runtime 树监听，无消费者也重建快照

**实测补充。** 临时测试直接运行真实 `watchAndBroadcast`，固定 provider 内容与时钟、零客户端，在 runtime 目录写三次 `unrelated.txt`，观察到四次 snapshot 构建（一次初始 + 三次通知）。命令：`go test -overlay=/tmp/noodle-runtime-audit/server.json ./server -run '^TestAuditSnapshotWithoutClients$' -count=1 -v`；日志 `/tmp/noodle-runtime-audit/server.log`。测试不计单次 CPU，只确认无消费者、无相关内容变化仍执行构建。

**生产路径。** `noodle start` 默认启用服务器，`Server.Start` 无条件启动 `watchAndBroadcast`；该函数递归注册 `.noodle` 下所有目录，所有 fsnotify 事件都进入同一个 300ms timer，不检查文件名或事件类型。timer 已经 pending 时不续期，因此持续文件活动会持续产生批次，不是等到所有写入静止后才构建。证据：[cmd_start.go:119](../../cmd_start.go#L119)、[cmd_start.go:131](../../cmd_start.go#L131)、[server/server.go:133](../../server/server.go#L133)、[server/ws_hub.go:197](../../server/ws_hub.go#L197)、[server/ws_hub.go:222](../../server/ws_hub.go#L222)。

`loadAndBroadcast` 没有检查 `clients` 是否为空，直接调用 `provider.State`、`LoadSnapshot`、`json.Marshal`、SHA-256，最后才检查 hash 和广播。hash 相同只能避免第二次完整 JSON 编码及发送，不能避免前面的读取和计算。关闭浏览器不会停止这部分服务端工作。证据：[server/ws_hub.go:247](../../server/ws_hub.go#L247)。

**扩散来源。** dispatcher 在收到流数据时写 stamped 与 canonical 文件；delta 也先被写入 canonical，再进入只发直播的分支。即使 delta 没有改变快照需要的最终事件字段，也会因目录监听导致重新构建。历史 `meta.json` 的重复写入、临时文件和其他 runtime 文件同样触发该监听；后者是否正在发生应以主调查现场计数为准。证据：[stamp/processor.go:75](../../stamp/processor.go#L75)、[stamp/processor.go:82](../../stamp/processor.go#L82)、[dispatcher/session_base.go:315](../../dispatcher/session_base.go#L315)、[dispatcher/session_base.go:338](../../dispatcher/session_base.go#L338)。

**最小方向与代价。** 无客户端时跳过 snapshot 构建；新客户端仍走已有即时初始快照。进一步以实际快照依赖限定通知来源，或者由语义状态版本驱动失效。前者改动小，后者需要明确活跃 session 事件、loop events、控制状态与 projection 的依赖，不能只过滤所有文件通知而漏掉合法更新。此链没有写回文件，不应描述成另一条独立自触发回路；它是上游写放大的下游消费者。

## D2：每份 snapshot 重读活跃会话完整日志

`LoadSnapshot` 注释声称没有 bulk event reading，但实际对每个 `ActiveCook` 调 `enrichActiveSession`，后者通过 `ReadSession` 从头读取并 JSON 解析整份 `events.ndjson`，再遍历全部事件，仅为计算当前 action 和累计输入 token。`EventReader` 没有增量 cursor 或 cache。证据：[internal/snapshot/snapshot_builder.go:18](../../internal/snapshot/snapshot_builder.go#L18)、[internal/snapshot/snapshot_builder.go:27](../../internal/snapshot/snapshot_builder.go#L27)、[internal/snapshot/snapshot_builder.go:188](../../internal/snapshot/snapshot_builder.go#L188)、[event/reader.go:51](../../event/reader.go#L51)。

单次成本随所有活跃日志总字节数增加，再乘以 D1 的快照频率。HTTP `/api/snapshot`、新 WS 连接的首份 snapshot 也分别调用同一构建函数，没有共享已完成的结果。没有 active cooks 时，这一项没有会话日志扫描成本，不能用它解释没有活跃会话的全部 CPU。证据：[server/server.go:167](../../server/server.go#L167)、[server/ws_hub.go:325](../../server/ws_hub.go#L325)。

此外每次还会读取控制队列与整个 `loop-events.ndjson`、排序 feed events；标准 loop writer 把后者限制为最近 200 行，所以不能说该文件无限增长。证据：[internal/snapshot/snapshot_builder.go:129](../../internal/snapshot/snapshot_builder.go#L129)、[internal/snapshot/feed_events.go:58](../../internal/snapshot/feed_events.go#L58)、[event/loop_event.go:41](../../event/loop_event.go#L41)。

**最小方向与代价。** 先消除 D1 无意义构建，再对活跃会话建立按日志变更失效的摘要，或让事件消费方维护 action/token 摘要。后者需覆盖启动恢复、外部事件写入、日志截断/替换、尾部半行与错误重试，不能简单假设日志永不变化。不要删除 action/cost 更新、重连初始快照或恢复时的必要首次读取。

## D3：无订阅者也序列化全部直播消息

`noodle start` 在判断是否启用 server 之前就创建 broker 并传给 runtime loop。因此即使 server 被配置为关闭，sink 仍然非 nil。`PublishSessionEvent` 先转换展示事件并编码 JSON，`PublishSessionDelta` 先编码 JSON，最后 `fanout` 才读取订阅列表。没有人看该 session 时，上述展示工作全部被丢弃。证据：[cmd_start.go:90](../../cmd_start.go#L90)、[server/session_broker.go:66](../../server/session_broker.go#L66)、[server/session_broker.go:83](../../server/session_broker.go#L83)、[server/session_broker.go:96](../../server/session_broker.go#L96)。

这属于活跃输出时按事件/delta 频率放大的成本，不是没有 provider 输出时的空闲 CPU 来源。建议在编码前取接收者快照，无接收者直接返回；保持锁外发送及慢消费者处理。需保留“先订阅再读取 backfill”的衔接语义，不能跳过持久化事件写入。

## D4：首屏、会话首次打开存在 HTTP/WS 双重全量读取机会

UI root 建立 WS，snapshot hook 同时允许首次 HTTP seed；服务器新 WS 连接立即构建 snapshot。浏览器收到 WS snapshot 后取消 HTTP query，但请求如果已到服务器，`LoadSnapshot` 没有使用请求 context 来中断已开始的扫描。因此首屏两次构建可能已经发生。证据：[ui/src/routes/__root.tsx:60](../../ui/src/routes/__root.tsx#L60)、[ui/src/client/hooks.tsx:25](../../ui/src/client/hooks.tsx#L25)、[server/ws_handler.go:20](../../server/ws_handler.go#L20)、[ui/src/client/ws.ts:102](../../ui/src/client/ws.ts#L102)、[server/server.go:167](../../server/server.go#L167)。

会话详情的 `useSessionEvents` 同时发送 subscribe 与启动 HTTP query。`events_by_session` 在 snapshot 中总是空 map，首次打开一般没有可复用 initialData；HTTP 和 WS subscribe 都调用 `ReadSessionEvents` 全量读文件。收到 backfill 时取消 HTTP，只能尽量避免过时响应覆盖。每次重新订阅（包括页面离开后回来、WS 重连）都会重新发送完整 backfill，即使客户端 cache 已经有历史。证据：[ui/src/client/hooks.tsx:62](../../ui/src/client/hooks.tsx#L62)、[internal/snapshot/snapshot_builder.go:145](../../internal/snapshot/snapshot_builder.go#L145)、[server/ws_hub.go:87](../../server/ws_hub.go#L87)、[server/server.go:178](../../server/server.go#L178)、[ui/src/client/ws.ts:112](../../ui/src/client/ws.ts#L112)、[ui/src/client/ws.ts:182](../../ui/src/client/ws.ts#L182)。

**边界。** 这是有限的加载/重连成本，源码没有 UI 定时 HTTP polling。`?after=` 目前只在全量读取、格式化后过滤，不能降低服务端磁盘扫描；当前 UI hook 传的 `after` 是 `undefined`，不能把此参数当成当前持续扫描的触发源。证据：[server/server.go:184](../../server/server.go#L184)、[server/server.go:194](../../server/server.go#L194)、[ui/src/client/hooks.tsx:73](../../ui/src/client/hooks.tsx#L73)。

**方向。** 正常路径择一获取首份数据，将另一条作为失败或超时 fallback；重连增量补齐使用稳定 cursor。代价是需要设计完整的重连、缓存重建与 HTTP fallback，避免在优化重复读取时丢掉事件。相比 D1/D2 应后置。

## D5：前端每来一条事件都扫描、复制、重排全部历史

WS `session_event` handler 使用 `old.some` 在整个会话历史中查重，随后复制数组并对全数组排序；feed 依赖数组变化又重新扫描分组。每次至少 O(n) 扫描/复制，整个会话逐条增长的累计工作至少 O(n²)，另有排序调用成本；不要假设 JS 对近乎有序数组每次都执行最坏 O(n log n)。证据：[ui/src/client/ws.ts:119](../../ui/src/client/ws.ts#L119)、[ui/src/components/AgentFeed.tsx:86](../../ui/src/components/AgentFeed.tsx#L86)、[ui/src/components/SchedulerFeed.tsx:59](../../ui/src/components/SchedulerFeed.tsx#L59)、[ui/src/components/group-tools.ts:11](../../ui/src/components/group-tools.ts#L11)。

这是浏览器进程工作，不直接计入 `noodle` PID。现有虚拟列表限制 DOM 行数，但不减少以上数据处理；已有 delta 专用总线与增量 markdown renderer，不能误称每个 token 都使整个 React Query 缓存重渲染。证据：[ui/src/components/VirtualizedFeed.tsx:57](../../ui/src/components/VirtualizedFeed.tsx#L57)、[ui/src/client/ws.ts:58](../../ui/src/client/ws.ts#L58)、[ui/src/components/StreamingDelta.tsx:88](../../ui/src/components/StreamingDelta.tsx#L88)。

方向是维护事件身份集合，通常按尾部有序追加，仅对乱序事件定位插入；分组增量更新可随后做。必须保留完整身份去重与乱序 steer/user 事件显示语义，不能只按 timestamp 查重或直接删除排序。

## D6：同一 canonical event 转换两次，落盘后再编解码用于直播

`consumeCanonicalLine` 先 `eventFromCanonical` 生成持久化 record，随后通过 `FormatEventLine` 再调用完全相同的 `eventFromCanonical` 生成 sink record。action 解析、map 分配与 payload marshal 因此做两遍；broker 再将 payload 解析成展示字段并编码 envelope。该 shared base 是 process 和 sprites 两条生产运行路径共用的代码。证据：[dispatcher/session_base.go:371](../../dispatcher/session_base.go#L371)、[dispatcher/session_helpers.go:17](../../dispatcher/session_helpers.go#L17)、[dispatcher/session_helpers.go:84](../../dispatcher/session_helpers.go#L84)、[dispatcher/process_session.go:48](../../dispatcher/process_session.go#L48)、[dispatcher/sprites_session.go:62](../../dispatcher/sprites_session.go#L62)。

另外，stamp 已得到 `CanonicalEvent` 结构，却为 sidecar 编码 JSON，经 `io.MultiWriter` 把相同字节交给 interceptor，再 JSON 解码回结构。证据：[stamp/processor.go:82](../../stamp/processor.go#L82)、[dispatcher/session_base.go:315](../../dispatcher/session_base.go#L315)、[dispatcher/session_base.go:332](../../dispatcher/session_base.go#L332)。

复用第一次转换后的 event 是较小改动；提供结构化 callback 避免 JSON roundtrip 则涉及 stamp API 和两类 session 的调整，只有活跃输出 profiling 证明收益时才值得做。stamped、canonical 与 domain events 有不同消费者和诊断用途，不能据“同一输出写三份”直接删掉某一份文件。

## D7：事件 writer 每次重新创建目录、打开关闭文件

`NewEventWriter` 已经创建 session 目录，但每次 `Append` 都执行 `MkdirAll`、`OpenFile(O_APPEND)`、写入、关闭。这是每个真实事件的系统调用开销，没有 idle 定时扫描。证据：[event/writer.go:30](../../event/writer.go#L30)、[event/writer.go:68](../../event/writer.go#L68)。

长生命周期 writer 可复用文件句柄、在初始化时处理目录，但需要显式关闭、并发追加语义、文件替换/删除后的重开策略；现有 `noodle event emit` 还是单次 CLI 调用，这种调用不会获得明显收益。生产流已有长期打开的 stamped/canonical 文件。证据：[cmd_event.go:51](../../cmd_event.go#L51)、[dispatcher/session_base.go:299](../../dispatcher/session_base.go#L299)。不建议为未量化的小成本先扩大资源管理复杂度。

## D8：loop 生命周期日志追加后总扫描，满 200 条后每次再全量重写

`LoopEventWriter.Emit` 追加一条后立刻调用 `truncate`；truncate 每次从头读取所有行，超过 200 条时保留末尾 200 条并 `os.WriteFile`。因此满容量后每个新生命周期事件带来一次 append 加一次全量替换，并让 D1 看到更多文件通知。证据：[event/loop_event.go:96](../../event/loop_event.go#L96)、[event/loop_event.go:113](../../event/loop_event.go#L113)、[event/loop_event.go:150](../../event/loop_event.go#L150)。生产 loop 在构造时创建 writer，任务完成、失败等路径使用它，属于可达代码。证据：[loop/loop.go:84](../../loop/loop.go#L84)、[loop/cook_failure.go:29](../../loop/cook_failure.go#L29)、[loop/cook_completion.go:301](../../loop/cook_completion.go#L301)。

这是有界日志保留策略成本，不能称为无条件每 tick 重写，也不是无限历史问题。可记录行数并批量裁剪或轮转，但会改变“严格最多 200 条”的契约，并且要考虑日志裁剪与读者增量 cursor 的配合。优先修复重复产生语义相同事件的上游原因，之后再评估此项。

## 下游验证建议

1. 使用相同 runtime 数据，分别测 server 关闭、server 开启但零客户端、一个客户端、多个客户端，记录 snapshot 构建次数、日志读取字节与 CPU；不能仅看发送帧数，因为 hash 去重会隐藏构建工作。
2. 用一个活跃会话逐渐增长事件文件，并分别只写无关日志、只发 delta、追加最终事件，确认快照失效的范围与读取量。
3. 无客户端下持续模拟 delta，比较 broker 编码次数；不能把 provider 子进程 CPU 记到 Noodle。
4. 记录首次打开会话与重连的 HTTP/WS 请求及读取计数，区分取消发生在服务端工作之前还是之后。
5. 浏览器长会话测试单独报告，与 `noodle` PID CPU 分开。验证去重、乱序插入、backfill 和重连不会漏事件。

除 D1 的零客户端/无关文件实验外，上述客户端数量与活跃输出性能矩阵尚未执行，不应视为已取得的测量结果。


## 推荐修复顺序与验收

1. **先切断重复唤醒并让稳定输出幂等（R1/R4）。** 改动边界小，因果实验最强。不要一开始重写全部 watcher 或引入全局缓存；先让未变订单、pending review、seq 不落盘，再明确 backend-only 输出是否应唤醒 loop。
2. **拆开时间检查与数据重算（R2/R3/R5）。** PID/heartbeat、deadline 和远端 backlog 新鲜度需要时间驱动；历史日志摘要、tickets、projection 只需在输入改变时更新。分离这些节奏比统一加大 poll interval 更可靠。缓存不能以“终态”代替失效规则。
3. **减少下游无消费/无依赖更新（D1/D2/D3、R6）。** 零客户端跳过 UI 快照，零订阅者跳过直播编码；保留持久化与首次订阅恢复。只构建一次最终 loop snapshot，随后再做活跃摘要缓存。
4. **单独修正语义触发（R7/R8）。** 警告状态与可调度输入变化采用明确的事件含义，不能用整个 brief 的 hash 变化代替“有新工作”。
5. **有实际活跃输出 profiling 后再处理 D4–D8。** 首屏去重、前端增量合并、结构化事件通道、长生命周期文件句柄有收益，但不是当前热循环的首要原因。多文件日志用途不同，不能直接删掉。

最低验收矩阵应覆盖：

| 情形 | 必须验证的性质 |
| --- | --- |
| 无订单 idle、paused、failed-only、等待审查、活跃任务但暂时无输出 | 无变化期间不自触发密集 cycle；不能只测 paused |
| 真实 control 命令、orders-next 原子提交、任务完成/失败、merge 结果 | 仍及时唤醒，命令只处理一次、不会遗漏后续阶段 |
| 重复相同 projection、同一 seq、空 review/tickets | 文件内容与修改时间稳定，没有多余原子替换 |
| 历史终态日志不变、活跃日志追加、日志截断/替换 | 稳定历史不重复全读，新增内容更新准确，失效后能重建 |
| 日志不变但进程退出、远端 heartbeat 过期 | 生命周期检查仍有效，不能因 cache 跳过存活判断 |
| server 开启但零客户端、零会话订阅者、首个客户端和重连 | 无消费者不构建展示数据；首次/重连仍完整补齐 |
| 持续相同 warning、恢复后再次失败 | 不逐轮追加同一告警，但真实变化仍可见 |
| 空 scheduler 结果、仅 scheduler 自身状态变化、真实 backlog/执行变化 | 自身变化不反复调用模型，真实新输入仍能唤醒 |

测量应分开报告：主进程 CPU、adapter/provider 子进程 CPU、浏览器 CPU；cycle 数、重复写次数、扫描字节和 snapshot 构建数应同时记录。只比较网络发送次数会被 hash gate 隐藏成本，只比较“能跑通”不能证明无多余工作。

## 已有保护与不应误删的操作

- `control.ndjson` 空队列早返回已修复，本次观测为零改写；不是当前剩余触发源。
- `status.json`、`mise.json` 已有内容比较；它们本次未改写，但构建前置工作仍需另行优化。
- skill watcher 已过滤 CHMOD、限定相关路径并 debounce；未发现 `.noodle` 通知会令其持续重建 registry 的证据。
- 主循环有一秒兜底 ticker；这不意味着每秒必须全量重建，也不意味着可以删除所有时间驱动检查。
- WebSocket 已有 300ms 合并、hash gate 和慢消费者断开；问题是去重发生得晚，不能描述为“完全没有去重”。
- UI 没有持续 HTTP 定时 polling；虚拟列表与 delta 专用通道已存在。全表事件合并是另一层成本。
- canonical checkpoint 在实际状态事件之后保存，承担恢复一致性；本次现场没有每轮改写。`handlePromotionResult` 中还有可能重复同步/保存的转换链，但它夹有取消旧会话等实际状态变化，未证明可以无条件删除，不列为当前热循环根因。
- 主循环两次 drain 包围控制处理，可以处理控制命令引发的完成/merge；未因“看起来重复”直接判定应删。`drainMergeResults` 等待中的 1ms timer 主要是唤醒/延迟设计问题，非当前无 merge 状态的 CPU 来源。

## Backlog 登记与评审路径

2026-10-15 按仓库 backlog adapter 登记以下工作；完整范围与验收条件见 [[todos]]。本报告是问题证据，不是已批准的实施方案，不使用 `plans/.../overview` 链接冒充完成规划。

| 待办 | 审计项 | 交付边界 / 依赖 |
| --- | --- | --- |
| 123 | R1、R4 | 先切断输出自触发并稳定 projection/review/seq 写入；本审计内优先 |
| 124 | R2 | 在 123 后分离存活检查与历史 metadata 刷新 |
| 125 | R3 | 在 123 后按变化重建 tickets，保留历史事件语义 |
| 126 | R5 | 在 123 后定义 adapter 刷新节奏与本地/远端新鲜度 |
| 127 | R6 | 在 123 后复用最终 projection/发布；与 115 协调 |
| 128 | D1、D3 | 在 123 后处理无消费者构建/编码与无关通知；不承担整个协议重设计 |
| 129 | R8 | 告警按真实变化发出；调度语义与 120 协调 |
| 130 | D4、D5 | 在 116 明确协议所有权后，按测量优化历史加载与前端合并 |
| 131 | D6、D7、D8 | 在 123–128 后测量活跃输出成本；先交付收益/成本判断，再登记值得实施的优化 |
| 120（复用） | R7 | 补充现有调度器自触发任务，不新建重复修复 |
| 116（复用） | D2、D1 剩余部分 | 在现有 projection/delta 协议任务内明确历史读取、失效与重连语义；复用 128 的消费者门控 |

123–128、130 及已有 120/116 均标记 `#needs_plan`。129 是单一告警行为修复；131 是边界明确的测量任务，当前不要求架构方案，后续若扩大实现范围须重新评估复杂度。保留已有全局 priority 顺序，以上依赖和顺序只约束本审计的工作，未宣称覆盖其他项目优先级。

仓库 [schedule skill](../../.agents/skills/schedule/SKILL.md) 规定：复杂、无方案的任务使用调用 `/plan` 的 prompt-only stage，后接独立 `adversarial-review` stage；不能用 `execute` 代替规划，也不在规划流水线加入代码用的 quality/reflect。形成方案后，下一轮按阶段安排 `execute → quality → reflect`。

[adversarial-review skill](../../.agents/skills/adversarial-review/SKILL.md) 要求用另一家模型的 CLI 评审，按不同视角给出 PASS / CONTESTED / REJECT、问题与 lead judgment。交互式 `/plan` 还会呈现方案供用户审阅；自动执行模式提交方案并 yield，不等待人工。这是技能/调度约定，不应把 `#needs_plan` 标签或本报告当成已经完成独立评审的证明；实际执行须核对订单阶段与评审产物。

本轮只登记 backlog 与证据，不启动 `/plan`、review 或实现，不改运行中的 orders。

## 本次调查的边界

所有建议仍需实现和回归测试；本次未把 overlay 实验修改应用到源码或安装程序。现场计数与三个隔离场景足以确认 R1 和 D1；其他条目的静态可达性、必要保留语义和额外验证成本已逐项列明，不承诺累计优化百分比。
