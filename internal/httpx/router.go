// Package httpx 提供 HTTP 路由、鉴权中间件与 REST 接口。
package httpx

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/render"

	"libra/internal/config"
	"libra/internal/scanner"
	"libra/internal/store"
	"libra/web"
)

// templateFuncs 是模板里可用的函数集合。
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"year": func() int { return time.Now().Year() },
	}
}

// devRenderer 在开发模式下接管 HTML 渲染：每次请求都从磁盘重新解析模板，
// 于是改 html 后刷新浏览器即可看到，不必重启进程（生产环境不使用它）。
type devRenderer struct {
	server *Server
}

// Instance 按 gin.HTMLRender 接口返回渲染实例。
func (d *devRenderer) Instance(name string, data any) render.Render {
	return render.HTML{Template: d.server.templates(), Name: name, Data: data}
}

// templates 解析磁盘上的模板；解析失败时回落到上一次成功的版本，
// 这样编辑到一半的模板不会让整个界面变成 500。
func (s *Server) templates() *template.Template {
	if s.devFS == nil {
		return s.devTpl
	}
	s.devMu.Lock()
	defer s.devMu.Unlock()
	if tpl, err := template.New("libra").Funcs(templateFuncs()).ParseFS(s.devFS, "templates/*.html"); err == nil {
		s.devTpl = tpl
	} else {
		log.Printf("[dev] 模板解析失败，继续使用上一版：%v", err)
	}
	return s.devTpl
}

// devReload 只在开发模式下挂载：返回一个随文件改动变化的时间戳。
// 前端脚本拿到它以后，版本一变就 location.reload() —— 于是改完 CSS/JS/模板
// 不用手动按 F5。生产环境这个路由根本不存在。
func (s *Server) devReload(c *gin.Context) {
	version := int64(0)
	stamp := strings.TrimSpace(s.cfg.DevStamp)
	if stamp != "" {
		if st, err := os.Stat(stamp); err == nil {
			version = st.ModTime().UnixMilli()
		}
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": version})
}

// Server 持有路由所需的全部依赖。
type Server struct {
	cfg        *config.Config
	st         *store.Store
	sc         *scanner.Scanner
	loginGuard *loginLimiter
	// devFS 非空时表示开发模式：模板与静态资源改读磁盘（cfg.WebDir），
	// 改完只要刷新浏览器就能看到，不必重启进程。生产环境为 nil，走 go:embed。
	devFS  fs.FS
	devMu  sync.Mutex
	devTpl *template.Template
}

// NewRouter 组装完整路由。
func NewRouter(cfg *config.Config, st *store.Store, sc *scanner.Scanner) (*gin.Engine, error) {
	s := &Server{cfg: cfg, st: st, sc: sc, loginGuard: newLoginLimiter()}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), securityHeaders())
	r.MaxMultipartMemory = 8 << 20 // 图标上传最大 8MB

	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("设置可信代理失败: %w", err)
	}

	// 资源来源：默认用内嵌 FS；设置 LIBRA_WEB_DIR 时改用磁盘目录。
	var assets fs.FS = web.FS
	if cfg.WebDir != "" {
		if _, err := os.Stat(cfg.WebDir); err != nil {
			return nil, fmt.Errorf("LIBRA_WEB_DIR 不可用: %w", err)
		}
		assets = os.DirFS(cfg.WebDir)
		s.devFS = assets
		log.Printf("[dev] 开发模式：模板与静态资源改读 %s（改动即时生效）", cfg.WebDir)
	}

	tpl, err := template.New("libra").Funcs(templateFuncs()).ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("解析模板失败: %w", err)
	}
	r.SetHTMLTemplate(tpl)
	s.devTpl = tpl

	// 开发模式下模板每次请求重新解析（函数指针由 HTMLRender 接管），
	// 并开一个 /__dev/reload 给浏览器轮询版本号，实现"改完自动刷新"。
	if s.devFS != nil {
		r.HTMLRender = &devRenderer{server: s}
		r.GET("/__dev/reload", s.devReload)
	}

	// ---- 无需登录 ----
	staticFS, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	r.GET("/static/*filepath", serveStatic(http.FS(staticFS), "/static"))
	r.GET("/healthz", s.health)
	r.GET("/login", s.pageLogin)
	r.GET("/setup", func(c *gin.Context) { c.Redirect(http.StatusFound, "/login") })

	r.POST("/api/auth/login", s.login)
	r.POST("/api/auth/register", s.register)
	r.GET("/api/auth/me", s.me)

	// ---- 需要登录（只读能力，普通成员也可用）----
	auth := r.Group("", s.requireAuth())

	auth.GET("/", s.pageApp)
	auth.POST("/api/auth/logout", s.logout)
	auth.POST("/api/auth/password", s.changePassword)

	// 封面与上传图标（同样要求登录，避免资源被匿名抓取）
	auth.GET("/covers/*filepath", serveFilesAuth(http.Dir(cfg.CoversDir), "/covers"))
	auth.GET("/thumbs/*filepath", s.serveThumb)
	auth.GET("/icons/*filepath", serveFilesAuth(http.Dir(cfg.IconsDir), "/icons"))

	auth.GET("/api/status", s.status)
	auth.GET("/api/icons", s.listIcons)

	auth.GET("/api/libraries", s.listLibraries)
	auth.GET("/api/libraries/:id", s.getLibrary)
	auth.GET("/api/libraries/:id/scan", s.scanStatus) // 只读扫描进度

	auth.GET("/api/books", s.listBooks)
	auth.GET("/api/featured", s.featured) // 首页横幅精选
	auth.GET("/api/books/:id", s.getBook)
	auth.GET("/download/:id", s.downloadBook)

	// ---- 需要管理员 ----
	// 所有会改动服务端状态、或能读到服务器文件系统的接口都收在这里。
	admin := auth.Group("", s.adminOnly())

	admin.POST("/api/libraries", s.createLibrary)
	admin.PUT("/api/libraries/:id", s.updateLibrary)
	admin.DELETE("/api/libraries/:id", s.deleteLibrary)
	admin.POST("/api/libraries/:id/cover", s.uploadLibraryCover)
	admin.DELETE("/api/libraries/:id/cover", s.deleteLibraryCover)
	admin.POST("/api/libraries/:id/scan", s.scanLibrary)

	admin.GET("/api/browse", s.browse) // 目录浏览：能枚举服务器目录，仅管理员
	admin.POST("/api/icons", s.uploadIcon)

	admin.GET("/api/users", s.listUsers)
	admin.POST("/api/users", s.createUser)
	admin.PUT("/api/users/:id/role", s.setUserRole)
	admin.PUT("/api/users/:id/password", s.resetUserPassword)
	admin.DELETE("/api/users/:id", s.deleteUser)

	return r, nil
}

// serveStatic 提供内嵌静态资源（无需登录）。
func serveStatic(fsys http.FileSystem, prefix string) gin.HandlerFunc {
	fileServer := http.StripPrefix(prefix, http.FileServer(fsys))
	return func(c *gin.Context) {
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

// serveFilesAuth 安全地提供用户上传/生成的目录内容：禁止目录列表、禁止路径穿越。
func serveFilesAuth(dir http.FileSystem, prefix string) gin.HandlerFunc {
	strip := http.StripPrefix(prefix, http.FileServer(dir))
	return func(c *gin.Context) {
		rel := strings.TrimPrefix(c.Param("filepath"), "/")
		// 文件名都是哈希或已净化过的名字，出现 .. 一律视为攻击。
		if rel == "" || strings.Contains(rel, "..") {
			c.Status(http.StatusNotFound)
			return
		}
		clean := path.Clean("/" + rel)
		f, err := dir.Open(clean)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil || info.IsDir() {
			c.Status(http.StatusNotFound)
			return
		}

		// 静态封面/图标可以放心长缓存，文件名是内容哈希或上传时间派生的。
		if !strings.HasPrefix(clean, "/.tmp") {
			c.Header("Cache-Control", "private, max-age=604800")
		}
		strip.ServeHTTP(c.Writer, c.Request)
	}
}

// securityHeaders 加上一组基础安全响应头。
// 前端不再引用任何 CDN，所以 CSP 可以收得很紧：只允许同源脚本与样式。
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+ // 主题脚本内联在 <head>，需放行 inline
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"connect-src 'self'; "+
				"font-src 'self' data:; "+
				"object-src 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self'; "+
				"frame-ancestors 'self'")
		c.Next()
	}
}
