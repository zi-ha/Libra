package scanner

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// maxOpfBytes 限制 OPF/container.xml 的读取上限，防御畸形 EPUB。
const (
	maxOPFBytes   = 8 << 20
	maxCoverBytes = 24 << 20
)

// epubInfo 是解析出的 EPUB 元数据。
type epubInfo struct {
	Title    string
	Author   string
	Cover    []byte
	CoverExt string // 带点，例如 .jpg
}

// containerDoc 对应 META-INF/container.xml。
type containerDoc struct {
	Rootfiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

// opfDoc 只声明我们关心的 OPF 节点。
type opfDoc struct {
	Metadata struct {
		Titles   []string `xml:"title"`
		Creators []string `xml:"creator"`
		Metas    []struct {
			Name     string `xml:"name,attr"`
			Content  string `xml:"content,attr"`
			Property string `xml:"property,attr"`
		} `xml:"meta"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
}

// parseEPUB 直接读取 EPUB 的 zip 结构，提取书名、作者与内嵌封面。
//
// 只依赖标准库（archive/zip + encoding/xml），不需要 CGO，也不引入额外依赖。
func parseEPUB(filePath string) (*epubInfo, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("打开 EPUB 失败: %w", err)
	}
	defer zr.Close()

	// 建立 zip 索引，同时保留一个小写索引用于大小写不一致的兜底查找。
	index := make(map[string]*zip.File, len(zr.File))
	lower := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		key := normalizeZipPath(f.Name)
		index[key] = f
		lower[strings.ToLower(key)] = f
	}

	lookup := func(name string) *zip.File {
		if f, ok := index[name]; ok {
			return f
		}
		return lower[strings.ToLower(name)]
	}

	opfPath, err := findOPFPath(index, lookup)
	if err != nil {
		return nil, err
	}
	opfFile := lookup(opfPath)
	if opfFile == nil {
		return nil, fmt.Errorf("OPF 文件不存在: %s", opfPath)
	}
	raw, err := readZipEntry(opfFile, maxOPFBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 OPF 失败: %w", err)
	}

	var opf opfDoc
	if err := xml.Unmarshal(raw, &opf); err != nil {
		return nil, fmt.Errorf("解析 OPF 失败: %w", err)
	}

	info := &epubInfo{
		Title:  cleanText(firstNonEmpty(opf.Metadata.Titles)),
		Author: cleanText(firstNonEmpty(opf.Metadata.Creators)),
	}

	baseDir := path.Dir(opfPath)
	if baseDir == "." || baseDir == "/" {
		baseDir = ""
	}

	if href := pickCoverHref(&opf); href != "" {
		target := resolveZipPath(baseDir, href)
		if f := lookup(target); f != nil {
			if data, err := readZipEntry(f, maxCoverBytes); err == nil && len(data) > 0 {
				info.Cover = data
				info.CoverExt = imageExt(f.Name, "")
			}
		}
	}
	return info, nil
}

// findOPFPath 优先读 container.xml，失败时扫描目录中最浅的 .opf 文件。
func findOPFPath(index map[string]*zip.File, lookup func(string) *zip.File) (string, error) {
	if f := lookup("META-INF/container.xml"); f != nil {
		if raw, err := readZipEntry(f, 1<<20); err == nil {
			var c containerDoc
			if err := xml.Unmarshal(raw, &c); err == nil {
				for _, rf := range c.Rootfiles {
					p := normalizeZipPath(rf.FullPath)
					if p != "" && lookup(p) != nil {
						return p, nil
					}
				}
			}
		}
	}

	best := ""
	bestDepth := 1 << 30
	for name := range index {
		if !strings.EqualFold(path.Ext(name), ".opf") {
			continue
		}
		depth := strings.Count(name, "/")
		if depth < bestDepth {
			best, bestDepth = name, depth
		}
	}
	if best == "" {
		return "", errors.New("EPUB 中未找到 OPF 文件")
	}
	return best, nil
}

// pickCoverHref 按优先级挑选封面资源：EPUB2 的 meta[name=cover] →
// EPUB3 的 properties="cover-image" → 常见命名 → 第一张图片。
func pickCoverHref(opf *opfDoc) string {
	byID := make(map[string]string, len(opf.Manifest.Items)) // id -> href
	for _, it := range opf.Manifest.Items {
		byID[strings.TrimSpace(it.ID)] = it.Href
	}

	// 1) <meta name="cover" content="cover-id"/>
	for _, m := range opf.Metadata.Metas {
		if strings.EqualFold(strings.TrimSpace(m.Name), "cover") {
			if href, ok := byID[strings.TrimSpace(m.Content)]; ok {
				return href
			}
			// 少数 EPUB 直接写文件名
			if m.Content != "" && isImageHref(m.Content) {
				return m.Content
			}
		}
	}

	// 2) EPUB3: properties 包含 cover-image
	for _, it := range opf.Manifest.Items {
		if strings.Contains(it.Properties, "cover-image") && isImageHref(it.Href) {
			return it.Href
		}
	}

	// 3) id / href 里带 cover 字样的图片
	for _, it := range opf.Manifest.Items {
		if !isImageHref(it.Href) {
			continue
		}
		id := strings.ToLower(it.ID)
		href := strings.ToLower(it.Href)
		if strings.Contains(id, "cover") || strings.Contains(href, "cover") {
			return it.Href
		}
	}

	// 4) 退化为 manifest 中的第一张图片
	for _, it := range opf.Manifest.Items {
		if isImageHref(it.Href) {
			return it.Href
		}
	}
	return ""
}

func isImageHref(href string) bool {
	return imageExt(href, "") != ""
}

// imageExt 根据文件名/媒体类型推断图片扩展名，非图片返回空串。
func imageExt(name, mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tif"
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return ext
	}
	return ""
}

// resolveZipPath 把 OPF 中的相对路径解析成 zip 内的绝对路径。
func resolveZipPath(baseDir, href string) string {
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		href = href[:i]
	}
	if decoded, err := url.PathUnescape(href); err == nil {
		href = decoded
	}
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	if strings.HasPrefix(href, "/") {
		return normalizeZipPath(strings.TrimPrefix(href, "/"))
	}
	if baseDir == "" {
		return normalizeZipPath(href)
	}
	return normalizeZipPath(path.Join(baseDir, href))
}

// normalizeZipPath 统一分隔符并清理 ./ ../。
func normalizeZipPath(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" {
		return ""
	}
	return path.Clean(name)
}

func readZipEntry(f *zip.File, limit int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

func firstNonEmpty(list []string) string {
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// cleanText 去掉首尾空白、折叠内部连续空白、剔除控制字符。
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > 512 {
		s = s[:512]
	}
	return s
}
