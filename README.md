# finfocus-plugin-flexera

A FinFocus **CostSource** plugin that reads cloud cost from Flexera One Cloud Cost Optimization (Bill Analysis API) over gRPC (`finfocus.v1.CostSourceService` from `finfocus-spec` v0.7.5).

## Capabilities

- **Actual cost** from `costs/select`, filtered by resource id, in day windows of at most 31 days
- **Projected cost** from `forecasts/report` on the generated Flexera client, with linear extrapolation when that report has no amounts
- **Multi-cloud support** for AWS, Azure, GCP, and other cloud providers
- **Regional deployment** support for NAM, EU, and APAC regions
- **Rule-based dimensions** for custom cost allocation and chargeback
- Pluggable, isolated process compatible with FinFocus plugin host

## Getting started

See [docs/getting-started.md](docs/getting-started.md) for a local build, the
refresh-token or service-account settings, and where to copy the org id and a
billing center id from Flexera One. Common failures are in
[docs/troubleshooting.md](docs/troubleshooting.md). Questions that come up
while wiring the plugin are in [docs/faq.md](docs/faq.md).

## Installation (dev)

```bash
git clone https://github.com/rshade/finfocus-plugin-flexera
cd finfocus-plugin-flexera
go mod tidy
make build
```

This builds `bin/finfocus-plugin-flexera`. Place it where FinFocus can find it:

```text
~/.finfocus/plugins/flexera/0.1.0/finfocus-plugin-flexera
```

## Folder structure

```text
finfocus-plugin-flexera/
├─ README.md
├─ go.mod
├─ go.sum
├─ cmd/
│  └─ finfocus-plugin-flexera/
│     └─ main.go
├─ internal/
│  ├─ server/
│  │  ├─ flexera_server.go
│  │  └─ flexera_server_test.go
│  └─ flexera/
│     ├─ client.go
│     ├─ client_test.go
│     ├─ config.go
│     └─ config_test.go
├─ pkg/
│  └─ version/
│     └─ version.go
├─ .claude/
│  └─ agents/
│     ├─ devops-finops-pm.md
│     └─ pulumi-finops-architect.md
├─ proto/                            # pulled in via submodule or copied from finfocus-spec
│  └─ costsource.proto               # (optional local copy for dev; canonical in finfocus-spec)
├─ plugin.manifest.json
├─ config.example.yaml
├─ swagger.json                      # Flexera Optima API specification
├─ Makefile
└─ testdata/
   ├─ sample_request.json
   ├─ sample_response_actual.json
   └─ sample_response_projected.json
```

## Configuration

Use environment variables or a YAML config file (path can be provided via `FLEXERA_CONFIG`):

## Environment Variables

```text
FLEXERA_REGION               # nam, eu, or apac (default: nam)
FLEXERA_ORG_ID               # Numeric Flexera organization ID (required)
FLEXERA_REFRESH_TOKEN        # Flexera One refresh token (preferred)
FLEXERA_CLIENT_ID            # Service account client id (with CLIENT_SECRET)
FLEXERA_CLIENT_SECRET        # Service account secret
FLEXERA_BILLING_CENTER_IDS   # Comma-separated ids required for cost RPCs
FLEXERA_COST_METRIC          # Default cost_amortized_unblended_adj
FLEXERA_BASE_URL             # Optional Bill Analysis base URL override
FLEXERA_LOG_LEVEL            # debug, info, warn, or error (default info)
FLEXERA_DEFAULT_WINDOW       # Default query window, e.g. 30d
FLEXERA_TIMEOUT              # HTTP timeout, e.g. 30s
FLEXERA_TLS_SKIP_VERIFY      # true or false (default false)
```

## YAML Configuration

See `config.example.yaml` for basic configuration and `config.billing-centers.example.yaml` for advanced billing center mappings:

```yaml
region: nam
orgId: "12345"
refreshToken: ""              # or clientId and clientSecret
billingCenterIds:
  - "unassigned"
costMetric: cost_amortized_unblended_adj
defaultWindow: 30d
timeout: 30s
tlsSkipVerify: false

## Optional: Billing Center Mappings (see full example below)
billingCenterMappings:
  tagMappings:
    "environment:production": "bc-prod-123"
    "team:platform": "bc-platform-abc"
  defaultBillingCenter: "bc-unallocated"
```

## Regional Endpoints

The plugin automatically configures the correct API endpoint based on your region:
- **NAM**: `https://api.optima.flexeraeng.com/bill-analysis`
- **EU**: `https://api.optima-eu.flexeraeng.com/bill-analysis`  
- **APAC**: `https://api.optima-apac.flexeraeng.com/bill-analysis`

## Protocol

The binary registers `finfocus.v1.CostSourceService`.

| RPC | v0.1.0 behavior |
| --- | --- |
| `Name` | Returns `flexera` |
| `GetPluginInfo` | Version, spec version, providers, implemented RPCs |
| `Supports` | Allowlist below. A `nam`/`eu`/`apac` region must match plugin config |
| `GetActualCost` | `costs/select` via the unified Flexera client |
| `GetProjectedCost` | `forecasts/report` for the current month. Extrapolates `costs/select` when the report is empty |
| `GetPricingSpec` | Billed-cost metadata. `rate_per_unit` is 0 |
| `GetRecommendations` | Optima recommendations index. Savings is the impact. Dismissed and snoozed rows are omitted unless `include_dismissed` is true |
| `DismissRecommendation` | Optima status update plus in-process dismissal |
| `GetBudgets` | Read-only Flexera budget index and budget report. No create, update, or delete |
| Health, estimate, batch | Not implemented |

## v0.1.0 limitations

- Cost RPCs need a refresh token or service account, a numeric org id, and at least one billing center id.
- Queries use day granularity, split into 31-day windows, and reject ranges longer than 24 months.
- Amounts are JSON floats rounded to the currency minor unit. Rows are not summed as raw floats.
- HTTP 202 with rows is accepted. HTTP 202 with no rows, or a truncated result, is an error.
- Projection uses the generated `forecasts/report` model. It does not depend on captured JSON fixtures.
- When `forecasts/report` returns no amounts, projection falls back to a local trend from `costs/select`.

Live integration test:

```bash
export FLEXERA_ORG_ID="12345"
export FLEXERA_REFRESH_TOKEN="refresh-token"
export FLEXERA_BILLING_CENTER_IDS="billing-center-id"
export FLEXERA_REGION="nam"
make test-integration
```

## Resource Mapping

ResourceDescriptor fields → Flexera Optima filters:

- `Provider`: Cloud provider (AWS, Azure, GCP, etc.)
- `ResourceType`: Supported types include:
  - `aws-ec2`, `aws-s3`, `aws-rds`
  - `azure-vm`, `azure-storage`
  - `gcp-compute`, `gcp-storage`
- `Region`: Cloud region for filtering
- `Tags`: Maps to Flexera tag dimensions

## Resource ID Patterns

`ActualCostQuery.ResourceID` accepts flexible IDs for cost filtering:

- `vendor_account/<account-id>` - Filter by cloud account
- `service/<vendor>/<service-name>` - Filter by vendor and service (e.g., `service/aws/ec2`)
- `region/<vendor>/<region>` - Filter by vendor and region (e.g., `region/azure/eastus`)
- `resource_group/<group-name>` - Filter by resource group
- A cloud resource id or ARN — `equal` filter on `resource_id`
- `billing_center/<bc-id>` is rejected. Billing centers are the request's `billing_center_ids` field

## Supported Flexera Dimensions

The plugin leverages Flexera's rich dimensional data model:
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

## Billing Center Tag Mapping

The plugin supports automatic billing center allocation based on resource tags. This powerful feature allows you to:

- **Map tags to billing centers** for automatic cost allocation
- **Define hierarchical rules** with priority-based evaluation
- **Set default billing centers** for untagged resources
- **Generate Flexera RBD rules** for billing center allocation

### Configuration Example

```yaml
billingCenterMappings:
  # Direct tag mappings
  tagMappings:
    "environment:production": "bc-prod-123"
    "environment:staging": "bc-staging-456"
    "team:platform": "bc-platform-abc"
    "cost-center:1001": "bc-finance-ghi"
  
  # Hierarchical mappings with priority
  hierarchicalMappings:
    - priority: 1
      tagKey: "department"
      mappings:
        "engineering": "bc-engineering-main"
        "finance": "bc-finance-main"
    
    - priority: 2
      tagKey: "project"
      mappings:
        "alpha": "bc-project-alpha"
        "beta": "bc-project-beta"
  
  # Default for unmatched resources
  defaultBillingCenter: "bc-unallocated"
  
  # Enable auto-discovery by name
  enableAutoDiscovery: true
```

### How It Works

1. **Tag Extraction**: The plugin extracts tags from resource metadata
2. **Rule Evaluation**: Tags are evaluated against mapping rules in order:
   - Direct tag mappings (highest priority)
   - Hierarchical mappings (by priority order)
   - Default billing center (fallback)
3. **Cost Enrichment**: `GetActualCost` requests a `tag_<key>` dimension for each configured tag key. A matching row, or `defaultBillingCenter` when the row has no tag values, is written onto that result's lineage
4. **Flexera Integration**: Mappings can generate Flexera RBD rules for native integration

### Use Cases

- **Departmental Chargeback**: Allocate costs to departments based on tags
- **Project Accounting**: Track project costs using project tags
- **Environment Segmentation**: Separate production, staging, and development costs
- **Team Attribution**: Assign costs to specific teams or owners
- **Cost Center Allocation**: Map to financial cost centers for accounting

See `config.billing-centers.example.yaml` for a complete configuration example with detailed comments.

## Security

- Cost calls use the Flexera unified client and HTTPS
- Refresh tokens and client secrets are not logged
- Supports regional data residency requirements
- Redact sensitive fields in errors/logs
- TLS certificate verification enabled by default

## Testing

```bash
make test
```

Unit tests mock the Flexera client. For a live call, set the variables in the integration example above and run `make test-integration`. `make test` does not call Flexera.

## License

[Apache-2.0](LICENSE)

## Plugin Manifest

The `plugin.manifest.json` defines the plugin capabilities:

```json
{
  "name": "flexera",
  "version": "0.1.0",
  "type": "costsource",
  "description": "Flexera One cost plugin for FinFocus. Actual cost comes from Bill Analysis costs/select.",
  "executable": "finfocus-plugin-flexera",
  "protocol": "grpc",
  "supported_resources": [
    "aws-ec2", "aws-s3", "aws-rds",
    "azure-vm", "azure-storage", 
    "gcp-compute", "gcp-storage"
  ]
}
```

## Getting Started

1. **Obtain Flexera One credentials**:
   - Create a refresh token, or a service account client id and secret
   - Note the numeric organization ID, zone (`nam`, `eu`, or `apac`), and a billing center id

2. **Configure the Plugin**:

   ```bash
   export FLEXERA_ORG_ID="12345"
   export FLEXERA_REFRESH_TOKEN="your-refresh-token"
   export FLEXERA_BILLING_CENTER_IDS="your-billing-center"
   export FLEXERA_REGION="nam"
   ```

3. **Build and Install**:

   ```bash
   make build
   make install
   ```

4. **Test the Plugin**:

   ```bash
   make test
   ```

For more details on Flexera Optima API capabilities, see the included `swagger.json` file.

## plugin.manifest.json

```json
{
  "name": "flexera",
  "version": "0.1.0",
  "kind": "cost",
  "providers": ["aws", "azure", "gcp"],
  "resourceTypes": ["cloud-resource"],
  "entrypoint": "finfocus-plugin-flexera"
}
