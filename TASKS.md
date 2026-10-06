<!-- markdownlint-disable MD022 MD060 -->

# finfocus-plugin-flexera v0.1.0 Implementation Plan

## State Audit

### Skeleton Code Status

**What is implemented:**

- `Config`: Environment and file-based configuration (YAML parsing working)
- `Client`: HTTP client structure with timeout support
- `BillingCenterResolver`: Tag-to-billing-center mapping logic
- `FlexeraServer`: gRPC server stub with method signatures
- `goreleaser_test.go`: Asset naming validation (working)

**What is NOT implemented:**

- HealthCheck (deferred to v0.1.1)
- Live calls still need org id, OAuth credentials, and a billing center id. The code does not need captured responses.

**Contract:**

- Request and response shapes come from `github.com/flexera-public/unified-go-client`
- `main` builds that client with `flexeraapi.New`, which exchanges a refresh token or service account for an access token

**Implemented in Phase A:**

- `github.com/rshade/finfocus-spec` v0.7.5 is imported and `CostSourceService` is registered
- `Name`, `GetPluginInfo`, and `Supports` use the generated proto types
- `GetActualCost`, `GetProjectedCost`, and `GetPricingSpec` use the generated proto types and the legacy HTTP client

### finfocus-spec v0.7.0 RPC Coverage

**CostSourceService** (core cost APIs):
- `Name()` — Return plugin name ✓ Returns `flexera`
- `GetPluginInfo()` — Return capabilities ✓ Version, spec version, providers, implemented RPCs
- `Supports()` — Check resource support ✓ Allowlist plus Flexera zone check
- `GetActualCost()` — `costs/select` through the generated Flexera client
- `GetProjectedCost()` — `forecasts/report` through the generated client, with linear extrapolation when that report has no amounts
- `GetPricingSpec()` — Return pricing details (NEW)

**ObservabilityService** (observability/monitoring):
- `HealthCheck()` — Connectivity validation
- **v0.1.0 decision:** ObservabilityService is optional for v0.1.0 MVP; HealthCheck can be deferred to v0.1.1

**Post-v0.1.0 (CostSourceService)**:
- EstimateCost (duplicate of GetProjectedCost)
- GetRecommendations, DismissRecommendation (recommendations feature)
- GetBudgets, DryRun, BatchCost, ResolveResourceTypes (advanced features)

### Flexera API Details — Verified and Unverified

**Research completed 2026-10-04** (see `../finfocus-pm/research/flexera-api-facts.md` for the full analysis).

**Claims from skeleton code (VERIFIED):**
- Bill Analysis API: `/bill-analysis/` endpoint with regional hosts (nam/eu/apac) — Confirmed (H: SW; PT)
- `billing_center_ids` is required for `costs/select` — Confirmed (H: SW, PT)
- Cost metric `cost_amortized_unblended_adj` (amortized unblended) — Confirmed (H: PT-focus, PT-avg)
- Time window with `start_at`/`end_at`, `granularity` day or month — Confirmed (H: SW)
- Daily granularity max 31 days, month max 24 months — Confirmed (M: SW)
- `end_at` is exclusive — Confirmed (M: SW)
- Format: YYYY-MM for month, YYYY-MM-DD for day — Confirmed (M: SW)

**CONTRADICTED claims (fix in A1/B1):**
- "OAuth 2.0 JWT token auth via Flexera Cloud Management API" → WRONG. Correct: Flexera One refresh token or service account exchanged at `https://login.flexera.<zone>/oidc/token`; access token 1h lifetime; config holds refresh token/client id+secret, NOT a pasted JWT.
- "POST `/bill-analysis/orgs/{org}/costs`" path → WRONG. Correct paths: `costs/select` (per-resource) and `costs/aggregated` (rollup).
- Response shape `results[]` with `meta.pagination` and currency field → WRONG. Correct: `rows[]` with `{timestamp, dimensions, metrics}`; no pagination object; currency from separate `settings/currency_code` call.
- Filter is `map[string]string` → WRONG. Correct: expression tree with `type`, `dimension`, `value`, `expressions` (e.g., `{"type":"equal","dimension":"vendor","value":"aws"}`).
- `billing_center_id` can be filtered → WRONG. Correct: scoped via required `billing_center_ids` body field only, not filterable.
- Dimensions: "vendor, service, resource_group, billing_center" → INCOMPLETE. Correct: vendor, vendor_account, service, region, resource_group, resource_type, category, instance_type, resource_id (select only), `tag_<key>` (if org created the tag dimension).

**Defined by the generated client (no capture required):**
- `costs/select` body: `billing_center_ids`, `metrics`, `dimensions`, `start_at`, `end_at`, `limit` (max 100000), optional `granularity`, and a filter expression (`equal`, `substring`, `and`, `or`, `not`)
- `rowsTruncated` on the select result. This plugin treats it as an error
- HTTP 202 is a generated response variant. Rows are used when present. An empty 202 is an error
- `forecasts/report` returns `segments[].forecastAmounts`. Metric `cost_amortized_unblended_adj` is one of the generated metric values
- `settings/currency_code` returns a setting `value`

**Still runtime behavior, not a code blocker:**
- Exact `resource_id` string each vendor stores
- How far back rows exist, and when yesterday's rows appear

The plugin sends the caller's resource id or ARN as the `resource_id` filter value.

---

## Issue Index

| Issue | Title | Type | Status | Notes |
|-------|-------|------|--------|-------|
| [#1](https://github.com/rshade/finfocus-plugin-flexera/issues/1) | Fix Code Quality Issues and Linting Violations | Code Quality | DONE (close) | `make lint` reports 0 issues (verified 2026-10-05); counts in the issue are stale |
| [#2](https://github.com/rshade/finfocus-plugin-flexera/issues/2) | Implement Missing Protocol Buffer Definitions and gRPC Integration | Core Feature | CLOSED | Covered by tasks A1-A3 |
| [#3](https://github.com/rshade/finfocus-plugin-flexera/issues/3) | Enhance Error Handling and Security for Production Deployment | Enhancement | PARTIAL, SPLIT | TLS min 1.2, timeout, and SDK token refresh exist; redaction, retry, per-RPC timeouts, status-code mapping are open (E1-E5) |
| [#4](https://github.com/rshade/finfocus-plugin-flexera/issues/4) | Implement Advanced Cost Optimization Recommendations Engine | Feature | POST-V0.1, NOT-STARTED | Narrow to `GetRecommendations`/`DismissRecommendation` (spec v0.7.5); Optima recommendations API is the data source |
| [#5](https://github.com/rshade/finfocus-plugin-flexera/issues/5) | Implement Budget Management and Alerting System | Feature | POST-V0.1, RE-SCOPE | Narrow to read-only `GetBudgets`; unified client has budget operations; alerting is out of scope |
| [#6](https://github.com/rshade/finfocus-plugin-flexera/issues/6) | Enhanced Showback and Chargeback Reporting System | Feature | POST-V0.1, RE-SCOPE | Reporting is out of scope; billing-center resolver in `internal/flexera` is not wired into the server (see F1) |
| [#7](https://github.com/rshade/finfocus-plugin-flexera/issues/7) | Implement Comprehensive CI/CD Pipeline with Security and Quality Gates | DevOps | PARTIAL, SPLIT | Test, lint, govulncheck, goreleaser, commitlint, prose exist; Release Please fails on main (missing token); no coverage, race, CodeQL, or branch protection (CI1-CI6) |
| [#8](https://github.com/rshade/finfocus-plugin-flexera/issues/8) | Comprehensive Testing Strategy for Enterprise FinOps Deployment | Testing | PARTIAL, SPLIT | Coverage: server 81.7%, flexera 61.0%, flexeraapi 47.5%, main 0%; 95% target is unrealistic, use 80% (T1-T5) |
| [#9](https://github.com/rshade/finfocus-plugin-flexera/issues/9) | Multi-Cloud Cost Management and Analytics Platform | Architecture | WONT-DO-AS-SCOPED | aws/azure/gcp routing already works; retitle to "extend resource-type coverage" or close |
| [#10](https://github.com/rshade/finfocus-plugin-flexera/issues/10) | AI-Driven Cost Optimization and Sustainability Metrics | Feature | WONT-DO-AS-SCOPED | Split into recommendations (see #4) and `ImpactMetric` carbon/energy, which is BLOCKED on a Flexera data source |
| [#11](https://github.com/rshade/finfocus-plugin-flexera/issues/11) | Improve Documentation and User Experience for FinOps Practitioners | Docs | PARTIAL | README and examples exist; manifest and stale `FLEXERA_API_TOKEN` references are wrong (D3-D5) |
| [#12](https://github.com/rshade/finfocus-plugin-flexera/issues/12) | Adopt OpenSpec as the change process after v0.1.0 | Process | POST-V0.1 | Gated on the v0.1.0 tag; nothing started (O1-O7) |

**Verification:** 12 issues found (11 open, 1 closed); 0 missing from live repo. Status refreshed 2026-10-05 from per-issue code analysis.

---

## Phased Implementation Tasks

### Phase A0: Dependency Adoption (DONE)

#### A0: Adopt Flexera Unified Go Client
- **Files**: `go.mod`, `internal/flexeraapi/client.go`, `internal/flexeraapi/client_test.go`
- **Acceptance**:
  - `go build ./...` succeeds
  - `go vet ./...` passes
  - `go test ./internal/flexeraapi/...` passes
  - Dependency pinned to `github.com/flexera-public/unified-go-client v0.0.0-20261001183948-92b58ffbc17f`
  - `internal/flexeraapi` wraps the client for `costs/select` queries
- **Verify**: `go build ./... && go vet ./... && go test ./internal/flexeraapi/...`
- **Tasks**:
  - ✓ Added `github.com/flexera-public/unified-go-client` v0.0.0-20261001183948-92b58ffbc17f to go.mod
  - ✓ Created `internal/flexeraapi` package with `Client` interface and `New()` constructor
  - ✓ Implemented `CostsSelect()` method wrapping `costs/select` endpoint
  - ✓ Added httptest integration test proving POST method, correct path, required body fields, Authorization header, and response row parsing
  - ✓ Added break check test: validation catches omitted `billing_center_ids` before HTTP call
- **Open Design Decision: Decimal Conversion for Float64 Metrics**
  - Flexera client returns metrics as JSON float64; the plugin must convert these at the boundary with explicit rounding to currency's minor unit
  - Decision on conversion approach (e.g., use `decimal` package, fixed-point scaling, or stream through Decimal type) is deferred to Phase B implementation
  - Never sum raw float64 across rows without a plan

### Phase A: Core Protocol Integration

#### A1: Import finfocus-spec and Register gRPC Service (DONE)
- **Files**: `cmd/finfocus-plugin-flexera/main.go`, `go.mod`
- **Acceptance**:
  - `go build ./...` succeeds
  - `go vet ./...` passes
  - Binary starts without import errors
- **Tasks**:
  - ✓ Added `github.com/rshade/finfocus-spec` v0.7.5 to `go.mod` and imported it in the server
  - ✓ Replaced stub types in `flexera_server.go` with `pbc` proto message types
  - ✓ `RegisterService()` calls `RegisterCostSourceServiceServer`; `main` registers the server and gRPC reflection

#### A2: Implement Name() and GetPluginInfo() RPCs (DONE)
- **Files**: `internal/server/flexera_server.go`
- **Acceptance**:
  - `Name()` returns "flexera" with no error
  - `GetPluginInfo()` returns plugin capabilities
  - Unit tests pass
- **Tasks**:
  - ✓ `Name()` returns `flexera`
  - ✓ `GetPluginInfo()` returns version, `pluginsdk.SpecVersion`, providers, and implemented capabilities
  - ✓ Unit tests cover both methods

#### A3: Implement Supports() RPC with Resource Type Logic (DONE)
- **Files**: `internal/server/flexera_server.go`, `internal/flexera/client.go`
- **Acceptance**:
  - `Supports()` returns `true` for known resource types (aws-ec2, aws-s3, azure-vm, gcp-compute)
  - Returns `false` for unsupported types
  - Handles region mismatches gracefully
  - Unit tests pass
- **Tasks**:
  - ✓ Allowlist: `aws-ec2`, `aws-s3`, `aws-rds`, `azure-vm`, `azure-storage`, `gcp-compute`, `gcp-storage`
  - ✓ Provider must match the resource type prefix when the host sends one
  - ✓ A descriptor region of `nam`, `eu`, or `apac` must match the configured Flexera zone; cloud regions such as `us-east-1` are accepted
  - ✓ Unit tests cover matches, mismatches, and unknown types

#### A4: Implement GetPluginInfo() RPC (Core CostSourceService) (DONE)
- **Files**: `internal/server/flexera_server.go`
- **Acceptance**:
  - `GetPluginInfo()` returns plugin version, capabilities, supported fields
  - Advertises which CostSourceService RPCs are implemented
  - Unit tests pass
- **Tasks**:
  - ✓ Capabilities: actual cost, projected cost, pricing spec
  - ✓ Metadata `implemented_rpcs` lists Name, GetPluginInfo, Supports, GetActualCost, GetProjectedCost, GetPricingSpec
  - ✓ Covered by `TestGetPluginInfo`

#### A5: HealthCheck() — Optional for v0.1.0 (ObservabilityService)
- **Status**: Post-v0.1.0 (ObservabilityService is optional for MVP)
- **Files**: `internal/server/flexera_server.go`, `internal/flexera/client.go`
- **Deferred to v0.1.1** when observability is required
- **BLOCKED-ON-CREDENTIALS**: Requires valid Flexera token for real implementation

### Phase B: Cost API Integration (IMPLEMENTED WITH MOCKS)

Live Flexera captures are still owner input. The RPCs call `internal/flexeraapi` and unit tests use a fake client.

#### B1: Implement GetActualCost() RPC - Flexera API Call (DONE)
- **Files**: `internal/server/flexera_server.go`, `internal/flexeraapi/client.go` (wrapper already in place)
- **Acceptance**:
  - `GetActualCost()` returns historical cost from Flexera
  - Maps resource ID to `resource_id` equal filter in `costs/select`
  - Parses Flexera response `rows[{timestamp, dimensions, metrics}]`
  - Reads currency from `settings/currency_code` endpoint (org-wide)
  - Unit tests pass (mocked Flexera responses)
  - **BLOCKED-ON-CREDENTIALS**: Requires valid test account and API token
- **Tasks**:
  - ✓ `GetActualCost` calls `flexeraapi.Client.CostsSelect`
  - ✓ Sends configured `billing_center_ids`
  - ✓ `resource_id` uses an `equal` filter. `service/`, `region/`, `vendor_account/`, and `resource_group/` ids map to those dimensions
  - ✓ Day windows are at most 31 days. Ranges over 24 months are rejected
  - ✓ `settings/currency_code` is cached for 15 minutes
  - ✓ Each metric is rounded to the currency minor unit
  - ✓ HTTP 202 with rows is kept. HTTP 202 without rows is an error. Truncation is an error
  - ✓ Unit tests cover the mock client

#### B2: Implement GetProjectedCost() RPC - Cost Estimation (DONE)
- **Files**: `internal/server/flexera_server.go`, `internal/flexera/client.go`
- **Acceptance**:
  - `GetProjectedCost()` returns estimated monthly cost
  - Uses Flexera cost data for projection or returns $0 with note
  - Handles unsupported resource types gracefully
  - Unit tests pass (mocked)
  - **BLOCKED-ON-CREDENTIALS**: Requires Flexera API exploration
- **Tasks**:
  - ✓ `forecasts/report` is called through the generated client. Empty or failed reports fall back to a 90-day linear extrapolation
  - ✓ `billing_detail` states that the figure is an estimate
  - ✓ Unsupported types return 0 with a note
  - ✓ Unit test covers the mock path

#### B3: Implement GetPricingSpec() RPC (DONE)
- **Files**: `internal/server/flexera_server.go`
- **Acceptance**:
  - `GetPricingSpec()` returns pricing breakdown if available
  - Returns error if not applicable to Flexera model
  - Unit tests pass
- **Tasks**:
  - ✓ Returns currency from `settings/currency_code` when the client is configured
  - ✓ `rate_per_unit` is 0. Assumptions explain that Flexera returns billed cost, not a rate card
  - ✓ Unsupported types return NotFound
  - ✓ Unit tests cover both paths

### Phase C: Testing and Validation

#### C1: Unit Tests for Core Logic (DONE)
- **Files**: `internal/server/flexera_server_test.go`, `internal/flexera/client_test.go`
- **Acceptance**:
  - All core RPC methods have unit tests
  - Coverage >= 70% for new code
  - Tests use mocked HTTP client
  - `go test ./... -race` passes
- **Tasks**:
  - ✓ Tests cover Name, GetPluginInfo, Supports, GetActualCost, GetProjectedCost, and GetPricingSpec
  - ✓ HealthCheck stays deferred with A5
  - ✓ Cost tests use a fake `flexeraapi.Client`
  - ✓ Error cases include a missing billing center, a long window, truncation, and API failure

#### C2: Integration Test (DONE, SKIPPED WITHOUT CREDENTIALS)
- **Files**: `test/integration/` (new directory)
- **Acceptance**:
  - Full request/response cycle with real Flexera API
  - **BLOCKED-ON-CREDENTIALS**: Only runs with FLEXERA_ORG_ID and FLEXERA_API_TOKEN env vars
  - Test documentation includes setup instructions
- **Tasks**:
  - ✓ `test/integration/live_test.go` calls currency and `costs/select`
  - ✓ Skips unless `FLEXERA_ORG_ID`, `FLEXERA_BILLING_CENTER_IDS`, and OAuth credentials are set
  - ✓ Documented in README. Run with `make test-integration`
  - ✓ Build tag: `//go:build integration`

#### C3: Goreleaser Asset Test Validation (DONE)
- **Files**: `goreleaser_test.go` (already exists)
- **Acceptance**:
  - `TestGoreleaserAssetNames` passes (already implemented)
  - Assets named correctly for installer discovery
- **Tasks**:
  - ✓ `TestGoreleaserAssetNames` is part of `go test ./...`

### Phase D: Documentation

#### D1: Update README.md with Flexera API Details (DONE)
- **Files**: `README.md`
- **Acceptance**:
  - README explains Flexera integration
  - Setup instructions included (note limitations)
  - API support matrix documented
  - markdownlint passes
- **Tasks**:
  - ✓ README documents Bill Analysis `costs/select` and the RPC matrix
  - ✓ Authentication is a refresh token or service account, not a pasted access token
  - ✓ Regions `nam`, `eu`, and `apac` are documented
  - ✓ Example configuration and v0.1.0 limitations are in the README

#### D2: Update CLAUDE.md with Implementation Notes (DONE)
- **Files**: `CLAUDE.md`
- **Acceptance**:
  - Implementation details documented
  - markdownlint passes
  - No references to the legacy product name
- **Tasks**:
  - ✓ Protocol section documents the unified client
  - ✓ Cost calculation, testing, and troubleshooting sections are present
  - ✓ The legacy product name is removed from CLAUDE.md

---

## Issue-Driven Follow-up Tasks

Derived from a per-issue code analysis on 2026-10-05. The umbrella issues are wish lists that mostly describe a platform, not a stateless `CostSourceService` plugin. Each group below keeps only the slice that fits the plugin. Task ids are proposed child issues.

### Working a task

File each task as a GitHub issue, then run it through `/pick-issue`. One task is one issue is one PR. The command lives in `.claude/commands/pick-issue.md`. It branches, implements with tests, validates, and commits; it does not push.

### Close or re-scope

- **#1**: Close. Lint is clean.
- **#9, #10**: Close as won't-do-as-scoped, or retitle. Dashboards, ML, NLP, SOC2, and uptime targets belong to the host or to Flexera One.
- **#5, #6**: Retitle to the narrow slice (read-only `GetBudgets`; billing-center attribution on cost rows). Alerting, ERP, and reporting are out of scope.

### CI and release (issue #7)

Release Please failed on `main` with "Input required and not supplied: token". The owner has since added the `RELEASE_PLEASE_TOKEN` secret that `.github/workflows/release-please.yml` reads.

- **CI1**: Confirm the next push to `main` runs Release Please successfully. The secret's presence could not be verified from this environment.
- **CI2**: Add `-race -coverprofile` to `test.yml` and the Makefile `test` target, with an 80% gate.
- **CI3**: Add CodeQL and a secret scanner workflow.
- **CI4**: Enable branch protection on `main` with required checks (the API returns 404 today).
- **CI5**: Make govulncheck blocking once the grpc fix lands (it is `continue-on-error` now).
- **CI6**: Add SBOM and signing to `.goreleaser.yaml` after the first tagged release.

### Hardening (issue #3)

Already present: TLS 1.2 minimum, `FLEXERA_TIMEOUT`, SDK-managed token refresh, wrapped errors.

- **E1**: Redaction helper with tests proving tokens never reach logs or errors.
- **E2**: Per-RPC `context.WithTimeout` in `internal/server`.
- **E3**: Retry with backoff and jitter for 429 and 5xx, honoring `Retry-After`, in `internal/flexeraapi`.
- **E4**: Map errors to gRPC status codes with one taxonomy.
- **E5**: Replace `log.Printf` in `main.go` with `slog` and add `FLEXERA_LOG_LEVEL`. Warn when `tlsSkipVerify` is set.
- Later: config file permission check, rate limiter, circuit breaker, health service, metrics, tracing. Confirm what finfocus-core expects before adding any listener.

### Tests (issue #8)

Coverage on 2026-10-05: `internal/server` 81.7%, `internal/flexera` 61.0%, `internal/flexeraapi` 47.5%, `cmd` 0%. Target 80%, not 95%.

- **T1**: Raise `internal/flexeraapi` to 80% (error, token, and 202 paths).
- **T2**: Raise `internal/flexera` to 80%.
- **T3**: Extract `run()` from `main.go` and test it.
- **T4**: Add context-cancellation and error-injection tests (timeouts, 429, 5xx).
- **T5**: Add fuzz and benchmark tests for the select-payload builder and config parsing (post-v0.1).

### Documentation (issue #11)

- **D3**: Fix `plugin.manifest.json`. `apiToken` is marked required, and `refreshToken`, `clientId`, `clientSecret`, `billingCenterIds`, and `costMetric` are missing. Check whether `goreleaser_test.go` or the host validates it first.
- **D4**: Remove stale `FLEXERA_API_TOKEN` guidance from `CLAUDE.md` (Testing Approach) and the README.
- **D5**: Add `docs/troubleshooting.md`, a FAQ, `CONTRIBUTING.md`, and a getting-started path that shows how to find the org id and billing center id.
- Later: architecture, security, and performance docs; a diagnostic subcommand. Drop the community and training items.

### Billing center code (issue #6)

- **F1**: `internal/flexera/billing_center.go` and the related `Client` methods are not called by `internal/server`. Wire them in or delete them. Until then the README claim that cost data is "automatically enriched" is wrong.
- **F2**: Delete the unused legacy `Client.GetBudgets`, `Costs`, `BuildCostQuery`, and `GetCloudAccounts` if nothing references them (check first).

### Optional RPCs (post-v0.1, need credentials)

- **R1** (#4, #10): `GetRecommendations` and `DismissRecommendation` from the Optima recommendations operations. Dismissal needs state or an upstream status update. Update `implemented_rpcs` and capabilities.
- **R2** (#5): Read-only `GetBudgets` from `BudgetBudgetIndex` and `BudgetBudgetReport`. Verify the report shape against a live org.
- **R3** (#10): Populate `ImpactMetric` carbon and energy. BLOCKED: no sustainability endpoint in the unified client.
- **R4** (#9): Add resource types (for example `aws-lambda`, `azure-sql`, `gcp-gke`) after validating service dimension names against a live org.

### OpenSpec adoption (issue #12)

Gate: do not start before the v0.1.0 tag and release exist.

- **O1**: Tag and release v0.1.0, then confirm with `gh release view`.
- **O2**: Pin `npm:@fission-ai/openspec` in `mise.toml`. Decide whether to drop the pinned `pipx:specify-cli`.
- **O3**: Run `openspec init` and commit the generated files with `chore:` or `docs:` types so Release Please does not react.
- **O4**: Align markdownlint scope between the Makefile (`README.md *.md`, root only) and CI, and check Vale scope for generated files.
- **O5**: Add `.claude/commands/pick-issue.md`. Done: the command exists; revisit it once OpenSpec changes replace `TASKS.md`.
- **O6**: Add `make spec-validate` and a CI step running `openspec validate --all --strict`.
- **O7**: Write baseline specs only for tested behavior. Blocked on recorded-response tests, which conflicts with the "captured responses not required" decision above; resolve that first.
- After adoption, freeze this file as history, update `CLAUDE.md` so agents stop following it, and turn the A5 HealthCheck deferral into the first OpenSpec change.

---

## Release Criteria for v0.1.0

- [x] All Phase A and C tasks complete (A5 HealthCheck stays deferred to v0.1.1)
- [x] Phase B tasks complete with mocked Flexera responses. Live captures remain owner input
- [x] `go build ./...` succeeds
- [x] `go test ./... -race` passes. `internal/server` coverage is 81.4%
- [x] `go vet ./...` passes
- [x] `golangci-lint run` is clean (0 issues, verified 2026-10-05). Issue #1 can be closed
- [x] README.md, CLAUDE.md, and TASKS.md pass markdownlint
- [x] Legacy product name removed from CLAUDE.md, README samples, and testdata
- [ ] Tag v0.1.0-alpha created (not created in this change)
- [ ] GitHub release published with setup instructions (not published in this change)

---

## Owner Inputs Required for a Live Call

Cost RPCs are implemented against `github.com/flexera-public/unified-go-client`. Captured JSON responses are not required. The generated models for `costs/select`, `settings/currency_code`, and `forecasts/report` are the contract.

A live call still needs credentials in the environment:

1. A refresh token, or a service account client ID and secret
2. A numeric org ID and zone (`nam`, `eu`, or `apac`)
3. At least one billing center ID

`make test-integration` skips when those values are absent.

## Known Blockers for v0.1.0

| Blocker | Impact | Mitigation |
|---------|--------|------------|
| Live credentials | `make test-integration` skips until org id, OAuth credentials, and a billing center id are set | Unit tests use the generated client types and a fake `flexeraapi.Client` |
| Forecast report empty | `forecasts/report` can return no `forecastAmounts` | Fall back to linear extrapolation of `costs/select` and say so in `billing_detail` |

---

## Post-v0.1.0 Roadmap (Issue References)

- **v0.1.x**: CI1-CI4 (#7), E1-E5 (#3), T1-T4 (#8), D3-D5 (#11), F1-F2 (#6)
- **v0.1.1**: A5 HealthCheck (ObservabilityService)
- **v0.2**: R1 recommendations (#4), R2 budgets (#5), R4 resource types (#9), O1-O7 OpenSpec (#12)
- **Blocked**: R3 carbon and energy metrics (#10), pending a Flexera data source
- **Closed or won't-do**: #1 (done), #9 and #10 as scoped, and the reporting, ERP, and alerting parts of #5 and #6
