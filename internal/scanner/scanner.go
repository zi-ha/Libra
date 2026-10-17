// Package scanner 负责递归扫描书库目录、提取书籍元数据与封面。
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// 注册图片解码器，供 image.DecodeConfig 读取内嵌封面尺寸。
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"libra/internal/config"
	"libra/internal/store"
)

// 支持的书籍格式。
const (
	FormatEPUB = "epub"
	FormatPDF  = "pdf"
)

// 封面状态，用于增量扫描决定是否重试。
const (
	coverStateOK     = "ok"     // 已有封面
	coverStateNone   = "none"   // 确认没有封面，不必重试
	coverStateFailed = "failed" // 提取失败，下次扫描重试
)

// 扫描状态机。
const (
	StateIdle     = "idle"
	StateScanning = "scanning"
	StateDone     = "done"
	StateError    = "error"
)

// ErrAlreadyRunning 表示该书库正在扫描中。
var ErrAlreadyRunning = errors.New("该书库正在扫描中")

// Status 是暴露给前端的扫描进度快照。
type Status struct {
	LibraryID  int64     `json:"library_id"`
	State      string    `json:"state"`
	Total      int       `json:"total"`
	Processed  int       `json:"processed"`
	Added      int       `json:"added"`
	Updated    int       `json:"updated"`
	Skipped    int       `json:"skipped"`
	Removed    int       `json:"removed"`
	Failed     int       `json:"failed"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	ElapsedSec float64   `json:"elapsed_sec"`
	Message    string    `json:"message"`
	Errors     []string  `json:"errors"`
}

// tracker 是带锁的状态容器。
type tracker struct {
	mu   sync.Mutex
	st   Status
	errs []string
}

func newTracker(libraryID int64) *tracker {
	return &tracker{st: Status{LibraryID: libraryID, State: StateScanning, StartedAt: time.Now()}}
}

func (t *tracker) incTotal() {
	t.mu.Lock()
	t.st.Total++
	t.mu.Unlock()
}

func (t *tracker) incSkipped() {
	t.mu.Lock()
	t.st.Skipped++
	t.st.Processed++
	t.mu.Unlock()
}

func (t *tracker) incDone(added bool) {
	t.mu.Lock()
	t.st.Processed++
	if added {
		t.st.Added++
	} else {
		t.st.Updated++
	}
	t.mu.Unlock()
}

func (t *tracker) incFailed() {
	t.mu.Lock()
	t.st.Processed++
	t.st.Failed++
	t.mu.Unlock()
}

func (t *tracker) setRemoved(n int) {
	t.mu.Lock()
	t.st.Removed += n
	t.mu.Unlock()
}

func (t *tracker) addError(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.errs) < 50 { // 只保留前 50 条，避免进度响应无限膨胀
		t.errs = append(t.errs, msg)
	}
}

func (t *tracker) finish(state, message string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.st.State = state
	t.st.Message = message
	t.st.FinishedAt = time.Now()
	t.st.ElapsedSec = t.st.FinishedAt.Sub(t.st.StartedAt).Seconds()
}

func (t *tracker) snapshot() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.st
	if st.State == StateScanning {
		st.ElapsedSec = time.Since(st.StartedAt).Seconds()
	}
	st.Errors = append([]string(nil), t.errs...)
	return st
}

// Scanner 调度所有书库的扫描任务。
type Scanner struct {
	store *store.Store
	cfg   *config.Config

	mu       sync.Mutex
	active   map[int64]bool
	statuses map[int64]*tracker

	toolsMu sync.Mutex
	tools   map[string]bool
}

// New 构造扫描器。
func New(st *store.Store, cfg *config.Config) *Scanner {
	return &Scanner{
		store:    st,
		cfg:      cfg,
		active:   make(map[int64]bool),
		statuses: make(map[int64]*tracker),
		tools:    make(map[string]bool),
	}
}

// Status 返回某个书库最近一次扫描的进度快照。
func (s *Scanner) Status(libraryID int64) *Status {
	s.mu.Lock()
	tk := s.statuses[libraryID]
	s.mu.Unlock()
	if tk == nil {
		return nil
	}
	st := tk.snapshot()
	return &st
}

// IsRunning 判断某个书库是否正在扫描。
func (s *Scanner) IsRunning(libraryID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active[libraryID]
}

// Start 异步启动一次扫描。若该库已经在扫描中则返回 ErrAlreadyRunning。
func (s *Scanner) Start(libraryID int64) error {
	lib, err := s.store.GetLibrary(libraryID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.active[libraryID] {
		s.mu.Unlock()
		return ErrAlreadyRunning
	}
	tk := newTracker(libraryID)
	s.active[libraryID] = true
	s.statuses[libraryID] = tk
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.active[libraryID] = false
			s.mu.Unlock()
		}()
		if err := s.run(lib, tk); err != nil {
			log.Printf("[scan] 书库 #%d (%s) 扫描失败: %v", lib.ID, lib.Name, err)
			tk.addError(err.Error())
			tk.finish(StateError, err.Error())
			_ = s.store.MarkLibraryScan(lib.ID, StateError)
			return
		}
		st := tk.snapshot()
		log.Printf("[scan] 书库 #%d (%s) 完成: 新增 %d / 更新 %d / 跳过 %d / 移除 %d / 失败 %d，耗时 %.1fs",
			lib.ID, lib.Name, st.Added, st.Updated, st.Skipped, st.Removed, st.Failed, st.ElapsedSec)
		tk.finish(StateDone, "")
		_ = s.store.MarkLibraryScan(lib.ID, StateDone)
	}()

	return nil
}

// StartAll 依次扫描所有书库（启动时自动扫描用）。
func (s *Scanner) StartAll() {
	libs, err := s.store.ListLibraries()
	if err != nil {
		log.Printf("[scan] 读取书库列表失败: %v", err)
		return
	}
	for _, lib := range libs {
		if err := s.Start(lib.ID); err != nil && !errors.Is(err, ErrAlreadyRunning) {
			log.Printf("[scan] 启动书库 #%d 扫描失败: %v", lib.ID, err)
		}
	}
}

// RemoveLibraryCovers 删除某个书库的全部封面目录（删除书库时调用）。
func (s *Scanner) RemoveLibraryCovers(libraryID int64) error {
	return s.RemoveLibraryCoversExcept(libraryID, "")
}

// RemoveLibraryCoversExcept 清空封面目录，但保留 keep 指定的那个文件。
// 改书库目录时会调用它，避免把管理员上传的书库封面一起删掉。
func (s *Scanner) RemoveLibraryCoversExcept(libraryID int64, keep string) error {
	dir := s.cfg.LibraryCoversDir(libraryID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if keep != "" && e.Name() == keep {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// scanJob 是一个待处理的文件。
type scanJob struct {
	path   string
	size   int64
	mtime  int64
	format string
}

// scanResult 是处理结果。
type scanResult struct {
	path       string
	title      string
	author     string
	format     string
	size       int64
	mtime      int64
	cover      string
	coverState string
	oldCover   string
	warn       string
	err        error
}

// run 执行一次完整的增量扫描。
func (s *Scanner) run(lib *store.Library, tk *tracker) error {
	root := lib.Path
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("目录不存在: %s", root)
		}
		return fmt.Errorf("无法访问目录 %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("路径不是目录: %s", root)
	}

	coversDir := s.cfg.LibraryCoversDir(lib.ID)
	tmpDir := filepath.Join(coversDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("创建封面目录失败: %w", err)
	}
	defer func() {
		// 清掉本次遗留的临时文件（成功的已被 rename 走）。
		if entries, err := os.ReadDir(tmpDir); err == nil {
			for _, e := range entries {
				_ = os.Remove(filepath.Join(tmpDir, e.Name()))
			}
		}
	}()

	index, err := s.store.IndexByLibrary(lib.ID)
	if err != nil {
		return fmt.Errorf("读取增量索引失败: %w", err)
	}

	jobs := make(chan scanJob, 128)
	results := make(chan scanResult, 128)

	// seen 记录本次扫描实际看到的文件，用于清理已从磁盘删除的记录。
	// 只有 producer 写；主协程在所有 worker 结束后才读，不存在数据竞争。
	seen := make(map[string]struct{}, len(index)+128)

	var wg sync.WaitGroup
	for i := 0; i < s.cfg.ScanConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				results <- s.process(context.Background(), coversDir, tmpDir, j, index)
			}
		}()
	}

	go func() {
		defer close(jobs)
		walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					tk.addError(fmt.Sprintf("跳过无法访问的目录 %s: %v", p, err))
					return fs.SkipDir
				}
				tk.addError(fmt.Sprintf("跳过无法访问的文件 %s: %v", p, err))
				return nil
			}
			name := d.Name()

			if d.IsDir() {
				if p != root && strings.HasPrefix(name, ".") {
					return fs.SkipDir // 跳过隐藏目录（.git、@eaDir 等）
				}
				return nil
			}
			if strings.HasPrefix(name, ".") {
				return nil
			}

			format := detectFormat(name)
			if format == "" {
				return nil
			}
			fi, err := d.Info()
			if err != nil {
				tk.addError(fmt.Sprintf("读取文件信息失败 %s: %v", p, err))
				return nil
			}

			abs := p
			seen[abs] = struct{}{}

			mtime := fi.ModTime().Unix()
			size := fi.Size()

			if old, ok := index[abs]; ok && old.Mtime == mtime && old.Size == size && old.CoverState != coverStateFailed {
				tk.incSkipped()
				return nil
			}

			tk.incTotal()
			jobs <- scanJob{path: abs, size: size, mtime: mtime, format: format}
			return nil
		})
		if walkErr != nil {
			tk.addError(fmt.Sprintf("目录遍历异常: %v", walkErr))
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			tk.incFailed()
			tk.addError(fmt.Sprintf("%s: %v", filepath.Base(r.path), r.err))
			continue
		}
		if r.warn != "" {
			tk.addError(fmt.Sprintf("%s: %s", filepath.Base(r.path), r.warn))
		}

		_, existed := index[r.path]
		_, err := s.store.UpsertBook(&store.Book{
			LibraryID:  lib.ID,
			Path:       r.path,
			Dir:        filepath.Dir(r.path),
			Filename:   filepath.Base(r.path),
			Title:      r.title,
			Author:     r.author,
			Format:     r.format,
			Size:       r.size,
			Mtime:      r.mtime,
			Cover:      r.cover,
			CoverState: r.coverState,
		})
		if err != nil {
			tk.incFailed()
			tk.addError(fmt.Sprintf("写入 %s 失败: %v", filepath.Base(r.path), err))
			continue
		}
		tk.incDone(!existed)

		// 封面扩展名变化时清掉旧文件，避免孤儿文件堆积。
		if r.oldCover != "" && r.oldCover != r.cover {
			_ = os.Remove(filepath.Join(coversDir, r.oldCover))
		}
	}

	// 清理磁盘上已不存在的书籍记录与封面。
	removedCount, staleCovers, err := s.store.DeleteBooksExcept(lib.ID, seen)
	if err != nil {
		tk.addError(fmt.Sprintf("清理失效记录失败: %v", err))
	} else {
		tk.setRemoved(removedCount)
		for _, c := range staleCovers {
			_ = os.Remove(filepath.Join(coversDir, c))
		}
	}

	return nil
}

// process 在 worker 中执行单个文件的元数据提取与封面生成。
// 该函数只读共享数据、只写自己那份结果，因此可以被多个 worker 并发调用。
func (s *Scanner) process(ctx context.Context, coversDir, tmpDir string, j scanJob, index map[string]store.BookIndexRow) scanResult {
	res := scanResult{
		path:   j.path,
		format: j.format,
		size:   j.size,
		mtime:  j.mtime,
		// 兜底元数据：文件名（去扩展名）作为书名。
		title:      cleanText(strings.TrimSuffix(filepath.Base(j.path), filepath.Ext(j.path))),
		coverState: coverStateNone,
	}
	if old, ok := index[j.path]; ok {
		res.oldCover = old.Cover
	}

	switch j.format {
	case FormatEPUB:
		s.processEPUB(j.path, coversDir, &res)
	case FormatPDF:
		s.processPDF(ctx, j.path, coversDir, tmpDir, &res)
	}
	return res
}

func (s *Scanner) processEPUB(path, coversDir string, res *scanResult) {
	info, err := parseEPUB(path)
	if err != nil {
		res.warn = "EPUB 解析失败，书名回退为文件名"
		res.coverState = coverStateFailed
		return
	}
	if info.Title != "" {
		res.title = info.Title
	}
	res.author = info.Author

	if len(info.Cover) == 0 {
		res.coverState = coverStateNone
		return
	}
	ext := info.CoverExt
	if ext == "" {
		ext = ".jpg"
	}
	name := coverFileName(path, ext)
	if err := writeFileAtomic(filepath.Join(coversDir, name), info.Cover); err != nil {
		res.warn = "写入 EPUB 封面失败: " + err.Error()
		res.coverState = coverStateFailed
		return
	}
	res.cover = name
	res.coverState = coverStateOK
}

func (s *Scanner) processPDF(ctx context.Context, path, coversDir, tmpDir string, res *scanResult) {
	if t := s.pdfTitle(ctx, path); t != "" {
		res.title = t
	} else {
		res.coverState = coverStateNone
	}

	// 优先使用 PDF 内嵌封面图（更清晰、体积更小）。
	if data, ext := s.extractEmbeddedPDFCover(ctx, path, tmpDir); len(data) > 0 {
		if ext == "" {
			ext = ".jpg"
		}
		name := coverFileName(path, ext)
		if err := writeFileAtomic(filepath.Join(coversDir, name), data); err == nil {
			res.cover = name
			res.coverState = coverStateOK
			return
		}
	}

	// 退化为渲染第 1 页。
	prefix := filepath.Join(tmpDir, coverFileName(path, ""))
	out, err := s.renderPDFCover(ctx, path, prefix, s.cfg.CoverWidth)
	if err != nil {
		res.warn = err.Error()
		res.coverState = coverStateFailed
		return
	}
	name := coverFileName(path, ".jpg")
	if err := os.Rename(out, filepath.Join(coversDir, name)); err != nil {
		// 跨设备 rename 会失败，退化为拷贝。
		data, readErr := os.ReadFile(out)
		if readErr != nil || writeFileAtomic(filepath.Join(coversDir, name), data) != nil {
			res.warn = "保存 PDF 封面失败: " + err.Error()
			res.coverState = coverStateFailed
			return
		}
	}
	res.cover = name
	res.coverState = coverStateOK
}

// detectFormat 依据扩展名判断格式，未支持的类型返回空串。
//
// 想扩展 mobi/azw3 等格式时，只需在此处加一行并在 process 中补一个分支。
func detectFormat(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".epub":
		return FormatEPUB
	case ".pdf":
		return FormatPDF
	default:
		return ""
	}
}

// coverFileName 生成稳定的封面文件名：基于绝对路径的哈希，重复扫描可原地覆盖。
func coverFileName(absPath, ext string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:])[:24] + ext
}

// writeFileAtomic 先写临时文件再改名，避免出现半截封面。
func writeFileAtomic(dst string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
