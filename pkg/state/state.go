package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UploadState 上传状态
type UploadState struct {
	Key           string          `json:"key"`
	UploadID      string          `json:"upload_id"`
	Bucket        string          `json:"bucket"`
	Provider      string          `json:"provider"`
	Endpoint      string          `json:"endpoint"`
	Region        string          `json:"region"`
	StorageClass  string          `json:"storage_class"`
	Encrypted     bool            `json:"encrypted"`
	Completed     []CompletedPart `json:"completed"`
	LastUpdated   time.Time       `json:"last_updated"`
	TotalBytes    int64           `json:"total_bytes"`
	UploadedBytes int64           `json:"uploaded_bytes"`
}

// CompletedPart 已完成的分块
type CompletedPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
	Size       int64  `json:"size"`
}

// StateManager 状态管理器
type StateManager struct {
	stateFile string
	state     *UploadState
	mu        sync.RWMutex
}

// NewStateManager 创建状态管理器
func NewStateManager(stateDir string, key string) *StateManager {
	if stateDir == "" {
		home, _ := os.UserHomeDir()
		stateDir = filepath.Join(home, ".s3backup", "state")
	}

	// 创建状态目录
	os.MkdirAll(stateDir, 0755)

	// 生成状态文件名（使用 key 的 hash）
	stateFile := filepath.Join(stateDir, safeFilename(key)+".json")

	return &StateManager{
		stateFile: stateFile,
	}
}

// safeFilename 生成安全的文件名
func safeFilename(key string) string {
	// 简单替换不安全字符
	result := make([]byte, 0, len(key))
	for _, c := range key {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result = append(result, byte(c))
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}

// Load 加载状态
func (sm *StateManager) Load() (*UploadState, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	data, err := os.ReadFile(sm.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // 状态文件不存在，返回 nil
		}
		return nil, err
	}

	var state UploadState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}

	sm.state = &state
	return &state, nil
}

// Save 保存状态
func (sm *StateManager) Save(state *UploadState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	state.LastUpdated = time.Now()
	sm.state = state

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.stateFile, data, 0644)
}

// SaveWithUploadID 保存带 UploadID 的状态
func (sm *StateManager) SaveWithUploadID(uploadID string, state *UploadState) error {
	state.UploadID = uploadID
	return sm.Save(state)
}

// Delete 删除状态
func (sm *StateManager) Delete() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.state = nil
	return os.Remove(sm.stateFile)
}

// AddCompletedPart 添加已完成的分块
func (sm *StateManager) AddCompletedPart(part CompletedPart) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state == nil {
		return nil
	}

	// 检查是否已存在
	for i, p := range sm.state.Completed {
		if p.PartNumber == part.PartNumber {
			sm.state.Completed[i] = part
			sm.state.LastUpdated = time.Now()
			go sm.saveAsync(sm.state)
			return nil
		}
	}

	sm.state.Completed = append(sm.state.Completed, part)
	sm.state.UploadedBytes += part.Size
	sm.state.LastUpdated = time.Now()

	// 异步保存
	go sm.saveAsync(sm.state)

	return nil
}

// saveAsync 异步保存
func (sm *StateManager) saveAsync(state *UploadState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(sm.stateFile, data, 0644)
}

// GetCompletedParts 获取已完成的分块
func (sm *StateManager) GetCompletedParts() map[int]CompletedPart {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.state == nil {
		return nil
	}

	parts := make(map[int]CompletedPart)
	for _, p := range sm.state.Completed {
		parts[p.PartNumber] = p
	}
	return parts
}

// GetState 获取当前状态
func (sm *StateManager) GetState() *UploadState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

// GetStateFile 获取状态文件路径
func (sm *StateManager) GetStateFile() string {
	return sm.stateFile
}
