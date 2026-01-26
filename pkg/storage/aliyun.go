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

// AliyunAdapter 阿里云 OSS 适配器
// 阿里云 OSS 支持 S3 协议，但存储类型映射不同
type AliyunAdapter struct {
	client *s3.Client
	bucket string
}

// NewAliyunAdapter 创建阿里云 OSS 适配器
func NewAliyunAdapter(ctx context.Context, region, endpoint, bucket, accessKey, secretKey string) (*AliyunAdapter, error) {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
			}, nil
		})),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load Aliyun config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
		}
	})

	return &AliyunAdapter{
		client: client,
		bucket: bucket,
	}, nil
}

// InitMultipartUpload 初始化 Multipart Upload
func (a *AliyunAdapter) InitMultipartUpload(ctx context.Context, key string, opts UploadOptions) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}

	// 阿里云存储类型通过 x-oss-storage-class header 设置
	if opts.StorageClass.IsValid() {
		aliyunStorageClass := a.mapStorageClass(opts.StorageClass)
		input.Metadata = map[string]string{
			"x-oss-storage-class": aliyunStorageClass,
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

	result, err := a.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create multipart upload: %w", err)
	}

	return *result.UploadId, nil
}

// UploadPart 上传分块
func (a *AliyunAdapter) UploadPart(ctx context.Context, key, uploadID string, partNum int, data io.Reader, size int64) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(a.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(int32(partNum)),
		Body:       data,
	}

	if size > 0 {
		input.ContentLength = aws.Int64(size)
	}

	result, err := a.client.UploadPart(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to upload part %d: %w", partNum, err)
	}

	return *result.ETag, nil
}

// CompleteMultipartUpload 完成上传
func (a *AliyunAdapter) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
	completedParts := make([]types.CompletedPart, len(parts))
	for i, p := range parts {
		completedParts[i] = types.CompletedPart{
			ETag:       aws.String(p.ETag),
			PartNumber: aws.Int32(int32(p.PartNumber)),
		}
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(a.bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completedParts},
	}

	_, err := a.client.CompleteMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to complete multipart upload: %w", err)
	}

	return nil
}

// AbortMultipartUpload 取消上传
func (a *AliyunAdapter) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(a.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := a.client.AbortMultipartUpload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to abort multipart upload: %w", err)
	}

	return nil
}

// SupportedStorageClasses 返回支持的存储类型
func (a *AliyunAdapter) SupportedStorageClasses() []StorageClass {
	return []StorageClass{
		StorageClassStandard,
		StorageClassInfrequent,
		StorageClassArchive,
		StorageClassDeepArchive,
	}
}

// SetStorageClass 设置存储类型
func (a *AliyunAdapter) SetStorageClass(ctx context.Context, key string, class StorageClass) error {
	copySource := fmt.Sprintf("%s/%s", a.bucket, key)
	aliyunStorageClass := a.mapStorageClass(class)

	input := &s3.CopyObjectInput{
		Bucket:            aws.String(a.bucket),
		CopySource:        aws.String(copySource),
		Key:               aws.String(key),
		Metadata:          map[string]string{"x-oss-storage-class": aliyunStorageClass},
		MetadataDirective: types.MetadataDirectiveReplace,
	}

	_, err := a.client.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set storage class: %w", err)
	}

	return nil
}

// mapStorageClass 将通用存储类型映射到阿里云 OSS 的存储类型值
// 阿里云 OSS 存储类型: Standard, IA, Archive, ColdArchive, DeepColdArchive
func (a *AliyunAdapter) mapStorageClass(sc StorageClass) string {
	switch sc {
	case StorageClassStandard:
		return "Standard"
	case StorageClassInfrequent:
		return "IA"
	case StorageClassArchive:
		return "Archive"
	case StorageClassDeepArchive:
		return "ColdArchive"
	default:
		return "Standard"
	}
}
