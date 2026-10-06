---
argument-hint: [issue-number]
description: Pick up one GitHub issue and carry it through to a validated commit
---

# Pick Issue

Work GitHub issue #$ARGUMENTS from start to a validated commit. One issue is
one branch is one PR. If `$ARGUMENTS` is empty, run
`gh issue list --state open` and ask which issue to take.

## Phase 1: Understand the issue

```bash
gh issue view $ARGUMENTS --json title,body,labels,milestone,assignees,url,comments
```

1. Read the body and every comment. Note the acceptance criteria.
2. Find the matching task in `TASKS.md` (for example `D3` or `F1`) and read its
   notes. The task text may carry constraints the issue lacks, such as "check
   first" or "BLOCKED".
3. Stop and report if the issue is blocked, already done, or needs live Flexera
   credentials you do not have. Do not guess at API shapes: the contract is the
   generated `github.com/flexera-public/unified-go-client` models.

## Phase 2: Plan

1. Read the code the change touches and its tests before editing.
2. Write a short todo list: implementation, tests, docs, validation, commit.
3. For an unused-code removal (such as F1 or F2), grep for every reference first.

## Phase 3: Branch

Work on a branch named `issue-$ARGUMENTS`. Never commit to `main`.

```bash
git switch -c issue-$ARGUMENTS
```

Reuse the branch if it already exists.

## Phase 4: Implement with tests

1. Write or update tests first where behavior changes. Use the fake Flexera
   client in `internal/server`; do not call the live API.
2. Make the smallest change that satisfies the acceptance criteria.
3. Follow `CLAUDE.md`: no comments unless the logic is non-obvious, no
   debug code, never log refresh tokens, client secrets, or access tokens.
4. Update `README.md`, `CLAUDE.md`, and `plugin.manifest.json` when behavior or
   config changes. Update the matching entry in `TASKS.md`.

## Phase 5: Validate

Run these and fix every failure before moving on:

```bash
make build
make test
go test -race ./...
make vet
make lint
markdownlint-cli2 <each changed .md file>
```

`make lint` can run longer than five minutes. Use an extended timeout. Do not
edit `.golangci.yml` to silence findings.

Run `make test-integration` only when `FLEXERA_ORG_ID`,
`FLEXERA_BILLING_CENTER_IDS`, and OAuth credentials are set.

## Phase 6: Commit

Commit once validation passes. Invoking `/pick-issue` is the instruction to
commit.

1. Stage only the files this issue changed. Check `git status` first.
2. Write a conventional commit message that references the issue, for example
   `fix(manifest): list the OAuth config keys` with `Refs #$ARGUMENTS` in the
   body. Use `Closes #$ARGUMENTS` only when the issue is fully resolved. Release
   Please reads these types, so use `chore:` or `docs:` for changes that should
   not cut a release.
3. Validate the message with commitlint before committing:

   ```bash
   echo "$MESSAGE" | npx commitlint
   ```

4. Do not add `Co-Authored-By` or `Claude-Session` trailers or generated-by
   footers.

## Phase 7: Report

Do not push or open a PR unless asked. Finish with:

- the branch name and commit hash
- what changed, and which validation commands passed
- anything skipped or left open, and why
