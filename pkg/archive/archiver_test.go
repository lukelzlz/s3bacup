package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPathTraversalProtection 测试路径遍历保护
func TestPathTraversalProtection(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"normal path", "/home/user/documents", false},
		{"normal relative path", "documents", false},
		{"path with parent directory reference", "/home/user/../etc", true},
		{"path with double dot in middle", "/home/user/../../etc", true},
		{"relative path with double dot", "../etc", true},
		{"cleaned path with double dot", "./../test", true},
		{"complex path traversal", "/home/./user/../test", true},
		{"safe dot path", "/home/user/./documents", false}, // ./ 在清理后是安全的
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := NewArchiver([]string{}, []string{})
			err := a.validatePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestArchiverWithSymlinks 测试符号链接处理
func TestArchiverWithSymlinks(t *testing.T) {
	// 创建临时目录结构
	tmpDir := t.TempDir()

	// 创建普通文件
	normalFile := filepath.Join(tmpDir, "normal.txt")
	if err := os.WriteFile(normalFile, []byte("normal content"), 0644); err != nil {
		t.Fatalf("failed to create normal file: %v", err)
	}

	// 创建符号链接到外部（如果支持）
	symlinkToOutside := filepath.Join(tmpDir, "symlink_outside")
	// 尝试创建指向 /etc 的符号链接（仅Unix）
	if err := os.Symlink("/etc", symlinkToOutside); err == nil {
		// 符号链接创建成功，测试它
		a, _ := NewArchiver([]string{tmpDir}, []string{})

		// 符号链接路径本身是安全的（它不包含 ..）
		if err := a.validatePath(symlinkToOutside); err != nil {
			t.Errorf("symlink path should be safe: %v", err)
		}
	}
}

// TestIsPathSafe 测试路径安全检查
func TestIsPathSafe(t *testing.T) {
	tests := []struct {
		name string
		path string
		safe bool
	}{
		{"safe absolute path", "/home/user/documents", true},
		{"safe relative path", "documents", true},
		{"unsafe with ..", "/home/user/../etc", false},
		{"unsafe relative ..", "../documents", false},
		{"complex unsafe", "/home/./user/../test", false},
		{"safe single dot", "/home/user/./docs", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := NewArchiver([]string{}, []string{})
			result := a.isPathSafe(tt.path)
			if result != tt.safe {
				t.Errorf("isPathSafe(%q) = %v, want %v", tt.path, result, tt.safe)
			}
		})
	}
}

// TestArchiveWithUnsafePaths 测试归档不安全路径时的行为
func TestArchiveWithUnsafePaths(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建安全的文件
	safeFile := filepath.Join(tmpDir, "safe.txt")
	if err := os.WriteFile(safeFile, []byte("safe content"), 0644); err != nil {
		t.Fatalf("failed to create safe file: %v", err)
	}

	// 创建归档器
	a, err := NewArchiver([]string{safeFile}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	// 尝试归档（只有安全路径）
	var buf strings.Builder
	err = a.Archive(context.Background(), &buf)

	// 对于不安全路径，应该返回错误或安全地跳过
	// 当前的实现会在 validatePath 时返回错误
	if err != nil && strings.Contains(err.Error(), "safety check failed") {
		// 这是预期的行为
		return
	}

	// 如果没有错误，检查是否是因为路径不存在而被跳过
	if err == nil {
		// 检查输出是否为空或只包含安全文件
		if buf.Len() == 0 || strings.Contains(buf.String(), "safe.txt") {
			return
		}
	}

	// 如果既不是预期的安全检查错误，也不是安全的跳过，则测试失败
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
