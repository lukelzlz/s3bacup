package progress

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Bar 终端进度条实现
type Bar struct {
	bar      *progressbar.ProgressBar
	start    time.Time
	bytes    int64
	lastSize int64
	mu       sync.Mutex
	speed    float64
}

// NewBar 创建新的进度条
func NewBar() *Bar {
	return &Bar{
		start: time.Now(),
	}
}

// Init 初始化进度条
func (b *Bar) Init(total int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.start = time.Now()
	b.bytes = 0
	b.lastSize = 0
	b.speed = 0

	// 未知总数时使用不确定模式
	if total <= 0 {
		total = -1
	}

	b.bar = progressbar.NewOptions64(
		total,
		progressbar.OptionSetDescription("Uploading"),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("B"),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "─",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	// 启动速度计算协程
	go b.updateSpeed()
}

// Add 增加已上传的字节数
func (b *Bar) Add(n int64) {
	if b.bar == nil {
		return
	}
	atomic.AddInt64(&b.bytes, n)
	b.bar.Add64(n)
}

// Complete 标记完成
func (b *Bar) Complete() {
	if b.bar == nil {
		return
	}
	b.bar.Finish()
}

// Close 关闭进度条
func (b *Bar) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.bar == nil {
		return nil
	}
	b.bar.Finish()
	b.bar = nil
	return nil
}

// updateSpeed 定期更新速度显示
func (b *Bar) updateSpeed() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		b.mu.Lock()
		if b.bar == nil {
			b.mu.Unlock()
			return
		}

		current := atomic.LoadInt64(&b.bytes)
		uploaded := current - b.lastSize
		elapsed := time.Since(b.start).Seconds()

		if elapsed > 0 {
			b.speed = float64(uploaded) / elapsed
		}

		b.lastSize = current
		b.mu.Unlock()
	}
}

// Write 实现 io.Writer 接口，用于兼容
func (b *Bar) Write(p []byte) (int, error) {
	n := len(p)
	b.Add(int64(n))
	return n, nil
}

// GetSpeed 获取当前速度（MB/s）
func (b *Bar) GetSpeed() float64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.speed
}

// GetBytes 获取已上传字节数
func (b *Bar) GetBytes() int64 {
	return atomic.LoadInt64(&b.bytes)
}

// GetElapsed 获取已用时间（秒）
func (b *Bar) GetElapsed() float64 {
	return time.Since(b.start).Seconds()
}

// NewBarWithWriter 创建使用自定义 writer 的进度条
func NewBarWithWriter(w io.Writer) *Bar {
	return &Bar{
		start: time.Now(),
	}
}
