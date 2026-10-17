package httpx

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"libra/internal/auth"
	"libra/internal/store"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// pageLogin 渲染登录/注册页。
func (s *Server) pageLogin(c *gin.Context) {
	// 已登录用户直接进主界面。
	if token, err := c.Cookie(auth.SessionCookie); err == nil && token != "" {
		if _, err := s.st.SessionUser(token); err == nil {
			c.Redirect(http.StatusFound, "/")
			return
		}
	}
	c.HTML(http.StatusOK, "login.html", gin.H{
		"AllowRegister": s.allowRegister(),
	})
}

// pageApp 渲染主应用页面。
func (s *Server) pageApp(c *gin.Context) {
	u := s.user(c)
	c.HTML(http.StatusOK, "app.html", gin.H{
		"Username": u.Username,
		"Role":     u.Role,
		"IsAdmin":  u.IsAdmin(),
	})
}

// me 返回当前登录状态，供前端在页面加载时判断。
// 该接口不需要登录，因为它同时承担"系统是否已初始化"的探测职责。
func (s *Server) me(c *gin.Context) {
	count, err := s.st.CountUsers()
	if err != nil {
		failErr(c, http.StatusInternalServerError, "读取用户失败", err)
		return
	}

	resp := gin.H{
		"authenticated":  false,
		"needs_setup":    count == 0,
		"allow_register": s.allowRegister() || count == 0,
		"user":           nil,
	}

	if token, err := c.Cookie(auth.SessionCookie); err == nil && token != "" {
		if u, err := s.st.SessionUser(token); err == nil {
			resp["authenticated"] = true
			resp["user"] = gin.H{"id": u.ID, "username": u.Username, "role": u.Role}
		}
	}
	ok(c, resp)
}

// register 注册账号。系统中的第一个用户自动成为管理员。
func (s *Server) register(c *gin.Context) {
	var req credentials
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)

	if err := auth.ValidateUsername(req.Username); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := auth.ValidatePassword(req.Username, req.Password); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	count, err := s.st.CountUsers()
	if err != nil {
		failErr(c, http.StatusInternalServerError, "读取用户失败", err)
		return
	}
	if count > 0 && !s.allowRegister() {
		fail(c, http.StatusForbidden, "系统已关闭自助注册，请联系管理员开通账号")
		return
	}

	role := store.RoleUser
	if count == 0 {
		role = store.RoleAdmin
	}

	hash, err := auth.HashPassword(req.Username, req.Password)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.st.CreateUser(req.Username, hash, role)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}

	if err := s.issueSession(c, u); err != nil {
		failErr(c, http.StatusInternalServerError, "创建会话失败", err)
		return
	}
	created(c, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
}

// login 校验账号密码并下发会话 Cookie。
func (s *Server) login(c *gin.Context) {
	var req credentials
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))

	key := c.ClientIP() + "|" + req.Username
	if !s.loginGuard.allow(key) {
		fail(c, http.StatusTooManyRequests, "尝试次数过多，请稍后再试")
		return
	}

	u, err := s.st.GetUserByUsername(req.Username)
	if err != nil || !auth.VerifyPassword(u.PasswordHash, req.Password) {
		s.loginGuard.fail(key)
		fail(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}
	s.loginGuard.reset(key)

	if err := s.issueSession(c, u); err != nil {
		failErr(c, http.StatusInternalServerError, "创建会话失败", err)
		return
	}
	ok(c, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
}

// logout 注销当前会话。
func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(auth.SessionCookie); err == nil && token != "" {
		_ = s.st.DeleteSession(token)
	}
	s.clearSessionCookie(c)
	ok(c, gin.H{"message": "已退出登录"})
}

// changePassword 修改当前用户密码，并使其它会话失效。
func (s *Server) changePassword(c *gin.Context) {
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}
	u := s.user(c)
	if !auth.VerifyPassword(u.PasswordHash, req.OldPassword) {
		fail(c, http.StatusBadRequest, "原密码不正确")
		return
	}
	hash, err := auth.HashPassword(u.Username, req.NewPassword)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateUserPassword(u.ID, hash); err != nil {
		failErr(c, http.StatusInternalServerError, "修改密码失败", err)
		return
	}
	// 改密后旧的会话全部作废，再给当前浏览器发一个新会话。
	_ = s.st.DeleteUserSessions(u.ID)
	u.PasswordHash = hash
	if err := s.issueSession(c, u); err != nil {
		failErr(c, http.StatusInternalServerError, "重建会话失败", err)
		return
	}
	ok(c, gin.H{"message": "密码已更新"})
}

func (s *Server) issueSession(c *gin.Context, u *store.User) error {
	token, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := s.st.CreateSession(token, u.ID, s.cfg.SessionTTL); err != nil {
		return err
	}
	// 清理过期会话，避免 sessions 表无限增长。
	_ = s.st.PurgeExpiredSessions()

	maxAge := int(s.cfg.SessionTTL.Seconds())
	if maxAge <= 0 {
		maxAge = 30 * 24 * 3600
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookie, token, maxAge, "/", "", isHTTPS(c), true)
	return nil
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookie, "", -1, "/", "", isHTTPS(c), true)
}

func isHTTPS(c *gin.Context) bool {
	return c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

// ---------- 登录失败限流（内存态，够用且零依赖）----------

const (
	loginMaxFails   = 8
	loginFailWindow = 5 * time.Minute
)

type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginEntry
}

type loginEntry struct {
	fails    int
	firstAt  time.Time
	lockedAt time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*loginEntry)}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok {
		return true
	}
	if time.Since(e.firstAt) > loginFailWindow {
		delete(l.entries, key)
		return true
	}
	return e.fails < loginMaxFails
}

func (l *loginLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[key]
	if !ok || time.Since(e.firstAt) > loginFailWindow {
		l.entries[key] = &loginEntry{fails: 1, firstAt: time.Now()}
		return
	}
	e.fails++
	e.lockedAt = time.Now()
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
