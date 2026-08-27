# gooq

`gooq` is a type-safe SQL query DSL for [GoFrame](https://goframe.org), inspired by [jOOQ](https://www.jooq.org/).

- **Type-safe** — `Field[T]` carries the column type at compile time: `User.Age.Gt(18)` compiles, `User.Age.Gt("x")` doesn't.
- **Offline rendering** — `ToSql(dialect)` produces SQL + args anywhere, without a database connection; execute with the standard library, gdb, or gooq's built-in gdb fusion.
- **Dialect aware** — MySQL / PostgreSQL / SQLite built in; drivers override rendering details via `RegisterDialect`.
- **Zero runtime guessing** — soft-delete conditions, auto-increment skipping, full-column derivation all come from static `TableMeta` written at codegen time.

All rendered output shown below comes from test assertions — copy-paste friendly.

## Quick Start

### 1. Generate table objects

```bash
cd cmd/ggen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

Generates three artifacts per table in one shot: `do/` (DO structs), `entity/` (entities with `json`/`orm` tags), `table/` (typed table objects).

### 2. Hand-write a table object (or use the generated one)

```go
type UserTable struct {
    *gooq.TableBase
    ID        gooq.Field[int64]
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
func newUserTable(alias string) *UserTable {
    t := &UserTable{TableBase: gooq.NewTableBase(UserMeta)}
    if alias != "" {
        t.TableBase = t.TableBase.As(alias)
    }
    t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
    t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
    t.Age = gooq.NewFieldAt[int](t.TableBase, "age")
    t.Status = gooq.NewFieldAt[string](t.TableBase, "status")
    t.CreatedAt = gooq.NewFieldAt[time.Time](t.TableBase, "created_at")
    t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
    return t
}

var User = newUserTable("") // package-level table object

func (t *UserTable) As(alias string) *UserTable    { return newUserTable(alias) } // JOINs / self-joins
func (t *UserTable) Clone() *UserTable             { return newUserTable("") }
```

### 3. First query

```go
import "github.com/lanceadd/gooq"

sql, args, err := gooq.Select(User.ID, User.Name).
    From(User).
    Where(User.Age.Gt(18)).
    Order(User.ID.Desc()).
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
err := gooq.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).
    Where(User.Age.Gt(18)).
    Scan(ctx, &users)
```

## Select

### Basics

```go
// All columns / set difference / alias / distinct
gooq.Select(User.AllFields()).From(User)                                     // SELECT `user`.`id`, `user`.`name`, ...
gooq.Select(User.ID).From(User).FieldsEx(User.CreatedAt, User.DeletedAt)     // difference: remaining columns
gooq.Select(User.Name.As("nickname")).From(User)                             // SELECT `user`.`name` AS nickname
gooq.Select(User.ID).From(User).Distinct()                                   // SELECT DISTINCT ...

// Paging
gooq.Select(User.ID).From(User).Limit(10)              // ... LIMIT 10
gooq.Select(User.ID).From(User).Offset(20).Limit(10)   // ... LIMIT 10 OFFSET 20
gooq.Select(User.ID).From(User).Page(2, 10)            // ... LIMIT 10 OFFSET 10（1-based pages）

// Ordering (NullsFirst/NullsLast render on PG only)
gooq.Select(User.ID).From(User).Order(User.Age.Desc(), User.ID.Asc()).ToSql(gooq.DialectPgsql)
// SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL ORDER BY "user"."age" DESC, "user"."id" ASC
gooq.Select(User.ID).From(User).Order(User.Age.Desc().NullsLast()).ToSql(gooq.DialectPgsql)
// ... ORDER BY "user"."age" DESC NULLS LAST
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
OrderItem.UserID.EqExpr(User.ID)      // `order_item`.`user_id` = `user`.`id`
User.ID.InExpr(subquery)              // `user`.`id` IN (SELECT ...)

// Combining: AND / OR / NOT.
gooq.Select(User.ID).From(User).
    Where(gooq.OR(User.Age.Lt(18), User.Status.Eq("vip"))).
    And(User.Name.Like("j%")).
    ToSql(gooq.DialectMySQL)
// ... WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL
// args: []any{18, "vip", "j%"}

// Dynamic assembly: Clone a base builder for reuse.
base := gooq.Select(User.ID).From(User)
q1 := base.Clone().Where(User.Age.Gt(18))
q2 := base.Clone().Where(User.Status.Eq("vip")) // independent
```

### JOINs

```go
u := User.As("u")
ur := UserRole.As("ur")
r := Role.As("r")

gooq.Select(u.ID, r.Name).From(u).
    InnerJoin(ur).On(ur.UserID.EqExpr(u.ID)).
    InnerJoin(r).On(r.ID.EqExpr(ur.RoleID)).
    Where(r.Name.Eq("admin")).
    ToSql(gooq.DialectMySQL)
// SELECT `u`.`id`, `r`.`name` FROM `user` AS u INNER JOIN `user_role` AS ur ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS r ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL

// Self-join: two alias instances of the same table.
u1 := User.As("u1")
u2 := User.As("u2")
gooq.Select(u1.ID, u2.ID).From(u1).InnerJoin(u2).On(u1.ID.EqExpr(u2.ID))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS u1 INNER JOIN `user` AS u2 ON `u1`.`id` = `u2`.`id`

// LATERAL derived tables (InnerLateral maps to CROSS JOIN LATERAL on SQLite).
lt := gooq.Select(gooq.CountFunc(UserRole.UserID).As("cnt")).
    From(UserRole).Where(UserRole.UserID.EqExpr(u.ID)).As("lt")
gooq.Select(u.ID, lt.Field("cnt")).From(u).LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).ToSql(gooq.DialectPgsql)
// ... LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 ...
```

### Subqueries

```go
sub := gooq.Select(Role.ID).From(Role).Where(Role.Name.Eq("admin"))

gooq.Select(User.ID).From(User).Where(User.ID.InExpr(sub))
// ... WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) ...

gooq.Select(User.ID).From(User).Where(gooq.Exists(sub))
// ... WHERE EXISTS (SELECT ...) ...
gooq.Select(User.ID).From(User).Where(gooq.NotExists(sub))

// Derived tables.
gooq.Select(User.ID).From(
    gooq.Select(User.ID).From(User).Where(User.Age.Gt(18)).As("t"),
)
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS t

// Correlated subqueries: reference the outer alias inside.
u := User.As("u")
ur := UserRole.As("ur")
gooq.Select(u.ID).From(u).Where(gooq.Exists(
    gooq.Select(ur.UserID).From(ur).Where(ur.UserID.EqExpr(u.ID)),
))
// ... WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS ur WHERE `ur`.`user_id` = `u`.`id`) ...
```

### Grouping & aggregation

```go
gooq.Select(User.Status, gooq.CountFunc(User.ID)).
    From(User).
    Group(User.Status).
    Having(gooq.Gt(gooq.CountFunc(User.ID), 2)).
    ToSql(gooq.DialectMySQL)
// SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?
// args: []any{2}

// Grouping extensions (dialect-aware: Rollup on MySQL/PG, Cube/GroupingSets PG-only — error on MySQL).
gooq.Select(User.Status).From(User).GroupRollup(User.Status).ToSql(gooq.DialectMySQL)
// ... GROUP BY `user`.`status` WITH ROLLUP
gooq.Select(User.Status).From(User).GroupCube(User.Status).ToSql(gooq.DialectPgsql)
// ... GROUP BY CUBE("user"."status")
```

### Set operations & CTEs

```go
gooq.Select(User.ID).From(User).Where(User.Age.Eq(1)).
    UnionAll(gooq.Select(User.ID).From(User).Where(User.Age.Eq(2)))
// ... UNION ALL ...

gooq.Select(User.ID).From(User).
    Intersect(gooq.Select(User.ID).From(User)).
    Except(gooq.Select(User.ID).From(User))
// ... INTERSECT ... EXCEPT ...

// CTE / recursive CTE.
gooq.With("adults", gooq.Select(User.ID).From(User).Where(User.Age.Gt(18))).
    Fields(gooq.Cte("adults").Field("id")).From(gooq.Cte("adults")).ToSql(gooq.DialectPgsql)
// WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"

gooq.WithRecursive("t", gooq.Select(User.ID).From(User).Where(User.ID.Eq(1)))
// WITH RECURSIVE t AS (...)
```

### Expressions & functions

```go
// String / math / date / aggregate / general.
gooq.ConcatFunc(User.Name, gooq.Str(","), User.Status)   // CONCAT(`user`.`name`, ',', `user`.`status`)
gooq.CoalesceFunc(User.Name, gooq.Str("unknown"))        // COALESCE(`user`.`name`, 'unknown')
gooq.SubstringFunc(User.Name, 1, 3)                      // SUBSTRING(`user`.`name`, ?, ?)
gooq.DateDiffFunc(User.CreatedAt, gooq.NowFunc())        // DATEDIFF(`user`.`created_at`, NOW())
gooq.CountDistinctFunc(User.Status)                      // COUNT(DISTINCT `user`.`status`)

// Date formatting across dialects: one expression, per-dialect rendering.
gooq.Select(gooq.DateFormatFunc(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectMySQL)
// SELECT DATE_FORMAT(`user`.`created_at`, '%Y-%m-%d') FROM ...
gooq.Select(gooq.DateFormatFunc(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectPgsql)
// SELECT TO_CHAR("user"."created_at", 'YYYY-MM-DD') FROM ...
gooq.Select(gooq.DateFormatFunc(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectSQLite)
// SELECT strftime('%Y-%m-%d', "user"."created_at") FROM ...

// String aggregation (MySQL GROUP_CONCAT / PG STRING_AGG / SQLite GROUP_CONCAT).
gooq.GroupConcatFunc(gooq.GroupConcatOptions{
    Field: User.Name, Separator: "-", OrderBy: []gooq.OrderClause{User.Name.Asc()},
}).ToSql(...)  // GROUP_CONCAT(`user`.`name` ORDER BY `user`.`name` ASC SEPARATOR '-')

// Window functions.
gooq.RankFunc().Over([]gooq.Expression{User.Status}, []gooq.OrderClause{User.Age.Desc()}).As("r")
// RANK() OVER (PARTITION BY `user`.`status` ORDER BY `user`.`age` DESC) AS r
gooq.RowNumberFunc().Over(nil, []gooq.OrderClause{User.ID.Asc()})
// ROW_NUMBER() OVER (ORDER BY `user`.`id` ASC)
gooq.SumFunc(User.Age).OverFrame(
    []gooq.Expression{User.Status},
    []gooq.OrderClause{User.ID.Asc()},
    gooq.RowsFrame("UNBOUNDED PRECEDING", "CURRENT ROW"),
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
gooq.Select(User.ID).From(User).Where(gooq.Raw("age > ?", 18))

// Custom operators: register + invoke.
gooq.OperatorFunc("JSON_EXTRACT", func(args ...any) (string, []any, error) {
    return fmt.Sprintf("JSON_EXTRACT(%s, %s)", args[0], args[1]), nil, nil
}, "mysql")
gooq.Select(gooq.Func("JSON_EXTRACT", User.Name, gooq.Str("$.key"))).From(User)
// SELECT JSON_EXTRACT(`user`.`name`, '$.key') FROM ...
```

### Row locks

```go
gooq.Select(User.ID).From(User).LockForUpdate().ToSql(gooq.DialectMySQL)   // ... FOR UPDATE
gooq.Select(User.ID).From(User).LockInShareMode().ToSql(gooq.DialectMySQL) // ... LOCK IN SHARE MODE
gooq.Select(User.ID).From(User).LockInShareMode().ToSql(gooq.DialectPgsql) // ... FOR SHARE
gooq.Select(User.ID).From(User).LockForUpdate().ToSql(gooq.DialectSQLite)  // ignored on SQLite
```

## DML

### Insert

```go
// Entity records: zero values and auto-increment columns are skipped.
gooq.Insert(User).Record(model.User{Name: "john", Age: 18})
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?)

// Positional columns/values, repeated Values for batch.
gooq.Insert(User).Columns(User.Name, User.Age).Values("a", 1).Values("b", 2)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)

// Record and Columns/Values can be mixed.
gooq.Insert(User).Record(model.User{Name: "a", Age: 1}).Columns(User.Name, User.Age).Values("b", 2)

// Chunked execution: RowsAffected aggregated.
gooq.Insert(User).Records([]model.User{{Name: "a"}, {Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)

// INSERT ... SELECT.
gooq.InsertFrom(User, gooq.Select(User.Name).From(User).Where(User.Age.Gt(18)))
// INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL
```

### Update

```go
// Chained Set.
gooq.Update(User).Set(User.Name, "x").Set(User.Age, 20).Where(User.ID.Eq(1))
// UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?

// Map full update.
gooq.Update(User).Data(map[string]any{"age": 1, "name": "x"})
// UPDATE `user` SET `age` = ?, `name` = ?

// Expression values render as SQL fragments (field arithmetic, Raw, etc.).
gooq.Update(User).Set(User.Age, User.Age.Add(1)).Where(User.ID.Eq(1))
// UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE `user`.`id` = ?
// (single-record Update via Record is not supported; use Set/Data + Where)

// Batch by primary key (or Keys(...)), RowsAffected aggregated.
gooq.Update(User).Records([]model.User{{Id: 1, Name: "a"}, {Id: 2, Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// Multi-table update (MySQL JOIN / PG+SQLite FROM).
u := User.As("u"); r := Role.As("r")
gooq.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.ID.EqExpr(u.ID)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS u INNER JOIN `role` AS r ON `r`.`id` = `u`.`id` SET `status` = ?
gooq.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.ID.EqExpr(u.ID)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS u SET "status" = $1 FROM "role" AS r WHERE "r"."id" = "u"."id"
```

### Delete

```go
// Soft-delete tables: DELETE auto-rewrites to UPDATE deleted_at.
gooq.Delete(User).Where(User.ID.Eq(1))
// UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?

// Unscoped(): real DELETE.
gooq.Delete(User).Unscoped().Where(User.ID.Eq(1))
// DELETE FROM `user` WHERE `user`.`id` = ?

// Non-soft-delete tables delete directly.
gooq.Delete(UserRole).Where(UserRole.ID.Eq(1))
// DELETE FROM `user_role` WHERE `user_role`.`id` = ?

// Batch delete by primary key (soft delete honored).
gooq.Delete(User).Records([]model.User{{Id: 1}, {Id: 2}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
```

### Upsert & Returning

```go
// MySQL: ON DUPLICATE KEY UPDATE.
gooq.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.ID).DoUpdate(User.Name, "x").ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)

// MySQL: INSERT IGNORE.
gooq.Insert(User).Columns(User.Name).Values("a").DoNothing().ToSql(gooq.DialectMySQL)
// INSERT IGNORE INTO `user` (`name`) VALUES (?)

// PG: ON CONFLICT.
gooq.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.ID).DoUpdate(User.Name, "x").ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3
gooq.Insert(User).Columns(User.Name).Values("a").
    OnConflictKey(User.ID).DoNothing().ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING

// Returning: PG / SQLite（error on MySQL）.
gooq.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).
    Returning(User.ID).ToSql(gooq.DialectPgsql)
// UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"
```

## Execution (gooq + gdb fusion)

```go
// Query + scan into a struct slice / scalar.
users := []model.User{}
err := gooq.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).Where(User.Age.Gt(18)).Order(User.ID.Desc()).Limit(10).
    Scan(ctx, &users)

count := int64(0)
err = gooq.Select(gooq.CountFunc(User.ID)).From(User).UseDB(gdb.DB()).Scan(ctx, &count)

// DML.
_, err = gooq.Insert(User).Record(model.User{Name: "john"}).UseDB(gdb.DB()).Exec(ctx)
_, err = gooq.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).UseDB(gdb.DB()).Exec(ctx)

// Transaction: UseTX binds the tx connection.
tx, _ := gdb.DB().Begin(ctx)
_, err = gooq.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).UseTX(tx).Exec(ctx)
tx.Commit()

// Convenience: Count auto-wraps COUNT(*), Exists wraps SELECT EXISTS(...).
total, err := gooq.SelectFrom(User).Where(User.Status.Eq("vip")).UseDB(gdb.DB()).Count(ctx)
exists, err := gooq.Select(User.ID).From(User).Where(User.Account.Eq("x")).UseDB(gdb.DB()).Exists(ctx)

// Typed row reads: the field type T is consumed at compile time.
row, err := gooq.Select(User.ID, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).UseDB(gdb.DB()).Row(ctx)
id := gooq.Get(row, User.ID)      // int64
name := gooq.Get(row, User.Name)  // string

// The dialect is derived from the gdb driver name; UseDB can be re-bound
// for multi-database / read-write splitting.
```

### Caching

```go
// Inject a global cache adapter (e.g. a gredis-backed implementation).
gooq.SetCacheAdapter(redisAdapter)
gooq.SetHashCacheAdapter(redisHashAdapter)

// Single-query cache: all query methods (Scan/Row/Rows/Count/Exists) go through it.
// Cache() with no arg or a zero option disables caching; a custom Name fixes the key
// (useful for manual invalidation).
err := gooq.Select(User.AllFields()).From(User).
    Cache(gooq.CacheOption{Duration: time.Minute}).
    UseDB(gdb.DB()).Scan(ctx, &users)

// Composite query: count runs first, rows after; count=0 short-circuits without querying rows.
rows, total, err := gooq.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// PageCache caches the composite query as one hash record (fields "count" and "rows").
// Requires SetHashCacheAdapter; the key includes limit/offset so each page is cached
// independently. RowsField/CountField override the field names; Force caches empty
// results (count=0) which are skipped by default.
rows, total, err = gooq.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    PageCache(gooq.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// ScanAndCount is the typed variant: scans data into dest and returns the total.
// It shares the same data cache (unified Result JSON, field "rows") with RowsAndCount.
var vips []User
total, err = gooq.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    PageCache(gooq.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).ScanAndCount(ctx, &vips)
```

## Dialects

| Dialect | Placeholder | Quote | Pagination |
| --- | --- | --- | --- |
| mysql | `?` | `` ` `` | LIMIT |
| pgsql | `$n` | `"` | LIMIT |
| sqlite | `?` | `"` | LIMIT |

- Unregistered dialects fall back to default rendering; `RegisterDialect` overrides built-ins incrementally.
- Schema-qualified tables render `schema.table.column` (alias shadows schema); ggen fills schema for PG only (`current_schema()`).
- Dialect-sensitive behavior is handled internally: pagination, `NullsFirst/NullsLast` (PG only), row locks (`FOR UPDATE`/`LOCK IN SHARE MODE`/`FOR SHARE`), LATERAL mapping (SQLite `INNER JOIN LATERAL` → `CROSS JOIN LATERAL`), upsert syntax, `DATE_FORMAT`/`TO_CHAR`/`strftime`, `GROUP_CONCAT`/`STRING_AGG`.

## ggen (codegen tool)

```bash
cd cmd/ggen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

- `-l/--link` database link (required); `-p/--path` output directory (default `internal`); `-t/--tpl` export built-in templates to `./template` for customization (local templates take precedence).
- Metadata derivation: primary key (`PRI`), auto-increment, soft delete (column-name convention), unique (`UNI`), `LocalType` markers; Go naming conventions (`id` → `ID`).
- Built-in drivers: mysql/pgsql/sqlite; others by uncommenting the import in `internal/cmd/cmd.go`.

## Testing

```bash
go test ./ -count=1                # core library (rendering assertions)
cd cmd/ggen && go test ./...       # ggen end-to-end (sqlite)
cd test && go test ./...           # integration against a real MySQL
```

## License

MIT
