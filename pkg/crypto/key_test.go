package crypto

import (
	"bytes"
	"testing"
)

// TestDeriveKey 测试密钥派生
func TestDeriveKey(t *testing.T) {
	password := "test-password-123"

	aesKey, hmacKey, err := DeriveKey(password, nil)
	if err != nil {
		t.Fatalf("failed to derive key: %v", err)
	}

	if len(aesKey) != AESKeySize {
		t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
	}

	if len(hmacKey) != HMACKeySize {
		t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
	}

	// 相同密码应产生不同密钥（因为生成不同的盐）
	aesKey2, hmacKey2, err := DeriveKey(password, nil)
	if err != nil {
		t.Fatalf("failed to derive key again: %v", err)
	}

	if bytes.Equal(aesKey, aesKey2) {
		t.Error("same password should produce different AES keys with different salts")
	}

	if bytes.Equal(hmacKey, hmacKey2) {
		t.Error("same password should produce different HMAC keys with different salts")
	}
}

// TestDeriveKeyWithSameSalt 测试相同盐值产生相同密钥
func TestDeriveKeyWithSameSalt(t *testing.T) {
	password := "test-password-123"
	salt := make([]byte, SaltSize)
	for i := range salt {
		salt[i] = byte(i)
	}

	aesKey1, hmacKey1, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatalf("failed to derive key: %v", err)
	}

	aesKey2, hmacKey2, err := DeriveKey(password, salt)
	if err != nil {
		t.Fatalf("failed to derive key again: %v", err)
	}

	if !bytes.Equal(aesKey1, aesKey2) {
		t.Error("same password and salt should produce same AES key")
	}

	if !bytes.Equal(hmacKey1, hmacKey2) {
		t.Error("same password and salt should produce same HMAC key")
	}
}

// TestDeriveKeyDifferentPasswords 测试不同密码产生不同密钥
func TestDeriveKeyDifferentPasswords(t *testing.T) {
	salt := make([]byte, SaltSize)

	aesKey1, _, err := DeriveKey("password1", salt)
	if err != nil {
		t.Fatalf("failed to derive key for password1: %v", err)
	}

	aesKey2, _, err := DeriveKey("password2", salt)
	if err != nil {
		t.Fatalf("failed to derive key for password2: %v", err)
	}

	if bytes.Equal(aesKey1, aesKey2) {
		t.Error("different passwords should produce different AES keys")
	}
}

// TestDeriveKeyFromPasswordFile 测试从密码文件派生密钥
func TestDeriveKeyFromPasswordFile(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid password", "my-secure-password", false},
		{"empty password", "", true},
		{"short password", "a", false},
		{"long password", string(make([]byte, 1000)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aesKey, hmacKey, err := DeriveKeyFromPasswordFile(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveKeyFromPasswordFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(aesKey) != AESKeySize {
					t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
				}
				if len(hmacKey) != HMACKeySize {
					t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
				}
			}
		})
	}
}

// TestGenerateKeyFile 测试生成密钥文件
func TestGenerateKeyFile(t *testing.T) {
	keyData1, err := GenerateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate key file: %v", err)
	}

	expectedSize := AESKeySize + HMACKeySize
	if len(keyData1) != expectedSize {
		t.Errorf("expected key file size %d, got %d", expectedSize, len(keyData1))
	}

	// 再次生成应该得到不同的密钥
	keyData2, err := GenerateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate key file again: %v", err)
	}

	if bytes.Equal(keyData1, keyData2) {
		t.Error("generating key file twice should produce different keys")
	}
}

// TestDeriveKeyFromKeyFile 测试从密钥文件读取密钥
func TestDeriveKeyFromKeyFile(t *testing.T) {
	keyData, err := GenerateKeyFile()
	if err != nil {
		t.Fatalf("failed to generate key file: %v", err)
	}

	aesKey, hmacKey, err := DeriveKeyFromKeyFile(keyData)
	if err != nil {
		t.Fatalf("failed to derive key from file: %v", err)
	}

	if len(aesKey) != AESKeySize {
		t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
	}

	if len(hmacKey) != HMACKeySize {
		t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
	}
}

// TestDeriveKeyFromInvalidKeyFile 测试无效密钥文件
func TestDeriveKeyFromInvalidKeyFile(t *testing.T) {
	tests := []struct {
		name    string
		keyData []byte
		wantErr bool
	}{
		{"empty file", []byte{}, true},
		{"too short", []byte{1, 2, 3}, true},
		{"almost enough", make([]byte, AESKeySize+HMACKeySize-1), true},
		{"exact size", make([]byte, AESKeySize+HMACKeySize), false},
		{"larger than needed", make([]byte, AESKeySize+HMACKeySize+10), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aesKey, hmacKey, err := DeriveKeyFromKeyFile(tt.keyData)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveKeyFromKeyFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(aesKey) != AESKeySize {
					t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
				}
				if len(hmacKey) != HMACKeySize {
					t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
				}
			}
		})
	}
}

// TestDeriveKeyWithCustomSalt 测试使用自定义盐值
func TestDeriveKeyWithCustomSalt(t *testing.T) {
	tests := []struct {
		name    string
		salt    []byte
		wantErr bool
	}{
		{"valid salt", make([]byte, SaltSize), false},
		{"empty salt", []byte{}, true},
		{"too small", make([]byte, SaltSize-1), true},
		{"too large", make([]byte, SaltSize+1), true},
	}

	password := "test-password"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aesKey, hmacKey, err := DeriveKeyWithCustomSalt(password, tt.salt)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveKeyWithCustomSalt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(aesKey) != AESKeySize {
					t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
				}
				if len(hmacKey) != HMACKeySize {
					t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
				}
			}
		})
	}
}

// TestGenerateRandomIV 测试生成随机 IV
func TestGenerateRandomIV(t *testing.T) {
	iv1, err := GenerateRandomIV()
	if err != nil {
		t.Fatalf("failed to generate IV: %v", err)
	}

	if len(iv1) != IVSize {
		t.Errorf("expected IV size %d, got %d", IVSize, len(iv1))
	}

	// 再次生成应该得到不同的 IV
	iv2, err := GenerateRandomIV()
	if err != nil {
		t.Fatalf("failed to generate IV again: %v", err)
	}

	if bytes.Equal(iv1, iv2) {
		t.Error("generating IV twice should produce different values")
	}
}

// TestDeriveKeyFromPasswordWithIterations 测试带迭代次数的密钥派生
func TestDeriveKeyFromPasswordWithIterations(t *testing.T) {
	password := "test-password"
	salt := make([]byte, SaltSize)

	tests := []struct {
		name       string
		iterations uint32
		wantErr    bool
	}{
		{"1 iteration", 1, false},
		{"100 iterations", 100, false},
		{"10000 iterations", 10000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aesKey, hmacKey, err := DeriveKeyFromPasswordWithIterations(password, salt, tt.iterations)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeriveKeyFromPasswordWithIterations() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(aesKey) != AESKeySize {
					t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey))
				}
				if len(hmacKey) != HMACKeySize {
					t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey))
				}
			}
		})
	}
}

// TestDeriveKeyFromPasswordWithIterationsNilSalt 测试带迭代次数和自动生成盐值的密钥派生
func TestDeriveKeyFromPasswordWithIterationsNilSalt(t *testing.T) {
	password := "test-password"

	aesKey1, hmacKey1, err := DeriveKeyFromPasswordWithIterations(password, nil, 100)
	if err != nil {
		t.Fatalf("failed to derive key with nil salt: %v", err)
	}

	if len(aesKey1) != AESKeySize {
		t.Errorf("expected AES key size %d, got %d", AESKeySize, len(aesKey1))
	}

	if len(hmacKey1) != HMACKeySize {
		t.Errorf("expected HMAC key size %d, got %d", HMACKeySize, len(hmacKey1))
	}

	// 相同密码但自动生成不同盐值应该产生不同密钥
	aesKey2, _, err := DeriveKeyFromPasswordWithIterations(password, nil, 100)
	if err != nil {
		t.Fatalf("failed to derive key with nil salt again: %v", err)
	}

	if bytes.Equal(aesKey1, aesKey2) {
		t.Error("same password with auto-generated salts should produce different AES keys")
	}
}

// TestKeySizes 测试密钥大小常量
func TestKeySizes(t *testing.T) {
	// 验证常量值是否合理
	if AESKeySize != 32 {
		t.Errorf("AESKeySize should be 32 (256 bits), got %d", AESKeySize)
	}

	if HMACKeySize != 64 {
		t.Errorf("HMACKeySize should be 64 (512 bits for SHA-512), got %d", HMACKeySize)
	}

	if IVSize != 16 {
		t.Errorf("IVSize should be 16 (128 bits for AES), got %d", IVSize)
	}

	if SaltSize != 32 {
		t.Errorf("SaltSize should be 32 (256 bits), got %d", SaltSize)
	}
}
