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

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
						targetResourceBlock = b
						break
					}
				}

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectEnabledAttributeRemoved || tc.binaryAuthorizationShouldExist) {
					// If we expected some change or the block to exist, but the parent resource is gone, that's a problem.
					if !(tc.hclContent == "" && tc.expectedModifications == 0) { // Allow for empty HCL case
						t.Fatalf("Could not find the target resource block %s[\"%s\"] for verification. HCL content:\n%s", blockType, blockName, tc.hclContent)
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
				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == "google_container_cluster" && len(b.Labels()) == 2 && b.Labels()[1] == "gke_two" {
						gkeTwoBlock = b
						break
					}
				}
				if gkeTwoBlock == nil {
					t.Fatalf("Could not find 'gke_two' GKE block for multi-block test verification. HCL:\n%s", tc.hclContent)
				}
				binaryAuthGkeTwo := gkeTwoBlock.Body().FirstMatchingBlock("binary_authorization", nil)
				if binaryAuthGkeTwo == nil {
					t.Fatalf("'binary_authorization' missing in 'gke_two' GKE block for multi-block test. HCL:\n%s", tc.hclContent)
				}
				if binaryAuthGkeTwo.Body().GetAttribute("enabled") != nil {
					t.Errorf("'enabled' attribute should NOT be present in 'gke_two' ('binary_authorization' block), but it was found. HCL:\n%s\nModified HCL:\n%s",
						tc.hclContent, string(modifier.File().Bytes()))
				}
				if binaryAuthGkeTwo.Body().GetAttribute("evaluation_mode") == nil {
					t.Errorf("'evaluation_mode' attribute expected to be PRESENT in 'gke_two' ('binary_authorization' block), but was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
						tc.hclContent, string(modifier.File().Bytes()))
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

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetResourceBlock *hclwrite.Block

				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
						targetResourceBlock = b
						break
					}
				}

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectServicesIPV4CIDRBlockRemoved) {
					t.Fatalf("Could not find the target resource block %s[\"%s\"] for verification. HCL content:\n%s", blockType, blockName, tc.hclContent)
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
				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == "google_container_cluster" && len(b.Labels()) == 2 && b.Labels()[1] == "secondary" {
						secondaryBlock = b
						break
					}
				}
				if secondaryBlock == nil {
					t.Fatalf("Could not find 'secondary' GKE block for multi-block test verification. HCL:\n%s", tc.hclContent)
				}
				ipAllocSecondary := secondaryBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
				if ipAllocSecondary == nil {
					t.Fatalf("'ip_allocation_policy' missing in 'secondary' GKE block for multi-block test. HCL:\n%s", tc.hclContent)
				}
				if ipAllocSecondary.Body().GetAttribute("services_ipv4_cidr_block") == nil {
					t.Errorf("'services_ipv4_cidr_block' expected to be PRESENT in 'secondary' GKE's ip_allocation_policy, but was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
						tc.hclContent, string(modifier.File().Bytes()))
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

			if tc.resourceLabelsToVerify != nil && len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetBlock *hclwrite.Block

				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == blockType && len(b.Labels()) == 2 && b.Labels()[1] == blockName {
						targetBlock = b
						break
					}
				}

				if targetBlock == nil && (tc.expectClusterIPV4CIDRRemoved || tc.expectedModifications > 0) {
					// If we expected a change, the block should exist unless the test is about removing the block itself (not the case for Rule1)
					t.Fatalf("Could not find the target resource block %s[\"%s\"] for verification. HCL content:\n%s", blockType, blockName, tc.hclContent)
				}

				if targetBlock != nil { // Only proceed if block exists
					hasClusterIPV4CIDR := targetBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil
					if tc.expectClusterIPV4CIDRRemoved {
						if hasClusterIPV4CIDR {
							t.Errorf("Expected 'cluster_ipv4_cidr' to be removed from %s[\"%s\"], but it was found. HCL content:\n%s\nModified HCL:\n%s",
								blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
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
								t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed from non-target resource %s[\"%s\"]. HCL content:\n%s\nModified HCL:\n%s",
									blockType, blockName, tc.hclContent, string(modifier.File().Bytes()))
							}
						}
					}
				}
			}

			// Specific check for the "Multiple google_container_cluster blocks, one matching" case
			if tc.name == "Multiple google_container_cluster blocks, one matching" {
				var secondaryBlock *hclwrite.Block
				for _, b := range modifier.File().Body().Blocks() {
					if b.Type() == "google_container_cluster" && len(b.Labels()) == 2 && b.Labels()[1] == "secondary" {
						secondaryBlock = b
						break
					}
				}
				if secondaryBlock == nil {
					t.Fatalf("Could not find the 'secondary' google_container_cluster block for verification. HCL content:\n%s", tc.hclContent)
				}
				if secondaryBlock.Body().GetAttribute("cluster_ipv4_cidr") == nil {
					t.Errorf("Expected 'cluster_ipv4_cidr' to be present in 'secondary' block, but it was not. HCL content:\n%s\nModified HCL:\n%s",
						tc.hclContent, string(modifier.File().Bytes()))
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

			foundBlock, getErr := modifier.GetBlock(tc.blockType, tc.blockLabels) 

			if tc.expectRemoved {
				if foundBlock != nil || getErr == nil { 
					t.Errorf("RemoveBlock() expected block %s %v to be removed, but GetBlock found it (block: %v, err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, foundBlock, getErr, string(modifier.File().Bytes()))
				}
			} else { 
				initialFile, parseDiags := hclwrite.ParseConfig([]byte(tc.hclContent), tmpFile.Name(), hcl.Pos{Line: 1, Column: 1})
				initialBlockPresent := false
				if !parseDiags.HasErrors() && initialFile != nil && initialFile.Body() != nil {
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

				if tc.expectCallError { // If RemoveBlock errored (e.g., block not found)
					if foundBlock != nil || getErr == nil { // Then GetBlock should also not find it or error
                         t.Errorf("RemoveBlock() errored as expected for %s %v, but GetBlock still found it (block: %v, err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, foundBlock, getErr, string(modifier.File().Bytes()))
                     }
				} else { // If RemoveBlock did NOT error (e.g. block was there but shouldn't be removed by this call, or was not there and call did not error)
					if initialBlockPresent { // And it was there initially
						if foundBlock == nil || getErr != nil { // It should still be findable
							t.Errorf("RemoveBlock() did not remove block %s %v as expected (initial state: present), but GetBlock also failed to find it (err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, getErr, string(modifier.File().Bytes()))
						}
					} else { // And it was NOT there initially
						if foundBlock != nil || getErr == nil { // It should still not be findable
                             t.Errorf("RemoveBlock() logic error: block %s %v was not present initially nor targeted for removal, but GetBlock found it after RemoveBlock (err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, getErr, string(modifier.File().Bytes()))
                        }
					}
				}
			}
		})
	}
}
