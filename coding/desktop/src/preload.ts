import { contextBridge, ipcRenderer } from 'electron'

type BrowserState = {
  tabID: string
  url: string
  title: string
  loading: boolean
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

contextBridge.exposeInMainWorld('codingDesktop', {
  platform: process.platform,
  chooseDirectory: (initialPath: string, title: string): Promise<string> =>
    ipcRenderer.invoke('desktop:choose-directory', initialPath, title),
  openExternalURL: (url: string): Promise<void> =>
    ipcRenderer.invoke('desktop:open-external', url),
  browser: {
    show: (input: unknown): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:show', input),
    hide: (tabID: string): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:hide', tabID),
    close: (tabID: string): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:close', tabID),
    goBack: (tabID: string): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:go-back', tabID),
    goForward: (tabID: string): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:go-forward', tabID),
    reload: (tabID: string): Promise<void> =>
      ipcRenderer.invoke('desktop:browser:reload', tabID),
    onState: (listener: (state: BrowserState) => void): (() => void) => {
      const handler = (_event: Electron.IpcRendererEvent, state: BrowserState) => {
        listener(state)
      }
      ipcRenderer.on('desktop:browser:state', handler)
      return () => ipcRenderer.removeListener('desktop:browser:state', handler)
    },
  },
})
