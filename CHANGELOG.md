# Changelog

本文件记录所有值得注意的变更。
格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/lang/zh-CN/)。

## [Unreleased]

M2 图与提示词（M2-00～10 旧模型已验收；M2-11A 本地脚本目标设计已验收；M2-11B 工程格式与图领域模型已验收（2026-11-06）；M2-11C 已验收（2026-11-07）；M2-11D 已验收（2026-11-07）；M2-12A 已于 2026-11-08 通过用户验收；M2-12B 已于 2026-11-08 通过用户验收（实现与自动化/离线门禁自检完成）；M2-13 已于 2026-11-08 通过用户验收，本地脚本闭环完成；用户同日确认 M2 整体验收通过）。

### Fixed

- Prompt 节点名称改为可选画布标签；空任务内容会返回 `PROMPT_TASK_EMPTY`，Agent 界面给出填写提示。

### Added

- U20（用户于 2026-11-14 确认验收通过）：活动画布使用 React Flow 22px 间距、约 2px 点径的 SVG 点阵，停用固定 CSS 底纹；透明画布容器配合 `z-index: 0` 的现有 SVG 背景，让点阵位于画布底色之上、节点之下。加载/错误/无工程占位状态保留静态点阵。正常模式仅作 10 秒低幅透明度变化，系统减少动效时保持静态。前端全量测试、类型检查、令牌检查、构建及离线 Wails 构建通过；lint 0 错误、2 条既存警告。用户于 2026-11-14 确认 AC-U-51 页面验收通过，详细操作记录未归档；见 `docs/dev/evidence/u20-dot-background.md`。
- U19（用户于 2026-11-14 确认验收通过）：画布空白右键先打开「新增节点」，再展开四类节点选项；快捷栏共用两级菜单，保留初始创建坐标、键盘逐层操作、候选来源说明及视口避边。定向测试 59 项、前端全量测试 235 项、类型检查、令牌检查、生产构建及离线 Wails 构建通过；lint 0 错误、2 条既存警告。用户于 2026-11-14 确认 AC-U-49～50 页面验收通过；助手未启动界面，具体操作记录和截图未归档；证据见 `docs/dev/evidence/u19-two-level-context-menu.md`。
- U15（已于 2026-11-13 通过用户验收）：Agent 启动前支持选择和排序直接输入；保留有效 Topic 单路径 v2 快照，无 Topic/DAG 使用 v3 输入子图快照；补充快照授权、输入祖先锁定、来源隔离只读工作区及无 Topic 工程工作区。Go 全量测试/vet、前端 211 项测试、typecheck、lint、令牌检查、前端构建及离线 Wails 构建通过；助手未启动 UI/浏览器或真实 Agent。U16 逐边版本、并行输出与 Topic 新候选批次不在本次实现范围。
- U14（用户于 2026-11-12 验收通过）：工程格式升为 v4，以持久化边集合作为唯一拓扑真相，支持任意阶段、任意创建顺序、多上游与同类型连接；普通连线只新增输入，候选生成来源独立保存且不可断开或改挂。新增显式改挂与断开普通连接的工程作用域命令；无 Topic、多上游或不符合现有执行契约时在启动前明确拒绝，不删除连接、不扩大工作区授权。前端按权威快照绘制全部边，并提供键盘输入关系管理与确认流程；叶子汇合显示真实上游集合。自检边界见 `docs/dev/evidence/u14-dag-transition.md`。
- U10（用户于 2026-11-10 验收通过）：正向、反向、窄间距与避障连线统一为平滑曲线；拖线预览复用节点避障路由，箭头、命中区、路径高亮及运行锁样式保留。路由、预览、flowModel 与 React Flow DOM 共 43 项通过；`WorkspaceCanvas.test.tsx` 单文件无输出未计通过；真实页面由用户完成验收。
- U11（用户于 2026-11-10 确认页面验收通过）：Agent 收起状态胶囊支持非按钮区域拖动和键盘移动，与展开面板共享会话内位置并按视口尺寸避边；定向测试 22 项、TypeScript、lint（0 错误，2 条既存警告）、令牌检查和前端生产构建通过。完整前端测试运行 600 秒无进度后停止，不计为通过。实际页面由用户测试，助手未启动浏览器。
- U13（用户于 2026-11-12 确认界面验收通过）：五类节点正文可折叠，状态按工程保存且展开恢复原尺寸；Topic 候选锚点固定于标题区，候选列表可定位真实节点。自动化与构建边界见 `docs/dev/evidence/u13-node-folding.md`。
- U16（用户于 2026-11-14 确认验收通过）：工程 v5 逐边 Script 版本引用、运行前冻结唯一输出目标、Topic 锁定参数重跑及待确认候选批次；交互见 `docs/design/u16-output-version-demo.html`。
- U17（用户于 2026-11-14 确认验收通过）：取消已有工作台的全局同步加载提示与正常提交/成功通知；同步错误在画布视口内浮层显示，原重试和节点保存状态保留。前端定向 28 项、全量 223 项、类型检查、令牌检查、生产构建和离线 Wails 构建通过；lint 0 错误、2 条既存警告。用户已完成真实页面验收；具体操作记录和截图未归档。证据见 `docs/dev/evidence/u17-sync-notices.md`。
- U18（用户于 2026-11-14 确认验收通过）：Agent 面板按画布工具栏实际底边初始化，鼠标/键盘可调整宽高且会话内保留；长回答、路径和工具摘要可换行，查看历史时不因新消息跳回底部；移除常驻「未决请求」区与「待处理」页签，紧凑计数入口按需定位或恢复请求与孤立结果。定向测试 33 项、完整前端测试 229 项、类型检查、令牌检查、生产构建及离线 Wails 构建通过；lint 0 错误、2 条既存警告。用户已完成真实页面验收；助手未启动界面。
- 工作台 U10～U20 计划：U10～U18 已按范围通过用户验收；U19 已于 2026-11-14 通过用户验收；U20 已于 2026-11-14 通过用户验收；各任务状态以路线图及对应条目为准。
- RM-020：依据公开官方资料索引记录 AI 视频生产流程和调研边界；用户确认优先评估可控画面与按需生成式素材的混合方向，继续使用现有 Claude Code 适配，不先自研 Agent；视频工具、闭环范围和排期仍待评审。

- U2：节点工程数据新增必需 `size`，按节点类型赋予默认尺寸；`SetNodeLayout` 在单次工程事务中保存位置与尺寸，纯布局变化不改变图内容或运行锁。
- U3（用户于 2026-11-09 确认验收）：五类节点支持四边四角缩放、固定标题栏和卡片内滚动正文；键盘可精确设置或恢复默认尺寸，缩放结束以单次布局命令保存位置与尺寸。
- U4（用户于 2026-11-09 确认验收）：移除画布左上角新建按钮，以空白处右键、Shift+F10/菜单键和叶子链快捷栏共用的新建菜单创建选题、提示词、Agent、脚本；菜单视口避边但不改变创建坐标，其他类型保持未连接，Candidate 仍只由选题运行生成。助手的 Edge 本地实测未覆盖可信 Escape/焦点回收及缩放后的屏幕落点，边界见优化计划 U4。
- U5（用户于 2026-11-09 确认验收）：Script 节点按工程/节点/版本安全读取正文，显示最新摘要并按需查看全文和历史；待确认结果依冻结输出目标归属 Script 卡片，预览、确认、丢弃与同步重试共用工程作用域控制器。助手未完成浏览器实测或归档截图/viewport；未调用真实 Agent 或外网。
- U6（用户于 2026-11-10 确认界面验收）：运行控制台改为默认收起的悬浮入口，收起时保留面板状态；运行请求和待确认结果回到对应节点，无法定位时保留控制台恢复入口。定向测试和生产构建通过；全量前端测试因 Node 堆上限退出，不能视为全量通过。
- U6 补充项（用户于 2026-11-10 验收通过）：Agent 运行面板增加标题栏拖动和键盘方向键移动，面板位置约束在窗口范围内，收起/展开保留位置，不写入工程数据。
- U7（用户于 2026-11-10 验收通过）：画布使用带方向箭头、宽命中区和避让路由的自定义连线；分支高亮与运行锁状态分离，共享前缀不再误选叶子链；Candidate/Script 批量生成与前端新增子节点按实际/默认尺寸确定性避让，不移动既有节点。Go 全量测试、前端定向测试、生产构建和 Wails 构建通过；全量前端测试受 Node 堆上限影响 OOM，浏览器页面未实测。Wails 构建任务调用 `npm install` 并报告依赖已最新，锁文件未变化，注册表访问情况无法确认。
- M2-13：新增可控 Driver 全链路集成回归，覆盖 topics Candidate 生成、候选分支与改挂快照、无绑定/空 Script/已有版本输出、非法 reply、版本确认及工程重载；用户于 2026-11-08 确认阶段验收完成，自检范围与执行边界见 `docs/dev/evidence/m2.md`。
- M2-12B：已有 Script 结果卡显示输出绑定、冻结基准版本、原/新来源运行与 Artifact、输出 Schema；有效结果可确认追加或丢弃，版本冲突不提供 force，invalid 结果不显示确认操作。
- M2-12B：Script 节点显示待处理数量与不可变版本历史；叶子链快捷栏消费服务端链路和运行状态，通过 `SelectNode` 切换，并持久化折叠状态。
- M2-12B：新增待确认/冲突/丢弃、共享前缀叶子链、Handle 测量与 React Flow 真实 DOM 自动化回归；全量自检及未覆盖项见 `docs/dev/evidence/m2-12b.md`。用户于 2026-11-08 确认验收；真实浏览器交互未执行。
- M2-11C（用户于 2026-11-07 验收）：Agent 作为本地运行入口，发送快照冻结图路径、Candidate 来源、Prompt 任务/约束、Script 祖先版本及输出绑定基准；运行锁同时保护路径与指定输出，路径外兄弟仍可编辑。
- M2-11C：Agent 上下文按快照读取单个候选和精确 Script 版本，不要求 Prompt 祖先存在旧式 applied round；MCP 图/产物查询使用快照授权，不回退实时工程图；run-scoped MCP 移除通用文件代理，本地内置文件读取限路径祖先与共享只读 `runtime/`，写入限当前节点 `steps/`。
- M2-12A：交付五类新模型画布节点，Candidate/Script 内容按工程快照呈现；Prompt 任务与约束保存并在 Agent 运行前 flush；仅 Agent 提供本地运行入口，并显示输出绑定、路径锁与 BUSY。加载/错误中的候选读取不再隐藏已存在的权威边。实现及未覆盖项见 `docs/dev/evidence/m2-12a.md`；用户于 2026-11-08 确认验收完成。
- M2-12A：新增 Prompt 保存失败屏障、Agent 锁/BUSY、非法连接、工程切换迟到候选响应、键盘操作与真实 React Flow DOM 回归；不代表 M2-13 完整链路或真实 CLI 验收通过。
- M2-11C 门禁和未覆盖项见 `docs/dev/evidence/m2-11c.md`。在 M2-11C 验收范围内，Agent Script 结构化提交返回 `OUTPUT_COMMIT_UNAVAILABLE`；M2-11D 后续开放了受限提交，M2-12A 已于 2026-11-08 通过用户验收。
- M2-11D（已于 2026-11-07 通过用户验收）：有效 topics reply 原子保存不可变 artifact、Topic round 与全部 Candidate；Agent Script 无绑定/空绑定自动写入，已有版本建立待确认结果并在基准仍匹配时追加；无效 payload 保留原文，重放幂等，版本冲突不能由 force 绕过。
- M2-11D：扩展 pending 事务记录和已提交恢复，校验持久化 Candidate/Script 与结果回执关联；MCP reply 保留活动运行租约并区分自动提交与真正 pending。组件自检及未覆盖项见 `docs/dev/evidence/m2-11d.md`；M2-13 端到端、GUI/真实 CLI 尚未验收。
- M2-11B（用户于 2026-11-06 验收）：`project.json` 升为 v2，新增 candidate/agent/script 节点及 Candidate 来源、Script 版本引用；Go 图校验实施类型连接矩阵、Candidate 来源轮次与索引、Script 版本序列和 Agent 单直接输出约束。
- M2-11B（用户于 2026-11-06 验收）：v1 工程明确返回 `UNSUPPORTED_VERSION`，旧工程仍出现在带版本/错误摘要的列表且不阻断有效工程新建、打开；M1 sessions 迁移直接创建 v2 工程。
- M2-11B（用户于 2026-11-06 验收）：图命令拒绝普通创建 Candidate、改挂 Candidate 及旧 `parentCandidate` 输入；prompt/agent/script 运行入口明确返回 `RUN_ENTRY_UNAVAILABLE`。旧 UI/RPC 的 `ParentCandidate` 仅作过渡字段保留，v2 拒绝非空值且不落盘。门禁和未覆盖项见 `docs/dev/evidence/m2-11b.md`。
- M2-09：接入并锁定 `@xyflow/react` 12.x，工作台由快照受控渲染节点和派生连线；支持新增 topic、未连接 prompt 草稿、prompt 子节点、画布平移缩放/重置/聚焦和位置防抖保存。
- M2-09：新增 Connect 拖拽、改挂确认（子树数量、原上游、跨 topic 树与 steps 迁移提示）、后端失败回弹、SelectNode 本地选中回退、Go 派生路径高亮和运行锁节点/端口展示；候选正文读取留至 M2-10B。
- M2-09 后续修复：图命令按发起工程 ID 校验并返回 mutation receipt；创建、连接和移动等待 receipt 对应的快照，不混淆命令提交与画布呈现；同步失败时仅重试快照读取，避免重复写入。
- M2-09：新增确定性节点避让与创建后聚焦；首次工程载入只 fit 一次，画布 flex 高度链固定；鼠标/键盘选择与位置更新进入统一持久化路径。
- M2-09：新增服务端回执与工作台快照一致性测试、项目切换范围回归，以及不 mock React Flow 的节点/Handle/SVG/键盘 DOM 集成测试；交互原型见 `docs/design/m2-09-canvas-demo.html`，任务自检证据见 `docs/dev/evidence/m2-09.md`。
- M2-09：用户于 2026-11-04 在隔离工程的生产 Wails 窗口手工新增 topic 与草稿 prompt，确认「已同步」、工程 revision 与节点数增长、节点可见且不重叠；验收截图见 `docs/dev/evidence/m2-09-webview-manual.png`。
- M2-10A：选题节点支持标题、想法、方向和 3～10 候选数编辑，防抖保存、字段校验、保存失败保留输入与运行前 flush。
- M2-10A：节点可按工程作用域启动一次性运行，防止重复提交并显示运行/终态；结构化错误携带稳定 code，`CONTEXT_TOO_LARGE` 使用估算值与预算字段。
- M2-10A：以工作台快照派生最新运行与 pending 摘要；事件先于启动响应时恢复运行选择；运行结束不自动应用。HTML 演示与验收证据见 `docs/design/m2-10a-topic-run-demo.html`、`docs/dev/evidence/m2-10a.md`。
- M2-10A：用户于 2026-11-05 指示完成子项验收；Go 与前端自动化、类型检查及前端构建通过，Wails 构建由用户手动完成。真实 CLI、GUI 集成与 M2-10C 留待后续。
- M2-10B：控制台待处理页按工程列出 pending/invalid 摘要，选择后按需读取原始正文；topics 候选预览、invalid 原文/校验错误、应用/丢弃、哈希冲突确认与成功后快照重载接入。
- M2-10B：新增只读 `GetTopicCandidates(nodeId, projectId)`，校验 topic 最新已应用轮次及 artifact payload，返回稳定 0 基 `index/title/angle/hook`；画布候选端口改为从服务端数据渲染。
- M2-10B：pending 读写和候选查询按发起工程 ID 校验；新增后端 artifact/工程作用域测试及前端 Base64 解码、无效 pending、force 确认、重复提交、迟到响应与真实 React Flow Handle 测试；自检及验收证据见 `docs/dev/evidence/m2-10b.md`。用户于 2026-11-05 指示验收通过；AC-04/07/AC-M2-3/4/12 仍只按子项部分覆盖。
- M2-10C：新增可控 AgentDriver 后端闭环集成测试，覆盖 pending 应用与候选分支、改挂确认、invalid 丢弃、BUSY、运行锁及工程切换；自动化和离线构建门禁通过，用户于 2026-11-05 确认页面验收完成。用户会话截图未归档至仓库，完整 AC 与 M2 整体验收仍在后续任务范围。
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
- M2-08B：接入工程切换、工作台快照同步及按 runId 恢复的运行控制台；自动测试与构建通过。用户于 2026-11-03 指定截图更名作为验收条件，并于 2026-11-04 确认完成验收，证据见 `docs/dev/evidence/m2-08b.md` 与 `docs/dev/evidence/m2-08b.png`。
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

- U0 工作台交互优化文档：确认 M2 后、M3 前的独立批次；本轮纳入空白画布新建菜单与键盘入口、Script 卡片预览确认和收起式 Agent 悬浮栏；开发期不要求兼容或迁移旧工程数据。此项是已确认设计，不代表功能已经实现。
- U2：开发期 `project.json` 升为 v3，v1/v2 工程不迁移并返回 `UNSUPPORTED_VERSION`；M1 导入直接创建 v3 工程。
- M2-11A 设计验收（用户于 2026-11-06 确认）：选择 RM-018 本地 `script` 闭环纳入 M2-11～13，更新候选/Agent/输出契约、上下文快照和版本确认的需求与设计稿；RM-019 云端与 RM-020 视频仍待评估。此项仅为已验收的设计与原型，不表示产品功能已实现。

- M2-10B：待应用列表、详情、应用/丢弃及候选读取调用均绑定预期 `projectId`；工程切换后不将迟到的数据应用到当前视图。
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

### Fixed

- 画布输入：区分节点参数编辑与图结构操作的禁用状态，修复 Topic/Prompt 自动保存期间中断输入；跨节点参数写入串行，按工程 revision 保护并发待同步回执，较新提交成功后忽略迟到的旧失败。保留同步失败草稿，切换/关窗时重试快照读取而不重复写；IME 组合期间不防抖保存，组合结束后保存最终草稿。运行前只 flush 目标祖先路径，并将临时锁限制在该路径及直接绑定 Script。切换/新建/关窗前 flush 当前工程草稿，失败时提供重试、明确放弃或留在当前工程/应用。真实 Wails/WebView2 页面已实测焦点、组合输入模拟、工程切换与关窗保护；Windows 原生 IME 和真实 Agent 未测。
- M2-13：修正 v2 Agent 脚本 pending 确认时输入哈希回退旧 `parentCandidate` 解析并误报 `CANDIDATE_INVALID`；改为按当前 v2 图生成执行快照重算。
- M2-13：修正 v2 Topic/Agent 运行上下文中与已实现 `reply` 自动提交相矛盾的指令，按 topics 自动生成、Script 未绑定/空绑定及已有版本确认分别说明；保留 v1 Topic 的人工应用语义。
- M2 Agent 启动竞态：关闭 SDK 对 AudioUI host 的重复 `-v` 探测，`-v` 改为无副作用版本退出；主 GUI 在工程初始化和运行恢复前取得单实例锁，防止二次启动将活动运行判为中断并删除其 MCP 配置。
- M2 Agent 启动：适配器在启动前验证每运行 `mcp.json` 存在、可读且结构有效，Windows host 在创建原生 CLI 子进程前再次检查；将配置缺失报告为 `MCP_CONFIG_NOT_FOUND`，不再将 CLI 初始化失败误报为 `AGENT_UNAVAILABLE`。运行回归验证配置在启动前可读、终态后清理。
- M2：系统面板与运行入口统一使用原生 CLI 解析规则；运行启动时重新解析，修复 `.cmd` 被误报可用及 CLI 在当前进程变得可发现后仍因启动时缓存空 driver 而返回 `AGENT_UNAVAILABLE` 的问题。
- M2-09：修复快照并发刷新时创建命令可成功但画布未确认新节点的问题；只有同一工程、足够的工程 revision 与目标节点 ID 都出现在工作台快照后才完成同步。快照失败保留已提交 receipt 并提示只重试读取，禁止重复创建/连接/移动；工程切换不会把迟到命令写入另一工程。
- M2-09：修复画布高度依赖不确定百分比链、同工程旧快照覆盖本地选择、键盘移动/选择未持久化、节点初始位置重叠，以及改挂提交期间可取消等交互问题。高度链、React Flow DOM 与键盘能力通过自动化测试，创建与快照同步链路已由用户在真实 Wails 窗口手工确认。
- M2-10B：修复异步候选端口与派生边不同步导致的 React Flow 错误 008；保留同一已应用轮次的候选读取结果，动态端口变化后更新节点内部测量，并在源/目标端口布局测量就绪前暂缓绘边。
- 画布：修复选择/快照重建节点时丢失 React Flow 已测尺寸与拖动状态导致的错误 015；仅在节点尺寸及 source/target Handle 与当前 DOM 布局一致后开放派生边。

### 说明

- 本节记录 M2 设计与 M2-01..07 后端实现及后续进展；M2-06/M2-07、M2-08、M2-09、M2-10A/B/C 与 M2 整体验收均已通过用户确认。助手自检未执行项见 `docs/dev/evidence/m2.md`；AC-T-04 视频、AC-T-11～15/17 云端仍暂缓，0.3.0/S6 收口另行确认。

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
