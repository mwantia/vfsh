package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mwantia/vfs"
	"github.com/mwantia/vfs/data"
	"github.com/mwantia/vfs/mount"
	"github.com/mwantia/vfs/mount/backend/ephemeral"
	"github.com/mwantia/vfs/mount/backend/sqlite"
)

func initializeVirtualFileSystem(ctx context.Context, configPath string) (vfs.VirtualFileSystem, error) {
	logPath := filepath.Join(configPath, "vfsh.log")

	fs, err := vfs.NewVirtualFileSystem(vfs.WithLogFile(logPath), vfs.WithoutTerminalLog())
	if err != nil {
		return nil, fmt.Errorf("failed to setup vfs: %v", err)
	}

	rootPath := filepath.Join(configPath, "vfsh.db")
	root, err := sqlite.NewSQLiteBackend(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to setup vfs: %v", err)
	}

	if err := fs.Mount(ctx, "/", root, mount.WithMetadata(root), mount.WithNamespace("root")); err != nil {
		return nil, fmt.Errorf("failed to setup vfs: %v", err)
	}

	ephemeral := ephemeral.NewEphemeralBackend()
	if err := fs.Mount(ctx, "/ephemeral", ephemeral); err != nil {
		return nil, fmt.Errorf("failed to setup vfs: %v", err)
	}

	return fs, nil
}

func initializeDemo(ctx context.Context, fs vfs.VirtualFileSystem) error {
	demo := ephemeral.NewEphemeralBackend()
	if err := fs.Mount(ctx, "/demo", demo); err != nil {
		return fmt.Errorf("failed to setup demo mount: %v", err)
	}

	demoDirectories := []string{
		"/demo/documents",
		"/demo/downloads",
		"/demo/logs",
		"/demo/config",
	}

	for _, dir := range demoDirectories {
		if err := fs.CreateDirectory(ctx, dir); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", dir, err)
		}
	}

	demoFiles := map[string]string{
		"/demo/readme.txt":          "Welcome to the VFS demo!",
		"/demo/documents/notes.txt": "This is a sample document",
		"/demo/downloads/file1.dat": "Download One",
		"/demo/downloads/file2.dat": "Download Two",
		"/demo/config/config.conf":  "# Configuration file\nenabled = true",
		"/demo/logs/system.log":     "System log entry 1\nSystem log entry 2\nSystem log entry 3",
	}

	for path, content := range demoFiles {
		// Create and write to the file
		file, err := fs.OpenFile(ctx, path, data.AccessModeWrite|data.AccessModeCreate)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}

		if _, err := file.Write([]byte(content)); err != nil {
			file.Close()
			return fmt.Errorf("failed to write to file %s: %w", path, err)
		}

		file.Close()
	}

	return nil
}
