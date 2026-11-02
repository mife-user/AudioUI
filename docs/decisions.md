# 架构决策记录（ADR）

## ADR-001 桌面框架选 Wails v3

- **背景**：需要 Go 后端、可视化界面、绿色单 exe。
- **决策**：Wails v3，系统 WebView2。
- **代价**：v3 较新，API 可能变动。
- **对策**：锁版本；壳层隔离在 `internal/app` 与 `handler`。

## ADR-002 节点画布选 React Flow

- **背景**：需要 ComfyUI 风格画布，节点内嵌预览。
- **决策**：`@xyflow/react`。
- **否决**：LiteGraph 为 Canvas 绘制，嵌视频与样式定制困难。

## ADR-003 编码选 WebCodecs + mediabunny

- **背景**：纯前端导出 MP4，确定性、零后端。
- **决策**：Canvas2D 逐帧 → VideoEncoder → mediabunny 封装。
- **代价**：依赖 WebView2 的 H.264 支持。
- **M4 决策（2026-11-02）**：MP4 是必需交付格式；H.264 不可用时明确阻止 MP4 导出，不以 WebM 作为成功回退。若未来引入其他 H.264 编码后端，须另行评估依赖并更新 ADR。

## ADR-004 Go 主导领域，前端作为渲染适配器

- **决策**：Go 负责图、校验、执行计划、缓存、状态机；前端实现 `RendererPort`。
- **代价**：跨进程一次 RPC。
- **对策**：单任务串行、超时、取消。

## ADR-005 不使用 go-zero / goctl

- **背景**：曾考虑用 goctl 或 go-zero 生成。
- **决策**：不使用其运行时，手工依赖注入。
- **理由**：微服务栈对单进程桌面应用是负担；生成的 handler 面向 HTTP，与 Wails 绑定不匹配。
- **保留**：DDD 分层、接口驱动、目录规范。

## ADR-006 绿色版路径策略

- **决策**：`os.Executable()` 定位根目录；WebView2 用户数据目录重定向；目录不可写则报错。
- **理由**：Wails/WebView2 默认写 AppData，必须显式覆盖。
- **代价**：不能放在只读目录。

## ADR-007 场景 DSL 而非模板参数

- **决策**：Agent 输出场景 DSL（图层 + 动画 + 样式令牌），渲染器确定性解释。
- **否决**：仅模板参数（自由度不足）、HTML/CSS（不确定、不安全、导出慢）。
- **后续**：必要时增加 HTML 逃生舱。

## ADR-008 树形链路与一链一会话

- **决策**：每个节点单主父；链路是树；一条链路一个 Agent 会话。
- **理由**：上下文隔离、缓存可复现、删除语义简单。
- **代价**：不支持任意 DAG 合并。
- **M2 修订（ADR-021）**：链不再绑定会话；每次节点执行一个一次性运行，上下文由 App 从祖先链构建。树形与单父约束保留。

## ADR-009 共享 cwd + 节点增量

- **决策**：所有会话共享一个工作根目录；每个节点写自己的 `steps/<nodeId>/`；读取沿主父链叠加。
- **理由**：Claude Code 会话按 cwd 归档；per-chain cwd 会破坏 resume 与分叉。
- **代价**：需要权限代理做路径白名单。
- **M2 修订（ADR-021）**：改为「topic 树内共享、树间隔离」；每棵树独立 cwd，节点增量 `steps/<nodeId>/`，读按当前路径叠加，写作用域限当前运行节点。

## ADR-010 级联删除 + 回收站

- **决策**：删除节点连带删除全部下游；增量与产物进回收站 24 小时。
- **理由**：树语义简单；避免悬空引用。
- **代价**：删除影响面大，必须预览确认。

## ADR-011 Agent 通过能力位降级

- **决策**：`Session` 上报 `Capabilities`；`ConversationRewind` 不可用时，删除走新会话 + 重建包。
- **理由**：Claude 程序化接口只支持文件回退，不支持对话截断；未来可替换适配器。
- **代价**：v1 丢失工具调用历史。
- **M2 修订（ADR-021）**：运行之间不共享 Agent 记忆，M2 不使用 Resume；`Capabilities.Resume` 不再参与策略选择。

## ADR-012 Bash 三档策略

- **决策**：只读白名单自动允许；写文件/解释执行/未知命令需确认；越界与受保护路径直接拒绝。
- **理由**：只读 Bash 有实际需求，但 shell 解析不可靠，必须分层。
- **代价**：确认弹窗会增加操作摩擦。

## ADR-013 结构化产物手动应用

- **决策**：Agent 的 `reply` 生成待应用产物；用户点击应用后才改图。
- **理由**：保留人工控制；避免 Agent 直接污染工程。
- **代价**：多一步操作。
- **M2 细化（ADR-022）**：pending 与 artifact 分离；无效输出保留原文；一运行一个结构化 pending；应用时校验 `ancestorHash`/`paramsHash`，`force` 可覆盖；discard 幂等。

## ADR-014 不做手动 `/rewind`

- **决策**：v1 不提供终端回退入口，只记录 checkpoint UUID。
- **理由**：TUI 自动化脆弱；自动路径用重建包更可靠。
- **后续**：更换支持对话回退的 Agent 适配器，或等 SDK 开放接口。

## ADR-015 Wails 入口保留在仓库根目录

- **背景**：原计划入口放在 `cmd/audio-ui/`，但 Wails v3 用 `//go:embed all:frontend/dist` 把前端打进单 exe，embed 模式不能跨到包目录之外。
- **决策**：根 `main.go` 作为极薄的组装入口，只负责 embed、版本变量与调用 `internal/app`；Wails API 仍只出现在 `internal/app`（handler 不 import Wails）。
- **代价**：目录与常规 Go 服务的 `cmd/` 约定不同；必须用文档与 ADR 说明。
- **否决**：构建时复制 `frontend/dist` 到 `cmd/`（多一份构建步骤与漂移风险）；运行时从磁盘读前端（破坏单 exe 绿色版）。

## ADR-016 Agent 适配器与 Claude SDK

- **背景**：M1 需要 stream-json、initialize、control_request、权限回调、resume 与 MCP HTTP；直接实现协议面大且易随 CLI 漂移。用户要求 Agent 可替换，domain 不应绑定 Claude。
- **决策**：domain 只暴露 `AgentDriver`、`Session`、类型化事件与不透明 `resumeRef`；Claude 细节由 `infra/agent/claudecli` 使用社区 SDK `severity1/claude-agent-sdk-go v0.10.0` 实现。CLI 最低 2.1.224；Windows 必须使用原生 `claude.exe`，拒绝 `.cmd`（BatBadBut）。
- **配置**：M1 默认复用现有 `~/.claude` profile（不设 `CLAUDE_CONFIG_DIR`）的凭据/session。以 `--setting-sources=user` 只加载用户级 settings：`~/.claude/settings.json` 的 `env` 与 hooks 生效，因此凭据无论来自 OAuth 文件、`env` 里的密钥（供应商切换器，如 cc-switch）还是 `apiKeyHelper`，都能到达 CLI；项目/本地 settings 不加载，MCP 由 `--strict-mcp-config` 锁定。应用数据/日志/工作区仍在 exe 目录。Agent 自身凭据/session 属适配器例外，便携 profile 记 RM-014。
- **代价**：用户 hooks、`permissions`、`model` 与用户 CLAUDE.md 会影响 Agent 会话；用户 hooks 可执行任意命令，属于用户自担的本地配置。若需完全隔离，改用 RM-014 的便携 profile（届时在应用内完成登录/密钥配置）。
- **MCP**：不使用 SDK `WithMcpServers`（会写 OS temp）；应用自写 `<exe>/data/agent/` 配置，使用 `--strict-mcp-config`，stderr 走回调。权限用 `PreToolUse` 硬门禁 + `CanUseTool` 交互确认。
- **代价**：SDK 为 pre-1.0 社区实现；必须锁版本、保留 raw protocol 回退，并做 live spike。
- **否决**：完全自研协议（M1 风险高）；官方 TS SDK sidecar（引入额外 Node 进程，偏离单 Go 进程）。
- **M2 修订（ADR-021/023）**：每次节点执行启动一次性会话；`WithAllowedTools` 与系统提示由 `RunSpec` 按运行注入 M2 工具集，不再硬编码 M1；不调用 Resume。

## ADR-017 M1 临时单链路与存储

- **背景**：M1 尚无图/项目，但会话、workspace、resume、token 需要有归属。
- **决策**：M1 使用隐式 `chain_default`；workspace `<exe>/data/workspace/chain_default`；会话元数据 `<exe>/data/sessions.json`（version 1，原子写，最近 200 条 UI transcript）；MCP 配置/临时文件在 `<exe>/data/agent/`；审计 `<exe>/logs/audit.log`。M2 引入项目 `sessions.json` 时迁移。
- **理由**：满足“一链一会话”和 token 作用域，不提前实现完整图模型。
- **代价**：M1 的路径与 M2 项目路径不同；移动 exe 会使 resume/workspace 失效，M1 记录为已知限制。
- **M2 迁移（ADR-019/021）**：`sessions.json` 的 transcript 导入为只读运行（`status=imported`），不再 resume；旧 workspace 复制到默认工程链根节点 steps，原件保留备份；迁移幂等，失败保留原文件。

## ADR-018 Windows Agent Host 代理

- **背景**：Claude CLI 是 console 子系统；SDK 直接启动会闪控制台，且只终止直接子进程。Windows batch shim 又有命令注入风险。
- **决策**：同一 exe 以 `AUDIOUI_AGENT_HOST=1` 作为 stdio 代理启动；代理用 `CREATE_NO_WINDOW` 隐藏原生 Claude，用 Windows Job Object（kill-on-close）绑定子进程树，并透明转发参数/标准流。SDK 的 CLI 路径指向该 exe，原生路径由环境变量传入。
- **代价**：多一层进程与转发；host 模式必须避免递归、stdout 只能承载 Claude 协议数据。
- **回退**：spike 失败时直接启动原生 CLI，记录 RM；不得接受无界子进程泄漏。

## ADR-019 工程模型、图数据与存储

- **背景**：M1 只有隐式单链路与会话，没有工程、图、轮次、待应用产物；M2 需要可持续编辑、可恢复、可迁移的本地工程。
- **决策**：
  - 每个工程一个 `project.json`，包含节点（含 `title`）、轮次、锁定、`activeNodeId` 与 revision；**链不落盘**，由「topic 根 → 末端节点」的路径派生。
  - 每个节点保存 `parent` 与 `parentCandidate`：`parent` 是唯一父；`parentCandidate` 只在父为 topic 时记录使用的是哪个候选端口（候选序号）。`edges` 由两者派生，不落盘。
  - 存储布局：`data/projects/<id>/{project.json,pending.json,pending/,artifacts/,runs.json,runs/,trees/<rootTopicId>/steps/<nodeId>/}`；`config.json` 记 `lastProjectId` 与 `contextTokenBudget`（默认 1,000,000）。
  - 工程 ID、节点 ID 由程序生成；topic 根节点 ID 同时作为工作区树 ID；节点 ID 只允许 `[a-z0-9_]`，拒绝 `..`、分隔符、盘符与 Windows 保留名。链 ID 在视图层派生为 `c_<tailNodeId>`，不持久化。
  - 除 `SelectNode` 外的成功写命令递增 `revision`；`SelectNode` 只持久化 `activeNodeId`，不递增图 revision、不广播 `workspace:changed`。所有工程文件用临时文件 + `Sync` + 重命名原子写。
  - M1 数据一次性迁移：`sessions.json` 的 transcript 导入为只读运行，旧 workspace 复制到默认 topic 根的 `steps/<rootNodeId>/`，原件保留；迁移幂等，失败保留原文件。
- **理由**：单文件真相、无数据库依赖；`parent` 单值避免边与父字段双写漂移；链作为派生视图，分支/改挂后无需维护链实体；内容寻址产物便于后续渲染与导出复用。
- **代价**：非主边（反馈/上下文边）暂不支持；链 ID 随末端节点变化；工程规模上限依赖内存与文件大小；迁移需要一次性兼容代码。
- **否决**：SQLite（M2 无必要）；同时持久化 `parent` 与 `edges`（双真相）；持久化链实体（与路径派生重复）；用工程名做目录（路径安全与重命名问题）。

## ADR-020 命令层、图不变量与连线语义

- **背景**：M2 需要用户与 Agent 都不能绕过校验改图；画布同时要求「新增子节点」与「端口拖拽连线」。
- **决策**：
  - 所有写操作走 `service/workflow` 串行命令：`NewProject/OpenProject/AddNode/Connect/MoveNode/UpdateNodeParams/RunNode/ApplyPending/DiscardPending/SelectNode`；handler 与 MCP 只做参数转换、读取与调用，不直接改图。
  - 图是**单父森林**：每个节点最多一个父；topic 必须无父且是一个树的根；prompt 有父或为草稿；成环、自连、topic 作子节点、跨工程连接一律拒绝。
  - `AddNode` 可带 `parent` 与 `parentCandidate`，也可创建无父草稿；草稿不属任何链、不能运行，可被连接。
  - **自由改挂**：任意 prompt 节点（含其整个子树）可重新连接到任意可用输出端口，**允许跨 topic 树**；child 的 `(parent, parentCandidate)` 与目标不同（含同一父换候选）时返回 `REPARENT_CONFIRM_REQUIRED`，UI 确认后带 `replace=true` 重发；同一父同一候选重复连接为幂等无操作；移动子树（同父换候选不移动子树）会改变被移动节点及其后代的上下文，并将该子树标记 `stale`。跨树时同步迁移该子树的 `steps/<nodeId>/` 到目标树（失败回滚，`WORKSPACE_MOVE_FAILED`），并同时检查源树与目标树的运行/排队锁。
  - **topic 候选端口**：topic 应用候选后，每个候选是一个输出端口；`parentCandidate` 记录子节点使用的是哪个候选；一个候选可连接多个子节点（分叉）。不再有「选定选题」步骤。
  - **链是派生路径**：链 = topic 根到末端节点的路径；分支产生多条共享前缀的链；快捷栏列出叶子链；链 ID 为 `c_<tailNodeId>`，不落盘。
  - **运行锁**：运行/排队中的节点，其「根 → 运行节点」路径上的所有节点只读（参数、拓扑、禁止新增子节点）；与路径共享的祖先只读；路径外的兄弟分支可编辑。M2 只有单在途运行。
  - 锁定：提示词应用首轮后锁定内容与执行；topic 应用候选后锁定内容与执行；锁定只冻结参数/内容与再次执行，拓扑在非运行链上仍可改，但会改变后代上下文。锁定后继续迭代只能新增下游节点或从候选端口开分支。
  - `ApplyPending` 要求节点未锁定；M2 的 topic/prompt 每节点最多一个已应用轮次，`rounds` 为通用列表，供后续节点类型与导入运行使用。
  - `SelectNode` 只更新 `activeNodeId`，不递增图 revision、不广播 `workspace:changed`；选中与切换链是视图操作。
  - `SelectTopic` 移除；连接候选端口即选定该候选。
- **理由**：用户要求任意连接且只受运行/排队限制；单父树保证上下文是唯一祖先链；候选端口让一个 topic 的多个方向自然分叉；链作为路径视图，不需要会话/链实体。
- **代价**：改挂（含跨树）会改变子树上下文，需要确认、stale 传播与 steps 迁移；链随末端节点变化；M2 不提供断开（无新父）操作。
- **否决**：多父 DAG（上下文合并、高亮、运行锁与删除语义复杂，M2 不引入）；只允许接入无父草稿（不满足任意连接）；把链持久化为实体（与路径派生重复）。

## ADR-021 运行级会话、上下文与工作区隔离

- **背景**：一链一会话依赖 Agent 自身记忆；分叉、改结构、删除都会污染或需要伪造会话，且跨链隔离依赖 cwd。M1 的 resume 又要求保持会话持久。
- **决策**：
  - **链与会话分离**：链是 topic 根到末端节点的派生路径；每次节点执行创建一个一次性运行（Run），启动一个 Agent 会话，首条消息是 App 构建的上下文；运行结束即销毁。运行之间不共享 Agent 记忆。
  - 上下文只含「topic 根 → 父节点」的路径：每个祖先的**最新已应用轮次**；topic 段只带该路径经候选端口选中的**那一个候选**，不带其他候选；目标节点只带当前参数；不含目标自身历史、兄弟分支、子节点历史、其他树与未应用产物。
  - 上下文按 token 计，默认 `contextTokenBudget=1,000,000`；超预算从最远祖先「全文 → 摘要 → 引用」降级，仍超则 `CONTEXT_TOO_LARGE`；发送前保守估算，运行后按 CLI 实际 usage 校准。
  - 上下文文本与 `ancestorHash`/`paramsHash` 可复现；不包含 runId/时间戳；工作区只注入清单与叠加规则。
  - 工作区按 **topic 树**共享：cwd = `trees/<rootTopicId>/`，节点增量 `steps/<nodeId>/`；写作用域限当前运行节点；读取走当前路径的祖先叠加。内置 Write/Edit 由 PreToolUse 做同样白名单；Bash 仍是协作式门禁（RM-017）。
  - **跨树改挂**：子树从树 A 移到树 B 时，把子树的 `steps/<nodeId>/` 从 A 迁到 B；迁移失败整体回滚；迁移后新运行使用 B 的 cwd 与路径。
  - **运行锁**：运行节点所在的「根 → 运行节点」路径只读（参数/拓扑/禁止新增子节点）；共享祖先只读；兄弟分支可编辑。M2 只有一个在途运行；M5 排队链同样锁定。
  - 全局同一时间只允许一个在途运行；并发/队列在 M5 按运行计数。
  - 重启只恢复工程/图/运行历史；未完成运行标记 `interrupted`，不自动 resume。
- **理由**：上下文由图推导，分叉/改挂无需 fork 或重建；候选端口使每条分支有确定输入；运行锁保证在途运行的祖先链不被改；输入可哈希、可追溯；M5 调度模型更简单。
- **代价**：每次运行启动 CLI 的固定延迟；上下文 token 成本高于常驻会话；放弃 M1 的跨重启 resume（AC-M1-3/4 由运行历史取代）；同一 topic 树内分支共享工作区，跨分支文件读取依赖路径策略（RM-017）。
- **取代**：ADR-008 的「一链一会话」；**修订** ADR-009 的「所有会话共享工作根」为「topic 树内共享、树间隔离」。
- **否决**：每链常驻会话（历史污染、隔离依赖提示）；项目级常驻会话（跨树泄漏）；多父 DAG（M2 不引入）；精确 tokenizer 预检（依赖/凭据/供应商兼容，留作评估）。

## ADR-022 待应用产物、轮次、锁定与内容寻址

- **背景**：Agent 结构化结果必须先待应用，人工确认后才改节点；无效输出要保留原文；应用后产生不可变轮次。
- **决策**：
  - pending 与 artifact 分离：pending 是工作流状态（`pending.json` + `pending/<id>.json`），artifact 是应用后的不可变结果（`artifacts/<sha256>/`）。
  - `reply` 生成 pending，绑定 runId/chainId（运行时的末端节点）/chainPath/nodeId/contextHash/ancestorHash/paramsHash；无效 Schema 也落盘，状态 `invalid`，节点标红，不自动重试。
  - 同一运行最多一个结构化 pending；`request_id` 存在时幂等。
  - `ApplyPending` 校验节点类型/kind、未锁定、参数与祖先哈希；不一致返回 `PARAMS_CHANGED`/`CONTEXT_CHANGED`，UI 确认后 `force=true`；应用 = 写 artifact + 追加轮次 + 锁定节点内容与执行 + 全部后代 `stale`。锁定后不可再次执行或应用新轮次；M2 的 topic/prompt 每节点最多一个已应用轮次，迭代通过新增下游节点或候选端口分支完成。
  - artifact 存于 `artifacts/<sha256>/payload.json` 与 `meta.json`；哈希按规范化 payload 字节计算，`meta.json` 记录 kind/nodeId/createdAt/sourceRunId/sourcePendingId。
  - `parentCandidate` 为 0 基候选序号；UI 显示为 1 基。
  - topic 应用 `topics` 后锁定参数、候选集合与执行；每个候选成为一个输出端口，可继续连接新分支；**不设「选定」步骤，连接哪个候选端口，哪条分支就用哪个候选**。
  - prompt 应用首轮后锁定内容与执行；拓扑在非运行链上仍可改挂，改挂会标记子树 `stale`。
  - `DiscardPending` 只改状态，不删除 payload；应用后丢弃返回 `PENDING_ALREADY_APPLIED`。
  - `apply_artifact` 永远只由 UI 调用，不注册为 MCP 工具。
- **理由**：生命周期与语义分离；无效输出可追溯；候选端口把「选择」变成结构事实，不再需要节点级 selectedTopic；应用是唯一改节点内容的路径。
- **代价**：多一步人工应用；pending 清理与引用回收留 M5；候选端口连接后改挂需重新校验上下文。
- **修订**：ADR-013 补充 pending 存储、无效输出、幂等与 discard；移除 selectedTopic 与 SelectTopic。

## ADR-023 只读图 MCP 与结构化 reply

- **背景**：运行级会话下，上下文由 App 注入；Agent 仍需要查询图、节点与工作区，但不能改图。
- **决策**：
  - M2 工具面：`get_chain`、`get_graph`、`get_node`、`list_node_types`、`validate`、`read_artifact`、`reply`、`read_file`、`list_dir`、`grep`、`stat`、`ask_user`。
  - 不提供 `create_node`、`connect`、`update_params`、`delete_*`、`apply_artifact`；写图工具登记 RM-016。
  - `reply` 的 `node_id` 必须是当前运行目标节点；kind 与节点类型匹配；`topics`/`script` 校验后生成 pending；`scene_dsl` 在 M2 返回 `UNSUPPORTED_UNTIL_M3`。
  - `get_graph` 返回树的节点与 `parent`/`parentCandidate` 摘要；路径链由 App 派生，不返回 selectedTopic。
  - `read_artifact` 只允许当前运行路径上的祖先产物，按 offset/limit 分块返回，单次 ≤16 KiB，用于预算降级后的按需读取。用户确认：`topics` 仅返回该路径所选候选的 JSON，偏移和长度作用于过滤后的 UTF-8 字节；候选缺失/越界时拒绝，绝不回退到整份产物。`artifactId` 仍标识原始完整产物，其哈希不等于过滤后字节的哈希；在 M2-06 实现。
  - 每运行独立 bearer token，绑定 runId/treeId（topic 根）/nodeId；跨树访问返回 `CHAIN_SCOPE_DENIED`；payload ≤256 KiB；每个 topic 树的 pending 上限 50。
  - 只读图工具返回摘要与引用，不返回大块产物正文。
- **理由**：上下文注入已满足主流程；只读工具用于补查；禁止写图保持不变量 5/8。
- **代价**：Agent 不能主动建图或改参数；需要用户手工操作。
- **修订**：ADR-016 的字段与工具注入；ADR-011 的 Resume 不再参与 M2 策略。

## ADR-024 事件协议与前端运行视图

- **背景**：M2 同时有工程/图/待应用变化与运行流式事件；路径链与运行必须按节点/路径分流。
- **决策**：
  - 命令返回 `{revision, snapshot}`；广播只发 `workspace:changed {revision, reason}` 作为失效信号，前端按 revision 重拉快照；不做增量 diff。
  - Agent 事件带 `runId`、`nodeId`、`chainPath`（运行时快照）与每运行独立 seq；前端 `runStore` 按运行保存状态与 transcript，`workspaceStore` 保存图与快照。
  - QuickBar 的链由叶子路径派生：链 ID = `c_<tailNodeId>`；派生链视图由 Go 在快照中计算（不落盘），前端只消费，不复制链规则；`activeNodeId` 决定当前路径；分支产生多条共享前缀的链。
  - `SelectNode` 只持久化 `activeNodeId`，不递增 revision、不广播 `workspace:changed`；选中与路径高亮由前端本地更新。
  - `workspace:changed.reason` 取 `project` / `graph` / `pending` / `run`。
  - 运行记录保留全部，单次 transcript 上限 500 条；控制台「对话」按运行分组，历史只读。
  - `run:changed` 只携带状态与标识，不携带 transcript。
- **理由**：避免拖拽/大量节点时全量快照放大传输；revision 失效信号简单可靠；路径派生让分支无需维护链实体；运行分组与一次性会话语义一致。
- **代价**：事件后需要一次快照请求；前端需要处理缺口与乱序；链 ID 随末端节点变化。
- **否决**：delta 合并（易漏状态）；每次事件都推全量快照（传输放大）；持久化链实体（与派生重复）。

## ADR-025 共享只读运行时位于 exe 同级 `runtime/`

- **背景**：M2 工作区读取需要跨 topic 树共享的只读素材；现有文档与发布结构对根 `runtime/` 和 `data/runtime/` 的语义不一致（RM-013）。
- **决策**：共享运行时资源固定放在 exe 同级 `<exe>/runtime/`，与 `data/`、`logs/`、`webview2/` 并列；它随绿色包发布、所有 topic 树只读共享，不属于工程数据，不由应用创建或修改；`data/runtime/` 不使用。
- **读取**：MCP 逻辑只读视图按当前节点 `steps/` → 当前运行路径祖先 `steps/` → exe 同级 `runtime/` 回退；较高优先级同名文件/目录遮蔽低优先级项。内置 Agent 文件工具不读取根 runtime，写作用域不包含 runtime。
- **失败处理**：缺少 `runtime/` 视为没有共享资源，不阻止工程或 Agent 运行；若目录或其路径组件为 reparse point/junction，拒绝读取，避免脱离绿色目录边界。
- **理由**：与现有绿色包结构、README 和根目录运行时约束一致；静态资源随程序发布，避免将只读依赖与可变工程数据混放。
- **代价**：运行时资源变更需随包更新；不能由 Agent 或工程配置修改；`AUDIOUI_HOME` 开发覆盖下运行时仍位于覆盖根目录的 `runtime/`。

## ADR-026 上下文指纹、预算降级与资源引用

- **背景**：M2-04 需要可复现的上下文与待应用校验指纹；工作区在运行期间可能产生文件，而用户要求不能截断目标指令，也不能将文件资源全文注入上下文。
- **决策**：目标参数始终完整保留；按根 → 直接父的顺序依次将祖先结构化产物从全文降为最多 2,000 估算 tokens 的确定性原文前缀，再降为产物引用。直接父最后降级；仍超预算返回 `CONTEXT_TOO_LARGE`，包含最终估算值与预算。
- **资源**：工作区文本和二进制文件均不自动注入内容；清单仅列当前路径叠加视图（含共享 runtime）的逻辑路径与大小，二进制资源只以路径引用。清单按确定顺序最多遍历 5,000 个文件/目录、深度最多 64，截断时明确标记。节点已应用的结构化 JSON 产物仍按上述全文/摘录/引用规则处理。
- **作用域**：`RunWorkspaceReader` 暴露工程、树、节点与路径身份；构建器在读取清单前与图推导路径逐项校验，拒绝误用其他运行的视图。
- **指纹**：`contextHash` 是上下文主体经 Go `encoding/json` 紧凑序列化字节的 SHA-256，包含清单但不包含自身元数据；`ancestorHash` 覆盖路径祖先、参数、所选候选及产物标识，不含工作区；`paramsHash` 覆盖目标完整参数。工作区写入可能是当前运行的输出，因此不能让它误触发应用时的祖先变化。清单只按路径与大小计入 `contextHash`，同路径同大小的内容变化、截断范围外变化不会被识别。
- **估算**：`estimatorVersion=1` 使用与供应商无关的保守启发式，估算完整首条消息，不能保证等于实际 CLI 用量；`ContextEstimate.WithActualInputTokens` 按本次预算计算 `overBudget`。运行完成后采集和持久化实际用量由 M2-07 接线。
- **理由**：保留用户任务原义，避免把大文件/二进制送入上下文；上游指纹不因运行自身写出的文件变化而产生伪冲突。
- **代价**：摘录是原文截取而非语义摘要；超限时仍可能拒绝运行；工作区文件内容变化不构成祖先哈希校验。
