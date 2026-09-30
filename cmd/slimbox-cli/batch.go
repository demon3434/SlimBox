package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"slimbox/internal/domain"
)

type BatchState struct {
	CompletedFiles map[string]string `json:"completed_files"` // local path -> remote task id / output name
	LastUpdated    string            `json:"last_updated"`
}

func loadBatchState(stateFilePath string) *BatchState {
	s := &BatchState{CompletedFiles: make(map[string]string)}
	data, err := os.ReadFile(stateFilePath)
	if err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.CompletedFiles == nil {
		s.CompletedFiles = make(map[string]string)
	}
	return s
}

func saveBatchState(stateFilePath string, s *BatchState) {
	s.LastUpdated = time.Now().Format(time.RFC3339)
	data, err := json.MarshalIndent(s, "", "  ")
	if err == nil {
		_ = os.WriteFile(stateFilePath, data, 0644)
	}
}

// RunBatch executes the automated batch compression pipeline (ADR-0005).
func RunBatch(
	client *APIClient,
	inputDir, listFile, outputDir, profile, codec, stateFile string,
	deleteServerArtifacts bool,
) error {
	var filesToProcess []string

	if listFile != "" {
		f, err := os.Open(listFile)
		if err != nil {
			return fmt.Errorf("failed to open list file: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				filesToProcess = append(filesToProcess, line)
			}
		}
	} else if inputDir != "" {
		entries, err := os.ReadDir(inputDir)
		if err != nil {
			return fmt.Errorf("failed to read input directory: %w", err)
		}

		supportedExts := map[string]bool{
			".mkv": true, ".mp4": true, ".ts": true,
			".avi": true, ".mov": true, ".flv": true,
			".wmv": true, ".m4v": true,
		}

		for _, e := range entries {
			if !e.IsDir() {
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if supportedExts[ext] {
					filesToProcess = append(filesToProcess, filepath.Join(inputDir, e.Name()))
				}
			}
		}
	} else {
		return fmt.Errorf("must specify either --input-dir (-d) or --list (-l)")
	}

	if len(filesToProcess) == 0 {
		log.Println("[Batch] No video files found to process.")
		return nil
	}

	if stateFile == "" {
		stateFile = filepath.Join(outputDir, ".slimbox-state.json")
	}

	state := loadBatchState(stateFile)
	log.Printf("[Batch] Found %d total files. %d already completed. Profile: %s (%s)",
		len(filesToProcess), len(state.CompletedFiles), profile, codec)

	var activeStagedTaskID string
	var mu sync.Mutex

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		mu.Lock()
		tid := activeStagedTaskID
		mu.Unlock()
		if tid != "" {
			fmt.Printf("\n⚠️ 检测到中断信号，正在回滚清理服务端未入队任务 (ID: %s)...\n", tid)
			_ = client.DeleteTask(tid)
		}
		os.Exit(130)
	}()

	for i, localFile := range filesToProcess {
		baseName := filepath.Base(localFile)
		if _, done := state.CompletedFiles[localFile]; done {
			log.Printf("[Batch %d/%d] Skipping already completed file: %s", i+1, len(filesToProcess), baseName)
			continue
		}

		log.Println("--------------------------------------------------------------------------------")
		log.Printf("[Batch %d/%d] Processing: %s", i+1, len(filesToProcess), baseName)

		// 1. Upload to N1 / Raspberry Pi
		log.Printf("  -> Uploading to SlimBox server...")
		lastPct := -1
		task, err := client.UploadVideo(localFile, func(uploaded, total int64) {
			if total > 0 {
				pct := int((uploaded * 100) / total)
				if pct != lastPct && pct%10 == 0 {
					lastPct = pct
					fmt.Printf("\r     [Upload] %d%% (%d MB / %d MB)", pct, uploaded/1024/1024, total/1024/1024)
				}
			}
		})
		fmt.Println()
		if err != nil {
			log.Printf("  ❌ Upload failed: %v. Moving to next file...", err)
			continue
		}

		mu.Lock()
		activeStagedTaskID = task.ID
		mu.Unlock()

		log.Printf("  -> Uploaded successfully (Task ID: %s)", task.ID)

		// 2. Start compression task
		log.Printf("  -> Queuing compression with profile [%s]...", profile)
		_, err = client.StartTask(task.ID, profile, codec)
		if err != nil {
			log.Printf("  ❌ Failed to start task: %v (automatically rolling back remote unstarted task...)", err)
			_ = client.DeleteTask(task.ID)
			mu.Lock()
			activeStagedTaskID = ""
			mu.Unlock()
			continue
		}

		mu.Lock()
		activeStagedTaskID = ""
		mu.Unlock()

		// 3. Poll progress until completion
		log.Printf("  -> Waiting in queue & transcoding...")
		var completedTask *domain.Task
		for {
			time.Sleep(2 * time.Second)
			t, err := client.GetTask(task.ID)
			if err != nil {
				continue
			}

			if t.Status == domain.StatusTranscoding {
				prog := t.Progress
				fmt.Printf("\r     [Transcoding] %.1f%% | FPS: %.1f | Speed: %s | Elapsed: %s | ETA: %ds   ",
					prog.Percent, prog.CurrentFPS, prog.Speed, prog.CurrentTimeStr, prog.ETASeconds)
			} else if t.Status == domain.StatusQueued {
				fmt.Printf("\r     [Queued] Waiting for previous task to finish...               ")
			} else if t.Status == domain.StatusCompleted {
				fmt.Println()
				log.Printf("  ✅ Transcoding complete!")
				completedTask = t
				break
			} else if t.Status == domain.StatusFailed {
				fmt.Println()
				log.Printf("  ❌ Transcoding failed on server: %s", t.ErrorMsg)
				break
			} else if t.Status == domain.StatusAborted {
				fmt.Println()
				log.Printf("  ⚠️ Task was aborted on server.")
				break
			}
		}

		if completedTask == nil {
			continue // Skip download if failed or aborted
		}

		// 4. Download output to local output directory
		localOutPath := filepath.Join(outputDir, completedTask.OutputFileName)
		log.Printf("  -> Downloading compressed result to %s...", localOutPath)
		if err := client.DownloadOutput(completedTask.ID, localOutPath); err != nil {
			log.Printf("  ❌ Download failed: %v", err)
			continue
		}

		origMB := float64(task.SourceFileSize) / 1024 / 1024
		compMB := float64(completedTask.OutputFileSize) / 1024 / 1024
		savings := (1.0 - (compMB / origMB)) * 100.0
		log.Printf("  🎉 Successfully compressed: %.1f MB -> %.1f MB (Savings: %.1f%%⬇)", origMB, compMB, savings)

		// 5. Cleanup server files if requested
		if deleteServerArtifacts {
			_ = client.DeleteTask(completedTask.ID)
		}

		// 6. Record state for resumption
		state.CompletedFiles[localFile] = completedTask.OutputFileName
		saveBatchState(stateFile, state)
	}

	log.Println("================================================================================")
	log.Printf("[Batch] All %d tasks in batch completed successfully!", len(filesToProcess))
	return nil
}
