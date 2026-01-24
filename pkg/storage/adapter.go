package storage

import (
	"context"
	"io"
)

// StorageAdapter 定义存储适配器接口
type StorageAdapter interface {
	// 初始化 Multipart Upload
	InitMultipartUpload(ctx context.Context, key string, opts UploadOptions) (uploadID string, err error)

	// 上传分块
	UploadPart(ctx context.Context, key, uploadID string, partNum int, data io.Reader, size int64) (etag string, err error)

	// 完成上传
	CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error

	// 取消上传
	AbortMultipartUpload(ctx context.Context, key, uploadID string) error

	// 获取支持的存储类型
	SupportedStorageClasses() []StorageClass

	// 设置存储类型（部分服务需要上传后修改）
	SetStorageClass(ctx context.Context, key string, class StorageClass) error
}

// UploadOptions 上传选项
type UploadOptions struct {
	StorageClass StorageClass
	ContentType  string
	Metadata     map[string]string
}

// CompletedPart 已完成的分块信息
type CompletedPart struct {
	PartNumber int
	ETag       string
}
