import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n.tsx'
import App from './App.tsx'
import { applyAppearancePreferences, readAppearancePreferences } from './lib/appearance.ts'

applyAppearancePreferences(readAppearancePreferences())

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
