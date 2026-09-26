package engine

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// CleanupPartialFile removes temporary unfinished .part files generated during transcoding.
// Per ADR-0011, this must be invoked synchronously whenever a task fails or is aborted.
func CleanupPartialFile(outputFilePath string) {
	if outputFilePath == "" {
		return
	}
	partPath := outputFilePath + ".part"
	if _, err := os.Stat(partPath); err == nil {
		if rmErr := os.Remove(partPath); rmErr != nil {
			log.Printf("[Cleaner] Failed to remove partial file %s: %v", partPath, rmErr)
		} else {
			log.Printf("[Cleaner] Successfully removed incomplete artifact %s", partPath)
		}
	}

	// Also check if the final outputFilePath exists partially (if not using .part)
	if _, err := os.Stat(outputFilePath); err == nil {
		_ = os.Remove(outputFilePath)
	}
}

// RemoveFile removes a file safely, ignoring non-existence.
func RemoveFile(filePath string) error {
	if filePath == "" {
		return nil
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// CleanOrphanPartials scans an output directory and cleans any .part files left over from crashes.
func CleanOrphanPartials(outputDir string) {
	files, err := os.ReadDir(outputDir)
	if err != nil {
		return
	}
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".part") {
			fullPath := filepath.Join(outputDir, f.Name())
			_ = os.Remove(fullPath)
			log.Printf("[Cleaner] Cleaned orphan partial file on startup: %s", fullPath)
		}
	}
}
