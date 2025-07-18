package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/ethpandaops/pandafuzz/pkg/storage/migrations"
	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDBPath = "./pandafuzz.db"
)

func main() {
	var (
		dbPath  = flag.String("db", defaultDBPath, "Path to the database file")
		verbose = flag.Bool("v", false, "Enable verbose output")
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "PandaFuzz Migration Tool\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <command>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  up        Apply all pending migrations\n")
		fmt.Fprintf(os.Stderr, "  down      Roll back the last applied migration\n")
		fmt.Fprintf(os.Stderr, "  status    Show migration status\n")
		fmt.Fprintf(os.Stderr, "  reset     Drop all tables and rerun migrations (DANGEROUS!)\n")
		fmt.Fprintf(os.Stderr, "  create    Create a new migration file\n")
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s up                    # Apply all pending migrations\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -db prod.db status    # Check migration status for prod.db\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s create add_user_table # Create a new migration\n", os.Args[0])
	}

	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := flag.Arg(0)

	// Initialize database connection
	db, err := openDatabase(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Create migration system
	ms, err := migrations.NewMigrationSystem(db)
	if err != nil {
		log.Fatalf("Failed to initialize migration system: %v", err)
	}

	// Execute command
	switch command {
	case "up":
		if err := runUp(ms, *verbose); err != nil {
			log.Fatalf("Migration failed: %v", err)
		}
		fmt.Println("Migrations applied successfully")

	case "down":
		if err := runDown(ms, *verbose); err != nil {
			log.Fatalf("Rollback failed: %v", err)
		}
		fmt.Println("Migration rolled back successfully")

	case "status":
		if err := showStatus(ms); err != nil {
			log.Fatalf("Failed to get status: %v", err)
		}

	case "reset":
		if !confirmReset() {
			fmt.Println("Reset cancelled")
			os.Exit(0)
		}
		if err := runReset(ms, *verbose); err != nil {
			log.Fatalf("Reset failed: %v", err)
		}
		fmt.Println("Database reset successfully")

	case "create":
		if flag.NArg() < 2 {
			log.Fatal("Migration name required. Usage: migration create <name>")
		}
		name := flag.Arg(1)
		if err := migrations.CreateMigration(name); err != nil {
			log.Fatalf("Failed to create migration: %v", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func openDatabase(dbPath string) (*sql.DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database with proper settings
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	return db, nil
}

func runUp(ms *migrations.MigrationSystem, verbose bool) error {
	startTime := time.Now()

	if verbose {
		fmt.Println("Checking for pending migrations...")
	}

	err := ms.Up()
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Completed in %v\n", time.Since(startTime))
	}

	return nil
}

func runDown(ms *migrations.MigrationSystem, verbose bool) error {
	startTime := time.Now()

	if verbose {
		fmt.Println("Rolling back last migration...")
	}

	err := ms.Down()
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Completed in %v\n", time.Since(startTime))
	}

	return nil
}

func showStatus(ms *migrations.MigrationSystem) error {
	statuses, err := ms.Status()
	if err != nil {
		return err
	}

	if len(statuses) == 0 {
		fmt.Println("No migrations found")
		return nil
	}

	// Create a tabwriter for formatted output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "VERSION\tNAME\tSTATUS\tAPPLIED AT\tDESCRIPTION")
	fmt.Fprintln(w, "-------\t----\t------\t----------\t-----------")

	pendingCount := 0
	appliedCount := 0

	for _, status := range statuses {
		statusStr := "Pending"
		appliedAtStr := "-"

		if status.Applied {
			statusStr = "Applied"
			appliedCount++
			if status.AppliedAt != nil {
				appliedAtStr = status.AppliedAt.Format("2006-01-02 15:04:05")
			}
		} else {
			pendingCount++
		}

		fmt.Fprintf(w, "%03d\t%s\t%s\t%s\t%s\n",
			status.Version,
			status.Name,
			statusStr,
			appliedAtStr,
			status.Description,
		)
	}

	w.Flush()
	fmt.Printf("\nTotal: %d migrations (%d applied, %d pending)\n",
		len(statuses), appliedCount, pendingCount)

	return nil
}

func runReset(ms *migrations.MigrationSystem, verbose bool) error {
	startTime := time.Now()

	if verbose {
		fmt.Println("Resetting database...")
	}

	err := ms.Reset()
	if err != nil {
		return err
	}

	if verbose {
		fmt.Printf("Completed in %v\n", time.Since(startTime))
	}

	return nil
}

func confirmReset() bool {
	fmt.Println("WARNING: This will DROP ALL TABLES and DELETE ALL DATA!")
	fmt.Print("Are you sure you want to continue? (yes/no): ")

	var response string
	fmt.Scanln(&response)

	return response == "yes"
}
