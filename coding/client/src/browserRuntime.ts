declare const browserRuntimeTabIDBrand: unique symbol

export type BrowserRuntimeTabID = string & {
  readonly [browserRuntimeTabIDBrand]: true
}

export function browserRuntimeTabID(
  workspaceID: string,
  tabID: string,
): BrowserRuntimeTabID {
  return `workspace:${encodeURIComponent(workspaceID)}:tab:${encodeURIComponent(tabID)}` as BrowserRuntimeTabID
}
