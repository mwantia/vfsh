package main

import (
	"fmt"
	"os"

	"github.com/mwantia/vfsh/cmd/cli"
)

var (
	version = "0.0.1-dev"
	commit  = "main"
)

func main() {
	root := cli.NewRootCommand(cli.VersionInfo{
		Version: version,
		Commit:  commit,
	})

	root.AddCommand(cli.NewVersionCommand())
	root.AddCommand(cli.NewTuiCommand())

	if err := root.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
