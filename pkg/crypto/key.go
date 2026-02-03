package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// AESKeySize AES 密钥大小（256位）
	AESKeySize = 32
	// HMACKeySize HMAC 密钥大小（512位）
	HMACKeySize = 64
	// IVSize 初始化向量大小（128位）
	IVSize = 16
	// SaltSize 盐值大小
	SaltSize = 32
)

// DeriveKey 使用 Argon2id 从密码派生密钥
// 返回 (AES密钥, HMAC密钥)
func DeriveKey(password string, salt []byte) (aesKey, hmacKey []byte, err error) {
	if salt == nil {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// 使用 Argon2id 派生密钥
	// 参数选择：
	// time=3, memory=64MB, threads=4, keyLen=96 (32+64)
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, AESKeySize+HMACKeySize)

	aesKey = key[:AESKeySize]
	hmacKey = key[AESKeySize:]

	return aesKey, hmacKey, nil
}

// DeriveKeyFromPasswordFile 从密码派生密钥并生成新的盐值
func DeriveKeyFromPasswordFile(password string) (aesKey, hmacKey []byte, err error) {
	if password == "" {
		return nil, nil, fmt.Errorf("password cannot be empty")
	}
	var k1, k2 []byte
	k1, k2, err = DeriveKey(password, nil)
	return k1, k2, err
}

// GenerateRandomIV 生成随机初始化向量
func GenerateRandomIV() ([]byte, error) {
	iv := make([]byte, IVSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}
	return iv, nil
}

// DeriveKeyWithCustomSalt 使用自定义盐值派生密钥
func DeriveKeyWithCustomSalt(password string, salt []byte) (aesKey, hmacKey []byte, err error) {
	if len(salt) != SaltSize {
		return nil, nil, fmt.Errorf("invalid salt size: expected %d, got %d", SaltSize, len(salt))
	}

	aesKey, hmacKey, err = DeriveKey(password, salt)
	return aesKey, hmacKey, err
}

// DeriveKeyFromKeyFile 从密钥文件派生密钥
// 密钥文件格式: [32 bytes AES key][64 bytes HMAC key]
func DeriveKeyFromKeyFile(keyData []byte) (aesKey, hmacKey []byte, err error) {
	if len(keyData) < AESKeySize+HMACKeySize {
		return nil, nil, fmt.Errorf("invalid key file size: expected at least %d, got %d", AESKeySize+HMACKeySize, len(keyData))
	}

	aesKey = keyData[:AESKeySize]
	hmacKey = keyData[AESKeySize : AESKeySize+HMACKeySize]

	return aesKey, hmacKey, nil
}

// GenerateKeyFile 生成密钥文件内容
func GenerateKeyFile() ([]byte, error) {
	keyData := make([]byte, AESKeySize+HMACKeySize)
	if _, err := rand.Read(keyData); err != nil {
		return nil, fmt.Errorf("failed to generate key file: %w", err)
	}
	return keyData, nil
}

// DeriveKeyFromPasswordWithIterations 使用指定迭代次数派生密钥（用于兼容性）
// 使用标准 PBKDF2-HMAC-SHA256 算法
func DeriveKeyFromPasswordWithIterations(password string, salt []byte, iterations uint32) (aesKey, hmacKey []byte, err error) {
	if salt == nil {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// 验证迭代次数
	if iterations < 1 {
		return nil, nil, fmt.Errorf("iterations must be at least 1, got %d", iterations)
	}

	// 使用标准 PBKDF2 派生密钥
	// PBKDF2-HMAC-SHA256 是广泛认可的密钥派生算法
	key := pbkdf2.Key([]byte(password), salt, int(iterations), AESKeySize+HMACKeySize, sha256.New)

	aesKey = key[:AESKeySize]
	hmacKey = key[AESKeySize : AESKeySize+HMACKeySize]

	return aesKey, hmacKey, nil
}
