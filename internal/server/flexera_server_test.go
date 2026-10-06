package server

import (
	"context"
	"testing"
	"time"

	"strings"

	"github.com/rshade/finfocus-plugin-flexera/internal/flexera"
	"github.com/rshade/finfocus-plugin-flexera/pkg/version"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
)

func newTestServer(t *testing.T, region string) *FlexeraServer {
	t.Helper()
	if region == "" {
		region = "nam"
	}
	cfg := flexera.Config{
		BaseURL:  "https://example.invalid/bill-analysis",
		OrgID:    "test-org",
		APIToken: "test-token",
		Region:   region,
		Timeout:  30 * time.Second,
	}
	client, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return NewFlexeraServer(client)
}

func TestRegisterService(t *testing.T) {
	gs := grpc.NewServer()
	newTestServer(t, "nam").RegisterService(gs)
	if _, ok := gs.GetServiceInfo()["finfocus.v1.CostSourceService"]; !ok {
		t.Fatalf("CostSourceService was not registered: %v", gs.GetServiceInfo())
	}
}

func TestFlexeraServerName(t *testing.T) {
	result, err := newTestServer(t, "nam").Name(context.Background(), &pbc.NameRequest{})
	if err != nil {
		t.Fatalf("Name() failed: %v", err)
	}
	if result.GetName() != "flexera" {
		t.Errorf("Expected name 'flexera', got '%s'", result.GetName())
	}
}

func TestGetPluginInfo(t *testing.T) {
	result, err := newTestServer(t, "nam").GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo() failed: %v", err)
	}
	if result.GetName() != "flexera" {
		t.Errorf("name = %q", result.GetName())
	}
	wantVersion := version.Version
	if wantVersion != "" && wantVersion[0] != 'v' {
		wantVersion = "v" + wantVersion
	}
	if result.GetVersion() != wantVersion {
		t.Errorf("version = %q, want %q", result.GetVersion(), wantVersion)
	}
	if result.GetSpecVersion() != pluginsdk.SpecVersion {
		t.Errorf("spec_version = %q, want %q", result.GetSpecVersion(), pluginsdk.SpecVersion)
	}
	if got := strings.Join(result.GetProviders(), ","); got != "aws,azure,gcp" {
		t.Errorf("providers = %q", got)
	}
	gotCaps := map[pbc.PluginCapability]bool{}
	for _, cap := range result.GetCapabilities() {
		gotCaps[cap] = true
	}
	for _, want := range []pbc.PluginCapability{
		pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_PRICING_SPEC,
	} {
		if !gotCaps[want] {
			t.Errorf("missing capability %s", want)
		}
	}
	if result.GetMetadata()["implemented_rpcs"] == "" {
		t.Error("metadata implemented_rpcs is empty")
	}
	if result.GetMetadata()["flexera_regions"] != "nam,eu,apac" {
		t.Errorf("flexera_regions = %q", result.GetMetadata()["flexera_regions"])
	}
}

func TestFlexeraServerSupports(t *testing.T) {
	srv := newTestServer(t, "nam")
	tests := []struct {
		name      string
		resource  *pbc.ResourceDescriptor
		supported bool
	}{
		{
			name:      "aws-ec2",
			resource:  &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws-ec2", Region: "us-east-1"},
			supported: true,
		},
		{name: "aws-s3", resource: &pbc.ResourceDescriptor{ResourceType: "aws-s3"}, supported: true},
		{name: "aws-rds", resource: &pbc.ResourceDescriptor{ResourceType: "aws-rds"}, supported: true},
		{
			name:      "azure-vm",
			resource:  &pbc.ResourceDescriptor{Provider: "azure", ResourceType: "azure-vm"},
			supported: true,
		},
		{name: "azure-storage", resource: &pbc.ResourceDescriptor{ResourceType: "azure-storage"}, supported: true},
		{
			name:      "gcp-compute",
			resource:  &pbc.ResourceDescriptor{Provider: "gcp", ResourceType: "gcp-compute", Region: "us-central1"},
			supported: true,
		},
		{name: "gcp-storage", resource: &pbc.ResourceDescriptor{ResourceType: "gcp-storage"}, supported: true},
		{
			name:      "configured flexera zone",
			resource:  &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "nam"},
			supported: true,
		},
		{
			name:      "flexera zone alias",
			resource:  &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "north-america"},
			supported: true,
		},
		{
			name:      "flexera zone mismatch",
			resource:  &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "eu"},
			supported: false,
		},
		{
			name:      "provider mismatch",
			resource:  &pbc.ResourceDescriptor{Provider: "azure", ResourceType: "aws-ec2"},
			supported: false,
		},
		{name: "k8s-pod", resource: &pbc.ResourceDescriptor{ResourceType: "k8s-pod"}, supported: false},
		{
			name:      "docker-container",
			resource:  &pbc.ResourceDescriptor{ResourceType: "docker-container"},
			supported: false,
		},
		{name: "unknown-type", resource: &pbc.ResourceDescriptor{ResourceType: "unknown-type"}, supported: false},
		{name: "cloud-account", resource: &pbc.ResourceDescriptor{ResourceType: "cloud-account"}, supported: false},
		{name: "missing resource", resource: nil, supported: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var req *pbc.SupportsRequest
			if test.resource != nil || test.name != "missing resource" {
				req = &pbc.SupportsRequest{Resource: test.resource}
			}
			if test.name == "missing resource" {
				req = &pbc.SupportsRequest{}
			}
			result, err := srv.Supports(context.Background(), req)
			if err != nil {
				t.Fatalf("Supports() failed: %v", err)
			}
			if result.GetSupported() != test.supported {
				t.Errorf(
					"supported = %t, want %t (reason %q)",
					result.GetSupported(),
					test.supported,
					result.GetReason(),
				)
			}
			if !test.supported && result.GetReason() == "" {
				t.Error("expected a reason for an unsupported resource")
			}
		})
	}
}

func TestSupportsNilRequest(t *testing.T) {
	result, err := newTestServer(t, "eu").Supports(context.Background(), nil)
	if err != nil {
		t.Fatalf("Supports(nil) error: %v", err)
	}
	if result.GetSupported() {
		t.Fatal("nil request should be unsupported")
	}
}

func TestParseResourceType(t *testing.T) {
	tests := []struct {
		resourceType     string
		expectedProvider string
		expectedService  string
		expectedRegion   string
	}{
		{"aws-ec2", "AWS", "ec2", "global"},
		{"azure-vm", "Azure", "vm", "global"},
		{"gcp-compute-engine", "GCP", "compute-engine", "global"},
		{"unknown-service", "unknown", "service", "global"},
		{"single", "unknown", "", "global"},
	}
	for _, test := range tests {
		provider, service, region := parseResourceType(test.resourceType)
		if provider != test.expectedProvider {
			t.Errorf(
				"For resourceType '%s', expected provider '%s', got '%s'",
				test.resourceType,
				test.expectedProvider,
				provider,
			)
		}
		if service != test.expectedService {
			t.Errorf(
				"For resourceType '%s', expected service '%s', got '%s'",
				test.resourceType,
				test.expectedService,
				service,
			)
		}
		if region != test.expectedRegion {
			t.Errorf(
				"For resourceType '%s', expected region '%s', got '%s'",
				test.resourceType,
				test.expectedRegion,
				region,
			)
		}
	}
}

func TestMapResourceDescriptorToID(t *testing.T) {
	tests := []struct {
		resourceType string
		expected     string
	}{
		{"aws-ec2", "service/Amazon Web Services/AmazonEC2"},
		{"azure-storage", "service/Microsoft Azure/Microsoft.Storage"},
		{"gcp-compute-engine", "service/gcp/compute-engine"},
		{"cloud-account", "vendor_account/*"},
		{"unknown-type", ""},
	}
	for _, test := range tests {
		result := mapResourceDescriptorToID(&pbc.ResourceDescriptor{ResourceType: test.resourceType})
		if result != test.expected {
			t.Errorf("For resourceType '%s', expected ID '%s', got '%s'", test.resourceType, test.expected, result)
		}
	}
}

func TestGetPricingSpec(t *testing.T) {
	resp, err := newTestServer(t, "nam").GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{
		Resource: &pbc.ResourceDescriptor{ResourceType: "aws-ec2", Region: "us-east-1"},
	})
	if err != nil {
		t.Fatalf("GetPricingSpec: %v", err)
	}
	spec := resp.GetSpec()
	if spec.GetProvider() != "AWS" || spec.GetResourceType() != "aws-ec2" || spec.GetRegion() != "us-east-1" {
		t.Fatalf("spec = %+v", spec)
	}
	if spec.GetSource() != "flexera" {
		t.Errorf("source = %q", spec.GetSource())
	}
}
