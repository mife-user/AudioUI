import type { ReactNode } from 'react'

type Tone = 'ok' | 'warn' | 'bad'

type Props = {
  tone?: Tone
  children: ReactNode
  value?: string
}

export function StatusChip({ tone = 'ok', children, value }: Props) {
  return (
    <span className="chip">
      <span className={`dot${tone === 'ok' ? '' : ` ${tone === 'warn' ? 'warn' : 'bad'}`}`} />
      <span>{children}</span>
      {value ? <code>{value}</code> : null}
    </span>
  )
}
