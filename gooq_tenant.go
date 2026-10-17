package gooq

import (
	"fmt"
	"reflect"

	"github.com/google/uuid"
)

// TenantColumnAllowed 报告这个本地类型能不能作为租户列。
// int64-bytes / uint64-bytes 的字段 Go 类型是 int64 / uint64。
// LocalTypeBigInt 只出现在手写元数据里，值同样用 int64。
func TenantColumnAllowed(localType LocalType) bool {
	_, ok := tenantValueType(localType)
	return ok
}

// CheckTenantValue 检查一个运行时值能不能绑定到这列。宽度不同的整数不算同一种类型。
func CheckTenantValue(localType LocalType, column string, value any) error {
	got, ok := tenantValueType(localType)
	if !ok {
		return fmt.Errorf("gooq: tenant column %q has unsupported type %q", column, localType)
	}
	if value == nil {
		return fmt.Errorf("gooq: tenant column %q value is nil", column)
	}
	if reflect.TypeOf(value) != got {
		return fmt.Errorf("gooq: tenant column %q expects %s, got %T", column, got, value)
	}
	return nil
}

func tenantValueType(localType LocalType) (reflect.Type, bool) {
	switch localType {
	case LocalTypeString:
		return reflect.TypeOf(""), true
	case LocalTypeUUID:
		return reflect.TypeOf(uuid.UUID{}), true
	case LocalTypeInt:
		return reflect.TypeOf(int(0)), true
	case LocalTypeUint:
		return reflect.TypeOf(uint(0)), true
	case LocalTypeInt32:
		return reflect.TypeOf(int32(0)), true
	case LocalTypeUint32:
		return reflect.TypeOf(uint32(0)), true
	case LocalTypeInt64, LocalTypeBigInt, LocalTypeInt64Bytes:
		return reflect.TypeOf(int64(0)), true
	case LocalTypeUint64, LocalTypeUint64Bytes:
		return reflect.TypeOf(uint64(0)), true
	default:
		return nil, false
	}
}
