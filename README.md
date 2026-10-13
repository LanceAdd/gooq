# gooq

`gooq` is a type-safe SQL query DSL for [GoFrame](https://goframe.org), inspired by [jOOQ](https://www.jooq.org/).

- **Type-safe** — `Field[T]` carries the column type at compile time: `User.Age.Gt(18)` compiles, `User.Age.Gt("x")` doesn't.
- **Offline rendering** — `ToSql(dialect)` produces SQL + args anywhere, without a database connection; execute with the standard library or `gooq/db`.
- **Dialect aware** — MySQL / MariaDB / TiDB / OceanBase / PostgreSQL / GaussDB / Dameng / SQLite each implement `DialectSpec`. A new dialect registers the whole spec. Connection drivers register separately through `db.Register`.
- **Zero runtime guessing** — soft-delete conditions and the full column list come from static `TableMeta` written at codegen time. INSERT and UPDATE write only columns that were set.
- **Layered packages** — `gooq` (core: fields, tables, expression language, dialects, render contract), `gooq/db` (connections, transactions, scanning), `gooq/dsl` (query and DML builders), `gooq/fn` (function library); generated table code only imports `gooq`.

All rendered output shown below comes from test assertions — copy-paste friendly.

## Behavior change

Multiple `Where` calls are combined with AND. Field comparisons return a `Condition`, which chains `.And`, `.Or`, and `.Not` left-associatively. A list of expressions uses `gooq.AND`, `gooq.OR`, and `gooq.NOT`. `WhereOr` has been removed.

## Quick Start

### 1. Generate table objects

```bash
cd cmd/gooq-gen && go run . init                                    # optional: scaffold hack/gooq.yaml + hack/template/{table,model,record}.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

Generates typed gooq table objects (`table/`) and `model/` for all tables in one shot. Each table produces `user_model.go` (`UserModel`) and `user_record.go` (`UserRecord`). `gooq-gen` can also read `hack/gooq.yaml` under the `gooq.gen.table` node instead of CLI options.

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
// sql:  SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10
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
dsl.Select(User.Id).From(User).Offset(20)             // MySQL: LIMIT 18446744073709551615 OFFSET 20; PG/SQLite: OFFSET 20
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
// SELECT `u`.`id`, `r`.`name` FROM `user` AS `u` INNER JOIN `user_role` AS `ur` ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS `r` ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL AND `r`.`deleted_at` IS NULL

// Self-join: two alias instances of the same table.
u1 := User.As("u1")
u2 := User.As("u2")
dsl.Select(u1.Id, u2.Id).From(u1).InnerJoin(u2).On(u1.Id.EqExpr(u2.Id))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS `u1` INNER JOIN `user` AS `u2` ON `u1`.`id` = `u2`.`id` WHERE `u1`.`deleted_at` IS NULL AND `u2`.`deleted_at` IS NULL

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
// ... WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) ...

dsl.Select(User.Id).From(User).Where(gooq.Exists(sub))
// ... WHERE EXISTS (SELECT ...) ...
dsl.Select(User.Id).From(User).Where(gooq.NotExists(sub))

// Derived tables.
dsl.Select(User.Id).From(
    dsl.Select(User.Id).From(User).Where(User.Age.Gt(18)).As("t"),
)
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS `t`

// Correlated subqueries: reference the outer alias inside.
u := User.As("u")
ur := UserRole.As("ur")
dsl.Select(u.Id).From(u).Where(gooq.Exists(
    dsl.Select(ur.UserId).From(ur).Where(ur.UserId.EqExpr(u.Id)),
))
// ... WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS `ur` WHERE `ur`.`user_id` = `u`.`id`) ...
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
```

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
// INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL
```

### Update

```go
// Chained Set.
dsl.Update(User).Set(User.Name, "x").Set(User.Age, 20).Where(User.Id.Eq(1))
// UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?

// Map full update.
dsl.Update(User).Data(map[string]any{"age": 1, "name": "x"})
// UPDATE `user` SET `age` = ?, `name` = ?

// Expression values render as SQL fragments (field arithmetic, Raw, etc.).
dsl.Update(User).Set(User.Age, User.Age.Add(1)).Where(User.Id.Eq(1))
// UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE `user`.`id` = ?

// One record row becomes SET. Several rows update by primary key. RowsAffected is aggregated.
dsl.Update(User).ValuesOf(model.NewUserRecord().SetName("x").SetAge(0)).Where(User.Id.Eq(1))
dsl.Update(User).ValuesOf(
    model.NewUserRecord().SetId(1).SetName("a"),
    model.NewUserRecord().SetId(2).SetName("b"),
).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// Multi-table update (MySQL JOIN / PG+SQLite FROM).
u := User.As("u"); r := Role.As("r")
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS `u` INNER JOIN `role` AS `r` ON `r`.`id` = `u`.`id` SET `status` = ?
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS "u" SET "status" = $1 FROM "role" AS "r" WHERE "r"."id" = "u"."id"
// Any LEFT / RIGHT / FULL join makes PostgreSQL, GaussDB, SQLite, and Dameng return an error at render time.
// MySQL keeps the outer join and writes it before SET.
```

### Delete

```go
// Soft-delete tables: DELETE auto-rewrites to UPDATE deleted_at.
dsl.Delete(User).Where(User.Id.Eq(1))
// UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?

// Unscoped(): real DELETE.
dsl.Delete(User).Unscoped().Where(User.Id.Eq(1))
// DELETE FROM `user` WHERE `user`.`id` = ?

// Non-soft-delete tables delete directly.
dsl.Delete(UserRole).Where(UserRole.Id.Eq(1))
// DELETE FROM `user_role` WHERE `user_role`.`id` = ?

// Batch delete by primary key (soft delete honored).
dsl.Delete(User).ValuesOf(
    model.NewUserRecord().SetId(1),
    model.NewUserRecord().SetId(2),
).Exec(ctx)
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

// Returning: PG / SQLite（error on MySQL）.
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

// Transaction puts the tx into ctx. Queries inside the callback join it.
database, err := db.Instance()
err = database.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
    _, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).Exec(ctx)
    return err
})

// Convenience: Count auto-wraps COUNT(*). Exists wraps a dialect-specific existence query:
// Dameng uses SELECT COUNT(*) FROM (...) WHERE ROWNUM = 1; the others use SELECT EXISTS(...).
total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).Count(ctx)
exists, err := dsl.Select(User.Id).From(User).Where(User.Account.Eq("x")).Exists(ctx)

// Typed row reads: the field type T is consumed at compile time.
row, err := dsl.Select(User.Id, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).Row(ctx)
id := db.Get(row, User.Id)      // int64
name := db.Get(row, User.Name)  // string

// RowOne enforces at most one row: nil when none matched, error when 2+ matched
// (Take-first-row semantics stay in Row; explicit Limit/Page/Cache is rejected).
row, err = dsl.Select(User.Id, User.Name).From(User).
    Where(User.Account.Eq("x")).RowOne(ctx)

// DB("orders") uses that configuration group. Omitting DB uses the default group.
// LoadConfig reads gooq.database, gooq.defaultGroup, and gooq.logger through gcfg.
// Runtime reads only a file named gooq, such as gooq.yaml, and does not read config.yaml.
// database is a list of groups: name is the group, endpoints are its master and slave nodes.
// A single group may omit name and is stored as default. Every group needs a name when there is more than one.
// defaultGroup selects the group used when the name is omitted. It must be one of the groups in this file; omitted, it is default.
// SetDebug logs SQL, SetDryRun skips execution, Prepare prepares a statement,
// and Schema switches the MySQL database or PostgreSQL search_path.
// Reads use a slave when the group has one; writes and BEGIN use a master.
// Master() and Slave() force the pool. A transaction is taken from ctx.
```

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

### Caching

```go
// Inject a global cache adapter (e.g. a gredis-backed implementation).
dsl.SetCacheAdapter(redisAdapter)
dsl.SetHashCacheAdapter(redisHashAdapter)

// Single-query cache: Value, Column, Row, Rows, Count, and Exists go through it.
// Cache() with no arg or a zero option disables caching; a custom Name fixes the key
// (useful for manual invalidation). Argument keys include the type name, so
// int64(1) and "1" do not share a key.
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

- `ToSql` requires a registered `DialectSpec`. An unknown dialect returns an error. Embed `MySQLDialect`, `PostgresDialect`, or `SQLiteDialect` and override the methods that differ, then call `RegisterDialect`.
- Opening a connection requires a blank import of the matching driver package (`drivers/mysql`, `drivers/mariadb`, `drivers/tidb`, `drivers/oceanbase`, `drivers/pgsql`, `drivers/gaussdb`, `drivers/dm`, `drivers/sqlite`). The driver registers itself with `db.Register`. `gooq-gen` imports these drivers.
- MariaDB, TiDB, and OceanBase use the MySQL wire protocol and the MySQL statement shape. MariaDB reads columns from `information_schema`, so a JSON value stored as `longtext` with a `json_valid` check is reported as `json`. On TiDB, `LockInShareMode` returns an error by default. After `tidb_enable_noop_functions` is enabled the statement runs, but it does not take a shared lock. `LockForUpdate` works.
- GaussDB means openGauss. Quoting, placeholders, row locks, `RETURNING`, deletes, and updates follow PostgreSQL. `DATE_ADD` is interval multiplication, not `make_interval`. Upsert is `ON DUPLICATE KEY UPDATE`, and the inserted value is `VALUES(col)`. `DoNothing` returns an error because GaussDB has no `ON CONFLICT DO NOTHING`. The `database/sql` driver name is `opengauss`.
- Dameng (`dm`) quotes identifiers in uppercase so they match names stored from unquoted DDL. Upsert is `MERGE` and needs `OnConflictKey`; the conflict column must be in the insert list. `DoNothing` inserts only when the row is not matched. There is no `RETURNING`. `Exists` renders `SELECT COUNT(*) FROM (subquery) WHERE ROWNUM = 1`. `GROUP_CONCAT` renders as `LISTAGG`, `DATE_FORMAT` as `TO_CHAR`, `IFNULL` as `NVL`, and `DATE_ADD` as `DATEADD`. `time.Time` is bound as a timestamp string; the zero time becomes `NULL`. An IANA timezone in the config becomes a minute offset, and an offset outside ±12 hours fails. `DELETE` with `JOIN` and `UPDATE` with an outer join return an error. Identity columns are not marked `AutoIncrement`. The `database/sql` driver name is `dm`. Link example: `dm:SYSDBA:SYSDBA001@tcp(127.0.0.1:5236)/SYSDBA`.
- Schema-qualified tables render `schema.table.column` (alias shadows schema); gooq-gen fills schema for PG only (`current_schema()`).
- `DialectSpec` covers statement shape: pagination, row locks, `JOIN` rewrites, upsert, delete, multi-table `UPDATE` (`RenderUpdate`), and existence queries (`RenderExists`). Function SQL such as `DATE_FORMAT`, `GROUP_CONCAT`, `IFNULL`, `CURDATE`, `DATE_ADD`, `DATEDIFF`, `ROUND`, and `MEDIAN` stays on the concrete dialect type. A dialect that does not implement one returns `dialect %q does not support ...` from `ToSql`. SQLite upsert uses `ON CONFLICT`.
- Table, column, and function aliases go through `Quote`. `numeric` / `decimal` generate `string`. PostgreSQL arrays generate a slice of the element type.
- PostgreSQL 16 can run `STRING_AGG(DISTINCT ...)`, `JSON_ARRAYAGG`, and `ANY_VALUE`.

## gooq-gen (codegen tool)

```bash
cd cmd/gooq-gen && go run . init                                    # scaffold hack/gooq.yaml + hack/template/{table,model,record}.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

- `init` creates `hack/gooq.yaml` and `hack/template/table.tmpl`, `model.tmpl`, `record.tmpl` for customization; existing files are never overwritten.
- Config file: reads `hack/gooq.yaml` under the `gooq.gen.table` node; CLI options take priority.
- Templates: `<hack>/template/table.tmpl`, `model.tmpl`, and `record.tmpl` take precedence over the embedded templates (delete one to fall back).
- `-l/--link` database link; `-p/--path` output directory (default `internal`).
- Generates typed table objects (`table/`) and `model/`. `model/user_model.go` defines `UserModel`, a plain struct tagged with `json` and `orm` for `db.Result.Structs` and JSON. `model/user_record.go` defines `UserRecord`, whose fields are `gooq.Property[T]`. `model.NewUserRecord().SetName("john")` marks columns for `ValuesOf`. Nullable columns also get `SetXxxNull`. `ToModel()` copies set values into `UserModel`. `NewUserRecordFrom` marks every column from a `UserModel`.
- Metadata derivation: primary key (`PRI`), auto-increment, soft delete (column-name convention), unique (`UNI`), `LocalType` markers; field naming (`id` → `Id`, matching `gf gen dao` output); columns colliding with table-base methods get a `Field` suffix (`meta` → `MetaField`).
- Built-in connection drivers live in `drivers/mysql`, `drivers/mariadb`, `drivers/tidb`, `drivers/oceanbase`, `drivers/pgsql`, `drivers/gaussdb`, `drivers/dm`, and `drivers/sqlite`. `gooq-gen` imports them. Library code blank-imports the one it uses. Link prefixes are `mysql`, `mariadb`, `tidb`, `oceanbase`, `pgsql`/`postgres`, `gaussdb`, `dm`, and `sqlite`.

## Testing

```bash
go test ./ -count=1                     # core library (rendering assertions)
cd cmd/gooq-gen && go test ./...        # gooq-gen end-to-end (sqlite)
cd test && go test ./...                # integration against a real MySQL
```

## License

MIT
