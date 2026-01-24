package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config 配置结构
type Config struct {
	Storage    StorageConfig    `yaml:"storage"`
	Encryption EncryptionConfig `yaml:"encryption"`
	Backup     BackupConfig     `yaml:"backup"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Provider     string `yaml:"provider"`     // aws, qiniu, aliyun
	Endpoint     string `yaml:"endpoint"`
	Region       string `yaml:"region"`
	Bucket       string `yaml:"bucket"`
	AccessKey    string `yaml:"access_key"`
	SecretKey    string `yaml:"secret_key"`
	StorageClass string `yaml:"storage_class"` // 存储类型
}

// EncryptionConfig 加密配置
type EncryptionConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Password string `yaml:"password"` // 用于派生密钥
	KeyFile  string `yaml:"key_file"` // 或直接使用密钥文件
}

// BackupConfig 备份配置
type BackupConfig struct {
	Includes    []string `yaml:"includes"`    // 包含路径
	Excludes    []string `yaml:"excludes"`    // 排除模式
	Compression string   `yaml:"compression"` // gzip, none
	ChunkSize   int64    `yaml:"chunk_size"`  // 分块大小，默认 5MB
	Concurrency int      `yaml:"concurrency"` // 并发上传数
}

// LoadConfig 加载配置
func LoadConfig(configPath, envPath string) (*Config, error) {
	// 加载 .env 文件
	if err := loadEnvFile(envPath); err != nil {
		return nil, fmt.Errorf("failed to load env file: %w", err)
	}

	// 设置 viper
	v := viper.New()
	v.SetConfigType("yaml")

	// 配置文件查找顺序
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// 默认查找路径
		v.SetConfigName(".s3backup")
		v.SetConfigType("yaml")

		// 查找路径
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME")
		v.AddConfigPath("$HOME/.config/s3backup")
	}

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		// 配置文件不存在不是错误，使用默认值
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	// 绑定环境变量
	v.SetEnvPrefix("S3BACKUP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 解析配置
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 填充默认值
	setDefaults(&cfg)

	return &cfg, nil
}

// loadEnvFile 加载 .env 文件
func loadEnvFile(envPath string) error {
	if envPath != "" {
		return godotenv.Load(envPath)
	}

	// 查找 .env 文件
	paths := []string{
		".s3backup.env",
		filepath.Join(os.Getenv("HOME"), ".s3backup.env"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return godotenv.Load(path)
		}
	}

	return nil
}

// setDefaults 设置默认值
func setDefaults(cfg *Config) {
	// 存储配置默认值
	if cfg.Storage.Provider == "" {
		cfg.Storage.Provider = "aws"
	}
	if cfg.Storage.Region == "" {
		cfg.Storage.Region = "us-east-1"
	}
	if cfg.Storage.StorageClass == "" {
		cfg.Storage.StorageClass = "standard"
	}

	// 备份配置默认值
	if cfg.Backup.Compression == "" {
		cfg.Backup.Compression = "gzip"
	}
	if cfg.Backup.ChunkSize == 0 {
		cfg.Backup.ChunkSize = 5 * 1024 * 1024 // 5MB
	}
	if cfg.Backup.Concurrency == 0 {
		cfg.Backup.Concurrency = 4
	}
}

// GetAccessKey 获取 Access Key（优先级：配置 > 环境变量）
func (c *Config) GetAccessKey() string {
	if c.Storage.AccessKey != "" {
		return c.Storage.AccessKey
	}
	return os.Getenv("S3BACKUP_ACCESS_KEY")
}

// GetSecretKey 获取 Secret Key（优先级：配置 > 环境变量）
func (c *Config) GetSecretKey() string {
	if c.Storage.SecretKey != "" {
		return c.Storage.SecretKey
	}
	return os.Getenv("S3BACKUP_SECRET_KEY")
}

// GetPassword 获取加密密码（优先级：配置 > 环境变量）
func (c *Config) GetPassword() string {
	if c.Encryption.Password != "" {
		return c.Encryption.Password
	}
	return os.Getenv("S3BACKUP_ENCRYPT_PASSWORD")
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Storage.Bucket == "" {
		return fmt.Errorf("storage bucket is required")
	}

	accessKey := c.GetAccessKey()
	if accessKey == "" {
		return fmt.Errorf("storage access_key is required")
	}

	secretKey := c.GetSecretKey()
	if secretKey == "" {
		return fmt.Errorf("storage secret_key is required")
	}

	if c.Encryption.Enabled {
		password := c.GetPassword()
		if password == "" && c.Encryption.KeyFile == "" {
			return fmt.Errorf("encryption password or key_file is required when encryption is enabled")
		}
	}

	return nil
}

// SaveConfig 保存配置到文件
func SaveConfig(cfg *Config, configPath string) error {
	v := viper.New()
	v.SetConfigType("yaml")

	// 设置配置值
	v.Set("storage", cfg.Storage)
	v.Set("encryption", cfg.Encryption)
	v.Set("backup", cfg.Backup)

	if err := v.SafeWriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
