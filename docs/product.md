# AudioUI 产品文档

| 项 | 内容 |
|---|---|
| 版本 | 0.2.0 |
| 状态 | 设计定稿草案 |
| 最后更新 | 2026-11-01 |
| 适用对象 | 产品、开发、设计 |
| 交互原型 | [`design/style-directions.html`](design/style-directions.html) · [`design/m2-workspace.html`](design/m2-workspace.html)（草稿，不等于最终样式） |

> 本文是产品总览。需求细节见 [PRD](prd.md)，技术方案见 [架构](architecture.md)，
> Agent 接入见 [协议](agent-protocol.md)，验收见 [验收标准](acceptance.md)，
> 进度与暂存见 [路线图](dev/roadmap.md)，开发做法见 [开发流程](dev/process.md)，
> 发布规范见 [release/](release/)。

---

## 1. 产品定位

### 1.1 一句话

AudioUI 是一个本地运行的、以节点图编排的内容生产工作台：用户在一张无限画布上完成选题、提示词、Agent 生成、渲染、配音和导出，再把素材交给剪映成片。

### 1.2 要解决的问题

- 内容生产散落在聊天、文档、剪辑软件之间，流程不可见。
- AI 生成结果不可复现、不可版本化、不可回滚。
- 每期视频从零开始，无法复用上一期的结构。
- 订阅制 AI 视频工具成本高，数据不在本地。

### 1.3 目标用户

- 单人创作者，具备开发能力。
- 零预算或极低预算。
- Windows 11。
- v1 垂直方向：音游作曲解析。

### 1.4 核心价值

| 价值 | 说明 |
|---|---|
| 节点即流程 | 把生产步骤变成可复用、可组合的节点 |
| 产物可追溯 | 每个节点有轮次、增量、产物哈希 |
| Agent 可替换 | 领域接口 + 能力位，不绑定单一供应商 |
| 本地优先 | 绿色 exe，数据在本地，无订阅 |
| 确定渲染 | 同一输入、同一版本、同一输出 |

### 1.5 成功标准

| 阶段 | 标准 |
|---|---|
| 首条链路 | 跑通一条链路，产出第一条完整视频 |
| 效率 | 单条视频制作 ≤ 3 小时；同模板第二条 ≤ 2 小时 |
| 复用 | 稳定更新；沉淀 3 套可复用节点模板 |

---

## 2. 核心概念

| 概念 | 定义 |
|---|---|
| **工作台 / 项目** | 一个工程，包含图、链路、素材、配置、运行记录 |
| **节点** | 图上的一个步骤，有类型、参数、输入输出、轮次 |
| **链路** | 一个 topic 根到某个末端节点的路径；分支产生多条共享前缀的链；链不绑定 Agent 会话 |
| **候选端口** | topic 的每个候选是一个输出端口；连接某个候选端口的分支使用该候选 |
| **运行** | 一次节点执行；启动一次性 Agent 会话，App 注入上下文，结束即销毁 |
| **轮次** | 同一节点的一次输出，只增不改 |
| **增量** | 节点写过的文件，存放在 `steps/<nodeId>/` |
| **运行时** | 所有 topic 树共享的渲染框架、组件、素材，位于 exe 同级 `runtime/`，只读 |
| **产物** | 应用后的不可变结果，按哈希存储 |
| **待应用产物** | Agent 已生成、尚未写入节点的结构化结果 |

关系：

```
工作台
└── topic 树
    └── 节点（单父；可自由改挂）
        ├── 候选端口（topic）
        ├── 运行（一次性执行、上下文、transcript）
        ├── 轮次（应用后的历史版本）
        └── 增量（steps/<nodeId>/）

链 = topic 根 → 某个末端节点的路径（派生，不持久化）
```

规则：

1. 图是单父森林：每个节点最多一个父；topic 必须无父且是树根；成环、自连、topic 作子节点拒绝。
2. 任意 prompt 节点（含其子树）可自由改挂，允许跨 topic 树；跨树时迁移子树 steps；运行/排队路径上的节点不可改。
3. topic 应用候选后锁定内容与执行；提示词应用首轮后锁定内容与执行；锁定后继续迭代只能新增下游节点或从候选端口开分支；非运行链的拓扑仍可改。
4. 每次运行启动一次性 Agent 会话，运行之间不共享上下文；运行/排队路径与共享祖先只读，路径外分支可编辑。

---

## 3. 用户流程

| 步骤 | 操作 | 结果 |
|---|---|---|
| 1 | 新建工作台 | 生成项目和空画布 |
| 2 | 打开选题节点，输入一句想法 | Agent 生成若干候选选题 |
| 3 | 从某个候选端口连线到提示词节点 | 建立该候选的分支；不同候选可各自分支，同一候选可再分叉；topic 应用候选后锁定内容 |
| 4 | 新增提示词节点，填写产出要求 | 指令进入该路径；应用首轮后锁定内容 |
| 5 | 执行节点 | App 构建上下文并启动一次运行；已有运行在途时提示忙碌 |
| 6 | Agent 返回结构化结果 | 控制台提示"待应用产物" |
| 7 | 在节点上点「应用」 | 产生新轮次；下游自动刷新预览 |
| 8 | 不满意 | 在当前节点后新增提示词节点，继续下一轮 |
| 9 | 配音 | TTS 节点生成音频，或跳过交给剪映 |
| 10 | 导出 | 输出分段 MP4、字幕、脚本 |
| 11 | 剪映成片 | 转场、配乐、精修在剪映完成 |

关键原则：

- **提示词只在提示词节点输入**，控制台不提供提示词输入。M2 已删除 M1 的临时 `PromptDock`。
- **Agent 只改增量，不改图**；结构化结果需人工应用。
- **候选端口即选择**：topic 的每个候选是一个输出端口；连接哪个端口，哪条分支就用哪个候选；没有单独的「选定」步骤。
- **任意改挂**：prompt 节点含子树可接到任意可用端口；同一父换候选同样按改挂确认并标记子树 stale；运行/排队路径与共享祖先只读，路径外分支可编辑。
- **每次运行由 App 注入上下文**：只含 topic 根 → 父节点路径上的最新已应用轮次；topic 段只带该路径选中的候选；目标自身历史、兄弟分支、子节点历史不进上下文。
- **视频由渲染节点执行产生**，不是 Agent 直接产出像素。

---

## 4. 功能范围

### 4.1 v1 范围

| 模块 | 能力 |
|---|---|
| 工作台 | 新建/打开项目；无限画布；拖拽平移、滚轮缩放 |
| 节点 | 新增、选中、删除、连线；树形约束校验 |
| 链路 | 节点高亮整条主链；快捷栏切换；分支共享链上下文 |
| 选题 | 输入想法 → Agent 生成候选 → 候选端口连线即选定 |
| 提示词 | 结构化指令；执行后锁定；新增节点继续迭代 |
| Agent | Claude Code（stream-json）；MCP HTTP；权限确认；并发与队列 |
| 渲染 | 场景 DSL；Canvas2D 预览；WebCodecs 导出 |
| 配音 | edge-tts；按时长驱动场景 |
| 导出 | 1080p30 H.264 MP4、SRT、script.md |
| 工作区 | topic 树内共享 + 节点增量；读按当前路径祖先叠加并回退到 exe 同级 `runtime/`；内置文件写入限当前运行节点；运行时只读；产物独立存储 |
| 删除 | 级联删除下游；回收站 24 小时 |
| 日志 | zap + lumberjack；界面日志面板 |

### 4.2 v2 方向

- 竖屏 9:16 模板。
- 自定义节点类型与节点模板库。
- 本地 TTS（MOSS-TTS-Nano）。
- 剪映草稿自动生成。
- 分叉增强、跨链路素材引用、主题系统。

状态与暂存情况见 [路线图](dev/roadmap.md)。

### 4.3 非目标

- 不做通用剪辑器。
- 不做云端协作与账号体系。
- 不做自动发布。
- 不允许 Agent 读取其他 topic 树或路径外节点。
- 不做手动 `/rewind` 入口。
- v1 不做竖屏。

---

## 5. 系统架构

### 5.1 技术栈

| 层 | 选型 |
|---|---|
| 桌面壳 | Wails v3（Windows WebView2） |
| 后端 | Go 1.26+ |
| 前端 | React 19 + TypeScript + Vite |
| 节点画布 | React Flow（@xyflow/react） |
| 状态 | Zustand |
| 渲染 | Canvas2D，确定性 `draw(t)` |
| 编码 | WebCodecs + mediabunny |
| 存储 | JSON 文件 + 内容寻址产物 |
| 日志 | zap + lumberjack |
| TTS | edge-tts（Go 实现） |
| Agent | Claude Code CLI，stream-json + MCP HTTP |

### 5.2 分层

```
presentation    Wails 窗口 / React 画布 / 渲染引擎
      ↓
handler         project / graph / agent / render / media
      ↓
service         project / workflow / run / context / render / media
      ↓
domain          entity / valueobject / event / port
      ↑
infra           json store / claudecli / edgetts / renderer bridge
```

依赖规则：domain 不依赖任何实现；service 只依赖 domain；infra 实现 domain 端口。

### 5.3 运行形态

- 单 exe、绿色版；Agent 运行使用同一 exe 的 host 辅助进程，不新增安装项。
- 所有数据在 exe 目录下，不写 AppData；WebView2 用户数据目录显式重定向。
- 首次启动检测 Claude CLI 与 WebView2，缺失时给出明确提示。
- 本机 MCP 服务仅监听 `127.0.0.1`，使用每运行独立 token。

---

## 6. 数据模型

### 6.1 目录

```
<exe>/
├── data/
│   ├── config.json                    # lastProjectId、contextTokenBudget
│   ├── projects/<projectId>/
│   │   ├── project.json               # 元信息、图、节点、轮次（链为派生视图）
│   │   ├── pending.json               # 待应用索引
│   │   ├── pending/<pendingId>.json   # 原始结构化输出（含无效）
│   │   ├── artifacts/<sha256>/        # 应用后的不可变产物
│   │   ├── runs.json                  # 运行索引
│   │   ├── runs/<runId>.json          # 上下文/预览 + transcript
│   │   └── trees/<rootTopicId>/steps/<nodeId>/  # topic 树 cwd 与节点增量
│   ├── agent/<projectId>/<runId>/mcp.json    # 每运行 MCP 配置
│   ├── backup/m1/                     # M1 会话与工作区备份（原文件不移动）
│   ├── workspace/chain_default/       # M1 遗留（只读备份）
│   └── cache/
├── runtime/                           # exe 同级随包资源（所有 topic 树共享，只读）
├── logs/
└── webview2/                          # WebView2 用户数据
```

### 6.2 图

```json
{
  "version": 1,
  "revision": 12,
  "activeNodeId": "n_b",
  "nodes": [
    {
      "id": "n_b",
      "type": "prompt",
      "title": "分镜脚本 A",
      "parent": "n_a",
      "parentCandidate": 0,
      "position": { "x": 200, "y": 70 },
      "params": {
        "instruction": "生成 3 个场景，左右分屏对比 BPM 切换",
        "outputKind": "script",
        "constraints": { "scenes": 3, "duration": 90 }
      },
      "rounds": [
        { "n": 1, "kind": "script", "artifactId": "sha256:...", "pendingId": "pa_...", "runId": "r_..." }
      ],
      "locked": true,
      "stale": false
    }
  ]
}
```

- `parent` 是唯一父；`parentCandidate` 只在父为 topic 时记录候选端口序号；边由两者派生，不落盘。
- `parentCandidate` 为 **0 基**候选序号，UI 显示为 1 基；父为 prompt 或节点为草稿时为空。
- `title` 是节点标题，创建时生成，未锁定前可修改；锁定后随内容一起冻结。
- `activeNodeId` 是当前选中节点；链由「topic 根 → 叶子/末端节点」派生，不持久化。链 ID 视图层为 `c_<tailNodeId>`。
- topic 应用候选后，每个候选是一个输出端口；子节点通过 `parentCandidate` 绑定候选。

### 6.3 运行

```json
{
  "version": 1,
  "runs": [
    {
      "id": "r_...",
      "chainId": "c_n_b",
      "chainPath": ["n_a", "n_b"],
      "nodeId": "n_b",
      "status": "succeeded",
      "contextVersion": 1,
      "contextHash": "sha256:...",
      "estimatedTokens": 12345,
      "actualInputTokens": 11200,
      "pendingId": "pa_...",
      "startedAt": "...",
      "finishedAt": "..."
    }
  ]
}
```

运行之间不共享 Agent 上下文；重启把未完成运行标记 `interrupted`，不自动 resume。`chainPath` 是「topic 根 → 运行目标」的快照，目标可能是内部节点；它不等同于快捷栏的叶子链，叶子链由当前树派生。运行记录中的 `chainId` 同样是运行时快照，仅用于历史展示；工程图不持久化链实体。

### 6.4 待应用与产物

待应用（pending）：

```json
{
  "id": "pa_...",
  "runId": "r_...",
  "chainId": "c_n_b",
  "chainPath": ["n_a", "n_b"],
  "nodeId": "n_b",
  "kind": "script",
  "status": "pending",
  "contextHash": "sha256:...",
  "ancestorHash": "sha256:...",
  "paramsHash": "sha256:...",
  "payload": "pending/pa_....json",
  "error": ""
}
```

应用后的产物：

```json
{
  "id": "sha256:...",
  "kind": "script",
  "nodeId": "n_b",
  "path": "artifacts/<sha256>/payload.json",
  "createdAt": "..."
}
```

### 6.5 不变量

1. 图是单父森林：每个节点最多一个父；topic 必须无父且是树根；边不悬空；成环、自连、topic 作子节点拒绝。
2. 节点 ID 项目内唯一；自由改挂移动整个子树。
3. 提示词节点应用首轮后锁定内容与执行；topic 应用候选后锁定内容与执行；锁定后不可再次执行，迭代通过新增下游节点或候选端口分支；非运行链的拓扑仍可改。
4. 应用后的产物不可变；修改产生新哈希。
5. 所有写操作经过命令层；Agent 不能直接改图；待应用产物只能由 UI 应用。
6. 删除节点必须同时处理其全部下游。
7. 运行与路径绑定；一次运行只读写本 topic 树；运行之间不共享 Agent 上下文。
8. 每次运行的上下文只含该路径祖先的最新轮次与 topic 候选；不含目标自身历史、兄弟分支与子节点。
9. 并发按在途运行计数；同一节点同一时间只允许一个运行；运行/排队路径与共享祖先只读。
10. 链是 topic 根到末端节点的派生路径；分支产生多条链；链不持久化。

---

## 7. 节点类型（v1）

| 类型 | 执行者 | 输入 | 输出 | 参数 |
|---|---|---|---|---|
| `topic` | Agent + UI | 一句想法 | 候选选题；**每个候选一个输出端口** | 候选数量、方向约束 |
| `prompt` | Agent | 上游路径上下文（含 topic 候选） | 待应用产物（M2 为脚本） | 指令、产出类型、场景数、时长 |
| `tts` | Go | 旁白文本 | 音频产物 | 音色、语速 |
| `render` | 前端 | 脚本 / 场景 DSL + 音频 | 预览视频 | 分辨率、帧率、主题 |
| `export` | 前端 + Go | 预览视频 + 音频 | MP4、SRT、script.md | 格式、文件名 |

### 7.1 提示词节点的产出类型

v1 支持三种 `outputKind`：

- `script`：结构化分镜脚本（含旁白与场景 DSL）。
- `scene_dsl`：直接输出场景 DSL（M3 接入校验与渲染）。
- `topics`：候选选题（topic 节点使用）。

M2 只接收 `topics` 与 `script`；`scene_dsl` 在 M3-01 接入。topic 是 prompt 的特殊子类型：共享参数/执行/锁定逻辑，额外提供候选端口。节点应用首轮/候选后锁定内容与执行；要修改产出，在当前节点后新增提示词节点，或从候选端口开新分支。

### 7.2 自定义节点

v2 引入节点类型注册表：每个类型带 JSON Schema、端口、执行者。MCP 的 `list_node_types` 与 UI 属性面板都由注册表生成。v1 不允许自定义。

---

## 8. 场景 DSL

### 8.1 脚本结构

```json
{
  "title": "BPM 砍半，手感就变了",
  "scenes": [
    {
      "id": "s1",
      "narration": "先听 174 的原速。",
      "duration": 6.0,
      "dsl": { "version": 1, "size": [1920, 1080], "duration": 6.0, "layers": [] }
    }
  ]
}
```

### 8.2 场景结构

```json
{
  "version": 1,
  "size": [1920, 1080],
  "duration": 6.0,
  "background": { "type": "solid", "color": "#0a0e11" },
  "layers": [
    {
      "id": "l1",
      "type": "text",
      "text": "BPM 从 174 切到 87",
      "box": [120, 320, 1400, 160],
      "style": { "size": 64, "weight": 700, "color": "#ffffff" },
      "anim": [
        { "prop": "opacity", "from": 0, "to": 1, "start": 0, "dur": 0.4, "ease": "easeOut" }
      ]
    }
  ]
}
```

### 8.3 原语

| 类型 | 用途 |
|---|---|
| `text` | 标题、正文、标注 |
| `rect` | 色块、边框、强调条 |
| `line` | 分割线、指示线 |
| `image` | 截图、谱面、封面 |
| `chart` | 柱状图、折线图 |
| `group` | 图层组合 |

动画属性：`opacity`、`translateX`、`translateY`、`scale`。缓动：`linear`、`easeIn`、`easeOut`、`easeInOut`。

### 8.4 规则

1. 使用 1920×1080 设计坐标，按分辨率缩放。
2. 所有动画由时间 `t` 推导，禁止随机与当前时间。
3. 图层按数组顺序绘制。
4. 未知 `type` 直接报错；未知字段忽略。
5. DSL 版本变化必须提供迁移。

---

## 9. Agent 接入

### 9.1 形态

- Claude Code CLI，`stream-json`；每次节点执行启动**一次性运行**，结束即销毁。
- App 在发送前构建上下文：祖先链最新轮次、任务参数、工作区清单、输出契约；运行之间不共享 Agent 记忆。
- MCP 服务运行在 App 内，监听本机 HTTP；**每次运行独立 token**，工具只能访问本链路。
- App 通过 `control_request / can_use_tool` 处理权限确认。

### 9.2 能力位

适配器启动时上报：

```go
type Capabilities struct {
    MultiTurn          bool
    Streaming          bool
    ToolPermissions    bool
    ConversationRewind bool
    FileRewind         bool
    ForkAtCheckpoint   bool
    Resume             bool
}
```

v1 Claude：`ConversationRewind=false`，`FileRewind=true`。M2 不使用 Resume；运行级上下文使分叉/删除不再需要 fork 或重建会话。

### 9.3 MCP 工具

| 工具 | 作用 |
|---|---|
| `get_chain` / `get_graph` / `get_node` | 读取当前路径与节点摘要（含 `parentCandidate`，不含产物正文） |
| `list_node_types` / `validate` | 类型与参数说明；只校验不写入 |
| `reply(node_id, kind, payload)` | 生成待应用产物或聊天消息 |
| `read_artifact(artifactId, offset, limit)` | 按块读取当前运行路径祖先产物（≤16 KiB/次） |
| `read_file` / `list_dir` / `grep` / `stat` | 安全的只读文件操作 |
| `ask_user` | 运行内向用户提问 |

M2 不提供 `create_node` / `connect` / `update_params` / `delete_*` / `apply_artifact`；写图工具登记 RM-016，应用产物永远只由 UI 触发。`reply(kind=message)` 只进对话；`topics`/`script` 生成待应用产物；`scene_dsl` 在 M3 接入。

### 9.4 权限档位

| 档位 | 内容 |
|---|---|
| 自动允许 | 读工具、`validate`、`reply(message/structured)`、`read_artifact`、`ask_user` |
| 交互确认 | `Write`、`Edit`、`NotebookEdit`、Bash 写/解释器/未知命令 |
| 硬拒绝 | 越界路径、`runtime/`、`artifacts/`、其他 topic 树、路径外节点目录 |

### 9.5 运行与队列

- 每次节点执行一个运行；M2 全局同一时间只允许一个在途运行。
- 运行/排队路径（根 → 运行节点）与共享祖先只读；路径外兄弟分支可编辑。
- 并发按在途运行计数，默认 3；队列、取消、停止、排序在 M5。
- 运行结束即关闭进程；没有常驻空闲会话。
- 重启把未完成运行标记 `interrupted`，不自动 resume。

### 9.6 分叉与删除

- 候选端口分支/改挂：新运行只从该路径祖先构建上下文，不需要 fork。
- 删除（M5）：级联删除全部下游；取消运行、关闭进程、增量与产物进回收站。
- 回收站保留 24 小时，之后物理删除。

---

## 10. 工作区与文件

### 10.1 叠加视图

```
topic 树 cwd：trees/<rootTopicId>/
路径 n_a → n_b → n_c 的 MCP 逻辑读取顺序：
steps/n_c → steps/n_b → steps/n_a → <exe>/runtime/
```

- 工作区按 topic 树共享；同一树内分支共享文件。
- 读：MCP 只读逻辑视图沿当前运行路径向上查找，第一个命中生效，再回退到 exe 同级 `runtime/`；其他 topic 树和兄弟节点拒绝。内置文件工具的物理读取只开放当前路径祖先的 `steps/<nodeId>/`。
- 写：内置 Write/Edit/NotebookEdit 只能写 `steps/<当前运行节点>/`，并需权限确认。
- 修改父节点文件：在自己的目录放同路径文件，遮蔽父版本。
- `runtime/` 只读随包资源；Agent 只可经 MCP 叠加视图读取，不得写入。
- 结果产物：不在这棵树上，Agent 只能通过受限 MCP 工具读取当前路径祖先产物。

### 10.2 权限三档

| 档位 | 规则 |
|---|---|
| 自动允许 | 只读白名单命令，路径在工作区内 |
| 需要确认 | 写文件、解释执行、未知命令、重定向/管道；弹确认卡片 |
| 直接拒绝 | `..` 越界、绝对路径逃逸、受保护目录 |

确认卡片展示命令、cwd、风险、影响路径；支持允许一次 / 本次运行允许 / 拒绝；超时 120 秒视为拒绝。高风险命令不提供"本次运行允许"。

---

## 11. 界面

### 11.1 结构

> M0 自检壳与 M1 临时会话页已验收；M2 起主区为工作台画布，会话记录并入右侧控制台的「对话」页（按运行分组），临时指令栏 `PromptDock` 删除；控制台只处理确认、选择、通知与运行记录。

- **无限画布**：拖拽平移、滚轮缩放；节点自由摆放。
- **快捷栏**：浮在画布底部，展示**叶子链**卡片（topic 根 → 末端节点的路径）与状态（工作中 / 待处理 / 空闲）；分支产生多条共享前缀的链。
- **调度面板**（M5）：浮在右上角，展示 Agent 并发数与队列；M2 只在顶栏显示全局在途状态。
- **Agent 交互栏**：右侧可折叠，展开 322px，收起 46px；只处理确认、选择、通知，不输入提示词。
- **节点**：紧凑卡片，显示类型、状态、待应用数量、关联运行状态。

### 11.2 交互原则

- 点节点或快捷栏卡片只切换视图，不占用并发。
- 只有执行节点才进入调度。
- 删除节点前展示影响面。
- 选中节点/链时高亮「topic 根 → 末端节点」路径，其余变暗；运行/排队路径只读。
- 控制台收起时，待确认数量显示在竖栏。

### 11.3 视觉

- 科技但沉稳：石墨底、细线、单一青色强调色、等宽字体用于 ID 与状态。
- 发光只用于活动态。
- 最终视觉规范未定，v1 先使用当前方向。

---

## 12. 里程碑

批次按依赖排序；批次内按需求等级 P0 → P1 → P2 推进。不设开发时间表。

| 批次 | 交付 | 验收 |
|---|---|---|
| M0 | 壳、绿色目录、zap、CLI 检测、启动自检壳 | 双击 exe 可用；日志落在 exe 目录；版本与最近日志可见 |
| M1 | Agent 会话 + MCP + 权限 + 临时会话页/控制台 | 能对话、确认/拒绝、问题选择；会话可 resume（M2 起由运行历史取代） |
| M2 | 单父树 + 候选端口 + 改挂 + 选题 + 提示词 + 待应用产物 + 运行级上下文 | 一条路径从选题走到待应用；多路径单在途；运行路径只读；上下文隔离 |
| M3 | 场景 DSL + 渲染预览 | 应用产物后自动刷新预览 |
| M4 | TTS + 导出 MP4/SRT | 导出可进剪映的成片素材 |
| M5 | 队列、回收站、完整日志与审计、文档 | 并发/排队/删除闭环 |

状态、优先级与暂存需求见 [路线图](dev/roadmap.md)。

---

## 13. 风险

| 风险 | 影响 | 对策 |
|---|---|---|
| stream-json 协议随版本变化 | Agent 面板失效 | 锁 CLI 版本；协议层隔离；失败回退外部终端 |
| CLI 未安装 | 无法使用 Agent | 启动检测；给出安装指引 |
| WebCodecs/H.264 不可用 | 无法导出 | 启动探测；回退 WebM |
| Wails v3 较新 | API 变动 | 锁版本；壳层隔离 |
| 多运行内存与额度 | 卡顿/超支 | 并发上限 3；M2 单在途；上下文 token 预算 |
| Agent 输出漂移 | 校验失败 | Schema 校验；结构化错误；人工修复 |
| 只读 Bash 被绕过 | 工作区污染 | 严格解析；路径白名单；审计日志 |
| 用户规模扩大 | 工程变慢 | 先文件存储，后加索引 |
| 范围蔓延 | 烂尾 | 严格按 M0–M5；v2 再扩 |

---

## 14. 术语表

| 术语 | 含义 |
|---|---|
| 工作台 | 一个项目工程 |
| 链路 | 一个 topic 根到某个末端节点的路径；分支产生多条链 |
| 运行 | 一次节点执行；一次性 Agent 会话 + App 构建的上下文 |
| 轮次 | 同一节点的一次已应用输出 |
| 增量 | 节点改过的文件 |
| 运行时 | 共享只读的渲染框架与素材 |
| 产物 | 不可变的最终结果 |
| 待应用产物 | Agent 已生成、等待人工应用的结构化结果 |
| 能力位 | 适配器上报的功能开关 |

---

## 15. 假设与待确认

以下为 v1 默认假设，确认前可调整：

1. v1 只做 B 站横屏 16:9、1080p30；竖屏放 v2。
2. v1 内置 edge-tts，可关闭交给剪映。
3. 产品名为 **AudioUI**（原名 Bili Studio）。
4. v1 节点集为 `topic` / `prompt` / `tts` / `render` / `export` 五个。
5. 场景 DSL 原语为 text / rect / line / image / chart / group。
6. 并发上限 3 个在途运行；M2 全局单在途，空闲休眠在 M5 评估。
7. 回收站 24 小时。
8. v1 不做自定义节点、竖屏、剪映草稿、手动回退。

---

## 16. 决策记录

| 日期 | 决策 |
|---|---|
| 2026-10-30 | 采用 Wails v3；DDD 分层；接口驱动 |
| 2026-10-30 | 节点图 + 链路；一条链路一个 Agent 会话（已由 2026-11-01 运行级改造取代） |
| 2026-10-30 | 提示词只在节点输入；控制台只做交互 |
| 2026-10-31 | M1 临时会话页与 PromptDock；控制台 322px；ask_user 问题卡；默认复用 Claude profile |
| 2026-10-30 | 结构化产物手动应用 |
| 2026-10-31 | 工作区采用共享根目录 + 节点增量叠加（已由 2026-11-01 的 topic 树隔离取代） |
| 2026-10-31 | 结果产物独立存储，MCP 写入 |
| 2026-10-31 | 删除级联下游；回收站 24 小时 |
| 2026-10-31 | Bash 三档：自动允许 / 需要确认 / 直接拒绝 |
| 2026-10-31 | 不做手动 `/rewind`；能力位降级 |
| 2026-10-31 | v1 节点集与输出形态定稿 |
| 2026-11-01 | 链与会话分离；每次节点执行一次性运行，App 从祖先链构建上下文（默认 1,000,000 tokens）；运行间不共享记忆 |
| 2026-11-01 | 端口拖拽与草稿节点进 M2；Agent 写图工具移出 M2；`scene_dsl` 与主题 B/C 延后 M3 |
| 2026-11-01 | 链 = topic 根到末端节点的路径；分支产生多条共享前缀的链；链不持久化 |
| 2026-11-01 | topic 的每个候选是一个输出端口；连接即选择；无单独选定步骤 |
| 2026-11-01 | prompt 节点含子树可自由改挂（允许跨 topic 树并迁移 steps）；运行/排队路径与共享祖先只读，路径外分支可编辑 |
| 2026-11-01 | 锁定节点不可再次执行；M2 的 topic/prompt 每节点最多一个已应用轮次，迭代新增下游节点或分支 |
| 2026-11-01 | 同一 topic 换候选按改挂确认并标记子树 stale；SelectNode 不递增图 revision、不广播；派生链由 Go 快照返回 |
