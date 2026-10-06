# Troubleshooting

## Cost RPCs return FailedPrecondition

The unified client is missing, or `billingCenterIds` is empty. Set
`FLEXERA_REFRESH_TOKEN` or both `FLEXERA_CLIENT_ID` and
`FLEXERA_CLIENT_SECRET`, and set `FLEXERA_BILLING_CENTER_IDS`.

## FLEXERA_ORG_ID must be numeric

`flexeraapi.New` sends the org id as an integer. Copy the number from the
Flexera One URL (`/orgs/{id}/`), not the organization name. The process
returns this error before it listens.

## Supports returns false for a region

A descriptor region of `nam`, `eu`, `apac`, `north-america`, `europe`, or
`asia-pacific` must match the plugin zone. A cloud region such as
`us-east-1` is accepted. Point `FLEXERA_REGION` at the zone where the org
lives.

## The call times out

`GetActualCost`, `GetProjectedCost`, and `GetPricingSpec` each cancel their
own work after `FLEXERA_TIMEOUT` (default 30s). A hung Bill Analysis call
returns `DeadlineExceeded` instead of blocking the RPC.

## HTTP 429 or 5xx

Those responses are retried with backoff and jitter. A `Retry-After` value
is honored up to 5 seconds. Other 4xx responses are not retried. The same
HTTP failure class maps to the same gRPC code on all three cost RPCs:
401 is `Unauthenticated`, 429 is `ResourceExhausted`, and 5xx is
`Unavailable`.

## HTTP 202 or a truncated result

HTTP 202 with rows is a successful `costs/select`. HTTP 202 with no rows is
an error. `rowsTruncated` is `ResourceExhausted`; narrow the window or the
filter. Day windows are at most 31 days, and the whole range must be within
24 months.

## Startup warns about tlsSkipVerify

`FLEXERA_TLS_SKIP_VERIFY=true` disables certificate checks. Leave it unset
outside local debugging. Logs follow `FLEXERA_LOG_LEVEL`. Refresh tokens,
client secrets, and access tokens are removed from error strings and logs.

## Where the ids are

The org id is the number after `/orgs/` in the Flexera One URL. A billing
center id is on the billing center page URL or in the `id` field from
`GET /bill-analysis/orgs/{orgId}/billing_centers`. See
[Getting started](getting-started.md).
