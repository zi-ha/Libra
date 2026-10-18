package store

import (
	"path/filepath"
	"testing"
)

// 字符串排序会把 10 排在 1 前面；自然排序必须是 1 < 2 < 10。
func TestNaturalSortTenthAfterSecond(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	lib, err := st.CreateLibrary("L", t.TempDir(), "builtin:book")
	if err != nil {
		t.Fatal(err)
	}

	titles := []string{"卷10", "卷2", "卷1", "三体 10", "三体 2", "三体 1", "Vol.3", "Vol.12"}
	for _, title := range titles {
		if _, err := st.UpsertBook(&Book{
			LibraryID: lib.ID,
			Path:      filepath.Join("/tmp", lib.Path, title+".epub"),
			Filename:  title + ".epub",
			Title:     title,
			Format:    "epub",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// 先确认默认的「书名 A→Z」（title）也已经是自然序：10 不会再排在 1、2 前面
	plain, _, err := st.ListBooks(BookQuery{Sort: "title", PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	var plainOrder []string
	for _, b := range plain {
		plainOrder = append(plainOrder, b.Title)
	}
	if plainOrder[0] != "Vol.3" {
		t.Fatalf("默认书名排序不应把 Vol.12 排到最前：%v", plainOrder)
	}

	got, _, err := st.ListBooks(BookQuery{Sort: "natural", PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, b := range got {
		order = append(order, b.Title)
	}

	// 拉丁字母按码位排在汉字前面（Vol. < 三体 < 卷），组内数字按大小。
	want := []string{"Vol.3", "Vol.12", "三体 1", "三体 2", "三体 10", "卷1", "卷2", "卷10"}
	if len(order) != len(want) {
		t.Fatalf("条数不对：%v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("自然排序顺序不对：\n got %v\nwant %v", order, want)
		}
	}

	// 倒序也应该是自然的倒序
	desc, _, err := st.ListBooks(BookQuery{Sort: "natural", Desc: true, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	// 倒序是正序的镜像：卷10 在最前，Vol.3 在最后
	if desc[0].Title != "卷10" || desc[len(desc)-1].Title != "Vol.3" {
		t.Fatalf("倒序不对：首项 %q，末项 %q", desc[0].Title, desc[len(desc)-1].Title)
	}
}

// 数字优先于原书名的字典序兜底：没有数字的书名之间仍然稳定。
func TestNaturalSplit(t *testing.T) {
	cases := []struct {
		in   string
		text string
		num  bool
		val  int64
	}{
		{"三体 10", "三体", true, 10},
		{"三体1", "三体", true, 1},
		{"Vol.12", "Vol", true, 12},
		{"没有数字", "没有数字", false, 0},
		{"a1b2", "ab", true, 1},
	}
	for _, c := range cases {
		text, num := naturalSplit(c.in)
		if text != c.text || num.Valid != c.num || (c.num && num.Int64 != c.val) {
			t.Fatalf("naturalSplit(%q) = (%q, %v) 期望 (%q, %v)", c.in, text, num, c.text, c.val)
		}
	}
}
