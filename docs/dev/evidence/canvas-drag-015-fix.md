# 拖动渲染错误 015 修复证据

| 项目 | 结果 |
|---|---|
| 状态 | 实现和本地自检完成，待用户评审；不代表 M2/P0 验收 |
| 范围 | React Flow 节点视图状态、动态 Handle 测量、位置拖动持久化回归 |
| 测试数据 | 本地内存 fixture；未读取真实工程，未调用真实 Agent/CLI，未联网 |
| 浏览器 | 本地 Microsoft Edge headless，1410 × 858 |
| 桌面边界 | 未操作 Wails/WebView2 窗口；该平台项未验收 |

## 修复内容

- `frontend/src/canvas/nodeState.ts`：按当前工程 ID、节点 ID 和节点类型匹配受控节点；仅对有效测量保留 `measured`，并保留 React Flow 回传的 `dragging` / `resizing`。不传递内部 `internals`，工程切换、节点替换和类型变化会重新测量。
- 同一工程快照只改变非节点字段时，浅比较相同业务数据并复用旧节点对象，避免无关模型刷新使 React Flow 重建节点。
- `frontend/src/canvas/WorkspaceCanvas.tsx`：快照模型更新与本地节点状态合并；仍由权威快照提供业务字段、位置和选择态。拖动期间优先保留当前 pending 坐标；既有防抖、失败回滚和工程隔离逻辑未改变。新增子节点回调不再因仅选中节点变化而整体重建。
- `frontend/src/canvas/useMeasuredNodeHandles.ts`：移除“等待固定两帧即认为测量完成”的判断。请求 React Flow 重新测量，在其内部节点有有效宽高，且当前 DOM Handle 集合与 React Flow 已测 source/target Handle（包含方向和 ID）一致后，才向边模型报告当前布局已就绪；布局仍未就绪时订阅测量更新，工程/布局变化时清理旧订阅。
- `PromptNode` 和 `TopicNode` 按实际渲染的输入、输出、候选及占位 Handle 声明需要测量的端口。连接关系仍由领域图派生；等待端口测量不发 Connect、不隐藏图关系、不允许虚构候选端口。
- 增加 `nodeState.test.ts` 和 React Flow 快照选择集成回归；保留原错误编号与消息显示行为，没有通过吞错或清空错误条掩盖错误。

## 自动化验证

从 `AudioUI/` 运行：

| 命令 | 结果 |
|---|---|
| `npm --prefix frontend run test` | 17 个文件、107 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 通过 |
| `npm --prefix frontend run check:tokens` | 通过 |
| `go test . ./internal/...`（`GOPROXY=off GOSUMDB=off`） | 全部包通过 |
| `git diff --check` | 通过；仅报告工作区其他既有 LF/CRLF 转换提示 |

新增自动化验证包括：保留同身份节点测量与交互状态、跨项目/节点/类型不复用、无效尺寸不作为已初始化节点、无关快照刷新复用相同节点对象，以及真实 React Flow 中选中节点后重建工作台快照、再用方向键移动并写入位置命令时连线/端口仍保持有效。

## 浏览器拖动验收

使用真实 `WorkspacePage`、Zustand 工作台 store、React Flow 与内存 `GraphAPI` fixture。为了覆盖竞态，夹具在开始拖动后将 ResizeObserver 回调延后 500ms；实际工程没有访问或改写。

| 操作 | 结果 |
|---|---|
| 拖动未选中的草稿提示词 | 节点移动且可见；选中、MoveNode fixture 和快照同步顺序正确；无 008/015；连线 2 条 |
| 拖动已连接提示词 | 节点移动且可见；两条既有派生边持续可见；无 008/015 |
| 拖动选题节点 | 节点移动且可见；候选 Handle 与两条派生边保留；无 008/015 |
| 浏览器控制台异常 | 0 |
| 非本机 / 非 fixture 网络请求 | 0（浏览器网络拦截启用） |

- 截图：[canvas-drag-015-fixed.png](canvas-drag-015-fixed.png)
- 复现前证据：[canvas-drag-015-repro.png](canvas-drag-015-repro.png)

## 最终 exe

- 正式前端构建：`npm --prefix frontend run build` 通过。Vite 输出 JS 517.62 kB（gzip 162.10 kB），有单个 chunk 超过 500 kB 默认提示线的非阻塞警告。
- 原 `bin/audio-ui.exe` 未覆盖。尝试 `wails3 build` 时被本地自动权限策略拒绝（被判定可能覆盖生成物）；因此未重试相同动作。为满足用户要求并保留旧文件，使用已缓存依赖及离线环境，通过等效 Windows production Go build 单独生成 `bin/audio-ui-fixed.exe`。
- 构建目标：Windows amd64，CGO 关闭，`production` tag，版本 `0.2.0`；`GOPROXY=off` / `GOSUMDB=off`，未访问网络。
- 文件大小：17,449,472 bytes；SHA-256：`42ee9e816600882a89dba54117448eecf4483c6f94ea7499bfe88c208732c854`。
- `bin/audio-ui-fixed.exe --version` 输出 `AudioUI v0.2.0`。
- 二进制内嵌的 `frontend/dist/index.html` 与 JS 入口 `/assets/index-GZrRhW-J.js`（517,625 bytes）均与本次构建资源逐字节匹配。原有 `bin/audio-ui.exe` SHA-256 仍为 `22efb31e9a533fe22785f8666f849bc602c62cdcf073d9f91951063109f74ab2`，未覆盖。

## 未覆盖

- 用户当前运行的实际 exe 和原始错误编号未提供；本次修复针对已受控复现的 React Flow 015，并同时强化 008 端口测量校验。
- 未在真实 Wails/WebView2 窗口操作节点、验收重启后位置持久化，亦未自动化打开桌面 GUI。不能据浏览器证据宣称这些平台验收通过。
- 原有 `bin/audio-ui.exe` 保留未覆盖；新修复产物在 `bin/audio-ui-fixed.exe`。
