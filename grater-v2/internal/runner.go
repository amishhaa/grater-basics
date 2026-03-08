module internal

import (
	"fmt"
)

func RunTests(repo, base, head string) {
	fmt.Printf("Running tests for repo: %s, base: %s, head: %s\n", repo, base, head)
}
