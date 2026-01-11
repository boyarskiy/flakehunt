// Package cypress implements the Cypress adapter for flakehunt.
package cypress

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/boyarskiy/flakehunt/internal/model"
)

// JUnitTestSuites represents the root element of JUnit XML.
type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a test suite in JUnit XML.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents a single test case in JUnit XML.
type JUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure,omitempty"`
	Error     *JUnitError   `xml:"error,omitempty"`
	Skipped   *JUnitSkipped `xml:"skipped,omitempty"`
}

// JUnitFailure represents a test failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitError represents a test error.
type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// Adapter implements the model.Adapter interface for Cypress.
type Adapter struct{}

// New creates a new Cypress adapter.
func New() *Adapter {
	return &Adapter{}
}

// BuildCommand returns the command arguments with JUnit XML reporter configured.
// The reporter writes XML files to the run directory.
func (a *Adapter) BuildCommand(runDir string, userCmd []string) []string {
	if len(userCmd) == 0 {
		return nil
	}

	// Copy user command to avoid mutation
	result := make([]string, len(userCmd))
	copy(result, userCmd)

	// Configure JUnit reporter to write to runDir
	// Cypress uses mocha-junit-reporter, configured via reporter options
	reporterOpts := fmt.Sprintf("mochaFile=%s/[hash].xml", runDir)

	// Add reporter configuration
	result = append(result, "--reporter", "junit")
	result = append(result, "--reporter-options", reporterOpts)

	return result
}

// Parse reads all XML files from runDir and returns aggregated test results.
// If the same test appears multiple times (retries), the last occurrence wins.
func (a *Adapter) Parse(runDir string) (*model.RunResult, error) {
	// Find all XML files in runDir
	xmlFiles, err := filepath.Glob(filepath.Join(runDir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob XML files in %s: %w", runDir, err)
	}

	if len(xmlFiles) == 0 {
		return nil, fmt.Errorf("no XML files found in %s: ensure Cypress is configured with JUnit reporter", runDir)
	}

	// Sort files for deterministic processing order
	sort.Strings(xmlFiles)

	// Map to track test results, using TestID as key
	// Last occurrence wins for handling retries
	testMap := make(map[string]model.TestResult)

	for _, xmlFile := range xmlFiles {
		testCases, err := parseXMLFile(xmlFile)
		if err != nil {
			return nil, err
		}

		for _, tc := range testCases {
			testID := buildTestID(tc.Classname, tc.Name)
			if testID == "" {
				return nil, fmt.Errorf("invalid test case in %s: missing classname or name attribute", xmlFile)
			}

			result := model.TestResult{
				TestID:   testID,
				Outcome:  determineOutcome(tc),
				Duration: time.Duration(tc.Time * float64(time.Second)),
			}

			// Extract failure message if present
			if tc.Failure != nil {
				result.FailureMessage = extractFailureMessage(tc.Failure.Message, tc.Failure.Content)
			} else if tc.Error != nil {
				result.FailureMessage = extractFailureMessage(tc.Error.Message, tc.Error.Content)
			}

			// Last occurrence wins (handles retries)
			testMap[testID] = result
		}
	}

	// Build results in deterministic order (sorted by TestID)
	testIDs := make([]string, 0, len(testMap))
	for id := range testMap {
		testIDs = append(testIDs, id)
	}
	sort.Strings(testIDs)

	tests := make([]model.TestResult, 0, len(testIDs))
	for _, id := range testIDs {
		tests = append(tests, testMap[id])
	}

	return &model.RunResult{
		Tests: tests,
	}, nil
}

// ExpectedArtifact returns the run directory path.
// Cypress produces multiple XML files, so we verify the directory exists
// and contains at least one XML file during parsing.
func (a *Adapter) ExpectedArtifact(runDir string) string {
	return runDir
}

// parseXMLFile parses a single JUnit XML file and returns all test cases.
func parseXMLFile(xmlPath string) ([]JUnitTestCase, error) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read XML file %s: %w", xmlPath, err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("XML file %s is empty", xmlPath)
	}

	// Try parsing as testsuites (multiple suites)
	var testSuites JUnitTestSuites
	if err := xml.Unmarshal(data, &testSuites); err == nil && len(testSuites.TestSuites) > 0 {
		var allCases []JUnitTestCase
		for _, suite := range testSuites.TestSuites {
			allCases = append(allCases, suite.TestCases...)
		}
		return allCases, nil
	}

	// Try parsing as single testsuite
	var singleSuite JUnitTestSuite
	if err := xml.Unmarshal(data, &singleSuite); err == nil && len(singleSuite.TestCases) > 0 {
		return singleSuite.TestCases, nil
	}

	return nil, fmt.Errorf("failed to parse XML file %s: invalid JUnit format or no test cases found", xmlPath)
}

// buildTestID constructs the TestID from classname and test name.
// Format: <specPath>::<fullTestName>
func buildTestID(classname, name string) string {
	classname = strings.TrimSpace(classname)
	name = strings.TrimSpace(name)

	if classname == "" || name == "" {
		return ""
	}

	return classname + "::" + name
}

// determineOutcome determines the test outcome from a JUnit test case.
func determineOutcome(tc JUnitTestCase) model.Outcome {
	if tc.Skipped != nil {
		return model.OutcomeSkip
	}
	if tc.Failure != nil || tc.Error != nil {
		return model.OutcomeFail
	}
	return model.OutcomePass
}

// extractFailureMessage extracts a clean failure message from JUnit failure/error.
func extractFailureMessage(message, content string) string {
	// Prefer message attribute, fall back to content
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = strings.TrimSpace(content)
	}

	// Truncate very long messages
	const maxLen = 500
	if len(msg) > maxLen {
		msg = msg[:maxLen] + "..."
	}

	return msg
}
