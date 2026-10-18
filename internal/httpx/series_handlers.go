package httpx

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	booklib "libra/internal/library"
	"libra/internal/store"
)

// 「系列作品聚合」的展示逻辑全在这个文件里。
//
// 规则只有一条，但需要一点上下文才成立：先把书库里所有书的相对深度数一遍，
// 取最大值作为基准深度；只有深度恰好等于基准深度的书才参与聚合，取直接父
// 文件夹名当系列名；更浅的书一律是散书。算法本身在 internal/library，
// 这里负责把它接到列表接口上：过滤、分组、返回每组数量。

// seriesGroupQuery 解析 ?series= 参数。
//
//	"__none__"（或 series=none）  只要散书
//	"三体系列"                    只要该系列
//	空                            不过滤
func seriesGroupQuery(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "none" {
		return store.SeriesNone
	}
	return v
}

// seriesCanvas 返回"参与系列聚合计算的书库根目录集合"以及书库开没开聚合。
//
// 范围跟着当前列表走：看的是全部书籍时，把打开了聚合的书库合在一起算；
// 看的是某一座书库时，就只算这一座。
func (s *Server) seriesCanvas(libraryID int64) (map[int64]string, bool, error) {
	roots, err := s.st.LibraryRoots()
	if err != nil {
		return nil, false, err
	}
	libs, err := s.st.ListLibraries()
	if err != nil {
		return nil, false, err
	}
	scope := make(map[int64]string, len(libs))
	enabled := false
	for _, l := range libs {
		if !l.SeriesGroup {
			continue
		}
		if libraryID > 0 && l.ID != libraryID {
			continue
		}
		enabled = true
		scope[l.ID] = roots[l.ID]
	}
	if !enabled {
		return nil, false, nil
	}
	return scope, true, nil
}

// seriesBaseEntries 取出范围内全部书的相对路径，用来定基准深度。
func (s *Server) seriesBaseEntries(scope map[int64]string) ([]booklib.Entry, error) {
	out := make([]booklib.Entry, 0, 256)
	page := 1
	for {
		q := store.BookQuery{Page: page, PageSize: 300, Sort: "id"}
		if len(scope) == 1 {
			for id := range scope {
				q.LibraryID = id
			}
		}
		books, total, err := s.st.ListBooksWithRel(q, scope)
		if err != nil {
			return nil, err
		}
		for _, b := range books {
			out = append(out, booklib.Entry{Rel: b.RelPath, Title: b.Title})
		}
		if len(books) == 0 || page*300 >= total {
			return out, nil
		}
		page++
	}
}

// seriesInfo 计算一本书的系列归属。
func seriesInfo(b *store.Book, baseDepth int) (name string, ok bool) {
	if b.RelPath == "" {
		return "", false
	}
	dir := filepath.ToSlash(filepath.Dir(b.RelPath))
	if dir == "." {
		dir = ""
	}
	return booklib.SeriesOf(dir, booklib.Depth(b.RelPath), baseDepth)
}

// seriesFilterIDs 返回"系列归属等于 want 的书"的 ID 集合；不过滤时返回 nil。
//
// 列表接口的筛选是在 SQL 里做的，而系列归属要靠目录层级现算，两者对不上，
// 所以这里先把范围内的书各算一遍拿到 ID，再去主查询里过滤。
// 几千本书的量级下这一步只是一次内存扫描，比把「系列」冗余进数据库更省事，
// 也不会因为改名文件夹而出现陈旧数据。
func (s *Server) seriesFilterIDs(want string, libraryID int64) ([]int64, error) {
	scope, enabled, err := s.seriesCanvas(libraryID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		// 没开聚合：没有"所属系列"可言，任何系列筛选都返回空。
		return []int64{}, nil
	}
	entries, err := s.seriesBaseEntries(scope)
	if err != nil {
		return nil, err
	}
	base := booklib.BaseDepth(entries)

	out := make([]int64, 0, 64)
	page := 1
	for {
		q := store.BookQuery{Page: page, PageSize: 300, Sort: "id"}
		if len(scope) == 1 {
			for id := range scope {
				q.LibraryID = id
			}
		}
		books, total, err := s.st.ListBooksWithRel(q, scope)
		if err != nil {
			return nil, err
		}
		for i := range books {
			name, ok := seriesInfo(&books[i], base)
			if !ok {
				name = store.SeriesNone
			}
			if name == want {
				out = append(out, books[i].ID)
			}
		}
		if len(books) == 0 || page*300 >= total {
			break
		}
		page++
	}
	return out, nil
}

// seriesGroups 返回"当前书库里有哪些系列"，供前端做成筛选胶囊。
// 只列真正成组的（至少两本）；散书始终存在，不单独列出。
func (s *Server) seriesGroups(libraryID int64) []gin.H {
	scope, enabled, err := s.seriesCanvas(libraryID)
	if err != nil || !enabled {
		return nil
	}
	entries, err := s.seriesBaseEntries(scope)
	if err != nil {
		return nil
	}
	base := booklib.BaseDepth(entries)
	if base <= 1 {
		return nil
	}

	type group struct {
		name  string
		count int
	}
	counts := map[string]*group{}
	page := 1
	for {
		q := store.BookQuery{Page: page, PageSize: 300, Sort: "id"}
		if len(scope) == 1 {
			for id := range scope {
				q.LibraryID = id
			}
		}
		books, total, err := s.st.ListBooksWithRel(q, scope)
		if err != nil {
			return nil
		}
		for i := range books {
			name, ok := seriesInfo(&books[i], base)
			if !ok {
				continue
			}
			g := counts[name]
			if g == nil {
				g = &group{name: name}
				counts[name] = g
			}
			g.count++
		}
		if len(books) == 0 || page*300 >= total {
			break
		}
		page++
	}

	list := make([]*group, 0, len(counts))
	for _, g := range counts {
		if g.count >= 2 {
			list = append(list, g)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].name < list[j].name
	})

	out := make([]gin.H, 0, len(list))
	for _, g := range list {
		out = append(out, gin.H{"name": g.name, "count": g.count})
	}
	return out
}

// bookSeriesJSON 给列表里的每本书补一个 series_name / rel_depth，
// 前端据此在卡片上打系列卷标、以及做分组展示。
func (s *Server) bookSeriesJSON(b *store.Book, base int) gin.H {
	extra := gin.H{}
	if name, ok := seriesInfo(b, base); ok {
		extra["series_name"] = name
		extra["rel_depth"] = booklib.Depth(b.RelPath)
	}
	return extra
}

// relPathsFor 为当前这一页的书补齐"相对书库根目录的路径"。
//
// 主列表查询不查 path 列（目录结构不对外暴露），而系列聚合只需要这一页的书，
// 所以按书库分组查一次，用 "path" 前缀拼出相对路径 —— 只做字符串运算，
// 不再落一次 SQL。
func (s *Server) relPathsFor(books []store.Book) map[int64]string {
	out := make(map[int64]string, len(books))
	if len(books) == 0 {
		return out
	}
	// 一次把书库里某个 ID 段的记录查出来代价太大：这里按书库分组，
	// 每座书库只查一次 path 列，然后按 ID 建索引。
	byLib := map[int64][]int64{}
	for _, b := range books {
		byLib[b.LibraryID] = append(byLib[b.LibraryID], b.ID)
	}
	roots := map[int64]string{}
	if all, err := s.st.ListLibraries(); err == nil {
		for _, l := range all {
			roots[l.ID] = l.Path
		}
	}
	for libID, ids := range byLib {
		paths, err := s.st.BookPathsByIDs(ids)
		if err != nil {
			continue
		}
		root := roots[libID]
		for id, p := range paths {
			if root != "" {
				if r, relErr := filepath.Rel(root, p); relErr == nil {
					out[id] = filepath.ToSlash(r)
					continue
				}
			}
			out[id] = filepath.ToSlash(p)
		}
	}
	return out
}

// seriesCounts 统计每一条系列的总本数（不只是当前这一页里出现的那几本）。
// 合并卡上写"共 N 本"，翻页把同一个系列切开时也不会显示成"3 本"。
//
// uid 的形态决定统计范围：
//
//	"书库ID|系列名"  只在该书库内数（看单座书库时）
//	"系列名"        跨所有开了聚合的书库数（看"全部书籍"时）
func (s *Server) seriesCounts(seriesOf map[int64]string) map[string]int {
	out := make(map[string]int)
	if len(seriesOf) == 0 {
		return out
	}
	// 书库范围从 uid 里取（uid = "书库ID|系列名"），只算这一页涉及的库。
	libs := map[int64]bool{}
	crossLib := false
	for _, uid := range seriesOf {
		if i := strings.Index(uid, "|"); i > 0 {
			if id, err := strconv.ParseInt(uid[:i], 10, 64); err == nil {
				libs[id] = true
			}
		} else {
			crossLib = true
		}
	}
	roots, err := s.st.LibraryRoots()
	if err != nil {
		return out
	}
	if crossLib {
		for id := range roots {
			libs[id] = true
		}
	}
	for libID := range libs {
		scope := map[int64]string{libID: roots[libID]}
		entries, err := s.seriesBaseEntries(scope)
		if err != nil {
			continue
		}
		base := booklib.BaseDepth(entries)
		page := 1
		for {
			q := store.BookQuery{Page: page, PageSize: 300, Sort: "id", LibraryID: libID}
			books, total, err := s.st.ListBooksWithRel(q, scope)
			if err != nil {
				break
			}
			for i := range books {
				if name, ok := seriesInfo(&books[i], base); ok {
					out[seriesKey(libID, name, 0)]++
				}
			}
			if len(books) == 0 || page*300 >= total {
				break
			}
			page++
		}
	}
	return out
}

// seriesKey 生成一本书的"系列归属键"。
//
//	看单座书库：书库ID|系列名 —— 两座库里都有"三体"时各算各的
//	看全部书籍：只有系列名 —— 同名文件夹跨库合并成一个系列
//
// scopeID 是当前列表的书库范围（>0 为单库）。
func seriesKey(libraryID int64, name string, scopeID int64) string {
	if scopeID > 0 {
		return strconv.FormatInt(libraryID, 10) + "|" + name
	}
	return name
}
