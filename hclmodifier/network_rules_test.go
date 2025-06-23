package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

func TestApplyServicesIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                                  string
		hclContentFile                        string
		expectedModifications                 int
		expectServicesIPV4CIDRBlockRemoved    bool
		resourceLabelsToVerify                []string
		ipAllocationPolicyShouldExistForCheck bool
	}{
		{
			name:                                  "Both attributes present in ip_allocation_policy",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_BothPresent.tf",
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Only services_ipv4_cidr_block present in ip_allocation_policy",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_OnlyServicesCIDR.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Only cluster_secondary_range_name present in ip_allocation_policy",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_OnlySecondaryRange.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Neither attribute relevant to Rule 2 present in ip_allocation_policy",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_NeitherPresent.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "ip_allocation_policy block is present but empty",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_EmptyPolicy.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "ip_allocation_policy block is missing entirely",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_NoPolicy.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: false,
		},
		{
			name:                                  "Non-matching resource type with similar nested structure",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_NonMatchingResource.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_compute_router", "default"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Multiple google_container_cluster blocks, one matching for Rule 2",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_MultipleResourcesOneMatch.tf",
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Multiple google_container_cluster blocks, ip_policy missing in one",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_MultipleResourcesOneMissingPolicy.tf",
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "beta"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Empty HCL content",
			hclContentFile:                        "testdata/TestApplyServicesIPV4CIDRRule_EmptyFile.tf",
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hclContent, err := os.ReadFile(tc.hclContentFile)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tc.hclContentFile, err)
			}

			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_rule2_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write(hclContent); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if string(hclContent) == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						modifications, ruleErr := 0, error(nil)
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyServicesIPV4CIDRRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyServicesIPV4CIDRRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ServicesIPV4CIDRRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(ServicesIPV4CIDRRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(ServicesIPV4CIDRRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if len(tc.resourceLabelsToVerify) == 2 {
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
							t.Errorf("Expected 'ip_allocation_policy' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, string(hclContent))
						}
					} else {
						if ipAllocationPolicyBlock == nil {
							if tc.expectServicesIPV4CIDRBlockRemoved || tc.expectedModifications > 0 {
								t.Fatalf("Expected 'ip_allocation_policy' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, string(hclContent))
							}
						} else {
							hasServicesCIDRBlock := ipAllocationPolicyBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil
							if tc.expectServicesIPV4CIDRBlockRemoved {
								if hasServicesCIDRBlock {
									t.Errorf("Expected 'services_ipv4_cidr_block' to be REMOVED from ip_allocation_policy in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, string(hclContent), string(modifier.File().Bytes()))
								}
							} else {
								originalParsedFile, _ := hclwrite.ParseConfig(hclContent, "", hcl.InitialPos)
								originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
								var originalIpAllocBlock *hclwrite.Block
								if originalResourceBlock != nil {
									originalIpAllocBlock = originalResourceBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
								}

								if originalIpAllocBlock != nil && originalIpAllocBlock.Body().GetAttribute("services_ipv4_cidr_block") != nil {
									if !hasServicesCIDRBlock {
										t.Errorf("Expected 'services_ipv4_cidr_block' to be PRESENT in ip_allocation_policy in %s[\"%s\"], but it was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
											blockType, blockName, string(hclContent), string(modifier.File().Bytes()))
									}
								}
							}
						}
					}
				}
			}
		})
	}
}

func TestApplyClusterIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                         string
		hclContentFile               string
		expectedModifications        int
		expectClusterIPV4CIDRRemoved bool
		resourceLabelsToVerify       []string
	}{
		{
			name:                         "Both attributes present",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_BothPresent.tf",
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Only cluster_ipv4_cidr present (no ip_allocation_policy block)",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_OnlyClusterIPV4CIDRPresent_NoIpAllocationPolicy.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Only cluster_ipv4_cidr present (ip_allocation_policy block exists but no cluster_ipv4_cidr_block)",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_OnlyClusterIPV4CIDRPresent_IpAllocationPolicyExists.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Only ip_allocation_policy.cluster_ipv4_cidr_block present",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_OnlyIpAllocationPolicyPresent.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Neither attribute relevant to Rule 1 present",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_NeitherAttributePresent.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "ip_allocation_policy block is missing entirely, cluster_ipv4_cidr present",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_IpAllocationPolicyMissing.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Non-matching resource type (google_compute_instance)",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_NonMatchingResourceType.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_compute_instance", "default"},
		},
		{
			name:                         "Multiple google_container_cluster blocks, one matching",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_MultipleGKEResources_OneMatching.tf",
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Multiple google_container_cluster blocks, none matching",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_MultipleGKEResources_NoneMatching.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Empty HCL content",
			hclContentFile:               "testdata/TestApplyClusterIPV4CIDRRule_Empty.tf",
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hclContent, err := os.ReadFile(tc.hclContentFile)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tc.hclContentFile, err)
			}

			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write(hclContent); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				if string(hclContent) == "" && tc.expectedModifications == 0 {
					if modifier == nil {
						modifications, ruleErr := 0, error(nil)
						if modifications != tc.expectedModifications {
							t.Errorf("ApplyClusterIPV4CIDRRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyClusterIPV4CIDRRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ClusterIPV4CIDRRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(ClusterIPV4CIDRRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(ClusterIPV4CIDRRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, hclContent, string(modifier.File().Bytes()))
			}

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if len(tc.resourceLabelsToVerify) == 2 {
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
					}
				}
			}
		})
	}
}
