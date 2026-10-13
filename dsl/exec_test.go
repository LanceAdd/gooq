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
	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

func TestExistsFlag(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ok, err := existsFlag(true)
		t.AssertNil(err)
		t.Assert(ok, true)
		ok, err = existsFlag(int64(0))
		t.AssertNil(err)
		t.Assert(ok, false)
		ok, err = existsFlag(float64(1))
		t.AssertNil(err)
		t.Assert(ok, true)
	})
}

type fakeExecutor struct {
	calls   []string
	queries []string
	allRows db.Result
	lastSql string
	dialect gooq.Dialect
}

type constResult int64

func (r constResult) LastInsertId() (int64, error) { return 0, nil }
func (r constResult) RowsAffected() (int64, error) { return int64(r), nil }

func (f *fakeExecutor) Dialect() gooq.Dialect {
	if f.dialect == "" {
		return gooq.DialectMySQL
	}
	return f.dialect
}

func (f *fakeExecutor) GetAll(ctx context.Context, query string, args ...any) (db.Result, error) {
	f.calls = append(f.calls, "GetAll")
	f.lastSql = query
	return f.allRows, nil
}

func (f *fakeExecutor) GetOne(ctx context.Context, query string, args ...any) (db.Record, error) {
	f.calls = append(f.calls, "GetOne")
	return nil, nil
}

func (f *fakeExecutor) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	f.calls = append(f.calls, "Exec")
	f.queries = append(f.queries, query)
	return constResult(1), nil
}

func (f *fakeExecutor) GetValue(ctx context.Context, query string, args ...any) (any, error) {
	f.calls = append(f.calls, "GetValue")
	return nil, nil
}

type scanStructTarget struct {
	Id int64
}

func TestCellDestinationChecks(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var row scanStructTarget
		t.AssertNE(db.CheckValue(&row), nil)
		var id int64
		t.AssertNil(db.CheckValue(&id))
		var ids []int64
		t.AssertNil(db.CheckColumn(&ids))
		var rows []scanStructTarget
		t.AssertNE(db.CheckColumn(&rows), nil)
	})
}

func TestRowOne(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		ctx := context.Background()

		fe := &fakeExecutor{allRows: db.Result{
			db.Record{"id": int64(1), "name": "john"},
		}}
		b := SelectFrom(testUser).Where(testUser.Name.Eq("john"))
		b.executor = fe
		record, err := b.RowOne(ctx)
		t.AssertNil(err)
		t.AssertEQ(db.Get(record, testUser.Id), int64(1))
		t.AssertEQ(db.Get(record, testUser.Name), "john")
		t.AssertEQ(fe.calls, []string{"GetAll"})
		t.Assert(strings.Contains(fe.lastSql, "LIMIT 2"), true)
		t.AssertEQ(b.limit, 0)

		fe = &fakeExecutor{}
		b = SelectFrom(testUser).Where(testUser.Id.Eq(1))
		b.executor = fe
		record, err = b.RowOne(ctx)
		t.AssertNil(err)
		t.AssertNil(record)

		fe = &fakeExecutor{allRows: db.Result{
			db.Record{"id": int64(1)},
			db.Record{"id": int64(2)},
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

func TestExistsBuildErr(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		fe := &fakeExecutor{}
		b := Select(testUser.Status).From(testUser).
			GroupRollup(testUser.Status).
			GroupCube(testUser.Status)
		b.executor = fe
		_, err := b.Exists(context.Background())
		t.AssertNE(err, nil)
		t.Assert(strings.Contains(err.Error(), "GROUP BY extension is already set"), true)
		t.AssertEQ(len(fe.calls), 0)
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
		record, err := Select(testUser.Id, testUser.Name).From(testUser).Where(testUser.Id.Eq(1)).Unscoped().Row(ctx)
		t.AssertNil(err)
		t.AssertNil(record.Struct(&row))
		t.AssertEQ(row.Id, int64(1))
		t.AssertEQ(row.Name, "john")

		err = Select(testUser.Name).From(testUser).DB("missing-group").Value(ctx, &row.Name)
		t.AssertNE(err, nil)

		err = database.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO user(id, name) VALUES (2, 'tx')`)
			if err != nil {
				return err
			}
			var name string
			if err = Select(testUser.Name).From(testUser).Where(testUser.Id.Eq(2)).Unscoped().Value(ctx, &name); err != nil {
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
