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
	resumeDir     string
	resumePaths   []string
	resumeExclude []string
)

// resumeCmd 恢复命令
var resumeCmd = &cobra.Command{
	Use:   "resume [backup-name]",
	Short: "恢复未完成的上传",
	Long:  `从上次中断的位置继续上传。需要重新提供原始路径。`,
	Args:  cobra.ExactArgs(1),
	RunE:  runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
	resumeCmd.Flags().StringVar(&resumeDir, "state-dir", "", "状态文件目录")
	resumeCmd.Flags().StringSliceVarP(&resumePaths, "path", "p", []string{}, "原始备份路径（可多次指定）")
	resumeCmd.Flags().StringSliceVar(&resumeExclude, "exclude", []string{}, "排除模式")
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

	// 检查是否提供了路径
	if len(resumePaths) == 0 {
		return fmt.Errorf("请使用 --path 参数提供原始备份路径")
	}

	uploadIDPreview := savedState.UploadID
	if len(uploadIDPreview) > 20 {
		uploadIDPreview = uploadIDPreview[:20] + "..."
	}

	fmt.Printf("恢复上传:\n")
	fmt.Printf("  备份文件: %s\n", backupName)
	fmt.Printf("  Upload ID: %s\n", uploadIDPreview)
	fmt.Printf("  已完成分块: %d\n", len(savedState.Completed))
	fmt.Printf("  已上传: %d MB\n", savedState.UploadedBytes/1024/1024)
	fmt.Printf("  并发数: %d\n", cfg.Backup.Concurrency)
	fmt.Println()

	// 创建存储适配器
	adapter, err := createStorageAdapterFromState(ctx, cfg, savedState)
	if err != nil {
		return fmt.Errorf("failed to create storage adapter: %w", err)
	}

	// 创建 io.Pipe
	pr, pw := io.Pipe()

	// 错误通道
	errChan := make(chan error, 2)

	// 启动数据读取 goroutine（简化版：只读取空数据用于测试）
	// 实际使用时需要根据 resumePaths 重新归档
	go func() {
		defer pw.Close()

		// TODO: 实现完整的归档恢复
		// 需要根据 resumePaths 重新创建归档器
		// 并跳过已上传的分块
		_, _ = io.Copy(pw, strings.NewReader(""))
	}()

	// 创建可恢复上传器
	upl := uploader.NewResumableUploader(adapter, cfg.Backup.ChunkSize, cfg.Backup.Concurrency, savedState)
	upl.SetStateManager(stateMgr)

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
		fmt.Printf("\n恢复失败，状态已保存。可以再次使用 resume 命令继续。\n")
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
		return storage.NewAWSAdapter(ctx, s.Region, s.Endpoint, s.Bucket, accessKey, secretKey)
	case "qiniu":
		return storage.NewQiniuAdapter(ctx, s.Endpoint, s.Bucket, accessKey, secretKey)
	case "aliyun":
		return storage.NewAliyunAdapter(ctx, s.Region, s.Endpoint, s.Bucket, accessKey, secretKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", s.Provider)
	}
}
