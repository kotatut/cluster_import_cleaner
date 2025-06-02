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
	Logger *zap.Logger
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

	return &Modifier{file: hclFile, Logger: logger}, nil
}

// File returns the underlying hclwrite.File object for inspection if needed.
func (m *Modifier) File() *hclwrite.File {
	return m.file
}

// WriteToFile saves the current state of the HCL file to the specified path.
func (m *Modifier) WriteToFile(filePath string) error {
	modifiedBytes := m.file.Bytes()
	m.Logger.Debug("Writing modified HCL to file", zap.String("filePath", filePath))
	err := os.WriteFile(filePath, modifiedBytes, 0644)
	if err != nil {
		m.Logger.Error("Error writing modified HCL to file", zap.String("filePath", filePath), zap.Error(err))
		return err
	}
	m.Logger.Info("Successfully wrote modified HCL to file", zap.String("filePath", filePath))
	return nil
}

// ModifyNameAttributes iterates through the HCL file and appends "-clone"
// to the value of any attribute named "name" that is a simple string literal.
// It returns the count of modified attributes.
func (m *Modifier) ModifyNameAttributes() (int, error) {
	modifiedCount := 0
	if m.file == nil || m.file.Body() == nil {
		m.Logger.Error("ModifyNameAttributes called on a Modifier with nil file or file body.")
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	for _, block := range m.file.Body().Blocks() {
		// Only modify "name" attributes within "resource" blocks
		if block.Type() != "resource" {
			m.Logger.Debug("Skipping block as it is not a resource type",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()))
			continue
		}

		nameAttribute, err := m.GetAttribute(block, "name")
		if err != nil {
			m.Logger.Debug("Attribute 'name' not found in block, skipping.",
				zap.String("blockType", block.Type()),
				zap.Strings("blockLabels", block.Labels()))
			continue
		}

		attrValue, err := m.GetAttributeValue(nameAttribute)
		if err != nil {
			m.Logger.Info("Skipping 'name' attribute: could not get simple literal value.",
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
				m.Logger.Error("Failed to set modified 'name' attribute",
					zap.String("blockType", block.Type()),
					zap.Strings("blockLabels", block.Labels()),
					zap.Error(err))
				continue
			}
			modifiedCount++
		}
	}

	if modifiedCount == 0 {
		m.Logger.Info("No 'name' attributes were modified.")
	} else {
		m.Logger.Info("Total 'name' attributes modified", zap.Int("count", modifiedCount))
	}
	return modifiedCount, nil
}

// GetBlock finds and returns a specific block based on its type and labels.
func (m *Modifier) GetBlock(blockType string, blockLabels []string) (*hclwrite.Block, error) {
	m.Logger.Debug("Searching for block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	for _, block := range m.file.Body().Blocks() {
		if block.Type() == blockType && slices.Equal(block.Labels(), blockLabels) {
			m.Logger.Debug("Found matching block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
			return block, nil
		}
	}
	m.Logger.Warn("Block not found", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil, fmt.Errorf("block %s %v not found", blockType, blockLabels)
}

// GetAttribute finds and returns a specific attribute from a block by its name.
func (m *Modifier) GetAttribute(block *hclwrite.Block, attributeName string) (*hclwrite.Attribute, error) {
	attribute := block.Body().GetAttribute(attributeName)
	if attribute == nil {
		m.Logger.Debug("Attribute not found in block",
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
		m.Logger.Error("Failed to re-parse attribute expression for evaluation.", zap.Error(diags))
		return cty.NilVal, fmt.Errorf("failed to parse expression: %w", diags)
	}

	// 3. Now, with an hcl.Expression, we can call .Value() to get the cty.Value.
	// We pass a nil EvalContext because we only want to resolve simple literals.
	val, diags := expr.Value(nil)
	if diags.HasErrors() {
		m.Logger.Debug("Attribute expression is not a simple literal", zap.String("expression", string(exprBytes)), zap.Error(diags))
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
	m.Logger.Debug("Successfully set attribute",
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
		m.Logger.Debug("Attribute to remove not found, no action needed.", zap.String("attributeName", attributeName))
		return nil
	}
	block.Body().RemoveAttribute(attributeName)
	m.Logger.Debug("Successfully removed attribute",
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
		m.Logger.Error("Failed to remove block, RemoveBlock method returned false", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
		return fmt.Errorf("failed to remove block %s %v", blockType, blockLabels)
	}
	m.Logger.Info("Successfully removed block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil
}

func (m *Modifier) RemoveAttributes(resourceTypeLabel string, optionalResourceName *string, attributesToRemove []string) (removedCount int, err error) {
	m.Logger.Debug("Attempting to remove attributes",
		zap.String("resourceTypeLabel", resourceTypeLabel),
		zap.Any("optionalResourceName", optionalResourceName),
		zap.Strings("attributesToRemove", attributesToRemove))

	if m.file == nil || m.file.Body() == nil {
		return 0, fmt.Errorf("modifier's file or file body cannot be nil")
	}

	if len(attributesToRemove) == 0 {
		m.Logger.Debug("No attributes specified to remove.")
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

		m.Logger.Debug("Processing matching block for attribute removal",
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))

		for _, attrName := range attributesToRemove {
			// Use the existing RemoveAttribute method on Modifier.
			// This method already handles logging and the case where attribute doesn't exist.
			errRemove := m.RemoveAttribute(block, attrName)
			if errRemove != nil {
				m.Logger.Error("Error removing attribute from block",
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
		m.Logger.Warn("Specified resource name not found",
			zap.String("resourceTypeLabel", resourceTypeLabel),
			zap.String("targetResourceName", targetResourceName))
		return removedCount, fmt.Errorf("resource '%s' with name '%s' not found", resourceTypeLabel, targetResourceName)
	}

	m.Logger.Info("Finished removing attributes",
		zap.Int("totalAttributesActuallyRemoved", removedCount), // This count might not be perfectly accurate as explained above.
		zap.String("resourceTypeLabel", resourceTypeLabel),
		zap.Any("optionalResourceName", optionalResourceName))
	return removedCount, nil
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

	logger := m.Logger.With(zap.Strings("path", path))
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

	logger := m.Logger.With(zap.Strings("path", path))
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

	logger := m.Logger.With(zap.Strings("path", path))
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

	logger := m.Logger.With(zap.Strings("path", path))
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
	m.Logger.Info("Starting ApplyRules processing.", zap.Int("numberOfRules", len(rules)))
	totalModifications := 0
	var collectedErrors []error

	if m.file == nil || m.file.Body() == nil {
		m.Logger.Error("ApplyRules: Modifier's file or file body is nil.")
		collectedErrors = append(collectedErrors, fmt.Errorf("modifier's file or file body cannot be nil"))
		return 0, collectedErrors
	}

	for _, rule := range rules {
		ruleLogger := m.Logger.With(zap.String("ruleName", rule.Name), zap.String("targetResourceType", rule.TargetResourceType))
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

	m.Logger.Info("ApplyRules processing finished.", zap.Int("totalModifications", totalModifications), zap.Int("numberOfErrors", len(collectedErrors)))
	if len(collectedErrors) > 0 {
		for _, e := range collectedErrors {
			m.Logger.Error("ApplyRules encountered an error during processing.", zap.Error(e))
		}
		return totalModifications, collectedErrors
	}
	return totalModifications, nil
}

// --- End of Rule Engine Structures and Processor Signature ---

