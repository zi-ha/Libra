package httpx

import (
	"strings"

	"github.com/gin-gonic/gin"

	"libra/internal/store"
)

// 「每日推荐」是首页横幅的数据源之一：一份管理员人工维护的书单。
// 书单本身不依赖任何扫描结果，删掉一本书之后书单里对应的记录也会自动失效，
// 重新加回来又会出现 —— 管理员不需要维护两遍。

// listDailyPicks 返回每日推荐书单（含书名 / 封面，按加入顺序）。
func (s *Server) listDailyPicks(c *gin.Context) {
	// 书库被删除后，个别 book_id 可能已经没有对应书籍：顺手清掉，书单保持干净。
	_, _ = s.st.PruneDailyPicks()

	picks, err := s.st.ListDailyPicks()
	if err != nil {
		failErr(c, 500, "读取每日推荐失败", err)
		return
	}

	items := make([]gin.H, 0, len(picks))
	for i := range picks {
		item := bookJSON(&picks[i].Book)
		item["picked_at"] = picks[i].PickCreatedAt
		items = append(items, item)
	}

	libIDs := s.st.FeaturedLibraryIDs()
	ok(c, gin.H{
		"books":         items,
		"total":         len(items),
		"picks_enabled": s.st.GetDailyPicksEnabled(),
		"library_ids":   libIDs,
		"refresh_ttl_s": int(store.FeaturedRotationTTL.Seconds()),
	})
}

// addDailyPick 把一本书加入每日推荐。
func (s *Server) addDailyPick(c *gin.Context) {
	var req struct {
		BookID int64 `json:"book_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BookID <= 0 {
		fail(c, 400, "请求格式错误")
		return
	}
	if err := s.st.AddDailyPick(req.BookID); err != nil {
		if err == store.ErrNotFound {
			fail(c, 404, "书籍不存在")
			return
		}
		failErr(c, 500, "加入每日推荐失败", err)
		return
	}
	s.resetFeaturedRotation()
	ok(c, gin.H{"book_id": req.BookID, "message": "已加入每日推荐"})
}

// removeDailyPick 把一本书移出每日推荐。
func (s *Server) removeDailyPick(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	if err := s.st.RemoveDailyPick(id); err != nil {
		if err == store.ErrNotFound {
			fail(c, 404, "这本书不在每日推荐里")
			return
		}
		failErr(c, 500, "移出每日推荐失败", err)
		return
	}
	s.resetFeaturedRotation()
	ok(c, gin.H{"book_id": id, "message": "已移出每日推荐"})
}

// setDailyPicks 整体替换每日推荐书单（用于批量编辑 / 排序）。
func (s *Server) setDailyPicks(c *gin.Context) {
	var req struct {
		BookIDs []int64 `json:"book_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}
	if len(req.BookIDs) > 200 {
		fail(c, 400, "每日推荐最多 200 本")
		return
	}
	if err := s.st.SetDailyPicks(req.BookIDs); err != nil {
		failErr(c, 500, "保存每日推荐失败", err)
		return
	}
	s.resetFeaturedRotation()
	ok(c, gin.H{"count": len(req.BookIDs), "message": "每日推荐已更新"})
}

// getFeaturedSource 返回首页横幅的数据源配置。
func (s *Server) getFeaturedSource(c *gin.Context) {
	ok(c, gin.H{
		"picks_enabled": s.st.GetDailyPicksEnabled(),
		"library_ids":   s.st.FeaturedLibraryIDs(),
		"refresh_ttl_s": int(store.FeaturedRotationTTL.Seconds()),
	})
}

// updateFeaturedSource 保存首页横幅的数据源配置。
//
// picks_enabled 与 library_ids 都是可选的：只传其中一个就只改那一个，
// 前端切换开关时不必每次把整份配置带回服务端。
func (s *Server) updateFeaturedSource(c *gin.Context) {
	var req struct {
		PicksEnabled *bool   `json:"picks_enabled"`
		LibraryIDs   []int64 `json:"library_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}

	if req.PicksEnabled == nil && req.LibraryIDs == nil {
		fail(c, 400, "没有要修改的内容")
		return
	}

	if req.PicksEnabled != nil {
		if err := s.st.SetDailyPicksEnabled(*req.PicksEnabled); err != nil {
			failErr(c, 500, "保存数据源设置失败", err)
			return
		}
	}
	if req.LibraryIDs != nil {
		// 只接受确实存在的书库，避免前端残留旧 ID 把横幅锁死在一个空书库上。
		valid := make([]int64, 0, len(req.LibraryIDs))
		seen := map[int64]bool{}
		for _, id := range req.LibraryIDs {
			if id <= 0 || seen[id] {
				continue
			}
			seen[id] = true
			if _, err := s.st.GetLibrary(id); err == nil {
				valid = append(valid, id)
			}
		}
		if err := s.st.SetFeaturedLibraryIDs(valid); err != nil {
			failErr(c, 500, "保存数据源设置失败", err)
			return
		}
	}

	s.resetFeaturedRotation()
	libIDs := s.st.FeaturedLibraryIDs()
	picksEnabled := s.st.GetDailyPicksEnabled()

	// 配置里只剩"每日推荐"而书单是空的：横幅会变成一片空白，
	// 这里直接告诉前端，让管理员知道该往书单里加书了。
	warning := ""
	if !picksEnabled && !hasPositiveID(libIDs) {
		// 不可能发生（都没有时就是"全部书库"），保留以防将来改默认值。
		warning = "未启用任何数据源，横幅将不显示"
	} else if picksEnabled && !hasPositiveID(libIDs) && s.countDailyPicks() == 0 {
		warning = "每日推荐还是空的，首页横幅暂时不会有内容"
	}

	ok(c, gin.H{
		"picks_enabled": picksEnabled,
		"library_ids":   libIDs,
		"picks_count":   s.countDailyPicks(),
		"warning":       warning,
	})
}

// resetFeaturedRotation 让下一轮首页横幅立刻重新随机，而不是继续用缓存的那批。
// 管理员改了数据源却要等一小时才看到效果是最让人困惑的事。
func (s *Server) resetFeaturedRotation() {
	_ = s.st.SetFeaturedRotation(nil)
}

// debugExpireFeatured 只在开发模式（LIBRA_WEB_DIR）下挂载：
// 把"下一批生成时间"拨到过去，用来验证一小时刷新是否真的生效。
// 生产环境这个路由不存在。
func (s *Server) debugExpireFeatured(c *gin.Context) {
	if err := s.st.SetSetting("featured_refresh_at", "0"); err != nil {
		failErr(c, 500, "清除横幅缓存失败", err)
		return
	}
	ok(c, gin.H{"message": "横幅缓存已过期，下次请求会重新随机"})
}

func hasPositiveID(ids []int64) bool {
	for _, id := range ids {
		if id > 0 {
			return true
		}
	}
	return false
}

// searchDailyPickCandidates 是给前端"添加书籍"弹窗用的搜索入口：
// 复用普通书籍搜索，但顺手标出哪些书已经在书单里。
func (s *Server) searchDailyPickCandidates(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	libID := int64(queryInt(c, "library_id", 0))

	picked, err := s.st.DailyPickIDs()
	if err != nil {
		failErr(c, 500, "读取每日推荐失败", err)
		return
	}
	inPicks := make(map[int64]bool, len(picked))
	for _, id := range picked {
		inPicks[id] = true
	}

	books, total, err := s.st.ListBooks(store.BookQuery{
		LibraryID: libID,
		Search:    q,
		Page:      1,
		PageSize:  24,
		Sort:      "created",
		Desc:      true,
	})
	if err != nil {
		failErr(c, 500, "搜索书籍失败", err)
		return
	}

	items := make([]gin.H, 0, len(books))
	for i := range books {
		item := bookJSON(&books[i])
		item["in_picks"] = inPicks[books[i].ID]
		items = append(items, item)
	}
	ok(c, gin.H{"books": items, "total": total})
}
