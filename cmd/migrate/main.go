package main

import (
	"flag"
	"fmt"
	"github.com/iamvalson/blink/internal/migration"
	"github.com/iamvalson/blink/migrations"
	"github.com/joho/godotenv"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: migrate {up|down|status|baseline -version N}; down reverts one migration")
	}
	command := args[0]
	if command != "up" && command != "down" && command != "status" && command != "baseline" {
		return fmt.Errorf("unknown migration command %q", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	version := flags.Uint("version", 0, "verified existing schema version, for baseline only")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected migration arguments")
	}
	if command == "baseline" && *version == 0 {
		return fmt.Errorf("baseline requires -version N after inspecting the existing schema")
	}
	if command != "baseline" && *version != 0 {
		return fmt.Errorf("-version is only supported for baseline")
	}
	_ = godotenv.Load()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	runner, err := migration.Open(databaseURL, migrations.Files)
	if err != nil {
		return err
	}
	defer runner.Close()
	switch command {
	case "up":
		err = runner.Up()
	case "down":
		err = runner.Down()
	case "baseline":
		err = runner.Baseline(*version)
	}
	if err != nil {
		return err
	}
	status, err := runner.Status()
	if err != nil {
		return err
	}
	if status.Empty {
		fmt.Printf("No tracked version; existing Blink schema: %t; latest embedded version: %d\n", status.ExistingSchema, status.Latest)
	} else {
		fmt.Printf("Migration version: %d; dirty: %t; latest embedded version: %d\n", status.Version, status.Dirty, status.Latest)
	}
	return nil
}
