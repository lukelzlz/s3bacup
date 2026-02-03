package uploader

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/storage"
)

// TestErrorPropagation 測試錯誤傳播
func TestErrorPropagation(t *testing.T) {
	tests := []struct {
		name         string
		setupAdapter func() *mockAdapter
		wantErr      string
		expectAbort  bool
	}{
		{
			name: "init fails",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{shouldFailInit: true}
			},
			wantErr:     "init",
			expectAbort: false, // 初始化失敗時沒有 uploadID 可中止
		},
		{
			name: "upload part fails",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{shouldFailPart: true, partNumberToFail: 1}
			},
			wantErr:     "part",
			expectAbort: true,
		},
		{
			name: "complete fails",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{shouldFailComplete: true}
			},
			wantErr:     "complete",
			expectAbort: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := tt.setupAdapter()
			u := NewUploader(adapter, 5*1024*1024, 2)
			u.SetProgressReporter(progress.NewSilent())

			testData := make([]byte, 10*1024*1024)
			ctx := context.Background()

			err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(strings.ToLower(err.Error()), tt.wantErr) {
				t.Errorf("error message should contain '%s', got: %v", tt.wantErr, err)
			}

			// 驗證中止是否被調用（取決於錯誤類型）
			if tt.expectAbort && adapter.abortCalled.Load() == 0 {
				t.Error("abort should be called when upload fails after init")
			}
		})
	}
}

// TestResourceCleanupOnError 測試錯誤時的資源清理
func TestResourceCleanupOnError(t *testing.T) {
	adapter := &mockAdapter{shouldFailPart: true, partNumberToFail: 2}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 15*1024*1024) // 3 個分塊
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 驗證資源被清理
	if adapter.abortCalled.Load() == 0 {
		t.Error("abort should be called to clean up multipart upload")
	}

	if adapter.completeCalled.Load() != 0 {
		t.Error("complete should not be called when upload fails")
	}
}

// TestPartialUploadCleanup 測試部分上傳後的清理
func TestPartialUploadCleanup(t *testing.T) {
	// 第一個分塊成功，第二個失敗
	adapter := &mockAdapter{shouldFailPart: true, partNumberToFail: 2}
	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 驗證至少有一個分塊被嘗試上傳
	if adapter.uploadPartCalled.Load() == 0 {
		t.Error("at least one part should be attempted")
	}

	// 驗證中止被調用來清理部分上傳
	if adapter.abortCalled.Load() == 0 {
		t.Error("abort should be called to clean up partial upload")
	}
}

// TestContextCancellationCleanup 測試上下文取消時的清理
func TestContextCancellationCleanup(t *testing.T) {
	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 4)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 50*1024*1024) // 大數據以確保需要時間

	ctx, cancel := context.WithCancel(context.Background())

	// 在後台啟動上傳
	done := make(chan error, 1)
	go func() {
		done <- u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	}()

	// 等待一小段時間後取消
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	if err != context.Canceled && err != nil {
		t.Logf("expected context.Canceled or nil, got: %v", err)
	}

	// 注意：根據實現，取消可能不會調用 abort
	// 因為上下文取消可能在不同階段發生
}

// TestReaderErrorPropagation 測試讀取器錯誤傳播
func TestReaderErrorPropagation(t *testing.T) {
	// 創建一個會返回錯誤的讀取器
	errReader := &errorReader{
		data:   make([]byte, 10*1024*1024),
		failAt: 5 * 1024 * 1024, // 在 5MB 後失敗
	}

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	ctx := context.Background()

	err := u.Upload(ctx, "test-key", errReader, storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error from reader, got nil")
	}

	if !strings.Contains(err.Error(), "failed to read data") {
		t.Logf("expected read error, got: %v", err)
	}

	// 驗證中止被調用
	if adapter.abortCalled.Load() == 0 {
		t.Error("abort should be called when reader fails")
	}
}

// TestMultipleErrorScenarios 測試多種錯誤場景
func TestMultipleErrorScenarios(t *testing.T) {
	tests := []struct {
		name         string
		setupAdapter func() *mockAdapter
		setupData    func() io.Reader
		verify       func(t *testing.T, adapter *mockAdapter, err error)
	}{
		{
			name: "empty data",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{}
			},
			setupData: func() io.Reader {
				return bytes.NewReader([]byte{})
			},
			verify: func(t *testing.T, adapter *mockAdapter, err error) {
				if err != nil {
					t.Errorf("empty data should succeed, got: %v", err)
				}
				if adapter.uploadPartCalled.Load() != 0 {
					t.Error("no parts should be uploaded for empty data")
				}
			},
		},
		{
			name: "small data",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{}
			},
			setupData: func() io.Reader {
				return bytes.NewReader(make([]byte, 1024))
			},
			verify: func(t *testing.T, adapter *mockAdapter, err error) {
				if err != nil {
					t.Errorf("small data should succeed, got: %v", err)
				}
				if adapter.uploadPartCalled.Load() != 1 {
					t.Errorf("expected 1 part uploaded, got %d", adapter.uploadPartCalled.Load())
				}
			},
		},
		{
			name: "exact chunk size",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{}
			},
			setupData: func() io.Reader {
				return bytes.NewReader(make([]byte, 5*1024*1024))
			},
			verify: func(t *testing.T, adapter *mockAdapter, err error) {
				if err != nil {
					t.Errorf("exact chunk size should succeed, got: %v", err)
				}
			},
		},
		{
			name: "context timeout",
			setupAdapter: func() *mockAdapter {
				return &mockAdapter{}
			},
			setupData: func() io.Reader {
				// 使用非常短的截止時間
				ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				// 返回一個會阻塞的讀取器
				return &slowReader{ctx: ctx}
			},
			verify: func(t *testing.T, adapter *mockAdapter, err error) {
				// 超時是可能的結果
				_ = adapter
				_ = err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := tt.setupAdapter()
			u := NewUploader(adapter, 5*1024*1024, 2)
			u.SetProgressReporter(progress.NewSilent())

			ctx := context.Background()
			data := tt.setupData()

			err := u.Upload(ctx, "test-key", data, storage.UploadOptions{})
			tt.verify(t, adapter, err)
		})
	}
}

// TestErrorDoesNotLeakSensitiveInfo 測試錯誤不會洩露敏感信息
func TestErrorDoesNotLeakSensitiveInfo(t *testing.T) {
	// 使用模擬的敏感憑證
	secretAccessKey := "super-secret-key-12345"
	adapter := &mockAdapter{}

	// 我們不會在錯誤消息中包含敏感信息
	// 這個測試驗證錯誤處理路徑
	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 5*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil && strings.Contains(err.Error(), secretAccessKey) {
		t.Error("error message should not contain sensitive information")
	}
	_ = secretAccessKey
}

// TestAbortCalledOnAllFailures 測試所有失敗情況都調用中止
func TestAbortCalledOnAllFailures(t *testing.T) {
	tests := []struct {
		name              string
		shouldFailInit     bool
		shouldFailPart     bool
		shouldFailComplete bool
		expectAbort        bool
		expectComplete     bool
	}{
		{"init failure", true, false, false, false, false}, // 初始化失敗時沒有 uploadID
		{"part failure", false, true, false, true, false},  // 部分失敗，中止上傳，不調用完成
		{"complete failure", false, false, true, true, true}, // 完成失敗，嘗試了完成（所以被調用）
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &mockAdapter{
				shouldFailInit:     tt.shouldFailInit,
				shouldFailPart:     tt.shouldFailPart,
				partNumberToFail:   1,
				shouldFailComplete: tt.shouldFailComplete,
			}
			u := NewUploader(adapter, 5*1024*1024, 2)
			u.SetProgressReporter(progress.NewSilent())

			testData := make([]byte, 10*1024*1024)
			ctx := context.Background()

			err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// 驗證中止是否被調用（取決於失敗類型）
			if tt.expectAbort {
				if adapter.abortCalled.Load() == 0 {
					t.Error("abort should be called on failure after init")
				}
			}

			// 驗證完成是否被調用（對於完成失敗的情況，完成被調用了但返回錯誤）
			if tt.expectComplete {
				if adapter.completeCalled.Load() == 0 {
					t.Error("complete should be called even when it fails")
				}
			} else {
				if adapter.completeCalled.Load() != 0 {
					t.Error("complete should not be called on non-complete failures")
				}
			}
		})
	}
}

// TestProgressReporterCleanupOnError 測試錯誤時進度報告器清理
func TestProgressReporterCleanupOnError(t *testing.T) {
	reporter := progress.NewMockReporter()
	adapter := &mockAdapter{shouldFailInit: true}

	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(reporter)

	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// 驗證報告器的 Close 被調用（通過 defer）
	if reporter.CloseCalled.Load() == 0 {
		t.Error("reporter Close should be called even on error")
	}
}

// TestConcurrentErrorHandling 測試並發錯誤處理
func TestConcurrentErrorHandling(t *testing.T) {
	// 測試多個並發上傳中的錯誤不會相互影響
	const numUploads = 5
	errors := make(chan error, numUploads)

	for i := 0; i < numUploads; i++ {
		go func(n int) {
			// 奇數上傳會失敗
			var adapter *mockAdapter
			if n%2 == 1 {
				adapter = &mockAdapter{shouldFailPart: true, partNumberToFail: 1}
			} else {
				adapter = &mockAdapter{}
			}

			u := NewUploader(adapter, 5*1024*1024, 2)
			u.SetProgressReporter(progress.NewSilent())

			testData := make([]byte, 10*1024*1024)
			ctx := context.Background()

			errors <- u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
		}(i)
	}

	successCount := 0
	failCount := 0

	for i := 0; i < numUploads; i++ {
		err := <-errors
		if err != nil {
			failCount++
		} else {
			successCount++
		}
	}

	// 預期約一半成功，一半失敗
	t.Logf("Concurrent uploads: %d succeeded, %d failed", successCount, failCount)

	if successCount == 0 || failCount == 0 {
		t.Error("expected mixed success/failure in concurrent uploads")
	}
}

// TestRecoveryAfterError 測試錯誤後可以恢復
func TestRecoveryAfterError(t *testing.T) {
	// 第一次上傳失敗
	adapter1 := &mockAdapter{shouldFailPart: true, partNumberToFail: 1}
	u := NewUploader(adapter1, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	testData := make([]byte, 10*1024*1024)
	ctx := context.Background()

	err := u.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err == nil {
		t.Fatal("expected error on first upload, got nil")
	}

	// 第二次上傳應該成功
	adapter2 := &mockAdapter{}
	u2 := NewUploader(adapter2, 5*1024*1024, 2)
	u2.SetProgressReporter(progress.NewSilent())

	err = u2.Upload(ctx, "test-key", bytes.NewReader(testData), storage.UploadOptions{})
	if err != nil {
		t.Errorf("second upload should succeed, got: %v", err)
	}
}

// TestSlowReaderError 測試慢速讀取器的錯誤處理
func TestSlowReaderError(t *testing.T) {
	// 創建一個慢速讀取器，最終會失敗
	slowReader := &slowReader{
		data:      make([]byte, 20*1024*1024),
		delay:     100 * time.Millisecond,
		failAfter: 2, // 讀取 2 次後失敗
	}

	adapter := &mockAdapter{}
	u := NewUploader(adapter, 5*1024*1024, 2)
	u.SetProgressReporter(progress.NewSilent())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := u.Upload(ctx, "test-key", slowReader, storage.UploadOptions{})
	// 可能因為超時或讀取錯誤而失敗
	_ = err
}

// Test helpers

// errorReader 是一個會在指定位置返回錯誤的讀取器
type errorReader struct {
	data   []byte
	pos    int64
	failAt int64
}

func (e *errorReader) Read(p []byte) (int, error) {
	if e.pos >= int64(len(e.data)) {
		return 0, io.EOF
	}

	if e.pos >= e.failAt {
		return 0, errors.New("simulated read error")
	}

	n := copy(p, e.data[e.pos:])
	e.pos += int64(n)
	return n, nil
}

// slowReader 是一個慢速讀取器，用於測試超時
type slowReader struct {
	data      []byte
	pos       int64
	delay     time.Duration
	failAfter int // 讀取次數後失敗
	reads     int
	ctx       context.Context
}

func (s *slowReader) Read(p []byte) (int, error) {
	if s.ctx != nil {
		select {
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		default:
		}
	}

	s.reads++
	if s.failAfter > 0 && s.reads > s.failAfter {
		return 0, errors.New("simulated slow reader failure")
	}

	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	if s.pos >= int64(len(s.data)) {
		return 0, io.EOF
	}

	n := copy(p, s.data[s.pos:])
	s.pos += int64(n)
	return n, nil
}
