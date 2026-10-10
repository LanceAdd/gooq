// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package dsl

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/test/gtest"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

type fakeExecutor struct {
	calls   []string
	allRows gooq.Result
	lastSql string
	dialect gooq.Dialect
}

func (f *fakeExecutor) Dialect() gooq.Dialect {
	if f.dialect == "" {
		return gooq.DialectMySQL
	}
	return f.dialect
}

func (f *fakeExecutor) GetAll(ctx context.Context, query string, args ...any) (gooq.Result, error) {
	f.calls = append(f.calls, "GetAll")
	f.lastSql = query
	return f.allRows, nil
}

func (f *fakeExecutor) GetOne(ctx context.Context, query string, args ...any) (gooq.Record, error) {
	f.calls = append(f.calls, "GetOne")
	return nil, nil
}

func (f *fakeExecutor) GetScan(ctx context.Context, dest any, query string, args ...any) error {
	f.calls = append(f.calls, "GetScan")
	return nil
}

func (f *fakeExecutor) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.calls = append(f.calls, "Exec")
	return nil, nil
}

func (f *fakeExecutor) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	f.calls = append(f.calls, "GetValue")
	return nil, nil
}

type scanStructTarget struct {
	Id int64
}

func TestScanExec_Routing(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()

		fe := &fakeExecutor{}
		var record *scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &record))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		fe = &fakeExecutor{}
		var value scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &value))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		fe = &fakeExecutor{}
		var list []*scanStructTarget
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &list))
		t.AssertEQ(fe.calls, []string{"GetScan"})

		fe = &fakeExecutor{}
		var count int64
		t.AssertNil(scanExec(ctx, fe, "select 1", nil, &count))
		t.AssertEQ(fe.calls, []string{"GetValue"})
	})
}

func TestRowOne(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()

		fe := &fakeExecutor{allRows: gooq.Result{
			gooq.Record{"id": int64(1), "name": "john"},
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

		fe = &fakeExecutor{allRows: gooq.Result{
			gooq.Record{"id": int64(1)},
			gooq.Record{"id": int64(2)},
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

func TestScanUsesDefaultInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "default.db")
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()
		err := db.SetConfig(db.DefaultGroupName, db.Config{Link: "sqlite::@file(" + path + ")"})
		t.AssertNil(err)
		defer func() {
			t.AssertNil(db.RemoveConfig(db.DefaultGroupName))
		}()

		database, err := db.Instance()
		t.AssertNil(err)
		_, err = database.Exec(ctx, `CREATE TABLE user (id INTEGER PRIMARY KEY, name TEXT)`)
		t.AssertNil(err)
		_, err = database.Exec(ctx, `INSERT INTO user(id, name) VALUES (1, 'john')`)
		t.AssertNil(err)

		var row struct {
			Id   int64
			Name string
		}
		err = Select(testUser.Id, testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).Unscoped().Scan(ctx, &row)
		t.AssertNil(err)
		t.AssertEQ(row.Id, int64(1))
		t.AssertEQ(row.Name, "john")

		err = Select(testUser.Name).From(testUser).DB("missing-group").Scan(ctx, &row.Name)
		t.AssertNE(err, nil)

		err = database.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO user(id, name) VALUES (2, 'tx')`)
			if err != nil {
				return err
			}
			var name string
			if err = Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(2)).Unscoped().Scan(ctx, &name); err != nil {
				return err
			}
			if name != "tx" {
				return errName(name)
			}
			return nil
		})
		t.AssertNil(err)
	})
}

type errName string

func (e errName) Error() string {
	return "unexpected name " + string(e)
}
