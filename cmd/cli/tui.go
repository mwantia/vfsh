package cli

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mwantia/vfsh/internal/config"
	"github.com/mwantia/vfsh/internal/tui"
	"github.com/spf13/cobra"
)

func NewTuiCommand() *cobra.Command {
	var configPath string
	var demoEnabled bool

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

			fs, err := initializeVirtualFileSystem(ctx, configPath)
			if err != nil {
				return fmt.Errorf("failed to initialize vfs: %v", err)
			}

			if demoEnabled {
				if err := initializeDemo(ctx, fs); err != nil {
					return fmt.Errorf("failed to initialize vfs: %v", err)
				}
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

	cmd.PersistentFlags().StringVar(&configPath, "config", "", "config path (default: ~/.config/vfsh)")
	cmd.PersistentFlags().BoolVar(&demoEnabled, "demo", false, "creates a /demo mount if enabled (default: false)")

	return cmd
}
