# Getting started

This plugin is a FinFocus CostSource process. It reads Flexera One Bill
Analysis with a refresh token or a service account. It does not use a pasted
access token.

## Build

```bash
mise install
make build
bin/finfocus-plugin-flexera --version
```

`make install` copies the binary to `~/.finfocus/plugins/flexera/0.1.0/`.

## Find the org id

1. Sign in to Flexera One for your zone: `https://app.flexera.com` (NAM),
   `https://app.flexera.eu` (EU), or `https://app.flexera.au` (APAC).
2. Open the organization you want to query.
3. Copy the numeric id from the browser URL. It is the path segment after
   `/orgs/`, for example `https://app.flexera.com/orgs/12345/...` means the
   org id is `12345`.

Set that number as `FLEXERA_ORG_ID`. A non-numeric value fails before the
process listens.

## Find a billing center id

1. In the same org, open Cloud Cost Optimization and then Billing Centers.
2. Open the billing center whose costs you want.
3. Copy the id from the billing center URL, or from the `id` field of
   `GET /bill-analysis/orgs/{orgId}/billing_centers`.

Set one or more ids as a comma-separated `FLEXERA_BILLING_CENTER_IDS` value.
Every `costs/select` and `forecasts/report` call sends them. The plugin does
not accept a billing center as a filter.

## Credentials

Use one of these:

- `FLEXERA_REFRESH_TOKEN` from a Flexera One refresh token
- `FLEXERA_CLIENT_ID` and `FLEXERA_CLIENT_SECRET` for a service account

```bash
export FLEXERA_ORG_ID="12345"
export FLEXERA_REGION="nam"
export FLEXERA_BILLING_CENTER_IDS="billing-center-id"
export FLEXERA_REFRESH_TOKEN="refresh-token"
make test-integration
```

`make test` does not call Flexera. `make test-integration` skips until the
org id, billing center ids, and OAuth credentials are set.

Optional settings are `FLEXERA_COST_METRIC` (default
`cost_amortized_unblended_adj`), `FLEXERA_TIMEOUT` (default `30s`), and
`FLEXERA_LOG_LEVEL` (`debug`, `info`, `warn`, or `error`).

Tag-to-billing-center rules belong in `billingCenterMappings`. See
`config.billing-centers.example.yaml`. When a cost row has a `tag:` or `tag_`
dimension that matches, `GetActualCost` puts that billing center id on the
result lineage.
