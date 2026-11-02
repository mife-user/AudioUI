# 架构设计

> 本文面向开发。产品概念与流程见 [product.md](product.md)；Agent 细节见 [agent-protocol.md](agent-protocol.md)。

## 1. 目标与约束

| 目标 | 约束 |
|---|---|
| 本地、绿色、单 exe | 不写 AppData；不要求管理员权限 |
| 确定性渲染 | 画面是时间 `t` 的纯函数 |
| Agent 可替换 | domain 不出现供应商专有名词 |
| 可并发 | 多链路、多运行、队列 |
| 可恢复 | 工程原子写入；运行历史可回看；未完成运行标记中断 |

## 2. 技术栈

| 层 | 选型 | 备注 |
|---|---|---|
| 桌面壳 | Wails v3.0.0（锁定） | 系统 WebView2 |
| 后端 | Go 1.26+ | 单主进程 + Agent host 辅助进程 |
| 前端 | React 19 + TypeScript + Vite | |
| 画布 | React Flow | DOM 节点 |
| 渲染 | Canvas2D | `draw(t)` 纯函数 |
| 编码 | WebCodecs + mediabunny | H.264 + AAC |
| 存储 | JSON + 内容寻址 | 无数据库 |
| 日志 | zap + lumberjack | UI 日志面板 |
| TTS | edge-tts | 可替换适配器 |

## 3. 分层

```
presentation  Wails 窗口 / React 画布 / 渲染引擎
      ↓
handler       project / workflow / run / artifact / render / media
      ↓
service       project / workflow / run / context / render / media
      ↓
domain        entity / valueobject / event / port
      ↑
infra         json / claudecli / edgetts / renderer-bridge
```

依赖规则：

- domain 不 import 任何 `internal` 实现包。
- service 只依赖 domain。
- infra 实现 domain 端口。
- handler 只做参数转换与错误映射。

## 4. 模块

| 模块 | 职责 |
|---|---|
| `domain/entity` | Project、Graph、Node、Round、Run、Pending、Artifact；Chain 为派生视图，不持久化 |
| `domain/valueobject` | NodeType、Hash、OutputKind、CandidateIndex、ChainPath、ContextVersion、TokenBudget、SceneDSL、Theme、EnvironmentInfo、LogEntry |
| `domain/event` | 领域事件 |
| `domain/port` | ProjectStore、ProjectSession/ProjectAccess、RunStore、PendingStore/Provider、ArtifactStore、ArtifactStoreProvider、ContextHasher、ConfigStore、M1Migrator、WorkspaceMover、WorkspaceReader、RunWorkspaceReader、GraphReader、ArtifactReader、ReplySink、RunLifecycle、TurnRunner、AgentDriver、RunToolServerFactory、ScopedPathResolver、EventBus、PermissionGate、QuestionGate、Media、CLIProbe、WebView2Probe、LogTailer |
| `service/project` | 工程生命周期、迁移、自动保存 |
| `service/system` | M0 环境自检、最近日志聚合 |
| `service/run` | 运行生命周期、一次性 Agent 会话、TurnGate、每运行 MCP/token、transcript |
| `service/context` | 路径上下文、候选解析、token 估算与预算、哈希、工作区清单 |
| `service/permission` | 三档权限、120s 超时、allow-run、审计 |
| `service/workflow` | 图命令、自由改挂、树校验、运行锁、待应用产物、轮次与锁定；提供 run-scope 图/节点/产物读取与 pending 创建适配 |
| `service/render` | 渲染任务、轮次、缓存 |
| `service/media` | TTS、音频探测、导出 |
| `infra/media/edgetts` | TTS 适配器 |
| `infra/renderer/bridge` | 前端渲染器桥 |
| `infra/eventbus/inproc` | 进程内事件总线 |
| `infra/paths` | 绿色根目录、目录创建与写权限探测 |
| `infra/environment` | Claude CLI、WebView2 探测 |
| `infra/logging` | zap + lumberjack、日志 tail |
| `infra/winmsg` | Wails 启动前的原生错误提示 |
| `infra/agent/claudecli` | Claude SDK 适配、原生 CLI、一次性运行、工具注入、事件映射 |
| `infra/agent/mcp` | HTTP MCP 服务、每运行 token、路径范围只读图工具、结构化 reply、artifact 分块读取 |
| `infra/agenthost` | Windows stdio 代理、隐藏窗口、Job Object |
| `infra/persistence` | `project.json`、`runs.json`/`runs/`、`pending.json`、`pending.txn.json`、artifact、`config.json`、迁移 |
| `infra/eventbus` | 预留；M2 的 Wails 事件桥仍在 `internal/app/eventbus.go` |
| `infra/audit` | 独立 audit.log（含 runId/treeId/nodeId/chainId） |
| `infra/workspace` | topic 树路径叠加读取（当前节点 → 祖先 → 根 `runtime/`）、节点写作用域、list/grep/stat |

## 5. 进程与并发

- 单主进程；前端在 WebView。每次 Agent 运行通过同一 exe 的 host 辅助进程管理原生 CLI（隐藏窗口 + Job Object），不参与应用状态。
- 每次节点执行一个一次性运行；运行与该运行路径绑定，运行之间不共享 Agent 上下文。
- M2 全局同一时间只允许一个在途运行；M5 并发上限 3，按在途运行计数，队列全局 FIFO，可取消、停止、调整顺序。
- 同一节点同一时间只允许一个运行；运行/排队路径（topic 根 → 运行节点）与共享祖先只读，路径外分支可编辑。
- 链 = topic 根到末端节点的派生路径；分支产生多条共享前缀的链。
- 跨树改挂移动子树 `steps` 并同时检查源/目标树运行锁；迁移失败回滚。`service/workflow` 只调用 `WorkspaceMover`，实际目录移动与回滚在 `infra/workspace`；图保存失败也会回滚已移动的 steps。
- 渲染任务串行或按资源上限并发；导出排队。

## 6. 数据流

### 6.1 Agent 生成

```
节点「执行」
  → ContextService 从 topic 根到父构建路径上下文（候选端口解析 + 最新轮次 + 任务 + 工作区清单 + 契约）
  → RunService 获取全局 TurnGate，记录 chainPath，启动一次性 Agent 会话
  → 适配器解析 stream-json
  → reply(kind) 经 Workflow 校验后写入 pending（无效也保留原文）
  → UI 提示待应用
  → 用户点击应用
  → Workflow 校验 ancestorHash/paramsHash，写 artifact、追加轮次、锁定、下游 stale
```

M2-04 的 `service/context.Builder` 从工程快照推导真实路径，并要求 `RunWorkspaceReader.Scope()` 与工程、树、目标和路径一致；祖先产物只取最新已应用轮次及对应 topic 候选。上下文只包含工作区文件清单，不读取文本或二进制文件正文。预算降级、三种哈希和清单限制见 ADR-026。`InputHashes` 不读取工作区，供后续待应用校验复用；`ContextEstimate.WithActualInputTokens` 计算实际用量是否超出本次预算，运行级采集与存储由 M2-07 实现。

### 6.2 渲染

```
render 节点
  → 组合节点的叠加视图
  → SceneDSL 解释
  → Canvas2D draw(t) 逐帧
  → WebCodecs 编码
  → MP4 上传/落盘
```

### 6.3 导出

```
export 节点
  → 选择视频 + 音频 + 字幕
  → 分段/整片编码
  → Go 写入 artifacts
  → 生成 manifest.json
```

### 6.4 图命令层

```
UI / handler
  → service/workflow 串行命令（AddNode / Connect / MoveNode / UpdateNodeParams / SelectNode）
  → 从 ProjectSession 取快照副本
  → 校验：单父树、无环、自连、topic 根、候选端口、锁定与运行路径
  → Connect 需要改挂时确认；跨树先移动子树 steps（可回滚）
  → 成功后 revision+1（SelectNode 除外）并原子保存 project.json
  → OnChange(revision, "graph")；SelectNode 不广播
```

- 图写入只在 `service/workflow`，handler 只做参数转换；`ProjectSession.Save` 是唯一持久化入口。
- `parentCandidate` 为 0 基；候选数量从父 topic 最新已应用 `topics` 产物读取，不复制到 project.json。
- `AcquireRunLock` 记录 topic 根 → 运行节点的真实父子路径；路径节点与共享祖先只读，路径外分支可编辑。`EnsureExecutable` 拒绝内容锁定、运行路径与草稿节点。
- `SelectNode` 只写 `activeNodeId`，不递增 revision、不触发 `OnChange`。
- 链由 `entity.Graph.LeafChains` 派生（`c_<tail>`），不落盘；快照在 M2-08 接入。

## 7. 存储

```
<exe>/
├── data/
│   ├── config.json                    # lastProjectId、contextTokenBudget
│   ├── projects/<id>/
│   │   ├── project.json               # 元信息、图、节点、轮次（链为派生视图）
│   │   ├── pending.json               # 待应用索引
│   │   ├── pending.txn.json            # pending 跨文件事务意图与恢复点
│   │   ├── pending/<pendingId>.json   # 原始结构化输出（含无效）
│   │   ├── artifacts/<sha256>/        # 应用后的不可变产物
│   │   ├── runs.json                  # 运行索引（含 chainPath）
│   │   ├── runs/<runId>.json          # 上下文/预览 + transcript
│   │   └── trees/<rootTopicId>/steps/<nodeId>/  # topic 树 cwd 与节点增量
│   ├── agent/<projectId>/<runId>/mcp.json    # 每运行 MCP 配置
│   ├── backup/m1/                     # M1 会话与工作区备份（原文件不移动）
│   ├── workspace/chain_default/       # M1 遗留（只读备份）
│   └── cache/
├── runtime/                           # exe 同级，随包共享只读运行时资源
├── logs/
│   ├── app.log
│   └── audit.log                      # 权限审计（runId/treeId/nodeId/chainId）
└── webview2/                          # WebView2 用户数据
```

共享只读运行时固定为 exe 同级 `runtime/`，随绿色包分发；应用不创建或修改该目录，MCP 逻辑读取视图把它作为最低优先级。`data/runtime/` 不使用（RM-013 已按用户决策纳入 M2-03）。

M2 收口后 `sessions.json` 不再写入；M2 开发期 M1 会话服务仍会读取/写入它，迁移不删除原文件。M2 首次启动把当时的 `activeChainId` 导入为只读运行。Agent profile（默认 `~/.claude`）是适配器例外，不属于应用数据。

- 原子写入：临时文件 + `Sync` + 重命名；新建工程与迁移先在 `data/projects/` 下的隐藏 staging 目录（`.creating-*` / `.m1-migration-*`）写完整目录，再整体重命名到 `projects/<id>`。
- 工程级锁：单实例锁；所有写命令按 `workflow.mu → ProjectSession` 操作锁排序。`ProjectSession.WithExclusive` 与 `New/Open` 共用操作锁，pending 多文件事务期间工程不能切换。
- Schema 版本：`project.json`、`pending.json`、`pending.txn.json`、`runs.json` 带 `version`；`config.json` 无版本，缺省预算在读取时归一为 1,000,000。
- 运行索引/详情：`runs.json` 只存摘要；`runs/<runId>.json` 存 transcript 与上下文预览。先写详情再写索引；索引引用缺失详情视为损坏，孤立详情不进入列表。
- pending 原文与索引分离；创建/应用/丢弃先原子写 `pending.txn.json` 和必需原文/CAS，再保存带目标 revision 的 `project.json`，最后更新 `pending.json` 并删除日志。工程加载/打开按 base/target revision 回滚未提交事务或幂等重放已提交事务；日志损坏、revision 无法判定或恢复失败时拒绝暴露工程。原文上限 256 KiB，无效 Schema 也保留原文；`pending` 与 `invalid` 计入每树 50 条上限，应用/丢弃释放名额。
- 结构化产物规范化使用保留 `json.Number` 精度的确定性 JSON；payload SHA-256 为 CAS ID。`Round.Kind` 是当前产物种类的授权真相，CAS `meta.json` 仅为首次写入元数据；元数据种类不一致时按轮次种类校验 payload。
- `contextTokenBudget` 默认 1,000,000；上下文估算与实际 usage 记入 run。
- 工程 ID、节点 ID、运行 ID、pending ID 只使用 `[a-z0-9_]`，由程序生成；迁移使用确定性 ID。

### 7.1 M1 迁移

- 触发：`data/sessions.json` 存在，且完成标记 `data/backup/m1/migration.json` 尚未写入。
- 产物：默认工程与 topic 根均命名「M1 迁移」；只导入 `activeChainId` 的 transcript，状态 `imported`、`chainPath=[root]`，不导入其他链、不 resume。
- 工作区：旧 `data/workspace/chain_default/` 复制到 `projects/p_m1_migration/trees/<root>/steps/<root>/`。
- 备份：`sessions.json` 与旧 workspace 复制到 `data/backup/m1/`；原文件不移动、不修改，M1 会话服务在迁移后仍可读取。
- 失败：备份、staging 或重命名任一步失败返回 `MIGRATION_FAILED`，清理 staging，不创建半成品工程；损坏 `sessions.json` 不重建。
- 重试：失败后下次启动重新执行；工程目录已存在但标记缺失时补写标记，不重复导入；标记存在而工程被删除时不再从旧 `sessions.json` 重建。config 的 `lastProjectId` 为空时指向迁移工程。

## 8. 叠加视图

topic 树 cwd = `data/projects/<id>/trees/<rootTopicId>/`；MCP 逻辑读取顺序：当前节点 `steps/<node>` → 路径祖先 → 根 `runtime/`。内置 Read/Glob/Grep 仅能读取当前运行路径的实际 `steps/<nodeId>/`；需要逻辑合并与 runtime 回退时使用 MCP 只读工具。

- 同一 topic 树内分支共享工作区；读取按当前运行路径叠加；其他 topic 树不可读。
- MCP `read_file` / `list_dir` / `grep` / `stat` 使用逻辑只读视图：当前节点 → 实际路径祖先 → exe 同级 `runtime/`；同名条目由更高优先级层遮蔽，list/grep 只返回合并后的可见路径。
- MCP 路径为相对逻辑路径；越界、兄弟节点、`steps/` 物理目录、`artifacts/`、`runtime/` 等保留目录名与 reparse point/junction 拒绝；读取、列表、grep 都有大小/条数上限。
- 内置 Read/Glob/Grep 的实际文件路径只允许当前运行路径上的 `steps/<nodeId>/`；Write/Edit/NotebookEdit 只允许写 `steps/<当前运行节点>/`，并保留交互确认。
- Bash 仍为协作式门禁，不能可靠解析同树兄弟读取与所有写目标，残余风险登记 RM-017；其他 topic 树、越界与受保护路径硬拒绝。
- 渲染前组合成临时构建目录，不污染其他树。

## 9. Agent 桥接

- 每次运行启动本机 HTTP MCP 服务，`127.0.0.1:0`，每运行独立 token（绑定 runId/treeId/nodeId）；结束即删配置。
- 使用 Claude 适配器：`claude-agent-sdk-go v0.10.0` + 原生 `claude.exe` + `--strict-mcp-config`；不使用 SDK 的临时 MCP/stderr 文件。
- Windows 通过 agent host 代理启动原生 CLI，`CREATE_NO_WINDOW` + Job Object 回收进程树。
- 运行级会话：App 构建上下文，首条消息注入；不依赖 Agent 记忆，不使用 resume/fork；Adapter 细节留在 `infra/agent/claudecli`。
- `RunSpec` 注入 M2 工具集与系统提示；`AgentRun` 表示运行内会话，不硬编码 M1 工具列表；MCP 工具按 runId/projectId/treeId/nodeId/chainPath 绑定；`get_graph`/`get_node` 只暴露路径快照，`read_artifact` 按祖先轮次授权并对 topic 产物过滤当前候选。
- `RunLifecycle` 为工具调用提供活动运行租约；pending 创建期间运行终止等待该操作完成。实际 run manager 接线在 M2-07。
- 权限由 `PreToolUse` 硬门禁 + `CanUseTool` 交互确认；审计独立 `logs/audit.log`，含 runId/treeId/nodeId/chainId。
- 前端渲染器通过 `RendererPort` 桥接：事件下发任务，完成回调。
- 能力位为 false 时走降级路径；M2 不依赖 Resume。

## 10. 安全与权限

| 面 | 措施 |
|---|---|
| 网络 | 仅 127.0.0.1；随机端口；每运行 token 鉴权 |
| 路径 | 读当前路径祖先叠加；写当前运行节点；其他 topic 树拒绝；产物只由 UI 应用 |
| Bash | 三档：自动允许 / 需要确认 / 直接拒绝 |
| 工具 | MCP 工具按档位确认；高风险每次确认 |
| 日志 | 本地；`logs/audit.log` 记录权限决策，允许失败即拒绝 |
| Agent profile | 默认复用 `~/.claude`（`--setting-sources=user` 读取用户级凭据/hooks）；项目级 settings 不加载；不在应用内复制凭据；便携 profile 记 RM-014 |
| 隐私 | 无应用遥测；Claude 请求由 CLI 自身发出 |

## 11. 绿色发布

- `os.Executable()` 定位根目录；开发用 `AUDIOUI_HOME` 覆盖。
- 首次启动创建 `data/`、`logs/`、`webview2/`，逐目录探测写权限。
- WebView2 用户数据目录重定向到 `webview2/`；启动前检测运行时，缺失时原生提示。
- 目录不可写时报错，不写 AppData。
- 单实例锁；二次启动 Restore/Show/Focus 已有窗口。
- zap + lumberjack 写入 `logs/app.log`；最近日志由 tail 接口读取。
- 版本从 `VERSION` 经 Taskfile 注入 `main.version`，关于入口与 `--version` 显示。
- 发布 zip：exe + runtime + docs + checksums。

## 12. 失败处理

| 失败 | 处理 |
|---|---|
| CLI 缺失 | 禁用 Agent，提示安装；M0 显示横幅，不阻止启动 |
| WebView2 缺失 | 原生中文提示与下载地址，退出；不写 AppData |
| 目录不可写 | 原生中文错误，退出；不回退 |
| 日志写入失败 | 启动失败并提示；不静默 |
| 运行中断（重启/退出） | 标记 `interrupted`，保留历史，重新执行节点 |
| M1 迁移失败 | 保留原 `sessions.json` 与 workspace，显示错误与备份路径，不自动重建 |
| 工程文件损坏 | 明确报错并保留原文件；不静默重建或覆盖 |
| 渲染失败 | 保留旧预览，记录日志 |
| 写盘失败 | 回滚临时文件，保持旧状态 |
| MCP 断开 | 本次运行失败；重新执行节点启动新运行 |

## 13. 测试策略

- domain：纯单元测试。
- M0：路径解析/创建、CLI 探测、WebView2 探测、日志 tail、版本一致性；`--self-check` 无 GUI 自检。
- M1：路径 guard、权限三档/超时/审计、MCP token/工具、事件映射回放、原生 CLI 解析、Windows host。
- M2：工程读写/迁移、树与命令层、自由改挂与子树移动（含跨树迁移与回滚）、候选端口解析、运行路径锁、上下文构建与 token 预算、运行生命周期与 TurnGate、pending 应用、MCP 跨树访问拒绝、事件缺口、前端叶子链与运行分组。
- 协议：stream-json/control 事件回放测试。
- 渲染：golden frame 对比。
- 集成：工程读写、队列、删除、回收。
- 发布：干净 Windows 冒烟测试。

## 14. 目录

```
main.go                    # Wails 入口 + agent-host 模式
internal/
  agenthost/               # Windows stdio 代理、Job Object
  domain/{entity,valueobject,event,port}
  service/{project,workflow,run,context,render,media,system,permission}
  handler/
  infra/{persistence,agent/{claudecli,mcp},media,renderer,eventbus,paths,environment,logging,winmsg,workspace,audit}
  app/
frontend/
  src/{canvas,nodes,engine,stores,api,ui,hooks,styles}
build/                     # Wails 构建资产（跟踪）
runtime/
docs/
```

入口放根目录而非 `cmd/`：Wails 的 `//go:embed all:frontend/dist` 不能跨包目录，见 [ADR-015](decisions.md#adr-015-wails-入口保留在仓库根目录)。
