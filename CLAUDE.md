# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Building and Testing

```bash
# Build the plugin binary
make build

# Run tests
make test

# Run linting (requires golangci-lint)
make lint

# Install to local plugin directory
make install
```

## Project Architecture

This is a gRPC plugin that implements the CostSource service from `pulumicost-spec`. Key components:

- **gRPC Server**: Listens on port 50051, implements the CostSource service methods
- **Flexera Optima Client**: HTTP client that queries the Flexera Optima Bill Analysis API for cost data
- **Configuration**: Supports both environment variables and YAML config files

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
- JWT token refresh handling
- All errors are propagated with context

## Dependencies

The plugin depends on:
- `github.com/yourorg/pulumicost-spec/sdk/go/proto` - Protocol buffer definitions (needs to be replaced with actual import path)
- Standard gRPC and protobuf libraries
- `gopkg.in/yaml.v3` for configuration parsing

## Testing Approach

- Unit tests for individual components (client, config, server methods)
- Integration tests can use the testdata/ JSON files
- For live testing, set `FLEXERA_API_TOKEN` and `FLEXERA_ORG_ID` environment variables

## Common Development Tasks

### Adding New Resource Types
1. Update the `Supports()` method in `flexera_server.go`
2. Add mapping logic in `GetActualCost()` for the new resource ID format
3. Update `plugin.manifest.json` with the new resource type

### Debugging the gRPC Server
The server includes reflection support, so you can use tools like `grpcurl`:
```bash
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe CostSource
```

### Modifying Flexera API Calls
The HTTP client in `client.go` handles the Flexera Optima API interaction. To add new endpoints:
1. Add new methods to the Client struct
2. Define request/response types matching Flexera API schemas
3. Handle the API call with proper error handling and timeout

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
- Cost data is automatically enriched with billing center IDs
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
This project was converted from a Kubecost plugin to Flexera Optima. Key learnings:

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
- **Validation**: Validate required fields (orgId, apiToken) at startup
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
# Test with environment variables
export FLEXERA_ORG_ID="test-org"
export FLEXERA_API_TOKEN="test-token"
export FLEXERA_REGION="nam"

# Validate config loading
go run ./cmd/pulumicost-flexera --version
```

### File Organization Patterns

#### Package Structure
- `internal/flexera/`: Core Flexera API integration
  - `config.go`: Configuration loading and validation
  - `client.go`: HTTP client and API calls
  - `billing_center.go`: Tag-to-billing-center mapping logic
- `internal/server/`: gRPC server implementation
- `cmd/pulumicost-flexera/`: Main application entry point

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