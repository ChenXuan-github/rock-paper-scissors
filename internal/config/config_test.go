package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "application.yml")
	content := []byte("server:\n  port: 9090\n")

	if err := os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if config.Server.Port != 9090 {
		t.Errorf("server port = %d, want 9090", config.Server.Port)
	}
}
