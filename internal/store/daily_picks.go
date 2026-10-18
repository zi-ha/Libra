package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// 每日推荐（编辑精选）是一份人工维护的书单，不属于任何"扫描"流程：
// 管理员在后台挑几本放进去，首页横幅从中随机取几本轮播。
// 只存书籍 ID，书名 / 封面 / 作者在查询时联表取最新值 ——
// 扫描改过元数据、或者书被移除（软删除）之后，书单里自动呈现最新状态。
//
// 书被软删除时不动推荐记录：这样误删后重新加回来，它仍然留在书单里。
// 是否出现在横幅里由联表条件 deleted = 0 决定。

// DailyPick 是一本每日推荐书，附带排序与加入时间。
type DailyPick struct {
	Book
	PickCreatedAt time.Time `json:"pick_created_at"`
}

const dailyPickColumns = bookColumns + `, d.created_at`

func scanDailyPick(row interface{ Scan(...any) error }) (*DailyPick, error) {
	var p DailyPick
	var created, updated, pickCreated int64
	if err := row.Scan(&p.ID, &p.LibraryID, &p.LibraryName, &p.Path, &p.Dir, &p.Filename,
		&p.Title, &p.Author, &p.Format, &p.Size, &p.Mtime, &p.Cover, &p.CoverState,
		&p.MetaLocked, &p.Deleted, &created, &updated, &pickCreated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.CreatedAt = unixToTime(created)
	p.UpdatedAt = unixToTime(updated)
	p.PickCreatedAt = unixToTime(pickCreated)
	return &p, nil
}

// ListDailyPicks 返回当前有效的每日推荐（跳过已软删除的书），按加入顺序排列。
//
// 书其实不存在时（比如书库被删了）顺手清掉对应记录，避免书单里残留幽灵条目；
// 返回的 books 集合里找不到对应书时删一行是安全的，因为这是单人维护的小书单。
func (s *Store) ListDailyPicks() ([]DailyPick, error) {
	rows, err := s.db.Query(`SELECT ` + dailyPickColumns + `
		FROM daily_picks d
		JOIN books b ON b.id = d.book_id
		LEFT JOIN libraries l ON l.id = b.library_id
		WHERE b.deleted = 0
		ORDER BY d.position ASC, d.created_at ASC, d.book_id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DailyPick, 0, 16)
	for rows.Next() {
		p, err := scanDailyPick(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// PruneDailyPicks 删除指向已不存在书籍（含软删除）的推荐记录，返回清理条数。
// 在读取书单时调用，保证书单不会随着时间推移积累僵尸条目。
func (s *Store) PruneDailyPicks() (int, error) {
	res, err := s.db.Exec(`DELETE FROM daily_picks
		WHERE book_id NOT IN (SELECT id FROM books)`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DailyPickIDs 返回书单里的书籍 ID（含已软删除的书），用于"是否已加入"的前端状态。
func (s *Store) DailyPickIDs() ([]int64, error) {
	rows, err := s.db.Query(`SELECT book_id FROM daily_picks ORDER BY position ASC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]int64, 0, 16)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// AddDailyPick 把一本书加入每日推荐。重复加入是幂等的（沿用原有顺序）。
func (s *Store) AddDailyPick(bookID int64) error {
	var exists int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM books WHERE id = ? AND deleted = 0`, bookID).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return ErrNotFound
	}
	var pos sql.NullInt64
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM daily_picks`).Scan(&pos); err != nil {
		return err
	}
	next := pos.Int64 + 1
	_, err := s.db.Exec(`INSERT INTO daily_picks (book_id, position, created_at) VALUES (?, ?, ?)
		ON CONFLICT(book_id) DO NOTHING`, bookID, next, now())
	return err
}

// RemoveDailyPick 把一本书移出每日推荐。
func (s *Store) RemoveDailyPick(bookID int64) error {
	res, err := s.db.Exec(`DELETE FROM daily_picks WHERE book_id = ?`, bookID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDailyPicks 用给定的 ID 列表整体替换书单（保持顺序，忽略无效 ID）。
// 用于批量编辑：前端一次提交完整列表，服务端一次事务落库。
func (s *Store) SetDailyPicks(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM daily_picks`); err != nil {
		return err
	}
	seen := map[int64]bool{}
	pos := int64(0)
	ts := now()
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM books WHERE id = ? AND deleted = 0`, id).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		pos++
		if _, err := tx.Exec(`INSERT INTO daily_picks (book_id, position, created_at) VALUES (?, ?, ?)`, id, pos, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListBooksByIDs 按给定 ID 批量读取书籍，返回顺序与传入的 ids 一致；
// 不存在或已软删除的 ID 会被直接跳过。
func (s *Store) ListBooksByIDs(ids []int64) ([]Book, error) {
	if len(ids) == 0 {
		return []Book{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT `+bookColumns+` FROM books b
		LEFT JOIN libraries l ON l.id = b.library_id
		WHERE b.deleted = 0 AND b.id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	found := make(map[int64]Book, len(ids))
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		found[b.ID] = *b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Book, 0, len(found))
	for _, id := range ids {
		if b, ok := found[id]; ok {
			out = append(out, b)
		}
	}
	return out, nil
}

// RandomBooks 从一本书库里随机取 limit 本书。onlyWithCover 为 true 时只取带封面的
// （横幅靠封面撑场面，没有封面的书放进去只是一块灰色）。
//
// 用 ORDER BY RANDOM() 而不是"先算总数再随机 offset"：书库几千本时前者也只需要
// 一次全表扫描，而后者在书很多时反而要多一次 COUNT 查询，代码也复杂。
func (s *Store) RandomBooks(libraryID int64, onlyWithCover bool, limit int) ([]Book, error) {
	if limit <= 0 {
		return []Book{}, nil
	}
	where := "b.deleted = 0"
	args := []any{}
	if libraryID > 0 {
		where += " AND b.library_id = ?"
		args = append(args, libraryID)
	}
	if onlyWithCover {
		where += " AND b.cover <> ''"
	}
	args = append(args, limit)

	rows, err := s.db.Query(`SELECT `+bookColumns+` FROM books b
		LEFT JOIN libraries l ON l.id = b.library_id
		WHERE `+where+` ORDER BY RANDOM() LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Book, 0, limit)
	for rows.Next() {
		b, err := scanBook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// RemoveBooksFromDailyPicks 在书籍被永久删除（或软删除后清理）时同步移出书单。
func (s *Store) RemoveBooksFromDailyPicks(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	_, err := s.db.Exec(`DELETE FROM daily_picks WHERE book_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	return err
}
