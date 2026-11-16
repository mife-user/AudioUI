import type { ReactNode } from 'react'

export function AppShell({ children, className = '' }: { children: ReactNode; className?: string }) {
  return <div className={`shell${className ? ` ${className}` : ''}`}>{children}</div>
}
