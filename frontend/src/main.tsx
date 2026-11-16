import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './styles/global.css'
import '@xyflow/react/dist/style.css'
import './styles/agent.css'
import './styles/workspace.css'

const root = ReactDOM.createRoot(document.getElementById('root') as HTMLElement)
const showFixture = import.meta.env.DEV && new URLSearchParams(window.location.search).get('fixture') === 'u9'

// Vite replaces DEV at build time, keeping the fixture out of production.
if (showFixture) {
  void import('./dev/U9IntegrationFixture').then(({ U9IntegrationFixture }) => {
    root.render(
      <React.StrictMode>
        <U9IntegrationFixture />
      </React.StrictMode>,
    )
  })
} else {
  root.render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  )
}
