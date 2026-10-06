# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Building and Testing

```bash
# Install the pinned toolchain (Go 1.27.1 and golangci-lint 2.14.0)
mise install

# Build the plugin binary
make build

# Run tests
make test

# Run linting with the mise-pinned golangci-lint
make lint

# Install to local plugin directory
make install
```

## Project Architecture

This is a gRPC plugin that implements the CostSource service from `finfocus-spec`. Key components:

- **gRPC Server**: Listens on port 50051 and registers `finfocus.v1.CostSourceService`
- **Flexera Optima Client**: HTTP client that queries the Flexera Optima Bill Analysis API for cost data
- **Configuration**: Supports both environment variables and YAML config files

### Protocol integration (v0.1.0 Phase A)

`FlexeraServer` implements `finfocus.v1.CostSourceService` from `finfocus-spec` v0.7.5. `main` calls `RegisterService` and enables gRPC reflection.

- `Name` returns `flexera`.
- `GetPluginInfo` reports the plugin version, `pluginsdk.SpecVersion`, providers `aws`, `azure`, and `gcp`, and capabilities for actual cost, projected cost, and pricing spec. Metadata key `implemented_rpcs` lists the RPCs this binary implements. Estimate, recommendations, budgets, dry run, batch, and type resolution stay on the embedded unimplemented server.
- `Supports` allows `aws-ec2`, `aws-s3`, `aws-rds`, `azure-vm`, `azure-storage`, `gcp-compute`, and `gcp-storage`. When the host sends a provider, it must match the resource type prefix. A descriptor region of `nam`, `eu`, or `apac` (including `north-america`, `europe`, and `asia-pacific`) must match the plugin's configured Flexera zone. Cloud regions such as `us-east-1` are accepted.
- `GetActualCost` calls `CostsSelect` (`BillAnalysisCostsSelectWithResponse`) with `billing_center_ids`, an `equal` filter, and day windows of at most 31 days. Currency comes from `BillAnalysisCurrencySettingShowWithResponse` and is cached for 15 minutes. Each row is rounded to the currency minor unit.
- `GetProjectedCost` calls `ForecastReport` (`BillAnalysisForecastsReportWithResponse`) for the current month. The monthly figure is the sum of `forecastAmounts`. If that report returns no amounts or fails, the plugin falls back to a 90-day linear extrapolation of `costs/select`. `main` builds the client from `FLEXERA_REFRESH_TOKEN` or `FLEXERA_CLIENT_ID` plus `FLEXERA_CLIENT_SECRET`. The legacy HTTP client in `internal/flexera` is not used for these RPCs.

## Key Implementation Details

### Flexera Optima API Integration

The plugin integrates with Flexera Cloud Cost Optimization Bill Analysis API:
- **Base URLs**:
  - NAM: `https://api.optima.flexeraeng.com/bill-analysis`
  - EU: `https://api.optima-eu.flexeraeng.com/bill-analysis`
  - APAC: `https://api.optima-apac.flexeraeng.com/bill-analysis`
- **Authentication**: JWT tokens from Cloud Management API
- **API Version**: "1.0" (set in Api-Version header)

### Resource ID Mapping

The plugin maps resource IDs to Flexera Optima dimensions and filters:
- `vendor_account/<account_id>` → filter by vendor account
- `service/<vendor>/<service_name>` → filter by vendor and service
- `region/<vendor>/<region>` → filter by vendor and region
- `resource_group/<group_name>` → filter by resource group
- `billing_center/<bc_id>` → filter by billing center

### Supported Flexera Dimensions

- `vendor` (AWS, Azure, GCP, etc.)
- `vendor_account`
- `service`
- `region`
- `resource_group`
- `resource_type`
- `instance_type`
- `billing_account`
- Custom tag dimensions
- Rule-based dimensions (RBDs)

### Cost Metrics

Uses `cost_amortized_unblended_adj` as the primary cost metric for accurate cost reporting.

### Cost Projections

`GetProjectedCost` calculates trends from historical cost data and projects future costs. This leverages Flexera's cost analytics capabilities.

### Error Handling

- HTTP client includes timeout support via context
- `GetActualCost`, `GetProjectedCost`, and `GetPricingSpec` each apply their own deadline (`FLEXERA_TIMEOUT`, default 30s)
- `internal/flexeraapi` retries HTTP 429 and 5xx with backoff and jitter and honors `Retry-After`. Other 4xx responses are not retried
- The same dependency failure maps to the same gRPC status on all three cost RPCs
- Refresh tokens, client secrets, bearer tokens, and JWTs are redacted from error strings
- Startup logging uses `slog` and `FLEXERA_LOG_LEVEL`. A warning is logged when `tlsSkipVerify` is set
- The unified client refreshes the access token from a refresh token or service account
- `make test` runs `go test -race -covermode=atomic` and fails when `internal/flexera` or `internal/flexeraapi` is under 80% statement coverage

### Cost calculation

`GetActualCost` rounds each `costs/select` metric to the currency minor unit (2 digits by default, 0 for JPY/KRW/VND, 3 for BHD/KWD/OMR). `GetProjectedCost` prefers the sum of `forecastAmounts` from `forecasts/report`. The fallback sums rounded `costs/select` rows as minor units over a 90-day lookback, divides by the number of rows, multiplies by 30, and applies a trend when there are at least 30 points. `billing_detail` says which method produced the figure.

### Testing

`make test` runs unit tests with a fake Flexera client. Recipe targets are `.PHONY` because a `test/` directory would otherwise make `make test` a no-op. `make test-integration` calls Bill Analysis and skips unless `FLEXERA_ORG_ID`, `FLEXERA_BILLING_CENTER_IDS`, and OAuth credentials are set. `go test -race ./...` is the race check.

### Troubleshooting

- Cost RPCs return FailedPrecondition when the unified client or `billingCenterIds` is missing.
- `FLEXERA_ORG_ID` must be numeric for `flexeraapi.New`.
- A descriptor region of `eu` against a plugin configured for `nam` makes `Supports` return false. Cloud regions such as `us-east-1` do not.
- HTTP 202 with an empty body and a truncated `costs/select` result are errors, not empty success.
- Do not log refresh tokens, client secrets, or access tokens.

## Flexera API Client

The plugin uses the generated Flexera unified Go client (`github.com/flexera-public/unified-go-client`, Apache-2.0) to query the Bill Analysis API. The client is wrapped by `internal/flexeraapi` which exposes a simple interface for costs/select queries.

**Key details:**
- **No tags**: The module version is a pseudo-version (`v0.0.0-20261001183948-92b58ffbc17f`). Bump it by checking the commit hash at the [unified-go-client repo](https://github.com/flexera-public/unified-go-client).
- **Generator refreshes specs**: The client is auto-generated from `unified-openapi` specs; check the README there for regeneration.
- **Authentication**: OAuth refresh token or service account credentials, exchanged at `https://login.flexera.<zone>/oidc/token` (zone = com, eu, au). Config holds refresh token or client id and secret, never access tokens (1-hour lifetime).
- **Request validation**: `billing_center_ids` is required; `metrics`, `dimensions`, and `limit` are also required. Amount values in response are JSON float64 (safe for decimal).
- **Day granularity**: Max 31 days per request; month granularity max 24 months. Format: YYYY-MM-DD for day, YYYY-MM for month.
- **Currency**: Read from `settings/currency_code` endpoint; org-wide setting, not per-row.
- **Contract**: Request and response shapes come from `github.com/flexera-public/unified-go-client`. Captured JSON fixtures are not required. `costs/select` uses snake_case body fields. `forecasts/report` uses the generated `BillAnalysisReportRequestBody3` (`billingCenterIds`, `startAt`, `endAt`, `lookbackPeriod`, `forecastAmounts`).

See `flexera-api-facts.md` in the PM research folder for the full API contract and implementation gaps.

## Dependencies

The plugin depends on:
- `github.com/rshade/finfocus-spec` v0.7.5 - Protocol buffer definitions and plugin SDK (`sdk/go/proto/finfocus/v1` imported as `pbc`)
- `github.com/flexera-public/unified-go-client` v0.0.0-20261001183948-92b58ffbc17f - Generated Flexera API client
- Standard gRPC and protobuf libraries
- `gopkg.in/yaml.v3` for configuration parsing

## Testing Approach

- Unit tests for individual components (client, config, server methods)
- Integration tests can use the testdata/ JSON files
- For a live call, set a numeric `FLEXERA_ORG_ID`, `FLEXERA_BILLING_CENTER_IDS`, and either `FLEXERA_REFRESH_TOKEN` or `FLEXERA_CLIENT_ID` plus `FLEXERA_CLIENT_SECRET`

## Common Development Tasks

### Adding New Resource Types

1. Update the `Supports()` method in `flexera_server.go`
2. Add mapping logic in `GetActualCost()` for the new resource ID format
3. Update `plugin.manifest.json` with the new resource type

### Debugging the gRPC Server

The server includes reflection support, so you can use tools like `grpcurl`:

```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe finfocus.v1.CostSourceService
```

### Modifying Flexera API Calls

Cost RPCs go through `internal/flexeraapi`. `internal/flexera/client.go` resolves billing centers and builds RBD config. To call another Bill Analysis endpoint:
1. Add a method on the flexeraapi client
2. Use the generated unified-go-client request types
3. Send the call through the shared retry helper so HTTP 429 and 5xx are retried and token material is redacted

## Flexera-Specific Notes

### Authentication

- Requires JWT token from Flexera Cloud Management API
- Token should be refreshed before expiration
- Supports different regions (NAM, EU, APAC)

### Cost Queries

- Supports complex filtering using Flexera's dimension system
- Can query historical data with date ranges
- Supports aggregation by multiple dimensions

### Regional Deployment

- Different API endpoints for different regions
- Configure region in plugin configuration

## Billing Center Tag Mapping

The plugin includes sophisticated billing center allocation capabilities:

### Features

- **Tag-based mapping**: Map resource tags to billing centers automatically
- **Hierarchical rules**: Priority-based rule evaluation for complex scenarios
- **Default fallback**: Configure default billing center for unmatched resources
- **Flexera RBD generation**: Generate native Flexera rule-based dimension configurations
- **Auto-discovery**: Optional billing center lookup by name

### Configuration

Add billing center mappings to your config file:

```yaml
billingCenterMappings:
  tagMappings:
    "environment:production": "bc-prod-123"
    "team:platform": "bc-platform-abc"
  
  hierarchicalMappings:
    - priority: 1
      tagKey: "department"
      mappings:
        "engineering": "bc-engineering-main"
  
  defaultBillingCenter: "bc-unallocated"
  enableAutoDiscovery: true
```

### Usage

- `GetActualCost` is automatically enriched with a billing center id on the result lineage when tag mappings match `tag:` or `tag_` dimensions
- Use `client.GetBillingCenterForTags()` to resolve billing centers programmatically
- Generate Flexera RBD configs with `client.GenerateFlexeraRBDConfig()`

### Common Development Tasks

#### Adding New Tag Mappings

1. Update the `billingCenterMappings` in your config file
2. Add new tag patterns to `tagMappings` or `hierarchicalMappings`
3. Test with `make test` to validate mappings

#### Generating Flexera RBD Configuration

Use the client to generate native Flexera rule-based dimension configs:

```go
config, err := client.GenerateFlexeraRBDConfig()
// Upload config to Flexera via their RBD API
```

## Development Learnings & Best Practices

### Plugin Conversion Process

This project was converted from an earlier cost plugin. Key learnings:

#### Conversion Strategy

1. **Start with Configuration**: Update config structures and environment variables first
2. **Convert Client Layer**: Implement new API client before server changes
3. **Update Server Logic**: Modify gRPC server implementation for new data sources
4. **Comprehensive Testing**: Create tests during conversion, not after
5. **Clean Up Last**: Remove old files only after everything is working

#### Common Issues Encountered

- **Unused Variables**: Go compiler is strict about unused imports/variables during conversion
- **Import Path Updates**: Remember to update all import paths when changing module names
- **Module Naming**: Update go.mod module declaration early in the process
- **Build Dependencies**: Test build frequently during conversion to catch issues early

#### Effective Development Patterns

- **TodoWrite Tool**: Excellent for tracking complex conversion progress systematically
- **MultiEdit Tool**: Very efficient for bulk file changes across multiple locations  
- **Incremental Building**: Build and test after each major component conversion
- **Test-Driven Conversion**: Write tests alongside new functionality

### Flexera Optima Integration Insights

#### API Patterns

- **Regional Endpoints**: Flexera uses different API endpoints per region (NAM/EU/APAC)
- **JWT Authentication**: Requires proper JWT tokens with Api-Version headers
- **Dimensional Filtering**: Rich filtering system using vendor, service, region, tags
- **Cost Metrics**: Primary metric is `cost_amortized_unblended_adj` for accurate reporting

#### Configuration Best Practices

- **Auto-Configuration**: Use region to auto-configure API endpoints
- **Validation**: Validate a numeric org id plus a refresh token or client id and secret at startup
- **Hierarchical Config**: Support both env vars and YAML for flexibility
- **Pulumi Patterns**: Follow Pulumi-style config conventions for consistency

#### Billing Center Allocation Patterns

- **Priority-Based Resolution**: Implement clear priority order for tag resolution
- **Fallback Strategies**: Always provide default billing center for unmatched resources
- **Tag Enrichment**: Enrich cost data with billing center IDs automatically
- **RBD Generation**: Generate native Flexera rule-based dimension configurations

### Testing Strategies

#### Conversion Testing

- **Unit Tests Per Component**: Test config, client, server, and billing center logic separately
- **Mock HTTP Servers**: Use httptest.NewServer for testing API interactions
- **Configuration Testing**: Test all config loading scenarios including validation
- **Integration Testing**: Test end-to-end workflows with realistic data

#### Test Organization

```bash
# Run specific test groups during development
go test ./internal/flexera -run TestBillingCenter -v
go test ./internal/flexera -run TestConfig -v
go test ./internal/server -v
```

### Common Development Commands

#### Linting and Quality

```bash
# Build and test in one command
make build && make test

# Run linter (expect some issues with generated code)
make lint

# Check specific package
go test ./internal/flexera -v
```

#### Configuration Validation

```bash
# Show the version without serving
go run ./cmd/finfocus-plugin-flexera --version

# Live calls need a numeric org id, billing center ids, and OAuth credentials
export FLEXERA_ORG_ID="12345"
export FLEXERA_BILLING_CENTER_IDS="billing-center-id"
export FLEXERA_REFRESH_TOKEN="refresh-token"
export FLEXERA_REGION="nam"
```

### File Organization Patterns

#### Package Structure

- `internal/flexera/`: Core Flexera API integration
  - `config.go`: Configuration loading and validation
  - `client.go`: Billing-center enrichment and RBD config. Cost RPCs use `internal/flexeraapi`
  - `billing_center.go`: Tag-to-billing-center mapping logic
- `internal/server/`: gRPC server implementation
- `cmd/finfocus-plugin-flexera/`: Main application entry point

#### Configuration Files

- `config.example.yaml`: Basic configuration template
- `config.billing-centers.example.yaml`: Advanced billing center mapping examples
- Environment variables follow `FLEXERA_*` pattern for consistency

### Performance Considerations

#### HTTP Client Optimization

- **Connection Pooling**: Use persistent HTTP client with proper timeouts
- **Regional Endpoints**: Connect to closest regional endpoint for latency
- **Concurrent Requests**: Client supports concurrent cost queries safely
- **Response Caching**: Consider caching billing center resolutions

#### Memory Management

- **Cost Data Processing**: Process cost points in streaming fashion for large datasets
- **Tag Extraction**: Efficient tag parsing without unnecessary allocations
- **Billing Center Cache**: Cache resolved billing centers to avoid repeated lookups

### Security Best Practices

#### Credential Management

- **JWT Tokens**: Never log JWT tokens in error messages or debug output
- **Environment Variables**: Prefer environment variables for sensitive configuration
- **TLS Configuration**: Default to secure TLS, only allow skip for development
- **Regional Data**: Respect data residency requirements with regional endpoints

#### Error Handling

- **Sensitive Data**: Redact sensitive information from error messages
- **Context Propagation**: Use context.Context for request timeouts and cancellation
- **Graceful Degradation**: Handle missing billing center mappings gracefully

## Project Management & GitHub CLI

### GitHub CLI Commands for Project Management

```bash
# List existing issues and milestones
gh issue list --repo OWNER/REPO
gh api /repos/OWNER/REPO/milestones

# Create milestones with proper naming convention (YYYY-Q[1-4] - Description)
gh api --method POST -H "Accept: application/vnd.github+json" /repos/OWNER/REPO/milestones \
  -f title="2025-Q3 - Foundation & Core Features" \
  -f due_on="2025-09-30T23:59:59Z"

# Create issues with proper structure
gh issue create --repo OWNER/REPO \
  --title "Implement Core Plugin Functionality" \
  --body "Detailed description with acceptance criteria" \
  --label "enhancement,finops,core"

# Assign issues to milestones and add labels
gh issue edit ISSUE_NUMBER --repo OWNER/REPO --milestone "2025-Q3 - Foundation & Core Features"
gh issue edit ISSUE_NUMBER --repo OWNER/REPO --add-label "priority:high"
```

### FinOps Project Management Patterns

#### Milestone Planning Strategy

- **Foundation Quarter**: Core functionality, basic integration, authentication
- **Enhancement Quarter**: Advanced features, optimization, analytics
- **Enterprise Quarter**: Multi-tenancy, governance, compliance
- **Innovation Quarter**: AI/ML features, automation, advanced analytics

#### Issue Categories for FinOps Tools

- **Core Infrastructure**: gRPC servers, API clients, data processing
- **Cost Management**: Billing centers, allocation, showback/chargeback
- **Security & Compliance**: Authentication, encryption, audit trails  
- **Performance**: Caching, concurrent processing, optimization
- **Analytics**: Reporting, anomaly detection, trend analysis
- **Integration**: Multi-cloud support, third-party connectors

#### Priority Classification

- **P0 (Critical)**: Core functionality, security vulnerabilities
- **P1 (High)**: Major features, performance issues
- **P2 (Medium)**: Enhancements, documentation, optimizations  
- **P3 (Low)**: Nice-to-have features, minor improvements

### Project Analysis Best Practices

#### Pre-Project Management Checklist

1. **Codebase Analysis**: Review existing files, architecture, dependencies
2. **Current State Assessment**: Check existing issues, PRs, documentation
3. **Gap Analysis**: Identify missing functionality, technical debt
4. **Stakeholder Mapping**: Understand user personas (DevOps, FinOps, Finance teams)

#### Comprehensive Issue Creation

- **Clear Titles**: Use action verbs and specific scope
- **Detailed Descriptions**: Include acceptance criteria, technical requirements
- **Proper Labeling**: Use consistent labels for categorization and filtering
- **Milestone Assignment**: Align with quarterly objectives and dependencies
- **Priority Setting**: Consider business impact and technical complexity

#### FinOps-Specific Considerations

- **Cost Visibility**: Ensure transparency in cost reporting and allocation
- **Multi-Cloud Support**: Plan for AWS, Azure, GCP integration patterns
- **Billing Integration**: Consider enterprise billing systems and workflows
- **Optimization Focus**: Include automated recommendations and alerting
- **Governance Features**: Plan for policy enforcement and compliance reporting
