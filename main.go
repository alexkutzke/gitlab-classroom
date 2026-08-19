package main

import (
	"fmt"
	"os"

	"github.com/alexkutzke/gitlab-classroom/internal/cli"
)

func main() {
	if err := cli.Executar(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
