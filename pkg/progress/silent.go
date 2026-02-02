package progress

// Silent 静默进度报告器（无操作）
type Silent struct{}

// NewSilent 创建新的静默报告器
func NewSilent() *Silent {
	return &Silent{}
}

// Init 初始化（无操作）
func (s *Silent) Init(total int64) {}

// Add 增加字节数（无操作）
func (s *Silent) Add(n int64) {}

// Complete 标记完成（无操作）
func (s *Silent) Complete() {}

// Close 关闭（无操作）
func (s *Silent) Close() error {
	return nil
}
