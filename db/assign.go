package db

import (
	"fmt"
	"reflect"

	"github.com/lanceadd/gooq"
)

// gooqAssign copies a scanned driver value into a settable destination.
func gooqAssign(dst reflect.Value, src any) error {
	if !dst.IsValid() || !dst.CanSet() {
		return fmt.Errorf("gooq: destination cannot be set")
	}
	if !dst.CanAddr() {
		return fmt.Errorf("gooq: destination cannot be addressed")
	}
	return gooq.Assign(dst.Addr().Interface(), src)
}
