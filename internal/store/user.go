package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"libra/internal/auth"
)

// RoleAdmin / RoleUser 是内置角色。
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// User 是账号记录。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// IsAdmin 判断是否管理员。
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// CountUsers 返回用户总数，用于判断是否处于"首次初始化"状态。
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser 新建账号。用户名大小写不敏感（统一转小写存储）。
func (s *Store) CreateUser(username, passwordHash, role string) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if role != RoleAdmin {
		role = RoleUser
	}
	ts := now()
	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, role, ts,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("用户名 %s 已存在", username)
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &User{ID: id, Username: username, Role: role, CreatedAt: unixToTime(ts)}, nil
}

// GetUserByUsername 按用户名查询（大小写不敏感）。
func (s *Store) GetUserByUsername(username string) (*User, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?`, username))
}

// GetUserByID 按 ID 查询。
func (s *Store) GetUserByID(id int64) (*User, error) {
	return s.scanUser(s.db.QueryRow(
		`SELECT id, username, password_hash, role, created_at FROM users WHERE id = ?`, id))
}

func (s *Store) scanUser(row *sql.Row) (*User, error) {
	var u User
	var created int64
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.CreatedAt = unixToTime(created)
	return &u, nil
}

// UpdateUserPassword 修改密码。
func (s *Store) UpdateUserPassword(id int64, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, passwordHash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountAdmins 返回管理员数量，用于保护"系统中至少要留一个管理员"。
func (s *Store) CountAdmins() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE role = ?`, RoleAdmin).Scan(&n)
	return n, err
}

// UpdateUserRole 变更账号角色。非法角色一律回落为普通成员。
func (s *Store) UpdateUserRole(id int64, role string) error {
	if role != RoleAdmin {
		role = RoleUser
	}
	res, err := s.db.Exec(`UPDATE users SET role = ? WHERE id = ?`, role, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser 删除账号。其会话随外键 ON DELETE CASCADE 一并清除。
func (s *Store) DeleteUser(id int64) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListUsers 返回所有账号（不含密码哈希）。
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, role, created_at FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		var u User
		var created int64
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &created); err != nil {
			return nil, err
		}
		u.CreatedAt = unixToTime(created)
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------- 会话 ----------
//
// 存入数据库的是令牌的 SHA-256 摘要，不是令牌原文：即使数据库文件
// 被拿走，里面的值也不能直接当 Cookie 用来冒充登录。

// CreateSession 写入一条会话记录。
func (s *Store) CreateSession(token string, userID int64, ttl time.Duration) error {
	ts := now()
	_, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		auth.TokenDigest(token), userID, ts, time.Now().Add(ttl).Unix(),
	)
	return err
}

// SessionUser 通过 token 取用户，token 过期或不存在时返回 ErrNotFound。
func (s *Store) SessionUser(token string) (*User, error) {
	var userID int64
	err := s.db.QueryRow(
		`SELECT user_id FROM sessions WHERE token = ? AND expires_at > ?`, auth.TokenDigest(token), now(),
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s.GetUserByID(userID)
}

// DeleteSession 注销单个会话。
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, auth.TokenDigest(token))
	return err
}

// DeleteUserSessions 注销某用户的全部会话（改密码时调用）。
func (s *Store) DeleteUserSessions(userID int64) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// PurgeExpiredSessions 清理过期会话。
func (s *Store) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, now())
	return err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "constraint failed") && strings.Contains(msg, "unique")
}
