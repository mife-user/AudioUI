# U21 底部链路浮出与快捷栏创建入口清理自检与验收

## 状态

用户于 2026-11-14 确认 AC-U-52～54 页面验收通过；详细操作记录和截图未提供，本证据不补写。助手未启动或操控浏览器、Wails 窗口。

## 实现

- `ChainQuickBar` 默认只在画布底边居中渲染 56 × 25 的上箭头按钮；列表绝对定位于按钮上方，以 1px 重叠避免指针跨入时出现间隙闪烁，不参与画布布局。
- 导航根节点统一处理悬停、焦点与固定状态：指针进入按钮或列表保持展开，单纯悬停移出后收起；点击按钮固定或关闭；点击外部关闭；Escape 关闭并把焦点交回按钮，且抑制回焦触发的重新展开。
- 浮出列表只显示链路名称和完整路径，长文本省略并保留 `title`；汇合项显示真实上游节点集合，不伪造单一路径；空快照仅在展开后显示「暂无链路」。
- 选择链路后收起浮层，只调用现有 `onSelectNode`；不递增图 revision、不启动或停止运行、不改变视口或图。
- 删除快捷栏的「新建节点」按钮、`onCreateNode` / `createNodeDisabled` 属性、运行/待处理状态摘要、本地存储折叠键及对应样式。画布键盘菜单继续使用 `openCreateMenuAtCenter`。
- 审计未发现快捷栏专属后端接口、生成绑定或前端包装：原入口调用与右键/键盘菜单共用的 `WorkflowService.AddNode`，因此本轮无可删除的后端专属代码，共用 `AddNode`、右键创建、节点内创建及 Agent 合法输出自动创建均保留。

## 自动化与构建

工作目录：`D:\vscode\AI\bilibili\AudioUI`。

| 检查 | 结果 |
|---|---|
| `npm --prefix frontend run test -- src/canvas/ChainQuickBar.test.tsx src/canvas/WorkspaceCanvas.test.tsx src/canvas/WorkspaceCanvas.realFlow.test.tsx src/canvas/flowModel.test.ts` | 4 个测试文件、75 项通过 |
| `npm --prefix frontend run test` | 24 个测试文件、236 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告，位于 `usePendingController.tsx` |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；Vite 提示未压缩 JS chunk 600.31 kB，超过 500 kB 提示线 |
| `GOPROXY=off GOSUMDB=off go vet . ./internal/...` | 通过 |
| `GOPROXY=off GOSUMDB=off go test . ./internal/...` | 全部通过 |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 离线构建通过，生成 `bin/audio-ui.exe`；用户于 2026-11-14 使用该构建完成页面验收 |
| `bin/audio-ui.exe --version` | 输出 `AudioUI v0.2.0` |
| `git diff --check` | 通过 |

## 测试覆盖

- 快捷栏：默认仅按钮、悬停展开/移出收起、点击固定与关闭、外部点击关闭、重新进入展开、焦点保持展开、Escape 回焦、选择链路只触发一次回调并收起、汇合真实上游集合、空链文案、无新建入口。
- 画布：既有叶子链选择经 `SelectNode` 且不触发图变更或启动运行；真实 React Flow DOM 下选择链路后边与版本展示稳定。
- 创建入口：画布右键两级菜单、键盘菜单、创建失败重试、在途重复创建拦截、同步期间禁用与「仅重试快照同步」回归；原快捷栏专属创建测试改为验证保留的画布键盘菜单流程。

## 用户验收与未覆盖项

- 用户于 2026-11-14 确认实际页面验收通过，AC-U-52～54 完成；具体各项操作结果和截图未提供，不补写。
- 助手按项目约定未启动或操控浏览器/Wails 窗口；WebView2 原生指针、DPI 100%/125%/150%、系统减少动效及触屏行为未由助手实测。
- 自动化与构建结果不等同于页面验收；本记录分别保留代码级门禁与用户验收结论。
