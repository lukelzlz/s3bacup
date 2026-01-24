package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	envFile string
)

// rootCmd 根命令
var rootCmd = &cobra.Command{
	Use:   "s3backup",
	Short: "流式备份工具，支持边打包边上传到 S3 兼容存储",
	Long: `S3Backup 是一个 Go 语言编写的命令行备份工具，
支持边打包压缩边上传到 S3 兼容存储，并能适配各云存储服务的独特功能（如存储类型设置）。

支持多云存储：
  - AWS S3
  - 七牛云 Kodo
  - 阿里云 OSS
  - 其他 S3 兼容存储

特性：
  - 流式处理，无需本地临时文件
  - AES-256-CTR + HMAC-SHA512 加密
  - 支持设置存储类型（低频、归档等）
  - Multipart Upload 并发上传`,
}

// Execute 执行根命令
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// 全局 flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "配置文件路径 (默认 ~/.s3backup.yaml)")
	rootCmd.PersistentFlags().StringVar(&envFile, "env-file", "", "环境变量文件路径 (默认 .s3backup.env)")
}

func initConfig() {
	// 配置初始化逻辑在子命令中处理
}
