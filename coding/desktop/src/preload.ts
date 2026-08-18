import { contextBridge, ipcRenderer } from 'electron'

const previewOriginArgument = '--or-preview-origin='
const previewOrigin = process.argv
  .find((argument) => argument.startsWith(previewOriginArgument))
  ?.slice(previewOriginArgument.length) ?? ''

contextBridge.exposeInMainWorld('codingDesktop', {
  platform: process.platform,
  browserMode: 'webview',
  previewOrigin,
  chooseDirectory: (initialPath: string, title: string): Promise<string> =>
    ipcRenderer.invoke('desktop:choose-directory', initialPath, title),
  revealPath: (target: string): Promise<void> =>
    ipcRenderer.invoke('desktop:reveal-path', target),
  openExternalURL: (url: string): Promise<void> =>
    ipcRenderer.invoke('desktop:open-external', url),
  onBrowserOpenTab: (
    listener: (request: { openerWebContentsID: number; url: string }) => void,
  ): (() => void) => {
    const handler = (
      _event: Electron.IpcRendererEvent,
      request: { openerWebContentsID?: unknown; url?: unknown },
    ) => {
      if (
        typeof request?.openerWebContentsID !== 'number' ||
        typeof request?.url !== 'string'
      ) return
      listener({ openerWebContentsID: request.openerWebContentsID, url: request.url })
    }
    ipcRenderer.on('desktop:browser-open-tab', handler)
    return () => ipcRenderer.removeListener('desktop:browser-open-tab', handler)
  },
})
