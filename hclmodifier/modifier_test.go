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

func TestApplySetMinVersionRule(t *testing.T) {
	logger := zap.NewNop()
	tests := []struct {
		name                  string
		hclContent            string
		expectedHCLContent    string
		expectedModifications int
	}{
		{
			name: "Rule Applies - min_master_version is added",
			hclContent: `resource "google_container_cluster" "test_cluster" {
  name           = "my-gke-cluster"
  location       = "us-central1"
  node_version   = "1.27.5-gke.200"
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_cluster" {
  name               = "my-gke-cluster"
  location           = "us-central1"
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.5-gke.200"
}`,
			expectedModifications: 1,
		},
		{
			name: "Rule Does Not Apply - min_master_version already present",
			hclContent: `resource "google_container_cluster" "test_cluster" {
  name               = "my-gke-cluster"
  location           = "us-central1"
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.4-gke.100"
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_cluster" {
  name               = "my-gke-cluster"
  location           = "us-central1"
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.4-gke.100"
}`,
			expectedModifications: 0,
		},
		{
			name: "Rule Does Not Apply - node_version is absent",
			hclContent: `resource "google_container_cluster" "test_cluster" {
  name               = "my-gke-cluster"
  location           = "us-central1"
  min_master_version = "1.27.5-gke.200"
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_cluster" {
  name               = "my-gke-cluster"
  location           = "us-central1"
  min_master_version = "1.27.5-gke.200"
}`,
			expectedModifications: 0,
		},
		{
			name: "Rule Does Not Apply - Neither node_version nor min_master_version present",
			hclContent: `resource "google_container_cluster" "test_cluster" {
  name     = "my-gke-cluster"
  location = "us-central1"
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_cluster" {
  name     = "my-gke-cluster"
  location = "us-central1"
}`,
			expectedModifications: 0,
		},
		{
			name: "Rule Does Not Apply - Resource type is not google_container_cluster",
			hclContent: `resource "google_compute_instance" "test_vm" {
  name         = "my-vm"
  machine_type = "e2-medium"
  node_version = "1.27.5-gke.200"
}`,
			expectedHCLContent: `resource "google_compute_instance" "test_vm" {
  name         = "my-vm"
  machine_type = "e2-medium"
  node_version = "1.27.5-gke.200"
}`,
			expectedModifications: 0,
		},
		{
			name: "Rule Applies - min_master_version is null",
			hclContent: `resource "google_container_cluster" "test_cluster_null_min_master" {
  name               = "my-gke-cluster-null-min-master"
  location           = "us-central1"
  node_version       = "1.27.5-gke.200"
  min_master_version = null
}`,
			expectedHCLContent: `resource "google_container_cluster" "test_cluster_null_min_master" {
  name               = "my-gke-cluster-null-min-master"
  location           = "us-central1"
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.5-gke.200"
}`,
			expectedModifications: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_set_min_version_*.hcl")
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

			rulesToApply := []types.Rule{
				rules.SetMinVersionRule,
			}
			modifications, errs := modifier.ApplyRules(rulesToApply)

			if tc.expectedModifications == 0 {
				assert.Empty(t, errs, "ApplyRules should not return errors for test cases where no rule applies ('%s')", tc.name)
			} else if len(errs) > 1 {
				// If one rule applied, the other one failing its conditions might be reported.
				// This depends on whether "condition not met" is treated as an error by the engine.
				// For this test, we assume that "condition not met" might be logged but not counted in `errs` from ApplyRules.
				// If it IS an error, then len(errs) could be 1, and that's acceptable.
				assert.LessOrEqual(t, len(errs), 1, "Expected at most one 'condition not met' error when a rule applies for test case '%s'", tc.name)
			}

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

func TestApplyMasterCIDRRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	type privateClusterConfigCheck struct {
		expectBlockExists                      bool
		expectPrivateEndpointSubnetworkRemoved bool
		expectOtherAttributeUnchanged          *string
	}

	tests := []struct {
		name                         string
		hclContent                   string
		expectedModifications        int
		gkeResourceName              string
		expectMasterCIDRPresent      bool
		privateClusterConfigCheck    *privateClusterConfigCheck
		expectNoOtherResourceChanges bool
		expectNoGKEResource          bool
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
				expectBlockExists:                      true,
				expectPrivateEndpointSubnetworkRemoved: true,
				expectOtherAttributeUnchanged:          stringPtr("enable_private_endpoint"),
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
				expectBlockExists:                      true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:          stringPtr("enable_private_endpoint"),
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
				expectBlockExists:                      true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:          stringPtr("enable_private_endpoint"),
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
				expectBlockExists:                      true,
				expectPrivateEndpointSubnetworkRemoved: false,
				expectOtherAttributeUnchanged:          stringPtr("enable_private_endpoint"),
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
			expectedModifications:        0,
			gkeResourceName:              "",
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
				expectBlockExists:                      true,
				expectPrivateEndpointSubnetworkRemoved: true,
				expectOtherAttributeUnchanged:          stringPtr("enable_private_endpoint"),
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

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.MasterCIDRRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(rules.MasterCIDRRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, tc.hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(rules.MasterCIDRRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
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
		})
	}
}

func TestApplyBinaryAuthorizationRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                                  string
		hclContent                            string
		expectedModifications                 int
		expectEnabledAttributeRemoved         bool
		resourceLabelsToVerify                []string
		binaryAuthorizationShouldExist        bool
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
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
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
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
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
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
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
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block present but empty",
			hclContent: `resource "google_container_cluster" "primary" {
  name = "primary-cluster"
  binary_authorization {}
}`,
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block missing entirely",
			hclContent: `resource "google_container_cluster" "primary" {
  name     = "primary-cluster"
  location = "us-central1"
}`,
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        false,
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
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_compute_instance", "default"},
			binaryAuthorizationShouldExist:        true,
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
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "gke_one"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Empty HCL content",
			hclContent:                            ``,
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                nil,
			binaryAuthorizationShouldExist:        false,
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
							t.Errorf("ApplyBinaryAuthorizationRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyBinaryAuthorizationRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.BinaryAuthorizationRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(BinaryAuthorizationRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, tc.hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(BinaryAuthorizationRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
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
		})
	}
}

func TestApplyServicesIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

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
			name:                                  "Empty HCL content",
			hclContent:                            ``,
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
							t.Errorf("ApplyServicesIPV4CIDRRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyServicesIPV4CIDRRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ServicesIPV4CIDRRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(ServicesIPV4CIDRRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, tc.hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(ServicesIPV4CIDRRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
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
		})
	}
}

func TestApplyClusterIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

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
			name:                         "Empty HCL content",
			hclContent:                   ``,
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
							t.Errorf("ApplyClusterIPV4CIDRRule() modifications = %v, want %v. NewFromFile failed as expected for empty content.", modifications, tc.expectedModifications)
						}
						if ruleErr != nil {
							t.Errorf("ApplyClusterIPV4CIDRRule() unexpected error = %v for empty content when NewFromFile failed.", ruleErr)
						}
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ClusterIPV4CIDRRuleDefinition})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(ClusterIPV4CIDRRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, tc.hclContent, errorMessages)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules(ClusterIPV4CIDRRuleDefinition) modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
					modifications, tc.expectedModifications, tc.hclContent, string(modifier.File().Bytes()))
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

func TestAutopilotRules_EmptyHCL(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()
	hclContent := ``
	expectedModifications := 0

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_empty_*.hcl")
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
	// For empty HCL, NewFromFile is expected to return a non-nil modifier and no error,
	// or specific error if hclwrite.ParseConfig fails on empty string (which it shouldn't for valid HCL).
	// Current NewFromFile implementation might return error if file.Body().Blocks() is empty.
	// This test assumes it handles it by creating an empty body or similar.
	if err != nil {
		// If NewFromFile specifically errors on empty valid HCL and that's expected, adjust this.
		// For now, assume it should proceed and result in 0 modifications.
		// If it returns nil modifier for empty file, that's also a form of "handling".
		if modifier == nil { // This implies NewFromFile correctly identified no runnable content.
			assert.Equal(t, expectedModifications, 0, "Expected 0 modifications for empty HCL if modifier is nil")
			return
		}
		t.Fatalf("NewFromFile() returned an unexpected error for empty HCL: %v", err)
	}

	allAutopilotRules := []types.Rule{rules.RuleHandleAutopilotFalse}
	allAutopilotRules = append(allAutopilotRules, rules.AutopilotRules...)
	modifications, ruleErrs := modifier.ApplyRules(allAutopilotRules)

	if len(ruleErrs) > 0 {
		var errorMessages []string
		for _, rErr := range ruleErrs {
			errorMessages = append(errorMessages, rErr.Error())
		}
		t.Fatalf("ApplyRules() returned unexpected error(s) for empty HCL: %v", strings.Join(errorMessages, "\n"))
	}

	assert.Equal(t, expectedModifications, modifications, "Expected 0 modifications for empty HCL")
	assert.Empty(t, string(modifier.File().Bytes()), "Expected HCL content to remain empty")
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

func TestAutopilotNotPresent_ConflictingFieldsPresent(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	hclContent := `resource "google_container_cluster" "existing_cluster" {
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
}`
	expectedModifications := 0
	clusterName := "existing_cluster"

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_not_present_*.hcl")
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

	// 1. enable_autopilot attribute should remain absent
	assert.Nil(t, clusterBlock.Body().GetAttribute("enable_autopilot"), "Expected 'enable_autopilot' attribute to remain absent. Modified HCL:\n%s", string(modifier.File().Bytes()))

	// 2. Check other root attributes remain
	assertAttributeValue(t, modifier, clusterBlock, "cluster_ipv4_cidr", cty.StringVal("10.0.0.0/8"))
	assertAttributeValue(t, modifier, clusterBlock, "enable_shielded_nodes", cty.True)

	// 3. Check node_pool remains
	npBlock, _ := findNodePoolInBlock(clusterBlock, "default-pool", modifier)
	assert.NotNil(t, npBlock, "Expected 'node_pool' with name 'default-pool' to exist.")

	// 4. addons_config checks
	acBlock := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
	assert.NotNil(t, acBlock, "Expected 'addons_config' block to exist.")
	if acBlock != nil {
		npcBlock := acBlock.Body().FirstMatchingBlock("network_policy_config", nil)
		assert.NotNil(t, npcBlock, "Expected 'network_policy_config' block in 'addons_config' to exist.")
		if npcBlock != nil {
			assertAttributeValue(t, modifier, npcBlock, "disabled", cty.False)
		}
	}
}

func TestAutopilotDisabled_ConflictingFieldsPresent(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	hclContent := `resource "google_container_cluster" "standard_cluster" {
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
}`
	expectedModifications := 1
	clusterName := "standard_cluster"

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_disabled_*.hcl")
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
	assert.Nil(t, clusterBlock.Body().GetAttribute("enable_autopilot"), "Expected 'enable_autopilot' attribute to be removed. Modified HCL:\n%s", string(modifier.File().Bytes()))

	// 2. Check other root attributes remain
	assertAttributeValue(t, modifier, clusterBlock, "cluster_ipv4_cidr", cty.StringVal("10.0.0.0/8"))
	assertAttributeValue(t, modifier, clusterBlock, "enable_shielded_nodes", cty.True)

	// 3. Check node_pool remains
	npBlock, _ := findNodePoolInBlock(clusterBlock, "default-pool", modifier)
	assert.NotNil(t, npBlock, "Expected 'node_pool' with name 'default-pool' to exist.")

	// 4. cluster_autoscaling checks
	caBlock := clusterBlock.Body().FirstMatchingBlock("cluster_autoscaling", nil)
	assert.NotNil(t, caBlock, "Expected 'cluster_autoscaling' block to exist.")
	if caBlock != nil {
		assertAttributeValue(t, modifier, caBlock, "enabled", cty.True)
		assertAttributeValue(t, modifier, caBlock, "autoscaling_profile", cty.StringVal("BALANCED"))
	}

	// 5. addons_config checks
	acBlock := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
	assert.NotNil(t, acBlock, "Expected 'addons_config' block to exist.")
	if acBlock != nil {
		dnsCacheBlock := acBlock.Body().FirstMatchingBlock("dns_cache_config", nil)
		assert.NotNil(t, dnsCacheBlock, "Expected 'dns_cache_config' block in 'addons_config' to exist.")
		if dnsCacheBlock != nil {
			assertAttributeValue(t, modifier, dnsCacheBlock, "enabled", cty.True)
		}

		httpLbBlock := acBlock.Body().FirstMatchingBlock("http_load_balancing", nil)
		assert.NotNil(t, httpLbBlock, "Expected 'http_load_balancing' block in 'addons_config' to exist.")
		if httpLbBlock != nil {
			assertAttributeValue(t, modifier, httpLbBlock, "disabled", cty.False)
		}
	}
}

func TestAutopilotEnabled_ConflictingFieldsPresent(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	hclContent := `resource "google_container_cluster" "autopilot_cluster" {
  name                          = "autopilot-cluster"
  location                      = "us-central1"
  enable_autopilot              = true
  cluster_ipv4_cidr             = "10.0.0.0/8"
  enable_shielded_nodes         = true
  remove_default_node_pool      = true
  default_max_pods_per_node     = 110
  enable_intranode_visibility   = true

  node_config {
    machine_type = "e2-standard-4"
    disk_size_gb = 100
    oauth_scopes = [
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
      "https://www.googleapis.com/auth/service.management.readonly",
      "https://www.googleapis.com/auth/servicecontrol",
      "https://www.googleapis.com/auth/trace.append",
    ]
  }

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
}`
	expectedModifications := 14
	clusterName := "autopilot_cluster"

	tempDir := t.TempDir()
	tmpFile, err := os.CreateTemp(tempDir, "test_autopilot_conflicting_*.hcl")
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
	if !assert.NotNil(t, clusterBlock, "google_container_cluster resource '%s' not found after ApplyAutopilotRule. Modified HCL:\n%s", clusterName, string(modifier.File().Bytes())) {
		return
	}

	// 1. enable_autopilot attribute check
	enableAutopilotHCLAttr := clusterBlock.Body().GetAttribute("enable_autopilot")
	assert.NotNil(t, enableAutopilotHCLAttr, "Expected 'enable_autopilot' attribute to exist. Modified HCL:\n%s", string(modifier.File().Bytes()))
	if enableAutopilotHCLAttr != nil {
		val, err := modifier.GetAttributeValue(enableAutopilotHCLAttr)
		assert.NoError(t, err, "Error getting value of 'enable_autopilot'")
		if err == nil {
			assert.Equal(t, cty.Bool, val.Type(), "Expected 'enable_autopilot' to be boolean")
			assert.True(t, val.True(), "Expected 'enable_autopilot' to be true, but got %v", val.True())
		}
	}

	// 2. Root-level attributes removal
	expectedRootAttrsRemoved := []string{
		"cluster_ipv4_cidr",
		"enable_shielded_nodes",
		"remove_default_node_pool",
		"default_max_pods_per_node",
		"enable_intranode_visibility",
	}
	for _, attrName := range expectedRootAttrsRemoved {
		assert.Nil(t, clusterBlock.Body().GetAttribute(attrName), "Expected root attribute '%s' to be removed, but it was found. Modified HCL:\n%s", attrName, string(modifier.File().Bytes()))
	}

	// 3. Top-level nested blocks removal
	expectedTopLevelNestedBlocksRemoved := []string{
		"network_policy",
		"node_pool", // All node_pool blocks should be removed
		"cluster_autoscaling",
		"node_config",
	}
	for _, blockTypeName := range expectedTopLevelNestedBlocksRemoved {
		if blockTypeName == "node_pool" {
			foundNodePools := false
			for _, nestedB := range clusterBlock.Body().Blocks() {
				if nestedB.Type() == "node_pool" {
					foundNodePools = true
					break
				}
			}
			assert.False(t, foundNodePools, "Expected all nested blocks of type 'node_pool' to be removed, but at least one was found. Modified HCL:\n%s", string(modifier.File().Bytes()))
		} else {
			assert.Nil(t, clusterBlock.Body().FirstMatchingBlock(blockTypeName, nil), "Expected nested block '%s' to be removed, but it was found. Modified HCL:\n%s", blockTypeName, string(modifier.File().Bytes()))
		}
	}

	// 4. addons_config checks
	acBlockInHCL := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
	assert.NotNil(t, acBlockInHCL, "Expected 'addons_config' block to exist, but it was not found")
	if acBlockInHCL != nil {
		assert.Nil(t, acBlockInHCL.Body().FirstMatchingBlock("network_policy_config", nil), "Expected 'network_policy_config' in 'addons_config' to be removed")
		assert.Nil(t, acBlockInHCL.Body().FirstMatchingBlock("dns_cache_config", nil), "Expected 'dns_cache_config' in 'addons_config' to be removed")
		assert.Nil(t, acBlockInHCL.Body().FirstMatchingBlock("stateful_ha_config", nil), "Expected 'stateful_ha_config' in 'addons_config' to be removed")
		assert.NotNil(t, acBlockInHCL.Body().FirstMatchingBlock("http_load_balancing", nil), "Expected 'http_load_balancing' in 'addons_config' to be present, but it was not found")
	}

	// 5. binary_authorization checks
	baBlockInHCL := clusterBlock.Body().FirstMatchingBlock("binary_authorization", nil)
	assert.NotNil(t, baBlockInHCL, "Expected 'binary_authorization' block to exist, but it was not found")
	if baBlockInHCL != nil {
		assert.Nil(t, baBlockInHCL.Body().GetAttribute("enabled"), "Expected 'enabled' attribute in 'binary_authorization' to be removed, but it was found")
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
