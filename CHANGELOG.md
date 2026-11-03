# Changelog

本文件记录所有值得注意的变更。
格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

M2 图与提示词（进行中：M2-00..07 已验收）。

### Added

- M2-04：`service/context.Builder` 从真实路径读取祖先最新已应用产物和对应 topic 候选，生成确定性上下文与 `contextHash`/`ancestorHash`/`paramsHash`。
- M2-04：保守 token 估算、祖先全文→2,000 tokens 原文摘录→引用降级、完整目标参数及 `CONTEXT_TOO_LARGE` 估算/预算详情；提供实际 usage 超预算比较值对象。
- M2-04：`RunWorkspaceReader` 作用域校验与工作区逻辑文件清单；文件正文不注入上下文，二进制资源仅引用，清单上限和截断显式标记。
- M2-04：golden、候选/路径隔离、预算与哈希、真实工作区叠加及文件引用测试；证据见 `docs/dev/evidence/m2-04.md`。
- M2-05：pending 原文与索引分离，保留无效 Schema 输出；实现 request_id 幂等/冲突处理、每运行一个结构化结果、每树 50 条活动上限及 256 KiB payload 上限。
- M2-05：实现 `topics`/`script` Schema 校验、pending 查询、应用与丢弃；应用时校验哈希、写内容寻址 artifact、追加轮次、锁定节点并传播后代 stale。
- M2-05：新增 `pending.txn.json` 跨文件事务日志与工程加载恢复；修复崩溃窗口下 project revision、pending 索引、原文与 artifact 的一致性。
- M2-05：新增 request 冲突、Schema/限额、force 限制、discard 幂等、artifact kind 与事务恢复测试；任务自检证据见 `docs/dev/evidence/m2-05.md`。
- M2-06：新增 run-scoped `get_chain/get_graph/get_node/list_node_types/validate/read_artifact/reply`；图与节点只暴露当前运行路径，保留 M1 MCP 工具兼容。
- M2-06：结构化 reply 从 MCP 原始 arguments 读取并委托 workflow 创建 pending；无效 Schema 的 JSON payload 保留为 invalid pending，不提供写图或 apply 工具。
- M2-06：新增路径祖先 artifact 授权、按 round kind 校验、topic 当前候选过滤、UTF-8 安全分块与冲突视图拒绝；单块上限 16 KiB。
- M2-06：新增 scope/生命周期、raw payload、并发 reply、topic 候选过滤、artifact 分块与工具权限测试；自检证据见 `docs/dev/evidence/m2-06.md`。
- M2-07：新增一次性运行管理、全局 owner-aware TurnGate、真实路径快照锁、per-run MCP/token/permission、运行 transcript/usage 持久化与 `run:<type>` 事件。
- M2-07：启动时恢复未完成记录为 interrupted；终止时等待活动 MCP/权限/Agent 事件租约并清理运行配置、MCP 服务和图锁；60 分钟无进展看门狗由运行事件重置，并保护显式中断不被 CLI idle 事件覆盖。
- M2-07：按首次 `AssistantMessage.Usage` 记录首次请求 input tokens；新增运行生命周期、资源释放、重启恢复、配置路径、项目切换、审计字段、usage 与可注入看门狗测试；自检证据见 `docs/dev/evidence/m2-07.md`。
- M2-08A：新增一致的工作台快照、工程作用域变更事件与进程内 workspace revision；图/pending/运行锁回调携带变更所属的 projectId。
- M2-08A：运行事件加入 projectId；新增 `GetRunSnapshot`，将运行记录、transcript、事件 lastSeq 和活动权限/问题请求置于同一同步点，保留现有运行记录磁盘格式与 `GetRun`。
- M2-08A：增加工作台链/pending/run 摘要、路径锁快照与并发事件恢复测试；实现自检证据见 `docs/dev/evidence/m2-08a.md`。
- M2-02：`service/workflow` 图命令层——`AddNode`（topic 根 / 带父 prompt / 草稿）、`Connect`（改挂确认）、`MoveNode`（位置）、`UpdateNodeParams`、`SelectNode`、`EnsureExecutable`。
- M2-02：单父树校验（无环、自连、topic 根、草稿父拒绝）、子树 `stale` 传播与 revision 规则；`SelectNode` 不递增 revision、不广播。
- M2-02：topic 候选端口解析——0 基 `parentCandidate` 从最新已应用 `topics` 产物计数；非法候选返回 `CANDIDATE_INVALID`；父为 prompt/草稿时清空候选。
- M2-02：运行路径锁注册表（`AcquireRunLock`/`ReleaseRunLock`/`LockedNodeIDs`）；路径节点与共享祖先只读，路径外分支可编辑；`EnsureExecutable` 拒绝锁定节点。
- M2-02：跨树改挂迁移子树 `steps` 并在失败时回滚（`WORKSPACE_MOVE_FAILED`）；`OnChange(revision, "graph")` 回调供 M2-08 事件桥接入。
- M2-02：`ProjectSession.Save`、`ArtifactStoreProvider`、`WorkspaceMover` 端口与 `WorkflowService` Wails 绑定。
- M2-03：`WorkspaceReader` / `workspace.View` 合并当前节点、路径祖先与共享 runtime；列表按 overlay 优先级遮蔽，文件/列表/grep 均有硬上限。
- M2-03：`RunGuard` 分离内置文件读写权限——读取限运行路径 steps，写入限当前运行节点 steps；拒绝越界路径与 reparse point/junction。
- M2-03：MCP `read_file` / `list_dir` / `grep` / `stat` 接入逻辑叠加读取；增加 run-scoped factory 注入口，保留 M1 直接工作区及绝对路径兼容。
- M2-03：工作区叠加、遮蔽、runtime 回退、兄弟隔离、UTF-8/尺寸限制、跨树 steps 移动/回滚、权限白名单与 MCP 集成测试。

- M2-01：`project.json` v1 工程模型（节点、轮次、锁定、stale、activeNodeId；链仍为派生视图，不落盘）。
- M2-01：`config.json`（`lastProjectId`、`contextTokenBudget`，默认 1,000,000）、工程路径布局与原子写。
- M2-01：`runs.json`/`runs/<runId>.json`、`pending.json`/`pending/<pendingId>.json`、`artifacts/<sha256>/` 存储；transcript 上限 500、pending 原文上限 256 KiB。
- M2-01：M1 迁移——默认工程与 topic 根「M1 迁移」、只导入 `activeChainId` 的只读运行、复制旧 workspace 到根节点 steps、备份到 `data/backup/m1/`、幂等与 `MIGRATION_FAILED`。
- M2-01：`ProjectService` Wails 绑定（列表、新建、打开、当前工程、配置）与启动时恢复上次工程。
- M2-01：存储/迁移/服务/绑定测试；前端与 Wails 构建通过。
- M2 设计：工程/图/运行/待应用的数据模型与存储布局；ADR-019..024。
- M2 设计：运行级一次性会话，App 从祖先链构建上下文；上下文预算按 token，默认 1,000,000。
- M2 设计：单父树 + 自由改挂（允许跨 topic 树并迁移 steps）；topic 每个候选一个输出端口，连接即选定；链 = topic 根到末端节点的派生路径。
- M2 设计：运行/排队路径与共享祖先只读，路径外兄弟分支可编辑。
- M2 设计：只读图 MCP 工具、结构化 `reply`、`read_artifact`；不开放写图工具。
- M2 设计：运行分组、待应用产物、叶子链快捷栏与工作台原型 `docs/design/m2-workspace.html`。
- M2 设计决策：锁定节点不可重跑，topic/prompt 每节点最多 1 个已应用轮次；同一父换候选按改挂确认；派生链视图由 Go 快照返回。

### Changed

- M2-06：将 `read_artifact` 的 topic 读取视图定义为当前运行路径所选的单候选对象；分块作用于过滤后的 UTF-8 字节，不回退完整产物。
- M2-02：活动工程以 `ProjectService` 为唯一内存源；workflow 命令通过 `ProjectSession.Save` 持久化并更新活动副本。
- M2-03：共享只读运行时统一为 exe 同级 `runtime/`，`data/runtime/` 不使用；新增 ADR-025，绿色包、架构、产品、Agent 协议、验收与开发文档同步。
- M2-01：应用启动时检查并执行 M1 迁移；`data/` 新增 `projects/` 与 `backup/m1/`，原 `sessions.json` 与旧 workspace 保留。
- M2-01：工程生命周期写操作同步落盘，重启恢复 `lastProjectId`；配置损坏时明确报错且不覆写。
- 链与会话分离：链不再绑定 Agent 会话；每次节点执行一个运行，运行之间不共享上下文。
- 链改为派生路径，不再持久化链实体；分支产生多条共享前缀的链。
- 工作区按 topic 树共享；写作用域限当前运行节点，读按当前路径叠加。
- M1 的跨重启 resume 由运行历史取代；AC-M1-3/4 标记为历史验收，`ResumeFailureCard`/`新开会话`/`取消恢复` 将在 M2 删除。
- 上下文预算从字节改为 token；`config.json` 新增 `contextTokenBudget`。
- 主题 B/C 与设置抽屉、`scene_dsl` 校验延后 M3；Agent 写图工具移出 M2，登记 RM-016。
- 工作区改为链内共享、链间隔离；写作用域限当前运行节点。
- 路线图：M2 进行中，RM-001 纳入 M2，新增 RM-016/RM-017。
- M2 范围纳入 F-06 运行级改造与 F-13 按运行权限/审计回归；出口补 AC-06、AC-13、AC-M1-1/2/5。
- `SelectNode` 只持久化 `activeNodeId`，不递增图 revision、不广播；选中与切换链为本地视图操作。
- M2 设计审计：统一 `scene_dsl` 命名与 `RunSpec`/`AgentRun` 命名；`RUN_ALREADY_ACTIVE` 在 M2 统一为 `BUSY`；节点补 `title` 字段；`parentCandidate` 明确为 0 基。

### 说明

- 本节记录 M2 设计与 M2-01..07 后端实现；M2-06/M2-07 已由用户验收；画布、选题/提示词 UI、前端事件桥、GUI/live CLI 与 M2 整体验收仍待后续任务。

---

## [0.2.0] - 2026-11-01

M1 Agent 会话：流式回复、MCP 工具、权限三档与审计、resume、临时会话页与控制台。

### Added

- M1：临时会话页、底部 PromptDock、右侧 Agent 控制台、流式 transcript、工具行。
- M1：HTTP MCP 服务、每会话 bearer token、workspace 只读工具、`reply` 与 `ask_user`。
- M1：社区 Claude Go SDK 适配、原生 `claude.exe`、Windows agent host、权限三档与审计。
- M1：单链路会话状态机、resume、最近 200 条 UI transcript、问题卡、resume 失败卡。
- M1：问题卡显示 120 秒倒计时，超时后锁定选项与输入。
- M1：会话状态机、MCP 工具、agent host、Wails 绑定、事件映射与前端卡片的自动化测试。

### Changed

- M1 默认复用现有 Claude profile；Agent 凭据/session 属适配器例外，应用数据仍在 exe 目录。
- M1 以 `--setting-sources=user` 加载 `~/.claude` 的用户级 settings（凭据、hooks、供应商切换器）；项目/本地 settings 与项目 CLAUDE.md 不加载。

### Fixed

- 原生 CLI 解析：修正 shim 内绝对 `claude.exe` 路径的正则（原写法匹配字面量 `[`，导致绝对路径候选失效）。
- Agent 凭据：修复使用供应商切换器（如 cc-switch）时 GUI 启动仍要求登录的问题。
- 工具摘要、stderr 与 grep 行按字符截断，不再把中文切成半个字符。

### Security

- MCP 仅监听 127.0.0.1，使用每会话 bearer token；token 不写日志。
- Claude 工具先过 PreToolUse 硬门禁，再进入 120 秒交互权限；审计独立写入 `logs/audit.log`，允许决策审计失败即拒绝。

---

## [0.1.0] - 2026-11-01

M0 壳与绿色版。

### Added

- 产品总览、PRD、架构、Agent 协议、验收标准。
- 版本管理、发布流程、发布检查清单、发布说明模板。
- 架构决策记录（ADR）。
- 开发流程、路线图（含暂存需求池）、`CLAUDE.md` 项目约束。
- 设计阶段文档集。
- 交互原型（仓库外 `../prototype/`）。
- M0：Wails v3.0.0 壳、绿色目录、单实例、WebView2 数据目录重定向。
- M0：zap + lumberjack 日志、最近日志面板、Claude CLI/WebView2 检测。
- M0：版本注入、顶栏版本徽标、关于弹窗、启动自检壳、`--self-check` 诊断模式。
- M0：CLI 超时终止子进程树、自检无界面错误输出、WebView2 版本校验、日志尾行丢弃等回归测试。

### Changed

- 产品文档迁入 `AudioUI/` 独立仓库。
- 文档结构按 PRD 与发布规范重组。
- 里程碑改为按依赖与需求优先级推进，移除开发时间表。
- Wails 入口保留在根 `main.go`（ADR-015），前端升级 React 19。
- 产品名由 Bili Studio 更名为 AudioUI；模块、二进制、单实例 ID、环境变量与发布产物同步更名。

### Fixed

- Windows 下通过 `cmd.exe /d /s /c call` 执行 `claude.cmd`，避免版本检测被错误引号破坏。
- CLI 探测超时会终止子进程树，避免 `claude.cmd` 的 Node 子进程拖住环境检测。
- `--self-check` 在启动错误时改为 stderr + 退出码，不再弹模态框阻塞无界面验证。
- 日志 tail 丢弃未写完的尾行；前端显示 CLI 版本检测错误与稳定日志 key。
- WebView2 检测拒绝 `0.0.0.0`、校验 `msedgewebview2.exe`，版本按数值比较。
- 路径 guard 允许 workspace 内的绝对路径，同时拒绝越界路径；读取 sessions.json 时去除 UTF-8 BOM。
