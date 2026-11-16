# CLAUDE.md

本文件是 Claude 在本仓库工作时的项目级约束。与临时对话结论冲突时，以仓库文档为准。

## 项目速览

AudioUI 是本地运行的 Windows 绿色版内容生产工作台：用户在节点画布上完成选题、提示词、Agent 生成、渲染、配音和导出，再把素材交给剪映成片。

- 当前状态：M0、M1 已验收；M2 已于 2026-11-08 通过用户整体验收；画布交互优化为 M2 后、M3 前的独立工作批次，U0 决策已确认，U1～U7 已验收，U8 已按用户确认的离线能力核对范围于 2026-11-16 完成（WebSearch 未接入、AC-U-18 未验收），U9 fixture 实现、代码级自检、离线构建门禁与用户最终页面验收均已完成；AC-U-18 仍未验收。U10～U25 已按各自范围完成实现/自检或用户验收。U24 已于 2026-11-15 通过用户页面验收；U25 已于 2026-11-15 通过 AC-U-66～67 页面验收。整体优先级以[路线图](docs/dev/roadmap.md)为准。0.3.0 版本与 S6 收口待另行确认。
- 平台：Windows 11、单 exe、数据只落在 exe 目录。
- 推进方式：按需求等级 P0 → P1 → P2 与依赖推进；不写开发时间承诺。
- 当前目标：U21 已于 2026-11-14 通过用户验收；U22 已于 2026-11-15 通过用户验收；U23 已于 2026-11-15 通过页面验收；U24 已于 2026-11-15 通过 AC-U-63～65 页面验收；U25 已于 2026-11-15 通过 AC-U-66～67 页面验收，代码自检与离线构建通过；U8 已按离线能力核对范围完成，WebSearch 未接入、AC-U-18 未验收；U9 fixture、自动化/离线构建门禁及用户页面验收已完成；AC-U-18 仍未验收。详细范围见 [优化计划](docs/plans/active/canvas-workbench-optimization-plan.md)。RM-019 云端与 RM-020 视频仍待评估。

## 开工前必读

按顺序读，读与任务相关的部分，不要凭记忆改代码：

1. [docs/README.md](docs/README.md)：文档地图。
2. [docs/prd.md](docs/prd.md)：需求、范围、优先级。
3. [docs/dev/roadmap.md](docs/dev/roadmap.md)：当前批次、状态、暂存池。
4. [docs/dev/process.md](docs/dev/process.md)：阶段、门禁、DoR/DoD。
5. [docs/architecture.md](docs/architecture.md) 与 [docs/decisions.md](docs/decisions.md)：技术方案。
6. 按任务补读：前端 / Agent 协议 / 验收标准。

权威边界：

| 问题 | 以谁为准 |
|---|---|
| 要什么 | PRD |
| 先做什么、什么被暂缓 | 路线图 |
| 技术怎么做 | 架构 + ADR |
| 界面怎么呈现 | 前端设计 + `docs/design/style-directions.html` |
| Agent 怎么接 | Agent 协议 |
| 做到什么算过 | 验收标准 |
| 开发怎么做 | 开发流程 |

冲突时停下来问用户，不要自行选择一套。

## 目录与分层

```text
main.go                          Wails 入口 + agent-host 模式
build/                           Wails 构建资产（跟踪）
internal/agenthost/              Windows stdio 代理、隐藏窗口、Job Object
internal/domain/{entity,valueobject,event,port}
internal/service/{project,workflow,run,context,system,permission,...}
internal/handler/                Wails 绑定：参数转换与错误映射
internal/infra/{paths,environment,logging,winmsg,persistence,eventbus,audit,workspace}
internal/infra/agent/{claudecli,mcp}
internal/app/                    生命周期、窗口、单实例、自检、Agent 组装
frontend/src/{api,ui,stores,hooks,styles,...}
runtime/                         共享只读运行时
docs/                             PRD、架构、设计、plans/{active,stages/M2}、dev/{roadmap,process,evidence}
data/                            运行时生成，gitignore
```

入口在仓库根目录而非 `cmd/`：Wails 的 `//go:embed all:frontend/dist` 不能跨包目录，见 ADR-015。

依赖规则：

- `domain` 不 import 任何 `internal` 实现包。
- `service` 只依赖 `domain`。
- `infra` 只实现 `domain/port`。
- `handler` 只做参数转换与错误映射，不写业务规则。
- 依赖注入手工组装，不引入 go-zero / goctl 运行时。

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3.0.0（锁定） |
| 后端 | Go 1.26+ |
| 前端 | React 19 + TypeScript strict + Vite |
| 画布 | React Flow（@xyflow/react） |
| 状态 | Zustand，按领域拆 store |
| 渲染 | Canvas2D，确定性 `draw(t)` |
| 编码 | WebCodecs + mediabunny |
| 存储 | JSON 工程 + 内容寻址产物 |
| 日志 | zap + lumberjack |
| Agent | Claude Code CLI，stream-json + MCP HTTP；每次节点执行一次性运行 |
| TTS | edge-tts（Go 实现） |

## 构建与测试命令

Wails CLI 用 v3.0.0。若 `wails3` 不在 PATH：

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

实际命令：

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

不要用 `go test ./...`：`frontend/node_modules` 里有第三方 Go 包，会被误扫。新增 Go 包时同步扩展 `. ./internal/...` 的范围。

开发：`wails3 dev`（Taskfile 会设置 `AUDIOUI_HOME` 指向仓库根）。生产：`bin/audio-ui.exe`。
诊断：`bin/audio-ui.exe --version`、`bin/audio-ui.exe --self-check`。
环境变量：`AUDIOUI_HOME` 覆盖便携根目录；`AUDIOUI_LOG_STDERR=1` 同时把日志写到 stderr；`AUDIOUI_CLAUDE_PATH` 覆盖原生 Claude CLI 路径。它们都是诊断/开发用途。
Agent 前提：只使用原生 `claude.exe`（拒绝 `.cmd`），默认复用现有 `~/.claude` profile，以 `--setting-sources=user` 加载用户级 settings（凭据、hooks、供应商切换器如 cc-switch）；项目/本地 settings 与项目 CLAUDE.md 不加载。Agent 凭据属适配器例外。M2 起每次节点执行启动一次性运行，App 从 topic 根 → 目标节点路径构建上下文（含该路径所选候选），运行之间不共享 Agent 记忆，不使用 resume；`PromptDock` 已删除，执行入口只来自节点与待处理卡，控制台内不得加提示词输入框。上下文预算 `config.json` 的 `contextTokenBudget` 默认 1,000,000 tokens。

## 领域不变量

> 工程格式 U16 实现目标为 v5：显式边携带 Script 来源版本，候选来源含 run ID；Agent v4 快照冻结逐边版本和唯一输出目标。当前实现状态以路线图为准。U14 图结构、U15 无 Topic/DAG 运行、输入合并和隔离工作区已通过自动化与用户界面验收；M2 历史验收事实保留。

实现任何功能时都不得破坏：

1. 图是显式边的有向无环图：边有稳定 ID，禁止自连与直接/间接成环；节点允许多上游，重复连接幂等；Candidate 必须有且仅有一条受保护的生成来源边。
2. 节点 ID 项目内唯一；`Project.Edges` 持久化拓扑，`parent` 不再作为真相。普通连接只新增输入；目标端改挂只允许选中普通边并直接提交，候选来源不可改挂或断开；断开仍需确认。节点记录 `workspaceRootId`，目标端改挂不迁移 steps；与运行要求不一致时阻止运行。U22 后端命令、U23 箭头抓手与关系浮层已通过用户验收；U24 已移除旧顶部入口、来源替换命令及自动输入关系栏，并于 2026-11-15 通过 AC-U-63～65 页面验收。
3. 提示词节点应用首轮后锁定内容与执行；topic 应用候选后锁定内容与执行；锁定后不可再次执行，M2 每节点最多 1 个已应用轮次，迭代新增下游节点或分支；非运行链的拓扑仍可改挂。
4. 产物不可变；修改产生新哈希。
5. 所有写操作经过命令层；Agent 不能直接改图；自由改挂只能由 UI 触发；`SelectNode` 只持久化 `activeNodeId`，不递增图 revision、不广播。
6. 删除节点必须级联处理全部下游，并进入回收站。
7. 运行依据实际冻结范围：v2 表示有效 Topic 单路径；v3 表示 U15 无 Topic/DAG 的所选输入边与祖先子图；U16 v4 按每条边冻结 Script 版本/artifact，并冻结单一输出目标。仅所选输入及必要祖先参与上下文和授权；目标节点工作区是唯一写入位置，输入来源只读隔离；运行之间不共享 Agent 上下文。
8. 结构化 Agent 输出先成为待应用产物；只有 UI 可以应用。
9. Topic 树单路径沿用共享 cwd + 节点增量；U15 DAG 输入工作区按快照来源隔离到 `inputs/<rootId>/<nodeId>/`，无 Topic 输入使用 `inputs/project/<nodeId>/`，均只读；运行目标为唯一写入位置，exe 同级 `runtime/` 只读。run-scoped MCP 只做快照授权的 AudioUI 应用查询，不代理通用文件操作；Agent 不得读取未选输入或其他分支。M1 legacy MCP 直接工作区工具独立保留；`runtime/` 是随包共享只读资源，不放在 `data/`。
10. 并发按在途运行计数；排队只在执行节点时发生；同一节点同一时间只允许一个运行；运行/排队路径与共享祖先只读。
11. 无歧义节点仍派生 topic 根到末端的路径；分支产生多条共享前缀的链；链不持久化；派生链视图由 Go 快照返回，前端不复制链规则。多上游汇合的叶子以真实上游集合（`nodeIds`、`ambiguous`）呈现，不伪造单一路径。

## Agent 集成约束

- MCP 只监听 `127.0.0.1`，**每运行**独立 token；作用域绑定 runId/projectId/nodeId 与执行快照。v2 使用 TreeID/ChainPath，v3/v4 可以无 TreeID/ChainPath、改由冻结快照授权；v4 按来源边授权精确 Script artifact。run-scoped 图/产物读取必须使用发送快照授权，不得以实时 `Project.Current` 替代。
- MCP 返回引用与摘要，不返回大块内容；大内容用 `read_artifact` 分块读取。topic 产物只能读取当前路径所选候选，不能回退整份产物；M2-11C 起按快照 Candidate 来源及 Script 不可变版本授权。
- 每次运行由 `service/context` 构建上下文：v2 仅含冻结 Topic 单路径、候选及任务；U15 v3 按快照选中输入边与顺序呈现多个 Prompt 任务及 Candidate/Script 上下文；U16 v4 逐边读取固定 Script 版本，并冻结运行输出目标。工作区正文不自动注入；预算按 token，默认 1,000,000，超限只降级祖先产物，目标任务始终完整。
- 结构化 `reply` 的 M2-11D 已实现受限提交：topics 原子生成候选；Agent script 按冻结输出绑定自动创建/写入首版，已有版本须待确认并按基准版本追加。结果事务、幂等与恢复自检见 `docs/dev/evidence/m2-11d.md`；M2-12A 新模型工作台已于 2026-11-08 通过用户验收，自检证据见 `docs/dev/evidence/m2-12a.md`；M2-12B 已实现脚本输出确认、版本历史和叶子链快捷栏，自动化自检记录见 `docs/dev/evidence/m2-12b.md`，真实浏览器实测因缺少驱动未执行；用户于 2026-11-08 确认验收。`apply_artifact` 仅 UI 可调用；旧模型已锁定节点拒绝再次执行与应用（`NODE_LOCKED`）。
- 权限三档：自动允许、确认、硬拒绝；越界路径、工程树内物理 `runtime/`、`artifacts/`、未选输入及其他节点目录硬拒绝。M2 v2 读取限冻结单路径祖先与 exe 同级共享 `runtime/`；U15 v3 读取限所选输入祖先的来源 steps 与只读 `runtime/`，写操作仅限本次目标工作区。M2/U15 run-scoped MCP 不提供通用文件代理；M1 legacy MCP 文件工具不用于运行作用域。
- 权限确认超时 120 秒视为拒绝，并记审计日志。
- 领域层不出现 Claude 专有词汇；通过 `AgentDriver`/`AgentRun` 端口与能力位适配，保留更换 Agent 的可能。
- 不在应用内实现 Agent 推理逻辑；应用只提供上下文、工具与运行编排。
- M2 不开放 Agent 写图工具（`create_node`/`connect`/`update_params`），登记 RM-016；写操作只能走命令层。
- topic 的每个候选是一个输出端口；连接候选端口即选定该候选；无节点级 `selectedTopic`。
- 运行/排队中的输入子图与绑定输出受锁保护；未参与本次快照的节点不可因共享后代而误锁。拓扑修改影响锁定范围时须由命令层拒绝。

## M2-11～13 本地脚本目标约束（M2-11B 已验收（2026-11-06）；目标闭环未完成）

- 节点类型为 `topic`、`candidate`、`prompt`、`agent`、`script`。M2-11B 历史连接矩阵为 `topic → candidate`（仅应用生成）、`candidate → prompt`、`prompt → agent`、`agent → script`、`script → prompt`；自 v4/U14 起普通连接不再受该矩阵限制，见上方领域不变量与 ADR-030。Go 命令层是连接/改挂合法性的唯一真相。
- topic 的有效 `topics` reply 复用现有 `ValidateStructuredPayload`，应用原子保存 artifact 并生成 candidate 节点；来源身份固定为 artifactId + 0 基索引。topic 生成候选后锁定内容/执行。
- prompt 只保存任务与约束，不独立运行；Agent 是本地运行入口，可在无活动运行锁时再次执行。每次发送冻结路径、候选来源、提示词、输出绑定及 `script@1` 契约；改挂后新运行重建快照，旧运行/结果来源不随图变化。
- Agent 仅能提交受限 `reply`：无指定输出时合法 script 结果自动建节点；指定空 Script 自动接收；已有版本则建立绑定基准版本的待确认结果，确认后追加、不覆盖旧 artifact，取消不改旧版本。错误/过期/越权 reply 不改图；提交成功不自动运行下游。
- 新工程 `project.json` v5；v1～v4 开发工程不迁移，返回 `UNSUPPORTED_VERSION`，不阻断有效工程列表/新建/打开。M1 导入直接创建当前格式。用户指定的 `bin` 旧工程仅按阶段约定手工清理，不写应用启动删除逻辑；`bin` 外旧工程不删除、不自动迁移。
- M2-11B 阶段限制：工程 v2 与 Go 图领域校验已实现，并于 2026-11-06 通过用户验收；当时仅开放 topic 运行入口。M2-11C 已于 2026-11-07 验收发送快照、上下文与运行授权；M2-11D 已实现 Candidate 原子生成和受限 Script reply 事务，并于 2026-11-07 通过用户验收；新模型工作台留给 M2-12A。当前 Prompt 不可独立运行，Agent 可作为本地入口运行；不得将此描述为完整脚本闭环。
- 旧 `ParentCandidate` 字段仅为现有 RPC/UI DTO 过渡保留；v2 图校验拒绝非空值，v2 工程序列化省略该字段。`Graph.ValidateForest()` 仅供尚未迁移的旧上下文构建器和旧候选只读查询使用；工程持久化与图写命令必须使用严格 `Graph.Validate()`。
- M2-11～13 任务状态见 [M2 阶段手册](docs/plans/stages/M2/m2-workplan.md)；M2 整体状态以路线图为准。

## 绿色版约束

- 用 `os.Executable()` 定位根目录，应用数据在 exe 目录；Agent 自身 profile 是适配器例外。
- 显式重定向 WebView2 用户数据目录；禁止写 AppData。
- 目录不可写时明确报错，不静默回退。
- 单实例锁；重复启动聚焦已有窗口。
- 日志用 zap + lumberjack，落在 exe 目录；日志面板可查询。
- 不采集远程遥测，不上传内容。

## 前端约束

- TypeScript strict；组件 PascalCase；文件与组件同名。
- 样式使用 CSS 变量令牌，禁止裸 hex；主题 A/B/C 随包。
- 不引入重型组件库；不复制业务规则到前端，业务真相在 Go。
- Zustand 使用选择器，避免整 store 订阅。
- 预览懒加载；`draw(t)` 保持纯函数，不在渲染循环里创建对象。
- 尊重 `prefers-reduced-motion`；键盘可达；对比度达标。U22～U24 已确认的未来例外是既有节点间手动连线和目标端改挂仅使用鼠标；链路选择、断开、版本设置及其他操作仍须键盘可达。
- 讨论界面、交互、样式时，生成可直接打开的单文件 HTML 原型，并给出本地 `file:///...` 链接。

## 测试要求

| 类型 | 要求 |
|---|---|
| domain | 纯净单测，不依赖文件系统与网络 |
| 上下文 | 路径祖先、候选解析、token 估算、预算降级、哈希；golden 对比 |
| 运行 | 一次性会话、TurnGate 释放、运行路径锁、重启标记中断；Go test + live CLI |
| 图 | 自由改挂/子树移动、同父换候选确认、topic 候选端口、单父/无环/运行锁、锁定不可重跑；Go test |
| 协议 | stream-json 事件回放 fixture；MCP reply/read_artifact |
| 渲染 | golden frame 对比 |
| 集成 | 工程读写、迁移、运行、队列、删除、回收；使用临时目录 |
| 前端交互 | 助手运行代码级自动化、类型检查、lint 与构建；真实页面操作和交互验收由用户执行。需要用户验收时，先在当前工作树离线重建最新 exe 并提供路径，不以构建结果替代页面验收 |
| 平台手工 | Wails/WebView2 专有行为、干净 Windows 冒烟、导出与剪映导入；浏览器无法覆盖的项单独列明 |
| 绿色版 | 不写 AppData；日志在 exe 目录 |

前端/UI 的真实页面操作、交互检查与验收由用户执行；助手不得启动或操控浏览器（包括无头浏览器）或 Wails 窗口。助手负责代码级自动化、类型检查、lint 和构建；如需用户验收，先在当前工作树使用离线配置重建最新 exe，提供 `bin/audio-ui.exe` 路径，并明确自动化/构建检查不等同于页面验收。未经明确授权不调用真实 Agent/CLI、不联网下载依赖。Wails/WebView2 专有行为由用户在实际应用中验收，单独记录未覆盖项。

测试与实现同一次变更完成，不留「以后再补」。

## 工作流

1. 从路线图取当前批次任务；确认满足 DoR。
2. 影响架构的，先补 ADR；影响交互的，先更新前端设计；影响 Agent 的，先更新协议。
3. 实现 + 测试 + 文档 + CHANGELOG 一起完成。
4. 跑任务级门禁，按自审清单检查。
5. 交给用户评审；不得自我验收 P0。
6. 达到 DoD 后更新路线图状态。

### 变更路由

| 变更 | 写到哪里 |
|---|---|
| 新需求 / 需求变更 | PRD 变更记录 + 验收标准 + 路线图；用户确认 |
| 当前无法处理 | 路线图暂存池，分配 `RM-xxx`；不写实现 |
| 架构 / 端口 / 存储 / 协议 | ADR + 架构文档 |
| 前端令牌 / 组件 / 交互 | 前端设计 + 设计原型 |
| Agent 工具 / 权限 / 运行 | Agent 协议 + 验收标准 + 架构 |
| 命令 / 目录 / 依赖 | 本文件 + README |
| 版本 / 发布 | `VERSION` + CHANGELOG + 发布文档 |
| 其他行为变化 | CHANGELOG 的 Unreleased |

范围纪律：

- 不实现路线图中没有的条目。
- 不把新想法直接塞进当前批次；先走暂存池。
- 开发期工程持久化格式可按当前需求直接演进，不要求兼容或迁移既有开发工程；旧开发数据可被丢弃。对已发布格式或用户明确要求保留的数据，另行确认兼容策略。
- 不写开发日期、周数、工时或完成期限。
- 发现文档冲突或需求不清，停止并询问，不猜测。

## Git

- 禁止在提交信息中追加 `Co-Authored-By`、`Co-authored-by` 或其他署名/致谢尾注。
- 未经用户要求，不执行 commit / push。
- 分支：`feat/Mx-短描述`、`fix/短描述`、`docs/短描述`、`hotfix/x.y.z`。
- 提交信息：`type(scope): 简述`，type 取 `feat|fix|docs|refactor|test|chore|build`。
- 一个提交只做一件逻辑上的事；不混入格式化、无关重命名、生成物。
- 不提交 `data/`、`logs/`、`webview2/`、构建产物、密钥。

## 禁止事项

- 引入 go-zero / goctl 运行时。
- 在应用内实现 Agent 推理逻辑。
- Agent 不得直接改图或绕过应用校验；M2-11 受限 `reply` 的自动提交仅由 Go 命令层完成，已有 script 版本须经 UI 确认后追加。
- 破坏树形约束、提示词锁定、产物不可变。
- 写入 AppData 或 exe 目录之外。
- 把业务规则复制到前端。
- 引入未经确认的重型依赖或 UI 库。
- 臆造命令、路径、脚本或协议字段。
- 静默扩大范围或留下未记录的 TODO。
- 修改已发布的 tag 或产物。

## 语言与文档

- 文档、UI 文案、提交简述用中文；代码标识符、协议字段用英文。
- 文档优先用表格与清单，少用形容词。
- 文档标注状态：草案 / 已评审；定稿后由用户确认改为「已评审」。
- 目录、命令、依赖、门禁变化时，同一次变更内更新本文件；本文件失效比没有更糟。
