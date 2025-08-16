package server

import (
	"context"
	"testing"
	"time"

	"github.com/rshade/pulumi-plugin-flexera/internal/flexera"
)

func TestFlexeraServerName(t *testing.T) {
	// Create a mock client (we don't need it for this test)
	cfg := flexera.Config{
		BaseURL:  "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:    "test-org",
		APIToken: "test-token",
		Timeout:  30 * time.Second,
	}
	
	client, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	server := NewFlexeraServer(client)
	
	result, err := server.Name(context.Background(), &Empty{})
	if err != nil {
		t.Fatalf("Name() failed: %v", err)
	}
	
	if result.Name != "flexera-optima" {
		t.Errorf("Expected name 'flexera-optima', got '%s'", result.Name)
	}
}

func TestFlexeraServerSupports(t *testing.T) {
	// Create a mock client
	cfg := flexera.Config{
		BaseURL:  "https://api.optima.flexeraeng.com/bill-analysis",
		OrgID:    "test-org",
		APIToken: "test-token",
		Timeout:  30 * time.Second,
	}
	
	client, err := flexera.NewClient(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	server := NewFlexeraServer(client)
	
	tests := []struct {
		resourceType string
		expected     bool
	}{
		{"aws-ec2", true},
		{"aws-s3", true},
		{"azure-vm", true},
		{"azure-storage", true},
		{"gcp-compute", true},
		{"gcp-storage", true},
		{"cloud-account", true},
		{"cloud-service", true},
		{"cloud-region", true},
		{"billing-center", true},
		{"resource-group", true},
		{"k8s-pod", false},        // Kubernetes resources not supported
		{"docker-container", false}, // Docker resources not supported
		{"unknown-type", false},   // Unknown type not supported
	}
	
	for _, test := range tests {
		result, err := server.Supports(context.Background(), &ResourceDescriptor{ResourceType: test.resourceType})
		if err != nil {
			t.Fatalf("Supports() failed for %s: %v", test.resourceType, err)
		}
		
		if result.Supported != test.expected {
			t.Errorf("For resource type '%s', expected supported=%t, got %t", test.resourceType, test.expected, result.Supported)
		}
	}
}

func TestParseResourceType(t *testing.T) {
	tests := []struct {
		resourceType     string
		expectedProvider string
		expectedService  string
		expectedRegion   string
	}{
		{
			"aws-ec2",
			"AWS",
			"ec2",
			"global",
		},
		{
			"azure-vm",
			"Azure",
			"vm",
			"global",
		},
		{
			"gcp-compute-engine",
			"GCP",
			"compute-engine",
			"global",
		},
		{
			"unknown-service",
			"unknown",
			"service",
			"global",
		},
		{
			"single",
			"unknown",
			"",
			"global",
		},
	}
	
	for _, test := range tests {
		provider, service, region := parseResourceType(test.resourceType)
		
		if provider != test.expectedProvider {
			t.Errorf("For resourceType '%s', expected provider '%s', got '%s'", test.resourceType, test.expectedProvider, provider)
		}
		if service != test.expectedService {
			t.Errorf("For resourceType '%s', expected service '%s', got '%s'", test.resourceType, test.expectedService, service)
		}
		if region != test.expectedRegion {
			t.Errorf("For resourceType '%s', expected region '%s', got '%s'", test.resourceType, test.expectedRegion, region)
		}
	}
}

func TestMapResourceDescriptorToID(t *testing.T) {
	tests := []struct {
		resourceType string
		expected     string
	}{
		{
			"aws-ec2",
			"service/aws/ec2",
		},
		{
			"azure-storage",
			"service/azure/storage",
		},
		{
			"gcp-compute-engine",
			"service/gcp/compute-engine",
		},
		{
			"cloud-account",
			"vendor_account/*",
		},
		{
			"unknown-type",
			"",
		},
	}
	
	for _, test := range tests {
		result := mapResourceDescriptorToID(&ResourceDescriptor{ResourceType: test.resourceType})
		
		if result != test.expected {
			t.Errorf("For resourceType '%s', expected ID '%s', got '%s'", test.resourceType, test.expected, result)
		}
	}
}