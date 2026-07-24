# Browser Controller Design

Status: Phase 1 and Phase 2 implemented, including Agent tab dispositions;
Phase 3 proposed

## Objective

Turn the right-side Browser from a display-only preview surface into a reliable,
session-aware browser controller. Phase 1 makes navigation and tab ownership
deterministic. Phase 2 adds an acknowledgement path so an agent can distinguish
a requested navigation from one that actually committed in Electron.

This design is independently implemented on Electron APIs. It borrows general
product patterns from mature browser automation systems, but does not reuse
proprietary implementation code.

## Original Problem

The renderer sent URL, navigation revision, bounds, and visibility through one
`browser.show` call. `BrowserSurface` also resolved the target URL in local React
state. On a second navigation, the new revision could be rendered before that
local URL caught up, producing a command with the new revision and previous URL.

This has three structural causes:

1. Layout synchronization and navigation share one command.
2. Desired navigation state and observed page state share the same fields.
3. Electron state events do not identify the navigation revision they describe.

## User Semantics

Each coding session has one deterministic reusable Agent tab:

```text
agent:<session-id>
```

The default agent action reuses that tab. The existing `WebContentsView` stays
alive so web-to-web navigation preserves back and forward history.

```text
"Open GitHub"                 -> reuse the session agent tab
"Now open Bilibili"          -> reuse the same tab
"Open Bilibili in a new tab" -> create and select a command-owned Agent tab
"Open Google in background"  -> create a command-owned Agent tab without selecting it
```

Tabs created by the user with the plus button are user-owned. Agent commands
must never replace a user-owned tab.

The supported dispositions are:

```ts
type BrowserDisposition =
  | 'reuse_agent_tab'
  | 'new_foreground_tab'
  | 'new_background_tab'
```

All three dispositions are exposed by `open_preview`. The default is
`reuse_agent_tab`; the model uses either new-tab disposition only when the user
explicitly asks for it. New tabs use a stable ID derived from the session and
browser command ID, so a pending command can be replayed after reconnect
without creating duplicates.

## Ownership

### Go product service

- Validate public, localhost, and workspace preview targets.
- Own agent browser requests and their durable session association.
- Persist the last requested preview for history restoration.
- Receive a terminal result for commands issued by the agent.
- Never own Electron view instances, bounds, active UI tabs, cookies, or page
  history.

### React renderer

- Own the workbench tab model and active tab selection.
- Route an agent request to its deterministic session tab.
- Keep every pending browser command until its result is acknowledged and route
  new-tab commands to stable command-owned tabs.
- Keep desired command state separate from observed page state.
- Ignore stale Electron events.
- Resolve localhost checks before issuing a navigation command.
- Render address, title, loading, failure, and history controls.

### Electron main process

- Own every `WebContentsView` and Electron `Session`.
- Apply navigation, visibility, and bounds commands.
- Enforce protocol, permission, cookie, and workspace isolation policies.
- Report main-frame navigation state with the applied revision.
- Preserve public-browser history and recreate a view when crossing the
  workspace-preview security boundary.

## Phase 1: Renderer and Electron Control Plane

Phase 1 fixes repeated navigation without changing the Go tool contract.

### Renderer tab state

Extract the tab model and reducer from `BrowserView.tsx` into
`browserTabs.ts`.

```ts
type BrowserTabOwner = 'agent' | 'user'
type BrowserTargetKind = 'web' | 'workspace-preview'

type DesiredNavigation = {
  revision: number
  requestedURL: string
  kind: BrowserTargetKind
  source: 'agent' | 'address' | 'reload'
}

type ObservedNavigation = {
  appliedRevision: number
  committedURL: string
  title: string
  status: 'idle' | 'navigating' | 'ready' | 'failed'
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

type BrowserTab = {
  id: string
  owner: BrowserTabOwner
  sessionID?: string
  addressDraft: string
  desired?: DesiredNavigation
  observed: ObservedNavigation
}
```

The address input writes only `addressDraft`. Submitting it creates a new
`desired` command. Electron state writes only `observed`. A committed observed
URL may update `addressDraft` only when its revision is not older than the
current desired revision.

### Revision rules

- Revisions are monotonic per tab.
- Every agent target, address submission, and explicit reload creates a new
  revision.
- Bounds, visibility, title, and loading changes never create a revision.
- A state event with `appliedRevision < desired.revision` is stale and cannot
  update URL, error, or terminal status.
- A state event for an unknown tab is ignored.
- Closing a tab invalidates all pending work for that tab.

### Desktop bridge

Replace the overloaded `show` API with independent operations:

```ts
type NativeBrowserNavigateInput = {
  tabID: string
  revision: number
  url: string
  kind: 'web' | 'workspace-preview'
}

type NativeBrowserViewportInput = {
  tabID: string
  visible: boolean
  bounds?: {
    x: number
    y: number
    width: number
    height: number
  }
}

type NativeBrowserState = {
  tabID: string
  appliedRevision: number
  requestedURL: string
  committedURL: string
  title: string
  status: 'navigating' | 'ready' | 'failed'
  canGoBack: boolean
  canGoForward: boolean
  error?: string
}

type NativeBrowserBridge = {
  navigate(input: NativeBrowserNavigateInput): Promise<NativeBrowserState>
  setViewport(input: NativeBrowserViewportInput): Promise<void>
  close(tabID: string): Promise<void>
  goBack(tabID: string): Promise<void>
  goForward(tabID: string): Promise<void>
  onState(listener: (state: NativeBrowserState) => void): () => void
}
```

`setViewport` must never cancel, reload, or supersede navigation. ResizeObserver
calls only `setViewport`.

### Native entry state

Electron keeps requested and committed state separately:

```ts
type BrowserEntry = {
  tabID: string
  kind: BrowserTargetKind
  view: WebContentsView
  appliedRevision: number
  requestedURL: string
  committedURL: string
  status: 'navigating' | 'ready' | 'failed'
  error?: string
}
```

`navigate` performs this sequence:

1. Parse and validate the target URL.
2. Ignore the command when its revision is older than `appliedRevision`.
3. Recreate the entry only when the target kind crosses the security boundary.
4. Record revision and requested URL before starting the load.
5. Stop a superseded main-frame load when necessary.
6. Call `loadURL` for a different target or `reload` for an explicit same-target
   revision.
7. Return a state carrying the same applied revision.

Main-frame `did-navigate` sets `committedURL`. `did-fail-load` sets `failed`
except for Electron's aborted-load error. Redirects are successful and retain
both requested and committed URLs.

### Browser controller hook

Extract native synchronization from `BrowserSurface.tsx` into
`useNativeBrowserController.ts`.

The hook has two independent effects:

- Navigation effect: watches the complete desired command and calls
  `browser.navigate` exactly once per revision.
- Viewport effect: watches bounds and visibility and calls
  `browser.setViewport` without touching navigation.

Public and workspace URLs are immediately resolved. Localhost validation uses
an abortable request keyed by its desired URL and revision. A result is discarded
unless it still matches the current desired command.

`BrowserSurface` becomes a viewport anchor plus loading and failure overlays. It
does not own URL state.

## Phase 2: Agent Command Acknowledgement

Phase 2 changes `open_preview` from a display intent into an agent browser
command with a terminal result.

### Command contract

```go
type BrowserRequest struct {
	Preview     PreviewRequest
	Disposition BrowserDisposition
}

type BrowserResult struct {
	ID           string
	Status       BrowserResultStatus
	RequestedURL string
	CommittedURL string
	Title        string
	Error        string
}
```

Terminal statuses are `committed`, `failed`, `cancelled`, and `timeout`.

### Broker

Add a `BrowserBroker` beside `ApprovalBroker` in the HTTP transport layer.
It owns pending command IDs, broadcasts a `browser_request` wire event, and
waits for one terminal response or context cancellation.

The browser result endpoint is session-scoped:

```text
POST /api/sessions/:sessionID/browser/:commandID/result
```

The first valid terminal response wins. Unknown, duplicate, or already-cancelled
IDs return a conflict or not-found response and cannot mutate the session.

Pending requests are included in history snapshots so reconnecting the desktop
can execute a command that was emitted immediately before a disconnect.

### Tool result language

With Phase 2:

```text
committed -> "Opened <committed URL>"
failed    -> "Could not open <requested URL>: <error>"
timeout   -> "The browser did not confirm the navigation"
```

The agent receives only its command result. User scrolling, link clicks,
history, cookies, storage, and page content are not added to conversation
context.

### Tab dispositions

The renderer keeps pending browser requests as a collection instead of a
single last-preview slot. This allows a history snapshot to restore multiple
commands. `reuse_agent_tab` supersedes an unfinished navigation in the same
session tab and reports the older command as cancelled. Foreground and
background requests receive independent tabs and can finish concurrently.

Every tab keeps its native controller mounted. Only the active tab gets a
visible viewport; inactive tabs can still navigate and report terminal state.
Closing a tab with an unfinished Agent command reports `cancelled`.

## Phase 3: Optional Capabilities

Capabilities are added after the navigation control plane is stable:

- `state`: current URL, title, and loading state.
- `screenshot`: bitmap capture of the controlled tab.
- `inspect`: visible text or a constrained DOM/accessibility snapshot.
- `interact`: click, type, select, and scroll through Playwright or CDP.
- `dev`: console errors and failed network requests for local app testing.

These capabilities use explicit tools. They do not automatically expose page
content, cookies, local storage, passwords, browsing history, or user-owned tabs
to the model.

## Security

- Public pages use the persistent browser session and deny browser permissions
  by default.
- Workspace previews use an isolated session with a path-limited desktop cookie
  and restricted Coding API access.
- Switching between `web` and `workspace-preview` recreates the native entry.
- Browser commands accept only HTTP(S) URLs after product validation.
- User-owned tabs require an explicit user request before agent control is
  added in a future phase.
- Sensitive form submission, uploads, purchases, messages, permission changes,
  CAPTCHA handling, and authentication actions require task-specific authority.

## Tests

### Pure reducer tests

- GitHub desired revision 1 commits successfully.
- Bilibili desired revision 2 ignores a late GitHub state for revision 1.
- Three rapid commands leave only revision 3 authoritative.
- A redirect stores requested and committed URLs separately.
- Closing a tab drops later state events.
- Agent commands never mutate a user-owned tab.

### Renderer tests

- GitHub to Bilibili reuses one agent tab and preserves active selection.
- Explicit foreground disposition creates and selects a second tab.
- Explicit background disposition creates a hidden tab without changing the
  selected tab.
- Multiple pending commands in a history snapshot restore independently.
- A delayed localhost probe cannot replace a newer public target.
- Resize and hide/show never call navigate.
- A failed navigation exposes retry without losing the requested address.

### Electron tests

- `setViewport` never changes revision or URL.
- Older navigate revisions are ignored.
- Same-target reload and different-target navigation are distinct.
- Web-to-workspace transitions replace the isolated entry.
- Main-frame redirect and failure states carry the applied revision.

### End-to-end sequence

```text
GitHub -> Bilibili -> localhost -> workspace HTML -> back/forward where valid
```

The test injects delayed state from every previous navigation and asserts that
none can overwrite the current tab.

## Delivery Order

1. Add `browserTabs.ts` and reducer sequence tests. Completed.
2. Split the preload and Electron bridge into navigate and viewport operations.
   Completed.
3. Add applied revision to native state and update `NativeBrowserManager`.
   Completed.
4. Add `useNativeBrowserController.ts` and simplify `BrowserSurface`. Completed.
5. Add repeated-navigation Playwright coverage and package the desktop app.
   Completed.
6. Add the disposition enum and generated wire DTOs, using
   `reuse_agent_tab` for `open_preview`. Completed.
7. Add `BrowserBroker`, the result endpoint, history restoration, and broker
   tests. Completed.
8. Add optional screenshot, inspection, and interaction capabilities only after
   the command/result path is stable.

Agent foreground/background tab dispositions and multi-command reconnect
recovery were completed as the final Phase 2 delivery. Phase 3 begins with
read-only page state and screenshot capabilities.

Phase 3 is the next implementation target. It must use explicit capabilities;
navigation acknowledgement alone does not authorize page observation or input.
