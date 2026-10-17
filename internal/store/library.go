package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Library 是一个媒体库。每个媒体库对应 NAS 上的一个本地目录。
type Library struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Icon          string    `json:"icon"`
	Cover         string    `json:"cover"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LastScanAt    time.Time `json:"last_scan_at"`
	LastScanState string    `json:"last_scan_state"`

	// 冗余统计字段，由查询时联表算出，不落库。
	BookCount int `json:"book_count"`
	EpubCount int `json:"epub_count"`
	PdfCount  int `json:"pdf_count"`
}

const libraryColumns = `l.id, l.name, l.path, l.icon, l.cover, l.created_at, l.updated_at, l.last_scan_at, l.last_scan_state,
	COALESCE((SELECT COUNT(*) FROM books b WHERE b.library_id = l.id), 0),
	COALESCE((SELECT COUNT(*) FROM books b WHERE b.library_id = l.id AND b.format = 'epub'), 0),
	COALESCE((SELECT COUNT(*) FROM books b WHERE b.library_id = l.id AND b.format = 'pdf'), 0)`

func scanLibrary(row interface{ Scan(...any) error }) (*Library, error) {
	var l Library
	var created, updated, lastScan int64
	if err := row.Scan(&l.ID, &l.Name, &l.Path, &l.Icon, &l.Cover, &created, &updated, &lastScan,
		&l.LastScanState, &l.BookCount, &l.EpubCount, &l.PdfCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.CreatedAt = unixToTime(created)
	l.UpdatedAt = unixToTime(updated)
	l.LastScanAt = unixToTime(lastScan)
	return &l, nil
}

// CreateLibrary 新建媒体库。
func (s *Store) CreateLibrary(name, path, icon string) (*Library, error) {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" {
		return nil, errors.New("媒体库名称不能为空")
	}
	if path == "" {
		return nil, errors.New("目录路径不能为空")
	}
	if icon == "" {
		icon = "builtin:book"
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO libraries (name, path, icon, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		name, path, icon, ts, ts,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, errors.New("该目录已经被其它媒体库使用")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetLibrary(id)
}

// UpdateLibrary 局部更新，nil 表示该字段不变。
func (s *Store) UpdateLibrary(id int64, name, path, icon *string) (*Library, error) {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if name != nil {
		v := strings.TrimSpace(*name)
		if v == "" {
			return nil, errors.New("媒体库名称不能为空")
		}
		sets = append(sets, "name = ?")
		args = append(args, v)
	}
	if path != nil {
		v := strings.TrimSpace(*path)
		if v == "" {
			return nil, errors.New("目录路径不能为空")
		}
		sets = append(sets, "path = ?")
		args = append(args, v)
	}
	if icon != nil {
		sets = append(sets, "icon = ?")
		args = append(args, strings.TrimSpace(*icon))
	}
	if len(sets) == 0 {
		return s.GetLibrary(id)
	}
	sets = append(sets, "updated_at = ?")
	args = append(args, now())
	args = append(args, id)

	res, err := s.db.Exec(`UPDATE libraries SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, errors.New("该目录已经被其它媒体库使用")
		}
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetLibrary(id)
}

// SetLibraryCover 记录媒体库自定义封面的文件名（空字符串表示恢复自动封面）。
func (s *Store) SetLibraryCover(id int64, cover string) error {
	res, err := s.db.Exec(`UPDATE libraries SET cover = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(cover), now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteLibrary 删除媒体库。书籍记录通过外键级联删除，调用方负责清理封面文件。
func (s *Store) DeleteLibrary(id int64) error {
	res, err := s.db.Exec(`DELETE FROM libraries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetLibrary 按 ID 查询。
func (s *Store) GetLibrary(id int64) (*Library, error) {
	return scanLibrary(s.db.QueryRow(`SELECT `+libraryColumns+` FROM libraries l WHERE l.id = ?`, id))
}

// ListLibraries 返回全部媒体库，附带书籍数量统计。
func (s *Store) ListLibraries() ([]Library, error) {
	rows, err := s.db.Query(`SELECT ` + libraryColumns + ` FROM libraries l ORDER BY l.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Library, 0, 8)
	for rows.Next() {
		l, err := scanLibrary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// HasLibraryWithPath 判断目录是否已被某个媒体库占用（可排除指定 ID）。
func (s *Store) HasLibraryWithPath(path string, excludeID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM libraries WHERE path = ? AND id <> ?`, path, excludeID).Scan(&n)
	return n > 0, err
}

// MarkLibraryScan 记录一次扫描的结果状态。
func (s *Store) MarkLibraryScan(id int64, state string) error {
	_, err := s.db.Exec(
		`UPDATE libraries SET last_scan_at = ?, last_scan_state = ? WHERE id = ?`, now(), state, id)
	return err
}
