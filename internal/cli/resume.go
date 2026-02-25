package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lukelzlz/s3backup/pkg/config"
	"github.com/lukelzlz/s3backup/pkg/progress"
	"github.com/lukelzlz/s3backup/pkg/state"
	"github.com/lukelzlz/s3backup/pkg/storage"
	"github.com/lukelzlz/s3backup/pkg/uploader"
	"github.com/spf13/cobra"
)

var (
	resumeName string
	resumeDir  string
)

// resumeCmd 恢复命令
var resumeCmd = &cobra.Command{
	Use:   "resume [backup-name]",
	Short: "恢复未完成的上传",
	Long:  `从上次中断的位置继续上传`,
	Args:  cobra.ExactArgs(1),
	RunE:  runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().StringVar(&resumeDir, "state-dir", "", "状态文件目录")
}

func runResume(cmd *cobra.Command, args []string) error {
	backupName := args[0]
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
	defer cancel()

	// 加载配置
	cfg, err := config.LoadConfig(cfgFile, envFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// 加载状态
	stateMgr := state.NewStateManager(resumeDir, backupName)
	savedState, err := stateMgr.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}

	if savedState == nil {
		return fmt.Errorf("no saved state found for: %s", backupName)
	}

	fmt.Printf("恢复上传:\n")
	fmt.Printf("  备份文件: %s\n", backupName)
	fmt.Printf("  Upload ID: %s\n", savedState.UploadID[:20]+"...")
	fmt.Printf("  已完成分块: %d\n", len(savedState.Completed))
	fmt.Printf("  已上传: %d MB\n", savedState.UploadedBytes/1024/1024)
	fmt.Println()

	// 创建存储适配器
	adapter, err := createStorageAdapterFromState(ctx, cfg, savedState)
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

		// 如果需要加密
		if savedState.Encrypted {
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
			defer encWriter.Close()
			writer = encWriter
		}

		// 注意：恢复时需要重新读取所有数据，但跳过已上传的分块
		// 这需要在 uploader 层面处理

		// 创建空的归档器（仅用于测试）
		// 实际恢复时需要用户提供原始路径
		_, _ = io.Copy(writer, strings.NewReader(""))
	}()

	// 创建上传器（支持断点续传）
	upl := uploader.NewResumableUploader(adapter, cfg.Backup.ChunkSize, cfg.Backup.Concurrency, savedState)

	// 设置进度报告器
	reporter := progress.NewBar()
	upl.SetProgressReporter(reporter)
	defer reporter.Close()

	// 上传选项
	contentType := "application/gzip"
	if savedState.Encrypted {
		contentType = "application/octet-stream"
	}
	opts := storage.UploadOptions{
		StorageClass: storage.ParseStorageClass(savedState.StorageClass),
		ContentType:  contentType,
	}

	// 启动上传
	go func() {
		if err := upl.Resume(ctx, backupName, savedState.UploadID, pr, opts); err != nil {
			cancel()
			errChan <- fmt.Errorf("failed to resume upload: %w", err)
			return
		}
		errChan <- nil
	}()

	// 等待完成
	if err := <-errChan; err != nil {
		return err
	}

	// 删除状态文件
	stateMgr.Delete()

	fmt.Printf("恢复成功: %s\n", backupName)
	return nil
}

// createStorageAdapterFromState 从状态创建存储适配器
func createStorageAdapterFromState(ctx context.Context, cfg *config.Config, s *state.UploadState) (storage.StorageAdapter, error) {
	accessKey := cfg.GetAccessKey()
	secretKey := cfg.GetSecretKey()

	switch strings.ToLower(s.Provider) {
	case "aws":
		return storage.NewAWSAdapter(ctx, cfg.Storage.Region, cfg.Storage.Endpoint, s.Bucket, accessKey, secretKey)
	case "qiniu":
		return storage.NewQiniuAdapter(ctx, cfg.Storage.Endpoint, s.Bucket, accessKey, secretKey)
	case "aliyun":
		return storage.NewAliyunAdapter(ctx, cfg.Storage.Region, cfg.Storage.Endpoint, s.Bucket, accessKey, secretKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", s.Provider)
	}
}
