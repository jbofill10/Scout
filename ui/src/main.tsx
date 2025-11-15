import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import { initTelemetry } from './telemetry'
import { initLogger } from './lib/logger'

// Initialize OpenTelemetry before rendering
initTelemetry()
initLogger()

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
