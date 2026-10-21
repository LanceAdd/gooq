English | [简体中文](README.zh_CN.md)

# gooq

`gooq` is a type-safe SQL query DSL for [GoFrame](https://goframe.org), inspired by [jOOQ](https://www.jooq.org/).

- **Type-safe** — `Field[T]` carries the column type at compile time: `User.Age.Gt(18)` compiles, `User.Age.Gt("x")` doesn't.
- **Offline rendering** — `ToSql(dialect)` produces SQL + args anywhere, without a database connection; execute with the standard library or `gooq/db`.
- **Dialect aware** — MySQL / MariaDB / TiDB / OceanBase / PostgreSQL / GaussDB / Dameng / SQLite each implement `DialectSpec`. A new dialect registers the whole spec. Connection drivers register separately through `db.Register`.
- **Zero runtime guessing** — soft-delete conditions and the full column list come from static `TableMeta` written at codegen time. INSERT and UPDATE write only columns that were set.
- **Layered packages** — `gooq` (core: fields, tables, expression language, dialects, render contract), `gooq/db` (connections, transactions, scanning), `gooq/dsl` (query and DML builders), `gooq/fn` (function library); generated table code only imports `gooq`.

All rendered output shown below comes from test assertions — copy-paste friendly.

## Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Select](#select)
  - [Basics](#basics)
  - [Conditions](#conditions)
  - [JOINs](#joins)
  - [Subqueries](#subqueries)
  - [Grouping & aggregation](#grouping--aggregation)
  - [Set operations & CTEs](#set-operations--ctes)
  - [Expressions & functions](#expressions--functions)
  - [Row locks](#row-locks)
- [DML](#dml)
  - [Insert](#insert)
  - [Update](#update)
  - [Delete](#delete)
  - [Upsert & Returning](#upsert--returning)
- [Execution](#execution)
- [Configuration and connections](#configuration-and-connections)
- [Transactions](#transactions)
- [Caching](#caching)
- [Dialects](#dialects)
  - [Custom dialects](#custom-dialects)
  - [Drivers](#drivers)
  - [Dialect notes](#dialect-notes)
- [Multi-tenancy](#multi-tenancy)
  - [Configuration](#configuration)
  - [Values](#values)
  - [Scope](#scope)
  - [Tenant joins](#tenant-joins)
  - [Writes](#writes)
  - [Tenant caching](#tenant-caching)
  - [Soft delete](#soft-delete)
- [Sharding](#sharding)
  - [Grouping](#grouping)
  - [Rules and ShardBy](#rules-and-shardby)
  - [Physical and generation](#physical-and-generation)
  - [Sharded caching](#sharded-caching)
  - [Conventions](#conventions)
  - [Creating shard tables](#creating-shard-tables)
- [gooq-gen (codegen tool)](#gooq-gen-codegen-tool)
- [Testing](#testing)

## Features

**Query building**
- Offline rendering: `ToSql(dialect)` produces SQL and arguments without a database connection; execution is a separate layer.
- Type-safe fields and conditions: `Field[T]` operators, `IN` / `BETWEEN` / `LIKE` / `IS NULL`, expression comparisons, chained `AND` / `OR` / `NOT`, and `EXISTS`.
- SELECT with joins (including LATERAL), self-joins, derived tables, correlated subqueries, CTEs and recursive CTEs, and `UNION` / `INTERSECT` / `EXCEPT`.
- Grouping and ordering: `ROLLUP` / `CUBE` / `GROUPING SETS`, `HAVING`, `DISTINCT`, ordering by expression, and `LIMIT` / `OFFSET` / `Page`.
- Row locks: `FOR UPDATE`, shared locks, `SKIP LOCKED`, and `NOWAIT` where the dialect supports them.
- Expressions: arithmetic, `CASE`, `CAST`, bound `Raw` fragments, and custom `Expression` nodes.
- Functions (`fn`): string, math, date, aggregate, window, `FILTER`, `GROUP_CONCAT`, and dialect-aware `DATE_FORMAT`.

**DML**
- INSERT: positional values, generated records, batch chunks, and `INSERT ... SELECT`.
- Upsert: `ON DUPLICATE KEY`, `ON CONFLICT`, `MERGE`, `DoNothing`, and `Excluded`.
- UPDATE: chained `Set`, map `Data`, generated records, expression values, and multi-table joins.
- DELETE: soft-delete rewrite, `Unscoped()` hard delete, and batch delete by key.
- `RETURNING` where supported; unsupported combinations fail before SQL is produced.

**Execution and database**
- Reads: `Rows`, `Structs`, `Row`, `Value`, `Column`, `Count`, `Exists`, and counted page reads.
- Connection groups, master/slave routing with weights, `DB(name)`, `Master()`, `Slave()`, and `Schema()`.
- Transactions: context propagation, nested savepoints, seven propagation modes, and explicit `Tx(tx)`.
- Debug SQL logging, dry run, prepared statements, and OpenTelemetry spans.
- Result mapping: ordered `Result`, column-keyed `Record`, typed cell helpers (`db.Get` / `db.Value` / `db.Assign`), `orm` tags, and `sql.Scanner` / `time.Time` cells.

**Built-in policies**
- Soft delete: predicates from `TableMeta`, join-aware placement, and automatic delete rewrite.
- Row-level multi-tenancy: automatic predicates, context or statement values, scoped `UnTenant()`, and write / conflict-target checks.
- Sharding: injected `ShardingRule`, table-level or query-level `ShardBy`, `Physical()`, scope isolation, failure on a missing shard value, and dynamic shard creation with `CreateTableLike`.
- Caching: single-query and counted-page caches, pluggable adapters and codec, and group-aware keys.
- Validation: construction-time errors, dialect capability checks, and unsupported features fail before execution.

**Dialects and tooling**
- Eight built-in dialects: MySQL, MariaDB, TiDB, OceanBase, PostgreSQL, GaussDB, Dameng, and SQLite, plus custom `DialectSpec` implementations with optional capability interfaces.
- Eight built-in drivers and custom `db.Driver` registration.
- `gooq-gen`: typed tables and models, custom templates, sharding groups, tenant columns, `tables init/check/diff`, and dynamic shard ignore patterns.

## Quick Start

Requires Go 1.24 or newer. Add the module with:

```bash
go get github.com/lanceadd/gooq@latest
```

### 1. Generate table objects

```bash
go install github.com/lanceadd/gooq/cmd/gooq-gen@latest
gooq-gen init                                       # optional: scaffold hack/gooq.yaml + templates
gooq-gen -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

Generates typed gooq table objects (`table/`) and `model/` for all tables in one shot, or for the groups listed in `hack/gooq.tables.yaml` when that file exists. Each table produces `user_model.go` (`UserModel`) and `user_record.go` (`UserRecord`). `gooq-gen` can also read `hack/gooq.yaml` under the `gooq.gen` node instead of CLI options. See the gooq-gen section below for configuration, sharding groups, and tenant settings.

### 2. Hand-write a table object (or use the generated one)

```go
type UserTable struct {
    *gooq.TableBase
    Id        gooq.Field[int64]
    Name      gooq.Field[string]
    Age       gooq.Field[int]
    Status    gooq.Field[string]
    CreatedAt gooq.Field[time.Time]
    DeletedAt gooq.Field[time.Time] // flagged as soft-delete column
}

var UserMeta = &gooq.TableMeta{
    TableName: "user",
    Fields: []gooq.FieldMeta{
        {ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true, AutoIncrement: true},
        {ColumnName: "name", LocalType: gooq.LocalTypeString},
        {ColumnName: "age", LocalType: gooq.LocalTypeInt},
        {ColumnName: "status", LocalType: gooq.LocalTypeString},
        {ColumnName: "created_at", LocalType: gooq.LocalTypeDatetime},
        {ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
    },
}

// Constructor pattern: fields bind to the instance via NewFieldAt,
// so As/Clone are one-line reconstructions.
func newUserTable(alias ...string) *UserTable {
    t := &UserTable{TableBase: gooq.NewTableBase(UserMeta)}
    if len(alias) > 0 {
        t.TableBase = t.TableBase.As(alias[0])
    }
    t.Id = gooq.NewFieldAt[int64](t.TableBase, "id")
    t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
    t.Age = gooq.NewFieldAt[int](t.TableBase, "age")
    t.Status = gooq.NewFieldAt[string](t.TableBase, "status")
    t.CreatedAt = gooq.NewFieldAt[time.Time](t.TableBase, "created_at")
    t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
    return t
}

var User = newUserTable() // package-level table object

func (t *UserTable) As(alias string) *UserTable    { return newUserTable(alias) } // JOINs / self-joins
func (t *UserTable) Clone() *UserTable             { return newUserTable() }
```

### 3. First query

```go
import (
    "github.com/lanceadd/gooq"     // core: Field/Table/Expression/Dialect
    "github.com/lanceadd/gooq/dsl" // builders: Select/Insert/Update/Delete
)

sql, args, err := dsl.Select(User.Id, User.Name).
    From(User).
    Where(User.Age.Gt(18)).
    Order(User.Id.Desc()).
    Limit(10).
    ToSql(gooq.DialectMySQL)
// sql:  SELECT `user`.`id`, `user`.`name` FROM `user` WHERE (`user`.`age` > ?) AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10
// args: []any{18}
```

`deleted_at IS NULL` appears automatically — the soft-delete condition comes from `TableMeta`, no manual filtering. `ToSql()` without a dialect renders MySQL.

### 4. Execute

```go
import (
    "github.com/lanceadd/gooq/db"
    _ "github.com/lanceadd/gooq/drivers/mysql" // pgsql, sqlite: drivers/pgsql, drivers/sqlite
)

err := db.SetConfig(db.DefaultGroupName, db.Config{
    Link: "mysql:root:pass@tcp(127.0.0.1:3306)/app",
})

var users []model.UserModel
rows, err := dsl.Select(User.AllFields()).From(User).
    Where(User.Age.Gt(18)).
    Rows(ctx)
err = rows.Structs(&users)
```

## Select

### Basics

```go
// All columns / set difference / alias / distinct
dsl.Select(User.AllFields()).From(User)                                     // SELECT `user`.`id`, `user`.`name`, ...
dsl.Select(User.Id).From(User).FieldsEx(User.CreatedAt, User.DeletedAt)     // difference: remaining columns
dsl.Select(User.Name.As("nickname")).From(User)                             // SELECT `user`.`name` AS `nickname`
dsl.Select(User.Id).From(User).Distinct()                                   // SELECT DISTINCT ...

// Paging
dsl.Select(User.Id).From(User).Limit(10)              // ... LIMIT 10
dsl.Select(User.Id).From(User).Offset(20).Limit(10)   // ... LIMIT 10 OFFSET 20
dsl.Select(User.Id).From(User).Offset(20)             // MySQL: LIMIT 18446744073709551615 OFFSET 20; PG: OFFSET 20; SQLite: LIMIT -1 OFFSET 20
dsl.Select(User.Id).From(User).Page(2, 10)            // ... LIMIT 10 OFFSET 10（1-based pages）

// Ordering: direction is part of the expression (Field.Asc()/Desc(), gooq.OrderAscExpr/DescExpr)
dsl.Select(User.Id).From(User).Order(User.Age.Desc(), User.Id.Asc()).ToSql(gooq.DialectPgsql)
// SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL ORDER BY "user"."age" DESC, "user"."id" ASC
dsl.Select(User.Id).From(User).OrderDesc(fn.Count(User.Id))       // shorthand: wrap each with DESC
// Order by expression / raw fragment — Order renders items as-is (NULLS etc. via Raw)
dsl.Select(User.Id).From(User).OrderDesc(gooq.Raw("ABS(`user`.`age`)"))
// ... ORDER BY ABS(`user`.`age`) DESC
dsl.Select(User.Id).From(User).Order(gooq.Raw("`user`.`age` DESC NULLS LAST"))
// ... ORDER BY `user`.`age` DESC NULLS LAST
```

### Conditions

```go
// Field operators: value type T is enforced at compile time.
User.Age.Gt(18)                       // `user`.`age` > ?
User.Age.Between(18, 60)              // `user`.`age` BETWEEN ? AND ?
User.Status.In("vip", "admin")        // `user`.`status` IN (?, ?)
User.Name.Like("j%")                  // `user`.`name` LIKE ?
User.DeletedAt.IsNull()               // `user`.`deleted_at` IS NULL

// Expression operands (column comparisons, subqueries, Raw) go through EqExpr.
OrderItem.UserId.EqExpr(User.Id)      // `order_item`.`user_id` = `user`.`id`
User.Id.InExpr(subquery)              // `user`.`id` IN (SELECT ...)

// A comparison is a Condition and can chain And / Or / Not. The chain is left-associative.
User.Age.Gt(11).Or(User.Age.Lt(-1))
// (`user`.`age` > ? OR `user`.`age` < ?)
User.Age.Gt(11).Or(User.Age.Lt(-1).And(User.Status.Eq("vip")))
// (`user`.`age` > ? OR (`user`.`age` < ? AND `user`.`status` = ?))
User.Status.Eq("banned").Not()
// (NOT `user`.`status` = ?)

// Multiple Where calls are combined with AND. Combine fields with chained Or / And.
dsl.Select(User.Id).From(User).
    Where(User.Age.Gt(11).Or(User.Age.Lt(-1))).
    Where(User.Status.Eq("vip")).
    ToSql(gooq.DialectMySQL)
// ... WHERE (`user`.`age` > ? OR `user`.`age` < ?) AND `user`.`status` = ? AND `user`.`deleted_at` IS NULL
// args: []any{11, -1, "vip"}

dsl.Select(User.Id).From(User).
    Where(User.Age.Lt(18).Or(User.Status.Eq("vip"))).
    Where(User.Name.Like("j%")).
    ToSql(gooq.DialectMySQL)
// ... WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL
// args: []any{18, "vip", "j%"}

// The left side is not a Field[T]: use gooq.Gt, gooq.Eq, and the rest. Wrap Raw with Cond.
gooq.Gt(User.Age.Add(1), 11).Or(gooq.Lt(User.Age, 0))
// ((`user`.`age` + ?) > ? OR `user`.`age` < ?)
gooq.Cond(gooq.Raw("a = 1")).Or(User.Age.Lt(-1))
// (a = 1 OR `user`.`age` < ?)

// Dynamic assembly: Clone a base builder for reuse. One builder cannot be used concurrently;
// execution writes the resolved executor back onto it.
base := dsl.Select(User.Id).From(User)
q1 := base.Clone().Where(User.Age.Gt(18))
q2 := base.Clone().Where(User.Status.Eq("vip")) // independent

// A string passed to Fields is spliced as Raw SQL and is not quoted as an identifier.
```

`In` and `NotIn` render no condition when the value list length is 0. A nil slice, an empty slice, and no arguments are the same, and `WHERE` drops that empty condition. The library does not rewrite it to `(1 = 0)` or `(1 = 1)`, and it does not return an error. When the statement must match no rows, the caller skips execution at length 0.

### JOINs

```go
u := User.As("u")
ur := UserRole.As("ur")
r := Role.As("r")

dsl.Select(u.Id, r.Name).From(u).
    InnerJoin(ur).On(ur.UserId.EqExpr(u.Id)).
    InnerJoin(r).On(r.Id.EqExpr(ur.RoleId)).
    Where(r.Name.Eq("admin")).
    ToSql(gooq.DialectMySQL)
// SELECT `u`.`id`, `r`.`name` FROM `user` AS `u` INNER JOIN `user_role` AS `ur` ON (`ur`.`user_id` = `u`.`id`) INNER JOIN `role` AS `r` ON (`r`.`id` = `ur`.`role_id`) WHERE (`r`.`name` = ?) AND `u`.`deleted_at` IS NULL AND `r`.`deleted_at` IS NULL

// Self-join: two alias instances of the same table.
u1 := User.As("u1")
u2 := User.As("u2")
dsl.Select(u1.Id, u2.Id).From(u1).InnerJoin(u2).On(u1.Id.EqExpr(u2.Id))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS `u1` INNER JOIN `user` AS `u2` ON (`u1`.`id` = `u2`.`id`) WHERE `u1`.`deleted_at` IS NULL AND `u2`.`deleted_at` IS NULL

// LATERAL derived tables. SQLite 3.46 does not support LATERAL and returns an error at render time.
lt := dsl.Select(fn.Count(UserRole.UserId).As("cnt")).
    From(UserRole).Where(UserRole.UserId.EqExpr(u.Id)).As("lt")
dsl.Select(u.Id, lt.Field("cnt")).From(u).LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).ToSql(gooq.DialectPgsql)
// ... LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS "cnt" FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS "lt" ON 1 = 1 ...
```

### Subqueries

```go
sub := dsl.Select(Role.Id).From(Role).Where(Role.Name.Eq("admin"))

dsl.Select(User.Id).From(User).Where(User.Id.InExpr(sub))
// ... WHERE (`user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE (`role`.`name` = ?) AND `role`.`deleted_at` IS NULL)) ...

dsl.Select(User.Id).From(User).Where(gooq.Exists(sub))
// ... WHERE EXISTS (SELECT ...) ...
dsl.Select(User.Id).From(User).Where(gooq.NotExists(sub))

// Derived tables.
dsl.Select(User.Id).From(
    dsl.Select(User.Id).From(User).Where(User.Age.Gt(18)).As("t"),
)
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE (`user`.`age` > ?) AND `user`.`deleted_at` IS NULL) AS `t`

// Correlated subqueries: reference the outer alias inside.
u := User.As("u")
ur := UserRole.As("ur")
dsl.Select(u.Id).From(u).Where(gooq.Exists(
    dsl.Select(ur.UserId).From(ur).Where(ur.UserId.EqExpr(u.Id)),
))
// ... WHERE (EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS `ur` WHERE (`ur`.`user_id` = `u`.`id`))) ...
```

### Grouping & aggregation

```go
dsl.Select(User.Status, fn.Count(User.Id)).
    From(User).
    Group(User.Status).
    Having(gooq.Gt(fn.Count(User.Id), 2)).
    ToSql(gooq.DialectMySQL)
// SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?
// args: []any{2}

// Grouping extensions (dialect-aware: Rollup on MySQL/PG, Cube/GroupingSets PG-only — error on MySQL).
dsl.Select(User.Status).From(User).GroupRollup(User.Status).ToSql(gooq.DialectMySQL)
// ... GROUP BY `user`.`status` WITH ROLLUP
dsl.Select(User.Status).From(User).GroupCube(User.Status).ToSql(gooq.DialectPgsql)
// ... GROUP BY CUBE("user"."status")
```

### Set operations & CTEs

```go
dsl.Select(User.Id).From(User).Where(User.Age.Eq(1)).
    UnionAll(dsl.Select(User.Id).From(User).Where(User.Age.Eq(2)))
// ... UNION ALL ...
// ORDER BY after a set operation can only name a result column. A table-qualified column is rejected
// with 1250 on MySQL, TiDB, and OceanBase, and the PostgreSQL family rejects it too.
// dsl.Select(User.Id).From(User).UnionAll(...).Order(gooq.Raw("id"))

dsl.Select(User.Id).From(User).
    Intersect(dsl.Select(User.Id).From(User)).
    Except(dsl.Select(User.Id).From(User))
// ... INTERSECT ... EXCEPT ...

// CTE / recursive CTE.
dsl.With("adults", dsl.Select(User.Id).From(User).Where(User.Age.Gt(18))).
    Fields(dsl.Cte("adults").Field("id")).From(dsl.Cte("adults")).ToSql(gooq.DialectPgsql)
// WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"

dsl.WithRecursive("t", dsl.Select(User.Id).From(User).Where(User.Id.Eq(1)))
// WITH RECURSIVE t AS (...)
```

### Expressions & functions

```go
// String / math / date / aggregate / general.
fn.Concat(User.Name, gooq.Str(","), User.Status)   // CONCAT(`user`.`name`, ',', `user`.`status`)
fn.Coalesce(User.Name, gooq.Str("unknown"))        // COALESCE(`user`.`name`, 'unknown')
fn.Substring(User.Name, 1, 3)                      // SUBSTRING(`user`.`name`, ?, ?)
fn.DateDiff(User.CreatedAt, fn.Now())        // DATEDIFF(`user`.`created_at`, NOW())
fn.CountDistinct(User.Status)                      // COUNT(DISTINCT `user`.`status`)

// Date formatting across dialects: one expression, per-dialect rendering.
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectMySQL)
// SELECT DATE_FORMAT(`user`.`created_at`, '%Y-%m-%d') FROM ...
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectPgsql)
// SELECT TO_CHAR("user"."created_at", 'YYYY-MM-DD') FROM ...
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectSQLite)
// SELECT strftime('%Y-%m-%d', "user"."created_at") FROM ...

// String aggregation (MySQL GROUP_CONCAT / PG STRING_AGG / SQLite GROUP_CONCAT).
fn.GroupConcat(fn.GroupConcatOptions{
    Field: User.Name, Separator: "-", OrderBy: []gooq.Expression{User.Name.Asc()},
}).ToSql(...)  // GROUP_CONCAT(`user`.`name` ORDER BY `user`.`name` ASC SEPARATOR '-')

// Window functions.
fn.Rank().Over([]gooq.Expression{User.Status}, []gooq.Expression{User.Age.Desc()}).As("r")
// RANK() OVER (PARTITION BY `user`.`status` ORDER BY `user`.`age` DESC) AS `r`
fn.RowNumber().Over(nil, []gooq.Expression{User.Id.Asc()})
// ROW_NUMBER() OVER (ORDER BY `user`.`id` ASC)
fn.Sum(User.Age).OverFrame(
    []gooq.Expression{User.Status},
    []gooq.Expression{User.Id.Asc()},
    fn.RowsFrame("UNBOUNDED PRECEDING", "CURRENT ROW"),
)
// SUM(`user`.`age`) OVER (PARTITION BY `user`.`status` ORDER BY `user`.`id` ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)

// CASE.
gooq.Case().When(User.Age.Gt(60)).Then(gooq.Str("old")).
    When(User.Age.Gt(18)).Then(gooq.Str("adult")).
    Else(gooq.Str("young")).End().As("bucket")
// CASE WHEN `user`.`age` > ? THEN 'old' WHEN `user`.`age` > ? THEN 'adult' ELSE 'young' END AS `bucket`

// Arithmetic & casting.
User.Age.Mul(2)                                 // (`user`.`age` * ?)
gooq.Add(User.Age, gooq.Mul(2, User.Age))       // (`user`.`age` + (? * `user`.`age`))
gooq.Negate(User.Age)                           // (-`user`.`age`)
User.Age.Cast(gooq.LocalTypeString)             // CAST(`user`.`age` AS CHAR)（PG: TEXT / SQLite: TEXT）

// Raw: structured SQL with parameter binding. Placeholders are not rewritten.
// Use ? for MySQL and SQLite. PostgreSQL keeps the text as written, so write $n yourself.
gooq.Raw("JSON_EXTRACT(data, ?)", "$.name")
dsl.Select(User.Id).From(User).Where(gooq.Raw("age > ?", 18))

// Generic functions render NAME(args...) as-is (no dialect awareness).
dsl.Select(fn.New("JSON_EXTRACT", User.Name, gooq.Str("$.key"))).From(User)
// SELECT JSON_EXTRACT(`user`.`name`, '$.key') FROM ...

// Attach a dialect render strategy to any FuncExpr (instance-scoped, no global registry):
concatPipe := fn.New("CONCAT", User.Name, gooq.Str("-x")).
    RenderWith(func(rc *gooq.RenderContext, argsSQL []string) string {
        if rc.Dialect() == gooq.DialectPgsql {
            return "(" + argsSQL[0] + " || " + argsSQL[1] + ")"
        }
        return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")"
    })
// A custom expression node implements gooq.Expression. Render returns SQL; arguments go through rc.AddArg.
```

### Row locks

```go
dsl.Select(User.Id).From(User).LockForUpdate().ToSql(gooq.DialectMySQL)   // ... FOR UPDATE
dsl.Select(User.Id).From(User).LockInShareMode().ToSql(gooq.DialectMySQL) // ... LOCK IN SHARE MODE
dsl.Select(User.Id).From(User).LockInShareMode().ToSql(gooq.DialectPgsql) // ... FOR SHARE
dsl.Select(User.Id).From(User).LockForUpdate().ToSql(gooq.DialectSQLite)  // ignored on SQLite
dsl.Select(User.Id).From(User).LockForUpdate().SkipLocked().ToSql(gooq.DialectMySQL)
// ... FOR UPDATE SKIP LOCKED
dsl.Select(User.Id).From(User).LockForUpdate().NoWait().ToSql(gooq.DialectPgsql)
// ... FOR UPDATE NOWAIT
```

`SkipLocked()` and `NoWait()` must follow `LockForUpdate()` or `LockInShareMode()`. The later wait policy replaces the earlier one. The MySQL family, PostgreSQL, and GaussDB emit the clause. SQLite and Dameng fail in `ToSql` instead of dropping the clause and running the query.

## DML

### Insert

```go
// Only setters that were called are written. SetAge(0) writes 0. SetDeletedAtNull writes NULL.
dsl.Insert(User).ValuesOf(model.NewUserRecord().SetName("john").SetAge(0))
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?)

// Positional columns/values, repeated Values for batch.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).Values("b", 2)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)

// Several record rows. Batch chunks execution and aggregates RowsAffected. Each chunk commits on its own; wrap the call in db.Transaction when the chunks must be atomic.
dsl.Insert(User).ValuesOf(
    model.NewUserRecord().SetName("a"),
    model.NewUserRecord().SetName("b"),
).Batch(100).Exec(ctx)

// Rows that set different columns become separate INSERT statements at Exec.
// Omitted columns keep the table default. ToSql rejects that shape; call Exec.
// Batch splits inside each column set and does not pad missing columns with NULL.

// INSERT ... SELECT.
dsl.InsertFrom(User, dsl.Select(User.Name).From(User).Where(User.Age.Gt(18)))
// INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE (`user`.`age` > ?) AND `user`.`deleted_at` IS NULL
```

### Update

```go
// Chained Set.
dsl.Update(User).Set(User.Name, "x").Set(User.Age, 20).Where(User.Id.Eq(1))
// UPDATE `user` SET `name` = ?, `age` = ? WHERE (`user`.`id` = ?)

// Map full update.
dsl.Update(User).Data(map[string]any{"age": 1, "name": "x"})
// UPDATE `user` SET `age` = ?, `name` = ?

// Expression values render as SQL fragments (field arithmetic, Raw, etc.).
dsl.Update(User).Set(User.Age, User.Age.Add(1)).Where(User.Id.Eq(1))
// UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE (`user`.`id` = ?)

// One record row becomes SET. Several rows update by primary key. RowsAffected is aggregated.
dsl.Update(User).ValuesOf(model.NewUserRecord().SetName("x").SetAge(0)).Where(User.Id.Eq(1))
dsl.Update(User).ValuesOf(
    model.NewUserRecord().SetId(1).SetName("a"),
    model.NewUserRecord().SetId(2).SetName("b"),
).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// Multi-table update (MySQL JOIN / PostgreSQL, SQLite, and Dameng FROM).
u := User.As("u"); r := Role.As("r")
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS `u` INNER JOIN `role` AS `r` ON `r`.`id` = `u`.`id` SET `status` = ?
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS "u" SET "status" = $1 FROM "role" AS "r" WHERE "r"."id" = "u"."id"
// Any LEFT / RIGHT / FULL join makes PostgreSQL, GaussDB, SQLite, and Dameng return an error at render time.
// MySQL keeps the outer join and writes it before SET.
```

`UPDATE` does not add the soft-delete predicate, including for joined tables. Add `Where(User.DeletedAt.IsNull())` explicitly when already-deleted rows must be skipped.

### Delete

```go
// Soft-delete tables: DELETE auto-rewrites to UPDATE deleted_at.
dsl.Delete(User).Where(User.Id.Eq(1))
// UPDATE `user` SET `deleted_at` = ? WHERE (`user`.`id` = ?)

// Unscoped(): real DELETE.
dsl.Delete(User).Unscoped().Where(User.Id.Eq(1))
// DELETE FROM `user` WHERE (`user`.`id` = ?)

// Non-soft-delete tables delete directly.
dsl.Delete(UserRole).Where(UserRole.Id.Eq(1))
// DELETE FROM `user_role` WHERE (`user_role`.`id` = ?)

// Batch delete by primary key. ValuesOf is not used on Delete.
dsl.Delete(User).Where(User.Id.In(1, 2)).Exec(ctx)
```

### Upsert & Returning

```go
// MySQL: DoUpdate writes the given value. OnConflictKey is optional.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.Id).DoUpdate(User.Name, "x").ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = ?

// Write the value from this insert: VALUES(col) on MySQL, EXCLUDED.col on PostgreSQL / SQLite.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    DoUpdate(User.Name, gooq.Excluded(User.Name)).ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)

// MySQL: INSERT IGNORE.
dsl.Insert(User).Columns(User.Name).Values("a").DoNothing().ToSql(gooq.DialectMySQL)
// INSERT IGNORE INTO `user` (`name`) VALUES (?)

// PG: ON CONFLICT.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.Id).DoUpdate(User.Name, "x").ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3
dsl.Insert(User).Columns(User.Name).Values("a").
    OnConflictKey(User.Id).DoNothing().ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING

// Returning: PostgreSQL, GaussDB, and SQLite; MySQL and Dameng return an error.
dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).
    Returning(User.Id).ToSql(gooq.DialectPgsql)
// UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"
```

## Execution

```go
// Query into a struct slice. Omitting DB uses the default group.
var users []model.UserModel
rows, err := dsl.Select(User.AllFields()).From(User).
    Where(User.Age.Gt(18)).Order(User.Id.Desc()).Limit(10).
    Rows(ctx)
err = rows.Structs(&users)

var count int64
err = dsl.Select(fn.Count(User.Id)).From(User).Value(ctx, &count)

// One column. time.Time and sql.Null* are cells, so they use Value or Column.
// A row struct uses Row and db.Record.Struct.
var created time.Time
err = dsl.Select(User.CreatedAt).From(User).Where(User.Id.Eq(1)).Value(ctx, &created)
var names []string
err = dsl.Select(User.Name).From(User).Column(ctx, &names)

// DML.
_, err = dsl.Insert(User).ValuesOf(model.NewUserRecord().SetName("john")).Exec(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).Exec(ctx)

// Count is the number of rows after pagination and ordering are removed.
// DISTINCT, grouping, and set operations are counted through a subquery.
// Exists reports whether that count is greater than zero: the select list becomes 1, and pagination and ordering are removed.
// Dameng uses SELECT COUNT(*) FROM (SELECT 1 ...) WHERE ROWNUM = 1. Other dialects use SELECT EXISTS (SELECT 1 ...).
// Value and Column require exactly one column, including RETURNING. Row and Value add LIMIT 1 when Limit is absent, and return an error when Limit is greater than 1.
total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).Count(ctx)
exists, err := dsl.Select(User.Id).From(User).Where(User.Account.Eq("x")).Exists(ctx)

// Typed row reads: the field type T is consumed at compile time.
row, err := dsl.Select(User.Id, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).Row(ctx)
id := db.Get(row, User.Id)      // int64
name := db.Get(row, User.Name)  // string
```

Execution binds `DB(name)` or the default configuration group and joins a transaction from ctx. See [Configuration and connections](#configuration-and-connections) and [Transactions](#transactions).

## Configuration and connections

`DB("orders")` selects that configuration group. Omitting `DB` uses the default group. `LoadConfig` reads `gooq.database`, `gooq.defaultGroup`, and `gooq.logger` through gcfg. Runtime reads only a file named `gooq`, such as `gooq.yaml`, and does not read `config.yaml`.

`database` is a list of groups: `name` is the group, `endpoints` are its master and slave nodes. A single group may omit `name` and is stored as `default`. Every group needs a name when there is more than one. `defaultGroup` selects the group used when the name is omitted; it must be one of the groups in this file; omitted, it is `default`.

Reads use a slave when the group has one; writes and `BEGIN` use a master. `Master()` and `Slave()` force the pool. `SetDebug` logs SQL, `SetDryRun` skips execution, `Prepare` prepares a statement, and `Schema` switches the MySQL database or PostgreSQL search_path.

```yaml
gooq:
  defaultGroup: orders
  database:
    - name: default
      endpoints:
        - link: "pgsql:postgres:postgres@tcp(127.0.0.1:5432)/test"
          role: master
          debug: true
    - name: orders
      endpoints:
        - link: "pgsql:postgres:postgres@tcp(127.0.0.1:5432)/orders"
          role: master
        - link: "pgsql:postgres:postgres@tcp(127.0.0.1:5433)/orders"
          role: slave
```

## Transactions

`Transaction` puts the tx into ctx. Queries inside the callback join it:

```go
database, err := db.Instance()
err = database.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
    _, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).Exec(ctx)
    return err
})
```

A transaction is taken from ctx and joins only the pool that started it. Another registered group, another `Open`, and a `Schema` that opened its own pool do not use that transaction and do not fail because it is present. `Master`, `Slave`, `WithRoute`, and a `Schema` that keeps the shared pool still join.

Concurrent requests may share one DB; each transaction lives in its own ctx. Do not hand a transactional ctx or `Tx` to another request, and do not run one `Tx` concurrently. `dsl Tx(tx)` runs that statement on the given connection; otherwise it still joins from ctx.

## Caching

```go
// Inject a global cache adapter (e.g. a gredis-backed implementation).
dsl.SetCacheAdapter(redisAdapter)
dsl.SetHashCacheAdapter(redisHashAdapter)
// Cached values are JSON unless SetCacheCodec installs another process-wide codec.
// For Row and Rows, the number and time types after a hit come from that codec.
// JSON keeps integers as json.Number and times as RFC3339 strings.
dsl.SetCacheCodec(codec)

// Single-query cache: Value, Column, Row, Rows, Count, and Exists go through it.
// Cache() with no arg or a zero option disables caching; a custom Name fixes the key
// (useful for manual invalidation). Argument keys include the type name, so
// int64(1) and "1" do not share a key.
// A generated key includes the config group that runs the query. Omitting DB() uses the
// current default group. A custom Name does not gain a group name.
rows, err := dsl.Select(User.AllFields()).From(User).
    Cache(dsl.CacheOption{Duration: time.Minute}).
    Rows(ctx)

// Composite query: count runs first, rows after; count=0 short-circuits without querying rows.
rows, total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    Page(1, 10).RowsAndCount(ctx)

// PageCache caches a counted page as one hash record (fields "count" and "rows").
// It applies to RowsAndCount, StructsAndCount, and ColumnAndCount.
// Requires SetHashCacheAdapter. The key hashes the count SQL and the page SQL together,
// arguments included, so selected columns, filters, order, and page each change the key.
// RowsField/CountField override the field names; Force caches empty
// results (count=0) which are skipped by default.
rows, total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).RowsAndCount(ctx)

// StructsAndCount fills a struct slice and returns the total.
// It is RowsAndCount followed by Result.Structs, and shares that page cache.
var vips []User
total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).StructsAndCount(ctx, &vips)
```

## Dialects

| Dialect | Placeholder | Quote | Pagination |
| --- | --- | --- | --- |
| mysql, mariadb, tidb, oceanbase | `?` | `` ` `` | LIMIT |
| pgsql, gaussdb | `$n` | `"` | LIMIT |
| sqlite | `?` | `"` | LIMIT |
| dm | `?` | `"` (uppercase) | LIMIT |

### Custom dialects

- `ToSql` requires a registered `DialectSpec`. An unknown dialect returns an error. Embed `MySQLDialect`, `PostgresDialect`, or `SQLiteDialect` and override the methods that differ, then call `RegisterDialect`.
- `DialectSpec` covers statement shape: pagination, row locks, `JOIN` rewrites, upsert, delete, multi-table `UPDATE` (`RenderUpdate`), and existence queries (`RenderExists`). Function SQL such as `DATE_FORMAT`, `GROUP_CONCAT`, `IFNULL`, `CURDATE`, `DATE_ADD`, `DATEDIFF`, `ROUND`, and `MEDIAN` stays on the concrete dialect type. A dialect that does not implement one returns `dialect %q does not support ...` from `ToSql`. SQLite upsert uses `ON CONFLICT`.
- Custom dialects with positional placeholders must keep argument order aligned with SQL text:
  - Implement `UpdateJoinsBeforeSet() bool` when `UPDATE` writes `JOIN` before `SET` (the MySQL dialect does).
  - Implement `FoldIdent() bool` when the database treats identifiers case-insensitively; tenant write checks use it. Numbered placeholders (`$n`) need neither.
  - Implement `UpsertConflictTarget() bool` and return true when upsert uses an explicit conflict target (`ON CONFLICT` or `MERGE`). A dialect that does not implement it skips the tenant conflict-target check.
  - Implement `ReleaseSavepoint() bool` and return false when the database has no `RELEASE SAVEPOINT`; a nested commit then keeps the savepoint and the outer transaction releases it.
- Table, column, and function aliases go through `Quote`. `numeric` / `decimal` generate `string`. PostgreSQL arrays generate a slice of the element type.

### Drivers

- Opening a connection requires a blank import of the matching driver package (`drivers/mysql`, `drivers/mariadb`, `drivers/tidb`, `drivers/oceanbase`, `drivers/pgsql`, `drivers/gaussdb`, `drivers/dm`, `drivers/sqlite`). The driver registers itself with `db.Register`. `gooq-gen` imports these drivers.

### Dialect notes

- MariaDB, TiDB, and OceanBase use the MySQL wire protocol and the MySQL statement shape. MariaDB reads columns from `information_schema`, so a JSON value stored as `longtext` with a `json_valid` check is reported as `json`. On TiDB, `LockInShareMode` returns an error by default. After `tidb_enable_noop_functions` is enabled the statement runs, but it does not take a shared lock. `LockForUpdate` works.
- GaussDB means openGauss. Quoting, placeholders, row locks, `RETURNING`, deletes, and updates follow PostgreSQL. `DATE_ADD` is interval multiplication, not `make_interval`. Upsert is `ON DUPLICATE KEY UPDATE`, and the inserted value is `VALUES(col)`. `DoNothing` returns an error because GaussDB has no `ON CONFLICT DO NOTHING`. The `database/sql` driver name is `opengauss`.
- Dameng (`dm`):
  - Quotes identifiers in uppercase so they match names stored from unquoted DDL.
  - Upsert is `MERGE` and needs `OnConflictKey`; the conflict column must be in the insert list. `DoNothing` inserts only when the row is not matched.
  - There is no `RETURNING` and no `RELEASE SAVEPOINT`; a nested transaction commit keeps the savepoint, and the outer transaction or a later savepoint with the same name releases it.
  - `Exists` renders `SELECT COUNT(*) FROM (subquery) WHERE ROWNUM = 1`. `GROUP_CONCAT` renders as `LISTAGG`, `DATE_FORMAT` as `TO_CHAR`, `IFNULL` as `NVL`, and `DATE_ADD` as `DATEADD`.
  - `time.Time` is bound as a timestamp string; the zero time becomes `NULL`. An IANA timezone in the config becomes a minute offset, and an offset outside ±12 hours fails.
  - `DELETE` with `JOIN` and `UPDATE` with an outer join return an error. Identity columns are not marked `AutoIncrement`.
  - The `database/sql` driver name is `dm`. Link example: `dm:SYSDBA:SYSDBA001@tcp(127.0.0.1:5236)/SYSDBA`.
- Schema-qualified tables render `schema.table.column` (alias shadows schema); gooq-gen fills schema for PG only (`current_schema()`).
- PostgreSQL 16 can run `STRING_AGG(DISTINCT ...)`, `JSON_ARRAYAGG`, and `ANY_VALUE`.

## Multi-tenancy

### Configuration

A tenant column is part of `TableMeta`, like a primary key or a soft-delete column. `gooq-gen` marks it only when `hack/gooq.yaml` sets `gooq.gen.tenant.column`. A missing or null `tenant` node leaves generation unchanged. Otherwise the node must be a map with a non-empty `column`; unknown fields are rejected. `tables` overrides that column for one logical table. An empty string turns the mark off for that table.

```yaml
gooq:
  gen:
    tenant:
      column: tenant_id
      tables:
        shop_order: org_id
        dict: ""
```

Generation-time column matching ignores case and underscores. Runtime tenant-column checks follow the dialect: MySQL family, SQLite, and DM compare case-insensitively; PostgreSQL and GaussDB compare exactly. Allowed values are `string`, `uuid.UUID`, and the integer widths. `int` and `int64` are not the same type.

### Values

The value comes from `dsl.WithTenant(ctx, id)` or from `Tenant(id)` on that builder. `WithTenant` carries one value per request; `Tenant(ids...)` is per-statement and renders `tenant_col IN (...)`. Writes (`INSERT`, or setting the tenant column in `UPDATE`) require exactly one value, and a slice passed to `WithTenant` is a type error, not multi-tenancy. `ToSql` does not read the context. Execution records the context value on the builder, then renders it into a new `RenderContext`. A statement that touches a tenant table and has no value returns an error. It does not emit `IS NULL`.

### Scope

`Tenant(id)` applies to that builder and to queries rendered inside it. A nested builder may call `Tenant(other)` or `UnTenant()`. The later call wins: `UnTenant().Tenant(7)` adds the predicate, and `Tenant(7).UnTenant()` does not. `Tenant()` with no arguments only turns filtering back on. An earlier `Tenant(id)` stays in place; otherwise the value comes from the request context. `UnTenant()` turns tenant filtering off for every query that builder renders: the main table, joins, subqueries, CTEs, and `UNION` branches. A nested builder turns it back on for its own subtree by calling `Tenant(id)`.

### Tenant joins

`LEFT JOIN`, `LEFT JOIN LATERAL`, `INNER JOIN`, `INNER JOIN LATERAL`, and `CROSS JOIN LATERAL` keep their current shape. A `RIGHT JOIN` or `FULL JOIN` in a statement that touches any tenant table returns an error. Rewrite that join as `LEFT JOIN`, or call `UnTenant()`.

### Writes

- `INSERT` into a tenant table must set the tenant column to the single current tenant value. The column is not filled in automatically. `INSERT ... SELECT` must list named columns and include the tenant column. `Select().From(src)` and a `Raw` select item fail before SQL is produced. `UnTenant()` skips these checks.
- Do not set the tenant column in `UPDATE` or in an upsert assignment. If it is set, the value must equal the current tenant; another value, `NULL`, or an expression returns an error.
- For `INSERT ... SELECT`, only the column list is checked; selected values are not compared statically.
- `OnConflictKey` is not rewritten. On PostgreSQL, SQLite, and Dameng, a tenant table's `OnConflictKey` must include the tenant column for both `DoUpdate` and `DoNothing`. MySQL, MariaDB, TiDB, OceanBase, and GaussDB pick the unique key in the database, so every unique key on a tenant table must include the tenant column.
- `Unscoped()` only bypasses soft delete; `UnTenant()` only bypasses tenant filtering.

### Tenant caching

`CacheOption.Name` is the cache key as written. Put the tenant in that name. Without `Name`, the key hashes the SQL and arguments, so different tenants do not share an entry.

### Soft delete

Each user `WHERE` and `ON` predicate is parenthesized before a soft-delete or tenant predicate is appended with `AND`.

A sharded table keeps its tenant mark through `Physical()`; call `UnTenant()` for cross-tenant scans or migrations.

## Sharding

### Grouping

Shard groups live in `hack/gooq.tables.yaml`. `single` lists ordinary tables. `groups` lists sharded tables. Without that file, each physical table is generated as its own object. `gooq-gen tables init` writes the file with every table under `single`. `tables check` reports duplicate, missing, and extra names. `tables diff` reports tables added or removed in the database. Neither command rewrites the file. Tables matching `gooq.tables.check.ignore` are excluded from the missing report and from `diff` new tables; listed tables are still required to exist.

```yaml
# hack/gooq.tables.yaml
single:
  - "dict"
  - "user"

groups:
  - name: "order"
    table:
      - "order_00"
      - "order_01"
```

The first table in a group supplies the schema used for generation; every table in the group must match it. `name` is the logical table name, so the example generates `table/order.go` and `table/gooq_sharding.go`. `gooq-gen tables init` only fills `single` and leaves `groups: []` for you to edit.

### Rules and ShardBy

A group generates one logical table, `RegisterXxxSharding`, and a package-level `RegisterSharding`. One call to `RegisterSharding` assigns that rule to every sharded logical table. Use `RegisterXxxSharding` to replace the rule on a table that shards differently. A nil rule panics. Registering again overwrites. The rule is stored on `TableMeta.Sharding`.

```go
// generated table/user.go — sharded tables only
func RegisterUserSharding(rule gooq.ShardingRule) {
    if rule == nil {
        panic("gooq: sharding rule is required")
    }
    UserMeta.Sharding = rule
}

// application package (blank-imported so its init runs first)
type userRule struct {
    shards int64
}

func (r userRule) TableName(table gooq.ShardingTableMeta, value any) (string, error) {
    uid, ok := value.(int64)
    if !ok {
        return "", fmt.Errorf("user shard key must be int64, got %T", value)
    }
    n := uid % r.shards
    if n < 0 {
        n += r.shards
    }
    return fmt.Sprintf("%s_%02d", table.LogicalName, n), nil
}

func init() { table.RegisterSharding(userRule{shards: 16}) }

// ShardBy applies only to the main table (From / Insert / Update / Delete).
err := dsl.Select(table.User.Id, table.User.Name).From(table.User).
    ShardBy(uid).Value(ctx, &name)

// a joined sharded table carries its own value; a plain table does not
r := table.UserRole.ShardBy(uid)
err = dsl.Select(table.User.Name, r.Name).From(table.User).
    InnerJoin(r).On(r.UserId.EqExpr(table.User.Id)).
    ShardBy(uid).Value(ctx, &name)
```

- The first argument of `TableName` is `ShardingTableMeta`. `LogicalName` is the logical table name. `ShardNames` lists the physical shard tables that existed when the code was generated; it is informational, and the returned name is not checked against it. The return value is one unquoted table name, such as `user_03`. gooq quotes it. Leave schema on `TableMeta`; do not put it in the return value. A non-negative id can use `uid % shards` directly. Do not take the absolute value first: `-3` and `3` would land on the same shard. Do not use `len(ShardNames)` as the divisor. The list is only the tables that existed then.
- `ShardBy` on a query applies only to that query's main table. A subquery, CTE, or each branch of `UNION` sets its own value and does not inherit the outer one. Another sharded table in the query calls `ShardBy` and returns a new table; `As` and `Clone` keep that value. A sharded table with neither `ShardBy` nor `Physical` fails in `ToSql`.
- `role.ShardBy(uid)` does not change the original `role`. Use fields from the returned object in `ON` and the select list. `role.UserId` is still bound to the original table and does not carry this value.
- A correlated subquery that references an outer sharded table's column needs that table to carry a table-level `ShardBy` (`u := table.User.ShardBy(uid)`); use fields from the returned object inside the subquery too. A query-level `ShardBy` on the outer query alone fails at render time.
- Two sharded tables may carry different values. gooq does not check that they use the same rule. SQL still runs when the values do not belong together.

### Physical and generation

- `Physical("user_05")` clears the rule and uses that table name. An empty name panics.
- Generation compares column name, order, `LocalType`, nullability, primary key, auto-increment, and uniqueness against the first table in the group. A group with one table skips that check. String length, defaults, and comments are not compared.

### Sharded caching

- Sharded queries can use `Cache` and `PageCache`. Without `Name`, the key is computed after SQL is rendered and includes the physical table name, arguments, result shape, and database group. Different shards do not collide. This fits data that is read often and refreshed by expiry. With `Name`, the key is that string alone. SQL and arguments are not included. The function around the query must put its inputs in the name, for example `user-name:19`. The name does not need to be `user_03`. A fixed `Name: "user-name"` makes different inputs share one entry. Writes do not delete cache entries. With an explicit name, the application deletes that same name.

### Conventions

- These conventions are for the caller. gooq does not reject violations. Do not keep a logical placeholder table in the database. Put the shard key in `WHERE`. Do not use an AutoIncrement primary key; use a business key, or snowflake ids with embedded shard bits. While sharding, do not use cross-shard transactions, cross-database joins, or automatic merge or broadcast. Those statements still run. Cross-shard reads are explicit application loops, one `Physical` per shard. Group multi-row writes by shard first. One `ShardBy` writes one physical table. Rows from other shards in the same write go into that table and do not raise an error. `Batch` only splits rows of that same table.

### Creating shard tables

`db.DB.CreateTableLike` creates a new physical shard by cloning an existing one:

```go
name, err := EmployeeMeta.Sharding.TableName(gooq.ShardingTableMeta{
    LogicalName: EmployeeMeta.TableName,
    ShardNames:  EmployeeMeta.ShardNames,
}, companyID)
if err != nil {
    return err
}
err = database.CreateTableLike(ctx, "employee_1", name)
```

- MySQL family: `CREATE TABLE IF NOT EXISTS `employee_3` LIKE `employee_1``.
- PostgreSQL and GaussDB: `CREATE TABLE IF NOT EXISTS "employee_3" (LIKE "employee_1" INCLUDING ALL)`.
- Indexes, defaults, and constraints are copied. Rows are not.
- `IF NOT EXISTS` makes repeat calls safe. An existing target table is left untouched, and its structure is not compared with the template.
- SQLite and Dameng return a `does not support CREATE TABLE LIKE` error before any SQL is sent.

Pass unquoted names for both arguments; the dialect quotes them. Compute the target with the same `ShardingRule` the queries use. The method uses the `db.DB` instance's dialect and executes through `Exec`, so an open transaction in ctx is used.

Both tables must be in the same schema. The method does not read `TableMeta.Schema`; for non-default schemas, call it on a `Schema(name)` view. Grant DDL permissions to a dedicated connection or config group, not the everyday read/write pool. MySQL-family DDL commits implicitly, so it cannot be atomic with application writes. New shards clone the template's current schema; existing shards must be migrated separately. Queries never create tables; call this explicitly before the first read or write of a new shard. Concurrent creation on PostgreSQL can still return a catalog uniqueness error; retry it.

## gooq-gen (codegen tool)

```bash
go install github.com/lanceadd/gooq/cmd/gooq-gen@latest             # install the released binary
# or run from a checkout:
cd cmd/gooq-gen && go run . init                                    # scaffold hack/gooq.yaml + hack/template/{table,sharding,model,record}.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
cd cmd/gooq-gen && go run . -l "sqlite::@file(./test.db)"            # SQLite example
```

`gooq-gen` searches for a `hack` directory from the current directory upward and uses `hack/gooq.yaml` when found. CLI flags take priority over the file. For the full command and flag reference, template details, and generated-file examples, see [cmd/gooq-gen/README.MD](cmd/gooq-gen/README.MD).

```yaml
gooq:
  gen:
    link: "mysql:root:pass@tcp(127.0.0.1:3306)/db"
    path: internal
    tenant:
      column: tenant_id
      tables:
        shop_order: org_id
        dict: ""
  tables:
    check:
      ignore:
        - "employee_*"
```

### Workflow

1. Run `gooq-gen init` once to scaffold `hack/gooq.yaml` and the templates.
2. Fill `gooq.gen.link` (and `path` if needed) in `hack/gooq.yaml`, or pass `-l` / `-p` on each run; CLI flags take priority.
3. If the schema has sharded tables, run `gooq-gen tables init` once, then edit `groups` in `hack/gooq.tables.yaml`. See [Sharding](#sharding) for the file format, checks, and ignore rules.
4. After a schema change, run `gooq-gen tables diff` to see added/removed tables, update `hack/gooq.tables.yaml`, then run `gooq-gen tables check`.
5. Run `gooq-gen` (or `gooq-gen -l ... -p ...`) to regenerate `table/` and `model/`.
6. Customize templates under `hack/template/` instead of editing generated files; templates are never overwritten.

Regeneration overwrites the generated files of current tables. Files for tables removed from the database or `hack/gooq.tables.yaml` are not deleted; remove them yourself. `table/gooq_sharding.go` is removed automatically when no sharded groups remain. Write the tenant config before generating when it is used.

- `init` creates `hack/gooq.yaml` and `hack/template/table.tmpl`, `sharding.tmpl`, `model.tmpl`, `record.tmpl` for customization; existing files are never overwritten.
- `-d/--debug` turns on debug logging.
- `-p internal` (the default) writes `internal/table/` and `internal/model/`. The package name is the directory base (`table`, `model`). `table/gooq_sharding.go` is generated only when `hack/gooq.tables.yaml` defines sharded groups.
- Config file: reads `hack/gooq.yaml` under `gooq.gen` for `link` and `path`; CLI options take priority. `gooq.gen.tenant` is read by the CLI and passed into generation. See Multi-tenancy above. `gooq.tables.check.ignore` lists glob patterns; see [Sharding](#sharding) for which tables it skips.
- Templates: `<hack>/template/table.tmpl`, `sharding.tmpl`, `model.tmpl`, and `record.tmpl` take precedence over the embedded templates (delete one to fall back).
- `-l/--link` database link; `-p/--path` output directory (default `internal`).
- Shard groups: `hack/gooq.tables.yaml`, maintained with `gooq-gen tables init`, `tables check`, and `tables diff`. See [Sharding](#sharding) for the format and ignore rules.
- Generates typed table objects (`table/`) and `model/`. `model/user_model.go` defines `UserModel`, a plain struct tagged with `json` and `orm` for `db.Result.Structs` and JSON. `model/user_record.go` defines `UserRecord`, whose fields are `gooq.Property[T]`. `model.NewUserRecord().SetName("john")` marks columns for `ValuesOf`. Nullable columns also get `SetXxxNull`. `ToModel()` copies set values into `UserModel`. `NewUserRecordFrom` marks every column from a `UserModel`.
- Metadata derivation: primary key (`PRI`), auto-increment, soft delete (columns folding to `deletedat` or `deleteat`, such as `deleted_at` / `delete_at`), unique (`UNI`), `LocalType` markers; field naming (`id` → `Id`, matching `gf gen dao` output); a column whose name collides with a generated field or method fails generation and reports the column and the conflicting declaration.
- Built-in connection drivers live in `drivers/mysql`, `drivers/mariadb`, `drivers/tidb`, `drivers/oceanbase`, `drivers/pgsql`, `drivers/gaussdb`, `drivers/dm`, and `drivers/sqlite`. `gooq-gen` imports them. Library code blank-imports the one it uses. Link prefixes are `mysql`, `mariadb`, `tidb`, `oceanbase`, `pgsql`/`postgres`, `gaussdb`, `dm`, and `sqlite`.

## Testing

```bash
go test ./... -count=1                  # core packages: rendering, dsl, db, fn, drivers
cd cmd/gooq-gen && go test ./...        # gooq-gen end-to-end (sqlite)

# Live driver suites are behind build tags and read the link from an env var.
GOOQ_MYSQL_LINK="mysql:root:pass@tcp(127.0.0.1:3306)/test" go test -tags mysql ./drivers/mysql
GOOQ_PG_LINK="pgsql:postgres:postgres@tcp(127.0.0.1:5432)/test" go test -tags pgsql ./drivers/pgsql
```

Tags and env vars: `mysql`/`GOOQ_MYSQL_LINK`, `mariadb`/`GOOQ_MARIADB_LINK`, `tidb`/`GOOQ_TIDB_LINK`, `oceanbase`/`GOOQ_OB_LINK`, `pgsql`/`GOOQ_PG_LINK`, `gaussdb`/`GOOQ_GAUSSDB_LINK`, `dm`/`GOOQ_DM_LINK`.

## License

MIT

gooq's design grew out of the shortcomings found while developing GoFrame's gdb, combined with ideas from [jOOQ](https://www.jooq.org/). The code evolved from the TableFields and SQL builder in GoFrame's gdb and is now developed independently.
