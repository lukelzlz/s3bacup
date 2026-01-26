package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lukelzlz/s3backup/pkg/archive"
	"github.com/lukelzlz/s3backup/pkg/config"
	"github.com/lukelzlz/s3backup/pkg/crypto"
	"github.com/lukelzlz/s3backup/pkg/storage"
	"github.com/lukelzlz/s3backup/pkg/uploader"
	"github.com/spf13/cobra"
)

var (
	provider      string
	bucket        string
	endpoint      string
	region        string
	accessKey     string
	secretKey     string
	storageClass  string
	encrypt       bool
	password      string
	keyFile       string
	excludes      []string
	backupName    string
	concurrency   int
	chunkSize     int64
	dryRun        bool
)

// backupCmd 备份命令
var backupCmd = &cobra.Command{
	Use:   "backup [paths...]",
	Short: "执行备份",
	Long:  `将指定路径打包压缩并上传到 S3 兼容存储`,
	Args:  cobra.MinimumNArgs(1),
	RunE:  runBackup,
}

func init() {
	rootCmd.AddCommand(backupCmd)

	// backup 命令 flags
	backupCmd.Flags().StringVarP(&provider, "provider", "p", "", "存储提供商 (aws/qiniu/aliyun)")
	backupCmd.Flags().StringVarP(&bucket, "bucket", "b", "", "存储桶名称")
	backupCmd.Flags().StringVar(&endpoint, "endpoint", "", "自定义端点")
	backupCmd.Flags().StringVar(&region, "region", "", "区域")
	backupCmd.Flags().StringVar(&accessKey, "access-key", "", "Access Key")
	backupCmd.Flags().StringVar(&secretKey, "secret-key", "", "Secret Key")
	backupCmd.Flags().StringVarP(&storageClass, "storage-class", "s", "", "存储类型 (standard/ia/archive/deep_archive)")
	backupCmd.Flags().BoolVarP(&encrypt, "encrypt", "e", false, "启用加密")
	backupCmd.Flags().StringVar(&password, "password", "", "加密密码")
	backupCmd.Flags().StringVar(&keyFile, "key-file", "", "密钥文件")
	backupCmd.Flags().StringSliceVar(&excludes, "exclude", []string{}, "排除模式（可多次指定）")
	backupCmd.Flags().StringVarP(&backupName, "name", "n", "", "备份文件名（默认：backup-{timestamp}.tar.gz.enc）")
	backupCmd.Flags().IntVar(&concurrency, "concurrency", 0, "并发上传数")
	backupCmd.Flags().Int64Var(&chunkSize, "chunk-size", 0, "分块大小（字节）")
	backupCmd.Flags().BoolVar(&dryRun, "dry-run", false, "模拟运行，不实际上传")
}

func runBackup(cmd *cobra.Command, args []string) error {
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	// 加载配置
	cfg, err := config.LoadConfig(cfgFile, envFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 命令行参数覆盖配置
	if provider != "" {
		cfg.Storage.Provider = provider
	}
	if bucket != "" {
		cfg.Storage.Bucket = bucket
	}
	if endpoint != "" {
		cfg.Storage.Endpoint = endpoint
	}
	if region != "" {
		cfg.Storage.Region = region
	}
	if accessKey != "" {
		cfg.Storage.AccessKey = accessKey
	}
	if secretKey != "" {
		cfg.Storage.SecretKey = secretKey
	}
	if storageClass != "" {
		cfg.Storage.StorageClass = storageClass
	}
	if encrypt {
		cfg.Encryption.Enabled = true
	}
	if password != "" {
		cfg.Encryption.Password = password
	}
	if keyFile != "" {
		cfg.Encryption.KeyFile = keyFile
	}
	if len(excludes) > 0 {
		cfg.Backup.Excludes = excludes
	}
	if concurrency > 0 {
		cfg.Backup.Concurrency = concurrency
	}
	if chunkSize > 0 {
		cfg.Backup.ChunkSize = chunkSize
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// 解析包含路径
	includes, err := archive.ResolveIncludes(args)
	if err != nil {
		return fmt.Errorf("failed to resolve includes: %w", err)
	}

	// 生成备份文件名
	if backupName == "" {
		timestamp := startTime.Format("20060102-150405")
		backupName = fmt.Sprintf("backup-%s.tar.gz", timestamp)
		if cfg.Encryption.Enabled {
			backupName += ".enc"
		}
	}

	fmt.Printf("备份配置:\n")
	fmt.Printf("  存储提供商: %s\n", cfg.Storage.Provider)
	fmt.Printf("  存储桶: %s\n", cfg.Storage.Bucket)
	fmt.Printf("  存储类型: %s\n", cfg.Storage.StorageClass)
	fmt.Printf("  加密: %v\n", cfg.Encryption.Enabled)
	fmt.Printf("  并发数: %d\n", cfg.Backup.Concurrency)
	fmt.Printf("  分块大小: %d MB\n", cfg.Backup.ChunkSize/1024/1024)
	fmt.Printf("  备份文件: %s\n", backupName)
	fmt.Printf("  包含路径: %d 个\n", len(includes))
	fmt.Println()

	// 创建存储适配器
	adapter, err := createStorageAdapter(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create storage adapter: %w", err)
	}

	// 创建 io.Pipe 连接归档和上传
	pr, pw := io.Pipe()

	// 错误通道
	errChan := make(chan error, 3)

	// 启动归档 goroutine
	go func() {
		defer pw.Close()
		var writer io.Writer = pw

		// 创建加密器
		if cfg.Encryption.Enabled {
			encryptor, err := createEncryptor(cfg)
			if err != nil {
				cancel()
				errChan <- err
				return
			}
			encWriter, err := encryptor.WrapWriter(pw)
			if err != nil {
				cancel()
				errChan <- fmt.Errorf("failed to create encrypt writer: %w", err)
				return
			}
			defer func() {
				if err := encWriter.Close(); err != nil {
					errChan <- fmt.Errorf("failed to close encryptor: %w", err)
				}
			}()
			writer = encWriter
		}

		// 创建归档器
		archiver, err := archive.NewArchiver(includes, cfg.Backup.Excludes)
		if err != nil {
			cancel()
			errChan <- fmt.Errorf("failed to create archiver: %w", err)
			return
		}

		// 执行归档
		if err := archiver.Archive(ctx, writer); err != nil {
			cancel()
			errChan <- fmt.Errorf("failed to archive: %w", err)
			return
		}
	}()

	// 上传
	if !dryRun {
		// 创建上传器
		upl := uploader.NewUploader(adapter, cfg.Backup.ChunkSize, cfg.Backup.Concurrency)

		// 上传选项
		contentType := "application/gzip"
		if cfg.Encryption.Enabled {
			contentType = "application/octet-stream"
		}
		opts := storage.UploadOptions{
			StorageClass: storage.ParseStorageClass(cfg.Storage.StorageClass),
			ContentType:  contentType,
		}

		// 启动上传 goroutine
		go func() {
			if err := upl.Upload(ctx, backupName, pr, opts); err != nil {
				cancel()
				errChan <- fmt.Errorf("failed to upload: %w", err)
				return
			}
			errChan <- nil
		}()

		// 等待完成
		if err := <-errChan; err != nil {
			return err
		}
	} else {
		// 模拟运行：只读取数据不上传
		go func() {
			buf := make([]byte, 32*1024)
			for {
				_, err := pr.Read(buf)
				if err != nil {
					if err == io.EOF {
						errChan <- nil
					} else {
						cancel()
						errChan <- fmt.Errorf("failed to read: %w", err)
					}
					return
				}
			}
		}()

		if err := <-errChan; err != nil {
			return err
		}
		fmt.Println("模拟运行完成（未实际上传）")
		return nil
	}

	fmt.Printf("备份成功: %s\n", backupName)
	return nil
}

// createStorageAdapter 创建存储适配器
func createStorageAdapter(ctx context.Context, cfg *config.Config) (storage.StorageAdapter, error) {
	accessKey := cfg.GetAccessKey()
	secretKey := cfg.GetSecretKey()

	switch strings.ToLower(cfg.Storage.Provider) {
	case "aws":
		return storage.NewAWSAdapter(ctx, cfg.Storage.Region, cfg.Storage.Endpoint, cfg.Storage.Bucket, accessKey, secretKey)
	case "qiniu":
		return storage.NewQiniuAdapter(ctx, cfg.Storage.Endpoint, cfg.Storage.Bucket, accessKey, secretKey)
	case "aliyun":
		return storage.NewAliyunAdapter(ctx, cfg.Storage.Region, cfg.Storage.Endpoint, cfg.Storage.Bucket, accessKey, secretKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Storage.Provider)
	}
}

// createEncryptor 创建加密器
func createEncryptor(cfg *config.Config) (*crypto.StreamEncryptor, error) {
	var aesKey, hmacKey []byte
	var err error

	if cfg.Encryption.KeyFile != "" {
		// 从密钥文件读取
		keyData, err := os.ReadFile(cfg.Encryption.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
		aesKey, hmacKey, err = crypto.DeriveKeyFromKeyFile(keyData)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key from file: %w", err)
		}
	} else {
		// 从密码派生
		password := cfg.GetPassword()
		if password == "" {
			return nil, fmt.Errorf("encryption password is required")
		}
		// TODO: Verify the correct function name when pkg/crypto package is implemented.
		// This should derive from a password, but the function name suggests it derives from a file.
		// Consider using a function like crypto.DeriveKeyFromPassword() instead.
		aesKey, hmacKey, err = crypto.DeriveKeyFromPasswordFile(password)
		if err != nil {
			return nil, fmt.Errorf("failed to derive key: %w", err)
		}
	}

	return crypto.NewStreamEncryptor(aesKey, hmacKey)
}
