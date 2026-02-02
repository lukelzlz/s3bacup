package progress

// Reporter 进度报告接口
type Reporter interface {
	// Init 初始化进度报告，total 为总字节数（如果未知则传 0）
	Init(total int64)

	// Add 增加已处理的字节数
	Add(n int64)

	// Complete 标记完成
	Complete()

	// Close 关闭报告器
	Close() error
}
