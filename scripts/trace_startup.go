package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	dataDir := "/Volumes/SlimBoxUSB/SlimBox"
	uploadDir := filepath.Join(dataDir, "uploads")
	outputDir := filepath.Join(dataDir, "outputs")
	dbPath := filepath.Join(dataDir, "slimbox.db")

	fmt.Println("Step 1: Check directories...")
	_ = os.MkdirAll(uploadDir, 0755)
	_ = os.MkdirAll(outputDir, 0755)
	fmt.Println("Step 1 OK.")

	fmt.Println("Step 2: ReadDir outputs...")
	files, err := os.ReadDir(outputDir)
	fmt.Printf("Step 2 OK: %d files, err: %v\n", len(files), err)

	fmt.Println("Step 3: ReadDir uploads...")
	upFiles, err := os.ReadDir(uploadDir)
	fmt.Printf("Step 3 OK: %d files, err: %v\n", len(upFiles), err)

	fmt.Println("Step 4: Open SQLite db with timeout...")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(3000)")
	if err != nil {
		fmt.Printf("Step 4 err: %v\n", err)
		return
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tasks;").Scan(&count)
	fmt.Printf("Step 4 Query OK: %d tasks, err: %v\n", count, err)
}
