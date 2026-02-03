package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync/atomic"
	"testing"

	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/storage"
)

// mockAdapter 是用于测试的模拟存储适配器
type mockAdapter struct {
	initCalled         atomic.Int64
	uploadPartCalled   atomic.Int64
	completeCalled     atomic.Int64
	abortCalled        atomic.Int64
	uploadedParts      []storage.CompletedPart
	shouldFailInit     bool
	shouldFailPart     bool
	shouldFailComplete bool
	partNumberToFail   int
}

func (m *mockAdapter) InitMultipartUpload(ctx context.Context, key string, opts storage.UploadOptions) (string, error) {
	m.initCalled.Add(1)
	if m.shouldFailInit {
		return "", storage.ErrMockInitFailed
	}
	return "mock-upload-id", nil
}

func (m *mockAdapter) UploadPart(ctx context.Context, key, uploadID string, partNumber int, r io.Reader, size int64) (string, error) {
	m.uploadPartCalled.Add(1)
	if m.shouldFailPart && partNumber == m.partNumberToFail {
		return "", storage.ErrMockUploadPartFailed
	}
	// 读取所有数据以确保正确传递
	data, _ := io.ReadAll(r)
	m.uploadedParts = append(m.uploadedParts, storage.CompletedPart{
		PartNumber: partNumber,
		ETag:       fmt.Sprintf("etag-%d", len(data)),
	})
	return fmt.Sprintf("etag-%d", len(data)), nil
}

func (m *mockAdapter) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []storage.CompletedPart) error {
	m.completeCalled.Add(1)
	if m.shouldFailComplete {
		return storage.ErrMockCompleteFailed
	}
	return nil
}

func (m *mockAdapter) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	m.abortCalled.Add(1)
	return nil
}

func (m *mockAdapter) SupportedStorageClasses() []storage.StorageClass {
	return []storage.StorageClass{storage.StorageClassStandard}
}

func (m *mockAdapter) SetStorageClass(ctx context.Context, key string, class storage.StorageClass) error {
	return nil
}

func (m *mockAdapter) reset() {
	m.initCalled.Store(0)
	m.uploadPartCalled.Add(-m.uploadPartCalled.Load())
	m.completeCalled.Store(0)
	m.abortCalled.Store(0)
	m.uploadedParts = nil
	m.shouldFailInit = false
	m.shouldFailPart = false
	m.shouldFailComplete = false
	m.partNumberToFail = 0
}

// TestNewUploader 测试创建上传管理器
func TestNewUploader(t *testing.T) {
	adapter := &mockAdapter{}

	tests := []struct {
		name        string
		chunkSize   int64
		concurrency int
		wantChunk   int64
		wantConc    int
	}{
		{"default values", 0, 0, 5 * 1024 * 1024, 4},
		{"custom values", 10 * 1024 * 1024, 8, 10 * 1024 * 1024, 8},
		{"negative values", -1, -1, 5 * 1024 * 1024, 4},
		{"valid values", 6 * 1024 * 1024, 2, 6 * 1024 * 1024, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := NewUploader(adapter, tt.chunkSize, tt.concurrency)
			if u.chunkSize != tt.wantChunk {
				t.Errorf("expected chunk size %d, got %d", tt.wantChunk, u.chunkSize)
			}
			if u.concurrency != tt.wantConc {
				t.Errorf("expected concurrency %d, got %d", tt.wantConc, u.concurrency)
			}
		})
	}
}

// TestSetProgressReporter 测试设置进度报告器
func TestSetProgressReporter(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)

	reporter := &progress.MockReporter{}
	u.SetProgressReporter(reporter)

	if u.reporter != reporter {
		t.Error("progress reporter was not set correctly")
	}
}

// TestUploadSuccess 测试成功上传
func TestUploadSuccess(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 15MB 的测试数据（3 个分块）
	testData := make([]byte, 15*1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	ctx := context.Background()
	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})

	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 验证初始化被调用一次
	if adapter.initCalled.Load() != 1 {
		t.Errorf("expected InitMultipartUpload to be called once, got %d", adapter.initCalled.Load())
	}

	// 验证完成了 3 个分块上传
	if adapter.uploadPartCalled.Load() != 3 {
		t.Errorf("expected 3 UploadPart calls, got %d", adapter.uploadPartCalled.Load())
	}

	// 验证完成被调用一次
	if adapter.completeCalled.Load() != 1 {
		t.Errorf("expected CompleteMultipartUpload to be called once, got %d", adapter.completeCalled.Load())
	}

	// 验证没有调用中止
	if adapter.abortCalled.Load() != 0 {
		t.Errorf("expected no AbortMultipartUpload calls, got %d", adapter.abortCalled.Load())
	}
}

// TestUploadInitFailure 测试初始化失败时的处理
func TestUploadInitFailure(t *testing.T) {
	adapter := &mockAdapter{}
	adapter.shouldFailInit = true
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	testData := []byte("test data")
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error for init failure, got nil")
	}

	// 验证不会尝试完成上传
	if adapter.completeCalled.Load() != 0 {
		t.Error("CompleteMultipartUpload should not be called when init fails")
	}
}

// TestUploadPartFailure 测试分块上传失败时的处理
func TestUploadPartFailure(t *testing.T) {
	adapter := &mockAdapter{}
	adapter.shouldFailPart = true
	adapter.partNumberToFail = 2
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 15MB 的测试数据（3 个分块）
	testData := make([]byte, 15*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error for part upload failure, got nil")
	}

	// 验证调用了中止上传
	if adapter.abortCalled.Load() == 0 {
		t.Error("AbortMultipartUpload should be called when part upload fails")
	}

	// 验证不会尝试完成上传
	if adapter.completeCalled.Load() != 0 {
		t.Error("CompleteMultipartUpload should not be called when part upload fails")
	}
}

// TestUploadCompleteFailure 测试完成上传失败时的处理
func TestUploadCompleteFailure(t *testing.T) {
	adapter := &mockAdapter{}
	adapter.shouldFailComplete = true
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 10MB 的测试数据（2 个分块）
	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error for complete failure, got nil")
	}

	// 验证调用了中止上传
	if adapter.abortCalled.Load() == 0 {
		t.Error("AbortMultipartUpload should be called when complete fails")
	}
}

// TestUploadContextCancellation 测试上下文取消
func TestUploadContextCancellation(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 20MB 的测试数据
	testData := make([]byte, 20*1024*1024)

	ctx, cancel := context.WithCancel(context.Background())

	// 在另一个 goroutine 中启动上传，然后立即取消
	done := make(chan error, 1)
	go func() {
		done <- u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	}()

	// 等待一小段时间然后取消
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

// TestUploadEmptyData 测试上传空数据
func TestUploadEmptyData(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	testData := []byte{}
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() with empty data failed: %v", err)
	}

	// 验证没有上传任何分块
	if adapter.uploadPartCalled.Load() != 0 {
		t.Errorf("expected no UploadPart calls for empty data, got %d", adapter.uploadPartCalled.Load())
	}
}

// TestUploadSmallData 测试上传小于一个分块的数据
func TestUploadSmallData(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	// 1MB 数据（小于 5MB 分块大小）
	testData := make([]byte, 1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 验证上传了 1 个分块
	if adapter.uploadPartCalled.Load() != 1 {
		t.Errorf("expected 1 UploadPart call, got %d", adapter.uploadPartCalled.Load())
	}
}

// TestSortParts 测试分块排序
func TestSortParts(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)

	parts := []storage.CompletedPart{
		{PartNumber: 3, ETag: "c"},
		{PartNumber: 1, ETag: "a"},
		{PartNumber: 4, ETag: "d"},
		{PartNumber: 2, ETag: "b"},
	}

	u.sortParts(parts)

	for i := 0; i < len(parts); i++ {
		if parts[i].PartNumber != i+1 {
			t.Errorf("expected part number %d at position %d, got %d", i+1, i, parts[i].PartNumber)
		}
	}
}

// TestSortPartsAlreadySorted 测试已排序的分块
func TestSortPartsAlreadySorted(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)

	parts := []storage.CompletedPart{
		{PartNumber: 1, ETag: "a"},
		{PartNumber: 2, ETag: "b"},
		{PartNumber: 3, ETag: "c"},
	}

	u.sortParts(parts)

	// 验证顺序不变
	if parts[0].PartNumber != 1 || parts[1].PartNumber != 2 || parts[2].PartNumber != 3 {
		t.Error("already sorted parts should remain in the same order")
	}
}

// TestSortPartsEmpty 测试空分块列表
func TestSortPartsEmpty(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)

	parts := []storage.CompletedPart{}

	// 不应该 panic
	u.sortParts(parts)

	if len(parts) != 0 {
		t.Error("empty parts list should remain empty")
	}
}

// TestSortPartsSingle 测试单个分块
func TestSortPartsSingle(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)

	parts := []storage.CompletedPart{
		{PartNumber: 1, ETag: "a"},
	}

	u.sortParts(parts)

	if len(parts) != 1 || parts[0].PartNumber != 1 {
		t.Error("single part should remain unchanged")
	}
}

// TestGetBuffer 测试缓冲池获取
func TestGetBuffer(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		minLen   int
		checkCap bool
	}{
		{"default size", 5 * 1024 * 1024, 5 * 1024 * 1024, true},
		{"small size", 1024, 1024, false},
		{"large size", 10 * 1024 * 1024, 10 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := getBuffer(tt.size)
			if len(buf) < tt.minLen {
				t.Errorf("expected buffer with at least %d bytes, got %d", tt.minLen, len(buf))
			}
			if tt.checkCap && cap(buf) != 5*1024*1024 {
				t.Errorf("expected buffer capacity %d, got %d", 5*1024*1024, cap(buf))
			}
		})
	}
}

// TestPutBuffer 测试缓冲池归还
func TestPutBuffer(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize int
		shouldPut  bool
	}{
		{"exact pool size", 5 * 1024 * 1024, true},
		{"small buffer", 1024, false},
		{"large buffer", 10 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := make([]byte, tt.bufferSize)
			// 不应该 panic
			putBuffer(buf)
		})
	}
}

// TestBufferPoolRoundTrip 测试缓冲池的循环使用
func TestBufferPoolRoundTrip(t *testing.T) {
	// 获取一个缓冲区
	buf1 := getBuffer(5 * 1024 * 1024)
	originalCap := cap(buf1)

	// 归还缓冲区
	putBuffer(buf1)

	// 再次获取，应该得到相同的缓冲区（从池中）
	buf2 := getBuffer(5 * 1024 * 1024)

	if cap(buf2) != originalCap {
		t.Errorf("expected to get buffer from pool with capacity %d, got %d", originalCap, cap(buf2))
	}

	putBuffer(buf2)
}

// TestUploadWithProgressReporter 测试带进度报告的上传
func TestUploadWithProgressReporter(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 2)

	reporter := progress.NewMockReporter()
	u.SetProgressReporter(reporter)

	// 创建 10MB 的测试数据
	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 验证进度报告器被调用
	if reporter.InitCalled.Load() == 0 {
		t.Error("Init should be called on progress reporter")
	}

	if reporter.CompleteCalled.Load() == 0 {
		t.Error("Complete should be called on progress reporter")
	}

	if reporter.CloseCalled.Load() == 0 {
		t.Error("Close should be called on progress reporter")
	}

	if reporter.AddCalled.Load() == 0 {
		t.Error("Add should be called at least once on progress reporter")
	}
}

// TestConcurrentUpload 测试并发上传
func TestConcurrentUpload(t *testing.T) {
	adapter := &mockAdapter{}
	defer adapter.reset()

	u := NewUploader(adapter, 5*1024*1024, 4) // 4 个并发 worker
	u.SetProgressReporter(progress.NewSilent())

	// 创建 25MB 的测试数据（5 个分块）
	testData := make([]byte, 25*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 验证所有分块都被上传
	if adapter.uploadPartCalled.Load() != 5 {
		t.Errorf("expected 5 UploadPart calls, got %d", adapter.uploadPartCalled.Load())
	}

	// 验证分块按正确顺序保存
	sort.Slice(adapter.uploadedParts, func(i, j int) bool {
		return adapter.uploadedParts[i].PartNumber < adapter.uploadedParts[j].PartNumber
	})

	for i, part := range adapter.uploadedParts {
		expectedPart := i + 1
		if part.PartNumber != expectedPart {
			t.Errorf("expected part number %d, got %d", expectedPart, part.PartNumber)
		}
	}
}
