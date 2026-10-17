package httpx

import (
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"libra/internal/store"
)

// listBooks 分页列出书籍，支持按书库、关键字、格式过滤与排序。
func (s *Server) listBooks(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "page_size", 60)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 60
	}
	if pageSize > 300 {
		pageSize = 300
	}

	q := store.BookQuery{
		LibraryID: int64(queryInt(c, "library_id", 0)),
		Search:    strings.TrimSpace(c.Query("q")),
		Format:    strings.ToLower(strings.TrimSpace(c.Query("format"))),
		Sort:      strings.ToLower(strings.TrimSpace(c.Query("sort"))),
		Desc:      strings.EqualFold(c.Query("order"), "desc"),
		Page:      page,
		PageSize:  pageSize,
	}

	start := time.Now()
	books, total, err := s.st.ListBooks(q)
	if err != nil {
		failErr(c, 500, "查询书籍失败", err)
		return
	}
	// elapsed_ms 给封面墙页脚用，顺便也是排查慢查询最省事的那个数字。
	elapsedMS := math.Round(float64(time.Since(start).Microseconds())/100) / 10

	items := make([]gin.H, 0, len(books))
	for i := range books {
		items = append(items, bookJSON(&books[i]))
	}

	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	ok(c, gin.H{
		"books":      items,
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"pages":      pages,
		"has_more":   page < pages,
		"elapsed_ms": elapsedMS,
	})
}

// getBook 返回单本书的详情。
func (s *Server) getBook(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	b, err := s.st.GetBook(id)
	if err != nil {
		fail(c, 404, "书籍不存在")
		return
	}
	detail := bookJSON(b)
	detail["dir"] = b.Dir
	detail["file_exists"] = fileExists(b.Path)
	ok(c, detail)
}

// featured 返回首页横幅（Hero）用的一组精选书籍。
//
// 编辑精选不需要用户维护任何东西：从"最近入库"里挑出带封面的，再加上
// "同系列里有多卷的那套"作为补偿 —— 前者保证新鲜，后者保证横幅里偶尔
// 出现成套的书，而不是随机一本。只在内存里挑，不写库、不加字段。
func (s *Server) featured(c *gin.Context) {
	limit := queryInt(c, "limit", 12)
	if limit < 1 || limit > 30 {
		limit = 12
	}

	// 最近入库（只看带封面的，横幅靠封面撑场面）
	recent, _, err := s.st.ListBooks(store.BookQuery{
		Sort: "created", Desc: true, Page: 1, PageSize: 40,
	})
	if err != nil {
		failErr(c, 500, "读取精选失败", err)
		return
	}

	var withCover []store.Book
	for _, b := range recent {
		if b.Cover != "" {
			withCover = append(withCover, b)
		}
	}
	if len(withCover) == 0 {
		// 一本封面都没有时退回全部最近入库，至少让横幅有内容。
		withCover = recent
	}

	// 系列补偿：同一系列的卷数够多时，把它推到横幅最前面。
	// 这里只取一页做近似（书库上千本时也不必全表扫），够横幅用。
	all, _, err := s.st.ListBooks(store.BookQuery{Sort: "created", Desc: true, Page: 1, PageSize: 300})
	if err == nil {
		type group struct {
			items  []store.Book
			latest int64
		}
		groups := map[string]*group{}
		for i := range all {
			series, _ := store.SplitSeries(all[i].Title, all[i].Dir)
			if series == "" {
				continue
			}
			g := groups[series]
			if g == nil {
				g = &group{}
				groups[series] = g
			}
			g.items = append(g.items, all[i])
			if t := all[i].CreatedAt.Unix(); t > g.latest {
				g.latest = t
			}
		}
		var best *group
		for _, g := range groups {
			if len(g.items) < 2 {
				continue
			}
			// 卷数多者优先；卷数相同则取入库更晚的一套。
			if best == nil || len(g.items) > len(best.items) ||
				(len(g.items) == len(best.items) && g.latest > best.latest) {
				best = g
			}
		}
		if best != nil {
			for _, b := range best.items {
				if b.Cover != "" {
					withCover = append([]store.Book{b}, withCover...)
					break
				}
			}
		}
	}

	// 去重后按 limit 截断，保持"先系列、再最近"的顺序。
	seen := map[int64]bool{}
	items := make([]gin.H, 0, limit)
	for _, b := range withCover {
		if seen[b.ID] {
			continue
		}
		seen[b.ID] = true
		items = append(items, bookJSON(&b))
		if len(items) >= limit {
			break
		}
	}

	ok(c, gin.H{"books": items})
}

// downloadBook 以流式方式返回原始文件，Content-Disposition 使用原始文件名。
func (s *Server) downloadBook(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	b, err := s.st.GetBook(id)
	if err != nil {
		fail(c, 404, "书籍不存在")
		return
	}

	fi, err := os.Stat(b.Path)
	if err != nil || fi.IsDir() {
		fail(c, http.StatusGone, "原始文件已不存在，可能已被移动或删除")
		return
	}

	name := b.Filename
	if name == "" {
		name = "book"
	}
	c.Header("Content-Disposition", contentDisposition(name))
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", strconv.FormatInt(fi.Size(), 10))
	c.Header("X-Content-Type-Options", "nosniff")

	// http.ServeFile 内部使用 io.Copy 流式发送，不会把整本书读进内存，
	// 同时天然支持 Range 断点续传。
	http.ServeFile(c.Writer, c.Request, b.Path)
}

// bookJSON 统一书籍对外结构。
func bookJSON(b *store.Book) gin.H {
	coverURL := ""
	thumbURL := ""
	if b.Cover != "" {
		coverURL = "/covers/" + strconv.FormatInt(b.LibraryID, 10) + "/" + url.PathEscape(b.Cover)
		thumbURL = "/thumbs/" + strconv.FormatInt(b.LibraryID, 10) + "/" + url.PathEscape(b.Cover)
	}
	title := b.Title
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(b.Filename, "."+b.Format)
	}
	out := gin.H{
		"id":           b.ID,
		"library_id":   b.LibraryID,
		"library_name": b.LibraryName,
		"title":        title,
		"author":       b.Author,
		"format":       b.Format,
		"size":         b.Size,
		"size_human":   humanSize(b.Size),
		"filename":     b.Filename,
		"has_cover":    b.Cover != "",
		"cover_url":    coverURL,
		"thumb_url":    thumbURL,
		"created_at":   b.CreatedAt,
		"updated_at":   b.UpdatedAt,
	}
	// 系列信息按需现算：前端拿它把同一套书并成卡片、详情页显示"第 N 卷"。
	if series, volume := store.SplitSeries(title, b.Dir); series != "" {
		out["series"] = series
		if volume != "" {
			out["volume"] = volume
		}
	}
	return out
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
