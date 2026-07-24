export const browserInspectionTextLimit = 12_000

export type BrowserInspectionText = {
  visibleText: string
  truncated: boolean
}

// This is fixed product code, never caller-provided JavaScript. NativeBrowser
// executes it in an isolated world so a page cannot replace the DOM helpers it
// relies on.
export const browserInspectionScript = `(() => {
  const limit = ${browserInspectionTextLimit};
  const excluded = 'script,style,noscript,template,input,textarea,select,option,[hidden],[inert],[aria-hidden="true"],[contenteditable]:not([contenteditable="false"])';
  const chunks = [];
  let length = 0;
  let truncated = false;
  const root = document.body;
  if (!root) return { visibleText: '', truncated: false };
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
    const node = walker.currentNode;
    const parent = node.parentElement;
    if (!parent || parent.closest(excluded)) continue;
    const visible = typeof parent.checkVisibility === 'function'
      ? parent.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })
      : (() => {
          const style = getComputedStyle(parent);
          return style.display !== 'none' && style.visibility !== 'hidden' && style.visibility !== 'collapse' && style.opacity !== '0';
        })();
    if (!visible) continue;
    if (parent.getClientRects().length === 0) continue;
    const text = (node.nodeValue || '').replace(/\\s+/g, ' ').trim();
    if (!text) continue;
    const separator = chunks.length > 0 ? 1 : 0;
    const remaining = limit - length - separator;
    if (remaining <= 0) {
      truncated = true;
      break;
    }
    if (text.length > remaining) {
      chunks.push(text.slice(0, remaining));
      length = limit;
      truncated = true;
      break;
    }
    chunks.push(text);
    length += text.length + separator;
  }
  return { visibleText: chunks.join('\\n'), truncated };
})()`

export function isBrowserInspectionText(value: unknown): value is BrowserInspectionText {
  if (!value || typeof value !== 'object') return false
  const result = value as { visibleText?: unknown; truncated?: unknown }
  return typeof result.visibleText === 'string' && typeof result.truncated === 'boolean'
}

export function isAgentBrowserTabID(value: unknown): value is string {
  return typeof value === 'string' && /^preview:[^:]+$/.test(value)
}
