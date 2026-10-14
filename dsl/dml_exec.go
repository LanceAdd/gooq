package dsl

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

func (b *DMLBuilder) Exec(ctx context.Context) (sql.Result, error) {
	if b.err != nil {
		return nil, b.err
	}
	restore, err := b.bind(ctx)
	if err != nil {
		return nil, err
	}
	defer restore()
	dialect := b.dmlDialect()
	if _, err = gooq.DialectSpecOf(dialect); err != nil {
		return nil, err
	}
	if (b.kind == dmlUpdate || b.kind == dmlDelete) && len(b.batchUpdateRows) > 0 {
		sqls, argss, err := b.renderBatchDML(dialect)
		if err != nil {
			return nil, err
		}
		if len(sqls) == 0 {
			return nil, fmt.Errorf("gooq: batch delete/update has no valid records")
		}
		var (
			total      int64
			lastResult sql.Result
		)
		for i, sqlStr := range sqls {
			result, err := b.executor.Exec(ctx, sqlStr, argss[i]...)
			if err != nil {
				return nil, err
			}
			if n, err := result.RowsAffected(); err == nil {
				total += n
			}
			lastResult = result
		}
		return &batchResult{result: lastResult, affected: total}, nil
	}
	// 缺列不补 NULL，留给表默认值。列集合不同拆成多条 INSERT；Batch 在每个列集合内再切分。
	if b.kind == dmlInsert && len(b.insertRows) > 0 {
		chunks := splitInsertChunks(b.insertRows, b.batch)
		if len(chunks) > 1 {
			return b.execInsertChunkList(ctx, dialect, chunks)
		}
	}
	sqlStr, args, err := b.ToSql(dialect)
	if err != nil {
		return nil, err
	}
	return b.executor.Exec(ctx, sqlStr, args...)
}

type batchResult struct {
	result   sql.Result
	affected int64
}

func (r *batchResult) LastInsertId() (int64, error) {
	return r.result.LastInsertId()
}

func (r *batchResult) RowsAffected() (int64, error) {
	return r.affected, nil
}

// Value writes the single RETURNING column of the first row into dest.
// More than one returned column is an error. Several returned rows still yield the first row.
func (b *DMLBuilder) Value(ctx context.Context, dest any) error {
	if err := db.CheckValue(dest); err != nil {
		return err
	}
	return b.readReturning(ctx, func(sqlStr string, args []any) error {
		return execValueExact(ctx, b.executor, sqlStr, args, dest)
	})
}

// Column writes the single RETURNING column of every row into dest.
// More than one returned column is an error.
func (b *DMLBuilder) Column(ctx context.Context, dest any) error {
	if err := db.CheckColumn(dest); err != nil {
		return err
	}
	return b.readReturning(ctx, func(sqlStr string, args []any) error {
		return execColumnExact(ctx, b.executor, sqlStr, args, dest)
	})
}

func (b *DMLBuilder) readReturning(ctx context.Context, run func(string, []any) error) error {
	restore, err := b.bind(ctx)
	if err != nil {
		return err
	}
	defer restore()
	if len(b.returning) == 0 {
		return fmt.Errorf("gooq: Value and Column on DML require Returning fields")
	}
	if len(b.returning) != 1 {
		return fmt.Errorf("gooq: query must select exactly one column")
	}
	sqlStr, args, err := b.ToSql(b.dmlDialect())
	if err != nil {
		return err
	}
	return run(sqlStr, args)
}

func (b *DMLBuilder) dmlDialect() gooq.Dialect {
	if b.executor != nil {
		return autoDialect(b.executor)
	}
	return gooq.DialectMySQL
}

// groupInsertRows 按列集合分组：同组内每行列集一致（可直接渲染，无 NULL 补位）；
// 组间列集不同 → 拆分多条 INSERT（缺失列走表默认值）。
func groupInsertRows(rows [][]columnValue) [][][]columnValue {
	groupMap := make(map[string][][]columnValue)
	var order []string
	for _, row := range rows {
		cols := make([]string, len(row))
		for i, cv := range row {
			cols[i] = cv.column
		}
		sort.Strings(cols)
		key := strings.Join(cols, ",")
		if _, ok := groupMap[key]; !ok {
			order = append(order, key)
		}
		groupMap[key] = append(groupMap[key], row)
	}
	groups := make([][][]columnValue, len(order))
	for i, key := range order {
		groups[i] = groupMap[key]
	}
	return groups
}

// splitInsertChunks 先按列集合分组，再按 batch 切开。batch <= 0 表示不切。
// 每个分片列集合一致，渲染时不会用 NULL 填没写的列。
func splitInsertChunks(rows [][]columnValue, batch int) [][][]columnValue {
	groups := groupInsertRows(rows)
	if batch <= 0 {
		return groups
	}
	chunks := make([][][]columnValue, 0, len(groups))
	for _, group := range groups {
		if len(group) <= batch {
			chunks = append(chunks, group)
			continue
		}
		for start := 0; start < len(group); start += batch {
			end := start + batch
			if end > len(group) {
				end = len(group)
			}
			chunks = append(chunks, group[start:end])
		}
	}
	return chunks
}

func (b *DMLBuilder) execInsertChunkList(ctx context.Context, dialect gooq.Dialect, chunks [][][]columnValue) (sql.Result, error) {
	var (
		total      int64
		lastResult sql.Result
	)
	for _, rows := range chunks {
		chunk := *b
		chunk.insertRows = rows
		sqlStr, args, err := chunk.ToSql(dialect)
		if err != nil {
			return nil, err
		}
		result, err := b.executor.Exec(ctx, sqlStr, args...)
		if err != nil {
			return nil, err
		}
		if n, err := result.RowsAffected(); err == nil {
			total += n
		}
		lastResult = result
	}
	return &batchResult{result: lastResult, affected: total}, nil
}
