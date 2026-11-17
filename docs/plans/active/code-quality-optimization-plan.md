# 前后端代码质量与精简函数注释计划

| 项 | 内容 |
|---|---|
| 状态 | **已评审，已列入路线图独立维护专项并设为 M3 启动前置；Q-0～Q-2 已于 2026-11-16 通过用户验收；Q-3 已于 2026-11-17 通过用户验收；Q-4～Q-7 未开始，不改变 M3 的 P0 等级或 M4/M5 既有依赖** |
| 分析基线 | `main` 的 `225c5b6`；分析时工作树无未提交改动 |
| 关联 | [PRD](../../prd.md) F-01～07、F-13、F-15～18；[路线图](../../dev/roadmap.md)；[开发流程](../../dev/process.md)；[ADR-030/031](../../decisions.md)；[验收标准](../../acceptance.md) |
| 本文产物 | 现状诊断、分批优化任务、函数注释规则、回归与验收方案；**本轮不实施代码或测试改动** |

## 1. 目标、范围与证据口径

- **目标**：在不改变已验收行为和协议的前提下，降低画布状态同步、运行快照、结果事务与恢复等关键路径的修改成本；统一代码风格和错误边界；只为不直观的函数契约写简短注释。
- **本轮范围**：`frontend/src/` 的画布、状态、API/桥接和代码样式组织；Go 的 `domain → service → handler/infra → app`；与这些改动直接相关的自动化测试及文档。前端“风格”仅指**代码风格与 CSS 组织**，不调整视觉、交互或文案。
- **不做**：不变更图模型/工程格式、节点功能、Agent/MCP 权限、云端或视频范围；不引入新框架/重型依赖；不做全仓机械格式化、无根据的性能重写或开发期旧工程迁移；不运行真实 Agent/CLI，不联网下载，不由助手启动 GUI/浏览器。历史 M1 导入及运行历史快照并不等于“可随意删除的死代码”。
- **证据等级**：以下文件大小、分支条件和重复代码是静态审阅事实；恢复风险是由代码路径推出的**待回归验证问题**，不是本轮已执行测试的结果。文件较长本身不构成性能缺陷，拆分以职责与测试边界为标准。
- **实施前置**：本计划已列入路线图第 4.7 节的独立维护专项，并作为 M3 启动门禁；M3 仅在 Q-0～Q-7 达到专项出口后启动。每项任务仍需按[开发流程 DoR](../../dev/process.md#12-dor--dod)确认具体范围、依赖、测试与验收口径后再实施。若发现行为/协议/存储变化，先回到相应 PRD/ADR/AC 评审，不把代码重构当作扩大产品范围的授权。

## 2. 代码现状与优先处理点

| 位置与静态证据 | 现状判断 / 优化方向 | 实施边界 |
|---|---|---|
| `internal/infra/persistence/pending_recovery.go:202-243`；`internal/domain/entity/graph.go:411-468` | **正确性风险，优先确认**：v5 图允许一个 Agent 有多个 Script 输出并在运行时选定目标，但恢复校验遍历 Agent 子节点，遇到第二个 Script 就报错。若工程已写入、`pending.txn.json` 尚在，可能无法恢复。现有恢复测试 `pending_recovery_test.go:145-284` 覆盖单一输出及新建输出，没有覆盖“已有其他 Script 的并行输出”。 | 先写故障窗口回归，再仅修正“选定目标”身份校验；仍需拒绝伪造目标、版本冲突和不完整事务。不以删除恢复校验解决。 |
| `internal/service/workflow/pending.go:195-235,906-943,990-1041` | 首次自动提交、历史手动应用、Topic 重跑在 Candidate 节点/来源边、编号、定位与回执构造上有三段相似实现；后续改动容易漏掉某个入口。 | 只抽取相同的纯构造规则；保留首次自动应用与重跑待确认的不同事务和状态语义。 |
| `internal/domain/entity/graph.go:132-260,264-539,755-954`；`internal/domain/entity/run.go:152-270` | 同一文件兼有旧快照分支、v4 DAG 输入冻结、输出目标选择与图校验；阅读时难以区分持久化格式和历史快照版本。 | 优先按“快照构造 / 图校验”拆文件或内部函数，保留公开入口与 JSON 字段；历史运行读取/导入必须单独核对，不能直接删 v1～v3 校验。 |
| `internal/service/context/builder.go:186-255,292-699` | 构建器同时处理预算降级、单路径、DAG、冻结 artifact 与工作区摘要；修改一类输入容易影响另一类。 | 先冻结 golden/hash/预算案例；提炼共享的 artifact 读取和工作区校验，但不得改变上下文 JSON 顺序、指纹、目标参数完整性或逐边版本。 |
| `internal/service/run/manager.go:154-392,808-935` | 启动与终态处理跨越全局门禁、图锁、MCP、会话、持久化和事件顺序；函数过长但顺序本身是业务契约。 | 先用 fake Driver/时钟覆盖启动失败、超时、权限租约、关闭、重复终态、写盘失败，再按阶段提取私有函数；不重排资源释放与终态广播。 |
| `internal/infra/workspace/view.go:60-255,363-624`；`internal/infra/paths/run_guard.go:14-25` | 单路径与 DAG 工作区构造、读取/列举/搜索位于同一大文件；路径和大小上限是安全边界。 | 仅按视图构造与有界读操作拆分，路径归一化、reparse point 拒绝、作用域和限额必须逐项回归。 |
| `internal/handler/workflow.go:38-185`；`internal/service/workflow/service.go:381-519` | RPC 入参裁剪/超时及若干简单图命令的校验—保存—回执模式重复。 | 只合并完全相同的转换/保存惯例，不抽出吞掉差异的通用“万能事务”；`SelectNode` 不增加 revision 或广播，连接/重连仍由 Go 判定。 |
| `internal/domain/entity/graph.go:754`、`internal/service/context/builder.go:270-272`、`internal/service/workflow/locks.go:24-25` | 少数既有注释仍用旧阶段或“将来使用”的表述，可能误导维护者。 | 校正事实而非增添逐行解释；将不变量优先写在权威文档，本地注释只提示与函数相邻的非显然约束。 |
| `frontend/src/canvas/WorkspaceCanvas.tsx:161-1390` | `CanvasBody` 同时管理候选读取、受控节点测量、图命令及 receipt 同步、位置/尺寸保存、运行前 flush 和关系手势；适合按状态所有权拆分。 | 不破坏 `nodeState.ts` 的测量保留、工程身份隔离、已提交只重试读取的逻辑；不得让前端代替 Go 校验图。 |
| `frontend/src/canvas/PromptParameters.tsx:83-215`；`TopicParameters.tsx:120-214` | 两类参数组件都有防抖、版本化草稿、串行保存、flush、IME 和重试，具体校验/运行行为不同。 | 先表格化两者状态差异再提炼共享保存 hook；Topic 已锁参数重跑与 Prompt 不可独立执行保持不变。 |
| `frontend/src/api/workspace.ts:14-19`；`frontend/src/api/runs.ts:44-49`；`frontend/src/canvas/graphApi.ts:16-18,35-43` | Wails 事件数据解包重复；生产 `GraphAPI.getTopicCandidates` 用宽松 fixture 类型，而生成 `TopicCandidate` 的 `index/angle/hook` 为必需字段（`frontend/bindings/audio-ui/internal/domain/entity/models.ts:340-348`）。 | 共享纯边界转换；测试 fixture 与生产 DTO 分型，避免为了测试放宽生产类型；变更绑定须遵循生成门禁。 |
| `frontend/src/hooks/usePendingController.tsx:50-52,174-182`；`frontend/src/ui/ProjectSwitcher.tsx:21-25` | `includes('PARAMS_CHANGED'/'CONTEXT_CHANGED'/'BUSY')` 根据错误文案决定操作分支，容易受包装文本变化影响。 | 优先使用已有结构化错误码；对抛出的 Wails 错误先核实实际格式并锁定测试，确需新增契约则单独评审，不能猜测字符串。 |
| `frontend/src/App.tsx:61-84`；`frontend/src/canvas/ScriptNode.tsx:23-39`；`frontend/src/ui/PendingArtifactCard.tsx:45-63` | 日志轮询可能在慢响应时重叠（静态风险、未复现）；Script 的已存版本与待确认结果各有不同展示解析规则。 | 先构造重叠响应测试再考虑串行化；两处解析先核对各自展示目的，Go 的 Schema 校验始终是权威。 |
| `frontend/src/styles/agent.css:249-265,346-352`；`frontend/src/styles/workspace.css:2340-2349`；`frontend/src/main.tsx:6-7` | 同一 `.agent-console` / `.agent-console-body` 在两个样式文件按加载顺序覆写；`frontend/scripts/check-tokens.mjs:8-25` 只检查颜色字面量。 | 归拢组件样式所有权，保持计算后样式与层级不变；不把可读的所有 px 值都机械改为令牌，也不把颜色检查误称为间距检查。 |

这些是**代码维护性**观察；涉及实际页面呈现的样式搬迁由用户按现有界面验收，本文不设计新 UI。

## 3. 分批任务卡（实施顺序建议；专项状态以路线图第 4.7 节为准）

> Q-0～Q-7 是路线图第 4.7 节独立维护专项的内部任务拆解；Q-0～Q-2 已于 2026-11-16 通过用户验收，Q-3 已于 2026-11-17 通过用户验收，Q-4～Q-7 尚未开始。编号不是 PRD 需求或 AC 编号。实施时逐任务满足 DoR、分别评审 diff/测试/风险；涉及产品行为变化时转需求/缺陷流程。

### Q-0 基线与恢复风险核对（先行）

- **状态**：多 Script 输出事务恢复缺陷已复现并作最小修正；定向、全量 Go 测试与 `go vet` 通过，用户于 2026-11-16 验收通过。证据见[Q-0 核对记录](../../dev/evidence/q0-pending-recovery-risk.md)。

- **目标 / 关联**：锁定当前行为，确认并行 Script 恢复风险；关联 F-03/F-07/F-15、AC-U-31/35/43。
- **影响**：`internal/infra/persistence/pending_recovery{,_test}.go`、`internal/domain/entity/graph{,_test}.go`，并对照 `docs/dev/evidence/` 既有 U16 门禁记录。
- **工作**：用隔离临时工程构造 v5 多 Script 输出：明确选定既有输出、在有旧输出时显式新建并行输出；分别模拟工程写入前/后及 journal 清理前重开；记录精确错误与 revision、artifact、pending 状态。对比只含一个输出的现有测试。若验证成立，将其作为独立正确性修复优先处理；若不成立，记录排除依据并取消修复，不扩张范围。
- **出口**：新增用例能重现（或证明排除）故障窗口；不影响已有图、候选来源与输出绑定的用例。此阶段不以“扫描发现”替代测试结果。

### Q-1 事务恢复与 Candidate 构造收口（依赖 Q-0）

- **状态**：用户于 2026-11-16 验收通过；证据见[Q-1 自检记录](../../dev/evidence/q1-topic-candidate-transaction.md)。

- **目标 / 关联**：降低不同 Topic 提交路径的规则漂移；让 v4 输出恢复只验证冻结目标而不误判合法并行输出；关联 AC-U-31～35、AC-U-43。
- **影响**：`internal/service/workflow/pending.go`、`internal/infra/persistence/pending_recovery.go` 及对应 `*_test.go`。
- **工作**：先补首轮/重跑、相同 artifact 不同 run ID、重复请求、基准版本冲突及故障重启测试；随后提炼 Candidate 构造函数（来源 `round/artifact/index/runId`、边 ID 与避让位置保持一致）；复核 Q-0 已修正的恢复目标校验并保持 `pending.txn.json` 写入顺序和失败关闭策略。
- **出口**：首次自动应用、重跑人工确认、Script 追加/并行输出在正常提交和恢复后的状态一致；可由 journal 重放的 CAS 正常补齐，被篡改快照/来源、无法恢复的 artifact 或写盘失败仍拒绝暴露工程；无新数据格式。

### Q-2 图与上下文的职责拆分（依赖 Q-0，建议在 Q-1 后）

- **状态**：用户于 2026-11-16 验收通过；证据见[Q-2 自检记录](../../dev/evidence/q2-graph-context-responsibilities.md)。

- **目标 / 关联**：使图校验、运行输入冻结、上下文呈现的修改有明确落点；关联 F-03/F-17、AC-U-36～43。
- **影响**：`internal/domain/entity/graph.go`、`run.go`、`internal/service/context/builder.go` 及其现有测试和 golden。
- **工作**：将校验、单路径快照、DAG 快照按职责整理；从上下文构建器提炼输入解析、artifact 加载和预算降级步骤；只复用真正相同的不变量。为同一 Script 两条边绑定不同版本、汇合祖先、无 Topic、多 Prompt 顺序、未选输入隔离和预算超限补边界回归。
- **出口**：快照字段、输入角色/顺序、artifact 引用、hash/golden、明确错误码与预算结果逐项等价；代码分层仍为 service 只依赖 domain，前端不复制执行规则。

### Q-3 运行生命周期与安全边界整理（依赖 Q-0，可与 Q-2 独立评审）

- **状态**：用户于 2026-11-17 验收通过；自检证据见[Q-3 自检记录](../../dev/evidence/q3-run-lifecycle-boundaries.md)。

- **目标 / 关联**：降低资源获取/释放与路径作用域代码的维护风险；关联 F-06/F-13/F-18、AC-M2-9/11、AC-U-42。
- **影响**：`internal/service/run/manager.go`、`internal/infra/workspace/view.go`、`internal/infra/paths/run_guard.go`、`internal/infra/agent/mcp/scoped_tools.go` 及对应测试。
- **工作**：把启动准备、运行中事件处理和终态清理分成可测试的私有步骤；工作区读视图按构造/查询职责整理；保证 `RunScope` 一直基于冻结快照。通过 fault injection 检查 MCP 启动/停止、配置清理、持久化失败、租约等待和路径越界；任何“共享校验”不得放宽原有根路径与 reparse point 规则。
- **出口**：同一节点单在途、门禁与运行锁最终释放；运行事件序号/未决请求不丢；未选输入及其他分支仍不可读，目标工作区是唯一写位置，`runtime/` 只读。

### Q-4 RPC 边界及代码风格整治（依赖 Q-1～Q-3）

- **目标 / 关联**：降低重复转换代码和跨层错误处理的不一致；关联 F-01/F-03/F-06。
- **影响**：`internal/handler/workflow.go`、`run.go`、`internal/service/workflow/service.go`、`frontend/src/api/`、绑定生成物。
- **工作**：核对 `projectId`、edge 身份、错误码、超时与回执契约；仅对相同入参转换或简单无副作用写命令提取 helper。Go 代码保持 `gofmt`、按层命名清晰；TS 保持 strict、显式 `unknown` 错误收窄。若 RPC 方法/DTO 真需改变，应另做契约评审、重新生成绑定并配套双端测试；本计划默认**不改**协议。
- **出口**：调用成功/失败时的返回值、revision、错误码、工程切换保护不变；没有新增依赖或静默回退。

### Q-5 前端画布与草稿保存职责拆分（可与 Q-0 并行；先锁定前端回归基线）

- **目标 / 关联**：使受控 React Flow 节点、草稿保存、图命令与运行前 flush 分别可测试；关联 F-02/F-03/F-05、AC-02、AC-U-55～65。
- **影响**：`frontend/src/canvas/WorkspaceCanvas.tsx:161-1390`、`nodeState.ts`、`flowModel.ts`、`PromptParameters.tsx:83-215`、`TopicParameters.tsx:120-214` 及其测试。
- **工作**：以 `WorkspaceCanvas.test.tsx`、`WorkspaceCanvas.realFlow.test.tsx` 锁定快照切换、React Flow 测量、候选加载与位置/尺寸回执；先提候选查询，再提布局持久化，最后才拆关系手势/浮层和图命令同步，避免只按行数切文件。对照 `PromptParameters.test.tsx`、`TopicParameters.test.tsx` 中两种草稿状态机；若重试、IME 与版本队列语义确实相同，再提取共享保存 hook，参数校验、Topic 首次保存/重跑和 Prompt 任务规则留在各自组件。`startNodeRun` 当前只 flush 已选**直接输入**中的 Prompt（`WorkspaceCanvas.tsx:770-786`），祖先草稿是否应额外纳入须独立核对后端契约，不借重构扩大屏障。
- **出口**：相同工程/节点身份才复用测量，变工程或删节点不复用；保存成功但同步失败只重拉快照、不重复提交；运行前当前直接输入 Prompt 的 flush、项目身份与输入锁范围保持一致。若发现现有祖先草稿缺口，另列缺陷/需求评审；前端真实页面行为仍须由用户验收。

### Q-6a 前端 API 类型与错误边界（可与 Q-0、Q-5 并行）

- **目标 / 关联**：让生产 DTO 和事件/错误边界表达真实契约，不再为测试放宽生产类型；关联 F-01/F-03/F-07、AC-U-35/58。
- **影响**：`frontend/src/api/workspace.ts`、`runs.ts`、`frontend/src/canvas/graphApi.ts`、`frontend/src/hooks/usePendingController.tsx`、`frontend/src/ui/ProjectSwitcher.tsx` 及对应测试。
- **工作**：抽取重复的 `messageOf` / Wails 事件解包辅助逻辑，保留各调用点的中文前缀，并为事件必需字段做轻量边界检查；`GraphAPI.getTopicCandidates` 的生产返回值使用生成的 `TopicCandidate`，宽松 fixture 类型留在测试/开发适配层，不手改绑定文件。先确认 Wails 抛错的稳定 code 来源，再考虑替换 `includes('BUSY'/'CONTEXT_CHANGED')` 等文案分支；若需变更 RPC 形状，先走契约评审，不猜测或复制后端规则。
- **出口**：生产 DTO 必需字段不能被 fixture 类型放宽，缺字段/事件错误有专项测试；有稳定 code 的分支使用 code，若没有则保持既有行为并登记契约待评审；无未经评审的协议改动。

### Q-6b 前端桥接与异步请求协调（依赖 Q-5 和 Q-6a）

- **目标 / 关联**：把现有工程快照、run 事件和请求状态的协调职责变得可单独测试；关联 F-06/F-07、AC-U-35/38/58。
- **影响**：`frontend/src/hooks/workspaceBridge.ts`、`frontend/src/stores/workspaceStore.ts`、`runStore.ts`、`frontend/src/hooks/usePendingController.tsx`、`frontend/src/ui/AgentConsole.tsx`、`frontend/src/App.tsx` 及测试。
- **工作**：以 `workspaceBridge.test.ts`、`runStore.test.ts` 的工程身份 + revision、runId + seq、事件缺口、迟到响应和 receipt 测试为基线，才提炼独立的事件缓冲/运行快照协调；保留现有 workspace/run Zustand 分层，不换库、不重写已被测试覆盖的 store。现缺 `App.test.tsx` 和独立 pending controller 测试：先用可控延迟 Promise 验证 `App.tsx:61-84` 日志轮询是否重叠/过期，并覆盖 pending 详情迟到、force 冲突和“提交成功、刷新失败”；确认失败模式后才加单飞或最新请求保护。核对 Script 已存版本与待确认卡片的展示解析语义，一致时才共用解析函数；Go 的 Schema 校验仍是权威。
- **出口**：事件缺口通过权威快照恢复，迟到响应不污染新工程；权限/问题/待确认结果在面板收起后仍可处理，重复事件不重复执行命令；日志慢请求不会让旧结果覆盖新结果。未经证明无效的现有状态保护与代次检查一律保留。

### Q-7 函数注释与样式代码收口（可与各任务同步，不单独批量刷文件）

- **目标**：让非显然的输入/输出、副作用及不变量在函数附近可读，同时减少过时和冗余文字。
- **影响**：上述 Go/TS 文件，`frontend/src/styles/agent.css`、`workspace.css`、`tokens.css`、`frontend/src/main.tsx` 与 `frontend/scripts/check-tokens.mjs`；**不改 CSS 呈现值或组件交互**。
- **工作**：按第 4 节规则随每张任务卡清理对应注释。对照现有 CSS 加载顺序与计算值，把 `.agent-console` 样式尽量归到组件所属文件，保留现有层级与滚动；仅把确有复用语义、与现有令牌一致的间距映射到 `tokens.css`，不触及节点尺寸/命中范围等功能几何值。现有 `check:tokens` 只扫描颜色，不在本任务未经评审时升级为“所有 px 禁用”。避免连带重命名 RPC 字段、强行拆分稳定组件、添加格式化依赖或全仓扫格式。
- **出口**：新增注释只描述代码无法直接表达的约束；误导性历史注释已纠正，必要的 React Flow/构建限制注释保留；同一 CSS 计算值/可用性保持，TypeScript 类型检查、lint 和颜色令牌检查通过；如触及界面表现，页面仍由用户验收。

## 4. 函数注释的最小规范

| 何时写 | 写什么（尽量一句话） | 不写什么 |
|---|---|---|
| Go 导出函数跨越事务/并发/授权边界 | 关键调用前提、不可变范围、失败后的状态或锁顺序；GoDoc 以符号名起首 | 重复函数名、参数名、流程步骤或项目阶段历史 |
| Go 私有函数与 TS helper | 只有隐藏的身份、版本、顺序、可重试性等约束不易从类型和代码读出时才写 | `// 获取节点`、`// 调用 API`、`// 返回结果` 这类逐行旁白 |
| React hook / 异步协调 | 工程身份/revision、run seq、取消与迟到响应的边界 | 把 Go 领域规则改写为前端“真相”，或用长注释掩盖过大的函数 |
| 类型/DTO/协议 | 非直观字段语义、空值与历史版本兼容边界 | 重复 JSON 字段拼写；在多处复制 PRD/ADR |

**拟调整示例（只是注释规范，不是本轮代码改动）**：`Graph.Validate` 当前注释提“typed node connection”，但 v5 允许任意类型的普通无环边（`graph.go:754-758`）；应说明 v5 的候选来源与版本约束，而非重述旧连接矩阵。`Builder.InputHashes` 仍写“future pending-application check”（`builder.go:270-272`），应按现有调用更新。`AcquireRunLock` 的 M2-07 历史说明（`locks.go:24-25`）应明确其单路径接口边界，避免误解为所有 DAG 运行入口。示意写法：`// BeginRunWithOptions holds the graph-command lock until the snapshot and run lock share one revision.`；实际措辞应与测试和当前契约核对后再落代码。

**保留的注释示例**：`frontend/src/main.tsx:12` 解释开发 fixture 不进入生产；`frontend/src/canvas/nodeState.ts:19-23` 与 `frontend/src/canvas/useMeasuredNodeHandles.ts:72-73` 解释 React Flow 测量/临时状态约束。这些不属于需要机械清理的“注释过多”。

**长度控制**：优先自解释命名和拆出小函数；普通函数零注释，高风险入口 1～2 行，只有必须说明的限制或兼容协议才延长；不为代码覆盖率、行数或“每个函数都要注释”增加噪音。

## 5. 验证矩阵与交付门禁

| 层 | 建议先补的回归 | 通过条件 |
|---|---|---|
| 领域/事务 | v5 多输出与选定目标、Script 按边固定版本、Topic 不同 run 同 artifact 批次、写入前后重启恢复、非法快照和幂等 | 正常结果可恢复；冲突拒绝，project/pending/CAS/journal 状态一致；旧产物不覆盖 |
| 运行/上下文/授权 | 无 Topic/DAG 汇合、多 Prompt 顺序、未选输入隔离、超预算降级、失败清理与事件序号 | frozen scope/hash 与 golden 一致；门禁、权限、工作区边界不放宽；无真实 CLI 调用 |
| 前端状态 | React Flow 测量与拖动、节点位置/尺寸提交、Topic/Prompt IME 与防抖、选定 Prompt flush、事件缺口与跨工程迟到回执 | 权威快照最终一致，无重复写；错误/请求仍可定位；真实页面行为留用户验收 |
| 前端边界/样式 | generated DTO 与测试 fixture 分型、Wails 事件解包、错误对象/字符串变体、日志轮询延迟、Script 两类预览、CSS 组件覆写 | 生产类型不被放宽；有稳定 code 时错误分支使用 code，无 code 时保持行为并标记契约待评审；旧轮询结果不覆盖新数据；CSS 计算值与可访问性由用户检查 |
| 代码风格与文档 | `gofmt`、`go vet`、TS strict、ESLint、颜色令牌脚本、变更文档 | 不引入裸颜色/重型库；注释简短准确；无与本任务无关的格式 diff |

实施阶段在 `AudioUI/` 工作目录，使用既有本地依赖并禁止自动联网：

```bash
GOPROXY=off GOSUMDB=off go vet . ./internal/...
GOPROXY=off GOSUMDB=off AUDIOUI_LIVE_CLI=0 go test . ./internal/...
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run check:tokens
npm --prefix frontend run test
npm --prefix frontend run build
```

涉及 Go RPC/DTO 时另运行仓库规范的 `wails3 generate bindings -ts -i -clean=true` 并核对生成差异；如需交用户实际界面验收，先按项目要求在**当前工作树离线重建** exe，提供路径，助手不启动或操控 Wails/浏览器。`go test -race` 仅在本机 Go/CGO 工具链可用且确认不会拉依赖时作为并发专项补充；不得以构建通过代替用户页面验收。上述门禁是未来实施要求，**本轮只读分析并写本文，尚未执行构建、测试或手工验收**。

## 6. 风险、停止点与文档同步

1. **恢复比“减少重复”优先**：Q-0 未确定前不对 `pending.go`/恢复代码做大范围抽象；事务任何一步改变都可能影响断电重启的可恢复性。
2. **不把历史代码当死代码清除**：当前仅打开 v5 开发工程（`internal/infra/persistence/project.go:209-219`），但 M1 导入仍在 `internal/app/project.go:20-39`，运行历史快照另有 v1～v3 逻辑；删除须逐调用点和历史运行读取确认。开发期工程不做迁移，也不自动删除用户已有文件。
3. **拆分≠改行为**：若改动影响 Go 端口、协议、持久化字段、权限范围或 UI 行为，先更新 ADR/架构/协议/前端设计与 AC，经用户确认再开工；本计划不代替该决策。若专项回归发现新缺陷，按开发流程单独登记、分级并确认优先级，不夹带在风格整理中。
4. **实施后的同步**：每张任务卡的代码、测试、相关文档和 `CHANGELOG.md` 同次完成；保留自检证据及未覆盖项，提交与发布仍由用户另行决定。本专项已进行，Q-0～Q-2 已于 2026-11-16 通过用户验收；Q-3 已于 2026-11-17 通过用户验收；Q-4～Q-7 尚未开始。M3 保持 P0，M4/M5 的优先级和既有依赖不变。路线图状态按任务门禁更新，不改动既有验收结论。
