package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewRootCommand(info VersionInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "vfsh",
		Short:         "VFS Shell",
		Long:          "A production-ready, cross-platform sync client for S3 (MinIO) that provides true bidirectional synchronization similar to MegaSync, Dropbox, or OneDrive.",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cmd.Version = fmt.Sprintf("%s.%s", info.Version, info.Commit)

	return cmd
}
