package httpx

import "github.com/gin-gonic/gin"

const settingAllowRegister = "allow_register"

// allowRegister 返回运行时的注册开关。
// 数据库里有值就以数据库为准；没有则回退到环境变量 LIBRA_ALLOW_REGISTER。
func (s *Server) allowRegister() bool {
	v, err := s.st.GetSetting(settingAllowRegister)
	if err == nil {
		return v == "1" || v == "true"
	}
	return s.cfg.AllowRegister
}

// getAdminSettings 返回管理后台可见的系统设置。
func (s *Server) getAdminSettings(c *gin.Context) {
	ok(c, gin.H{
		"allow_register": s.allowRegister(),
		"version":        Version,
	})
}

// updateAdminSettings 更新运行时设置。
func (s *Server) updateAdminSettings(c *gin.Context) {
	var req struct {
		AllowRegister *bool `json:"allow_register"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, 400, "请求格式错误")
		return
	}
	if req.AllowRegister != nil {
		v := "0"
		if *req.AllowRegister {
			v = "1"
		}
		if err := s.st.SetSetting(settingAllowRegister, v); err != nil {
			failErr(c, 500, "保存注册设置失败", err)
			return
		}
	}
	ok(c, gin.H{"allow_register": s.allowRegister()})
}
