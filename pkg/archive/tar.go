package archive

import (
	"archive/tar"
	"io"
	"time"
)

// TarWriter tar 写入器包装
type TarWriter struct {
	*tar.Writer
}

// NewTarWriter 创建 tar 写入器
func NewTarWriter(w io.Writer) *TarWriter {
	return &TarWriter{Writer: tar.NewWriter(w)}
}

// TarHeader tar 头部包装
type TarHeader struct {
	Name       string
	Mode       int64
	Uid        int
	Gid        int
	Size       int64
	ModTime    time.Time
	Typeflag   byte
	Linkname   string
	Uname      string
	Gname      string
	Devmajor   int64
	Devminor   int64
	AccessTime time.Time
	ChangeTime time.Time
	Xattrs     map[string]string
}

// WriteHeader 写入 tar 头部
func (tw *TarWriter) WriteHeader(hdr *TarHeader) error {
	return tw.Writer.WriteHeader(&tar.Header{
		Name:       hdr.Name,
		Mode:       hdr.Mode,
		Uid:        hdr.Uid,
		Gid:        hdr.Gid,
		Size:       hdr.Size,
		ModTime:    hdr.ModTime,
		Typeflag:   hdr.Typeflag,
		Linkname:   hdr.Linkname,
		Uname:      hdr.Uname,
		Gname:      hdr.Gname,
		Devmajor:   hdr.Devmajor,
		Devminor:   hdr.Devminor,
		AccessTime: hdr.AccessTime,
		ChangeTime: hdr.ChangeTime,
		Xattrs:     hdr.Xattrs,
	})
}

// 文件类型常量
const (
	TypeReg  = tar.TypeReg  // 普通文件
	TypeLink = tar.TypeLink // 硬链接
	TypeDir  = tar.TypeDir  // 目录
)
