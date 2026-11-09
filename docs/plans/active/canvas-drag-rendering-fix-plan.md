# 提示词节点拖动渲染错误：诊断与修复计划

| 项目 | 内容 |
|---|---|
| 状态 | 实现与本地自检完成，待用户评审；不代表 M2/P0 验收 |
| 范围 | React Flow 受控节点、尺寸和端口测量、节点位置保存、相关回归 |
| 需求关联 | F-02 / AC-02；不改变图拓扑规则和提示词执行规则 |
| 未知项 | 用户本次错误原始编号和实际启动路径未提供；已验证并修复的目标是受控复现 015 |
| 限制 | 不调用真实 Agent/CLI、不读取真实工程、不自动化操作 Wails 桌面窗口 |

## 一、结论与证据等级

1. **已验证的缺陷**：修复前，节点选择、快照刷新等导致画布节点模型重建时，现有实现会以新节点对象覆盖受控节点，丢失 React Flow 维护的 `measured` 等视图状态。若拖动在重新测量完成前继续，React Flow 可能产生 **015：拖动未初始化节点**。该状态协调缺陷已修复。
2. **受控复现**：本地 Edge 浏览器、1410 × 858、真实 `WorkspacePage` / `WorkspaceCanvas` / React Flow、仅内存 GraphAPI fixture；人为延迟 `ResizeObserver` 回调后，从未选中状态拖动草稿提示词或已连接提示词，修复前观察到 015、最终修复后不再出现；[最终截图](../../dev/evidence/canvas-drag-015-fixed.png)与[修复前截图](../../dev/evidence/canvas-drag-015-repro.png)。
3. **构建差异**：初次诊断时，原 `bin/audio-ui.exe` 内嵌 `index-B_cH0elX.js`，而当时前端 dist 是 `index-B0FbMVaA.js`；表明原二进制未包含当时前端修复。现在独立 `bin/audio-ui-fixed.exe` 内嵌 `/assets/index-GZrRhW-J.js`，HTML/JS 与最新 `frontend/dist` 逐字节一致；原 exe 未覆盖。
4. **错误编号边界**：用户未提供本次完整错误编号。015 的受控复现是已修复的独立缺陷，但不能断言它就是用户所见错误。如果实际看到 008，仍需按用户原始 source / target Handle ID 与 edge ID 继续核对；其他编号按 React Flow 原始消息定位。

交互机制示意：[canvas-drag-measurement-diagnosis.html](../../design/canvas-drag-measurement-diagnosis.html)。这是独立模拟，不读取数据、不依赖外部资源，不是修复实现。

## 二、原因链路

| 次序 | 代码位置 | 行为 |
|---|---|---|
| 1 | `frontend/src/ui/WorkspacePage.tsx:31-39`、`frontend/src/stores/workspaceStore.ts:62-70` | 节点选择更新工作台快照中的 `activeNodeId`。 |
| 2 | `frontend/src/canvas/WorkspaceCanvas.tsx:359-370`、`frontend/src/canvas/nodeState.ts:20-56` | 修复前用 `model.nodes` 新对象直接覆盖本地受控节点；现在按工程/节点/类型合并有效尺寸与交互状态，并对等价节点复用对象。 |
| 3 | `frontend/src/canvas/flowModel.ts:126-151`、`frontend/src/canvas/nodeState.ts:20-56` | flow model 只构造业务状态；画布协调层为同身份节点补入临时 `measured` / `dragging` / `resizing`，不复制 React Flow 私有 internals。 |
| 4 | `frontend/src/canvas/WorkspaceCanvas.tsx:443-464` | `applyNodeChanges` 持续接收尺寸、选择和拖动变更；修复前模型替换可能抹掉这些本地视图状态，现在同身份节点通过协调函数保留有效状态。 |
| 5 | `frontend/node_modules/@xyflow/system/dist/esm/index.js:1674-1706`、`:425-456` | 当前安装版本从传入节点重建内部测量状态；拖动计算发现宽或高为 `undefined` 时调用 `onError('015', ...)`。 |
| 6 | `frontend/src/canvas/WorkspaceCanvas.tsx:740` | 所有 React Flow 错误被统一显示成“画布渲染错误 编号：消息”。这不是视频渲染引擎报错，也不单独证明工程数据损坏。 |

此外，修复前的 `useMeasuredNodeHandles.ts` 通过等待两个动画帧宣布端口布局可用，但帧数本身不能证明指定 Handle 已实际测量。这是 008 的独立时序风险；本次已改为核验有效尺寸及 DOM / React Flow 的 source、target Handle 集合与方向一致后再放行派生边。

## 三、实施步骤

### 0. 锁定真实版本和现象

- [ ] 对照用户实际启动路径、可执行文件及其内嵌资源，记录完整错误编号、原始英文消息和出现时动作；区分拖动中与松开后、草稿与有连线节点。
- [x] 不覆盖用户现有未提交修改，不为了诊断直接运行真实 CLI 或操作真实工程。
- [ ] 若看到 008，记录消息里的 source / target Handle ID 与 edge ID；若为其他编号，先依据其真实触发点调整对应修复，不用 015 解释所有错误。

### 1. 先增加失败回归

- [x] 建立真实 React Flow + `WorkspacePage` + Zustand 选择链路的安全回归，GraphAPI 和快照使用内存替身；浏览器实测延迟 ResizeObserver 回调后，从未选中提示词开始选择并连续拖动。
- [x] 在整个交互期间收集错误条变化与 React Flow 错误结果，而不是只检查最终状态；受控延迟下修复前复现 015，修复后无 015、节点和连线未异常隐藏。
- [x] 覆盖草稿提示词、topic → prompt、prompt → prompt，以及已选中 / 未选中节点；覆盖鼠标拖动和选中后方向键移动。
- [x] 覆盖拖动带来的选择快照更新、候选异步加载、位置保存成功/失败、工程切换取消排队写入；验证位置、连线和命令范围。

### 2. 按身份协调快照节点与视图节点

主要文件：`frontend/src/canvas/WorkspaceCanvas.tsx`；如需抽取纯函数，放在 `frontend/src/canvas/` 并配套单元测试。

- [x] 把全量替换改为函数式更新：按 **projectId + nodeId + node type** 查找本地已有节点，仅复用同身份节点的必要视图状态。新节点、已删除节点、类型变化及工程切换不复用旧测量。
- [x] 对同身份节点保留 React Flow 回传的 `measured` 和确需保留的拖动 / 缩放临时状态；业务 `data`、运行锁、候选、选中和图关系仍来自最新快照及既有选择逻辑。不复制 React Flow 私有 `internals`。
- [x] 原有 `applyNodeChanges` 继续处理尺寸、选择与位置变化；拖动期间不使用固定延时、两帧假设或屏蔽 015。
- [x] 保持位置优先级：当前拖动 / 尚未同步的本地最新坐标优先；权威快照确认后清除 pending。保留 240ms 防抖、逐节点队列、失败回滚、工程切换隔离；MoveNode 已提交但快照同步失败时仅重试快照读取。
- [x] 对业务数据浅比较并复用等价节点，减少普通快照刷新导致整图重建；节点真实尺寸变化仍交由 React Flow / ResizeObserver 重新测量。

### 3. 收紧动态 Handle 与边同步

主要文件：`frontend/src/canvas/useMeasuredNodeHandles.ts`、`frontend/src/canvas/flowModel.ts`、`frontend/src/canvas/TopicNode.tsx`、`frontend/src/canvas/PromptNode.tsx`。

- [x] 不以“已经过去两帧”作为唯一就绪证据；端口布局变化后请求 `useUpdateNodeInternals`，订阅并核验同工程、同节点、同布局 key 的有效尺寸，以及 DOM 和 React Flow 内部 source / target Handle（含方向和 ID）确实一致。
- [x] 以布局 key 使候选变化、草稿/连接状态变化、节点卸载重建和工程切换的旧就绪状态失效；不凭同名 Handle 的旧缓存放行。
- [x] 只在相关端口未就绪时暂缓对应边；就绪后恢复，不永久隐藏、不重发 `Connect`、不更改领域图。零尺寸、删除和迟到回调不标记就绪。
- [x] 为候选端口、占位锚点和 prompt 输入 / 输出端口声明实际需测 Handle；不虚构可连接候选。

### 4. 诊断与错误呈现

- [x] 保留 React Flow 真实错误编号与原始消息；015、008 与位置保存失败分开处理，不吞错、不靠同步成功清空错误作为验证。
- [x] 不新增工程数据诊断日志；本次以错误条回归和本地 fixture 记录安全验证结果，不记录提示词内容或上传内容。
- [x] 错误条的用户可见样式未改变；更新独立的交互诊断示意和证据文档。

### 5. 测试、构建与交付

- [x] 自动化：节点协调纯函数、真实 React Flow 快照选择与键盘移动、候选异步加载/动态 Handle、拖动失败回滚与工程切换；完整前端测试、typecheck、lint、token 检查与前端构建均通过。
- [x] 本地浏览器：使用隔离内存 fixture，真实拖动草稿、已连接提示词和 topic，受控延迟 ResizeObserver 回调；记录 viewport、截图、节点坐标同步和错误状态。缩放边界及位置保存失败由自动化回归覆盖。
- [ ] 桌面平台：Wails/WebView2 下的拖动、缩放和重启后位置持久化未操作；按项目约束不自动化驱动桌面窗口，需另行平台验收。
- [x] 前端构建后生成独立的 `bin/audio-ui-fixed.exe` 并核验其内嵌 html/JS 与 dist 完全一致。`wails3 build` 被本地安全策略拒绝后未重试；使用 GOPROXY/GOSUMDB 关闭的 Windows production Go build 生成新文件，未覆盖旧 exe。
- [x] 同步 CHANGELOG、文档地图与证据；保留 Wails 未覆盖项，交用户评审，不自行宣布 P0 验收通过。

## 四、验收标准

1. [x] 在受控测量延迟下，重复拖动草稿、已连接 prompt 与 topic，无 015；节点位置正确且不隐藏。
2. [x] 动态 / 静态 Handle 均经实际测量核验，浏览器拖动期间无 008；Handle 与候选索引和图关系一致，移动不调用 Connect。
3. [x] 自动化验证位置防抖、保存失败回滚、工程切换隔离；[ ] Wails/WebView2 重启后坐标持久化待平台验收。
4. [x] 前端门禁、本地浏览器实测与最终 exe 内嵌资源对照有证据；[ ] 用户原始错误编号及真实桌面窗口行为未确认。

## 五、验证证据

- 完整前端：17 个测试文件、107 项通过；typecheck、lint、token 检查通过。
- Go：`GOPROXY=off GOSUMDB=off go test . ./internal/...` 全部通过；`go mod tidy -diff` 离线检查无差异。
- 浏览器：本地 Edge 1410 × 858，内存 fixture；草稿/已连接提示词/topic 拖动，在 500ms ResizeObserver 延迟下均无 008/015，位置同步、连线持续可见，控制台错误 0，外网请求 0。截图见 [canvas-drag-015-fixed.png](../../dev/evidence/canvas-drag-015-fixed.png)。
- exe：`bin/audio-ui-fixed.exe --version` 输出 `AudioUI v0.2.0`；SHA-256 `42ee9e816600882a89dba54117448eecf4483c6f94ea7499bfe88c208732c854`；内嵌页面和 `/assets/index-GZrRhW-J.js` 与最新 `frontend/dist` 逐字节一致。旧 `bin/audio-ui.exe` 未覆盖。
- 构建边界：Wails CLI 构建被本地自动安全策略拒绝；没有重试或绕过。因用户明确要求交付 exe，使用离线等效 Go production build 输出独立的新文件。Wails/WebView2 GUI 行为仍需用户评审/平台验收。
