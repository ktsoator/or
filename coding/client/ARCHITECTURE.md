# Client architecture

The client is moving incrementally from a flat `src` layout to feature-first
ownership. New code should use these top-level boundaries:

```text
src/
|-- app/          application shell, navigation, and feature composition
|-- features/     user-facing capabilities with private implementation files
|-- shared/       domain-neutral utilities and reusable UI foundations
|-- generated/    generated protocol types
`-- *.ts(x)       transitional shared and not-yet-migrated modules
```

## Dependency direction

The normal dependency direction is:

```text
app -> features -> shared
```

- `shared` must not import from `features` or `app`.
- A feature must not import from `app`.
- Consumers should import a feature through its narrow `index.ts` entry point.
- A cross-feature integration may use an explicit public module such as
  `features/session/commands.ts`; it must not reach into private reducers or
  component files.
- Avoid catch-all barrel files. Feature entry points should only expose the
  small surface used by other ownership areas.

## Current ownership

- `app` owns the shell, `AppView` navigation state, application sidebar state,
  profile menu, and session dialogs.
- `features/session` owns session requests, streaming recovery, state stores,
  reducers, and the `useSession` facade.
- `features/composer` owns message composition, attachments UI, catalogs,
  compaction feedback, and composer-only controls.
- `features/conversation` owns transcript rendering, message and tool grouping,
  response actions, diffs, thinking blocks, and conversation scroll behavior.
- `features/browser` owns browser tabs, workspace state, webview runtime,
  navigation coordination, and browser command reporting.
- `features/workbench` owns the secondary panel, task view, conversation/browser
  composition, and responsive panel layout.
- `features/settings` owns settings navigation, provider configuration, usage,
  default models and provider connection testing.
- `features/skills` owns the skill catalog API, invocation parsing, and its lazy
  catalog page.
- `shared/attachments.ts` is shared because attachment metadata is rendered by
  both the composer and the conversation transcript.
- `shared/ui` contains provider identity and thinking controls reused by
  Settings and Composer, the sidebar toggle reused across application views,
  plus Markdown rendering shared by Conversation and the Skills catalog.
- `shared/lib/sidebarLayout.ts` owns sidebar sizing calculations shared by the
  application and Settings sidebars. Application-specific session grouping
  remains under `app`.
- `shared/lib/theme.ts` and `shared/hooks/useTheme.ts` own theme persistence,
  resolution, and React synchronization.
- `shared/lib/highlightRuntime.ts` owns the syntax-highlighting dependency and
  theme. It is loaded dynamically only when rendered content contains a code
  block, tool read preview, or diff.

The generic `lib/desktop.ts` module only exposes platform, directory picker,
and external-link capabilities. Electron webview registration and browser
runtime state belong to `features/browser`.

The remaining root modules are intentionally transitional. Move them by
capability in behavior-preserving batches; do not mix directory migration with
product changes.

## Loading boundaries

Non-primary views stay behind `React.lazy` boundaries in `app/App.tsx`.
Feature entry points must not eagerly export lazy pages, otherwise importing a
feature can pull those pages back into the initial chunk.
