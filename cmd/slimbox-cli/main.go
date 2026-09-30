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
	case "push":
		runPushCmd(os.Args[2:])
	case "pull":
		runPullCmd(os.Args[2:])
	case "batch":
		runBatchCmd(os.Args[2:])
	case "status":
		runStatusCmd(os.Args[2:])
	case "abort":
		runAbortCmd(os.Args[2:])
	case "retry":
		runRetryCmd(os.Args[2:])
	case "top":
		runTopCmd(os.Args[2:])
	case "bottom":
		runBottomCmd(os.Args[2:])
	case "move":
		runMoveCmd(os.Args[2:])
	case "admin":
		runAdminCmd(os.Args[2:])
	case "clean":
		runCleanCmd(os.Args[2:])
	case "version":
		fmt.Printf("slimbox-cli version %s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`SlimBox CLI - 跨平台视频压缩批处理客户端

用法:
  slimbox-cli <command> [options] [arguments]

核心命令:
  push      批量上传视频并加入服务端转码队列后即退 (支持关机脱机运行)
  pull      随时拉取所有已转码完成的成品到本地 (支持自动清理服务端)
  status    查询当前 SlimBox 服务状态、排队情况、任务 ID 与转码进度
  clean     扫描并彻底清理服务端残留文件与废弃临时分片 (支持 --dry-run)
  abort     立即终止指定转码任务并清理未完成残片 (slimbox-cli abort <task-id>)
  retry     重试或继续已终止/失败的任务 (slimbox-cli retry <task-id>)
  top       一键将排队任务置顶 (最高优先级) (slimbox-cli top <task-id>)
  bottom    一键将排队任务置底 (最低优先级) (slimbox-cli bottom <task-id>)
  move      调整排队任务优先级 (-d top|bottom|up|down <task-id>)
  admin     管理员本地维护命令 (如重置 Web 主密码)
  version   查看 CLI 版本

示例 (服务端 IP 假设为 192.168.1.100:8080):
  # 1. 批量推送入队并脱机 (支持直接指定多个文件)
  slimbox-cli push -s "http://192.168.1.100:8080" -p 720p movie1.mkv movie2.mp4

  # 2. 查询排队与当前转码进度 (获取任务 ID)
  slimbox-cli status -s "http://192.168.1.100:8080"

  # 3. 终止指定任务 (支持直接传入任务 ID)
  slimbox-cli abort -s "http://192.168.1.100:8080" <task-id>

  # 4. 继续/重试已终止或失败的任务
  slimbox-cli retry -s "http://192.168.1.100:8080" <task-id>

  # 5. 调整任务顺序 (置顶 / 置底)
  slimbox-cli top -s "http://192.168.1.100:8080" <task-id>
  slimbox-cli bottom -s "http://192.168.1.100:8080" <task-id>

  # 6. 随时拉取所有转码完成的成品并清理服务端
  slimbox-cli pull -s "http://192.168.1.100:8080" -o "D:\Done" --clean

  # 7. 若服务端开启安全鉴权，使用 -k (或 --key) 传入 API Key:
  slimbox-cli status -s "http://192.168.1.100:8080" -k "YOUR_API_KEY"
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
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	stateFile := fs.String("state-file", "", "断点续传状态清单文件路径")
	cleanRemote := fs.Bool("clean-remote", true, "下载完成后自动清理服务端的临时任务与文件")

	_ = fs.Parse(args)

	if *inputDir == "" && *listFile == "" {
		fmt.Println("错误: 必须指定待处理目录 (--input-dir / -d) 或文件清单 (--list / -l)")
		fs.Usage()
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
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
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	client := NewAPIClient(*serverURL, *key)
	tasks, err := client.ListTasks()
	if err != nil {
		fmt.Printf("获取服务状态失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("================== SlimBox 任务队列概览 ==================")
	activeCount := 0
	queuedCount := 0
	abortedCount := 0
	completedCount := 0

	for _, t := range tasks {
		switch t.Status {
		case "transcoding":
			activeCount++
			fmt.Printf("▶ [正在转码] [ID: %s] %s (档位: %s) -> %.1f%% (FPS: %.1f, ETA: %ds)\n",
				t.ID, t.SourceFileName, t.Params.ProfileName, t.Progress.Percent, t.Progress.CurrentFPS, t.Progress.ETASeconds)
		case "queued":
			queuedCount++
			fmt.Printf("⏳ [等待排队] [ID: %s] %s (档位: %s, 优先级: %d)\n", t.ID, t.SourceFileName, t.Params.ProfileName, t.Priority)
		case "aborted":
			abortedCount++
			fmt.Printf("⏹ [已中止]   [ID: %s] %s\n", t.ID, t.SourceFileName)
		case "failed":
			fmt.Printf("❌ [已失败]   [ID: %s] %s (错误: %s)\n", t.ID, t.SourceFileName, t.ErrorMsg)
		case "completed":
			completedCount++
		}
	}

	if activeCount == 0 && queuedCount == 0 && abortedCount == 0 {
		fmt.Println("服务当前处于空闲状态，无待处理任务。")
	}
	fmt.Printf("总计: 正在转码 %d 个 | 等待中 %d 个 | 已中止 %d 个 | 历史完成 %d 个\n", activeCount, queuedCount, abortedCount, completedCount)
	fmt.Println("==========================================================")
}

func parseTaskID(fs *flag.FlagSet, flagVal *string) string {
	if flagVal != nil && *flagVal != "" {
		return *flagVal
	}
	if fs.NArg() > 0 {
		return fs.Arg(0)
	}
	return ""
}

func runAbortCmd(args []string) {
	fs := flag.NewFlagSet("abort", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需中止的任务 ID")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	targetID := parseTaskID(fs, taskID)
	if targetID == "" {
		fmt.Println("错误: 必须指定任务 ID，用法: slimbox-cli abort -s \"...\" <task-id>")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	if err := client.AbortTask(targetID); err != nil {
		fmt.Printf("中止任务失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已成功终止任务 %s，CPU 已释放并清理未完成残片。\n", targetID)
}

func runRetryCmd(args []string) {
	fs := flag.NewFlagSet("retry", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需重试/继续的任务 ID")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	targetID := parseTaskID(fs, taskID)
	if targetID == "" {
		fmt.Println("错误: 必须指定任务 ID，用法: slimbox-cli retry -s \"...\" <task-id>")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	if err := client.RetryTask(targetID); err != nil {
		fmt.Printf("重试/继续任务失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已成功重试任务 %s，已重新推入转码队列等待执行。\n", targetID)
}

func runTopCmd(args []string) {
	fs := flag.NewFlagSet("top", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需置顶的任务 ID")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	targetID := parseTaskID(fs, taskID)
	if targetID == "" {
		fmt.Println("错误: 必须指定任务 ID，用法: slimbox-cli top -s \"...\" <task-id>")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	if err := client.AdjustPriority(targetID, "top"); err != nil {
		fmt.Printf("置顶任务失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已成功将任务 %s 置顶 (最高排队优先级)。\n", targetID)
}

func runBottomCmd(args []string) {
	fs := flag.NewFlagSet("bottom", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需置底的任务 ID")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	targetID := parseTaskID(fs, taskID)
	if targetID == "" {
		fmt.Println("错误: 必须指定任务 ID，用法: slimbox-cli bottom -s \"...\" <task-id>")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	if err := client.AdjustPriority(targetID, "bottom"); err != nil {
		fmt.Printf("置底任务失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已成功将任务 %s 置底 (最低排队优先级)。\n", targetID)
}

func runMoveCmd(args []string) {
	fs := flag.NewFlagSet("move", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务地址 (简写)")
	taskID := fs.String("task-id", "", "需调整的任务 ID")
	direction := fs.String("direction", "top", "移动方向 (top, bottom, up, down) (-d)")
	fs.StringVar(direction, "d", "top", "移动方向 (简写)")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	_ = fs.Parse(args)

	targetID := parseTaskID(fs, taskID)
	if targetID == "" {
		fmt.Println("错误: 必须指定任务 ID，用法: slimbox-cli move -s \"...\" -d <top|bottom|up|down> <task-id>")
		os.Exit(1)
	}

	client := NewAPIClient(*serverURL, *key)
	if err := client.AdjustPriority(targetID, *direction); err != nil {
		fmt.Printf("调整任务顺序失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ 已成功将任务 %s 方向调整为: %s\n", targetID, *direction)
}
