package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"libra/internal/auth"
	"libra/internal/store"
)

// ctxUserKey 是存入 gin.Context 的当前用户键。
const ctxUserKey = "libra.user"

// requireAuth 校验会话 Cookie。API 路径返回 401 JSON，页面路径跳转登录页。
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token, err := c.Cookie(auth.SessionCookie)
		if err == nil && token != "" {
			if user, err := s.st.SessionUser(token); err == nil {
				c.Set(ctxUserKey, user)
				c.Next()
				return
			}
		}

		if wantsJSON(c.Request.URL.Path) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"ok": false, "error": "未登录或登录已过期"})
			return
		}
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
	}
}

// user 取当前登录用户，未登录返回 nil。
func (s *Server) user(c *gin.Context) *store.User {
	if v, ok := c.Get(ctxUserKey); ok {
		if u, ok := v.(*store.User); ok {
			return u
		}
	}
	return nil
}

// RoleAdmin / RoleUser 见 store 包。
//
// 权限模型：
//   - 管理员（admin）：书库增删改、触发扫描、上传图标、浏览服务器目录、管理账号。
//   - 普通成员（user）：只读 —— 浏览封面墙、搜索、查看详情、下载原文件。
//
// adminOnly 必须挂在 requireAuth 之后：它假定 ctxUserKey 已被设置。
func (s *Server) adminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := s.user(c)
		if u == nil || !u.IsAdmin() {
			fail(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// wantsJSON 判断请求应该收到 JSON 错误而不是跳转登录页。
// 接口与静态资源（封面 / 图标）都走 JSON，页面才跳转。
func wantsJSON(p string) bool {
	return strings.HasPrefix(p, "/api/") ||
		strings.HasPrefix(p, "/covers/") ||
		strings.HasPrefix(p, "/icons/") ||
		strings.HasPrefix(p, "/download/")
}
