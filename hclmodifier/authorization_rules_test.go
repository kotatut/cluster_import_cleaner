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

func TestApplyBinaryAuthorizationRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                                  string
		hclContentFile                        string
		expectedModifications                 int
		expectEnabledAttributeRemoved         bool
		resourceLabelsToVerify                []string
		binaryAuthorizationShouldExist        bool
		binaryAuthorizationShouldHaveEvalMode bool
	}{
		{
			name:                                  "Both enabled and evaluation_mode present",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_BothPresent.tf",
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Only enabled present",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_OnlyEnabled.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name:                                  "Only evaluation_mode present",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_OnlyEvaluationMode.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Neither enabled nor evaluation_mode present",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_NeitherPresent.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name:                                  "binary_authorization block present but empty",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_EmptyBlock.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name:                                  "binary_authorization block missing entirely",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_NoBlock.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name:                                  "Non-matching resource type with binary_authorization",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_NonMatchingResource.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_compute_instance", "default"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Multiple GKE resources, one with conflict",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_MultipleResourcesOneMatch.tf",
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "gke_one"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Empty HCL content",
			hclContentFile:                        "testdata/TestApplyBinaryAuthorizationRule_EmptyFile.tf",
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                nil,
			binaryAuthorizationShouldExist:        false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hclContent, err := os.ReadFile(tc.hclContentFile)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tc.hclContentFile, err)
			}

			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_rule3_*.hcl")
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
							t.Errorf("ApplyBinaryAuthorizationRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyBinaryAuthorizationRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.BinaryAuthorizationRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(BinaryAuthorizationRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(BinaryAuthorizationRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
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

				if targetResourceBlock == nil && (tc.expectedModifications > 0 || tc.expectEnabledAttributeRemoved || tc.binaryAuthorizationShouldExist) {
					if !(string(hclContent) == "" && tc.expectedModifications == 0) {
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
							t.Errorf("Expected 'binary_authorization' block NOT to exist for %s[\"%s\"], but it was found. HCL:\n%s", blockType, blockName, string(hclContent))
						}
					} else {
						if binaryAuthBlock == nil {
							if tc.expectEnabledAttributeRemoved || tc.expectedModifications > 0 || tc.binaryAuthorizationShouldHaveEvalMode {
								t.Fatalf("Expected 'binary_authorization' block for %s[\"%s\"], but it was not found. HCL:\n%s", blockType, blockName, string(hclContent))
							}
						} else {
							hasEnabledAttr := binaryAuthBlock.Body().GetAttribute("enabled") != nil
							hasEvalModeAttr := binaryAuthBlock.Body().GetAttribute("evaluation_mode") != nil

							if tc.expectEnabledAttributeRemoved {
								if hasEnabledAttr {
									t.Errorf("Expected 'enabled' attribute to be REMOVED from 'binary_authorization' in %s[\"%s\"], but it was FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, string(hclContent), string(modifier.File().Bytes()))
								}
							} else {
								originalParsedFile, _ := hclwrite.ParseConfig(hclContent, "", hcl.InitialPos)
								originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
								var originalBinaryAuthBlock *hclwrite.Block
								if originalResourceBlock != nil {
									originalBinaryAuthBlock = originalResourceBlock.Body().FirstMatchingBlock("binary_authorization", nil)
								}

								if originalBinaryAuthBlock != nil && originalBinaryAuthBlock.Body().GetAttribute("enabled") != nil {
									if !hasEnabledAttr {
										t.Errorf("Expected 'enabled' attribute to be PRESENT in 'binary_authorization' in %s[\"%s\"], but it was NOT FOUND (removed). HCL:\n%s\nModified HCL:\n%s",
											blockType, blockName, string(hclContent), string(modifier.File().Bytes()))
									}
								}
							}

							if tc.binaryAuthorizationShouldHaveEvalMode {
								if !hasEvalModeAttr {
									t.Errorf("Expected 'evaluation_mode' attribute to be PRESENT in 'binary_authorization' in %s[\"%s\"], but it was NOT FOUND. HCL:\n%s\nModified HCL:\n%s",
										blockType, blockName, string(hclContent), string(modifier.File().Bytes()))
								}
							}
						}
					}
				}
			}
		})
	}
}
