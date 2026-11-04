export type ConsoleTab = 'conversation' | 'pending'

type Props = {
  active: ConsoleTab
  pendingCount: number
  onChange: (tab: ConsoleTab) => void
}

export function ConsoleTabs({ active, pendingCount, onChange }: Props) {
  return (
    <div className="console-tabs" role="tablist" aria-label="运行控制台">
      <button
        id="console-tab-conversation"
        type="button"
        role="tab"
        aria-selected={active === 'conversation'}
        aria-controls="console-panel-conversation"
        onClick={() => onChange('conversation')}
      >
        对话
      </button>
      <button
        id="console-tab-pending"
        type="button"
        role="tab"
        aria-selected={active === 'pending'}
        aria-controls="console-panel-pending"
        onClick={() => onChange('pending')}
      >
        待处理 <span>{pendingCount}</span>
      </button>
    </div>
  )
}
