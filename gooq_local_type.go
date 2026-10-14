package gooq

// LocalType 是字段值的本地存储类型。
// 它决定字段值在本地如何处理。
type LocalType string

const (
	LocalTypeUndefined    LocalType = ""
	LocalTypeString       LocalType = "string"
	LocalTypeTime         LocalType = "time"
	LocalTypeDate         LocalType = "date"
	LocalTypeDatetime     LocalType = "datetime"
	LocalTypeInt          LocalType = "int"
	LocalTypeUint         LocalType = "uint"
	LocalTypeInt32        LocalType = "int32"
	LocalTypeUint32       LocalType = "uint32"
	LocalTypeInt64        LocalType = "int64"
	LocalTypeUint64       LocalType = "uint64"
	LocalTypeBigInt       LocalType = "bigint"
	LocalTypeIntSlice     LocalType = "[]int"
	LocalTypeUintSlice    LocalType = "[]uint"
	LocalTypeInt32Slice   LocalType = "[]int32"
	LocalTypeUint32Slice  LocalType = "[]uint32"
	LocalTypeInt64Slice   LocalType = "[]int64"
	LocalTypeUint64Slice  LocalType = "[]uint64"
	LocalTypeStringSlice  LocalType = "[]string"
	LocalTypeInt64Bytes   LocalType = "int64-bytes"
	LocalTypeUint64Bytes  LocalType = "uint64-bytes"
	LocalTypeFloat32      LocalType = "float32"
	LocalTypeFloat64      LocalType = "float64"
	LocalTypeDecimal      LocalType = "decimal"
	LocalTypeFloat32Slice LocalType = "[]float32"
	LocalTypeFloat64Slice LocalType = "[]float64"
	LocalTypeBytes        LocalType = "[]byte"
	LocalTypeBytesSlice   LocalType = "[][]byte"
	LocalTypeBool         LocalType = "bool"
	LocalTypeBoolSlice    LocalType = "[]bool"
	LocalTypeJson         LocalType = "json"
	LocalTypeJsonb        LocalType = "jsonb"
	LocalTypeUUID         LocalType = "uuid.UUID"
	LocalTypeUUIDSlice    LocalType = "[]uuid.UUID"
)
