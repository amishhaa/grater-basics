package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// ModuleStatus is what results.json contains (written by run.go)
type ModuleStatus struct {
	Module string `json:"module"`
	Status string `json:"status"` // PASS, BROKEN, REGRESSION, FIXED, SKIPPED, ERROR
}

type ReportSummary struct {
	TotalModules int            `json:"total_modules"`
	BaseRef      string         `json:"base_ref"`
	HeadRef      string         `json:"head_ref"`
	Status       string         `json:"status"` // SAFE, UNSAFE, INCONCLUSIVE
	Regressions  []ModuleStatus `json:"regressions,omitempty"`
	Fixed        []ModuleStatus `json:"fixed,omitempty"`
	Broken       []ModuleStatus `json:"broken,omitempty"`
	Skipped      []ModuleStatus `json:"skipped,omitempty"`
	Passed       []ModuleStatus `json:"passed,omitempty"`
	Errors       []ModuleStatus `json:"errors,omitempty"`
}

var (
	outputFormat string
	verbose      bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Analyze test results and report regressions",
	Long: `Analyze the results from the 'run' command and generate a report.

Examples:
  grater report                    # Show summary report
  grater report --format json      # Output as JSON
  grater report --verbose          # Show detailed output`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}

		graterDir := filepath.Join(projectRoot, ".grater")
		resultsFile := filepath.Join(graterDir, "results.json")

		data, err := os.ReadFile(resultsFile)
		if err != nil {
			return fmt.Errorf("no results found. Run 'grater run' first: %w", err)
		}

		var results []ModuleStatus
		if err := json.Unmarshal(data, &results); err != nil {
			return fmt.Errorf("failed to parse results.json: %w", err)
		}

		// base and head come from the run command flags
		report := analyzeResults(results, base, head)
		return outputReport(report)
	},
}

func analyzeResults(results []ModuleStatus, baseRef, headRef string) ReportSummary {
	summary := ReportSummary{
		TotalModules: len(results),
		BaseRef:      baseRef,
		HeadRef:      headRef,
		Regressions:  []ModuleStatus{},
		Fixed:        []ModuleStatus{},
		Broken:       []ModuleStatus{},
		Skipped:      []ModuleStatus{},
		Passed:       []ModuleStatus{},
		Errors:       []ModuleStatus{},
	}

	if len(results) == 0 {
		summary.Status = "INCONCLUSIVE"
		return summary
	}

	for _, r := range results {
		switch r.Status {
		case "PASS":
			summary.Passed = append(summary.Passed, r)
		case "REGRESSION":
			summary.Regressions = append(summary.Regressions, r)
		case "FIXED":
			summary.Fixed = append(summary.Fixed, r)
		case "BROKEN":
			summary.Broken = append(summary.Broken, r)
		case "SKIPPED":
			summary.Skipped = append(summary.Skipped, r)
		case "ERROR":
			summary.Errors = append(summary.Errors, r)
		}
	}

	if len(summary.Regressions) > 0 {
		summary.Status = "UNSAFE"
	} else if len(summary.Errors) > 0 || len(summary.Skipped) > 0 {
		summary.Status = "INCONCLUSIVE"
	} else if len(summary.Broken) > 0 && len(summary.Passed) == 0 && len(summary.Fixed) == 0 {
		summary.Status = "INCONCLUSIVE"
	} else {
		summary.Status = "SAFE"
	}

	return summary
}

func outputReport(summary ReportSummary) error {
	switch outputFormat {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	default:
		outputHuman(summary)
		return nil
	}
}

func outputHuman(summary ReportSummary) {
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("📊 GRATER TEST REPORT")
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Base ref:       %s\n", summary.BaseRef)
	fmt.Printf("Head ref:       %s\n", summary.HeadRef)
	fmt.Printf("Modules tested: %d\n", summary.TotalModules)
	fmt.Println()

	fmt.Print("Overall Status: ")
	switch summary.Status {
	case "SAFE":
		fmt.Println("✅ SAFE — no regressions detected")
	case "UNSAFE":
		fmt.Println("❌ UNSAFE — regressions found!")
	case "INCONCLUSIVE":
		fmt.Println("⚠️  INCONCLUSIVE — some tests errored or were skipped")
	}
	fmt.Println()

	if len(summary.Regressions) > 0 {
		fmt.Printf("🔴 REGRESSIONS (%d) — base passed, head failed:\n", len(summary.Regressions))
		for _, r := range summary.Regressions {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	if len(summary.Fixed) > 0 {
		fmt.Printf("🟢 FIXED (%d) — base failed, head passed:\n", len(summary.Fixed))
		for _, r := range summary.Fixed {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	if len(summary.Broken) > 0 {
		fmt.Printf("🔧 BROKEN (%d) — both refs fail:\n", len(summary.Broken))
		for _, r := range summary.Broken {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	if len(summary.Skipped) > 0 {
		fmt.Printf("⏸️  SKIPPED (%d) — timed out:\n", len(summary.Skipped))
		for _, r := range summary.Skipped {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	if len(summary.Errors) > 0 {
		fmt.Printf("⚠️  ERRORS (%d) — container or execution failed:\n", len(summary.Errors))
		for _, r := range summary.Errors {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	if verbose && len(summary.Passed) > 0 {
		fmt.Printf("✅ PASSING (%d):\n", len(summary.Passed))
		for _, r := range summary.Passed {
			fmt.Printf("   • %s\n", r.Module)
		}
		fmt.Println()
	}

	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("✅ %d passed  🔴 %d regressions  🟢 %d fixed  🔧 %d broken  ⏸️  %d skipped  ⚠️  %d errors\n",
		len(summary.Passed),
		len(summary.Regressions),
		len(summary.Fixed),
		len(summary.Broken),
		len(summary.Skipped),
		len(summary.Errors),
	)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")

	if summary.Status == "UNSAFE" {
		fmt.Println("\n❌ REGRESSIONS DETECTED — head ref is not safe to merge")
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(reportCmd)

	reportCmd.Flags().StringVar(&outputFormat, "format", "simple", "Output format: simple or json")
	reportCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show passing modules too")
}