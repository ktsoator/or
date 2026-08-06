import type {
  DetailedHTMLProps,
  HTMLAttributes,
  Ref,
} from 'react'
import type { BrowserWebviewElement } from './features/browser/webviewBrowser'

declare module 'react' {
  namespace JSX {
    interface IntrinsicElements {
      webview: DetailedHTMLProps<HTMLAttributes<BrowserWebviewElement>, BrowserWebviewElement> & {
        allowpopups?: boolean
        ref?: Ref<BrowserWebviewElement>
        src?: string
      }
    }
  }
}
