package httpx

import (
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"libra/internal/bookmeta"
	booklib "libra/internal/library"
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

	// 系列聚合：按 ?series= 过滤出某个系列 / 散书。
	// 关着聚合的书库不受影响，行为和以前完全一致。
	var seriesCanvasOn bool
	if canvas, on, cerr := s.seriesCanvas(q.LibraryID); cerr == nil {
		seriesCanvasOn = on
		_ = canvas
	}
	seriesFilter := seriesGroupQuery(c.Query("series"))
	if seriesFilter != "" && seriesCanvasOn {
		ids, err := s.seriesFilterIDs(seriesFilter, q.LibraryID)
		if err != nil {
			failErr(c, 500, "计算系列归属失败", err)
			return
		}
		if len(ids) == 0 {
			ok(c, gin.H{
				"books":     []gin.H{},
				"total":     0,
				"page":      page,
				"page_size": pageSize,
				"pages":     0,
				"has_more":  false,
				"groups":    s.seriesGroups(q.LibraryID),
			})
			return
		}
		q.SeriesIDs = ids
	}

	start := time.Now()
	books, total, err := s.st.ListBooks(q)
	if err != nil {
		failErr(c, 500, "查询书籍失败", err)
		return
	}
	// elapsed_ms 给封面墙页脚用，顺便也是排查慢查询最省事的那个数字。
	elapsedMS := math.Round(float64(time.Since(start).Microseconds())/100) / 10

	// 系列归属：把当前这一页的书换成带相对路径的查询结果，再标上系列名。
	// 列表主查询本来就不查 path 列（不对外暴露目录结构），这里只为这一页补查一次。
	var relByID map[int64]string
	var base int
	seriesOn := false
	if scope, on, serr := s.seriesCanvas(q.LibraryID); serr == nil && on {
		seriesOn = true
		if entries, eerr := s.seriesBaseEntries(scope); eerr == nil {
			base = booklib.BaseDepth(entries)
		}
		relByID = s.relPathsFor(books)
	}

	// 这一页的书各自属于哪个系列。
	// · 看的是单座书库：uid 带上书库，前端按 uid 合并成一张卡；
	// · 看的是"全部书籍"：同名系列跨书库合并成一张（uid 只有系列名），
	//   于是列表里能出现一条同时含多座书库的书、本数也是跨库合计的系列。
	seriesOf := make(map[int64]string, len(books))
	for i := range books {
		if base <= 0 || relByID == nil {
			continue
		}
		rel, ok := relByID[books[i].ID]
		if !ok || rel == "" {
			continue
		}
		b := books[i]
		b.RelPath = rel
		if name, hit := seriesInfo(&b, base); hit {
			seriesOf[b.ID] = seriesKey(b.LibraryID, name, q.LibraryID)
		}
	}

	// 合并卡上要显示的"共 N 本"是整条系列的总数，不是这一页里露出的那几本。
	counts := s.seriesCounts(seriesOf)

	items := make([]gin.H, 0, len(books))
	for i := range books {
		item := bookJSON(&books[i])
		if uid, hit := seriesOf[books[i].ID]; hit {
			name := uid[strings.Index(uid, "|")+1:]
			item["series_name"] = name
			item["series_uid"] = uid
			item["series_count"] = counts[uid]
			item["rel_depth"] = booklib.Depth(relByID[books[i].ID])
		}
		items = append(items, item)
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
		"series":     seriesOn,
		"groups":     seriesGroupsJSON(s.seriesGroups(q.LibraryID)),
	})
}

// seriesGroupsJSON 保证空切片序列化成 [] 而不是 null。
func seriesGroupsJSON(groups []gin.H) []gin.H {
	if groups == nil {
		return []gin.H{}
	}
	return groups
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

// featured 返回首页横幅（Hero）用的精选书籍。
//
// 内容完全由后台的"横幅数据源"决定：
//
//	每日推荐 + 指定书库   两边各随机取若干本，合并后打乱（默认即是这个组合）
//	只用每日推荐          管理员手工挑的那份书单
//	只随机书库            从指定书库里随机取（没选书库时就是全部书库）
//
// 每次刷新都会随机，但结果缓存一小时：读者在首页来回翻页、刷新都不会换味道，
// 整点之后再来才会看到新的一批。随机结果里的先后顺序也一并记下来 ——
// 否则每次请求都重排，同一个小时内轮播顺序也会变来变去。
// 响应里只有书本身，不告诉前端"这批来自哪里"：横幅就是横幅。
func (s *Server) featured(c *gin.Context) {
	limit := queryInt(c, "limit", 12)
	if limit < 1 || limit > 30 {
		limit = 12
	}
	// refresh=1 跳过缓存立刻重抽一轮，只给调试 / 后台预览用。
	refresh := c.Query("refresh") == "1"

	picksEnabled := s.st.GetDailyPicksEnabled()
	libIDs := s.st.FeaturedLibraryIDs()

	rotation, fresh := s.st.FeaturedRotation()
	// 缓存里一本书都没有，说明上次算出来就是空的：要么数据源没配好，要么
	// 管理员刚清空了书单。这种空结果不应该被缓存住一小时 —— 否则刚加完书，
	// 首页还是一个空横幅，看起来就像没保存成功。所以空缓存一律重新算。
	cacheable := len(rotation) > 0
	if !refresh && fresh && cacheable {
		if books := s.loadRotation(s.rotationSnapshot(rotation, picksEnabled, libIDs), limit); len(books) > 0 {
			ok(c, gin.H{"books": books})
			return
		}
	}

	books, err := s.buildFeatured(picksEnabled, libIDs, limit)
	if err != nil {
		failErr(c, 500, "读取精选失败", err)
		return
	}

	ids := make([]int64, 0, len(books))
	for _, b := range books {
		if id, conv := b["id"].(int64); conv {
			ids = append(ids, id)
		}
	}
	// 空结果也写进缓存，但下一轮不会命中（见上面的 cacheable 判断），
	// 于是空态与正常态之间切换时不会卡住。
	_ = s.st.SetFeaturedRotation(ids)

	ok(c, gin.H{"books": books})
}

// rotationSnapshot 把缓存的顺序截断到本轮应该展示的条数，并顺手去掉已经
// 不该出现在横幅里的书（比如该库已经被移出数据源）。
func (s *Server) rotationSnapshot(rotation []int64, picksEnabled bool, libIDs []int64) []int64 {
	allowed := map[int64]bool{}
	if picksEnabled {
		ids, err := s.st.DailyPickIDs()
		if err == nil {
			for _, id := range ids {
				allowed[id] = true
			}
		}
	}
	if b, err := s.st.ListBooksByIDs(rotation); err == nil {
		for _, x := range b {
			if libAllowed(x.LibraryID, libIDs) {
				allowed[x.ID] = true
			}
		}
	}
	out := make([]int64, 0, len(rotation))
	for _, id := range rotation {
		if allowed[id] {
			out = append(out, id)
		}
	}
	return out
}

// libAllowed 判断某本书是否落在配置的书库范围内，0 表示全部书库。
func libAllowed(libraryID int64, libIDs []int64) bool {
	if len(libIDs) == 0 {
		return true
	}
	for _, id := range libIDs {
		if id <= 0 || id == libraryID {
			return true
		}
	}
	return false
}

// buildFeatured 按数据源拼出本轮横幅内容。
//
// 混合模式（每日推荐 + 指定书库）下两边各取一半：每日推荐是人工挑的，
// 理应优先露出；指定书库那半边负责"每天都有新鲜面孔"。
// 任一来源不够时由另一边补满 —— 管理员刚配好书单只有两本，也照样能轮播。
func (s *Server) buildFeatured(picksEnabled bool, libIDs []int64, limit int) ([]gin.H, error) {
	picksQuota := 0
	if picksEnabled {
		picksQuota = (limit + 1) / 2
	}
	autoBooks := s.librarySourceAvailable(libIDs)

	var out []store.Book
	if picksQuota > 0 {
		picks, err := s.st.ListDailyPicks()
		if err != nil {
			return nil, err
		}
		books := make([]store.Book, 0, len(picks))
		for _, p := range picks {
			books = append(books, p.Book)
		}
		out = append(out, pickCovers(books, picksQuota)...)
	}

	// 书库负责补齐剩下的位置；只用书库时就是全部 limit。
	// 书库不存在（配置里指的是已被删除的库）时 autoBooks 为 false，
	// 这时不拿其它书库的书来凑数，横幅就是书单本身。
	if rest := limit - len(out); rest > 0 && autoBooks {
		extra, err := s.randomLibraryBooks(libIDs, rest, out)
		if err != nil {
			return nil, err
		}
		out = append(out, extra...)
	}

	shuffleBooks(out)

	// 书单、书库都可能不够 limit，能出多少算多少；一本都没有时返回空数组，
	// 前端自然隐藏横幅。
	items := make([]gin.H, 0, len(out))
	for i := range out {
		items = append(items, bookJSON(&out[i]))
	}
	return items, nil
}

// librarySourceAvailable 判断"随机书库"这个数据源是否还可用：
// 配置里是全部书库时永远可用；指定了书库时要至少有一个还没被删掉。
func (s *Server) librarySourceAvailable(libIDs []int64) bool {
	if !hasPositiveID(libIDs) {
		return true
	}
	for _, id := range libIDs {
		if _, err := s.st.GetLibrary(id); err == nil {
			return true
		}
	}
	return false
}

// randomLibraryBooks 从配置的书库里随机取 n 本，优先带封面，跳过 excluded 里的书。
// libIDs 里出现 0 表示"全部书库"。
func (s *Server) randomLibraryBooks(libIDs []int64, n int, excluded []store.Book) ([]store.Book, error) {
	if n <= 0 {
		return nil, nil
	}
	skip := make(map[int64]bool, len(excluded))
	for _, b := range excluded {
		skip[b.ID] = true
	}

	// 候选池取目标数量的三倍（夹在 12~90 之间）：随机之后还能有足够的余量
	// 剔除掉书单里已经出现过的书，不会因为重复而凑不满。
	want := n * 3
	if want < 12 {
		want = 12
	}
	if want > 90 {
		want = 90
	}

	ids := libIDs
	if len(ids) == 0 {
		ids = []int64{0}
	}

	var pool []store.Book
	for _, libID := range ids {
		got, err := s.st.RandomBooks(libID, true, want)
		if err != nil {
			return nil, err
		}
		if len(got) == 0 && libID > 0 {
			// 该库里一本带封面的都没有：退回该库的全部书，至少横幅有内容。
			got, err = s.st.RandomBooks(libID, false, want)
			if err != nil {
				return nil, err
			}
		}
		pool = append(pool, got...)
	}
	if len(pool) == 0 {
		// 指定书库里没有带封面的书，退回全部书库再试一次。
		got, err := s.st.RandomBooks(0, true, want)
		if err != nil {
			return nil, err
		}
		if len(got) == 0 {
			got, err = s.st.RandomBooks(0, false, want)
			if err != nil {
				return nil, err
			}
		}
		pool = got
	}

	shuffleBooks(pool)
	out := make([]store.Book, 0, n)
	for _, b := range pool {
		if skip[b.ID] {
			continue
		}
		skip[b.ID] = true
		out = append(out, b)
		if len(out) >= n {
			break
		}
	}
	return out, nil
}

// pickCovers 从书单里取前 n 本。书单本身是人工挑选的，不按有没有封面筛。
func pickCovers(books []store.Book, n int) []store.Book {
	if len(books) <= n {
		return books
	}
	return books[:n]
}

// shuffleBooks Fisher–Yates 洗牌。这里不需要可复现：每轮刷新本来就该不一样。
func shuffleBooks(books []store.Book) {
	for i := len(books) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		books[i], books[j] = books[j], books[i]
	}
}

// loadRotation 按记住的顺序取书；顺序里已不存在的书会被跳过。
func (s *Server) loadRotation(ids []int64, limit int) []gin.H {
	books, err := s.st.ListBooksByIDs(ids)
	if err != nil {
		return nil
	}
	items := make([]gin.H, 0, len(books))
	for i := range books {
		items = append(items, bookJSON(&books[i]))
		if len(items) >= limit {
			break
		}
	}
	return items
}

func (s *Server) countDailyPicks() int {
	picks, err := s.st.ListDailyPicks()
	if err != nil {
		return 0
	}
	return len(picks)
}

// featuredSourceLabel 给前端展示用的来源描述。
func featuredSourceLabel(picksEnabled bool, libIDs []int64) string {
	hasLib := false
	for _, id := range libIDs {
		if id > 0 {
			hasLib = true
		}
	}
	switch {
	case picksEnabled && hasLib:
		return "mix"
	case picksEnabled:
		return "picks"
	case hasLib:
		return "libraries"
	default:
		return "all"
	}
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

// updateBookMetadata 允许管理员修改书名 / 作者。
// 写入后 meta_locked=1，后续扫描不会再把它们覆盖回文件内嵌值。
func (s *Server) updateBookMetadata(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	var req struct {
		Title  string `json:"title"`
		Author string `json:"author"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		fail(c, 400, "书名不能为空")
		return
	}
	if len(title) > 500 {
		fail(c, 400, "书名过长")
		return
	}
	author := strings.TrimSpace(req.Author)
	if len(author) > 300 {
		fail(c, 400, "作者名过长")
		return
	}

	b, err := s.st.GetBook(id)
	if err != nil {
		fail(c, 404, "书籍不存在")
		return
	}
	// 先写回源文件，成功后再更新数据库；文件不可写时不要制造"数据库改了、文件没改"的状态。
	if err := bookmeta.Write(b.Path, b.Format, title, author); err != nil {
		fail(c, 500, "写入源文件失败: "+err.Error())
		return
	}
	if err := s.st.UpdateBookManualMeta(id, title, author); err != nil {
		if err == store.ErrNotFound {
			fail(c, 404, "书籍不存在")
			return
		}
		failErr(c, 500, "保存书籍元数据失败", err)
		return
	}
	b, err = s.st.GetBook(id)
	if err != nil {
		failErr(c, 500, "读取书籍失败", err)
		return
	}
	ok(c, gin.H{"book": bookJSON(b)})
}

// deleteBookAdmin 从书库移除一本书。
// 采用软删除：保留 NAS 上的原始文件，只移除索引与封面，重新扫描也不会加回来。
func (s *Server) deleteBookAdmin(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	b, err := s.st.GetBook(id)
	if err != nil {
		fail(c, 404, "书籍不存在")
		return
	}
	if err := s.st.DeleteBook(id); err != nil {
		if err == store.ErrNotFound {
			fail(c, 404, "书籍不存在")
			return
		}
		failErr(c, 500, "删除书籍失败", err)
		return
	}
	s.removeBookCover(b.LibraryID, b.Cover)
	ok(c, gin.H{"message": "书籍已从书库移除"})
}

func (s *Server) removeBookCover(libraryID int64, cover string) {
	if cover == "" {
		return
	}
	dir := s.cfg.LibraryCoversDir(libraryID)
	_ = os.Remove(filepath.Join(dir, cover))
	base := strings.TrimSuffix(cover, filepath.Ext(cover)) + ".jpg"
	_ = os.Remove(filepath.Join(dir, "thumbs", base))
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
		"meta_locked":  b.MetaLocked == 1,
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
