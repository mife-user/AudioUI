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
- **决策**：同一 exe 以 `AUDIOUI_AGENT_HOST=1` 作为 stdio 代理启动；代理用 `CREATE_NO_WINDOW` 隐藏原生 Claude，用 Windows Job Object（kill-on-close）绑定子进程树，并透明转发参数/标准流。SDK 的 CLI 路径指向该 exe，原生路径由环境变量传入。主 GUI 在工程初始化与 `starting`/`running` 恢复前取得单实例锁，避免二次启动清理活动运行；因 SDK 的版本探测目标是该 host 而非 Claude CLI，应用关闭 SDK 重复探测，原生 CLI 版本仍由 `ResolveNative` 校验，`-v` 只输出版本并立即退出。
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
  - 上下文按 token 计，默认 `contextTokenBudget=1,000,000`；超预算从最远祖先「全文 → 摘要 → 引用」降级，仍超则 `CONTEXT_TOO_LARGE`；发送前保守估算，运行后按首个 `AssistantMessage.Usage` 的首次请求用量（包括缓存 token）校准；缺失时保持未知，不把多轮累计 usage 当作上下文用量。
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
  - pending 与 artifact 分离：pending 是工作流状态（`pending.json` + `pending/<id>.json`），artifact 是应用后的不可变结果（`artifacts/<sha256>/`）。pending 记录可选 `request_id`，限制为 128 字节。
  - `reply` 生成 pending，绑定 runId/chainId（运行时的末端节点）/chainPath/nodeId/contextHash/ancestorHash/paramsHash；无效 Schema 也落盘，状态 `invalid`，保留原文与错误，不自动重试。
  - 同一运行最多一个结构化 pending；相同 runId + `request_id` + 请求内容重放时返回原 pending，复用 request id 但内容不同返回 `REQUEST_ID_CONFLICT`。
  - 每 topic 树最多 50 条 `pending`/`invalid`；应用或丢弃释放名额，历史索引与原文继续保留。
  - `ApplyPending` 校验节点类型/kind、未锁定、参数与祖先哈希；不一致返回 `PARAMS_CHANGED`/`CONTEXT_CHANGED`，UI 确认后 `force=true` 只覆盖哈希冲突，不能绕过 Schema、节点、运行锁或内容锁。应用 = 写 artifact + 追加唯一轮次 + 锁定节点内容与执行 + 全部后代 `stale`。M2 的 topic/prompt 每节点最多一个已应用轮次，迭代通过新增下游节点或候选端口分支完成。
  - artifact 存于 `artifacts/<sha256>/payload.json` 与 `meta.json`；payload 按保留数字精度的确定性 JSON 规范化后计算 SHA-256。`meta.json` 以首次写入为准，当前轮次的 `kind` 才是授权真相；跨 kind 复用时按轮次 kind 校验 payload。
  - `pending.txn.json` 是 pending、artifact 与 `project.json` revision 的工程内恢复日志：先写意图和必需原文/CAS，再保存目标 revision 的工程，最后更新 pending 索引并删除日志。工程加载/打开比较 base/target revision，未提交时删除日志，已提交时幂等重放索引与 CAS；revision 不匹配、日志损坏或恢复失败时阻止操作，不暴露半成品。
  - 创建/应用/丢弃与 `ProjectService.New/Open` 共用 `ProjectSession.WithExclusive` 操作锁，避免工程切换穿过跨文件事务；锁顺序为 workflow 命令锁后工程操作锁。
  - `parentCandidate` 为 0 基候选序号；UI 显示为 1 基。
  - topic 应用 `topics` 后锁定参数、候选集合与执行；每个候选成为一个输出端口，可继续连接新分支；**不设「选定」步骤，连接哪个候选端口，哪条分支就用哪个候选**。
  - prompt 应用首轮后锁定内容与执行；拓扑在非运行链上仍可改挂，改挂会标记子树 `stale`。
  - `DiscardPending` 幂等，只改状态且不删除 payload；应用后丢弃返回 `PENDING_ALREADY_APPLIED`。
  - `apply_artifact` 永远只由 UI 调用，不注册为 MCP 工具。
- **理由**：生命周期与语义分离；无效输出可追溯；候选端口把「选择」变成结构事实，不再需要节点级 selectedTopic；应用是唯一改节点内容的路径。
- **代价**：多一步人工应用；pending 清理与引用回收留 M5；候选端口连接后改挂需重新校验上下文。
- **修订**：ADR-013 补充 pending 存储、无效输出、幂等与 discard；移除 selectedTopic 与 SelectTopic。

## ADR-023 只读图 MCP 与结构化 reply

- **背景**：运行级会话下，上下文由 App 注入；Agent 仍需要查询图、节点与工作区，但不能改图。
- **决策**：
  - M2 工具面：`get_chain`、`get_graph`、`get_node`、`list_node_types`、`validate`、`read_artifact`、`reply`、`read_file`、`list_dir`、`grep`、`stat`、`ask_user`。
  - 不提供 `create_node`、`connect`、`update_params`、`delete_*`、`apply_artifact`；写图工具登记 RM-016。
  - `reply` 的 `node_id` 必须是当前运行目标节点；运行已结束或节点在路径中但不是目标返回 `TURN_NOT_ACTIVE`，路径外节点返回 `CHAIN_SCOPE_DENIED`；kind 与节点类型匹配；`topics`/`script` 校验后生成 pending；`scene_dsl` 在 M2 返回 `UNSUPPORTED_UNTIL_M3`。结构化 payload 从 MCP 原始 arguments 提取，Schema 无效但 JSON 合法时保留原文为 invalid pending。
  - `get_graph` 只返回当前运行 `chainPath` 节点的摘要，不暴露同树兄弟或其他树；`get_node` 只读路径内节点参数与轮次元数据；路径快照与当前工程图不一致时 fail closed。
  - `read_artifact` 只允许路径祖先任一轮次引用，按 round kind 而非 CAS 首写 meta 校验；topic 产物先取下游边所选的单候选对象并紧凑序列化，再按 UTF-8 字节分页。单次 ≤16 KiB；offset 必须位于字符边界；重复 artifact ID 的视图冲突时拒绝；不得回退完整 topics。过滤后视图的哈希不代替原始 `artifactId`。
  - MCP 工具调用绑定 runId/projectId/treeId/nodeId/chainPath；活动运行租约覆盖读取/写入，运行终止不得与 pending 写入竞态。
  - 每运行独立 bearer token；结构化 payload ≤256 KiB；每个 topic 树的活动 pending 上限 50。
  - 只读图工具返回摘要与引用，不返回大块产物正文。
- **理由**：上下文注入已满足主流程；只读工具用于补查；禁止写图保持不变量 5/8。
- **代价**：Agent 不能主动建图或改参数；需要用户手工操作。
- **修订**：ADR-016 的字段与工具注入；ADR-011 的 Resume 不再参与 M2 策略。

## ADR-024 事件协议与前端运行视图

- **背景**：M2 同时有工程/图/待应用变化与运行流式事件；路径链与运行必须按节点/路径分流。
- **决策**：
  - 命令返回 `{revision, snapshot}`；广播只发 `workspace:changed {revision, reason}` 作为失效信号，前端按 revision 重拉快照；不做增量 diff。
  - Agent 事件带 `runId`、`treeId`、`chainId`、`nodeId`、`chainPath`（运行时快照）与每运行独立 seq；运行期 Wails 事件名为 `run:<type>`，前端 `runStore` 按运行保存状态与 transcript，`workspaceStore` 保存图与快照。
  - QuickBar 的链由叶子路径派生：链 ID = `c_<tailNodeId>`；派生链视图由 Go 在快照中计算（不落盘），前端只消费，不复制链规则；`activeNodeId` 决定当前路径；分支产生多条共享前缀的链。
  - `SelectNode` 只持久化 `activeNodeId`，不递增 revision、不广播 `workspace:changed`；选中与路径高亮由前端本地更新。
  - `workspace:changed.reason` 取 `project` / `graph` / `pending`；运行路径锁变化由携带 `chainPath` 的 `run:status` 表达，不递增工程 revision。
  - 运行记录保留全部，单次 transcript 上限 500 条；控制台「对话」按运行分组，历史只读。
  - `run:status` 携带 `chainPath` 与状态；运行开始/结束的 status 事件同时作为路径锁变更信号，M2-08 前端据此刷新锁定视图。运行事件不携带完整 transcript。
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

## ADR-027 目标设计：显式 Agent 节点与本地脚本输出闭环

- **状态**：设计已验收（用户于 2026-11-06 确认；本地脚本子范围纳入 M2-11～13）；M2-11B 工程格式与图领域模型已于 2026-11-06 通过用户验收；M2-11C 发送快照、上下文与运行授权已于 2026-11-07 通过用户验收；M2-11D 受限结果事务已于 2026-11-07 通过用户验收；M2-12A 已于 2026-11-08 通过用户验收，M2-12B 已于 2026-11-08 通过用户验收（实现与自动化自检完成；真实浏览器交互未执行）；M2-13 已于 2026-11-08 通过用户验收。M2-00～10 的旧模型与验收事实保留，不能把旧证据视为新目标通过。
- **范围**：仅本地执行，复用现有 `topics` 与 `script` Schema。RM-019 云端执行和 RM-020 视频产物/渲染仍待评估，不由本 ADR 纳入。
- **节点与连接**：节点类型为 `topic`、`candidate`、`prompt`、`agent`、`script`。唯一允许的父子类型为 `topic → candidate`（仅应用自动生成）、`candidate → prompt`、`prompt → agent`、`agent → script`、`script → prompt`；Candidate 与 Script 可各有多个 Prompt 子节点形成后续分支。单父、无环和全部图写入经 Go 命令层校验；前端不得独立裁定连接合法性。
- **候选身份**：选题返回通过 `ValidateStructuredPayload(OutputKindTopics, ...)` 的 payload 后，应用在一个可恢复事务中保存不可变 `topics` artifact 并生成对应 candidate 节点。Candidate 绑定 `(artifactId, candidateIndex)`，索引为 0 基；不再使用 topic 端口或 `parentCandidate`。topic 生成候选后沿用已验收的内容/执行锁定规则；非法结果不生成候选。
- **节点职责**：prompt 只保存任务与约束，不启动运行；Agent 节点从当前路径构建上下文并启动本地一次性运行。prompt 在不受活动运行锁时可编辑；Agent 可在非运行期间再次运行。每个 Agent 至多一个直接 script 输出节点。
- **输出映射**：topic 的 `topics` 结果映射为 candidate 节点；Agent 的唯一输出 kind 为现有 `script` Schema v1，复用 `ValidateStructuredPayload` 与 `CanonicalJSON`，不复制 Schema。Script 节点的每个已确认版本均引用不可变 CAS artifact 和来源运行。
- **reply 与版本**：无直接输出绑定时，合法 `script` reply 在应用命令层原子创建直接 script 子节点并关联结果；指定空 script 节点时自动写入首个版本；目标已有版本时保存绑定该目标及当前版本的待确认结果，只有用户确认才追加新版本。确认需重验目标绑定与基准版本；版本已改变则拒绝，不使用旧 `force` 绕过归属/契约。取消不改旧版本或 artifact。新建/追加后不自动运行下游。非法 JSON/Schema、白名单外 kind、越权节点、过期运行或契约不符不得改变图/输出版本，并保留有界诊断证据。
- **幂等与事务**：节点、父子关系、artifact、run 来源和去重身份作为一个可恢复事务提交；按运行身份、请求身份及 payload 指纹幂等。无 request ID 时同运行同内容也不得重复建节点；同运行不同内容/身份冲突拒绝。提交后索引/事件失败可恢复，不得半提交或假报成功；不删除可能共享的 CAS。
- **发送快照**：运行冻结 topic root → Agent 路径、candidate 来源 artifact/index、prompt 任务/约束、输出绑定（或未绑定状态）、契约 kind/schema version 与授权的产物版本。改挂后的下一次运行重建上下文；旧运行和结果来源永不随实时图改写。运行内应用查询以冻结授权投影为准；活动期间保护运行路径和输出绑定，提交时仍复核身份与版本。
- **工程格式**：新模型 `project.json` 使用版本 2。旧 v1 工程明确返回 `UNSUPPORTED_VERSION`，不静默迁移；不兼容工程不能阻止有效工程列表、新建或打开。M1 `sessions.json` 导入直接产出 v2 工程，原有备份/恢复规则保留。`bin` 中已指定旧工程的一次性清理只按 M2-11B 计划在关闭应用并复核路径后执行，不得实现成应用自动删数据；`bin` 外旧工程不删除、不自动迁移。
- **错误码**：复用 `UNSUPPORTED_VERSION`、`SCHEMA_INVALID`、`OUTPUT_KIND_INVALID`、`OUTPUT_KIND_MISMATCH`、`TURN_NOT_ACTIVE`、`CHAIN_SCOPE_DENIED`、`NODE_LOCKED`、`BUSY`、`REQUEST_ID_CONFLICT`；新增目标模型需稳定区分 `NODE_CONNECTION_INVALID`、`OUTPUT_BINDING_CONFLICT`、`OUTPUT_VERSION_CONFLICT`、`RUN_RESULT_CONFLICT`、`RUN_PENDING_CONFLICT`。M2-11D 已接通受限 `reply`，不再返回历史过渡错误 `OUTPUT_COMMIT_UNAVAILABLE`。错误均由 Go 命令/服务端产生，UI 按 code 展示。

- **不变量**：单父森林、应用命令层写图、artifact 内容寻址不可变、路径隔离、全局单在途运行和本地绿色版边界保持。受限 `reply` 是应用侧命令入口，不授予 Agent 通用 `create_node`/`connect`/`update_params` 权限。
- **待评估边界**：视频输出节点、scene DSL/前端代码选择与渲染导出属于 RM-020；云端适配、账户、素材上传、隔离、留存与计费属于 ADR-028/RM-019。任意生成代码不得在有应用凭据或本机权限的环境中直接执行。

## ADR-028 目标设计：位置无关的本地/云端 Agent 执行

- **状态**：草案；RM-019，未纳入当前 M2。尚未实现，传输选型、账户/计费、资源政策及上线范围待评审。
- **边界**：桌面应用保留图、上下文构建、输出契约和结果提交；统一 Agent 端口对上提供启动、事件、权限/问题应答、取消和结果，对下由本地与远程适配器分别执行。以 gRPC 类流式接口为设计类比，不预先锁定协议或声称已有服务。
- **工具职责**：Web 搜索、文件创建/读取/修改/删除、Bash 等属于具体 Agent 及其执行环境；MCP 仅暴露经授权的 AudioUI 应用交互，如 `reply`、问题与必要的应用数据查询。适配器报告实际能力，不假设更换 Agent 后所有工具均存在；不可用能力需明确提示。
- **文件**：本地执行使用本地受限工作区；云端执行使用云端隔离工作区，用户明确选择需要上传的本地文件，未上传的本地文件不可见；云端 Agent 使用自身文件/Bash 工具，不要求将通用工具调用反向代理到用户电脑。返回文件需按运行和用户授权带回应用。
- **应用通信**：本地运行的应用 MCP 可留在本机；云端运行的应用交互端点可与 Agent 同处云端隔离环境，`reply`、问题和事件由远程运行服务转发到本地应用，最终由本地 Workflow 校验并写图；不将用户电脑的 MCP 端口直接暴露到公网。云端服务的身份、运行作用域及资源归属须独立鉴权。
- **运行快照**：统一请求应表达位置无关的任务、上下文、输出契约和经用户选择的文件引用；本机绝对路径、MCP 配置路径只属于具体适配器。上传文件应绑定内容标识和运行范围，避免同名或同大小文件变化造成错误关联。具体线协议和字段待设计。
- **隔离与可靠性**：云端按用户及运行隔离工作区、凭据与资源；限制可用资源和网络，避免任意 Bash 越权；保护传输和存储；明确取消、超时、重连、重复提交与清理/保留策略。工具提示或应用级路径检查不能替代操作系统级隔离。
- **隐私与用量**：云端模式需用户登录，服务端校验用户及运行归属；具体登录方式待定。云端运行会发送为任务构建的提示词及上下文，并在用户选择时上传文件，不等同于现行“内容不上传”。若平台代付，需有独立额度与完整用量对账；M2 的首次请求输入 token 不是完整账单。
- **关系**：此目标修订 ADR-016/021/023 的部署与工具边界，不改变现有本地实现的事实；保留历史 ADR 和验收记录。

## ADR-029 工作台布局与请求呈现

- **状态**：U0 决策由用户于 2026-11-08 确认；独立工作台交互优化批次，位于 M2 验收之后、M3 开始之前。U2 实现、Go 全量测试/vet、前端自动化/类型/lint/令牌检查与前端/Wails 构建均通过；用户于 2026-11-09 验收前端交互，U2 完成。U3、U4、U5 均于 2026-11-09 经用户确认验收；U5 助手未完成真实浏览器实测或截图/viewport 归档，真实 Agent 与 WebView2 平台行为未覆盖。U1 仍待用户验收，Windows 原生 IME 与真实 Agent 未测。
- **背景**：卡片尺寸与位置需要一并持久化；Script 正文与待确认结果必须按发送时冻结目标授权；固定侧栏挤占画布且不能丢失权限、问题和 pending 请求。
- **布局**：节点工程数据同时保存 `position` 与必需 `size: {width, height}`。U2 将 `project.json` 演进为 v3，v1/v2 不提供兼容或迁移；八向缩放只在本地交互状态中连续更新，结束时通过一个 Workflow 布局命令原子写入位置和尺寸，并复用工程作用域 receipt/revision。纯布局修改允许发生在内容运行锁期间，但不得改变图拓扑、运行路径、上下文哈希、artifact 或 stale 状态。
- **开发期数据**：当前工程持久化格式仍处于开发期，可按当前模型直接演进；不为既有开发工程提供迁移、旧版读取或双向兼容，历史开发数据可被丢弃。正式发布前另行冻结 schema 与兼容政策。
- **新建入口**：仅画布空白处支持右键菜单与键盘等价入口；菜单提交现有 Go `AddNode` 命令并传入点击画布坐标。除 Topic 外的新节点均为无父草稿；Candidate 仍只能由 Topic 结果自动生成。节点右键菜单与命令面板不纳入本批，双击聚焦维持现状。
- **Script 结果**：卡片正文通过 `projectId + scriptNodeId + version` 读取。服务端验证节点类型、版本引用及 CAS 关系后才返回 `script@1` 内容；不允许按任意 hash 读取。待确认目标从 Pending 冻结的输出绑定生成，禁止按实时 Agent 子节点推断；确认仍由现有命令重验绑定与基准版本。
- **Agent 面板**：默认收起为悬浮状态入口；运行列表、transcript、停止和历史仍可访问。权限、问题和 pending 使用既有 run/request/pending 身份路由到归属节点；收起或关闭面板不停止运行、不自动授权、不丢弃请求。无法定位的请求保留异常恢复入口。
- **不变量**：本决策不改变 Go 连接矩阵、改挂确认、运行快照、Agent/MCP 工具职责、权限审计或 M2 历史验收结论；视觉与布局操作不能绕过现有服务端校验。

## ADR-030 多上游无环图与版本引用（草案）

- **状态**：用户已确认任意阶段多上游无环连接、版本引用、独立 Prompt → Agent 运行及连接/执行分离原则；具体图存储、输入合并、工作区与授权方案仍须评审。未实现；不得把此草案描述成现有功能或据此跳过 DoR。
- **背景**：当前 M2 的 `Node.parent`、Topic 根、`chainPath`、工作区及 Agent 唯一输出依赖单父森林，不能表达自由创建的片段、不同阶段相连和多上游汇合。用户要求在现有验收事实之后演进图模型，而非撤销历史验收。
- **已确认的图目标**：各阶段的不同节点可按任意创建顺序建立有方向连接，允许多上游与多下游；不按现有阶段顺序拒绝连接。禁止自连和直接或间接形成环；跨工程连接不允许。独立 Prompt 和 Agent 必须可以先手动连接。单父约束与仅 Topic 根约束不能作为新目标的连接限制。
- **已确认的运行原则**：独立 Prompt → Agent 有有效任务时，可不依赖 Topic/Candidate 发起 Agent 运行；连接不自动表示可运行。运行时依据目标任务和工具的实际输入契约检查已选输入；缺失或不兼容时说明原因，不静默忽略输入或删除连线。不以 Agent 文本任务的“单主任务”示例限制所有节点；运行下游不自动重新运行上游。
- **已确认的结果目标**：无绑定结果时自动建节点、已绑定空节点时写首版；已有结果默认预览确认后追加不可变版本，也允许用户主动选择新建并行结果节点。已有下游继续引用原版本，用户可为每条输入连接明确选择其他版本；预览不改变绑定。新版本不能静默改变旧下游或旧运行快照。实现显式并行结果须替换现有 Agent 至多一个直接 Script 输出的限制，并设计运行目标绑定和事务。
- **Topic 重跑目标**：允许已有候选的 Topic 再次生成；新结果先预览、确认后以新批次生成独立 Candidate，保留旧候选、下游及 artifact/index 来源，取消或失败不改变旧批次。即使内容寻址 artifact 相同，主动新运行也须保留独立运行/批次来源；首次生成沿用现有自动提交行为。Topic 参数编辑、批次 schema 和事务仍须设计。
- **建议的模型，待设计**：使用可独立标识的有向边集合替代单 `parent` 作为唯一拓扑真相；边可表达精确版本引用，Go 命令层原子检查节点、环、锁与工程作用域。候选来源 `(topicId, artifactId, candidateIndex)` 与产物/运行来源独立于普通连接，新增或调整其他边不能伪造或更改原来源。边字段、顺序、相同端点/多端口、删除和回收语义尚未定稿。
- **运行待设计**：具体输入选择与默认值、角色和顺序、共享祖先去重且保留不同版本/角色、多个输出目标绑定、多根工作区身份、同名文件处理、读写授权、子图快照、并发锁和冲突处理；不能沿用单一路径而悄悄丢输入。删除/回收 `A → C ← B` 中的 A 时须先明确共享 C 的影响，不能静默级联删除共享节点；具体可选操作和恢复策略另行评审。
- **安全与切换门禁**：冻结本次实际输入、精确版本和输出目标；不让实时改线或新版本改写旧运行；不因汇合自动扩大不同根的工作区访问权限。变更必须覆盖权限、锁、事务与失败回滚。开发期数据格式可按已确认策略演进，但新图上线前必须保证原有效单路径运行及其安全/输出回归，不以临时不兼容破坏已验收闭环。
- **关系**：此目标将修订 ADR-020/021/027/029 中的单父、Topic 根、唯一输出及单路径相关约束；在技术方案评审和实现完成前，这些 ADR 仍描述当前已验收实现。RM-020 采用可控画面与按需生成式素材的混合评估方向，继续使用现有 Claude Code 适配、不以自研 Agent 为前提；具体视频工具、服务和输出契约另行评审，不由本 ADR 预选或纳入实现。
