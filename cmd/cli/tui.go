package cli

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwantia/vfs"
	"github.com/mwantia/vfs/mount"
	"github.com/mwantia/vfs/mount/backend/ephemeral"
	"github.com/mwantia/vfs/mount/backend/sqlite"
	"github.com/mwantia/vfsh/internal/config"
	"github.com/mwantia/vfsh/internal/tui"
	"github.com/spf13/cobra"
)

func NewTuiCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Terminal user interface",
		Long:  `Run the VFS Shell as terminal user interface.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if configPath == "" {
				path, err := config.GetConfigDirectory()
				if err != nil {
					return fmt.Errorf("failed to setup vfs: %v", err)
				}
				configPath = path
			}

			logFilePath := filepath.Join(configPath, "vfsh.log")
			fs, err := vfs.NewVirtualFileSystem(vfs.WithLogFile(logFilePath), vfs.WithoutTerminalLog())
			if err != nil {
				return fmt.Errorf("failed to setup vfs: %v", err)
			}

			rootDbPath := filepath.Join(configPath, "vfsh.db")
			root, err := sqlite.NewSQLiteBackend(rootDbPath)
			if err != nil {
				return fmt.Errorf("failed to setup vfs: %v", err)
			}

			if err := fs.Mount(ctx, "/", root, mount.WithMetadata(root), mount.WithNamespace("root")); err != nil {
				return fmt.Errorf("failed to setup vfs: %v", err)
			}

			ephemeral := ephemeral.NewEphemeralBackend()
			if err := fs.Mount(ctx, "/ephemeral", ephemeral); err != nil {
				return fmt.Errorf("failed to setup vfs: %v", err)
			}

			// Create VFS adapter and TUI model
			adapter := tui.NewVFSAdapter(ctx, fs)
			model := tui.NewModel(adapter)

			p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("tui error: %v", err)
			}

			// Shutdown up VFS mounts before exiting
			if err := fs.Shutdown(ctx); err != nil {
				return fmt.Errorf("failed to properly close VFS: %v", err)
			}

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "config path (default is ~/.config/vfsh)")

	return cmd
}
