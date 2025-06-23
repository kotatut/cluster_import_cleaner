package hclmodifier

import (
	"fmt"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/zclconf/go-cty/cty"
)

func findBlockInParsedFile(file *hclwrite.File, blockType string, blockName string) (*hclwrite.Block, error) {
	for _, block := range file.Body().Blocks() {
		if block.Type() == "resource" && len(block.Labels()) == 2 && block.Labels()[0] == blockType && block.Labels()[1] == blockName {
			return block, nil
		}
	}
	return nil, fmt.Errorf("block not found")
}

func assertAttributeValue(t *testing.T, modifier *Modifier, block *hclwrite.Block, attributeName string, expectedValue cty.Value) {
	t.Helper()
	value, err := modifier.GetAttributeValue(block.Body().GetAttribute(attributeName))
	if err != nil {
		t.Fatalf("Error getting attribute value: %v", err)
	}
	assert.Equal(t, expectedValue, value, "attribute %s should have value %s", attributeName, expectedValue.GoString())
}

func findNodePoolInBlock(gkeBlock *hclwrite.Block, nodePoolName string, modifier *Modifier) (*hclwrite.Block, error) {
	for _, block := range gkeBlock.Body().Blocks() {
		if block.Type() == "node_pool" {
			value, err := modifier.GetAttributeValue(block.Body().GetAttribute("name"))
			if err != nil {
				continue
			}
			if value.AsString() == nodePoolName {
				return block, nil
			}
		}
	}
	return nil, fmt.Errorf("node pool not found")
}

func assertNodePoolAttributeAbsent(t *testing.T, modifier *Modifier, gkeResourceName string, nodePoolName string, attributeName string) {
	t.Helper()
	gkeBlock, err := findBlockInParsedFile(modifier.File(), "google_container_cluster", gkeResourceName)
	if err != nil {
		t.Fatalf("Failed to find GKE resource: %v", err)
	}
	nodePoolBlock, err := findNodePoolInBlock(gkeBlock, nodePoolName, modifier)
	if err != nil {
		t.Fatalf("Failed to find node pool: %v", err)
	}
	assert.Nil(t, nodePoolBlock.Body().GetAttribute(attributeName), "attribute %s should be absent", attributeName)
}

func assertNodePoolAttributeExists(t *testing.T, modifier *Modifier, gkeResourceName string, nodePoolName string, attributeName string) {
	t.Helper()
	gkeBlock, err := findBlockInParsedFile(modifier.File(), "google_container_cluster", gkeResourceName)
	if err != nil {
		t.Fatalf("Failed to find GKE resource: %v", err)
	}
	nodePoolBlock, err := findNodePoolInBlock(gkeBlock, nodePoolName, modifier)
	if err != nil {
		t.Fatalf("Failed to find node pool: %v", err)
	}
	assert.NotNil(t, nodePoolBlock.Body().GetAttribute(attributeName), "attribute %s should be present", attributeName)
}

func assertNodePoolAttributeValue(t *testing.T, modifier *Modifier, gkeResourceName string, nodePoolName string, attributeName string, expectedValue cty.Value) {
	t.Helper()
	gkeBlock, err := findBlockInParsedFile(modifier.File(), "google_container_cluster", gkeResourceName)
	if err != nil {
		t.Fatalf("Failed to find GKE resource: %v", err)
	}
	nodePoolBlock, err := findNodePoolInBlock(gkeBlock, nodePoolName, modifier)
	if err != nil {
		t.Fatalf("Failed to find node pool: %v", err)
	}
	value, err := modifier.GetAttributeValue(nodePoolBlock.Body().GetAttribute(attributeName))
	if err != nil {
		t.Fatalf("Error getting attribute value: %v", err)
	}

	if expectedValue.Type().IsPrimitiveType() && value.Type().IsPrimitiveType() {
		if expectedValue.Type() == cty.Number && value.Type() == cty.Number {
			expected, _ := expectedValue.AsBigFloat().Float64()
			actual, _ := value.AsBigFloat().Float64()
			assert.InDelta(t, expected, actual, 0.001, "attribute %s should have value %s", attributeName, expectedValue.GoString())
			return
		}
	}

	assert.Equal(t, expectedValue, value, "attribute %s should have value %s", attributeName, expectedValue.GoString())
}

func intPtr(i int) *int {
	return &i
}
