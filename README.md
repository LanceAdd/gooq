# gooq

`gooq` is a type-safe SQL query DSL for [GoFrame](https://goframe.org), inspired by [jOOQ](https://www.jooq.org/).

- **Type-safe** — `Field[T]` carries the column type at compile time: `User.Age.Gt(18)` compiles, `User.Age.Gt("x")` doesn't.
- **Offline rendering** — `ToSql(dialect)` produces SQL + args anywhere, without a database connection; execute with the standard library, gdb, or gooq's built-in gdb fusion.
- **Dialect aware** — MySQL / PostgreSQL / SQLite built in; drivers override rendering details via `RegisterDialect`.
- **Zero runtime guessing** — soft-delete conditions, auto-increment skipping, full-column derivation all come from static `TableMeta` written at codegen time.
- **Layered packages** — `gooq` (core: fields, tables, expression language, dialects, render contract), `gooq/dsl` (query & DML builders), `gooq/fn` (function library); generated table code only imports `gooq`.

All rendered output shown below comes from test assertions — copy-paste friendly.

## Quick Start

### 1. Generate table objects

```bash
cd cmd/gooq-gen && go run . init                                    # optional: scaffold hack/gooq.yaml + hack/template/table.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

Generates typed gooq table objects (`table/`) for all tables in one shot. `do/`/`entity/` structs are left to GoFrame's own `gf gen dao`; `gooq-gen` can also read `hack/gooq.yaml` under the `gooq.gen.table` node instead of CLI options.

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
// Standard library or any driver.
rows, _ := db.Query(sql, args...)

// Or gdb fusion: bind a database and scan directly.
users := []model.User{}
err := dsl.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).
    Where(User.Age.Gt(18)).
    Scan(ctx, &users)
```

## Select

### Basics

```go
// All columns / set difference / alias / distinct
dsl.Select(User.AllFields()).From(User)                                     // SELECT `user`.`id`, `user`.`name`, ...
dsl.Select(User.Id).From(User).FieldsEx(User.CreatedAt, User.DeletedAt)     // difference: remaining columns
dsl.Select(User.Name.As("nickname")).From(User)                             // SELECT `user`.`name` AS nickname
dsl.Select(User.Id).From(User).Distinct()                                   // SELECT DISTINCT ...

// Paging
dsl.Select(User.Id).From(User).Limit(10)              // ... LIMIT 10
dsl.Select(User.Id).From(User).Offset(20).Limit(10)   // ... LIMIT 10 OFFSET 20
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

// Combining: AND / OR / NOT.
dsl.Select(User.Id).From(User).
    Where(gooq.OR(User.Age.Lt(18), User.Status.Eq("vip"))).
    And(User.Name.Like("j%")).
    ToSql(gooq.DialectMySQL)
// ... WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL
// args: []any{18, "vip", "j%"}

// Dynamic assembly: Clone a base builder for reuse.
base := dsl.Select(User.Id).From(User)
q1 := base.Clone().Where(User.Age.Gt(18))
q2 := base.Clone().Where(User.Status.Eq("vip")) // independent
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
// SELECT `u`.`id`, `r`.`name` FROM `user` AS u INNER JOIN `user_role` AS ur ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS r ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL

// Self-join: two alias instances of the same table.
u1 := User.As("u1")
u2 := User.As("u2")
dsl.Select(u1.Id, u2.Id).From(u1).InnerJoin(u2).On(u1.Id.EqExpr(u2.Id))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS u1 INNER JOIN `user` AS u2 ON `u1`.`id` = `u2`.`id`

// LATERAL derived tables (InnerLateral maps to CROSS JOIN LATERAL on SQLite).
lt := dsl.Select(fn.Count(UserRole.UserId).As("cnt")).
    From(UserRole).Where(UserRole.UserId.EqExpr(u.Id)).As("lt")
dsl.Select(u.Id, lt.Field("cnt")).From(u).LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).ToSql(gooq.DialectPgsql)
// ... LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 ...
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
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS t

// Correlated subqueries: reference the outer alias inside.
u := User.As("u")
ur := UserRole.As("ur")
dsl.Select(u.Id).From(u).Where(gooq.Exists(
    dsl.Select(ur.UserId).From(ur).Where(ur.UserId.EqExpr(u.Id)),
))
// ... WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS ur WHERE `ur`.`user_id` = `u`.`id`) ...
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
// RANK() OVER (PARTITION BY `user`.`status` ORDER BY `user`.`age` DESC) AS r
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
// CASE WHEN `user`.`age` > ? THEN 'old' WHEN `user`.`age` > ? THEN 'adult' ELSE 'young' END AS bucket

// Arithmetic & casting.
User.Age.Mul(2)                                 // (`user`.`age` * ?)
gooq.Add(User.Age, gooq.Mul(2, User.Age))       // (`user`.`age` + (? * `user`.`age`))
gooq.Negate(User.Age)                           // (-`user`.`age`)
User.Age.Cast(gooq.LocalTypeString)             // CAST(`user`.`age` AS CHAR)（PG: BIGINT / SQLite: TEXT）

// Raw: structured SQL with parameter binding.
gooq.Raw("JSON_EXTRACT(data, ?)", "$.name")
dsl.Select(User.Id).From(User).Where(gooq.Raw("age > ?", 18))

// Generic functions render NAME(args...) as-is (no dialect awareness).
dsl.Select(fn.New("JSON_EXTRACT", User.Name, gooq.Str("$.key"))).From(User)
// SELECT JSON_EXTRACT(`user`.`name`, '$.key') FROM ...

// Attach a dialect render strategy to any FuncExpr (instance-scoped, no global registry):
concatPipe := fn.New("CONCAT", User.Name, gooq.Str("-x")).
    RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
        if rc.Dialect() == gooq.DialectPgsql {
            return "(" + argsSQL[0] + " || " + argsSQL[1] + ")", nil
        }
        return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")", nil
    })
// Full custom expression nodes can implement gooq.Expression + gooq.Renderer instead.
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
// Entity records: zero values and auto-increment columns are skipped.
dsl.Insert(User).Record(model.User{Name: "john", Age: 18})
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?)

// Positional columns/values, repeated Values for batch.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).Values("b", 2)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)

// Record and Columns/Values can be mixed.
dsl.Insert(User).Record(model.User{Name: "a", Age: 1}).Columns(User.Name, User.Age).Values("b", 2)

// Chunked execution: RowsAffected aggregated.
dsl.Insert(User).Records([]model.User{{Name: "a"}, {Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)

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
// (single-record Update via Record is not supported; use Set/Data + Where)

// Batch by primary key (or Keys(...)), RowsAffected aggregated.
dsl.Update(User).Records([]model.User{{Id: 1, Name: "a"}, {Id: 2, Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// Multi-table update (MySQL JOIN / PG+SQLite FROM).
u := User.As("u"); r := Role.As("r")
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS u INNER JOIN `role` AS r ON `r`.`id` = `u`.`id` SET `status` = ?
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS u SET "status" = $1 FROM "role" AS r WHERE "r"."id" = "u"."id"
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
dsl.Delete(User).Records([]model.User{{Id: 1}, {Id: 2}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
```

### Upsert & Returning

```go
// MySQL: ON DUPLICATE KEY UPDATE.
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.Id).DoUpdate(User.Name, "x").ToSql(gooq.DialectMySQL)
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

## Execution (gooq + gdb fusion)

```go
// Query + scan into a struct slice / scalar.
users := []model.User{}
err := dsl.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).Where(User.Age.Gt(18)).Order(User.Id.Desc()).Limit(10).
    Scan(ctx, &users)

count := int64(0)
err = dsl.Select(fn.Count(User.Id)).From(User).UseDB(gdb.DB()).Scan(ctx, &count)

// DML.
_, err = dsl.Insert(User).Record(model.User{Name: "john"}).UseDB(gdb.DB()).Exec(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).UseDB(gdb.DB()).Exec(ctx)

// Transaction: UseTX binds the tx connection.
tx, _ := gdb.DB().Begin(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).UseTX(tx).Exec(ctx)
tx.Commit()

// Convenience: Count auto-wraps COUNT(*), Exists wraps SELECT EXISTS(...).
total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).UseDB(gdb.DB()).Count(ctx)
exists, err := dsl.Select(User.Id).From(User).Where(User.Account.Eq("x")).UseDB(gdb.DB()).Exists(ctx)

// Typed row reads: the field type T is consumed at compile time.
row, err := dsl.Select(User.Id, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).UseDB(gdb.DB()).Row(ctx)
id := gooq.Get(row, User.Id)      // int64
name := gooq.Get(row, User.Name)  // string

// RowOne enforces at most one row: nil when none matched, error when 2+ matched
// (Take-first-row semantics stay in Row; explicit Limit/Page/Cache is rejected).
row, err = dsl.Select(User.Id, User.Name).From(User).
    Where(User.Account.Eq("x")).UseDB(gdb.DB()).RowOne(ctx)

// The dialect is derived from the gdb driver name; UseDB can be re-bound
// for multi-database / read-write splitting.
```

### Caching

```go
// Inject a global cache adapter (e.g. a gredis-backed implementation).
dsl.SetCacheAdapter(redisAdapter)
dsl.SetHashCacheAdapter(redisHashAdapter)

// Single-query cache: all query methods (Scan/Row/Rows/Count/Exists) go through it.
// Cache() with no arg or a zero option disables caching; a custom Name fixes the key
// (useful for manual invalidation).
err := dsl.Select(User.AllFields()).From(User).
    Cache(dsl.CacheOption{Duration: time.Minute}).
    UseDB(gdb.DB()).Scan(ctx, &users)

// Composite query: count runs first, rows after; count=0 short-circuits without querying rows.
rows, total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// PageCache caches the composite query as one hash record (fields "count" and "rows").
// Requires SetHashCacheAdapter; the key includes limit/offset so each page is cached
// independently. RowsField/CountField override the field names; Force caches empty
// results (count=0) which are skipped by default.
rows, total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// ScanAndCount is the typed variant: scans data into dest and returns the total.
// It shares the same data cache (unified Result JSON, field "rows") with RowsAndCount.
var vips []User
total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).ScanAndCount(ctx, &vips)
```

## Dialects

| Dialect | Placeholder | Quote | Pagination |
| --- | --- | --- | --- |
| mysql | `?` | `` ` `` | LIMIT |
| pgsql | `$n` | `"` | LIMIT |
| sqlite | `?` | `"` | LIMIT |

- Unregistered dialects fall back to default rendering; `RegisterDialect` overrides built-ins incrementally.
- Schema-qualified tables render `schema.table.column` (alias shadows schema); gooq-gen fills schema for PG only (`current_schema()`).
- Dialect-sensitive behavior is handled internally: pagination, row locks (`FOR UPDATE`/`LOCK IN SHARE MODE`/`FOR SHARE`), LATERAL mapping (SQLite `INNER JOIN LATERAL` → `CROSS JOIN LATERAL`), upsert syntax, `DATE_FORMAT`/`TO_CHAR`/`strftime`, `GROUP_CONCAT`/`STRING_AGG`.

## gooq-gen (codegen tool)

```bash
cd cmd/gooq-gen && go run . init                                    # scaffold hack/gooq.yaml + hack/template/table.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

- `init` creates `hack/gooq.yaml` and `hack/template/table.tmpl` for customization; existing files are never overwritten.
- Config file: reads `hack/gooq.yaml` under the `gooq.gen.table` node; CLI options take priority.
- Templates: `<hack>/template/table.tmpl` takes precedence over the embedded template (delete it to fall back).
- `-l/--link` database link; `-p/--path` output directory (default `internal`).
- Generates only typed gooq table objects (`table/`); `do/`/`entity/` are left to `gf gen dao`.
- Metadata derivation: primary key (`PRI`), auto-increment, soft delete (column-name convention), unique (`UNI`), `LocalType` markers; field naming (`id` → `Id`, matching `gf gen dao` output); columns colliding with table-base methods get a `Field` suffix (`meta` → `MetaField`).
- Built-in drivers: mysql/pgsql/sqlite; others by uncommenting the import in `internal/cmd/cmd.go`.

## Testing

```bash
go test ./ -count=1                     # core library (rendering assertions)
cd cmd/gooq-gen && go test ./...        # gooq-gen end-to-end (sqlite)
cd test && go test ./...                # integration against a real MySQL
```

## License

MIT
