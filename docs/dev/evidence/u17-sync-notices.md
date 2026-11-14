# U17 正常同步提示与画布抖动自检

## 状态

用户于 2026-11-14 确认 U17 验收通过。

## 实现

- 已有快照显示期间不再插入全局同步加载提示；首次加载且尚无快照时仍显示加载状态。
- 节点编辑、布局和图命令的正常提交/同步不再显示全局成功提示；节点自身保存状态保持不变。
- 画布同步错误和页面级快照错误显示在画布视口内的浮层中，不改变画布布局；保留关闭错误、页面重试及仅重试快照同步入口。
- 同步成功后不重复创建节点或写入布局；页面级错误的重试仍由原回调执行。

## 自动化与构建

在仓库根目录 `D:\vscode\AI\bilibili\AudioUI` 执行：

| 检查 | 结果 |
|---|---|
| `npm --prefix frontend run test -- src/ui/WorkspacePage.test.tsx src/canvas/WorkspaceCanvas.test.tsx` | 2 个测试文件、28 项通过 |
| `npm --prefix frontend run test` | 24 个测试文件、223 项通过 |
| `npm --prefix frontend run typecheck` | 通过 |
| `npm --prefix frontend run lint` | 0 错误；2 条既存 `react-refresh/only-export-components` 警告 |
| `npm --prefix frontend run check:tokens` | 通过 |
| `npm --prefix frontend run build` | 通过；Vite 报告现有主 chunk 大于 500 kB |
| `GOPROXY=off GOSUMDB=off npm_config_offline=true wails3 build` | 通过，生成 `bin/audio-ui.exe`；构建未访问网络 |

定向测试覆盖已有快照加载时不显示正常提示、初次无快照的加载状态保留、成功同步不发出全局通知、页面级同步失败可重试、画布同步失败可仅重试快照且不会重复创建节点。错误提示被渲染在画布视口内，样式使用绝对定位覆盖而非参与布局。

## 验收边界

助手未启动或操控 Wails、浏览器、真实 Agent 或网络。用户于 2026-11-14 确认 U17 真实页面验收通过；具体逐项操作记录和截图未提供，本记录不补写。代码自动化与离线构建结果仍不替代用户验收。
