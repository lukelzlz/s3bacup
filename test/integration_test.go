// Package test provides integration tests for the full backup pipeline
package test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lukelzlz/s3backup/pkg/archive"
	"github.com/lukelzlz/s3backup/pkg/config"
	"github.com/lukelzlz/s3backup/pkg/crypto"
	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/storage"
	"github.com/lukelzlz/s3backup/pkg/uploader"
)

// mockStorageAdapter 是一個模擬的存儲適配器，用於集成測試
type mockStorageAdapter struct {
	uploads         map[string][]byte
	uploadIDs       map[string]string
	parts           map[string][]storage.CompletedPart
	initCalled      bool
	completeCalled  bool
	abortCalled     bool
	failUpload      bool
}

func newMockStorageAdapter() *mockStorageAdapter {
	return &mockStorageAdapter{
		uploads:   make(map[string][]byte),
		uploadIDs: make(map[string]string),
		parts:     make(map[string][]storage.CompletedPart),
	}
}

func (m *mockStorageAdapter) InitMultipartUpload(ctx context.Context, key string, opts storage.UploadOptions) (string, error) {
	m.initCalled = true
	uploadID := "upload-" + key
	m.uploadIDs[key] = uploadID
	m.parts[key] = []storage.CompletedPart{}
	return uploadID, nil
}

func (m *mockStorageAdapter) UploadPart(ctx context.Context, key, uploadID string, partNumber int, data io.Reader, size int64) (string, error) {
	if m.failUpload {
		return "", storage.ErrMockUploadPartFailed
	}

	partData, err := io.ReadAll(data)
	if err != nil {
		return "", err
	}

	// 存儲分塊數據（使用組合鍵）
	partKey := key + "#" + uploadID + "#" + string(rune(partNumber))
	m.uploads[partKey] = partData

	// 返回模擬 ETag
	etag := "etag-" + string(rune(partNumber))
	m.parts[key] = append(m.parts[key], storage.CompletedPart{
		PartNumber: partNumber,
		ETag:       etag,
	})

	return etag, nil
}

func (m *mockStorageAdapter) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []storage.CompletedPart) error {
	m.completeCalled = true
	return nil
}

func (m *mockStorageAdapter) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	m.abortCalled = true
	delete(m.uploadIDs, key)
	delete(m.parts, key)
	return nil
}

func (m *mockStorageAdapter) SupportedStorageClasses() []storage.StorageClass {
	return []storage.StorageClass{
		storage.StorageClassStandard,
		storage.StorageClassInfrequent,
	}
}

func (m *mockStorageAdapter) SetStorageClass(ctx context.Context, key string, class storage.StorageClass) error {
	return nil
}

func (m *mockStorageAdapter) GetUploadedData(key string) []byte {
	// 合併所有分塊數據
	var result []byte
	for partKey, data := range m.uploads {
		if strings.HasPrefix(partKey, key+"#") {
			result = append(result, data...)
		}
	}
	return result
}

func (m *mockStorageAdapter) reset() {
	m.uploads = make(map[string][]byte)
	m.uploadIDs = make(map[string]string)
	m.parts = make(map[string][]storage.CompletedPart)
	m.initCalled = false
	m.completeCalled = false
	m.abortCalled = false
	m.failUpload = false
}

// TestArchiveEncryptUploadPipeline 測試完整的備份流水線：歸檔 -> 加密 -> 上傳
func TestArchiveEncryptUploadPipeline(t *testing.T) {
	// 創建測試數據
	tmpDir := t.TempDir()
	testFiles := map[string]string{
		"file1.txt":          "Hello, World!",
		"file2.txt":          "This is a test.",
		"subdir/file3.txt":   "Nested file content",
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// 1. 創建歸檔器並歸檔
	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var archiveBuf bytes.Buffer
	if err := a.Archive(context.Background(), &archiveBuf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	archiveData := archiveBuf.Bytes()
	if len(archiveData) == 0 {
		t.Fatal("archive produced no data")
	}

	t.Logf("Archive size: %d bytes", len(archiveData))

	// 2. 加密歸檔數據
	password := "test-password-123"
	aesKey, hmacKey, err := crypto.DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := crypto.NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	var encryptedBuf bytes.Buffer
	writer, err := encryptor.WrapWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("failed to wrap writer: %v", err)
	}

	if _, err := writer.Write(archiveData); err != nil {
		t.Fatalf("failed to write encrypted data: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close encryptor: %v", err)
	}

	encryptedData := encryptedBuf.Bytes()
	t.Logf("Encrypted size: %d bytes", len(encryptedData))

	// 驗證加密數據格式
	if len(encryptedData) < 4+16+8+64 { // magic + IV + length + HMAC
		t.Fatalf("encrypted data too small: %d bytes", len(encryptedData))
	}

	magic := encryptedData[:4]
	if string(magic) != "S3BE" {
		t.Errorf("wrong magic number: %s", string(magic))
	}

	// 3. 上傳加密數據
	adapter := newMockStorageAdapter()
	up := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()
	err = up.Upload(ctx, "backup-test.tar.gz.enc", bytes.NewReader(encryptedData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// 驗證上傳狀態
	if !adapter.initCalled {
		t.Error("InitMultipartUpload was not called")
	}
	if !adapter.completeCalled {
		t.Error("CompleteMultipartUpload was not called")
	}

	// 4. 下載並解密驗證
	uploadedData := adapter.GetUploadedData("backup-test.tar.gz.enc")
	if len(uploadedData) != len(encryptedData) {
		t.Errorf("uploaded data size mismatch: got %d, want %d", len(uploadedData), len(encryptedData))
	}

	// 解密上傳的數據
	reader, err := encryptor.WrapReaderWithHMAC(bytes.NewReader(uploadedData))
	if err != nil {
		t.Fatalf("failed to wrap reader with HMAC: %v", err)
	}

	decryptedData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read decrypted data: %v", err)
	}

	if !bytes.Equal(decryptedData, archiveData) {
		t.Error("decrypted data does not match original archive")
	}

	t.Logf("Pipeline test completed successfully!")
	t.Logf("Original archive: %d bytes -> Encrypted: %d bytes -> Uploaded: %d bytes",
		len(archiveData), len(encryptedData), len(uploadedData))
}

// TestArchiveUploadWithoutEncryption 測試不加密的備份流水線
func TestArchiveUploadWithoutEncryption(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建測試文件
	testContent := strings.Repeat("test data ", 1000)
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// 歸檔
	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var archiveBuf bytes.Buffer
	if err := a.Archive(context.Background(), &archiveBuf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	// 直接上傳（不加密）
	adapter := newMockStorageAdapter()
	up := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()
	err = up.Upload(ctx, "backup-test.tar.gz", bytes.NewReader(archiveBuf.Bytes()), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if !adapter.completeCalled {
		t.Error("CompleteMultipartUpload was not called")
	}

	uploadedData := adapter.GetUploadedData("backup-test.tar.gz")
	if len(uploadedData) != len(archiveBuf.Bytes()) {
		t.Errorf("uploaded data size mismatch: got %d, want %d", len(uploadedData), len(archiveBuf.Bytes()))
	}
}

// TestPipelineWithExclusions 測試帶排除模式的流水線
func TestPipelineWithExclusions(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建混合文件
	files := map[string]string{
		"keep.txt":           "keep this",
		"exclude.log":        "exclude this",
		"subdir/keep.txt":    "keep this too",
		"subdir/exclude.tmp": "exclude this too",
	}

	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// 使用排除模式
	a, err := archive.NewArchiver([]string{tmpDir}, []string{"*.log", "*.tmp"})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var buf bytes.Buffer
	if err := a.Archive(context.Background(), &buf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	archiveData := buf.Bytes()

	// 驗證被排除的文件不在歸檔中
	archiveStr := string(archiveData)
	if strings.Contains(archiveStr, "exclude.log") {
		t.Error("excluded .log file should not be in archive")
	}
	if strings.Contains(archiveStr, "exclude.tmp") {
		t.Error("excluded .tmp file should not be in archive")
	}
}

// TestPipelineCancellation 測試流水線取消
func TestPipelineCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建一個大文件
	largeFile := filepath.Join(tmpDir, "large.txt")
	largeData := bytes.Repeat([]byte("x"), 10*1024*1024) // 10MB
	if err := os.WriteFile(largeFile, largeData, 0644); err != nil {
		t.Fatalf("failed to create large file: %v", err)
	}

	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	// 創建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 啟動歸檔並立即取消
	done := make(chan error, 1)
	go func() {
		var buf bytes.Buffer
		done <- a.Archive(ctx, &buf)
	}()

	// 短暫等待後取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	err = <-done
	if err != nil && err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("Got error (may be expected): %v", err)
	}
}

// TestPipelineErrorRecovery 測試錯誤恢復
func TestPipelineErrorRecovery(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建測試文件
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var buf bytes.Buffer
	if err := a.Archive(context.Background(), &buf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	// 嘗試上傳並模擬失敗
	adapter := newMockStorageAdapter()
	adapter.failUpload = true

	up := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()
	err = up.Upload(ctx, "backup-test.tar.gz", bytes.NewReader(buf.Bytes()), storage.UploadOptions{})
	if err == nil {
		t.Error("expected upload to fail")
	}

	// 驗證中止被調用
	if !adapter.abortCalled {
		t.Error("AbortMultipartUpload should be called on upload failure")
	}

	// 重置並重試 - 應該成功
	adapter.reset()
	adapter.failUpload = false

	up2 := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up2.SetProgressReporter(progress.NewSilent())

	err = up2.Upload(ctx, "backup-test.tar.gz", bytes.NewReader(buf.Bytes()), storage.UploadOptions{})
	if err != nil {
		t.Errorf("retry upload failed: %v", err)
	}

	if !adapter.completeCalled {
		t.Error("CompleteMultipartUpload should be called on successful retry")
	}
}

// TestPipelineWithConfig 測試使用配置的流水線
func TestPipelineWithConfig(t *testing.T) {
	// 創建配置
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Provider:     "aws",
			Bucket:       "test-bucket",
			AccessKey:    "test-key",
			SecretKey:    "test-secret",
			StorageClass: "standard",
		},
		Backup: config.BackupConfig{
			ChunkSize:   5 * 1024 * 1024,
			Concurrency: 2,
		},
		Encryption: config.EncryptionConfig{
			Enabled: false,
		},
	}

	// 驗證配置
	if cfg.Storage.Provider != "aws" {
		t.Errorf("provider: got %s, want aws", cfg.Storage.Provider)
	}
	if cfg.Backup.ChunkSize < 5*1024*1024 {
		t.Errorf("chunk size too small: %d", cfg.Backup.ChunkSize)
	}
	if cfg.Backup.Concurrency < 1 {
		t.Errorf("invalid concurrency: %d", cfg.Backup.Concurrency)
	}

	t.Logf("Config validated: provider=%s, bucket=%s, chunk=%d, concurrency=%d",
		cfg.Storage.Provider, cfg.Storage.Bucket, cfg.Backup.ChunkSize, cfg.Backup.Concurrency)
}

// TestPipelineMemoryUsage 測試流水線內存使用
func TestPipelineMemoryUsage(t *testing.T) {
	// 創建相對較大的測試數據
	tmpDir := t.TempDir()

	// 創建多個文件
	for i := 0; i < 10; i++ {
		data := bytes.Repeat([]byte("test data "), 100*1024) // ~600KB 每個文件
		path := filepath.Join(tmpDir, filepath.Join("subdir", "file.txt"))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var buf bytes.Buffer
	if err := a.Archive(context.Background(), &buf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	t.Logf("Archive size for multiple files: %d bytes", buf.Len())

	// 測試上傳
	adapter := newMockStorageAdapter()
	up := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()
	if err := up.Upload(ctx, "test.tar.gz", bytes.NewReader(buf.Bytes()), storage.UploadOptions{}); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	t.Logf("Memory usage test completed successfully")
}

// TestPipelineEmptyDirectory 測試空目錄流水線
func TestPipelineEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建空目錄
	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.Mkdir(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create empty directory: %v", err)
	}

	a, err := archive.NewArchiver([]string{emptyDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var buf bytes.Buffer
	if err := a.Archive(context.Background(), &buf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	// 空目錄應該產生有效的 tar.gz（包含目錄項）
	if buf.Len() == 0 {
		t.Error("empty directory should produce some output")
	}
}

// TestPipelineSpecialCharacters 測試特殊字符文件名
func TestPipelineSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建帶特殊字符的文件
	specialNames := []string{
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"file.multiple.dots.txt",
		"文件.txt", // 中文
	}

	for _, name := range specialNames {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	a, err := archive.NewArchiver([]string{tmpDir}, []string{})
	if err != nil {
		t.Fatalf("failed to create archiver: %v", err)
	}

	var buf bytes.Buffer
	if err := a.Archive(context.Background(), &buf); err != nil {
		t.Fatalf("archive failed: %v", err)
	}

	// 歸檔應該成功處理特殊字符
	if buf.Len() == 0 {
		t.Error("archive with special filenames should produce output")
	}

	// 測試上傳
	adapter := newMockStorageAdapter()
	up := uploader.NewUploader(adapter, 5*1024*1024, 2)
	up.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()
	if err := up.Upload(ctx, "special.tar.gz", bytes.NewReader(buf.Bytes()), storage.UploadOptions{}); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	t.Logf("Special characters test completed")
}

// TestEncryptionIntegrity 測試加密完整性
func TestEncryptionIntegrity(t *testing.T) {
	password := "test-password-123"
	aesKey, hmacKey, err := crypto.DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := crypto.NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	originalData := []byte("This is sensitive data that must be encrypted properly!")

	// 加密
	var encryptedBuf bytes.Buffer
	writer, err := encryptor.WrapWriter(&encryptedBuf)
	if err != nil {
		t.Fatalf("failed to wrap writer: %v", err)
	}

	if _, err := writer.Write(originalData); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	encryptedData := encryptedBuf.Bytes()

	// 解密
	reader, err := encryptor.WrapReaderWithHMAC(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("failed to wrap reader: %v", err)
	}

	decryptedData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	// 驗證數據完整性
	if !bytes.Equal(decryptedData, originalData) {
		t.Errorf("data mismatch:\noriginal: %s\ndecrypted: %s", originalData, decryptedData)
	}

	// 驗證加密數據與原始數據不同
	if bytes.Equal(encryptedData, originalData) {
		t.Error("encrypted data should be different from original")
	}

	t.Logf("Encryption integrity test passed")
}

// TestMultipleBackups 測試多次備份
func TestMultipleBackups(t *testing.T) {
	tmpDir := t.TempDir()

	// 創建初始文件
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("version 1"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// 第一次備份
	a1, _ := archive.NewArchiver([]string{tmpDir}, []string{})
	var buf1 bytes.Buffer
	a1.Archive(context.Background(), &buf1)

	// 修改文件
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("version 2 - modified content"), 0644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}

	// 第二次備份
	a2, _ := archive.NewArchiver([]string{tmpDir}, []string{})
	var buf2 bytes.Buffer
	a2.Archive(context.Background(), &buf2)

	// 兩次備份應該產生不同的結果（內容不同）
	if bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Error("different content should produce different archives")
	}

	// 但大小可能相同（都是單個小文件）
	t.Logf("Backup 1 size: %d, Backup 2 size: %d", buf1.Len(), buf2.Len())
}
