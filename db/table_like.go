package db

import (
	"context"
	"fmt"

	"github.com/lanceadd/gooq"
)

// CreateTableLike 创建 target，结构复制自 template。语句带 IF NOT EXISTS，重复调用安全。
// 方言必须实现 gooq.TableLikeRenderer；未实现时返回错误，不降级。
func (d *DB) CreateTableLike(ctx context.Context, template, target string) error {
	spec, err := gooq.DialectSpecOf(d.dialect)
	if err != nil {
		return err
	}
	renderer, ok := spec.(gooq.TableLikeRenderer)
	if !ok {
		return fmt.Errorf("gooq: dialect %q does not support CREATE TABLE LIKE", d.dialect)
	}
	rc := gooq.NewRenderContext(d.dialect)
	statement, err := renderer.RenderCreateTableLike(rc, template, target)
	if err != nil {
		return err
	}
	if err = rc.Err(); err != nil {
		return err
	}
	_, err = d.Exec(ctx, statement)
	return err
}
