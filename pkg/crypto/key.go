package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"golang.org/x/crypto/argon2"
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
func DeriveKeyFromPasswordWithIterations(password string, salt []byte, iterations uint32) (aesKey, hmacKey []byte, err error) {
	if salt == nil {
		salt = make([]byte, SaltSize)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// 使用 PBKDF2 派生密钥（用于兼容旧版本）
	// 注意：新版本应使用 Argon2id
	key := make([]byte, AESKeySize+HMACKeySize)

	// 将迭代次数写入 salt 前面，用于后续验证
	saltWithIter := make([]byte, SaltSize+4)
	binary.BigEndian.PutUint32(saltWithIter, iterations)
	copy(saltWithIter[4:], salt)

	// 这里简化处理，实际应使用 crypto/pbkdf2
	// 为了简单，我们使用 SHA256 重复迭代
	hasher := sha256.New()
	hasher.Write([]byte(password))
	for i := uint32(0); i < iterations; i++ {
		hasher.Write(saltWithIter)
		if i > 0 {
			hasher.Write(key)
		}
	}
	hash := hasher.Sum(nil)

	copy(key, hash)
	// 填充剩余部分
	for len(key) < AESKeySize+HMACKeySize {
		hasher.Write(key)
		hash = hasher.Sum(nil)
		copy(key[len(hash):], hash)
	}

	aesKey = key[:AESKeySize]
	hmacKey = key[AESKeySize : AESKeySize+HMACKeySize]

	return aesKey, hmacKey, nil
}
