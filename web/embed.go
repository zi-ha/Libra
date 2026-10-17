// Package web 内嵌前端模板与静态资源，保证单二进制部署。
package web

import "embed"

// FS 包含 templates/ 与 static/ 两个目录。
//
//go:embed all:templates all:static
var FS embed.FS
