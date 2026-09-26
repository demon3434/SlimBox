package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "batch":
		runBatchCmd(os.Args[2:])
	case "status":
		runStatusCmd(os.Args[2:])
	case "abort":
		runAbortCmd(os.Args[2:])
	case "version":
		fmt.Printf("slimbox-cli version %s\n", version)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`SlimBox CLI - 非图形化视频批量压缩客户端

用法:
  slimbox-cli <command> [arguments]

可用命令:
  batch     执行批量视频上传、转码与自动下载任务
  status    查询当前 SlimBox 服务状态与排队情况
  abort     立即终止指定正在运行的转码任务并清理残片
  version   查看 CLI 版本

示例:
  # 批量压缩本地目录中的所有视频（默认 720p H.265，自动下载至 ./output）
  slimbox-cli batch -s "http://192.168.1.100:8080" -d "D:\Movies" -o "D:\Compressed" --profile 720p

  # 使用清单文本文件批量压缩
  slimbox-cli batch -s "http://192.168.1.100:8080" -l "series.txt" -o "D:\Compressed" --profile 1080p

  # 查询服务状态
  slimbox-cli status -s "http://192.168.1.100:8080"
`)
}

func runBatchCmd(args []string) {
	fs := flag.NewFlagSet("batch", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	inputDir := fs.String("input-dir", "", "待压缩本地视频目录 (-d)")
	fs.StringVar(inputDir, "d", "", "待压缩本地视频目录 (简写)")
	listFile := fs.String("list", "", "待压缩视频路径清单文件 (-l)")
	fs.StringVar(listFile, "l", "", "待压缩视频路径清单文件 (简写)")
	outputDir := fs.String("output-dir", "./compressed", "压缩成品下载保存目录 (-o)")
	fs.StringVar(outputDir, "o", "./compressed", "压缩成品下载保存目录 (简写)")
	profile := fs.String("profile", "720p", "压缩预设枚举档位 (360p, 480p, 720p, 1080p, 2k, 4k)")
	fs.StringVar(profile, "p", "720p", "压缩预设枚举档位 (简写)")
	codec := fs.String("codec", "libx265", "视频编码器覆盖 (libx265 / libx264)")
	token := fs.String("token", "", "SlimBox API Token / 访问密码 (-t)")
	fs.StringVar(token, "t", "", "SlimBox API Token (简写)")
	stateFile := fs.String("state-file", "", "断点续传状态清单文件路径")
	cleanRemote := fs.Bool("clean-remote", true, "下载完成后自动清理服务端的临时任务与文件")

	_ = fs.Parse(args)

	if *inputDir == "" && *listFile == "" {
		fmt.Println("错误: 必须指定待处理目录 (--input-dir / -d) 或文件清单 (--list / -l)")
		fs.Usage()
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *token)
	err := RunBatch(client, *inputDir, *listFile, *outputDir, *profile, *codec, *stateFile, *cleanRemote)
	if err != nil {
		fmt.Printf("批量任务执行异常: %v\n", err)
		os.Exit(1)
	}
}

func runStatusCmd(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	token := fs.String("token", "", "SlimBox API Token (-t)")
	fs.StringVar(token, "t", "", "SlimBox API Token (简写)")
	_ = fs.Parse(args)

	client := NewAPIClient(*serverURL, *token)
	tasks, err := client.ListTasks()
	if err != nil {
		fmt.Printf("获取服务状态失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================== SlimBox 任务队列概览 ==================")
	activeCount := 0
	queuedCount := 0
	completedCount := 0

	for _, t := range tasks {
		switch t.Status {
		case "transcoding":
			activeCount++
			fmt.Printf("▶ [正在转码] %s (档位: %s) -> %.1f%% (FPS: %.1f, ETA: %ds)\n",
				t.SourceFileName, t.Params.ProfileName, t.Progress.Percent, t.Progress.CurrentFPS, t.Progress.ETASeconds)
		case "queued":
			queuedCount++
			fmt.Printf("⏳ [等待中] %s (档位: %s)\n", t.SourceFileName, t.Params.ProfileName)
		case "completed":
			completedCount++
		}
	}

	if activeCount == 0 && queuedCount == 0 {
		fmt.Println("服务当前处于空闲状态，无排队转码任务。")
	}
	fmt.Printf("总计: 正在转码 %d 个 | 等待中 %d 个 | 历史完成 %d 个\n", activeCount, queuedCount, completedCount)
	fmt.Println("==========================================================")
}

func runAbortCmd(args []string) {
	fs := flag.NewFlagSet("abort", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需中止的任务 ID")
	token := fs.String("token", "", "SlimBox API Token (-t)")
	fs.StringVar(token, "t", "", "SlimBox API Token (简写)")
	_ = fs.Parse(args)

	if *taskID == "" {
		fmt.Println("错误: 必须指定 --task-id")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *token)
	if err := client.AbortTask(*taskID); err != nil {
		fmt.Printf("中止任务失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("已成功向任务 %s 发送终止信号，CPU 已释放并清理未完成残片。\n", *taskID)
}
