package config

import (
	"os"
	"testing"
)

func TestInitConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Change to temp directory
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	// Test initialization
	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}

	// Check if file exists
	if _, err := os.Stat("axel.yaml"); os.IsNotExist(err) {
		t.Error("axel.yaml was not created")
	}
}

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	
	// Change to temp directory
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	// Create config file first
	if err := InitConfig(); err != nil {
		t.Fatalf("InitConfig failed: %v", err)
	}

	// Test loading
	cfg, err := LoadConfig("axel.yaml")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify config values
	if cfg.Database.Type != "postgres" {
		t.Errorf("Expected database type 'postgres', got '%s'", cfg.Database.Type)
	}

	if cfg.Output.Directory != "./generated" {
		t.Errorf("Expected output directory './generated', got '%s'", cfg.Output.Directory)
	}

	if len(cfg.Generators) == 0 {
		t.Error("Expected at least one generator")
	}
}
