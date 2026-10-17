// Package config 负责集中读取运行期配置。
//
// 所有配置项都可以通过环境变量覆盖，便于容器化部署。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config 是 Libra 的全部运行期配置。
type Config struct {
	// Addr 是 HTTP 监听地址，例如 ":8080"。
	Addr string
	// DataDir 是持久化根目录，库文件（libra.db）与封面/图标都放在这里。
	DataDir string

	// 以下三个路径由 DataDir 派生，也可以在环境变量中单独覆盖。
	DBPath    string
	CoversDir string
	IconsDir  string

	// LibraryRoot 是"浏览目录"接口的根，用于在 Web 上挑选书库目录。
	// 为空表示不限制（允许浏览整个文件系统）。
	LibraryRoot string

	// ScanConcurrency 是扫描时并行的外部进程（pdftoppm/pdfimages）数量上限。
	ScanConcurrency int
	// CoverWidth 是 PDF 首页渲染的目标宽度（像素），也是封面图的缩放上限。
	CoverWidth int
	// ScanOnStart 为真时，进程启动后自动对所有书库做一次扫描。
	ScanOnStart bool
	// SessionTTL 是登录会话有效期。
	SessionTTL time.Duration
	// AllowRegister 允许非首位用户自助注册。第一个注册的用户必定是管理员。
	AllowRegister bool
	// TrustedProxies 传给 gin 的可信代理列表，为空表示不信任任何代理。
	TrustedProxies []string

	// WebDir 是开发模式下的前端资源目录（含 templates/ 与 static/）。
	// 为空表示使用 go:embed 打进二进制的那一份 —— 生产环境应当保持为空。
	// 设置后模板每次请求重新解析、静态资源直接读磁盘，改完刷新浏览器即可生效。
	WebDir string

	// DevStamp 是看护脚本写的时间戳文件，/__dev/reload 把它的修改时间当作版本号。
	// 为空时版本号恒为 0（不影响功能，只是不会自动刷新）。
	DevStamp string
}

// Load 读取环境变量并返回配置。
func Load() *Config {
	dataDir := abs(env("LIBRA_DATA_DIR", "./data"))

	c := &Config{
		Addr:            env("LIBRA_ADDR", ":8080"),
		DataDir:         dataDir,
		DBPath:          abs(env("LIBRA_DB_PATH", filepath.Join(dataDir, "libra.db"))),
		CoversDir:       abs(env("LIBRA_COVERS_DIR", filepath.Join(dataDir, "covers"))),
		IconsDir:        abs(env("LIBRA_ICONS_DIR", filepath.Join(dataDir, "icons"))),
		LibraryRoot:     env("LIBRA_LIBRARY_ROOT", ""),
		ScanConcurrency: envInt("LIBRA_SCAN_CONCURRENCY", 4),
		CoverWidth:      envInt("LIBRA_COVER_WIDTH", 600),
		ScanOnStart:     envBool("LIBRA_SCAN_ON_START", true),
		SessionTTL:      time.Duration(envInt("LIBRA_SESSION_TTL_HOURS", 24*30)) * time.Hour,
		AllowRegister:   envBool("LIBRA_ALLOW_REGISTER", true),
		TrustedProxies:  splitList(env("LIBRA_TRUSTED_PROXIES", "")),
		WebDir:          env("LIBRA_WEB_DIR", ""),
		DevStamp:        env("LIBRA_DEV_STAMP", ""),
	}

	if c.LibraryRoot != "" {
		c.LibraryRoot = abs(c.LibraryRoot)
	}
	if c.WebDir != "" {
		c.WebDir = abs(c.WebDir)
	}
	if c.DevStamp != "" {
		c.DevStamp = abs(c.DevStamp)
	}
	if c.ScanConcurrency < 1 {
		c.ScanConcurrency = 1
	}
	if c.ScanConcurrency > 32 {
		c.ScanConcurrency = 32
	}
	if c.CoverWidth < 100 {
		c.CoverWidth = 100
	}
	if c.CoverWidth > 2000 {
		c.CoverWidth = 2000
	}
	return c
}

// EnsureDirs 创建所有必要的目录。
func (c *Config) EnsureDirs() error {
	for _, dir := range []string{
		c.DataDir,
		filepath.Dir(c.DBPath),
		c.CoversDir,
		c.IconsDir,
	} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// LibraryCoversDir 返回某个书库的封面目录。
func (c *Config) LibraryCoversDir(libraryID int64) string {
	return filepath.Join(c.CoversDir, strconv.FormatInt(libraryID, 10))
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func envInt(key string, def int) int {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func splitList(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func abs(p string) string {
	if p == "" {
		return p
	}
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
