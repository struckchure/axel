package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// Default scaffold layout, relative to the target directory.
const (
	initConfigFile    = "axel.yaml"
	initSchemaRelPath = "axel/schema.asl"
	initMigrationsDir = "axel/migrations"
)

// starterSchema is the default ASL schema written by `axel init`. It defines a
// reusable `Base` abstract type — a uuid primary key plus created_at/updated_at
// timestamps — and a starter `User` type extending it.
const starterSchema = `use extension 'pgcrypto';

# Reusable base: uuid primary key + created/updated timestamps.
# Extend it on every type to inherit these fields.
abstract type Base {
  required id: uuid {
    default := gen_uuid();
    constraint exclusive;
    constraint pk;
  };
  required created_at: datetime { default := datetime_current(); };
  required updated_at: datetime {
    default := datetime_current();
    rewrite update := datetime_current();
  };
}

type User extending Base {
  required email: str {
    constraint exclusive;
    constraint max_length(255);
  };
  name: str;
  active: bool { default := true };
}
`

// starterConfig is the default axel.yaml written by `axel init`. Kept as a
// commented template so the generated file is self-documenting.
const starterConfig = `# Axel project configuration.
schema-path: %s
migrations-dir: %s
database-url: %q
`

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold a new Axel project (config + starter schema)",
	Long: "Create an axel.yaml config, a starter ASL schema with a Base model\n" +
		"(uuid id + created_at/updated_at timestamps), and a migrations directory.",
	// Override parent PersistentPreRun — there is no config to load yet, and we
	// must not try to open a database connection.
	PersistentPreRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		// Target directory: --dir (defaults to the current directory).
		targetDir := projectDir

		// Default to an env reference so secrets stay out of the config file.
		// Axel expands `$env.NAME` when it loads the config.
		dbURL := databaseURL
		if dbURL == "" {
			dbURL = "$env.DATABASE_URL"
		}

		cfgBytes := []byte(fmt.Sprintf(starterConfig, initSchemaRelPath, initMigrationsDir, dbURL))

		configPath := filepath.Join(targetDir, initConfigFile)
		schemaPath := filepath.Join(targetDir, initSchemaRelPath)
		migrationsPath := filepath.Join(targetDir, initMigrationsDir)

		if err := writeFile(configPath, cfgBytes, force); err != nil {
			return err
		}
		if err := writeFile(schemaPath, []byte(starterSchema), force); err != nil {
			return err
		}
		if err := os.MkdirAll(migrationsPath, 0755); err != nil {
			return fmt.Errorf("creating migrations dir %q: %w", migrationsPath, err)
		}

		fmt.Printf("Initialized Axel project in %s\n", filepath.Clean(targetDir))
		fmt.Printf("  %s\n", configPath)
		fmt.Printf("  %s\n", schemaPath)
		fmt.Printf("  %s/\n", migrationsPath)
		fmt.Println("\nNext steps:")
		fmt.Println("  1. Set your database URL in axel.yaml")
		fmt.Println("  2. Edit the schema, then run: axel validate")
		fmt.Println("  3. Generate a migration:      axel generate -n init")
		return nil
	},
}

// writeFile writes data to path, creating parent directories. It refuses to
// overwrite an existing file unless force is set.
func writeFile(path string, data []byte, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", path)
		}
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating dir %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %q: %w", path, err)
	}
	return nil
}

func init() {
	initCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
	RootCmd.AddCommand(initCmd)
}
