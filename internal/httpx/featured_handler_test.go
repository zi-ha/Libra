package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// decodeData 取出统一响应结构里的 data 字段。
func decodeData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("解析响应失败: %v (%s)", err, body)
	}
	return env.Data
}

// 空态检查：只用每日推荐、书单为空、书库源不可用 → 横幅必须返回空数组。
// 顺便验证"空结果不粘住缓存"：加一本书之后下一次请求立刻就能取到。
func TestFeaturedEmptyThenFilled(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 3, true)

	s := &Server{st: st}
	gin.SetMode(gin.TestMode)

	// 数据源：只开每日推荐，书库指向不存在的库（等价于"只用每日推荐"）
	if err := st.SetDailyPicksEnabled(true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetDailyPicks(nil); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFeaturedLibraryIDs([]int64{424242}); err != nil {
		t.Fatal(err)
	}

	call := func() []any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/featured?limit=5", nil)
		s.featured(c)
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 %d: %s", w.Code, w.Body.String())
		}
		data := decodeData(t, w.Body.Bytes())
		books, _ := data["books"].([]any)
		return books
	}

	if got := call(); len(got) != 0 {
		t.Fatalf("书单为空时应该返回空横幅，实际 %d 本", len(got))
	}

	all := allBooks(t, st)
	if err := st.AddDailyPick(all[0].ID); err != nil {
		t.Fatal(err)
	}
	got := call()
	if len(got) != 1 {
		t.Fatalf("加入一本后应该立刻返回 1 本，实际 %d 本", len(got))
	}
}

// 缓存命中时，一小时内两次请求的顺序必须完全一致（服务端随机结果不抖动）。
func TestFeaturedHandlerStableWithinTTL(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 10, true)
	if err := st.SetDailyPicksEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFeaturedLibraryIDs(nil); err != nil {
		t.Fatal(err)
	}
	s := &Server{st: st}
	gin.SetMode(gin.TestMode)

	call := func() []any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/featured?limit=6", nil)
		s.featured(c)
		return decodeData(t, w.Body.Bytes())["books"].([]any)
	}

	first := call()
	second := call()
	if len(first) != 6 || len(second) != 6 {
		t.Fatalf("期望各 6 本，实际 %d / %d", len(first), len(second))
	}
	for i := range first {
		a := first[i].(map[string]any)["id"]
		b := second[i].(map[string]any)["id"]
		if a != b {
			t.Fatalf("同一小时内顺序应保持不变，位置 %d：%v vs %v", i, a, b)
		}
	}

	// refresh=1 应该绕过缓存重新随机（这里只验证不报错且仍然是 6 本）
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/featured?limit=6&refresh=1", nil)
	s.featured(c)
	if got := decodeData(t, w.Body.Bytes())["books"].([]any); len(got) != 6 {
		t.Fatalf("refresh 后应仍为 6 本，实际 %d", len(got))
	}
}
