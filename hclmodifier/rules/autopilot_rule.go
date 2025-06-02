package rules

import (
	"fmt"

	"github.com/GoogleCloudPlatform/hcl-modifier/pkg/hclmodifier"
	"github.com/hashicorp/hcl/v2/hclwrite" // Required for hclwrite.Block type hint if used
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"
)

// AutopilotRuleDefinition is a placeholder for the AutopilotRule.
// The current ApplyRules engine may not fully support its complex logic (conditional attribute value checks, multiple different removals).
// Thus, ApplyAutopilotRule will continue to use its direct implementation.
var AutopilotRuleDefinition = hclmodifier.Rule{
	Name:               "AutopilotRule: Placeholder - Complex logic handled by ApplyAutopilotRule",
	TargetResourceType: "google_container_cluster",
}

// ApplyAutopilotRule implements the logic for applying Autopilot configurations.
// 1. Iterate through all blocks in the HCL file.
// 2. Identify `resource` blocks with type `google_container_cluster`.
// 3. For each such block:
//    a. Check for the `enable_autopilot` attribute.
//    b. If the attribute exists:
//        i. Get its value.
//        ii. If the value is `true`:
//            - Log the action.
//            - Define a list of attributes to remove.
//            - Remove these attributes from the block.
//            - Define a list of nested blocks to remove.
//            - Remove these nested blocks.
//            - Specifically handle `cluster_autoscaling` block.
//            - Specifically handle `binary_authorization` block.
//        iii. If the value is `false`:
//            - Log the action.
//            - Remove the `enable_autopilot` attribute itself.
//    c. If the `enable_autopilot` attribute is not found, log this.
// 4. Return the total count of modifications and any error encountered.
func (m *hclmodifier.Modifier) ApplyAutopilotRule() (modifications int, err error) {
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
	// "resource_limits" is a block within cluster_autoscaling
	clusterAutoscalingSubBlocksToRemoveWhenTrue := []string{"resource_limits"}

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
						if m.File().Body().FirstMatchingBlock("node_pool", npBlock.Labels()) != nil {
							if removed := m.File().Body().RemoveBlock(npBlock); removed {
								modificationCount++
								resLogger.Info("Removed 'node_pool' block instance.", zap.Strings("labels", npBlock.Labels()))
							} else {
								errRemoveNP := fmt.Errorf("failed to remove node_pool block instance (labels: %v)", npBlock.Labels())
								resLogger.Error("Error removing 'node_pool' block instance.", zap.Error(errRemoveNP))
								if firstError == nil {
									firstError = errRemoveNP
								}
							}
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
								if firstError == nil {	firstError = errRemove }
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
								if firstError == nil { firstError = errRemove }
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
								if firstError == nil { firstError = errRemove }
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
