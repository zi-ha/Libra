// Package bookmeta 把管理员修改的书名 / 作者写回 EPUB / PDF 源文件。
//
// 写入采用"临时文件 + rename"的原子替换方式；文件所在目录必须可写。
package bookmeta

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func init() {
	// 容器 / NAS 上不一定有可写的用户配置目录；禁用 pdfcpu 的 config dir。
	api.DisableConfigDir()
}

// Write 把标题 / 作者写回源文件。
func Write(path, format, title, author string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "epub":
		return writeEPUB(path, title, author)
	case "pdf":
		return writePDF(path, title, author)
	default:
		return fmt.Errorf("不支持写入 %s 格式的元数据", format)
	}
}

// ---------------------------------------------------------------- EPUB

type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

func writeEPUB(path, title, author string) error {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("打开 EPUB 失败: %w", err)
	}
	defer zr.Close()

	opfName, err := epubOPFPath(zr)
	if err != nil {
		return err
	}
	opf, err := readZipFile(zr, opfName)
	if err != nil {
		return fmt.Errorf("读取 EPUB OPF 失败: %w", err)
	}
	opf, err = setEPUBMetadata(opf, title, author)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".libra-meta-*.epub")
	if err != nil {
		return fmt.Errorf("创建临时文件失败（书库目录是否可写？）: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	zw := zip.NewWriter(tmp)
	if zr.Comment != "" {
		_ = zw.SetComment(zr.Comment)
	}
	for _, f := range zr.File {
		hdr := f.FileHeader
		w, err := zw.CreateHeader(&hdr)
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("写入 EPUB 条目失败: %w", err)
		}
		if f.Name == opfName {
			if _, err := w.Write(opf); err != nil {
				_ = zw.Close()
				_ = tmp.Close()
				return fmt.Errorf("写入 OPF 失败: %w", err)
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("读取 EPUB 条目 %s 失败: %w", f.Name, err)
		}
		_, copyErr := io.Copy(w, rc)
		_ = rc.Close()
		if copyErr != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return fmt.Errorf("复制 EPUB 条目 %s 失败: %w", f.Name, copyErr)
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("关闭 EPUB 写入器失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("写入 EPUB 失败: %w", err)
	}
	// Windows 上 zip.OpenReader 会持有原文件的句柄，必须先关闭再 rename。
	_ = zr.Close()
	return installFile(tmpName, path)
}

func epubOPFPath(zr *zip.ReadCloser) (string, error) {
	container, err := readZipFile(zr, "META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("读取 EPUB container.xml 失败: %w", err)
	}
	var c containerXML
	if err := xml.Unmarshal(container, &c); err != nil {
		return "", fmt.Errorf("解析 EPUB container.xml 失败: %w", err)
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return "", fmt.Errorf("EPUB 中没有找到 OPF 文件")
	}
	return strings.TrimPrefix(c.Rootfiles[0].FullPath, "/"), nil
}

func readZipFile(zr *zip.ReadCloser, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("找不到 %s", name)
}

func setEPUBMetadata(opf []byte, title, author string) ([]byte, error) {
	out, err := setDCElement(opf, "title", title)
	if err != nil {
		return nil, err
	}
	out, err = setDCElement(out, "creator", author)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func setDCElement(opf []byte, name, value string) ([]byte, error) {
	re := regexp.MustCompile(`(?is)<dc:` + name + `(\s[^>]*?)?(?:/>|>.*?</dc:` + name + `>)`)
	loc := re.FindSubmatchIndex(opf)
	escaped := escapeXML(value)
	if loc != nil {
		attrs := ""
		if loc[2] >= 0 && loc[3] >= 0 {
			attrs = string(opf[loc[2]:loc[3]])
		}
		replacement := []byte("<dc:" + name + attrs + ">" + escaped + "</dc:" + name + ">")
		result := make([]byte, 0, len(opf)+len(replacement))
		result = append(result, opf[:loc[0]]...)
		result = append(result, replacement...)
		result = append(result, opf[loc[1]:]...)
		return result, nil
	}

	// 找不到元素时，插到 </metadata> 前面。
	end := bytes.LastIndex(opf, []byte("</metadata>"))
	if end < 0 {
		return nil, fmt.Errorf("EPUB OPF 中找不到 metadata 节点")
	}
	insert := []byte("<dc:" + name + ">" + escaped + "</dc:" + name + ">\n")
	result := make([]byte, 0, len(opf)+len(insert))
	result = append(result, opf[:end]...)
	result = append(result, insert...)
	result = append(result, opf[end:]...)
	return result, nil
}

func escapeXML(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// ---------------------------------------------------------------- PDF

func writePDF(path, title, author string) error {
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		return fmt.Errorf("读取 PDF 失败: %w", err)
	}

	var info types.Dict
	if ctx.Info == nil {
		info = types.NewDict()
		ir, err := ctx.IndRefForNewObject(info)
		if err != nil {
			return fmt.Errorf("创建 PDF 信息字典失败: %w", err)
		}
		ctx.Info = ir
	} else {
		info, err = ctx.DereferenceDict(*ctx.Info)
		if err != nil || info == nil {
			return fmt.Errorf("读取 PDF 信息字典失败: %w", err)
		}
	}
	info.Update("Title", types.StringLiteral(title))
	info.Update("Author", types.StringLiteral(author))

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".libra-meta-*.pdf")
	if err != nil {
		return fmt.Errorf("创建临时文件失败（书库目录是否可写？）: %w", err)
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpName) }()

	if err := api.WriteContextFile(ctx, tmpName); err != nil {
		return fmt.Errorf("写入 PDF 元数据失败: %w", err)
	}
	return installFile(tmpName, path)
}

// installFile 保留原文件权限，并用临时文件原子替换目标文件。
func installFile(tmpName, dst string) error {
	fi, err := os.Stat(dst)
	if err != nil {
		return err
	}
	if err := os.Chmod(tmpName, fi.Mode().Perm()); err != nil {
		// 权限设置失败不阻断写入（某些文件系统不支持）
		_ = err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		// Windows 不允许 rename 覆盖已存在文件；先备份，再替换，失败时恢复。
		backup := dst + ".libra-bak"
		_ = os.Remove(backup)
		if moveErr := os.Rename(dst, backup); moveErr != nil {
			return fmt.Errorf("替换原文件失败: %w", err)
		}
		if moveErr := os.Rename(tmpName, dst); moveErr != nil {
			_ = os.Rename(backup, dst)
			return fmt.Errorf("替换原文件失败: %w", moveErr)
		}
		_ = os.Remove(backup)
	}
	return nil
}
