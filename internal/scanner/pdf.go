package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 外部命令超时。pdftoppm 渲染超大页面时可能较慢，给足余量。
const (
	pdfRenderTimeout = 90 * time.Second
	pdfProbeTimeout  = 20 * time.Second
	// 内嵌图片被采纳为封面所需的最小尺寸与最大体积。
	minEmbeddedCoverSide = 300
	maxEmbeddedCoverSize = 5 << 20
)

// errPopplerMissing 表示容器内没有安装 poppler-utils。
var errPopplerMissing = errors.New("未找到 poppler-utils（pdftoppm），无法为 PDF 生成封面")

// toolAvailable 检查外部命令是否存在，结果缓存到 map 里避免重复 LookPath。
func (s *Scanner) toolAvailable(name string) bool {
	s.toolsMu.Lock()
	defer s.toolsMu.Unlock()
	if v, ok := s.tools[name]; ok {
		return v
	}
	_, err := exec.LookPath(name)
	s.tools[name] = err == nil
	return err == nil
}

// PDFSupport 报告容器内是否具备 PDF 封面渲染能力（poppler-utils 是否就绪）。
func (s *Scanner) PDFSupport() bool { return s.toolAvailable("pdftoppm") }

// pdfTitle 通过 pdfinfo 读取 PDF 元数据标题；失败或为空时返回空串。
func (s *Scanner) pdfTitle(ctx context.Context, src string) string {
	if !s.toolAvailable("pdfinfo") {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, pdfProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "pdfinfo", src).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(strings.ToLower(line), "title:") {
			continue
		}
		title := cleanText(strings.TrimSpace(line[len("title:"):]))
		// 很多 PDF 的默认标题是文件名或 Untitled，此时宁可用文件名。
		if title == "" || strings.EqualFold(title, "untitled") || strings.EqualFold(title, "无标题") {
			return ""
		}
		return title
	}
	return ""
}

// renderPDFCover 调用 pdftoppm 把第 1 页渲染成 JPEG 封面。
// dstPrefix 不带扩展名；成功时返回实际写出的文件路径。
func (s *Scanner) renderPDFCover(ctx context.Context, src, dstPrefix string, width int) (string, error) {
	if !s.toolAvailable("pdftoppm") {
		return "", errPopplerMissing
	}
	ctx, cancel := context.WithTimeout(ctx, pdfRenderTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "pdftoppm",
		"-jpeg",
		"-singlefile",
		"-f", "1",
		"-l", "1",
		"-scale-to", strconv.Itoa(width),
		src,
		dstPrefix,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("pdftoppm 渲染失败: %s", truncate(msg, 300))
	}

	out := dstPrefix + ".jpg"
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("pdftoppm 未产出封面文件: %w", err)
	}
	return out, nil
}

// extractEmbeddedPDFCover 尝试抽出第 1 页内嵌的位图。
// 若该位图足够大，说明 PDF 自带封面图，直接采用比渲染更清晰。
// 返回字节内容与扩展名；不满足条件时返回空。
func (s *Scanner) extractEmbeddedPDFCover(ctx context.Context, src, tmpDir string) ([]byte, string) {
	if !s.toolAvailable("pdfimages") {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(ctx, pdfProbeTimeout)
	defer cancel()

	prefix := filepath.Join(tmpDir, "embedded")
	cmd := exec.CommandContext(ctx, "pdfimages", "-f", "1", "-l", "1", "-j", src, prefix)
	if err := cmd.Run(); err != nil {
		return nil, ""
	}

	matches, _ := filepath.Glob(prefix + "-*")
	best := ""
	var bestSize int64
	for _, m := range matches {
		ext := strings.ToLower(filepath.Ext(m))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			continue // pdfimages 对非 DCT 图会输出 .ppm，标准库无法解析
		}
		fi, err := os.Stat(m)
		if err != nil || fi.Size() == 0 || fi.Size() > maxEmbeddedCoverSize {
			continue
		}
		if fi.Size() > bestSize {
			best, bestSize = m, fi.Size()
		}
	}
	defer func() {
		if matches != nil {
			for _, m := range matches {
				_ = os.Remove(m)
			}
		}
	}()
	if best == "" {
		return nil, ""
	}

	data, err := os.ReadFile(best)
	if err != nil || len(data) == 0 {
		return nil, ""
	}
	w, h, err := imageSize(data)
	if err != nil {
		return nil, ""
	}
	// 只有"像一页封面"的图才采纳：宽高都不小于阈值，且接近竖向版面。
	if w < minEmbeddedCoverSide || h < minEmbeddedCoverSide {
		return nil, ""
	}
	if h < w {
		return nil, "" // 横向长条图通常是插图，不是封面
	}
	return data, imageExt(best, "")
}

// imageSize 解析图片宽高，只支持标准库内置的 jpeg/png/gif。
func imageSize(data []byte) (int, int, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
