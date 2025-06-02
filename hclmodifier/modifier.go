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
	m.logger.Info("Starting ApplyRule1")
	modificationCount := 0

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyRule1 called on a Modifier with nil file or file body.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		// Rule 2: Identify `resource` blocks with type `google_container_cluster`.
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			m.logger.Debug("Checking 'google_container_cluster' resource", zap.String("name", resourceName))

			// Rule 3a: Check for the existence of the `cluster_ipv4_cidr` attribute.
			mainClusterCIDRAttribute := block.Body().GetAttribute("cluster_ipv4_cidr")

			// Rule 3b: Check for the existence of an `ip_allocation_policy` nested block.
			var ipAllocationPolicyBlock *hclwrite.Block
			// Iterate over nested blocks of the current resource block
			for _, nestedBlock := range block.Body().Blocks() {
				if nestedBlock.Type() == "ip_allocation_policy" {
					ipAllocationPolicyBlock = nestedBlock
					m.logger.Debug("Found 'ip_allocation_policy' block", zap.String("resourceName", resourceName))
					break
				}
			}

			// Rule 3c: If ip_allocation_policy block exists, check for cluster_ipv4_cidr_block.
			var nestedClusterCIDRAttribute *hclwrite.Attribute
			if ipAllocationPolicyBlock != nil {
				nestedClusterCIDRAttribute = ipAllocationPolicyBlock.Body().GetAttribute("cluster_ipv4_cidr_block")
				if nestedClusterCIDRAttribute != nil {
					m.logger.Debug("Found 'cluster_ipv4_cidr_block' in 'ip_allocation_policy'", zap.String("resourceName", resourceName))
				}
			}

			// Rule 3d: If both attributes are found, remove the one from the main block.
			if mainClusterCIDRAttribute != nil && nestedClusterCIDRAttribute != nil {
				m.logger.Info("Found 'cluster_ipv4_cidr' in main block and 'cluster_ipv4_cidr_block' in 'ip_allocation_policy'",
					zap.String("resourceName", resourceName),
					zap.String("attributeToRemove", "cluster_ipv4_cidr"))

				block.Body().RemoveAttribute("cluster_ipv4_cidr")
				modificationCount++ // Rule 3e: Increment counter
				m.logger.Info("Removed 'cluster_ipv4_cidr' attribute", zap.String("resourceName", resourceName))
			} else {
				if mainClusterCIDRAttribute == nil {
					m.logger.Debug("Attribute 'cluster_ipv4_cidr' not found in main block", zap.String("resourceName", resourceName))
				}
				if ipAllocationPolicyBlock == nil {
					m.logger.Debug("'ip_allocation_policy' block not found", zap.String("resourceName", resourceName))
				} else if nestedClusterCIDRAttribute == nil {
					m.logger.Debug("Attribute 'cluster_ipv4_cidr_block' not found in 'ip_allocation_policy' block", zap.String("resourceName", resourceName))
				}
			}
		}
	}

	m.logger.Info("ApplyRule1 finished", zap.Int("modifications", modificationCount))
	return modificationCount, nil
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
	m.logger.Info("Starting ApplyRule2")
	modificationCount := 0

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyRule2 called on a Modifier with nil file or file body.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		// Rule 2: Identify `resource` blocks with type `google_container_cluster`.
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			m.logger.Debug("Checking 'google_container_cluster' resource for Rule 2", zap.String("name", resourceName))

			// Rule 3a: Find the `ip_allocation_policy` nested block.
			var ipAllocationPolicyBlock *hclwrite.Block
			for _, nestedBlock := range block.Body().Blocks() {
				if nestedBlock.Type() == "ip_allocation_policy" {
					ipAllocationPolicyBlock = nestedBlock
					m.logger.Debug("Found 'ip_allocation_policy' block for Rule 2", zap.String("resourceName", resourceName))
					break
				}
			}

			// Rule 3b: If `ip_allocation_policy` block exists.
			if ipAllocationPolicyBlock != nil {
				// Rule 3b.i: Check for `services_ipv4_cidr_block`.
				servicesCIDRAttribute := ipAllocationPolicyBlock.Body().GetAttribute("services_ipv4_cidr_block")
				// Rule 3b.ii: Check for `cluster_secondary_range_name`.
				secondaryRangeAttribute := ipAllocationPolicyBlock.Body().GetAttribute("cluster_secondary_range_name")

				// Rule 3b.iii: If both attributes are found, remove `services_ipv4_cidr_block`.
				if servicesCIDRAttribute != nil && secondaryRangeAttribute != nil {
					m.logger.Info("Found 'services_ipv4_cidr_block' and 'cluster_secondary_range_name' in 'ip_allocation_policy'",
						zap.String("resourceName", resourceName),
						zap.String("attributeToRemove", "services_ipv4_cidr_block"))

					ipAllocationPolicyBlock.Body().RemoveAttribute("services_ipv4_cidr_block")
					modificationCount++ // Rule 3b.iv: Increment counter
					m.logger.Info("Removed 'services_ipv4_cidr_block' attribute from 'ip_allocation_policy'", zap.String("resourceName", resourceName))
				} else {
					if servicesCIDRAttribute == nil {
						m.logger.Debug("Attribute 'services_ipv4_cidr_block' not found in 'ip_allocation_policy'", zap.String("resourceName", resourceName))
					}
					if secondaryRangeAttribute == nil {
						m.logger.Debug("Attribute 'cluster_secondary_range_name' not found in 'ip_allocation_policy'", zap.String("resourceName", resourceName))
					}
				}
			} else {
				m.logger.Debug("'ip_allocation_policy' block not found for Rule 2", zap.String("resourceName", resourceName))
			}
		}
	}

	m.logger.Info("ApplyRule2 finished", zap.Int("modifications", modificationCount))
	return modificationCount, nil
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
	m.logger.Info("Starting ApplyRule3")
	modificationCount := 0

	if m.file == nil || m.file.Body() == nil {
		m.logger.Error("ApplyRule3 called on a Modifier with nil file or file body.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		// Rule 2: Identify `resource` blocks with type `google_container_cluster`.
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == "google_container_cluster" {
			resourceName := block.Labels()[1]
			m.logger.Debug("Checking 'google_container_cluster' resource for Rule 3", zap.String("name", resourceName))

			// Rule 3a: Check for a nested block named `binary_authorization`.
			var binaryAuthorizationBlock *hclwrite.Block
			// Iterate over nested blocks of the current resource block
			for _, nestedBlock := range block.Body().Blocks() {
				if nestedBlock.Type() == "binary_authorization" {
					binaryAuthorizationBlock = nestedBlock
					m.logger.Debug("Found 'binary_authorization' block for Rule 3", zap.String("resourceName", resourceName))
					break
				}
			}

			// Rule 3b: If the `binary_authorization` block exists.
			if binaryAuthorizationBlock != nil {
				// Rule 3b.i: Check for an attribute named `enabled` within this nested block.
				enabledAttribute := binaryAuthorizationBlock.Body().GetAttribute("enabled")
				// Rule 3b.ii: Check for an attribute named `evaluation_mode` within this nested block.
				evaluationModeAttribute := binaryAuthorizationBlock.Body().GetAttribute("evaluation_mode")

				// Rule 3b.iii: If both `enabled` and `evaluation_mode` attributes are found, remove the `enabled` attribute.
				if enabledAttribute != nil && evaluationModeAttribute != nil {
					m.logger.Info("Found 'enabled' and 'evaluation_mode' attributes in 'binary_authorization' block",
						zap.String("resourceName", resourceName),
						zap.String("attributeToRemove", "enabled"))

					binaryAuthorizationBlock.Body().RemoveAttribute("enabled")
					modificationCount++ // Rule 3b.iv: Increment counter
					m.logger.Info("Removed 'enabled' attribute from 'binary_authorization' block", zap.String("resourceName", resourceName))
				} else {
					if enabledAttribute == nil {
						m.logger.Debug("Attribute 'enabled' not found in 'binary_authorization' block", zap.String("resourceName", resourceName))
					}
					if evaluationModeAttribute == nil {
						m.logger.Debug("Attribute 'evaluation_mode' not found in 'binary_authorization' block", zap.String("resourceName", resourceName))
					}
				}
			} else {
				m.logger.Debug("'binary_authorization' block not found for Rule 3", zap.String("resourceName", resourceName))
			}
		}
	}

	m.logger.Info("ApplyRule3 finished", zap.Int("modifications", modificationCount))
	return modificationCount, nil
}
