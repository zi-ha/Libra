package httpx

import (
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// thumbWidth 是封面缩略图的目标宽度。
// 首页横排 / 封面墙的卡片显示宽度通常只有 100–220px，
// 400px 足够覆盖 2x DPR，同时比动辄几 MB 的原图省很多带宽。
const thumbWidth = 400

// serveThumb 返回书籍封面缩略图。
//
// 首次访问时从原图生成并缓存到 covers/<库ID>/thumbs/，
// 之后直接读缓存。原图本身仍由 /covers/ 提供，详情页和轮播背景继续使用原图。
func (s *Server) serveThumb(c *gin.Context) {
	rel := strings.TrimPrefix(c.Param("filepath"), "/")
	if rel == "" || strings.Contains(rel, "..") {
		c.Status(http.StatusNotFound)
		return
	}

	clean := strings.TrimPrefix(path.Clean("/"+rel), "/")
	parts := strings.Split(clean, "/")
	if len(parts) != 2 {
		c.Status(http.StatusNotFound)
		return
	}
	libID, name := parts[0], parts[1]
	if _, err := strconv.ParseInt(libID, 10, 64); err != nil {
		c.Status(http.StatusNotFound)
		return
	}

	root := s.cfg.CoversDir
	src := filepath.Join(root, libID, name)
	absRoot, errRoot := filepath.Abs(root)
	absSrc, errSrc := filepath.Abs(src)
	if errRoot != nil || errSrc != nil ||
		!strings.HasPrefix(absSrc, absRoot+string(os.PathSeparator)) {
		c.Status(http.StatusNotFound)
		return
	}

	srcInfo, err := os.Stat(src)
	if err != nil || srcInfo.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	thumbDir := filepath.Join(root, libID, "thumbs")
	thumbPath := filepath.Join(thumbDir, name+".jpg")

	// 缓存命中且比原图新：直接发缓存。
	if ti, err := os.Stat(thumbPath); err == nil && !ti.IsDir() &&
		ti.Size() > 0 && !srcInfo.ModTime().After(ti.ModTime()) {
		serveThumbFile(c, thumbPath)
		return
	}

	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		// 目录不可写时退回原图，保证封面仍能显示
		serveThumbFile(c, src)
		return
	}

	f, err := os.Open(src)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	img, _, decodeErr := image.Decode(f)
	_ = f.Close()
	if decodeErr != nil {
		// 不支持的格式（例如某些特殊编码）直接回退原图
		serveThumbFile(c, src)
		return
	}

	bounds := img.Bounds()
	if bounds.Dx() <= thumbWidth {
		// 原图已经比目标尺寸小，没必要重新编码
		serveThumbFile(c, src)
		return
	}

	dstW := thumbWidth
	dstH := int(float64(bounds.Dy())*float64(dstW)/float64(bounds.Dx()) + 0.5)
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)

	tmp, err := os.CreateTemp(thumbDir, ".thumb-*.jpg")
	if err != nil {
		serveThumbFile(c, src)
		return
	}
	tmpName := tmp.Name()
	if err := jpeg.Encode(tmp, dst, &jpeg.Options{Quality: 84}); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		serveThumbFile(c, src)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		serveThumbFile(c, src)
		return
	}
	if err := os.Rename(tmpName, thumbPath); err != nil {
		_ = os.Remove(tmpName)
		serveThumbFile(c, src)
		return
	}

	serveThumbFile(c, thumbPath)
}

func serveThumbFile(c *gin.Context, p string) {
	c.Header("Cache-Control", "private, max-age=86400")
	http.ServeFile(c.Writer, c.Request, p)
}
