package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Result struct {
	Module string `json:"module"`
	Passed bool   `json:"passed"`
}

func run(image, module, repo, ref string) (Result, error) {
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

func main() {
	list := flag.String("list", "modules.txt", "")
	repo := flag.String("repo", "", "")
	base := flag.String("base", "main", "")
	head := flag.String("head", "HEAD", "")
	image := flag.String("image", "grater-runner", "")
	flag.Parse()

	data, _ := os.ReadFile(*list)
	modules := strings.Split(strings.TrimSpace(string(data)), "\n")

	for _, m := range modules {
		baseRes, _ := run(*image, m, *repo, *base)
		headRes, _ := run(*image, m, *repo, *head)

		status := "PASS"
		if baseRes.Passed && !headRes.Passed {
			status = "REGRESSION"
		} else if !baseRes.Passed {
			status = "BROKEN"
		}

		fmt.Printf("%s => %s\n", m, status)
	}
}
