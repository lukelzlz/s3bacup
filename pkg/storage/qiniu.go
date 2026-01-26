package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// QiniuAdapter 七牛云适配器
// 七牛云 Kodo 支持 S3 协议，但存储类型映射不同
type QiniuAdapter struct {
	client *s3.Client
	bucket string
}

// NewQiniuAdapter 创建七牛云适配器
func NewQiniuAdapter(ctx context.Context, endpoint, bucket, accessKey, secretKey string) (*QiniuAdapter, error) {
	// 七牛云 S3 协议端点格式: s3.<region>.qiniucs.com
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("qiniu"), // 七牛云使用自定义 region
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load Qiniu config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
		}
	})

	return &QiniuAdapter{
		client: client,
		bucket: bucket,
	}, nil
}

// InitMultipartUpload 初始化 Multipart Upload
func (q *QiniuAdapter) InitMultipartUpload(ctx context.Context, key string, opts UploadOptions) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(q.bucket),
		Key:    aws.String(key),
	}

	// 七牛云存储类型通过 x-qiniu-headers 设置
	if opts.StorageClass.IsValid() {
		// 七牛云存储类型映射到 x-qiniu-storage-class
		qiniuStorageClass := q.mapStorageClass(opts.StorageClass)
		input.Metadata = map[string]string{
			"x-qiniu-storage-class": qiniuStorageClass,
		}
	}
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}
	if len(opts.Metadata) > 0 {
		if input.Metadata == nil {
			input.Metadata = make(map[string]string)
		}
		for k, v := range opts.Metadata {
			input.Metadata[k] = v
		}
	}

	result, err := q.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create multipart upload: %w", err)
	}

	return *result.UploadId, nil
}

// UploadPart 上传分块
func (q *QiniuAdapter) UploadPart(ctx context.Context, key, uploadID string, partNum int, data io.Reader, size int64) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(q.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(int32(partNum)),
		Body:       data,
	}

	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}

	result, err := q.client.UploadPart(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload part %d: %w", partNum, err)
	}

	return *result.ETag, nil
}

// CompleteMultipartUpload 完成上传
func (q *QiniuAdapter) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	completedParts := make([]types.CompletedPart, len(parts))
	for i, p := range parts {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(p.ETag),
			PartNumber: aws.Int32(int32(p.PartNumber)),
		}
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(q.bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completedParts},
	}

	_, err := q.client.CompleteMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

// AbortMultipartUpload 取消上传
func (q *QiniuAdapter) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(q.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := q.client.AbortMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// SupportedStorageClasses 返回支持的存储类型
func (q *QiniuAdapter) SupportedStorageClasses() []StorageClass {
	return []StorageClass{
		StorageClassStandard,
		StorageClassInfrequent,
		StorageClassArchive,
		StorageClassDeepArchive,
	}
}

// SetStorageClass 设置存储类型
// 七牛云通过 chtype API 修改存储类型，这里使用 CopyObject 模拟
func (q *QiniuAdapter) SetStorageClass(ctx context.Context, key string, class StorageClass) error {
	copySource := fmt.Sprintf("%s/%s", q.bucket, key)
	qiniuStorageClass := q.mapStorageClass(class)

	input := &s3.CopyObjectInput{
		Bucket:            aws.String(q.bucket),
		CopySource:        aws.String(copySource),
		Key:               aws.String(key),
		Metadata:          map[string]string{"x-qiniu-storage-class": qiniuStorageClass},
		MetadataDirective: types.MetadataDirectiveReplace,
	}

	_, err := q.client.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set storage class: %w", err)
	}

	return nil
}

// mapStorageClass 将通用存储类型映射到七牛云的存储类型值
// 七牛云存储类型: 0=标准, 1=低频, 2=归档, 3=深度归档, 4=归档直读, 5=智能分层
func (q *QiniuAdapter) mapStorageClass(sc StorageClass) string {
	switch sc {
	case StorageClassStandard:
		return "0" // 标准存储
	case StorageClassInfrequent:
		return "1" // 低频存储
	case StorageClassArchive:
		return "2" // 归档存储
	case StorageClassDeepArchive:
		return "3" // 深度归档
	case StorageClassGlacierIR:
		return "4" // 归档直读
	case StorageClassIntelligentTiering:
		return "5" // 智能分层
	default:
		return "0"
	}
}
