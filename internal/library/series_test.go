package library

import (
	"path/filepath"
	"testing"
)

func relFor(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// 文档里的完整示例：只有最深的一层参与聚合。
func TestSeriesOfSpecExample(t *testing.T) {
	root := "/booklib"
	files := []string{
		"/booklib/心理学入门.epub",
		"/booklib/金庸/射雕英雄传.epub",
		"/booklib/科幻/三体系列/三体1 地球往事.epub",
		"/booklib/科幻/三体系列/三体2 黑暗森林.epub",
		"/booklib/科幻/三体系列/三体3 死神永生.epub",
	}
	entries := make([]Entry, 0, len(files))
	for _, f := range files {
		entries = append(entries, Entry{Rel: relFor(root, f)})
	}

	base := BaseDepth(entries)
	if base != 3 {
		t.Fatalf("BASE_DEPTH 期望 3，实际 %d", base)
	}

	want := map[string]string{
		"心理学入门.epub":            "",
		"金庸/射雕英雄传.epub":         "",
		"科幻/三体系列/三体1 地球往事.epub": "三体系列",
		"科幻/三体系列/三体2 黑暗森林.epub": "三体系列",
		"科幻/三体系列/三体3 死神永生.epub": "三体系列",
	}
	for _, e := range entries {
		dir := filepath.Dir(e.Rel)
		if dir == "." {
			dir = ""
		}
		got, ok := SeriesOf(dir, Depth(e.Rel), base)
		if want[e.Rel] == "" {
			if ok {
				t.Fatalf("%s 不应参与聚合，却被归为 %q", e.Rel, got)
			}
			continue
		}
		if !ok || got != want[e.Rel] {
			t.Fatalf("%s 期望系列 %q，实际 %q（参与=%v）", e.Rel, want[e.Rel], got, ok)
		}
	}
}

// 所有书都在根目录时，谁都不参与聚合。
func TestSeriesOfAllAtRoot(t *testing.T) {
	entries := []Entry{{Rel: "a.epub"}, {Rel: "b.pdf"}}
	base := BaseDepth(entries)
	if base != 1 {
		t.Fatalf("BASE_DEPTH 期望 1，实际 %d", base)
	}
	for _, e := range entries {
		if s, ok := SeriesOf("", Depth(e.Rel), base); ok {
			t.Fatalf("%s 不应聚合，得到 %q", e.Rel, s)
		}
	}
}

// 所有书都在同一层：按父文件夹名聚合，同名文件夹合并。
func TestSeriesOfSameDepthMergesSameName(t *testing.T) {
	entries := []Entry{
		{Rel: "科幻/三体/1.epub"},
		{Rel: "文学/三体/2.epub"},
		{Rel: "科幻/三体/3.epub"},
		{Rel: "其他/孤本.epub"},
	}
	base := BaseDepth(entries)
	if base != 3 {
		t.Fatalf("BASE_DEPTH 期望 3，实际 %d", base)
	}
	count := 0
	for _, e := range entries {
		if s, ok := SeriesOf(filepath.Dir(e.Rel), Depth(e.Rel), base); ok && s == "三体" {
			count++
		}
	}
	if count != 3 {
		t.Fatalf("不同父目录下的同名文件夹应合并为一个系列，实际命中 %d 本", count)
	}
}

// 新增一本更深层的书之后，原来参与聚合的浅层书自动变成散书。
func TestSeriesOfDeeperBookIntroduced(t *testing.T) {
	before := []Entry{{Rel: "金庸/射雕.epub"}, {Rel: "金庸/神雕.epub"}}
	base := BaseDepth(before)
	if s, ok := SeriesOf("金庸", 2, base); !ok || s != "金庸" {
		t.Fatalf("深度 2 且 BASE_DEPTH=2 时应聚合为 金庸，实际 %q/%v", s, ok)
	}

	// 新来一本 depth=3 的书：BASE_DEPTH 变成 3，原来的深度 2 变成散书。
	after := append(before, Entry{Rel: "科幻/三体/1.epub"})
	base = BaseDepth(after)
	if base != 3 {
		t.Fatalf("BASE_DEPTH 期望 3，实际 %d", base)
	}
	if s, ok := SeriesOf("金庸", 2, base); ok {
		t.Fatalf("深层书出现后，深度 2 的书应该变成散书，却得到 %q", s)
	}
	if s, ok := SeriesOf("科幻/三体", 3, base); !ok || s != "三体" {
		t.Fatalf("深度 3 的书应聚合为 三体，实际 %q/%v", s, ok)
	}
}

func TestDepth(t *testing.T) {
	cases := map[string]int{
		"a.epub":       1,
		"金庸/射雕.epub":   2,
		"科幻/三体/1.epub": 3,
		"a\\b\\c.epub": 3,
		"./a/b.epub":   2,
		"":             0,
	}
	for in, want := range cases {
		if got := Depth(in); got != want {
			t.Fatalf("Depth(%q) 期望 %d，实际 %d", in, want, got)
		}
	}
}
