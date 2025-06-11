package hclmodifier

import (
	"fmt"

	"github.com/hashicorp/hcl/v2/hclwrite" // Required for hclwrite.Block type hint if used
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types" // Import for type definitions
)

// AutopilotRuleDefinition is a placeholder for the AutopilotRule because its complex logic,
// involving conditional checks and removal of various attributes and blocks,
// is handled by the direct implementation in ApplyAutopilotRule.
// This definition is not meant to be used by the generic ApplyRules engine.
//
// What it does (effectively via ApplyAutopilotRule): If `enable_autopilot` is true, it removes many attributes
// and blocks that are incompatible or managed by Autopilot (e.g., `node_pool` blocks, `cluster_ipv4_cidr`,
// certain `addons_config` sub-blocks, `cluster_autoscaling.enabled`, etc.). If `enable_autopilot` is explicitly
// set to false or is not a boolean, it removes the `enable_autopilot` attribute itself to clean up potentially
// invalid or confusing states from an import.
//
// Why it's necessary for GKE imports: When importing a standard GKE cluster and then trying to convert it to
// Autopilot by setting `enable_autopilot = true`, or when importing an existing Autopilot cluster, many
// attributes valid for standard clusters become invalid or are managed by Autopilot. Terraform might produce
// errors if these attributes are left in the configuration. This rule (via its direct function) cleans these up.
// If an imported cluster has `enable_autopilot = false` or an invalid value for it, removing the attribute
// helps avoid potential errors if the user intends to manage it as a standard cluster or correct the attribute.
var AutopilotRuleDefinition = types.Rule{ // Use types.Rule
	Name:               "Autopilot Configuration Cleanup Rule (handled by ApplyAutopilotRule)",
	TargetResourceType: "google_container_cluster",
	// Conditions and Actions are omitted as this rule is not processed by the generic engine.
}

// ApplyAutopilotRule cleans a `google_container_cluster` resource configuration based on the `enable_autopilot` attribute.
// If `enable_autopilot = true`, it removes attributes and blocks that are incompatible with Autopilot mode.
// This includes node pools, certain network settings, and specific autoscaling/binary authorization fields.
// If `enable_autopilot = false` or is not a boolean value (which can happen after import if the
// attribute was manually edited or if the source was not a pure Autopilot cluster), this function
// removes the `enable_autopilot` attribute itself to prevent errors, allowing the cluster to be
// treated as a standard cluster or requiring the user to explicitly set `enable_autopilot = true`.
//
// This function is called directly and does not use the generic Rule engine due to its complex conditional logic.
func (m *Modifier) ApplyAutopilotRule() (modifications int, err error) {
	modificationCount := 0
	var firstError error

	if m.File() == nil || m.File().Body() == nil {
		m.Logger.Error("ApplyAutopilotRule: Modifier's file or file body is nil.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	attributesToRemoveWhenTrue := []string{
		"cluster_ipv4_cidr",
		"enable_shielded_nodes",
		"remove_default_node_pool",
		"default_max_pods_per_node",
		"enable_intranode_visibility",
	}
	topLevelNestedBlocksToRemoveWhenTrue := []string{"network_policy"} // node_pool handled separately

	addonsConfigSubBlocksToRemoveWhenTrue := []string{
		"network_policy_config",
		"dns_cache_config",
		"stateful_ha_config",
	}
	clusterAutoscalingAttributesToRemoveWhenTrue := []string{"enabled"}
	// "resource_limits" is a block within cluster_autoscaling and could be repeated 3 times
	clusterAutoscalingSubBlocksToRemoveWhenTrue := []string{"resource_limits", "resource_limits", "resource_limits"}

	binaryAuthorizationAttributesToRemoveWhenTrue := []string{"enabled"}

	for _, block := range m.File().Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			resLogger := m.Logger.With(zap.String("resourceName", resourceName), zap.String("rule", "ApplyAutopilotRule"))
			resLogger.Debug("Checking 'google_container_cluster' resource for Autopilot config.")

			autopilotVal, autopilotAttr, errAttr := m.GetAttributeValueByPath(block.Body(), []string{"enable_autopilot"})
			attributeExists := errAttr == nil && autopilotAttr != nil

			if !attributeExists {
				resLogger.Debug("Attribute 'enable_autopilot' not found. No changes for this resource based on this rule.")
				continue
			}

			isAutopilotTrue := false
			removeEnableAutopilotAttr := false

			if autopilotVal.Type() == cty.Bool {
				if autopilotVal.True() {
					isAutopilotTrue = true
					resLogger.Info("Autopilot enabled. Applying necessary modifications.")
				} else {
					// enable_autopilot = false
					resLogger.Info("Autopilot explicitly disabled. Removing 'enable_autopilot' attribute itself.")
					removeEnableAutopilotAttr = true
				}
			} else {
				// enable_autopilot is not a boolean (e.g., "not_a_boolean")
				resLogger.Warn("'enable_autopilot' attribute is not a boolean value. Removing attribute.", zap.String("valueType", autopilotVal.Type().FriendlyName()))
				removeEnableAutopilotAttr = true
			}

			if removeEnableAutopilotAttr {
				if _, existingAttrCheck, _ := m.GetAttributeValueByPath(block.Body(), []string{"enable_autopilot"}); existingAttrCheck != nil {
					if errRemove := m.RemoveAttributeByPath(block.Body(), []string{"enable_autopilot"}); errRemove != nil {
						resLogger.Error("Error removing 'enable_autopilot' attribute.", zap.Error(errRemove))
						if firstError == nil {
							firstError = errRemove
						}
					} else {
						modificationCount++
						resLogger.Info("Successfully removed 'enable_autopilot' (false) attribute.")
					}
				}
				continue
			}

			if isAutopilotTrue {
				// Remove defined top-level attributes IF THEY EXIST
				for _, attrName := range attributesToRemoveWhenTrue {
					if _, existingAttr, _ := m.GetAttributeValueByPath(block.Body(), []string{attrName}); existingAttr != nil {
						resLogger.Debug("Attempting to remove attribute.", zap.String("attributeName", attrName))
						if errRemove := m.RemoveAttributeByPath(block.Body(), []string{attrName}); errRemove != nil {
							resLogger.Error("Error removing attribute.", zap.String("attributeName", attrName), zap.Error(errRemove))
							if firstError == nil {
								firstError = errRemove
							}
						} else {
							modificationCount++
							resLogger.Info("Removed attribute.", zap.String("attributeName", attrName))
						}
					}
				}

				// Remove defined top-level nested blocks IF THEY EXIST
				for _, blockName := range topLevelNestedBlocksToRemoveWhenTrue {
					if existingBlock, _ := m.GetNestedBlock(block.Body(), []string{blockName}); existingBlock != nil {
						resLogger.Debug("Attempting to remove top-level nested block.", zap.String("blockName", blockName))
						if errRemove := m.RemoveNestedBlockByPath(block.Body(), []string{blockName}); errRemove != nil {
							resLogger.Error("Error removing top-level nested block.", zap.String("blockName", blockName), zap.Error(errRemove))
							if firstError == nil {
								firstError = errRemove
							}
						} else {
							modificationCount++
							resLogger.Info("Removed top-level nested block.", zap.String("blockName", blockName))
						}
					}
				}

				// Remove all "node_pool" blocks specifically
				var nodePoolBlocksToRemove []*hclwrite.Block
				for _, currentNestedBlock := range block.Body().Blocks() {
					if currentNestedBlock.Type() == "node_pool" {
						nodePoolBlocksToRemove = append(nodePoolBlocksToRemove, currentNestedBlock)
					}
				}
				if len(nodePoolBlocksToRemove) > 0 {
					resLogger.Debug("Attempting to remove all 'node_pool' blocks.", zap.Int("count", len(nodePoolBlocksToRemove)))
					for _, npBlock := range nodePoolBlocksToRemove {
						// Check if block still exists before attempting removal, as its Body reference might be stale if parent was modified
						// Corrected: Search within the current cluster block's body (block.Body())
						matchingBlockInParent := false
						for _, b := range block.Body().Blocks() {
							if b == npBlock { // Compare block pointers
								matchingBlockInParent = true
								break
							}
						}
						if matchingBlockInParent {
							if removed := block.Body().RemoveBlock(npBlock); removed { // Corrected: Remove from block.Body()
								modificationCount++
								resLogger.Info("Removed 'node_pool' block instance.", zap.Strings("labels", npBlock.Labels()))
							} else {
								errRemoveNP := fmt.Errorf("failed to remove node_pool block instance (labels: %v)", npBlock.Labels())
								resLogger.Error("Error removing 'node_pool' block instance.", zap.Error(errRemoveNP))
								if firstError == nil {
									firstError = errRemoveNP
								}
							}
						} else {
							resLogger.Debug("Node pool block instance already removed or not found in parent for removal.", zap.Strings("labels", npBlock.Labels()))
						}
					}
				}

				// Handle addons_config sub-blocks
				if addonsConfigBlock, errGetAddons := m.GetNestedBlock(block.Body(), []string{"addons_config"}); errGetAddons == nil && addonsConfigBlock != nil {
					resLogger.Debug("Processing 'addons_config' for sub-block removal.")
					for _, subBlockName := range addonsConfigSubBlocksToRemoveWhenTrue {
						if existingSubBlock, _ := m.GetNestedBlock(addonsConfigBlock.Body(), []string{subBlockName}); existingSubBlock != nil {
							if errRemove := m.RemoveNestedBlockByPath(addonsConfigBlock.Body(), []string{subBlockName}); errRemove != nil {
								resLogger.Error("Error removing sub-block from 'addons_config'.", zap.String("subBlockName", subBlockName), zap.Error(errRemove))
								if firstError == nil {
									firstError = errRemove
								}
							} else {
								modificationCount++
								resLogger.Info("Removed sub-block from 'addons_config'.", zap.String("subBlockName", subBlockName))
							}
						}
					}
				} else if errGetAddons != nil && len(addonsConfigSubBlocksToRemoveWhenTrue) > 0 {
					resLogger.Debug("'addons_config' block not found, skipping removal of its sub-blocks.", zap.Error(errGetAddons))
				}

				// Handle cluster_autoscaling attributes and sub-blocks
				if caBlock, errGetCA := m.GetNestedBlock(block.Body(), []string{"cluster_autoscaling"}); errGetCA == nil && caBlock != nil {
					resLogger.Debug("Processing 'cluster_autoscaling'.")
					for _, attrName := range clusterAutoscalingAttributesToRemoveWhenTrue {
						if _, existingAttr, _ := m.GetAttributeValueByPath(caBlock.Body(), []string{attrName}); existingAttr != nil {
							if errRemove := m.RemoveAttributeByPath(caBlock.Body(), []string{attrName}); errRemove != nil {
								resLogger.Error("Error removing attribute from 'cluster_autoscaling'.", zap.String("attributeName", attrName), zap.Error(errRemove))
								if firstError == nil {
									firstError = errRemove
								}
							} else {
								modificationCount++
								resLogger.Info("Removed attribute from 'cluster_autoscaling'.", zap.String("attributeName", attrName))
							}
						}
					}
					for _, subBlockName := range clusterAutoscalingSubBlocksToRemoveWhenTrue {
						if existingSubBlock, _ := m.GetNestedBlock(caBlock.Body(), []string{subBlockName}); existingSubBlock != nil {
							if errRemove := m.RemoveNestedBlockByPath(caBlock.Body(), []string{subBlockName}); errRemove != nil {
								resLogger.Error("Error removing sub-block from 'cluster_autoscaling'.", zap.String("subBlockName", subBlockName), zap.Error(errRemove))
								if firstError == nil {
									firstError = errRemove
								}
							} else {
								modificationCount++
								resLogger.Info("Removed sub-block from 'cluster_autoscaling'.", zap.String("subBlockName", subBlockName))
							}
						}
					}
				} else if errGetCA != nil && (len(clusterAutoscalingAttributesToRemoveWhenTrue) > 0 || len(clusterAutoscalingSubBlocksToRemoveWhenTrue) > 0) {
					resLogger.Debug("'cluster_autoscaling' block not found.", zap.Error(errGetCA))
				}

				// Handle binary_authorization attributes
				if baBlock, errGetBA := m.GetNestedBlock(block.Body(), []string{"binary_authorization"}); errGetBA == nil && baBlock != nil {
					resLogger.Debug("Processing 'binary_authorization' for attribute removal.")
					for _, attrName := range binaryAuthorizationAttributesToRemoveWhenTrue {
						if _, existingAttr, _ := m.GetAttributeValueByPath(baBlock.Body(), []string{attrName}); existingAttr != nil {
							if errRemove := m.RemoveAttributeByPath(baBlock.Body(), []string{attrName}); errRemove != nil {
								resLogger.Error("Error removing attribute from 'binary_authorization'.", zap.String("attributeName", attrName), zap.Error(errRemove))
								if firstError == nil {
									firstError = errRemove
								}
							} else {
								modificationCount++
								resLogger.Info("Removed attribute from 'binary_authorization'.", zap.String("attributeName", attrName))
							}
						}
					}
				} else if errGetBA != nil && len(binaryAuthorizationAttributesToRemoveWhenTrue) > 0 {
					resLogger.Debug("'binary_authorization' block not found.", zap.Error(errGetBA))
				}
			}
		}
	}

	if firstError != nil {
		m.Logger.Error("ApplyAutopilotRule encountered errors during processing.", zap.Error(firstError))
	}
	return modificationCount, firstError
}
