# M2-11～13：切换新节点模型并完成本地脚本闭环

| 项 | 内容 |
|---|---|
| 状态 | M2-11A 已验收（2026-11-06）；M2-11B 已验收（2026-11-06）；M2-11C 已验收（2026-11-07）；M2-11D～13 未开始 |
| 范围 | M2-11～13 后续实现规划；本地执行与 `script` 输出闭环 |
| 关联 | [M2 逐任务执行手册](m2-workplan.md)、[路线图](roadmap.md)、[PRD 新目标](../prd.md)、[ADR-027/028](../decisions.md)、[新目标验收标准](../acceptance.md) |

## 背景

M2-00～10 已按旧模型验收；M2-11A 设计已验收，M2-11B 工程格式与图领域模型已于 2026-11-06 通过用户验收；M2-11C 已于 2026-11-07 验收，M2-11D～13 尚未开始。现有前端仍保留旧工作台；旧阶段的验收事实保留，不声称旧证据覆盖新目标。

本文仍是 M2-11～13 的阶段计划；M2-11A 需求、契约与交互设计已由用户于 2026-11-06 验收，仅代表设计阶段通过。M2-11B 工程格式与图领域模型已于 2026-11-06 通过用户验收；M2-11C 已于 2026-11-07 通过用户验收；M2-11D 及后续子项仍须分别按任务门禁执行；本计划未授权跳过逐项评审。

## 用户已确定的范围与规则

- 只做本地执行和 `script` 输出闭环：使用已有 `ValidateStructuredPayload` 的脚本 Schema；视频节点/产物具体形式、渲染导出与云端执行不因本计划自动纳入。AC-T-04 的视频专项留待后续，不把脚本替代测试记成视频 AC 通过。
- 每个 Agent 至多一个直接输出节点。无指定输出时，合法 `reply` 经应用命令层校验后自动创建脚本节点并关联结果；不得以人工确认作为建节点的前置条件。指定节点为空时自动接收；已有内容时产生待确认结果，用户确认后追加新版本，旧版本和产物保留。
- 选题结果校验通过后由应用生成候选节点；提示词只构建任务，不独立运行；Agent 可在非运行期间再次运行，改挂后从新路径重建上下文。默认沿用现有 topic 生成结果后锁定内容/执行的规则；已发送运行的来源不可因改挂而改变。生成节点后不自动执行下游。
- 现阶段无需把旧版 M2 图迁移为新图。用户明确允许删除 `bin` 下旧工程；规划时曾核对到 `AudioUI/bin/data/projects/p_7660938088c54bee/` 为 v1 工程，且 `AudioUI/bin/data/config.json` 的 `lastProjectId` 指向它。M2-11B 实施前复核时，该工程目录和配置文件均不存在，因此未执行清理。若目标再次出现，执行前先关闭应用并复核，只删除用户指定的旧工程目录；只清空该失效 `lastProjectId`、保留 `contextTokenBudget` 及其他配置；损坏配置不可静默重建。不删除整个 `bin`、其他数据目录、M1 备份、日志、运行时或其他工程。这个一次性清理不得实现为应用自动删用户数据的逻辑。
- 对 `bin` 之外发现的旧版工程一律不删除、不自动迁移：版本不兼容时明确报告，不能把旧图误当新图或阻止新建、打开其他有效工程。原有 M1 `sessions.json` 迁移和备份仍需回归，直接产出新模型的可用工程，不因清理 `bin` 而移除。
- 保持单父、无环、应用命令层写图、产物内容寻址不可变、路径隔离、全局单在途运行和本地绿色版边界。Agent 不获得通用 `create_node` / `connect` / `update_params`；受限 `reply` 的自动建图是应用侧命令，不等于给 Agent 通用写图权限。

## 按依赖拆分的实施计划

### M2-11A：先收口需求、契约和交互设计

1. 将用户选择写入 `docs/prd.md`、`docs/dev/roadmap.md`、`docs/dev/m2-workplan.md`、`docs/decisions.md` ADR-027、`docs/architecture.md`、`docs/agent-protocol.md` 和 `docs/acceptance.md`：RM-018 的本地脚本子范围纳入 M2；RM-019 云端和 RM-020 视频仍待评估。调整 F-04/F-05/F-07/F-15/F-17 及本地 F-18 的适用范围与 AC，保留历史基线和 M2-10 已验收记录；同步 `CLAUDE.md`、`AGENTS.md`、README 中与新行为冲突的规则。
2. 定义新的节点类型/产物类型映射、可连接矩阵、单输出绑定、版本/待确认语义、运行锁、错误码和版本化存储契约。最小必要链路为 `topic → candidate → prompt → agent → script`；允许从候选或脚本结果形成多个后续分支。草稿与改挂必须经过 Go 图命令校验，不能由前端自行决定业务合法性。
3. 在真正讨论/实现新界面前，以现有[目标工作流单文件示意](file:///D:/vscode/AI/bilibili/AudioUI/docs/design/target-agent-flow-demo.html)为基础，在项目中创建新的单文件 HTML 演示 `docs/design/m2-11-13-local-flow-demo.html`，仅用本地模拟数据展示本批的候选、提示词/Agent、脚本输出与确认状态，并给出 `file:///` 链接；视频和云端不作为已实现行为。更新 `docs/frontend-design.md` 并请用户确认设计。此步骤在计划获批准后的实施阶段进行，不在本计划存档时创建。
4. 退出门禁：需求和 AC 有明确映射、协议及原型经用户评审，且每个后续子项有可测试出口；需要产品选择而无法从上述决策推导的情形先停止并询问。

### M2-11B：工程版本与图领域模型

- **状态**：已验收（用户于 2026-11-06 确认；仅工程格式与图领域模型，不代表本地脚本闭环完成）。
- **实现**：`project.json` 升为 v2，新增 Candidate 来源及 Script 版本来源字段与 `candidate`/`agent`/`script` 节点类型；Graph 校验单父森林、连接矩阵、候选来源与最新 `topics` round、Script 版本连续性和 Agent 单直接 Script 输出。图写命令拒绝手工创建或改挂 Candidate、旧 `parentCandidate` 语义及非法连线。
- **工程生命周期**：持久化创建/保存/加载执行严格 v2 校验；v1 工程打开明确返回 `UNSUPPORTED_VERSION`，列表保留带版本/错误摘要的不可用工程，不阻断有效工程列表、新建和打开；M1 导入直接生成 v2。已有不兼容迁移目标会跳过且不覆写。
- **验收映射**：AC-T-01 仅覆盖候选来源身份与图模型校验，合法 `topics` 结果原子生成 Candidate 留给 M2-11D；AC-T-10 覆盖静态单输出约束，在途绑定保护留给 M2-11C；AC-T-19 覆盖 Go 连接矩阵。跨树改挂/steps 回滚及 M1 导入、重启和旧版隔离纳入回归。
- **阶段边界**：M2-11B 验收时仅开放 topic 运行入口；prompt、agent、script 当时均返回 `RUN_ENTRY_UNAVAILABLE`。M2-11C 现已开放 Agent 运行并实现发送快照/上下文/运行授权；Candidate 原子生成留给 M2-11D，新模型工作台留给 M2-12A；不得把 11B 描述成可运行的 Agent/脚本闭环。`ParentCandidate` 只为旧 UI/RPC DTO 过渡保留，v2 校验拒绝非空值且序列化省略；`Graph.ValidateForest()` 仅服务旧上下文兼容构建和旧候选只读查询，持久化和图写命令使用严格 `Graph.Validate()`。
- **清理边界**：实施复核时，用户指定的旧工程目录和 `config.json` 均不存在，未执行一次性清理；未删除其他数据，也未加入启动删除逻辑。
- **证据**：[M2-11B 自检证据](evidence/m2-11b.md)。

### M2-11C：发送快照、上下文和输出契约

- **状态**：已通过用户验收（2026-11-07）；可按依赖进入 M2-11D。

1. 复用 `workflow.BeginRun` 在同一命令锁内验证可执行 Agent、真实路径及目标绑定，扩展 `internal/domain/port/agent.go` 的 `RunScope` / 运行契约，并保护整个祖先路径及指定的下游输出绑定。允许锁外兄弟分支编辑；运行终止、失败或取消按 `run.Manager` 唯一终结路径释放锁和全局 gate。
2. 改造 `internal/service/context/builder.go`：对候选节点读取其所指的单个候选，对提示词读取任务和约束，对脚本结果读取对应不可变版本；不再要求每个祖先都有旧式 applied round。目标任务和约束完整保留，祖先仍按既有预算策略降级；不注入兄弟分支、其他候选或历史对话。改挂后的下一次运行重新构建，既有运行保存原路径、候选/产物版本、提示词、输出目标与契约版本的快照及来源。
3. 修改 `internal/service/workflow/agent_tools.go` 和 `internal/infra/agent/mcp/scoped_tools.go`：运行内图/产物查询以发送时冻结的授权投影和产物版本为准，不再以实时 `Project.Current` 内容冒充快照；版本缺失则显式失败。原 topic 产物引用只能返回当前候选，不能泄漏其他候选；保留 UTF-8 分块和活动运行租约。
4. 按新目标将通用文件读取从应用 MCP 迁至本地 Agent 自身工具；先设计并测试路径/工作区适配，确保祖先文件、当前节点写目录和共享 `runtime/` 的可用性及边界不比当前实现更宽松。复用 `internal/infra/workspace/view.go`、`internal/infra/paths/run_guard.go` 和 `internal/service/permission/`，显式保留 RM-017 的 Bash 协作式门禁残余风险，不宣称操作系统沙箱。不得因删除旧 MCP 工具而悄悄丢失文件访问能力或放宽越界限制。
5. 单测/集成测试：候选 A/B 上下文隔离、无 round 的提示词、完整任务/预算降级、改挂前后两次快照、运行时图/产物版本变化、目标契约变化、活动租约、路径锁/兄弟分支、工作区作用域和 `BUSY`。关联 AC-T-02/03/08/09/10/16 与适配后的 AC-M2-8/10/11/12。

### M2-11D：受限 reply、原子提交和待确认结果

1. 扩展 `internal/service/workflow/pending.go`、`internal/infra/persistence/pending_recovery.go`、`internal/domain/{entity,port}` 和 MCP reply 入口，复用 `ValidateStructuredPayload`、`CanonicalJSON`、内容寻址 artifact 存储及现有事务恢复思路；不复制一套 Schema。`topics` 作为选题运行结果原子生成整批候选节点；`script` 根据运行时冻结的单输出契约处理。
2. 无输出绑定时，一次可恢复事务提交输出节点、父子关系、脚本 artifact、运行来源和去重记录；指定空节点则提交到该节点；指定已有内容则仅建立待确认记录，确认时追加结果版本。运行中的自动提交须持有活动运行租约，并以运行 ID 校验锁的所有权，不能为了写结果而解除或普遍绕过其他命令的路径/目标锁。无效 JSON/Schema、非白名单类型、越权节点、过期运行或不匹配的输出契约不得写图，保留适当原始错误证据。
3. 以运行身份、请求身份及 payload 指纹确保重试/重放幂等；无 request ID 的重复结果也不得重复建节点，同运行不同内容或身份冲突明确拒绝。冻结路径、候选版本、目标绑定、契约或确认时的已有输出版本发生不匹配时必须拒绝，不能用旧 `force` 绕过归属与契约检查。提交后事件/索引写失败可恢复；重启不得出现半个节点、重复节点、错误关联或假报成功；允许安全的未引用 CAS 垃圾而不删除共享产物。
4. 故障注入覆盖写意图、artifact、图提交、pending/运行索引及事件的各个崩溃点；覆盖重复/并发 reply、取消/结束竞态、非法类型、指定/未指定输出、已有结果确认/取消和旧版本保留。关联 AC-T-01/03/05/06/07/09/10；视频特定 AC-T-04 不计通过。

### M2-12A：新模型工作台与脚本闭环

1. 调整 `frontend/src/canvas/{WorkspaceCanvas,flowModel,graphApi,PromptNode,TopicNode}.ts(x)` 并新增候选、Agent、脚本节点组件：从权威快照渲染节点/边；取消旧候选端口模型，提示词保存任务、Agent 启动本地运行；复用选题参数防抖/运行前 flush、工程作用域 receipt、动态 handle 测量与快照同步机制。
2. 修改 `frontend/src/{ui/WorkspacePage.tsx,api/runs.ts,stores, hooks}` 中受影响接线，消费本地 Agent 启动、输出绑定与锁状态；仍保持订阅/快照/seq 恢复、工程切换隔离、运行事件先于 RPC 返回和提交后只重试快照读取。业务合法性、锁和单输出规则以后端为准。
3. 实测选题生成候选 → 提示词 → Agent → 自动创建脚本节点，以及预先指定空脚本节点的路径；覆盖非法连接、运行锁、`BUSY`、保存失败、迟到事件、键盘操作和可访问性。M2-09/M2-10 已有测量、拖动、工程作用域和锁逻辑复用并回归，不重复造轮子。

### M2-12B：已有输出确认、叶子链和运行状态

1. 复用 `frontend/src/ui/{AgentConsole,PendingArtifactCard}.tsx`、`frontend/src/api/pending.ts`：展示已有脚本节点的新结果、原版本来源及确认/丢弃/错误；确认只能对绑定的输出和冻结版本生效，重复提交及提交成功但同步失败按现有回执/重试原则处理。无指定输出的自动建节点不出现多余人工确认。
2. 在 `WorkspacePage` 增加叶子链快捷栏，消费 `WorkspaceSnapshot.chains` / `Graph.LeafChains`，不在前端重写链派生；切换仅通过既有 `SelectNode` 与本地选择，不递增图 revision、不广播、不占运行槽。路径高亮、共享祖先锁、其他分支可编辑和工程切换继续使用权威快照。
3. 前端测试覆盖脚本 pending、已有输出版本保护、确认冲突/取消、无指定输出自动提交、多个叶子链/共享前缀、选择不重拉快照、项目切换/事件乱序与动态 Handle 回归。关联更新后的 AC-03/05/07、AC-M2-5/11 和 AC-T-05/07/10。

### M2-13：完整集成、验收和证据

1. 基于 `internal/app/topic_closure_integration_test.go` 的可控 `DriverProvider` 和临时目录，扩展完整本地链路：有效选题自动生成候选 → 两条分支 → 编辑提示词并运行 Agent → 自动脚本输出；另测指定空节点、已有脚本确认后追加版本、候选 A 改挂到 B 后的新旧快照、非法 reply 和故障恢复。
2. 逐项回归图结构、上下文预算及候选隔离、权限/问题/流式事件、全局 BUSY、重启中断、M1 导入、工程切换、绿色目录，以及在 `bin` 删除旧工程后仍能启动和新建工程。使用临时目录，不读取或改写用户实际工程作为测试数据。
3. 助手用可控本地 fixture 启动真实前端页面，在本机浏览器实际操作黄金路径与错误态，保留截图、viewport 和 AC 对照；不能只用 jsdom、单测或构建代替。Wails/WebView2 专有行为和干净 Windows 冒烟单列；未经明确授权不自动化驱动桌面窗口。若浏览器或 fixture 不可用，记录阻塞和未覆盖项，不宣称通过。
4. 真实 CLI / 外部访问可能产生费用，执行前另行征得用户明确同意；未获同意即记录未执行，不能用 fake 结果代替。记录 `docs/dev/evidence/m2-11*.md`、`m2-12*.md`、`m2.md`，逐条标记新 AC、沿用回归、历史替代、视频/云端暂缓和未覆盖项；同步 CHANGELOG Unreleased。只有用户验收后更新任务完成状态；0.3.0 及 S6 收口由用户另行确认。

## 任务级验证与交付约束

- 每个子项单独实现、测试、更新受影响文档和证据，状态先标“待验收”，用户评审后再进入下一子项；不将分组规划理解成一次性无门禁实现。
- 先跑对应 Go/前端聚焦测试，再从 `AudioUI/` 执行下列门禁；只对变动 Go 文件执行 `gofmt`，不使用会误扫 `node_modules` 的 `go test ./...`。记录首屏 JS gzip（目标 ≤300 KB）、失败与未覆盖项。

```bash
export GOPROXY=off GOSUMDB=off npm_config_offline=true
go vet . ./internal/...
go test . ./internal/...
wails3 generate bindings -ts -i -clean=true
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run check:tokens
npm --prefix frontend run test
npm --prefix frontend run build
wails3 build
git diff --check
git status --short
```

- 使用现有依赖，不未经授权联网下载；工具或缓存缺失时记录阻塞。生成绑定只通过命令生成，不手改。若环境权限阻止操作，不重试相同被拒操作，提供中文手工步骤、工作目录、可复制命令与成功标准。
- M2-11B 已由用户于 2026-11-06 验收；实现和门禁结果见[自检证据](evidence/m2-11b.md)。指定旧工程路径不存在，未执行清理。M2-11C 已由用户于 2026-11-07 验收，实现与自检证据见 [M2-11C 自检证据](evidence/m2-11c.md)；M2-11D～13 未开始。本计划不授权跳过逐项用户评审。
