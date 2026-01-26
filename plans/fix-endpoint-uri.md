# 修复端点 URI 格式问题

## 问题分析

### 错误信息
```
Error: failed to upload: failed to init multipart upload: failed to create multipart upload: operation error S3: CreateMultipartUpload, resolve auth scheme: resolve endpoint: endpoint rule error, Custom endpoint `s3.cn-north-1.qiniucs.com` was not a valid URI
```

### 根本原因
AWS SDK v2 要求 `BaseEndpoint` 参数必须是一个完整的 URI（包含协议前缀如 `https://`），但当前代码和配置文件中的端点格式为 `s3.cn-north-1.qiniucs.com`，缺少协议前缀。

### 影响范围
- 所有使用自定义端点的存储提供商：七牛云、阿里云 OSS
- 受影响的文件：
  - `pkg/storage/aws.go` (第37行)
  - `pkg/storage/qiniu.go` (第39行)
  - `pkg/storage/aliyun.go` (第38行)
  - `.s3backup.example.yaml` (第13行)
  - `README.md` (第95、98行)

## 修复方案

### 方案概述
采用**向后兼容**的方式修复：
1. 在代码中自动为端点添加协议前缀（如果缺失）
2. 更新文档说明正确的端点格式
3. 确保现有配置仍然可用

### 实施步骤

#### 步骤 1: 添加端点格式化工具函数

在 `pkg/storage/adapter.go` 中添加辅助函数：

```go
import (
    "strings"
)

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
```

#### 步骤 2: 修改存储适配器

修改三个适配器文件，在设置 `BaseEndpoint` 前调用 `normalizeEndpoint`：

**pkg/storage/aws.go**
```go
client := s3.NewFromConfig(cfg, func(o *s3.Options) {
    if endpoint != "" {
        o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
    }
})
```

**pkg/storage/qiniu.go**
```go
client := s3.NewFromConfig(cfg, func(o *s3.Options) {
    if endpoint != "" {
        o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
    }
})
```

**pkg/storage/aliyun.go**
```go
client := s3.NewFromConfig(cfg, func(o *s3.Options) {
    if endpoint != "" {
        o.BaseEndpoint = aws.String(normalizeEndpoint(endpoint))
    }
})
```

#### 步骤 3: 更新配置文件示例

**.s3backup.example.yaml**
```yaml
# 七牛云 S3 协议端点
# 华东: https://s3.cn-east-1.qiniucs.com
# 华北: https://s3.cn-north-1.qiniucs.com
# 华南: https://s3.cn-south-1.qiniucs.com
endpoint: https://s3.cn-east-1.qiniucs.com
```

添加说明注释：
```yaml
  # 七牛云 S3 协议端点
  # 注意：端点必须包含协议前缀（https://）
  # 华东: https://s3.cn-east-1.qiniucs.com
  # 华北: https://s3.cn-north-1.qiniucs.com
  # 华南: https://s3.cn-south-1.qiniucs.com
  # 兼容性：如果不包含协议前缀，系统会自动添加 https://
  endpoint: https://s3.cn-east-1.qiniucs.com
```

#### 步骤 4: 更新 README.md 文档

**第95行附近**：
```markdown
# 七牛云
s3backup backup --provider qiniu --endpoint https://s3.cn-east-1.qiniucs.com --bucket my-bucket /path/to/backup
```

**第98行附近**：
```markdown
# 阿里云 OSS
s3backup backup --provider aliyun --endpoint https://oss-cn-hangzhou.aliyuncs.com --bucket my-bucket /path/to/backup
```

**第340行附近**（故障排查部分）：
```markdown
**Q: 连接七牛云失败**
A: 检查 endpoint 是否正确，七牛云 S3 协议端点格式为 `https://s3.<region>.qiniucs.com`
   注意：端点必须包含协议前缀（https://），如果不包含，系统会自动添加。
```

#### 步骤 5: 添加单元测试（可选）

在 `pkg/storage/adapter_test.go` 中添加测试：

```go
package storage

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"s3.cn-east-1.qiniucs.com", "https://s3.cn-east-1.qiniucs.com"},
        {"https://s3.cn-east-1.qiniucs.com", "https://s3.cn-east-1.qiniucs.com"},
        {"http://s3.cn-east-1.qiniucs.com", "http://s3.cn-east-1.qiniucs.com"},
        {"", ""},
        {"  s3.cn-east-1.qiniucs.com  ", "https://s3.cn-east-1.qiniucs.com"},
    }

    for _, tt := range tests {
        result := normalizeEndpoint(tt.input)
        if result != tt.expected {
            t.Errorf("normalizeEndpoint(%q) = %q, want %q", tt.input, result, tt.expected)
        }
    }
}
```

## 验证计划

### 测试场景

1. **向后兼容性测试**
   - 使用旧格式端点（无协议前缀）：`s3.cn-north-1.qiniucs.com`
   - 验证自动添加 `https://` 前缀
   - 确保备份成功

2. **新格式测试**
   - 使用新格式端点（包含协议前缀）：`https://s3.cn-north-1.qiniucs.com`
   - 验证直接使用，不重复添加前缀
   - 确保备份成功

3. **HTTP 协议测试**
   - 使用 `http://` 前缀（不推荐但应支持）
   - 验证保留原协议
   - 确保备份成功

4. **空端点测试**
   - 不提供端点参数
   - 验证不报错
   - 使用默认端点

### 验证命令

```bash
# 测试旧格式（自动添加 https://）
s3backup backup --provider qiniu --endpoint s3.cn-north-1.qiniucs.com --bucket cdn_local --encrypt /d

# 测试新格式
s3backup backup --provider qiniu --endpoint https://s3.cn-north-1.qiniucs.com --bucket cdn_local --encrypt /d

# 测试 HTTP 协议
s3backup backup --provider qiniu --endpoint http://s3.cn-north-1.qiniucs.com --bucket cdn_local --encrypt /d
```

## 优势

1. **向后兼容**：现有配置无需修改即可正常工作
2. **用户友好**：自动处理端点格式，减少配置错误
3. **文档清晰**：明确说明正确的端点格式
4. **代码健壮**：统一的端点处理逻辑
5. **安全性**：默认使用 HTTPS，保护数据传输安全

## 风险评估

- **低风险**：代码改动小，仅添加辅助函数和修改三处调用
- **向后兼容**：不会破坏现有配置
- **测试覆盖**：通过多个测试场景确保功能正确

## 后续优化建议

1. 考虑在配置验证阶段检查端点可达性
2. 添加端点连接测试命令
3. 支持自定义协议前缀配置
4. 添加端点格式验证的单元测试
