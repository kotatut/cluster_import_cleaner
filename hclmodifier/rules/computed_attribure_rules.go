package rules

import (
	"fmt"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

// TopLevelComputedAttributesRule defines a rule to clean computed attributes
// as 'label_fingerprint' or 'master_auth.cluster_ca_certificate'
//
// Why it's necessary for GKE imports: such attributes should not be used during apply since they are provided by GCP.
var TopLevelComputedAttributesRules = []types.Rule{
	createRemoveAttributeRule("google_container_cluster", []string{"endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"effective_labels"}),
	createRemoveAttributeRule("google_container_cluster", []string{"id"}),
	createRemoveAttributeRule("google_container_cluster", []string{"label_fingerprint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"self_link"}),
}

var OtherComputedAttributesRules = []types.Rule{
	createRemoveAttributeRule("google_container_cluster", []string{"label_fingerprint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"self_link"}),
	createRemoveAttributeRule("google_container_cluster", []string{"master_auth", "cluster_ca_certificate"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "instance_group_urls"}),
	createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "managed_instance_group_urls"}),
	// createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "autoscaling", "total_max_node_count"}), // Will be handled by dedicated rule
	// createRemoveAttributeRule("google_container_cluster", []string{"node_pool", "autoscaling", "total_min_node_count"}), // Will be handled by dedicated rule
	createRemoveAttributeRule("google_container_cluster", []string{"private_cluster_config", "private_endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"private_cluster_config", "public_endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"control_plane_endpoints_config", "dns_endpoint_config", "endpoint"}),
	createRemoveAttributeRule("google_container_cluster", []string{"database_encryption", "state"}),
}

// RemoveNodePoolTotalMaxNodeCountRuleDefinition removes total_max_node_count from all node_pool.autoscaling blocks.
var RemoveNodePoolTotalMaxNodeCountRuleDefinition = types.Rule{
	Name:                  "Remove total_max_node_count from node_pool.autoscaling",
	TargetResourceType:    "google_container_cluster",
	ExecutionType:         types.RuleExecutionForEachNestedBlock,
	NestedBlockTargetType: "node_pool",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"autoscaling", "total_max_node_count"}, // Path relative to node_pool body
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"autoscaling", "total_max_node_count"}, // Path relative to node_pool body
		},
	},
}

// RemoveNodePoolTotalMinNodeCountRuleDefinition removes total_min_node_count from all node_pool.autoscaling blocks.
var RemoveNodePoolTotalMinNodeCountRuleDefinition = types.Rule{
	Name:                  "Remove total_min_node_count from node_pool.autoscaling",
	TargetResourceType:    "google_container_cluster",
	ExecutionType:         types.RuleExecutionForEachNestedBlock,
	NestedBlockTargetType: "node_pool",
	Conditions: []types.RuleCondition{
		{
			Type: types.AttributeExists,
			Path: []string{"autoscaling", "total_min_node_count"}, // Path relative to node_pool body
		},
	},
	Actions: []types.RuleAction{
		{
			Type: types.RemoveAttribute,
			Path: []string{"autoscaling", "total_min_node_count"}, // Path relative to node_pool body
		},
	},
}

func createRemoveAttributeRule(resourceType string, path []string) types.Rule {
	return types.Rule{
		Name:               fmt.Sprintf("Remove attribute '%s' from '%s'", path, resourceType),
		TargetResourceType: resourceType,
		Conditions: []types.RuleCondition{
			{
				Type: types.AttributeExists,
				Path: path,
			},
		},
		Actions: []types.RuleAction{
			{
				Type: types.RemoveAttribute,
				Path: path,
			},
		},
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
    key_name = "some_key"
  }
}`,
			rulesToApply:          rules.OtherComputedAttributesRules, // This slice now includes the new rules
			expectedModifications: 2,
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
      total_max_node_count = 5
      // total_min_node_count is absent
    }
  }
  node_pool {
    name = "pool3" // No autoscaling block
  }
  node_pool {
    name = "pool4"
    autoscaling { // Autoscaling without target attributes
      location_policy = "ANY"
    }
  }
  node_pool {
    name = "pool5"
    autoscaling { // Only one of the target attributes
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
    autoscaling {}
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
    autoscaling {}
  }
}`,
			rulesToApply: []types.Rule{
				rules.RemoveNodePoolTotalMaxNodeCountRuleDefinition,
				rules.RemoveNodePoolTotalMinNodeCountRuleDefinition,
			},
			expectedModifications: 3, // pool1 (max, min), pool2 (max), pool5 (min)
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
