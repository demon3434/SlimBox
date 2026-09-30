package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

func runCleanCmd(args []string) {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	serverURL := fs.String("server", "http://localhost:8080", "SlimBox 服务端地址 (-s)")
	fs.StringVar(serverURL, "s", "http://localhost:8080", "SlimBox 服务端地址 (简写)")
	key := fs.String("key", "", "SlimBox API Key / 访问密钥 (-k)")
	fs.StringVar(key, "k", "", "SlimBox API Key (简写)")
	fs.StringVar(key, "token", "", "兼容别名: API Key / Token (--token)")
	fs.StringVar(key, "t", "", "兼容别名: API Key / Token (-t)")
	dryRun := fs.Bool("dry-run", false, "仅执行扫描检测并估算可释放空间，不执行实际物理删除")
	yes := fs.Bool("yes", false, "跳过交互式二次确认提示，直接执行清理 (-y)")
	fs.BoolVar(yes, "y", false, "跳过交互式二次确认提示，直接执行清理 (简写)")

	_ = fs.Parse(args)

	client := NewAPIClient(*serverURL, *key)

	fmt.Println("================================================================================")
	fmt.Printf("🧹 SlimBox 存储维护与残留文件清理工具\n")
	fmt.Printf("目标服务: %s\n", *serverURL)
	fmt.Println("================================================================================")
	fmt.Print("-> 正在扫描服务端存储挂载点 (检测脱节文件与废弃临时分片)... ")

	summary, err := client.ScanOrphans()
	if err != nil {
		fmt.Printf("\n❌ 扫描失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("完成!")

	if summary.TotalCount == 0 {
		fmt.Println("\n✨ 服务端存储环境健康，未检测到任何残留文件或临时分片残留。")
		return
	}

	fmt.Printf("\n📊 扫描结果汇总 (共检测到 %d 个脱节与缓存文件):\n", summary.TotalCount)
	fmt.Printf("  • 临时上传分片缓存 (uploads/temp): %.2f MB\n", float64(summary.TempChunkBytes)/1024/1024)
	fmt.Printf("  • 未关联脱节源视频 (uploads/):     %.2f MB\n", float64(summary.UnlinkedUploadBytes)/1024/1024)
	fmt.Printf("  • 未关联脱节成品视频 (outputs/):   %.2f MB\n", float64(summary.UnlinkedOutputBytes)/1024/1024)
	fmt.Printf("  • 遗留未完成转码残片 (*.part):     %.2f MB\n", float64(summary.StalePartialBytes)/1024/1024)
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("🔥 预计可释放总磁盘空间: %.2f MB\n", float64(summary.TotalOrphanBytes)/1024/1024)
	fmt.Println("--------------------------------------------------------------------------------")

	if *dryRun {
		fmt.Println("提示: 当前处于 --dry-run 预检模式，未执行任何物理文件删除。")
		return
	}

	if !*yes {
		fmt.Print("⚠️ 即将物理删除上述残留文件且不可恢复，是否确认清理? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("操作已取消。")
			return
		}
	}

	fmt.Print("-> 正在执行清理... ")
	deletedCount, freedBytes, err := client.CleanOrphans(nil)
	if err != nil {
		fmt.Printf("\n❌ 清理失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("完成!")
	fmt.Println("================================================================================")
	fmt.Printf("🎉 清理成功! 共删除 %d 个残留文件，释放存储空间: %.2f MB\n",
		deletedCount, float64(freedBytes)/1024/1024)
	fmt.Println("================================================================================")
}
