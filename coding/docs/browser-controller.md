# Browser Controller Design

Status: Phase 1 and Phase 2 implemented, including Agent tab dispositions;
Phase 3 read-only text inspection implemented, remaining capabilities proposed

## Objective

Turn the right-side Browser from a display-only preview surface into a reliable,
session-aware browser controller. Phase 1 makes navigation and tab ownership
deterministic. Phase 2 adds an acknowledgement path so an agent can distinguish
a requested navigation from one that actually committed in Electron. The first
Phase 3 capability adds explicit, bounded, read-only observation of the stable
Agent tab.

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
3. Browser state events do not identify the navigation revision they describe.

## User Semantics

Each coding session has one deterministic reusable Agent tab:

```text
preview:<session-id>
```

The default agent action reuses that tab. Its existing `<webview>` stays alive
so web-to-web navigation preserves back and forward history.

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
- Own each `<webview>`, its DOM layout, and the browser runtime registry.
- Route an agent request to its deterministic session tab.
- Handle inspection requests independently of whether `BrowserView` is mounted,
  waiting for a restored reusable-tab navigation before observing it.
- Keep every pending browser command until its result is acknowledged and route
  new-tab commands to stable command-owned tabs.
- Keep desired command state separate from observed page state.
- Ignore stale Electron events.
- Resolve localhost checks before issuing a navigation command.
- Render address, title, loading, failure, and history controls.

### Electron main process

- Enable the webview tag for the renderer.
- Handle guest attempts to open new windows and external protocols.
- Supervise the sidecar and install the desktop session cookie.
- Add attach-time validation, dedicated partitions, permission policy, and
  workspace request isolation before untrusted preview isolation is claimed.

## Phase 1: Renderer Browser Runtime

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
`desired` command. Browser runtime state writes only `observed`. A committed observed
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

### Browser runtime bridge

The renderer registry is the only browser runtime. The desktop shell exposes
`browserMode: 'webview'` and nothing else, so tests drive the shipped bridge
through a stand-in `<webview>` guest rather than a second adapter:

```ts
type BrowserNavigateInput = {
  tabID: string
  revision: number
  url: string
  kind: 'web' | 'workspace-preview'
}

type BrowserRuntimeState = {
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

type BrowserRuntimeBridge = {
  navigate(input: BrowserNavigateInput): Promise<BrowserRuntimeState>
  close(tabID: string): Promise<void>
  goBack(tabID: string): Promise<void>
  goForward(tabID: string): Promise<void>
  onState(listener: (state: BrowserRuntimeState) => void): () => void
}
```

Electron webviews participate in DOM layout directly, so the runtime owns no
bounds or visibility protocol.

### Webview entry state

The renderer registry keeps requested and committed state separately:

```ts
type BrowserEntry = {
  tabID: string
  element: BrowserWebviewElement
  operation: number
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
3. Record revision and requested URL before starting the load.
4. Claim the navigation, then wait for the webview's first `dom-ready` event.
5. Stop a superseded main-frame load when necessary.
6. Call `loadURL` for a different target or `reload` for an explicit same-target
   revision, marking the navigation as issued.
7. Ignore completion from an operation superseded by a newer revision.
8. Return a state carrying the same applied revision.

A freshly mounted guest loads its own `about:blank` document before any
requested load. That document commits, fires `did-navigate`, and finishes while
the navigation is still waiting at step 4, so a claimed navigation is not
observable until it has been issued: until then the guest's `did-navigate`,
`did-navigate-in-page`, load-completion, `did-fail-load`, and
`render-process-gone` events are ignored. Without that rule the guest's initial
document is reported as the requested page's arrival, with a ready status and a
non-HTTP committed URL. Claiming before the wait also bounds it by the
navigation timeout.

Main-frame `did-navigate` sets `committedURL`. `did-fail-load` sets `failed`
except for Electron's aborted-load error. Redirects are successful and retain
both requested and committed URLs.

### Browser controller hook

Extract browser synchronization from `BrowserSurface.tsx` into
`useBrowserController.ts`.

The hook separates registration, target resolution, navigation, and observation:

- Registration effect: connects the mounted webview to the runtime registry.
  Releasing a registration also forgets which revision has been issued, because
  a navigation issued against a replaced registry entry never completes.
- Navigation effect: watches the complete desired command and calls
  `browser.navigate` exactly once per revision.

Public and workspace URLs are immediately resolved. Localhost validation uses
an abortable request keyed by its desired URL and revision. A result is discarded
unless it still matches the current desired command.

`BrowserSurface` hosts the webview plus loading and failure overlays. It does not
own URL state.

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

Every tab keeps its browser controller and webview mounted. Only the active tab
is visible; inactive tabs can still navigate and report terminal state.
Closing a tab with an unfinished Agent command reports `cancelled`.

A command has exactly one result, so the renderer only reports `committed` for a
page with an HTTP(S) committed URL, and a report that is retrying always sends
the newest observation rather than the one it started with. Otherwise a rejected
result consumes the command's only acknowledgement and the tool waits for its
timeout even though the page loaded.

## Phase 3: Explicit Browser Capabilities

The first implemented capability is `inspect_browser`. It returns the stable
Agent tab's final HTTP(S) URL, title, page status, applied revision, and at most
12,000 characters of rendered text.

```text
inspect_browser tool
  -> BrowserBroker broadcasts browser_inspect_request
  -> Renderer inspection controller waits for pending Agent navigation
  -> The stable Agent webview runs one fixed extractor
  -> React POSTs one terminal result
  -> BrowserBroker returns the observation to the tool
```

The result endpoint is session and inspection scoped:

```text
POST /api/sessions/:sessionID/browser/inspect/:inspectionID/result
```

Pending inspection requests are included in history snapshots so a renderer
reconnect does not strand the waiting tool call. The first terminal result wins;
context cancellation, timeout, transport replacement, duplicates, and late
results use the same broker lifecycle rules as navigation commands.

The fixed extractor excludes form controls, editable regions, script and style
content, hidden/inert/ARIA-hidden content, and non-rendered text. The renderer
registry accepts only `preview:<session-id>` IDs, so user tabs and command-owned
new tabs cannot be inspected. It rejects the result if the tab or revision
changes while extraction is running.

The page text is untrusted external data. The tool description explicitly tells
the model not to interpret instructions in the page as system or tool
instructions. The capability does not expose raw DOM, arbitrary JavaScript,
cookies, storage, form values, passwords, or browsing history.

Remaining capabilities are proposed and must be added separately:

- `state`: current URL, title, and loading state.
- `screenshot`: bitmap capture of the controlled tab.
- `inspect`: constrained DOM or accessibility snapshots beyond bounded text.
- `interact`: click, type, select, and scroll through Playwright or CDP.
- `dev`: console errors and failed network requests for local app testing.

These capabilities use explicit tools. They do not automatically expose page
content, cookies, local storage, passwords, browsing history, or user-owned tabs
to the model.

## Security

- Public pages and workspace previews currently share the desktop session.
- Workspace preview HTTP routes allow only `GET` and `HEAD`, reject hidden and
  credential-like files, prevent symlink escape, and return a restrictive CSP.
- Browser commands accept only HTTP(S) URLs after product validation.
- Browser inspection runs fixed product code in the guest main world and checks
  that the applied revision remains unchanged before returning data.
- User-owned tabs require an explicit user request before agent control is
  added in a future phase.
- Sensitive form submission, uploads, purchases, messages, permission changes,
  CAPTCHA handling, and authentication actions require task-specific authority.

Before untrusted workspace previews are considered isolated, Electron must add
attach-time preference validation, dedicated partitions, path-scoped cookies,
permission denial, request filtering, navigation restrictions, and an isolated
execution world for inspection.

## Tests

### Pure reducer tests

- GitHub desired revision 1 commits successfully.
- Bilibili desired revision 2 ignores a late GitHub state for revision 1.
- Three rapid commands leave only revision 3 authoritative.
- A redirect stores requested and committed URLs separately.
- Closing a tab drops later state events.
- Agent commands never mutate a user-owned tab.

Renderer tests install a stand-in `<webview>` guest — one that attaches
asynchronously and commits its own `about:blank` document first — so they
exercise the shipped bridge, controller, and reducer together. A test-only
runtime that the desktop shell does not implement would hide exactly the
defects that live in that seam.

### Renderer tests

- GitHub to Bilibili reuses one agent tab and preserves active selection.
- Explicit foreground disposition creates and selects a second tab.
- Explicit background disposition creates a hidden tab without changing the
  selected tab.
- Multiple pending commands in a history snapshot restore independently.
- A delayed localhost probe cannot replace a newer public target.
- Resize and hide/show never call navigate.
- A failed navigation exposes retry without losing the requested address.
- The guest's own initial document neither completes nor acknowledges the first
  agent navigation of a session; the requested page is still loaded and reported
  exactly once.

### Electron tests

- The webview guest viewport fills its DOM host rather than retaining the
  default 150-pixel iframe height.
- A renderer menu remains the top hit-test target where it overlaps the webview.

### Inspection tests

- Broker resolution is one-shot and covers timeout, context cancellation,
  transport close, and reconnect restoration.
- The endpoint rejects invalid statuses, revisions, URLs, credentials, and
  inconsistent completed-page state, and enforces the text limit again in Go.
- Rendered headings and button text are returned while form values, editable
  text, script content, and hidden text are excluded.
- Inspection still reads the stable Agent tab when a command-owned new tab is
  active, and duplicate wire events produce exactly one result POST.
- Inspection handling is independent of `BrowserView`; a closed or absent Agent
  tab returns a prompt failure without reopening the workbench or navigating.

### End-to-end sequence

```text
GitHub -> Bilibili -> localhost -> workspace HTML -> back/forward where valid
```

The test injects delayed state from every previous navigation and asserts that
none can overwrite the current tab.

## Delivery Order

1. Add `browserTabs.ts` and reducer sequence tests. Completed.
2. Separate navigation operations from DOM layout and visibility. Completed.
3. Add applied revision to browser runtime state and reject stale operations.
   Completed.
4. Add `useBrowserController.ts` and simplify `BrowserSurface`. Completed.
5. Add repeated-navigation Playwright coverage and package the desktop app.
   Completed.
6. Add the disposition enum and generated wire DTOs, using
   `reuse_agent_tab` for `open_preview`. Completed.
7. Add `BrowserBroker`, the result endpoint, history restoration, and broker
   tests. Completed.
8. Add bounded read-only text inspection for the stable Agent tab. Completed.
9. Add optional screenshot, structured inspection, interaction, and developer
   capabilities independently.

Agent foreground/background tab dispositions and multi-command reconnect
recovery were completed as the final Phase 2 delivery. Phase 3 now includes
read-only page URL, title, state, and visible text through `inspect_browser`.

Screenshot, structured page inspection, input, and developer diagnostics remain
separate future capabilities. Navigation acknowledgement alone does not
authorize any of them.
