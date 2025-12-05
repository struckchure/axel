package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the axel configuration
type Config struct {
	Database   DatabaseConfig   `yaml:"database"`
	Generators []GeneratorConfig `yaml:"generators"`
	Output     OutputConfig     `yaml:"output"`
}

// DatabaseConfig holds database connection settings
type DatabaseConfig struct {
	Type     string `yaml:"type"`     // postgres, mysql, sqlite
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"sslMode"`
}

// GeneratorConfig specifies code generation settings
type GeneratorConfig struct {
	Language string            `yaml:"language"` // go, python, typescript, javascript
	Options  map[string]string `yaml:"options"`
}

// OutputConfig specifies output settings
type OutputConfig struct {
	Directory string `yaml:"directory"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

// InitConfig creates a default configuration file
func InitConfig() error {
	defaultConfig := Config{
		Database: DatabaseConfig{
			Type:     "postgres",
			Host:     "localhost",
			Port:     5432,
			Database: "mydb",
			Username: "user",
			Password: "password",
			SSLMode:  "disable",
		},
		Generators: []GeneratorConfig{
			{
				Language: "go",
				Options: map[string]string{
					"package": "models",
				},
			},
		},
		Output: OutputConfig{
			Directory: "./generated",
		},
	}

	data, err := yaml.Marshal(&defaultConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile("axel.yaml", data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
