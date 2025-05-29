package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax" // Added for ParseExpression
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty" // Added for cty.String
	"go.uber.org/zap"
)

func TestModifyNameAttributes(t *testing.T) {
	t.Helper()

	const expectedSuffix = "-clone"

	tests := []struct {
		name              string
		hclContent        string
		expectedName      string 
		expectedAttrCount int
	}{
		{
			name: "single resource with name",
			hclContent: `
resource "aws_instance" "example" {
  name = "test_name"
}`,
			expectedName:      "test_name" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "multiple resources with name",
			hclContent: `
resource "aws_instance" "example1" {
  name = "test_name1"
}
resource "aws_instance" "example2" {
  name = "test_name2"
}`,
			expectedName:      "test_name1" + expectedSuffix, 
			expectedAttrCount: 2,
		},
		{
			name: "resource without name",
			hclContent: `
resource "aws_instance" "example" {
  ami = "ami-0c55b31ad2c454370"
}`,
			expectedName:      "", 
			expectedAttrCount: 0,
		},
		{
			name: "resource with empty name",
			hclContent: `
resource "aws_instance" "example" {
  name = ""
}`,
			expectedName:      "" + expectedSuffix,
			expectedAttrCount: 1,
		},
		{
			name: "terraform block with name attribute",
			hclContent: `
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 4.0"
    }
  }
  name = "my_terraform_block" 
}`,
			expectedName:      "", 
			expectedAttrCount: 0, // Now expecting 0 due to modifier.go change
		},
		{
			name: "resource with complex name expression",
			hclContent: `
resource "aws_instance" "example" {
  name = local.name
}`,
			expectedName:      "", 
			expectedAttrCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), nil) 
			if err != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			}

			modifiedCount, err := modifier.ModifyNameAttributes()
			if err != nil {
				t.Fatalf("ModifyNameAttributes() error = %v", err)
			}

			if modifiedCount != tc.expectedAttrCount {
				t.Errorf("ModifyNameAttributes() modifiedCount = %v, want %v", modifiedCount, tc.expectedAttrCount)
			}

			if tc.expectedAttrCount > 0 && tc.expectedName != "" {
				var foundTheSpecificExpectedName bool

				for _, block := range modifier.File().Body().Blocks() { 
					if block.Type() != "resource" {
						continue
					}
					if nameAttr, ok := block.Body().Attributes()["name"]; ok {
						exprBytes := nameAttr.Expr().BuildTokens(nil).Bytes()
						expr, diagsParse := hclsyntax.ParseExpression(exprBytes, tmpFile.Name()+"#nameattr", hcl.Pos{Line: 1, Column: 1})
						if diagsParse.HasErrors() {
							t.Logf("Skipping attribute due to parse error during verification: %v", diagsParse)
							continue
						}
						
						valNode, diagsValue := expr.Value(nil) 
						if diagsValue.HasErrors() {
							t.Logf("Skipping attribute due to value error during verification: %v", diagsValue)
							continue
						}

						if valNode.Type() == cty.String {
							extractedName := valNode.AsString()
							if extractedName == tc.expectedName {
								foundTheSpecificExpectedName = true
								break 
							}
						}
					}
				}

				if !foundTheSpecificExpectedName {
					t.Errorf("ModifyNameAttributes() did not find a resource with the expected modified name '%s'. Output HCL:\n%s", tc.expectedName, string(modifier.File().Bytes()))
				}
			} else if tc.expectedAttrCount == 0 && tc.expectedName != "" {
				// This case implies that if no attributes are expected to be counted as modified,
				// then no specific name should be searched for if it was expected to be modified.
				// If the intent is to check the name *wasn't* modified, this logic would need to be different.
				// For now, ensuring the count is 0 is the primary check for these cases.
				// A specific check for the *absence* of tc.expectedName (if it was the original name) could be added.
				t.Logf("Test case info: expectedAttrCount is 0. expectedName ('%s') is set; this test primarily verifies count, not specific name absence/presence unless count > 0.", tc.expectedName)
			}
		})
	}
}

func TestRemoveBlock(t *testing.T) {
	t.Helper()

	tests := []struct {
		name            string
		hclContent      string
		blockType       string
		blockLabels     []string
		expectRemoved   bool 
		expectCallError bool 
	}{
		{
			name: "remove existing resource block",
			hclContent: `
resource "aws_instance" "my_test_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-test-bucket"
}`,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   true,
			expectCallError: false,
		},
		{
			name: "attempt to remove non-existing resource block by name",
			hclContent: `
resource "aws_instance" "another_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}`,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "non_existent_instance"},
			expectRemoved:   false, 
			expectCallError: true,  
		},
		{
			name: "attempt to remove block with incorrect type but existing labels",
			hclContent: `
resource "aws_instance" "my_test_instance" {
  ami           = "ami-0c55b31ad2c454370"
  instance_type = "t2.micro"
}`,
			blockType:       "data", 
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   false,
			expectCallError: true, 
		},
		{
			name:            "empty HCL content",
			hclContent:      ``,
			blockType:       "resource",
			blockLabels:     []string{"aws_instance", "my_test_instance"},
			expectRemoved:   false,
			expectCallError: true, 
		},
		{
			name: "remove data block",
			hclContent: `
data "aws_caller_identity" "current" {}
resource "aws_instance" "main" {}
`,
			blockType:       "data",
			blockLabels:     []string{"aws_caller_identity", "current"},
			expectRemoved:   true,
			expectCallError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			if _, err := tmpFile.Write([]byte(tc.hclContent)); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}
			
			var logger *zap.Logger 

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil && tc.hclContent != "" { 
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, tc.hclContent)
			} else if err == nil && tc.hclContent == "" {
                 // Allow empty HCL to proceed if NewFromFile handles it (e.g. creates empty body)
            }


			err = modifier.RemoveBlock(tc.blockType, tc.blockLabels) 
			if (err != nil) != tc.expectCallError {
				t.Fatalf("RemoveBlock() error status = %v (err: %v), expectCallError %v. HCL:\n%s", (err != nil), err, tc.expectCallError, tc.hclContent)
			}

			foundBlock, getErr := modifier.GetBlock(tc.blockType, tc.blockLabels) 

			if tc.expectRemoved {
				if foundBlock != nil || getErr == nil { 
					t.Errorf("RemoveBlock() expected block %s %v to be removed, but GetBlock found it (block: %v, err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, foundBlock, getErr, string(modifier.File().Bytes()))
				}
			} else { 
				initialFile, parseDiags := hclwrite.ParseConfig([]byte(tc.hclContent), tmpFile.Name(), hcl.Pos{Line: 1, Column: 1})
				initialBlockPresent := false
				if !parseDiags.HasErrors() && initialFile != nil && initialFile.Body() != nil {
					for _, b := range initialFile.Body().Blocks() {
						if b.Type() == tc.blockType && len(b.Labels()) == len(tc.blockLabels) {
							match := true
							for i, l := range b.Labels() {
								if l != tc.blockLabels[i] {
									match = false; break
								}
							}
							if match { initialBlockPresent = true; break }
						}
					}
				}

				if tc.expectCallError { // If RemoveBlock errored (e.g., block not found)
					if foundBlock != nil || getErr == nil { // Then GetBlock should also not find it or error
                         t.Errorf("RemoveBlock() errored as expected for %s %v, but GetBlock still found it (block: %v, err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, foundBlock, getErr, string(modifier.File().Bytes()))
                     }
				} else { // If RemoveBlock did NOT error (e.g. block was there but shouldn't be removed by this call, or was not there and call did not error)
					if initialBlockPresent { // And it was there initially
						if foundBlock == nil || getErr != nil { // It should still be findable
							t.Errorf("RemoveBlock() did not remove block %s %v as expected (initial state: present), but GetBlock also failed to find it (err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, getErr, string(modifier.File().Bytes()))
						}
					} else { // And it was NOT there initially
						if foundBlock != nil || getErr == nil { // It should still not be findable
                             t.Errorf("RemoveBlock() logic error: block %s %v was not present initially nor targeted for removal, but GetBlock found it after RemoveBlock (err: %v). Output HCL:\n%s", tc.blockType, tc.blockLabels, getErr, string(modifier.File().Bytes()))
                        }
					}
				}
			}
		})
	}
}
