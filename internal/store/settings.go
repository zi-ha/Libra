package store

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GetSetting 读取一个设置项；不存在时返回 ErrNotFound。
func (s *Store) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return value, err
}

// SetSetting 写入或更新一个设置项。
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// DeleteSetting 删除一个设置项（回到默认值）。
func (s *Store) DeleteSetting(key string) error {
	_, err := s.db.Exec(`DELETE FROM settings WHERE key = ?`, key)
	return err
}

// ------------------------------------------------------------------ 首页横幅

const (
	// settingFeaturedRotation 记录首页横幅上一轮取出的书籍 ID（逗号分隔）。
	// 它的时间戳就是"上次刷新时刻" —— 横幅内容每小时换一批，而读者翻页时
	// 看到的先后顺序在整个小时里保持稳定，不会刷新一次页面就换一套。
	settingFeaturedRotation = "featured_rotation"
	// settingFeaturedRefreshAt 是横幅下一批内容的生成时间（Unix 秒）。
	settingFeaturedRefreshAt = "featured_refresh_at"

	// FeaturedRotationTTL 是首页横幅刷新一轮内容的间隔：一小时。
	// 一小时内顺序稳定，整点过后读者刷新页面才会换一批。
	FeaturedRotationTTL = time.Hour
)

// GetDailyPicksEnabled 返回"每日推荐"内容是否参与首页横幅。
// 设置项不存在时默认开启：开箱即用，管理员想关再关。
func (s *Store) GetDailyPicksEnabled() bool {
	v, err := s.GetSetting("daily_picks_enabled")
	if err != nil {
		return true
	}
	return v == "1" || v == "true"
}

// SetDailyPicksEnabled 保存"每日推荐"内容是否参与首页横幅。
func (s *Store) SetDailyPicksEnabled(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return s.SetSetting("daily_picks_enabled", v)
}

// FeaturedLibraryIDs 返回参与首页横幅的书库 ID 集合。
// 一个都没配置时返回单元素切片 [0]，0 表示"全部书库"，即升级前的老行为。
func (s *Store) FeaturedLibraryIDs() []int64 {
	v, err := s.GetSetting("featured_libraries")
	if err != nil || strings.TrimSpace(v) == "" {
		return []int64{0}
	}
	out := make([]int64, 0, 4)
	for _, part := range strings.Split(v, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return []int64{0}
	}
	return out
}

// SetFeaturedLibraryIDs 保存参与首页横幅的书库；空列表等价于"全部书库"。
func (s *Store) SetFeaturedLibraryIDs(ids []int64) error {
	seen := map[int64]bool{}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	if len(parts) == 0 {
		return s.DeleteSetting("featured_libraries")
	}
	return s.SetSetting("featured_libraries", strings.Join(parts, ","))
}

// FeaturedRotation 返回上一轮横幅的书籍顺序，以及这批内容是否已经过期。
// 过期时调用方应该重新生成；未过期就直接沿用，保证同一小时内稳定。
func (s *Store) FeaturedRotation() (ids []int64, fresh bool) {
	raw, err := s.GetSetting(settingFeaturedRotation)
	if err == nil {
		for _, part := range strings.Split(raw, ",") {
			if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
				ids = append(ids, id)
			}
		}
	}
	nextRaw, err := s.GetSetting(settingFeaturedRefreshAt)
	if err != nil {
		return ids, false
	}
	next, err := strconv.ParseInt(strings.TrimSpace(nextRaw), 10, 64)
	if err != nil {
		return ids, false
	}
	return ids, time.Now().Unix() < next
}

// SetFeaturedRotation 记录本轮横幅的书籍顺序，并把刷新时刻推进一个周期。
func (s *Store) SetFeaturedRotation(ids []int64) error {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	if err := s.SetSetting(settingFeaturedRotation, strings.Join(parts, ",")); err != nil {
		return err
	}
	next := time.Now().Add(FeaturedRotationTTL).Unix()
	return s.SetSetting(settingFeaturedRefreshAt, fmt.Sprintf("%d", next))
}
