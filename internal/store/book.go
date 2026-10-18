package store

import (
	"database/sql"
	"errors"
	"fmt"
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
	Title       string `json:"title"`
	Author      string `json:"author"`
	Format      string `json:"format"` // epub / pdf
	Size        int64  `json:"size"`
	Mtime       int64  `json:"mtime"`
	Cover       string `json:"cover"` // 封面文件名（相对封面目录）
	// CoverState 记录封面提取结果（ok / none / failed），增量扫描据此决定是否重试。
	CoverState string    `json:"-"`
	MetaLocked int       `json:"-"` // 1 = 管理员手动改过书名/作者，扫描不覆盖
	Deleted    int       `json:"-"` // 1 = 从书库移除（软删除，保留原文件）
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

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
}

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

// escapeLike 转义 LIKE 通配符，配合 ESCAPE '\' 使用。
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
