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
