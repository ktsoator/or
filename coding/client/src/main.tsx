import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n.tsx'
import App from './App.tsx'
import { applyAppearancePreferences, readAppearancePreferences } from './lib/appearance.ts'

applyAppearancePreferences(readAppearancePreferences())

type DesktopRuntime = {
  WindowToggleMaximise: () => void
}

const desktopRuntime = (window as Window & { runtime?: Partial<DesktopRuntime> }).runtime

if (desktopRuntime && navigator.platform.startsWith('Mac')) {
  document.documentElement.classList.add('wails-macos')

  document.addEventListener('dblclick', (event) => {
    const target = event.target
    if (!(target instanceof Element)) return
    if (getComputedStyle(target).getPropertyValue('--wails-draggable').trim() !== 'drag') return
    if (typeof desktopRuntime.WindowToggleMaximise !== 'function') return

    event.preventDefault()
    desktopRuntime.WindowToggleMaximise()
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
