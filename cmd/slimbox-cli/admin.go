package main

import (
	"flag"
	"fmt"
	"os"

	"slimbox/internal/repository"
)

func runAdminCmd(args []string) {
	if len(args) < 1 {
		printAdminUsage()
		os.Exit(1)
	}

	subCmd := args[0]
	switch subCmd {
	case "reset-password":
		runAdminResetPassword(args[1:])
	default:
		fmt.Printf("未知 admin 子命令: %s\n", subCmd)
		printAdminUsage()
		os.Exit(1)
	}
}

func printAdminUsage() {
	fmt.Print(`SlimBox CLI 管理员控制台维护命令

用法:
  slimbox-cli admin <command> [arguments] [options]

命令:
  reset-password <new_password>   直接修改本地 SQLite 数据库重置 Web 端管理员主密码

选项:
  --db, -d <path>                 SQLite 数据库文件路径 (默认: ./data/slimbox.db)

示例:
  slimbox-cli admin reset-password "MyNewPass123"
  slimbox-cli admin reset-password "MyNewPass123" --db /opt/docker/slimbox/data/slimbox.db
`)
}

func runAdminResetPassword(args []string) {
	fs := flag.NewFlagSet("admin reset-password", flag.ExitOnError)
	dbPath := fs.String("db", "./data/slimbox.db", "SQLite 数据库文件路径 (-d)")
	fs.StringVar(dbPath, "d", "./data/slimbox.db", "SQLite 数据库文件路径 (简写)")

	_ = fs.Parse(args)

	newPassword := ""
	if fs.NArg() > 0 {
		newPassword = fs.Arg(0)
	}

	if newPassword == "" {
		fmt.Println("错误: 必须指定新的管理员密码。")
		fmt.Println("用法: slimbox-cli admin reset-password <new_password> [--db /path/to/slimbox.db]")
		os.Exit(1)
	}

	if len(newPassword) < 6 {
		fmt.Println("警告: 管理员密码长度建议不少于 6 位。")
	}

	db, err := repository.NewDB(*dbPath)
	if err != nil {
		fmt.Printf("错误: 无法打开数据库文件 %s: %v\n", *dbPath, err)
		os.Exit(1)
	}
	defer db.Close()

	adminRepo := repository.NewAdminRepository(db)
	if err := adminRepo.SetAdminPassword(newPassword); err != nil {
		fmt.Printf("错误: 重置管理员密码失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("==========================================================")
	fmt.Println("✓ 管理员主密码重置成功！")
	fmt.Printf("  数据库路径: %s\n", *dbPath)
	fmt.Println("  您现在可以使用新密码直接登录 Web 控制台。")
	fmt.Println("==========================================================")
}
