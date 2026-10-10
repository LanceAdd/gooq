package db

import (
	"fmt"
	"reflect"

	"github.com/gogf/gf/v2/util/gconv"
)

// gooqAssign copies a scanned driver value into a settable destination.
func gooqAssign(dst reflect.Value, src any) error {
	if !dst.IsValid() || !dst.CanSet() {
		return fmt.Errorf("gooq: destination cannot be set")
	}
	if dst.Kind() != reflect.Pointer {
		if !dst.CanAddr() {
			return fmt.Errorf("gooq: destination cannot be addressed")
		}
		return gconv.Scan(src, dst.Addr().Interface())
	}
	if dst.IsNil() {
		dst.Set(reflect.New(dst.Type().Elem()))
	}
	return gconv.Scan(src, dst.Interface())
}
