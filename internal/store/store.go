// Package store 封装 SQLite 持久化。
//
// 使用 modernc.org/sqlite（纯 Go，无 CGO），整个系统只需要一个 .db 文件。
package store

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"modernc.org/sqlite" // 注册 sqlite 驱动，并提供标量函数注册接口
)

// Store 是数据库句柄的薄封装。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）SQLite 数据库。
func Open(path string) (*Store, error) {
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(10000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite 在 WAL 模式下允许并发读 + 单写，连接数保持在较小值即可，
	// 配合 busy_timeout 可以避免写冲突直接报错。
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(time.Hour)

	registerNaturalSort(db)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// Pinger 供健康检查使用。
func (s *Store) Ping() error { return s.db.Ping() }

// DB 暴露底层句柄，仅供 store 包内部与测试使用。
func (s *Store) DB() *sql.DB { return s.db }

// Migrate 建表。所有语句都是幂等的，可以反复执行。
func (s *Store) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      TEXT    NOT NULL UNIQUE,
			password_hash TEXT    NOT NULL,
			role          TEXT    NOT NULL DEFAULT 'user',
			created_at    INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token      TEXT    PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,

		`CREATE TABLE IF NOT EXISTS libraries (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			name         TEXT    NOT NULL,
			path         TEXT    NOT NULL UNIQUE,
			icon         TEXT    NOT NULL DEFAULT 'builtin:book',
			cover        TEXT    NOT NULL DEFAULT '',
			created_at   INTEGER NOT NULL,
			updated_at   INTEGER NOT NULL,
			last_scan_at INTEGER NOT NULL DEFAULT 0,
			last_scan_state TEXT NOT NULL DEFAULT '',
			series_group INTEGER NOT NULL DEFAULT 0
		)`,

		`CREATE TABLE IF NOT EXISTS books (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			library_id INTEGER NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
			path       TEXT    NOT NULL,
			dir        TEXT    NOT NULL DEFAULT '',
			filename   TEXT    NOT NULL,
			title      TEXT    NOT NULL DEFAULT '',
			author     TEXT    NOT NULL DEFAULT '',
			format     TEXT    NOT NULL DEFAULT '',
			size       INTEGER NOT NULL DEFAULT 0,
			mtime      INTEGER NOT NULL DEFAULT 0,
			cover      TEXT    NOT NULL DEFAULT '',
			cover_state TEXT   NOT NULL DEFAULT '',
			meta_locked INTEGER NOT NULL DEFAULT 0,
			deleted     INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_books_library_path ON books(library_id, path)`,
		`CREATE INDEX IF NOT EXISTS idx_books_library ON books(library_id)`,
		`CREATE INDEX IF NOT EXISTS idx_books_title ON books(title)`,
		`CREATE INDEX IF NOT EXISTS idx_books_author ON books(author)`,
		`CREATE INDEX IF NOT EXISTS idx_books_format ON books(format)`,

		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		// 每日推荐（编辑精选）：管理员手工维护的一小份书单，首页横幅从中随机取。
		// position 决定后台列表里的显示顺序；书本身被删时这里不动，
		// 查询时联表 deleted = 0 过滤，误删后再加回来仍然留在书单里。
		`CREATE TABLE IF NOT EXISTS daily_picks (
			book_id    INTEGER PRIMARY KEY REFERENCES books(id) ON DELETE CASCADE,
			position   INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		)`,
	}

	for i, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("执行迁移语句 #%d 失败: %w", i+1, err)
		}
	}

	// 兼容旧库：新加的列需要手动补，CREATE TABLE IF NOT EXISTS 不会改已有表。
	if err := s.ensureColumn("libraries", "cover", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("迁移 libraries.cover 失败: %w", err)
	}
	if err := s.ensureColumn("books", "meta_locked", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("迁移 books.meta_locked 失败: %w", err)
	}
	if err := s.ensureColumn("books", "deleted", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("迁移 books.deleted 失败: %w", err)
	}
	if err := s.ensureColumn("libraries", "series_group", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("迁移 libraries.series_group 失败: %w", err)
	}
	return nil
}

// ensureColumn 检查表中是否已有某列，没有则追加。仅用于本包内的固定表名/列名。
func (s *Store) ensureColumn(table, column, decl string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if found {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + decl)
	return err
}

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("记录不存在")

// ------------------------------------------------------------------ 自然排序
//
// 书名里带数字（"卷10"、"三体 2"、"Vol.12"）时，字符串排序会把 10 排到 1
// 前面。这里注册两个 SQLite 自定义函数，把书名拆成"文字段"和"第一段数字"：
//
//	natural_text('三体 10') → '三体 '   （把数字整段抹掉）
//	natural_num ('三体 10') → 10        （第一段连续数字，没有则 NULL）
//
// ORDER BY natural_text, natural_num 就得到自然的顺序。
// 数字用 int64 解析；超过 18 位（几乎不可能是卷号）时按 0 处理，
// 免得溢出后出现负数把排序搅乱。

func naturalSplit(s string) (string, sql.NullInt64) {
	rs := []rune(s)
	var text strings.Builder
	text.Grow(len(rs))
	// 分隔符（空格、冒号、括号…）不参与比较：这样"三体1"与"三体 1"排在
	// 同一个位置，只按数字区分先后。字母与汉字保留，仍然是先比文字。
	joiners := " \t\u3000:：·・-_/|,，.。()（）[]【】<>《》、"
	var num sql.NullInt64
	numDone := false
	i := 0
	for i < len(rs) {
		if rs[i] >= '0' && rs[i] <= '9' {
			j := i
			for j < len(rs) && rs[j] >= '0' && rs[j] <= '9' {
				j++
			}
			// 第一段数字留下来做排序键，后面的数字同样从文字段里抹掉
			// （比较顺位时这些数字已经不再起作用）。
			if !numDone && j-i <= 18 {
				if v, err := strconv.ParseInt(string(rs[i:j]), 10, 64); err == nil {
					num = sql.NullInt64{Int64: v, Valid: true}
				}
				numDone = true
			}
			i = j
			continue
		}
		if !strings.ContainsRune(joiners, rs[i]) {
			text.WriteRune(rs[i])
		}
		i++
	}
	return text.String(), num
}

// registerNaturalSort 注册自然排序用的两个 SQLite 函数。
//
// modernc.org/sqlite 的函数表是进程级的，同名重复注册会返回错误（测试里会
// 反复 Open 数据库）。所以这里忽略"已注册"错误：函数实现是纯函数，
// 重复注册本来就等价。
func registerNaturalSort(db *sql.DB) {
	_ = db
	_ = sqlite.RegisterDeterministicScalarFunction(naturalTextFn, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			s, _ := sqliteString(args)
			text, _ := naturalSplit(s)
			return text, nil
		})
	_ = sqlite.RegisterDeterministicScalarFunction(naturalNumFn, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			s, _ := sqliteString(args)
			_, num := naturalSplit(s)
			if !num.Valid {
				return nil, nil
			}
			return num.Int64, nil
		})
}

// sqliteString 把函数参数转成字符串；NULL 当作空串。
func sqliteString(args []driver.Value) (string, bool) {
	if len(args) == 0 || args[0] == nil {
		return "", false
	}
	switch v := args[0].(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return fmt.Sprint(v), true
	}
}

func now() int64 { return time.Now().Unix() }

func unixToTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
