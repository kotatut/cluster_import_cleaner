package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

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
