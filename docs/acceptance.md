# 验收标准

> 约定：每条验收用例使用 Given-When-Then，必须可测试、无歧义、覆盖异常。
> 发布门禁：所有 P0 用例通过；冒烟测试通过；无 blocker。

## 1. 里程碑验收

### M0 壳与绿色版

| 用例 | 内容 |
|---|---|
| AC-M0-1 | Given 干净 Windows 11，When 双击 exe，Then 窗口打开且不写入 AppData |
| AC-M0-2 | Given 首次启动，When 查看目录，Then `data/`、`logs/`、`webview2/` 在 exe 同级生成 |
| AC-M0-3 | Given CLI 未安装，When 启动，Then 明确提示安装方式，不崩溃 |
| AC-M0-4 | Given 已有实例，When 再次启动，Then 聚焦已有窗口，不重复开进程 |
| AC-M0-5 | Given 构建产物，When 点击顶栏版本徽标或运行 `--version`，Then 显示 `0.1.0` 且与 `VERSION`、构建配置、CHANGELOG 一致 |

### M1 Agent 会话

| 用例 | 内容 |
|---|---|
| AC-M1-1 | Given 已配置 MCP，When 发送一条消息，Then 收到流式回复 |
| AC-M1-2 | Given Agent 请求写文件，When 权限卡片出现，Then 允许/拒绝均生效并记审计日志 |
| AC-M1-3 | Given 会话已存在，When 重启应用，Then 自动 resume 当前链路（**M2 起由运行历史取代**） |
| AC-M1-4 | Given resume 失败，When 启动，Then 控制台显示确认卡片，选择新开会话或取消（**M2 起移除**） |
| AC-M1-5 | Given Agent 调用 `ask_user`，When 问题卡出现，Then 选择/其他文本回传，超时返回 `QUESTION_TIMEOUT` 且运行不阻塞 |

> M1 用例保留为历史验收记录。M2 起「链与会话分离」，AC-M1-3/4 不再适用；重启后展示运行历史，未完成运行标记 `interrupted`，由 AC-M2-9 覆盖。

### M2 图与提示词

| 用例 | 内容 |
|---|---|
| AC-M2-1 | Given 空画布，When 新增/连接/改挂节点，Then 单父、无环、非自身、topic 根校验生效，改挂移动整个子树 |
| AC-M2-2 | Given 提示词节点已应用首轮，When 尝试编辑内容或再次执行，Then 编辑与执行均被拒绝；可新增提示词节点或分支继续 |
| AC-M2-3 | Given 一次运行返回结构化结果，When 未点击应用，Then 节点内容不变 |
| AC-M2-4 | Given 点击应用，When 上下文与参数校验通过，Then 产生新轮次且下游标记为待刷新 |
| AC-M2-5 | Given 多条叶子链，When 切换链路，Then 视图切换且不占并发；另一链运行中执行返回 `BUSY` |
| AC-M2-6 | Given 已有父的提示词节点，When 端口拖拽改挂（含跨 topic 树或同一 topic 换候选），Then 提示影响面；确认后移动子树（同父换候选不移动子树）并迁移 steps；自连、成环、topic 作子节点被拒；源/目标树运行锁均生效 |
| AC-M2-7 | Given 0.2.0 数据，When 首次启动 0.3.0，Then 默认工程含旧对话（只读运行）与工作区副本，原文件保留 |
| AC-M2-8 | Given 分支路径，When 执行节点，Then 上下文只含该路径祖先最新轮次与所选候选，不含目标历史/兄弟/子节点；相同输入哈希一致 |
| AC-M2-9 | Given 运行中关闭应用，When 重启，Then 该运行标记 `interrupted`，历史可回看，不自动 resume |
| AC-M2-10 | Given 上下文估算超过 `contextTokenBudget`（默认 1,000,000），When 执行，Then 按祖先降级；仍超则拒绝并返回估算值与预算 |
| AC-M2-11 | Given 运行/排队路径，When 编辑该路径节点、改挂或新增子节点，Then 拒绝；路径外兄弟分支可编辑；Agent 内置 Read/Glob/Grep 只可读该运行路径的祖先/当前节点 steps，Write/Edit 只可写当前运行节点 steps；MCP 文件读取只返回当前路径叠加及 exe 同级 runtime 的可见内容，兄弟节点、其他 topic 树与越界内容不可见 |
| AC-M2-12 | Given topic 的多个候选端口，When 分别连接分支并执行，Then 每条分支的上下文取对应候选；同一候选的分支共享该候选 |

> M2 的 topic/prompt 节点应用后即锁定内容与执行，每节点最多一个已应用轮次；继续迭代通过新增下游节点或候选端口分支。`SelectNode` 与快捷栏切换只改视图，不递增图 revision、不广播快照。

### M3 渲染

| 用例 | 内容 |
|---|---|
| AC-M3-1 | Given 有效场景 DSL，When 预览，Then 画面与 DSL 一致 |
| AC-M3-2 | Given 相同输入与版本，When 连续渲染两次，Then 输出帧一致 |
| AC-M3-3 | Given 应用新产物，When 防抖结束，Then 最近渲染节点自动刷新，其余标记过期 |
| AC-M3-4 | Given 未知图层类型，When 渲染，Then 明确报错，不静默跳过 |

### M4 配音与导出

| 用例 | 内容 |
|---|---|
| AC-M4-1 | Given 旁白文本，When 生成配音，Then 返回音频与时长 |
| AC-M4-2 | Given 有配音，When 渲染，Then 场景时长由音频时长决定 |
| AC-M4-3 | Given 完整场景，When 导出，Then 生成 MP4、SRT、script.md |
| AC-M4-4 | Given 导出产物，When 导入剪映，Then 可正常播放与编辑 |

> F-09 配音为 P1：AC-M4-1/2 不阻塞 M4 的 P0 收口，但 1.0 前必须完成或转入路线图暂存池。

### M5 队列与回收站

| 用例 | 内容 |
|---|---|
| AC-M5-1 | Given 并发已满，When 发送指令，Then 进入队列并显示位置 |
| AC-M5-2 | Given 队列项，When 拖动排序/取消，Then 顺序与状态更新 |
| AC-M5-3 | Given 运行中回合，When 停止，Then 释放并发位并触发下一个出队 |
| AC-M5-4 | Given 删除中间节点，When 确认，Then 连带删除下游并进回收站 |
| AC-M5-5 | Given 回收站内项目，When 24 小时内恢复，Then 节点与产物恢复 |

---

## 2. 功能验收（AC 编号对应 PRD）

| 编号 | 验收要点 |
|---|---|
| AC-01 | 新建/打开/切换项目；重启后恢复 |
| AC-02 | 拖拽平移、滚轮缩放；节点位置持久化 |
| AC-03 | 单父约束；自由改挂（含同一 topic 换候选确认）；整条路径高亮；快捷栏按叶子链切换不占并发、不触发图变更 |
| AC-04 | 输入想法 → 候选 ≥3 → 应用 → 候选端口连线建分支；可多个候选、同一候选可再分叉 |
| AC-05 | 提示词节点应用首轮后锁定内容与执行；新增节点或分支可继续 |
| AC-06 | 每次运行内流式回复；工具调用可见；权限可确认；问题卡可选择 |
| AC-07 | 待应用产物可预览、应用、丢弃；应用后新轮次 |
| AC-08 | 应用后自动刷新预览；失败保留旧预览 |
| AC-09 | edge-tts 生成音频；可关闭 |
| AC-10 | 导出 MP4/SRT/script.md；文件可播放 |
| AC-11 | 并发上限 3 个在途运行；排队、取消、停止、排序（M5） |
| AC-12 | 级联删除；影响面预览；回收站 24 小时 |
| AC-13 | 三档权限；越界硬拒绝；审计日志含 runId/chainId/nodeId |
| AC-14 | 绿色目录；zap 日志；日志面板可见 |

---

## 3. 非功能验收

| 项 | 标准 |
|---|---|
| 启动 | 参考机冷启动 ≤ 3 秒 |
| 预览 | 1080p30 播放不卡顿 |
| 导出 | 快于实时 |
| 内存 | 3 个活跃运行 ≤ 1.5 GB；M2 全局单在途 |
| 上下文 | 默认 1,000,000 tokens；超预算降级或拒绝；记录估算与实际 usage |
| 安全 | 仅本机监听；每运行独立 token；无远程遥测 |
| 兼容 | Windows 11 22H2+；WebView2 可用 |
| 恢复 | 工程崩溃后可打开，无半损坏 JSON |

---

## 4. 测试矩阵

| 类型 | 范围 | 工具/方式 |
|---|---|---|
| 单元 | domain 实体、校验、缓存键 | Go test |
| 上下文 | 路径祖先、候选解析、token 估算、预算降级、哈希 | Go test + golden |
| 运行 | 一次性会话、TurnGate、中断标记、运行路径锁、每运行 token | Go test + live CLI |
| 图 | 改挂/子树移动、候选端口、单父/无环/运行锁 | Go test |
| 协议 | stream-json 事件解析、权限应答、reply/read_artifact | 回放 fixture + MCP 集成 |
| 渲染 | DSL 解析、golden frame | 截图对比 |
| 集成 | 工程读写、队列、删除、回收 | Go test + 临时目录 |
| 手工 | 画布、控制台、导出、剪映导入 | 冒烟清单 |

---

## 5. 发布门禁

里程碑验收按 [开发流程](dev/process.md) 的 S5 执行；本文提供用例与发布门禁。

1. P0 用例 100% 通过。
2. 冒烟测试在干净 Windows 通过。
3. 版本一致性检查通过（见 [release/versioning.md](release/versioning.md)）。
4. 无 blocker / critical 缺陷。
5. 发布说明、检查清单、CHANGELOG 已更新并归档。
