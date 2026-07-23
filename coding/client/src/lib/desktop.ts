type DesktopBridge = {
  ChooseDirectory: (initialPath: string, title: string) => Promise<string>
}

type WailsWindow = Window & {
  runtime?: {
    BrowserOpenURL?: (url: string) => void
  }
  go?: {
    main?: {
      DesktopBridge?: Partial<DesktopBridge>
    }
  }
}

// Opens a URL outside Coding when the native runtime is available, with the
// browser's normal new-tab behavior as the web-client fallback.
export function openExternalURL(url: string): void {
  const open = (window as WailsWindow).runtime?.BrowserOpenURL
  if (typeof open === 'function') {
    open(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

// Returns undefined when the browser has no native desktop bridge. An empty
// string is a valid desktop response and means the user cancelled the dialog.
export async function chooseNativeDirectory(
  initialPath: string | undefined,
  title: string,
): Promise<string | undefined> {
  const bridge = (window as WailsWindow).go?.main?.DesktopBridge
  if (typeof bridge?.ChooseDirectory !== 'function') return undefined
  return bridge.ChooseDirectory(initialPath ?? '', title)
}
