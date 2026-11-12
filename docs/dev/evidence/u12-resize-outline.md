# U12 卡片缩放控制边框自检

| 项目 | 结果 |
|---|---|
| 状态 | 用户于 2026-11-12 确认页面验收通过 |
| 验收 | AC-U-23；回归 U3 八方向缩放与尺寸对话框 |
| 页面测试 | 用户于 2026-11-12 确认实际页面验收通过；助手未启动浏览器或 Wails 窗口。未提供逐项操作记录，故不补写具体观察 |

## 实现范围

- `NodeResizer` 的四边控制线设为透明，保留 React Flow 的四个边缘缩放控制和四个角部手柄。
- 移除控制线的旧透明度规则；保留边缘控制区的 8px 尺寸、角部手柄样式和尺寸操作。
- 普通卡片边框、选中状态、键盘焦点、运行锁下的尺寸设置均未更改。

## 自动化自检

- `npm_config_offline=true npm --prefix frontend run test -- src/canvas/WorkspaceCanvas.realFlow.test.tsx -t "hides the resize outline"`：1 项通过；真实 React Flow DOM 保留四边控制线与四角手柄，控制线颜色为透明。
- `npm_config_offline=true npm --prefix frontend run test -- src/canvas/CanvasNodes.test.tsx -t "adds bounded resize and keyboard size controls"`：5 项通过，覆盖五类节点的尺寸控制与透明控制线配置。
- `npm_config_offline=true npm --prefix frontend run typecheck`：通过。
- `npm_config_offline=true npm --prefix frontend run lint`：0 错误，`usePendingController.tsx` 有 2 条既存 Fast Refresh 警告。
- `npm_config_offline=true npm --prefix frontend run check:tokens`：通过。
- `npm_config_offline=true npm --prefix frontend run build`：通过；Vite 提示主 JS chunk 580.78 kB，超过 500 kB 提醒阈值。
- `CanvasNodes.test.tsx` 整文件运行超过 180 秒无测试进度，已停止；完整前端测试套件未计为通过。

## 页面验收范围

1. 检查五类节点选中和未选中时均不显示完整缩放框线，普通卡片边框与选中状态仍清晰。
2. 检查四边、四角缩放手柄及八方向调整仍可使用，尺寸对话框、键盘焦点和运行路径锁定时的布局操作正常。

[U10～U16 单文件交互原型](../../design/u10-u16-canvas-optimization-demo.html)用于查看视觉方向，不替代产品页面验收。
