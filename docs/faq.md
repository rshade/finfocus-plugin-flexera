# FAQ

## Which credential should I set?

A Flexera One refresh token (`FLEXERA_REFRESH_TOKEN`), or a service account
(`FLEXERA_CLIENT_ID` and `FLEXERA_CLIENT_SECRET`). Cost RPCs exchange that
for a short-lived access token. Do not paste an access token into the config.

## Where is the org id?

Sign in to Flexera One and copy the number after `/orgs/` in the URL. NAM is
`app.flexera.com`, EU is `app.flexera.eu`, and APAC is `app.flexera.au`. The
value must be numeric.

## Where is a billing center id?

Open Cloud Cost Optimization, then Billing Centers, and open one center. The
id is in that page's URL. The same id is the `id` field from
`GET /bill-analysis/orgs/{orgId}/billing_centers`. Put one or more ids in
`FLEXERA_BILLING_CENTER_IDS`.

## Why is projected cost different from the Flexera forecast screen?

`GetProjectedCost` sums `forecastAmounts` for the current month. If that
report is empty or fails, the plugin falls back to a 90-day linear
extrapolation of `costs/select`. `billing_detail` says which method ran.

## Does the plugin allocate cost to a billing center from tags?

When `billingCenterMappings` is set and a row dimension uses a `tag:` or
`tag_` key, `GetActualCost` writes the matched billing center id onto the
result lineage. Rows without matching tags are left unchanged unless a
default billing center is configured.

## What does the tlsSkipVerify warning mean?

The process logs a warning at startup when certificate verification is
disabled. Turn `FLEXERA_TLS_SKIP_VERIFY` off for any shared environment.
