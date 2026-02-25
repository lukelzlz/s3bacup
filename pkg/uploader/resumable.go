package uploader

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/state"
	"github.com/lukelzlz/s3backup/pkg/storage"
)

// ResumableUploader 支持断点续传的上传器
type ResumableUploader struct {
	adapter     storage.StorageAdapter
	chunkSize   int64
	concurrency int
	reporter    progress.Reporter
	uploaded    atomic.Int64
	savedState  *state.UploadState
	stateMgr    *state.StateManager
}

// NewResumableUploader 创建支持断点续传的上传器
func NewResumableUploader(adapter storage.StorageAdapter, chunkSize int64, concurrency int, savedState *state.UploadState) *ResumableUploader {
	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024
	}
	if concurrency <= 0 {
		concurrency = 4
	}

	return &ResumableUploader{
		adapter:     adapter,
		chunkSize:   chunkSize,
		concurrency: concurrency,
		reporter:    progress.NewSilent(),
		savedState:  savedState,
	}
}

// SetProgressReporter 设置进度报告器
func (u *ResumableUploader) SetProgressReporter(r progress.Reporter) {
	u.reporter = r
}

// SetStateManager 设置状态管理器
func (u *ResumableUploader) SetStateManager(sm *state.StateManager) {
	u.stateMgr = sm
}

// Upload 从 reader 读取数据并上传（支持断点续传）
func (u *ResumableUploader) Upload(ctx context.Context, key string, r io.Reader, opts storage.UploadOptions) (err error) {
	// 检查是否有已保存的状态
	if u.savedState != nil && u.savedState.UploadID != "" {
		return u.Resume(ctx, key, u.savedState.UploadID, r, opts)
	}

	// 新上传，使用普通上传器
	upl := NewUploader(u.adapter, u.chunkSize, u.concurrency)
	upl.SetProgressReporter(u.reporter)
	return upl.Upload(ctx, key, r, opts)
}

// Resume 从断点恢复上传
func (u *ResumableUploader) Resume(ctx context.Context, key string, uploadID string, r io.Reader, opts storage.UploadOptions) (err error) {
	// 初始化进度报告
	u.reporter.Init(0)

	// 确保在出错时清理资源
	defer func() {
		if err != nil {
			_ = u.reporter.Close()
		}
	}()

	// 获取已完成的分块
	completedParts := make(map[int]state.CompletedPart)
	if u.savedState != nil {
		for _, p := range u.savedState.Completed {
			completedParts[p.PartNumber] = p
		}
		// 更新进度
		u.reporter.Add(u.savedState.UploadedBytes)
	}

	// 创建分块通道
	chunkChan := make(chan *chunk, u.concurrency*2)
	resultChan := make(chan *partResult, u.concurrency)
	errorChan := make(chan error, 1)

	// 用于跟踪读取是否完成
	readDone := make(chan struct{})

	// 启动 worker goroutines
	var wg sync.WaitGroup
	for i := 0; i < u.concurrency; i++ {
		wg.Add(1)
		go u.worker(ctx, &wg, key, uploadID, chunkChan, resultChan, errorChan, completedParts)
	}

	// 读取数据并发送分块
	go func() {
		u.readChunks(ctx, r, chunkChan, errorChan)
		close(readDone)
	}()

	// 收集结果
	var parts []storage.CompletedPart

	// 添加已完成的分块
	for _, p := range completedParts {
		parts = append(parts, storage.CompletedPart{
			PartNumber: p.PartNumber,
			ETag:       p.ETag,
		})
	}

	// 等待所有 worker 完成和结果收集
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 处理结果和错误
	for {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			return err

		case result, ok := <-resultChan:
			if !ok {
				// resultChan 已关闭，所有 worker 完成
				goto complete
			}
			parts = append(parts, storage.CompletedPart{
				PartNumber: result.partNumber,
				ETag:       result.etag,
			})

		case uploadErr := <-errorChan:
			// 有错误发生
			err = uploadErr
			return uploadErr

		case <-readDone:
			// 读取完成，继续等待 resultChan 关闭
		}
	}

complete:
	// 按分块号排序
	u.sortParts(parts)

	// 完成上传
	if completeErr := u.adapter.CompleteMultipartUpload(ctx, key, uploadID, parts); completeErr != nil {
		err = fmt.Errorf("failed to complete multipart upload: %w", completeErr)
		return err
	}

	u.reporter.Complete()
	_ = u.reporter.Close()

	return nil
}

// worker 处理分块上传（支持跳过已完成的分块）
func (u *ResumableUploader) worker(ctx context.Context, wg *sync.WaitGroup, key, uploadID string,
	chunkChan <-chan *chunk, resultChan chan<- *partResult, errorChan chan<- error,
	completedParts map[int]state.CompletedPart) {

	defer wg.Done()

	for chunk := range chunkChan {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 检查该分块是否已完成
		if completed, ok := completedParts[chunk.partNumber]; ok {
			// 跳过已完成的分块
			resultChan <- &partResult{
				partNumber: completed.PartNumber,
				etag:       completed.ETag,
			}
			putBuffer(chunk.data)
			continue
		}

		// 上传分块
		etag, err := u.adapter.UploadPart(ctx, key, uploadID, chunk.partNumber, bytes.NewReader(chunk.data), chunk.size)
		if err != nil {
			errorChan <- fmt.Errorf("failed to upload part %d: %w", chunk.partNumber, err)
			return
		}

		// 更新进度
		u.reporter.Add(chunk.size)

		resultChan <- &partResult{
			partNumber: chunk.partNumber,
			etag:       etag,
		}

		// 保存状态
		if u.stateMgr != nil {
			u.stateMgr.AddCompletedPart(state.CompletedPart{
				PartNumber: chunk.partNumber,
				ETag:       etag,
				Size:       chunk.size,
			})
		}

		// 回收缓冲区
		putBuffer(chunk.data)
	}
}

// readChunks 读取数据并发送分块
func (u *ResumableUploader) readChunks(ctx context.Context, r io.Reader, chunkChan chan<- *chunk, errorChan chan<- error) {
	defer close(chunkChan)

	partNumber := 1

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 获取缓冲区
		buf := getBuffer(u.chunkSize)

		// 读取数据
		n, err := io.ReadFull(r, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			putBuffer(buf)
			errorChan <- fmt.Errorf("failed to read data: %w", err)
			return
		}

		if n == 0 {
			putBuffer(buf)
			return
		}

		// 发送分块
		chunkChan <- &chunk{
			partNumber: partNumber,
			data:       buf[:n],
			size:       int64(n),
		}

		partNumber++
	}
}

// sortParts 按分块号排序
func (u *ResumableUploader) sortParts(parts []storage.CompletedPart) {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
}
