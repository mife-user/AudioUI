# AudioUI

本地运行的、以节点图编排的内容生产工作台。用户在一张无限画布上完成选题、提示词、Agent 生成、渲染、配音和导出，再把素材交给剪映成片。

> **当前状态**：M0、M1 已验收，M2 已于 2026-11-08 通过用户整体验收；画布交互优化作为 M2 后、M3 前的独立工作批次进行中，M3 尚未开始。以[路线图](docs/dev/roadmap.md)为准。
> **平台**：Windows 11 · 绿色版单 exe · 应用数据落在 exe 目录（Agent 自身凭据除外）。
> **范围**：RM-018 本地脚本子范围已纳入并验收；RM-019 云端与 RM-020 视频仍待评估。


---

## 文档

文档入口和分类索引统一在 [docs/README.md](docs/README.md)。查看当前进行中的计划、M2 阶段手册或已完成阶段计划，请从[计划导航](docs/plans/README.md)进入。

常用入口：[当前工作计划](docs/plans/active/canvas-workbench-optimization-plan.md) · [M2 阶段任务手册](docs/plans/stages/M2/m2-workplan.md) · [路线图](docs/dev/roadmap.md) · [开发流程](docs/dev/process.md)。

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
├── docs/                   # 产品、设计、阶段计划与当前计划
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

- 当前版本：**0.2.0（M2 已通过用户验收；0.3.0 版本与 S6 收口待另行确认）**
- 版本规范：[docs/release/versioning.md](docs/release/versioning.md)
- 发布流程：[docs/release/release-process.md](docs/release/release-process.md)
- 发布检查清单：[docs/release/release-checklist.md](docs/release/release-checklist.md)

---

## 许可证

待定（TBD）。在确定许可证之前，不要对外分发构建产物。
