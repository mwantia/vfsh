package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

type VersionInfo struct {
	Version string
	Commit  string
}

func NewVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long:  `Display version information for this application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			short, _ := cmd.Flags().GetBool("short")
			json, _ := cmd.Flags().GetBool("json")

			version := cmd.Root().Version
			if version == "" {
				return fmt.Errorf("failed to parse version")
			}

			if short {
				fmt.Println(version)
				return nil
			}

			if json {
				fmt.Printf(`{
    "version": "%s",
    "go_version": "%s",
    "platform": "%s/%s"
}
`, version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			} else {
				fmt.Printf(`VFS Shell
Version:    %s
Go version: %s
Platform:   %s/%s
`, version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
			}

			return nil
		},
	}

	cmd.Flags().Bool("short", false, "show only version number")
	cmd.Flags().Bool("json", false, "output version as json")

	return cmd
}
