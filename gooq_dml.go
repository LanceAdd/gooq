// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

// 本文件实现写操作 DSL：Insert/Update/Delete 与 Upsert。
package gooq

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/util/gconv"
)

// dmlKind 是写操作类型。
type dmlKind int

const (
	dmlInsert dmlKind = iota
	dmlInsertFrom
	dmlUpdate
	dmlDelete
)

// upsertClause 是 Upsert 子句。
type upsertClause struct {
	conflictCols []string
	updateMap    map[string]any
}

// DMLBuilder 是写操作构建器：Insert/Update/Delete。
type DMLBuilder struct {
	db            gdb.DB
	ctx           context.Context
	table         Table
	kind          dmlKind
	data          map[string]any
	dataList      []map[string]any // 批量 INSERT 数据。
	selectBuilder *SelectBuilder   // INSERT ... SELECT 数据源。
	conditions    []Expression
	unscoped      bool
	upsert        *upsertClause
}

// Insert 创建 INSERT 构建器（data 为 entity/map/do 对象，或 []map/[]struct 批量数据）。
func Insert(t Table, data any) *DMLBuilder {
	b := &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlInsert,
		data:  make(map[string]any),
	}
	switch v := data.(type) {
	case []map[string]any:
		for _, row := range v {
			b.dataList = append(b.dataList, row)
		}
	case []any:
		for _, row := range v {
			b.dataList = append(b.dataList, gconv.Map(row))
		}
	default:
		if isSliceValue(v) {
			for _, row := range toSlice(v) {
				b.dataList = append(b.dataList, gconv.Map(row))
			}
		} else {
			b.data = gconv.Map(v)
		}
	}
	return b
}

// InsertFrom 创建 INSERT ... SELECT 构建器（从查询结果插入）。
func InsertFrom(t Table, sub *SelectBuilder) *DMLBuilder {
	return &DMLBuilder{
		ctx:           context.Background(),
		table:         t,
		kind:          dmlInsertFrom,
		selectBuilder: sub,
	}
}

// isSliceValue 判断值是否为切片。
func isSliceValue(v any) bool {
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
}

// toSlice 将切片值转为 []any。
func toSlice(v any) []any {
	rv := reflect.ValueOf(v)
	result := make([]any, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result
}

// Update 创建 UPDATE 构建器。
func Update(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlUpdate,
		data:  make(map[string]any),
	}
}

// Delete 创建 DELETE 构建器（软删表默认转为 UPDATE deleted_at）。
func Delete(t Table) *DMLBuilder {
	return &DMLBuilder{
		ctx:   context.Background(),
		table: t,
		kind:  dmlDelete,
	}
}

// DB 绑定显式实例。
func (b *DMLBuilder) DB(db gdb.DB) *DMLBuilder {
	b.db = db
	return b
}

// Ctx 设置上下文。
func (b *DMLBuilder) Ctx(ctx context.Context) *DMLBuilder {
	b.ctx = ctx
	return b
}

// Data 设置数据（entity/map/do 对象）。
func (b *DMLBuilder) Data(data any) *DMLBuilder {
	for k, v := range gconv.Map(data) {
		b.data[k] = v
	}
	return b
}

// Set 类型化设置单列值（接受任意 Field[T]）。
func (b *DMLBuilder) Set(field interface{ ColumnName() string }, v any) *DMLBuilder {
	b.data[field.ColumnName()] = v
	return b
}

// Where 追加条件（默认 AND）。
func (b *DMLBuilder) Where(conds ...Expression) *DMLBuilder {
	b.conditions = append(b.conditions, conds...)
	return b
}

// Unscoped 绕过软删除（Delete 变为真 DELETE）。
func (b *DMLBuilder) Unscoped() *DMLBuilder {
	b.unscoped = true
	return b
}

// OnConflictKey 设置 Upsert 冲突目标列（PG 语义要求唯一索引）。
func (b *DMLBuilder) OnConflictKey(fields ...interface{ ColumnName() string }) *DMLBuilder {
	if b.upsert == nil {
		b.upsert = &upsertClause{updateMap: make(map[string]any)}
	}
	for _, f := range fields {
		b.upsert.conflictCols = append(b.upsert.conflictCols, f.ColumnName())
	}
	return b
}

// DoUpdate 设置 Upsert 冲突后的更新列。
func (b *DMLBuilder) DoUpdate(field interface{ ColumnName() string }, v any) *DMLBuilder {
	if b.upsert == nil {
		b.upsert = &upsertClause{updateMap: make(map[string]any)}
	}
	b.upsert.updateMap[field.ColumnName()] = v
	return b
}

// resolveDB 返回执行实例。
func (b *DMLBuilder) resolveDB() (gdb.DB, error) {
	if b.db != nil {
		return b.db, nil
	}
	db, err := gdb.Instance()
	if err != nil {
		return nil, err
	}
	return db, nil
}

// ToSql 离线渲染写操作 SQL（不连库）。
func (b *DMLBuilder) ToSql(dialect Dialect) (string, []any, error) {
	if dialect == "" {
		dialect = DialectMySQL
	}
	rc := newRenderContext(b.ctx, nil, dialect)
	switch b.kind {
	case dmlInsert:
		return b.renderInsert(rc)
	case dmlInsertFrom:
		return b.renderInsertFrom(rc)
	case dmlUpdate:
		return b.renderUpdate(rc)
	case dmlDelete:
		return b.renderDelete(rc)
	default:
		return "", nil, fmt.Errorf("gooq: unknown dml kind %d", b.kind)
	}
}

// renderInsertFrom 渲染 INSERT ... SELECT。
func (b *DMLBuilder) renderInsertFrom(rc *renderContext) (string, []any, error) {
	// 列清单：优先子查询字段的列名，否则表全列。
	var columns []string
	if b.selectBuilder != nil && len(b.selectBuilder.fields) > 0 {
		for _, f := range b.selectBuilder.fields {
			if field, ok := f.(interface{ ColumnName() string }); ok {
				columns = append(columns, field.ColumnName())
			}
		}
	}
	if len(columns) == 0 {
		columns = b.table.AllColumns()
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert-from columns is empty")
	}
	subSQL, subArgs := b.selectBuilder.renderSelect(rc)
	var sqlStr = fmt.Sprintf(
		"INSERT INTO %s (%s) %s",
		b.table.TableName(),
		strings.Join(columns, ", "),
		subSQL,
	)
	return sqlStr, subArgs, nil
}

// renderInsert 渲染 INSERT（含批量与 Upsert，跳过自增列；列名排序保证确定性输出）。
func (b *DMLBuilder) renderInsert(rc *renderContext) (string, []any, error) {
	if len(b.dataList) > 0 {
		return b.renderInsertBatch(rc)
	}
	var (
		columns  []string
		autoIncr = autoIncrementColumn(b.table)
	)
	for col := range b.data {
		if col == autoIncr {
			continue
		}
		columns = append(columns, col)
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert data is empty")
	}
	sort.Strings(columns)
	values := make([]string, len(columns))
	args := make([]any, len(columns))
	for i, col := range columns {
		values[i] = rc.addArg(b.data[col])
		args[i] = b.data[col]
	}
	var sqlStr = fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		b.table.TableName(),
		strings.Join(columns, ", "),
		strings.Join(values, ", "),
	)
	if b.upsert != nil {
		sqlStr += renderUpsertClause(rc, b.upsert, rc.dialect)
	}
	return sqlStr, args, nil
}

// renderInsertBatch 渲染批量 INSERT：列 = 各行键并集，缺失列补 nil。
func (b *DMLBuilder) renderInsertBatch(rc *renderContext) (string, []any, error) {
	var (
		columnSet = make(map[string]bool)
		autoIncr  = autoIncrementColumn(b.table)
	)
	for _, row := range b.dataList {
		for col := range row {
			if col != autoIncr {
				columnSet[col] = true
			}
		}
	}
	columns := make([]string, 0, len(columnSet))
	for col := range columnSet {
		columns = append(columns, col)
	}
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("gooq: insert batch data is empty")
	}
	sort.Strings(columns)
	var (
		rows []string
		args []any
	)
	for _, row := range b.dataList {
		placeholders := make([]string, len(columns))
		for i, col := range columns {
			placeholders[i] = rc.addArg(row[col])
			args = append(args, row[col])
		}
		rows = append(rows, "("+strings.Join(placeholders, ", ")+")")
	}
	var sqlStr = fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES %s",
		b.table.TableName(),
		strings.Join(columns, ", "),
		strings.Join(rows, ", "),
	)
	if b.upsert != nil {
		sqlStr += renderUpsertClause(rc, b.upsert, rc.dialect)
	}
	return sqlStr, args, nil
}

// renderUpdate 渲染 UPDATE。
func (b *DMLBuilder) renderUpdate(rc *renderContext) (string, []any, error) {
	var (
		sets []string
		args []any
	)
	for col, val := range b.data {
		sets = append(sets, fmt.Sprintf(`%s = %s`, col, rc.addArg(val)))
		args = append(args, val)
	}
	if len(sets) == 0 {
		return "", nil, fmt.Errorf("gooq: update data is empty")
	}
	var sqlStr = fmt.Sprintf("UPDATE %s SET %s", b.table.TableName(), strings.Join(sets, ", "))
	if where, whereArgs := b.renderWhere(rc); where != "" {
		sqlStr += " WHERE " + where
		args = append(args, whereArgs...)
	}
	return sqlStr, args, nil
}

// renderDelete 渲染删除；软删表默认转为 UPDATE deleted_at，Unscoped 时真 DELETE。
func (b *DMLBuilder) renderDelete(rc *renderContext) (string, []any, error) {
	if !b.unscoped && b.table.Meta() != nil {
		if softField := b.table.Meta().SoftDeleteField(); softField != nil {
			softCol := softField.ColumnName
			now := time.Now()
			sqlStr := fmt.Sprintf(
				"UPDATE %s SET %s = %s",
				b.table.TableName(),
				softCol,
				rc.addArg(now),
			)
			var args = []any{now}
			if where, whereArgs := b.renderWhere(rc); where != "" {
				sqlStr += " WHERE " + where
				args = append(args, whereArgs...)
			}
			return sqlStr, args, nil
		}
	}
	var sqlStr = "DELETE FROM " + b.table.TableName()
	var args []any
	if where, whereArgs := b.renderWhere(rc); where != "" {
		sqlStr += " WHERE " + where
		args = whereArgs
	}
	return sqlStr, args, nil
}

// Insert 执行 INSERT（含 Upsert）。
func (b *DMLBuilder) Insert() (sql.Result, error) {
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sqlStr, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	return db.Exec(b.ctx, sqlStr, args...)
}

// Update 执行 UPDATE（返回影响行数）。
func (b *DMLBuilder) Update() (sql.Result, error) {
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sqlStr, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	return db.Exec(b.ctx, sqlStr, args...)
}

// Delete 执行删除；软删表默认转为 UPDATE deleted_at，Unscoped 时真 DELETE。
func (b *DMLBuilder) Delete() (sql.Result, error) {
	db, err := b.resolveDB()
	if err != nil {
		return nil, err
	}
	sqlStr, args, err := b.ToSql(dialectOf(db))
	if err != nil {
		return nil, err
	}
	return db.Exec(b.ctx, sqlStr, args...)
}

// renderWhere 渲染条件（空返回空串）。
func (b *DMLBuilder) renderWhere(rc *renderContext) (string, []any) {
	var (
		parts []string
		args  []any
	)
	for _, c := range b.conditions {
		condSQL, condArgs := rc.render(c)
		if condSQL != "" {
			parts = append(parts, condSQL)
			args = append(args, condArgs...)
		}
	}
	return strings.Join(parts, " AND "), args
}

// autoIncrementColumn 返回自增列名（无则空串）。
func autoIncrementColumn(t Table) string {
	if t.Meta() == nil {
		return ""
	}
	for i := range t.Meta().Fields {
		if t.Meta().Fields[i].AutoIncrement {
			return t.Meta().Fields[i].ColumnName
		}
	}
	return ""
}

// renderUpsertClause 按方言渲染 Upsert 子句。
func renderUpsertClause(rc *renderContext, clause *upsertClause, dialect Dialect) string {
	switch dialect {
	case DialectPgsql:
		var sets []string
		for col, val := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = %s`, col, rc.addArg(val)))
		}
		return fmt.Sprintf(
			" ON CONFLICT (%s) DO UPDATE SET %s",
			strings.Join(clause.conflictCols, ", "),
			strings.Join(sets, ", "),
		)
	default:
		var sets []string
		for col := range clause.updateMap {
			sets = append(sets, fmt.Sprintf(`%s = VALUES(%s)`, col, col))
		}
		return " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}
}
