package engine

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// CleanOrphanTempUploads scans uploads/temp and evicts abandoned session dirs older than maxAge.
func CleanOrphanTempUploads(uploadDir string, maxAge time.Duration) (int, int64) {
	tempRootDir := filepath.Join(uploadDir, "temp")
	entries, err := os.ReadDir(tempRootDir)
	if err != nil {
		return 0, 0
	}

	now := time.Now()
	deletedCount := 0
	var freedBytes int64 = 0

	for _, entry := range entries {
		sessionPath := filepath.Join(tempRootDir, entry.Name())
		fi, statErr := os.Stat(sessionPath)
		if statErr != nil {
			continue
		}

		if entry.IsDir() {
			var sessionSize int64
			var latestMod time.Time = fi.ModTime()
			subEntries, _ := os.ReadDir(sessionPath)
			for _, sub := range subEntries {
				if subFi, err := sub.Info(); err == nil {
					sessionSize += subFi.Size()
					if subFi.ModTime().After(latestMod) {
						latestMod = subFi.ModTime()
					}
				}
			}
			if now.Sub(latestMod) > maxAge {
				if rmErr := os.RemoveAll(sessionPath); rmErr == nil {
					deletedCount++
					freedBytes += sessionSize
					log.Printf("[Cleaner] Evicted abandoned chunk session: %s (%d bytes)", sessionPath, sessionSize)
				}
			}
		} else if now.Sub(fi.ModTime()) > maxAge {
			if rmErr := os.Remove(sessionPath); rmErr == nil {
				deletedCount++
				freedBytes += fi.Size()
				log.Printf("[Cleaner] Evicted orphan temp file: %s (%d bytes)", sessionPath, fi.Size())
			}
		}
	}
	return deletedCount, freedBytes
}

type OrphanItem struct {
	Path     string    `json:"path"`
	Category string    `json:"category"` // "temp_chunk", "unlinked_upload", "unlinked_output", "stale_partial"
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
}

type OrphanScanSummary struct {
	Items               []OrphanItem `json:"items"`
	TotalOrphanBytes    int64        `json:"total_orphan_bytes"`
	TempChunkBytes      int64        `json:"temp_chunk_bytes"`
	UnlinkedUploadBytes int64        `json:"unlinked_upload_bytes"`
	UnlinkedOutputBytes int64        `json:"unlinked_output_bytes"`
	StalePartialBytes   int64        `json:"stale_partial_bytes"`
	TotalCount          int          `json:"total_count"`
}

type PurgeResult struct {
	DeletedCount int   `json:"deleted_count"`
	FreedBytes   int64 `json:"freed_bytes"`
}

// ScanOrphanArtifacts inspects uploadDir and outputDir against database-tracked paths.
func ScanOrphanArtifacts(
	uploadDir, outputDir string,
	trackedPaths map[string]struct{},
	tempGracePeriod, uploadGracePeriod time.Duration,
) (*OrphanScanSummary, error) {
	summary := &OrphanScanSummary{
		Items: make([]OrphanItem, 0),
	}
	now := time.Now()

	// 1. Scan uploads/temp for abandoned chunk sessions
	tempRootDir := filepath.Join(uploadDir, "temp")
	if entries, err := os.ReadDir(tempRootDir); err == nil {
		for _, entry := range entries {
			sessionPath := filepath.Join(tempRootDir, entry.Name())
			fi, statErr := os.Stat(sessionPath)
			if statErr != nil {
				continue
			}
			if entry.IsDir() {
				var sessionSize int64
				var latestMod time.Time = fi.ModTime()
				subEntries, _ := os.ReadDir(sessionPath)
				for _, sub := range subEntries {
					if subFi, err := sub.Info(); err == nil {
						sessionSize += subFi.Size()
						if subFi.ModTime().After(latestMod) {
							latestMod = subFi.ModTime()
						}
					}
				}
				if now.Sub(latestMod) > tempGracePeriod {
					summary.Items = append(summary.Items, OrphanItem{
						Path:     filepath.Clean(sessionPath),
						Category: "temp_chunk",
						Size:     sessionSize,
						ModTime:  latestMod,
					})
					summary.TempChunkBytes += sessionSize
				}
			} else if now.Sub(fi.ModTime()) > tempGracePeriod {
				summary.Items = append(summary.Items, OrphanItem{
					Path:     filepath.Clean(sessionPath),
					Category: "temp_chunk",
					Size:     fi.Size(),
					ModTime:  fi.ModTime(),
				})
				summary.TempChunkBytes += fi.Size()
			}
		}
	}

	// 2. Scan uploads/ for unlinked files
	if entries, err := os.ReadDir(uploadDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() == "temp" {
				continue
			}
			fullPath := filepath.Clean(filepath.Join(uploadDir, entry.Name()))
			if _, isTracked := trackedPaths[fullPath]; !isTracked {
				fi, statErr := entry.Info()
				if statErr != nil {
					continue
				}
				if now.Sub(fi.ModTime()) > uploadGracePeriod {
					summary.Items = append(summary.Items, OrphanItem{
						Path:     fullPath,
						Category: "unlinked_upload",
						Size:     fi.Size(),
						ModTime:  fi.ModTime(),
					})
					summary.UnlinkedUploadBytes += fi.Size()
				}
			}
		}
	}

	// 3. Scan outputs/ for unlinked files or stale .part files
	if entries, err := os.ReadDir(outputDir); err == nil {
		for _, entry := range entries {
			fullPath := filepath.Clean(filepath.Join(outputDir, entry.Name()))
			fi, statErr := entry.Info()
			if statErr != nil {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".part") {
				if now.Sub(fi.ModTime()) > uploadGracePeriod {
					summary.Items = append(summary.Items, OrphanItem{
						Path:     fullPath,
						Category: "stale_partial",
						Size:     fi.Size(),
						ModTime:  fi.ModTime(),
					})
					summary.StalePartialBytes += fi.Size()
				}
			} else if _, isTracked := trackedPaths[fullPath]; !isTracked {
				if now.Sub(fi.ModTime()) > uploadGracePeriod {
					summary.Items = append(summary.Items, OrphanItem{
						Path:     fullPath,
						Category: "unlinked_output",
						Size:     fi.Size(),
						ModTime:  fi.ModTime(),
					})
					summary.UnlinkedOutputBytes += fi.Size()
				}
			}
		}
	}

	summary.TotalCount = len(summary.Items)
	summary.TotalOrphanBytes = summary.TempChunkBytes + summary.UnlinkedUploadBytes + summary.UnlinkedOutputBytes + summary.StalePartialBytes
	return summary, nil
}

// PurgeOrphanArtifacts safely deletes the provided orphan items.
func PurgeOrphanArtifacts(items []OrphanItem) (*PurgeResult, error) {
	result := &PurgeResult{}
	for _, item := range items {
		fi, err := os.Stat(item.Path)
		if err != nil {
			continue
		}
		var size int64 = item.Size
		if fi.IsDir() {
			err = os.RemoveAll(item.Path)
		} else {
			size = fi.Size()
			err = os.Remove(item.Path)
		}
		if err == nil {
			result.DeletedCount++
			result.FreedBytes += size
			log.Printf("[Cleaner] Purged orphan artifact (%s): %s (%d bytes)", item.Category, item.Path, size)
		} else {
			log.Printf("[Cleaner] Failed to remove orphan artifact %s: %v", item.Path, err)
		}
	}
	return result, nil
}

