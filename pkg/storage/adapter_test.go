package storage

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "无协议前缀",
			input:    "s3.cn-east-1.qiniucs.com",
			expected: "https://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "已有 HTTPS 前缀",
			input:    "https://s3.cn-east-1.qiniucs.com",
			expected: "https://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "已有 HTTP 前缀",
			input:    "http://s3.cn-east-1.qiniucs.com",
			expected: "http://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "带空格的端点",
			input:    "  s3.cn-east-1.qiniucs.com  ",
			expected: "https://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "大写 HTTP 前缀",
			input:    "HTTP://s3.cn-east-1.qiniucs.com",
			expected: "HTTP://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "混合大小写 HTTPS 前缀",
			input:    "HtTpS://s3.cn-east-1.qiniucs.com",
			expected: "HtTpS://s3.cn-east-1.qiniucs.com",
		},
		{
			name:     "阿里云端点无协议前缀",
			input:    "oss-cn-hangzhou.aliyuncs.com",
			expected: "https://oss-cn-hangzhou.aliyuncs.com",
		},
		{
			name:     "阿里云端点有协议前缀",
			input:    "https://oss-cn-hangzhou.aliyuncs.com",
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
