package httpx

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"libra/internal/store"
)

// newTestStore 打开一个临时 SQLite 数据库并完成建表。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "libra.db"))
	if err != nil {
		t.Fatalf("打开测试数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return st
}

// seedLibrary 建一座书库并塞入 n 本书；withCover 控制这些书有没有封面。
func seedLibrary(t *testing.T, st *store.Store, name string, n int, withCover bool) *store.Library {
	t.Helper()
	lib, err := st.CreateLibrary(name, "/tmp/"+name, "builtin:book")
	if err != nil {
		t.Fatalf("创建书库失败: %v", err)
	}
	for i := 0; i < n; i++ {
		b := &store.Book{
			LibraryID: lib.ID,
			Path:      filepath.Join("/tmp", name, fmt.Sprintf("book%02d.epub", i)),
			Filename:  fmt.Sprintf("book%02d.epub", i),
			Title:     fmt.Sprintf("%s 第 %d 本", name, i+1),
			Format:    "epub",
		}
		if withCover {
			b.Cover = fmt.Sprintf("cover%02d.jpg", i)
		}
		if _, err := st.UpsertBook(b); err != nil {
			t.Fatalf("写入书籍失败: %v", err)
		}
	}
	return lib
}

func bookIDs(items []gin.H) []int64 {
	out := make([]int64, 0, len(items))
	for _, it := range items {
		if id, ok := it["id"].(int64); ok {
			out = append(out, id)
		}
	}
	return out
}

func idsOf(books []store.Book) []int64 {
	out := make([]int64, 0, len(books))
	for _, b := range books {
		out = append(out, b.ID)
	}
	return out
}

func allBooks(t *testing.T, st *store.Store) []store.Book {
	t.Helper()
	books, _, err := st.ListBooks(store.BookQuery{Page: 1, PageSize: 300, Sort: "created"})
	if err != nil {
		t.Fatalf("读取书籍失败: %v", err)
	}
	return books
}

// 只用每日推荐：横幅内容必须全部来自书单，一本都不能多。
func TestBuildFeaturedPicksOnly(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 5, true)
	all := allBooks(t, st)

	if err := st.SetDailyPicksEnabled(true); err != nil {
		t.Fatal(err)
	}
	for _, id := range idsOf(all[:2]) {
		if err := st.AddDailyPick(id); err != nil {
			t.Fatal(err)
		}
	}
	// 用一个不存在的书库过滤，等于"只用每日推荐、不掺书库随机"。
	if err := st.SetFeaturedLibraryIDs([]int64{9999}); err != nil {
		t.Fatal(err)
	}

	s := &Server{st: st}
	got, err := s.buildFeatured(true, st.FeaturedLibraryIDs(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("期望 2 本，实际 %d", len(got))
	}
	picked := map[int64]bool{}
	ids, _ := st.DailyPickIDs()
	for _, id := range ids {
		picked[id] = true
	}
	for _, id := range bookIDs(got) {
		if !picked[id] {
			t.Fatalf("书 %d 不在每日推荐里", id)
		}
	}
}

// 只用书库：必须是配置的那个书库里的书。
func TestBuildFeaturedLibraryOnly(t *testing.T) {
	st := newTestStore(t)
	libA := seedLibrary(t, st, "A", 6, true)
	libB := seedLibrary(t, st, "B", 6, true)
	if err := st.SetDailyPicksEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := st.SetFeaturedLibraryIDs([]int64{libB.ID}); err != nil {
		t.Fatal(err)
	}

	s := &Server{st: st}
	got, err := s.buildFeatured(false, st.FeaturedLibraryIDs(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("期望 5 本，实际 %d", len(got))
	}
	for _, it := range got {
		if it["library_id"].(int64) != libB.ID {
			t.Fatalf("书 %v 来自错误的书库 %v（期望 %d）", it["id"], it["library_id"], libB.ID)
		}
	}
	_ = libA
}

// 混合模式：书单优先，书库补满；不能出现重复书。
func TestBuildFeaturedMix(t *testing.T) {
	st := newTestStore(t)
	lib := seedLibrary(t, st, "A", 12, true)
	all := allBooks(t, st)
	for _, id := range idsOf(all[:3]) {
		if err := st.AddDailyPick(id); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.SetFeaturedLibraryIDs([]int64{lib.ID}); err != nil {
		t.Fatal(err)
	}

	s := &Server{st: st}
	got, err := s.buildFeatured(true, st.FeaturedLibraryIDs(), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 8 {
		t.Fatalf("期望补满 8 本，实际 %d", len(got))
	}
	seen := map[int64]bool{}
	for _, id := range bookIDs(got) {
		if seen[id] {
			t.Fatalf("出现重复的书 %d", id)
		}
		seen[id] = true
	}
	// 书单 3 本 + 书库 5 本。
	picksInResult := 0
	pickIDs, _ := st.DailyPickIDs()
	isPick := map[int64]bool{}
	for _, id := range pickIDs {
		isPick[id] = true
	}
	for id := range seen {
		if isPick[id] {
			picksInResult++
		}
	}
	if picksInResult != 3 {
		t.Fatalf("期望 3 本来自每日推荐，实际 %d", picksInResult)
	}
}

// 书单为空时，混合模式退回纯书库，横幅不会空着。
func TestBuildFeaturedEmptyPicksFallback(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 5, true)
	if err := st.SetFeaturedLibraryIDs(nil); err != nil {
		t.Fatal(err)
	}
	s := &Server{st: st}
	got, err := s.buildFeatured(true, st.FeaturedLibraryIDs(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("期望 5 本，实际 %d", len(got))
	}
}

// 缓存：一小时内两次请求拿到同样的顺序；refresh 之后才允许变化。
func TestFeaturedRotationCache(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 10, true)

	s := &Server{st: st}
	first, err := s.buildFeatured(true, st.FeaturedLibraryIDs(), 6)
	if err != nil {
		t.Fatal(err)
	}
	ids := bookIDs(first)
	if err := st.SetFeaturedRotation(ids); err != nil {
		t.Fatal(err)
	}

	rot, fresh := st.FeaturedRotation()
	if !fresh {
		t.Fatal("刚写入的缓存应该仍然是新鲜的")
	}
	second := s.loadRotation(s.rotationSnapshot(rot, true, st.FeaturedLibraryIDs()), 6)
	if len(second) != len(ids) {
		t.Fatalf("缓存读取条数不一致：%d vs %d", len(second), len(ids))
	}
	for i := range ids {
		if second[i]["id"].(int64) != ids[i] {
			t.Fatalf("缓存顺序不一致：位置 %d", i)
		}
	}
}

// 缓存里的书被移出数据源后，不应继续出现在横幅里。
func TestRotationDropsDisallowedBooks(t *testing.T) {
	st := newTestStore(t)
	lib := seedLibrary(t, st, "A", 4, true)
	lib2 := seedLibrary(t, st, "B", 4, true)
	all := allBooks(t, st)
	var inA, inB []int64
	for _, b := range all {
		if b.LibraryID == lib.ID {
			inA = append(inA, b.ID)
		} else if b.LibraryID == lib2.ID {
			inB = append(inB, b.ID)
		}
	}

	s := &Server{st: st}
	rot := append(append([]int64{}, inA...), inB...)
	kept := s.rotationSnapshot(rot, false, []int64{lib.ID})
	if len(kept) != len(inA) {
		t.Fatalf("期望只保留 A 库的 %d 本，实际 %d", len(inA), len(kept))
	}
	for _, id := range kept {
		found := false
		for _, a := range inA {
			if a == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("书 %d 不属于 A 库", id)
		}
	}
}

// 每日推荐：重复加入幂等；软删除的书不出现在书单里；移出后消失。
func TestDailyPickCRUD(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 3, true)
	all := allBooks(t, st)

	if err := st.AddDailyPick(all[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDailyPick(all[0].ID); err != nil {
		t.Fatal(err)
	}
	picks, err := st.ListDailyPicks()
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 1 {
		t.Fatalf("重复加入应该幂等，实际 %d 条", len(picks))
	}

	if err := st.DeleteBook(all[0].ID); err != nil {
		t.Fatal(err)
	}
	picks, err = st.ListDailyPicks()
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 0 {
		t.Fatalf("软删除的书不应出现在书单里，实际 %d 条", len(picks))
	}

	if err := st.AddDailyPick(-1); err != store.ErrNotFound {
		t.Fatalf("不存在的书应该返回 ErrNotFound，实际 %v", err)
	}
}

// 每日推荐整体替换：保持顺序、忽略无效 ID。
func TestSetDailyPicks(t *testing.T) {
	st := newTestStore(t)
	seedLibrary(t, st, "A", 4, true)
	all := allBooks(t, st)

	want := []int64{all[3].ID, all[1].ID, 99999}
	if err := st.SetDailyPicks(want); err != nil {
		t.Fatal(err)
	}
	picks, err := st.ListDailyPicks()
	if err != nil {
		t.Fatal(err)
	}
	if len(picks) != 2 || picks[0].ID != all[3].ID || picks[1].ID != all[1].ID {
		t.Fatalf("书单顺序不对：%v", idsOfPickBooks(picks))
	}

	if err := st.RemoveDailyPick(all[3].ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RemoveDailyPick(all[3].ID); err != store.ErrNotFound {
		t.Fatalf("重复移出应该返回 ErrNotFound，实际 %v", err)
	}
}

func idsOfPickBooks(picks []store.DailyPick) []int64 {
	out := make([]int64, 0, len(picks))
	for _, p := range picks {
		out = append(out, p.ID)
	}
	return out
}

// 数据源设置的默认值与往返。
func TestFeaturedSourceSettings(t *testing.T) {
	st := newTestStore(t)
	if !st.GetDailyPicksEnabled() {
		t.Fatal("每日推荐默认应该开启")
	}
	if ids := st.FeaturedLibraryIDs(); len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("默认应该是全部书库 [0]，实际 %v", ids)
	}
	if err := st.SetDailyPicksEnabled(false); err != nil {
		t.Fatal(err)
	}
	if st.GetDailyPicksEnabled() {
		t.Fatal("关闭后应该返回 false")
	}
	if err := st.SetFeaturedLibraryIDs([]int64{3, 0, 3, 7}); err != nil {
		t.Fatal(err)
	}
	if ids := st.FeaturedLibraryIDs(); len(ids) != 2 || ids[0] != 3 || ids[1] != 7 {
		t.Fatalf("书库过滤应去重并去掉 0，实际 %v", ids)
	}
	if err := st.SetFeaturedLibraryIDs(nil); err != nil {
		t.Fatal(err)
	}
	if ids := st.FeaturedLibraryIDs(); len(ids) != 1 || ids[0] != 0 {
		t.Fatalf("清空后应回到全部书库，实际 %v", ids)
	}
}
