package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

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
