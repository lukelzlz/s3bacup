# S3Backup

流式备份工具，支持边打包边上传到 S3 兼容存储。

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
s3backup backup --provider qiniu --endpoint s3.cn-east-1.qiniucs.com --bucket my-bucket /path/to/backup

# 阿里云 OSS
s3backup backup --provider aliyun --endpoint oss-cn-hangzhou.aliyuncs.com --bucket my-bucket /path/to/backup
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

## 项目结构

```
s3backup/
├── cmd/s3backup/          # 程序入口
├── internal/cli/           # CLI 命令
├── pkg/
│   ├── config/            # 配置管理
│   ├── storage/           # 存储适配器
│   ├── crypto/            # 加密模块
│   ├── archive/           # 归档模块
│   └── uploader/          # 上传管理器
└── plans/                # 架构设计文档
```

## 依赖

- [github.com/aws/aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) - AWS S3 SDK
- [github.com/spf13/cobra](https://github.com/spf13/cobra) - CLI 框架
- [github.com/spf13/viper](https://github.com/spf13/viper) - 配置管理
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - .env 文件加载
- [golang.org/x/crypto](https://golang.org/x/crypto) - 加密算法
- [github.com/gobwas/glob](https://github.com/gobwas/glob) - Glob 模式匹配

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
