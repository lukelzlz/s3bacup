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

// AWSAdapter AWS S3 适配器
type AWSAdapter struct {
	client *s3.Client
	bucket string
}

// NewAWSAdapter 创建 AWS S3 适配器
func NewAWSAdapter(ctx context.Context, region, endpoint, bucket, accessKey, secretKey string) (*AWSAdapter, error) {
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
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
		}
	})

	return &AWSAdapter{
		client: client,
		bucket: bucket,
	}, nil
}

// InitMultipartUpload 初始化 Multipart Upload
func (a *AWSAdapter) InitMultipartUpload(ctx context.Context, key string, opts UploadOptions) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	}

	if opts.StorageClass.IsValid() {
		input.StorageClass = types.StorageClass(opts.StorageClass.String())
	}
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}

	result, err := a.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create multipart upload: %w", err)
	}

	return *result.UploadId, nil
}

// UploadPart 上传分块
func (a *AWSAdapter) UploadPart(ctx context.Context, key, uploadID string, partNum int, data io.Reader, size int64) (string, error) {
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
func (a *AWSAdapter) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []CompletedPart) error {
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
func (a *AWSAdapter) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
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
func (a *AWSAdapter) SupportedStorageClasses() []StorageClass {
	return []StorageClass{
		StorageClassStandard,
		StorageClassInfrequent,
		StorageClassArchive,
		StorageClassDeepArchive,
		StorageClassGlacierIR,
		StorageClassIntelligentTiering,
	}
}

// SetStorageClass 设置存储类型（AWS S3 支持在上传时指定，此方法用于后续修改）
func (a *AWSAdapter) SetStorageClass(ctx context.Context, key string, class StorageClass) error {
	// AWS S3 需要通过 CopyObject 来修改存储类型
	// CopySource 格式: source-bucket/source-key (SDK 会进行 URL 编码)
	copySource := fmt.Sprintf("%s/%s", a.bucket, key)
	input := &s3.CopyObjectInput{
		Bucket:            aws.String(a.bucket),
		CopySource:        aws.String(copySource),
		Key:               aws.String(key),
		StorageClass:      types.StorageClass(class.String()),
		MetadataDirective: types.MetadataDirectiveReplace,
	}

	_, err := a.client.CopyObject(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to set storage class: %w", err)
	}

	return nil
}
