# AGENTS.md

本文件是 Codex 在本仓库工作时的项目级约束。与临时对话结论冲突时，以仓库文档为准。

## 项目速览

AudioUI 是本地运行的 Windows 绿色版内容生产工作台：用户在节点画布上完成选题、提示词、Agent 生成、渲染、配音和导出，再把素材交给剪映成片。

- 当前状态：M0、M1 已验收（`VERSION` 仍为 0.2.0）；M2 图与提示词进行中（M2-00..09 已验收，M2-10A 已验收，M2-10B 已验收，M2-10C 已验收（用户于 2026-11-05 确认页面验收），M2-11～13 未开始），M2 收口后升至 0.3.0。
- 平台：Windows 11、单 exe、数据只落在 exe 目录。
- 推进方式：按需求等级 P0 → P1 → P2 与依赖推进；不写开发时间承诺。
- 新目标已记录为草案、尚未实现或排期：候选节点、提示词/Agent 分离、受限 `reply` 自动建输出节点、本地/云端适配及发送时重建上下文。见 `docs/prd.md` 1.1 与 ADR-027/028；本文件下文仍约束当前 M2 基线。新目标与既有行为的切换、待决项和 M2 后续安排须由用户决定；不得将新目标当成已实现，也不得继续按旧任务实现与新目标冲突的行为。

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
docs/
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
Agent 前提：只使用原生 `claude.exe`（拒绝 `.cmd`），默认复用现有 `~/.claude` profile，以 `--setting-sources=user` 加载用户级 settings（凭据、hooks、供应商切换器如 cc-switch）；项目/本地 settings 与项目 AGENTS.md 不加载。Agent 凭据属适配器例外。M2 起每次节点执行启动一次性运行，App 从 topic 根 → 目标节点路径构建上下文（含该路径所选候选），运行之间不共享 Agent 记忆，不使用 resume；`PromptDock` 已删除，执行入口只来自节点与待处理卡，控制台内不得加提示词输入框。上下文预算 `config.json` 的 `contextTokenBudget` 默认 1,000,000 tokens。

## 领域不变量

实现任何功能时都不得破坏：

1. 图是单父森林：每个节点最多一个父；topic 必须无父且是树根；边不悬空；成环、自连、topic 作子节点拒绝。
2. 节点 ID 项目内唯一；边由 `parent` + `parentCandidate` 派生，不落盘；`parentCandidate` 为 0 基；改挂允许跨 topic 树，同一父换候选也需确认，跨树迁移子树 steps 并回滚失败。
3. 提示词节点应用首轮后锁定内容与执行；topic 应用候选后锁定内容与执行；锁定后不可再次执行，M2 每节点最多 1 个已应用轮次，迭代新增下游节点或分支；非运行链的拓扑仍可改挂。
4. 产物不可变；修改产生新哈希。
5. 所有写操作经过命令层；Agent 不能直接改图；自由改挂只能由 UI 触发；`SelectNode` 只持久化 `activeNodeId`，不递增图 revision、不广播。
6. 删除节点必须级联处理全部下游，并进入回收站。
7. 运行与路径绑定；一次运行只读写本 topic 树；上下文只含该路径与所选候选；运行之间不共享 Agent 上下文。
8. 结构化 Agent 输出先成为待应用产物；只有 UI 可以应用。
9. topic 树内共享 cwd + 节点增量；写作用域限当前运行节点；内置文件读取按当前路径祖先叠加，MCP 只读视图再回退到 exe 同级 `runtime/`；Agent 读取其他 topic 树拒绝；跨树改挂由命令层迁移子树 steps。`runtime/` 是随包共享只读资源，不放在 `data/`。
10. 并发按在途运行计数；排队只在执行节点时发生；同一节点同一时间只允许一个运行；运行/排队路径与共享祖先只读。
11. 链是 topic 根到末端节点的派生路径；分支产生多条共享前缀的链；链不持久化；派生链视图由 Go 快照返回，前端不复制链规则。

## Agent 集成约束

- MCP 只监听 `127.0.0.1`，**每运行**独立 token，作用域绑定 runId/treeId/nodeId。
- MCP 返回引用与摘要，不返回大块内容；大内容用 `read_artifact` 分块读取。topic 产物只能读取当前路径所选候选，不能回退整份产物；该规则由 M2-06 实现。
- 每次运行由 `service/context` 构建上下文：只含 topic 根 → 父节点路径的最新已应用轮次与所选候选，不含目标自身历史、兄弟分支、子节点、其他候选；预算按 token，默认 1,000,000。超限先从远到近降级祖先、直接父最后；目标参数始终完整，仍超即拒绝。工作区文件只提供逻辑路径与大小，不自动注入正文；二进制资源仅引用（ADR-026）。
- 结构化 `reply` 只生成待应用产物；`apply_artifact` 仅 UI 可调用；已锁定节点拒绝再次执行与应用（`NODE_LOCKED`）。
- 权限三档：自动允许、确认、硬拒绝；越界路径、工程树内物理 `runtime/`、`artifacts/`、其他节点目录硬拒绝。exe 同级共享 `runtime/` 只可经 MCP 逻辑叠加视图只读访问。
- 权限确认超时 120 秒视为拒绝，并记审计日志。
- 领域层不出现 Claude 专有词汇；通过 `AgentDriver`/`AgentRun` 端口与能力位适配，保留更换 Agent 的可能。
- 不在应用内实现 Agent 推理逻辑；应用只提供上下文、工具与运行编排。
- M2 不开放 Agent 写图工具（`create_node`/`connect`/`update_params`），登记 RM-016；写操作只能走命令层。
- topic 的每个候选是一个输出端口；连接候选端口即选定该候选；无节点级 `selectedTopic`。
- 运行/排队路径（topic 根 → 运行节点）与共享祖先只读；路径外兄弟分支可编辑；自由改挂需走命令层并确认。

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
- 尊重 `prefers-reduced-motion`；键盘可达；对比度达标。
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
| 手工 | 画布、控制台、导出、剪映导入；干净 Windows 冒烟 |
| 绿色版 | 不写 AppData；日志在 exe 目录 |

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
- 让 Agent 直接修改图或绕过待应用产物。
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
