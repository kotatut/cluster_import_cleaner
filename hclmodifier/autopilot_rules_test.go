package hclmodifier

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

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
