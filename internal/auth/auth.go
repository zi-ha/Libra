// Package auth 提供密码哈希、密码策略与会话令牌的生成/校验。
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// SessionCookie 是登录态 Cookie 名。
const SessionCookie = "libra_session"

// bcryptCost 是 bcrypt 的计算强度。
// 12 在现代 x86 上约 200ms，在 ARM NAS 上约 0.5–1s —— 登录是低频操作，
// 用这点延迟换来的离线爆破成本提升是划算的。
// 已存在的旧哈希（cost 10）仍能正常校验，bcrypt 把 cost 存在哈希串里。
const bcryptCost = 12

// minPasswordLen 是密码最小长度。
const minPasswordLen = 8

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{2,32}$`)

// weakPasswords 是一小撮最常见的弱口令，直接拒绝。
var weakPasswords = map[string]struct{}{
	"password": {}, "password1": {}, "password12": {}, "passw0rd": {}, "p@ssw0rd": {},
	"12345678": {}, "123456789": {}, "1234567890": {}, "87654321": {}, "123456abc": {},
	"qwertyui": {}, "qwerty123": {}, "abc12345": {}, "a1234567": {},
	"11111111": {}, "00000000": {}, "88888888": {}, "iloveyou": {},
	"admin123": {}, "admin1234": {}, "administrator": {}, "letmein1": {}, "welcome1": {},
}

// HashPassword 使用 bcrypt 生成密码哈希，同时执行强度校验。
func HashPassword(username, plain string) (string, error) {
	if err := ValidatePassword(username, plain); err != nil {
		return "", err
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// VerifyPassword 校验明文密码是否匹配哈希。
func VerifyPassword(hash, plain string) bool {
	if hash == "" || plain == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// NewToken 生成一个 32 字节（256 位熵）的随机会话令牌。
func NewToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// TokenDigest 计算会话令牌的存储摘要。
// 数据库里只保存摘要而非令牌原文 —— 即使 libra.db 文件被拿走，
// 也无法直接把里面的值当成 Cookie 拿去冒充登录。
func TokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ValidateUsername 校验用户名规则。
func ValidateUsername(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("用户名不能为空")
	}
	if !usernameRe.MatchString(name) {
		return errors.New("用户名只能包含字母、数字、下划线、点和短横线，长度 2-32")
	}
	return nil
}

// ValidatePassword 校验密码强度。
// username 用于拒绝"密码与用户名相同"这种最常见的情况，可传空串跳过该校验。
func ValidatePassword(username, p string) error {
	if utf8.RuneCountInString(p) < minPasswordLen {
		return errors.New("密码长度至少 8 位")
	}
	if len(p) > 200 {
		return errors.New("密码过长")
	}

	lower := strings.ToLower(p)
	if u := strings.ToLower(strings.TrimSpace(username)); u != "" && lower == u {
		return errors.New("密码不能与用户名相同")
	}
	if _, bad := weakPasswords[lower]; bad {
		return errors.New("这个密码过于常见，请换一个")
	}

	// 至少包含「字母 / 数字 / 其它符号」中的两类，挡住纯数字和纯字母。
	var hasLetter, hasDigit, hasOther bool
	for _, r := range p {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasOther = true
		}
	}
	kinds := 0
	for _, ok := range []bool{hasLetter, hasDigit, hasOther} {
		if ok {
			kinds++
		}
	}
	if kinds < 2 {
		return errors.New("密码需包含字母、数字、符号中的至少两类")
	}
	return nil
}

// ConstantTimeEqual 做常数时间的字符串比较，避免时序侧信道。
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
