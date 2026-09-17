// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package dsl

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
)

type fakeExecutor struct {
	calls   []string
	allRows gdb.Result
	lastSql string
}

func (f *fakeExecutor) GetAll(ctx context.Context, sql string, args ...any) (gdb.Result, error) {
	f.calls = append(f.calls, "GetAll")
	f.lastSql = sql
	return f.allRows, nil
}

func (f *fakeExecutor) GetOne(ctx context.Context, sql string, args ...any) (gdb.Record, error) {
	f.calls = append(f.calls, "GetOne")
	return nil, nil
}

func (f *fakeExecutor) GetScan(ctx context.Context, pointer any, sql string, args ...any) error {
	f.calls = append(f.calls, "GetScan")
	return nil
}

func (f *fakeExecutor) GetArray(ctx context.Context, sql string, args ...any) (gdb.Array, error) {
	f.calls = append(f.calls, "GetArray")
	return nil, nil
}

func (f *fakeExecutor) Exec(ctx context.Context, sql string, args ...any) (sql.Result, error) {
	f.calls = append(f.calls, "Exec")
	return nil, nil
}

func (f *fakeExecutor) GetValue(ctx context.Context, sql string, args ...any) (gdb.Value, error) {
	f.calls = append(f.calls, "GetValue")
	return gvar.New(nil), nil
}

type scanStructTarget struct {
	Id int64
}

func TestScanExec_Routing(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()

		// **T：单行扫描走 GetScan（gdb 原生支持 **T）。
		fe := &fakeExecutor{}
		var record *scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &record))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		// 值结构体走 GetScan。
		fe = &fakeExecutor{}
		var value scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &value))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		// 结构体切片走 GetScan。
		fe = &fakeExecutor{}
		var list []*scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &list))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		// 标量走 GetValue。
		fe = &fakeExecutor{}
		var count int64
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &count))
		t.AssertEQ(fe.calls, []string{"GetValue"})
	})
}

func TestRowOne(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()

		fe := &fakeExecutor{allRows: gdb.Result{
			gdb.Record{"id": gvar.New(int64(1)), "name": gvar.New("john")},
		}}
		b := SelectFrom(testUser).Where(testUser.Name.Eq("john"))
		b.executor = fe
		record, err := b.RowOne(ctx)
		t.AssertNil(err)
		t.AssertEQ(gooq.Get(record, testUser.Id), int64(1))
		t.AssertEQ(gooq.Get(record, testUser.Name), "john")
		t.AssertEQ(fe.calls, []string{"GetAll"})
		t.Assert(strings.Contains(fe.lastSql, "LIMIT 2"), true)
		t.AssertEQ(b.limit, 0)

		fe = &fakeExecutor{}
		b = SelectFrom(testUser).Where(testUser.Id.Eq(1))
		b.executor = fe
		record, err = b.RowOne(ctx)
		t.AssertNil(err)
		t.AssertNil(record)

		fe = &fakeExecutor{allRows: gdb.Result{
			gdb.Record{"id": gvar.New(int64(1))},
			gdb.Record{"id": gvar.New(int64(2))},
		}}
		b = SelectFrom(testUser).Where(testUser.Name.Eq("john"))
		b.executor = fe
		_, err = b.RowOne(ctx)
		t.AssertNE(err, nil)

		fe = &fakeExecutor{}
		b = SelectFrom(testUser).Limit(1)
		b.executor = fe
		_, err = b.RowOne(ctx)
		t.AssertNE(err, nil)
		t.AssertEQ(len(fe.calls), 0)

		fe = &fakeExecutor{}
		b = SelectFrom(testUser).PageCache(CacheOption{Duration: time.Minute})
		b.executor = fe
		_, err = b.RowOne(ctx)
		t.AssertNE(err, nil)

		fe = &fakeExecutor{}
		b = SelectFrom(testUser).Cache(CacheOption{Duration: time.Minute})
		b.executor = fe
		_, err = b.RowOne(ctx)
		t.AssertNE(err, nil)

		_, err = SelectFrom(testUser).RowOne(ctx)
		t.AssertNE(err, nil)
	})
}
