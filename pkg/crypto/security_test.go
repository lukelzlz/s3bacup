package crypto

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestKeyDerivationSecurity 測試密鑰派生安全性
func TestKeyDerivationSecurity(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		salt      []byte
		wantError bool
	}{
		{
			name:      "empty password",
			password:  "",
			salt:      nil,
			wantError: true,
		},
		{
			name:      "weak short password",
			password:  "123",
			salt:      nil,
			wantError: false, // 接受弱密碼（用戶責任）
		},
		{
			name:      "strong password",
			password:  "ThisIsAVeryStrongPassword!@#123",
			salt:      nil,
			wantError: false,
		},
		{
			name:      "password with unicode",
			password:  "密碼🔐пароль",
			salt:      nil,
			wantError: false,
		},
		{
			name:      "same password different salt",
			password:  "testpassword",
			salt:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 測試 DeriveKeyFromPasswordFile
			if tt.salt == nil {
				_, _, err := DeriveKeyFromPasswordFile(tt.password)
				if tt.wantError && err == nil {
					t.Error("expected error for weak/empty password")
				}
				if !tt.wantError && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			// 測試帶鹽值的密鑰派生
			if tt.salt != nil || !tt.wantError {
				salt := tt.salt
				if salt == nil {
					salt = make([]byte, SaltSize)
				}
				aesKey, hmacKey, err := DeriveKey(tt.password, salt)
				if err != nil {
					t.Errorf("DeriveKey failed: %v", err)
				}
				if len(aesKey) != AESKeySize {
					t.Errorf("AES key size: got %d, want %d", len(aesKey), AESKeySize)
				}
				if len(hmacKey) != HMACKeySize {
					t.Errorf("HMAC key size: got %d, want %d", len(hmacKey), HMACKeySize)
				}
			}
		})
	}
}

// TestKeyUniqueness 測試不同密碼/鹽值產生不同密鑰
func TestKeyUniqueness(t *testing.T) {
	password := "testpassword"

	// 不同鹽值應該產生不同密鑰
	salt1 := make([]byte, SaltSize)
	salt2 := make([]byte, SaltSize)
	salt2[0] = 1 // 修改一個字節

	aesKey1, _, _ := DeriveKey(password, salt1)
	aesKey2, _, _ := DeriveKey(password, salt2)

	if bytes.Equal(aesKey1, aesKey2) {
		t.Error("different salts should produce different keys")
	}

	// 不同密碼應該產生不同密鑰（相同鹽值）
	salt := make([]byte, SaltSize)
	aesKey3, _, _ := DeriveKey("password1", salt)
	aesKey4, _, _ := DeriveKey("password2", salt)

	if bytes.Equal(aesKey3, aesKey4) {
		t.Error("different passwords should produce different keys")
	}
}

// TestIVUniqueness 測試每次生成的 IV 都是唯一的
func TestIVUniqueness(t *testing.T) {
	ivs := make(map[string]bool)
	// 生成多個 IV，檢查是否有重複
	for i := 0; i < 1000; i++ {
		iv, err := GenerateRandomIV()
		if err != nil {
			t.Fatalf("GenerateRandomIV failed: %v", err)
		}
		key := string(iv)
		if ivs[key] {
			t.Error("generated duplicate IV")
		}
		ivs[key] = true
	}
}

// TestKeyFileValidation 測試密鑰文件驗證
func TestKeyFileValidation(t *testing.T) {
	tests := []struct {
		name      string
		keyData   []byte
		wantError bool
	}{
		{
			name:      "valid key file",
			keyData:   make([]byte, AESKeySize+HMACKeySize),
			wantError: false,
		},
		{
			name:      "larger key file",
			keyData:   make([]byte, AESKeySize+HMACKeySize+10),
			wantError: false,
		},
		{
			name:      "too small",
			keyData:   make([]byte, AESKeySize+HMACKeySize-1),
			wantError: true,
		},
		{
			name:      "empty",
			keyData:   []byte{},
			wantError: true,
		},
		{
			name:      "nil",
			keyData:   nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DeriveKeyFromKeyFile(tt.keyData)
			if tt.wantError && err == nil {
				t.Error("expected error")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestEncryptionDecryptionRoundtrip 測試加密解密往返
func TestEncryptionDecryptionRoundtrip(t *testing.T) {
	password := "testpassword123"
	aesKey, hmacKey, err := DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	testData := []byte("This is a test message for encryption!")
	var buf bytes.Buffer

	// 加密
	writer, err := encryptor.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}

	n, err := writer.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write length: got %d, want %d", n, len(testData))
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	encryptedData := buf.Bytes()

	// 解密（使用帶 HMAC 驗證的方法）
	reader, err := encryptor.WrapReaderWithHMAC(bytes.NewReader(encryptedData))
	if err != nil {
		t.Fatalf("WrapReaderWithHMAC failed: %v", err)
	}

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Errorf("decrypted data mismatch:\ngot  %x\nwant %x", decrypted, testData)
	}
}

// TestHMACVerificationComprehensive 測試 HMAC 驗證的各種情況
// 注意：TestHMACVerification 已在 stream_test.go 中
func TestHMACVerificationComprehensive(t *testing.T) {
	password := "testpassword123"
	aesKey, hmacKey, err := DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	testData := []byte("This is test data")
	var buf bytes.Buffer

	writer, err := encryptor.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}
	writer.Write(testData)
	writer.Close()

	validEncrypted := buf.Bytes()

	tests := []struct {
		name      string
		modify    func([]byte) []byte
		wantError bool
	}{
		{
			name: "valid data",
			modify: func(b []byte) []byte {
				return b
			},
			wantError: false,
		},
		{
			name: "corrupt encrypted data",
			modify: func(b []byte) []byte {
				// 修改加密數據
				result := make([]byte, len(b))
				copy(result, b)
				// 修改 IV 之後的數據
				if len(result) > 24 {
					result[24] ^= 0xff
				}
				return result
			},
			wantError: true,
		},
		{
			name: "corrupt HMAC",
			modify: func(b []byte) []byte {
				result := make([]byte, len(b))
				copy(result, b)
				// 修改最後的 HMAC
				if len(result) > 0 {
					result[len(result)-1] ^= 0xff
				}
				return result
			},
			wantError: true,
		},
		{
			name: "truncate data",
			modify: func(b []byte) []byte {
				if len(b) > 10 {
					return b[:len(b)-10]
				}
				return b
			},
			wantError: true,
		},
		{
			name: "wrong magic",
			modify: func(b []byte) []byte {
				result := make([]byte, len(b))
				copy(result, b)
				// 修改魔數
				copy(result, []byte("XXXX"))
				return result
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifiedData := tt.modify(validEncrypted)
			_, err := encryptor.WrapReaderWithHMAC(bytes.NewReader(modifiedData))
			if tt.wantError && err == nil {
				t.Error("expected HMAC verification error")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestInvalidKeySizesSecurity 測試無效的密鑰大小
// 注意：TestInvalidKeySize 已在 stream_test.go 中
func TestInvalidKeySizesSecurity(t *testing.T) {
	tests := []struct {
		name     string
		aesKey   []byte
		hmacKey  []byte
		wantErr  bool
	}{
		{
			name:    "valid keys",
			aesKey:  make([]byte, AESKeySize),
			hmacKey: make([]byte, HMACKeySize),
			wantErr: false,
		},
		{
			name:    "short AES key",
			aesKey:  make([]byte, AESKeySize-1),
			hmacKey: make([]byte, HMACKeySize),
			wantErr: true,
		},
		{
			name:    "long AES key",
			aesKey:  make([]byte, AESKeySize+1),
			hmacKey: make([]byte, HMACKeySize),
			wantErr: true,
		},
		{
			name:    "short HMAC key",
			aesKey:  make([]byte, AESKeySize),
			hmacKey: make([]byte, HMACKeySize-1),
			wantErr: true,
		},
		{
			name:    "long HMAC key",
			aesKey:  make([]byte, AESKeySize),
			hmacKey: make([]byte, HMACKeySize+1),
			wantErr: true,
		},
		{
			name:    "empty keys",
			aesKey:  []byte{},
			hmacKey: []byte{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewStreamEncryptor(tt.aesKey, tt.hmacKey)
			if tt.wantErr && err == nil {
				t.Error("expected error for invalid key size")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestEmptyDataSecurity 測試空數據加密
// 注意：TestEmptyData 已在 stream_test.go 中
func TestEmptyDataSecurity(t *testing.T) {
	password := "testpassword123"
	aesKey, hmacKey, err := DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	var buf bytes.Buffer
	writer, err := encryptor.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}

	// 寫入空數據
	n, err := writer.Write([]byte{})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 0 {
		t.Errorf("Write length: got %d, want 0", n)
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 應該有 header 和 trailer（magic + IV + length + HMAC）
	encrypted := buf.Bytes()
	minSize := 4 + IVSize + 8 + 64 // magic + IV + length + HMAC
	if len(encrypted) < minSize {
		t.Errorf("encrypted size: got %d, want at least %d", len(encrypted), minSize)
	}
}

// TestLargeDataSecurity 測試大量數據加密
// 注意：TestLargeData 已在 stream_test.go 中
func TestLargeDataSecurity(t *testing.T) {
	password := "testpassword123"
	aesKey, hmacKey, err := DeriveKeyFromPasswordFile(password)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor, err := NewStreamEncryptor(aesKey, hmacKey)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	// 1MB 數據
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	var buf bytes.Buffer
	writer, err := encryptor.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}

	// 分塊寫入
	chunkSize := 4096
	for i := 0; i < len(testData); i += chunkSize {
		end := i + chunkSize
		if end > len(testData) {
			end = len(testData)
		}
		if _, err := writer.Write(testData[i:end]); err != nil {
			t.Fatalf("Write failed at chunk %d: %v", i/chunkSize, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 解密並驗證
	reader, err := encryptor.WrapReaderWithHMAC(&buf)
	if err != nil {
		t.Fatalf("WrapReaderWithHMAC failed: %v", err)
	}

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if !bytes.Equal(decrypted, testData) {
		t.Errorf("large data decryption failed")
	}
}

// TestSaltValidation 測試鹽值驗證
func TestSaltValidation(t *testing.T) {
	tests := []struct {
		name      string
		salt      []byte
		wantError bool
	}{
		{
			name:      "valid salt",
			salt:      make([]byte, SaltSize),
			wantError: false,
		},
		{
			name:      "short salt",
			salt:      make([]byte, SaltSize-1),
			wantError: true,
		},
		{
			name:      "long salt",
			salt:      make([]byte, SaltSize+1),
			wantError: true,
		},
		{
			name:      "nil salt",
			salt:      nil,
			wantError: false, // DeriveKey 接受 nil 並生成新的
		},
	}

	password := "testpassword"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.salt != nil {
				_, _, err := DeriveKeyWithCustomSalt(password, tt.salt)
				if tt.wantError && err == nil {
					t.Error("expected error for invalid salt")
				}
				if !tt.wantError && err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				// nil salt 應該由 DeriveKey 處理
				_, _, err := DeriveKey(password, nil)
				if err != nil {
					t.Errorf("DeriveKey with nil salt failed: %v", err)
				}
			}
		})
	}
}

// TestPBKDF2Iterations 測試 PBKDF2 迭代次數驗證
func TestPBKDF2Iterations(t *testing.T) {
	tests := []struct {
		name       string
		iterations uint32
		wantError  bool
	}{
		{
			name:       "zero iterations",
			iterations: 0,
			wantError:  true,
		},
		{
			name:       "one iteration",
			iterations: 1,
			wantError:  false,
		},
		{
			name:       "normal iterations",
			iterations: 100000,
			wantError:  false,
		},
		{
			name:       "high iterations",
			iterations: 1000000,
			wantError:  false,
		},
	}

	password := "testpassword"
	salt := make([]byte, SaltSize)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := DeriveKeyFromPasswordWithIterations(password, salt, tt.iterations)
			if tt.wantError && err == nil {
				t.Error("expected error for invalid iterations")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestWrongKeyDecryption 測試使用錯誤密鑰解密
func TestWrongKeyDecryption(t *testing.T) {
	password1 := "password1"
	password2 := "password2"

	aesKey1, hmacKey1, err := DeriveKeyFromPasswordFile(password1)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	aesKey2, hmacKey2, err := DeriveKeyFromPasswordFile(password2)
	if err != nil {
		t.Fatalf("failed to derive keys: %v", err)
	}

	encryptor1, err := NewStreamEncryptor(aesKey1, hmacKey1)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	testData := []byte("Secret message")
	var buf bytes.Buffer

	writer, err := encryptor1.WrapWriter(&buf)
	if err != nil {
		t.Fatalf("WrapWriter failed: %v", err)
	}
	writer.Write(testData)
	writer.Close()

	// 使用錯誤的密鑰嘗試解密
	encryptor2, err := NewStreamEncryptor(aesKey2, hmacKey2)
	if err != nil {
		t.Fatalf("failed to create encryptor: %v", err)
	}

	_, err = encryptor2.WrapReaderWithHMAC(&buf)
	if err == nil {
		t.Error("expected HMAC verification failure with wrong key")
	}
	// 錯誤消息應該包含 "HMAC"
	if err != nil && !strings.Contains(err.Error(), "HMAC") {
		t.Logf("error message: %v", err)
	}
}

// TestGenerateKeyFileUniqueness 測試密鑰文件生成的唯一性
// 注意：TestGenerateKeyFile 已在 key_test.go 中
func TestGenerateKeyFileUniqueness(t *testing.T) {
	// 生成多個密鑰文件，確保它們是唯一的
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, err := GenerateKeyFile()
		if err != nil {
			t.Fatalf("GenerateKeyFile failed: %v", err)
		}
		keyStr := string(key)
		if keys[keyStr] {
			t.Error("generated duplicate key")
		}
		keys[keyStr] = true
	}
}
