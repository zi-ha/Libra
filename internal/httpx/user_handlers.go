package httpx

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"libra/internal/auth"
	"libra/internal/store"
)

// 账号管理（仅管理员可用，路由已挂在 adminOnly 分组下）。
//
// 三条不可越过的保护线：
//  1. 不能删除/降级最后一个管理员 —— 否则系统会永久失去管理入口。
//  2. 不能删除自己，也不能改自己的角色 —— 避免误操作把自己锁在门外。
//  3. 重置密码会清掉该账号的全部会话，强制其重新登录。

type userCreateReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) listUsers(c *gin.Context) {
	users, err := s.st.ListUsers()
	if err != nil {
		failErr(c, http.StatusInternalServerError, "读取账号列表失败", err)
		return
	}
	me := s.user(c)

	out := make([]gin.H, 0, len(users))
	for i := range users {
		u := users[i]
		out = append(out, gin.H{
			"id":         u.ID,
			"username":   u.Username,
			"role":       u.Role,
			"is_admin":   u.IsAdmin(),
			"is_self":    me != nil && me.ID == u.ID,
			"created_at": u.CreatedAt,
		})
	}
	ok(c, gin.H{"users": out, "total": len(out)})
}

func (s *Server) createUser(c *gin.Context) {
	var req userCreateReq
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

	hash, err := auth.HashPassword(req.Username, req.Password)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.st.CreateUser(req.Username, hash, req.Role)
	if err != nil {
		fail(c, http.StatusConflict, err.Error())
		return
	}
	created(c, gin.H{"id": u.ID, "username": u.Username, "role": u.Role})
}

func (s *Server) setUserRole(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}

	if me := s.user(c); me != nil && me.ID == id {
		fail(c, http.StatusConflict, "不能修改自己的角色")
		return
	}

	target, err := s.st.GetUserByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}

	role := store.RoleUser
	if req.Role == store.RoleAdmin {
		role = store.RoleAdmin
	}

	// 把最后一个管理员降级会让系统再也无法管理。
	if target.IsAdmin() && role != store.RoleAdmin {
		n, err := s.st.CountAdmins()
		if err != nil {
			failErr(c, http.StatusInternalServerError, "读取管理员数量失败", err)
			return
		}
		if n <= 1 {
			fail(c, http.StatusConflict, "至少要保留一个管理员，无法降级")
			return
		}
	}

	if err := s.st.UpdateUserRole(id, role); err != nil {
		failErr(c, http.StatusInternalServerError, "修改角色失败", err)
		return
	}
	ok(c, gin.H{"id": id, "username": target.Username, "role": role})
}

func (s *Server) resetUserPassword(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "请求格式错误")
		return
	}

	target, err := s.st.GetUserByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}
	if err := auth.ValidatePassword(target.Username, req.Password); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(target.Username, req.Password)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpdateUserPassword(id, hash); err != nil {
		failErr(c, http.StatusInternalServerError, "重置密码失败", err)
		return
	}
	// 强制该账号下线：清掉它的全部会话（含管理员自己在别处的登录）。
	_ = s.st.DeleteUserSessions(id)

	ok(c, gin.H{"message": "密码已重置"})
}

func (s *Server) deleteUser(c *gin.Context) {
	id, okID := paramID(c, "id")
	if !okID {
		return
	}

	me := s.user(c)
	if me != nil && me.ID == id {
		fail(c, http.StatusConflict, "不能删除当前登录的账号")
		return
	}

	target, err := s.st.GetUserByID(id)
	if err != nil {
		fail(c, http.StatusNotFound, "账号不存在")
		return
	}

	if target.IsAdmin() {
		n, err := s.st.CountAdmins()
		if err != nil {
			failErr(c, http.StatusInternalServerError, "读取管理员数量失败", err)
			return
		}
		if n <= 1 {
			fail(c, http.StatusConflict, "至少要保留一个管理员，无法删除")
			return
		}
	}

	if err := s.st.DeleteUser(id); err != nil {
		failErr(c, http.StatusInternalServerError, "删除账号失败", err)
		return
	}
	ok(c, gin.H{"message": "账号已删除"})
}
