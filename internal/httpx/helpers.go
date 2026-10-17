package httpx

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ok 统一的成功响应。
func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"ok": true, "data": data})
}

// created 创建成功的响应。
func created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, gin.H{"ok": true, "data": data})
}

// fail 统一的失败响应。
func fail(c *gin.Context, code int, msg string) {
	c.JSON(code, gin.H{"ok": false, "error": msg})
}

// failErr 把内部错误转成 500（不泄露细节，细节记到日志以外由调用方决定）。
func failErr(c *gin.Context, code int, msg string, err error) {
	if err != nil {
		msg = msg + ": " + err.Error()
	}
	fail(c, code, msg)
}

// paramID 解析路径中的整型 ID。
func paramID(c *gin.Context, name string) (int64, bool) {
	v, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || v <= 0 {
		fail(c, http.StatusBadRequest, "非法的 ID")
		return 0, false
	}
	return v, true
}

// queryInt 读取整型查询参数。
func queryInt(c *gin.Context, name string, def int) int {
	v := strings.TrimSpace(c.Query(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// contentDisposition 生成同时兼容新旧浏览器（含中文文件名）的响应头。
func contentDisposition(filename string) string {
	ascii := make([]rune, 0, len(filename))
	for _, r := range filename {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			ascii = append(ascii, '_')
			continue
		}
		ascii = append(ascii, r)
	}
	fallback := strings.TrimSpace(string(ascii))
	if fallback == "" || fallback == "_" {
		fallback = "download"
	}
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", fallback, url.PathEscape(filename))
}

// humanSize 供模板/接口展示用的可读体积。
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.2f %s", value, units[i])
}
