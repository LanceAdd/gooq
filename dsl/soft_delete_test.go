package dsl

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceadd/gooq"
)

func softTable(localType gooq.LocalType) *gooq.TableBase {
	return gooq.NewTableBase(&gooq.TableMeta{
		TableName: "item",
		Fields: []gooq.FieldMeta{
			{ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true},
			{ColumnName: "deleted", LocalType: localType, SoftDelete: true},
		},
	})
}

func TestSoftDeleteValueAndPredicate(t *testing.T) {
	sqlText, args, err := Delete(softTable(gooq.LocalTypeInt)).Where(gooq.NewFieldAt[int64](softTable(gooq.LocalTypeInt), "id").Eq(int64(1))).ToSql(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `UPDATE "item" SET "deleted" = ? WHERE ("item"."id" = ?)` || len(args) != 2 || args[0] != 1 {
		t.Fatalf("int sql %s args %#v", sqlText, args)
	}
	read, _, err := Select(gooq.Raw("id")).From(softTable(gooq.LocalTypeInt)).ToSql(gooq.DialectSQLite)
	if err != nil || !strings.Contains(read, `"deleted" = 0`) {
		t.Fatalf("int read %s %v", read, err)
	}

	sqlText, args, err = Delete(softTable(gooq.LocalTypeBool)).Where(gooq.NewFieldAt[int64](softTable(gooq.LocalTypeBool), "id").Eq(int64(1))).ToSql(gooq.DialectPgsql)
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `UPDATE "item" SET "deleted" = $1 WHERE ("item"."id" = $2)` || len(args) != 2 || args[0] != true {
		t.Fatalf("bool sql %s args %#v", sqlText, args)
	}
	read, _, err = Select(gooq.Raw("id")).From(softTable(gooq.LocalTypeBool)).ToSql(gooq.DialectPgsql)
	if err != nil || !strings.Contains(read, `"deleted" = false`) || strings.Contains(read, "= 0") {
		t.Fatalf("bool read %s %v", read, err)
	}

	sqlText, args, err = Delete(softTable(gooq.LocalTypeDatetime)).Where(gooq.NewFieldAt[int64](softTable(gooq.LocalTypeDatetime), "id").Eq(int64(1))).ToSql(gooq.DialectSQLite)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := args[0].(time.Time); !ok {
		t.Fatalf("datetime arg %#v", args[0])
	}
	read, _, err = Select(gooq.Raw("id")).From(softTable(gooq.LocalTypeDatetime)).ToSql(gooq.DialectSQLite)
	if err != nil || !strings.Contains(read, `"deleted" IS NULL`) {
		t.Fatalf("time read %s %v", read, err)
	}

	_, _, err = Delete(softTable(gooq.LocalTypeString)).Where(gooq.NewFieldAt[int64](softTable(gooq.LocalTypeString), "id").Eq(int64(1))).ToSql(gooq.DialectSQLite)
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("string err %v", err)
	}
}
