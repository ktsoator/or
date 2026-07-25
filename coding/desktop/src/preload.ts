import { contextBridge, ipcRenderer } from 'electron'

contextBridge.exposeInMainWorld('codingDesktop', {
  platform: process.platform,
  browserMode: 'webview',
  chooseDirectory: (initialPath: string, title: string): Promise<string> =>
    ipcRenderer.invoke('desktop:choose-directory', initialPath, title),
  openExternalURL: (url: string): Promise<void> =>
    ipcRenderer.invoke('desktop:open-external', url),
})
