# S3Backup

[![Go Version](https://img.shields.io/badge/Go-1.23.0-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

流式备份工具，支持边打包边上传到 S3 兼容存储。

**当前版本：v1.0.1**

## 特性

- **流式处理**：边打包压缩边上传，无需本地临时文件，内存占用低
- **加密支持**：AES-256-CTR + HMAC-SHA512 流式加密
- **多云适配**：支持 AWS S3、七牛云 Kodo、阿里云 OSS 等 S3 兼容存储
- **存储类型**：支持设置低频存储、归档存储等低成本存储类型
- **断点续传**：基于 S3 Multipart Upload 实现大文件可靠上传

## 安装

### 从源码编译

```bash
git clone https://github.com/lukelzlz/s3backup.git
cd s3backup
go build -o s3backup cmd/s3backup/main.go
```

### 使用 Go 安装

```bash
go install github.com/lukelzlz/s3backup@latest
```

## 配置

### 配置文件

复制示例配置文件到用户目录：

```bash
mkdir -p ~/.config/s3backup
cp .s3backup.example.yaml ~/.s3backup.yaml
cp .s3backup.example.env ~/.s3backup.env
```

编辑配置文件：

```bash
nano ~/.s3backup.yaml
```

### 环境变量

创建 `.s3backup.env` 文件：

```bash
# 存储凭证
S3BACKUP_ACCESS_KEY=your-access-key
S3BACKUP_SECRET_KEY=your-secret-key

# 加密密码
S3BACKUP_ENCRYPT_PASSWORD=your-password
```

### 配置优先级

1. 命令行参数（最高）
2. 环境变量
3. `.s3backup.env` 文件
4. `~/.s3backup.yaml` 配置文件
5. 默认值（最低）

## 使用

### 基本用法

```bash
# 备份单个目录
s3backup backup /path/to/backup

# 备份多个目录
s3backup backup /path/to/dir1 /path/to/dir2

# 使用配置文件
s3backup backup --config ~/.s3backup.yaml /path/to/backup
```

### 指定存储提供商

```bash
# AWS S3
s3backup backup --provider aws --bucket my-bucket /path/to/backup

# 七牛云
s3backup backup --provider qiniu --endpoint https://s3.cn-east-1.qiniucs.com --bucket my-bucket /path/to/backup

# 阿里云 OSS
s3backup backup --provider aliyun --endpoint https://oss-cn-hangzhou.aliyuncs.com --bucket my-bucket /path/to/backup
```

### 设置存储类型

```bash
# 标准存储
s3backup backup --storage-class standard /path/to/backup

# 低频访问存储
s3backup backup --storage-class ia /path/to/backup

# 归档存储
s3backup backup --storage-class archive /path/to/backup

# 深度归档存储
s3backup backup --storage-class deep_archive /path/to/backup
```

### 加密备份

```bash
# 使用密码加密
s3backup backup --encrypt --password "my-secret-password" /path/to/backup

# 使用环境变量中的密码
export S3BACKUP_ENCRYPT_PASSWORD="my-secret-password"
s3backup backup --encrypt /path/to/backup
```

### 排除文件

```bash
# 排除特定文件
s3backup backup --exclude "*.log" --exclude "*.tmp" /path/to/backup

# 排除目录
s3backup backup --exclude ".git/**" --exclude "node_modules/**" /path/to/backup
```

### 高级选项

```bash
# 自定义并发数和分块大小
s3backup backup --concurrency 8 --chunk-size 10485760 /path/to/backup

# 自定义备份文件名
s3backup backup --name "my-backup.tar.gz" /path/to/backup

# 模拟运行（不实际上传）
s3backup backup --dry-run /path/to/backup
```

## 存储类型说明

### AWS S3

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| STANDARD | 标准存储 | 频繁访问的数据 |
| STANDARD_IA | 低频访问 | 不常访问但需要快速访问的数据 |
| GLACIER | 归档存储 | 很少访问的数据 |
| DEEP_ARCHIVE | 深度归档 | 长期归档数据 |

### 七牛云 Kodo

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| 0 (标准) | 标准存储 | 频繁访问的数据 |
| 1 (低频) | 低频存储 | 不常访问的数据 |
| 2 (归档) | 归档存储 | 很少访问的数据 |
| 3 (深度归档) | 深度归档 | 长期归档数据 |

### 阿里云 OSS

| 类型 | 说明 | 适用场景 |
|------|------|----------|
| Standard | 标准存储 | 频繁访问的数据 |
| IA | 低频访问 | 不常访问但需要快速访问的数据 |
| Archive | 归档存储 | 很少访问的数据 |
| ColdArchive | 冷归档 | 长期归档数据 |

## 变更日志

### v1.0.1 (2026-01-26)
- 完善项目文档和 README
- 优化配置加载逻辑
- 改进错误处理和用户提示

### v1.0.0
- 初始版本发布
- 支持流式备份到 S3 兼容存储
- 支持 AWS S3、七牛云 Kodo、阿里云 OSS
- 支持 AES-256-CTR + HMAC-SHA512 流式加密
- 支持设置存储类型（标准、低频、归档、深度归档）
- 支持并发分块上传
- 支持文件排除模式

## 项目结构

```
s3backup/
├── cmd/s3backup/          # 程序入口
│   └── main.go
├── internal/cli/           # CLI 命令
│   ├── root.go            # 根命令定义
│   └── backup.go          # backup 命令实现
├── pkg/
│   ├── config/            # 配置管理
│   │   └── config.go
│   ├── storage/           # 存储适配器
│   │   ├── adapter.go     # 存储适配器接口
│   │   ├── aws.go         # AWS S3 适配器
│   │   ├── qiniu.go       # 七牛云适配器
│   │   ├── aliyun.go      # 阿里云 OSS 适配器
│   │   └── storage_class.go # 存储类型定义
│   ├── crypto/            # 加密模块
│   │   ├── stream.go      # 流式加密/解密
│   │   └── key.go         # 密钥派生
│   ├── archive/           # 归档模块
│   │   ├── archiver.go    # 归档器实现
│   │   └── tar.go         # tar 格式处理
│   └── uploader/          # 上传管理器
│       └── uploader.go    # Multipart Upload 实现
├── plans/                 # 架构设计文档
│   └── architecture.md
├── .s3backup.example.yaml # 配置文件示例
├── .s3backup.example.env  # 环境变量示例
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

## 依赖

- [github.com/aws/aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) v1.33.0 - AWS S3 SDK
- [github.com/aws/aws-sdk-go-v2/config](https://github.com/aws/aws-sdk-go-v2) v1.29.0 - AWS SDK 配置
- [github.com/aws/aws-sdk-go-v2/service/s3](https://github.com/aws/aws-sdk-go-v2) v1.58.0 - AWS S3 服务
- [github.com/spf13/cobra](https://github.com/spf13/cobra) v1.10.2 - CLI 框架
- [github.com/spf13/viper](https://github.com/spf13/viper) v1.21.0 - 配置管理
- [github.com/joho/godotenv](https://github.com/joho/godotenv) v1.5.1 - .env 文件加载
- [golang.org/x/crypto](https://golang.org/x/crypto) v0.32.0 - 加密算法
- [github.com/gobwas/glob](https://github.com/gobwas/glob) v0.2.3 - Glob 模式匹配

## 技术实现

### 流式处理架构

S3Backup 采用流式处理架构，通过 `io.Pipe` 连接归档和上传流程：

```
文件系统 → tar.Writer → gzip.Writer → EncryptWriter → io.PipeWriter
                                                              ↓
                                                        io.PipeReader
                                                              ↓
                                                        分块缓冲池 → 并发上传 → S3
```

**关键设计点：**

1. **零临时文件**：整个流程无需创建临时文件，节省磁盘空间
2. **内存可控**：内存占用约为 `并发数 × 分块大小`（默认 4 × 5MB = 20MB）
3. **并发上传**：使用 worker pool 模式并发上传多个分块
4. **缓冲池**：使用 `sync.Pool` 复用缓冲区，减少 GC 压力

### 加密方案

使用 AES-256-CTR + HMAC-SHA512 进行流式加密：

**加密文件格式：**
```
[4 bytes magic "S3BE"][16 bytes IV][encrypted data...][8 bytes data length][64 bytes HMAC]
```

**密钥派生：**
- 从密码派生：使用 Argon2id 算法
- 从密钥文件派生：直接读取密钥文件

**特性：**
- 每次加密使用随机 IV，确保相同数据加密结果不同
- HMAC-SHA512 提供完整性验证
- 支持流式加密/解密，适合大文件处理

### Multipart Upload

基于 S3 Multipart Upload API 实现大文件上传：

1. **初始化上传**：调用 `InitMultipartUpload` 获取 uploadID
2. **分块上传**：将数据分块并发上传
3. **完成上传**：调用 `CompleteMultipartUpload` 合并所有分块
4. **错误处理**：出错时调用 `AbortMultipartUpload` 取消上传

**默认配置：**
- 分块大小：5MB（S3 最小要求）
- 并发数：4
- 超时时间：24小时

### 配置加载优先级

配置加载遵循以下优先级（从高到低）：

1. **命令行参数**：`--access-key`, `--secret-key`, `--password` 等
2. **环境变量**：`S3BACKUP_ACCESS_KEY`, `S3BACKUP_SECRET_KEY` 等
3. **.env 文件**：`.s3backup.env`（当前目录或 `~/.s3backup.env`）
4. **配置文件**：`~/.s3backup.yaml` 或 `--config` 指定的文件
5. **默认值**：代码中定义的默认值

## 已知限制

1. **仅支持备份**：当前版本仅支持备份功能，不支持恢复
2. **无增量备份**：每次备份都是完整备份，不支持增量
3. **无进度显示**：当前版本不显示上传进度
4. **无断点续传**：上传中断后需要重新开始
5. **加密文件格式**：加密文件格式为自定义格式，需要使用本工具解密

## 安全建议

1. **密钥管理**：
   - 不要将 `.s3backup.env` 文件提交到版本控制系统
   - 使用强密码或密钥文件
   - 定期轮换密钥

2. **传输安全**：
   - 确保使用 HTTPS 端点
   - 验证服务器证书

3. **备份验证**：
   - 定期验证备份的完整性
   - 测试恢复流程（后续版本支持）

## 故障排查

### 常见问题

**Q: 上传失败，提示 "chunk_size must be at least 5MB"**
A: 配置文件中的 `chunk_size` 必须至少为 5MB（5242880 字节）

**Q: 加密后无法解密**
A: 确保使用相同的密码或密钥文件。密钥派生使用 Argon2id 算法，密码区分大小写

**Q: 连接七牛云失败**
A: 检查 endpoint 是否正确，七牛云 S3 协议端点格式为 `https://s3.<region>.qiniucs.com`
   注意：端点必须包含协议前缀（https://），如果不包含，系统会自动添加。

**Q: 某些文件被排除**
A: 检查配置文件中的 `excludes` 模式，支持 glob 模式匹配

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License

## 开发

```bash
# 运行测试
go test ./...

# 构建
go build -o s3backup cmd/s3backup/main.go

# 运行
./s3backup backup --help
```

## 许可证

MIT License
