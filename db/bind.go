package db

// ValueBinder rewrites a parameter before it reaches database/sql.
// Drivers that do not implement it leave arguments unchanged.
type ValueBinder interface {
	Bind(v any) any
}

// ValueDecoder rewrites a scanned value before it is stored on a Record.
// Drivers that do not implement it leave values unchanged.
// Decode may receive a ColumnValue when the column type name is known.
// Returning that same ColumnValue means the value was not decoded.
type ValueDecoder interface {
	Decode(v any) any
}

// ColumnValue is a scanned value plus the database type name from the driver.
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
