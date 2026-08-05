# Contributing to or

Thanks for your interest in contributing! This document explains how to get
started and the conventions this project follows.

## Reporting issues

- Search existing issues first to avoid duplicates.
- Use the **Bug report** or **Feature request** templates when opening a new issue.
- For security issues, follow the [security policy](.github/SECURITY.md) — do
  **not** open a public issue.

## Development setup

For Go changes:

```bash
git clone https://github.com/ktsoator/or.git
cd or
go test ./...      # run the full test suite
go vet ./...       # static checks
```

For Or client or desktop changes, install
[Bun](https://bun.sh/) and run:

```bash
cd coding/client
bun install --frozen-lockfile
bun run lint
bun run test
bun run build

cd ../desktop
bun install --frozen-lockfile
bun run build:main
bun run build:sidecar
```

See [RELEASING.md](RELEASING.md) for release steps.

## Branching

Always branch off the latest `main`:

```bash
git checkout main && git pull
git checkout -b type/short-description
```

- Name branches `type/description`, e.g. `feat/add-retry`, `fix/nil-panic`,
  `test/openai-coverage`, `docs/update-readme`.
- Keep one branch focused on a single change.

## Commit messages

- Follow [Conventional Commits](https://www.conventionalcommits.org/):
  `type(scope): subject`, e.g. `fix(openai): handle empty response`.
- Subject-only commits are preferred; add a body only when extra context helps.
- Common types: `feat`, `fix`, `test`, `docs`, `chore`, `ci`.

## Pull requests

1. Push your branch and open a PR against `main`.
2. Link the related issue in the PR body with `Closes #123` (or `Part of #123`
   if it only partially addresses the issue).
3. Make sure **all CI checks pass** before requesting a merge.
4. PRs are merged with **squash** to keep history linear — no merge commits.

## Code of conduct

Be respectful and constructive in all interactions.
