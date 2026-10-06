# Contributing

## Setup

```bash
mise install
make test
make lint
```

`make test` runs the race detector and fails if `internal/flexera` or
`internal/flexeraapi` statement coverage is under 80%. `make test-integration`
calls Bill Analysis and skips unless `FLEXERA_ORG_ID`,
`FLEXERA_BILLING_CENTER_IDS`, and a refresh token or client id plus secret
are set.

## Changes

- Keep commits focused. Use a conventional subject: `type(scope): subject`.
- Do not commit refresh tokens, client secrets, access tokens, or `.env` files.
- Update `README.md` when an operator-facing setting changes.
- New and edited Markdown should pass `npx markdownlint-cli`.
- Run `actionlint .github/workflows/*.yml` when a workflow changes.

## Finding ids for a live call

The org id is the numeric `/orgs/{id}/` segment in the Flexera One URL. A
billing center id is on the billing center page or in the billing-centers
API `id` field. Steps are in [docs/getting-started.md](docs/getting-started.md).

## Pull requests

Describe the user-visible change and how you tested it. `make lint` and
`make test` should be clean before review.
