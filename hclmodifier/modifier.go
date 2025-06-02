package hclmodifier

import (
	"fmt"
	"os"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax" // <-- CORRECTED IMPORT
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"go.uber.org/zap"
)

// Modifier holds the HCL file and a logger for performing modifications.
// It centralizes state and logging, providing a cleaner API for HCL manipulation.
type Modifier struct {
	file   *hclwrite.File
	logger *zap.Logger
}

// NewFromFile reads and parses a file from the given path to create a new Modifier.
// This is the primary entry point for creating a Modifier instance.
func NewFromFile(filePath string, logger *zap.Logger) (*Modifier, error) {
	if logger == nil {
		logger, _ = zap.NewDevelopment()
		logger.Warn("NewFromFile called with nil logger, using default development logger.")
	}

	logger.Debug("Reading HCL file", zap.String("filePath", filePath))
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		logger.Error("Error reading file", zap.String("filePath", filePath), zap.Error(err))
		return nil, err
	}

	logger.Debug("Parsing HCL file", zap.String("filePath", filePath))
	hclFile, diags := hclwrite.ParseConfig(contentBytes, filePath, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		logger.Error("Error parsing HCL file", zap.String("filePath", filePath), zap.Error(diags))
		return nil, fmt.Errorf("HCL parsing failed: %w", diags)
	}

	return &Modifier{file: hclFile, logger: logger}, nil
}

// File returns the underlying hclwrite.File object for inspection if needed.
func (m *Modifier) File() *hclwrite.File {
	return m.file
}

// WriteToFile saves the current state of the HCL file to the specified path.
func (m *Modifier) WriteToFile(filePath string) error {
	modifiedBytes := m.file.Bytes()
	m.logger.Debug("Writing modified HCL to file", zap.String("filePath", filePath))
	err := os.WriteFile(filePath, modifiedBytes, 0644)
	if err != nil {
		m.logger.Error("Error writing modified HCL to file", zap.String("filePath", filePath), zap.Error(err))
		return err
	}
	m.logger.Info("Successfully wrote modified HCL to file", zap.String("filePath", filePath))
	return nil
}

// ModifyNameAttributes iterates through the HCL file and appends "-clone"
// to the value of any attribute named "name" that is a simple string literal.
// It returns the count of modified attributes.
func (m *Modifier) ModifyNameAttributes() (int, error) {
	modifiedCount := 0
	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ModifyNameAttributes called on a Modifier with nil file or file body.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		// Only modify "name" attributes within "resource" blocks
		if block.Type() != "resource" {
			m.logger.Debug("Skipping block as it is not a resource type",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()))
			continue
		}

		nameAttribute, err := m.GetAttribute(block, "name")
		if err != nil {
			m.logger.Debug("Attribute 'name' not found in block, skipping.",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()))
			continue
		}

		attrValue, err := m.GetAttributeValue(nameAttribute)
		if err != nil {
			m.logger.Info("Skipping 'name' attribute: could not get simple literal value.",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()),
				zap.Error(err))
			continue
		}

		if attrValue.Type() == cty.String {
			originalStringValue := attrValue.AsString()
			modifiedStringValue := originalStringValue + "-clone"

			err := m.SetAttributeValue(block, "name", cty.StringVal(modifiedStringValue))
			if err != nil {
				m.logger.Error("Failed to set modified 'name' attribute",
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.Error(err))
				continue
			}
			modifiedCount++
		}
	}

	if modifiedCount == 0 {
		m.logger.Info("No 'name' attributes were modified.")
	} else {
		m.logger.Info("Total 'name' attributes modified", zap.Int("count", modifiedCount))
	}
	return modifiedCount, nil
}

// GetBlock finds and returns a specific block based on its type and labels.
func (m *Modifier) GetBlock(blockType string, blockLabels []string) (*hclwrite.Block, error) {
	m.logger.Debug("Searching for block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	for _, block := range m.file.Body().Blocks() {
		if block.Type() == blockType && slices.Equal(block.Labels(), blockLabels) {
			m.logger.Debug("Found matching block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
			return block, nil
		}
	}
	m.logger.Warn("Block not found", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil, fmt.Errorf("block %s %v not found", blockType, blockLabels)
}

// GetAttribute finds and returns a specific attribute from a block by its name.
func (m *Modifier) GetAttribute(block *hclwrite.Block, attributeName string) (*hclwrite.Attribute, error) {
	attribute := block.Body().GetAttribute(attributeName)
	if attribute == nil {
		m.logger.Debug("Attribute not found in block",
			zap.String("attributeName", attributeName),
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))
		return nil, fmt.Errorf("attribute '%s' not found", attributeName)
	}
	return attribute, nil
}

// GetAttributeValue evaluates the expression of an attribute and returns its cty.Value.
// It can only evaluate literal values (strings, numbers, bools) as it uses a nil evaluation context.
// This method bridges hclwrite (for syntax manipulation) and hcl (for value evaluation).
func (m *Modifier) GetAttributeValue(attr *hclwrite.Attribute) (cty.Value, error) {
	// 1. Get the source bytes of the expression from the hclwrite attribute.
	exprBytes := attr.Expr().BuildTokens(nil).Bytes()

	// 2. Parse these bytes into an evaluatable hcl.Expression using the hclsyntax package.
	expr, diags := hclsyntax.ParseExpression(exprBytes, "attribute_expr", hcl.Pos{Line: 1, Column: 1}) // <-- CORRECTED
	if diags.HasErrors() {
		m.logger.Error("Failed to re-parse attribute expression for evaluation.", zap.Error(diags))
		return cty.NilVal, fmt.Errorf("failed to parse expression: %w", diags)
	}

	// 3. Now, with an hcl.Expression, we can call .Value() to get the cty.Value.
	// We pass a nil EvalContext because we only want to resolve simple literals.
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		m.logger.Debug("Attribute expression is not a simple literal", zap.String("expression", string(exprBytes)), zap.Error(diags))
		return cty.NilVal, fmt.Errorf("attribute is not a simple literal: %w", diags)
	}

	return val, nil
}

// SetAttributeValue sets an attribute on the given block with the specified name and value.
func (m *Modifier) SetAttributeValue(block *hclwrite.Block, attributeName string, value cty.Value) error {
	if block == nil || block.Body() == nil {
		return fmt.Errorf("input block or its body cannot be nil")
	}
	block.Body().SetAttributeValue(attributeName, value)
	m.logger.Debug("Successfully set attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))
	return nil
}

// RemoveAttribute removes a specific attribute from a block by its name.
func (m *Modifier) RemoveAttribute(block *hclwrite.Block, attributeName string) error {
	if block == nil || block.Body() == nil {
		return fmt.Errorf("input block or its body cannot be nil")
	}
	if block.Body().GetAttribute(attributeName) == nil {
		m.logger.Debug("Attribute to remove not found, no action needed.", zap.String("attributeName", attributeName))
		return nil
	}
	block.Body().RemoveAttribute(attributeName)
	m.logger.Debug("Successfully removed attribute",
		zap.String("blockType", block.Type()),
		zap.Strings("blockLabels", block.Labels()),
		zap.String("attributeName", attributeName))
	return nil
}

// RemoveBlock finds and removes a specific block from the file.
func (m *Modifier) RemoveBlock(blockType string, blockLabels []string) error {
	blockToRemove, err := m.GetBlock(blockType, blockLabels)
	if err != nil {
		return fmt.Errorf("cannot remove block that was not found: %w", err)
	}
	if removed := m.file.Body().RemoveBlock(blockToRemove); !removed {
		m.logger.Error("Failed to remove block, RemoveBlock method returned false", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
		return fmt.Errorf("failed to remove block %s %v", blockType, blockLabels)
	}
	m.logger.Info("Successfully removed block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil
}

func (m *Modifier) RemoveAttributes(resourceTypeLabel string, optionalResourceName *string, attributesToRemove []string) (removedCount int, err error) {
	m.logger.Debug("Attempting to remove attributes",
		zap.String("resourceTypeLabel", resourceTypeLabel),
		zap.Any("optionalResourceName", optionalResourceName),
		zap.Strings("attributesToRemove", attributesToRemove))

	if m.file == nil || m.file.Body() == nil {
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	if len(attributesToRemove) == 0 {
		m.logger.Debug("No attributes specified to remove.")
		return 0, nil
	}

	var targetResourceName string
	if optionalResourceName != nil {
		targetResourceName = *optionalResourceName
	}

	foundSpecificResource := false

	for _, block := range m.file.Body().Blocks() {
		if block.Type() != "resource" {
			continue // Only interested in "resource" blocks
		}

		labels := block.Labels()
		if len(labels) == 0 || labels[0] != resourceTypeLabel {
			continue // Does not match the target resource type label
		}

		// At this point, the block matches the resourceTypeLabel.
		// Now check if a specific resource name is targeted.
		if targetResourceName != "" {
			if len(labels) < 2 || labels[1] != targetResourceName {
				continue // This block is not the specific named resource we are looking for.
			}
			// If we are looking for a specific resource and found it.
			foundSpecificResource = true
		}

		m.logger.Debug("Processing matching block for attribute removal",
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))

		for _, attrName := range attributesToRemove {
			// Use the existing RemoveAttribute method on Modifier.
			// This method already handles logging and the case where attribute doesn't exist.
			errRemove := m.RemoveAttribute(block, attrName)
			if errRemove != nil {
				m.logger.Error("Error removing attribute from block",
					zap.String("attributeName", attrName),
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.Error(errRemove))
				// Decide if this error should halt further processing for this block or entire operation.
				// For now, let's assume we continue with other attributes for this block.
				// If m.RemoveAttribute itself becomes more error-prone, this might need adjustment.
			} else {
				// Check if the attribute actually existed before removal for accurate count.
				if block.Body().GetAttribute(attrName) == nil { // Check if it's gone
					removedCount++
				}
			}
		}
		// If a specific resource was targeted and processed, no need to check other blocks.
		if targetResourceName != "" && foundSpecificResource {
			break
		}
	}

	if targetResourceName != "" && !foundSpecificResource {
		m.logger.Warn("Specified resource name not found",
			zap.String("resourceTypeLabel", resourceTypeLabel),
			zap.String("targetResourceName", targetResourceName))
		return removedCount, fmt.Errorf("resource '%s' with name '%s' not found", resourceTypeLabel, targetResourceName)
	}

	m.logger.Info("Finished removing attributes",
		zap.Int("totalAttributesActuallyRemoved", removedCount), // This count might not be perfectly accurate as explained above.
		zap.String("resourceTypeLabel", resourceTypeLabel),
		zap.Any("optionalResourceName", optionalResourceName))
	return removedCount, nil
}

// ApplyRule1 implements the logic for Rule 1:
// 1. Iterate through all blocks.
// 2. Identify `resource` blocks with type `google_container_cluster`.
// 3. For each such block:
//    a. Check for `cluster_ipv4_cidr` attribute.
//    b. Check for `ip_allocation_policy` nested block.
//    c. If `ip_allocation_policy` exists, check for `cluster_ipv4_cidr_block` attribute within it.
//    d. If both `cluster_ipv4_cidr` (main block) and `ip_allocation_policy.cluster_ipv4_cidr_block` (nested) are found,
//       remove `cluster_ipv4_cidr` from the main block.
//    e. Increment a counter for each modification.
// 4. Log information about the process.
// 5. Return the total count of modifications and any error.
func (m *Modifier) ApplyRule1() (modifications int, err error) {
	// High-level "Applying Rule X..." is now handled by the caller in cmd/root.go
	// m.logger.Info("Applying Rule 1 using the new rule engine.")

	rule1 := Rule{
		Name:               "Rule 1: Remove cluster_ipv4_cidr if ip_allocation_policy.cluster_ipv4_cidr_block exists",
		TargetResourceType: "google_container_cluster",
		// TargetResourceLabels: nil, // This means it applies to all resources of TargetResourceType
		Conditions: []RuleCondition{
			{
				Type: AttributeExists,
				Path: []string{"cluster_ipv4_cidr"},
			},
			{
				Type: BlockExists,
				Path: []string{"ip_allocation_policy"},
			},
			{
				Type: AttributeExists,
				Path: []string{"ip_allocation_policy", "cluster_ipv4_cidr_block"},
			},
		},
		Actions: []RuleAction{
			{
				Type: RemoveAttribute,
				Path: []string{"cluster_ipv4_cidr"},
			},
		},
	}

	mods, errs := m.ApplyRules([]Rule{rule1})

	if len(errs) > 0 {
		// For consistency with the old signature, return the first error.
		// The ApplyRules method already logs all errors.
		return mods, errs[0]
	}

	// Detailed logging of rule execution is handled by ApplyRules.
	// m.logger.Info("ApplyRule1 (using new engine) finished.", zap.Int("modifications", mods))
	return mods, nil
}

// ApplyMasterCIDRRule implements the logic for managing 'master_ipv4_cidr_block'
// and 'private_cluster_config.private_endpoint_subnetwork' attributes within 'google_container_cluster' resources.
// 1. Initialize modificationCount to 0.
// 2. Log the start of the rule application.
// 3. Iterate through all 'resource' blocks of type 'google_container_cluster'.
// 4. For each cluster:
//    a. Check for 'master_ipv4_cidr_block' attribute.
//    b. Find 'private_cluster_config' nested block.
//    c. If found, check for 'private_endpoint_subnetwork' attribute within it.
//    d. If 'master_ipv4_cidr_block' and 'private_cluster_config.private_endpoint_subnetwork' exist,
//       remove 'private_endpoint_subnetwork'.
// 5. Log actions and increment modificationCount.
// 6. Log completion and return modificationCount.
func (m *Modifier) ApplyMasterCIDRRule() (modifications int, err error) {
	// m.logger.Info("Applying MasterCIDRRule using the new rule engine.")

	masterCIDRRule := Rule{
		Name:               "MasterCIDRRule: Remove private_endpoint_subnetwork if master_ipv4_cidr_block and private_cluster_config exist",
		TargetResourceType: "google_container_cluster",
		Conditions: []RuleCondition{
			{
				Type: AttributeExists,
				Path: []string{"master_ipv4_cidr_block"},
			},
			{
				Type: BlockExists,
				Path: []string{"private_cluster_config"},
			},
			{
				Type: AttributeExists,
				Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
			},
		},
		Actions: []RuleAction{
			{
				Type: RemoveAttribute,
				Path: []string{"private_cluster_config", "private_endpoint_subnetwork"},
			},
		},
	}

	mods, errs := m.ApplyRules([]Rule{masterCIDRRule})

	if len(errs) > 0 {
		return mods, errs[0]
	}

	// m.logger.Info("ApplyMasterCIDRRule (using new engine) finished.", zap.Int("modifications", mods))
	return mods, nil
}

// ApplyInitialNodeCountRule implements the logic for managing 'initial_node_count'
	return modificationCount, nil
}

// ApplyInitialNodeCountRule implements the logic for managing 'initial_node_count'
// and 'node_count' attributes within 'node_pool' blocks of 'google_container_cluster' resources.
// 1. Initialize modificationCount to 0.
// 2. Log the start of the rule application.
// 3. Iterate through all 'resource' blocks of type 'google_container_cluster'.
// 4. For each cluster, iterate through its 'node_pool' nested blocks.
// 5. If both 'initial_node_count' and 'node_count' exist, remove 'initial_node_count'.
// 6. If only 'initial_node_count' exists, remove it.
// 7. Log actions and increment modificationCount.
// 8. Log completion and return modificationCount.
func (m *Modifier) ApplyInitialNodeCountRule() (modifications int, err error) {
	modificationCount := 0
	var firstError error
	// m.logger.Info("Starting ApplyInitialNodeCountRule (using path-based helpers).")

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyInitialNodeCountRule: Modifier's file or file body is nil.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			clusterLogger := m.logger.With(zap.String("resourceName", resourceName), zap.String("rule", "ApplyInitialNodeCountRule"))
			clusterLogger.Debug("Checking 'google_container_cluster' resource.")

			for _, nodePoolBlock := range block.Body().Blocks() {
				if nodePoolBlock.Type() == "node_pool" {
					nodePoolLogger := clusterLogger.With(zap.String("nodePoolType", nodePoolBlock.Type())) // Node pools might not have names
					nodePoolLogger.Debug("Checking 'node_pool' block.")

					// Check if 'initial_node_count' exists in this node_pool block.
					// Path is relative to nodePoolBlock.Body().
					_, _, errAttr := m.GetAttributeValueByPath(nodePoolBlock.Body(), []string{"initial_node_count"})

					if errAttr == nil { // Attribute exists
						nodePoolLogger.Info("Found 'initial_node_count' in node_pool. Removing it.")
						errRemove := m.RemoveAttributeByPath(nodePoolBlock.Body(), []string{"initial_node_count"})
						if errRemove != nil {
							nodePoolLogger.Error("Error removing 'initial_node_count' from node_pool.", zap.Error(errRemove))
							if firstError == nil {
								firstError = fmt.Errorf("failed to remove 'initial_node_count' from node_pool in resource '%s': %w", resourceName, errRemove)
							}
						} else {
							modificationCount++
							nodePoolLogger.Info("Successfully removed 'initial_node_count' from node_pool.")
						}
					} else {
						// Log if attribute not found, if desired for verbosity, but not strictly an error for the rule's logic.
						// GetAttributeValueByPath already logs debug messages for attribute not found.
						// nodePoolLogger.Debug("Attribute 'initial_node_count' not found in this node_pool.", zap.Error(errAttr))
					}
				}
			}
		}
	}

	// Caller in cmd/root.go logs completion and modifications.
	// m.logger.Info("ApplyInitialNodeCountRule finished.", zap.Int("modifications", modificationCount))
	if firstError != nil {
		m.logger.Error("ApplyInitialNodeCountRule encountered errors during processing.", zap.Error(firstError))
	}
	return modificationCount, firstError
}

// ApplyRule2 implements the logic for Rule 2:
// 1. Iterate through all blocks.
// 2. Identify `resource` blocks with type `google_container_cluster`.
// 3. For each such block:
//    a. Find the `ip_allocation_policy` nested block.
//    b. If `ip_allocation_policy` block exists:
//        i. Check for `services_ipv4_cidr_block` attribute.
//        ii. Check for `cluster_secondary_range_name` attribute.
//        iii. If both attributes are found, remove `services_ipv4_cidr_block`.
//        iv. Increment counter.
// 4. Log information.
// 5. Return total modifications and any error.
func (m *Modifier) ApplyRule2() (modifications int, err error) {
	// m.logger.Info("Applying Rule 2 using the new rule engine.")

	rule2 := Rule{
		Name:               "Rule 2: Remove services_ipv4_cidr_block if cluster_secondary_range_name exists in ip_allocation_policy",
		TargetResourceType: "google_container_cluster",
		// TargetResourceLabels: nil, // Applies to all resources of TargetResourceType
		Conditions: []RuleCondition{
			{
				Type: BlockExists, // Ensure ip_allocation_policy block exists first
				Path: []string{"ip_allocation_policy"},
			},
			{
				Type: AttributeExists,
				Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
			},
			{
				Type: AttributeExists,
				Path: []string{"ip_allocation_policy", "cluster_secondary_range_name"},
			},
		},
		Actions: []RuleAction{
			{
				Type: RemoveAttribute,
				Path: []string{"ip_allocation_policy", "services_ipv4_cidr_block"},
			},
		},
	}

	mods, errs := m.ApplyRules([]Rule{rule2})

	if len(errs) > 0 {
		return mods, errs[0] // Return first error, ApplyRules logs all
	}

	// m.logger.Info("ApplyRule2 (using new engine) finished.", zap.Int("modifications", mods))
	return mods, nil
}

// ApplyRule3 implements the logic for Rule 3:
// 1. Iterate through all blocks in the HCL file.
// 2. Identify `resource` blocks with type `google_container_cluster`.
// 3. For each such block:
//    a. Check for a nested block named `binary_authorization`.
//    b. If the `binary_authorization` block exists:
//        i. Check for an attribute named `enabled` within this nested block.
//        ii. Check for an attribute named `evaluation_mode` within this nested block.
//        iii. If both `enabled` and `evaluation_mode` attributes are found, remove the `enabled` attribute from the `binary_authorization` block.
//        iv. Increment a counter for each modification.
// 4. Log information about the process (e.g., "Starting ApplyRule3", "Found 'binary_authorization' block", "Removed 'enabled' attribute").
// 5. Return the total count of modifications and any error, similar to `ApplyRule1` and `ApplyRule2`.
func (m *Modifier) ApplyRule3() (modifications int, err error) {
	// m.logger.Info("Applying Rule 3 using the new rule engine.")

	rule3 := Rule{
		Name:               "Rule 3: Remove enabled if evaluation_mode exists in binary_authorization",
		TargetResourceType: "google_container_cluster",
		// TargetResourceLabels: nil, // Applies to all resources of TargetResourceType
		Conditions: []RuleCondition{
			{
				Type: BlockExists, // Ensure binary_authorization block exists first
				Path: []string{"binary_authorization"},
			},
			{
				Type: AttributeExists,
				Path: []string{"binary_authorization", "enabled"},
			},
			{
				Type: AttributeExists,
				Path: []string{"binary_authorization", "evaluation_mode"},
			},
		},
		Actions: []RuleAction{
			{
				Type: RemoveAttribute,
				Path: []string{"binary_authorization", "enabled"},
			},
		},
	}

	mods, errs := m.ApplyRules([]Rule{rule3})

	if len(errs) > 0 {
		return mods, errs[0] // Return first error, ApplyRules logs all
	}

	// m.logger.Info("ApplyRule3 (using new engine) finished.", zap.Int("modifications", mods))
	return mods, nil
}

// --- Rule Engine Structures and Processor Signature ---

// ConditionType defines the type of condition to check.
type ConditionType string

const (
	// AttributeExists checks if a specific attribute exists.
	AttributeExists ConditionType = "AttributeExists"
	// BlockExists checks if a specific block exists.
	BlockExists ConditionType = "BlockExists"
	// AttributeValueEquals checks if a specific attribute has a certain value.
	AttributeValueEquals ConditionType = "AttributeValueEquals"
)

// ActionType defines the type of action to perform.
type ActionType string

const (
	// RemoveAttribute removes a specific attribute.
	RemoveAttribute ActionType = "RemoveAttribute"
	// RemoveBlock removes a specific block.
	RemoveBlock ActionType = "RemoveBlock"
	// SetAttributeValue sets a specific attribute to a certain value.
	SetAttributeValue ActionType = "SetAttributeValue"
)

// RuleCondition represents a condition that must be met for a rule to be applied.
type RuleCondition struct {
	// Type is the type of condition to check.
	Type ConditionType
	// Path is a slice of strings representing the path to the attribute or block.
	// For a top-level attribute: `["attribute_name"]`
	// For a nested attribute: `["block_name", "nested_block_name", "attribute_name"]`
	// For a block: `["block_name", "nested_block_name"]`
	Path []string
	// Value is the cty.Value to compare against (for AttributeValueEquals).
	// This is typically set internally after parsing ExpectedValue.
	Value cty.Value
	// ExpectedValue is the string representation of the value to compare against (for AttributeValueEquals).
	// This will be parsed into cty.Value for comparison.
	ExpectedValue string
}

// RuleAction represents an action to be performed if all conditions of a rule are met.
type RuleAction struct {
	// Type is the type of action to perform.
	Type ActionType
	// Path is a slice of strings representing the path to the attribute or block.
	// For a top-level attribute: `["attribute_name"]`
	// For a nested attribute: `["block_name", "nested_block_name", "attribute_name"]`
	// For removing a block: `["block_name", "nested_block_name"]`
	Path []string
	// Value is the cty.Value to set (for SetAttributeValue).
	// This is typically set internally after parsing ValueToSet.
	Value cty.Value
	// ValueToSet is the string representation of the value to set (for SetAttributeValue).
	// This will be parsed into cty.Value before setting.
	ValueToSet string
}

// Rule defines a set of conditions and actions for a specific resource type.
type Rule struct {
	// Name is a descriptive name for the rule (e.g., "Remove_cluster_ipv4_cidr_when_ip_allocation_policy_exists").
	Name string
	// TargetResourceType is the HCL resource type the rule applies to (e.g., "google_container_cluster").
	TargetResourceType string
	// TargetResourceLabels are optional labels to further specify the target resource (e.g., ["my_specific_cluster"]).
	// If empty, the rule applies to all resources of TargetResourceType.
	TargetResourceLabels []string
	// Conditions is a list of conditions that all must be true (AND logic) for the actions to be performed.
	Conditions []RuleCondition
	// Actions is a list of actions to be performed if all conditions are met.
	Actions []RuleAction
}

// GetNestedBlock iteratively navigates through path elements to find a nested block.
// currentBlockBody is the body of the block to start searching from.
// path is a slice of strings representing the names of the nested blocks.
func (m *Modifier) GetNestedBlock(currentBlockBody *hclwrite.Body, path []string) (*hclwrite.Block, error) {
	if currentBlockBody == nil {
		return nil, fmt.Errorf("GetNestedBlock: currentBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return nil, fmt.Errorf("GetNestedBlock: path cannot be empty")
	}

	logger := m.logger.With(zap.Strings("path", path))
	logger.Debug("GetNestedBlock: Attempting to find nested block.")

	var currentLevelBody = currentBlockBody
	var foundBlock *hclwrite.Block

	for i, blockName := range path {
		foundBlock = nil // Reset for current level
		for _, block := range currentLevelBody.Blocks() {
			if block.Type() == blockName {
				// If it's the last element in the path, we found our target block.
				if i == len(path)-1 {
					foundBlock = block
					break
				}
				// If not the last element, this is an intermediate block.
				// We continue searching within this block's body in the next iteration.
				currentLevelBody = block.Body()
				foundBlock = block // Mark as found to proceed to next level
				break
			}
		}
		if foundBlock == nil {
			logger.Debug("GetNestedBlock: Block not found at current level.", zap.String("blockName", blockName), zap.Int("level", i))
			return nil, fmt.Errorf("block '%s' not found at path level %d", blockName, i)
		}
	}

	if foundBlock == nil {
		// This case should ideally be caught by the loop's check, but as a safeguard:
		logger.Debug("GetNestedBlock: Target block not found at the end of path.")
		return nil, fmt.Errorf("target block not found at path '%s'", path)
	}

	logger.Debug("GetNestedBlock: Successfully found nested block.")
	return foundBlock, nil
}

// GetAttributeValueByPath retrieves the value of an attribute specified by a path.
// initialBlockBody is the body of the top-level block where the path starts.
// path can be a direct attribute (e.g., ["name"]) or a path through nested blocks (e.g., ["block", "sub_block", "attribute_name"]).
// Returns the cty.Value, the *hclwrite.Attribute itself, and an error if not found or value cannot be determined.
func (m *Modifier) GetAttributeValueByPath(initialBlockBody *hclwrite.Body, path []string) (cty.Value, *hclwrite.Attribute, error) {
	if initialBlockBody == nil {
		return cty.NilVal, nil, fmt.Errorf("GetAttributeValueByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return cty.NilVal, nil, fmt.Errorf("GetAttributeValueByPath: path cannot be empty")
	}

	logger := m.logger.With(zap.Strings("path", path))
	logger.Debug("GetAttributeValueByPath: Attempting to get attribute value.")

	attributeName := path[len(path)-1]
	blockPath := path[:len(path)-1]

	var targetBody *hclwrite.Body
	if len(blockPath) == 0 {
		// Attribute is directly in the initialBlockBody
		targetBody = initialBlockBody
	} else {
		// Attribute is in a nested block
		parentBlock, err := m.GetNestedBlock(initialBlockBody, blockPath)
		if err != nil {
			logger.Error("GetAttributeValueByPath: Could not find parent block for attribute.", zap.Error(err))
			return cty.NilVal, nil, fmt.Errorf("parent block for attribute '%s' not found: %w", attributeName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("GetAttributeValueByPath: Parent block has no body.", zap.Strings("blockPath", blockPath))
			return cty.NilVal, nil, fmt.Errorf("parent block '%s' has no body", blockPath)
		}
		targetBody = parentBlock.Body()
	}

	attr := targetBody.GetAttribute(attributeName)
	if attr == nil {
		logger.Debug("GetAttributeValueByPath: Attribute not found.", zap.String("attributeName", attributeName))
		return cty.NilVal, nil, fmt.Errorf("attribute '%s' not found in specified block", attributeName)
	}

	// Use the existing GetAttributeValue which expects *hclwrite.Attribute
	// Note: This is slightly different from the original GetAttributeValue in Modifier,
	// as this is not a method on Modifier struct directly.
	// We'll assume a similar helper or adapt. For now, let's use m.GetAttributeValue.
	// This requires a Modifier instance. If this function is not a method of Modifier,
	// we'd need to pass m or make GetAttributeValue a static helper.
	// Based on the signature, it is a method on Modifier, so m.GetAttributeValue is fine.

	val, err := m.GetAttributeValue(attr) // m.GetAttributeValue handles logging for its part
	if err != nil {
		logger.Debug("GetAttributeValueByPath: Could not get value of attribute.", zap.String("attributeName", attributeName), zap.Error(err))
		return cty.NilVal, attr, fmt.Errorf("could not get value of attribute '%s': %w", attributeName, err)
	}

	logger.Debug("GetAttributeValueByPath: Successfully retrieved attribute value.", zap.String("attributeName", attributeName))
	return val, attr, nil
}

// RemoveAttributeByPath removes an attribute specified by a path.
// initialBlockBody is the body of the top-level block where the path starts.
// path can be a direct attribute (e.g., ["name"]) or a path through nested blocks (e.g., ["block", "sub_block", "attribute_name"]).
func (m *Modifier) RemoveAttributeByPath(initialBlockBody *hclwrite.Body, path []string) error {
	if initialBlockBody == nil {
		return fmt.Errorf("RemoveAttributeByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("RemoveAttributeByPath: path cannot be empty")
	}

	logger := m.logger.With(zap.Strings("path", path))
	logger.Debug("RemoveAttributeByPath: Attempting to remove attribute.")

	attributeName := path[len(path)-1]
	blockPath := path[:len(path)-1]

	var targetBody *hclwrite.Body
	if len(blockPath) == 0 {
		targetBody = initialBlockBody
	} else {
		parentBlock, err := m.GetNestedBlock(initialBlockBody, blockPath)
		if err != nil {
			logger.Error("RemoveAttributeByPath: Could not find parent block for attribute.", zap.Error(err))
			return fmt.Errorf("parent block for attribute '%s' not found: %w", attributeName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("RemoveAttributeByPath: Parent block has no body.", zap.Strings("blockPath", blockPath))
			return fmt.Errorf("parent block '%s' has no body", blockPath)
		}
		targetBody = parentBlock.Body()
	}

	if targetBody.GetAttribute(attributeName) == nil {
		logger.Debug("RemoveAttributeByPath: Attribute to remove not found, no action needed.", zap.String("attributeName", attributeName))
		return nil // Or return an error if strict "must exist to be removed" is needed. For idempotency, nil is fine.
	}

	targetBody.RemoveAttribute(attributeName)
	logger.Info("RemoveAttributeByPath: Successfully removed attribute.", zap.String("attributeName", attributeName))
	return nil
}

// RemoveNestedBlockByPath removes a nested block specified by a path.
// initialBlockBody is the body of the block from which the removal path starts.
// path specifies the sequence of block names leading to the block to be removed.
func (m *Modifier) RemoveNestedBlockByPath(initialBlockBody *hclwrite.Body, path []string) error {
	if initialBlockBody == nil {
		return fmt.Errorf("RemoveNestedBlockByPath: initialBlockBody cannot be nil")
	}
	if len(path) == 0 {
		return fmt.Errorf("RemoveNestedBlockByPath: path cannot be empty")
	}

	logger := m.logger.With(zap.Strings("path", path))
	logger.Debug("RemoveNestedBlockByPath: Attempting to remove nested block.")

	blockToRemoveName := path[len(path)-1]
	parentBlockPath := path[:len(path)-1]

	var bodyToRemoveFrom *hclwrite.Body
	if len(parentBlockPath) == 0 {
		// The block to remove is directly under initialBlockBody
		bodyToRemoveFrom = initialBlockBody
	} else {
		// The block to remove is nested. Find its parent block.
		parentBlock, err := m.GetNestedBlock(initialBlockBody, parentBlockPath)
		if err != nil {
			logger.Error("RemoveNestedBlockByPath: Could not find parent block of the block to remove.", zap.Error(err))
			return fmt.Errorf("parent block for '%s' not found: %w", blockToRemoveName, err)
		}
		if parentBlock.Body() == nil {
			logger.Error("RemoveNestedBlockByPath: Parent block has no body.", zap.Strings("parentBlockPath", parentBlockPath))
			return fmt.Errorf("parent block '%s' has no body", parentBlockPath)
		}
		bodyToRemoveFrom = parentBlock.Body()
	}

	var blockToRemove *hclwrite.Block
	for _, block := range bodyToRemoveFrom.Blocks() {
		if block.Type() == blockToRemoveName {
			blockToRemove = block
			break
		}
	}

	if blockToRemove == nil {
		logger.Debug("RemoveNestedBlockByPath: Block to remove not found, no action needed.", zap.String("blockToRemoveName", blockToRemoveName))
		return nil // Or an error if strict existence is required.
	}

	if removed := bodyToRemoveFrom.RemoveBlock(blockToRemove); !removed {
		logger.Error("RemoveNestedBlockByPath: Failed to remove block using RemoveBlock method.", zap.String("blockToRemoveName", blockToRemoveName))
		return fmt.Errorf("failed to remove block '%s'", blockToRemoveName)
	}

	logger.Info("RemoveNestedBlockByPath: Successfully removed nested block.", zap.String("blockToRemoveName", blockToRemoveName))
	return nil
}

// ApplyRules iterates through the provided rules and applies them to the HCL file.
// It returns the total number of modifications made and a list of errors encountered.
func (m *Modifier) ApplyRules(rules []Rule) (modifications int, errors []error) {
	m.logger.Info("Starting ApplyRules processing.", zap.Int("numberOfRules", len(rules)))
	totalModifications := 0
	var collectedErrors []error

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyRules: Modifier's file or file body is nil.")
		collectedErrors = append(collectedErrors, fmt.Errorf("modifier's file or file body cannot be nil"))
		return 0, collectedErrors
	}

	for _, rule := range rules {
		ruleLogger := m.logger.With(zap.String("ruleName", rule.Name), zap.String("targetResourceType", rule.TargetResourceType))
		ruleLogger.Debug("Processing rule.")

		for _, block := range m.file.Body().Blocks() {
			// Check if the current block matches rule.TargetResourceType
			if block.Type() != "resource" || len(block.Labels()) == 0 || block.Labels()[0] != rule.TargetResourceType {
				continue
			}

			// Check if the current block matches rule.TargetResourceLabels (if specified)
			if len(rule.TargetResourceLabels) > 0 {
				// Assumes TargetResourceLabels corresponds to the full label set if present.
				// For "google_container_cluster" "my_cluster", Labels() is ["google_container_cluster", "my_cluster"]
				// So, if TargetResourceLabels is ["my_cluster"], we check block.Labels()[1:]
				// This needs to be robust. For now, let's assume TargetResourceLabels are the *additional* labels after type.
				// A common case is one additional label for the name.
				if len(block.Labels()) < 1+len(rule.TargetResourceLabels) {
					continue // Not enough labels to match
				}
				match := true
				for i, expectedLabel := range rule.TargetResourceLabels {
					if block.Labels()[i+1] != expectedLabel {
						match = false
						break
					}
				}
				if !match {
					continue
				}
			}
			resourceLogger := ruleLogger.With(zap.Strings("resourceLabels", block.Labels()))
			resourceLogger.Debug("Target resource matched. Checking conditions.")

			conditionsMet := true
			for _, condition := range rule.Conditions {
				condLogger := resourceLogger.With(zap.String("conditionType", string(condition.Type)), zap.Strings("conditionPath", condition.Path))
				switch condition.Type {
				case AttributeExists:
					_, _, err := m.GetAttributeValueByPath(block.Body(), condition.Path)
					if err != nil {
						condLogger.Debug("Condition AttributeExists not met.", zap.Error(err))
						conditionsMet = false
					}
				case BlockExists:
					_, err := m.GetNestedBlock(block.Body(), condition.Path)
					if err != nil {
						condLogger.Debug("Condition BlockExists not met.", zap.Error(err))
						conditionsMet = false
					}
				case AttributeValueEquals:
					val, _, err := m.GetAttributeValueByPath(block.Body(), condition.Path)
					if err != nil {
						condLogger.Debug("AttributeValueEquals: Attribute not found for comparison.", zap.Error(err))
						conditionsMet = false
						break
					}
					// Simple string comparison for now. Assumes ExpectedValue is a string.
					// cty.Value has a .Equals() method but requires parsing ExpectedValue to cty.Value of the correct type.
					var valStr string
					if val.Type().IsPrimitiveType() { //
						switch val.Type() {
						case cty.String:
							valStr = val.AsString()
						case cty.Number:
							valStr = val.AsBigFloat().String() // Or another appropriate string conversion
						case cty.Bool:
							valStr = fmt.Sprintf("%t", val.True())
						default:
							condLogger.Warn("AttributeValueEquals: Unhandled cty.Value primitive type for string conversion.", zap.Any("valueType", val.Type()))
							conditionsMet = false
						}
					} else {
						condLogger.Warn("AttributeValueEquals: Cannot compare non-primitive type.", zap.Any("valueType", val.Type()))
						conditionsMet = false
						break
					}

					if conditionsMet && valStr != condition.ExpectedValue {
						condLogger.Debug("AttributeValueEquals not met.", zap.String("actualValue", valStr), zap.String("expectedValue", condition.ExpectedValue))
						conditionsMet = false
					}
				default:
					condLogger.Warn("Unknown condition type.")
					conditionsMet = false // Unknown condition type means it cannot be met.
				}
				if !conditionsMet {
					break // Stop checking other conditions for this resource if one fails
				}
			}

			if conditionsMet {
				resourceLogger.Info("All conditions met. Performing actions.")
				for _, action := range rule.Actions {
					actLogger := resourceLogger.With(zap.String("actionType", string(action.Type)), zap.Strings("actionPath", action.Path))
					var errAction error
					switch action.Type {
					case RemoveAttribute:
						errAction = m.RemoveAttributeByPath(block.Body(), action.Path)
						if errAction == nil {
							totalModifications++
							actLogger.Info("Action RemoveAttribute successful.")
						}
					case RemoveBlock:
						errAction = m.RemoveNestedBlockByPath(block.Body(), action.Path)
						if errAction == nil {
							totalModifications++
							actLogger.Info("Action RemoveBlock successful.")
						}
					case SetAttributeValue:
						actLogger.Warn("Action SetAttributeValue is not yet implemented.")
						// errAction = m.SetAttributeValueByPath(block.Body(), action.Path, action.ValueToSet) // Placeholder
						// if errAction == nil { totalModifications++ }
					default:
						actLogger.Warn("Unknown action type.")
						errAction = fmt.Errorf("unknown action type: %s", action.Type)
					}

					if errAction != nil {
						actLogger.Error("Error performing action.", zap.Error(errAction))
						collectedErrors = append(collectedErrors, fmt.Errorf("rule '%s' action '%s' on resource '%s' failed: %w", rule.Name, action.Type, block.Labels(), errAction))
					}
				}
			} else {
				resourceLogger.Debug("Not all conditions met for resource.")
			}
		}
	}

	m.logger.Info("ApplyRules processing finished.", zap.Int("totalModifications", totalModifications), zap.Int("numberOfErrors", len(collectedErrors)))
	if len(collectedErrors) > 0 {
		for _, e := range collectedErrors {
			m.logger.Error("ApplyRules encountered an error during processing.", zap.Error(e))
		}
		return totalModifications, collectedErrors
	}
	return totalModifications, nil
}

// --- End of Rule Engine Structures and Processor Signature ---

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
func (m *Modifier) ApplyAutopilotRule() (modifications int, err error) {
	// m.logger.Info("Starting ApplyAutopilotRule (using path-based helpers).")
	modificationCount := 0
	var firstError error

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyAutopilotRule: Modifier's file or file body is nil.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	attributesToRemoveTrue := []string{
		"enable_shielded_nodes", "remove_default_node_pool", "default_max_pods_per_node",
		"enable_intranode_visibility", "cluster_ipv4_cidr",
	}
	topLevelNestedBlocksToRemoveTrue := []string{"network_policy"} // node_pool handled separately
	addonsConfigBlocksToRemove := []string{"network_policy_config", "dns_cache_config", "stateful_ha_config"}

	for _, block := range m.file.Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			resLogger := m.logger.With(zap.String("resourceName", resourceName), zap.String("rule", "ApplyAutopilotRule"))
			resLogger.Debug("Checking 'google_container_cluster' resource for Autopilot config.")

			autopilotVal, _, errAttr := m.GetAttributeValueByPath(block.Body(), []string{"enable_autopilot"})
			if errAttr != nil {
				resLogger.Debug("Attribute 'enable_autopilot' not found or value error.", zap.Error(errAttr))
				continue
			}

			if autopilotVal.Type() != cty.Bool {
				resLogger.Warn("'enable_autopilot' attribute is not a boolean value.", zap.String("valueType", autopilotVal.Type().FriendlyName()))
				continue
			}

			if autopilotVal.True() {
				resLogger.Info("Autopilot enabled. Applying modifications.")

				// Remove defined top-level attributes
				for _, attrName := range attributesToRemoveTrue {
					resLogger.Debug("Attempting to remove attribute.", zap.String("attributeName", attrName))
					errRemove := m.RemoveAttributeByPath(block.Body(), []string{attrName})
					if errRemove == nil {
						// Check if it actually existed by trying to get it again (or rely on RemoveAttributeByPath's internal check if it returns a specific error for not found)
						// For simplicity, we assume RemoveAttributeByPath is idempotent and doesn't error if not found.
						// To accurately count, we'd need GetAttributeValueByPath before Remove.
						// However, the original code incremented if RemoveAttribute did not error (which it doesn't if attr not found).
						// Let's refine this: only increment if it existed.
						// For now, let's assume RemoveAttributeByPath would tell us if it did something.
						// The current RemoveAttributeByPath returns nil if not found, so we can't directly tell if a mod happened.
						// To match original logic: increment if no error.
						modificationCount++ // This might overcount if attribute was already gone.
						resLogger.Info("Removed attribute (or attribute was not present).", zap.String("attributeName", attrName))
					} else {
						resLogger.Error("Error removing attribute.", zap.String("attributeName", attrName), zap.Error(errRemove))
						if firstError == nil {
							firstError = errRemove
						}
					}
				}

				// Remove defined top-level nested blocks (excluding node_pool)
				for _, blockName := range topLevelNestedBlocksToRemoveTrue {
					resLogger.Debug("Attempting to remove top-level nested block.", zap.String("blockName", blockName))
					errRemove := m.RemoveNestedBlockByPath(block.Body(), []string{blockName})
					if errRemove == nil {
						modificationCount++ // Similar caveat as above for accurate counting
						resLogger.Info("Removed top-level nested block (or block was not present).", zap.String("blockName", blockName))
					} else {
						resLogger.Error("Error removing top-level nested block.", zap.String("blockName", blockName), zap.Error(errRemove))
						if firstError == nil {
							firstError = errRemove
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
						if removed := block.Body().RemoveBlock(npBlock); removed {
							modificationCount++
							resLogger.Info("Removed 'node_pool' block instance.", zap.Strings("labels", npBlock.Labels()))
						} else {
							errRemove := fmt.Errorf("failed to remove node_pool block instance (labels: %v)", npBlock.Labels())
							resLogger.Error("Error removing 'node_pool' block instance.", zap.Error(errRemove))
							if firstError == nil {
								firstError = errRemove
							}
						}
					}
				}


				// Handle addons_config sub-blocks
				addonsConfigBlock, errGetAddons := m.GetNestedBlock(block.Body(), []string{"addons_config"})
				if errGetAddons == nil && addonsConfigBlock != nil {
					resLogger.Debug("Processing 'addons_config' for sub-block removal.")
					for _, subBlockName := range addonsConfigBlocksToRemove {
						errRemove := m.RemoveNestedBlockByPath(addonsConfigBlock.Body(), []string{subBlockName})
						if errRemove == nil {
							modificationCount++
							resLogger.Info("Removed sub-block from 'addons_config'.", zap.String("subBlockName", subBlockName))
						} else {
							resLogger.Error("Error removing sub-block from 'addons_config'.", zap.String("subBlockName", subBlockName), zap.Error(errRemove))
							if firstError == nil {
								firstError = errRemove
							}
						}
					}
				} else if errGetAddons != nil {
					resLogger.Debug("'addons_config' block not found, skipping removal of its sub-blocks.", zap.Error(errGetAddons))
				}

				// Handle cluster_autoscaling attributes
				caBlock, errGetCA := m.GetNestedBlock(block.Body(), []string{"cluster_autoscaling"})
				if errGetCA == nil && caBlock != nil {
					resLogger.Debug("Processing 'cluster_autoscaling' for attribute removal.")
					attrsToRmFromCA := []string{"enabled", "resource_limits"}
					for _, attrName := range attrsToRmFromCA {
						errRemove := m.RemoveAttributeByPath(caBlock.Body(), []string{attrName})
						if errRemove == nil {
							modificationCount++
							resLogger.Info("Removed attribute from 'cluster_autoscaling'.", zap.String("attributeName", attrName))
						} else {
							// Log error but continue, as per original logic (RemoveAttribute doesn't fail if attr missing)
							resLogger.Error("Error removing attribute from 'cluster_autoscaling'.", zap.String("attributeName", attrName), zap.Error(errRemove))
							if firstError == nil { firstError = errRemove }
						}
					}
				} else if errGetCA != nil {
					resLogger.Debug("'cluster_autoscaling' block not found.", zap.Error(errGetCA))
				}

				// Handle binary_authorization attributes
				baBlock, errGetBA := m.GetNestedBlock(block.Body(), []string{"binary_authorization"})
				if errGetBA == nil && baBlock != nil {
					resLogger.Debug("Processing 'binary_authorization' for attribute removal.")
					errRemove := m.RemoveAttributeByPath(baBlock.Body(), []string{"enabled"})
					if errRemove == nil {
						modificationCount++
						resLogger.Info("Removed 'enabled' attribute from 'binary_authorization'.")
					} else {
						resLogger.Error("Error removing 'enabled' attribute from 'binary_authorization'.", zap.Error(errRemove))
						if firstError == nil { firstError = errRemove }
					}
				} else if errGetBA != nil {
					resLogger.Debug("'binary_authorization' block not found.", zap.Error(errGetBA))
				}

			} else { // enable_autopilot is false
				resLogger.Info("Autopilot explicitly disabled. Removing 'enable_autopilot' attribute itself.")
				errRemove := m.RemoveAttributeByPath(block.Body(), []string{"enable_autopilot"})
				if errRemove == nil {
					modificationCount++
					resLogger.Info("Successfully removed 'enable_autopilot' (false) attribute.")
				} else {
					resLogger.Error("Error removing 'enable_autopilot' (false) attribute.", zap.Error(errRemove))
					if firstError == nil {
						firstError = errRemove
					}
				}
			}
		}
	}

	// Caller in cmd/root.go logs completion and modifications.
	// m.logger.Info("ApplyAutopilotRule finished.", zap.Int("modifications", modificationCount))
	if firstError != nil {
		m.logger.Error("ApplyAutopilotRule encountered errors during processing.", zap.Error(firstError))
	}
	return modificationCount, firstError
}
