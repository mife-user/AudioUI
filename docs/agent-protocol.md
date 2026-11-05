# Agent 接入协议

> 目标：让 Agent 以结构化、可校验、可替换的方式服务工作台。
> v1 适配器：Claude Code CLI（stream-json + control protocol）+ 本机 MCP HTTP。
>
> **M2 范围**：运行级一次性会话、App 构建上下文、只读图工具、结构化 `reply`、待应用产物；多链单在途。写图工具、队列、删除、渲染在后续批次。

## 目标协议草案（未实现）

> 本节描述用户提出的新目标和协议语义；下文仍为当前 M2 的实际协议，不应把这里的概念字段或远程能力当成现有 API。优先级、线协议、节点迁移与未决行为见 [路线图](dev/roadmap.md) 和 ADR-027/028。

### 执行请求与上下文

- 本地应用在每次发送前按当前实际图路径构建提示词、祖先产物、所选候选和任务约束。改挂后下次发送使用新路径；已发出的运行保持其快照，不复用 Agent 的旧会话记忆。
- 输出节点是 Agent 的后继，不能混同于祖先上下文；若存在指定输出节点，应用把其身份、节点类型、产物 Schema/版本及约束作为独立契约绑定运行。若无指定输出节点，绑定应用允许选择的类型及映射。
- 执行端口表达任务快照、可用的文件引用、输出契约、运行身份及事件/交互通道；本机 workspace 和 MCP 配置路径应留在本地适配器中。字段名和具体序列化格式尚未确定。
- 本地和云端适配器向应用提供同类运行生命周期、事件、取消、权限/问题应答和结果语义；通过能力声明告知 Agent 工具差异，不承诺不同 Agent 的工具完全相同。
- 运行内经 MCP 查询图、节点或应用产物时，查询作用域与数据版本必须绑定该运行发送时的快照；不得重新读取改挂后的实时路径并混入旧运行。快照或对应版本不可用时明确失败，不静默返回新数据；Agent 执行环境内的文件操作不受这条应用快照规则替代。

### `reply` 与应用交互

| 场景 | 目标语义 |
|---|---|
| 选题节点运行 | 返回候选结果；应用验证后生成候选节点。 |
| Agent 后已有指定输出节点 | 应用确定输出契约；Agent 返回符合该契约的结果，不能自行更改目标类型。 |
| Agent 后没有指定输出节点 | Agent 从应用允许的类型中选择；应用验证合法结果后自动创建并连接对应输出节点，关联结果，不以人工确认作为创建前置。 |
| 非法类型/Schema、错误归属、过期运行或绑定冲突 | 拒绝改图并报告原因；不得静默降级、覆盖其他节点或产生半成品。 |

`reply` 只表达结果及受限类型选择，不是通用建图命令。应用负责校验并提交节点、连接和结果；对重复请求、重连重放应幂等，对写入失败应可恢复。已有输出节点的结果覆盖/确认策略、多个后继如何选定和输出节点后是否自动执行仍待决定。

### 工具与远程边界

- Web 搜索、文件读写/删除和 Bash 由 Agent/执行环境提供；AudioUI MCP 只暴露与应用交互的能力，例如 `reply`、用户提问、必要且按运行授权的应用数据查询。现行 M2 MCP 中的通用文件读取属于旧基线，不表示目标方案继续要求 MCP 提供这些工具。
- 云端 Agent 在云端隔离工作区使用自己的工具。本地文件仅在用户选择上传后可用，下载结果也须绑定该用户和运行；不要求把通用文件操作或 Bash 转发到本机。
- 云端应用 MCP 可在云端运行，`reply` 与交互事件由远程运行服务返回桌面应用，由本地 Workflow 执行应用侧校验及写入；不能把本机 localhost MCP 直接当成云端可访问地址。需要鉴权、加密传输、运行/用户隔离、断线重连、取消、去重以及保留/清理策略，具体实现未定。

### 一次远程运行的目标时序

1. 用户在桌面端发起运行；应用从当时的图构建上下文快照，绑定输出契约和用户选择的素材。未提供的本地文件不能被云端 Agent 直接读取。
2. 桌面端向云端服务认证，并传递任务快照及经选择的文件；云端为本次运行准备隔离工作区和受限的应用交互入口，启动具备相应能力的 Agent。
3. Agent 自主使用其可用的搜索、文件与 Bash 工具；运行事件和应用问题经远程服务返回桌面端，用户应答按运行身份回传。
4. Agent 调用应用 `reply`；远程服务把结果与运行身份交给桌面端，本地 Workflow 检查授权、快照、输出契约及图状态，再执行受控提交。
5. **接收到远程消息不等于本地提交成功**：只有本地事务成功才可报告节点已创建/结果已提交。客户端断线时不得假报成功；云端如何暂存、重放或告知失败，以及具体留存期限待设计，重连后的重复投递必须幂等。
6. 运行取消、失败或终止后按经确认的数据保留策略处理云端环境与文件；运行记录、输出内容与模型用量应区分，不能以 M2 的首次输入 token 冒充完整账单。

## 1. 传输与鉴权

| 项 | 约定 |
|---|---|
| MCP 传输 | Streamable HTTP，仅监听 `127.0.0.1`，随机端口 |
| 配置 | App 在 `<exe>/data/agent/<projectId>/<runId>/mcp.json` 生成配置，使用 `--strict-mcp-config`；适配器验证 JSON，Windows host 在启动原生 CLI 前再次验证文件存在且可读；不写 OS temp |
| 鉴权 | **每运行**独立 bearer token；`Authorization: Bearer <token>` |
| 作用域 | token → runId + treeId（topic 根）+ nodeId；工具只能访问本运行目标节点与该运行路径的祖先 |
| 运行 | 每次节点执行启动一次性会话；原生 `claude.exe`，stream-json 双向，`--permission-prompt-tool stdio`；SDK 的 host 版本探测关闭，原生 CLI 版本由 `ResolveNative` 校验 |
| 权限 | `PreToolUse` 硬门禁 + `CanUseTool` 交互确认 |
| 服务器名 | `audioui`；工具名 `mcp__audioui__<tool>` |
| `alwaysLoad` | `true`，避免依赖 `ToolSearch` |

规则：

- token 每次运行重新生成，不落日志、不持久化；运行结束删除 MCP 配置。
- 禁止 `.cmd`/`.bat` 作为 CLI 路径（Windows 命令注入风险）；仅接受原生 `.exe`。
- MCP 请求体、文件读取、grep 结果、artifact 分块都有大小限制。
- 运行之间不共享 Agent 记忆；不得依赖 `--resume` 或 fork。

## 2. 适配器与运行规格

domain 不读取 Claude 文件，也不依赖 Claude 会话格式：

```go
type AgentDriver interface {
    Start(ctx context.Context, spec RunSpec) (AgentRun, error)
    // Resume 仅为未来适配器保留；M2 不调用。
    Resume(ctx context.Context, spec RunSpec, ref json.RawMessage) (AgentRun, error)
}

type AgentRun interface {
    Send(ctx context.Context, text string) error
    Interrupt(ctx context.Context) error
    Close() error
}

type RunSpec struct {
    RunID, TreeID, NodeID  string
    ChainPath              []string   // topic 根 → 目标节点
    Workspace              string
    ContextVersion         int
    ContextHash            string
    SystemPrompt           string
    InitialPrompt          string
    Tools                  []string
    MCPConfigPath, MCPURL, MCPToken string
    EventSink              func(AgentEvent)
    PermissionGate         PermissionGate
    QuestionGate           QuestionGate
    ToolPolicy             ToolPolicy
}
```

- 运行规格由 `service/run` 组装；上下文由 `service/context` 生成，`InitialPrompt` 即上下文包。
- `ContextHash` 与 `ContextVersion` 随事件与 pending 记录，用于追溯。
- 不保存 `resumeRef`；运行结束即关闭进程与 MCP。
- 适配器字段 `adapter` 记录 `claude`；更换 Agent 时重新实现 `AgentDriver`，domain 不变。
- 默认复用现有 Claude profile（`~/.claude`），以 `--setting-sources=user` 加载用户级 settings；项目/本地 settings 与项目 CLAUDE.md 不加载；MCP 由 `--strict-mcp-config` 锁定。便携 profile 记 RM-014。

## 3. 能力位

适配器启动时上报：

```go
type Capabilities struct {
    MultiTurn          bool
    Streaming          bool
    ToolPermissions    bool
    Question           bool
    ConversationRewind bool
    FileRewind         bool
    ForkAtCheckpoint   bool
    Resume             bool
}
```

| 能力 | Claude v1 | M2 |
|---|---|---|
| MultiTurn | ✅ | 运行内多轮（工具/问询） |
| Streaming | ✅ | `stream_event` / partial messages |
| ToolPermissions | ✅ | `can_use_tool` control request |
| Question | ✅（自定义 MCP 工具） | `ask_user` → QuestionCard |
| ConversationRewind | ❌ | 不提供 |
| FileRewind | ✅ | M2 不使用 |
| ForkAtCheckpoint | ❌ | 不需要；上下文按链重建 |
| Resume | ✅ | **M2 不使用**；运行之间不共享记忆 |

## 4. 工具

### 4.1 M2 可用

| 工具 | 读写 | 说明 |
|---|---|---|
| `get_chain` | 读 | 返回 runId、treeId、chainId、chainPath、目标 nodeId 与 workspace |
| `get_graph` | 读 | 仅返回当前运行路径节点摘要（id/type/title/parent/parentCandidate/locked/stale），不返回路径外节点或产物正文 |
| `get_node(nodeId)` | 读 | 只读当前运行路径节点的参数与轮次元数据（n/kind/artifactId/pendingId/runId），不含产物正文 |
| `list_node_types` | 读 | `topic` / `prompt` 的类型、参数名与产出类型 |
| `validate(kind, payload)` | 读 | 复用 `topics`/`script` 唯一 Schema 校验器，不落盘；`scene_dsl` 返回 `UNSUPPORTED_UNTIL_M3` |
| `read_artifact(artifactId, offset, limit)` | 读 | 当前运行路径祖先产物分块读取，单次 ≤16 KiB；`topics` 只返回该路径所选候选对象 |
| `reply(node_id, kind, payload, request_id?)` | 写消息 | `message` 的 payload 为 `{"text":"..."}` 并进入对话；`topics`/`script` 委托 workflow 创建待应用产物 |
| `read_file` / `list_dir` / `grep` / `stat` | 读 | 当前运行路径的逻辑叠加视图：当前节点 → 路径祖先 → exe 同级只读 `runtime/`；大小/条数限制；不暴露物理 `steps/` 路径 |
| `ask_user` | 交互 | 触发 QuestionCard，等待用户选择/输入 |

`get_graph` 和 `get_node` 必须先校验 runId/projectId/treeId/nodeId/chainPath 与当前工程图一致；两者只暴露 `chainPath` 中的节点，路径外（即使同树）统一返回 `CHAIN_SCOPE_DENIED`。路径快照与当前图不一致时 fail closed。`reply` 先校验运行活动状态：运行已结束或 node_id 是路径内非目标节点时返回 `TURN_NOT_ACTIVE`；路径外 node_id 返回 `CHAIN_SCOPE_DENIED`。运行生命周期端口须持有活动运行租约直到写操作结束，避免终止与 pending 提交竞态。

`read_artifact` 仅授权当前运行路径祖先节点任一轮次引用的 artifact；目标节点自身、兄弟、其他树或未引用的 ID 均返回 `CHAIN_SCOPE_DENIED`。授权后以轮次 `kind` 为准（不信任 CAS 首写 `meta.kind`）；同一 artifact ID 若在多个祖先引用但过滤视图不一致，拒绝读取。对 `topics` 轮次，先按路径下一个节点的 0 基 `parentCandidate` 取出单个候选对象，紧凑 JSON 序列化后再分页；不返回 `topics` 外层数组，也不得回退完整产物。`artifactId` 仍标识原始完整产物，其哈希不等于过滤后字节的哈希。缺失/越界候选返回 `CANDIDATE_INVALID`。

`offset`/`limit` 是过滤后 UTF-8 字节视图的零基字节偏移与最大字节数；`limit` 必须大于 0，超过 16 KiB 时按 16 KiB 处理。offset 必须在数据范围内且位于 UTF-8 字符边界；返回 `nextOffset` 为实际下一字节偏移，不拆分字符；limit 小到无法容纳下一个字符时返回 `ARTIFACT_LIMIT_INVALID`。`offset == totalBytes` 返回空块和 `eof=true`。`reply`/`validate` 的 `payload` 使用 MCP 原始 arguments 读取；Schema 无效但 JSON 合法的结构化 payload 进入 M2-05 校验并按原始 payload 留在 invalid pending，传输层 JSON 无法解析时由 MCP 协议拒绝。

### 4.2 后续批次

| 工具 | 批次 | 说明 |
|---|---|---|
| `create_node` / `connect` / `update_params` | 暂存 RM-016 | Agent 直接写图需要确认与审计模型 |
| `run` / `render` | M3 | 预览级自动允许 |
| `delete_*` / `overwrite` / `export` | M5 | 每次确认 |
| `apply_artifact` | 永不暴露 | 仅 UI 调用 |

原则：**MCP 返回引用与摘要，不返回大块内容**；大内容通过 `read_artifact` 分块读取。

## 5. 运行上下文与 reply 契约

### 5.1 上下文包

App 在启动运行前构建并作为首条消息注入：

| 段 | 内容 |
|---|---|
| 运行目标 | treeId（topic 根）、chainPath、nodeId、节点类型、产出类型（不放 runId，保证同输入同哈希） |
| 任务 | 目标节点当前参数；不含目标节点自身历史轮次 |
| 路径 | topic 根 → 父节点：类型、标题、参数、最新已应用轮次产物 |
| 候选 | 路径从 topic 的哪个候选端口进入，就只带该候选；不带其他候选 |
| 工作区 | topic 树 cwd、`steps/` 叠加顺序、当前路径祖先文件清单（路径 + 大小）；MCP 只读视图最低优先级为 exe 同级 `runtime/` |
| 契约 | `reply` 用法、Schema、禁止改图、大小限制、歧义用 `ask_user` |
| 元数据 | `contextVersion=1`、`contextHash`、`estimatedTokens` |

- `parentCandidate` 为 0 基候选序号（存储与 MCP 参数一致）；上下文中展示给 Agent 时显示为 1 基。
- 不进上下文：目标自身历史、兄弟分支、子节点历史、其他 topic 树、其他候选、未应用产物。
- 工作区只注入当前运行路径叠加视图的逻辑文件路径与大小，包含可见的共享 runtime 项；不自动注入任何工作区文件正文，二进制资源仅通过路径引用。清单最多遍历 5,000 个文件/目录、深度 64，超限标记 `truncated`。
- 预算 1,000,000 tokens（`contextTokenBudget`）；超预算从最远祖先逐个「全文 → 原文摘录 → 引用」降级，直接父最后；目标参数始终完整保留。仍超返回 `CONTEXT_TOO_LARGE`，附最终估算值与预算。
- 摘录含参数、最多 2,000 估算 tokens 的 UTF-8 安全原文前缀与产物哈希，不调用模型生成摘要；引用保留路径/轮次元数据、参数及产物哈希，可通过 `read_artifact` 分块读取。
- `estimatorVersion=1`：ASCII 字母、数字、下划线及连续水平空白分别按每 3 字节向上取整，ASCII 标点/换行各计 1，非 ASCII 按 UTF-8 字节数除以 2 向上取整；估算覆盖完整首条消息，包括元数据，不能保证等于实际 CLI 用量。实际 input tokens 和 `overBudget` 在 M2-07 运行完成后记录。
- `contextHash` 为去掉元数据后的 `context` 主体经 Go `encoding/json` 紧凑序列化字节的 SHA-256；`ancestorHash` 为图路径祖先、已应用产物标识及所选候选的指纹，不含工作区；`paramsHash` 为目标完整参数的指纹（详见 ADR-026）。工作区清单路径/大小变化会改变 `contextHash`，但不会触发祖先哈希冲突；同大小内容变化与清单截断之外的变化不可由清单识别。
- 上下文不含时间戳/runId；相同图、参数与工作区清单得到相同哈希。

### 5.2 reply 输入

```json
{
  "node_id": "n_b",
  "kind": "script",
  "payload": { "title": "...", "scenes": [ ... ] },
  "request_id": "req_01J..."
}
```

| 字段 | 约定 |
|---|---|
| `node_id` | 必须等于当前运行目标节点，否则 `TURN_NOT_ACTIVE` |
| `kind` | `message` / `topics` / `script`；`scene_dsl` 在 M2 返回 `UNSUPPORTED_UNTIL_M3` |
| `payload` | 结构化内容 ≤256 KiB；`message` 可为 `{"text":"..."}` |
| `request_id` | 可选，最多 128 字节；同一 runId 下相同 request id、绑定信息与原始 payload 重放时返回原 pending；内容不一致返回 `REQUEST_ID_CONFLICT` |

校验：

- `kind` 必须与节点类型匹配：topic → `topics`；prompt → `script`；不匹配返回 `NODE_KIND_MISMATCH`。
- 结构化 payload 上限 256 KiB。校验失败仍创建 `status=invalid` 的 pending，逐字节保留原文与校验错误；不能应用，不自动重试。
- `topics`：顶层对象的 `topics` 数组含 3–10 个对象；每项的 `title`、`angle`、`hook` 必须为字符串，额外字段保留。
- `script`：顶层 `title` 为字符串，`scenes` 至少一个对象；每项 `narration` 为字符串、`duration` 为 JSON 数字（允许 0），额外字段保留。
- 同一运行最多一个结构化 pending；相同 runId + request_id 且运行/节点/类型/哈希绑定与原始 payload 均相同则幂等返回原记录；相同 ID 内容不同返回 `REQUEST_ID_CONFLICT`，同运行另一个结构化结果返回 `TURN_PENDING_EXISTS`。
- 每个 topic 树最多 50 条 `pending`/`invalid`；应用或丢弃释放名额，历史索引与原文保留。
- 应用时重新计算参数与祖先哈希；`force=true` 只覆盖 `PARAMS_CHANGED`/`CONTEXT_CHANGED`，不绕过 Schema、节点类型、运行锁或内容锁。应用后节点内容与执行锁定、追加轮次并将全部后代标记为 stale；M2 的 topic/prompt 每节点最多一个已应用轮次。
- 目标节点已锁定（应用过首轮/候选）时，运行与应用均被拒绝（`NODE_LOCKED`）；M2 的 topic/prompt 每节点最多一个已应用轮次，迭代新增下游节点或分支。

Schema（M2 最小）：

- `topics`：`{"topics":[{"title","angle","hook"}]}`，3–10 条。
- `script`：`{"title","scenes":[{"narration","duration"}]}`，≥1 场景。

**reply 参数中的节点 ID 不得由 Agent 任意指定；越权返回 `CHAIN_SCOPE_DENIED`。**

## 6. ask_user 契约

输入：

```json
{
  "question": "选择哪种剪辑节奏？",
  "options": ["紧凑", "舒缓"],
  "allowOther": true
}
```

输出：`{"answer":"紧凑","via":"option"}` 或 `{"answer":"其他文本","via":"other"}`。

- App 发出 `agent:question` 事件，QuestionCard 展示选项与“其他”输入。
- 120 秒无响应返回 `{"error":"QUESTION_TIMEOUT"}`，不阻塞后续步骤。
- 同一运行同时最多一个待回答问题。

## 7. 权限档位

| 档位 | 工具/操作 |
|---|---|
| 自动允许 | `get_chain`、`get_graph`、`get_node`、`list_node_types`、`validate`、`read_artifact`、`reply(message/structured)`、`ask_user`、经服务端叠加视图校验的 `read_file`、`list_dir`、`grep`、`stat`、当前运行路径内只读 Bash allowlist |
| 交互确认 | 内置 `Write`、`Edit`、`NotebookEdit`（目标必须位于 `steps/<当前运行节点>/`）、Bash 写/解释器/未知命令 |
| 硬拒绝 | 越界路径、topic 树内的物理 `runtime/` 与 `artifacts/`、其他 topic 树、当前运行路径外的内置文件读写、明显破坏性系统命令；exe 同级 `runtime/` 仅由 MCP 叠加视图只读暴露 |

规则：

- `PreToolUse` 对所有工具执行硬门禁；`CanUseTool` 只负责交互确认。
- 确认请求带工具名、参数摘要、风险、影响路径与 runId。
- 超时 120 秒视为拒绝。
- 支持允许一次 / 本次运行允许 / 拒绝；“本次运行允许”只对非解释器、非高风险操作开放。
- 所有决策写 `<exe>/logs/audit.log`，含 runId/treeId/nodeId/chainId；允许决策的审计写失败 → fail closed。
- 内置 Read/Glob/Grep 物理路径只允许当前运行路径的 `steps/<nodeId>/`；内置 Write/Edit/NotebookEdit 只允许写当前运行节点 `steps/<nodeId>/`，并需交互确认。其他路径在 `PreToolUse` 阶段硬拒绝。
- MCP 文件读取使用 topic 树内逻辑叠加视图，按当前节点 → 路径祖先 → exe 同级 `runtime/` 查找；返回的路径保持逻辑相对路径。内置文件工具不读取 exe 外部 runtime。
- topic 树 cwd 中名为 `runtime/`、`artifacts/`、`steps/` 的物理目录为保留路径；不得将根 runtime 复制进工程工作区。
- Bash 三档：只读 allowlist 自动允许；写/解释器/未知确认；越界路径/受保护目录/破坏性命令硬拒绝。Bash 仍是协作式门禁；同树兄弟读取与复杂写目标不能可靠解析，残余风险登记 RM-017。
- M2 不是 OS 沙箱；这是协作式策略门禁。

## 8. 运行生命周期

| 状态 | 含义 |
|---|---|
| `starting` | 正在创建运行、MCP 与进程 |
| `running` | 运行进行中 |
| `succeeded` | 回合正常结束 |
| `failed` | 启动或回合失败 |
| `interrupted` | 用户中断、应用退出或重启时未完成 |
| `imported` | M1 迁移导入的只读历史 |

- 每次节点执行一个运行；M2 全局同一时间只允许一个在途运行。
- 运行中再次执行任意节点返回 `BUSY`；M5 引入队列。
- 运行/排队路径（topic 根 → 运行节点）与共享祖先只读；路径外兄弟分支可编辑。
- 运行结束即关闭进程、MCP 与 token；不保留空闲会话。
- 应用重启时，`starting`/`running` 记录标记 `interrupted`；不自动 resume。
- 运行记录全部保留；单次 transcript 上限 500 条。
- 运行失败后重新执行节点会创建新运行，旧记录保留；已锁定节点不可再次执行。
- M2 的 topic/prompt 每节点最多一个已应用轮次；继续迭代通过新增下游节点或候选端口分支。

## 9. 分支与删除（后续批次）

| 场景 | 方式 | 批次 |
|---|---|---|
| 候选端口分支 | 新运行只从该路径祖先构建上下文，并带该端口候选；不需要 fork | M2 |
| 改挂已有节点 | 移动子树，后代上下文重建；运行/排队路径禁止 | M2 |
| 删除节点 | 级联删除、取消运行、关闭进程、回收站 | M5 |

## 10. 事件映射与传输

| CLI/SDK 事件 | Domain 事件 |
|---|---|
| `system/init` | `RunStarted` |
| `stream_event` | `AssistantDelta` |
| `assistant` | `AssistantMessage` |
| `tool_use` | `ToolStarted` |
| `tool_result` | `ToolFinished` |
| `control_request(can_use_tool)` | `PermissionRequested` |
| `ask_user` MCP 调用 | `QuestionRequested` |
| `result` | `RunCompleted`（含 usage） |
| 错误/断连 | `RunErrored` |

当前运行由 Wails `RunService` 一次启动并发送首条上下文消息；`Start` 仅在 MCP/权限/driver 完成初始化并成功提交首条消息后返回。M2 同时只允许一个在途运行，竞争请求立即返回 `BUSY`。运行历史经 `ListRuns`/`GetRun` 读取；`Interrupt`、`RespondPermission` 与 `RespondQuestion` 按 runId 定位活动运行。主 GUI 仅在取得单实例锁后执行启动恢复：所有工程中遗留的 `starting`/`running` 记录转为 `interrupted`，不会恢复 Agent 会话。

### 8.1 工作台启动 RPC（非 Agent/CLI/MCP 协议）

选题卡的 Wails 调用为 `RunService.Start(nodeID, projectID)`。`projectID` 是发起请求时的活动工程标识；Workflow 在同一图命令锁内验证当前工程后再取得运行路径锁，工程不匹配则拒绝，不会把旧画布请求改投新工程。

返回值是应用层 DTO `RunStartResult`，包含可选的 `run` 与 `error`。错误包含稳定 `code` 和可读 `message`；`CONTEXT_TOO_LARGE` 另提供整数 `estimatedTokens`、`tokenBudget`，UI 必须读取结构化字段，不从文案推算数值。此 DTO 不改变 `AgentDriver`、CLI stream-json、MCP 工具或 `reply` 契约。

实际运行的 Wails 事件统一按 `run:<type>` 发布，每条事件都有工程/运行身份与单调序号：

```text
run:status           { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, status, error? }
run:message          { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, message }
run:tool             { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, tool }
run:permission       { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, permission }
run:question         { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, question }
run:error            { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, error }
run:pending          { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, pendingId }
run:request_resolved { v, seq, runId, projectId, treeId, chainId, nodeId, chainPath, type, resolvedKind, resolvedId }
```

- `projectId` 来自启动该运行时绑定的 `RunScope`，不得用 nodeId/treeId 推导。
- `chainPath` 是运行时的 topic 根 → 目标节点路径快照；链本身是派生视图。
- envelope 的 `v=1`；`seq` 每个 run 独立、单调递增，并与运行视图/待决请求的内存更新在同一同步点递增；消费者按 projectId/runId 丢弃其他工程、其他运行或重复序号的事件。
- 终态 `status` 含 `succeeded`、`failed`、`interrupted`；`error` 使用稳定错误码。`actualInputTokens` 表示首个 `AssistantMessage.Usage` 中的首次请求用量（包括缓存 token），随终态事件发送；没有该 usage 时字段缺失，不用 `ResultMessage` 多轮累计量替代。
- MCP `reply` 创建 pending 后发布 `run:pending`；权限与问题请求解决后发布 `run:request_resolved`。
- 前端接线属于 M2-08；启动/切换工程时先订阅事件并按工程代次缓冲，再取 `GetWorkspaceSnapshot` 与活动运行的 `GetRunSnapshot`，按 revision/lastSeq 丢弃旧事件并重放更新；发生缺口时暂停对应流并重拉。
- 事件 payload 版本 `v=1`。
- 运行事件不携带完整 transcript；transcript 由 `GetRun` 获取。
- `GetRunSnapshot(runId)` 返回 `{ runId, projectId, run, lastSeq, permissions, questions }`。活动运行的 `run`、`lastSeq` 和未决请求在同一把运行状态锁下读取；请求到达、解决、超时及终止清理都与事件序号原子对应。终态和历史运行返回持久化 `run`、空请求列表；进程重启后事件序号重新从新快照基准开始，不跨进程比较。
- `workspace:changed` 返回 `{ projectId, revision, reason, projectRevision }`。`reason` 取 `project`（新建/切换工程）、`graph`（图命令提交）、`pending`（待应用新增/应用/丢弃）、`run_lock`（运行路径锁取得/释放）。事件里的工程 ID 来自发生变更的命令/锁记录；不能在回调中查询“当前工程”推断归属。
- 工作台外层 `revision` 是本进程内单调递增的同步版本，覆盖工程切换、图/pending 提交及路径锁变化，不落盘；`projectRevision` 和 `snapshot.project.revision` 是持久化工程 revision。重启后以新快照为基准。
- `SelectNode` 只持久化 `activeNodeId`，不递增 revision、不广播；选中与路径高亮由前端本地更新。

### 10.1 工作台快照最小字段

`GetWorkspaceSnapshot` 返回 `{ revision, snapshot }`；若读取期间工程切换、图/pending 提交或运行锁变化，服务端重试；仍不稳定时返回可重试错误 `WORKSPACE_CHANGED`，不返回混合工程数据。无活动工程时 `snapshot.project` 为 `null`，列表为空数组。快照只含 UI 渲染所需字段，正文按需通过 `GetRun`/`GetRunSnapshot`/`GetPending` 获取：

| 字段 | 内容 |
|---|---|
| `project` | `id/name/revision/activeNodeId` |
| `nodes[]` | `id/type/title/parent/parentCandidate/position/params/rounds/locked/stale`；`rounds` 只含元数据（n/kind/artifactId/pendingId/runId） |
| `pending[]` | `id/nodeId/runId/chainId/kind/status/summary/error`；正文用 `GetPending` |
| `runs[]` | `id/nodeId/status/chainId/chainPath/contextHash/estimatedTokens/actualInputTokens/startedAt/finishedAt`；transcript 用 `GetRun` |
| `chains[]` | Go 派生叶子链视图：`id/tailNodeId/path/label/status`；`status` 取 `idle/running/pending`，不落盘 |
| `lockedNodeIds[]` | 当前运行/排队路径与共享祖先；M2 为单在途运行的路径 |

- 外层 `revision` 只在图/工程/pending/运行锁变化时递增；`snapshot.project.revision` 只由持久化图/pending 写入规则递增。`SelectNode` 不改变外层 revision。
- 快照不内联 transcript、pending 正文或 artifact 正文；前端只消费 `chains`，不复制链派生规则。
- `actualInputTokens` 在实际 usage 未知时省略，不伪装为 `0`。派生 `chains` 的 `status` 按活动运行路径和活动 pending 推导。
- 运行路径锁已取得但运行记录仍在初始化时，`runs[]` 可带临时 `starting` 摘要；运行记录保存后以完整摘要替换，启动失败则随解锁事件消失。

## 11. 错误码

| 码 | 含义 |
|---|---|
| `UNKNOWN_NODE_TYPE` | 类型不在注册表 |
| `NODE_TYPE_UNSUPPORTED` | 类型不在当前批次 |
| `SCHEMA_INVALID` | 产物不符合 Schema |
| `NODE_NOT_FOUND` | 节点不存在或已删除 |
| `NODE_LOCKED` | 节点已锁定；参数编辑或再次执行被拒绝 |
| `NODE_KIND_MISMATCH` | kind 与节点类型不匹配 |
| `REPARENT_CONFIRM_REQUIRED` | 目标已有不同父或同一父不同候选，需确认改挂 |
| `RUNNING_CHAIN_LOCKED` | 运行/排队路径上的节点只读 |
| `WORKSPACE_MOVE_FAILED` | 跨树改挂时 steps 迁移失败，已回滚 |
| `SELF_CONNECT` | 节点连接到自身 |
| `CANDIDATE_INVALID` | 父为 topic 但候选序号无效 |
| `TOPIC_ROOT_PARENT` | 试图给 topic 根设置父节点 |
| `NOT_IN_CHAIN` | 节点未接入 topic 树 |
| `CYCLE_DETECTED` | 连接会成环 |
| `CHAIN_SCOPE_DENIED` | 跨 topic 树或路径外节点访问 |
| `PATH_DENIED` | 越界或受保护路径 |
| `ANCESTOR_NOT_READY` | 祖先缺少已应用轮次 |
| `CONTEXT_TOO_LARGE` | 上下文估算超过预算 |
| `CONTEXT_CHANGED` | 祖先轮次在运行后变化 |
| `PARAMS_CHANGED` | 目标参数在运行后变化 |
| `RUN_NOT_FOUND` | 运行不存在 |
| `RUN_ALREADY_ACTIVE` | M1 遗留；M2 全局单在途统一返回 `BUSY` |
| `RUN_TIMEOUT` | 看门狗超时 |
| `TURN_NOT_ACTIVE` | 运行已结束或节点不匹配 |
| `TURN_PENDING_EXISTS` | 本次运行已有结构化 pending |
| `PENDING_NOT_FOUND` | pending 不存在 |
| `PENDING_INVALID` | Schema 无效或 pending 不可应用 |
| `PENDING_NOT_APPLICABLE` | pending 已丢弃或状态不允许应用 |
| `PENDING_ALREADY_APPLIED` | pending 已应用 |
| `PENDING_LIMIT_REACHED` | topic 树已有 50 条活动 pending |
| `PENDING_PAYLOAD_TOO_LARGE` | payload 超过 256 KiB |
| `ARTIFACT_NOT_FOUND` | 已授权的祖先轮次引用了缺失产物 |
| `ARTIFACT_CORRUPT` | 产物哈希、内容或 Schema 校验失败 |
| `ARTIFACT_OFFSET_INVALID` | artifact 偏移越界或落在 UTF-8 字符内部 |
| `ARTIFACT_LIMIT_INVALID` | artifact limit 非正数或无法容纳下一个 UTF-8 字符 |
| `OUTPUT_KIND_INVALID` | 产出类型无效 |
| `INVALID_INPUT` | MCP 工具参数缺失或格式错误 |
| `REQUEST_ID_CONFLICT` | 同一运行的 request_id 被不同请求内容复用 |
| `BASE_ROUND_CHANGED` | 旧版错误码，M2 由 `CONTEXT_CHANGED` 取代 |
| `QUESTION_TIMEOUT` | ask_user 超时 |
| `SEND_FAILED` | 指令未能送达 Agent，可重试 |
| `TURN_FAILED` | 本回合以错误结束 |
| `SESSION_ENDED` | 流意外结束；M2 标记运行失败 |
| `BUSY` | 已有在途运行 |
| `AUDIT_UNAVAILABLE` | 审计写失败，拒绝允许 |
| `TOKEN_INVALID` | MCP token 缺失/错误 |
| `MCP_CONFIG_NOT_FOUND` | 每运行 MCP 配置文件在 CLI 启动时不存在 |
| `MCP_CONFIG_UNREADABLE` | 每运行 MCP 配置文件无法读取 |
| `MCP_CONFIG_INVALID` | 每运行 MCP 配置不是有效的 MCP server JSON |
| `AGENT_START_FAILED` | CLI 已发现，但会话初始化失败 |
| `UNSUPPORTED_UNTIL_M3` | `scene_dsl` 等 M3 能力 |
| `MIGRATION_FAILED` | M1 数据迁移失败 |

## 12. 兼容性与降级

- 锁 `claude-agent-sdk-go v0.10.0`；Claude CLI 最低 `2.1.224`，启动时校验。
- Windows 只使用原生 `claude.exe`；找不到时禁用 Agent 并给出原生安装指引。
- SDK/CLI 变化时，适配器隔离，不影响 domain。
- 能力位为 false 时走降级路径；M2 不依赖 Resume/Fork。
- 默认复用现有 Claude profile；Agent 自身凭据/session 属适配器例外，应用数据仍在 exe。
- 便携 profile、完整历史回放记入 roadmap。

## 13. 安全

- MCP 仅本机；每运行 token；常量时间比较；不落日志。
- 工具只能访问绑定运行的 topic 树与当前路径节点；路径 guard 覆盖 `..`、绝对路径、UNC、盘符相对、受保护目录。
- 文件写入受白名单限制；Bash 三档策略；高风险命令不提供“本次运行允许”。
- 审计文件独立于操作日志；允许决策 fail closed。
- 不把 Claude profile 内容复制进应用数据。
- Claude 子进程通过 agent host + Job Object 管理，运行时结束与退出时回收进程树。
