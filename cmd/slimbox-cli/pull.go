package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slimbox/internal/domain"
	"time"
)

func runPullCmd(args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	outputDir := fs.String("output", "./compressed", "成品下载目标目录 (-o)")
	fs.StringVar(outputDir, "o", "./compressed", "成品下载目标目录 (简写)")
	cleanRemote := fs.Bool("clean", false, "下载成功后自动删除服务端的任务与成品文件以释放存储空间 (-c)")
	fs.BoolVar(cleanRemote, "c", false, "下载成功后自动清理服务端 (-c)")

	_ = fs.Parse(args)

	client := NewAPIClient(*serverURL, *key)
	fmt.Println("================================================================================")
	fmt.Printf("📦 SlimBox 批量成品拉取 (从 %s 拉取到 %s)\n", *serverURL, *outputDir)
	if *cleanRemote {
		fmt.Println("模式: 下载成功后将自动清理服务端对应文件 (--clean 已启用)")
	}
	fmt.Println("================================================================================")

	tasks, err := client.ListTasks()
	if err != nil {
		fmt.Printf("错误: 无法获取服务端任务列表: %v\n", err)
		os.Exit(1)
	}

	var completedTasks []*domain.Task
	for _, t := range tasks {
		if t.Status == domain.StatusCompleted {
			completedTasks = append(completedTasks, t)
		}
	}

	if len(completedTasks) == 0 {
		fmt.Println("服务端当前没有已完成 (completed) 的转码任务。")
		fmt.Println("提示: 可使用 'slimbox-cli status' 查询是否有正在转码或排队中的任务。")
		return
	}

	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Printf("错误: 无法创建本地输出目录 %s: %v\n", *outputDir, err)
		os.Exit(1)
	}

	fmt.Printf("发现 %d 个已完成的转码成品，开始下载...\n\n", len(completedTasks))

	downloadedCount := 0
	skippedCount := 0
	failedCount := 0
	totalSavedBytes := int64(0)
	startTime := time.Now()

	for idx, t := range completedTasks {
		outName := t.OutputFileName
		if outName == "" {
			outName = fmt.Sprintf("compressed_%s.mp4", t.ID[:8])
		}
		localDestPath := filepath.Join(*outputDir, outName)

		// Check if already downloaded
		if fi, err := os.Stat(localDestPath); err == nil && fi.Size() == t.OutputFileSize && t.OutputFileSize > 0 {
			fmt.Printf("[%d/%d] ⏭️ 跳过已存在成品: %s (%.1f MB)\n",
				idx+1, len(completedTasks), outName, float64(t.OutputFileSize)/1024/1024)
			skippedCount++
			if *cleanRemote {
				_ = client.DeleteTask(t.ID)
			}
			continue
		}

		fmt.Printf("[%d/%d] ⬇️ 正在下载: %s (%.1f MB) -> %s\n",
			idx+1, len(completedTasks), outName, float64(t.OutputFileSize)/1024/1024, localDestPath)

		downloadErr := client.DownloadOutput(t.ID, localDestPath)
		if downloadErr != nil {
			log.Printf("  ❌ 下载失败: %v", downloadErr)
			failedCount++
			continue
		}

		downloadedCount++
		savedBytes := t.SourceFileSize - t.OutputFileSize
		if savedBytes > 0 {
			totalSavedBytes += savedBytes
		}

		origMB := float64(t.SourceFileSize) / 1024 / 1024
		compMB := float64(t.OutputFileSize) / 1024 / 1024
		pct := 0.0
		if origMB > 0 {
			pct = (1.0 - (compMB / origMB)) * 100
		}
		fmt.Printf("  ✅ 下载成功! 压缩效果: %.1f MB -> %.1f MB (体积节省: %.1f%%)\n", origMB, compMB, pct)

		if *cleanRemote {
			if err := client.DeleteTask(t.ID); err != nil {
				fmt.Printf("  ⚠️ 清理服务端任务失败: %v\n", err)
			} else {
				fmt.Printf("  🧹 已清理服务端临时任务与存储占用 (ID: %s)\n", t.ID)
			}
		}
	}

	elapsed := time.Since(startTime).Round(time.Second)
	fmt.Println("\n============================ 拉取结果汇总 ============================")
	fmt.Printf("总计: 成功下载 %d 个 | 跳过已存在 %d 个 | 失败 %d 个 | 耗时: %v\n",
		downloadedCount, skippedCount, failedCount, elapsed)
	if totalSavedBytes > 0 {
		fmt.Printf("本批次累计节省存储空间: %.2f GB\n", float64(totalSavedBytes)/1024/1024/1024)
	}
	fmt.Printf("成品保存目录: %s\n", *outputDir)
	fmt.Println("========================================================================")
}
