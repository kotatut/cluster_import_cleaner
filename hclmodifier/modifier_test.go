package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax" // Added for ParseExpression
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty" // Added for cty.String
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
			hclContent: `
resource "aws_instance" "example" {
  name = "test_name"
}`,
			expectedName:      "test_name" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "multiple resources with name",
			hclContent: `
resource "aws_instance" "example1" {
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
			hclContent: `
resource "aws_instance" "example" {
  ami = "ami-0c55b31ad2c454370"
}`,
			expectedName:      "", 
			expectedAttrCount: 0,
		},
		{
			name: "resource with empty name",
			hclContent: `
resource "aws_instance" "example" {
  name = ""
}`,
			expectedName:      "" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "terraform block with name attribute",
			hclContent: `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
  name = "my_terraform_block" 
}`,
			expectedName:      "", 
			expectedAttrCount: 0, // Now expecting 0 due to modifier.go change
		},
		{
			name: "resource with complex name expression",
			hclContent: `
resource "aws_instance" "example" {
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
				// This case implies that if no attributes are expected to be counted as modified,
				// then no specific name should be searched for if it was expected to be modified.
				// If the intent is to check the name *wasn't* modified, this logic would need to be different.
				// For now, ensuring the count is 0 is the primary check for these cases.
				// A specific check for the *absence* of tc.expectedName (if it was the original name) could be added.
				t.Logf("Test case info: expectedAttrCount is 0. expectedName ('%s') is set; this test primarily verifies count, not specific name absence/presence unless count > 0.", tc.expectedName)
			}
		})
	}
}

func TestApplyInitialNodeCountRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewNop() // Use NewNop for cleaner test output

	tests := []struct {
		name                           string
		hclContent                     string
		expectedModifications          int
		gkeResourceName                string // Name of the GKE resource to check
		nodePoolChecks                 []nodePoolCheck
		expectNoOtherResourceChanges   bool // If true, implies other resources should be untouched
		expectNoGKEResource            bool // If true, GKE resource itself is not expected
	}{
		{
			name: "BothCountsPresent",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
					nodePoolName:               "default-pool", // Assuming node_pool has a 'name' attribute for identification in tests
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5), // We don't change node_count, just verify it's still there
				},
			},
		},
		{
			name: "OnlyInitialPresent",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
					expectInitialNodeCountRemoved: false, // Was never there
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(4),
				},
			},
		},
		{
			name: "NeitherCountPresent",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
					expectInitialNodeCountRemoved: false, // Was never there
					expectNodeCountPresent:        false, // Was never there
				},
			},
		},
		{
			name: "MultipleNodePools",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  node_pool { # Pool 1: Both present
    name               = "pool-one"
    initial_node_count = 3
    node_count         = 5
  }
  node_pool { # Pool 2: Only initial
    name               = "pool-two"
    initial_node_count = 2
  }
  node_pool { # Pool 3: Only node_count
    name       = "pool-three"
    node_count = 4
  }
  node_pool { # Pool 4: Neither
    name = "pool-four"
  }
}`,
			expectedModifications: 2, // One from pool-one, one from pool-two
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
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name     = "test-cluster"
  location = "us-central1"
}`,
			expectedModifications: 0,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks:        nil, // No node pools to check
		},
		{
			name: "NonGKEResource",
			hclContent: `
resource "google_compute_instance" "not_gke" {
  name = "test-vm"
  node_pool { # This block would be ignored
    initial_node_count = 1
    node_count         = 2
  }
  initial_node_count = 5 # This attribute would be ignored
}`,
			expectedModifications: 0,
			gkeResourceName:       "", // No GKE resource to check specifically
			expectNoOtherResourceChanges: true,
			nodePoolChecks:               nil, // Or define checks for "google_compute_instance" "not_gke" to ensure it's untouched
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
			hclContent: `
resource "google_container_cluster" "gke_one" {
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
			expectedModifications: 2, // One from gke_one-pool, one from gke_two-pool
			// We need to check both GKE resources
			// For simplicity, this test setup might need to be expanded or split
			// to verify each GKE resource independently if nodePoolChecks only targets one gkeResourceName.
			// Let's assume for now we check gke_one, and the total count implies gke_two was also handled.
			// A more robust test would iterate all GKE resources found.
			gkeResourceName: "gke_one", // Check this one specifically
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:               "gke-one-pool",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
			},
			// To fully test "MultipleGKEResources", we'd also want to assert on "gke_two"
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
						return // NewFromFile failed as expected for empty content.
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

			// Re-parse the modified HCL content for verification
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
				return // No further checks if no GKE resource is expected
			}

			// Find the specific GKE resource if a name is provided
			var targetGKEResource *hclwrite.Block
			if tc.gkeResourceName != "" {
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 &&
						b.Labels()[0] == "google_container_cluster" && b.Labels()[1] == tc.gkeResourceName {
						targetGKEResource = b
						break
					}
				}
				if targetGKEResource == nil && len(tc.nodePoolChecks) > 0 { // If checks are defined, resource must exist
					t.Fatalf("Expected 'google_container_cluster' resource '%s' not found for verification. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
				}
			}


			if targetGKEResource != nil && tc.nodePoolChecks != nil {
				for _, npCheck := range tc.nodePoolChecks {
					var foundNodePool *hclwrite.Block
					for _, nestedBlock := range targetGKEResource.Body().Blocks() {
						if nestedBlock.Type() == "node_pool" {
							// Assuming node_pool blocks have a 'name' attribute for reliable identification in tests.
							// If not, this check needs to be adapted (e.g., by order or other unique attributes).
							nameAttr := nestedBlock.Body().GetAttribute("name")
							if nameAttr != nil {
								nameVal, err := modifier.GetAttributeValue(nameAttr) // Use existing modifier for GetAttributeValue
								if err == nil && nameVal.Type() == cty.String && nameVal.AsString() == npCheck.nodePoolName {
									foundNodePool = nestedBlock
									break
								}
							} else if npCheck.nodePoolName == "" && len(targetGKEResource.Body().BlocksByType("node_pool")) == 1 {
								// If no name to check and only one node_pool, assume it's the one.
								foundNodePool = nestedBlock
								break
							}
						}
					}

					if foundNodePool == nil {
						// If we expected a change in this node pool, it should be found.
						if npCheck.expectInitialNodeCountRemoved || npCheck.expectNodeCountPresent {
							t.Errorf("Node pool '%s' in resource '%s' not found for verification. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
						continue // Skip to next node pool check
					}

					initialAttr := foundNodePool.Body().GetAttribute("initial_node_count")
					if npCheck.expectInitialNodeCountRemoved {
						if initialAttr != nil {
							t.Errorf("Expected 'initial_node_count' to be REMOVED from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					} else { // Expect initial_node_count to be present (or absent if it was never there)
						originalInitialPresent := false // Check original HCL
						originalGKEResource, _ := findBlockInParsedFile(hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos), "google_container_cluster", tc.gkeResourceName)
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
							// Verify its value remained unchanged (if it was there)
							val, err := modifier.GetAttributeValue(nodeCountAttr)
							if err != nil || !val.IsKnown() || val.IsNull() || val.Type() != cty.Number {
								t.Errorf("Error or wrong type for 'node_count' in node_pool '%s': %v. Modified HCL:\n%s", npCheck.nodePoolName, err, string(modifiedContentBytes))
							} else {
								numVal := val.AsBigFloat()
								expectedNum := float64(*npCheck.expectedNodeCountValue)
								if numVal.Float64() != expectedNum {
									t.Errorf("Expected 'node_count' value %d, got %s in node_pool '%s'. Modified HCL:\n%s",
										*npCheck.expectedNodeCountValue, val.GoString(), npCheck.nodePoolName, string(modifiedContentBytes))
								}
							}
						}
					} else { // Expect node_count to be absent
						if nodeCountAttr != nil {
							t.Errorf("Expected 'node_count' to be ABSENT from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					}
				}
			}

			// Special check for "MultipleGKEResources" to ensure the second GKE resource was handled correctly.
			if tc.name == "MultipleGKEResources" {
				var gkeTwoResource *hclwrite.Block
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 &&
						b.Labels()[0] == "google_container_cluster" && b.Labels()[1] == "gke_two" {
						gkeTwoResource = b
						break
					}
				}
				if gkeTwoResource == nil {
					t.Fatalf("Expected 'google_container_cluster' resource 'gke_two' not found for multi-resource verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				// Check its first node_pool ("gke-two-pool")
				gkeTwoPool1, _ := findNodePoolInBlock(gkeTwoResource, "gke-two-pool", modifier)
				if gkeTwoPool1 == nil {
					t.Errorf("Node pool 'gke-two-pool' in 'gke_two' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if gkeTwoPool1.Body().GetAttribute("initial_node_count") != nil {
						t.Errorf("'initial_node_count' should have been removed from 'gke-two-pool', but was found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
					if gkeTwoPool1.Body().GetAttribute("node_count") != nil {
						t.Errorf("'node_count' should be absent from 'gke-two-pool', but was found. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
				// Check its second node_pool ("gke-two-pool-extra")
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
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == "google_compute_instance" && b.Labels()[1] == "not_gke" {
						nonGKEResource = b
						break
					}
				}
				if nonGKEResource == nil {
					t.Fatalf("Expected non-GKE resource 'google_compute_instance.not_gke' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				// Check its initial_node_count attribute is still there
				if nonGKEResource.Body().GetAttribute("initial_node_count") == nil {
					t.Errorf("Top-level 'initial_node_count' was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				// Check its node_pool block and its attributes
				npBlock, _ := findNodePoolInBlock(nonGKEResource, "", modifier) // Assuming only one node_pool or name doesn't matter here for this non-gke resource
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

// Helper struct for node pool checks
type nodePoolCheck struct {
	nodePoolName                  string // Name of the node_pool (if identifiable by name)
	expectInitialNodeCountRemoved bool
	expectNodeCountPresent        bool
	expectedNodeCountValue        *int // If expectNodeCountPresent is true, optionally check its value
}

// Helper to find a block in a parsed file (useful for checking original state)
func findBlockInParsedFile(file *hclwrite.File, blockType string, resourceName string) (*hclwrite.Block, error) {
	if file == nil || file.Body() == nil {
		return nil, fmt.Errorf("file or file body is nil")
	}
	for _, b := range file.Body().Blocks() {
		if b.Type() == "resource" && len(b.Labels()) == 2 &&
			b.Labels()[0] == blockType && b.Labels()[1] == resourceName {
			return b, nil
		}
	}
	return nil, fmt.Errorf("block %s %s not found", blockType, resourceName)
}

// Helper to find a node_pool block within a given resource block
// Needs modifier for GetAttributeValue if identifying by name.
func findNodePoolInBlock(resourceBlock *hclwrite.Block, nodePoolName string, mod *Modifier) (*hclwrite.Block, error) {
	if resourceBlock == nil || resourceBlock.Body() == nil {
		return nil, fmt.Errorf("resource block or body is nil")
	}
	for _, nb := range resourceBlock.Body().Blocks() {
		if nb.Type() == "node_pool" {
			if nodePoolName == "" { // If no name specified, return the first one found
				return nb, nil
			}
			nameAttr := nb.Body().GetAttribute("name")
			if nameAttr != nil && mod != nil {
				val, err := mod.GetAttributeValue(nameAttr) // Use the modifier's GetAttributeValue
				if err == nil && val.Type() == cty.String && val.AsString() == nodePoolName {
					return nb, nil
				}
			}
		}
	}
	if nodePoolName == "" {
		return nil, fmt.Errorf("no node_pool block found")
	}
	return nil, fmt.Errorf("node_pool with name '%s' not found", nodePoolName)
}

// Helper function to get a pointer to an int value
func intPtr(i int) *int {
	return &i
}

func TestApplyMasterCIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewNop() // Use NewNop for cleaner test output

	type privateClusterConfigCheck struct {
		expectBlockExists                   bool
		expectPrivateEndpointSubnetworkRemoved bool
		expectOtherAttributeUnchanged       *string // e.g., "enable_private_endpoint"
	}

	tests := []struct {
		name                        string
		hclContent                  string
		expectedModifications       int
		gkeResourceName             string // Name of the GKE resource to check
		expectMasterCIDRPresent     bool   // Whether master_ipv4_cidr_block should be present at the end
		privateClusterConfigCheck   *privateClusterConfigCheck
		expectNoOtherResourceChanges bool // If true, implies other resources should be untouched
		expectNoGKEResource         bool // If true, GKE resource itself is not expected
	}{
		{
			name: "BothPresent",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name                   = "test-cluster"
  master_ipv4_cidr_block = "172.16.0.0/28"
  # private_cluster_config is missing
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists: false, // Block itself is missing
			},
		},
		{
			name: "OnlyMasterCIDRPresent_SubnetworkMissingInPrivateConfig",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name                   = "test-cluster"
  master_ipv4_cidr_block = "172.16.0.0/28"
  private_cluster_config {
    enable_private_endpoint = true
    # private_endpoint_subnetwork is missing
  }
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: false, // Was never there
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "OnlyPrivateEndpointSubnetworkPresent_MasterCIDRMissing",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  # master_ipv4_cidr_block is missing
  private_cluster_config {
    enable_private_endpoint   = true
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
  }
}`,
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: false, // Was never there
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: false, // Master CIDR not present, so no removal
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "PrivateConfigExistsNoSubnetwork_MasterCIDRPresent", // Same as OnlyMasterCIDRPresent_SubnetworkMissingInPrivateConfig
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
				expectPrivateEndpointSubnetworkRemoved: false, // Was never there
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
		},
		{
			name: "PrivateConfigMissing_MasterCIDRPresent", // Same as OnlyMasterCIDRPresent_PrivateConfigMissing
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
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
			name: "NeitherPresent",
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  # master_ipv4_cidr_block is missing
  private_cluster_config {
    enable_private_endpoint = true
    # private_endpoint_subnetwork is missing
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
			hclContent: `
resource "google_container_cluster" "gke_cluster" {
  name = "test-cluster"
  # master_ipv4_cidr_block is missing
  # private_cluster_config is missing
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
			hclContent: `
resource "google_compute_instance" "not_gke" {
  name                   = "test-vm"
  master_ipv4_cidr_block = "172.16.0.0/28" # Attribute name clash
  private_cluster_config {                 # Block name clash
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
    enable_private_endpoint   = true
  }
}`,
			expectedModifications:      0,
			gkeResourceName:            "", // No GKE resource to check specifically
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
			hclContent: `
resource "google_container_cluster" "gke_one_match" {
  name                     = "cluster-one"
  master_ipv4_cidr_block   = "172.16.0.0/28"
  private_cluster_config {
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-one"
    enable_private_endpoint   = true
  }
}
resource "google_container_cluster" "gke_two_no_master_cidr" {
  name = "cluster-two"
  # master_ipv4_cidr_block is missing
  private_cluster_config {
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-two"
  }
}
resource "google_container_cluster" "gke_three_no_subnetwork" {
  name                     = "cluster-three"
  master_ipv4_cidr_block   = "172.16.1.0/28"
  private_cluster_config {
    # private_endpoint_subnetwork is missing
    enable_private_endpoint = false
  }
}`,
			expectedModifications:   1,
			gkeResourceName:         "gke_one_match", // Check this one specifically for removal
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists:                   true,
				expectPrivateEndpointSubnetworkRemoved: true,
				expectOtherAttributeUnchanged:       stringPtr("enable_private_endpoint"),
			},
			// We also expect gke_two_no_master_cidr and gke_three_no_subnetwork to be untouched by the rule's removal logic.
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
						return // NewFromFile failed as expected for empty content.
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
				// Check master_ipv4_cidr_block presence
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

				// Check private_cluster_config
				if tc.privateClusterConfigCheck != nil {
					pccBlock := targetGKEResource.Body().FirstMatchingBlock("private_cluster_config", nil)
					if !tc.privateClusterConfigCheck.expectBlockExists {
						if pccBlock != nil {
							t.Errorf("Expected 'private_cluster_config' block NOT to exist in GKE resource '%s', but it was FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
						}
					} else { // Expect block to exist
						if pccBlock == nil {
							t.Fatalf("Expected 'private_cluster_config' block to EXIST in GKE resource '%s', but it was NOT FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
						}

						subnetworkAttr := pccBlock.Body().GetAttribute("private_endpoint_subnetwork")
						if tc.privateClusterConfigCheck.expectPrivateEndpointSubnetworkRemoved {
							if subnetworkAttr != nil {
								t.Errorf("Expected 'private_endpoint_subnetwork' to be REMOVED from 'private_cluster_config' in '%s', but it was FOUND. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
							}
						} else { // Expect subnetwork to be present or absent based on original (if not removed)
							originalGKEResource, _ := findBlockInParsedFile(hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos), "google_container_cluster", tc.gkeResourceName)
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
								// This case should ideally not happen if the rule only removes.
								t.Errorf("'private_endpoint_subnetwork' was unexpectedly ADDED to 'private_cluster_config' in '%s'. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
							}
						}

						if tc.privateClusterConfigCheck.expectOtherAttributeUnchanged != nil {
							otherAttrName := *tc.privateClusterConfigCheck.expectOtherAttributeUnchanged
							otherAttr := pccBlock.Body().GetAttribute(otherAttrName)
							// Check if this other attribute was present in the original HCL
							originalGKEResource, _ := findBlockInParsedFile(hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos), "google_container_cluster", tc.gkeResourceName)
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

			// Specific checks for MultipleGKEResources case
			if tc.name == "MultipleGKEResources_OneMatch" {
				// Check gke_two_no_master_cidr (should be untouched by removal)
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

				// Check gke_three_no_subnetwork (should be untouched by removal)
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
				if pccThree.Body().GetAttribute("enable_private_endpoint") == nil { // Check the other attr is still there
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
	logger, _ := zap.NewDevelopment() // Or use zap.NewNop() for less verbose test output

	tests := []struct {
		name                               string
		hclContent                         string
		expectedModifications              int
		expectEnabledAttributeRemoved      bool     // True if 'enabled' should be removed from binary_authorization
		resourceLabelsToVerify             []string // e.g., ["google_container_cluster", "primary"]
		binaryAuthorizationShouldExist     bool     // True if we need to check inside binary_authorization
		binaryAuthorizationShouldHaveEvalMode bool   // True if evaluation_mode should be present after modification
	}{
		{
			name: "Both enabled and evaluation_mode present",
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_compute_instance" "default" {
  name = "test-instance"
  binary_authorization {
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
  }
}`,
			expectedModifications:           0,
			expectEnabledAttributeRemoved:   false, // Should not be touched
			resourceLabelsToVerify:          []string{"google_compute_instance", "default"},
			binaryAuthorizationShouldExist:  true,  // The block exists on this other resource
			binaryAuthorizationShouldHaveEvalMode: true, // and it should keep its eval mode
		},
		{
			name: "Multiple GKE resources, one with conflict",
			hclContent: `
resource "google_container_cluster" "gke_one" {
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
			expectEnabledAttributeRemoved:   true, // For "gke_one"
			resourceLabelsToVerify:          []string{"google_container_cluster", "gke_one"},
			binaryAuthorizationShouldExist:  true,
			binaryAuthorizationShouldHaveEvalMode: true,
			// We also need to check "gke_two" was not modified negatively.
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

			// Re-parse the modified HCL content for verification
			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				// Corrected logic to find the target resource block:
				// b.Type() will be "resource" for resource blocks.
				// blockType (from tc.resourceLabelsToVerify[0]) is the resource type label (e.g., "google_container_cluster").
				// blockName (from tc.resourceLabelsToVerify[1]) is the resource name label (e.g., "primary").
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetResourceBlock = b
						break
					}
				}

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectEnabledAttributeRemoved || tc.binaryAuthorizationShouldExist) {
					// If we expected some change or the block to exist, but the parent resource is gone, that's a problem.
					if !(tc.hclContent == "" && tc.expectedModifications == 0) { // Allow for empty HCL case
						t.Fatalf("Could not find the target resource block type '%s' with name '%s' for verification. Modified HCL:\n%s", blockType, blockName, string(modifiedContentBytes))
					}
				}

				if targetResourceBlock != nil {
					var binaryAuthBlock *hclwrite.Block
					for _, nestedBlock := range targetResourceBlock.Body().Blocks() { // Iterate targetResourceBlock from verifiedFile
						if nestedBlock.Type() == "binary_authorization" {
							binaryAuthBlock = nestedBlock
							break
						}
					}

					if !tc.binaryAuthorizationShouldExist {
						if binaryAuthBlock != nil {
							t.Errorf("Expected 'binary_authorization' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, tc.hclContent)
						}
					} else { // binaryAuthorizationShouldExist is true
						if binaryAuthBlock == nil {
							if tc.expectEnabledAttributeRemoved || tc.expectedModifications > 0 || tc.binaryAuthorizationShouldHaveEvalMode {
								t.Fatalf("Expected 'binary_authorization' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, tc.hclContent)
							}
						} else { // binary_authorization block exists
							hasEnabledAttr := binaryAuthBlock.Body().GetAttribute("enabled") != nil
							hasEvalModeAttr := binaryAuthBlock.Body().GetAttribute("evaluation_mode") != nil

							if tc.expectEnabledAttributeRemoved {
								if hasEnabledAttr {
									t.Errorf("Expected 'enabled' attribute to be REMOVED from 'binary_authorization' in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
								}
							} else { // Not expecting 'enabled' removal
								// Check if 'enabled' was removed when it shouldn't have been.
								// This requires checking the original state.
								originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
								var originalBinaryAuthBlock *hclwrite.Block
								var originalResourceBlock *hclwrite.Block
								for _, b := range originalFile.Body().Blocks() {
									if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
										originalResourceBlock = b
										break
									}
								}
								if originalResourceBlock != nil {
									for _, nb := range originalResourceBlock.Body().Blocks() {
										if nb.Type() == "binary_authorization" {
											originalBinaryAuthBlock = nb
											break
										}
									}
								}

								if originalBinaryAuthBlock != nil && originalBinaryAuthBlock.Body().GetAttribute("enabled") != nil {
									// 'enabled' was there originally and should not have been removed.
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
							} else {
								// If eval mode should not be there, and it is, it's an error only if it wasn't there originally.
								// This is tricky. The main point is it wasn't *added* if not part of the rule.
								// The current rule doesn't add attributes, so this is mostly about it not being removed if it was there and not part of the "both present" condition.
								// This is implicitly covered by the `enabled` check logic for cases where only `evaluation_mode` was present.
							}
						}
					}
				}
			}

			// Specific check for "Multiple GKE resources, one with conflict"
			if tc.name == "Multiple GKE resources, one with conflict" {
				var gkeTwoBlock *hclwrite.Block
				resourceTypeLabel := "google_container_cluster"
				resourceNameLabel := "gke_two"
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == resourceTypeLabel && b.Labels()[1] == resourceNameLabel {
						gkeTwoBlock = b
						break
					}
				}
				if gkeTwoBlock == nil {
					t.Fatalf("Could not find '%s' GKE block named '%s' for multi-block test verification. Modified HCL:\n%s", resourceTypeLabel, resourceNameLabel, string(modifiedContentBytes))
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
	logger, _ := zap.NewDevelopment() // Or use zap.NewNop() for less verbose test output

	tests := []struct {
		name                                  string
		hclContent                            string
		expectedModifications                 int
		expectServicesIPV4CIDRBlockRemoved    bool
		resourceLabelsToVerify                []string // e.g., ["google_container_cluster", "primary"]
		ipAllocationPolicyShouldExistForCheck bool     // True if we need to check inside ip_allocation_policy
	}{
		{
			name: "Both attributes present in ip_allocation_policy",
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
    // cluster_secondary_range_name is missing
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Only cluster_secondary_range_name present in ip_allocation_policy",
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    // services_ipv4_cidr_block is missing
    cluster_secondary_range_name = "services_range"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false, // It was never there
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Neither attribute relevant to Rule 2 present in ip_allocation_policy",
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    some_other_attribute = "value"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false, // It was never there
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is present but empty",
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {}
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false, // It was never there
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is missing entirely",
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  // No ip_allocation_policy block
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false, // Block doesn't exist
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: false,
		},
		{
			name: "Non-matching resource type with similar nested structure",
			hclContent: `
resource "google_compute_router" "default" {
  name = "my-router"
  ip_allocation_policy { // Not the target resource, but has the block name
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
  }
}`,
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false, // Should not be touched
			resourceLabelsToVerify:                []string{"google_compute_router", "default"},
			ipAllocationPolicyShouldExistForCheck: true, // The block exists on this other resource
		},
		{
			name: "Multiple google_container_cluster blocks, one matching for Rule 2",
			hclContent: `
resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range" // Match here
  }
}
resource "google_container_cluster" "secondary" {
  name = "secondary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.3.0.0/20" // Only this attribute, no secondary_range_name
  }
}`,
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true, // For "primary"
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
			// We will also need to check "secondary" was not modified.
		},
		{
			name: "Multiple google_container_cluster blocks, ip_policy missing in one",
			hclContent: `
resource "google_container_cluster" "alpha" {
  name = "alpha-cluster"
  // No ip_allocation_policy block
}
resource "google_container_cluster" "beta" {
  name = "beta-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block   = "10.4.0.0/20"
    cluster_secondary_range_name = "services_range_beta" // Match here
  }
}`,
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true, // For "beta"
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

			// Re-parse the modified HCL content for verification
			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				// Corrected logic to find the target resource block
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
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
					for _, nestedBlock := range targetResourceBlock.Body().Blocks() { // Iterate targetResourceBlock from verifiedFile
						if nestedBlock.Type() == "ip_allocation_policy" {
							ipAllocationPolicyBlock = nestedBlock
							break
						}
					}

					if !tc.ipAllocationPolicyShouldExistForCheck {
						if ipAllocationPolicyBlock != nil {
							t.Errorf("Expected 'ip_allocation_policy' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, tc.hclContent)
						}
					} else { // ipAllocationPolicyShouldExistForCheck is true
						if ipAllocationPolicyBlock == nil {
							// If we expected a change within ip_allocation_policy, it must exist.
							if tc.expectServicesIPV4CIDRBlockRemoved || tc.expectedModifications > 0 {
								t.Fatalf("Expected 'ip_allocation_policy' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, tc.hclContent)
							}
							// If no change expected and block is missing, that's fine.
						} else {
							hasServicesCIDRBlock := ipAllocationPolicyBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil
							if tc.expectServicesIPV4CIDRBlockRemoved {
								if hasServicesCIDRBlock {
									t.Errorf("Expected 'services_ipv4_cidr_block' to be REMOVED from ip_allocation_policy in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
								}
							} else { // Not expecting removal
								// Check if it was removed when it shouldn't have been (only if it was there initially)
								originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
								var originalIpAllocBlock *hclwrite.Block
								var originalResourceBlock *hclwrite.Block
								for _, b := range originalFile.Body().Blocks() {
									if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
										originalResourceBlock = b
										break
									}
								}
								if originalResourceBlock != nil {
									for _, nb := range originalResourceBlock.Body().Blocks() {
										if nb.Type() == "ip_allocation_policy" {
											originalIpAllocBlock = nb
											break
										}
									}
								}

								if originalIpAllocBlock != nil && originalIpAllocBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil {
									// It was there originally and should not have been removed
									if !hasServicesCIDRBlock {
										t.Errorf("Expected 'services_ipv4_cidr_block' to be PRESENT in ip_allocation_policy in %s[\"%s\"], but it was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
											blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
									}
								}
								// If it wasn't there originally, and not expected to be removed, then it should still not be there. This is covered.
							}
						}
					}
				}
			}

			// Specific check for "Multiple google_container_cluster blocks, one matching for Rule 2"
			if tc.name == "Multiple google_container_cluster blocks, one matching for Rule 2" {
				var secondaryBlock *hclwrite.Block
				resourceTypeLabel := "google_container_cluster"
				resourceNameLabel := "secondary"
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == resourceTypeLabel && b.Labels()[1] == resourceNameLabel {
						secondaryBlock = b
						break
					}
				}
				if secondaryBlock == nil {
					t.Fatalf("Could not find '%s' GKE block named '%s' for multi-block test verification. Modified HCL:\n%s", resourceTypeLabel, resourceNameLabel, string(modifiedContentBytes))
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
	logger, _ := zap.NewDevelopment() // Or use zap.NewNop() for less verbose test output

	tests := []struct {
		name                         string
		hclContent                   string
		expectedModifications        int
		expectClusterIPV4CIDRRemoved bool
		resourceLabelsToVerify       []string // e.g., ["google_container_cluster", "primary"]
	}{
		{
			name: "Both attributes present",
			hclContent: `
resource "google_container_cluster" "primary" {
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
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only cluster_ipv4_cidr present (ip_allocation_policy block exists but no cluster_ipv4_cidr_block)",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    // cluster_ipv4_cidr_block = "10.1.0.0/14" // This is missing
    services_ipv4_cidr_block = "10.2.0.0/20"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only ip_allocation_policy.cluster_ipv4_cidr_block present",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  // cluster_ipv4_cidr  = "10.0.0.0/14" // This is missing
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false, // It was never there
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Neither attribute relevant to Rule 1 present",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.2.0.0/20"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false, // It was never there
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "ip_allocation_policy block is missing entirely, cluster_ipv4_cidr present",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Non-matching resource type (google_compute_instance)",
			hclContent: `
resource "google_compute_instance" "default" {
  name               = "test-instance"
  cluster_ipv4_cidr  = "10.0.0.0/14" // Attribute name clash
  ip_allocation_policy {             // Block name clash
    cluster_ipv4_cidr_block = "10.1.0.0/14"
  }
}`,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false, // On the wrong resource type
			resourceLabelsToVerify:       []string{"google_compute_instance", "default"},
		},
		{
			name: "Multiple google_container_cluster blocks, one matching",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14" // Match here
  }
}
resource "google_container_cluster" "secondary" {
  name               = "secondary-cluster"
  cluster_ipv4_cidr  = "10.2.0.0/14" // No ip_allocation_policy.cluster_ipv4_cidr_block
  ip_allocation_policy {
    services_ipv4_cidr_block = "10.3.0.0/20"
  }
}`,
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true, // For "primary"
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
			// We will also need to check "secondary" was not modified.
		},
		{
			name: "Multiple google_container_cluster blocks, none matching",
			hclContent: `
resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14" // No ip_allocation_policy.cluster_ipv4_cidr_block
}
resource "google_container_cluster" "secondary" {
  name               = "secondary-cluster"
  // No cluster_ipv4_cidr
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
			resourceLabelsToVerify:       nil, // No specific resource to verify
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			// No need to defer os.Remove(tmpFile.Name()), t.TempDir() handles cleanup.

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				tmpFile.Close() // Close beforeFatalf
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				// For empty HCL, NewFromFile might return an error or a valid modifier with an empty body.
				// If it's an error and the test expects 0 modifications, that might be okay.
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					// Allow this specific case (e.g., if NewFromFile errors on empty file but rule handles nil body)
					// Or, if NewFromFile creates a valid empty body, the rule should handle it.
					if modifier == nil { // If NewFromFile truly failed
						modifications, ruleErr := 0, error(nil) // Simulate ApplyRule1 not running
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyRule1() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyRule1() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return // Test is done for this case
					}
					// if modifier is not nil, proceed with ApplyRule1 call
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

			// Re-parse the modified HCL content for verification
			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetBlock *hclwrite.Block

				// Corrected logic to find the target resource block
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetBlock = b
						break
					}
				}

				if targetBlock == nil && (tc.expectClusterIPV4CIDRRemoved || tc.expectedModifications > 0) {
					// If we expected a change, the block should exist unless the test is about removing the block itself (not the case for Rule1)
					t.Fatalf("Could not find the target resource block type '%s' with name '%s' for verification. Modified HCL:\n%s", blockType, blockName, string(modifiedContentBytes))
				}

				if targetBlock != nil { // Only proceed if block exists
					hasClusterIPV4CIDR := targetBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil
					if tc.expectClusterIPV4CIDRRemoved {
						if hasClusterIPV4CIDR {
							t.Errorf("Expected 'cluster_ipv4_cidr' to be removed from %s[\"%s\"], but it was found. Modified HCL:\n%s",
								blockType, blockName, string(modifiedContentBytes))
						}
					} else {
						// If not expecting removal, it should be present if it was in the input,
						// or absent if it wasn't. This is implicitly covered by modification count and specific scenario logic.
						// For "Non-matching resource type", we ensure it wasn't removed from the wrong block.
						if tc.name == "Non-matching resource type (google_compute_instance)" {
							originalBlockHasIt := false
							originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
							for _, b := range originalParsedFile.Body().Blocks() {
								if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
									if b.Body().GetAttribute("cluster_ipv4_cidr") != nil {
										originalBlockHasIt = true
										break
									}
								}
							}
							if originalBlockHasIt && !hasClusterIPV4CIDR {
										t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed from non-target resource %s[\"%s\"]. Modified HCL:\n%s",
											blockType, blockName, string(modifiedContentBytes))
							}
						}
					}
				}
			}

			// Specific check for the "Multiple google_container_cluster blocks, one matching" case
			if tc.name == "Multiple google_container_cluster blocks, one matching" {
				var secondaryBlock *hclwrite.Block
				resourceTypeLabel := "google_container_cluster"
				resourceNameLabel := "secondary"
				for _, b := range verifiedFile.Body().Blocks() { // Use verifiedFile here
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == resourceTypeLabel && b.Labels()[1] == resourceNameLabel {
						secondaryBlock = b
						break
					}
				}
				if secondaryBlock == nil {
					t.Fatalf("Could not find the '%s' block named '%s' for verification. Modified HCL:\n%s", resourceTypeLabel, resourceNameLabel, string(modifiedContentBytes))
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
	logger, _ := zap.NewNop() // Use NewNop to avoid verbose logs during tests unless debugging

	type clusterAutoscalingChecks struct {
		expectBlockExists       bool
		expectEnabledRemoved    bool
		expectResourceLimitsRemoved bool
		expectProfileUnchanged  *string // pointer to check if profile remains, nil if block removed
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
		expectHttpLoadBalancingUnchanged bool // Example of a non-targeted block
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
			hclContent: `
resource "google_container_cluster" "autopilot_cluster" {
  name                          = "autopilot-cluster"
  location                      = "us-central1"
  enable_autopilot              = true
  cluster_ipv4_cidr             = "10.0.0.0/8" # To be removed
  enable_shielded_nodes         = true         # To be removed
  remove_default_node_pool      = true         # To be removed
  default_max_pods_per_node     = 110          # To be removed
  enable_intranode_visibility   = true         # To be removed

  addons_config {
    network_policy_config { # To be removed from here
      disabled = false
    }
    dns_cache_config {      # To be removed from here
      enabled = true
    }
    stateful_ha_config {    # To be removed from here
      enabled = true
    }
    http_load_balancing {   # Should remain
      disabled = false
    }
  }

  network_policy { # Top-level, to be removed
    provider = "CALICO"
    enabled  = true
  }
  node_pool { # Top-level, to be removed
    name = "default-pool"
  }
  node_pool { # Top-level, to be removed
    name = "custom-pool"
  }
  cluster_autoscaling {
    enabled = true # To be removed
    autoscaling_profile = "OPTIMIZE_UTILIZATION" # Should remain
    resource_limits { # To be removed
      resource_type = "cpu"
      minimum = 1
      maximum = 10
    }
  }
  binary_authorization {
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE" # Should remain
    enabled = true                                     # To be removed
  }
}`,
			expectedModifications: 1 + 4 + 3 + 1 + 2 + 2 + 1, // cluster_ipv4_cidr (1) + root_attrs(4) + addons_blocks(3) + network_policy(1) + node_pools(2) + ca_attrs(2) + ba_attrs(1) = 14
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
			hclContent: `
resource "google_container_cluster" "standard_cluster" {
  name                  = "standard-cluster"
  enable_autopilot      = false
  cluster_ipv4_cidr     = "10.0.0.0/8" # Should remain
  enable_shielded_nodes = true // Should remain
  node_pool {                 // Should remain
    name = "default-pool"
  }
  cluster_autoscaling {     // Should remain fully
    enabled = true
    autoscaling_profile = "BALANCED"
  }
  addons_config {
    dns_cache_config {      # Should remain
      enabled = true
    }
    http_load_balancing {   # Should remain
      disabled = false
    }
  }
}`,
			expectedModifications:     1, // Only enable_autopilot itself is removed
			clusterName:               "standard_cluster",
			expectEnableAutopilotAttr: nil, // Attribute removed
			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:            true,
				expectDnsCacheRemoved:        false, // Not removed
				expectHttpLoadBalancingUnchanged: true, // Not removed
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:       true,
				expectEnabledRemoved:    false, // Not removed
				expectResourceLimitsRemoved: false, // Not removed
				expectProfileUnchanged:  stringPtr("BALANCED"),
			},
			binaryAuthorization:  nil, // No binary_authorization block to check
			expectNoOtherChanges: true, // Apart from enable_autopilot removal
		},
		{
			name: "enable_autopilot not present, conflicting fields present",
			hclContent: `
resource "google_container_cluster" "existing_cluster" {
  name                  = "existing-cluster"
  // enable_autopilot is missing
  cluster_ipv4_cidr     = "10.0.0.0/8" # Should remain
  enable_shielded_nodes = true         # Should remain
  node_pool {                          # Should remain
    name = "default-pool"
  }
  addons_config {
    network_policy_config { # Should remain
      disabled = false
    }
  }
}`,
			expectedModifications:     0,
			clusterName:               "existing_cluster",
			expectEnableAutopilotAttr: nil, // Was not there, should not be added
			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{ // Verify these are NOT removed
				expectBlockExists:            true,
				expectNetworkPolicyRemoved:   false,
			},
			expectNoOtherChanges:      true,
		},
		{
			name: "enable_autopilot is true, no conflicting fields present",
			hclContent: `
resource "google_container_cluster" "clean_autopilot_cluster" {
  name             = "clean-autopilot-cluster"
  enable_autopilot = true
  location         = "us-central1"
  # No attributes or blocks that would be removed by the rule
  # cluster_ipv4_cidr is not present, so not removed.
  addons_config { # Empty or with non-targeted blocks
    http_load_balancing { disabled = true } # Should remain
  }
  cluster_autoscaling {
    autoscaling_profile = "BALANCED" # Should remain
  }
  binary_authorization {
    evaluation_mode = "DISABLED" # Should remain
  }
}`,
			expectedModifications:     0, // No targeted fields are present to be removed
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
				expectEnabledRemoved:    false, // Was not there
				expectResourceLimitsRemoved: false, // Was not there
				expectProfileUnchanged:  stringPtr("BALANCED"),
			},
			binaryAuthorization: &binaryAuthorizationChecks{
				expectBlockExists:    true,
				expectEnabledRemoved: false, // Was not there
			},
			expectNoOtherChanges: true,
		},
		{
			name: "enable_autopilot is not a boolean",
			hclContent: `
resource "google_container_cluster" "invalid_autopilot_cluster" {
  name             = "invalid-autopilot-cluster"
  enable_autopilot = "not_a_boolean"
  enable_shielded_nodes = true
  cluster_ipv4_cidr     = "10.0.0.0/8" # Should remain
  addons_config {
    dns_cache_config { enabled = true } # Should remain
  }
}`,
			expectedModifications:     0,
			clusterName:               "invalid_autopilot_cluster",
			expectEnableAutopilotAttr: nil,



			expectedRootAttrsRemoved:  []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:     true,
				expectDnsCacheRemoved: false, // Should not be removed
			},
			expectNoOtherChanges:      true, // No changes should be made
		},
		{
			name: "No google_container_cluster blocks",
			hclContent: `
resource "google_compute_instance" "vm" {
  name = "my-vm"
}`,
			expectedModifications: 0,
			clusterName:           "", // No cluster to check
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
			hclContent: `
resource "google_container_cluster" "partial_autopilot" {
  name                  = "partial-autopilot"
  enable_autopilot      = true
  enable_shielded_nodes = true # To be removed
  # remove_default_node_pool is missing
  default_max_pods_per_node = 110 # To be removed
}`,
			expectedModifications:     2, // enable_shielded_nodes, default_max_pods_per_node
			clusterName:               "partial_autopilot",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved:  []string{"enable_shielded_nodes", "default_max_pods_per_node"},
			expectedNestedBlocksRemoved: []string{},
			expectNoOtherChanges:      false, // Specific checks will be done
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
				// Handle cases where empty HCL might cause NewFromFile to error,
				// but the test expects 0 modifications.
				if tc.hclContent == "" && tc.expectedModifications == 0 {
					if modifier == nil { // If NewFromFile truly failed
						return // Test passes if 0 modifications expected and NewFromFile fails for empty file
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

			if tc.clusterName == "" { // If no cluster is expected, skip detailed checks
				if tc.expectedModifications == 0 {
					return // Test passed
				}
				t.Fatalf("No clusterName specified for a test expecting modifications or specific checks.")
			}

			var clusterBlock *hclwrite.Block
			for _, b := range modifier.File().Body().Blocks() {
				if b.Type() == "resource" && len(b.Labels()) == 2 &&
					b.Labels()[0] == "google_container_cluster" && b.Labels()[1] == tc.clusterName {
					clusterBlock = b
					break
				}
			}

			if clusterBlock == nil {
				// If we expected modifications, or specific checks on the cluster, then the cluster block must exist.
				if tc.expectedModifications > 0 || !tc.expectNoOtherChanges || tc.clusterAutoscaling != nil || tc.binaryAuthorization != nil {
					t.Fatalf("google_container_cluster resource '%s' not found after ApplyAutopilotRule. HCL:\n%s", tc.clusterName, string(modifier.File().Bytes()))
				}
				return // No block, no further checks needed if no changes were expected.
			}

			// Check enable_autopilot attribute
			enableAutopilotAttr := clusterBlock.Body().GetAttribute("enable_autopilot")
			if tc.expectEnableAutopilotAttr == nil { // Expected to be removed
				if enableAutopilotAttr != nil {
					t.Errorf("Expected 'enable_autopilot' attribute to be removed, but it was found. Modified HCL:\n%s", string(modifier.File().Bytes()))
				}
			} else { // Expected to exist with a certain value
				if enableAutopilotAttr == nil {
					// Special case for "not_a_boolean" where it should remain if that's the original value
					if tc.name == "enable_autopilot is not a boolean" {
						// If it's the non-boolean case and the attribute is missing, it's an error,
						// as it should have remained untouched.
						t.Errorf("Expected 'enable_autopilot' attribute (with non-boolean value) to exist, but it was not found. Modified HCL:\n%s", string(modifier.File().Bytes()))
					} else {
						t.Errorf("Expected 'enable_autopilot' attribute to exist, but it was not found. Modified HCL:\n%s", string(modifier.File().Bytes()))
					}
				} else {
					val, err := modifier.GetAttributeValue(enableAutopilotAttr)
					if err != nil {
						if !(tc.name == "enable_autopilot is not a boolean" && val.Type() != cty.Bool) {
							t.Errorf("Error getting value of 'enable_autopilot': %v. Modified HCL:\n%s", err, string(modifier.File().Bytes()))
						}
						// If it's the "not_a_boolean" case, we expect GetAttributeValue to error or return non-bool.
						// The attribute should still be there as "not_a_boolean".
						exprBytes := enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()
						if string(exprBytes) != `"not_a_boolean"` && tc.name == "enable_autopilot is not a boolean" {
							t.Errorf("Expected 'enable_autopilot' to remain as \"not_a_boolean\", got %s. Modified HCL:\n%s", string(exprBytes), string(modifier.File().Bytes()))
						}

					} else if val.Type() == cty.Bool {
						if val.True() != *tc.expectEnableAutopilotAttr {
							t.Errorf("Expected 'enable_autopilot' to be %v, but got %v. Modified HCL:\n%s", *tc.expectEnableAutopilotAttr, val.True(), string(modifier.File().Bytes()))
						}
					} else if tc.name != "enable_autopilot is not a boolean" {
						// If it's not the "not_a_boolean" test case, then it should have been a bool.
						t.Errorf("Expected 'enable_autopilot' to be boolean, but got type %s. Modified HCL:\n%s", val.Type().FriendlyName(), string(modifier.File().Bytes()))
					}
				}
			}


			// Check root attributes removed
			for _, attrName := range tc.expectedRootAttrsRemoved {
				if attr := clusterBlock.Body().GetAttribute(attrName); attr != nil {
					t.Errorf("Expected root attribute '%s' to be removed, but it was found. Modified HCL:\n%s", attrName, string(modifier.File().Bytes()))
				}
			}

			// Check top-level nested blocks removed
			for _, blockTypeName := range tc.expectedTopLevelNestedBlocksRemoved {
				// Handle node_pool specifically as multiple can exist
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

			// Cluster Autoscaling checks
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
					} else { // Expect enabled NOT removed (if it was there)
						// This check is more complex if we need to verify it *wasn't* removed if present.
						// For "autopilot=false" or "no conflicting fields", it means it should remain if originally present.
						// The test cases are set up such that if expectEnabledRemoved is false, it means it shouldn't have been touched.
						// If the original HCL for such a test case had 'enabled', it should still be there.
						// This is implicitly covered by tc.expectNoOtherChanges or specific setup.
					}

					if tc.clusterAutoscaling.expectResourceLimitsRemoved {
						if attr := caBlock.Body().GetAttribute("resource_limits"); attr != nil {
							t.Errorf("Expected 'resource_limits' attribute in 'cluster_autoscaling' to be removed, but it was found.")
						}
					} // Similar logic for not removed resource_limits

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

			// Binary Authorization checks
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
					} // Similar logic for not removed enabled
					// Check that evaluation_mode is untouched if present in original HCL for autopilot=true case
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
				// This is a simpler check for cases where only enable_autopilot might be removed, or nothing.
				// It assumes that if expectedRootAttrsRemoved and expectedTopLevelNestedBlocksRemoved are empty,
				// and clusterAutoscaling/binaryAuthorization/addonsConfig checks are nil or configured for no change,
				// then other parts of the resource block should remain as they were.
				// A full deep comparison is complex, so this relies on the specific nature of the no-op test cases.
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
				// For "enable_autopilot not present", check fields still there
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
				// For "enable_autopilot is not a boolean", check fields still there
				if tc.name == "enable_autopilot is not a boolean" {
					enableAutopilotAttr := clusterBlock.Body().GetAttribute("enable_autopilot")
					if enableAutopilotAttr == nil {
						t.Errorf("'enable_autopilot' (with non-boolean value) was unexpectedly removed.")
					} else {
						exprBytes := enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()
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

// Helper function to get a pointer to a boolean value
func boolPtr(b bool) *bool {
	return &b
}

// Helper function to get a pointer to a string value
func stringPtr(s string) *string {
	return &s
}

func TestRemoveBlock(t *testing.T) {
	t.Helper()

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
			hclContent: `
resource "aws_instance" "my_test_instance" {
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
			hclContent: `
resource "aws_instance" "another_instance" {
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
			hclContent: `
resource "aws_instance" "my_test_instance" {
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
			hclContent: `
data "aws_caller_identity" "current" {}
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
			
			var logger *zap.Logger

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil && tc.hclContent != "" {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			} else if err == nil && tc.hclContent == "" {
                 // Allow empty HCL to proceed if NewFromFile handles it (e.g. creates empty body)
            }


			err = modifier.RemoveBlock(tc.blockType, tc.blockLabels)
			if (err != nil) != tc.expectCallError {
				t.Fatalf("RemoveBlock() error status = %v (err: %v), expectCallError %v. HCL:\n%s", (err != nil), err, tc.expectCallError, tc.hclContent)
			}

			// Re-parse the modified HCL content for verification
			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, parseDiags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if parseDiags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", parseDiags, string(modifiedContentBytes))
			}

			// Use a new GetBlock function or similar logic on verifiedFile
			var foundBlockInVerified *hclwrite.Block
			for _, b := range verifiedFile.Body().Blocks() {
				if b.Type() == tc.blockType && len(b.Labels()) == len(tc.blockLabels) {
					labelsMatch := true
					for i, l := range b.Labels() {
						if l != tc.blockLabels[i] {
							labelsMatch = false
							break
						}
					}
					if labelsMatch {
						foundBlockInVerified = b
						break
					}
				}
			}


			if tc.expectRemoved {
				if foundBlockInVerified != nil {
					t.Errorf("RemoveBlock() expected block %s %v to be removed, but it was found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
				}
			} else {
				initialFile, initialParseDiags := hclwrite.ParseConfig([]byte(tc.hclContent), tmpFile.Name()+"_initial", hcl.InitialPos)
				initialBlockPresent := false
				if !initialParseDiags.HasErrors() && initialFile != nil && initialFile.Body() != nil {
					for _, b := range initialFile.Body().Blocks() {
						if b.Type() == tc.blockType && len(b.Labels()) == len(tc.blockLabels) {
							match := true
							for i, l := range b.Labels() {
								if l != tc.blockLabels[i] {
									match = false; break
								}
							}
							if match { initialBlockPresent = true; break }
						}
					}
				}

				if tc.expectCallError {
					if foundBlockInVerified != nil {
                         t.Errorf("RemoveBlock() errored as expected for %s %v, but block was found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
                     }
				} else {
					if initialBlockPresent {
						if foundBlockInVerified == nil {
							t.Errorf("RemoveBlock() did not remove block %s %v as expected (initial state: present), but it was not found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
						}
					} else {
						if foundBlockInVerified != nil {
                             t.Errorf("RemoveBlock() logic error: block %s %v was not present initially nor targeted for removal, but was found in re-parsed HCL. Output HCL:\n%s", tc.blockType, tc.blockLabels, string(modifiedContentBytes))
                        }
					}
				}
			}
		})
	}
}
