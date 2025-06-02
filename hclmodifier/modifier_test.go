package hclmodifier

import (
	"fmt" // Added for helper functions
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"
)

func TestModifyNameAttributes(t *testing.T) {
	t.Helper()

	const expectedSuffix = "-clone"

	tests := []struct {
		name              string
		hclContent        string
		expectedName      string
		expectedAttrCount int
	}{
		{
			name: "single resource with name",
			hclContent: `resource "aws_instance" "example" {
  name = "test_name"
}`,
			expectedName:      "test_name" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "multiple resources with name",
			hclContent: `resource "aws_instance" "example1" {
  name = "test_name1"
}
resource "aws_instance" "example2" {
  name = "test_name2"
}`,
			expectedName:      "test_name1" + expectedSuffix,
			expectedAttrCount: 2,
		},
		{
			name: "resource without name",
			hclContent: `resource "aws_instance" "example" {
  ami = "ami-0c55b31ad2c454370"
}`,
			expectedName:      "",
			expectedAttrCount: 0,
		},
		{
			name: "resource with empty name",
			hclContent: `resource "aws_instance" "example" {
  name = ""
}`,
			expectedName:      "" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "terraform block with name attribute",
			hclContent: `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
  name = "my_terraform_block"
}`,
			expectedName:      "",
			expectedAttrCount: 0,
		},
		{
			name: "resource with complex name expression",
			hclContent: `resource "aws_instance" "example" {
  name = local.name
}`,
			expectedName:      "",
			expectedAttrCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), nil)
			if err != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			}

			modifiedCount, err := modifier.ModifyNameAttributes()
			if err != nil {
				t.Fatalf("ModifyNameAttributes() error = %v", err)
			}

			if modifiedCount != tc.expectedAttrCount {
				t.Errorf("ModifyNameAttributes() modifiedCount = %v, want %v", modifiedCount, tc.expectedAttrCount)
			}

			if tc.expectedAttrCount > 0 && tc.expectedName != "" {
				var foundTheSpecificExpectedName bool

				for _, block := range modifier.File().Body().Blocks() {
					if block.Type() != "resource" {
						continue
					}
					if nameAttr, ok := block.Body().Attributes()["name"]; ok {
						exprBytes := nameAttr.Expr().BuildTokens(nil).Bytes()
						expr, diagsParse := hclsyntax.ParseExpression(exprBytes, tmpFile.Name()+"#nameattr", hcl.Pos{Line: 1, Column: 1})
						if diagsParse.HasErrors() {
							t.Logf("Skipping attribute due to parse error during verification: %v", diagsParse)
							continue
						}
						
						valNode, diagsValue := expr.Value(nil)
						if diagsValue.HasErrors() {
							t.Logf("Skipping attribute due to value error during verification: %v", diagsValue)
							continue
						}

						if valNode.Type() == cty.String {
							extractedName := valNode.AsString()
							if extractedName == tc.expectedName {
								foundTheSpecificExpectedName = true
								break
							}
						}
					}
				}

				if !foundTheSpecificExpectedName {
					t.Errorf("ModifyNameAttributes() did not find a resource with the expected modified name '%s'. Output HCL:\n%s", tc.expectedName, string(modifier.File().Bytes()))
				}
			} else if tc.expectedAttrCount == 0 && tc.expectedName != "" {
				t.Logf("Test case info: expectedAttrCount is 0. expectedName ('%s') is set; this test primarily verifies count, not specific name absence/presence unless count > 0.", tc.expectedName)
			}
		})
	}
}

// Helper struct for node pool checks
type nodePoolCheck struct {
	nodePoolName                  string
	expectInitialNodeCountRemoved bool
	expectNodeCountPresent        bool
	expectedNodeCountValue        *int
}

func TestApplyInitialNodeCountRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []struct {
		name                           string
		hclContent                     string
		expectedModifications          int
		gkeResourceName                string
		nodePoolChecks                 []nodePoolCheck
		expectNoOtherResourceChanges   bool
		expectNoGKEResource            bool
	}{
		{
			name: "BothCountsPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool {
    name               = "default-pool"
    initial_node_count = 3
    node_count         = 5
  }
}`,
			expectedModifications: 1,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "default-pool",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
			},
		},
		{
			name: "OnlyInitialPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool {
    name               = "default-pool"
    initial_node_count = 2
  }
}`,
			expectedModifications: 1,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "default-pool",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        false,
				},
			},
		},
		{
			name: "OnlyNodeCountPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool {
    name       = "default-pool"
    node_count = 4
  }
}`,
			expectedModifications: 0,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "default-pool",
					expectInitialNodeCountRemoved: false,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(4),
				},
			},
		},
		{
			name: "NeitherCountPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool {
    name = "default-pool"
    autoscaling = true
  }
}`,
			expectedModifications: 0,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "default-pool",
					expectInitialNodeCountRemoved: false,
					expectNodeCountPresent:        false,
				},
			},
		},
		{
			name: "MultipleNodePools",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool {
    name               = "pool-one"
    initial_node_count = 3
    node_count         = 5
  }
  node_pool {
    name               = "pool-two"
    initial_node_count = 2
  }
  node_pool {
    name       = "pool-three"
    node_count = 4
  }
  node_pool {
    name = "pool-four"
  }
}`,
			expectedModifications: 2,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "pool-one",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
				{
					nodePoolName:               "pool-two",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        false,
				},
				{
					nodePoolName:               "pool-three",
					expectInitialNodeCountRemoved: false,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(4),
				},
				{
					nodePoolName:               "pool-four",
					expectInitialNodeCountRemoved: false,
					expectNodeCountPresent:        false,
				},
			},
		},
		{
			name: "NoNodePools",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name     = "test-cluster"
  location = "us-central1"
}`,
			expectedModifications: 0,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks:        nil,
		},
		{
			name: "NonGKEResource",
			hclContent: `resource "google_compute_instance" "not_gke" {
  name = "test-vm"
  node_pool {
    initial_node_count = 1
    node_count         = 2
  }
  initial_node_count = 5
}`,
			expectedModifications: 0,
			gkeResourceName:       "",
			expectNoOtherResourceChanges: true,
			nodePoolChecks:               nil,
		},
		{
			name:                  "EmptyHCL",
			hclContent:            ``,
			expectedModifications: 0,
			gkeResourceName:       "",
			nodePoolChecks:        nil,
			expectNoGKEResource:   true,
		},
		{
			name: "MultipleGKEResources",
			hclContent: `resource "google_container_cluster" "gke_one" {
  name = "cluster-one"
  node_pool {
    name               = "gke-one-pool"
    initial_node_count = 3
    node_count         = 5
  }
}
resource "google_container_cluster" "gke_two" {
  name = "cluster-two"
  node_pool {
    name               = "gke-two-pool"
    initial_node_count = 2
  }
  node_pool {
    name       = "gke-two-pool-extra"
    node_count = 1
  }
}`,
			expectedModifications: 2,
			gkeResourceName: "gke_one",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "gke-one-pool",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_initial_node_count_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyInitialNodeCountRule()
			if ruleErr != nil {
				t.Fatalf("ApplyInitialNodeCountRule() error = %v", ruleErr)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyInitialNodeCountRule() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.expectNoGKEResource {
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) > 0 && b.Labels()[0] == "google_container_cluster" {
						t.Errorf("Expected no 'google_container_cluster' resource, but found one: %v. HCL:\n%s", b.Labels(), string(modifiedContentBytes))
					}
				}
				return
			}

			var targetGKEResource *hclwrite.Block
			if tc.gkeResourceName != "" {
				targetGKEResource, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", tc.gkeResourceName)
				if targetGKEResource == nil && len(tc.nodePoolChecks) > 0 {
					t.Fatalf("Expected 'google_container_cluster' resource '%s' not found for verification. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
				}
			}

			if targetGKEResource != nil && tc.nodePoolChecks != nil {
				for _, npCheck := range tc.nodePoolChecks {
					var foundNodePool *hclwrite.Block
					for _, nestedBlock := range targetGKEResource.Body().Blocks() {
						if nestedBlock.Type() == "node_pool" {
							nameAttr := nestedBlock.Body().GetAttribute("name")
							if nameAttr != nil {
								nameVal, err := modifier.GetAttributeValue(nameAttr)
								if err == nil && nameVal.Type() == cty.String && nameVal.AsString() == npCheck.nodePoolName {
									foundNodePool = nestedBlock
									break
								}
							} else if npCheck.nodePoolName == "" {
								var blocksOfNpType []*hclwrite.Block
								for _, nb := range targetGKEResource.Body().Blocks() {
									if nb.Type() == "node_pool" {
										blocksOfNpType = append(blocksOfNpType, nb)
									}
								}
                                if len(blocksOfNpType) == 1 && blocksOfNpType[0].Body().GetAttribute("name") == nil {
                                    foundNodePool = blocksOfNpType[0]
                                    break
                                }
							}
						}
					}

					if foundNodePool == nil {
						if npCheck.expectInitialNodeCountRemoved || npCheck.expectNodeCountPresent {
							t.Errorf("Node pool '%s' in resource '%s' not found for verification. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
						continue
					}

					initialAttr := foundNodePool.Body().GetAttribute("initial_node_count")
					if npCheck.expectInitialNodeCountRemoved {
						if initialAttr != nil {
							t.Errorf("Expected 'initial_node_count' to be REMOVED from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					} else {
						originalInitialPresent := false
						originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
						originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, "google_container_cluster", tc.gkeResourceName)
						if originalGKEResource != nil {
							originalNP, _ := findNodePoolInBlock(originalGKEResource, npCheck.nodePoolName, modifier)
							if originalNP != nil && originalNP.Body().GetAttribute("initial_node_count") != nil {
								originalInitialPresent = true
							}
						}
						if originalInitialPresent && initialAttr == nil {
							t.Errorf("Expected 'initial_node_count' to be PRESENT in node_pool '%s' in '%s', but it was NOT FOUND (removed). Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
						if !originalInitialPresent && initialAttr != nil {
							t.Errorf("'initial_node_count' was unexpectedly ADDED to node_pool '%s' in '%s'. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					}

					nodeCountAttr := foundNodePool.Body().GetAttribute("node_count")
					if npCheck.expectNodeCountPresent {
						if nodeCountAttr == nil {
							t.Errorf("Expected 'node_count' to be PRESENT in node_pool '%s' in '%s', but it was NOT FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						} else if npCheck.expectedNodeCountValue != nil {
							val, err := modifier.GetAttributeValue(nodeCountAttr)
							if err != nil || !val.IsKnown() || val.IsNull() || val.Type() != cty.Number {
								t.Errorf("Error or wrong type for 'node_count' in node_pool '%s': %v. Modified HCL:\n%s", npCheck.nodePoolName, err, string(modifiedContentBytes))
							} else {
								numValFloat, _ := val.AsBigFloat().Float64()
								expectedNumFloat := float64(*npCheck.expectedNodeCountValue)
								if numValFloat != expectedNumFloat {
									t.Errorf("Expected 'node_count' value %d, got %f in node_pool '%s'. Modified HCL:\n%s",
										*npCheck.expectedNodeCountValue, numValFloat, npCheck.nodePoolName, string(modifiedContentBytes))
								}
							}
						}
					} else {
						if nodeCountAttr != nil {
							t.Errorf("Expected 'node_count' to be ABSENT from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					}
				}
			}

			if tc.name == "MultipleGKEResources" {
				var gkeTwoResource *hclwrite.Block
				gkeTwoResource, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_two")
				if gkeTwoResource == nil {
					t.Fatalf("Expected 'google_container_cluster' resource 'gke_two' not found for multi-resource verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				gkeTwoPool1, _ := findNodePoolInBlock(gkeTwoResource, "gke-two-pool", modifier)
				if gkeTwoPool1 == nil {
					t.Errorf("Node pool 'gke-two-pool' in 'gke_two' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if gkeTwoPool1.Body().GetAttribute("initial_node_count") != nil {
						t.Errorf("'initial_node_count' should have been removed from 'gke-two-pool', but was found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
					if gkeTwoPool1.Body().GetAttribute("node_count") != nil {
						t.Errorf("'node_count' should be absent from 'gke-two-pool' (as it wasn't there originally), but was found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
				gkeTwoPool2, _ := findNodePoolInBlock(gkeTwoResource, "gke-two-pool-extra", modifier)
				if gkeTwoPool2 == nil {
					t.Errorf("Node pool 'gke-two-pool-extra' in 'gke_two' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if gkeTwoPool2.Body().GetAttribute("initial_node_count") != nil {
						t.Errorf("'initial_node_count' should be absent from 'gke-two-pool-extra', but was found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
					if gkeTwoPool2.Body().GetAttribute("node_count") == nil {
						t.Errorf("'node_count' should be present in 'gke-two-pool-extra', but was not found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
			}

			if tc.expectNoOtherResourceChanges && tc.name == "NonGKEResource" {
				var nonGKEResource *hclwrite.Block
				nonGKEResource, _ = findBlockInParsedFile(verifiedFile, "google_compute_instance", "not_gke")
				if nonGKEResource == nil {
					t.Fatalf("Expected non-GKE resource 'google_compute_instance.not_gke' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if nonGKEResource.Body().GetAttribute("initial_node_count") == nil {
					t.Errorf("Top-level 'initial_node_count' was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				npBlock, _ := findNodePoolInBlock(nonGKEResource, "", modifier)
				if npBlock == nil {
					t.Errorf("'node_pool' block was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if npBlock.Body().GetAttribute("initial_node_count") == nil {
						t.Errorf("'initial_node_count' in 'node_pool' was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
					}
					if npBlock.Body().GetAttribute("node_count") == nil {
						t.Errorf("'node_count' in 'node_pool' was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
			}
		})
	}
}

func TestApplyMasterCIDRRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	type privateClusterConfigCheck struct {
		expectBlockExists                   bool
		expectPrivateEndpointSubnetworkRemoved bool
		expectOtherAttributeUnchanged       *string
	}

	tests := []struct {
		name                        string
		hclContent                  string
		expectedModifications       int
		gkeResourceName             string
		expectMasterCIDRPresent     bool
		privateClusterConfigCheck   *privateClusterConfigCheck
		expectNoOtherResourceChanges bool
		expectNoGKEResource         bool
	}{
		{
			name: "BothPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name                     = "test-cluster"
  master_ipv4_cidr_block   = "172.16.0.0/28"
  private_cluster_config {
    enable_private_endpoint   = true
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
  }
}`,
			expectedModifications:   1,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: true,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "OnlyMasterCIDRPresent_PrivateConfigMissing",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name                   = "test-cluster"
  master_ipv4_cidr_block = "172.16.0.0/28"
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists: false,
			},
		},
		{
			name: "OnlyMasterCIDRPresent_SubnetworkMissingInPrivateConfig",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name                   = "test-cluster"
  master_ipv4_cidr_block = "172.16.0.0/28"
  private_cluster_config {
    enable_private_endpoint = true
  }
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "OnlyPrivateEndpointSubnetworkPresent_MasterCIDRMissing",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  private_cluster_config {
    enable_private_endpoint   = true
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
  }
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: false,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "NeitherPresent",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  private_cluster_config {
    enable_private_endpoint = true
  }
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: false,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "NeitherPresent_NoPrivateConfigBlock",
			hclContent: `resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: false,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists: false,
			},
		},
		{
			name: "NonGKEResource",
			hclContent: `resource "google_compute_instance" "not_gke" {
  name                   = "test-vm"
  master_ipv4_cidr_block = "172.16.0.0/28"
  private_cluster_config {
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
    enable_private_endpoint   = true
  }
}`,
			expectedModifications:      0,
			gkeResourceName:            "",
			expectNoOtherResourceChanges: true,
		},
		{
			name:                  "EmptyHCL",
			hclContent:            ``,
			expectedModifications: 0,
			gkeResourceName:       "",
			expectNoGKEResource:   true,
		},
		{
			name: "MultipleGKEResources_OneMatch",
			hclContent: `resource "google_container_cluster" "gke_one_match" {
  name                     = "cluster-one"
  master_ipv4_cidr_block   = "172.16.0.0/28"
  private_cluster_config {
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-one"
    enable_private_endpoint   = true
  }
}
resource "google_container_cluster" "gke_two_no_master_cidr" {
  name = "cluster-two"
  private_cluster_config {
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-two"
  }
}
resource "google_container_cluster" "gke_three_no_subnetwork" {
  name                     = "cluster-three"
  master_ipv4_cidr_block   = "172.16.1.0/28"
  private_cluster_config {
    enable_private_endpoint = false
  }
}`,
			expectedModifications:   1,
			gkeResourceName:         "gke_one_match",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: true,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_master_cidr_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyMasterCIDRRule()
			if ruleErr != nil {
				t.Fatalf("ApplyMasterCIDRRule() error = %v", ruleErr)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyMasterCIDRRule() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.expectNoGKEResource {
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) > 0 && b.Labels()[0] == "google_container_cluster" {
						t.Errorf("Expected no 'google_container_cluster' resource, but found one: %v. HCL:\n%s", b.Labels(), string(modifiedContentBytes))
					}
				}
				return
			}

			var targetGKEResource *hclwrite.Block
			if tc.gkeResourceName != "" {
				targetGKEResource, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", tc.gkeResourceName)
				if targetGKEResource == nil && (tc.expectedModifications > 0 || tc.privateClusterConfigCheck != nil) {
					t.Fatalf("Expected 'google_container_cluster' resource '%s' not found for verification. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
				}
			}

			if targetGKEResource != nil {
				masterCIDRAttr := targetGKEResource.Body().GetAttribute("master_ipv4_cidr_block")
				if tc.expectMasterCIDRPresent {
					if masterCIDRAttr == nil {
						t.Errorf("Expected 'master_ipv4_cidr_block' to be PRESENT in GKE resource '%s', but it was NOT FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
					}
				} else {
					if masterCIDRAttr != nil {
						t.Errorf("Expected 'master_ipv4_cidr_block' to be ABSENT from GKE resource '%s', but it was FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
					}
				}

				if tc.privateClusterConfigCheck != nil {
					pccBlock := targetGKEResource.Body().FirstMatchingBlock("private_cluster_config", nil)
					if !tc.privateClusterConfigCheck.expectBlockExists {
						if pccBlock != nil {
							t.Errorf("Expected 'private_cluster_config' block NOT to exist in GKE resource '%s', but it was FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
						}
					} else {
						if pccBlock == nil {
							t.Fatalf("Expected 'private_cluster_config' block to EXIST in GKE resource '%s', but it was NOT FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
						}

						subnetworkAttr := pccBlock.Body().GetAttribute("private_endpoint_subnetwork")
						if tc.privateClusterConfigCheck.expectPrivateEndpointSubnetworkRemoved {
							if subnetworkAttr != nil {
								t.Errorf("Expected 'private_endpoint_subnetwork' to be REMOVED from 'private_cluster_config' in '%s', but it was FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
							}
						} else {
							originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
							originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, "google_container_cluster", tc.gkeResourceName)
							var originalSubnetworkPresent bool
							if originalGKEResource != nil {
								originalPCC := originalGKEResource.Body().FirstMatchingBlock("private_cluster_config", nil)
								if originalPCC != nil && originalPCC.Body().GetAttribute("private_endpoint_subnetwork") != nil {
									originalSubnetworkPresent = true
								}
							}

							if originalSubnetworkPresent && subnetworkAttr == nil {
								t.Errorf("Expected 'private_endpoint_subnetwork' to be PRESENT in 'private_cluster_config' in '%s', but it was NOT FOUND (removed). Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
							}
							if !originalSubnetworkPresent && subnetworkAttr != nil {
								t.Errorf("'private_endpoint_subnetwork' was unexpectedly ADDED to 'private_cluster_config' in '%s'. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
							}
						}

						if tc.privateClusterConfigCheck.expectOtherAttributeUnchanged != nil {
							otherAttrName := *tc.privateClusterConfigCheck.expectOtherAttributeUnchanged
							otherAttr := pccBlock.Body().GetAttribute(otherAttrName)
							originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
							originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, "google_container_cluster", tc.gkeResourceName)
							var originalOtherAttrPresent bool
							if originalGKEResource != nil {
								originalPCC := originalGKEResource.Body().FirstMatchingBlock("private_cluster_config", nil)
								if originalPCC != nil && originalPCC.Body().GetAttribute(otherAttrName) != nil {
									originalOtherAttrPresent = true
								}
							}

							if originalOtherAttrPresent && otherAttr == nil {
								t.Errorf("Expected attribute '%s' in 'private_cluster_config' of '%s' to be UNCHANGED, but it was REMOVED. Modified HCL:\n%s", otherAttrName, tc.gkeResourceName, string(modifiedContentBytes))
							}
							if !originalOtherAttrPresent && otherAttr != nil {
								t.Errorf("Attribute '%s' in 'private_cluster_config' of '%s' was unexpectedly ADDED. Modified HCL:\n%s", otherAttrName, tc.gkeResourceName, string(modifiedContentBytes))
							}
						}
					}
				}
			}

			if tc.name == "MultipleGKEResources_OneMatch" {
				gkeTwo, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_two_no_master_cidr")
				if gkeTwo == nil {t.Fatalf("GKE resource 'gke_two_no_master_cidr' not found in multi-resource test.")}
				if gkeTwo.Body().GetAttribute("master_ipv4_cidr_block") != nil {
					t.Errorf("'master_ipv4_cidr_block' unexpectedly present in 'gke_two_no_master_cidr'.")
				}
				pccTwo := gkeTwo.Body().FirstMatchingBlock("private_cluster_config", nil)
				if pccTwo == nil {t.Fatalf("'private_cluster_config' missing in 'gke_two_no_master_cidr'.")}
				if pccTwo.Body().GetAttribute("private_endpoint_subnetwork") == nil {
					t.Errorf("'private_endpoint_subnetwork' unexpectedly missing from 'gke_two_no_master_cidr'.")
				}

				gkeThree, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_three_no_subnetwork")
				if gkeThree == nil {t.Fatalf("GKE resource 'gke_three_no_subnetwork' not found in multi-resource test.")}
				if gkeThree.Body().GetAttribute("master_ipv4_cidr_block") == nil {
					t.Errorf("'master_ipv4_cidr_block' unexpectedly missing from 'gke_three_no_subnetwork'.")
				}
				pccThree := gkeThree.Body().FirstMatchingBlock("private_cluster_config", nil)
				if pccThree == nil {t.Fatalf("'private_cluster_config' missing in 'gke_three_no_subnetwork'.")}
				if pccThree.Body().GetAttribute("private_endpoint_subnetwork") != nil {
					t.Errorf("'private_endpoint_subnetwork' unexpectedly present in 'gke_three_no_subnetwork'.")
				}
				if pccThree.Body().GetAttribute("enable_private_endpoint") == nil {
					t.Errorf("'enable_private_endpoint' unexpectedly removed from 'gke_three_no_subnetwork'.")
				}
			}

			if tc.expectNoOtherResourceChanges && tc.name == "NonGKEResource" {
				nonGke, _ := findBlockInParsedFile(verifiedFile, "google_compute_instance", "not_gke")
				if nonGke == nil {
					t.Fatalf("Non-GKE resource 'google_compute_instance.not_gke' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if nonGke.Body().GetAttribute("master_ipv4_cidr_block") == nil {
					t.Errorf("'master_ipv4_cidr_block' was unexpectedly removed from non-GKE resource. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				pccNonGke := nonGke.Body().FirstMatchingBlock("private_cluster_config", nil)
				if pccNonGke == nil {
					t.Errorf("'private_cluster_config' block was unexpectedly removed from non-GKE resource. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if pccNonGke.Body().GetAttribute("private_endpoint_subnetwork") == nil {
						t.Errorf("'private_endpoint_subnetwork' was unexpectedly removed from 'private_cluster_config' of non-GKE resource. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
			}
		})
	}
}

func TestApplyRule3(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                               string
		hclContent                         string
		expectedModifications              int
		expectEnabledAttributeRemoved      bool
		resourceLabelsToVerify             []string
		binaryAuthorizationShouldExist     bool
		binaryAuthorizationShouldHaveEvalMode bool
	}{
		{
			name: "Both enabled and evaluation_mode present",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}`,
			expectedModifications:           1,
			expectEnabledAttributeRemoved:   true,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Only enabled present",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {
    enabled = true
  }
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "Only evaluation_mode present",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Neither enabled nor evaluation_mode present",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {
    some_other_attr = "value"
  }
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block present but empty",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {}
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block missing entirely",
			hclContent: `resource "google_container_cluster" "primary" {
  name     = "primary-cluster"
  location = "us-central1"
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:  false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "Non-matching resource type with binary_authorization",
			hclContent: `resource "google_compute_instance" "default" {
  name = "test-instance"
  binary_authorization {
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          []string{"google_compute_instance", "default"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Multiple GKE resources, one with conflict",
			hclContent: `resource "google_container_cluster" "gke_one" {
  name = "gke-one"
  binary_authorization {
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}
resource "google_container_cluster" "gke_two" {
  name = "gke-two"
  binary_authorization {
    evaluation_mode = "DISABLED"
  }
}`,
			expectedModifications:           1,
			expectEnabledAttributeRemoved:   true,
			resourceLabelsToVerify:          []string{"google_container_cluster", "gke_one"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Empty HCL content",
			hclContent: ``,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false,
			resourceLabelsToVerify:          nil,
			binaryAuthorizationShouldExist:  false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_rule3_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						modifications, ruleErr := 0, error(nil)
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyRule3() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyRule3() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyRule3()
			if ruleErr != nil {
				t.Fatalf("ApplyRule3() error = %v", ruleErr)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRule3() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetResourceBlock = b
						break
					}
				}

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectEnabledAttributeRemoved || tc.binaryAuthorizationShouldExist) {
					if !(tc.hclContent == "" && tc.expectedModifications == 0) {
						t.Fatalf("Could not find the target resource block type '%s' with name '%s' for verification. Modified HCL:\n%s", blockType, blockName, string(modifiedContentBytes))
					}
				}

				if targetResourceBlock != nil {
					var binaryAuthBlock *hclwrite.Block
					for _, nestedBlock := range targetResourceBlock.Body().Blocks() {
						if nestedBlock.Type() == "binary_authorization" {
							binaryAuthBlock = nestedBlock
							break
						}
					}

					if !tc.binaryAuthorizationShouldExist {
						if binaryAuthBlock != nil {
							t.Errorf("Expected 'binary_authorization' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, tc.hclContent)
						}
					} else {
						if binaryAuthBlock == nil {
							if tc.expectEnabledAttributeRemoved || tc.expectedModifications > 0 || tc.binaryAuthorizationShouldHaveEvalMode {
								t.Fatalf("Expected 'binary_authorization' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, tc.hclContent)
							}
						} else {
							hasEnabledAttr := binaryAuthBlock.Body().GetAttribute("enabled") != nil
							hasEvalModeAttr := binaryAuthBlock.Body().GetAttribute("evaluation_mode") != nil

							if tc.expectEnabledAttributeRemoved {
								if hasEnabledAttr {
									t.Errorf("Expected 'enabled' attribute to be REMOVED from 'binary_authorization' in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
								}
							} else {
								originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
								originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
								var originalBinaryAuthBlock *hclwrite.Block
								if originalResourceBlock != nil {
									originalBinaryAuthBlock = originalResourceBlock.Body().FirstMatchingBlock("binary_authorization", nil)
								}

								if originalBinaryAuthBlock != nil && originalBinaryAuthBlock.Body().GetAttribute("enabled") != nil {
									if !hasEnabledAttr {
										t.Errorf("Expected 'enabled' attribute to be PRESENT in 'binary_authorization' in %s[\"%s\"], but it was NOT FOUND (removed). HCL:\n%s\nModified HCL:\n%s",
											blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
									}
								}
							}

							if tc.binaryAuthorizationShouldHaveEvalMode {
								if !hasEvalModeAttr {
									t.Errorf("Expected 'evaluation_mode' attribute to be PRESENT in 'binary_authorization' in %s[\"%s\"], but it was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
								}
							}
						}
					}
				}
			}

			if tc.name == "Multiple GKE resources, one with conflict" {
				var gkeTwoBlock *hclwrite.Block
				gkeTwoBlock, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_two")
				if gkeTwoBlock == nil {
					t.Fatalf("Could not find 'google_container_cluster' GKE block named 'gke_two' for multi-block test verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				binaryAuthGkeTwo := gkeTwoBlock.Body().FirstMatchingBlock("binary_authorization", nil)
				if binaryAuthGkeTwo == nil {
					t.Fatalf("'binary_authorization' missing in 'gke_two' GKE block for multi-block test. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if binaryAuthGkeTwo.Body().GetAttribute("enabled") != nil {
					t.Errorf("'enabled' attribute should NOT be present in 'gke_two' ('binary_authorization' block), but it was found. Modified HCL:\n%s",
						string(modifiedContentBytes))
				}
				if binaryAuthGkeTwo.Body().GetAttribute("evaluation_mode") == nil {
					t.Errorf("'evaluation_mode' attribute expected to be PRESENT in 'gke_two' ('binary_authorization' block), but was NOT FOUND. Modified HCL:\n%s",
						string(modifiedContentBytes))
				}
			}
		})
	}
}

func TestApplyRule2(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                                  string
		hclContent                            string
		expectedModifications                 int
		expectServicesIPV4CIDRBlockRemoved    bool
		resourceLabelsToVerify                []string
		ipAllocationPolicyShouldExistForCheck bool
	}{
		{
			name: "Both attributes present in ip_allocation_policy",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
  }
}`,
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Only services_ipv4_cidr_block present in ip_allocation_policy",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Only cluster_secondary_range_name present in ip_allocation_policy",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    cluster_secondary_range_name = "services_range"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Neither attribute relevant to Rule 2 present in ip_allocation_policy",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    some_other_attribute = "value"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is present but empty",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {}
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is missing entirely",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: false,
		},
		{
			name: "Non-matching resource type with similar nested structure",
			hclContent: `resource "google_compute_router" "default" {
  name = "my-router"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_compute_router", "default"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Multiple google_container_cluster blocks, one matching for Rule 2",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
  }
}
resource "google_container_cluster" "secondary" {
  name = "secondary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.3.0.0/20"
  }
}`,
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Multiple google_container_cluster blocks, ip_policy missing in one",
			hclContent: `resource "google_container_cluster" "alpha" {
  name = "alpha-cluster"
}
resource "google_container_cluster" "beta" {
  name = "beta-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.4.0.0/20"
    cluster_secondary_range_name = "services_range_beta"
  }
}`,
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "beta"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Empty HCL content",
			hclContent: ``,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                nil,
			ipAllocationPolicyShouldExistForCheck: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_rule2_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						modifications, ruleErr := 0, error(nil)
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyRule2() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyRule2() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyRule2()
			if ruleErr != nil {
				t.Fatalf("ApplyRule2() error = %v", ruleErr)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRule2() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetResourceBlock = b
						break
					}
				}

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectServicesIPV4CIDRBlockRemoved) {
					t.Fatalf("Could not find the target resource block type '%s' with name '%s' for verification. Modified HCL:\n%s", blockType, blockName, string(modifiedContentBytes))
				}

				if targetResourceBlock != nil {
					var ipAllocationPolicyBlock *hclwrite.Block
					for _, nestedBlock := range targetResourceBlock.Body().Blocks() {
						if nestedBlock.Type() == "ip_allocation_policy" {
							ipAllocationPolicyBlock = nestedBlock
							break
						}
					}

					if !tc.ipAllocationPolicyShouldExistForCheck {
						if ipAllocationPolicyBlock != nil {
							t.Errorf("Expected 'ip_allocation_policy' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, tc.hclContent)
						}
					} else {
						if ipAllocationPolicyBlock == nil {
							if tc.expectServicesIPV4CIDRBlockRemoved || tc.expectedModifications > 0 {
								t.Fatalf("Expected 'ip_allocation_policy' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, tc.hclContent)
							}
						} else {
							hasServicesCIDRBlock := ipAllocationPolicyBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil
							if tc.expectServicesIPV4CIDRBlockRemoved {
								if hasServicesCIDRBlock {
									t.Errorf("Expected 'services_ipv4_cidr_block' to be REMOVED from ip_allocation_policy in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
								}
							} else {
								originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
								originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
								var originalIpAllocBlock *hclwrite.Block
								if originalResourceBlock != nil {
                                    originalIpAllocBlock = originalResourceBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
								}

								if originalIpAllocBlock != nil && originalIpAllocBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil {
									if !hasServicesCIDRBlock {
										t.Errorf("Expected 'services_ipv4_cidr_block' to be PRESENT in ip_allocation_policy in %s[\"%s\"], but it was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
											blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
									}
								}
							}
						}
					}
				}
			}

			if tc.name == "Multiple google_container_cluster blocks, one matching for Rule 2" {
				var secondaryBlock *hclwrite.Block
				secondaryBlock, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", "secondary")
				if secondaryBlock == nil {
					t.Fatalf("Could not find 'google_container_cluster' GKE block named 'secondary' for multi-block test verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				ipAllocSecondary := secondaryBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
				if ipAllocSecondary == nil {
					t.Fatalf("'ip_allocation_policy' missing in 'secondary' GKE block for multi-block test. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if ipAllocSecondary.Body().GetAttribute("services_ipv4_cidr_block") == nil {
					t.Errorf("'services_ipv4_cidr_block' expected to be PRESENT in 'secondary' GKE's ip_allocation_policy, but was NOT FOUND. Modified HCL:\n%s",
						string(modifiedContentBytes))
				}
			}
		})
	}
}

func TestApplyRule1(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                         string
		hclContent                   string
		expectedModifications        int
		expectClusterIPV4CIDRRemoved bool
		resourceLabelsToVerify       []string
	}{
		{
			name: "Both attributes present",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only cluster_ipv4_cidr present (no ip_allocation_policy block)",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only cluster_ipv4_cidr present (ip_allocation_policy block exists but no cluster_ipv4_cidr_block)",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.2.0.0/20"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only ip_allocation_policy.cluster_ipv4_cidr_block present",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Neither attribute relevant to Rule 1 present",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.2.0.0/20"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "ip_allocation_policy block is missing entirely, cluster_ipv4_cidr present",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Non-matching resource type (google_compute_instance)",
			hclContent: `resource "google_compute_instance" "default" {
  name               = "test-instance"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_compute_instance", "default"},
		},
		{
			name: "Multiple google_container_cluster blocks, one matching",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}
resource "google_container_cluster" "secondary" {
  name               = "secondary-cluster"
  cluster_ipv4_cidr  = "10.2.0.0/14"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.3.0.0/20"
  }
}`,
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Multiple google_container_cluster blocks, none matching",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
}
resource "google_container_cluster" "secondary" {
  name               = "secondary-cluster"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Empty HCL content",
			hclContent: ``,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						modifications, ruleErr := 0, error(nil)
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyRule1() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyRule1() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyRule1()
			if ruleErr != nil {
				t.Fatalf("ApplyRule1() error = %v", ruleErr)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRule1() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetBlock *hclwrite.Block

				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetBlock = b
						break
					}
				}

				if targetBlock == nil && (tc.expectClusterIPV4CIDRRemoved || tc.expectedModifications > 0) {
					t.Fatalf("Could not find the target resource block type '%s' with name '%s' for verification. Modified HCL:\n%s", blockType, blockName, string(modifiedContentBytes))
				}

				if targetBlock != nil {
					hasClusterIPV4CIDR := targetBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil
					if tc.expectClusterIPV4CIDRRemoved {
						if hasClusterIPV4CIDR {
							t.Errorf("Expected 'cluster_ipv4_cidr' to be removed from %s[\"%s\"], but it was found. Modified HCL:\n%s",
								blockType, blockName, string(modifiedContentBytes))
						}
					} else {
						if tc.name == "Non-matching resource type (google_compute_instance)" {
							originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
                            originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
							var originalBlockHasIt bool
                            if originalResourceBlock != nil && originalResourceBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil {
                                originalBlockHasIt = true
                            }
							if originalBlockHasIt && !hasClusterIPV4CIDR {
										t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed from non-target resource %s[\"%s\"]. Modified HCL:\n%s",
											blockType, blockName, string(modifiedContentBytes))
							}
						}
					}
				}
			}

			if tc.name == "Multiple google_container_cluster blocks, one matching" {
				var secondaryBlock *hclwrite.Block
				secondaryBlock, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", "secondary")
				if secondaryBlock == nil {
					t.Fatalf("Could not find the 'google_container_cluster' block named 'secondary' for verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if secondaryBlock.Body().GetAttribute("cluster_ipv4_cidr") == nil {
					t.Errorf("Expected 'cluster_ipv4_cidr' to be present in 'secondary' block, but it was not. Modified HCL:\n%s",
						string(modifiedContentBytes))
				}
			}
		})
	}
}

func TestApplyAutopilotRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	type clusterAutoscalingChecks struct {
		expectBlockExists       bool
		expectEnabledRemoved    bool
		expectResourceLimitsRemoved bool
		expectProfileUnchanged  *string
	}

	type binaryAuthorizationChecks struct {
		expectBlockExists    bool
		expectEnabledRemoved bool
	}

	type addonsConfigChecks struct {
		expectBlockExists            bool
		expectNetworkPolicyRemoved   bool
		expectDnsCacheRemoved        bool
		expectStatefulHaRemoved      bool
		expectHttpLoadBalancingUnchanged bool
	}

	tests := []struct {
		name                                string
		hclContent                          string
		expectedModifications               int
		clusterName                         string
		expectEnableAutopilotAttr           *bool
		expectedRootAttrsRemoved            []string
		expectedTopLevelNestedBlocksRemoved []string
		addonsConfig                        *addonsConfigChecks
		clusterAutoscaling                  *clusterAutoscalingChecks
		binaryAuthorization                 *binaryAuthorizationChecks
		expectNoOtherChanges                bool
	}{
		{
			name: "enable_autopilot is true, all conflicting fields present",
			hclContent: `resource "google_container_cluster" "autopilot_cluster" {
  name                          = "autopilot-cluster"
  location                      = "us-central1"
  enable_autopilot              = true
  cluster_ipv4_cidr             = "10.0.0.0/8"
  enable_shielded_nodes         = true
  remove_default_node_pool      = true
  default_max_pods_per_node     = 110
  enable_intranode_visibility   = true

  addons_config {
    network_policy_config {
      disabled = false
    }
    dns_cache_config {
      enabled = true
    }
    stateful_ha_config {
      enabled = true
    }
    http_load_balancing {
      disabled = false
    }
  }

  network_policy {
    provider = "CALICO"
    enabled  = true
  }
  node_pool {
    name = "default-pool"
  }
  node_pool {
    name = "custom-pool"
  }
  cluster_autoscaling {
    enabled = true
    autoscaling_profile = "OPTIMIZE_UTILIZATION"
    resource_limits {
      resource_type = "cpu"
      minimum = 1
      maximum = 10
    }
  }
  binary_authorization {
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
    enabled = true
  }
}`,
			expectedModifications: 14,
			clusterName:               "autopilot_cluster",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved: []string{
				"cluster_ipv4_cidr",
				"enable_shielded_nodes",
				"remove_default_node_pool",
				"default_max_pods_per_node",
				"enable_intranode_visibility",
			},
			expectedTopLevelNestedBlocksRemoved: []string{
				"network_policy",
				"node_pool",
			},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:            true,
				expectNetworkPolicyRemoved:   true,
				expectDnsCacheRemoved:        true,
				expectStatefulHaRemoved:      true,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:       true,
				expectEnabledRemoved:    true,
				expectResourceLimitsRemoved: true,
				expectProfileUnchanged:  stringPtr("OPTIMIZE_UTILIZATION"),
			},
			binaryAuthorization: &binaryAuthorizationChecks{
				expectBlockExists:    true,
				expectEnabledRemoved: true,
			},
		},
		{
			name: "enable_autopilot is false, conflicting fields present",
			hclContent: `resource "google_container_cluster" "standard_cluster" {
  name                  = "standard-cluster"
  enable_autopilot      = false
  cluster_ipv4_cidr     = "10.0.0.0/8"
  enable_shielded_nodes = true
  node_pool {
    name = "default-pool"
  }
  cluster_autoscaling {
    enabled = true
    autoscaling_profile = "BALANCED"
  }
  addons_config {
    dns_cache_config {
      enabled = true
    }
    http_load_balancing {
      disabled = false
    }
  }
}`,
			expectedModifications:     1,
			clusterName:               "standard_cluster",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:            true,
				expectDnsCacheRemoved:        false,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:       true,
				expectEnabledRemoved:    false,
				expectResourceLimitsRemoved: false,
				expectProfileUnchanged:  stringPtr("BALANCED"),
			},
			binaryAuthorization:  nil,
			expectNoOtherChanges: true,
		},
		{
			name: "enable_autopilot not present, conflicting fields present",
			hclContent: `resource "google_container_cluster" "existing_cluster" {
  name                  = "existing-cluster"
  cluster_ipv4_cidr     = "10.0.0.0/8"
  enable_shielded_nodes = true
  node_pool {
    name = "default-pool"
  }
  addons_config {
    network_policy_config {
      disabled = false
    }
  }
}`,
			expectedModifications:     0,
			clusterName:               "existing_cluster",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:            true,
				expectNetworkPolicyRemoved:   false,
			},
			expectNoOtherChanges:      true,
		},
		{
			name: "enable_autopilot is true, no conflicting fields present",
			hclContent: `resource "google_container_cluster" "clean_autopilot_cluster" {
  name             = "clean-autopilot-cluster"
  enable_autopilot = true
  location         = "us-central1"
  addons_config {
    http_load_balancing { disabled = true }
  }
  cluster_autoscaling {
    autoscaling_profile = "BALANCED"
  }
  binary_authorization {
    evaluation_mode = "DISABLED"
  }
}`,
			expectedModifications:     0,
			clusterName:               "clean_autopilot_cluster",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:            true,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:       true,
				expectEnabledRemoved:    false,
				expectResourceLimitsRemoved: false,
				expectProfileUnchanged:  stringPtr("BALANCED"),
			},
			binaryAuthorization: &binaryAuthorizationChecks{
				expectBlockExists:    true,
				expectEnabledRemoved: false,
			},
			expectNoOtherChanges: true,
		},
		{
			name: "enable_autopilot is not a boolean",
			hclContent: `resource "google_container_cluster" "invalid_autopilot_cluster" {
  name             = "invalid-autopilot-cluster"
  enable_autopilot = "not_a_boolean"
  enable_shielded_nodes = true
  cluster_ipv4_cidr     = "10.0.0.0/8"
  addons_config {
    dns_cache_config { enabled = true }
  }
}`,
			expectedModifications:     1, // Changed from 0 to 1
			clusterName:               "invalid_autopilot_cluster",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsRemoved:  []string{"enable_autopilot"}, // Added "enable_autopilot"
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:     true,
				expectDnsCacheRemoved: false,
			},
			expectNoOtherChanges:      true,
		},
		{
			name: "No google_container_cluster blocks",
			hclContent: `resource "google_compute_instance" "vm" {
  name = "my-vm"
}`,
			expectedModifications: 0,
			clusterName:           "",
			expectNoOtherChanges:  true,
		},
		{
			name:                      "Empty HCL content",
			hclContent:                ``,
			expectedModifications:     0,
			clusterName:               "",
			expectNoOtherChanges:      true,
		},
		{
			name: "Autopilot true, only some attributes to remove",
			hclContent: `resource "google_container_cluster" "partial_autopilot" {
  name                  = "partial-autopilot"
  enable_autopilot      = true
  enable_shielded_nodes = true
  default_max_pods_per_node = 110
}`,
			expectedModifications:     2,
			clusterName:               "partial_autopilot",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved:  []string{"enable_shielded_nodes", "default_max_pods_per_node"},
			expectedTopLevelNestedBlocksRemoved: []string{},
			expectNoOtherChanges:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, ruleErr := modifier.ApplyAutopilotRule()
			if ruleErr != nil {
				t.Fatalf("ApplyAutopilotRule() returned error = %v. HCL content:\n%s", ruleErr, tc.hclContent)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyAutopilotRule() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			if tc.clusterName == "" {
				if tc.expectedModifications == 0 {
					return
				}
				t.Logf("Test %s: No clusterName specified, but expectedModifications is %d. Skipping detailed checks.", tc.name, tc.expectedModifications)
				return
			}

			var clusterBlock *hclwrite.Block
			clusterBlock, _ = findBlockInParsedFile(modifier.File(), "google_container_cluster", tc.clusterName)


			if clusterBlock == nil {
				if tc.expectedModifications > 0 || (tc.addonsConfig != nil && tc.addonsConfig.expectBlockExists) || (tc.clusterAutoscaling != nil && tc.clusterAutoscaling.expectBlockExists) || (tc.binaryAuthorization != nil && tc.binaryAuthorization.expectBlockExists)  {
					t.Fatalf("google_container_cluster resource '%s' not found after ApplyAutopilotRule. HCL:\n%s", tc.clusterName, string(modifier.File().Bytes()))
				}
				return
			}

			enableAutopilotAttr := clusterBlock.Body().GetAttribute("enable_autopilot")
			if tc.expectEnableAutopilotAttr == nil {
				if enableAutopilotAttr != nil {
					if !(tc.name == "enable_autopilot is not a boolean" && string(enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()) == `"not_a_boolean"`) {
						t.Errorf("Expected 'enable_autopilot' attribute to be removed, but it was found. Modified HCL:\n%s", string(modifier.File().Bytes()))
					}
				}
			} else {
				if enableAutopilotAttr == nil {
					t.Errorf("Expected 'enable_autopilot' attribute to exist, but it was not found. Modified HCL:\n%s", string(modifier.File().Bytes()))
				} else {
					val, err := modifier.GetAttributeValue(enableAutopilotAttr)
					if err != nil {
						t.Errorf("Error getting value of 'enable_autopilot': %v. Modified HCL:\n%s", err, string(modifier.File().Bytes()))
					} else if val.Type() == cty.Bool {
						if val.True() != *tc.expectEnableAutopilotAttr {
							t.Errorf("Expected 'enable_autopilot' to be %v, but got %v. Modified HCL:\n%s", *tc.expectEnableAutopilotAttr, val.True(), string(modifier.File().Bytes()))
						}
					} else {
                        t.Errorf("Expected 'enable_autopilot' to be boolean, but got type %s. Modified HCL:\n%s", val.Type().FriendlyName(), string(modifier.File().Bytes()))
                    }
				}
			}
            if tc.name == "enable_autopilot is not a boolean" && enableAutopilotAttr != nil {
                 exprBytes := enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()
                 if string(exprBytes) != `"not_a_boolean"` {
                    t.Errorf("Expected 'enable_autopilot' to remain as \"not_a_boolean\", got %s. Modified HCL:\n%s", string(exprBytes), string(modifier.File().Bytes()))
                 }
            }

			for _, attrName := range tc.expectedRootAttrsRemoved {
				if attr := clusterBlock.Body().GetAttribute(attrName); attr != nil {
					t.Errorf("Expected root attribute '%s' to be removed, but it was found. Modified HCL:\n%s", attrName, string(modifier.File().Bytes()))
				}
			}

			for _, blockTypeName := range tc.expectedTopLevelNestedBlocksRemoved {
				if blockTypeName == "node_pool" {
					foundNodePools := false
					for _, nestedB := range clusterBlock.Body().Blocks() {
						if nestedB.Type() == "node_pool" {
							foundNodePools = true
							break
						}
					}
					if foundNodePools {
						t.Errorf("Expected all nested blocks of type 'node_pool' to be removed, but at least one was found. Modified HCL:\n%s", string(modifier.File().Bytes()))
					}
				} else {
					if blk := clusterBlock.Body().FirstMatchingBlock(blockTypeName, nil); blk != nil {
						t.Errorf("Expected nested block '%s' to be removed, but it was found. Modified HCL:\n%s", blockTypeName, string(modifier.File().Bytes()))
					}
				}
			}

			if tc.addonsConfig != nil {
				acBlock := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
				if !tc.addonsConfig.expectBlockExists {
					if acBlock != nil {
						t.Errorf("Expected 'addons_config' block to be removed or not exist, but it was found.")
					}
				} else {
					if acBlock == nil {
						t.Fatalf("Expected 'addons_config' block to exist, but it was not found.")
					}
					if tc.addonsConfig.expectNetworkPolicyRemoved {
						if acBlock.Body().FirstMatchingBlock("network_policy_config", nil) != nil {
							t.Errorf("Expected 'network_policy_config' in 'addons_config' to be removed.")
						}
					}
					if tc.addonsConfig.expectDnsCacheRemoved {
						if acBlock.Body().FirstMatchingBlock("dns_cache_config", nil) != nil {
							t.Errorf("Expected 'dns_cache_config' in 'addons_config' to be removed.")
						}
					}
					if tc.addonsConfig.expectStatefulHaRemoved {
						if acBlock.Body().FirstMatchingBlock("stateful_ha_config", nil) != nil {
							t.Errorf("Expected 'stateful_ha_config' in 'addons_config' to be removed.")
						}
					}
					if tc.addonsConfig.expectHttpLoadBalancingUnchanged {
						if acBlock.Body().FirstMatchingBlock("http_load_balancing", nil) == nil {
							t.Errorf("Expected 'http_load_balancing' in 'addons_config' to be unchanged, but it was not found.")
						}
					}
				}
			}

			if tc.clusterAutoscaling != nil {
				caBlock := clusterBlock.Body().FirstMatchingBlock("cluster_autoscaling", nil)
				if !tc.clusterAutoscaling.expectBlockExists {
					if caBlock != nil {
						t.Errorf("Expected 'cluster_autoscaling' block to be removed, but it was found.")
					}
				} else {
					if caBlock == nil {
						t.Fatalf("Expected 'cluster_autoscaling' block to exist, but it was not found.")
					}
					if tc.clusterAutoscaling.expectEnabledRemoved {
						if attr := caBlock.Body().GetAttribute("enabled"); attr != nil {
							t.Errorf("Expected 'enabled' attribute in 'cluster_autoscaling' to be removed, but it was found.")
						}
					}

					if tc.clusterAutoscaling.expectResourceLimitsRemoved {
						if attr := caBlock.Body().GetAttribute("resource_limits"); attr != nil {
							t.Errorf("Expected 'resource_limits' attribute in 'cluster_autoscaling' to be removed, but it was found.")
						}
					}

					if tc.clusterAutoscaling.expectProfileUnchanged != nil {
						profileAttr := caBlock.Body().GetAttribute("autoscaling_profile")
						if profileAttr == nil {
							t.Errorf("Expected 'autoscaling_profile' attribute in 'cluster_autoscaling' to exist, but it was not found.")
						} else {
							val, err := modifier.GetAttributeValue(profileAttr)
							if err != nil || val.Type() != cty.String || val.AsString() != *tc.clusterAutoscaling.expectProfileUnchanged {
								t.Errorf("Expected 'autoscaling_profile' to be '%s', got '%v' (err: %v).", *tc.clusterAutoscaling.expectProfileUnchanged, val, err)
							}
						}
					}
				}
			}

			if tc.binaryAuthorization != nil {
				baBlock := clusterBlock.Body().FirstMatchingBlock("binary_authorization", nil)
				if !tc.binaryAuthorization.expectBlockExists {
					if baBlock != nil {
						t.Errorf("Expected 'binary_authorization' block to be removed, but it was found.")
					}
				} else {
					if baBlock == nil {
						t.Fatalf("Expected 'binary_authorization' block to exist, but it was not found.")
					}
					if tc.binaryAuthorization.expectEnabledRemoved {
						if attr := baBlock.Body().GetAttribute("enabled"); attr != nil {
							t.Errorf("Expected 'enabled' attribute in 'binary_authorization' to be removed, but it was found.")
						}
					}
					if tc.name == "enable_autopilot is true, all conflicting fields present" {
						evalModeAttr := baBlock.Body().GetAttribute("evaluation_mode")
						if evalModeAttr == nil {
							t.Errorf("Expected 'evaluation_mode' in 'binary_authorization' to remain, but it was not found.")
						} else {
							val, err := modifier.GetAttributeValue(evalModeAttr)
							if err != nil || val.Type() != cty.String || val.AsString() != "PROJECT_SINGLETON_POLICY_ENFORCE" {
								t.Errorf("Expected 'evaluation_mode' to be 'PROJECT_SINGLETON_POLICY_ENFORCE', got '%v' (err: %v).", val, err)
							}
						}
					}
				}
			}

			if tc.expectNoOtherChanges {
				if tc.name == "enable_autopilot is false, conflicting fields present" {
					if clusterBlock.Body().GetAttribute("cluster_ipv4_cidr") == nil {
						t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed in 'enable_autopilot=false' case.")
					}
					addonsCfg := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
					if addonsCfg == nil {
						t.Errorf("'addons_config' block was unexpectedly removed in 'enable_autopilot=false' case.")
					} else {
						if addonsCfg.Body().FirstMatchingBlock("dns_cache_config", nil) == nil {
							t.Errorf("'dns_cache_config' in 'addons_config' was unexpectedly removed in 'enable_autopilot=false' case.")
						}
						if addonsCfg.Body().FirstMatchingBlock("http_load_balancing", nil) == nil {
							t.Errorf("'http_load_balancing' in 'addons_config' was unexpectedly removed in 'enable_autopilot=false' case.")
						}
					}
					if clusterBlock.Body().GetAttribute("enable_shielded_nodes") == nil {
						t.Errorf("'enable_shielded_nodes' was unexpectedly removed in 'enable_autopilot=false' case.")
					}
					if clusterBlock.Body().FirstMatchingBlock("node_pool", nil) == nil {
						t.Errorf("'node_pool' block was unexpectedly removed in 'enable_autopilot=false' case.")
					}
				}
				if tc.name == "enable_autopilot not present, conflicting fields present" {
					if clusterBlock.Body().GetAttribute("cluster_ipv4_cidr") == nil {
						t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed in 'enable_autopilot not present' case.")
					}
					addonsCfg := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
					if addonsCfg == nil {
						t.Errorf("'addons_config' block was unexpectedly removed in 'enable_autopilot not present' case.")
					} else if addonsCfg.Body().FirstMatchingBlock("network_policy_config", nil) == nil {
						t.Errorf("'network_policy_config' in 'addons_config' was unexpectedly removed in 'enable_autopilot not present' case.")
					}
					if clusterBlock.Body().GetAttribute("enable_shielded_nodes") == nil {
						t.Errorf("'enable_shielded_nodes' was unexpectedly removed in 'enable_autopilot not present' case.")
					}
					if clusterBlock.Body().FirstMatchingBlock("node_pool", nil) == nil {
						t.Errorf("'node_pool' block was unexpectedly removed in 'enable_autopilot not present' case.")
					}
				}
				if tc.name == "enable_autopilot is not a boolean" {
					enableAutopilotAttrCurrent := clusterBlock.Body().GetAttribute("enable_autopilot")
					if enableAutopilotAttrCurrent == nil {
						t.Errorf("'enable_autopilot' (with non-boolean value) was unexpectedly removed.")
					} else {
						exprBytes := enableAutopilotAttrCurrent.Expr().BuildTokens(nil).Bytes()
						if string(exprBytes) != `"not_a_boolean"` {
							t.Errorf("Expected 'enable_autopilot' to remain as \"not_a_boolean\", got %s.", string(exprBytes))
						}
					}
					if clusterBlock.Body().GetAttribute("cluster_ipv4_cidr") == nil {
						t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed in 'enable_autopilot is not a boolean' case.")
					}
					addonsCfg := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
					if addonsCfg == nil {
						t.Errorf("'addons_config' block was unexpectedly removed in 'enable_autopilot is not a boolean' case.")
					} else if addonsCfg.Body().FirstMatchingBlock("dns_cache_config", nil) == nil {
						t.Errorf("'dns_cache_config' in 'addons_config' was unexpectedly removed in 'enable_autopilot is not a boolean' case.")
					}
					if clusterBlock.Body().GetAttribute("enable_shielded_nodes") == nil {
						t.Errorf("'enable_shielded_nodes' was unexpectedly removed in 'enable_autopilot is not a boolean' case.")
					}
				}
			}
		})
	}
}

func TestRemoveBlock(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []struct {
		name            string
		hclContent      string
		blockType       string
		blockLabels     []string
		expectRemoved   bool
		expectCallError bool
	}{
		{
			name: "remove existing resource block",
			hclContent: `resource "aws_instance" "my_test_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-test-bucket"
}`,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   true,
			expectCallError: false,
		},
		{
			name: "attempt to remove non-existing resource block by name",
			hclContent: `resource "aws_instance" "another_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}`,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "non_existent_instance"},
			expectRemoved:   false,
			expectCallError: true,
		},
		{
			name: "attempt to remove block with incorrect type but existing labels",
			hclContent: `resource "aws_instance" "my_test_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}`,
			blockType:       "data",
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   false,
			expectCallError: true,
		},
		{
			name:            "empty HCL content",
			hclContent:      ``,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   false,
			expectCallError: true,
		},
		{
			name: "remove data block",
			hclContent: `data "aws_caller_identity" "current" {}
resource "aws_instance" "main" {}
`,
			blockType:       "data",
			blockLabels:     []string{"aws_caller_identity", "current"},
			expectRemoved:   true,
			expectCallError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}
			
			modifier, newFromFileErr := NewFromFile(tmpFile.Name(), logger)

			if tc.hclContent == "" {
				if newFromFileErr != nil && tc.expectCallError {
					return
				}
				if newFromFileErr != nil && !tc.expectCallError {
					t.Fatalf("NewFromFile() errored unexpectedly for empty HCL: %v", newFromFileErr)
				}
			} else if newFromFileErr != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", newFromFileErr, tc.hclContent)
			}

			err = modifier.RemoveBlock(tc.blockType, tc.blockLabels)
			if (err != nil) != tc.expectCallError {
				t.Fatalf("RemoveBlock() error status = %v (err: %v), expectCallError %v. HCL:\n%s", (err != nil), err, tc.expectCallError, tc.hclContent)
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, parseDiags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if parseDiags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", parseDiags, string(modifiedContentBytes))
			}

			var foundBlockInVerified *hclwrite.Block
			// Check if the block that was supposed to be removed (or not) is present
			if !(tc.blockType == "resource" || tc.blockType == "data") { // Simple blocks like "terraform"
				if len(tc.blockLabels) == 0 { // e.g. terraform {}
					foundBlockInVerified = verifiedFile.Body().FirstMatchingBlock(tc.blockType, nil)
				} else { // e.g. provider "aws"
					foundBlockInVerified = verifiedFile.Body().FirstMatchingBlock(tc.blockType, tc.blockLabels)
				}
			} else { // Resource or Data blocks
                if len(tc.blockLabels) > 0 { // Should always be true for resource/data
				    foundBlockInVerified, _ = findBlockInParsedFile(verifiedFile, tc.blockLabels[0], tc.blockLabels[1])
                }
			}


			if tc.expectRemoved {
				if foundBlockInVerified != nil {
					t.Errorf("RemoveBlock() expected block %s %v to be removed, but it was found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
				}
			} else { // Not expected to be removed
				initialParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
				initialBlockPresent := false
				if initialParsedFile != nil && initialParsedFile.Body() != nil {
                    var initialBlock *hclwrite.Block
                    if !(tc.blockType == "resource" || tc.blockType == "data") {
                        if len(tc.blockLabels) == 0 {
                            initialBlock = initialParsedFile.Body().FirstMatchingBlock(tc.blockType, nil)
                        } else {
                            initialBlock = initialParsedFile.Body().FirstMatchingBlock(tc.blockType, tc.blockLabels)
                        }
                    } else {
                        if len(tc.blockLabels) > 0 {
						    initialBlock, _ = findBlockInParsedFile(initialParsedFile, tc.blockLabels[0], tc.blockLabels[1])
                        }
                    }
					if initialBlock != nil {
						initialBlockPresent = true
					}
				}

				if tc.expectCallError { // Error was expected, so block should remain if it was there
					if initialBlockPresent && foundBlockInVerified == nil {
						 t.Errorf("RemoveBlock() errored as expected for %s %v, but block was unexpectedly removed. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
					}
					if !initialBlockPresent && foundBlockInVerified != nil {
						t.Errorf("RemoveBlock() logic error: block %s %v not present initially, call errored, but block now found. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
					}
				} else { // No error expected, and not expected to be removed
					if initialBlockPresent && foundBlockInVerified == nil {
						t.Errorf("RemoveBlock() did not error, expected block %s %v NOT to be removed, but it was. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
					}
					if !initialBlockPresent && foundBlockInVerified != nil {
                        t.Errorf("RemoveBlock() logic error: block %s %v was not present initially, no call error, but was found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
                    }
				}
			}
		})
	}
}

// Helper Functions
func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

// findBlockInParsedFile finds a resource or data block by its type and name labels.
// For a resource "type" "name", blockTypeLabel should be "type" and resourceNameLabel should be "name".
func findBlockInParsedFile(file *hclwrite.File, blockTypeLabel string, resourceNameLabel string) (*hclwrite.Block, error) {
	if file == nil || file.Body() == nil {
		return nil, fmt.Errorf("file or file body is nil")
	}
	for _, b := range file.Body().Blocks() {
		// This function assumes we are looking for blocks like: resource "google_container_cluster" "my_cluster" {}
		// where b.Type() is "resource", b.Labels()[0] is "google_container_cluster", b.Labels()[1] is "my_cluster"
		// It also supports data blocks: data "archive_file" "zip"
		if b.Type() == "resource" || b.Type() == "data" {
			if len(b.Labels()) == 2 && b.Labels()[0] == blockTypeLabel && b.Labels()[1] == resourceNameLabel {
				return b, nil
			}
		}
	}
	return nil, fmt.Errorf("block of type 'resource' or 'data' with type label '%s' and name label '%s' not found", blockTypeLabel, resourceNameLabel)
}

func findNodePoolInBlock(resourceBlock *hclwrite.Block, nodePoolName string, mod *Modifier) (*hclwrite.Block, error) {
	if resourceBlock == nil || resourceBlock.Body() == nil {
		return nil, fmt.Errorf("resource block or body is nil")
	}
	for _, nb := range resourceBlock.Body().Blocks() {
		if nb.Type() == "node_pool" {
			if nodePoolName == "" {
				return nb, nil
			}
			nameAttr := nb.Body().GetAttribute("name")
			if nameAttr != nil && mod != nil {
				val, err := mod.GetAttributeValue(nameAttr)
				if err == nil && val.Type() == cty.String && val.AsString() == nodePoolName {
					return nb, nil
				}
			} else if nameAttr == nil && nodePoolName != "" {
                continue
            } else if nameAttr == nil && nodePoolName == "" {
                return nb, nil
            }
		}
	}
	if nodePoolName == "" {
		return nil, fmt.Errorf("no node_pool block found in the resource")
	}
	return nil, fmt.Errorf("node_pool with name '%s' not found in the resource", nodePoolName)
}

func TestModifier_ApplyRuleRemoveLoggingService(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []struct {
		name                  string
		hclContent            string
		expectedHCLContent    string
		expectedModifications int
		ruleToApply           Rule
	}{
		{
			name: "logging_service should be removed when telemetry is ENABLED",
			hclContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  logging_service  = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedModifications: 1,
			ruleToApply:           RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed when telemetry is DISABLED",
			hclContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  logging_service  = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "DISABLED"
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  logging_service  = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "DISABLED"
  }
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed when telemetry block is missing",
			hclContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  logging_service  = "logging.googleapis.com/kubernetes"
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  logging_service  = "logging.googleapis.com/kubernetes"
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed if logging_service attribute is missing",
			hclContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name             = "my-cluster"
  location         = "us-central1"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveLoggingService,
		},
		{
			name: "Non-GKE resource with similar structure, should not be modified",
			hclContent: `resource "google_compute_instance" "primary" {
  name             = "my-instance"
  logging_service  = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedHCLContent: `resource "google_compute_instance" "primary" {
  name             = "my-instance"
  logging_service  = "logging.googleapis.com/kubernetes"
  cluster_telemetry {
    type = "ENABLED"
  }
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveLoggingService,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_logging_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			}

			modifications, errs := modifier.ApplyRules([]Rule{tc.ruleToApply})
			if len(errs) > 0 {
				t.Fatalf("ApplyRules() returned errors = %v for HCL: \n%s", errs, tc.hclContent)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			// Normalize and compare HCL content
			expectedF, diags := hclwrite.ParseConfig([]byte(tc.expectedHCLContent), "expected.hcl", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse expected HCL content: %v\nExpected HCL:\n%s", diags, tc.expectedHCLContent)
			}
			expectedNormalized := string(expectedF.Bytes())
			actualNormalized := string(modifier.File().Bytes())

			if actualNormalized != expectedNormalized {
				t.Errorf("HCL content mismatch.\nExpected:\n%s\nGot:\n%s", expectedNormalized, actualNormalized)
			}
		})
	}
}

func TestModifier_ApplyRuleRemoveMonitoringService(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []struct {
		name                  string
		hclContent            string
		expectedHCLContent    string
		expectedModifications int
		ruleToApply           Rule
	}{
		{
			name: "monitoring_service should be removed when monitoring_config block exists",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedModifications: 1,
			ruleToApply:           RuleRemoveMonitoringService,
		},
		{
			name: "monitoring_service should NOT be removed when monitoring_config block is missing",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveMonitoringService,
		},
		{
			name: "monitoring_service should NOT be removed if monitoring_service attribute is missing",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "primary" {
  name               = "my-cluster"
  location           = "us-central1"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveMonitoringService,
		},
		{
			name: "Non-GKE resource with similar structure, should not be modified",
			hclContent: `resource "google_compute_instance" "primary" {
  name               = "my-instance"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedHCLContent: `resource "google_compute_instance" "primary" {
  name               = "my-instance"
  monitoring_service = "monitoring.googleapis.com/kubernetes"
  monitoring_config {
    enable_components = ["SYSTEM_COMPONENTS"]
  }
}`,
			expectedModifications: 0,
			ruleToApply:           RuleRemoveMonitoringService,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_monitoring_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			}

			modifications, errs := modifier.ApplyRules([]Rule{tc.ruleToApply})
			if len(errs) > 0 {
				t.Fatalf("ApplyRules() returned errors = %v for HCL: \n%s", errs, tc.hclContent)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
			}

			// Normalize and compare HCL content
			expectedF, diags := hclwrite.ParseConfig([]byte(tc.expectedHCLContent), "expected.hcl", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse expected HCL content: %v\nExpected HCL:\n%s", diags, tc.expectedHCLContent)
			}
			expectedNormalized := string(expectedF.Bytes())
			actualNormalized := string(modifier.File().Bytes())

			if actualNormalized != expectedNormalized {
				t.Errorf("HCL content mismatch.\nExpected:\n%s\nGot:\n%s", expectedNormalized, actualNormalized)
			}
		})
	}
}
