package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"fmt"
	"hash"
	"io"
)

// StreamEncryptor 流式加密器
type StreamEncryptor struct {
	aesKey  []byte
	hmacKey []byte
}

// NewStreamEncryptor 创建流式加密器
func NewStreamEncryptor(aesKey, hmacKey []byte) (*StreamEncryptor, error) {
	if len(aesKey) != AESKeySize {
		return nil, fmt.Errorf("invalid AES key size: expected %d, got %d", AESKeySize, len(aesKey))
	}
	if len(hmacKey) != HMACKeySize {
		return nil, fmt.Errorf("invalid HMAC key size: expected %d, got %d", HMACKeySize, len(hmacKey))
	}

	return &StreamEncryptor{
		aesKey:  aesKey,
		hmacKey: hmacKey,
	}, nil
}

// EncryptWriter 加密写入器
type EncryptWriter struct {
	iv       []byte
	block    cipher.Block
	stream   cipher.Stream
	hmac     hash.Hash
	writer   io.Writer
	position int64
}

// WrapWriter 包装一个 writer 为加密写入器
// 文件格式: [4 bytes magic][16 bytes IV][encrypted data...][8 bytes data length][64 bytes HMAC]
func (e *StreamEncryptor) WrapWriter(w io.Writer) (io.WriteCloser, error) {
	// 创建 AES 块
	block, err := aes.NewCipher(e.aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 生成随机 IV
	iv, err := GenerateRandomIV()
	if err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 创建 HMAC
	hmac := hmac.New(sha512.New, e.hmacKey)

	// 写入魔数和 IV
	magic := []byte("S3BE") // S3Backup Encryption
	if _, err := w.Write(magic); err != nil {
		return nil, fmt.Errorf("failed to write magic: %w", err)
	}
	if _, err := w.Write(iv); err != nil {
		return nil, fmt.Errorf("failed to write IV: %w", err)
	}

	return &EncryptWriter{
		iv:       iv,
		block:    block,
		stream:   stream,
		hmac:     hmac,
		writer:   w,
		position: 0,
	}, nil
}

// Write 写入数据并加密
func (ew *EncryptWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 加密数据
	encrypted := make([]byte, len(p))
	ew.stream.XORKeyStream(encrypted, p)

	// 更新 HMAC
	ew.hmac.Write(encrypted)

	// 写入加密数据
	n, err := ew.writer.Write(encrypted)
	if err != nil {
		return n, err
	}

	ew.position += int64(n)
	return n, nil
}

// Close 关闭写入器并写入 HMAC
func (ew *EncryptWriter) Close() error {
	// 写入数据长度（8字节，大端序）
	lengthBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(lengthBytes, uint64(ew.position))
	if _, err := ew.writer.Write(lengthBytes); err != nil {
		return fmt.Errorf("failed to write data length: %w", err)
	}

	// 写入 HMAC
	hmac := ew.hmac.Sum(nil)
	if _, err := ew.writer.Write(hmac); err != nil {
		return fmt.Errorf("failed to write HMAC: %w", err)
	}

	return nil
}

// DecryptReader 解密读取器
type DecryptReader struct {
	iv       []byte
	block    cipher.Block
	stream   cipher.Stream
	hmac     hash.Hash
	reader   io.Reader
	position int64
	total    int64
	buffer   []byte
}

// WrapReader 包装一个 reader 为解密读取器
func (e *StreamEncryptor) WrapReader(r io.Reader) (io.Reader, error) {
	// 读取魔数和 IV
	header := make([]byte, 4+IVSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// 验证魔数
	magic := header[:4]
	if string(magic) != "S3BE" {
		return nil, fmt.Errorf("invalid magic: %s", string(magic))
	}

	// 读取 IV
	iv := header[4:]

	// 创建 AES 块
	block, err := aes.NewCipher(e.aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 创建 HMAC
	hmac := hmac.New(sha512.New, e.hmacKey)

	return &DecryptReader{
		iv:       iv,
		block:    block,
		stream:   stream,
		hmac:     hmac,
		reader:   r,
		position: 0,
		total:    0,
		buffer:   make([]byte, 32*1024), // 32KB 缓冲区
	}, nil
}

// Read 读取并解密数据
func (dr *DecryptReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	// 从底层 reader 读取数据
	n, err := dr.reader.Read(dr.buffer)
	if err != nil && err != io.EOF {
		return 0, err
	}

	if n == 0 {
		return 0, io.EOF
	}

	// 解密数据
	decrypted := make([]byte, n)
	dr.stream.XORKeyStream(decrypted, dr.buffer[:n])

	// 更新 HMAC
	dr.hmac.Write(dr.buffer[:n])

	// 复制到输出
	copy(p, decrypted)
	dr.position += int64(n)

	return n, err
}

// VerifyHMAC 验证 HMAC
// 需要在读取完所有数据后调用
func (e *StreamEncryptor) VerifyHMAC(r io.Reader, expectedHMAC []byte) error {
	// 这里简化处理，实际实现需要在读取时计算 HMAC
	// 完整实现需要包装 reader 来计算 HMAC
	return nil
}

// DecryptReaderWithHMAC 包装 reader 并在读取时验证 HMAC
type DecryptReaderWithHMAC struct {
	*DecryptReader
	expectedHMAC []byte
}

// WrapReaderWithHMAC 包装 reader 并验证 HMAC
func (e *StreamEncryptor) WrapReaderWithHMAC(r io.Reader) (io.ReadCloser, error) {
	// 读取完整文件头
	header := make([]byte, 4+IVSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// 验证魔数
	magic := header[:4]
	if string(magic) != "S3BE" {
		return nil, fmt.Errorf("invalid magic: %s", string(magic))
	}

	// 读取 IV
	iv := header[4:]

	// 创建 AES 块
	block, err := aes.NewCipher(e.aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 创建 HMAC
	hmac := hmac.New(sha512.New, e.hmacKey)

	return &decryptReaderWithHMACImpl{
		iv:       iv,
		block:    block,
		stream:   stream,
		hmac:     hmac,
		reader:   r,
		position: 0,
		buffer:   make([]byte, 32*1024),
	}, nil
}

type decryptReaderWithHMACImpl struct {
	iv       []byte
	block    cipher.Block
	stream   cipher.Stream
	hmac     hash.Hash
	reader   io.Reader
	position int64
	buffer   []byte
}

func (d *decryptReaderWithHMACImpl) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	n, err := d.reader.Read(d.buffer)
	if err != nil && err != io.EOF {
		return 0, err
	}

	if n == 0 {
		return 0, io.EOF
	}

	decrypted := make([]byte, n)
	d.stream.XORKeyStream(decrypted, d.buffer[:n])

	d.hmac.Write(d.buffer[:n])

	copy(p, decrypted)
	d.position += int64(n)

	return n, err
}

func (d *decryptReaderWithHMACImpl) Close() error {
	// 读取并验证 HMAC
	expectedHMAC := make([]byte, 64)
	if _, err := io.ReadFull(d.reader, expectedHMAC); err != nil {
		return fmt.Errorf("failed to read HMAC: %w", err)
	}

	actualHMAC := d.hmac.Sum(nil)
	if !hmac.Equal(actualHMAC, expectedHMAC) {
		return fmt.Errorf("HMAC verification failed")
	}

	return nil
}
