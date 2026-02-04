package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type Result struct {
	Module string `json:"module"`
	Passed bool   `json:"passed"`
}

var (
	repo  string
	base  string
	head  string
	image string
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run downstream tests on base and head and detect regressions",
	RunE: func(cmd *cobra.Command, args []string) error {
		graterDir := ".grater"

		modFile := filepath.Join(graterDir, "modules.txt")
		data, err := os.ReadFile(modFile)
		if err != nil {
			return fmt.Errorf("run prepare first: %w", err)
		}

		modules := strings.Split(strings.TrimSpace(string(data)), "\n")

		resultsFile := filepath.Join(graterDir, "results.json")
		var allResults []map[string]string

		for _, m := range modules {
			fmt.Println("Testing:", m)

			baseRes, _ := runContainer(image, m, repo, base)
			headRes, _ := runContainer(image, m, repo, head)

			status := "PASS"
			if baseRes.Passed && !headRes.Passed {
				status = "REGRESSION"
			} else if !baseRes.Passed {
				status = "BROKEN"
			}

			fmt.Printf("%s => %s\n", m, status)

			allResults = append(allResults, map[string]string{
				"module": m,
				"status": status,
			})
		}

		out, _ := json.MarshalIndent(allResults, "", "  ")
		return os.WriteFile(resultsFile, out, 0644)
	},
}

func runContainer(image, module, repo, ref string) (Result, error) {
	cmd := exec.Command("docker", "run", "--rm",
		"-e", "MODULE="+module,
		"-e", "REPO="+repo,
		"-e", "REF="+ref,
		image,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return Result{}, err
	}

	var r Result
	json.Unmarshal(out.Bytes(), &r)
	return r, nil
}

func init() {
	rootCmd.AddCommand(runCmd)

	runCmd.Flags().StringVar(&repo, "repo", "", "Repo under test (e.g github.com/open-telemetry/opentelemetry-go)")
	runCmd.Flags().StringVar(&base, "base", "main", "Base git ref")
	runCmd.Flags().StringVar(&head, "head", "HEAD", "Head git ref")
	runCmd.Flags().StringVar(&image, "image", "grater-runner", "Docker runner image")

	runCmd.MarkFlagRequired("repo")
}
