package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// DualResult matches the detailed structure from runner.sh
type DualResult struct {
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


var (
	repo  string
	base  string
	head  string
	image string
)

func writeResults(resultsFile, detailedFile string, allResults []ModuleStatus, detailedResults []DualResult) error {
	simpleOut, err := json.MarshalIndent(allResults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	if err := os.WriteFile(resultsFile, simpleOut, 0644); err != nil {
		return fmt.Errorf("failed to write results.json: %w", err)
	}

	detailedOut, err := json.MarshalIndent(detailedResults, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal detailed results: %w", err)
	}
	if err := os.WriteFile(detailedFile, detailedOut, 0644); err != nil {
		return fmt.Errorf("failed to write detailed_results.json: %w", err)
	}

	return nil
}

func printSummary(allResults []ModuleStatus) {
	fmt.Println("\n========================================")
	fmt.Println("📊 TEST SUMMARY")
	fmt.Println("========================================")
	for _, result := range allResults {
		symbol := map[string]string{
			"PASS":       "✅",
			"BROKEN":     "💔",
			"REGRESSION": "📉",
			"FIXED":      "🔧",
			"SKIPPED":    "⏰",
			"ERROR":      "❌",
		}[result.Status]
		fmt.Printf("%s %s: %s\n", symbol, result.Module, result.Status)
	}
	fmt.Println("========================================")
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run downstream tests on base and head and detect regressions",
	RunE: func(cmd *cobra.Command, args []string) error {

		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}

		graterDir := filepath.Join(projectRoot, ".grater")
		modulesFile := filepath.Join(graterDir, "modules.txt")
		resultsFile := filepath.Join(graterDir, "results.json")
		detailedFile := filepath.Join(graterDir, "detailed_results.json")

		if err := os.MkdirAll(graterDir, 0755); err != nil {
			return fmt.Errorf("failed to create .grater directory: %w", err)
		}

		if _, err := os.Stat(modulesFile); os.IsNotExist(err) {
			return fmt.Errorf("modules.txt not found. Run 'grater prepare' first: %w", err)
		}

		dockerfilePath := filepath.Join(projectRoot, "docker", "dockerfile")
		dockerContext := filepath.Join(projectRoot, "docker")

		if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
			return fmt.Errorf("dockerfile not found at %s", dockerfilePath)
		}

		data, err := os.ReadFile(modulesFile)
		if err != nil {
			return fmt.Errorf("failed to read modules.txt: %w", err)
		}

		rawModules := strings.Split(string(data), "\n")
		var modules []string
		for _, m := range rawModules {
			if trimmed := strings.TrimSpace(m); trimmed != "" {
				modules = append(modules, trimmed)
			}
		}

		if len(modules) == 0 {
			return fmt.Errorf("no modules found in modules.txt")
		}

		fmt.Println("Building docker image...")
		build := exec.Command(
			"docker", "build",
			"-t", image,
			"-f", dockerfilePath,
			dockerContext,
		)
		build.Stdout = os.Stdout
		build.Stderr = os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("docker build failed: %w", err)
		}

		var allResults []ModuleStatus
		var detailedResults []DualResult

		// Handle Ctrl+C: save whatever completed so far then exit
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\n\n⚠️  Interrupted — saving partial results...")
			if err := writeResults(resultsFile, detailedFile, allResults, detailedResults); err != nil {
				fmt.Printf("❌ Failed to save partial results: %v\n", err)
			} else {
				fmt.Printf("💾 Partial results saved to %s\n", graterDir)
				printSummary(allResults)
			}
			os.Exit(1)
		}()

		for i, m := range modules {
			fmt.Println("\n========================================")
			fmt.Printf("Testing module [%d/%d]: %s\n", i+1, len(modules), m)
			fmt.Println("========================================")

			dualResult, err := runDualContainer(image, m, repo, base, head)
			if err != nil {
				fmt.Printf("❌ Container error: %v\n", err)
				errorResult := DualResult{Module: m}
				errorResult.Base.Ref = base
				errorResult.Head.Ref = head
				errorResult.Base.Error = err.Error()
				errorResult.Head.Error = err.Error()
				errorResult.Base.Skipped = true
				errorResult.Head.Skipped = true
				allResults = append(allResults, ModuleStatus{Module: m, Status: "ERROR"})
				detailedResults = append(detailedResults, errorResult)
			} else {
				status := "PASS"
				if dualResult.Base.Skipped || dualResult.Head.Skipped {
					status = "SKIPPED"
				} else if dualResult.Base.Passed && !dualResult.Head.Passed {
					status = "REGRESSION"
				} else if !dualResult.Base.Passed && dualResult.Head.Passed {
					status = "FIXED"
				} else if !dualResult.Base.Passed && !dualResult.Head.Passed {
					status = "BROKEN"
				}

				fmt.Printf("\n📊 Results for %s:\n", m)
				fmt.Printf("   Base (%s): ", dualResult.Base.Ref)
				if dualResult.Base.Skipped {
					fmt.Printf("⏰ SKIPPED - %s\n", dualResult.Base.Error)
				} else if dualResult.Base.Passed {
					fmt.Printf("✅ PASS\n")
				} else {
					fmt.Printf("❌ FAIL - %s\n", dualResult.Base.Error)
				}

				fmt.Printf("   Head (%s): ", dualResult.Head.Ref)
				if dualResult.Head.Skipped {
					fmt.Printf("⏰ SKIPPED - %s\n", dualResult.Head.Error)
				} else if dualResult.Head.Passed {
					fmt.Printf("✅ PASS\n")
				} else {
					fmt.Printf("❌ FAIL - %s\n", dualResult.Head.Error)
				}
				fmt.Printf("   Status: %s\n", status)

				allResults = append(allResults, ModuleStatus{Module: m, Status: status})
				detailedResults = append(detailedResults, dualResult)
			}

			// Incremental save after every module regardless of pass/fail/error
			if err := writeResults(resultsFile, detailedFile, allResults, detailedResults); err != nil {
				fmt.Printf("⚠️  Failed to save progress: %v\n", err)
			} else {
				fmt.Printf("💾 Progress saved [%d/%d]\n", i+1, len(modules))
			}
		}

		signal.Stop(sigCh)

		fmt.Printf("\n✅ results.json saved to %s\n", resultsFile)
		fmt.Printf("✅ detailed_results.json saved to %s\n", detailedFile)
		printSummary(allResults)

		return nil
	},
}

func runDualContainer(image, module, repo, baseRef, headRef string) (DualResult, error) {
	cmd := exec.Command(
		"docker", "run", "--rm",
		"-e", "MODULE="+module,
		"-e", "REPO="+repo,
		"-e", "BASE_REF="+baseRef,
		"-e", "HEAD_REF="+headRef,
		image,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	// Stream container stderr so logs are visible in the terminal
	if stderr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "%s", stderr.String())
	}

	rawJSON := bytes.TrimSpace(stdout.Bytes())

	if len(rawJSON) == 0 {
		if runErr != nil {
			return DualResult{}, fmt.Errorf("container exited with error and produced no JSON: %v", runErr)
		}
		return DualResult{}, fmt.Errorf("container produced no JSON output (stdout was empty)")
	}

	var r DualResult
	if err := json.Unmarshal(rawJSON, &r); err != nil {
		return DualResult{}, fmt.Errorf("failed to parse container JSON: %v\nraw output: %s", err, string(rawJSON))
	}

	if r.Module == "" {
		r.Module = module
	}
	if r.Base.Ref == "" {
		r.Base.Ref = baseRef
	}
	if r.Head.Ref == "" {
		r.Head.Ref = headRef
	}

	return r, nil
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&repo, "repo", "", "Repo under test")
	runCmd.Flags().StringVar(&base, "base", "main", "Base git ref")
	runCmd.Flags().StringVar(&head, "head", "HEAD", "Head git ref")
	runCmd.Flags().StringVar(&image, "image", "grater-runner", "Docker image name")

	runCmd.MarkFlagRequired("repo")
}