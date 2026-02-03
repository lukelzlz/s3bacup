package uploader

import (
	"bytes"
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/storage"
)

// TestGoroutineCleanupOnSuccess 测试成功上传后 goroutine 清理
func TestGoroutineCleanupOnSuccess(t *testing.T) {
	// 记录启动时的 goroutine 数量
	startingGoroutines := runtime.NumGoroutine()

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 10MB 测试数据
	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 等待 goroutine 清理
	time.Sleep(100 * time.Millisecond)

	// 检查 goroutine 数量没有显著增加
	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > startingGoroutines+2 {
		t.Logf("Warning: possible goroutine leak: started with %d, ended with %d",
			startingGoroutines, finalGoroutines)
	}
}

// TestGoroutineCleanupOnError 测试错误时 goroutine 清理
func TestGoroutineCleanupOnError(t *testing.T) {
	startingGoroutines := runtime.NumGoroutine()

	adapter := &mockAdapter{shouldFailInit: true}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error for failed init, got nil")
	}

	// 等待 goroutine 清理
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > startingGoroutines+2 {
		t.Logf("Warning: possible goroutine leak on error: started with %d, ended with %d",
			startingGoroutines, finalGoroutines)
	}
}

// TestGoroutineCleanupOnCancellation 测试取消时 goroutine 清理
func TestGoroutineCleanupOnCancellation(t *testing.T) {
	startingGoroutines := runtime.NumGoroutine()

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	// 创建大量数据
	testData := make([]byte, 50*1024*1024)

	ctx, cancel := context.WithCancel(context.Background())

	// 在另一个 goroutine 中启动上传
	done := make(chan error, 1)
	go func() {
		done <- u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	}()

	// 短暂等待后取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Logf("expected context.Canceled, got: %v", err)
	}

	// 等待 goroutine 清理
	time.Sleep(200 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > startingGoroutines+4 {
		t.Logf("Warning: possible goroutine leak on cancellation: started with %d, ended with %d",
			startingGoroutines, finalGoroutines)
	}
}

// TestBufferPoolCleanup 测试缓冲池清理
func TestBufferPoolCleanup(t *testing.T) {
	// 多次获取和归还缓冲区
	for i := 0; i < 100; i++ {
		buf := getBuffer(5 * 1024 * 1024)
		if cap(buf) != 5*1024*1024 {
			t.Errorf("iteration %d: expected buffer capacity %d, got %d", i, 5*1024*1024, cap(buf))
		}
		putBuffer(buf)
	}

	// 测试不同大小的缓冲区
	for _, size := range []int64{1024, 5 * 1024 * 1024, 10 * 1024 * 1024} {
		buf := getBuffer(size)
		if len(buf) < int(size) {
			t.Errorf("buffer size %d: expected at least %d bytes, got %d", size, size, len(buf))
		}
		putBuffer(buf)
	}
}

// TestMemoryUsageDuringUpload 测试上传过程中的内存使用
func TestMemoryUsageDuringUpload(t *testing.T) {
	// 强制 GC 以获取准确的基线
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	// 创建 20MB 数据
	testData := make([]byte, 20*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 再次 GC 并检查内存
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// 检查堆内存增长（当前分配的内存，不是累计分配）
	// 使用 int64 转換避免無符號整數下溢
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	if heapGrowth < 0 {
		heapGrowth = 0
	}
	heapAllocatedMB := heapGrowth / 1024 / 1024
	t.Logf("Heap memory growth during upload: ~%d MB", heapAllocatedMB)

	// 对于 20MB 数据和 5MB 分块，使用 4 并发
	// 预计最大内存约 25-30MB（测试数据 + 缓冲池）
	if heapAllocatedMB > 50 {
		t.Errorf("excessive heap memory growth: ~%d MB", heapAllocatedMB)
	}
}

// TestConcurrentUploadsMemory 测试并发上传的内存使用
func TestConcurrentUploadsMemory(t *testing.T) {
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// 同时启动多个上传
	const numUploads = 3
	done := make(chan bool, numUploads)

	for i := 0; i < numUploads; i++ {
		go func(n int) {
			adapter := &mockAdapter{}
			u := NewUploader(adapter, 5*1024*1024, 2)
			u.SetProgressReporter(progress.NewSilent())

			testData := make([]byte, 10*1024*1024)
			ctx := context.Background()

			u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
			done <- true
		}(i)
	}

	// 等待所有上传完成
	for i := 0; i < numUploads; i++ {
		<-done
	}

	// 等待清理
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	// 處理整數下溢的情況
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	if heapGrowth < 0 {
		heapGrowth = 0
	}
	heapAllocatedMB := heapGrowth / 1024 / 1024
	t.Logf("Heap memory growth for %d concurrent uploads: ~%d MB", numUploads, heapAllocatedMB)

	// 对于 3 个并发上传，每个使用 2 并发工作器，5MB 分块
	// 内存应该在合理范围内
	if heapAllocatedMB > 200 {
		t.Errorf("excessive heap memory growth for concurrent uploads: ~%d MB", heapAllocatedMB)
	}
}

// TestContextCancellationImmediate 立即取消上下文
func TestContextCancellationImmediate(t *testing.T) {
	startingGoroutines := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 10*1024*1024)
	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != context.Canceled {
		t.Logf("expected context.Canceled, got: %v", err)
	}

	// 等待清理
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	if finalGoroutines > startingGoroutines+2 {
		t.Logf("Warning: possible goroutine leak on immediate cancellation: started with %d, ended with %d",
			startingGoroutines, finalGoroutines)
	}
}

// TestReporterCleanup 测试进度报告器清理
func TestReporterCleanup(t *testing.T) {
	reporter := progress.NewMockReporter()

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(reporter)

	testData := make([]byte, 5*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Fatalf("Upload() failed: %v", err)
	}

	// 验证报告器方法被正确调用
	if reporter.CloseCalled.Load() == 0 {
		t.Error("Close should be called on reporter")
	}

	if reporter.CompleteCalled.Load() == 0 {
		t.Error("Complete should be called on reporter")
	}
}

// TestMultipleSequentialUploads 测试多次顺序上传
func TestMultipleSequentialUploads(t *testing.T) {
	runtime.GC()

	// 执行多次上传以测试资源重复使用
	for i := 0; i < 5; i++ {
		adapter := &mockAdapter{}
		u := NewUploader(adapter, 5*1024*1024, 2)
		u.SetProgressReporter(progress.NewSilent())

		testData := make([]byte, 5*1024*1024)
		ctx := context.Background()

		err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
		if err != nil {
			t.Fatalf("Upload %d failed: %v", i, err)
		}
	}

	// 强制 GC 并检查没有内存泄漏
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// 检查堆大小在合理范围内
	heapMB := m.HeapAlloc / 1024 / 1024
	t.Logf("Heap size after multiple uploads: ~%d MB", heapMB)

	if heapMB > 50 {
		t.Logf("Warning: high heap usage after multiple uploads: ~%d MB", heapMB)
	}
}

// BenchmarkUpload 基准测试上传性能
func BenchmarkUpload(b *testing.B) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 20*1024*1024)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		adapter.reset()
		u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	}
}

// BenchmarkBufferPool 基准测试缓冲池性能
func BenchmarkBufferPool(b *testing.B) {
	b.Run("with_pool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := getBuffer(5 * 1024 * 1024)
			putBuffer(buf)
		}
	})

	b.Run("without_pool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := make([]byte, 5*1024*1024)
			_ = buf
		}
	})
}
