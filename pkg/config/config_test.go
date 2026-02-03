package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSetDefaults 测试默认值设置
func TestSetDefaults(t *testing.T) {
	cfg := &Config{}

	setDefaults(cfg)

	// 检查存储默认值
	if cfg.Storage.Provider != "aws" {
		t.Errorf("expected default provider 'aws', got '%s'", cfg.Storage.Provider)
	}
	if cfg.Storage.Region != "us-east-1" {
		t.Errorf("expected default region 'us-east-1', got '%s'", cfg.Storage.Region)
	}
	if cfg.Storage.StorageClass != "standard" {
		t.Errorf("expected default storage class 'standard', got '%s'", cfg.Storage.StorageClass)
	}

	// 检查备份默认值
	if cfg.Backup.Compression != "gzip" {
		t.Errorf("expected default compression 'gzip', got '%s'", cfg.Backup.Compression)
	}
	if cfg.Backup.ChunkSize != 5*1024*1024 {
		t.Errorf("expected default chunk size 5MB, got %d", cfg.Backup.ChunkSize)
	}
	if cfg.Backup.Concurrency != 4 {
		t.Errorf("expected default concurrency 4, got %d", cfg.Backup.Concurrency)
	}
}

// TestSetDefaultsPreservesExisting 测试保留现有值
func TestSetDefaultsPreservesExisting(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{
			Provider:     "qiniu",
			Region:       "cn-east",
			StorageClass: "ia",
		},
		Backup: BackupConfig{
			Compression: "none",
			ChunkSize:   10 * 1024 * 1024,
			Concurrency: 8,
		},
	}

	setDefaults(cfg)

	if cfg.Storage.Provider != "qiniu" {
		t.Errorf("expected provider 'qiniu' to be preserved, got '%s'", cfg.Storage.Provider)
	}
	if cfg.Backup.ChunkSize != 10*1024*1024 {
		t.Errorf("expected chunk size 10MB to be preserved, got %d", cfg.Backup.ChunkSize)
	}
}

// TestValidateProvider 测试存储提供商验证
func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		wantErr  bool
	}{
		{"AWS", "aws", false},
		{"AWS uppercase", "AWS", false},
		{"Qiniu", "qiniu", false},
		{"Aliyun", "aliyun", false},
		{"invalid provider", "gcp", true},
		{"empty provider", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  tt.provider,
					Bucket:    "test-bucket",
					AccessKey: "test-key",
					SecretKey: "test-secret",
				},
				Backup: BackupConfig{
					ChunkSize: 5 * 1024 * 1024,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateBucket 测试 bucket 验证
func TestValidateBucket(t *testing.T) {
	tests := []struct {
		name    string
		bucket  string
		wantErr bool
	}{
		{"valid bucket", "my-bucket", false},
		{"empty bucket", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  "aws",
					Bucket:    tt.bucket,
					AccessKey: "test-key",
					SecretKey: "test-secret",
				},
				Backup: BackupConfig{
					ChunkSize: 5 * 1024 * 1024,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateAccessKey 测试 access key 验证
func TestValidateAccessKey(t *testing.T) {
	tests := []struct {
		name     string
		accessKey string
		wantErr  bool
	}{
		{"valid key in config", "config-key", false},
		{"empty key in config", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  "aws",
					Bucket:    "test-bucket",
					AccessKey: tt.accessKey,
					SecretKey: "test-secret",
				},
				Backup: BackupConfig{
					ChunkSize: 5 * 1024 * 1024,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateSecretKey 测试 secret key 验证
func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name     string
		secretKey string
		wantErr  bool
	}{
		{"valid key in config", "config-secret", false},
		{"empty key in config", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  "aws",
					Bucket:    "test-bucket",
					AccessKey: "test-key",
					SecretKey: tt.secretKey,
				},
				Backup: BackupConfig{
					ChunkSize: 5 * 1024 * 1024,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateChunkSize 测试分块大小验证
func TestValidateChunkSize(t *testing.T) {
	tests := []struct {
		name     string
		chunkSize int64
		wantErr  bool
	}{
		{"5MB minimum", 5 * 1024 * 1024, false},
		{"10MB", 10 * 1024 * 1024, false},
		{"below minimum", 4 * 1024 * 1024, true},
		{"zero", 0, true},
		{"negative", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  "aws",
					Bucket:    "test-bucket",
					AccessKey: "test-key",
					SecretKey: "test-secret",
				},
				Backup: BackupConfig{
					ChunkSize: tt.chunkSize,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateEncryption 测试加密配置验证
func TestValidateEncryption(t *testing.T) {
	tests := []struct {
		name         string
		enabled      bool
		password     string
		keyFile      string
		wantErr      bool
	}{
		{"encryption disabled", false, "", "", false},
		{"encryption with password", true, "test-password", "", false},
		{"encryption with key file", true, "", "/path/to/key", false},
		{"encryption with both", true, "password", "/path/to/key", false},
		{"encryption without credentials", true, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Storage: StorageConfig{
					Provider:  "aws",
					Bucket:    "test-bucket",
					AccessKey: "test-key",
					SecretKey: "test-secret",
				},
				Encryption: EncryptionConfig{
					Enabled:  tt.enabled,
					Password: tt.password,
					KeyFile:  tt.keyFile,
				},
				Backup: BackupConfig{
					ChunkSize: 5 * 1024 * 1024,
				},
			}

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetAccessKey 测试获取 Access Key
func TestGetAccessKey(t *testing.T) {
	tests := []struct {
		name          string
		configKey     string
		envKey        string
		expectedKey   string
	}{
		{"from config", "config-key", "env-key", "config-key"},
		{"from env when config empty", "", "env-key", "env-key"},
		{"config takes priority", "config-key", "env-key", "config-key"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			if tt.envKey != "" {
				os.Setenv("S3BACKUP_ACCESS_KEY", tt.envKey)
				defer os.Unsetenv("S3BACKUP_ACCESS_KEY")
			}

			cfg := &Config{
				Storage: StorageConfig{
					AccessKey: tt.configKey,
				},
			}

			key := cfg.GetAccessKey()
			if key != tt.expectedKey {
				t.Errorf("expected access key '%s', got '%s'", tt.expectedKey, key)
			}
		})
	}
}

// TestGetSecretKey 测试获取 Secret Key
func TestGetSecretKey(t *testing.T) {
	tests := []struct {
		name          string
		configKey     string
		envKey        string
		expectedKey   string
	}{
		{"from config", "config-secret", "env-secret", "config-secret"},
		{"from env when config empty", "", "env-secret", "env-secret"},
		{"config takes priority", "config-secret", "env-secret", "config-secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			if tt.envKey != "" {
				os.Setenv("S3BACKUP_SECRET_KEY", tt.envKey)
				defer os.Unsetenv("S3BACKUP_SECRET_KEY")
			}

			cfg := &Config{
				Storage: StorageConfig{
					SecretKey: tt.configKey,
				},
			}

			key := cfg.GetSecretKey()
			if key != tt.expectedKey {
				t.Errorf("expected secret key '%s', got '%s'", tt.expectedKey, key)
			}
		})
	}
}

// TestGetPassword 测试获取密码
func TestGetPassword(t *testing.T) {
	tests := []struct {
		name          string
		configPwd     string
		envPwd        string
		expectedPwd   string
	}{
		{"from config", "config-password", "env-password", "config-password"},
		{"from env when config empty", "", "env-password", "env-password"},
		{"config takes priority", "config-password", "env-password", "config-password"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			if tt.envPwd != "" {
				os.Setenv("S3BACKUP_ENCRYPT_PASSWORD", tt.envPwd)
				defer os.Unsetenv("S3BACKUP_ENCRYPT_PASSWORD")
			}

			cfg := &Config{
				Encryption: EncryptionConfig{
					Password: tt.configPwd,
				},
			}

			pwd := cfg.GetPassword()
			if pwd != tt.expectedPwd {
				t.Errorf("expected password '%s', got '%s'", tt.expectedPwd, pwd)
			}
		})
	}
}

// TestLoadEnvFile 测试加载环境文件
func TestLoadEnvFile(t *testing.T) {
	// 创建临时 .env 文件
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".s3backup.env")
	content := []byte("S3BACKUP_ACCESS_KEY=test-key\nS3BACKUP_SECRET_KEY=test-secret\n")
	if err := os.WriteFile(envPath, content, 0600); err != nil {
		t.Fatalf("failed to create temp env file: %v", err)
	}

	// 测试加载
	if err := loadEnvFile(envPath); err != nil {
		t.Errorf("loadEnvFile() failed: %v", err)
	}

	// 验证环境变量已设置
	if os.Getenv("S3BACKUP_ACCESS_KEY") != "test-key" {
		t.Error("S3BACKUP_ACCESS_KEY not set correctly")
	}

	// 清理
	os.Unsetenv("S3BACKUP_ACCESS_KEY")
	os.Unsetenv("S3BACKUP_SECRET_KEY")
}

// TestLoadEnvFileNonExistent 测试加载不存在的环境文件
func TestLoadEnvFileNonExistent(t *testing.T) {
	// 不存在的文件不应报错
	if err := loadEnvFile("/nonexistent/path/.env"); err != nil {
		t.Errorf("loadEnvFile() with non-existent file should not error, got: %v", err)
	}
}

// TestValidConfig 测试完整有效配置
func TestValidConfig(t *testing.T) {
	cfg := &Config{
		Storage: StorageConfig{
			Provider:     "aws",
			Bucket:       "my-backup-bucket",
			Region:       "us-west-2",
			AccessKey:    "AKIAIOSFODNN7EXAMPLE",
			SecretKey:    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			StorageClass: "standard",
		},
		Encryption: EncryptionConfig{
			Enabled:  true,
			Password: "secure-password-123",
		},
		Backup: BackupConfig{
			Includes:    []string{"/home/user/documents"},
			Excludes:    []string{"*.tmp", "*.log"},
			Compression: "gzip",
			ChunkSize:   10 * 1024 * 1024,
			Concurrency: 8,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass validation, got error: %v", err)
	}
}

// TestConfigDefaults 测试配置结构体初始化
func TestConfigDefaults(t *testing.T) {
	cfg := &Config{}

	if cfg.Storage.Provider != "" {
		t.Error("new config should have empty provider")
	}
	if cfg.Backup.ChunkSize != 0 {
		t.Error("new config should have zero chunk size")
	}
}
