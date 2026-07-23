import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n.tsx'
import App from './App.tsx'
import { applyAppearancePreferences, readAppearancePreferences } from './lib/appearance.ts'

applyAppearancePreferences(readAppearancePreferences())

type DesktopRuntime = {
  BrowserOpenURL: (url: string) => void
  WindowToggleMaximise: () => void
}

const desktopRuntime = (window as Window & { runtime?: Partial<DesktopRuntime> }).runtime

if (desktopRuntime) {
  document.addEventListener('click', (event) => {
    const target = event.target
    if (!(target instanceof Element)) return

    const anchor = target.closest<HTMLAnchorElement>('a[href]')
    if (!anchor || typeof desktopRuntime.BrowserOpenURL !== 'function') return

    const url = new URL(anchor.href, window.location.href)
    if (!['http:', 'https:', 'mailto:', 'tel:'].includes(url.protocol)) return
    if (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.origin === window.location.origin
    ) {
      return
    }

    event.preventDefault()
    desktopRuntime.BrowserOpenURL(url.href)
  })
}

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
