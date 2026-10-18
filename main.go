// Libra —— 运行在 NAS 上的轻量电子书展示与下载服务。
//
// 单二进制 + 单 SQLite 文件，零外部服务依赖；仅 PDF 封面渲染需要容器内的 poppler-utils。
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"libra/internal/auth"
	"libra/internal/config"
	"libra/internal/httpx"
	"libra/internal/scanner"
	"libra/internal/store"
)

// version 会在构建时通过 -ldflags 覆盖。
var version = "1.0.1"

const usage = `Libra - 轻量电子书管理系统

用法:
  libra                     启动服务
  libra reset-password <用户名> <新密码>
                            重置某个账号的密码（忘记密码时使用）
  libra version             打印版本号

常用环境变量:
  LIBRA_ADDR              监听地址，默认 ":8080"
  LIBRA_DATA_DIR          数据目录（数据库 + 封面），默认 "./data"
  LIBRA_LIBRARY_ROOT      允许浏览/添加的根目录，默认不限制
  LIBRA_SCAN_CONCURRENCY  PDF 封面渲染并发数，默认 4
  LIBRA_COVER_WIDTH       PDF 封面渲染宽度，默认 600
  LIBRA_SCAN_ON_START     启动时自动扫描，默认 true
  LIBRA_ALLOW_REGISTER    允许自助注册，默认 true
  LIBRA_SESSION_TTL_HOURS 会话有效期（小时），默认 720
`

func main() {
	log.SetFlags(log.LstdFlags)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-password":
			resetPassword(os.Args[2:])
			return
		case "version", "-v", "--version":
			fmt.Printf("libra %s\n", version)
			return
		case "help", "-h", "--help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "未知命令: %s\n\n%s", os.Args[1], usage)
			os.Exit(2)
		}
	}

	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("初始化目录失败: %v", err)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.Migrate(); err != nil {
		log.Fatalf("初始化数据库结构失败: %v", err)
	}

	sc := scanner.New(st, cfg)

	httpVersion := version
	httpx.Version = httpVersion

	engine, err := httpx.NewRouter(cfg, st, sc)
	if err != nil {
		log.Fatalf("初始化路由失败: %v", err)
	}

	if !sc.PDFSupport() {
		log.Printf("[warn] 未检测到 pdftoppm，PDF 将无法生成封面（请安装 poppler-utils）")
	}

	// 启动后自动做一次增量扫描，保证索引与磁盘一致。
	if cfg.ScanOnStart {
		go sc.StartAll()
	}

	log.Printf("Libra %s 启动成功", version)
	log.Printf("  监听地址: %s", cfg.Addr)
	log.Printf("  数据目录: %s", cfg.DataDir)
	log.Printf("  数据库:   %s", cfg.DBPath)
	log.Printf("  扫描并发: %d  封面宽度: %dpx", cfg.ScanConcurrency, cfg.CoverWidth)

	if err := engine.Run(cfg.Addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// resetPassword 实现 `libra reset-password <用户名> <新密码>`。
func resetPassword(args []string) {
	if len(args) < 2 {
		fmt.Fprint(os.Stderr, "用法: libra reset-password <用户名> <新密码>\n")
		os.Exit(2)
	}
	username := strings.TrimSpace(args[0])
	newPassword := args[1]

	cfg := config.Load()
	if err := cfg.EnsureDirs(); err != nil {
		log.Fatalf("初始化目录失败: %v", err)
	}
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := auth.ValidatePassword(username, newPassword); err != nil {
		log.Fatalf("密码不符合要求: %v", err)
	}
	u, err := st.GetUserByUsername(username)
	if err != nil {
		log.Fatalf("账号 %s 不存在", username)
	}
	hash, err := auth.HashPassword(u.Username, newPassword)
	if err != nil {
		log.Fatalf("生成密码哈希失败: %v", err)
	}
	if err := st.UpdateUserPassword(u.ID, hash); err != nil {
		log.Fatalf("更新密码失败: %v", err)
	}
	_ = st.DeleteUserSessions(u.ID)
	fmt.Printf("账号 %s 的密码已重置，请使用新密码登录。\n", u.Username)
}
