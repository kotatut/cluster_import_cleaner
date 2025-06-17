package hclmodifier

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
)

// HCL Test Snippet Constants and Helpers

const basicGKEClusterTemplate = `resource "google_container_cluster" "%s" {
  name     = "%s"
  location = "us-central1"
%s // for additional attributes
%s // for additional blocks
}`

// getBasicGKECluster formats a basic GKE cluster HCL string.
// attrs and blocks should be pre-formatted strings with correct indentation and newlines.
func getBasicGKECluster(resourceName, clusterName, attrs, blocks string) string {
	return fmt.Sprintf(basicGKEClusterTemplate, resourceName, clusterName, attrs, blocks)
}

const nodePoolTemplate = `
  node_pool {
    name = "%s"
%s // for additional attributes
  }`

// getNodePoolBlock formats a node_pool block HCL string.
// attrs should be pre-formatted strings with correct indentation and newlines.
func getNodePoolBlock(poolName, attrs string) string {
	return fmt.Sprintf(nodePoolTemplate, poolName, attrs)
}

const monitoringConfigTemplate = `
  monitoring_config {
%s // for additional attributes
  }`

// getMonitoringConfigBlock formats a monitoring_config block HCL string.
// attrs should be pre-formatted strings with correct indentation and newlines.
func getMonitoringConfigBlock(attrs string) string {
	return fmt.Sprintf(monitoringConfigTemplate, attrs)
}

const advancedDatapathObservabilityConfigTemplate = `
    advanced_datapath_observability_config {
%s // for additional attributes
    }`

// getAdvancedDatapathObservabilityConfigBlock formats an advanced_datapath_observability_config block HCL string.
// attrs should be pre-formatted strings with correct indentation and newlines.
func getAdvancedDatapathObservabilityConfigBlock(attrs string) string {
	return fmt.Sprintf(advancedDatapathObservabilityConfigTemplate, attrs)
}

const clusterTelemetryTemplate = `
  cluster_telemetry {
    type = "%s"
  }`

func getClusterTelemetryBlock(telemetryType string) string {
	return fmt.Sprintf(clusterTelemetryTemplate, telemetryType)
}

const addonsConfigWithHttpLoadBalancingTemplate = `
  addons_config {
    http_load_balancing {
      disabled = %t
    }
  }`

func getAddonsConfigWithHttpLoadBalancing(disabled bool) string {
	return fmt.Sprintf(addonsConfigWithHttpLoadBalancingTemplate, disabled)
}

// getNestedBlock creates a generic nested block string.
// content should be pre-formatted with correct indentation and newlines.
func getNestedBlock(blockName, content string) string {
	// Ensure content starts with a newline if it's not empty, for proper formatting.
	if content != "" && !strings.HasPrefix(content, "\n") && !strings.HasPrefix(content, " ") {
	if content != "" && !strings.HasPrefix(content, "\n") && !strings.HasPrefix(content, " ") {
		content = "\n    " + strings.TrimSpace(content) // Add indentation if missing
	}
	// Ensure content ends with a newline if it's not empty, before closing brace.
	if content != "" && !strings.HasSuffix(content, "\n") {
		content = content + "\n  "
	} else if content != "" {
		// Ensure there's some space before the closing brace if content already ends with a newline
		content = content + "  "
	}

	return fmt.Sprintf(`
  %s {%s}`, blockName, content)
}

const privateClusterConfigTemplate = `
  private_cluster_config {
%s // attributes
  }`

func getPrivateClusterConfig(attrs string) string {
	return fmt.Sprintf(privateClusterConfigTemplate, attrs)
}

const binaryAuthorizationTemplate = `
  binary_authorization {
%s // attributes
  }`

func getBinaryAuthorizationBlock(attrs string) string {
	return fmt.Sprintf(binaryAuthorizationTemplate, attrs)
}

const ipAllocationPolicyTemplate = `
  ip_allocation_policy {
%s // attributes
  }`

func getIpAllocationPolicyBlock(attrs string) string {
	return fmt.Sprintf(ipAllocationPolicyTemplate, attrs)
}

const nodeConfigTemplate = `
  node_config {
%s // attributes
  }`

func getNodeConfigBlock(attrs string) string {
	return fmt.Sprintf(nodeConfigTemplate, attrs)
}

const addonsConfigTemplate = `
  addons_config {
%s // nested blocks or attributes
  }`

func getAddonsConfigBlock(content string) string {
	return fmt.Sprintf(addonsConfigTemplate, content)
}

const networkPolicyConfigTemplate = `
    network_policy_config {
%s // attributes
    }`

func getNetworkPolicyConfigBlock(attrs string) string {
	return fmt.Sprintf(networkPolicyConfigTemplate, attrs)
}

const dnsCacheConfigTemplate = `
    dns_cache_config {
%s // attributes
    }`

func getDnsCacheConfigBlock(attrs string) string {
	return fmt.Sprintf(dnsCacheConfigTemplate, attrs)
}

const statefulHaConfigTemplate = `
    stateful_ha_config {
%s // attributes
    }`

func getStatefulHAConfigBlock(attrs string) string { // Renamed for consistency
	return fmt.Sprintf(statefulHaConfigTemplate, attrs)
}

const clusterAutoscalingTemplate = `
  cluster_autoscaling {
%s // attributes and sub-blocks
  }`

func getClusterAutoscalingBlock(content string) string {
	return fmt.Sprintf(clusterAutoscalingTemplate, content)
}

const resourceLimitsTemplate = `
      resource_limits {
%s // attributes
      }`

func getResourceLimitsBlock(attrs string) string {
	return fmt.Sprintf(resourceLimitsTemplate, attrs)
}

const networkPolicyFrameworkBlockTemplate = `
  network_policy {
%s // attributes
  }`

// Renamed to avoid conflict with addons_config internal block
func getNetworkPolicyFrameworkBlock(attrs string) string {
	return fmt.Sprintf(networkPolicyFrameworkBlockTemplate, attrs)
}

// General Test Helper Functions

func createModifierFromHCL(t *testing.T, hclContent string, logger *zap.Logger) (*Modifier, string) {
	t.Helper()
	tmpFile, err := os.CreateTemp(t.TempDir(), "test_hcl_*.hcl")
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
		if hclContent != "" {
			t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
		}
	}
	return modifier, tmpFile.Name()
}

func assertAttributeExists(t *testing.T, modifier *Modifier, resourceLabels []string, attributePath []string) {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		t.Fatalf("Error getting resource block %v: %v", resourceLabels, err)
	}
	assert.NotNil(t, resourceBlock, "Resource block %v should exist", resourceLabels)
	if resourceBlock == nil {
		return
	}

	_, attr, err := modifier.GetAttributeValueByPath(resourceBlock.Body(), attributePath)
	assert.NoError(t, err, "Error getting attribute %v from %v", attributePath, resourceLabels)
	assert.NotNil(t, attr, "Attribute %v should exist in %v", attributePath, resourceLabels)
}

func assertAttributeAbsent(t *testing.T, modifier *Modifier, resourceLabels []string, attributePath []string) {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		return
	}
	assert.NotNil(t, resourceBlock, "Resource block %v should exist for checking attribute absence", resourceLabels)
	if resourceBlock == nil {
		return
	}

	_, attr, err := modifier.GetAttributeValueByPath(resourceBlock.Body(), attributePath)
	assert.Error(t, err, "Expected error when getting absent attribute %v from %v", attributePath, resourceLabels)
	assert.Nil(t, attr, "Attribute %v should be absent in %v", attributePath, resourceLabels)
}

func assertAttributeValue(t *testing.T, modifier *Modifier, resourceLabels []string, attributePath []string, expectedValue cty.Value) {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		t.Fatalf("Error getting resource block %v: %v", resourceLabels, err)
	}
	assert.NotNil(t, resourceBlock, "Resource block %v should exist", resourceLabels)
	if resourceBlock == nil {
		return
	}

	val, attr, err := modifier.GetAttributeValueByPath(resourceBlock.Body(), attributePath)
	assert.NoError(t, err, "Error getting attribute %v from %v", attributePath, resourceLabels)
	assert.NotNil(t, attr, "Attribute %v should exist in %v", attributePath, resourceLabels)
	if attr != nil {
		assert.True(t, expectedValue.Equals(val).True(), "Attribute %v in %v: expected %s, got %s", attributePath, resourceLabels, expectedValue.GoString(), val.GoString())
	}
}

func assertNestedBlockExists(t *testing.T, modifier *Modifier, resourceLabels []string, blockPath []string) {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		t.Fatalf("Error getting resource block %v: %v", resourceLabels, err)
	}
	assert.NotNil(t, resourceBlock, "Resource block %v should exist", resourceLabels)
	if resourceBlock == nil {
		return
	}

	nestedBlock, err := modifier.GetNestedBlock(resourceBlock.Body(), blockPath)
	assert.NoError(t, err, "Error getting nested block %v from %v", blockPath, resourceLabels)
	assert.NotNil(t, nestedBlock, "Nested block %v should exist in %v", blockPath, resourceLabels)
}

func assertNestedBlockAbsent(t *testing.T, modifier *Modifier, resourceLabels []string, blockPath []string) {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		return
	}
	assert.NotNil(t, resourceBlock, "Resource block %v should exist for checking nested block absence", resourceLabels)
	if resourceBlock == nil {
		return
	}

	nestedBlock, err := modifier.GetNestedBlock(resourceBlock.Body(), blockPath)
	assert.Error(t, err, "Expected error when getting absent nested block %v from %v", blockPath, resourceLabels)
	assert.Nil(t, nestedBlock, "Nested block %v should be absent in %v", blockPath, resourceLabels)
}

// Node Pool Specific Assertion Helpers
func findSpecificNodePool(t *testing.T, modifier *Modifier, resourceLabels []string, poolName string) *hclwrite.Block {
	t.Helper()
	resourceBlock, err := modifier.GetBlock("resource", resourceLabels)
	if err != nil {
		t.Fatalf("Error getting resource block %v for node pool checks: %v", resourceLabels, err)
		return nil
	}
	if resourceBlock == nil {
		 t.Fatalf("Resource block %v not found for node pool checks", resourceLabels)
		return nil
	}

	for _, block := range resourceBlock.Body().Blocks() {
		if block.Type() == "node_pool" {
			nameAttr := block.Body().GetAttribute("name")
			if nameAttr != nil {
				nameVal, valErr := modifier.GetAttributeValue(nameAttr)
				if valErr == nil && nameVal.Type() == cty.String && nameVal.AsString() == poolName {
					return block
				}
			} else if poolName == "" {
				// This logic might be too simplistic if multiple unnamed node_pools exist.
				// Consider returning a slice or requiring names for specific checks.
				return block
			}
		}
	}
	t.Logf("Node pool '%s' not found in resource '%s'", poolName, resourceLabels)
	return nil
}

func assertNodePoolAttributeExists(t *testing.T, modifier *Modifier, resourceLabels []string, poolName string, attributeName string) {
	t.Helper()
	nodePoolBlock := findSpecificNodePool(t, modifier, resourceLabels, poolName)
	assert.NotNil(t, nodePoolBlock, "Node pool '%s' in resource '%s' should exist", poolName, resourceLabels)
	if nodePoolBlock != nil {
		attr := nodePoolBlock.Body().GetAttribute(attributeName)
		assert.NotNil(t, attr, "Attribute '%s' should exist in node_pool '%s' of resource '%s'", attributeName, poolName, resourceLabels)
	}
}

func assertNodePoolAttributeAbsent(t *testing.T, modifier *Modifier, resourceLabels []string, poolName string, attributeName string) {
	t.Helper()
	nodePoolBlock := findSpecificNodePool(t, modifier, resourceLabels, poolName)
	if nodePoolBlock == nil {
		// If node pool is not found, attribute is considered absent. This is fine for this helper.
		// The test expecting the pool to exist should use assertNestedBlockExists or findSpecificNodePool directly and assert.NotNil.
		return
	}
	attr := nodePoolBlock.Body().GetAttribute(attributeName)
	assert.Nil(t, attr, "Attribute '%s' should be absent in node_pool '%s' of resource '%s'", attributeName, poolName, resourceLabels)
}

func assertNodePoolAttributeValue(t *testing.T, modifier *Modifier, resourceLabels []string, poolName string, attributeName string, expectedValue cty.Value) {
	t.Helper()
	nodePoolBlock := findSpecificNodePool(t, modifier, resourceLabels, poolName)
	assert.NotNil(t, nodePoolBlock, "Node pool '%s' in resource '%s' should exist", poolName, resourceLabels)
	if nodePoolBlock != nil {
		attr := nodePoolBlock.Body().GetAttribute(attributeName)
		assert.NotNil(t, attr, "Attribute '%s' should exist in node_pool '%s' of resource '%s'", attributeName, poolName, resourceLabels)
		if attr != nil {
			val, err := modifier.GetAttributeValue(attr)
			assert.NoError(t, err, "Error getting value of attribute '%s' in node_pool '%s'", attributeName, poolName)
			if err == nil {
				assert.True(t, expectedValue.Equals(val).True(), "Attribute '%s' in node_pool '%s': expected %s, got %s", attributeName, poolName, expectedValue.GoString(), val.GoString())
			}
		}
	}
}


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
			hclContent: getBasicGKECluster("test", "test-cluster", `
  enable_legacy_abac = true
  some_other_attr = "foo"
`, ""),
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
			hclContent: getBasicGKECluster("test", "test-cluster", `
  enable_legacy_abac = false
  some_other_attr = "bar"
`, ""),
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
			hclContent: getBasicGKECluster("test", "test-cluster", `
  max_pods_per_node = 110
  another_attr = "baz"
`, ""),
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
			hclContent: getBasicGKECluster("test", "test-cluster", `attr_to_remove = "qux"`,
				getMonitoringConfigBlock(
					getAdvancedDatapathObservabilityConfigBlock(`relay_log_level_percent = 50.5`),
				),
			),
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
			hclContent: getBasicGKECluster("test", "test-cluster", `
  enable_legacy_abac = true
  some_other_attr = "foo"
`, ""),
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
			modifier, _ := createModifierFromHCL(t, tc.hclContent, logger)
			// Temp file cleanup is handled by t.TempDir() used in createModifierFromHCL

			modifications, errs := modifier.ApplyRules([]types.Rule{tc.rule})
			assert.Empty(t, errs, "ApplyRules should not return errors for these test cases")
			assert.Equal(t, tc.expectedModifications, modifications)

			resourceLabels := []string{"google_container_cluster", "test"}
			if tc.hclContent == "" && tc.rule.TargetResourceType == "google_container_cluster" { // if the hclContent is empty, the resource block won't exist
				resourceLabels = []string{tc.rule.TargetResourceType, "test"} // default name if not specified in HCL
			} else if len(tc.rule.TargetResourceType) > 0 {
				if strings.Contains(tc.hclContent, `"test_cluster"`) {
					resourceLabels = []string{tc.rule.TargetResourceType, "test_cluster"}
				} else {
					resourceLabels = []string{tc.rule.TargetResourceType, "test"}
				}
			}

			if tc.expectAttributeRemoved {
				assertAttributeAbsent(t, modifier, resourceLabels, tc.attributePathToRemove)
			} else if tc.expectedModifications == 0 {
				assertAttributeExists(t, modifier, resourceLabels, tc.attributePathToRemove)
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
			hclContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  node_version   = "1.27.5-gke.200"
`, ""),
			expectedHCLContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.5-gke.200"
`, ""),
			expectedModifications: 1,
		},
		{
			name: "Rule Does Not Apply - min_master_version already present",
			hclContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.4-gke.100"
`, ""),
			expectedHCLContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  node_version       = "1.27.5-gke.200"
  min_master_version = "1.27.4-gke.100"
`, ""),
			expectedModifications: 0,
		},
		{
			name: "Rule Does Not Apply - node_version is absent",
			hclContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  min_master_version = "1.27.5-gke.200"
`, ""),
			expectedHCLContent: getBasicGKECluster("test_cluster", "my-gke-cluster", `
  min_master_version = "1.27.5-gke.200"
`, ""),
			expectedModifications: 0,
		},
		{
			name: "Rule Does Not Apply - Neither node_version nor min_master_version present",
			hclContent: getBasicGKECluster("test_cluster", "my-gke-cluster", "", ""),
			expectedHCLContent: getBasicGKECluster("test_cluster", "my-gke-cluster", "", ""),
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
			modifier, _ := createModifierFromHCL(t, tc.hclContent, logger)

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.SetMinVersionRuleDefinition})
			assert.Empty(t, errs, "ApplyRules should not return errors for these test cases")
			assert.Equal(t, tc.expectedModifications, modifications)

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
			hclContent: getBasicGKECluster("test", "test-cluster", `
  description = "Initial description"
`, ""),
			rule: types.Rule{
				Name:               "TestSetStringValue",
				TargetResourceType: "google_container_cluster",
				Conditions:         []types.RuleCondition{{Type: types.AttributeExists, Path: []string{"description"}}},
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"description"}, ValueToSet: "Updated description"}},
			},
			expectedModifications: 1,
			expectedHCLContent: getBasicGKECluster("test", "test-cluster", `
  description = "Updated description"
`, ""),
		},
		{
			name: "Set boolean attribute",
			hclContent: getBasicGKECluster("test", "test-cluster", `
  enable_shielded_nodes = false
`, ""),
			rule: types.Rule{
				Name:               "TestSetBoolValue",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"enable_shielded_nodes"}, ValueToSet: "true"}},
			},
			expectedModifications: 1,
			expectedHCLContent: getBasicGKECluster("test", "test-cluster", `
  enable_shielded_nodes = true
`, ""),
		},
		{
			name: "Set integer attribute",
			hclContent: getBasicGKECluster("test", "test-cluster", `
  node_locations = []
`, getMonitoringConfigBlock(
				getAdvancedDatapathObservabilityConfigBlock(`relay_log_level_percent = 0`),
			)),
			rule: types.Rule{
				Name:               "TestSetIntValue",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"monitoring_config", "advanced_datapath_observability_config", "relay_log_level_percent"}, ValueToSet: "75"}},
			},
			expectedModifications: 1,
			expectedHCLContent: getBasicGKECluster("test", "test-cluster", `
  node_locations = []
`, getMonitoringConfigBlock(
				getAdvancedDatapathObservabilityConfigBlock(`relay_log_level_percent = 75`),
			)),
		},
		{
			name: "Set new attribute",
			hclContent: getBasicGKECluster("test", "test-cluster", "", ""),
			rule: types.Rule{
				Name:               "TestSetNewAttribute",
				TargetResourceType: "google_container_cluster",
				Actions:            []types.RuleAction{{Type: types.SetAttributeValue, Path: []string{"new_attribute"}, ValueToSet: "new_value"}},
			},
			expectedModifications: 1,
			expectedHCLContent: getBasicGKECluster("test", "test-cluster", `
  new_attribute = "new_value"
`, ""),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclContent, logger)

			modifications, errs := modifier.ApplyRules([]types.Rule{tc.rule})
			assert.Empty(t, errs, "ApplyRules should not return errors for these test cases")
			assert.Equal(t, tc.expectedModifications, modifications)

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

// initialNodeCountTestCase is the struct for table-driven tests in TestApplyInitialNodeCountRule.
type initialNodeCountTestCase struct {
	name                         string
	hclInput                     string
	expectedModifications        int
	gkeResourceName              string
	nodePoolChecks               []nodePoolCheck // Reusing the existing nodePoolCheck struct
	expectNoOtherResourceChanges bool
	expectNoGKEResource          bool
}

func TestApplyInitialNodeCountRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	testCases := []initialNodeCountTestCase{
		{
			name: "BothCountsPresent",
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getNodePoolBlock("default-pool", `
    initial_node_count = 3
    node_count         = 5
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getNodePoolBlock("default-pool", `
    initial_node_count = 2
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getNodePoolBlock("default-pool", `
    node_count = 4
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getNodePoolBlock("default-pool", `
    autoscaling = true
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "",
				getNodePoolBlock("pool-one", `
    initial_node_count = 3
    node_count         = 5
`) + getNodePoolBlock("pool-two", `
    initial_node_count = 2
`) + getNodePoolBlock("pool-three", `
    node_count = 4
`) + getNodePoolBlock("pool-four", "")),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", ``, ""),
			expectedModifications: 0,
			gkeResourceName:       "gke_cluster",
			nodePoolChecks:        nil,
		},
		{
			name: "NonGKEResource",
			hclInput: `resource "google_compute_instance" "not_gke" {
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
			hclInput:            ``,
			expectedModifications: 0,
			gkeResourceName:       "",
			nodePoolChecks:        nil,
			expectNoGKEResource:   true,
		},
		{
			name: "MultipleGKEResources",
			hclInput: getBasicGKECluster("gke_one", "cluster-one", "", getNodePoolBlock("gke-one-pool", `
    initial_node_count = 3
    node_count         = 5
`)) + "\n" + getBasicGKECluster("gke_two", "cluster-two", "",
				getNodePoolBlock("gke-two-pool", `
    initial_node_count = 2
`) + getNodePoolBlock("gke-two-pool-extra", `
    node_count = 1
`)),
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for non-empty HCL input")
			}


			modifications, ruleErr := modifier.ApplyRules([]types.Rule{rules.InitialNodeCountRuleDefinition})
			assert.Empty(t, ruleErr, "ApplyRules returned errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, "verified.hcl", hcl.InitialPos)
			assert.False(t, diags.HasErrors(), "Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))

			if tc.expectNoGKEResource {
				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) > 0 && b.Labels()[0] == "google_container_cluster" {
						t.Errorf("Expected no 'google_container_cluster' resource, but found one: %v. HCL:\n%s", b.Labels(), string(modifiedContentBytes))
					}
				}
				return
			}

			resourceLabels := []string{"google_container_cluster", tc.gkeResourceName}
			if tc.gkeResourceName == "" && !tc.expectNoOtherResourceChanges {
				if len(tc.nodePoolChecks) > 0 {
					t.Fatalf("gkeResourceName must be set for node pool checks")
				}
			}


			var targetGKEResource *hclwrite.Block
			if tc.gkeResourceName != "" {
				var errBlock error
				targetGKEResource, errBlock = modifier.GetBlock("resource", resourceLabels)
				if errBlock != nil && len(tc.nodePoolChecks) > 0 {
					t.Fatalf("Expected 'google_container_cluster' resource '%s' not found for verification. Modified HCL:\n%s. Error: %v", tc.gkeResourceName, string(modifiedContentBytes), errBlock)
				}
				if targetGKEResource == nil && len(tc.nodePoolChecks) > 0 {
                     t.Fatalf("Expected 'google_container_cluster' resource '%s' not found (nil block) for verification. Modified HCL:\n%s", tc.gkeResourceName, string(modifiedContentBytes))
                }
			}


			if targetGKEResource != nil && tc.nodePoolChecks != nil {
				for _, npCheck := range tc.nodePoolChecks {
					nodePoolBlock := findSpecificNodePool(t, modifier, resourceLabels, npCheck.nodePoolName)

					if npCheck.expectInitialNodeCountRemoved {
						assertNodePoolAttributeAbsent(t, modifier, resourceLabels, npCheck.nodePoolName, "initial_node_count")
					} else {
						originalParsedFile, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
						originalGKEResource, _ := findBlockInParsedFile(originalParsedFile, resourceLabels[0], resourceLabels[1])
						var originalInitialPresent bool
						if originalGKEResource != nil {
							originalNP, _ := findNodePoolInBlock(originalGKEResource, npCheck.nodePoolName, modifier)
							if originalNP != nil && originalNP.Body().GetAttribute("initial_node_count") != nil {
								originalInitialPresent = true
							}
						}
						if originalInitialPresent { // If it was there, it should still be there
							assertNodePoolAttributeExists(t, modifier, resourceLabels, npCheck.nodePoolName, "initial_node_count")
						} else { // If it wasn't there, it should still be absent
							assertNodePoolAttributeAbsent(t, modifier, resourceLabels, npCheck.nodePoolName, "initial_node_count")
						}
					}

					if npCheck.expectNodeCountPresent {
						assertNodePoolAttributeExists(t, modifier, resourceLabels, npCheck.nodePoolName, "node_count")
						if npCheck.expectedNodeCountValue != nil {
							assertNodePoolAttributeValue(t, modifier, resourceLabels, npCheck.nodePoolName, "node_count", cty.NumberIntVal(int64(*npCheck.expectedNodeCountValue)))
						}
					} else {
						assertNodePoolAttributeAbsent(t, modifier, resourceLabels, npCheck.nodePoolName, "node_count")
					}
				}
			}

			if tc.name == "MultipleGKEResources" {
				gkeTwoResourceLabels := []string{"google_container_cluster", "gke_two"}
				assertNodePoolAttributeAbsent(t, modifier, gkeTwoResourceLabels, "gke-two-pool", "initial_node_count")
				assertNodePoolAttributeAbsent(t, modifier, gkeTwoResourceLabels, "gke-two-pool", "node_count")
				assertNodePoolAttributeAbsent(t, modifier, gkeTwoResourceLabels, "gke-two-pool-extra", "initial_node_count")
				assertNodePoolAttributeExists(t, modifier, gkeTwoResourceLabels, "gke-two-pool-extra", "node_count")
			}

			if tc.expectNoOtherResourceChanges && tc.name == "NonGKEResource" {
				nonGKEResourceLabels := []string{"google_compute_instance", "not_gke"}
				assertAttributeExists(t, modifier, nonGKEResourceLabels, []string{"initial_node_count"})

				resourceBlock, _ := modifier.GetBlock("resource", nonGKEResourceLabels)
				assert.NotNil(t, resourceBlock)
				if resourceBlock != nil {
					npBlockInNonGKE := findSpecificNodePool(t, modifier, nonGKEResourceLabels, "") // Assuming unnamed node pool
					assert.NotNil(t, npBlockInNonGKE, "'node_pool' block was unexpectedly removed from 'google_compute_instance.not_gke'")
					if npBlockInNonGKE != nil {
						assertNodePoolAttributeExists(t, modifier, nonGKEResourceLabels, "", "initial_node_count")
						assertNodePoolAttributeExists(t, modifier, nonGKEResourceLabels, "", "node_count")
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
		expectOtherAttributeUnchanged          *string // Pointer to check for presence vs. nil for not checking
	}

	type masterCIDRTestCase struct {
		name                         string
		hclInput                     string
		expectedModifications        int
		gkeResourceName              string
		expectMasterCIDRPresent      bool
		privateClusterConfigCheck    *privateClusterConfigCheck
		expectNoOtherResourceChanges bool // For non-GKE resource types
		expectNoGKEResource          bool // For empty HCL
	}

	testCases := []masterCIDRTestCase{
		{
			name: "BothPresent",
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", `
  master_ipv4_cidr_block   = "172.16.0.0/28"
`, getPrivateClusterConfig(`
    enable_private_endpoint   = true
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", `
  master_ipv4_cidr_block = "172.16.0.0/28"
`, ""),
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: true,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists: false,
			},
		},
		{
			name: "OnlyMasterCIDRPresent_SubnetworkMissingInPrivateConfig",
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", `
  master_ipv4_cidr_block = "172.16.0.0/28"
`, getPrivateClusterConfig(`
    enable_private_endpoint = true
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getPrivateClusterConfig(`
    enable_private_endpoint   = true
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/my-subnetwork"
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", getPrivateClusterConfig(`
    enable_private_endpoint = true
`)),
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
			hclInput: getBasicGKECluster("gke_cluster", "test-cluster", "", ""),
			expectedModifications:   0,
			gkeResourceName:         "gke_cluster",
			expectMasterCIDRPresent: false,
			privateClusterConfigCheck: &privateClusterConfigCheck{
				expectBlockExists: false,
			},
		},
		{
			name: "NonGKEResource",
			hclInput: `resource "google_compute_instance" "not_gke" {
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
			hclInput:            ``,
			expectedModifications: 0,
			gkeResourceName:       "",
			expectNoGKEResource:   true,
		},
		{
			name: "MultipleGKEResources_OneMatch",
			hclInput: getBasicGKECluster("gke_one_match", "cluster-one", `
  master_ipv4_cidr_block   = "172.16.0.0/28"
`, getPrivateClusterConfig(`
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-one"
    enable_private_endpoint   = true
`)) + "\n" + getBasicGKECluster("gke_two_no_master_cidr", "cluster-two", "", getPrivateClusterConfig(`
    private_endpoint_subnetwork = "projects/my-project/regions/us-central1/subnetworks/sub-two"
`)) + "\n" + getBasicGKECluster("gke_three_no_subnetwork", "cluster-three", `
  master_ipv4_cidr_block   = "172.16.1.0/28"
`, getPrivateClusterConfig(`
    enable_private_endpoint = false
`)),
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for non-empty HCL input")
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.MasterCIDRRuleDefinition})
			assert.Empty(t, errs, "ApplyRules should not return errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			modifiedFile, parseDiags := hclwrite.ParseConfig(modifier.File().Bytes(), "modified.hcl", hcl.InitialPos)
			assert.False(t, parseDiags.HasErrors(), "Failed to parse modified HCL for verification")

			if tc.expectNoGKEResource {
				for _, b := range modifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) > 0 && b.Labels()[0] == "google_container_cluster" {
						t.Errorf("Expected no 'google_container_cluster' resource, but found one: %v", b.Labels())
					}
				}
				return
			}

			resourceLabels := []string{"google_container_cluster", tc.gkeResourceName}
			if tc.gkeResourceName != "" {
				targetGKEResource, err := modifier.GetBlock("resource", resourceLabels)
				if (tc.expectedModifications > 0 || tc.privateClusterConfigCheck != nil || tc.expectMasterCIDRPresent) && err != nil {
					t.Fatalf("Expected GKE resource '%s' not found after rule application: %v", tc.gkeResourceName, err)
				}

				if targetGKEResource != nil {
					if tc.expectMasterCIDRPresent {
						assertAttributeExists(t, modifier, resourceLabels, []string{"master_ipv4_cidr_block"})
					} else {
						if targetGKEResource.Labels()[0] == "google_container_cluster" {
							assertAttributeAbsent(t, modifier, resourceLabels, []string{"master_ipv4_cidr_block"})
						}
					}

					if tc.privateClusterConfigCheck != nil {
						pccPath := []string{"private_cluster_config"}
						if tc.privateClusterConfigCheck.expectBlockExists {
							assertNestedBlockExists(t, modifier, resourceLabels, pccPath)
							if tc.privateClusterConfigCheck.expectPrivateEndpointSubnetworkRemoved {
								assertAttributeAbsent(t, modifier, resourceLabels, append(pccPath, "private_endpoint_subnetwork"))
							} else {
								originalParsed, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
								originalGKEResource, _ := findBlockInParsedFile(originalParsed, "google_container_cluster", tc.gkeResourceName)
								if originalGKEResource != nil {
									originalPCC := originalGKEResource.Body().FirstMatchingBlock("private_cluster_config", nil)
									if originalPCC != nil && originalPCC.Body().GetAttribute("private_endpoint_subnetwork") != nil {
										assertAttributeExists(t, modifier, resourceLabels, append(pccPath, "private_endpoint_subnetwork"))
									}
								}
							}
							if tc.privateClusterConfigCheck.expectOtherAttributeUnchanged != nil {
								assertAttributeExists(t, modifier, resourceLabels, append(pccPath, *tc.privateClusterConfigCheck.expectOtherAttributeUnchanged))
							}
						} else {
							assertNestedBlockAbsent(t, modifier, resourceLabels, pccPath)
						}
					}
				}
			}

			if tc.name == "MultipleGKEResources_OneMatch" {
				gkeTwoLabels := []string{"google_container_cluster", "gke_two_no_master_cidr"}
				assertAttributeAbsent(t, modifier, gkeTwoLabels, []string{"master_ipv4_cidr_block"})
				assertNestedBlockExists(t, modifier, gkeTwoLabels, []string{"private_cluster_config"})
				assertAttributeExists(t, modifier, gkeTwoLabels, []string{"private_cluster_config", "private_endpoint_subnetwork"})

				gkeThreeLabels := []string{"google_container_cluster", "gke_three_no_subnetwork"}
				assertAttributeExists(t, modifier, gkeThreeLabels, []string{"master_ipv4_cidr_block"})
				assertNestedBlockExists(t, modifier, gkeThreeLabels, []string{"private_cluster_config"})
				assertAttributeAbsent(t, modifier, gkeThreeLabels, []string{"private_cluster_config", "private_endpoint_subnetwork"})
				assertAttributeExists(t, modifier, gkeThreeLabels, []string{"private_cluster_config", "enable_private_endpoint"})
			}

			if tc.expectNoOtherResourceChanges && tc.name == "NonGKEResource" {
				nonGkeLabels := []string{"google_compute_instance", "not_gke"}
				assertAttributeExists(t, modifier, nonGkeLabels, []string{"master_ipv4_cidr_block"})
				assertNestedBlockExists(t, modifier, nonGkeLabels, []string{"private_cluster_config"})
				assertAttributeExists(t, modifier, nonGkeLabels, []string{"private_cluster_config", "private_endpoint_subnetwork"})
			}
		})
	}
}

func TestApplyBinaryAuthorizationRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	type binaryAuthorizationTestCase struct {
		name                                  string
		hclInput                              string
		expectedModifications                 int
		resourceLabelsToVerify                []string
		expectEnabledAttributeRemoved         bool
		binaryAuthorizationShouldExist        bool
		binaryAuthorizationShouldHaveEvalMode bool
	}

	testCases := []binaryAuthorizationTestCase{
		{
			name: "Both enabled and evaluation_mode present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getBinaryAuthorizationBlock(`
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
`)),
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Only enabled present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getBinaryAuthorizationBlock(`
    enabled = true
`)),
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "Only evaluation_mode present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getBinaryAuthorizationBlock(`
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
`)),
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name: "Neither enabled nor evaluation_mode present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getBinaryAuthorizationBlock(`
    some_other_attr = "value"
`)),
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block present but empty",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getBinaryAuthorizationBlock("")),
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "binary_authorization block missing entirely",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  location = "us-central1"
`, ""),
			expectedModifications:                 0,
			expectEnabledAttributeRemoved:         false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			binaryAuthorizationShouldExist:        false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
		{
			name: "Non-matching resource type with binary_authorization",
			hclInput: `resource "google_compute_instance" "default" {
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
			hclInput: getBasicGKECluster("gke_one", "gke-one", "", getBinaryAuthorizationBlock(`
    enabled          = true
    evaluation_mode  = "PROJECT_SINGLETON_POLICY_ENFORCE"
`)) + "\n" + getBasicGKECluster("gke_two", "gke-two", "", getBinaryAuthorizationBlock(`
    evaluation_mode = "DISABLED"
`)),
			expectedModifications:                 1,
			expectEnabledAttributeRemoved:         true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "gke_one"},
			binaryAuthorizationShouldExist:        true,
			binaryAuthorizationShouldHaveEvalMode: true,
		},
		{
			name:                                  "Empty HCL content",
			hclInput:                            ``,
			expectedModifications:                 0,
			resourceLabelsToVerify:                nil,
			expectEnabledAttributeRemoved:         false,
			binaryAuthorizationShouldExist:        false,
			binaryAuthorizationShouldHaveEvalMode: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for non-empty HCL input")
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.BinaryAuthorizationRuleDefinition})
			assert.Empty(t, errs, "ApplyRules should not return errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			if tc.resourceLabelsToVerify == nil {
				return
			}

			if tc.binaryAuthorizationShouldExist {
				assertNestedBlockExists(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization"})
				if tc.expectEnabledAttributeRemoved {
					assertAttributeAbsent(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "enabled"})
				} else {
					originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
					originalResource, _ := findBlockInParsedFile(originalFile, tc.resourceLabelsToVerify[0], tc.resourceLabelsToVerify[1])
					if originalResource != nil {
						originalBA := originalResource.Body().FirstMatchingBlock("binary_authorization", nil)
						if originalBA != nil && originalBA.Body().GetAttribute("enabled") != nil {
							assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "enabled"})
						}
					}
				}
				if tc.binaryAuthorizationShouldHaveEvalMode {
					assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "evaluation_mode"})
				} else {
                      originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
                      originalResource, _ := findBlockInParsedFile(originalFile, tc.resourceLabelsToVerify[0], tc.resourceLabelsToVerify[1])
                      var originalHadEvalModeWithEnabled bool
                      if originalResource != nil {
                          originalBA := originalResource.Body().FirstMatchingBlock("binary_authorization", nil)
                          if originalBA != nil && originalBA.Body().GetAttribute("evaluation_mode") != nil && originalBA.Body().GetAttribute("enabled") != nil {
                              originalHadEvalModeWithEnabled = true
                          }
                           if originalBA != nil && originalBA.Body().GetAttribute("evaluation_mode") != nil && originalBA.Body().GetAttribute("enabled") == nil {
                              assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "evaluation_mode"})
                          } else if !originalHadEvalModeWithEnabled && originalBA != nil && originalBA.Body().GetAttribute("evaluation_mode") != nil {
							  assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "evaluation_mode"})
						  } else if originalBA != nil && originalBA.Body().GetAttribute("evaluation_mode") == nil {
							  assertAttributeAbsent(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization", "evaluation_mode"})
						  }
                      }
				}
			} else {
				assertNestedBlockAbsent(t, modifier, tc.resourceLabelsToVerify, []string{"binary_authorization"})
			}

			if tc.name == "Multiple GKE resources, one with conflict" {
				gkeTwoLabels := []string{"google_container_cluster", "gke_two"}
				assertNestedBlockExists(t, modifier, gkeTwoLabels, []string{"binary_authorization"})
				assertAttributeAbsent(t, modifier, gkeTwoLabels, []string{"binary_authorization", "enabled"})
				assertAttributeExists(t, modifier, gkeTwoLabels, []string{"binary_authorization", "evaluation_mode"})
			}
		})
	}
}

func TestApplyServicesIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	type servicesIPV4CIDRTestCase struct {
		name                                  string
		hclInput                              string
		expectedModifications                 int
		resourceLabelsToVerify                []string
		expectServicesIPV4CIDRBlockRemoved    bool
		ipAllocationPolicyShouldExistForCheck bool
	}

	testCases := []servicesIPV4CIDRTestCase{
		{
			name: "Both attributes present in ip_allocation_policy",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
`)),
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Only services_ipv4_cidr_block present in ip_allocation_policy",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block   = "10.2.0.0/20"
`)),
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Only cluster_secondary_range_name present in ip_allocation_policy",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    cluster_secondary_range_name = "services_range"
`)),
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Neither attribute relevant to Rule 2 present in ip_allocation_policy",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    some_other_attribute = "value"
`)),
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is present but empty",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock("")),
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "ip_allocation_policy block is missing entirely",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", ""),
			expectedModifications:                 0,
			expectServicesIPV4CIDRBlockRemoved:    false,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: false,
		},
		{
			name: "Non-matching resource type with similar nested structure",
			hclInput: `resource "google_compute_router" "default" {
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
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block   = "10.2.0.0/20"
    cluster_secondary_range_name = "services_range"
`)) + "\n" + getBasicGKECluster("secondary", "secondary-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block = "10.3.0.0/20"
`)),
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "primary"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name: "Multiple google_container_cluster blocks, ip_policy missing in one",
			hclInput: getBasicGKECluster("alpha", "alpha-cluster", "", "") + "\n" + getBasicGKECluster("beta", "beta-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block   = "10.4.0.0/20"
    cluster_secondary_range_name = "services_range_beta"
`)),
			expectedModifications:                 1,
			expectServicesIPV4CIDRBlockRemoved:    true,
			resourceLabelsToVerify:                []string{"google_container_cluster", "beta"},
			ipAllocationPolicyShouldExistForCheck: true,
		},
		{
			name:                                  "Empty HCL content",
			hclInput:                            ``,
			expectedModifications:                 0,
			resourceLabelsToVerify:                nil,
			expectServicesIPV4CIDRBlockRemoved:    false,
			ipAllocationPolicyShouldExistForCheck: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for non-empty HCL input")
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ServicesIPV4CIDRRuleDefinition})
			assert.Empty(t, errs, "ApplyRules should not return errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			if tc.resourceLabelsToVerify == nil {
				return
			}

			ipAllocPath := []string{"ip_allocation_policy"}
			servicesCIDRPath := append(ipAllocPath, "services_ipv4_cidr_block")

			if tc.ipAllocationPolicyShouldExistForCheck {
				assertNestedBlockExists(t, modifier, tc.resourceLabelsToVerify, ipAllocPath)
				if tc.expectServicesIPV4CIDRBlockRemoved {
					assertAttributeAbsent(t, modifier, tc.resourceLabelsToVerify, servicesCIDRPath)
				} else {
					originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
					originalResource, _ := findBlockInParsedFile(originalFile, tc.resourceLabelsToVerify[0], tc.resourceLabelsToVerify[1])
					if originalResource != nil {
						originalIPAlloc := originalResource.Body().FirstMatchingBlock("ip_allocation_policy", nil)
						if originalIPAlloc != nil && originalIPAlloc.Body().GetAttribute("services_ipv4_cidr_block") != nil {
							assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, servicesCIDRPath)
						}
					}
				}
			} else {
				assertNestedBlockAbsent(t, modifier, tc.resourceLabelsToVerify, ipAllocPath)
			}

			if tc.name == "Multiple google_container_cluster blocks, one matching for Rule 2" {
				secondaryLabels := []string{"google_container_cluster", "secondary"}
				assertNestedBlockExists(t, modifier, secondaryLabels, ipAllocPath)
				assertAttributeExists(t, modifier, secondaryLabels, append(ipAllocPath, "services_ipv4_cidr_block"))
			}
			if tc.name == "Multiple google_container_cluster blocks, ip_policy missing in one" {
				alphaLabels := []string{"google_container_cluster", "alpha"}
				assertNestedBlockAbsent(t, modifier, alphaLabels, ipAllocPath)
			}
		})
	}
}

func TestApplyClusterIPV4CIDRRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	type clusterIPV4CIDRTestCase struct {
		name                         string
		hclInput                     string
		expectedModifications        int
		resourceLabelsToVerify       []string
		expectClusterIPV4CIDRRemoved bool
	}

	testCases := []clusterIPV4CIDRTestCase{
		{
			name: "Both attributes present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, getIpAllocationPolicyBlock(`
    cluster_ipv4_cidr_block = "10.1.0.0/14"
`)),
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only cluster_ipv4_cidr present (no ip_allocation_policy block)",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, ""),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only cluster_ipv4_cidr present (ip_allocation_policy block exists but no cluster_ipv4_cidr_block)",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block = "10.2.0.0/20"
`)),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Only ip_allocation_policy.cluster_ipv4_cidr_block present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    cluster_ipv4_cidr_block = "10.1.0.0/14"
`)),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Neither attribute relevant to Rule 1 present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", "", getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block = "10.2.0.0/20"
`)),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "ip_allocation_policy block is missing entirely, cluster_ipv4_cidr present",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, ""),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Non-matching resource type (google_compute_instance)",
			hclInput: `resource "google_compute_instance" "default" {
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
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, getIpAllocationPolicyBlock(`
    cluster_ipv4_cidr_block = "10.1.0.0/14"
`)) + "\n" + getBasicGKECluster("secondary", "secondary-cluster", `
  cluster_ipv4_cidr  = "10.2.0.0/14"
`, getIpAllocationPolicyBlock(`
    services_ipv4_cidr_block = "10.3.0.0/20"
`)),
			expectedModifications:        1,
			expectClusterIPV4CIDRRemoved: true,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name: "Multiple google_container_cluster blocks, none matching",
			hclInput: getBasicGKECluster("primary", "primary-cluster", `
  cluster_ipv4_cidr  = "10.0.0.0/14"
`, "") + "\n" + getBasicGKECluster("secondary", "secondary-cluster", "", getIpAllocationPolicyBlock(`
    cluster_ipv4_cidr_block = "10.1.0.0/14"
`)),
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       []string{"google_container_cluster", "primary"},
		},
		{
			name:                         "Empty HCL content",
			hclInput:                   ``,
			expectedModifications:        0,
			expectClusterIPV4CIDRRemoved: false,
			resourceLabelsToVerify:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for non-empty HCL input")
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.ClusterIPV4CIDRRuleDefinition})
			assert.Empty(t, errs, "ApplyRules should not return errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			if tc.resourceLabelsToVerify == nil {
				return
			}

			if tc.expectClusterIPV4CIDRRemoved {
				assertAttributeAbsent(t, modifier, tc.resourceLabelsToVerify, []string{"cluster_ipv4_cidr"})
			} else {
				originalFile, _ := hclwrite.ParseConfig([]byte(tc.hclInput), "", hcl.InitialPos)
				originalResource, _ := findBlockInParsedFile(originalFile, tc.resourceLabelsToVerify[0], tc.resourceLabelsToVerify[1])
				if originalResource != nil && originalResource.Body().GetAttribute("cluster_ipv4_cidr") != nil {
					assertAttributeExists(t, modifier, tc.resourceLabelsToVerify, []string{"cluster_ipv4_cidr"})
				} else if originalResource != nil && originalResource.Body().GetAttribute("cluster_ipv4_cidr") == nil {
					assertAttributeAbsent(t, modifier, tc.resourceLabelsToVerify, []string{"cluster_ipv4_cidr"})
				}
			}

			if tc.name == "Multiple google_container_cluster blocks, one matching" {
				secondaryLabels := []string{"google_container_cluster", "secondary"}
				assertAttributeExists(t, modifier, secondaryLabels, []string{"cluster_ipv4_cidr"})
			}

			if tc.name == "Multiple google_container_cluster blocks, none matching" {
				secondaryLabels := []string{"google_container_cluster", "secondary"}
				assertAttributeAbsent(t, modifier, secondaryLabels, []string{"cluster_ipv4_cidr"})
				assertAttributeExists(t, modifier, secondaryLabels, []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"})
			}
		})
	}
}

type autopilotAddonsConfigCheck struct {
	expectBlockExists                bool
	expectNetworkPolicyConfigRemoved bool
	expectDnsCacheConfigRemoved      bool
	expectStatefulHaConfigRemoved    bool
	expectHttpLoadBalancingUnchanged bool
}

type autopilotClusterAutoscalingCheck struct {
	expectBlockExists           bool
	expectEnabledRemoved        bool
	expectResourceLimitsRemoved bool
	expectProfileUnchanged      *string
}

type autopilotBinaryAuthorizationCheck struct {
	expectBlockExists    bool
	expectEnabledRemoved bool
}

type autopilotTestCase struct {
	name                                string
	hclInput                            string
	expectedModifications               int
	clusterName                         string
	expectEnableAutopilotAttr           *bool
	expectedRootAttrsRemoved            []string
	expectedRootAttrsKept               []string
	expectedTopLevelNestedBlocksRemoved []string
	expectedTopLevelNestedBlocksKept    []string
	addonsConfigCheck                   *autopilotAddonsConfigCheck
	clusterAutoscalingCheck             *autopilotClusterAutoscalingCheck
	binaryAuthorizationCheck            *autopilotBinaryAuthorizationCheck
}

func TestApplyAutopilotRule(t *testing.T) {
	t.Helper()
	logger := zap.NewNop()

	testCases := []autopilotTestCase{
		{
			name: "enable_autopilot is true, all conflicting fields present",
			hclInput: getBasicGKECluster("autopilot_cluster", "autopilot-cluster", `
  location                      = "us-central1"
  enable_autopilot              = true
  cluster_ipv4_cidr             = "10.0.0.0/8"
  enable_shielded_nodes         = true
  remove_default_node_pool      = true
  default_max_pods_per_node     = 110
  enable_intranode_visibility   = true
`,
				getNodeConfigBlock(`
    machine_type = "e2-standard-4"
    disk_size_gb = 100
    oauth_scopes = [
      "https://www.googleapis.com/auth/devstorage.read_only",
      "https://www.googleapis.com/auth/logging.write",
      "https://www.googleapis.com/auth/monitoring",
    ]
`) + "\n" +
					getAddonsConfigBlock(
						getNetworkPolicyConfigBlock(`disabled = false`) + "\n" +
							getDnsCacheConfigBlock(`enabled = true`) + "\n" +
							getStatefulHAConfigBlock(`enabled = true`) + "\n" +
							getNestedBlock("http_load_balancing", `disabled = false`),
					) + "\n" +
					getNetworkPolicyFrameworkBlock(`
    provider = "CALICO"
    enabled  = true
`) + "\n" +
					getNodePoolBlock("default-pool", `initial_node_count = 1`) + "\n" +
					getNodePoolBlock("custom-pool", `initial_node_count = 1`) + "\n" +
					getClusterAutoscalingBlock(strings.TrimSpace(`
    enabled = true
    autoscaling_profile = "OPTIMIZE_UTILIZATION"
`) + "\n" +
						getResourceLimitsBlock(`
      resource_type = "cpu"
      minimum = 1
      maximum = 10
`)) + "\n" +
					getBinaryAuthorizationBlock(`
    evaluation_mode = "PROJECT_SINGLETON_POLICY_ENFORCE"
    enabled = true
`),
			),
			expectedModifications:     14,
			clusterName:               "autopilot_cluster",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved: []string{
				"cluster_ipv4_cidr", "enable_shielded_nodes", "remove_default_node_pool",
				"default_max_pods_per_node", "enable_intranode_visibility",
			},
			expectedTopLevelNestedBlocksRemoved: []string{"node_config", "network_policy", "node_pool", "cluster_autoscaling"},
			addonsConfigCheck: &autopilotAddonsConfigCheck{
				expectBlockExists:                true,
				expectNetworkPolicyConfigRemoved: true,
				expectDnsCacheConfigRemoved:      true,
				expectStatefulHaConfigRemoved:    true,
				expectHttpLoadBalancingUnchanged: true,
			},
			binaryAuthorizationCheck: &autopilotBinaryAuthorizationCheck{
				expectBlockExists:    true,
				expectEnabledRemoved: true,
			},
		},
		{
			name: "enable_autopilot is false, conflicting fields present",
			hclInput: getBasicGKECluster("standard_cluster", "standard-cluster", `
  enable_autopilot      = false
  cluster_ipv4_cidr     = "10.0.0.0/8"
  enable_shielded_nodes = true
`,
				getNodePoolBlock("default-pool", "") + "\n" +
					getClusterAutoscalingBlock(`
    enabled = true
    autoscaling_profile = "BALANCED"
`) + "\n" +
					getAddonsConfigBlock(
						getDnsCacheConfigBlock(`enabled = true`) + "\n" +
							getNestedBlock("http_load_balancing", `disabled = false`),
					),
			),
			expectedModifications:     1,
			clusterName:               "standard_cluster",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsKept:     []string{"cluster_ipv4_cidr", "enable_shielded_nodes"},
			expectedTopLevelNestedBlocksKept: []string{"node_pool", "cluster_autoscaling", "addons_config"},
			addonsConfigCheck: &autopilotAddonsConfigCheck{
				expectBlockExists:                true,
				expectDnsCacheConfigRemoved:      false,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscalingCheck: &autopilotClusterAutoscalingCheck{
				expectBlockExists:           true,
				expectEnabledRemoved:        false,
				expectResourceLimitsRemoved: false,
				expectProfileUnchanged:      stringPtr("BALANCED"),
			},
		},
		{
			name: "enable_autopilot not present, conflicting fields present",
			hclInput: getBasicGKECluster("existing_cluster", "existing-cluster", `
  cluster_ipv4_cidr     = "10.0.0.0/8"
  enable_shielded_nodes = true
`,
				getNodePoolBlock("default-pool", "") + "\n" +
					getAddonsConfigBlock(
						getNetworkPolicyConfigBlock(`disabled = false`),
					),
			),
			expectedModifications:     0,
			clusterName:               "existing_cluster",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsKept:     []string{"cluster_ipv4_cidr", "enable_shielded_nodes"},
			expectedTopLevelNestedBlocksKept: []string{"node_pool", "addons_config"},
			addonsConfigCheck: &autopilotAddonsConfigCheck{
				expectBlockExists:                true,
				expectNetworkPolicyConfigRemoved: false,
			},
		},
		{
			name: "enable_autopilot is true, no conflicting fields present",
			hclInput: getBasicGKECluster("clean_autopilot_cluster", "clean-autopilot-cluster", `
  enable_autopilot = true
  location         = "us-central1"
`,
				getAddonsConfigBlock(
					getNestedBlock("http_load_balancing", `disabled = true`),
				) + "\n" +
					getClusterAutoscalingBlock(`autoscaling_profile = "BALANCED"`) + "\n" +
					getBinaryAuthorizationBlock(`evaluation_mode = "DISABLED"`),
			),
			expectedModifications:               1,
			clusterName:                         "clean_autopilot_cluster",
			expectEnableAutopilotAttr:           boolPtr(true),
			expectedTopLevelNestedBlocksRemoved: []string{"cluster_autoscaling"},
			expectedTopLevelNestedBlocksKept:    []string{"addons_config", "binary_authorization"},
			addonsConfigCheck: &autopilotAddonsConfigCheck{
				expectBlockExists:                true,
				expectHttpLoadBalancingUnchanged: true,
			},
			clusterAutoscalingCheck: &autopilotClusterAutoscalingCheck{
				expectBlockExists: false,
			},
			binaryAuthorizationCheck: &autopilotBinaryAuthorizationCheck{
				expectBlockExists:    true,
				expectEnabledRemoved: false,
			},
		},
		{
			name:                  "No google_container_cluster blocks",
			hclInput:              `resource "google_compute_instance" "vm" { name = "my-vm" }`,
			expectedModifications: 0,
			clusterName:           "",
		},
		{
			name:                  "Empty HCL content",
			hclInput:              ``,
			expectedModifications: 0,
			clusterName:           "",
		},
		{
			name: "Autopilot true, only some attributes to remove",
			hclInput: getBasicGKECluster("partial_autopilot", "partial-autopilot", `
  enable_autopilot      = true
  enable_shielded_nodes = true
  default_max_pods_per_node = 110
`, ""),
			expectedModifications:     2,
			clusterName:               "partial_autopilot",
			expectEnableAutopilotAttr: boolPtr(true),
			expectedRootAttrsRemoved:  []string{"enable_shielded_nodes", "default_max_pods_per_node"},
		},
		{
			name: "enable_autopilot is not a boolean",
			hclInput: getBasicGKECluster("non_bool_autopilot", "non-bool-autopilot", `
  enable_autopilot = "not_a_boolean"
  cluster_ipv4_cidr = "10.1.0.0/16"
`, getNodePoolBlock("default", "")),
			expectedModifications:     0,
			clusterName:               "non_bool_autopilot",
			expectEnableAutopilotAttr: nil,
			expectedRootAttrsKept:     []string{"enable_autopilot", "cluster_ipv4_cidr"},
			expectedTopLevelNestedBlocksKept: []string{"node_pool"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			modifier, _ := createModifierFromHCL(t, tc.hclInput, logger)
			if modifier == nil && tc.hclInput == "" && tc.expectedModifications == 0 {
				return
			}
			if modifier == nil {
				t.Fatalf("createModifierFromHCL returned nil for HCL: %s", tc.hclInput)
			}

			autopilotRules := []types.Rule{rules.RuleHandleAutopilotFalse}
			autopilotRules = append(autopilotRules, rules.AutopilotRules...)
			modifications, ruleErrs := modifier.ApplyRules(autopilotRules)
			assert.Empty(t, ruleErrs, "ApplyRules returned errors")
			assert.Equal(t, tc.expectedModifications, modifications)

			if tc.clusterName == "" {
				return
			}

			resourceLabels := []string{"google_container_cluster", tc.clusterName}

			if tc.expectEnableAutopilotAttr == nil {
				if tc.name == "enable_autopilot is not a boolean" {
					assertAttributeExists(t, modifier, resourceLabels, []string{"enable_autopilot"})
				} else if tc.name == "enable_autopilot is false, conflicting fields present" {
					assertAttributeAbsent(t, modifier, resourceLabels, []string{"enable_autopilot"})
				} else if tc.name == "enable_autopilot not present, conflicting fields present" {
					assertAttributeAbsent(t, modifier, resourceLabels, []string{"enable_autopilot"})
				}
			} else {
				assertAttributeValue(t, modifier, resourceLabels, []string{"enable_autopilot"}, cty.BoolVal(*tc.expectEnableAutopilotAttr))
			}

			for _, attr := range tc.expectedRootAttrsRemoved {
				assertAttributeAbsent(t, modifier, resourceLabels, []string{attr})
			}
			for _, attr := range tc.expectedRootAttrsKept {
				assertAttributeExists(t, modifier, resourceLabels, []string{attr})
			}
			for _, blockName := range tc.expectedTopLevelNestedBlocksRemoved {
				if blockName == "node_pool" {
					targetGKEResource, _ := modifier.GetBlock("resource", resourceLabels)
					if targetGKEResource != nil {
						foundNodePools := false
						for _, b := range targetGKEResource.Body().Blocks() {
							if b.Type() == "node_pool" {
								foundNodePools = true
								break
							}
						}
						assert.False(t, foundNodePools, "Expected all 'node_pool' blocks to be removed in test: %s", tc.name)
					}
				} else {
					assertNestedBlockAbsent(t, modifier, resourceLabels, []string{blockName})
				}
			}
			for _, blockName := range tc.expectedTopLevelNestedBlocksKept {
				assertNestedBlockExists(t, modifier, resourceLabels, []string{blockName})
			}

			if tc.addonsConfigCheck != nil {
				path := []string{"addons_config"}
				if tc.addonsConfigCheck.expectBlockExists {
					assertNestedBlockExists(t, modifier, resourceLabels, path)
					if tc.addonsConfigCheck.expectNetworkPolicyConfigRemoved {
						assertNestedBlockAbsent(t, modifier, resourceLabels, append(path, "network_policy_config"))
					} else {
						originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
						if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("addons_config",nil) != nil && originalBlock.Body().FirstMatchingBlock("addons_config",nil).Body().FirstMatchingBlock("network_policy_config",nil) != nil {
							assertNestedBlockExists(t, modifier, resourceLabels, append(path, "network_policy_config"))
						}
					}
					if tc.addonsConfigCheck.expectDnsCacheConfigRemoved {
						assertNestedBlockAbsent(t, modifier, resourceLabels, append(path, "dns_cache_config"))
					} else {
						originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
						if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("addons_config",nil) != nil && originalBlock.Body().FirstMatchingBlock("addons_config",nil).Body().FirstMatchingBlock("dns_cache_config",nil) != nil {
							assertNestedBlockExists(t, modifier, resourceLabels, append(path, "dns_cache_config"))
						}
					}
					if tc.addonsConfigCheck.expectStatefulHaConfigRemoved {
						assertNestedBlockAbsent(t, modifier, resourceLabels, append(path, "stateful_ha_config"))
					}
					if tc.addonsConfigCheck.expectHttpLoadBalancingUnchanged {
						assertNestedBlockExists(t, modifier, resourceLabels, append(path, "http_load_balancing"))
					}
				} else {
					assertNestedBlockAbsent(t, modifier, resourceLabels, path)
				}
			}

			if tc.clusterAutoscalingCheck != nil {
				path := []string{"cluster_autoscaling"}
				if tc.clusterAutoscalingCheck.expectBlockExists {
					assertNestedBlockExists(t, modifier, resourceLabels, path)
					if tc.clusterAutoscalingCheck.expectEnabledRemoved {
						assertAttributeAbsent(t, modifier, resourceLabels, append(path, "enabled"))
					} else {
						originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
						if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("cluster_autoscaling",nil) != nil && originalBlock.Body().FirstMatchingBlock("cluster_autoscaling",nil).Body().GetAttribute("enabled") != nil {
							assertAttributeExists(t, modifier, resourceLabels, append(path, "enabled"))
						}
					}
					if tc.clusterAutoscalingCheck.expectResourceLimitsRemoved {
						assertNestedBlockAbsent(t, modifier, resourceLabels, append(path, "resource_limits"))
					} else {
						originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
						if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("cluster_autoscaling",nil) != nil && originalBlock.Body().FirstMatchingBlock("cluster_autoscaling",nil).Body().FirstMatchingBlock("resource_limits",nil) != nil {
							assertNestedBlockExists(t, modifier, resourceLabels, append(path, "resource_limits"))
						}
					}
					if tc.clusterAutoscalingCheck.expectProfileUnchanged != nil {
						assertAttributeExists(t, modifier, resourceLabels, append(path, "autoscaling_profile"))
					}
				} else {
					assertNestedBlockAbsent(t, modifier, resourceLabels, path)
				}
			}

			if tc.binaryAuthorizationCheck != nil {
				path := []string{"binary_authorization"}
				if tc.binaryAuthorizationCheck.expectBlockExists {
					assertNestedBlockExists(t, modifier, resourceLabels, path)
					if tc.binaryAuthorizationCheck.expectEnabledRemoved {
						assertAttributeAbsent(t, modifier, resourceLabels, append(path, "enabled"))
					} else {
						originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
						if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("binary_authorization",nil) != nil && originalBlock.Body().FirstMatchingBlock("binary_authorization",nil).Body().GetAttribute("enabled") != nil {
							assertAttributeExists(t, modifier, resourceLabels, append(path, "enabled"))
						}
					}
					originalBlock, _:= findBlockInParsedFile(hclwrite.MustParseConfig([]byte(tc.hclInput), "", hcl.InitialPos), resourceLabels[0], resourceLabels[1])
					if originalBlock != nil && originalBlock.Body().FirstMatchingBlock("binary_authorization",nil) != nil && originalBlock.Body().FirstMatchingBlock("binary_authorization",nil).Body().GetAttribute("evaluation_mode") != nil {
						assertAttributeExists(t, modifier, resourceLabels, append(path, "evaluation_mode"))
					}
				} else {
					assertNestedBlockAbsent(t, modifier, resourceLabels, path)
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
			hclContent: getBasicGKECluster("primary", "my-cluster", `
  logging_service  = "logging.googleapis.com/kubernetes"
`, getClusterTelemetryBlock("ENABLED")),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", "", getClusterTelemetryBlock("ENABLED")),
			expectedModifications: 1,
			ruleToApply:           rules.RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed when telemetry is DISABLED",
			hclContent: getBasicGKECluster("primary", "my-cluster", `
  logging_service  = "logging.googleapis.com/kubernetes"
`, getClusterTelemetryBlock("DISABLED")),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", `
  logging_service  = "logging.googleapis.com/kubernetes"
`, getClusterTelemetryBlock("DISABLED")),
			expectedModifications: 0,
			ruleToApply:           rules.RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed when telemetry block is missing",
			hclContent: getBasicGKECluster("primary", "my-cluster", `
  logging_service  = "logging.googleapis.com/kubernetes"
`, ""),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", `
  logging_service  = "logging.googleapis.com/kubernetes"
`, ""),
			expectedModifications: 0,
			ruleToApply:           rules.RuleRemoveLoggingService,
		},
		{
			name: "logging_service should NOT be removed if logging_service attribute is missing",
			hclContent: getBasicGKECluster("primary", "my-cluster", "", getClusterTelemetryBlock("ENABLED")),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", "", getClusterTelemetryBlock("ENABLED")),
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
			hclContent: getBasicGKECluster("primary", "my-cluster", `
  monitoring_service = "monitoring.googleapis.com/kubernetes"
`, getMonitoringConfigBlock(`enable_components = ["SYSTEM_COMPONENTS"]`)),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", "", getMonitoringConfigBlock(`enable_components = ["SYSTEM_COMPONENTS"]`)),
			expectedModifications: 1,
			ruleToApply:           rules.RuleRemoveMonitoringService,
		},
		{
			name: "monitoring_service should NOT be removed when monitoring_config block is missing",
			hclContent: getBasicGKECluster("primary", "my-cluster", `
  monitoring_service = "monitoring.googleapis.com/kubernetes"
`, ""),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", `
  monitoring_service = "monitoring.googleapis.com/kubernetes"
`, ""),
			expectedModifications: 0,
			ruleToApply:           rules.RuleRemoveMonitoringService,
		},
		{
			name: "monitoring_service should NOT be removed if monitoring_service attribute is missing",
			hclContent: getBasicGKECluster("primary", "my-cluster", "", getMonitoringConfigBlock(`enable_components = ["SYSTEM_COMPONENTS"]`)),
			expectedHCLContent: getBasicGKECluster("primary", "my-cluster", "", getMonitoringConfigBlock(`enable_components = ["SYSTEM_COMPONENTS"]`)),
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
	modifier, _ := createModifierFromHCL(t, tc.hclContent, logger)
	// Note: Temp file removal is handled by t.TempDir() used in createModifierFromHCL

	// Handle cases where modifier might be nil (e.g. empty HCL content and NewFromFile fails)
	if modifier == nil {
		if tc.expectedModifications == 0 && strings.TrimSpace(tc.hclContent) == "" {
			// This might be an expected case for empty input leading to no modifications
			return
		}
		t.Fatalf("Modifier is nil, cannot proceed with rule application. HCL Content:\n%s", tc.hclContent)
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
