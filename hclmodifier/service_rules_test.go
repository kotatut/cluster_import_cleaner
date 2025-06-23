package hclmodifier

import (
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/kotatut/cluster_import_cleaner/hclmodifier/rules"
	"github.com/kotatut/cluster_import_cleaner/hclmodifier/types"
)

func TestApplyLoggingServiceRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                        string
		hclContentFile              string
		expectedModifications       int
		expectLoggingServiceRemoved bool
		resourceLabelsToVerify      []string
	}{
		{
			name:                        "Logging service present without telemetry",
			hclContentFile:              "testdata/TestApplyLoggingServiceRule_LoggingServicePresent.tf",
			expectedModifications:       0,
			expectLoggingServiceRemoved: false,
			resourceLabelsToVerify:      []string{"google_container_cluster", "primary"},
		},
		{
			name:                        "Logging service and telemetry present",
			hclContentFile:              "testdata/TestApplyLoggingServiceRule_LoggingServiceAndTelemetryPresent.tf",
			expectedModifications:       1,
			expectLoggingServiceRemoved: true,
			resourceLabelsToVerify:      []string{"google_container_cluster", "primary"},
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

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.RuleRemoveLoggingService})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(LoggingServiceRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetBlock *hclwrite.Block

				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetBlock = b
						break
					}
				}

				if targetBlock != nil {
					hasLoggingService := targetBlock.Body().GetAttribute("logging_service") != nil
					if tc.expectLoggingServiceRemoved {
						assert.False(t, hasLoggingService, "Expected 'logging_service' to be removed")
					} else {
						assert.True(t, hasLoggingService, "Expected 'logging_service' to be present")
					}
				}
			}
		})
	}
}

func TestApplyMonitoringServiceRule(t *testing.T) {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	tests := []struct {
		name                           string
		hclContentFile                 string
		expectedModifications          int
		expectMonitoringServiceRemoved bool
		resourceLabelsToVerify         []string
	}{
		{
			name:                           "Monitoring service present without monitoring_config",
			hclContentFile:                 "testdata/TestApplyMonitoringServiceRule_MonitoringServicePresent.tf",
			expectedModifications:          0,
			expectMonitoringServiceRemoved: false,
			resourceLabelsToVerify:         []string{"google_container_cluster", "primary"},
		},
		{
			name:                           "Monitoring service and monitoring_config present",
			hclContentFile:                 "testdata/TestApplyMonitoringServiceRule_MonitoringServiceAndConfigPresent.tf",
			expectedModifications:          1,
			expectMonitoringServiceRemoved: true,
			resourceLabelsToVerify:         []string{"google_container_cluster", "primary"},
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

			modifications, errs := modifier.ApplyRules([]types.Rule{rules.RuleRemoveMonitoringService})
			if len(errs) > 0 {
				var errorMessages string
				for _, e := range errs {
					errorMessages += e.Error() + "\n"
				}
				t.Fatalf("ApplyRules(MonitoringServiceRuleDefinition) returned errors = %v for HCL: \n%s. Errors:\n%s", errs, hclContent, errorMessages)
			}

			assert.Equal(t, tc.expectedModifications, modifications)

			modifiedContentBytes := modifier.File().Bytes()
			verifiedFile, diags := hclwrite.ParseConfig(modifiedContentBytes, tmpFile.Name()+"_verified", hcl.InitialPos)
			if diags.HasErrors() {
				t.Fatalf("Failed to parse modified HCL content for verification: %v\nModified HCL:\n%s", diags, string(modifiedContentBytes))
			}

			if len(tc.resourceLabelsToVerify) == 2 {
				blockType := tc.resourceLabelsToVerify[0]
				blockName := tc.resourceLabelsToVerify[1]
				var targetBlock *hclwrite.Block

				for _, b := range verifiedFile.Body().Blocks() {
					if b.Type() == "resource" && len(b.Labels()) == 2 && b.Labels()[0] == blockType && b.Labels()[1] == blockName {
						targetBlock = b
						break
					}
				}

				if targetBlock != nil {
					hasMonitoringService := targetBlock.Body().GetAttribute("monitoring_service") != nil
					if tc.expectMonitoringServiceRemoved {
						assert.False(t, hasMonitoringService, "Expected 'monitoring_service' to be removed")
					} else {
						assert.True(t, hasMonitoringService, "Expected 'monitoring_service' to be present")
					}
				}
			}
		})
	}
}
