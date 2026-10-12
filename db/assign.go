package db

import (
	"fmt"
	"reflect"

	"github.com/lanceadd/gooq"
)

// gooqAssign 把扫到的驱动值写入可设置的目标。目标不能取地址时返回错误。
func gooqAssign(dst reflect.Value, src any) error {
	if !dst.IsValid() || !dst.CanSet() {
		return fmt.Errorf("gooq: destination cannot be set")
	}
	if !dst.CanAddr() {
		return fmt.Errorf("gooq: destination cannot be addressed")
	}
	return gooq.Assign(dst.Addr().Interface(), src)
}
