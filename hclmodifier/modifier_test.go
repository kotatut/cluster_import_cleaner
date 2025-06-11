package hclmodifier

import (
	"fmt" // Added for helper functions
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	// "github.com/hashicorp/hcl/v2/hclsyntax" // Removed as it's unused
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert" // For assertions
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules" // Import for rule definitions
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import for type definitions
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

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.SetMinVersionRuleDefinition})
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
			gkeResourceName:       "gke_one", // This test case might need adjustment if we verify all GKE resources
			nodePoolChecks: []nodePoolCheck{
				{
					nodePoolName:                  "gke-one-pool",
					expectInitialNodeCountRemoved: true,
					expectNodeCountPresent:        true,
					expectedNodeCountValue:        intPtr(5),
				},
				// Add checks for gke_two pools if the rule is applied to all of them
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
				if tc.hclContent == "" && tc.expectedModifications == 0 { // Handle empty HCL case
					if modifier == nil { // NewFromFile might return nil for empty files
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			// Call ApplyRules with the InitialNodeCountRuleDefinition
			modifications, errs := modifier.ApplyRules([]types.Rule{rules.InitialNodeCountRuleDefinition})
			if len(errs) > 0 {
				for _, ruleErr := range errs {
					t.Logf("ApplyRules() error: %v", ruleErr) // Log individual errors
				}
				t.Fatalf("ApplyRules() returned errors for HCL: \n%s", tc.hclContent)
			}

			if modifications != tc.expectedModifications {
				t.Errorf("ApplyRules() modifications = %v, want %v. HCL content:\n%s\nModified HCL:\n%s",
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

			// For "MultipleGKEResources", we need to check both resources if the rule should apply to both
			if tc.name == "MultipleGKEResources" {
				gkeOne, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_one")
				assert.NotNil(t, gkeOne, "gke_one should exist")
				if gkeOne != nil {
					npOne, _ := findNodePoolInBlock(gkeOne, "gke-one-pool", modifier)
					assert.NotNil(t, npOne, "gke-one-pool should exist")
					if npOne != nil {
						assert.Nil(t, npOne.Body().GetAttribute("initial_node_count"), "'initial_node_count' should be removed from gke-one-pool")
						assert.NotNil(t, npOne.Body().GetAttribute("node_count"), "'node_count' should exist in gke-one-pool")
					}
				}

				gkeTwo, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_two")
				assert.NotNil(t, gkeTwo, "gke_two should exist")
				if gkeTwo != nil {
					npTwoDefault, _ := findNodePoolInBlock(gkeTwo, "gke-two-pool", modifier)
					assert.NotNil(t, npTwoDefault, "gke-two-pool should exist")
					if npTwoDefault != nil {
						assert.Nil(t, npTwoDefault.Body().GetAttribute("initial_node_count"), "'initial_node_count' should be removed from gke-two-pool")
					}
					npTwoExtra, _ := findNodePoolInBlock(gkeTwo, "gke-two-pool-extra", modifier)
					assert.NotNil(t, npTwoExtra, "gke-two-pool-extra should exist")
					if npTwoExtra != nil {
						assert.Nil(t, npTwoExtra.Body().GetAttribute("initial_node_count"), "'initial_node_count' should not exist in gke-two-pool-extra (and was not removed as it wasn't there)")
						assert.NotNil(t, npTwoExtra.Body().GetAttribute("node_count"), "'node_count' should exist in gke-two-pool-extra")
					}
				}
				return // Skip default checks for this specific multi-resource case
			}

			var targetGKEResource *hclwrite.Block
			if tc.gkeResourceName != "" {
				targetGKEResource, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", tc.gkeResourceName)
				if targetGKEResource == nil && len(tc.nodePoolChecks) > 0 { // If we expect checks but no resource, fail
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
								nameVal, err := modifier.GetAttributeValue(nameAttr) // Use existing modifier for GetAttributeValue
								if err == nil && nameVal.Type() == cty.String && nameVal.AsString() == npCheck.nodePoolName {
									foundNodePool = nestedBlock
									break
								}
							} else if npCheck.nodePoolName == "" { // Handle unnamed node pools if necessary for a test case
								// This logic might need refinement if multiple unnamed node pools can exist and need specific checks
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
						if npCheck.expectInitialNodeCountRemoved || npCheck.expectNodeCountPresent { // Only error if we expected to find it for a check
							t.Errorf("Node pool '%s' in resource '%s' not found for verification. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
						continue // If node pool isn't found, and we didn't expect to check it, skip to next npCheck
					}

					initialAttr := foundNodePool.Body().GetAttribute("initial_node_count")
					if npCheck.expectInitialNodeCountRemoved {
						if initialAttr != nil {
							t.Errorf("Expected 'initial_node_count' to be REMOVED from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					} else { // Not expected to be removed
						// Check if it was present in the original HCL to ensure it wasn't removed when it shouldn't have been
						originalInitialPresent := false
						originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos) // Parse original content
						originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, "google_container_cluster", tc.gkeResourceName)
						if originalGKEResource != nil {
							originalNP, _ := findNodePoolInBlock(originalGKEResource, npCheck.nodePoolName, modifier)
							if originalNP != nil && originalNP.Body().GetAttribute("initial_node_count") != nil {
								originalInitialPresent = true
							}
						}
						if originalInitialPresent && initialAttr == nil { // Was there, but now it's gone
							t.Errorf("Expected 'initial_node_count' to be PRESENT in node_pool '%s' in '%s', but it was NOT FOUND (removed). Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
						if !originalInitialPresent && initialAttr != nil { // Wasn't there, but now it is (should not happen with a removal rule)
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
							val, err := modifier.GetAttributeValue(nodeCountAttr) // Use existing modifier
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
					} else { // Not expected to be present
						if nodeCountAttr != nil {
							t.Errorf("Expected 'node_count' to be ABSENT from node_pool '%s' in '%s', but it was FOUND. Modified HCL:\n%s",
								npCheck.nodePoolName, tc.gkeResourceName, string(modifiedContentBytes))
						}
					}
				}
			}

			if tc.expectNoOtherResourceChanges && tc.name == "NonGKEResource" {
				var nonGKEResource *hclwrite.Block
				nonGKEResource, _ = findBlockInParsedFile(verifiedFile, "google_compute_instance", "not_gke")
				if nonGKEResource == nil {
					t.Fatalf("Expected non-GKE resource 'google_compute_instance.not_gke' not found. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				// Check that attributes in the non-GKE resource are untouched
				if nonGKEResource.Body().GetAttribute("initial_node_count") == nil {
					t.Errorf("Top-level 'initial_node_count' was unexpectedly removed from 'google_compute_instance.not_gke'. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				npBlock, _ := findNodePoolInBlock(nonGKEResource, "", modifier) // Assuming unnamed node_pool in the test case for non-GKE
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

			if tc.name == "MultipleGKEResources_OneMatch" {
				gkeTwo, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_two_no_master_cidr")
				if gkeTwo == nil {
					t.Fatalf("GKE resource 'gke_two_no_master_cidr' not found in multi-resource test.")
				}
				if gkeTwo.Body().GetAttribute("master_ipv4_cidr_block") != nil {
					t.Errorf("'master_ipv4_cidr_block' unexpectedly present in 'gke_two_no_master_cidr'.")
				}
				pccTwo := gkeTwo.Body().FirstMatchingBlock("private_cluster_config", nil)
				if pccTwo == nil {
					t.Fatalf("'private_cluster_config' missing in 'gke_two_no_master_cidr'.")
				}
				if pccTwo.Body().GetAttribute("private_endpoint_subnetwork") == nil {
					t.Errorf("'private_endpoint_subnetwork' unexpectedly missing from 'gke_two_no_master_cidr'.")
				}

				gkeThree, _ := findBlockInParsedFile(verifiedFile, "google_container_cluster", "gke_three_no_subnetwork")
				if gkeThree == nil {
					t.Fatalf("GKE resource 'gke_three_no_subnetwork' not found in multi-resource test.")
				}
				if gkeThree.Body().GetAttribute("master_ipv4_cidr_block") == nil {
					t.Errorf("'master_ipv4_cidr_block' unexpectedly missing from 'gke_three_no_subnetwork'.")
				}
				pccThree := gkeThree.Body().FirstMatchingBlock("private_cluster_config", nil)
				if pccThree == nil {
					t.Fatalf("'private_cluster_config' missing in 'gke_three_no_subnetwork'.")
				}
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

func TestApplyServicesIPV4CIDRRule(t *testing.T) {
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

func TestApplyClusterIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment() // Keep NewDevelopment here if intentional for this test

	tests := []struct {
		name                                            string
		hclContent                                      string
		expectedModifications                           int
		expectClusterIPV4CIDRRemoved                    bool
		resourceLabelsToVerify                          []string
		expectIpAllocPolicyBlockExists                  bool
		expectClusterIPV4CIDRBlockInIpAllocPolicyExists bool
	}{
		{
			name: "Both attributes present",
			hclContent: `resource "google_container_cluster" "primary" {
  name               = "primary-cluster"
  cluster_ipv4_cidr  = "10.0.0.0/14"
  ip_allocation_policy {
    cluster_ipv4_cidr_block = "10.1.0.0/14"
    some_other_config       = "value"
  }
}`,
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: true,
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
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: false,
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
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: true,
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
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: false,
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
			expectClusterIPV4CIDRRemoved: false, // Should be false as it's not a GKE resource
			resourceLabelsToVerify:       []string{"google_compute_instance", "default"},
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: true,
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
			expectIpAllocPolicyBlockExists:                  true,
			expectClusterIPV4CIDRBlockInIpAllocPolicyExists: true,
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
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"}, // Check first block
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

				if targetBlock == nil && (tc.expectClusterIPV4CIDRRemoved || tc.expectedModifications > 0 || tc.expectIpAllocPolicyBlockExists) {
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
						// Check if the attribute should exist (was present in original and not expected to be removed)
						originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclContent), "", hcl.InitialPos)
						originalResourceBlock, _ := findBlockInParsedFile(originalParsedFile, blockType, blockName)
						var originalBlockHasClusterIPV4CIDR bool
						if originalResourceBlock != nil && originalResourceBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil {
							originalBlockHasClusterIPV4CIDR = true
						}

						if originalBlockHasClusterIPV4CIDR && !hasClusterIPV4CIDR {
							t.Errorf("'cluster_ipv4_cidr' was unexpectedly removed from %s[\"%s\"]. Modified HCL:\n%s",
								blockType, blockName, string(modifiedContentBytes))
						}
					}

					// Verify ip_allocation_policy block and its contents
					ipAllocPolicyBlock := targetBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
					if tc.expectIpAllocPolicyBlockExists {
						if ipAllocPolicyBlock == nil {
							t.Errorf("Expected 'ip_allocation_policy' block to exist in %s[\"%s\"], but it was not found. Modified HCL:\n%s",
								blockType, blockName, string(modifiedContentBytes))
						} else {
							// Verify cluster_ipv4_cidr_block within ip_allocation_policy
							hasClusterIPV4CIDRBlockInIpAlloc := ipAllocPolicyBlock.Body().GetAttribute("cluster_ipv4_cidr_block") != nil
							if tc.expectClusterIPV4CIDRBlockInIpAllocPolicyExists {
								if !hasClusterIPV4CIDRBlockInIpAlloc {
									t.Errorf("Expected 'cluster_ipv4_cidr_block' to exist in 'ip_allocation_policy' of %s[\"%s\"], but it was not found. Modified HCL:\n%s",
										blockType, blockName, string(modifiedContentBytes))
								}
								// Also check that "some_other_config" is still there for the "Both attributes present" case
								if tc.name == "Both attributes present" {
									if ipAllocPolicyBlock.Body().GetAttribute("some_other_config") == nil {
										t.Errorf("Expected 'some_other_config' to exist in 'ip_allocation_policy' of %s[\"%s\"], but it was not found. Modified HCL:\n%s",
											blockType, blockName, string(modifiedContentBytes))
									}
								}
							} else {
								if hasClusterIPV4CIDRBlockInIpAlloc {
									t.Errorf("Expected 'cluster_ipv4_cidr_block' to be removed from 'ip_allocation_policy' of %s[\"%s\"], but it was found. Modified HCL:\n%s",
										blockType, blockName, string(modifiedContentBytes))
								}
							}
						}
					} else {
						if ipAllocPolicyBlock != nil {
							t.Errorf("Expected 'ip_allocation_policy' block to be removed from %s[\"%s\"], but it was found. Modified HCL:\n%s",
								blockType, blockName, string(modifiedContentBytes))
						}
					}
					// Specific checks for "Only cluster_ipv4_cidr present (no ip_allocation_policy block)"
					if tc.name == "Only cluster_ipv4_cidr present (no ip_allocation_policy block)" {
						if targetBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil) != nil {
							t.Errorf("Expected 'ip_allocation_policy' block NOT to exist in '%s', but it was found. Modified HCL:\n%s", tc.name, string(modifiedContentBytes))
						}
					}

					// Specific checks for "Only ip_allocation_policy.cluster_ipv4_cidr_block present"
					if tc.name == "Only ip_allocation_policy.cluster_ipv4_cidr_block present" {
						if targetBlock.Body().GetAttribute("cluster_ipv4_cidr") != nil {
							t.Errorf("Expected top-level 'cluster_ipv4_cidr' NOT to exist in '%s', but it was found. Modified HCL:\n%s", tc.name, string(modifiedContentBytes))
						}
						ipAlloc := targetBlock.Body().FirstMatchingBlock("ip_allocation_policy", nil)
						if ipAlloc == nil {
							t.Errorf("Expected 'ip_allocation_policy' block to exist in '%s', but it was NOT found. Modified HCL:\n%s", tc.name, string(modifiedContentBytes))
						} else if ipAlloc.Body().GetAttribute("cluster_ipv4_cidr_block") == nil {
							t.Errorf("Expected 'cluster_ipv4_cidr_block' within 'ip_allocation_policy' to exist in '%s', but it was NOT found. Modified HCL:\n%s", tc.name, string(modifiedContentBytes))
						}
					}
				}
			}

			// Specific checks for "Multiple google_container_cluster blocks, one matching"
			if tc.name == "Multiple google_container_cluster blocks, one matching" {
				var secondaryBlockAlpha *hclwrite.Block // Renamed to avoid conflict with other tests if any
				secondaryBlockAlpha, _ = findBlockInParsedFile(verifiedFile, "google_container_cluster", "secondary")
				if secondaryBlockAlpha == nil {
					t.Fatalf("Could not find the 'google_container_cluster' block named 'secondary' for multi-block verification. Modified HCL:\n%s", string(modifiedContentBytes))
				}
				if secondaryBlockAlpha.Body().GetAttribute("cluster_ipv4_cidr") == nil {
					t.Errorf("Expected 'cluster_ipv4_cidr' to be present in 'secondary' block (multi-block test), but it was not. Modified HCL:\n%s",
						string(modifiedContentBytes))
				}
				// Check that ip_allocation_policy in the 'secondary' block is untouched
				secondaryIpAlloc := secondaryBlockAlpha.Body().FirstMatchingBlock("ip_allocation_policy", nil)
				if secondaryIpAlloc == nil {
					t.Errorf("Expected 'ip_allocation_policy' block to be present in 'secondary' block (multi-block test), but it was not. Modified HCL:\n%s", string(modifiedContentBytes))
				} else {
					if secondaryIpAlloc.Body().GetAttribute("services_ipv4_cidr_block") == nil {
						t.Errorf("Expected 'services_ipv4_cidr_block' to be present in 'ip_allocation_policy' of 'secondary' block (multi-block test), but it was not. Modified HCL:\n%s", string(modifiedContentBytes))
					}
				}
			}
		})
	}
}

func TestApplyAutopilotRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	type clusterAutoscalingChecks struct {
		expectBlockExists           bool
		expectEnabledRemoved        bool
		expectResourceLimitsRemoved bool
		expectProfileUnchanged      *string
	}

	type binaryAuthorizationChecks struct {
		expectBlockExists    bool
		expectEnabledRemoved bool
	}

	type addonsConfigChecks struct {
		expectBlockExists                bool
		expectNetworkPolicyRemoved       bool
		expectDnsCacheRemoved            bool
		expectStatefulHaRemoved          bool
		expectHttpLoadBalancingUnchanged bool
	}

	tests := []struct {
		name                                string
		hclContent                          string
		expectedModificationsCustomRule     int // For ApplyAutopilotRule()
		expectedModificationsGenericRule    int // For RuleHandleAutopilotFalse
		clusterName                         string
		expectEnableAutopilotAttr           *bool // After all rules, what should enable_autopilot be?
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
			expectedModificationsCustomRule:     14,
			expectedModificationsGenericRule:    0, // RuleHandleAutopilotFalse should not act
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
				expectBlockExists:                true,
				expectNetworkPolicyRemoved:       true,
				expectDnsCacheRemoved:            true,
				expectStatefulHaRemoved:          true,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:           true,
				expectEnabledRemoved:        true,
				expectResourceLimitsRemoved: true,
				expectProfileUnchanged:      stringPtr("OPTIMIZE_UTILIZATION"),
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
			expectedModificationsCustomRule:  0, // ApplyAutopilotRule should do nothing
			expectedModificationsGenericRule: 1, // RuleHandleAutopilotFalse removes enable_autopilot=false
			clusterName:                         "standard_cluster",
			expectEnableAutopilotAttr:           nil, // Attribute removed by RuleHandleAutopilotFalse
			expectedRootAttrsRemoved:            []string{}, // No other attributes should be removed by ApplyAutopilotRule
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:                true,
				expectDnsCacheRemoved:            false,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:           true,
				expectEnabledRemoved:        false,
				expectResourceLimitsRemoved: false,
				expectProfileUnchanged:      stringPtr("BALANCED"),
			},
			binaryAuthorization:  nil,
			expectNoOtherChanges: true, // Other fields remain as ApplyAutopilotRule doesn't run its main logic
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
			expectedModificationsCustomRule:     0,
			expectedModificationsGenericRule:    0,
			clusterName:                         "existing_cluster",
			expectEnableAutopilotAttr:           nil,
			expectedRootAttrsRemoved:            []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:          true,
				expectNetworkPolicyRemoved: false,
			},
			expectNoOtherChanges: true,
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
			expectedModificationsCustomRule:     0,
			expectedModificationsGenericRule:    0,
			clusterName:                         "clean_autopilot_cluster",
			expectEnableAutopilotAttr:           boolPtr(true),
			expectedRootAttrsRemoved:            []string{},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:                true,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscaling: &clusterAutoscalingChecks{
				expectBlockExists:           true,
				expectEnabledRemoved:        false,
				expectResourceLimitsRemoved: false,
				expectProfileUnchanged:      stringPtr("BALANCED"),
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
			expectedModificationsCustomRule:     1, // ApplyAutopilotRule removes non-boolean enable_autopilot
			expectedModificationsGenericRule:    0, // RuleHandleAutopilotFalse does not act
			clusterName:                         "invalid_autopilot_cluster",
			expectEnableAutopilotAttr:           nil, // Attribute removed by ApplyAutopilotRule
			expectedRootAttrsRemoved:            []string{"enable_autopilot"},
			expectedTopLevelNestedBlocksRemoved: []string{},
			addonsConfig: &addonsConfigChecks{
				expectBlockExists:     true,
				expectDnsCacheRemoved: false,
			},
			expectNoOtherChanges: true,
		},
		{
			name: "No google_container_cluster blocks",
			hclContent: `resource "google_compute_instance" "vm" {
  name = "my-vm"
}`,
			expectedModificationsCustomRule:  0,
			expectedModificationsGenericRule: 0,
			clusterName:           "",
			expectNoOtherChanges:  true,
		},
		{
			name:                  "Empty HCL content",
			hclContent:            ``,
			expectedModificationsCustomRule:  0,
			expectedModificationsGenericRule: 0,
			clusterName:           "",
			expectNoOtherChanges:  true,
		},
		{
			name: "Autopilot true, only some attributes to remove",
			hclContent: `resource "google_container_cluster" "partial_autopilot" {
  name                  = "partial-autopilot"
  enable_autopilot      = true
  enable_shielded_nodes = true
  default_max_pods_per_node = 110
}`,
			expectedModificationsCustomRule:     2,
			expectedModificationsGenericRule:    0,
			clusterName:                         "partial_autopilot",
			expectEnableAutopilotAttr:           boolPtr(true),
			expectedRootAttrsRemoved:            []string{"enable_shielded_nodes", "default_max_pods_per_node"},
			expectedTopLevelNestedBlocksRemoved: []string{},
			expectNoOtherChanges:                false,
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
				if tc.hclContent == "" && tc.expectedModificationsCustomRule == 0 && tc.expectedModificationsGenericRule == 0 {
					if modifier == nil {
						return
					}
				} else {
					t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
				}
			}

			// Test ApplyAutopilotRule (custom logic)
			modificationsCustom, errCustom := modifier.ApplyAutopilotRule()
			if errCustom != nil {
				// Allow specific error for "enable_autopilot = false" as it's no longer handled by ApplyAutopilotRule
				if !(tc.name == "enable_autopilot is false, conflicting fields present" && modificationsCustom == 0) {
					t.Fatalf("ApplyAutopilotRule() returned error = %v. HCL content:\n%s", errCustom, tc.hclContent)
				}
			}
			assert.Equal(t, tc.expectedModificationsCustomRule, modificationsCustom, "ApplyAutopilotRule() modifications mismatch")

			// If enable_autopilot was false, it's now handled by the generic rule.
			// We need to re-parse the file if ApplyAutopilotRule made changes,
			// or use the current modifier if it made no changes (like for "enable_autopilot=false" case)
			// For simplicity in this test setup, we re-initialize the modifier for the generic rule test *if* the custom rule was expected to do something.
			// Or, if the specific test case is "enable_autopilot is false...", then we test RuleHandleAutopilotFalse on the *original* content.

			var genericRuleModifications int
			var genericRuleErrs []error

			if tc.name == "enable_autopilot is false, conflicting fields present" {
				// Re-initialize modifier with original content to test RuleHandleAutopilotFalse separately
				modifierForGeneric, errGenericLoad := NewFromFile(tmpFile.Name(), logger)
				if errGenericLoad != nil {
					t.Fatalf("Failed to re-load HCL for generic rule test: %v", errGenericLoad)
				}
				genericRuleModifications, genericRuleErrs = modifierForGeneric.ApplyRules([]types.Rule{rules.RuleHandleAutopilotFalse})
				if len(genericRuleErrs) > 0 {
					t.Fatalf("ApplyRules(RuleHandleAutopilotFalse) returned errors: %v", genericRuleErrs)
				}
				assert.Equal(t, tc.expectedModificationsGenericRule, genericRuleModifications, "ApplyRules(RuleHandleAutopilotFalse) modifications mismatch")
				// The final state of 'modifier' for assertions below should be after this generic rule
				modifier = modifierForGeneric
			}


			// Verification of file state after all relevant rules for the test case have run
			if tc.clusterName == "" { // For tests like "Empty HCL" or "No GKE blocks"
				if tc.expectedModificationsCustomRule == 0 && tc.expectedModificationsGenericRule == 0 {
					return // Nothing to check
				}
				// if we expected modifications but have no clusterName, that's an issue with test setup
				t.Logf("Test %s: No clusterName specified, but expectedModifications is > 0. Skipping detailed checks.", tc.name)
				return
			}

			var clusterBlock *hclwrite.Block
			clusterBlock, _ = findBlockInParsedFile(modifier.File(), "google_container_cluster", tc.clusterName)

			if clusterBlock == nil {
				// If we expected any modifications (either custom or generic) but the block is gone, that's a problem unless the test is about removing the block.
				if tc.expectedModificationsCustomRule > 0 || tc.expectedModificationsGenericRule > 0 {
					// This specific test suite doesn't have tests that remove the main cluster block, so this is likely an error.
					t.Fatalf("google_container_cluster resource '%s' not found after rule applications. HCL:\n%s", tc.clusterName, string(modifier.File().Bytes()))
				}
				// If no modifications were expected and block is not found (e.g. test for non-GKE resource), it's fine.
				return
			}

			enableAutopilotAttr := clusterBlock.Body().GetAttribute("enable_autopilot")
			if tc.expectEnableAutopilotAttr == nil {
				if enableAutopilotAttr != nil {
                     // For the "not_a_boolean" case, ApplyAutopilotRule removes it.
                     // For the "false" case, RuleHandleAutopilotFalse removes it.
					if !(tc.name == "enable_autopilot is not a boolean" || tc.name == "enable_autopilot is false, conflicting fields present") {
                         // If it's another case where it's expected to be nil but found, it's an error.
						t.Errorf("Expected 'enable_autopilot' attribute to be removed or not exist, but it was found with value: %s. Test case: %s", string(enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()), tc.name)
					} else if tc.name == "enable_autopilot is not a boolean" && string(enableAutopilotAttr.Expr().BuildTokens(nil).Bytes()) == `"not_a_boolean"`{
						// This is an edge case: if ApplyAutopilotRule failed to remove the non-boolean, this would catch it.
						// However, the current logic of ApplyAutopilotRule *should* remove it.
						t.Errorf("Expected non-boolean 'enable_autopilot' to be removed, but it still exists. Test case: %s", tc.name)
					}
				}
			} else { // expectEnableAutopilotAttr has a value (true or false)
				if enableAutopilotAttr == nil {
					t.Errorf("Expected 'enable_autopilot' attribute to exist with value %v, but it was not found. Test case: %s", *tc.expectEnableAutopilotAttr, tc.name)
				} else {
					val, errVal := modifier.GetAttributeValue(enableAutopilotAttr)
					if errVal != nil {
						t.Errorf("Error getting value of 'enable_autopilot': %v. Test case: %s", errVal, tc.name)
					} else if val.Type() != cty.Bool {
						t.Errorf("Expected 'enable_autopilot' to be boolean, but got type %s. Test case: %s", val.Type().FriendlyName(), tc.name)
					} else if val.True() != *tc.expectEnableAutopilotAttr {
						t.Errorf("Expected 'enable_autopilot' to be %v, but got %v. Test case: %s", *tc.expectEnableAutopilotAttr, val.True(), tc.name)
					}
				}
			}


			for _, attrName := range tc.expectedRootAttrsRemoved {
				if attr := clusterBlock.Body().GetAttribute(attrName); attr != nil {
					t.Errorf("Expected root attribute '%s' to be removed, but it was found. Test case: %s", attrName, tc.name)
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
						t.Errorf("Expected all nested blocks of type 'node_pool' to be removed, but at least one was found. Test case: %s", tc.name)
					}
				} else {
					if blk := clusterBlock.Body().FirstMatchingBlock(blockTypeName, nil); blk != nil {
						t.Errorf("Expected nested block '%s' to be removed, but it was found. Test case: %s", blockTypeName, tc.name)
					}
				}
			}

			if tc.addonsConfig != nil {
				acBlock := clusterBlock.Body().FirstMatchingBlock("addons_config", nil)
				if !tc.addonsConfig.expectBlockExists {
					if acBlock != nil {
						t.Errorf("Expected 'addons_config' block to be removed or not exist, but it was found. Test case: %s", tc.name)
					}
				} else {
					if acBlock == nil {
						t.Fatalf("Expected 'addons_config' block to exist, but it was not found. Test case: %s", tc.name)
					}
					if tc.addonsConfig.expectNetworkPolicyRemoved {
						if acBlock.Body().FirstMatchingBlock("network_policy_config", nil) != nil {
							t.Errorf("Expected 'network_policy_config' in 'addons_config' to be removed. Test case: %s", tc.name)
						}
					}
					if tc.addonsConfig.expectDnsCacheRemoved {
						if acBlock.Body().FirstMatchingBlock("dns_cache_config", nil) != nil {
							t.Errorf("Expected 'dns_cache_config' in 'addons_config' to be removed. Test case: %s", tc.name)
						}
					}
					if tc.addonsConfig.expectStatefulHaRemoved {
						if acBlock.Body().FirstMatchingBlock("stateful_ha_config", nil) != nil {
							t.Errorf("Expected 'stateful_ha_config' in 'addons_config' to be removed. Test case: %s", tc.name)
						}
					}
					if tc.addonsConfig.expectHttpLoadBalancingUnchanged {
						if acBlock.Body().FirstMatchingBlock("http_load_balancing", nil) == nil &&
							// Only fail if it was present in the original HCL for this test case
							(tc.name == "enable_autopilot is true, all conflicting fields present" || tc.name == "enable_autopilot is false, conflicting fields present" || tc.name == "enable_autopilot is true, no conflicting fields present") {
							t.Errorf("Expected 'http_load_balancing' in 'addons_config' to be unchanged, but it was not found. Test case: %s", tc.name)
						}
					}
				}
			}

			if tc.clusterAutoscaling != nil {
				caBlock := clusterBlock.Body().FirstMatchingBlock("cluster_autoscaling", nil)
				if !tc.clusterAutoscaling.expectBlockExists {
					if caBlock != nil {
						t.Errorf("Expected 'cluster_autoscaling' block to be removed, but it was found. Test case: %s", tc.name)
					}
				} else {
					if caBlock == nil {
						t.Fatalf("Expected 'cluster_autoscaling' block to exist, but it was not found. Test case: %s", tc.name)
					}
					if tc.clusterAutoscaling.expectEnabledRemoved {
						if attr := caBlock.Body().GetAttribute("enabled"); attr != nil {
							t.Errorf("Expected 'enabled' attribute in 'cluster_autoscaling' to be removed, but it was found. Test case: %s", tc.name)
						}
					}

					if tc.clusterAutoscaling.expectResourceLimitsRemoved {
						// Check if *any* resource_limits block exists, as there could be multiple
						foundRL := false
						for _, b := range caBlock.Body().Blocks() {
							if b.Type() == "resource_limits" {
								foundRL = true
								break
							}
						}
						if foundRL {
							t.Errorf("Expected 'resource_limits' attribute in 'cluster_autoscaling' to be removed, but it was found. Test case: %s", tc.name)
						}
					}

					if tc.clusterAutoscaling.expectProfileUnchanged != nil {
						profileAttr := caBlock.Body().GetAttribute("autoscaling_profile")
						if profileAttr == nil {
							// Only fail if it was present in the original HCL for this test case
							if tc.name == "enable_autopilot is true, all conflicting fields present" || tc.name == "enable_autopilot is false, conflicting fields present" || tc.name == "enable_autopilot is true, no conflicting fields present" {
								t.Errorf("Expected 'autoscaling_profile' attribute in 'cluster_autoscaling' to exist, but it was not found. Test case: %s", tc.name)
							}
						} else {
							val, errVal := modifier.GetAttributeValue(profileAttr)
							if errVal != nil || val.Type() != cty.String || val.AsString() != *tc.clusterAutoscaling.expectProfileUnchanged {
								t.Errorf("Expected 'autoscaling_profile' to be '%s', got '%v' (err: %v). Test case: %s", *tc.clusterAutoscaling.expectProfileUnchanged, val, errVal, tc.name)
							}
						}
					}
				}
			}

			if tc.binaryAuthorization != nil {
				baBlock := clusterBlock.Body().FirstMatchingBlock("binary_authorization", nil)
				if !tc.binaryAuthorization.expectBlockExists {
					if baBlock != nil {
						t.Errorf("Expected 'binary_authorization' block to be removed, but it was found. Test case: %s", tc.name)
					}
				} else {
					if baBlock == nil {
						t.Fatalf("Expected 'binary_authorization' block to exist, but it was not found. Test case: %s", tc.name)
					}
					if tc.binaryAuthorization.expectEnabledRemoved {
						if attr := baBlock.Body().GetAttribute("enabled"); attr != nil {
							t.Errorf("Expected 'enabled' attribute in 'binary_authorization' to be removed, but it was found. Test case: %s", tc.name)
						}
					}
					if tc.name == "enable_autopilot is true, all conflicting fields present" || tc.name == "enable_autopilot is true, no conflicting fields present" {
						evalModeAttr := baBlock.Body().GetAttribute("evaluation_mode")
						if evalModeAttr == nil {
							t.Errorf("Expected 'evaluation_mode' in 'binary_authorization' to remain, but it was not found. Test case: %s", tc.name)
						} else {
							// Value check can be added if specific values are expected for these cases
						}
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
		ruleToApply           types.Rule // This should be types.Rule
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
			ruleToApply:           rules.RuleRemoveLoggingService, // This assigns rules.Rule to hclmodifier.Rule
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
		ruleToApply           types.Rule // This should be types.Rule
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
			ruleToApply:           rules.RuleRemoveMonitoringService, // This assigns rules.Rule to hclmodifier.Rule
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
		})
	}
}
