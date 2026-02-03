package progress

import (
	"sync/atomic"
)

// MockReporter 是用于测试的模拟进度报告器
type MockReporter struct {
	InitCalled     atomic.Int64
	AddCalled      atomic.Int64
	CompleteCalled atomic.Int64
	CloseCalled    atomic.Int64
	AddTotal       atomic.Int64
}

// NewMockReporter 创建新的模拟报告器
func NewMockReporter() *MockReporter {
	return &MockReporter{}
}

// Init 初始化进度报告
func (m *MockReporter) Init(total int64) {
	m.InitCalled.Add(1)
}

// Add 增加已处理的数量
func (m *MockReporter) Add(n int64) {
	m.AddCalled.Add(1)
	m.AddTotal.Add(n)
}

// Complete 标记完成
func (m *MockReporter) Complete() {
	m.CompleteCalled.Add(1)
}

// Close 关闭报告器
func (m *MockReporter) Close() error {
	m.CloseCalled.Add(1)
	return nil
}

// Reset 重置所有计数器
func (m *MockReporter) Reset() {
	m.InitCalled.Store(0)
	m.AddCalled.Store(0)
	m.CompleteCalled.Store(0)
	m.CloseCalled.Store(0)
	m.AddTotal.Store(0)
}
