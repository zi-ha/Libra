package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Book 是一本书的索引记录。
type Book struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"library_id"`
	LibraryName string `json:"library_name,omitempty"`
	Path        string `json:"-"` // 绝对路径，不对外暴露
	Dir         string `json:"-"`
	Filename    string `json:"filename"`
	// RelPath 是相对书库根目录的路径，由 ListBooksWithRel 填充；
	// 列表接口不带它（Path 同理），避免把服务器目录结构暴露给前端。
	RelPath string `json:"-"`
	Title   string `json:"title"`
	Author  string `json:"author"`
	Format  string `json:"format"` // epub / pdf
	Size    int64  `json:"size"`
	Mtime   int64  `json:"mtime"`
	Cover   string `json:"cover"` // 封面文件名（相对封面目录）
	// CoverState 记录封面提取结果（ok / none / failed），增量扫描据此决定是否重试。
	CoverState string    `json:"-"`
	MetaLocked int       `json:"-"` // 1 = 管理员手动改过书名/作者，扫描不覆盖
	Deleted    int       `json:"-"` // 1 = 从书库移除（软删除，保留原文件）
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Series / Volume 是展示层顺手解析出来的系列信息（不落库）：从书名或目录名里
// 识别 "三体 2"、"Vol. 3"、"第3卷" 这类写法。它们不进 Book 结构体，是为了
// 不动数据库扫描的那一列对应关系 —— 需要的时候用 SplitSeries 现算。

const bookColumns = `b.id, b.library_id, COALESCE(l.name, ''), b.path, b.dir, b.filename,
	b.title, b.author, b.format, b.size, b.mtime, b.cover, b.cover_state,
	b.meta_locked, b.deleted, b.created_at, b.updated_at`

func scanBook(row interface{ Scan(...any) error }) (*Book, error) {
	var b Book
	var created, updated int64
	if err := row.Scan(&b.ID, &b.LibraryID, &b.LibraryName, &b.Path, &b.Dir, &b.Filename,
		&b.Title, &b.Author, &b.Format, &b.Size, &b.Mtime, &b.Cover, &b.CoverState,
		&b.MetaLocked, &b.Deleted, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	b.CreatedAt = unixToTime(created)
	b.UpdatedAt = unixToTime(updated)
	return &b, nil
}

// BookQuery 是书籍列表的查询条件。
type BookQuery struct {
	LibraryID int64  // 0 表示全部书库
	Search    string // 匹配书名或作者
	Format    string // "" 全部 / epub / pdf
	Sort      string // title / natural / author / created / size / updated
	Desc      bool
	Page      int
	PageSize  int

	// SeriesGroup 非空时按"系列作品聚合"的归属过滤：
	//   SeriesNone（"__none__"）  只要散书（不参与聚合的书）
	//   "三体系列"                只要归在「三体系列」下的书
	// 空字符串表示不过滤。
	SeriesGroup string

	// SeriesIDs 非空时只返回这些 ID 的书（系列筛选用）。
	// 调用方先算好归属拿到 ID，再让 SQL 过滤并分页，避免把整页之后的
	// 书全读到内存里再筛；空切片表示"没有匹配"，与 nil 不同。
	SeriesIDs []int64

	// WithRelPath 为真时额外带出"相对书库根目录"的路径，
	// 供调用方按目录层级计算系列归属（列表接口不需要）。
	WithRelPath bool
}

// SeriesNone 是 SeriesGroup 里表示"散书"的保留值，正常文件夹名不会用它。
const SeriesNone = "__none__"

func (q *BookQuery) normalize() {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 60
	}
	if q.PageSize > 300 {
		q.PageSize = 300
	}
}

// where 生成 WHERE 子句与参数。
func (q *BookQuery) where() (string, []any) {
	conds := make([]string, 0, 4)
	args := make([]any, 0, 6)

	// 软删除的书籍不再出现在列表 / 精选里。
	conds = append(conds, "b.deleted = 0")

	if q.LibraryID > 0 {
		conds = append(conds, "b.library_id = ?")
		args = append(args, q.LibraryID)
	}
	if f := strings.ToLower(strings.TrimSpace(q.Format)); f == "epub" || f == "pdf" {
		conds = append(conds, "b.format = ?")
		args = append(args, f)
	}
	if term := strings.TrimSpace(q.Search); term != "" {
		like := "%" + escapeLike(term) + "%"
		conds = append(conds, `(b.title LIKE ? ESCAPE '\' OR b.author LIKE ? ESCAPE '\' OR b.filename LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like)
	}
	if q.SeriesIDs != nil {
		if len(q.SeriesIDs) == 0 {
			// 空集合：明确表示"没有匹配的书"，不能退化成"不过滤"。
			conds = append(conds, "1 = 0")
		} else {
			placeholders := make([]string, len(q.SeriesIDs))
			for i, id := range q.SeriesIDs {
				placeholders[i] = "?"
				args = append(args, id)
			}
			conds = append(conds, "b.id IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

func (q *BookQuery) orderBy() string {
	var col string
	// 书名相关的排序都走"自然排序"：把书名拆成"文字段 / 数字段"两列，先比文字
	// 再比数字，于是 卷1 < 卷2 < 卷10。默认的字符串序会把 10 排到 1、2 前面，
	// 对"三体1 / 三体2 / 三体10"这种卷号书名完全没法看，所以 title 也走这一套。
	// SQLite 的字符串比较本身是逐字节的，正好按中文的 UTF-8 码位排序，
	// 与"按拼音"略有出入，但至少稳定且不会出现 10 在 1 前面。
	switch strings.ToLower(strings.TrimSpace(q.Sort)) {
	case "author":
		col = "b.author"
	case "created":
		col = "b.created_at"
	case "updated":
		col = "b.updated_at"
	case "size":
		col = "b.size"
	case "title", "natural":
		dir := "ASC"
		if q.Desc {
			dir = "DESC"
		}
		// 文字段 → 数字段 → 原书名 → id，四层兜底，分页顺序始终稳定。
		return fmt.Sprintf(
			" ORDER BY natural_text(b.title) %s, COALESCE(natural_num(b.title), 0) %s, b.title %s, b.id %s",
			dir, dir, dir, dir)
	default:
		col = "b.title"
	}
	dir := "ASC"
	if q.Desc {
		dir = "DESC"
	}
	// 加 id 作为兜底排序键，保证分页结果稳定。
	return fmt.Sprintf(" ORDER BY %s %s, b.id %s", col, dir, dir)
}

// naturalTextSQL / naturalNumSQL 见 store.go 里注册的两个 SQLite 自定义函数。
const (
	naturalTextFn = "natural_text"
	naturalNumFn  = "natural_num"
)

// ListBooks 分页查询书籍，同时返回符合条件的总数。
func (s *Store) ListBooks(q BookQuery) ([]Book, int, error) {
	q.normalize()
	whereSQL, args := q.where()

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM books b`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + bookColumns + ` FROM books b LEFT JOIN libraries l ON l.id = b.library_id` +
		whereSQL + q.orderBy() + ` LIMIT ? OFFSET ?`
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Book, 0, q.PageSize)
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *b)
	}
	return out, total, rows.Err()
}

// ListBooksWithRel 与 ListBooks 一样，但额外查一列 path，调用方用它算相对路径
// （系列聚合需要按目录层级判断归属）。排序与分页仍然交给数据库完成 ——
// 书多了以后在内存里重排整表会白白吃掉几百毫秒。
func (s *Store) ListBooksWithRel(q BookQuery, roots map[int64]string) ([]Book, int, error) {
	q.normalize()
	q.WithRelPath = true
	whereSQL, args := q.where()

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM books b`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT ` + bookColumns + ` FROM books b LEFT JOIN libraries l ON l.id = b.library_id` +
		whereSQL + q.orderBy() + ` LIMIT ? OFFSET ?`
	args = append(args, q.PageSize, (q.Page-1)*q.PageSize)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]Book, 0, q.PageSize)
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, 0, err
		}
		// 相对书库根目录的路径：系列聚合按它数深度、取父文件夹名。
		// 取不到相对路径时退回绝对路径，最坏只是不参与聚合。
		rel := b.Path
		if root, ok := roots[b.LibraryID]; ok && root != "" {
			if r, err := filepath.Rel(root, b.Path); err == nil {
				rel = r
			}
		}
		b.RelPath = filepath.ToSlash(rel)
		out = append(out, *b)
	}
	return out, total, rows.Err()
}

// BookPathsByIDs 返回 ID → 绝对路径，供系列聚合按目录层级算归属。
func (s *Store) BookPathsByIDs(ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT id, path FROM books WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// GetBook 按 ID 查询单本。
func (s *Store) GetBook(id int64) (*Book, error) {
	return scanBook(s.db.QueryRow(
		`SELECT `+bookColumns+` FROM books b LEFT JOIN libraries l ON l.id = b.library_id WHERE b.id = ? AND b.deleted = 0`, id))
}

// BookStat 是聚合统计。
type BookStat struct {
	Total     int   `json:"total"`
	Epub      int   `json:"epub"`
	Pdf       int   `json:"pdf"`
	TotalSize int64 `json:"total_size"`
}

// Stats 返回整体（或某个书库）的统计信息。
func (s *Store) Stats(libraryID int64) (BookStat, error) {
	var st BookStat
	where := " WHERE deleted = 0"
	args := []any{}
	if libraryID > 0 {
		where += " AND library_id = ?"
		args = append(args, libraryID)
	}
	err := s.db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN format = 'epub' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN format = 'pdf' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(size), 0)
		FROM books`+where, args...).Scan(&st.Total, &st.Epub, &st.Pdf, &st.TotalSize)
	return st, err
}

// BookIndexRow 是扫描时用的轻量索引（只取增量比对需要的字段）。
type BookIndexRow struct {
	Path       string
	Mtime      int64
	Size       int64
	Cover      string
	Format     string
	CoverState string
}

// IndexByLibrary 返回某个书库已有书籍的增量比对索引，以 path 为键。
func (s *Store) IndexByLibrary(libraryID int64) (map[string]BookIndexRow, error) {
	rows, err := s.db.Query(
		`SELECT path, mtime, size, cover, format, cover_state FROM books WHERE library_id = ?`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]BookIndexRow, 512)
	for rows.Next() {
		var r BookIndexRow
		if err := rows.Scan(&r.Path, &r.Mtime, &r.Size, &r.Cover, &r.Format, &r.CoverState); err != nil {
			return nil, err
		}
		out[r.Path] = r
	}
	return out, rows.Err()
}

// UpsertBook 写入或更新一本书。
//
// 已存在时保留 created_at（入库时间）；封面为空时保留旧封面，避免一次失败的
// 封面提取把已有封面清掉。
func (s *Store) UpsertBook(b *Book) (int64, error) {
	ts := now()
	res, err := s.db.Exec(`
		INSERT INTO books (library_id, path, dir, filename, title, author, format, size, mtime, cover, cover_state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(library_id, path) DO UPDATE SET
			dir         = excluded.dir,
			filename    = excluded.filename,
			title       = CASE
				WHEN books.meta_locked = 1 THEN books.title
				WHEN excluded.title <> '' THEN excluded.title
				ELSE books.title END,
			author      = CASE
				WHEN books.meta_locked = 1 THEN books.author
				WHEN excluded.author <> '' THEN excluded.author
				ELSE books.author END,
			format      = excluded.format,
			size        = excluded.size,
			mtime       = excluded.mtime,
			cover       = CASE WHEN excluded.cover  <> '' THEN excluded.cover  ELSE books.cover  END,
			cover_state = excluded.cover_state,
			updated_at  = excluded.updated_at
	`, b.LibraryID, b.Path, b.Dir, b.Filename, b.Title, b.Author, b.Format, b.Size, b.Mtime, b.Cover, b.CoverState, ts, ts)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err == nil && id > 0 {
		return id, nil
	}
	var existing int64
	if err := s.db.QueryRow(`SELECT id FROM books WHERE library_id = ? AND path = ?`, b.LibraryID, b.Path).Scan(&existing); err != nil {
		return 0, err
	}
	return existing, nil
}

// UpdateBookManualMeta 写入管理员手动修改的书名 / 作者，并锁定元数据，
// 后续扫描不会把它们覆盖回文件里的原值。
func (s *Store) UpdateBookManualMeta(id int64, title, author string) error {
	res, err := s.db.Exec(`UPDATE books SET title = ?, author = ?, meta_locked = 1, updated_at = ?
		WHERE id = ? AND deleted = 0`, title, author, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBook 从书库移除一本书（软删除：保留原始文件，扫描不会重新加回来）。
func (s *Store) DeleteBook(id int64) error {
	res, err := s.db.Exec(`UPDATE books SET deleted = 1, updated_at = ? WHERE id = ? AND deleted = 0`, now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteBooksByLibrary 清空某个书库下的所有书籍记录。
func (s *Store) DeleteBooksByLibrary(libraryID int64) error {
	_, err := s.db.Exec(`DELETE FROM books WHERE library_id = ?`, libraryID)
	return err
}

// DeleteBooksExcept 删除某个书库下不在 keep 集合中的记录（清理磁盘上已消失的文件）。
// keep 为路径集合。返回被删除的记录数，以及需要一并删除的封面文件名。
func (s *Store) DeleteBooksExcept(libraryID int64, keep map[string]struct{}) (int, []string, error) {
	rows, err := s.db.Query(`SELECT id, path, cover FROM books WHERE library_id = ?`, libraryID)
	if err != nil {
		return 0, nil, err
	}
	type stale struct {
		id    int64
		cover string
	}
	var drop []stale
	for rows.Next() {
		var id int64
		var path, cover string
		if err := rows.Scan(&id, &path, &cover); err != nil {
			rows.Close()
			return 0, nil, err
		}
		if _, ok := keep[path]; !ok {
			drop = append(drop, stale{id: id, cover: cover})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, nil, err
	}
	if len(drop) == 0 {
		return 0, nil, nil
	}

	covers := make([]string, 0, len(drop))
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`DELETE FROM books WHERE id = ?`)
	if err != nil {
		return 0, nil, err
	}
	defer stmt.Close()

	for _, d := range drop {
		if _, err := stmt.Exec(d.id); err != nil {
			return 0, nil, err
		}
		if d.cover != "" {
			covers = append(covers, d.cover)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return len(drop), covers, nil
}

// ------------------------------------------------------------------ 系列识别
//
// 「系列」不是一个需要维护的字段，而是一个可以从现有数据里推出来的东西：
// 书库管理员给多卷书命名的方式高度收敛（"三体 2"、"基地 Vol.3"、"第3卷"），
// 目录名也常常直接就是系列名。于是这里只做一次纯文本解析，不引入新表、
// 不改扫描逻辑，旧数据库升级上来立刻就有系列可看。解析失败就是"没有系列"，
// 界面上自然退化成普通卡片，不需要任何特判。

var (
	// 明确带卷号记号的写法：Vol.3 / vol 3 / Volume III / 第3卷 / 第三部 / Book 2。
	// 注意 Go 的 regexp 不支持局部大小写开关，整条表达式已经统一用 (?i)。
	volumeRe = regexp.MustCompile(`(?i)(?:\bvol(?:ume)?\.?|\bbook|第)\s*` +
		`([0-9]{1,4}|[ivx]{1,5}|[零一二两三四五六七八九十百]{1,4})` +
		`\s*(?:卷|册|部|篇|集|季)?`)
	// 标题末尾的裸卷号：冰与火之歌 2 / 冰与火之歌（2）。
	// 只认 1~2 位数字，避免把 "1984" 这类数字标题当成卷号。
	volumeTrailRe = regexp.MustCompile(`(?:[\s\-_·:：]+[0-9]{1,2}(?:[\s\-_·:：]+[0-9]{1,2}){0,2}` +
		`(?:[\s\-_·:：]*[（(][0-9]{1,2}[)）])*|[\s\-_·:：]*[（(][0-9]{1,2}[)）])$`)
	// 上下册："人类简史 上" / "生活与命运（上下册）"
	splitTailRe = regexp.MustCompile(`[\s\-_·:：]*[（(]?[上中下]册?[)）]?$`)
	// "全9册"这类整体标注：先从标题里抹掉，它不参与卷号判定。
	allVolsRe = regexp.MustCompile(`[\s\-_·:：]*[（(]?\s*全\s*[0-9一二三四五六七八九十百]{1,3}\s*册\s*[)）]?`)
	decorRe   = regexp.MustCompile(`^[\s\-_·:：,.、|]+|[\s\-_·:：,.、|]+$`)
	// 书名里常见的版本/来源标记，识别系列前先剥掉。
	noiseRe = regexp.MustCompile(`(?i)[\[【(（]\s*(?:epub|pdf|mobi|azw3?|txt|扫描版|影印版|完整版|精校版)\s*[\]】)）]`)
)

var cnDigits = map[rune]int{
	'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4,
	'五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
}

var romanDigits = map[string]string{
	"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5",
	"vi": "6", "vii": "7", "viii": "8", "ix": "9", "x": "10",
}

// cnNumber 把 1~999 的中文数字转成阿拉伯数字；无法解析时返回空串。
func cnNumber(s string) string {
	if s == "" {
		return ""
	}
	total, cur := 0, 0
	for _, r := range s {
		switch r {
		case '百':
			if cur == 0 {
				cur = 1
			}
			total += cur * 100
			cur = 0
		case '十':
			if cur == 0 {
				cur = 1
			}
			total += cur * 10
			cur = 0
		default:
			d, ok := cnDigits[r]
			if !ok {
				return ""
			}
			cur = d
		}
	}
	total += cur
	if total <= 0 || total > 999 {
		return ""
	}
	return strconv.Itoa(total)
}

// normVolume 把各种卷号写法统一成阿拉伯数字字符串。
func normVolume(raw string) string {
	raw = strings.TrimSpace(raw)
	low := strings.ToLower(raw)
	if v, ok := romanDigits[low]; ok && !strings.ContainsAny(low, "0123456789") {
		return v
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return strconv.Itoa(n)
	}
	return cnNumber(raw)
}

// SplitSeries 从书名与目录名里识别系列名与卷号。
//
// 返回的系列名为空表示"这本书不属于任何系列"。判定刻意保守：只有书名主体
// 后面明确跟着卷号记号时才给出系列名 —— 宁可漏判，也不要误判。误判会把毫不
// 相干的书并成一个假系列，比不做聚合更难看。给出"某某"但没有卷号的情形，
// 由调用方按"同名至少两本"决定是否成组。
func SplitSeries(title, dir string) (series, volume string) {
	t := decorRe.ReplaceAllString(noiseRe.ReplaceAllString(strings.TrimSpace(title), " "), "")
	t = decorRe.ReplaceAllString(allVolsRe.ReplaceAllString(t, ""), "")
	if t == "" {
		return "", ""
	}

	// 1) 明确带卷号记号：第3卷 / Vol.3 / 第三部 / Book 2。
	//    卷号后面必须紧跟"卷/册/部…"或直接到句末，否则它只是标题里的一个数字
	//    （"三体 2：黑暗森林"里的 2 是卷号，但 "第 2 版" 里的 2 不是）。
	if m := volumeRe.FindStringSubmatchIndex(t); m != nil {
		if vol := normVolume(t[m[2]:m[3]]); vol != "" {
			head := decorRe.ReplaceAllString(t[:m[0]], "")
			term := decorRe.ReplaceAllString(t[m[1]:], "")
			hasUnit := m[1] > m[3] && decorRe.ReplaceAllString(t[m[3]:m[1]], "") != ""
			if utf8RuneCount(head) >= 2 && (term == "" || hasUnit) {
				return head, vol
			}
		}
	}

	// 2) 标题末尾的裸卷号：冰与火之歌 2 / 冰与火之歌（2）。
	//    紧邻卷号的是拉丁字母时放过（"Windows 10" 这类版本号不是卷号）。
	if m := volumeTrailRe.FindStringIndex(t); m != nil {
		head := decorRe.ReplaceAllString(t[:m[0]], "")
		if vol := normVolume(lastNumber(t[m[0]:])); vol != "" &&
			utf8RuneCount(head) >= 2 && !hasLatinLetterAt(t, m[0]) {
			return head, vol
		}
	}

	// 3) 上下册："人类简史 上" / "生活与命运（上下册）"，卷号留空。
	if m := splitTailRe.FindStringIndex(t); m != nil {
		if head := decorRe.ReplaceAllString(t[:m[0]], ""); utf8RuneCount(head) >= 2 {
			return head, ""
		}
	}

	// 4) 目录名就是系列名、书名是其中一卷（/books/三体/黑暗森林.epub）。
	//    只有书名以目录名开头时才算数，否则同一目录下不相干的两本书
	//    会被并成一个假系列 —— 那比不聚合更难看。
	if d := decorRe.ReplaceAllString(strings.TrimSpace(dir), ""); d != "" && d != t && utf8RuneCount(d) >= 2 {
		if strings.HasPrefix(t, d) {
			return d, ""
		}
	}
	return "", ""
}

// hasLatinLetterAt 判断 pos 之前紧邻的字符是不是拉丁字母 ——
// 用来放过 "Windows 10" 这类"产品名 + 版本号"，它们不该被当成第 10 卷。
func hasLatinLetterAt(s string, pos int) bool {
	r := []rune(s[:pos])
	if len(r) == 0 {
		return false
	}
	c := r[len(r)-1]
	return c < 0x80 && ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'))
}

// lastNumber 取片段里最后一个数字（"12" 取 12，"2 (3)" 取 3）。
func lastNumber(s string) string {
	end := -1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] >= '0' && s[i] <= '9' {
			end = i
			break
		}
	}
	if end < 0 {
		return ""
	}
	start := end
	for start > 0 && s[start-1] >= '0' && s[start-1] <= '9' {
		start--
	}
	return s[start : end+1]
}

func utf8RuneCount(s string) int { return len([]rune(s)) }

// escapeLike 转义 LIKE 通配符，配合 ESCAPE '\' 使用。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
