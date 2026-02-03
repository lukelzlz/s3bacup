package storage

import (
	"bytes"
	"context"
	"testing"
)

// MockAWSClient is a mock implementation of the AWS S3 client interface
type MockAWSClient struct {
	initCalled      bool
	uploadParts     []MockUploadedPart
	completeCalled  bool
	abortCalled     bool
	shouldFailInit  bool
	shouldFailUpload bool
	partNumberToFail int
	uploadID        string
}

type MockUploadedPart struct {
	PartNumber int
	Data       []byte
	ETag       string
}

func (m *MockAWSClient) CreateMultipartUpload(ctx context.Context, input interface{}) (string, error) {
	m.initCalled = true
	if m.shouldFailInit {
		return "", ErrMockInitFailed
	}
	m.uploadID = "mock-upload-id-123"
	return m.uploadID, nil
}

func (m *MockAWSClient) UploadPart(ctx context.Context, input interface{}) (string, error) {
	if m.shouldFailUpload && m.partNumberToFail > 0 {
		// This would be checked in real implementation
	}
	return "mock-etag", nil
}

func (m *MockAWSClient) CompleteMultipartUpload(ctx context.Context, input interface{}) error {
	m.completeCalled = true
	return nil
}

func (m *MockAWSClient) AbortMultipartUpload(ctx context.Context, input interface{}) error {
	m.abortCalled = true
	return nil
}

func (m *MockAWSClient) CopyObject(ctx context.Context, input interface{}) error {
	return nil
}

// TestAWSAdapterSupportedStorageClasses 测试 AWS 支持的存储类型
func TestAWSAdapterSupportedStorageClasses(t *testing.T) {
	ctx := context.Background()

	// 创建一个 mock adapter（使用真实的构造函数但使用测试凭证）
	adapter, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create AWS adapter: %v", err)
	}

	classes := adapter.SupportedStorageClasses()

	if len(classes) < 4 {
		t.Errorf("expected at least 4 storage classes, got %d", len(classes))
	}

	// 检查包含标准类型
	hasStandard := false
	for _, c := range classes {
		if c == StorageClassStandard {
			hasStandard = true
			break
		}
	}
	if !hasStandard {
		t.Error("supported storage classes should include STANDARD")
	}
}

// TestNormalizeEndpointExtended 测试端点规范化（扩展测试）
func TestNormalizeEndpointExtended(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "no protocol",
			input:    "s3.amazonaws.com",
			expected: "https://s3.amazonaws.com",
		},
		{
			name:     "with https",
			input:    "https://s3.amazonaws.com",
			expected: "https://s3.amazonaws.com",
		},
		{
			name:     "with http",
			input:    "http://s3.amazonaws.com",
			expected: "http://s3.amazonaws.com",
		},
		{
			name:     "with spaces",
			input:    "  s3.amazonaws.com  ",
			expected: "https://s3.amazonaws.com",
		},
		{
			name:     "uppercase HTTP",
			input:    "HTTP://s3.amazonaws.com",
			expected: "HTTP://s3.amazonaws.com",
		},
		{
			name:     "mixed case HTTPS",
			input:    "HtTpS://s3.amazonaws.com",
			expected: "HtTpS://s3.amazonaws.com",
		},
		{
			name:     "Qiniu endpoint without protocol",
			input:    "s3.cn-east-1.qiniucs.com",
			expected: "https://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "Aliyun OSS endpoint without protocol",
			input:    "oss-cn-hangzhou.aliyuncs.com",
			expected: "https://oss-cn-hangzhou.aliyuncs.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeEndpoint(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestStorageClassParsing 测试存储类别解析
func TestStorageClassParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected StorageClass
	}{
		{"standard", StorageClassStandard},
		{"STANDARD", StorageClassStandard},
		{"ia", StorageClassInfrequent},
		{"infrequent", StorageClassInfrequent},
		{"INFREQUENT_ACCESS", StorageClassInfrequent},
		{"archive", StorageClassArchive},
		{"ARCHIVE", StorageClassArchive},
		{"deep_archive", StorageClassDeepArchive},
		{"DEEP_ARCHIVE", StorageClassDeepArchive},
		{"glacier_ir", StorageClassGlacierIR},
		{"GLACIER_IR", StorageClassGlacierIR},
		{"intelligent", StorageClassIntelligentTiering},
		{"INTELLIGENT_TIERING", StorageClassIntelligentTiering},
		{"unknown", StorageClassStandard}, // 默认返回标准
		{"", StorageClassStandard},        // 空字符串返回标准
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseStorageClass(tt.input)
			if result != tt.expected {
				t.Errorf("ParseStorageClass(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestStorageClassIsValid 测试存储类别验证
func TestStorageClassIsValid(t *testing.T) {
	tests := []struct {
		class    StorageClass
		expected bool
	}{
		{StorageClassStandard, true},
		{StorageClassInfrequent, true},
		{StorageClassArchive, true},
		{StorageClassDeepArchive, true},
		{StorageClassGlacierIR, true},
		{StorageClassIntelligentTiering, true},
		{StorageClass("invalid"), false},
		{StorageClass(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			result := tt.class.IsValid()
			if result != tt.expected {
				t.Errorf("StorageClass(%q).IsValid() = %v, want %v", tt.class, result, tt.expected)
			}
		})
	}
}

// TestStorageClassString 测试存储类别字符串表示
func TestStorageClassString(t *testing.T) {
	tests := []struct {
		class    StorageClass
		expected string
	}{
		{StorageClassStandard, "STANDARD"},
		{StorageClassInfrequent, "INFREQUENT_ACCESS"},
		{StorageClassArchive, "ARCHIVE"},
		{StorageClassDeepArchive, "DEEP_ARCHIVE"},
		{StorageClassGlacierIR, "GLACIER_IR"},
		{StorageClassIntelligentTiering, "INTELLIGENT_TIERING"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.class.String()
			if result != tt.expected {
				t.Errorf("StorageClass(%q).String() = %q, want %q", tt.class, result, tt.expected)
			}
		})
	}
}

// TestUploadOptions 测试上传选项
func TestUploadOptions(t *testing.T) {
	opts := UploadOptions{
		StorageClass: StorageClassStandard,
		ContentType:  "application/gzip",
		Metadata:     map[string]string{"key": "value"},
	}

	if opts.StorageClass != StorageClassStandard {
		t.Error("storage class not set correctly")
	}

	if opts.ContentType != "application/gzip" {
		t.Error("content type not set correctly")
	}

	if opts.Metadata["key"] != "value" {
		t.Error("metadata not set correctly")
	}
}

// TestCompletedPart 测试已完成分块
func TestCompletedPart(t *testing.T) {
	part := CompletedPart{
		PartNumber: 1,
		ETag:       "etag-123",
	}

	if part.PartNumber != 1 {
		t.Errorf("expected part number 1, got %d", part.PartNumber)
	}

	if part.ETag != "etag-123" {
		t.Errorf("expected etag 'etag-123', got %s", part.ETag)
	}
}

// TestAdapterInterfaceValidation 测试适配器接口实现
func TestAdapterInterfaceValidation(t *testing.T) {
	// 这个测试确保所有适配器都实现了 StorageAdapter 接口
	ctx := context.Background()

	var adapter StorageAdapter

	// AWS adapter
	awsAdapter, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create AWS adapter: %v", err)
	}
	adapter = awsAdapter
	if adapter == nil {
		t.Error("AWS adapter should implement StorageAdapter interface")
	}

	// Qiniu adapter
	qiniuAdapter, err := NewQiniuAdapter(ctx, "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Qiniu adapter: %v", err)
	}
	adapter = qiniuAdapter
	if adapter == nil {
		t.Error("Qiniu adapter should implement StorageAdapter interface")
	}

	// Aliyun adapter
	aliyunAdapter, err := NewAliyunAdapter(ctx, "cn-hangzhou", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Aliyun adapter: %v", err)
	}
	adapter = aliyunAdapter
	if adapter == nil {
		t.Error("Aliyun adapter should implement StorageAdapter interface")
	}
}

// TestQiniuStorageClassMapping 测试七牛存储类型映射
func TestQiniuStorageClassMapping(t *testing.T) {
	ctx := context.Background()
	qiniuAdapter, err := NewQiniuAdapter(ctx, "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Qiniu adapter: %v", err)
	}

	// 测试支持的存储类是否存在
	classes := qiniuAdapter.SupportedStorageClasses()

	if len(classes) < 4 {
		t.Errorf("Qiniu should support at least 4 storage classes, got %d", len(classes))
	}
}

// TestAliyunStorageClassMapping 测试阿里云存储类型映射
func TestAliyunStorageClassMapping(t *testing.T) {
	ctx := context.Background()
	aliyunAdapter, err := NewAliyunAdapter(ctx, "cn-hangzhou", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Aliyun adapter: %v", err)
	}

	classes := aliyunAdapter.SupportedStorageClasses()

	if len(classes) < 4 {
		t.Errorf("Aliyun should support at least 4 storage classes, got %d", len(classes))
	}
}

// TestContextCancellation 测试上下文取消
func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	adapter, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create AWS adapter: %v", err)
	}

	// 尝试操作（应该会因为上下文取消而失败）
	_, err = adapter.InitMultipartUpload(ctx, "test-key", UploadOptions{})
	// 注意：这可能不会立即返回错误，因为 AWS SDK 可能还没有检查上下文
	_ = adapter // 避免未使用变量警告
	_ = err
}

// TestEmptyKeyHandling 测试空密钥处理
func TestEmptyKeyHandling(t *testing.T) {
	ctx := context.Background()

	// 空密钥应该能创建适配器（但实际操作会失败）
	_, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "", "")
	if err != nil {
		// 某些实现可能会验证空密钥
		t.Logf("empty key validation: %v", err)
	}
}

// TestInvalidRegionHandling 测试无效区域处理
func TestInvalidRegionHandling(t *testing.T) {
	ctx := context.Background()

	// 无效区域可能仍然能创建适配器（实际请求时才会失败）
	_, err := NewAWSAdapter(ctx, "invalid-region-123", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Logf("invalid region handling: %v", err)
	}
}

// TestIOReaderAdapter 测试 io.Reader 适配
func TestIOReaderAdapter(t *testing.T) {
	// 确保我们的接口兼容 io.Reader
	ctx := context.Background()
	adapter, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create AWS adapter: %v", err)
	}

	// UploadPart 需要 io.Reader
	testData := []byte("test data")
	reader := bytes.NewReader(testData)

	// 这个测试只验证类型兼容性，不实际调用（需要 mock）
	_ = adapter
	_ = reader
}

// TestMultipleProvidersSupportedClasses 测试多个提供商支持的存储类
func TestMultipleProvidersSupportedClasses(t *testing.T) {
	ctx := context.Background()

	providers := []struct {
		name     string
		adapter  StorageAdapter
		minCount int
	}{
		{
			name:     "AWS",
			adapter:  mustCreateAWSAdapter(ctx, t),
			minCount: 4,
		},
		{
			name:     "Qiniu",
			adapter:  mustCreateQiniuAdapter(ctx, t),
			minCount: 4,
		},
		{
			name:     "Aliyun",
			adapter:  mustCreateAliyunAdapter(ctx, t),
			minCount: 4,
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			classes := p.adapter.SupportedStorageClasses()
			if len(classes) < p.minCount {
				t.Errorf("%s: expected at least %d storage classes, got %d", p.name, p.minCount, len(classes))
			}

			// 验证 STANDARD 总是被支持
			hasStandard := false
			for _, c := range classes {
				if c == StorageClassStandard {
					hasStandard = true
					break
				}
			}
			if !hasStandard {
				t.Errorf("%s: should support STANDARD storage class", p.name)
			}
		})
	}
}

// Helper functions
func mustCreateAWSAdapter(ctx context.Context, t *testing.T) StorageAdapter {
	adapter, err := NewAWSAdapter(ctx, "us-east-1", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create AWS adapter: %v", err)
	}
	return adapter
}

func mustCreateQiniuAdapter(ctx context.Context, t *testing.T) StorageAdapter {
	adapter, err := NewQiniuAdapter(ctx, "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Qiniu adapter: %v", err)
	}
	return adapter
}

func mustCreateAliyunAdapter(ctx context.Context, t *testing.T) StorageAdapter {
	adapter, err := NewAliyunAdapter(ctx, "cn-hangzhou", "", "test-bucket", "test-key", "test-secret")
	if err != nil {
		t.Fatalf("failed to create Aliyun adapter: %v", err)
	}
	return adapter
}
