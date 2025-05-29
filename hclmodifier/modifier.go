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
