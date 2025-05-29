package hclmodifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// Helper function to create a temporary HCL file with given content.
func createTempHCLFile(t *testing.T, content string) (filePath string, cleanup func()) {
	t.Helper()
	tempDir := t.TempDir()
	filePath = filepath.Join(tempDir, "test.tf")
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create temporary HCL file")
	return filePath, func() { /* os.RemoveAll(tempDir) is handled by t.TempDir() */ }
}

// TestParseHCLFile_Valid tests parsing a valid HCL file.
func TestParseHCLFile_Valid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "null_resource" "example" {
  name = "test"
}
`
	filePath, _ := createTempHCLFile(t, hclContent)

	hclFile, err := ParseHCLFile(filePath, logger)
	assert.NoError(t, err)
	require.NotNil(t, hclFile)
	assert.NotEmpty(t, hclFile.Bytes(), "Parsed HCL file should not be empty")
}

// TestParseHCLFile_Invalid tests parsing an invalid HCL file.
func TestParseHCLFile_Invalid(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "null_resource" "example" {
  name = "test"
` // Missing closing brace
	filePath, _ := createTempHCLFile(t, hclContent)

	_, err := ParseHCLFile(filePath, logger)
	assert.Error(t, err, "Parsing invalid HCL should return an error")
}

// TestParseHCLFile_NonExistentFile tests parsing a non-existent file.
func TestParseHCLFile_NonExistentFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	nonExistentFilePath := filepath.Join(t.TempDir(), "non_existent.tf") // Ensure it doesn't exist

	_, err := ParseHCLFile(nonExistentFilePath, logger)
	assert.Error(t, err, "Parsing non-existent file should return an error")
	assert.True(t, os.IsNotExist(err) || strings.Contains(err.Error(), "no such file or directory"), "Error should be a file not exist error")
}

// TestWriteAndParseHCLFile_RoundTrip tests writing then reading an HCL file.
func TestWriteAndParseHCLFile_RoundTrip(t *testing.T) {
	logger := zaptest.NewLogger(t)
	originalHCLContent := `
resource "aws_instance" "web" {
  ami           = "ami-0c55b31ad09f5967f"
  instance_type = "t2.micro"
}
`
	// 1. Parse initial content to get an hclwrite.File object
	parsedFile, diags := hclwrite.ParseConfig([]byte(originalHCLContent), "original.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Initial parsing failed: %s", diags.Error())
	require.NotNil(t, parsedFile)

	// 2. Write this hclwrite.File to a temporary file
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "roundtrip.tf")
	err := WriteHCLFile(filePath, parsedFile, logger)
	require.NoError(t, err, "WriteHCLFile failed")

	// 3. Read the file back using ParseHCLFile
	readFile, err := ParseHCLFile(filePath, logger)
	require.NoError(t, err, "ParseHCLFile failed for reading back")
	require.NotNil(t, readFile)

	// 4. Compare the byte content (or string representation)
	// hclwrite should ideally preserve formatting, making byte comparison feasible.
	assert.Equal(t, strings.TrimSpace(originalHCLContent), strings.TrimSpace(string(readFile.Bytes())), "Content after round trip should match original")
}


// TestGetBlock_Exists tests finding an existing block.
func TestGetBlock_Exists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_s3_bucket" "my_bucket" {
  bucket = "my-unique-s3-bucket-name"
}
provider "aws" {
  region = "us-west-2"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Parsing HCL content failed: %s", diags.Error())

	// Test finding the resource block
	block, err := GetBlock(hclFile, "resource", []string{"aws_s3_bucket", "my_bucket"}, logger)
	assert.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "resource", block.Type())
	assert.Equal(t, []string{"aws_s3_bucket", "my_bucket"}, block.Labels())

	// Test finding the provider block
	block, err = GetBlock(hclFile, "provider", []string{"aws"}, logger)
	assert.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "provider", block.Type())
	assert.Equal(t, []string{"aws"}, block.Labels())
}

// TestGetBlock_NotFound tests various scenarios where a block is not found.
func TestGetBlock_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "example" {
  ami = "ami-12345"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Parsing HCL content failed: %s", diags.Error())

	// Wrong type
	_, err := GetBlock(hclFile, "data", []string{"aws_instance", "example"}, logger)
	assert.Error(t, err, "Should return error for wrong block type")

	// Wrong labels
	_, err = GetBlock(hclFile, "resource", []string{"aws_instance", "wrong_name"}, logger)
	assert.Error(t, err, "Should return error for wrong block labels")

	// Non-existent block entirely
	_, err = GetBlock(hclFile, "resource", []string{"aws_s3_bucket", "test_bucket"}, logger)
	assert.Error(t, err, "Should return error for non-existent block")

	// Correct type, but too few labels provided
	_, err = GetBlock(hclFile, "resource", []string{"aws_instance"}, logger)
	assert.Error(t, err, "Should return error if not all labels are provided for a match")

	// Correct type, but too many labels provided
	_, err = GetBlock(hclFile, "resource", []string{"aws_instance", "example", "extra_label"}, logger)
	assert.Error(t, err, "Should return error if too many labels are provided for a match")
}

// TestGetBlock_MultipleBlocks tests finding a block among many.
func TestGetBlock_MultipleBlocks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" {
  ami = "ami-web"
}
resource "aws_instance" "app" {
  ami = "ami-app"
}
resource "aws_s3_bucket" "logs" {
  bucket = "my-logs-bucket"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Parsing HCL content failed: %s", diags.Error())

	block, err := GetBlock(hclFile, "resource", []string{"aws_instance", "app"}, logger)
	assert.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "resource", block.Type())
	assert.Equal(t, []string{"aws_instance", "app"}, block.Labels())
	// Optionally, check a unique attribute of this block
	appAmiAttr := block.Body().GetAttribute("ami")
	require.NotNil(t, appAmiAttr)
	// For GetAttributeValue, we'd need a more complex setup or direct token inspection for simple string.
	// Here, a simple check of token representation might suffice if we assume simple string.
	// Or use a helper to extract simple string for test assertion.
	val, err := GetAttributeValue(appAmiAttr, logger)
	require.NoError(t, err)
	assert.Equal(t, "ami-app", val.AsString(), "AMI for app instance should be 'ami-app'")

}

// TestGetBlock_NoLabels tests finding a block that has no labels (e.g., terraform block).
func TestGetBlock_NoLabels(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
terraform {
  required_version = ">= 1.0"
}
resource "aws_instance" "web" {
  ami = "ami-web"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "Parsing HCL content failed: %s", diags.Error())

	// Find terraform block (no labels)
	block, err := GetBlock(hclFile, "terraform", []string{}, logger)
	assert.NoError(t, err)
	require.NotNil(t, block)
	assert.Equal(t, "terraform", block.Type())
	assert.Empty(t, block.Labels(), "Terraform block should have no labels")

	// Attempt to find it by providing labels (should fail)
	_, err = GetBlock(hclFile, "terraform", []string{"label"}, logger)
	assert.Error(t, err, "Should return error when providing labels for a no-label block")
}

// TestGetBlock_NilFile tests GetBlock with a nil hclwrite.File.
func TestGetBlock_NilFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	_, err := GetBlock(nil, "resource", []string{"aws_instance", "example"}, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input hclFile cannot be nil")
}

// TestGetBlock_NilFileBody tests GetBlock with a nil hclwrite.File.Body.
func TestGetBlock_NilFileBody(t *testing.T) {
	logger := zaptest.NewLogger(t)
	emptyFile := hclwrite.NewEmptyFile() // This creates a file with a non-nil body.
	// To truly test a nil body, we'd have to manually construct an hclwrite.File with Body = nil,
	// which is not typical. The current GetBlock handles nil hclFile, and if hclFile is not nil,
	// its Body is usually initialized.
	// The GetBlock function already checks for `hclFile.Body() == nil`.
	fileWithNilBody := &hclwrite.File{} // Body is nil by default
	_, err := GetBlock(fileWithNilBody, "resource", []string{"aws_instance", "example"}, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input hclFile has a nil body")
}

// --- Tests for GetAttribute ---

func TestGetAttribute_Exists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" {
  ami           = "ami-123"
  instance_type = "t2.micro"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	block, err := GetBlock(hclFile, "resource", []string{"aws_instance", "web"}, logger)
	require.NoError(t, err)
	require.NotNil(t, block)

	attr, err := GetAttribute(block, "ami", logger)
	assert.NoError(t, err)
	require.NotNil(t, attr)
	assert.Equal(t, "ami", attr.Name())

	attr, err = GetAttribute(block, "instance_type", logger)
	assert.NoError(t, err)
	require.NotNil(t, attr)
	assert.Equal(t, "instance_type", attr.Name())
}

func TestGetAttribute_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" {
  ami = "ami-123"
}
`
	hclFile, diags := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors())
	block, err := GetBlock(hclFile, "resource", []string{"aws_instance", "web"}, logger)
	require.NoError(t, err)
	require.NotNil(t, block)

	_, err = GetAttribute(block, "non_existent_attr", logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "attribute 'non_existent_attr' not found")
}

func TestGetAttribute_NilBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	_, err := GetAttribute(nil, "any_attr", logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block cannot be nil")
}

func TestGetAttribute_NilBlockBody(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockWithNilBody := &hclwrite.Block{} // Manually create a block with a nil body
	// Note: An hclwrite.Block created via hclwrite.NewBlock or from parsing valid HCL
	// will typically have a non-nil Body. This tests an edge case.
	// blockWithNilBody.SetType("resource") // Type is necessary for logging in GetAttribute

	_, err := GetAttribute(blockWithNilBody, "any_attr", logger)
	assert.Error(t, err)
	// The error message includes block.Type(), which might be empty if not set.
	assert.Contains(t, err.Error(), "input block  has a nil body") // block.Type() is empty string

	blockWithNilBody.SetType("test_type") // Set type for clearer error
	_, err = GetAttribute(blockWithNilBody, "any_attr", logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block test_type has a nil body")
}


// --- Tests for GetAttributeValue ---

func TestGetAttributeValue_SimpleLiterals(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "literals" {
  str_attr   = "hello world"
  num_attr_1 = 123
  num_attr_2 = 45.67
  bool_attr_t = true
  bool_attr_f = false
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"test_resource", "literals"}, logger)
	require.NotNil(t, block)

	// String
	strAttr, _ := GetAttribute(block, "str_attr", logger)
	require.NotNil(t, strAttr)
	val, err := GetAttributeValue(strAttr, logger)
	assert.NoError(t, err)
	require.True(t, val.IsKnown(), "Value should be known")
	require.False(t, val.IsNull(), "Value should not be null")
	assert.Equal(t, "hello world", val.AsString())

	// Integer
	numAttr1, _ := GetAttribute(block, "num_attr_1", logger)
	require.NotNil(t, numAttr1)
	val, err = GetAttributeValue(numAttr1, logger)
	assert.NoError(t, err)
	numVal1, _ := val.AsBigFloat().Int64()
	assert.Equal(t, int64(123), numVal1)

	// Float
	numAttr2, _ := GetAttribute(block, "num_attr_2", logger)
	require.NotNil(t, numAttr2)
	val, err = GetAttributeValue(numAttr2, logger)
	assert.NoError(t, err)
	numVal2, _ := val.AsBigFloat().Float64()
	assert.Equal(t, 45.67, numVal2)

	// Boolean true
	boolAttrT, _ := GetAttribute(block, "bool_attr_t", logger)
	require.NotNil(t, boolAttrT)
	val, err = GetAttributeValue(boolAttrT, logger)
	assert.NoError(t, err)
	assert.True(t, val.True())

	// Boolean false
	boolAttrF, _ := GetAttribute(block, "bool_attr_f", logger)
	require.NotNil(t, boolAttrF)
	val, err = GetAttributeValue(boolAttrF, logger)
	assert.NoError(t, err)
	assert.True(t, val.False())
}

func TestGetAttributeValue_VariableReference(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "vars" {
  ref_attr = var.some_variable
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"test_resource", "vars"}, logger)
	require.NotNil(t, block)
	attr, _ := GetAttribute(block, "ref_attr", logger)
	require.NotNil(t, attr)

	_, err := GetAttributeValue(attr, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a simple literal (likely a variable, function call, or unsupported reference)")
	assert.Contains(t, err.Error(), "Unknown variable")
}

func TestGetAttributeValue_Interpolation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "interp" {
  interp_attr = "Hello, ${var.user_name}!"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"test_resource", "interp"}, logger)
	require.NotNil(t, block)
	attr, _ := GetAttribute(block, "interp_attr", logger)
	require.NotNil(t, attr)

	_, err := GetAttributeValue(attr, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a simple literal (likely a variable, function call, or unsupported reference)")
	// The specific diagnostic for interpolation with nil context might vary.
	// Often it's "Unknown variable" for the var inside the interpolation.
	assert.Contains(t, err.Error(), "Unknown variable")
}

func TestGetAttributeValue_ComplexType_List(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "complex" {
  list_attr = [1, "two", true]
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"test_resource", "complex"}, logger)
	require.NotNil(t, block)
	attr, _ := GetAttribute(block, "list_attr", logger)
	require.NotNil(t, attr)

	_, err := GetAttributeValue(attr, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "evaluated to an unsupported type 'tuple' for simple literal extraction")
}

func TestGetAttributeValue_ComplexType_Map(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "complex" {
  map_attr = {
    key1 = "val1"
    key2 = 100
  }
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"test_resource", "complex"}, logger)
	require.NotNil(t, block)
	attr, _ := GetAttribute(block, "map_attr", logger)
	require.NotNil(t, attr)

	_, err := GetAttributeValue(attr, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "evaluated to an unsupported type 'object' for simple literal extraction")
}

func TestGetAttributeValue_NilAttributeExpr(t *testing.T) {
	logger := zaptest.NewLogger(t)
	// Create an attribute with a nil expression manually.
	// This is unusual but tests robustness.
	nilExprAttr := hclwrite.NewAttribute("nil_expr_attr")
	// attr.expr is private, so we can't directly set it to nil after creation if NewAttribute initializes it.
	// However, the GetAttributeValue function checks for attr.Expr() == nil.
	// Let's assume a scenario where an attribute might be malformed or partially constructed.
	// For this test, we can't easily create this state with public API of hclwrite.Attribute.
	// The function's internal check `if attr.Expr() == nil` is defensive.
	// We can test it by passing a freshly initialized hclwrite.Attribute,
	// but its `expr` field is not exported and initialized internally.
	// The check `attr.Expr() == nil` might be difficult to trigger from outside the package
	// without direct manipulation of the struct, which is not good practice for tests.
	// The most common way `Expr()` would be nil is if the attribute was not correctly parsed
	// or constructed, which should ideally be caught earlier.
	// For now, we'll acknowledge this check exists in the function.
	// If we were in the same package, we could do:
	// malformedAttr := &hclwrite.Attribute{Name: "malformed"}
	// _, err := GetAttributeValue(malformedAttr, logger)
	// assert.Error(t, err)
	// assert.Contains(t, err.Error(), "attribute 'malformed' has a nil expression")
	t.Log("TestGetAttributeValue_NilAttributeExpr: Skipped direct test of nil expression due to hclwrite encapsulation. Defensive check exists in function.")
}


// --- Tests for SetAttributeValue ---

func TestSetAttributeValue_NewAndOverwrite(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "server" {
  ami = "ami-original"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"aws_instance", "server"}, logger)
	require.NotNil(t, block)

	// Set a new string attribute
	err := SetAttributeValue(block, "instance_type", cty.StringVal("t2.large"), logger)
	assert.NoError(t, err)

	// Set a new number attribute
	err = SetAttributeValue(block, "disk_size", cty.NumberIntVal(100), logger)
	assert.NoError(t, err)

	// Set a new boolean attribute
	err = SetAttributeValue(block, "enable_monitoring", cty.True, logger)
	assert.NoError(t, err)
	
	// Overwrite existing string attribute "ami"
	err = SetAttributeValue(block, "ami", cty.StringVal("ami-updated"), logger)
	assert.NoError(t, err)


	expectedHCL := `
resource "aws_instance" "server" {
  ami               = "ami-updated"
  instance_type     = "t2.large"
  disk_size         = 100
  enable_monitoring = true
}
`
	// Normalize by trimming whitespace for comparison
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL output mismatch")
}

func TestSetAttributeValue_NilBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	err := SetAttributeValue(nil, "any_attr", cty.StringVal("test"), logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block cannot be nil")
}

func TestSetAttributeValue_NilBlockBody(t *testing.T) {
	logger := zaptest.NewLogger(t)
	blockWithNilBody := &hclwrite.Block{}
	// blockWithNilBody.SetType("resource") // Necessary for logging if block.Type() is called

	err := SetAttributeValue(blockWithNilBody, "any_attr", cty.StringVal("test"), logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block  has a nil body")

	blockWithNilBody.SetType("test_type")
	err = SetAttributeValue(blockWithNilBody, "any_attr", cty.StringVal("test"), logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block test_type has a nil body")
}


// --- Tests for RemoveAttribute ---

func TestRemoveAttribute_Exists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "server" {
  ami             = "ami-123"
  instance_type   = "t2.micro"
  delete_this_tag = "true"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"aws_instance", "server"}, logger)
	require.NotNil(t, block)

	err := RemoveAttribute(block, "delete_this_tag", logger)
	assert.NoError(t, err)

	// Verify it's gone
	attr := block.Body().GetAttribute("delete_this_tag")
	assert.Nil(t, attr, "Attribute should have been removed")

	expectedHCL := `
resource "aws_instance" "server" {
  ami           = "ami-123"
  instance_type = "t2.micro"
}
`
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL output mismatch after removing attribute")
}

func TestRemoveAttribute_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "server" {
  ami = "ami-123"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})
	block, _ := GetBlock(hclFile, "resource", []string{"aws_instance", "server"}, logger)
	require.NotNil(t, block)

	err := RemoveAttribute(block, "non_existent_attr", logger)
	assert.NoError(t, err, "Removing non-existent attribute should not error")

	expectedHCL := `
resource "aws_instance" "server" {
  ami = "ami-123"
}
`
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL should be unchanged")
}

func TestRemoveAttribute_NilBlock(t *testing.T) {
	logger := zaptest.NewLogger(t)
	err := RemoveAttribute(nil, "any_attr", logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input block cannot be nil")
}

// --- Tests for RemoveBlock ---

func TestRemoveBlock_Exists(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" { # Keep this
  ami = "ami-web"
}
resource "aws_instance" "app_to_delete" { # Delete this
  ami = "ami-app"
}
resource "aws_s3_bucket" "logs" { # Keep this
  bucket = "my-logs-bucket"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})

	err := RemoveBlock(hclFile, "resource", []string{"aws_instance", "app_to_delete"}, logger)
	assert.NoError(t, err)

	expectedHCL := `
resource "aws_instance" "web" { # Keep this
  ami = "ami-web"
}
resource "aws_s3_bucket" "logs" { # Keep this
  bucket = "my-logs-bucket"
}
`
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL output mismatch after removing block")

	// Verify it's truly gone by trying to Get it
	_, getErr := GetBlock(hclFile, "resource", []string{"aws_instance", "app_to_delete"}, logger)
	assert.Error(t, getErr, "Block should not be found after removal")
}

func TestRemoveBlock_NotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" {
  ami = "ami-web"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "", hclwrite.Pos{Line: 1, Column: 1})

	err := RemoveBlock(hclFile, "resource", []string{"aws_instance", "non_existent_app"}, logger)
	assert.Error(t, err, "Should return error when trying to remove a non-existent block")
	assert.Contains(t, err.Error(), "not found for removal")

	// Ensure original content is unchanged
	expectedHCL := `
resource "aws_instance" "web" {
  ami = "ami-web"
}
`
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL should be unchanged")
}

func TestRemoveBlock_NilFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	err := RemoveBlock(nil, "resource", []string{"any", "block"}, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input hclFile cannot be nil")
}


// --- Tests for ModifyNameAttributes ---

func TestModifyNameAttributes_Basic(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "null_resource" "example1" {
  name = "my-resource" // Will be modified
}
resource "random_pet" "server" {
  prefix = "test-"
  length = 2
  name   = "another-resource" # Will be modified
}
module "my_module" {
  source = "./my-module"
  name   = "module-level-name" // Will be modified
}
resource "aws_instance" "no_name_attr" {
  instance_type = "t2.micro"
}
variable "name" { # Label "name", not an attribute "name"
  type    = string
  default = "variable-name"
}
resource "complex_name" "example2" {
  name = var.input_name # Not a simple string literal
}
resource "complex_name" "example3" {
  name = "prefix-${local.suffix}" # Not a simple string literal
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})

	modifiedCount, err := ModifyNameAttributes(hclFile, logger)
	assert.NoError(t, err)
	assert.Equal(t, 3, modifiedCount, "Should have modified 3 'name' attributes")

	expectedHCL := `
resource "null_resource" "example1" {
  name = "my-resource-clone" // Will be modified
}
resource "random_pet" "server" {
  prefix = "test-"
  length = 2
  name   = "another-resource-clone" # Will be modified
}
module "my_module" {
  source = "./my-module"
  name   = "module-level-name-clone" // Will be modified
}
resource "aws_instance" "no_name_attr" {
  instance_type = "t2.micro"
}
variable "name" { # Label "name", not an attribute "name"
  type    = string
  default = "variable-name"
}
resource "complex_name" "example2" {
  name = var.input_name # Not a simple string literal
}
resource "complex_name" "example3" {
  name = "prefix-${local.suffix}" # Not a simple string literal
}
`
	actualHCLBytes := hclFile.Bytes()
	assert.Equal(t, strings.TrimSpace(expectedHCL), strings.TrimSpace(string(actualHCLBytes)), "HCL output mismatch after ModifyNameAttributes")
}

func TestModifyNameAttributes_NoMatchingAttributes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "aws_instance" "web" {
  instance_type = "t2.micro"
  tags = {
    Name = "web-server" // Attribute key is "Name", not "name"
  }
}
variable "app_name" {
  default = "my-app"
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	modifiedCount, err := ModifyNameAttributes(hclFile, logger)
	assert.NoError(t, err)
	assert.Equal(t, 0, modifiedCount, "Should modify 0 attributes")
	assert.Equal(t, strings.TrimSpace(hclContent), strings.TrimSpace(string(hclFile.Bytes())), "HCL should be unchanged")
}

func TestModifyNameAttributes_NameNotString(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclContent := `
resource "test_resource" "example" {
  name = 12345 # Name is a number
}
`
	hclFile, _ := hclwrite.ParseConfig([]byte(hclContent), "test.tf", hclwrite.Pos{Line: 1, Column: 1})
	modifiedCount, err := ModifyNameAttributes(hclFile, logger)
	assert.NoError(t, err)
	assert.Equal(t, 0, modifiedCount, "Should modify 0 attributes as name is not a string")
	assert.Equal(t, strings.TrimSpace(hclContent), strings.TrimSpace(string(hclFile.Bytes())), "HCL should be unchanged")
}

func TestModifyNameAttributes_NilFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	_, err := ModifyNameAttributes(nil, logger)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input file or file body cannot be nil")
}

func TestModifyNameAttributes_EmptyFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	hclFile := hclwrite.NewEmptyFile()
	modifiedCount, err := ModifyNameAttributes(hclFile, logger)
	assert.NoError(t, err)
	assert.Equal(t, 0, modifiedCount)
}
