// Package library 处理书库层面的展示规则。
//
// 目前只有一件事：按目录层级把同一系列的书聚合起来（见 series.go）。
// 它不落库、不改扫描结果 —— 每次渲染前根据当前的书籍列表现算，
// 所以目录里加了书、删了书，下次刷新自动生效，不需要重新扫描。
package library

import (
	"path/filepath"
	"strings"
)

// Layout 是一次系列计算的输入。
type Layout struct {
	// Root 是书库根目录。
	Root string
	// Entries 是书库里的全部书籍（相对路径 + 书名），用来定基准深度。
	Entries []Entry
}

// Entry 描述一本书在书库里的位置。
type Entry struct {
	// Rel 是相对于书库根目录的路径。
	Rel string
	// Title 是书名，聚合后用来辨认这本书。
	Title string
}

// Depth 返回相对路径的层数：书库根目录下的文件深度为 1。
//
//	杂书.epub                 → 1
//	金庸/射雕.epub            → 2
//	科幻/三体/三体1.epub      → 3
func Depth(rel string) int {
	rel = strings.ReplaceAll(strings.TrimSpace(rel), `\`, "/")
	parts := make([]string, 0, 4)
	for _, p := range strings.Split(rel, "/") {
		if p != "" && p != "." {
			parts = append(parts, p)
		}
	}
	return len(parts)
}

// SeriesOf 判断某个相对路径是否参与系列聚合，以及它归属的系列名。
//
// 规则（与后台「是否启用系列作品聚合」开关配套）：
//   - 先看书库里最深的书有多深，记为 BASE_DEPTH；
//   - 只有深度恰好等于 BASE_DEPTH 的书参与聚合，取它的直接父文件夹名作系列名；
//   - 更浅的书一律是散书；
//   - BASE_DEPTH 为 1（书库里所有书都直接放在根目录）时，谁都不参与聚合。
//
// 不同父目录下的同名文件夹按名字字符串相同合并成一个系列。
func SeriesOf(relDir string, depth, baseDepth int) (string, bool) {
	if baseDepth <= 1 || depth <= 1 || depth != baseDepth {
		return "", false
	}
	name := filepath.Base(strings.ReplaceAll(strings.TrimSpace(relDir), `\`, "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == "/" {
		return "", false
	}
	return name, true
}

// BaseDepth 返回一组路径里的最大深度。
func BaseDepth(entries []Entry) int {
	base := 0
	for _, e := range entries {
		if d := Depth(e.Rel); d > base {
			base = d
		}
	}
	return base
}
