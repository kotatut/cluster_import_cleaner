package hclmodifier

import (
	"fmt"
	"os"
	"slices" // Using standard library for slice comparison (Go 1.21+)
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
		// Fallback if no logger is provided, though ideally the caller should always provide one.
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
		// Wrap the diagnostics error for better context upstream.
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
			// This is expected for blocks without a "name", so we log at debug level and continue.
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
				// This would be an unexpected error.
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
		// Logged at a lower level as it's often an expected condition, not a warning.
		m.logger.Debug("Attribute not found in block",
			zap.String("attributeName", attributeName),
			zap.String("blockType", block.Type()),
			zap.Strings("blockLabels", block.Labels()))
		return nil, fmt.Errorf("attribute '%s' not found", attributeName)
	}
	return attribute, nil
}

// GetAttributeValue extracts a cty.Value from an attribute,
// ensuring it's a simple literal (string, number, bool) without references.
func (m *Modifier) GetAttributeValue(attr *hclwrite.Attribute) (cty.Value, error) {
	// We inspect the raw tokens of the expression.
	tokens := attr.Expr().BuildTokens(nil)

	// A simple string literal consists of just one token of type TokenQuotedLit.
	if len(tokens) == 1 && tokens[0].Type == hclsyntax.TokenQuotedLit {
		// The token bytes include the quotes. strconv.Unquote safely removes them.
		value, err := strconv.Unquote(string(tokens[0].Bytes))
		if err != nil {
			m.logger.Warn("Failed to unquote string token",
				zap.Error(err))
			return cty.NilVal, fmt.Errorf("attribute has an invalid string literal: %w", err)
		}
		return cty.StringVal(value), nil
	}

	// If the expression is not a simple string literal, we return an error.
	m.logger.Debug("Attribute is not a simple string literal, skipping.")
	return cty.NilVal, fmt.Errorf("attribute is not a simple string literal")
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
		return nil // Not an error if it doesn't exist.
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
		// This is unlikely to happen if the block was found correctly.
		m.logger.Error("Failed to remove block, RemoveBlock method returned false", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
		return fmt.Errorf("failed to remove block %s %v", blockType, blockLabels)
	}

	m.logger.Info("Successfully removed block", zap.String("blockType", blockType), zap.Strings("blockLabels", blockLabels))
	return nil
}