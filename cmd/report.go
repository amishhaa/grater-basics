package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Result represents the test result for a single module
type ModuleResult struct {
	Module string `json:"module"`
	Base   struct {
		Ref     string `json:"ref"`
		Passed  bool   `json:"passed"`
		Error   string `json:"error"`
		Skipped bool   `json:"skipped"`
	} `json:"base"`
	Head struct {
		Ref     string `json:"ref"`
		Passed  bool   `json:"passed"`
		Error   string `json:"error"`
		Skipped bool   `json:"skipped"`
	} `json:"head"`
}

// Summary of all results
type ReportSummary struct {
	TotalModules int            `json:"total_modules"`
	BaseRef      string         `json:"base_ref"`
	HeadRef      string         `json:"head_ref"`
	Status       string         `json:"status"` // SAFE, UNSAFE, INCONCLUSIVE
	Regressions  []ModuleResult `json:"regressions,omitempty"`
	Fixed        []ModuleResult `json:"fixed,omitempty"`
	Broken       []ModuleResult `json:"broken,omitempty"`
	Skipped      []ModuleResult `json:"skipped,omitempty"`
	Passed       []ModuleResult `json:"passed,omitempty"`
	Errors       []ModuleResult `json:"errors,omitempty"`
}

var (
	outputFormat string
	verbose      bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Analyze test results and report regressions",
	Long: `Analyze the results from the 'run' command and generate a report.
Identifies regressions, fixed issues, and provides a summary of the test run.

Examples:
  grater report                    # Show summary report
  grater report --format json      # Output as JSON
  grater report --verbose          # Show detailed error messages`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get project root
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}

		graterDir := filepath.Join(projectRoot, ".grater")
		resultsFile := filepath.Join(graterDir, "results.json")

		// Read results file
		data, err := os.ReadFile(resultsFile)
		if err != nil {
			return fmt.Errorf("no results found. Run 'grater run' first: %w", err)
		}

		// Parse results
		var results []ModuleResult
		if err := json.Unmarshal(data, &results); err != nil {
			// Try parsing as single result (backward compatibility)
			var singleResult ModuleResult
			if err2 := json.Unmarshal(data, &singleResult); err2 != nil {
				return fmt.Errorf("failed to parse results.json: %w", err)
			}
			results = []ModuleResult{singleResult}
		}

		// Generate report
		report := analyzeResults(results)

		// Output report
		return outputReport(report)
	},
}

func analyzeResults(results []ModuleResult) ReportSummary {
	summary := ReportSummary{
		TotalModules: len(results),
		Regressions:  []ModuleResult{},
		Fixed:        []ModuleResult{},
		Broken:       []ModuleResult{},
		Skipped:      []ModuleResult{},
		Passed:       []ModuleResult{},
		Errors:       []ModuleResult{},
	}

	if len(results) == 0 {
		summary.Status = "INCONCLUSIVE"
		return summary
	}

	// Set refs from first result
	summary.BaseRef = results[0].Base.Ref
	summary.HeadRef = results[0].Head.Ref

	// Categorize each module
	for _, r := range results {
		// Check for skips first
		if r.Base.Skipped || r.Head.Skipped {
			summary.Skipped = append(summary.Skipped, r)
			continue
		}

		// Check for errors (failed with error message)
		if (!r.Base.Passed && r.Base.Error != "") || (!r.Head.Passed && r.Head.Error != "") {
			summary.Errors = append(summary.Errors, r)
			continue
		}

		switch {
		case r.Base.Passed && r.Head.Passed:
			summary.Passed = append(summary.Passed, r)

		case r.Base.Passed && !r.Head.Passed:
			summary.Regressions = append(summary.Regressions, r)

		case !r.Base.Passed && r.Head.Passed:
			summary.Fixed = append(summary.Fixed, r)

		case !r.Base.Passed && !r.Head.Passed:
			summary.Broken = append(summary.Broken, r)
		}
	}

	// Determine overall status
	if len(summary.Regressions) > 0 {
		summary.Status = "UNSAFE"
	} else if len(summary.Errors) > 0 {
		summary.Status = "INCONCLUSIVE"
	} else if len(summary.Broken) == summary.TotalModules {
		summary.Status = "BROKEN"
	} else {
		summary.Status = "SAFE"
	}

	return summary
}

func outputReport(summary ReportSummary) error {
	switch outputFormat {
	case "json":
		return outputJSON(summary)
	case "simple":
		fallthrough
	default:
		outputHuman(summary)
		return nil
	}
}

func outputJSON(summary ReportSummary) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(summary)
}

func outputHuman(summary ReportSummary) {
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("📊 GRATER TEST REPORT")
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Base ref:  %s\n", summary.BaseRef)
	fmt.Printf("Head ref:  %s\n", summary.HeadRef)
	fmt.Printf("Modules tested: %d\n", summary.TotalModules)
	fmt.Println()

	// Overall status with color/emoji
	fmt.Print("Overall Status: ")
	switch summary.Status {
	case "SAFE":
		fmt.Println("✅ SAFE - No regressions detected")
	case "UNSAFE":
		fmt.Println("❌ UNSAFE - Regressions found!")
	case "INCONCLUSIVE":
		fmt.Println("⚠️  INCONCLUSIVE - Some tests had errors")
	case "BROKEN":
		fmt.Println("🔧 BROKEN - All modules are failing")
	}
	fmt.Println()

	// Regressions (most important)
	if len(summary.Regressions) > 0 {
		fmt.Println("🔴 REGRESSIONS (base passed, head failed):")
		for _, r := range summary.Regressions {
			fmt.Printf("   • %s\n", r.Module)
			if verbose && r.Head.Error != "" {
				fmt.Printf("     Error: %s\n", r.Head.Error)
			}
		}
		fmt.Println()
	}

	// Fixed issues
	if len(summary.Fixed) > 0 {
		fmt.Println("🟢 FIXED (base failed, head passed):")
		for _, r := range summary.Fixed {
			fmt.Printf("   • %s\n", r.Module)
			if verbose && r.Base.Error != "" {
				fmt.Printf("     Base error: %s\n", r.Base.Error)
			}
		}
		fmt.Println()
	}

	// Broken modules
	if len(summary.Broken) > 0 {
		fmt.Println("🔧 STILL BROKEN (both refs fail):")
		for _, r := range summary.Broken {
			fmt.Printf("   • %s\n", r.Module)
			if verbose {
				if r.Base.Error != "" {
					fmt.Printf("     Base error: %s\n", r.Base.Error)
				}
				if r.Head.Error != "" {
					fmt.Printf("     Head error: %s\n", r.Head.Error)
				}
			}
		}
		fmt.Println()
	}

	// Skipped due to timeout
	if len(summary.Skipped) > 0 {
		fmt.Println("⏸️  SKIPPED (timeout):")
		for _, r := range summary.Skipped {
			fmt.Printf("   • %s\n", r.Module)
			if verbose {
				if r.Base.Skipped {
					fmt.Printf("     Base: %s\n", r.Base.Error)
				}
				if r.Head.Skipped {
					fmt.Printf("     Head: %s\n", r.Head.Error)
				}
			}
		}
		fmt.Println()
	}

	// Errors
	if len(summary.Errors) > 0 {
		fmt.Println("⚠️  ERRORS (test execution failed):")
		for _, r := range summary.Errors {
			fmt.Printf("   • %s\n", r.Module)
			if verbose {
				if !r.Base.Passed && r.Base.Error != "" {
					fmt.Printf("     Base: %s\n", r.Base.Error)
				}
				if !r.Head.Passed && r.Head.Error != "" {
					fmt.Printf("     Head: %s\n", r.Head.Error)
				}
			}
		}
		fmt.Println()
	}

	// Passing modules (optional, only in verbose)
	if verbose && len(summary.Passed) > 0 {
		fmt.Println("✅ PASSING (both refs work):")
		for _, r := range summary.Passed {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	// Summary counts
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Summary: %d total | ✅ %d passed | 🔴 %d regressions | 🟢 %d fixed | 🔧 %d broken | ⏸️  %d skipped | ⚠️  %d errors\n",
		summary.TotalModules,
		len(summary.Passed),
		len(summary.Regressions),
		len(summary.Fixed),
		len(summary.Broken),
		len(summary.Skipped),
		len(summary.Errors),
	)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")

	// Exit with non-zero status if unsafe
	if summary.Status == "UNSAFE" {
		fmt.Println("\n❌ REGRESSIONS DETECTED - Check the report above")
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(reportCmd)

	reportCmd.Flags().StringVar(&outputFormat, "format", "simple", "Output format (simple or json)")
	reportCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed error messages")
}
