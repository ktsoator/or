import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import './i18n.tsx'
import App from './App.tsx'
import { applyAppearancePreferences, readAppearancePreferences } from './lib/appearance.ts'
import {
  desktopPlatform,
  hasDesktopRuntime,
  openExternalURL,
} from './lib/desktop.ts'

applyAppearancePreferences(readAppearancePreferences())

if (hasDesktopRuntime()) {
  document.addEventListener('click', (event) => {
    const target = event.target
    if (!(target instanceof Element)) return

    const anchor = target.closest<HTMLAnchorElement>('a[href]')
    if (!anchor) return

    const url = new URL(anchor.href, window.location.href)
    if (!['http:', 'https:', 'mailto:', 'tel:'].includes(url.protocol)) return
    if (
      (url.protocol === 'http:' || url.protocol === 'https:') &&
      url.origin === window.location.origin
    ) {
      return
    }

    event.preventDefault()
    openExternalURL(url.href)
  })
}

if (desktopPlatform() === 'darwin') {
  document.documentElement.classList.add('desktop-macos')
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
