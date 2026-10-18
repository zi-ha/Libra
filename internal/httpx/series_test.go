package httpx

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"libra/internal/store"
)

// addBook 在指定书库的某个相对路径上放一本书。
func addBook(t *testing.T, st *store.Store, libID int64, root, rel string) int64 {
	t.Helper()
	b := &store.Book{
		LibraryID: libID,
		Path:      filepath.Join(root, filepath.FromSlash(rel)),
		Filename:  filepath.Base(rel),
		Title:     rel,
		Format:    "epub",
		Cover:     "c.jpg",
	}
	id, err := st.UpsertBook(b)
	if err != nil {
		t.Fatalf("写入书籍失败: %v", err)
	}
	return id
}

// 文档里的例子：散书两本 + 三体系列三本，开启聚合后 series 生效、可以按组筛选。
func TestSeriesGroupingSpecExample(t *testing.T) {
	st := newTestStore(t)
	root := t.TempDir()
	lib, err := st.CreateLibrary("测试", root, "builtin:book")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"金庸", "科幻/三体系列"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	addBook(t, st, lib.ID, root, "心理学入门.epub")
	addBook(t, st, lib.ID, root, "金庸/射雕英雄传.epub")
	three := []string{
		"科幻/三体系列/三体1 地球往事.epub",
		"科幻/三体系列/三体2 黑暗森林.epub",
		"科幻/三体系列/三体3 死神永生.epub",
	}
	for _, rel := range three {
		addBook(t, st, lib.ID, root, rel)
	}

	s := &Server{st: st}
	gin.SetMode(gin.TestMode)

	call := func(query string) map[string]any {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/books?page_size=60&"+query, nil)
		s.listBooks(c)
		if w.Code != http.StatusOK {
			t.Fatalf("状态码 %d: %s", w.Code, w.Body.String())
		}
		return decodeData(t, w.Body.Bytes())
	}

	// 默认关闭：不带系列信息，全部 5 本都是原来的平铺展示。
	before := call("library_id=" + itoa(lib.ID))
	if before["series"] == true {
		t.Fatal("默认不应该启用系列聚合")
	}
	if n := len(before["books"].([]any)); n != 5 {
		t.Fatalf("关闭聚合时应展示全部 5 本，实际 %d", n)
	}

	// 打开开关
	if err := st.SetLibrarySeriesGroup(lib.ID, true); err != nil {
		t.Fatal(err)
	}

	all := call("library_id=" + itoa(lib.ID))
	if all["series"] != true {
		t.Fatal("开启后 series 应为 true")
	}
	groups := all["groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("应只有 1 个系列，实际 %d: %v", len(groups), groups)
	}
	g := groups[0].(map[string]any)
	if g["name"] != "三体系列" || g["count"].(float64) != 3 {
		t.Fatalf("系列应是 三体系列·3，实际 %v", g)
	}

	// 只看某个系列
	one := call("library_id=" + itoa(lib.ID) + "&series=" + "三体系列")
	if n := len(one["books"].([]any)); n != 3 {
		t.Fatalf("三体系列应有 3 本，实际 %d", n)
	}
	for _, raw := range one["books"].([]any) {
		if raw.(map[string]any)["series_name"] != "三体系列" {
			t.Fatalf("系列里的书应带 series_name，实际 %v", raw.(map[string]any)["series_name"])
		}
	}

	// 只看散书
	loose := call("library_id=" + itoa(lib.ID) + "&series=none")
	if n := len(loose["books"].([]any)); n != 2 {
		t.Fatalf("散书应有 2 本，实际 %d", n)
	}

	// 关闭开关：系列筛选不再生效，回到平铺展示
	if err := st.SetLibrarySeriesGroup(lib.ID, false); err != nil {
		t.Fatal(err)
	}
	off := call("library_id=" + itoa(lib.ID))
	if off["series"] == true {
		t.Fatal("关闭后不应再标记系列")
	}
	if n := len(off["books"].([]any)); n != 5 {
		t.Fatalf("关闭后应展平展示全部 5 本，实际 %d", n)
	}
	if gs, _ := off["groups"].([]any); len(gs) != 0 {
		t.Fatalf("关闭后不应返回系列列表，实际 %v", gs)
	}
	// 关掉之后，伪造的系列筛选也不能把书筛没：没有"系列"这个概念了。
	if n := len(call("library_id=" + itoa(lib.ID) + "&series=" + "三体系列")["books"].([]any)); n != 5 {
		t.Fatalf("关闭聚合后系列筛选应被忽略，实际剩 %d 本", n)
	}
}

// 所有书都直接在书库根目录：BASE_DEPTH = 1，谁都不参与聚合。
func TestSeriesGroupingAllAtRoot(t *testing.T) {
	st := newTestStore(t)
	root := t.TempDir()
	lib, err := st.CreateLibrary("平铺", root, "builtin:book")
	if err != nil {
		t.Fatal(err)
	}
	addBook(t, st, lib.ID, root, "a.epub")
	addBook(t, st, lib.ID, root, "b.epub")
	if err := st.SetLibrarySeriesGroup(lib.ID, true); err != nil {
		t.Fatal(err)
	}

	s := &Server{st: st}
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/books?library_id="+itoa(lib.ID), nil)
	s.listBooks(c)
	data := decodeData(t, w.Body.Bytes())
	if gs := data["groups"].([]any); len(gs) != 0 {
		t.Fatalf("全是根目录书不应产生系列，实际 %v", gs)
	}
	for _, raw := range data["books"].([]any) {
		if _, ok := raw.(map[string]any)["series_name"]; ok {
			t.Fatal("根目录的书不应带 series_name")
		}
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
