# AudioUI

本地运行的、以节点图编排的内容生产工作台。用户在一张无限画布上完成选题、提示词、Agent 生成、渲染、配音和导出，再把素材交给剪映成片。

> **状态**：M1 已完成（v0.2.0）· M2-00～10 旧模型已验收；M2-11A 设计已验收；M2-11B 工程格式与图领域模型已验收（2026-11-06）；M2-11C 已验收（2026-11-07）；M2-11D 已验收（2026-11-07）；M2-12A～13 未开始，完整本地脚本闭环尚未实现。
> **平台**：Windows 11 · 绿色版单 exe · 当前本地应用数据落在 exe 目录（Agent 自身凭据为例外，模型请求由 Agent 发出）。
>
> **新目标范围已选定，设计已验收**：M2-11～13 仅纳入 RM-018 本地脚本子范围（候选节点、提示词/Agent 分离、受限 `reply` 输出与发送时上下文）。M2-11B 工程 v2 与图领域模型已验收；M2-11C 的运行快照、上下文、运行授权和本地文件边界已实现并验收；M2-11D 受限输出事务已于 2026-11-07 通过用户验收；新模型工作台尚未实现。云端 Agent（RM-019）与视频输出/渲染（RM-020）仍待评估。详见 [PRD 1.1](docs/prd.md#11-新目标需求m2-本地脚本子范围已选定设计已验收完整闭环未完成)、[路线图](docs/dev/roadmap.md) 与 [M2 目标闭环计划](docs/dev/m2-target-closure-plan.md)。

---

## 文档

| 文档 | 说明 |
|---|---|
| [产品总览](docs/product.md) | 概念、用户流程、架构、数据模型、节点、DSL |
| [PRD](docs/prd.md) | 背景、目标、范围、功能需求、业务规则、非功能需求 |
| [架构设计](docs/architecture.md) | 技术栈、分层、数据流、存储、渲染、安全 |
| [前端设计](docs/frontend-design.md) | 设计令牌、组件清单、交互状态、响应式、可访问性、验收 |
| [设计依据（原型）](docs/design/style-directions.html) | 主题、布局、交互原型 |
| [M0 启动自检壳](docs/design/m0-startup-shell.html) | M0 临时界面原型 |
| [M1 会话页](docs/design/m1-session-shell.html) | M1 临时会话/控制台原型（M2 已删除） |
| [M2 工作台](docs/design/m2-workspace.html) | M2 画布、草稿节点、运行分组原型 |
| [M2 选题闭环演示](docs/design/m2-topic-closure-demo.html) | 工作台至选题分支的本地交互演示 |
| [M2-11A 本地脚本流程演示](docs/design/m2-11-13-local-flow-demo.html) | 候选→提示词→Agent→script 输出的模拟交互；设计已验收，原型不连接 Agent |
| [开发流程](docs/dev/process.md) | 阶段、工作分解、门禁、变更控制 |
| [路线图](docs/dev/roadmap.md) | 需求优先级、里程碑批次、暂存需求池 |
| [M2 逐任务执行手册](docs/dev/m2-workplan.md) | M2 开工协议、任务卡、DoR/DoD 与状态 |
| [M2 目标闭环计划](docs/dev/m2-target-closure-plan.md) | M2-11～13 本地脚本闭环设计、任务与阶段门禁 |
| [M2 选题闭环计划](docs/dev/m2-topic-closure-plan.md) | 工作台、画布、选题与待应用的逐项实现和验收计划 |
| [Agent 协议](docs/agent-protocol.md) | MCP 工具、能力位、权限、会话、分叉与删除 |
| [验收标准](docs/acceptance.md) | Given-When-Then、测试矩阵、发布门禁 |
| [决策记录](docs/decisions.md) | ADR 列表 |
| [发布规范](docs/release/) | 版本管理、发布流程、检查清单、发布说明模板 |
| [CHANGELOG](CHANGELOG.md) | 版本变更 |

推荐阅读顺序：产品总览 → PRD → 路线图 → 架构 → Agent 协议 → 开发流程 → 验收 → 发布规范。

使用 Claude Code 开发时，先读 [CLAUDE.md](CLAUDE.md)。

---

## 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3.0.0（Windows WebView2） |
| 后端 | Go 1.26+ |
| 前端 | React 19 + TypeScript + Vite |
| 节点画布 | React Flow（@xyflow/react） |
| 渲染 | Canvas2D，确定性 `draw(t)` |
| 编码 | WebCodecs + mediabunny |
| 存储 | JSON 文件 + 内容寻址产物 |
| 日志 | zap + lumberjack |
| Agent | Claude Code CLI，stream-json + MCP HTTP |
| TTS | edge-tts（Go 实现） |

---

## 目录

```
AudioUI/
├── main.go                 # Wails 入口（embed frontend/dist）
├── build/                  # Wails 构建资产
├── internal/
│   ├── domain/             # 值对象与端口
│   ├── service/            # 应用服务
│   ├── handler/            # Wails 绑定
│   ├── infra/              # 路径、环境探测、日志、Agent 适配/MCP
│   ├── agenthost/          # Windows CLI stdio host
│   └── app/                # 生命周期、窗口、单实例、自检、Agent 组装
├── frontend/               # React + Vite
├── runtime/                # 共享只读运行时
├── docs/                   # 设计文档
└── data/                   # 运行时生成（gitignore）
```

---

## 开发

```bash
# Wails CLI（v3.0.0）
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0
export PATH="$(go env GOPATH)/bin:$PATH"

# 后端
go test . ./internal/...

# 前端
npm --prefix frontend install
npm --prefix frontend run typecheck
npm --prefix frontend run lint
npm --prefix frontend run check:tokens
npm --prefix frontend run test
npm --prefix frontend run build

# 构建与运行
wails3 build
bin/audio-ui.exe
bin/audio-ui.exe --version
bin/audio-ui.exe --self-check
```

`--self-check` 会在 exe 同级创建 `data/`、`logs/`、`webview2/`，并把环境报告写入 `logs/self-check.json`。

Agent 前提：使用原生 `claude.exe`（拒绝 `.cmd`），复用现有 `~/.claude` 配置（`--setting-sources=user`，因此 OAuth 登录、`settings.json` 里的供应商密钥/`apiKeyHelper`、用户 hooks 都会生效；项目级 settings 不加载）。应用数据、日志、workspace、运行记录仍在 exe 目录；Agent 自身凭据属适配器例外。切换供应商后需重启应用。

M2 起每次节点执行启动一次性运行：App 从祖先链构建上下文（只含根 → 父节点的最新已应用轮次，不含目标自身历史/兄弟/子节点），运行之间不共享 Agent 记忆，不使用 resume。上下文预算由 `data/config.json` 的 `contextTokenBudget` 控制，默认 1,000,000 tokens；目标参数保持完整，工作区文件只提供路径与大小。M2-04 已实现上下文构建器，实际运行接线及用量记录在 M2-07。Pending 后端按 runId 幂等保存结构化原文；无效 Schema 仍保留且不可应用。每个 topic 树最多 50 条活动 pending；应用后写不可变 artifact、追加轮次、锁定节点并将后代标记 stale，事务日志支持工程重载恢复。M2 每节点最多一个已应用轮次；继续迭代新增下游节点或从候选端口开分支。同一 topic 换候选按改挂确认并标记子树 stale。工作区共享只读资源固定为 exe 同级 `runtime/`；Agent 通过路径祖先 → runtime 的只读叠加视图读取，内置文件写入仅允许当前运行节点的 `steps/<nodeId>/`。

开发/诊断环境变量：`AUDIOUI_HOME` 覆盖便携根目录，`AUDIOUI_LOG_STDERR=1` 同时输出日志到 stderr，`AUDIOUI_CLAUDE_PATH` 覆盖原生 CLI 路径。

---

## 版本与发布

- 当前版本：**0.2.0（M1 已完成；M2 进行中，收口后升 0.3.0）**
- 版本规范：[docs/release/versioning.md](docs/release/versioning.md)
- 发布流程：[docs/release/release-process.md](docs/release/release-process.md)
- 发布检查清单：[docs/release/release-checklist.md](docs/release/release-checklist.md)

---

## 许可证

待定（TBD）。在确定许可证之前，不要对外分发构建产物。
