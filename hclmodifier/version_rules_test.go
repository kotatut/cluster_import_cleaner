package hclmodifier

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

func TestApplyNodeVersionRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                     string
		hclContentFile           string
		expectedModifications    int
		expectNodeVersionRemoved bool
		resourceLabelsToVerify   []string
	}{
		{
			name:                     "Node version present without min_master_version",
			hclContentFile:           "testdata/TestApplyNodeVersionRule_NodeVersionPresent.tf",
			expectedModifications:    1,
			expectNodeVersionRemoved: false,
			resourceLabelsToVerify:   []string{"google_container_cluster", "primary"},
		},
		{
			name:                     "Node version and min_master_version present",
			hclContentFile:           "testdata/TestApplyNodeVersionRule_NodeVersionAndMinMasterVersionPresent.tf",
			expectedModifications:    0,
			expectNodeVersionRemoved: true,
			resourceLabelsToVerify:   []string{"google_container_cluster", "primary"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hclContent, err := os.ReadFile(tc.hclContentFile)
			if err != nil {
				t.Fatalf("Failed to read test file %s: %v", tc.hclContentFile, err)
			}

			tempDir := t.TempDir()
			tmpFile, err := os.CreateTemp(tempDir, "test_*.hcl")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}

			if _, err := tmpFile.Write(hclContent); err != nil {
				tmpFile.Close()
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			if err := tmpFile.Close(); err != nil {
				t.Fatalf("Failed to close temp file: %v", err)
			}

			modifier, err := NewFromFile(tmpFile.Name(), logger)
			if err != nil {
				t.Fatalf("NewFromFile() error = %v for HCL: \n%s", err, hclContent)
			}

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.SetMinVersionRule})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(NodeVersionRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)
		})
	}
}
