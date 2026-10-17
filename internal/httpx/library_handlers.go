package httpx

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"libra/internal/scanner"
	"libra/internal/store"
)

// ---------- 媒体库 CRUD ----------

func (s *Server) listLibraries(c *gin.Context) {
	libs, err := s.st.ListLibraries()
	if err != nil {
		failErr(c, 500, "读取媒体库失败", err)
		return
	}

	out := make([]gin.H, 0, len(libs))
	for _, lib := range libs {
		out = append(out, libraryJSON(&lib, s.sc.Status(lib.ID)))
	}
	ok(c, gin.H{"libraries": out})
}

func (s *Server) getLibrary(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	lib, err := s.st.GetLibrary(id)
	if err != nil {
		fail(c, 404, "媒体库不存在")
		return
	}
	ok(c, libraryJSON(lib, s.sc.Status(id)))
}

func (s *Server) createLibrary(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Icon string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Path = strings.TrimSpace(req.Path)
	if req.Name == "" {
		fail(c, 400, "请填写媒体库名称")
		return
	}
	if req.Path == "" {
		fail(c, 400, "请选择目录路径")
		return
	}
	if !s.pathAllowed(req.Path) {
		fail(c, 403, "该目录不在允许的根目录范围内")
		return
	}
	fi, err := os.Stat(req.Path)
	if err != nil {
		fail(c, 400, "目录不存在或不可访问: "+req.Path)
		return
	}
	if !fi.IsDir() {
		fail(c, 400, "路径不是目录: "+req.Path)
		return
	}
	if exists, err := s.st.HasLibraryWithPath(req.Path, 0); err != nil {
		failErr(c, 500, "校验目录失败", err)
		return
	} else if exists {
		fail(c, 409, "该目录已经被其它媒体库使用")
		return
	}

	lib, err := s.st.CreateLibrary(req.Name, req.Path, normalizeIcon(req.Icon))
	if err != nil {
		fail(c, 400, err.Error())
		return
	}

	// 新建后自动触发一次全量扫描。
	scanStarted := true
	if err := s.sc.Start(lib.ID); err != nil {
		scanStarted = false
	}

	created(c, gin.H{
		"library":      libraryJSON(lib, s.sc.Status(lib.ID)),
		"scan_started": scanStarted,
	})
}

func (s *Server) updateLibrary(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	old, err := s.st.GetLibrary(id)
	if err != nil {
		fail(c, 404, "媒体库不存在")
		return
	}

	var req struct {
		Name *string `json:"name"`
		Path *string `json:"path"`
		Icon *string `json:"icon"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}
	if req.Icon != nil {
		v := normalizeIcon(*req.Icon)
		req.Icon = &v
	}
	if req.Path != nil {
		p := strings.TrimSpace(*req.Path)
		if !s.pathAllowed(p) {
			fail(c, 403, "该目录不在允许的根目录范围内")
			return
		}
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() {
			fail(c, 400, "目录不存在或不是目录: "+p)
			return
		}
		if exists, err := s.st.HasLibraryWithPath(p, id); err != nil {
			failErr(c, 500, "校验目录失败", err)
			return
		} else if exists {
			fail(c, 409, "该目录已经被其它媒体库使用")
			return
		}
		req.Path = &p
	}

	if s.sc.IsRunning(id) && req.Path != nil && *req.Path != old.Path {
		fail(c, 409, "该媒体库正在扫描中，请稍后再修改目录")
		return
	}

	lib, err := s.st.UpdateLibrary(id, req.Name, req.Path, req.Icon)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}

	// 目录换了：清空旧记录与封面，然后重新扫描。
	rescanned := false
	if req.Path != nil && *req.Path != old.Path {
		_ = s.st.DeleteBooksByLibrary(id)
		_ = s.sc.RemoveLibraryCoversExcept(id, old.Cover)
		if err := s.sc.Start(id); err == nil {
			rescanned = true
		}
	}

	ok(c, gin.H{
		"library":        libraryJSON(lib, s.sc.Status(id)),
		"rescan_started": rescanned,
	})
}

func (s *Server) deleteLibrary(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	if s.sc.IsRunning(id) {
		fail(c, 409, "该媒体库正在扫描中，请等待扫描结束后再删除")
		return
	}
	lib, err := s.st.GetLibrary(id)
	if err != nil {
		fail(c, 404, "媒体库不存在")
		return
	}
	// 先删记录（外键级联），再删封面文件。
	if err := s.st.DeleteLibrary(id); err != nil {
		failErr(c, 500, "删除媒体库失败", err)
		return
	}
	if err := s.sc.RemoveLibraryCovers(id); err != nil {
		failErr(c, 500, "清理封面文件失败", err)
		return
	}
	ok(c, gin.H{"message": "媒体库「" + lib.Name + "」及其书籍记录、封面已全部清除"})
}

// ---------- 媒体库封面 ----------

// uploadLibraryCover 保存管理员上传的媒体库封面。
// 文件放在该库的封面目录里，用 _library- 前缀与书籍封面区分，
// 扫描清理书籍封面时不会误删。
func (s *Server) uploadLibraryCover(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	lib, err := s.st.GetLibrary(id)
	if err != nil {
		fail(c, 404, "媒体库不存在")
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		fail(c, 400, "请选择要上传的封面图片")
		return
	}
	if file.Size > 8<<20 {
		fail(c, 400, "封面图片不能超过 8MB")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".gif": true}
	if !allowed[ext] {
		fail(c, 400, "仅支持 png / jpg / webp / gif 格式")
		return
	}

	dir := s.cfg.LibraryCoversDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		failErr(c, 500, "创建封面目录失败", err)
		return
	}
	name := "_library-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	dst := filepath.Join(dir, name)
	if err := c.SaveUploadedFile(file, dst); err != nil {
		failErr(c, 500, "保存封面失败", err)
		return
	}
	if err := s.st.SetLibraryCover(id, name); err != nil {
		_ = os.Remove(dst)
		failErr(c, 500, "更新媒体库封面失败", err)
		return
	}
	// 新封面写入成功后再删旧文件
	if lib.Cover != "" && lib.Cover != name {
		_ = os.Remove(filepath.Join(dir, lib.Cover))
	}

	ok(c, gin.H{
		"cover_url": "/covers/" + strconv.FormatInt(id, 10) + "/" + url.PathEscape(name),
		"cover":     name,
	})
}

// deleteLibraryCover 清除自定义封面，恢复为自动拼贴。
func (s *Server) deleteLibraryCover(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	lib, err := s.st.GetLibrary(id)
	if err != nil {
		fail(c, 404, "媒体库不存在")
		return
	}
	if lib.Cover != "" {
		_ = os.Remove(filepath.Join(s.cfg.LibraryCoversDir(id), lib.Cover))
	}
	if err := s.st.SetLibraryCover(id, ""); err != nil {
		failErr(c, 500, "清除媒体库封面失败", err)
		return
	}
	ok(c, gin.H{"message": "已恢复自动封面"})
}

// ---------- 扫描 ----------

func (s *Server) scanLibrary(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	if err := s.sc.Start(id); err != nil {
		if err == scanner.ErrAlreadyRunning {
			fail(c, 409, err.Error())
			return
		}
		fail(c, 404, "媒体库不存在: "+err.Error())
		return
	}
	ok(c, gin.H{"message": "扫描已启动", "status": s.sc.Status(id)})
}

func (s *Server) scanStatus(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	st := s.sc.Status(id)
	if st == nil {
		// 没有内存中的进度，就回落到数据库里记录的最后一次结果。
		lib, err := s.st.GetLibrary(id)
		if err != nil {
			fail(c, 404, "媒体库不存在")
			return
		}
		state := lib.LastScanState
		if state == "" {
			state = scanner.StateIdle
		}
		ok(c, gin.H{"status": gin.H{
			"library_id":  id,
			"state":       state,
			"finished_at": lib.LastScanAt,
		}})
		return
	}
	ok(c, gin.H{"status": st})
}

// ---------- 辅助 ----------

func libraryJSON(lib *store.Library, st *scanner.Status) gin.H {
	out := gin.H{
		"id":              lib.ID,
		"name":            lib.Name,
		"path":            lib.Path,
		"icon":            lib.Icon,
		"created_at":      lib.CreatedAt,
		"last_scan_at":    lib.LastScanAt,
		"last_scan_state": lib.LastScanState,
		"book_count":      lib.BookCount,
		"epub_count":      lib.EpubCount,
		"pdf_count":       lib.PdfCount,
		"scanning":        false,
	}
	if lib.Cover != "" {
		out["cover_url"] = "/covers/" + strconv.FormatInt(lib.ID, 10) + "/" + url.PathEscape(lib.Cover)
	}
	if st != nil {
		out["scanning"] = st.State == scanner.StateScanning
		out["scan"] = st
	}
	return out
}

// normalizeIcon 规范图标标识，非法值回落到默认图标。
func normalizeIcon(icon string) string {
	icon = strings.TrimSpace(icon)
	switch {
	case icon == "":
		return "builtin:book"
	case strings.HasPrefix(icon, "builtin:"):
		name := strings.TrimPrefix(icon, "builtin:")
		if name == "" || strings.ContainsAny(name, "/\\.") {
			return "builtin:book"
		}
		return "builtin:" + name
	case strings.HasPrefix(icon, "upload:"):
		name := filepath.Base(strings.TrimPrefix(icon, "upload:"))
		if name == "" || name == "." || strings.Contains(name, "..") {
			return "builtin:book"
		}
		return "upload:" + name
	default:
		return "builtin:book"
	}
}
