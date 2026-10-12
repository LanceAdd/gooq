package db

// ValueBinder 在参数进入 database/sql 之前改写它。
// 驱动没有实现这个接口时，参数保持原样。
type ValueBinder interface {
	Bind(v any) any
}

// ValueDecoder 在扫描值写入 Record 之前改写它。
// 驱动没有实现这个接口时，值保持原样。
// 列类型名已知时，Decode 可能收到 ColumnValue。
// 原样返回同一个 ColumnValue 表示这个值没有被解码。
type ValueDecoder interface {
	Decode(v any) any
}

// ColumnValue 是扫描到的值，加上驱动给出的数据库类型名。
type ColumnValue struct {
	DBType string
	Value  any
}

func bindArgs(driver Driver, args []any) []any {
	binder, ok := driver.(ValueBinder)
	if !ok || len(args) == 0 {
		return args
	}
	bound := make([]any, len(args))
	for i, arg := range args {
		bound[i] = binder.Bind(arg)
	}
	return bound
}

func (d *DB) bindArgs(args []any) []any {
	if d == nil {
		return args
	}
	return bindArgs(d.driver, args)
}
