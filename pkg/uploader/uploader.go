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

// Uploader 上传管理器
type Uploader struct {
	adapter     storage.StorageAdapter
	chunkSize   int64
	concurrency int
	reporter    progress.Reporter
	uploaded    atomic.Int64
	stateMgr    *state.StateManager
}

// NewUploader 创建上传管理器
func NewUploader(adapter storage.StorageAdapter, chunkSize int64, concurrency int) *Uploader {
	if chunkSize <= 0 {
		chunkSize = 5 * 1024 * 1024 // 默认 5MB
	}
	if concurrency <= 0 {
		concurrency = 4 // 默认并发数
	}

	return &Uploader{
		adapter:     adapter,
		chunkSize:   chunkSize,
		concurrency: concurrency,
		reporter:    progress.NewSilent(),
	}
}

// SetProgressReporter 设置进度报告器
func (u *Uploader) SetProgressReporter(r progress.Reporter) {
	u.reporter = r
}

// SetStateManager 设置状态管理器
func (u *Uploader) SetStateManager(sm *state.StateManager) {
	u.stateMgr = sm
}

// Upload 从 reader 读取数据并上传
func (u *Uploader) Upload(ctx context.Context, key string, r io.Reader, opts storage.UploadOptions) (err error) {
	// 初始化进度报告
	u.reporter.Init(0)

	// 确保在出错时清理资源（包括进度报告器）
	defer func() {
		if err != nil {
			_ = u.reporter.Close()
		}
	}()

	// 初始化 Multipart Upload
	uploadID, initErr := u.adapter.InitMultipartUpload(ctx, key, opts)
	if initErr != nil {
		return fmt.Errorf("failed to init multipart upload: %w", initErr)
	}

	// 保存 UploadID 到状态文件
	if u.stateMgr != nil {
		initialState := &state.UploadState{
			Key:          key,
			UploadID:     uploadID,
			StorageClass: string(opts.StorageClass),
			Encrypted:    false, // 由调用者设置
			Completed:    []state.CompletedPart{},
		}
		u.stateMgr.Save(initialState)
	}

	// 确保在出错时取消上传
	// 使用命名返回值 err，确保任何返回路径都会触发清理
	defer func() {
		if err != nil {
			_ = u.adapter.AbortMultipartUpload(ctx, key, uploadID)
		}
	}()

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
		go u.worker(ctx, &wg, key, uploadID, chunkChan, resultChan, errorChan)
	}

	// 读取数据并发送分块
	go func() {
		u.readChunks(ctx, r, chunkChan, errorChan)
		close(readDone)
	}()

	// 收集结果
	var parts []storage.CompletedPart

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
			// 读取完成，但可能还有结果在路上
			// 继续等待 resultChan 关闭
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

// worker 处理分块上传
func (u *Uploader) worker(ctx context.Context, wg *sync.WaitGroup, key, uploadID string,
	chunkChan <-chan *chunk, resultChan chan<- *partResult, errorChan chan<- error) {

	defer wg.Done()

	for chunk := range chunkChan {
		select {
		case <-ctx.Done():
			return
		default:
		}

		etag, err := u.adapter.UploadPart(ctx, key, uploadID, chunk.partNumber, bytes.NewReader(chunk.data), chunk.size)
		if err != nil {
			errorChan <- fmt.Errorf("failed to upload part %d: %w", chunk.partNumber, err)
			return
		}

		// 更新进度
		u.reporter.Add(chunk.size)

		// 保存状态（用于断点续传）
		if u.stateMgr != nil {
			u.stateMgr.AddCompletedPart(state.CompletedPart{
				PartNumber: chunk.partNumber,
				ETag:       etag,
				Size:       chunk.size,
			})
		}

		resultChan <- &partResult{
			partNumber: chunk.partNumber,
			etag:       etag,
		}

		// 回收缓冲区
		putBuffer(chunk.data)
	}
}

// readChunks 读取数据并发送分块
func (u *Uploader) readChunks(ctx context.Context, r io.Reader, chunkChan chan<- *chunk, errorChan chan<- error) {
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
func (u *Uploader) sortParts(parts []storage.CompletedPart) {
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
}

// chunk 数据分块
type chunk struct {
	partNumber int
	data       []byte
	size       int64
}

// partResult 分块上传结果
type partResult struct {
	partNumber int
	etag       string
}

// 缓冲池
var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 5*1024*1024) // 5MB
	},
}

// getBuffer 从池中获取缓冲区
func getBuffer(size int64) []byte {
	buf, ok := bufferPool.Get().([]byte)
	if !ok || int64(len(buf)) < size {
		// 如果类型断言失败或缓冲区太小，创建新的
		return make([]byte, size)
	}
	return buf
}

// putBuffer 将缓冲区放回池中
func putBuffer(buf []byte) {
	if cap(buf) == 5*1024*1024 {
		bufferPool.Put(buf)
	}
}
