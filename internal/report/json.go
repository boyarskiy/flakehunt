package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// WriteJSON writes the report as JSON to the specified output directory.
// The file is written to <outDir>/report.json
func WriteJSON(outDir string, report *model.Report) error {
	if report == nil {
		return fmt.Errorf("report is required")
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outDir, err)
	}

	path := filepath.Join(outDir, "report.json")

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write report to %s: %w", path, err)
	}

	return nil
}

// MarshalJSON returns the report as a JSON byte slice.
func MarshalJSON(report *model.Report) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("report is required")
	}
	return json.MarshalIndent(report, "", "  ")
}
