type DesktopBridge = {
  ChooseDirectory: (initialPath: string, title: string) => Promise<string>
}

type WailsWindow = Window & {
  go?: {
    main?: {
      DesktopBridge?: Partial<DesktopBridge>
    }
  }
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
