package httpx

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Version 由 main 包在启动时注入。
var Version = "dev"

// builtinIcons 是内置图标名列表，需与前端 static/app.js 中的 ICONS 保持一致。
var builtinIcons = []string{
	"book", "books", "library", "star", "heart", "globe",
	"moon", "sun", "leaf", "flame", "cube", "music",
	"film", "folder", "compass", "sparkle", "shield", "cloud",
}

// health 供容器编排做探活。
func (s *Server) health(c *gin.Context) {
	if err := s.st.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "version": Version})
}

// status 返回首页概览数据。
func (s *Server) status(c *gin.Context) {
	stat, err := s.st.Stats(0)
	if err != nil {
		failErr(c, 500, "读取统计失败", err)
		return
	}
	libs, err := s.st.ListLibraries()
	if err != nil {
		failErr(c, 500, "读取书库失败", err)
		return
	}

	scanning := make([]int64, 0, 2)
	for _, l := range libs {
		if s.sc.IsRunning(l.ID) {
			scanning = append(scanning, l.ID)
		}
	}

	ok(c, gin.H{
		"version":       Version,
		"stats":         stat,
		"library_count": len(libs),
		"scanning":      scanning,
		"pdf_support":   s.sc.PDFSupport(),
		"data_dir":      s.cfg.DataDir,
	})
}

// listIcons 返回内置图标集合与已上传的图标。
func (s *Server) listIcons(c *gin.Context) {
	uploaded := make([]string, 0, 8)
	entries, err := os.ReadDir(s.cfg.IconsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			uploaded = append(uploaded, e.Name())
		}
	}
	sort.Strings(uploaded)

	uploads := make([]gin.H, 0, len(uploaded))
	for _, name := range uploaded {
		uploads = append(uploads, gin.H{
			"name": name,
			"id":   "upload:" + name,
			"url":  "/icons/" + name,
		})
	}

	ok(c, gin.H{
		"builtin":  builtinIcons,
		"uploaded": uploads,
	})
}

// uploadIcon 接收用户上传的图标文件。
func (s *Server) uploadIcon(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, "请选择要上传的图标文件")
		return
	}
	if file.Size > 2<<20 {
		fail(c, 400, "图标文件不能超过 2MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
		".webp": true, ".svg": true, ".bmp": true, ".ico": true,
	}
	if !allowed[ext] {
		fail(c, 400, "仅支持 png / jpg / gif / webp / svg / bmp / ico 格式")
		return
	}

	base := sanitizeName(strings.TrimSuffix(filepath.Base(file.Filename), ext))
	if base == "" {
		base = "icon"
	}
	name := fmt.Sprintf("%d-%s%s", time.Now().Unix(), base, ext)
	dst := filepath.Join(s.cfg.IconsDir, name)

	if err := os.MkdirAll(s.cfg.IconsDir, 0o755); err != nil {
		failErr(c, 500, "创建图标目录失败", err)
		return
	}
	if err := c.SaveUploadedFile(file, dst); err != nil {
		failErr(c, 500, "保存图标失败", err)
		return
	}

	ok(c, gin.H{"id": "upload:" + name, "name": name, "url": "/icons/" + name})
}

// browse 列出目录下的子目录，用于在 Web 上挑选书库路径。
func (s *Server) browse(c *gin.Context) {
	target := strings.TrimSpace(c.Query("path"))
	root := s.cfg.LibraryRoot

	if target == "" {
		target = root
		if target == "" {
			target = defaultBrowseRoot()
		}
	}
	target = filepath.Clean(target)

	if !s.pathAllowed(target) {
		fail(c, 403, fmt.Sprintf("只能浏览 %s 以内的目录", root))
		return
	}

	entries, err := os.ReadDir(target)
	if err != nil {
		fail(c, 400, "无法读取目录: "+err.Error())
		return
	}

	dirs := make([]gin.H, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		child := filepath.Join(target, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, gin.H{
			"name":       e.Name(),
			"path":       child,
			"mod_time":   info.ModTime(),
			"readable":   readable(child),
			"is_library": false,
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i]["name"].(string)) < strings.ToLower(dirs[j]["name"].(string))
	})

	parent := filepath.Dir(target)
	if parent == target || !s.pathAllowed(parent) {
		parent = ""
	}

	ok(c, gin.H{
		"path":   target,
		"parent": parent,
		"root":   root,
		"roots":  candidateRoots(),
		"dirs":   dirs,
	})
}

// pathAllowed 判断路径是否在允许的浏览根之内。
func (s *Server) pathAllowed(p string) bool {
	if s.cfg.LibraryRoot == "" {
		return true // 未设置限制，允许浏览本机文件系统（家庭 NAS 场景）
	}
	root := filepath.Clean(s.cfg.LibraryRoot)
	abs := filepath.Clean(p)
	if abs == root {
		return true
	}
	return strings.HasPrefix(abs, root+string(filepath.Separator))
}

// defaultBrowseRoot 在未配置 LIBRA_LIBRARY_ROOT 时给出一个合理的起点。
func defaultBrowseRoot() string {
	for _, p := range candidateRoots() {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return string(filepath.Separator)
}

// candidateRoots 返回常见的书籍存放位置，方便前端做快捷入口。
func candidateRoots() []string {
	if runtime.GOOS == "windows" {
		out := make([]string, 0, 8)
		for c := 'C'; c <= 'Z'; c++ {
			p := fmt.Sprintf("%c:\\", c)
			if fi, err := os.Stat(p); err == nil && fi.IsDir() {
				out = append(out, p)
			}
		}
		return out
	}
	candidates := []string{"/", "/data", "/media", "/mnt", "/srv", "/volume1", "/vol1", "/share", "/mnt/user", "/mnt/user/books"}
	out := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func readable(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// sanitizeName 只保留文件名安全字符。
func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), ".-_")
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
