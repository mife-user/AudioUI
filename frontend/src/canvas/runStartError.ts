import type { RunStartError } from '../../bindings/audio-ui/internal/domain/entity'

export function formatRunStartError(failure: RunStartError): string {
  switch (failure.code) {
    case 'AGENT_UNAVAILABLE':
      return 'Claude CLI 不可用。请在系统面板检查 CLI 安装状态后重试。'
    case 'BUSY':
      return '已有运行正在进行，当前无法启动另一个运行。'
    case 'NODE_LOCKED':
      return '此选题已应用候选并锁定，不能再次执行。'
    case 'PROMPT_TASK_EMPTY':
      return '请先填写提示词内容，再启动 Agent。'
    case 'RUNNING_CHAIN_LOCKED':
      return '此节点位于运行中的路径，当前不能编辑或再次执行。'
    case 'CONTEXT_TOO_LARGE':
      if (typeof failure.estimatedTokens === 'number' && typeof failure.tokenBudget === 'number') {
        return `上下文超过预算：估算 ${failure.estimatedTokens.toLocaleString()} tokens，预算 ${failure.tokenBudget.toLocaleString()} tokens。`
      }
      return '上下文超过预算，无法启动运行。'
    default:
      return `${failure.code}: ${failure.message}`
  }
}
