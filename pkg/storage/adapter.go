package storage

import (
	"context"
	"errors"
	"io"
	"strings"
)

// Mock 错误类型，用于测试
var (
	ErrMockInitFailed       = errors.New("mock: init multipart upload failed")
	ErrMockUploadPartFailed = errors.New("mock: upload part failed")
	ErrMockCompleteFailed   = errors.New("mock: complete multipart upload failed")
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

// normalizeEndpoint 规范化端点格式，确保包含协议前缀
// 如果端点不包含 http:// 或 https://，则自动添加 https://
func normalizeEndpoint(endpoint string) string {
	if endpoint == "" {
		return endpoint
	}
	endpoint = strings.TrimSpace(endpoint)
	if !strings.HasPrefix(strings.ToLower(endpoint), "http://") &&
		!strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return "https://" + endpoint
	}
	return endpoint
}
