package config

import (
	"fmt"
	"os"
	"path/filepath"
)

func GetConfigDirectory() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %v", err)
	}

	path := filepath.Join(config, "vfsh")
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %v", err)
	}

	return path, nil
}
