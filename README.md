# pulumi-plugin-flexera

A PulumiCost **CostSource** plugin that reads **actual** and **projected** cloud costs from **Flexera Optima** via its Bill Analysis API, exposed over gRPC using the `costsource.proto` from `pulumicost-spec`.

## Capabilities

- **Actual cost** by cloud dimensions (vendor, service, region, account, resource group, billing center)
- **Projected cost** using Flexera's advanced cost analytics and trend analysis
- **Multi-cloud support** for AWS, Azure, GCP, and other cloud providers
- **Regional deployment** support for NAM, EU, and APAC regions
- **Rule-based dimensions** for custom cost allocation and chargeback
- Pluggable, isolated process compatible with PulumiCost plugin host

## Installation (dev)

```bash
git clone https://github.com/rshade/pulumi-plugin-flexera
cd pulumi-plugin-flexera
go mod tidy
make build
```

This builds `bin/pulumicost-flexera`. Place it where PulumiCost can find it:

```text
~/.pulumicost/plugins/flexera/1.0.0/pulumicost-flexera
```

# Folder structure
```text
pulumi-plugin-flexera/
├─ README.md
├─ go.mod
├─ go.sum
├─ cmd/
│  └─ pulumicost-flexera/
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
├─ proto/                            # pulled in via submodule or copied from pulumicost-spec
│  └─ costsource.proto               # (optional local copy for dev; canonical in pulumicost-spec)
├─ plugin.manifest.json
├─ config.example.yaml
├─ swagger.json                      # Flexera Optima API specification
├─ Makefile
└─ testdata/
   ├─ sample_request.json
   ├─ sample_response_actual.json
   └─ sample_response_projected.json
```
# Configuration

Use environment variables or a YAML config file (path can be provided via `FLEXERA_CONFIG`):

## Environment Variables
```text
FLEXERA_REGION          # Region: nam, eu, apac (default: nam)
FLEXERA_ORG_ID          # Your Flexera organization ID (required)
FLEXERA_API_TOKEN       # JWT token from Flexera Cloud Management API (required)
FLEXERA_BASE_URL        # Override auto-configured API endpoint (optional)
FLEXERA_DEFAULT_WINDOW  # Default query window, e.g., 30d (default: 30d)
FLEXERA_TIMEOUT         # HTTP timeout, e.g., 30s (default: 30s)
FLEXERA_TLS_SKIP_VERIFY # Skip TLS verification: true|false (default: false)
```

## YAML Configuration
See `config.example.yaml` for basic configuration and `config.billing-centers.example.yaml` for advanced billing center mappings:

```yaml
region: nam                    # nam, eu, or apac
orgId: "12345"                # Your Flexera org ID
apiToken: "jwt-token-here"    # JWT from Cloud Management API
defaultWindow: 30d
timeout: 30s
tlsSkipVerify: false

# Optional: Billing Center Mappings (see full example below)
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

# Protocol
Implements CostSource from pulumicost-spec/proto/costsource.proto. Methods:

* Name()
* Supports(ResourceDescriptor)
* GetActualCost(ActualCostQuery)
* GetProjectedCost(ResourceDescriptor)
* GetPricingSpec(ResourceDescriptor)


# Resource Mapping

ResourceDescriptor fields → Flexera Optima filters:

* `Provider`: Cloud provider (AWS, Azure, GCP, etc.)
* `ResourceType`: Supported types include:
  - `aws-ec2`, `aws-s3`, `aws-rds`
  - `azure-vm`, `azure-storage`
  - `gcp-compute`, `gcp-storage`
  - `cloud-account`, `cloud-service`, `cloud-region`
  - `billing-center`, `resource-group`
* `Region`: Cloud region for filtering
* `Tags`: Maps to Flexera tag dimensions

## Resource ID Patterns

`ActualCostQuery.ResourceID` accepts flexible IDs for cost filtering:

* `vendor_account/<account-id>` - Filter by cloud account
* `service/<vendor>/<service-name>` - Filter by vendor and service (e.g., `service/aws/ec2`)
* `region/<vendor>/<region>` - Filter by vendor and region (e.g., `region/azure/eastus`)
* `resource_group/<group-name>` - Filter by resource group
* `billing_center/<bc-id>` - Filter by billing center
* `tag/<key>/<value>` - Filter by tag (e.g., `tag/environment/production`)

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
3. **Cost Enrichment**: Cost data is automatically enriched with billing center IDs
4. **Flexera Integration**: Mappings can generate Flexera RBD rules for native integration

### Use Cases

- **Departmental Chargeback**: Allocate costs to departments based on tags
- **Project Accounting**: Track project costs using project tags
- **Environment Segmentation**: Separate production, staging, and development costs
- **Team Attribution**: Assign costs to specific teams or owners
- **Cost Center Allocation**: Map to financial cost centers for accounting

See `config.billing-centers.example.yaml` for a complete configuration example with detailed comments.

# Security

* All API calls use HTTPS to Flexera Optima endpoints
* JWT tokens are handled securely and not logged
* Supports regional data residency requirements
* Redact sensitive fields in errors/logs
* TLS certificate verification enabled by default

# Testing
```bash
make test
```
Use testdata/ JSON fixtures. For live testing, set required environment variables:
```bash
export FLEXERA_ORG_ID="your-org-id"
export FLEXERA_API_TOKEN="your-jwt-token"
export FLEXERA_REGION="nam"  # or eu/apac
make test
```

# License
[Apache-2.0](LICENSE)

# Plugin Manifest

The `plugin.manifest.json` defines the plugin capabilities:

```json
{
  "name": "flexera",
  "version": "1.0.0",
  "type": "costsource",
  "description": "Flexera Optima plugin for Pulumicost - provides actual and projected cost for cloud resources",
  "executable": "pulumicost-flexera",
  "protocol": "grpc",
  "supported_resources": [
    "aws-ec2", "aws-s3", "aws-rds",
    "azure-vm", "azure-storage", 
    "gcp-compute", "gcp-storage",
    "cloud-account", "cloud-service", "cloud-region",
    "billing-center", "resource-group"
  ]
}
```

## Getting Started

1. **Obtain Flexera Credentials**:
   - Get a JWT token from the Flexera Cloud Management API
   - Identify your organization ID
   - Determine your region (NAM, EU, or APAC)

2. **Configure the Plugin**:
   ```bash
   export FLEXERA_ORG_ID="12345"
   export FLEXERA_API_TOKEN="your-jwt-token"
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

# plugin.manifest.json

```json
{
  "name": "kubecost",
  "version": "1.0.0",
  "kind": "cost",
  "providers": ["kubernetes", "aws", "gcp", "azure"],
  "resourceTypes": ["k8s-namespace", "k8s-pod", "k8s-controller", "k8s-node"],
  "entrypoint": "pulumicost-kubecost"
}
