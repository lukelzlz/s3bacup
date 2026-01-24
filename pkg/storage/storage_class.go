package storage

// StorageClass 存储类型
type StorageClass string

const (
	StorageClassStandard       StorageClass = "STANDARD"
	StorageClassInfrequent     StorageClass = "INFREQUENT_ACCESS"
	StorageClassArchive        StorageClass = "ARCHIVE"
	StorageClassDeepArchive    StorageClass = "DEEP_ARCHIVE"
	StorageClassGlacierIR      StorageClass = "GLACIER_IR"
	StorageClassIntelligentTiering StorageClass = "INTELLIGENT_TIERING"
)

// ParseStorageClass 解析存储类型字符串
func ParseStorageClass(s string) StorageClass {
	switch s {
	case "standard", "STANDARD":
		return StorageClassStandard
	case "ia", "infrequent", "INFREQUENT_ACCESS":
		return StorageClassInfrequent
	case "archive", "ARCHIVE":
		return StorageClassArchive
	case "deep_archive", "DEEP_ARCHIVE":
		return StorageClassDeepArchive
	case "glacier_ir", "GLACIER_IR":
		return StorageClassGlacierIR
	case "intelligent", "INTELLIGENT_TIERING":
		return StorageClassIntelligentTiering
	default:
		return StorageClassStandard
	}
}

// String 返回存储类型的字符串表示
func (sc StorageClass) String() string {
	return string(sc)
}

// IsValid 检查存储类型是否有效
func (sc StorageClass) IsValid() bool {
	switch sc {
	case StorageClassStandard,
		StorageClassInfrequent,
		StorageClassArchive,
		StorageClassDeepArchive,
		StorageClassGlacierIR,
		StorageClassIntelligentTiering:
		return true
	default:
		return false
	}
}
