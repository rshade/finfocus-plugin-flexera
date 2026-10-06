package server

import "strings"

const (
	vendorAWS           = "Amazon Web Services"
	vendorAzure         = "Microsoft Azure"
	vendorGoogle        = "Google"
	serviceEC2          = "AmazonEC2"
	serviceS3           = "AmazonS3"
	serviceRDS          = "AmazonRDS"
	serviceAzureVM      = "Microsoft.Compute"
	serviceAzureStorage = "Microsoft.Storage"
	serviceGCPCompute   = "Compute Engine"
	serviceGCPStorage   = "Cloud Storage"
)

// flexeraBilling is one supported type and the Cloud Vendor and Service
// values Flexera stores for it.
type flexeraBilling struct {
	Literal string
	Vendor  string
	Service string
}

func billingForType(raw string) (flexeraBilling, bool) {
	literals, tokens := billingTables()
	trimmed := strings.TrimSpace(raw)
	if bill, ok := tokens[trimmed]; ok {
		return bill, true
	}
	bill, ok := literals[strings.ToLower(trimmed)]
	return bill, ok
}

func billingTables() (map[string]flexeraBilling, map[string]flexeraBilling) {
	ec2 := flexeraBilling{Literal: "aws-ec2", Vendor: vendorAWS, Service: serviceEC2}
	s3 := flexeraBilling{Literal: "aws-s3", Vendor: vendorAWS, Service: serviceS3}
	rds := flexeraBilling{Literal: "aws-rds", Vendor: vendorAWS, Service: serviceRDS}
	vm := flexeraBilling{Literal: "azure-vm", Vendor: vendorAzure, Service: serviceAzureVM}
	storage := flexeraBilling{Literal: "azure-storage", Vendor: vendorAzure, Service: serviceAzureStorage}
	compute := flexeraBilling{Literal: "gcp-compute", Vendor: vendorGoogle, Service: serviceGCPCompute}
	bucket := flexeraBilling{Literal: "gcp-storage", Vendor: vendorGoogle, Service: serviceGCPStorage}
	literals := map[string]flexeraBilling{
		ec2.Literal:     ec2,
		s3.Literal:      s3,
		rds.Literal:     rds,
		vm.Literal:      vm,
		storage.Literal: storage,
		compute.Literal: compute,
		bucket.Literal:  bucket,
	}
	tokens := map[string]flexeraBilling{
		"aws:ec2/instance:Instance":                                 ec2,
		"aws-native:ec2:Instance":                                   ec2,
		"aws:s3/bucket:Bucket":                                      s3,
		"aws:s3/bucketV2:BucketV2":                                  s3,
		"aws-native:s3:Bucket":                                      s3,
		"aws:rds/instance:Instance":                                 rds,
		"aws-native:rds:DbInstance":                                 rds,
		"azure-native:compute:VirtualMachine":                       vm,
		"azure:compute/virtualMachine:VirtualMachine":               vm,
		"azure:compute/linuxVirtualMachine:LinuxVirtualMachine":     vm,
		"azure:compute/windowsVirtualMachine:WindowsVirtualMachine": vm,
		"azure-native:storage:StorageAccount":                       storage,
		"gcp:compute/instance:Instance":                             compute,
		"google-native:compute/v1:Instance":                         compute,
		"gcp:storage/bucket:Bucket":                                 bucket,
		"google-native:storage/v1:Bucket":                           bucket,
	}
	return literals, tokens
}
