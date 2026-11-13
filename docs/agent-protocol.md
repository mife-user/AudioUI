# Agent 接入协议

> 目标：让 Agent 以结构化、可校验、可替换的方式服务工作台。
> v1 适配器：Claude Code CLI（stream-json + control protocol）+ 本机 MCP HTTP。
>
> **M2-00～10 实际范围**：运行级一次性会话、App 构建上下文、只读图工具、结构化 `reply`、待应用产物；多链单在途。该范围是旧节点模型的历史基线。M2-11～13 的选定本地脚本目标见下文。

## 多上游图与版本的协议演进（U15 已验收；U16 设计已确认、实施中）

用户已确认目标图允许多个上游且无环，独立 Prompt → Agent 的有效任务可无 Topic 运行；运行时按任务/工具契约检查实际输入，不能静默丢弃或删除不兼容连接。U15 保留旧有效单路径 v2 快照；无 Topic 或 DAG 汇合使用 v3 快照，以有序输入边与必要祖先集合表示运行范围，不伪造 `chainPath`。直接输入默认全选，运行前可取消可选输入并重排；角色按节点类型确定，多个 Prompt 分段呈现，不自动裁决冲突。各输入工作区按 Topic 根和节点目录命名空间隔离，只读；目标工作区为唯一写入位置；无 Topic 运行使用工程专属 workspace。

U16 设计已确认：工程 v5 的每条 Script 来源边显式固定版本；预览不修改绑定，未绑定边不得自动选择最新版本。Agent 启动前冻结唯一输出目标；多个现有输出时必须明确选择，新建并行 Script 先在快照中预留 ID、仅于合法 reply 事务提交时创建。Topic 首批仍自动提交；重跑使用已锁定参数，合法结果进入待确认，应用后以独立批次生成 Candidate。相同 Topic 存在未决重跑 pending 时拒绝再次运行；Candidate 的来源冻结 round、artifact 与 run ID。细节见 [ADR-030](decisions.md#adr-030-多上游无环图与版本引用草案)。

v3 `ExecutionSnapshot` 保留 U15 行为；v4 快照在冻结工程 revision、所选普通边、祖先节点/边、任务顺序与工作区身份的基础上，逐边记录 Script 版本/artifact，并固定唯一输出目标。run-scoped MCP 只授权该冻结子图和列明的 artifact；本地 Read/Glob/Grep 仅授权被选输入祖先 steps 与 exe 同级只读 `runtime/`，写入只限运行目标工作区。U16 不改变已验收的 `reply` Schema、事务幂等和权限确认语义；原单路径及 U15 行为需要回归。视频工具/产物也尚未接入。

**U14 过渡方案（已通过用户验收）**：暂不扩展运行协议；仅在所有路径输入及绑定输出均满足旧单路径契约时构建既有冻结快照。无 Topic、多输入、无效类型组合或输出歧义在启动前明确拒绝，不丢弃输入、不启动上游；文件与 MCP 授权不扩大。新图下的结果事务及恢复校验真实边、候选来源和冻结输出绑定。多输入快照与工作区协议已由 U15 实现并通过验收；逐边版本属于 U16；详见 [ADR-030](decisions.md#u14-实施与运行过渡方案已通过用户验收)。

## M2-11～13 本地脚本目标协议（设计已验收；分阶段实现）

> 本节是用户验收的 RM-018 本地脚本子范围设计。M2-11B 与 M2-11C（发送快照、上下文和运行授权）均已验收；M2-11D 受限结构化结果提交已于 2026-11-07 通过用户验收；M2-12A 前端工作台实现已于 2026-11-08 通过用户验收；M2-12B 前端确认/版本历史与叶子链已于 2026-11-08 通过用户验收（真实浏览器交互未执行）；M2-13 已于 2026-11-08 通过用户验收。M2-00～10 实际协议保留在下方章节；云端仍属 RM-019，视频仍属 RM-020。

> **M2-11C 实现状态（已于 2026-11-07 验收）**：`BeginRun` 可运行 Agent 并在同一工作流锁内冻结 Agent 路径、候选来源、Prompt 任务/约束、输出绑定基准和输出契约；Prompt 不能独立运行。路径祖先与指定 Script 输出受运行锁保护，路径外兄弟可编辑。上下文读取单个 Candidate、完整当前 Prompt 任务及冻结 Script 版本，不要求中间 Prompt 有 applied round；MCP 图/产物查询使用快照授权，不回退实时图。run-scoped MCP 不注册通用文件工具；本地 Agent 内置文件工具读取祖先与 exe 同级只读 `runtime/`、写入当前 `steps/`。

> **M2-11D 实现状态（已验收，2026-11-07）**：结构化 `reply` 经过活动运行租约与冻结输出契约校验；合法 topics 原子生成 Candidate，合法 Script 无绑定时建节点、空绑定时写首版、已有版本时进入绑定基准的待确认追加流程。无效结构化 payload 保留有界原文；同运行结果重放幂等，版本冲突不受 `force` 绕过。自检范围和未覆盖项见 `docs/dev/evidence/m2-11d.md`。

### 发送快照

本地 App 每次启动 Agent 前按同一工程快照验证图路径、候选来源、提示词和输出绑定，并生成不可变运行快照。概念字段至少包括：

| 字段 | 语义 |
|---|---|
| `runId` / `projectId` / `treeId` / `agentNodeId` | 本次运行与图作用域；reply 的 `node_id` 必须等于作用域内 Agent 节点，不代表输出节点 |
| `chainPath` | topic root 到 Agent 的发送时节点路径 |
| candidate source | 每个 candidate 对应的 `artifactId` 与 0 基 `candidateIndex`，不得以实时图替换 |
| prompt task | 路径内 prompt 的完整任务与约束；目标参数不得被预算截断 |
| output binding | 唯一 Script 目标的节点 ID、模式（既有节点或预留新节点）及发送时基准版本；新目标 ID 在合法结果提交前不创建 |
| output contract | `kind=script`、Schema version 1；topic 运行则为 `kind=topics` |
| artifact authorization | 当前路径允许读取的不可变产物版本；运行时读取不能回退到改挂后的实时版本 |

快照不含依赖本机绝对路径的通用字段。活动运行锁定路径和输出绑定；终态后改挂影响下一次运行，不改既有运行和结果来源。运行之间不共享 Agent 记忆。

### `reply` 与结果提交

M2-11～13 仅使用 `topics` 和 `script` 两种现有输出 Schema，均经唯一的 `ValidateStructuredPayload` 校验。Agent 不选择输出节点 ID，也不调用通用图写工具；目标由应用从运行快照解析。

| 运行目标 / 输出状态 | 处理语义 |
|---|---|
| `topic` 首批 + 合法 `topics` | 原子保存不可变 topics artifact 并生成所有 candidate 节点；完成后锁定 topic，不要求逐候选人工应用 |
| `topic` 重跑 + 合法 `topics` | 创建带冻结轮次/参数来源的待确认批次；确认时原子追加 Topic round 与独立 Candidate，丢弃不改旧批次；存在未决批次时不能再次重跑 |
| `agent` + 运行前选择新建输出 | 合法 `script` reply 原子创建快照预留的直接 Script 子节点、首个版本与运行来源；失败或无效结果不留下空节点 |
| `agent` + 运行前选择既有空 Script | 合法 `script` reply 自动写入该节点首个版本 |
| `agent` + 运行前选择已有版本的 Script | 合法 `script` reply 进入绑定输出节点和基准版本的待确认记录；确认时重验版本并追加，取消保持原版本不变 |
| `agent` + 多个可选 Script 输出 | 启动前必须明确选择既有目标或新建并行目标；选择结果写入运行快照 |
| 非法 Schema/kind、越权或过期运行、目标/契约不匹配 | 不改图、输出版本或 artifact 关联；返回稳定错误并保留有界诊断证据 |

新增/追加输出不得自动触发下游运行。所有节点、关系、artifact、运行来源与幂等身份由应用命令层可恢复地提交。重复同身份同内容返回既有结果；同一运行不同内容/身份冲突明确拒绝。确认结果必须验证绑定输出及基准版本，不能以旧 `force` 绕过。

MCP `reply` 的 `node_id` 指当前 topic 或 Agent 运行目标；Agent 的脚本输出节点由应用解析，不可由 Agent 通过 `node_id` 指定。目标契约只允许 `script@1`；`scene_dsl`/视频类 kind 不属于本阶段。

### 工具与本地权限边界

- 本地 Agent 的搜索、文件读写/删除和 Bash 能力由 Agent 执行环境提供；应用 MCP 不再作为通用文件代理，只负责受运行授权的应用交互与必要的图/产物快照查询。
- 文件读取/写入的路径边界不比当前更宽：读范围受当前路径祖先与共享 `runtime/` 只读视图约束，写入限当前运行节点增量目录；其他 topic、越界、保留目录硬拒绝。Bash 仍是 RM-017 的协作式门禁残余风险，不宣称 OS 沙箱。
- 应用 MCP 只监听 `127.0.0.1`，每次运行独立 token，并绑定 `runId/projectId/treeId/agentNodeId/chainPath`；活动运行租约覆盖图/产物查询与 reply 提交。
- 本阶段不提供云端 Agent、文件上传或远程 MCP；相关需求仍按 ADR-028/RM-019 评估。

### 错误码草案

| code | 场景 |
|---|---|
| `NODE_CONNECTION_INVALID` | 连接组合不在 M2-11 类型矩阵 |
| `OUTPUT_BINDING_CONFLICT` | Agent 已绑定另一个直接输出，或 reply 不匹配发送时绑定 |
| `OUTPUT_KIND_MISMATCH` | reply kind 与冻结输出契约或目标节点类型不匹配 |
| `OUTPUT_VERSION_CONFLICT` | 确认时脚本目标的基准版本已变化 |
| `REQUEST_ID_CONFLICT` | 同一 request ID 被复用于不同结果 |
| `RUN_RESULT_CONFLICT` | 同一运行收到不同身份/内容的结构化结果 |
| `RUN_PENDING_CONFLICT` | 同一运行记录了不同的 pending 身份 |
| `PENDING_PAYLOAD_TOO_LARGE` | reply 原始 payload 超过持久化上限 |
| `UNSUPPORTED_VERSION` | 工程格式版本不可用 |

继续复用 `SCHEMA_INVALID`、`OUTPUT_KIND_INVALID`、`TURN_NOT_ACTIVE`、`CHAIN_SCOPE_DENIED`、`NODE_LOCKED`、`BUSY`、`REQUEST_ID_CONFLICT` 等既有错误码。错误由 Go 服务端产生，UI 以稳定 code 呈现。

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
| `get_graph` | 读 | 返回发送快照中的路径节点摘要及冻结输出绑定；不返回路径外节点或产物正文 |
| `get_node(nodeId)` | 读 | 只读快照路径节点参数；旧模型可返回轮次元数据，v2 Script 节点返回快照中的最新版本信息；不含产物正文 |
| `list_node_types` | 读 | `topic` / `candidate` / `prompt` / `agent` / `script` 的类型、参数名与输出类型 |
| `validate(kind, payload)` | 读 | 复用 `topics`/`script` 唯一 Schema 校验器，不落盘；`scene_dsl` 返回 `UNSUPPORTED_UNTIL_M3` |
| `read_artifact(artifactId, offset, limit)` | 读 | 当前运行路径祖先产物分块读取，单次 ≤16 KiB；`topics` 只返回该路径所选候选对象 |
| `reply(node_id, kind, payload, request_id?)` | 写消息/结果 | `message` 进入对话；v2 topic 的合法 `topics` 自动生成 Candidate；Agent `script` 按冻结绑定自动建节点、写入空输出首版，已有版本时生成待确认追加结果。Schema 无效时保留 invalid 原文；运行身份、契约或目标不匹配时拒绝，不提供 Agent apply/写图工具 |
| `read_file` / `list_dir` / `grep` / `stat` | M1 legacy MCP | 只在非 run-scoped 的 M1 直接工作区注册；M2 运行不通过 MCP 代理通用文件操作 |
| `ask_user` | 交互 | 触发 QuestionCard，等待用户选择/输入 |

`get_graph` 和 `get_node` 以运行保存的 `ExecutionSnapshot` 为唯一授权数据，不查询 `Project.Current` 替代发送时图；两者只暴露快照路径节点，路径外（即使同树）统一返回 `CHAIN_SCOPE_DENIED`。`get_graph` 可返回快照中绑定的 Script 输出标识与基准版本。`reply` 先校验运行活动状态：运行已结束或 node_id 是路径内非目标节点时返回 `TURN_NOT_ACTIVE`；路径外 node_id 返回 `CHAIN_SCOPE_DENIED`。M2-11D 的结构化提交在活动运行租约内完成，topics/script 只能按冻结契约进入 workflow 原子提交；已有 Script 确认时复核基准版本。运行生命周期端口须持有活动运行租约直到查询或写操作结束，避免终止与调用竞态。

`read_artifact` 仅授权发送快照中明确引用的路径祖先 artifact；目标节点自身、兄弟、其他树或未引用的 ID 均返回 `CHAIN_SCOPE_DENIED`。candidate 产物只返回 `CandidateSource` 指定的单个 topics 对象；Script 仅授权快照中的不可变版本。授权 kind 以快照引用为准，不信任 CAS 首写 `meta.kind`；同一 artifact ID 若在多个祖先引用但过滤视图不一致，拒绝读取。缺失或越界候选返回 `CANDIDATE_INVALID`，缺失脚本版本明确失败，不退回实时图。

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

> M2-00～10 旧模型按 applied round 构建上下文；M2-11C 的 Agent 运行额外使用 `ExecutionSnapshot`：Candidate 读取来源 artifact 的单个索引、当前 Prompt 任务/约束完整保留、祖先 Script 读取冻结的不可变版本。v2 路径不要求每个 Prompt 都有旧式 applied round；本地文件工具边界见第 7 节。

App 在启动运行前构建并作为首条消息注入：

| 段 | 内容 |
|---|---|
| 运行目标 | treeId（topic 根）、chainPath、nodeId、节点类型、产出类型（不放 runId，保证同输入同哈希） |
| 任务 | Topic 运行使用目标节点参数；Agent 运行使用路径内当前 Prompt 的完整任务与约束，不含目标自身历史 |
| 路径 | 旧模型为最新 applied round；v2 路径按快照呈现节点身份、单个 Candidate 对象与祖先 Script 不可变版本 |
| 候选 | v2 Candidate 按 `CandidateSource.artifactId/index` 读取；旧模型 `parentCandidate` 为 0 基序号。只带所选候选，不带其他候选 |
| 工作区 | topic 树 cwd、`steps/` 叠加顺序、路径清单（路径 + 大小）；M2 内置工具读祖先与 exe 同级只读 `runtime/`，写当前节点 `steps/` |
| 契约 | `reply` 用法、Schema、输出绑定/基准版本、禁止改图、大小限制、歧义用 `ask_user` |
| 元数据 | `contextVersion=1`、`contextHash`、`estimatedTokens` |

- v2 路径中的候选索引来自 `CandidateSource.index`，存储为 0 基；上下文展示候选序号时为 1 基。旧 `parentCandidate` 只适用于旧图回归。
- 不进上下文：目标自身历史、兄弟分支、子节点历史、其他 topic 树、其他候选、未应用产物。
- 工作区只注入当前运行路径叠加视图的逻辑文件路径与大小，包含可见的共享 runtime 项；不自动注入任何工作区文件正文，二进制资源仅通过路径引用。清单最多遍历 5,000 个文件/目录、深度 64，超限标记 `truncated`。
- 预算 1,000,000 tokens（`contextTokenBudget`）；超预算只降级祖先产物，从全文到摘录再到引用；当前 Prompt 任务与约束始终完整保留。仍超返回 `CONTEXT_TOO_LARGE`，附最终估算值与预算。
- 摘录含参数、最多 2,000 估算 tokens 的 UTF-8 安全原文前缀与产物哈希，不调用模型生成摘要；引用保留路径/轮次元数据、参数及产物哈希，可通过 `read_artifact` 分块读取。
- `estimatorVersion=1`：ASCII 字母、数字、下划线及连续水平空白分别按每 3 字节向上取整，ASCII 标点/换行各计 1，非 ASCII 按 UTF-8 字节数除以 2 向上取整；估算覆盖完整首条消息，包括元数据，不能保证等于实际 CLI 用量。实际 input tokens 和 `overBudget` 在 M2-07 运行完成后记录。
- `contextHash` 为去掉元数据后的 `context` 主体经 Go `encoding/json` 紧凑序列化字节的 SHA-256；旧模型 `ancestorHash` 为 applied round 与所选候选指纹，v2 为冻结路径身份和产物版本指纹；Agent `paramsHash` 覆盖当前 Prompt 任务、Agent 参数、输出绑定与契约。工作区清单路径/大小变化会改变 `contextHash`，但不会触发祖先哈希冲突。
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
| 自动允许 | run-scoped 应用工具 `get_chain/get_graph/get_node/list_node_types/validate/read_artifact/reply(message)/ask_user`；受 RunGuard 限制的内置只读 `Read`/`Glob`/`Grep`；当前路径内只读 Bash allowlist。M1 legacy MCP 文件工具只在直接工作区模式使用 |
| 交互确认 | 内置 `Write`、`Edit`、`NotebookEdit`（目标必须位于 `steps/<当前运行节点>/`）、Bash 写/解释器/未知命令 |
| 硬拒绝 | 越界路径、topic 树内物理 `runtime/` 与 `artifacts/`、其他 topic 树、当前运行路径外的内置文件读写、明显破坏性系统命令；exe 同级 `runtime/` 仅允许内置工具读取 |

规则：

- `PreToolUse` 对所有工具执行硬门禁；`CanUseTool` 只负责交互确认。
- 确认请求带工具名、参数摘要、风险、影响路径与 runId。
- 超时 120 秒视为拒绝。
- 支持允许一次 / 本次运行允许 / 拒绝；“本次运行允许”只对非解释器、非高风险操作开放。
- 所有决策写 `<exe>/logs/audit.log`，含 runId/treeId/nodeId/chainId；允许决策的审计写失败 → fail closed。
- 内置 Read/Glob/Grep 的物理路径限于运行路径祖先及 exe 同级 `runtime/`（只读）；内置 Write/Edit/NotebookEdit 只允许写当前运行节点 `steps/<nodeId>/`，并需交互确认。其他路径在 `PreToolUse` 阶段硬拒绝。
- run-scoped MCP 不代理通用文件操作；M1 legacy MCP 文件工具仍使用独立直接工作区视图，不作为 M2 权限路径。M2 内置 `runtime/` 读取由 RunGuard 约束，不提供 runtime 写入。
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
