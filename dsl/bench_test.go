package dsl

import (
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/fn"
)

// 渲染分配基线：三档覆盖 基础 SELECT / 条件+JOIN+函数 / 批量 INSERT。
// 改动渲染热点时对照 ns/op 与 allocs/op（断言保证输出逐字节不变）。

func BenchmarkSelect_Basic(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Select(testUser.ID, testUser.Name, testUser.Age).From(testUser).
			Where(testUser.Age.Gt(18)).
			Order(testUser.ID.Desc()).
			Limit(10).
			ToSql(gooq.DialectMySQL)
	}
}

func BenchmarkSelect_Complex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Select(testUser.Status, fn.Count(testUser.ID).As("cnt")).
			From(testUser).
			InnerJoin(testUserRole).On(testUserRole.UserID.EqExpr(testUser.ID)).
			Where(testUser.Age.Gt(18)).Where(testUser.Name.Like("j%")).
			Group(testUser.Status).
			Having(gooq.Gt(fn.Count(testUser.ID), 2)).
			Order(testUser.Status.Asc()).
			ToSql(gooq.DialectPgsql)
	}
}

func BenchmarkInsert_Batch(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = Insert(testUser).
			Columns(testUser.Name, testUser.Age).
			Values("a", 1).Values("b", 2).Values("c", 3).
			ToSql(gooq.DialectMySQL)
	}
}
