package archive

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gobwas/glob"
)

// Archiver 归档器
type Archiver struct {
	includes []string
	excludes []glob.Glob
}

// NewArchiver 创建归档器
func NewArchiver(includes, excludes []string) (*Archiver, error) {
	excludePatterns := make([]glob.Glob, len(excludes))
	for i, pattern := range excludes {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile exclude pattern %s: %w", pattern, err)
		}
		excludePatterns[i] = g
	}

	return &Archiver{
		includes: includes,
		excludes: excludePatterns,
	}, nil
}

// Archive 将文件打包为 tar.gz 流写入到 writer
func (a *Archiver) Archive(ctx context.Context, w io.Writer) error {
	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()

	tarWriter := NewTarWriter(gzWriter)
	defer tarWriter.Close()

	for _, include := range a.includes {
		if err := a.archivePath(ctx, tarWriter, include, ""); err != nil {
			return fmt.Errorf("failed to archive %s: %w", include, err)
		}
	}

	return nil
}

// archivePath 递归归档路径
func (a *Archiver) archivePath(ctx context.Context, tw *TarWriter, path, base string) error {
	// 检查是否被排除
	if a.isExcluded(path) {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}

	// 计算归档内的路径
	archivePath := path
	if base != "" {
		archivePath = filepath.Join(base, filepath.Base(path))
	}

	// 检查上下文是否取消
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if info.IsDir() {
		// 写入目录 header
		if err := tw.WriteHeader(&TarHeader{
			Name:       archivePath + "/",
			Mode:       0755,
			ModTime:    info.ModTime(),
			Typeflag:   TypeDir,
			AccessTime: time.Now(),
			ChangeTime: time.Now(),
		}); err != nil {
			return fmt.Errorf("failed to write dir header: %w", err)
		}

		// 递归处理目录内容
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("failed to read dir %s: %w", path, err)
		}

		for _, entry := range entries {
			fullPath := filepath.Join(path, entry.Name())
			if err := a.archivePath(ctx, tw, fullPath, archivePath); err != nil {
				return err
			}
		}
	} else {
		// 写入文件
		if err := a.archiveFile(tw, path, archivePath, info); err != nil {
			return err
		}
	}

	return nil
}

// archiveFile 归档单个文件
func (a *Archiver) archiveFile(tw *TarWriter, path, archivePath string, info os.FileInfo) error {
	// 检查是否被排除
	if a.isExcluded(path) {
		return nil
	}

	// 打开文件
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	// 写入 header
	header := &TarHeader{
		Name:       archivePath,
		Mode:       int64(info.Mode()),
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		Typeflag:   TypeReg,
		AccessTime: time.Now(),
		ChangeTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// 写入文件内容
	if _, err := io.Copy(tw, file); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// isExcluded 检查路径是否被排除
func (a *Archiver) isExcluded(path string) bool {
	// 标准化路径（使用 / 作为分隔符）
	normalizedPath := filepath.ToSlash(path)

	for _, g := range a.excludes {
		if g.Match(normalizedPath) {
			return true
		}
	}
	return false
}

// GetTotalSize 计算所有包含文件的总大小
func (a *Archiver) GetTotalSize(ctx context.Context) (int64, error) {
	var total int64

	for _, include := range a.includes {
		size, err := a.getPathSize(ctx, include)
		if err != nil {
			return 0, err
		}
		total += size
	}

	return total, nil
}

// getPathSize 递归计算路径大小
func (a *Archiver) getPathSize(ctx context.Context, path string) (int64, error) {
	if a.isExcluded(path) {
		return 0, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	if info.IsDir() {
		var total int64
		entries, err := os.ReadDir(path)
		if err != nil {
			return 0, fmt.Errorf("failed to read dir %s: %w", path, err)
		}

		for _, entry := range entries {
			fullPath := filepath.Join(path, entry.Name())
			size, err := a.getPathSize(ctx, fullPath)
			if err != nil {
				return 0, err
			}
			total += size
		}

		return total, nil
	}

	return info.Size(), nil
}

// ResolveIncludes 解析包含路径，展开通配符
func ResolveIncludes(includes []string) ([]string, error) {
	var resolved []string

	for _, include := range includes {
		// 检查是否包含通配符
		if strings.ContainsAny(include, "*?[]") {
			matches, err := filepath.Glob(include)
			if err != nil {
				return nil, fmt.Errorf("failed to glob %s: %w", include, err)
			}
			if len(matches) == 0 {
				return nil, fmt.Errorf("no matches found for pattern: %s", include)
			}
			resolved = append(resolved, matches...)
		} else {
			// 检查路径是否存在
			if _, err := os.Stat(include); err != nil {
				return nil, fmt.Errorf("path not found: %s", include)
			}
			resolved = append(resolved, include)
		}
	}

	return resolved, nil
}
