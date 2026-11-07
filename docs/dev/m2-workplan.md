# M2 逐任务执行手册

| 项 | 内容 |
|---|---|
| 状态 | 草案（M2-00..10 旧模型已验收；M2-11A 已验收（2026-11-06）；M2-11B 已验收（2026-11-06）；M2-11C 已验收（2026-11-07）；M2-11D 已验收（2026-11-07）；M2-12A～13 未开始） |
| 最后更新 | 2026-11-07 |
| 适用 | 每个新开对话只执行一个 M2 任务 |
| 关联 | [路线图](roadmap.md)、[流程](process.md)、[PRD](../prd.md)、[架构](../architecture.md)、[协议](../agent-protocol.md)、[验收](../acceptance.md)、[ADR](../decisions.md) |

> **M2 不一口气写完。** 每个任务单独开一个对话，做完、跑门禁、更新文档与证据，交给用户评审后再开下一个。任何任务都不得顺手实现其他任务的代码。

> M2-08～10 的选题闭环细化任务及顺序见 [工作台至选题闭环计划](m2-topic-closure-plan.md)。该计划将 M2-08 和 M2-10 拆成单独验收的子项，并在其中安排本闭环必需的部分 M2-12 功能；各原任务状态仍以本手册状态表和用户验收为准。

> **M2-11～13 目标切换**：用户选择 RM-018 的本地脚本子范围纳入 M2-11～13，详见[目标闭环计划](m2-target-closure-plan.md)。M2-00～10 旧模型实现与验收事实保留，不覆盖新目标；M2-11A 已于 2026-11-06 验收，M2-11B 工程格式与图领域模型已于 2026-11-06 验收；M2-11C 已验收（2026-11-07）；M2-11D 已验收（2026-11-07）；M2-12A～13 仍须按各自依赖、DoR 和门禁推进。云端 RM-019 与视频 RM-020 仍待评估，不能混入本批次。

---

## 1. 当前模型（每次开工先对齐）

> 以下 #1～12 是 M2-00～10 已验收旧模型规则。M2-11～13 以本手册对应的新任务卡、`m2-target-closure-plan.md`、ADR-027 和用户评审后的设计为准；冲突时不能把旧规则套用到新目标。

| # | 决策 |
|---|---|
| 1 | 图是**单父森林**：每节点最多一个父；topic 必须无父且是树根；成环、自连、topic 作子节点拒绝。 |
| 2 | 任意 prompt 节点（含子树）可**自由改挂**到任意可用输出端口；目标已有不同父或同一父不同候选需确认；改挂移动子树并将该子树标记 `stale`。 |
| 3 | **topic 是特殊 prompt**：应用候选后锁定内容与执行；每个候选是一个输出端口；连接端口即选定该候选；没有 `selectedTopic`/`SelectTopic`。 |
| 4 | **链 = topic 根到末端节点的派生路径**，不持久化；分支产生多条共享前缀的链；链 ID 视图层为 `c_<tailNodeId>`。 |
| 5 | 链与会话分离：每次节点执行一个**一次性运行**；App 从路径构建上下文；运行之间不共享 Agent 记忆；不使用 resume。 |
| 6 | 上下文只含该路径祖先的最新已应用轮次与**该路径所选候选**；不含目标自身历史、兄弟分支、子节点、其他候选、其他 topic 树。 |
| 7 | 上下文预算按 **token**，默认 `contextTokenBudget=1,000,000`；发送前保守估算，运行后记录实际 usage。 |
| 8 | **运行锁**：运行/排队路径（topic 根 → 运行节点）与共享祖先只读（参数、拓扑、执行、新增子节点）；路径外兄弟分支可编辑。 |
| 9 | 工作区按 **topic 树**共享；内置读限当前运行路径、写限当前运行节点；MCP 只读叠加视图沿路径祖先并回退到 exe 同级 `runtime/`；其他 topic 树拒绝。 |
| 10 | M2 不开放 Agent 写图工具；`apply_artifact` 永远只由 UI 调用；M2 全局单在途运行，队列在 M5。 |
| 11 | **锁定后不可再次执行**；M2 的 topic/prompt 每节点最多一个已应用轮次；迭代通过新增下游节点或候选端口分支。 |
| 12 | **同一父换候选按改挂确认**并标记子树 stale；`SelectNode` 只持久化 `activeNodeId`，不递增图 revision、不广播；派生链由 Go 快照返回。 |

权威文档：模型细节以 `decisions.md` ADR-019..026 为准；需求以 PRD、验收以 acceptance 为准。发现冲突先停下来问用户。

---

## 2. 每次新对话的开工协议

新对话建议直接粘贴：

```text
按 CLAUDE.md 与 docs/dev/m2-workplan.md 执行 M2-XX。
开工前读：CLAUDE.md、docs/dev/roadmap.md、docs/dev/process.md、
docs/dev/m2-workplan.md、该任务涉及的 ADR 与验收用例。
只实现 M2-XX，不顺手做其他任务；测试与文档同次完成；
跑任务级门禁；完成后把任务状态改为「待验收」并列出证据，不要自我验收。
```

步骤：

1. 读 `CLAUDE.md`、路线图、流程、本手册与相关 ADR；确认任务状态、依赖、DoR。
2. 检查上一个任务是否已完成/待验收；依赖未满足不抢跑。
3. 实现代码 + 测试 + 受影响文档 + `CHANGELOG.md` Unreleased。
4. 跑本手册第 4 节门禁；保留输出。
5. 更新本手册任务状态为「待验收」（只能由用户改为「已完成」），写清 AC 映射与证据路径。
6. 停下等用户评审；不自动进入下一任务。

DoR（任一不满足则不开工）：任务有 F-xx/AC；依赖已完成；设计文档已更新；测试方式明确；不在暂存池。

DoD：分层与不变量未破坏；新增/修改逻辑有测试；任务级门禁通过；文档与 CHANGELOG 同步；`process.md` §14.4 自审完成；用户评审通过。

---

## 3. 任务卡

### M2-00 设计收口

- **状态**：已完成（用户确认开工 M2-01）。
- **目标**：ADR、PRD、架构、协议、前端设计、验收、路线图、流程、CLAUDE、README、CHANGELOG 与原型一致。
- **产物**：`docs/decisions.md` ADR-019..024；`docs/design/m2-workspace.html`；各文档同步。
- **已确认决策**：锁定节点不可重跑；M2 范围纳入 F-06/F-13；同一父换候选需确认；`SelectNode` 不递增图 revision、不广播；派生链由 Go 快照返回。
- **出口**：用户确认设计；未确认前不得写 M2 实现代码。

### M2-01 工程存储与 M1 迁移

- **状态**：已完成（用户确认开始 M2-02）。
- **需求 / 验收**：F-01；AC-01、AC-M2-7。
- **依赖**：M2-00（已完成）。
- **目标**：`project.json` v1、`config.json`、`runs.json`/`runs/`、`pending.json`、artifact 存储、路径布局、自动保存、M1 迁移。
- **关键设计**：
  - `project.json`：`version/id/name/revision/activeNodeId/nodes[]`；节点含 `id/type/title/parent/parentCandidate/position/params/rounds/locked/stale`；**不存 chains**。
  - `config.json`：`lastProjectId`、`contextTokenBudget`（默认 1,000,000）；读取损坏配置直接报错，不覆写。
  - 目录：`projects/<id>/{project.json,pending.json,pending/,artifacts/,runs.json,runs/,trees/<rootTopicId>/steps/<nodeId>/}`。
  - 原子写：临时文件 + `Sync` + 重命名；工程创建与迁移先写 staging 目录再整体重命名；损坏文件不静默重建。
  - 运行索引/详情：`runs.json` 只存摘要，`runs/<runId>.json` 存 transcript 与上下文预览；先写详情再写索引，索引引用缺失详情视为损坏。
  - 迁移：仅当 `data/sessions.json` 存在；创建默认工程与 topic 根，名称均为「M1 迁移」；只导入 `activeChainId`（M1 仅有 `chain_default`，其他记录留在备份不导入）；transcript 导入为 `status=imported` 只读运行（`chainPath=[root]`）；旧 workspace 复制到 `trees/<root>/steps/<root>`；`sessions.json` 与旧 workspace 复制到 `data/backup/m1/`，原文件不移动；以 `data/backup/m1/migration.json` 作为完成标记，工程目录存在但标记缺失时补写，标记存在而工程被删除时不重新导入；失败 `MIGRATION_FAILED`，staging 清理。
  - 自动保存：生命周期命令（新建/打开/切换）同步落盘；后续图命令在命令层成功后立即原子写入，不提供手动保存。
- **文件**：`internal/domain/{valueobject,port,entity}/{project,id,config,run,pending,artifact}*.go`；`internal/infra/persistence/{project,runs,pending,artifacts,config,migrate,atomic,copy}.go`；`internal/infra/paths/{layout,project}.go`；`internal/service/project/`；`internal/handler/project.go`；`internal/app/project.go`；Wails 绑定。
- **测试**：原子写/损坏/版本；新建/打开/切换/重启恢复；迁移有数据/无数据/损坏/目标已存在/失败清理/幂等；payload 限额与原文保留；artifact 去重与不可变；目录与 bundle 不写入 AppData。
- **证据**：`go test . ./internal/...`、`go vet . ./internal/...`；`internal/infra/persistence/*_test.go`、`internal/service/project/service_test.go`、`internal/handler/project_test.go`、`internal/app/project_test.go`；绑定 `frontend/bindings/audio-ui/internal/handler/projectservice.ts`。
- **门禁**：gofmt / go vet / go test、bindings、typecheck、lint、check:tokens、前端测试 21 项、前端构建、`wails3 build` 全部通过；前端首屏 JS 83.53 kB gzip；`--version` 与 `--self-check` 通过。
- **未包含**：工程入口/切换 UI（M2-08）；运行/pending/artifact 的业务语义与 apply（M2-05/M2-07）；画布命令（M2-02）。
- **风险**：迁移半途失败；路径安全；`runs.json` 与 `runs/<id>` 双文件一致性；共享内容寻址产物的 meta 以首次写入为准。

### M2-02 图命令层、自由改挂与运行锁

- **状态**：已完成（用户确认 AC-M2-1/6/11 验收通过；2026-11-02）。
- **需求 / 验收**：F-03；AC-M2-1、AC-M2-6、AC-M2-11。
- **依赖**：M2-01（已完成）。
- **目标**：单父树校验、草稿、自由改挂/子树移动、topic 候选端口、运行路径锁、revision。
- **关键设计**：
  - 命令：`AddNode`（topic 根 / prompt 带父或草稿）、`Connect`（含 `replace` 改挂）、`MoveNode`（画布位置）、`UpdateNodeParams`（可在未锁定时同时更新 `title`）、`SelectNode`（activeNodeId）；`EnsureExecutable` 供 M2-07 执行前检查锁定。
  - `Connect(parent, child, parentCandidate, replace)`：child 为 prompt；parent 为任意 topic 树内的 topic/prompt；self/cycle/topic-as-child/父为草稿拒绝；child 的 `(parent, parentCandidate)` 与目标不同（含同一父换候选）且 `replace=false` → `REPARENT_CONFIRM_REQUIRED`；同一父同一候选重复连接为幂等无操作；确认后移动子树（同父换候选不移动子树）并将被移动节点及其后代标记 `stale`。跨树时迁移子树 steps；失败 `WORKSPACE_MOVE_FAILED`；保存失败回滚 steps。
  - topic 候选：`parentCandidate` 为 0 基；候选数量从该 topic 最新已应用 `topics` 产物读取；缺失/越界/产物不可读 → `CANDIDATE_INVALID`。父为 prompt 或草稿时清空 `parentCandidate`。
  - 锁定：topic 应用后、prompt 首轮后内容与执行锁定；`UpdateNodeParams` 与 `EnsureExecutable` 拒绝（`NODE_LOCKED`）；锁定节点的拓扑仍可改，运行链上则拒绝。
  - 运行锁：`AcquireRunLock(runID, treeID, nodeID, chainPath)` 记录 topic 根 → 运行节点路径；路径节点（含共享祖先）只读；对路径节点拒绝参数、改挂、新增子节点与再次执行（`RUNNING_CHAIN_LOCKED`）；路径外分支允许。路径必须是真实父子链。
  - revision：`AddNode` / `Connect` / `MoveNode` / `UpdateNodeParams` 成功后递增并触发 `OnChange(revision, "graph")`；幂等无操作不递增；`SelectNode` 只持久化 `activeNodeId`，不递增、不广播。
  - 链为派生：`entity.Graph.LeafChains` 返回 `c_<tail>` 路径；不落盘；`ActiveNodeID` 仍由工程持久化。
- **文件**：`internal/domain/entity/graph.go`；`internal/domain/port/{project,workspace}.go`；`internal/service/workflow/{service,candidates,validate,locks,errors}.go`；`internal/service/project/service.go`（`Save`）；`internal/infra/workspace/mover.go`；`internal/infra/persistence/artifacts_provider.go`；`internal/handler/workflow.go`；`internal/app/workflow.go`；测试。
- **测试**：单父/自连/成环/悬空/重复 ID/非法候选；改挂确认与子树 stale；同一父换候选确认/幂等；跨树 steps 迁移/保存失败回滚；运行锁路径/共享祖先/兄弟分支；锁定节点执行与参数拒绝、拓扑可变；`SelectNode` 不递增 revision/不广播；非法 ID/位置；候选计数从 artifacts 读取；Wails 绑定与工程接线。
- **证据**：`go test -count=1 . ./internal/...`；`internal/domain/entity/graph_test.go`、`internal/service/workflow/*_test.go`、`internal/infra/workspace/mover_test.go`、`internal/handler/workflow_test.go`、`internal/app/project_test.go`；`frontend/bindings/audio-ui/internal/handler/workflowservice.ts`。
- **门禁**：gofmt / go vet / go test、bindings、typecheck、lint、check:tokens、前端测试、前端构建、`wails3 build` 通过；前端首屏 JS 83.53 kB gzip 不变。
- **未包含**：运行启动/上下文（M2-04/M2-07）、应用产物与锁定写入（M2-05）、快照与事件桥/画布 UI（M2-08/M2-09）、工作区读写 guard（M2-03）。`OnChange` 仅提供回调，Wails 事件桥在 M2-08 接入。
- **风险**：子树移动遗漏 stale；运行锁与并发命令竞态；链派生与 activeNode 不一致；跨树 steps 移动在进程崩溃窗口内可能不同步。

### M2-03 工作区与写作用域

- **状态**：已完成（用户确认验收通过；2026-11-02）。
- **验收记录**：[M2-03 验收证据](evidence/m2-03.md)（用户已确认验收完成）。
- **需求 / 验收**：F-03、F-13；不变量 9；AC-M2-11（路径隔离）、AC-M1-2（权限回归）。
- **依赖**：M2-01（已完成）；M2-02（已验收通过）。
- **目标**：topic 树 cwd、`steps/<nodeId>`、路径叠加读取、内置文件读写白名单、Bash 残余风险登记；跨树 steps 移动沿用 M2-02 已实现的 mover。
- **实现**：
  - `NewRunView` 将 topic 树 cwd 定位到 `trees/<rootTopicId>/`，确保运行路径中各节点 `steps/<nodeId>/` 存在；MCP 逻辑读取顺序为当前节点 → 路径祖先 → exe 同级根 `runtime/`。
  - MCP `read_file` / `list_dir` / `grep` / `stat` 消费 `WorkspaceReader`；列表按路径优先级合并，同名条目由当前节点/最近祖先遮蔽；Agent 只看到逻辑相对路径，不可访问兄弟节点、其他 topic 树、物理 `steps/`、`artifacts/` 或保留目录名。
  - MCP 读取限额：单文件 256 KiB、目录 200 项、grep 200 hits / 5,000 files / 单文件 512 KiB；无效 UTF-8 文件不作为文本返回，grep 跳过二进制文件。
  - `RunGuard` 实现 `ScopedPathResolver`：内置 Read/Glob/Grep 仅限运行路径的实际 steps；Write/Edit/NotebookEdit 仅限当前运行节点 steps，并保留交互确认；越界/reparse point/junction 拒绝。
  - `RunToolServerFactory.NewForRun` 支持按运行注入 overlay reader；M1 旧工厂继续使用 `NewDirectView`，保留旧 workspace 与绝对路径兼容。Bash 仍是协作式门禁，残余项 RM-017。
  - 用户确认共享只读运行时为 exe 同级 `runtime/`；`data/runtime/` 不使用，已同步文档与发布目录语义（RM-013）。
- **文件**：`internal/domain/port/{agent,workspace}.go`、`internal/domain/valueobject/environment.go`、`internal/infra/paths/{layout,guard,run_guard}.go`、`internal/infra/workspace/{view,mover}.go`、`internal/infra/agent/mcp/server.go`、`internal/service/permission/policy.go`、`internal/app/tools.go` 与测试；README、CLAUDE、架构、产品、协议、验收、路线图、CHANGELOG。
- **测试**：`go test -count=1 ./internal/infra/paths ./internal/infra/workspace ./internal/infra/agent/mcp ./internal/service/permission ./internal/app`；覆盖路径与写白名单、overlay precedence/遮蔽/list/grep、runtime 回退、兄弟隔离、限额/UTF-8、跨树移动与回滚、reparse point/junction、MCP 注入与 M1 绝对路径回归。
- **证据**：`go test -count=1 . ./internal/...` 全量通过；`go vet . ./internal/...` 通过；前端 typecheck/lint/check:tokens 通过，6 个文件 / 21 项测试通过；绑定生成、前端构建（首屏 JS 83.53 kB gzip）、`wails3 build` 通过；`bin/audio-ui.exe --version` 与 `--self-check` 通过。首次与其他门禁并行时环境超时测试受负载影响，单独重跑及随后全量 Go 测试均通过。
- **未包含**：上下文构建（M2-04）、MCP 图工具/reply（M2-06）、RunService/每运行 `RunGuard` 与 reader 的实际启动接线（M2-07）。
- **风险**：Bash 命令语法无法被完整解析；同一 topic 树内共享文件，RM-017 保留。

### M2-04 上下文构建器

- **状态**：已完成（用户审阅并确认验收通过；2026-11-02）。
- **需求 / 验收**：F-04/F-05；AC-M2-8、AC-M2-10、AC-M2-12 的构建器部分。
- **依赖**：M2-01、M2-02（已完成）。
- **目标**：从 topic 根 → 目标节点构建路径上下文；候选解析；token 估算/降级；哈希；工作区清单。
- **实现**：
  - 推导真实路径，祖先只取最新已应用轮次；topic 只带所选候选，目标只带完整当前参数，不带自身历史、兄弟、子节点、其他候选或其他树。
  - `RunWorkspaceReader.Scope()` 校验工程/树/目标/路径；清单仅包含逻辑路径与大小，不读取工作区文件内容；二进制资源仅引用。清单最多 5,000 个文件/目录、深度 64，截断显式标记。
  - 默认预算 1,000,000 tokens；超出时从最远祖先逐个「全文 → 2,000 估算 tokens 原文摘录 → 引用」，直接父最后；目标参数始终完整；仍超返回 `CONTEXT_TOO_LARGE` 和估算值/预算。
  - `estimatorVersion=1` 覆盖完整首条消息；`ContextEstimate.WithActualInputTokens` 可计算 `overBudget`；实际 CLI usage 采集与持久化留 M2-07 接线。
  - `contextHash` 覆盖上下文主体和工作区清单，`ancestorHash` 覆盖图祖先与候选但不含工作区，`paramsHash` 覆盖目标参数；`InputHashes` 可供 M2-05 校验复用。详见 ADR-026。
- **文件**：`internal/service/context/`；`internal/domain/valueobject/context.go`；`internal/domain/port/workspace.go`；`internal/infra/workspace/view.go`。
- **测试**：golden、同输入同哈希、候选分支、路径隔离、降级顺序、完整目标、超限、混合文本估算、清单上限、真实叠加视图与文件正文隔离。
- **证据与门禁**：[M2-04 自检证据](evidence/m2-04.md)；最终 Go 全量及 vet、前端门禁、临时副本绑定/构建通过；首屏 JS 83.53 kB gzip。
- **未包含**：待应用校验接线（M2-05）、运行启动与实际 usage 采集/存储（M2-07）、GUI 与 live CLI 端到端验收（M2-13）。
- **风险**：token 为启发式估算；清单只按路径与大小入 `contextHash`，无法识别同大小内容变化或截断范围外变化。

### M2-05 Pending 后端

- **状态**：已完成（用户验收通过；2026-11-02）；不代表 AC-M2-3/4、AC-07 的 UI/端到端验收完成。
- **需求 / 验收**：F-07；AC-M2-3/4、AC-07（后端组件级覆盖）。
- **依赖**：M2-02、M2-04。
- **目标**：pending 存储、Schema、应用/丢弃、轮次、锁定、下游 stale、应用后提升为 artifact。
- **实现与不变量**：
  - `pending.json` 保存索引，`pending/<id>.json` 保存原始 payload；`pending.txn.json` 协调 pending、artifact 与 `project.json` revision。工程加载按 base/target revision 回滚未提交意图或幂等恢复已提交索引、原文与 CAS；revision 不匹配、日志损坏或恢复失败时拒绝暴露工程。
  - Pending 绑定 runId/chainId/chainPath/nodeId/kind/contextHash/ancestorHash/paramsHash；payload 上限 256 KiB，`request_id` 上限 128 字节。无效 JSON/Schema 原文与错误持久化，状态 `invalid`，不可应用。
  - `topics` 要求 3–10 个对象候选，各候选 `title`/`angle`/`hook` 为字符串；`script` 要求顶层 `title` 为字符串、至少一个 scene，scene 的 `narration` 为字符串、`duration` 为 JSON 数字。保留额外字段；不添加正数时长约束；`scene_dsl` 返回 `UNSUPPORTED_UNTIL_M3`。
  - 同一运行最多一个结构化结果；相同 runId + request_id 且绑定信息与原始 payload 一致时幂等返回原记录，不重复递增 revision/广播；同 ID 内容冲突返回 `REQUEST_ID_CONFLICT`，第二个结果返回 `TURN_PENDING_EXISTS`。
  - 每 topic 树最多 50 条 `pending`/`invalid`；应用或丢弃释放名额，历史索引及原文不删除。
  - Apply 重验节点/kind、Schema、运行锁、内容锁，并重算 `ancestorHash`/`paramsHash`；`force=true` 仅绕过哈希差异，不绕过其他校验。成功后写确定性 JSON CAS artifact、追加轮次、锁定节点内容与执行、将全部后代标记 stale；当前轮次的 `Round.Kind` 是 artifact kind 的权威值。
  - Discard 幂等，只更新状态；已应用 pending 不可丢弃。成功写操作递增 project revision、以 `pending` reason 通知；项目切换与多文件写入由同一工程操作锁串行化。
  - Wails handler 提供列表、详情、应用与丢弃；创建入口留给 M2-06 的 `reply` 接线。topic 应用后候选端口仍由现有候选计数逻辑读取，无 selectedTopic。
- **文件**：`internal/domain/{entity/pending.go,port/project.go,port/context.go,valueobject/project.go,valueobject/structured_payload.go}`；`internal/infra/persistence/{pending.go,pending_provider.go,pending_recovery.go,project.go}`；`internal/service/{project/service.go,workflow/pending.go,workflow/service.go,workflow/errors.go,workflow/candidates.go,context/builder.go}`；`internal/app/workflow.go`；`internal/handler/workflow.go`。
- **测试**：Schema 与 canonical JSON 精度/额外字段；幂等重放、request_id 冲突、同 run 第二结果拒绝；无效原文保留；payload/每树限额及释放名额；哈希冲突与 force 边界；轮次、锁定、后代 stale、CAS 去重/跨 kind；discard 幂等及重复 apply；pending ID 身份冲突不覆盖原文；未提交回滚、已提交恢复、恢复失败阻断及重试。主要用例位于 `internal/domain/valueobject/structured_payload_test.go`、`internal/service/workflow/pending_test.go`、`internal/infra/persistence/pending*_test.go`、`internal/handler/workflow_test.go`。
- **证据**：[M2-05 自检证据](evidence/m2-05.md)。
- **未包含**：MCP `reply` 与 create pending 的协议接线、只读图工具/`read_artifact`（M2-06）；运行启动与实际 usage 存储（M2-07）；前端 pending 卡片及 GUI/live CLI 端到端验收（后续任务）。
- **风险**：事务日志损坏或磁盘错误时工程 fail closed，需人工修复/恢复文件；内容寻址目录的 `meta.json` 首写为准，消费端必须按 project round kind 校验；组件级自动化不等同于 GUI/端到端 AC 验收。

### M2-06 MCP 只读图与结构化 reply

- **状态**：已完成（用户于 2026-11-03 确认验收）。GUI/live CLI 与 M2 整体端到端验收仍按后续任务范围推进。
- **证据**：[M2-06 自检与验收证据](evidence/m2-06.md)。
- **需求 / 验收**：F-04、F-07；AC-M2-3、AC-M2-12、AC-06（MCP/后端组件级覆盖）。
- **依赖**：M2-02、M2-05（均已验收）。
- **实现**：
  - 新增 `RunScope`、`GraphReader`、`ArtifactReader`、`ReplySink`、`RunLifecycle` 端口；scope 绑定 runId/projectId/treeId/nodeId/chainId/chainPath 与上下文哈希。活动运行租约必须覆盖工具读写，终止不能与 pending 提交竞态。
  - `get_chain/get_graph/get_node/list_node_types/validate/read_artifact/reply` 仅在 run-scoped MCP 注册；M1 `New` 与 `reply(message)` 保留兼容。图/节点读取只返回当前运行路径，不暴露同树兄弟。
  - `validate` 复用 M2-05 唯一 Schema 校验器且不落盘；`reply` 与 `validate` 通过 MCP 原始 arguments 取得 payload。结构化 reply 委托 workflow 创建 pending；Schema 无效时仍保留原文为 invalid；不注册写图或 apply 工具。
  - `reply.node_id` 先校验运行活动，再校验目标：结束运行或路径内非目标节点返回 `TURN_NOT_ACTIVE`，路径外节点返回 `CHAIN_SCOPE_DENIED`。
  - `read_artifact` 仅授权路径祖先轮次引用，授权以 `Round.Kind` 为准；topic 先过滤当前路径所选候选对象，再紧凑序列化并分页；跨祖先相同 ID 视图冲突时 fail closed。单次 ≤16 KiB，offset/nextOffset 按 UTF-8 字节边界计算；不回退完整 topic 产物。
  - 新 MCP 工具加入自动允许列表，作用域仍由服务端强制校验；已存在文件/问题工具也在 scoped 服务中验证活动运行租约。
- **文件**：`internal/domain/port/agent.go`、`internal/service/workflow/{agent_tools,pending}.go`、`internal/infra/agent/mcp/{server,scoped_tools}.go`、`internal/service/permission/policy.go`、`internal/app/{tools,agent,workflow,app}.go` 与测试；`docs/decisions.md` ADR-023、架构、Agent 协议、验收、CHANGELOG。
- **测试**：完整工具清单与禁止工具；路径图/节点和工程/树 scope；过期运行与结束竞态；raw payload 保真、无效 Schema pending、幂等与并发重复回复；topic 多候选与相同 artifact ID 冲突；round kind 与 artifact meta 冲突；UTF-8 边界、空尾块、非法 offset/limit、16 KiB 上限；权限自动允许与 M1 文件工具回归。
- **门禁**：`docs/dev/evidence/m2-06.md` 记录实际 Go、vet、bindings、前端、Wails 门禁结果。
- **未包含**：真实 `RunService`/CLI 的生命周期实现与节点执行接线（M2-07）；前端、GUI/live CLI 端到端 AC 验收（M2-08..13）。
- **风险/边界**：真实运行租约已由 M2-07 接线并验收；GUI/live CLI 与 M2 整体端到端验收仍属于后续任务。

### M2-07 运行管理与运行锁

- **状态**：已完成（用户于 2026-11-03 确认验收）。GUI/live CLI 与 AC-M2-9/11 的后续端到端验收按 M2-13 推进。
- **证据**：[M2-07 自检与验收证据](evidence/m2-07.md)。
- **需求 / 验收**：F-06、F-13；AC-06、AC-13、AC-M1-1/2/5、AC-M2-9、AC-M2-11。
- **依赖**：M2-03、M2-04、M2-06。
- **目标**：一次性 driver、per-run MCP/token/permission、TurnGate、run store、runId/treeId/chainPath 事件、运行路径锁、重启中断。
- **关键设计**：
  - `RunSpec`：runId/treeId/nodeId/chainPath/workspace/context/version/hash/tools/system prompt/MCP/gates。
  - per-run MCP 配置 `data/agent/<projectId>/<runId>/mcp.json`；结束删除；token 不落日志。
  - 全局 TurnGate（M2 1 个）；所有终态释放；带 runId owner；60 分钟无有效进展时 `RUN_TIMEOUT`，流式、工具、权限/问题和 MCP 活动会重置看门狗。
  - 运行锁在启动时记录路径快照；路径节点只读；共享祖先只读。
  - 重启把 starting/running 标记 interrupted；transcript ≤500；运行记录全部保留。
  - driver 的 allowed tools/system prompt 按运行注入，删除 M1 硬编码。
  - 终止会等待 Agent 事件、权限应答与 MCP 活动操作租约归还；transcript 限制为最近 500 条。
  - 从首个 `AssistantMessage.Usage` 读取首次请求 input tokens（含缓存 token），不把多轮累计 `ResultMessage.Usage` 当作上下文用量；缺失时保持未知。与估算/预算一并持久化；预览最大 16 KiB。
- **文件**：`internal/service/run/{manager,gate}.go`、run handler、`internal/infra/paths/run_config.go`、run store/workspace provider、Claude driver、permission/audit、workflow 运行锁、Wails app wiring/eventbus。
- **测试**：TurnGate 所有权与释放、运行成功/usage、BUSY、Send 失败资源释放、活动租约终止、跨工程重启恢复、配置目录路径安全、工程切换保护、审计身份字段、首次 usage 与缺失 usage、60 分钟无进展及有效事件重置；live CLI（需用户另行明确同意外部访问和潜在费用）。
- **门禁**：`docs/dev/evidence/m2-07.md` 记录实际 Go/vet、bindings、前端、Wails 门禁与 live CLI 结果。
- **未包含**：前端事件桥与运行工作台（M2-08）；GUI 手工验收、启用真实 Claude CLI 的端到端验收与 AC-M2-9/11 完整抽查（M2-13）。
- **风险**：真实 CLI 与 UI 的集成验收仍未执行；Bash 命令隔离保留 RM-017 的协作式门禁边界。

### M2-08 前端工作台骨架

- **需求 / 验收**：F-01、F-03；AC-01、AC-03。
- **依赖**：M2-07。
- **子项进度**：M2-08A 后端快照、事件与恢复契约已验收（用户于 2026-11-03 确认）；任务级 Go、前端、绑定生成与 Wails 构建门禁通过。AC-01/03/06 有接口级自动化覆盖，证据见 [M2-08A 自检与验收证据](evidence/m2-08a.md)。M2-08B 前端工作台与运行控制台已实现、自动化及构建门禁通过；用户于 2026-11-03 明确指定截图更名后即算完成验收，证据见 [M2-08B 自检证据](evidence/m2-08b.md)。原任务 M2-08 已完成；M2-09 已于 2026-11-04 验收，证据见 [M2-09](evidence/m2-09.md)。
- **M2-08A 接口**：`GetWorkspaceSnapshot` 返回进程内工作台 revision 与工程、节点、pending/run 摘要、Go 派生链及路径锁；`workspace:changed` 携带来源 `projectId`、reason 和持久化工程 revision。运行事件加入 `projectId`，`GetRunSnapshot` 同步返回 transcript、`lastSeq` 与活动待决请求。两种 revision 不落盘混用，既有 `GetRun` 和运行记录格式保持兼容。
- **目标**：workspaceStore、runStore、事件桥、控制台分页、工程入口、删除 M1 临时 UI。
- **关键设计**：快照字段以 `agent-protocol.md` §10.1 为准；快照 + revision/seq；快照含 Go 派生的链视图与 `lockedNodeIds`；runStore 按 runId；控制台「对话/待处理」；删除 PromptDock、SessionPage、ResumeFailureCard/新开会话/取消恢复；工程切换 dialog；运行在途禁止切换工程；`SelectNode` 本地更新，不递增 revision、不重拉快照。
- **文件**：`frontend/src/stores/{workspaceStore,runStore}.ts`、`api/*`、`hooks/*`、`ui/{WorkspacePage,ConsoleTabs,RunList,ProjectSwitcher}.tsx`。
- **测试**：store 缺口/乱序、运行分组、工程切换、无输入框、旧事件不污染、选中不触发快照重拉。
- **风险**：Wails 事件 payload 形状；绑定重新生成；旧组件删除遗漏。

### M2-09 画布与节点交互

- **状态**：已完成（用户于 2026-11-04 确认验收）；执行结果与 AC 覆盖见 [M2-09 证据](evidence/m2-09.md)。

- **需求 / 验收**：F-02、F-03；AC-02、AC-M2-1/6、AC-M2-11。
- **依赖**：M2-02、M2-08。
- **目标**：React Flow、节点卡、平移缩放、位置持久化、topic 多候选端口、自由改挂确认、路径高亮、草稿、运行锁视觉。
- **关键设计**：topic 候选行右侧端口；prompt 底部输出/顶部输入；拖到已有父弹确认并显示子树影响，同一 topic 换候选同样确认；失败不本地加边；运行路径节点禁用编辑；锁定节点禁用执行；QuickBar 叶子链由 M2-12 接入。
- **文件**：`frontend/src/canvas/{WorkspaceCanvas,TopicNode,PromptNode,flowModel,graphApi}.ts(x)`、`styles/workspace.css`、`@xyflow/react`。
- **实现**：AddNode/Connect/MoveNode/UpdateNodeParams/SelectNode 按发起工程 ID 校验；图写命令返回工程 ID、持久化 revision 与目标节点 ID 的 receipt。前端等待 receipt 对应的权威快照（工程、revision、节点 ID）后才结束同步；命令提交成功但快照失败时显示已提交状态，仅允许重试快照读取，避免重复写入。新节点确定性避让并在同步后聚焦；工程切换清除旧工程 pending 状态；同工程快照不覆盖本地选择。画布尺寸链使用明确的列 flex 高度与绝对填充 React Flow 根；键盘选择/位置变化与鼠标操作共用持久化路径。改挂确认在提交期间禁用取消、背景关闭与 Escape。
- **提前交付**：M2-09 已显示快照 `lockedNodeIds` 路径锁，并阻止该路径端口连接和新增子节点；锁定内容节点仍可执行其旧基线允许的拓扑操作。旧 M2-11 的 prompt 直接执行入口已被 M2-11～13 本地脚本目标取代，尚未实现。
- **测试**：端口连接/改挂确认、同父换候选与跨树提示/取消/失败、非法目标、位置防抖/回退/工程切换、缩放/重置/聚焦、键盘连接、路径高亮与锁状态；新增工程范围/回执/快照一致性及同步失败只重试读取；新增不 mock `@xyflow/react` 的 DOM 集成测试，验证节点 wrapper 测量、候选 Handle、SVG edge 和键盘选择。HTML/jsdom 测试不替代 WebView2 GUI 尺寸与像素验收；详情见 [证据](evidence/m2-09.md)。
- **状态边界**：用户在隔离工程的生产 Wails 窗口中手工新增 topic 与草稿 prompt，确认工程 revision 12→13→14、节点 4→5→6、界面显示「已同步」且 6 个节点可见不重叠；同步失败重试、较小窗口、鼠标/键盘操作仍由自动测试覆盖，未在真实窗口逐项手工复测。保留截图与测量于 [M2-09 证据](evidence/m2-09.md)；不启动真实 Claude CLI。
- **剩余风险**：已存节点坐标不会自动迁移；同步失败重试和窄窗口布局仍需在后续 GUI 验收中补查。

### M2-10 选题流程

- **需求 / 验收**：F-04；AC-04、AC-M2-12。
- **依赖**：M2-05、M2-06、M2-09。
- **目标**：topic 参数、执行、候选展示、应用、候选端口连线、锁定；无「选定」按钮。
- **关键设计**：想法/方向/候选数 ≥3；运行 `topics`；待应用；应用后候选列表 + 每候选端口；连接端口即选定；topic 锁定内容与执行（不可重跑）；同候选可多分支。
- **文件**：`TopicNode.tsx`、pending UI、workflow handler。
- **测试**：候选 ≥3、无效 Schema、应用/丢弃、端口连接、锁定后新增分支、不同候选上下文。
- **风险**：候选端口与 React Flow handle 同步；应用前禁止连线。
- **子项状态**：按[工作台至选题闭环计划](m2-topic-closure-plan.md)独立验收；M2-10A、M2-10B 已验收（[10A 证据](evidence/m2-10a.md)，[10B 证据](evidence/m2-10b.md)）；M2-10C 已验收（用户确认页面验收；[10C 证据](evidence/m2-10c.md)）；M2-10 原任务完成。
- **M2-10A 已交付边界**：选题参数编辑、防抖保存与运行前 flush、一次性运行启动和结构化启动错误；本子项不读取候选正文、不预览/应用/丢弃 pending、不创建真实候选端口，也不做真实 CLI 或 GUI 验收。

### M2-11A 需求、契约与交互设计

- **状态**：已完成（用户于 2026-11-06 验收；仅需求、契约与交互设计完成，产品功能未实现）。
- **需求 / 验收**：F-04/F-05/F-07/F-15/F-17/本地 F-18；AC-T-01～03、05～10、18～19。
- **目标**：统一本地脚本范围、节点/产物映射、连接矩阵、输出版本语义、工程版本策略、错误码与交互原型。
- **产物**：PRD、路线图、ADR-027、架构、Agent 协议、验收、前端设计、CLAUDE/AGENTS/README；[本地流程演示](../design/m2-11-13-local-flow-demo.html)；[原型浏览器自检证据](evidence/m2-11a.md)。详细设计见[目标闭环计划](m2-target-closure-plan.md) § M2-11A。
- **门禁**：文档一致性检查、原型浏览器实测与视口记录；无 Go/前端产品实现。评审通过前不得进入 M2-11B。

### M2-11B 工程版本与图领域模型

- **状态**：已完成（用户于 2026-11-06 验收；仅工程格式与图领域模型，不代表本地脚本闭环完成）。
- **需求 / 验收**：F-03/F-04/F-15；AC-T-01 的候选来源/类型校验部分、AC-T-10 的静态单输出部分、AC-T-19 的连接矩阵。
- **依赖**：M2-11A 用户评审已通过。
- **目标**：`project.json` v2、candidate/agent/script 节点与来源引用、连接矩阵、草稿、改挂确认、派生链和 M1 迁移新格式；v1 明确不兼容。
- **实现边界**：严格 `Graph.Validate()` 校验 v2 持久化与图写命令；`Graph.ValidateForest()` 仅供旧候选只读查询及旧上下文兼容回归使用。Candidate 原子生成留给 M2-11D；M2-11C 负责在途输出绑定保护和发送快照。
- **阶段限制**：M2-11B 验收时仅开放 topic 运行入口；prompt、agent、script 当时均返回 `RUN_ENTRY_UNAVAILABLE`。M2-11C 现允许 Agent 运行并冻结发送快照；Candidate 原子生成留给 M2-11D。旧 `ParentCandidate` 仅作为过渡 DTO 保留，v2 校验拒绝非空值且序列化省略该字段。
- **测试**：节点/连接矩阵、候选来源身份、静态单输出、单父/无环、改挂、跨树 steps 回滚、v1 不兼容但有效工程可用、M1 迁移和重启；自动化门禁结果见[自检证据](evidence/m2-11b.md)。
- **特殊操作**：仅按目标闭环计划，在应用关闭并复核后一次性清理用户指定的 `bin` 旧工程；不删除其他数据、不实现启动时自动删除。本次指定工程目录与配置文件不存在，因此未执行清理。

### M2-11C 发送快照、上下文与输出契约

- **状态**：已通过用户验收（2026-11-07）；可按依赖进入 M2-11D。
- **需求 / 验收**：F-15/F-17/F-18；AC-T-02、AC-T-03、AC-T-08～10、AC-T-16。
- **依赖**：M2-11B。
- **目标**：运行绑定真实 Agent 路径、candidate 来源、完整提示词/约束和指定/未指定 script 输出契约；改挂后重建下一次上下文；应用查询按运行快照授权；本地文件工具边界不放宽。
- **测试**：不同候选隔离、无旧 round 的 prompt、预算降级、改挂前后快照、运行时图/产物/输出契约变化、活动租约、路径锁/兄弟分支、文件权限与 BUSY。

### M2-11D 受限 reply、原子提交与待确认结果

- **状态**：已于 2026-11-07 通过用户验收；M2-12A 尚未开始。
- **需求 / 验收**：F-04/F-07/F-15；AC-T-01、AC-T-03、AC-T-05～07、AC-T-10、AC-T-18（组件级测试，不替代 M2-13 完整验收）。
- **依赖**：M2-11B、M2-11C。
- **实现**：`SubmitRunReply` 按发送快照与活动运行锁校验 kind/绑定；topics 原子写 artifact、Round、Candidate 与幂等回执；Agent Script 无绑定/空绑定自动写入，已有版本转为带基准的确认结果；确认不可用 `force` 绕过版本冲突。MCP 保留原始 payload 与活动运行租约，仅将 pending/invalid 登记到运行记录。
- **恢复**：复用 `pending.txn.json`；提交 revision 是恢复分界，已提交工程恢复 CAS 与 pending 索引前核对 Candidate/Script 图关联，基准 revision 回滚意图；支持安全未引用 CAS。
- **测试**：topics/Script 自动结果、空输出与已有版本确认/取消、版本冲突、request/no-request 幂等、并发冲突、过期/变化快照拒绝、提交各写入阶段故障与恢复、运行 pending 索引失败重试及事件去重、MCP 租约/原文/待处理通知；证据见 [M2-11D 自检证据](evidence/m2-11d.md)。真实 CLI、GUI 与 M2-13 跨层黄金链路未执行。

### M2-12A 新模型工作台与脚本闭环

- **状态**：未开始。
- **需求 / 验收**：F-04/F-05/F-15；AC-T-01～03、AC-T-05、AC-T-19。
- **依赖**：M2-11B～D。
- **目标**：按权威快照渲染 Candidate/Prompt/Agent/Script；提示词保存任务，Agent 启动本地运行；覆盖无绑定自动建脚本和指定空节点接收。
- **测试**：黄金流程、非法连接、运行锁/BUSY、保存失败、迟到事件、键盘可达性与既有工程作用域/动态 Handle 回归；浏览器实测真实页面并记录截图、viewport。

### M2-12B 已有输出确认、叶子链与运行状态

- **状态**：未开始。
- **需求 / 验收**：F-03/F-07/F-17；AC-T-08～10、AC-T-18～19。
- **依赖**：M2-12A。
- **目标**：显示已有 Script 版本来源与待确认结果；确认仅在冻结版本匹配时追加，取消不修改旧版本；QuickBar 消费 Go 快照派生链。
- **测试**：确认/冲突/取消、无绑定自动提交、多个叶子链/共享前缀、选择不重拉快照、工程切换/事件乱序及动态 Handle 回归。

### M2-13 完整集成、验收与证据

- **状态**：未开始。
- **需求 / 验收**：M2-11～12 全部目标需求；AC-T-01～03、AC-T-05～10、AC-T-16、AC-T-18～19，以及既有基线回归。
- **依赖**：M2-11A～12B 均经用户评审。
- **目标**：临时工程与可控 DriverProvider 覆盖选题候选、分支、prompt/agent、自动脚本节点、指定空/已有输出、改挂新旧快照、非法 reply 和故障恢复；回归 M1 导入、工程切换、绿色目录与全局 BUSY。
- **门禁**：浏览器真实操作黄金路径与错误态；Wails/WebView2 专有行为和干净 Windows 冒烟单列。真实 CLI/外部访问可能产生费用，执行前另行取得明确同意；未获同意不伪造 live 结果。视频 AC-T-04、云端 AC-T-11～15/17 明确列未覆盖。
- **出口**：记录 `docs/dev/evidence/m2-11*.md`、`m2-12*.md`、`m2.md`，用户验收后再更新任务状态；0.3.0 与 S6 收口由用户另行确认。

---

## 4. 任务级门禁

```bash
gofmt -w main.go internal
go vet . ./internal/...
go test . ./internal/...
wails3 generate bindings -ts -i -clean=true
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run check:tokens
npm --prefix frontend run test
npm --prefix frontend run build
wails3 build
```

- 不用 `go test ./...`。
- 前端 bundle 首屏 ≤300KB gzip；引入 React Flow 后记录实际值。
- live CLI 测试 `AUDIOUI_LIVE_CLI=1` 不进门禁，但 M2-07/M2-13 需记录结果。
- 每个任务结束运行 `git status` 确认没有生成物、`data/`、`logs/`、密钥进入提交范围。

---

## 5. 常见返工点（新对话先读）

> 以下旧模型注意事项适用于 M2-00～10 及其回归。M2-11～13 目标模型以新任务卡和[目标闭环计划](m2-target-closure-plan.md)为准；候选节点不再使用 `parentCandidate`，新目标的连接、锁定和输出语义不可从旧清单推断。

1. 不要把链重新做成实体或持久化 `chains` 数组；链是派生路径。
2. 不要恢复 `selectedTopic`/`SelectTopic`；候选端口连接即选择。
3. 不要恢复一链一会话、resume、`ResumeFailureCard`、PromptDock。
4. `parentCandidate` 只在父为 topic 时有意义；改挂到 prompt 时清空。
5. 运行锁按「topic 根 → 运行节点」路径 + 共享祖先；不是整棵子树，也不是全工程。
6. 上下文只带路径祖先与所选候选；不要带目标历史、兄弟、子节点、其他候选。
7. token 预算不是字节；估算保守、运行后记录实际 usage。
8. 所有写操作走 `service/workflow`；Agent 写图工具不开放。
9. 文档、验收、CHANGELOG 与代码同次完成；不要「下个任务再补」。
10. 不执行 commit/push，除非用户明确要求。
11. 跨树改挂必须迁移子树 steps、检查源/目标树运行锁，失败整体回滚（`WORKSPACE_MOVE_FAILED`）。
12. 不要把锁定节点做成可重跑；M2 一节点一轮，迭代新增下游节点或分支。
13. 不要让 `SelectNode`/快捷栏切换递增 revision 或触发全量快照重拉。
14. 派生链由 Go 快照返回；前端不要自行实现链派生规则。
15. 同一父换候选也要确认，不要当成普通连接或幂等操作。

---

## 6. 状态表

| 任务 | 状态 | 证据 |
|---|---|---|
| M2-00 设计收口 | 已完成 | ADR/文档/原型；用户确认开工 M2-01 |
| M2-01 工程存储与迁移 | 已完成 | `internal/infra/persistence`、`internal/service/project`、`internal/handler/project_test.go`、绑定生成；用户确认开始 M2-02 |
| M2-02 图命令层/改挂/运行锁 | 已完成 | 用户确认 AC-M2-1/6/11 通过；`internal/domain/entity/graph.go`、`internal/service/workflow`、`internal/infra/workspace/mover.go`、`internal/handler/workflow_test.go`、全量 Go/前端/Wails 门禁通过 |
| M2-03 工作区与写作用域 | 已完成 | [验收证据](evidence/m2-03.md)；用户确认通过（2026-11-02） |
| M2-04 上下文构建器 | 已完成 | [验收证据](evidence/m2-04.md)；用户确认通过（2026-11-02） |
| M2-05 Pending 后端 | 已完成 | [自检与验收证据](evidence/m2-05.md)；用户已确认验收通过（2026-11-02） |
| M2-06 MCP 只读与 reply | 已完成 | [自检与验收证据](evidence/m2-06.md)；用户于 2026-11-03 确认验收 |
| M2-07 运行管理 | 已完成 | [自检与验收证据](evidence/m2-07.md)；用户于 2026-11-03 确认验收 |
| M2-08 前端工作台骨架 | 已完成（08A、08B 均已验收） | [M2-08A 证据](evidence/m2-08a.md)；[M2-08B 自检证据](evidence/m2-08b.md) |
| M2-09 画布与节点交互 | 已完成 | [自检证据](evidence/m2-09.md)；用户于 2026-11-04 通过真实窗口手工新增确认验收 |
| M2-10 选题流程 | 已完成（M2-10A、10B、10C 均已验收） | [M2-10A 证据](evidence/m2-10a.md)、[M2-10B 证据](evidence/m2-10b.md)、[M2-10C 证据](evidence/m2-10c.md) |
| M2-11A 需求、契约与交互设计 | 已完成（设计阶段，用户于 2026-11-06 验收） | [自检证据](evidence/m2-11a.md)；PRD/ADR/架构/协议/验收/前端设计与本地演示 |
| M2-11B 工程版本与图领域模型 | 已完成（用户于 2026-11-06 验收） | [M2-11B 自检证据](evidence/m2-11b.md) |
| M2-11C 发送快照、上下文与输出契约 | 已完成（用户于 2026-11-07 验收） | [自检与验收证据](evidence/m2-11c.md) |
| M2-11D 受限 reply 与原子提交 | 已完成（用户于 2026-11-07 验收） | [M2-11D 自检证据](evidence/m2-11d.md) |
| M2-12A 新模型工作台与脚本闭环 | 未开始 | — |
| M2-12B 已有输出确认与叶子链 | 未开始 | — |
| M2-13 集成与验收证据 | 未开始 | — |

状态词表沿用 [路线图第 2.3 节](roadmap.md#23-状态词表)：未开始 / 进行中 / 待验收 / 已完成 / 阻塞 / 暂缓。只有用户可以把「待验收」改为「已完成」。
