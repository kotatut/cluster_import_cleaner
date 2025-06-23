package hclmodifier

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

func TestEnhancedAttributeValueEquals(t *testing.T) {
	logger := zap.NewNop()
	tests := []struct {
		name                   string
		hclContent             string
		rule                   types.Rule
		expectedModifications  int
		expectAttributeRemoved bool     // Or other specific checks for your test
		attributePathToRemove  []string // Path to attribute that rule should remove
	}{
		{
			name: "AttributeValueEquals with boolean true",
			hclContent: `resource "google_container_cluster" "test" {
				enable_legacy_abac = true
				some_other_attr = "foo"
			}`,
			rule: types.Rule{
				Name:               "TestBoolEqualsTrue",
				TargetResourceType: "google_container_cluster",
				Conditions: []types.RuleCondition{
					{Type: types.AttributeExists, Path: []string{"enable_legacy_abac"}},
					{Type: types.AttributeValueEquals, Path: []string{"enable_legacy_abac"}, ExpectedValue: "true"},
				},
				Actions: []types.RuleAction{{Type: types.RemoveAttribute, Path: []string{"some_other_attr"}}},
			},
			expectedModifications:  1,
			expectAttributeRemoved: true,
			attributePathToRemove:  []string{"some_other_attr"},
		},
		{
			name: "AttributeValueEquals with boolean false",
			hclContent: `resource "google_container_cluster" "test" {
				enable_legacy_abac = false
				some_other_attr = "bar"
			}`,
			rule: types.Rule{
				Name:               "TestBoolEqualsFalse",
				TargetResourceType: "google_container_cluster",
				Conditions: []types.RuleCondition{
					{Type: types.AttributeValueEquals, Path: []string{"enable_legacy_abac"}, ExpectedValue: "false"},
				},
				Actions: []types.RuleAction{{Type: types.RemoveAttribute, Path: []string{"some_other_attr"}}},
			},
			expectedModifications:  1,
			expectAttributeRemoved: true,
			attributePathToRemove:  []string{"some_other_attr"},
		},
		{
			name: "AttributeValueEquals with integer",
			hclContent: `resource "google_container_cluster" "test" {
				max_pods_per_node = 110
				another_attr = "baz"
			}`,
			rule: types.Rule{
				Name:               "TestIntEquals",
				TargetResourceType: "google_container_cluster",
				Conditions: []types.RuleCondition{
					{Type: types.AttributeValueEquals, Path: []string{"max_pods_per_node"}, ExpectedValue: "110"},
				},
				Actions: []types.RuleAction{{Type: types.RemoveAttribute, Path: []string{"another_attr"}}},
			},
			expectedModifications:  1,
			expectAttributeRemoved: true,
			attributePathToRemove:  []string{"another_attr"},
		},
		{
			name: "AttributeValueEquals with float (parsed as number)",
			hclContent: `resource "google_container_cluster" "test" {
				monitoring_config {
					advanced_datapath_observability_config {
						relay_log_level_percent = 50.5
					}
				}
				attr_to_remove = "qux"
			}`,
			rule: types.Rule{
				Name:               "TestFloatEquals",
				TargetResourceType: "google_container_cluster",
				Conditions: []types.RuleCondition{
					{Type: types.AttributeValueEquals, Path: []string{"monitoring_config", "advanced_datapath_observability_config", "relay_log_level_percent"}, ExpectedValue: "50.5"},
				},
				Actions: []types.RuleAction{{Type: types.RemoveAttribute, Path: []string{"attr_to_remove"}}},
			},
			expectedModifications:  1,
			expectAttributeRemoved: true,
			attributePathToRemove:  []string{"attr_to_remove"},
		},
		{
			name: "AttributeValueEquals - condition not met (bool)",
			hclContent: `resource "google_container_cluster" "test" {
				enable_legacy_abac = true
				some_other_attr = "foo"
			}`,
			rule: types.Rule{
				Name:               "TestBoolNotEquals",
				TargetResourceType: "google_container_cluster",
				Conditions: []types.RuleCondition{
					{Type: types.AttributeValueEquals, Path: []string{"enable_legacy_abac"}, ExpectedValue: "false"},
				},
				Actions: []types.RuleAction{{Type: types.RemoveAttribute, Path: []string{"some_other_attr"}}},
			},
			expectedModifications:  0,
			expectAttributeRemoved: false,
			attributePathToRemove:  []string{"some_other_attr"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_attr_val_equals_*.hcl")
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
				t.Fatalf("NewFromFile() error = %v", err)
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{tc.rule})
			assert.Empty(t, errs, "ApplyRules should not return errors for these test cases")
			assert.Equal(t, tc.expectedModifications, modifications)

			if tc.expectAttributeRemoved {
				resourceBlock, _ := modifier.GetBlock("resource", []string{"google_container_cluster", "test"})
				assert.NotNil(t, resourceBlock, "Test resource block should exist")
				if resourceBlock != nil {
					_, attr, _ := modifier.GetAttributeValueByPath(resourceBlock.Body(), tc.attributePathToRemove)
					assert.Nil(t, attr, "Attribute '%s' should have been removed", tc.attributePathToRemove)
				}
			} else if tc.expectedModifications == 0 { // Check attribute still exists if no modification was expected
				resourceBlock, _ := modifier.GetBlock("resource", []string{"google_container_cluster", "test"})
				assert.NotNil(t, resourceBlock, "Test resource block should exist")
				if resourceBlock != nil {
					_, attr, _ := modifier.GetAttributeValueByPath(resourceBlock.Body(), tc.attributePathToRemove)
					assert.NotNil(t, attr, "Attribute '%s' should still exist", tc.attributePathToRemove)
				}
			}
		})
	}
}

func TestRuleSetAttributeValue(t *testing.T) {
	logger := zap.NewNop()
	tests := []struct {
		name                  string
		hclContent            string
		rule                  types.Rule
		expectedModifications int
		expectedHCLContent    string
	}{
		{
			name: "Set string attribute",
			hclContent: `resource "google_container_cluster" "test" {
				description = "Initial description"
			}`,
			rule: types.Rule{
				Name:               "TestSetStringValue",
				TargetResourceType: "google_container_cluster",
				Conditions:         []types.RuleCondition{{Type: types.AttributeExists, Path: []string{"description"}}},
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"description"}, ValueToSet: "Updated description"}},
			},
			expectedModifications: 1,
			expectedHCLContent: `resource "google_container_cluster" "test" {
				description = "Updated description"
			}`,
		},
		{
			name: "Set boolean attribute",
			hclContent: `resource "google_container_cluster" "test" {
				enable_shielded_nodes = false
			}`,
			rule: types.Rule{
				Name:               "TestSetBoolValue",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"enable_shielded_nodes"}, ValueToSet: "true"}},
			},
			expectedModifications: 1,
			expectedHCLContent: `resource "google_container_cluster" "test" {
				enable_shielded_nodes = true
			}`,
		},
		{
			name: "Set integer attribute",
			hclContent: `resource "google_container_cluster" "test" {
				node_locations = []
				monitoring_config {
					advanced_datapath_observability_config {
						relay_log_level_percent = 0
					}
				}
			}`,
			rule: types.Rule{
				Name:               "TestSetIntValue",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"monitoring_config", "advanced_datapath_observability_config", "relay_log_level_percent"}, ValueToSet: "75"}},
			},
			expectedModifications: 1,
			expectedHCLContent: `resource "google_container_cluster" "test" {
				node_locations = []
				monitoring_config {
					advanced_datapath_observability_config {
						relay_log_level_percent = 75
					}
				}
			}`,
		},
		{
			name: "Set new attribute",
			hclContent: `resource "google_container_cluster" "test" {
			}`,
			rule: types.Rule{
				Name:               "TestSetNewAttribute",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"new_attribute"}, ValueToSet: "new_value"}},
			},
			expectedModifications: 1,
			expectedHCLContent: `resource "google_container_cluster" "test" {
				new_attribute = "new_value"
			}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_set_attr_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.Write([]byte(tc.hclContent))
			assert.NoError(t, err, "Failed to write to temp file")
			err = tmpFile.Close()
			assert.NoError(t, err, "Failed to close temp file")

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			assert.NoError(t, err, "NewFromFile() error")

			modifications, errs := modifier.ApplyRules([]types.Rule{tc.rule})
			assert.Empty(t, errs, "ApplyRules should not return errors for these test cases")
			assert.Equal(t, tc.expectedModifications, modifications)

			// Normalize and compare HCL content
			expectedF, diags := hclwrite.ParseConfig([]byte(tc.expectedHCLContent), "expected.hcl", hcl.InitialPos)
			assert.False(t, diags.HasErrors(), "Failed to parse expected HCL content: %v", diags)

			actualF, diags := hclwrite.ParseConfig(modifier.File().Bytes(), "actual.hcl", hcl.InitialPos)
			assert.False(t, diags.HasErrors(), "Failed to parse actual HCL content: %v", diags)

			assert.Equal(t, string(expectedF.Bytes()), string(actualF.Bytes()), "HCL content mismatch")
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
		name                         string
		hclContent                   string
		expectedModifications        int
		gkeResourceName              string
		nodePoolChecks               []nodePoolCheck
		expectNoOtherResourceChanges bool
		expectNoGKEResource          bool
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
					nodePoolName:                  "default-pool",
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
					nodePoolName:                  "default-pool",
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
					nodePoolName:                  "default-pool",
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
					nodePoolName:                  "default-pool",
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
					nodePoolName:                  "pool-one",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
				{
					nodePoolName:                  "pool-two",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        false,
				},
				{
					nodePoolName:                  "pool-three",
					expectInitialNodeCountRemoved: false,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(4),
				},
				{
					nodePoolName:                  "pool-four",
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
			expectedModifications:        0,
			gkeResourceName:              "",
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
			gkeResourceName:       "gke_one",
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:                  "gke-one-pool",
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

			modifications, ruleErr := modifier.ApplyRules([]types.Rule{rules.InitialNodeCountRuleDefinition})
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
					// Determine original state of initial_node_count
					originalInitialPresent := false
					if tc.gkeResourceName != "" && npCheck.nodePoolName != "" { // Only check if we have names to find
						originalParsedFile, parseDiags := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
						if !parseDiags.HasErrors() {
							originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, "google_container_cluster", tc.gkeResourceName)
							if originalGKEResource != nil {
								// Pass nil for modifier as we are only checking structure, not live values from a Modifier instance
								originalNP, _ := findNodePoolInBlock(originalGKEResource, npCheck.nodePoolName, nil)
								if originalNP != nil && originalNP.Body().GetAttribute("initial_node_count") != nil {
									originalInitialPresent = true
								}
							}
						} else {
							t.Logf("Could not parse original HCL content for npCheck in test '%s': %v", tc.name, parseDiags)
						}
					}

					if npCheck.expectInitialNodeCountRemoved {
						assertNodePoolAttributeAbsent(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "initial_node_count")
					} else {
						if originalInitialPresent {
							assertNodePoolAttributeExists(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "initial_node_count")
						} else {
							// If it wasn't present originally and not expected to be removed (i.e. not added either),
							// then it should still be absent.
							assertNodePoolAttributeAbsent(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "initial_node_count")
						}
					}

					if npCheck.expectNodeCountPresent {
						if npCheck.expectedNodeCountValue != nil {
							assertNodePoolAttributeValue(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "node_count", cty.NumberIntVal(int64(*npCheck.expectedNodeCountValue)))
						} else {
							assertNodePoolAttributeExists(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "node_count")
						}
					} else {
						assertNodePoolAttributeAbsent(t, modifier, tc.gkeResourceName, npCheck.nodePoolName, "node_count")
					}
				}
			}
		})
	}
}

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
			ipAllocationPolicyShouldExistForCheck: false,
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

func TestAutopilotEnabled_NoConflictingRootFields(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	hclContent := `resource "google_container_cluster" "clean_autopilot_cluster" {
  name             = "clean-autopilot-cluster"
  enable_autopilot = true
  location         = "us-central1"
  addons_config {
    http_load_balancing { disabled = true }
  }
  cluster_autoscaling {
    autoscaling_profile = "BALANCED" // This whole block should be removed
  }
  binary_authorization {
    evaluation_mode = "DISABLED"
  }
}`
	expectedModifications := 1 // cluster_autoscaling block removal
	clusterName := "clean_autopilot_cluster"

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_no_conflicting_fields_*.hcl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpFile.Write([]byte(hclContent)); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	modifier, err := NewFromFile(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
	}

	autopilotRules := []types.Rule{rules.RuleHandleAutopilotFalse}
	autopilotRules = append(autopilotRules, rules.AutopilotRules...)
	modifications, ruleErr := modifier.ApplyRules(autopilotRules)
	if ruleErr != nil {
		t.Fatalf("ApplyAutopilotRule() returned error = %v. HCL content:\n%s", ruleErr, hclContent)
	}

	assert.Equal(t, expectedModifications, modifications, "ApplyAutopilotRule() modifications mismatch. HCL content:\n%s\nModified HCL:\n%s", hclContent, string(modifier.File().Bytes()))

	clusterBlock, _ := findBlockInParsedFile(modifier.File(), "google_container_cluster", clusterName)
	if !assert.NotNil(t, clusterBlock, "google_container_cluster resource '%s' not found. Modified HCL:\n%s", clusterName, string(modifier.File().Bytes())) {
		return
	}

	// 1. enable_autopilot attribute check
	assertAttributeValue(t, modifier, clusterBlock, "enable_autopilot", cty.True)

	// 2. Check cluster_autoscaling block is removed
	assert.Nil(t, clusterBlock.Body().FirstMatchingBlock("cluster_autoscaling", nil), "Expected 'cluster_autoscaling' block to be removed. Modified HCL:\n%s", string(modifier.File().Bytes()))

	// 3. addons_config checks (should remain)
	acBlock := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
	assert.NotNil(t, acBlock, "Expected 'addons_config' block to exist.")
	if acBlock != nil {
		httpLbBlock := acBlock.Body().FirstMatchingBlock("http_load_balancing", nil)
		assert.NotNil(t, httpLbBlock, "Expected 'http_load_balancing' block in 'addons_config' to exist.")
		if httpLbBlock != nil {
			assertAttributeValue(t, modifier, httpLbBlock, "disabled", cty.True)
		}
	}

	// 4. binary_authorization checks (should remain)
	baBlock := clusterBlock.Body().FirstMatchingBlock("binary_authorization", nil)
	assert.NotNil(t, baBlock, "Expected 'binary_authorization' block to exist.")
	if baBlock != nil {
		assertAttributeValue(t, modifier, baBlock, "evaluation_mode", cty.StringVal("DISABLED"))
		assert.Nil(t, baBlock.Body().GetAttribute("enabled"), "Expected 'enabled' attribute in 'binary_authorization' to be absent if not originally present and not conflicting.")
	}

	// 5. No root attributes should have been removed (other than enable_autopilot if it were false)
	assert.NotNil(t, clusterBlock.Body().GetAttribute("location"), "'location' attribute should not be removed.")
}

func TestAutopilotEnabled_PartialConflictingRootAttributes(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	hclContent := `resource "google_container_cluster" "partial_autopilot" {
  name                  = "partial-autopilot"
  enable_autopilot      = true
  enable_shielded_nodes = true
  default_max_pods_per_node = 110
}`
	expectedModifications := 2
	clusterName := "partial_autopilot"

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_partial_conflicting_*.hcl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if _, err := tmpFile.Write([]byte(hclContent)); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	modifier, err := NewFromFile(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
	}

	autopilotRules := []types.Rule{rules.RuleHandleAutopilotFalse}
	autopilotRules = append(autopilotRules, rules.AutopilotRules...)
	modifications, ruleErr := modifier.ApplyRules(autopilotRules)
	if ruleErr != nil {
		t.Fatalf("ApplyAutopilotRule() returned error = %v. HCL content:\n%s", ruleErr, hclContent)
	}

	assert.Equal(t, expectedModifications, modifications, "ApplyAutopilotRule() modifications mismatch. HCL content:\n%s\nModified HCL:\n%s", hclContent, string(modifier.File().Bytes()))

	clusterBlock, _ := findBlockInParsedFile(modifier.File(), "google_container_cluster", clusterName)
	if !assert.NotNil(t, clusterBlock, "google_container_cluster resource '%s' not found. Modified HCL:\n%s", clusterName, string(modifier.File().Bytes())) {
		return
	}

	// 1. enable_autopilot attribute check
	assertAttributeValue(t, modifier, clusterBlock, "enable_autopilot", cty.True)

	// 2. Specified root attributes are removed
	assert.Nil(t, clusterBlock.Body().GetAttribute("enable_shielded_nodes"), "Expected 'enable_shielded_nodes' to be removed. Modified HCL:\n%s", string(modifier.File().Bytes()))
	assert.Nil(t, clusterBlock.Body().GetAttribute("default_max_pods_per_node"), "Expected 'default_max_pods_per_node' to be removed. Modified HCL:\n%s", string(modifier.File().Bytes()))

	// 3. Ensure 'name' attribute is still present (was not part of removal list)
	assert.NotNil(t, clusterBlock.Body().GetAttribute("name"), "'name' attribute should not be removed.")
}

func TestAutopilotRules_NoGKEResource(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()
	hclContent := `resource "google_compute_instance" "vm" {
  name = "my-vm"
}`
	expectedModifications := 0

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_no_gke_*.hcl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	originalContent := []byte(hclContent) // Save original content for comparison
	if _, err := tmpFile.Write(originalContent); err != nil {
		tmpFile.Close()
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	modifier, err := NewFromFile(tmpFile.Name(), logger)
	if err != nil {
		t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
	}

	allAutopilotRules := []types.Rule{rules.RuleHandleAutopilotFalse}
	allAutopilotRules = append(allAutopilotRules, rules.AutopilotRules...)
	modifications, ruleErrs := modifier.ApplyRules(allAutopilotRules)

	if len(ruleErrs) > 0 {
		var errorMessages []string
		for _, rErr := range ruleErrs {
			errorMessages = append(errorMessages, rErr.Error())
		}
		t.Fatalf("ApplyRules() returned unexpected error(s): %v. HCL content:\n%s", strings.Join(errorMessages, "\n"), hclContent)
	}

	assert.Equal(t, expectedModifications, modifications, "Expected 0 modifications for HCL with no GKE resource")
	assert.Equal(t, string(originalContent), string(modifier.File().Bytes()), "HCL content should remain unchanged")
}

func TestApplyLoggingServiceRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                        string
		hclContentFile              string
		expectedModifications       int
		expectLoggingServiceRemoved bool
		resourceLabelsToVerify      []string
	}{
		{
			name:                        "Logging service present without telemetry",
			hclContentFile:              "testdata/TestApplyLoggingServiceRule_LoggingServicePresent.tf",
			expectedModifications:       0,
			expectLoggingServiceRemoved: false,
			resourceLabelsToVerify:      []string{"google_container_cluster", "primary"},
		},
		{
			name:                        "Logging service and telemetry present",
			hclContentFile:              "testdata/TestApplyLoggingServiceRule_LoggingServiceAndTelemetryPresent.tf",
			expectedModifications:       1,
			expectLoggingServiceRemoved: true,
			resourceLabelsToVerify:      []string{"google_container_cluster", "primary"},
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
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.RuleRemoveLoggingService})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(LoggingServiceRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)

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

				if targetBlock != nil {
					hasLoggingService := targetBlock.Body().GetAttribute("logging_service") != nil
					if tc.expectLoggingServiceRemoved {
						assert.False(t, hasLoggingService, "Expected 'logging_service' to be removed")
					} else {
						assert.True(t, hasLoggingService, "Expected 'logging_service' to be present")
					}
				}
			}
		})
	}
}

func TestApplyMonitoringServiceRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                           string
		hclContentFile                 string
		expectedModifications          int
		expectMonitoringServiceRemoved bool
		resourceLabelsToVerify         []string
	}{
		{
			name:                           "Monitoring service present without monitoring_config",
			hclContentFile:                 "testdata/TestApplyMonitoringServiceRule_MonitoringServicePresent.tf",
			expectedModifications:          0,
			expectMonitoringServiceRemoved: false,
			resourceLabelsToVerify:         []string{"google_container_cluster", "primary"},
		},
		{
			name:                           "Monitoring service and monitoring_config present",
			hclContentFile:                 "testdata/TestApplyMonitoringServiceRule_MonitoringServiceAndConfigPresent.tf",
			expectedModifications:          1,
			expectMonitoringServiceRemoved: true,
			resourceLabelsToVerify:         []string{"google_container_cluster", "primary"},
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
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.RuleRemoveMonitoringService})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(MonitoringServiceRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)

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

				if targetBlock != nil {
					hasMonitoringService := targetBlock.Body().GetAttribute("monitoring_service") != nil
					if tc.expectMonitoringServiceRemoved {
						assert.False(t, hasMonitoringService, "Expected 'monitoring_service' to be removed")
					} else {
						assert.True(t, hasMonitoringService, "Expected 'monitoring_service' to be present")
					}
				}
			}
		})
	}
}

func TestApplyNodeVersionRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                     string
		hclContentFile           string
		expectedModifications    int
		expectNodeVersionRemoved bool
		resourceLabelsToVerify   []string
	}{
		{
			name:                     "Node version present without min_master_version",
			hclContentFile:           "testdata/TestApplyNodeVersionRule_NodeVersionPresent.tf",
			expectedModifications:    1,
			expectNodeVersionRemoved: false,
			resourceLabelsToVerify:   []string{"google_container_cluster", "primary"},
		},
		{
			name:                     "Node version and min_master_version present",
			hclContentFile:           "testdata/TestApplyNodeVersionRule_NodeVersionAndMinMasterVersionPresent.tf",
			expectedModifications:    0,
			expectNodeVersionRemoved: true,
			resourceLabelsToVerify:   []string{"google_container_cluster", "primary"},
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
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.SetMinVersionRule})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(NodeVersionRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)
		})
	}
}

func TestApplyMoreComputedAttributesRules(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name                  string
		hclContent            string
		expectedHCLContent    string
		rulesToApply          []types.Rule
		expectedModifications int
	}{
		{
			name: "Remove control_plane_endpoints and database_encryption.state",
			hclContent: `resource "google_container_cluster" "test" {
  control_plane_endpoints_config {
    dns_endpoint_config {
      endpoint = "some-endpoint"
      allow_external_traffic = false
    }
  }
  database_encryption {
    state    = "DECRYPTED"
    key_name = "some_key"
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "test" {
  control_plane_endpoints_config {
    dns_endpoint_config {
      allow_external_traffic = false
    }
  }
  database_encryption {
    state    = "DECRYPTED"
    key_name = "some_key"
  }
}`,
			rulesToApply:          rules.OtherComputedAttributesRules, // This slice now includes the new rules
			expectedModifications: 1,
		},
		{
			name: "Remove node_pool.autoscaling total_counts",
			hclContent: `resource "google_container_cluster" "test_np_computed" {
  node_pool {
    name = "pool1"
    autoscaling {
      total_max_node_count = 10
      total_min_node_count = 1
      location_policy      = "BALANCED"
    }
  }
  node_pool {
    name = "pool2"
    autoscaling {
      total_max_node_count = 5 // total_min_node_count is absent
    }
  }
  node_pool {
    name = "pool3"
  }
  node_pool {
    name = "pool4"
    autoscaling {
      location_policy = "ANY"
    }
  }
  node_pool {
    name = "pool5"
    autoscaling {
      total_min_node_count = 2
    }
  }
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_np_computed" {
  node_pool {
    name = "pool1"
    autoscaling {
      location_policy = "BALANCED"
    }
  }
  node_pool {
    name = "pool2"
    autoscaling {
	}
  }
  node_pool {
    name = "pool3"
  }
  node_pool {
    name = "pool4"
    autoscaling {
      location_policy = "ANY"
    }
  }
  node_pool {
    name = "pool5"
    autoscaling {
	}
  }
}`,
			rulesToApply:          rules.OtherComputedAttributesRules,
			expectedModifications: 4, // pool1 (max, min), pool2 (max), pool5 (min)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_more_computed_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.Write([]byte(tc.hclContent))
			assert.NoError(t, err, "Failed to write to temp file for '%s'", tc.name)
			err = tmpFile.Close()
			assert.NoError(t, err, "Failed to close temp file for '%s'", tc.name)

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			assert.NoError(t, err, "NewFromFile() error for '%s'", tc.name)
			if err != nil {
				return
			}

			modifications, errs := modifier.ApplyRules(tc.rulesToApply)
			assert.Empty(t, errs, "ApplyRules returned errors for '%s': %v", tc.name, errs)
			assert.Equal(t, tc.expectedModifications, modifications, "Modification count mismatch for '%s'", tc.name)

			// Normalize and compare HCL content
			expectedF, diags := hclwrite.ParseConfig([]byte(tc.expectedHCLContent), "expected.hcl", hcl.InitialPos)
			assert.False(t, diags.HasErrors(), "Failed to parse expected HCL content for '%s': %v", tc.name, diags)
			if diags.HasErrors() {
				return
			}

			actualF, diags := hclwrite.ParseConfig(modifier.File().Bytes(), "actual.hcl", hcl.InitialPos)
			assert.False(t, diags.HasErrors(), "Failed to parse actual HCL content for '%s': %v", tc.name, diags)
			if diags.HasErrors() {
				return
			}

			assert.Equal(t, string(expectedF.Bytes()), string(actualF.Bytes()), "HCL content mismatch for '%s'", tc.name)
		})
	}
}

type testCase struct {
	name                  string
	hclContent            string
	expectedHCLContent    string
	expectedModifications int
	ruleToApply           types.Rule
}

func TestModifier_ApplyRuleRemoveLoggingService(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []testCase{
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
			ruleToApply:           rules.RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed when telemetry is disabled",
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
			ruleToApply:           rules.RuleRemoveLoggingService,
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
			ruleToApply:           rules.RuleRemoveLoggingService,
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
			ruleToApply:           rules.RuleRemoveLoggingService,
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
			ruleToApply:           rules.RuleRemoveLoggingService,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			applyRuleTestCase(tc, t, logger)
		})
	}
}

func TestModifier_ApplyRuleRemoveMonitoringService(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	tests := []testCase{
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
			ruleToApply:           rules.RuleRemoveMonitoringService,
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
			ruleToApply:           rules.RuleRemoveMonitoringService,
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
			ruleToApply:           rules.RuleRemoveMonitoringService,
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
			ruleToApply:           rules.RuleRemoveMonitoringService,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			applyRuleTestCase(tc, t, logger)
		})
	}
}

func applyRuleTestCase(tc testCase, t *testing.T, logger *zap.Logger) {
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

	modifications, errs := modifier.ApplyRules([]types.Rule{tc.ruleToApply})
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
}

// Helper Functions

// assertAttributeValue is a helper to check attribute existence and value within a block
func assertAttributeValue(t *testing.T, mod *Modifier, block *hclwrite.Block, attrName string, expectedValue cty.Value) {
	t.Helper()
	attr := block.Body().GetAttribute(attrName)
	if !assert.NotNil(t, attr, "Attribute '%s' should exist in block '%s'", attrName, block.Type()) {
		return
	}
	val, err := mod.GetAttributeValue(attr)
	assert.NoError(t, err, "Error getting value of '%s' in block '%s'", attrName, block.Type())
	if err == nil {
		assert.Equal(t, expectedValue.Type(), val.Type(), "Expected attribute '%s' in block '%s' to be type %s, but got %s", attrName, block.Type(), expectedValue.Type().FriendlyName(), val.Type().FriendlyName())
		assert.True(t, expectedValue.Equals(val).True(), "Expected attribute '%s' in block '%s' to be %v, but got %v", attrName, block.Type(), expectedValue.GoString(), val.GoString())
	}
}

func intPtr(i int) *int {
	return &i
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

// findNodePoolInModifier finds a node_pool block within a given GKE resource block in a Modifier.
func findNodePoolInModifier(t *testing.T, mod *Modifier, gkeResourceName string, nodePoolName string) *hclwrite.Block {
	t.Helper()
	gkeResourceBlock, err := mod.GetBlock("resource", []string{"google_container_cluster", gkeResourceName})
	if err != nil {
		t.Fatalf("Error getting GKE resource '%s': %v", gkeResourceName, err)
		return nil
	}
	if gkeResourceBlock == nil {
		t.Fatalf("GKE resource '%s' not found", gkeResourceName)
		return nil
	}

	for _, block := range gkeResourceBlock.Body().Blocks() {
		if block.Type() == "node_pool" {
			nameAttr := block.Body().GetAttribute("name")
			if nameAttr == nil {
				if nodePoolName == "" { // Useful if there's only one anonymous node pool
					return block
				}
				continue
			}
			nameVal, err := mod.GetAttributeValue(nameAttr)
			if err != nil {
				t.Logf("Warning: could not get value for 'name' attribute in a node_pool of GKE resource '%s': %v", gkeResourceName, err)
				continue
			}
			if nameVal.Type() == cty.String && nameVal.AsString() == nodePoolName {
				return block
			}
		}
	}
	return nil
}

func assertNodePoolAttributeAbsent(t *testing.T, mod *Modifier, gkeResourceName string, nodePoolName string, attributeName string) {
	t.Helper()
	nodePoolBlock := findNodePoolInModifier(t, mod, gkeResourceName, nodePoolName)
	if nodePoolBlock == nil {
		t.Fatalf("Node pool '%s' not found in GKE resource '%s'", nodePoolName, gkeResourceName)
		return
	}

	attr := nodePoolBlock.Body().GetAttribute(attributeName)
	assert.Nil(t, attr, "Attribute '%s' should be absent from node_pool '%s' in GKE resource '%s', but was found.", attributeName, nodePoolName, gkeResourceName)
}

func assertNodePoolAttributeExists(t *testing.T, mod *Modifier, gkeResourceName string, nodePoolName string, attributeName string) {
	t.Helper()
	nodePoolBlock := findNodePoolInModifier(t, mod, gkeResourceName, nodePoolName)
	if nodePoolBlock == nil {
		t.Fatalf("Node pool '%s' not found in GKE resource '%s'", nodePoolName, gkeResourceName)
		return
	}

	attr := nodePoolBlock.Body().GetAttribute(attributeName)
	assert.NotNil(t, attr, "Attribute '%s' should exist in node_pool '%s' in GKE resource '%s', but was not found.", attributeName, nodePoolName, gkeResourceName)
}

func assertNodePoolAttributeValue(t *testing.T, mod *Modifier, gkeResourceName string, nodePoolName string, attributeName string, expectedValue cty.Value) {
	t.Helper()
	nodePoolBlock := findNodePoolInModifier(t, mod, gkeResourceName, nodePoolName)
	if nodePoolBlock == nil {
		t.Fatalf("Node pool '%s' not found in GKE resource '%s'", nodePoolName, gkeResourceName)
		return
	}

	attr := nodePoolBlock.Body().GetAttribute(attributeName)
	if !assert.NotNil(t, attr, "Attribute '%s' should exist in node_pool '%s' in GKE resource '%s' for value assertion, but was not found.", attributeName, nodePoolName, gkeResourceName) {
		return
	}

	actualValue, err := mod.GetAttributeValue(attr)
	if !assert.NoError(t, err, "Error getting value for attribute '%s' in node_pool '%s' of GKE resource '%s'", attributeName, nodePoolName, gkeResourceName) {
		return
	}

	if !assert.True(t, expectedValue.Type().Equals(actualValue.Type()), "Type mismatch for attribute '%s' in node_pool '%s'. Expected type %s, got %s", attributeName, nodePoolName, expectedValue.Type().FriendlyName(), actualValue.Type().FriendlyName()) {
		return
	}

	assert.True(t, expectedValue.Equals(actualValue).True(), "Value mismatch for attribute '%s' in node_pool '%s'. Expected '%s', got '%s'", attributeName, nodePoolName, expectedValue.GoString(), actualValue.GoString())
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
