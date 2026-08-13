---
name: git-workflow
description: Follow this repository's conventions for git branches, commit messages, pull requests, and version releases. Use when the user asks to branch, commit, push, open a PR, merge, or publish a release in this repository.
---

# Git Workflow

## Scope

Use this skill only inside the `ktsoator/or` repository. Do not apply these
conventions to other repositories unless the user explicitly says so.

## Workflow order

1. Confirm the working tree is clean and local `main` is up to date.
2. Create a focused branch from `main`.
3. Make the change and run the local checks below.
4. Show the user the commit draft and wait for approval.
5. Commit.
6. Show the user the final PR title/body and wait for approval.
7. Push and open the PR.
8. Wait for CI, and address review feedback on the same branch if needed.
9. Squash-merge to `main`, then return to `main` and `git pull --ff-only`.

## Branches

Always branch from the latest `main`:

```bash
git fetch origin
git checkout main
git pull --ff-only
git checkout -b type/short-description
```

- Use `type/description` names: `feat/add-retry`, `fix/nil-panic`,
  `test/openai-coverage`, `docs/update-readme`, `chore/update-model-catalog`.
- Keep one branch focused on a single change.
- Use lowercase, hyphen-separated descriptions.

## Commit messages

Follow Conventional Commits:

```text
type(scope): subject

body
```

- Always include a body. The subject is a concise, imperative summary; the body
  explains what changed and why.
- Common types: `feat`, `fix`, `test`, `docs`, `chore`, `ci`.
- Scopes seen in this repo include `coding`, `desktop`, `llm`, `client`, `deps`,
  `permission`, `cli`, `mcp`, and `release`.
- Keep the subject short, imperative, and lowercase. No emoji.

Example:

```text
fix(coding): refresh default model catalog after provider config

The default-model picker now reloads /api/models after provider
configuration so newly added providers appear without leaving Settings.
```

## Before committing

Always show the user the planned branch name, commit subject, and commit body
before running `git commit`. Wait for explicit approval before committing. Do
not commit until the user has reviewed it.

## Issues

- Search existing issues before filing a new one.
- Use the repository templates: bug reports use `[Bug]:` titles and the `bug`
  label; feature requests use `[Feature]:` titles and the `enhancement` label.
- Fill every required template field. Bug reports need package, version,
  Go version, environment, a minimal reproduction, expected behavior, and logs
  with secrets removed.

## Pull requests

Before opening a PR, show the user the final PR title and body and wait for
explicit approval. Do not push or open the PR until the user has reviewed it.

Then push the branch and open the PR against `main`:

```bash
git push -u origin <branch>
```

Use the repository PR template, which has these sections:

- **Summary**: explain what the PR changes and why.
- **Changes**: list the important implementation or documentation changes.
- **Testing**: describe the checks run and relevant results.
- **Checklist**: confirm the change is focused, tests were added/updated, docs were updated for user-facing changes, Go checks pass, and breaking changes are documented.

Issue linking rules:

- If the change has a related issue, link it in the PR body with `Closes #123`,
  or `Part of #123` when it only partially addresses the issue.
- If there is no related issue, do not add an issue link.

Before opening a PR, run the same checks CI will run (see below). Wait for all
CI checks to pass before merging.

Merge with **squash** to keep history linear; do not use merge commits. Keep the
PR title in the same Conventional Commit style as the commit.

## PR review and updates

- If CI fails or a reviewer requests changes, stay on the same branch and add
  new commits; do not open another PR.
- Do not force-push a branch that has already been pushed and shared, unless the
  user explicitly asks for it.
- After the PR is squash-merged, switch back to `main`, run
  `git pull --ff-only`, and delete the merged branch locally.

## Local checks before push

Run these in the repository root unless noted:

```bash
# Go formatting and module files
gofmt -w .
git diff --exit-code
go mod tidy
git diff --exit-code -- go.mod go.sum

# Version and static checks
node scripts/set-version.mjs "$(node -p "require('./coding/desktop/package.json').version")" --check
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
go test ./...
go test -race ./...

# Coding client
cd coding/client
bun install --frozen-lockfile
bun run lint
bun run test
bun run build
bunx playwright install --with-deps chromium
bun run test:ui

# Coding desktop
cd ../desktop
bun install --frozen-lockfile
node scripts/check-artifact-names.mjs
bun run test:unit
bun run build:main
bun run build:sidecar
```

## Safety and ignored files

Never commit:

- `.env`, API keys, tokens, credentials, or other secrets
- `node_modules`, `dist`, build output, `test-results`, or Playwright artifacts
- release artifacts such as `.dmg`, `.zip`, or `SHA256SUMS`
- generated dependency caches or local configuration

Keep prompts, logs, and release notes free of credentials and personal data.

## Releases

The Go packages and Or share one version: `vX.Y.Z`. A tag publishes the Go
module and creates a GitHub Release with macOS installers for Apple Silicon and
Intel Macs.

### Prepare a release

1. Add release notes at `.github/release-notes/vX.Y.Z.md`. Use these sections,
   including only the ones that apply:

   ```markdown
   ### Added

   - ...

   ### Changed

   - ...

   ### Fixed

   - ...

   ### Full Changelog

   [Compare vX.Y.Z-1...vX.Y.Z](https://github.com/ktsoator/or/compare/vX.Y.Z-1...vX.Y.Z)
   ```

2. Update the shared package versions:

   ```bash
   node scripts/set-version.mjs vX.Y.Z
   ```

3. Run the same checks as the CI release workflow:

   ```bash
   node scripts/set-version.mjs vX.Y.Z --check
   go test ./...
   go vet ./...

   cd coding/client
   bun install --frozen-lockfile
   bun run lint
   bun run test
   bun run build

   cd ../desktop
   bun install --frozen-lockfile
   node scripts/check-artifact-names.mjs
   bun run test:unit
   bun run build:main
   bun run build:sidecar
   ```

4. Commit the version change, merge it to `main`, and wait for CI to pass.

### Publish a release

Before tagging, verify:

- `.github/release-notes/vX.Y.Z.md` exists and matches the tag name exactly.
- `node scripts/set-version.mjs vX.Y.Z --check` passes.
- No uncommitted release-related changes remain.

Then tag the release commit and push the tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

The release workflow then:

1. Validates that package versions match the tag, checks release artifact
   names, and requires `.github/release-notes/vX.Y.Z.md`.
2. Runs Go, client, and desktop checks.
3. Builds and signs the macOS app on `macos-15` (arm64) and
   `macos-15-intel` (x64).
4. Collects the `.dmg`/`.zip` artifacts, generates `SHA256SUMS`, and creates
   the GitHub Release with the release-notes file as its body.
5. Treats tags containing `-` as prereleases.

Never move a published tag. Publish a new patch version instead.
