package internal

import (
    "fmt"
    "net/http"
    "os/exec"
    "strings"

	"github.com/PuerkitoBio/goquery"
)


func LoadModules(limit int, repo string) ([]string, error) {
	if repo == "" {
		out, err := exec.Command("git", "remote", "get-url", "origin").Output()
		if err != nil {
			return nil, fmt.Errorf("repo not provided: %w", err)
		}
		repo = strings.TrimSpace(string(out))
	}
	repo = cleanRepoURL(repo)
	return FetchFromPkgGoDev(limit, repo)
}

func FetchFromPkgGoDev(limit int, module string) ([]string, error) {
	url := fmt.Sprintf("https://pkg.go.dev/%s?tab=importedby", module)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pkg.go.dev returned status: %s", resp.Status)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var results []string
	doc.Find(".ImportedBy-details a").Each(func(i int, s *goquery.Selection) {
		if limit > 0 && len(results) >= limit {
			return
		}
		path := strings.TrimSpace(s.Text())
		if path != "" {
			results = append(results, path)
		}
	})

	return results, nil
}

func cleanRepoURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	if strings.HasPrefix(url, "git@") {
		url = strings.Replace(url, ":", "/", 1)
		url = strings.TrimPrefix(url, "git@")
	}
	return url
}
