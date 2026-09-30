package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type QueuedSummary struct {
	Index    int
	FileName string
	FileSize int64
	TaskID   string
	Profile  string
	Status   string
	ErrMsg   string
}

func runPushCmd(args []string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	listFile := fs.String("file", "", "待压缩视频清单文本文件 (-f / -l)")
	fs.StringVar(listFile, "f", "", "待压缩视频清单文本文件 (-f)")
	fs.StringVar(listFile, "l", "", "待压缩视频清单文本文件 (简写)")
	profile := fs.String("profile", "720p", "压缩预设枚举档位 (-p, 如 360p, 480p, 720p, 1080p, 2k, 4k)")
	fs.StringVar(profile, "p", "720p", "压缩预设枚举档位 (简写)")
	codec := fs.String("codec", "", "视频编码器覆盖 (libx265 / libx264)")
	crf := fs.Int("crf", 0, "CRF 质量因子覆盖 (16-32)")
	preset := fs.String("preset", "", "编码速度预设覆盖 (ultrafast~slow)")
	audioBitrate := fs.String("audio-bitrate", "", "音频码率覆盖 (如 64k, 96k, 128k)")
	subtitlePolicy := fs.String("subtitle-policy", "", "字幕保留策略 (copy_all / drop)")

	_ = fs.Parse(args)
	positionalFiles := fs.Args()

	var filesToProcess []string

	// 1. Parse manifest list file if provided
	if *listFile != "" {
		f, err := os.Open(*listFile)
		if err != nil {
			fmt.Printf("错误: 无法打开清单文件 %s: %v\n", *listFile, err)
			os.Exit(1)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				filesToProcess = append(filesToProcess, line)
			}
		}
	}

	// 2. Append positional file arguments or expand directory
	for _, arg := range positionalFiles {
		cleanArg := strings.Trim(arg, "\"'")
		fi, err := os.Stat(cleanArg)
		if err != nil {
			fmt.Printf("警告: 路径不存在或无法访问，已跳过: %s\n", cleanArg)
			continue
		}
		if fi.IsDir() {
			supportedExts := map[string]bool{
				".mkv": true, ".mp4": true, ".ts": true,
				".avi": true, ".mov": true, ".flv": true,
				".wmv": true, ".m4v": true,
			}
			entries, _ := os.ReadDir(cleanArg)
			for _, e := range entries {
				if !e.IsDir() {
					ext := strings.ToLower(filepath.Ext(e.Name()))
					if supportedExts[ext] {
						filesToProcess = append(filesToProcess, filepath.Join(cleanArg, e.Name()))
					}
				}
			}
		} else {
			filesToProcess = append(filesToProcess, cleanArg)
		}
	}

	if len(filesToProcess) == 0 {
		fmt.Println("错误: 未指定任何待压缩视频文件。")
		fmt.Println("使用方式:")
		fmt.Println("  slimbox-cli push -s http://<IP>:8080 -p 720p video1.mp4 video2.mkv")
		fmt.Println("  slimbox-cli push -s http://<IP>:8080 -p 1080p -f my_series.txt")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	opts := StartTaskOptions{
		ProfileName:    *profile,
		VideoCodec:     *codec,
		CRF:            *crf,
		Preset:         *preset,
		AudioBitrate:   *audioBitrate,
		SubtitlePolicy: *subtitlePolicy,
	}

	fmt.Println("================================================================================")
	fmt.Printf("🚀 SlimBox 批量推送入队任务 (共 %d 个文件)\n", len(filesToProcess))
	fmt.Printf("目标服务: %s | 目标档位: %s\n", *serverURL, *profile)
	if *codec != "" || *crf > 0 || *preset != "" || *audioBitrate != "" || *subtitlePolicy != "" {
		fmt.Printf("显式覆盖参数: codec=%s crf=%d preset=%s audio=%s subs=%s\n",
			*codec, *crf, *preset, *audioBitrate, *subtitlePolicy)
	}
	fmt.Println("================================================================================")

	var summaries []QueuedSummary
	startTime := time.Now()

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

	for idx, filePath := range filesToProcess {
		fileName := filepath.Base(filePath)
		fi, err := os.Stat(filePath)
		fileSize := int64(0)
		if err == nil {
			fileSize = fi.Size()
		}

		fmt.Printf("\n[%d/%d] 正在处理: %s (%.1f MB)\n", idx+1, len(filesToProcess), fileName, float64(fileSize)/1024/1024)

		// 1. 流式上传
		fmt.Printf("  -> 正在上传至 SlimBox 服务端...")
		lastPct := -1
		task, err := client.UploadVideo(filePath, func(uploaded, total int64) {
			if total > 0 {
				pct := int((uploaded * 100) / total)
				if pct != lastPct && pct%5 == 0 {
					lastPct = pct
					fmt.Printf("\r  -> 上传进度: %d%% (%d MB / %d MB)", pct, uploaded/1024/1024, total/1024/1024)
				}
			}
		})
		fmt.Println()

		if err != nil {
			log.Printf("  ❌ 上传失败: %v (已自动清理临时残片，跳至下一项)", err)
			summaries = append(summaries, QueuedSummary{
				Index:    idx + 1,
				FileName: fileName,
				FileSize: fileSize,
				Status:   "FAILED_UPLOAD",
				ErrMsg:   err.Error(),
			})
			continue
		}

		mu.Lock()
		activeStagedTaskID = task.ID
		mu.Unlock()

		// 2. 加入串行转码排队
		startedTask, err := client.StartTaskWithOptions(task.ID, opts)
		if err != nil {
			log.Printf("  ❌ 排队启动失败: %v (正在回滚撤销服务端未入队任务...)", err)
			_ = client.DeleteTask(task.ID)
			mu.Lock()
			activeStagedTaskID = ""
			mu.Unlock()

			summaries = append(summaries, QueuedSummary{
				Index:    idx + 1,
				FileName: fileName,
				FileSize: fileSize,
				TaskID:   task.ID,
				Status:   "FAILED_START",
				ErrMsg:   err.Error(),
			})
			continue
		}

		mu.Lock()
		activeStagedTaskID = ""
		mu.Unlock()

		fmt.Printf("  ✅ 成功入队! Task ID: %s | 队列状态: %s\n", startedTask.ID, startedTask.Status)
		summaries = append(summaries, QueuedSummary{
			Index:    idx + 1,
			FileName: fileName,
			FileSize: fileSize,
			TaskID:   startedTask.ID,
			Profile:  startedTask.Params.ProfileName,
			Status:   "QUEUED",
		})
	}

	elapsed := time.Since(startTime).Round(time.Second)
	fmt.Println("\n============================ 批量入队完成汇总 ============================")
	successCount := 0
	for _, s := range summaries {
		if s.Status == "QUEUED" {
			successCount++
			fmt.Printf("  ✓ [%s] %-30s (%.1f MB) -> ID: %s\n", s.Profile, s.FileName, float64(s.FileSize)/1024/1024, s.TaskID)
		} else {
			fmt.Printf("  ✗ [失败] %-30s -> %s: %s\n", s.FileName, s.Status, s.ErrMsg)
		}
	}
	fmt.Printf("总计: 成功入队 %d / %d 个任务 | 耗时: %v\n", successCount, len(filesToProcess), elapsed)
	fmt.Println("提示: 所有任务已在远端安全入队，服务端将独立执行串行转码。当前客户端可安全退出或关机。")
	fmt.Println("随时可通过 'slimbox-cli status' 查询进度，或使用 'slimbox-cli pull' 批量取回成品。")
	fmt.Println("==========================================================================")
}
