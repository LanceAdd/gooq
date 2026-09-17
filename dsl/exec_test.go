// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package dsl

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/test/gtest"
)

type fakeExecutor struct {
	calls []string
}

func (f *fakeExecutor) GetAll(ctx context.Context, sql string, args ...any) (gdb.Result, error) {
	f.calls = append(f.calls, "GetAll")
	return nil, nil
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
