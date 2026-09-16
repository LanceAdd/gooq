# gooq

`gooq` 是面向 [GoFrame](https://goframe.org) 的类型安全 SQL 查询 DSL，灵感来自 [jOOQ](https://www.jooq.org/)。

- **类型安全** — `Field[T]` 在编译期携带列类型，`User.Age.Gt(18)` 编译通过，`User.Age.Gt("x")` 直接报错。
- **离线渲染** — `ToSql(dialect)` 在任何环境产出 SQL + 参数，不连库；执行方式任选（标准库、gdb、gooq 内置融合执行）。
- **方言感知** — 内置 MySQL / PostgreSQL / SQLite，驱动可通过 `RegisterDialect` 增量覆盖渲染细节。
- **运行时零猜测** — 软删除自动条件、自增列跳过、全列派生全部来自生成期静态化的 `TableMeta`。
- **分层子包** — `gooq`（核心：字段/表/表达式语言/方言与渲染契约）、`gooq/dsl`（查询与 DML 构建器）、`gooq/fn`（函数库）；生成代码只依赖 `gooq`。

下文所有示例的渲染输出均来自测试断言，可直接运行验证。

## 快速开始

### 1. 生成表对象

```bash
cd cmd/gooq-gen && go run . init                                    # 可选：初始化 hack/gooq.yaml + hack/template/table.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

一次生成所有表的 gooq 类型化表对象（`table/`）。`do/`/`entity/` 结构体交由 GoFrame 自身的 `gf gen dao` 生成；`gooq-gen` 也可不传命令行参数，改为从 `hack/gooq.yaml` 的 `gooq.gen.table` 节点读取配置。

### 2. 手写表对象（或直接用生成产物）

```go
type UserTable struct {
    *gooq.TableBase
    ID        gooq.Field[int64]
    Name      gooq.Field[string]
    Age       gooq.Field[int]
    Status    gooq.Field[string]
    CreatedAt gooq.Field[time.Time]
    DeletedAt gooq.Field[time.Time] // 标记为软删列
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

// 构造函数模式：字段经 NewFieldAt 绑定实例，As/Clone 一行重建。
func newUserTable(alias ...string) *UserTable {
    t := &UserTable{TableBase: gooq.NewTableBase(UserMeta)}
    if len(alias) > 0 {
        t.TableBase = t.TableBase.As(alias[0])
    }
    t.ID = gooq.NewFieldAt[int64](t.TableBase, "id")
    t.Name = gooq.NewFieldAt[string](t.TableBase, "name")
    t.Age = gooq.NewFieldAt[int](t.TableBase, "age")
    t.Status = gooq.NewFieldAt[string](t.TableBase, "status")
    t.CreatedAt = gooq.NewFieldAt[time.Time](t.TableBase, "created_at")
    t.DeletedAt = gooq.NewFieldAt[time.Time](t.TableBase, "deleted_at")
    return t
}

var User = newUserTable() // 包级表对象

func (t *UserTable) As(alias string) *UserTable    { return newUserTable(alias) } // JOIN / 自连接
func (t *UserTable) Clone() *UserTable             { return newUserTable() }
```

### 3. 第一个查询

```go
import (
    "github.com/lanceadd/gooq"     // 核心：Field/Table/Expression/Dialect
    "github.com/lanceadd/gooq/dsl" // 构建器：Select/Insert/Update/Delete
)

sql, args, err := dsl.Select(User.ID, User.Name).
    From(User).
    Where(User.Age.Gt(18)).
    Order(User.ID.Desc()).
    Limit(10).
    ToSql(gooq.DialectMySQL)
// sql:  SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10
// args: []any{18}
```

`deleted_at IS NULL` 自动出现——软删条件来自 `TableMeta`，无需手动过滤。`ToSql()` 不传方言时默认 MySQL 渲染。

### 4. 执行

```go
// 标准库或任意驱动。
rows, _ := db.Query(sql, args...)

// 或 gdb 融合：绑定数据库直接扫描。
users := []model.User{}
err := dsl.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).
    Where(User.Age.Gt(18)).
    Scan(ctx, &users)
```

## 查询

### 基础

```go
// 全列 / 差集 / 别名 / 去重
dsl.Select(User.AllFields()).From(User)                                     // SELECT `user`.`id`, `user`.`name`, ...
dsl.Select(User.ID).From(User).FieldsEx(User.CreatedAt, User.DeletedAt)     // 差集：其余列
dsl.Select(User.Name.As("nickname")).From(User)                             // SELECT `user`.`name` AS nickname
dsl.Select(User.ID).From(User).Distinct()                                   // SELECT DISTINCT ...

// 分页
dsl.Select(User.ID).From(User).Limit(10)              // ... LIMIT 10
dsl.Select(User.ID).From(User).Offset(20).Limit(10)   // ... LIMIT 10 OFFSET 20
dsl.Select(User.ID).From(User).Page(2, 10)            // ... LIMIT 10 OFFSET 10（页码从 1 起）

// 排序（NullsFirst/NullsLast 仅 PG 渲染）
dsl.Select(User.ID).From(User).Order(User.Age.Desc(), User.ID.Asc()).ToSql(gooq.DialectPgsql)
// SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL ORDER BY "user"."age" DESC, "user"."id" ASC
dsl.Select(User.ID).From(User).Order(User.Age.Desc().NullsLast()).ToSql(gooq.DialectPgsql)
// ... ORDER BY "user"."age" DESC NULLS LAST
```

### 条件

```go
// 字段操作符：值类型 T 编译期强约束。
User.Age.Gt(18)                       // `user`.`age` > ?
User.Age.Between(18, 60)              // `user`.`age` BETWEEN ? AND ?
User.Status.In("vip", "admin")        // `user`.`status` IN (?, ?)
User.Name.Like("j%")                  // `user`.`name` LIKE ?
User.DeletedAt.IsNull()               // `user`.`deleted_at` IS NULL

// 表达式操作数（列比较、子查询、Raw）走 EqExpr 系列。
OrderItem.UserID.EqExpr(User.ID)      // `order_item`.`user_id` = `user`.`id`
User.ID.InExpr(subquery)              // `user`.`id` IN (SELECT ...)

// 组合：AND / OR / NOT。
dsl.Select(User.ID).From(User).
    Where(gooq.OR(User.Age.Lt(18), User.Status.Eq("vip"))).
    And(User.Name.Like("j%")).
    ToSql(gooq.DialectMySQL)
// ... WHERE (`user`.`age` < ? OR `user`.`status` = ?) AND `user`.`name` LIKE ? AND `user`.`deleted_at` IS NULL
// args: []any{18, "vip", "j%"}

// 动态组装：Clone 复用基准构建器。
base := dsl.Select(User.ID).From(User)
q1 := base.Clone().Where(User.Age.Gt(18))
q2 := base.Clone().Where(User.Status.Eq("vip")) // 互不干扰
```

### JOIN

```go
u := User.As("u")
ur := UserRole.As("ur")
r := Role.As("r")

dsl.Select(u.ID, r.Name).From(u).
    InnerJoin(ur).On(ur.UserID.EqExpr(u.ID)).
    InnerJoin(r).On(r.ID.EqExpr(ur.RoleID)).
    Where(r.Name.Eq("admin")).
    ToSql(gooq.DialectMySQL)
// SELECT `u`.`id`, `r`.`name` FROM `user` AS u INNER JOIN `user_role` AS ur ON `ur`.`user_id` = `u`.`id` INNER JOIN `role` AS r ON `r`.`id` = `ur`.`role_id` WHERE `r`.`name` = ? AND `u`.`deleted_at` IS NULL

// 自连接：同一表的两个别名实例。
u1 := User.As("u1")
u2 := User.As("u2")
dsl.Select(u1.ID, u2.ID).From(u1).InnerJoin(u2).On(u1.ID.EqExpr(u2.ID))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS u1 INNER JOIN `user` AS u2 ON `u1`.`id` = `u2`.`id`

// LATERAL 派生表（InnerLateral 在 SQLite 下映射为 CROSS JOIN LATERAL）。
lt := dsl.Select(fn.Count(UserRole.UserID).As("cnt")).
    From(UserRole).Where(UserRole.UserID.EqExpr(u.ID)).As("lt")
dsl.Select(u.ID, lt.Field("cnt")).From(u).LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).ToSql(gooq.DialectPgsql)
// ... LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS cnt FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS lt ON 1 = 1 ...
```

### 子查询

```go
sub := dsl.Select(Role.ID).From(Role).Where(Role.Name.Eq("admin"))

dsl.Select(User.ID).From(User).Where(User.ID.InExpr(sub))
// ... WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) ...

dsl.Select(User.ID).From(User).Where(gooq.Exists(sub))
// ... WHERE EXISTS (SELECT ...) ...
dsl.Select(User.ID).From(User).Where(gooq.NotExists(sub))

// 派生表。
dsl.Select(User.ID).From(
    dsl.Select(User.ID).From(User).Where(User.Age.Gt(18)).As("t"),
)
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS t

// 相关子查询：子查询内引用外层别名。
u := User.As("u")
ur := UserRole.As("ur")
dsl.Select(u.ID).From(u).Where(gooq.Exists(
    dsl.Select(ur.UserID).From(ur).Where(ur.UserID.EqExpr(u.ID)),
))
// ... WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS ur WHERE `ur`.`user_id` = `u`.`id`) ...
```

### 分组聚合

```go
dsl.Select(User.Status, fn.Count(User.ID)).
    From(User).
    Group(User.Status).
    Having(gooq.Gt(fn.Count(User.ID), 2)).
    ToSql(gooq.DialectMySQL)
// SELECT `user`.`status`, COUNT(`user`.`id`) FROM `user` WHERE `user`.`deleted_at` IS NULL GROUP BY `user`.`status` HAVING COUNT(`user`.`id`) > ?
// args: []any{2}

// 分组扩展（方言感知：Rollup 支持 MySQL/PG，Cube/GroupingSets 仅 PG，MySQL 渲染报错）。
dsl.Select(User.Status).From(User).GroupRollup(User.Status).ToSql(gooq.DialectMySQL)
// ... GROUP BY `user`.`status` WITH ROLLUP
dsl.Select(User.Status).From(User).GroupCube(User.Status).ToSql(gooq.DialectPgsql)
// ... GROUP BY CUBE("user"."status")
```

### 集合操作与 CTE

```go
dsl.Select(User.ID).From(User).Where(User.Age.Eq(1)).
    UnionAll(dsl.Select(User.ID).From(User).Where(User.Age.Eq(2)))
// ... UNION ALL ...

dsl.Select(User.ID).From(User).
    Intersect(dsl.Select(User.ID).From(User)).
    Except(dsl.Select(User.ID).From(User))
// ... INTERSECT ... EXCEPT ...

// CTE / 递归 CTE。
dsl.With("adults", dsl.Select(User.ID).From(User).Where(User.Age.Gt(18))).
    Fields(dsl.Cte("adults").Field("id")).From(dsl.Cte("adults")).ToSql(gooq.DialectPgsql)
// WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"

dsl.WithRecursive("t", dsl.Select(User.ID).From(User).Where(User.ID.Eq(1)))
// WITH RECURSIVE t AS (...)
```

### 表达式与函数

```go
// 字符串 / 数学 / 日期 / 聚合 / 通用。
fn.Concat(User.Name, gooq.Str(","), User.Status)   // CONCAT(`user`.`name`, ',', `user`.`status`)
fn.Coalesce(User.Name, gooq.Str("unknown"))        // COALESCE(`user`.`name`, 'unknown')
fn.Substring(User.Name, 1, 3)                      // SUBSTRING(`user`.`name`, ?, ?)
fn.DateDiff(User.CreatedAt, fn.Now())        // DATEDIFF(`user`.`created_at`, NOW())
fn.CountDistinct(User.Status)                      // COUNT(DISTINCT `user`.`status`)

// 日期格式化跨方言：同一种写法，三个库各自渲染。
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectMySQL)
// SELECT DATE_FORMAT(`user`.`created_at`, '%Y-%m-%d') FROM ...
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectPgsql)
// SELECT TO_CHAR("user"."created_at", 'YYYY-MM-DD') FROM ...
dsl.Select(fn.DateFormat(User.CreatedAt, "%Y-%m-%d")).From(User).ToSql(gooq.DialectSQLite)
// SELECT strftime('%Y-%m-%d', "user"."created_at") FROM ...

// 字符串聚合（MySQL GROUP_CONCAT / PG STRING_AGG / SQLite GROUP_CONCAT）。
fn.GroupConcat(fn.GroupConcatOptions{
    Field: User.Name, Separator: "-", OrderBy: []gooq.OrderClause{User.Name.Asc()},
}).ToSql(...)  // GROUP_CONCAT(`user`.`name` ORDER BY `user`.`name` ASC SEPARATOR '-')

// 窗口函数。
fn.Rank().Over([]gooq.Expression{User.Status}, []gooq.OrderClause{User.Age.Desc()}).As("r")
// RANK() OVER (PARTITION BY `user`.`status` ORDER BY `user`.`age` DESC) AS r
fn.RowNumber().Over(nil, []gooq.OrderClause{User.ID.Asc()})
// ROW_NUMBER() OVER (ORDER BY `user`.`id` ASC)
fn.Sum(User.Age).OverFrame(
    []gooq.Expression{User.Status},
    []gooq.OrderClause{User.ID.Asc()},
    fn.RowsFrame("UNBOUNDED PRECEDING", "CURRENT ROW"),
)
// SUM(`user`.`age`) OVER (PARTITION BY `user`.`status` ORDER BY `user`.`id` ASC ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW)

// CASE 条件表达式。
gooq.Case().When(User.Age.Gt(60)).Then(gooq.Str("old")).
    When(User.Age.Gt(18)).Then(gooq.Str("adult")).
    Else(gooq.Str("young")).End().As("bucket")
// CASE WHEN `user`.`age` > ? THEN 'old' WHEN `user`.`age` > ? THEN 'adult' ELSE 'young' END AS bucket

// 算术与类型转换。
User.Age.Mul(2)                                 // (`user`.`age` * ?)
gooq.Add(User.Age, gooq.Mul(2, User.Age))       // (`user`.`age` + (? * `user`.`age`))
gooq.Negate(User.Age)                           // (-`user`.`age`)
User.Age.Cast(gooq.LocalTypeString)             // CAST(`user`.`age` AS CHAR)（PG: BIGINT / SQLite: TEXT）

// Raw：结构化 SQL，参数绑定。
gooq.Raw("JSON_EXTRACT(data, ?)", "$.name")
dsl.Select(User.ID).From(User).Where(gooq.Raw("age > ?", 18))

// 通用函数按 NAME(args...) 原样渲染（无方言感知）。
dsl.Select(fn.New("JSON_EXTRACT", User.Name, gooq.Str("$.key"))).From(User)
// SELECT JSON_EXTRACT(`user`.`name`, '$.key') FROM ...

// 任意 FuncExpr 可挂方言渲染策略（实例级，无全局注册表）：
concatPipe := fn.New("CONCAT", User.Name, gooq.Str("-x")).
    RenderWith(func(rc *gooq.RenderContext, argsSQL []string) (string, []any) {
        if rc.Dialect() == gooq.DialectPgsql {
            return "(" + argsSQL[0] + " || " + argsSQL[1] + ")", nil
        }
        return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")", nil
    })
// 完全自定义的表达式节点则实现 gooq.Expression + gooq.Renderer。
```

### 行锁

```go
dsl.Select(User.ID).From(User).LockForUpdate().ToSql(gooq.DialectMySQL)   // ... FOR UPDATE
dsl.Select(User.ID).From(User).LockInShareMode().ToSql(gooq.DialectMySQL) // ... LOCK IN SHARE MODE
dsl.Select(User.ID).From(User).LockInShareMode().ToSql(gooq.DialectPgsql) // ... FOR SHARE
dsl.Select(User.ID).From(User).LockForUpdate().ToSql(gooq.DialectSQLite)  // SQLite 忽略
```

## 写操作

### 插入

```go
// 实体记录：零值字段与自增列自动跳过。
dsl.Insert(User).Record(model.User{Name: "john", Age: 18})
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?)

// 列值对应 + 多次 Values 批量。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).Values("b", 2)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)

// Record 与 Columns/Values 可混用。
dsl.Insert(User).Record(model.User{Name: "a", Age: 1}).Columns(User.Name, User.Age).Values("b", 2)

// 批量执行：分批提交，RowsAffected 聚合。
dsl.Insert(User).Records([]model.User{{Name: "a"}, {Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)

// INSERT ... SELECT。
dsl.InsertFrom(User, dsl.Select(User.Name).From(User).Where(User.Age.Gt(18)))
// INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL
```

### 更新

```go
// 链式 Set。
dsl.Update(User).Set(User.Name, "x").Set(User.Age, 20).Where(User.ID.Eq(1))
// UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?

// map 全量更新。
dsl.Update(User).Data(map[string]any{"age": 1, "name": "x"})
// UPDATE `user` SET `age` = ?, `name` = ?

// 表达式值渲染为 SQL 片段（字段算术、Raw 等）。
dsl.Update(User).Set(User.Age, User.Age.Add(1)).Where(User.ID.Eq(1))
// UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE `user`.`id` = ?
// （单条 Record 更新不支持，使用 Set/Data + Where）

// 按主键批量（或 Keys(...) 自定义键），RowsAffected 聚合。
dsl.Update(User).Records([]model.User{{Id: 1, Name: "a"}, {Id: 2, Name: "b"}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// 多表更新（MySQL JOIN / PG+SQLite FROM）。
u := User.As("u"); r := Role.As("r")
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.ID.EqExpr(u.ID)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS u INNER JOIN `role` AS r ON `r`.`id` = `u`.`id` SET `status` = ?
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.ID.EqExpr(u.ID)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS u SET "status" = $1 FROM "role" AS r WHERE "r"."id" = "u"."id"
```

### 删除

```go
// 软删表：DELETE 自动转 UPDATE deleted_at。
dsl.Delete(User).Where(User.ID.Eq(1))
// UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?

// Unscoped()：真删除。
dsl.Delete(User).Unscoped().Where(User.ID.Eq(1))
// DELETE FROM `user` WHERE `user`.`id` = ?

// 非软删表直接 DELETE。
dsl.Delete(UserRole).Where(UserRole.ID.Eq(1))
// DELETE FROM `user_role` WHERE `user_role`.`id` = ?

// 按主键批量删除（软删同样生效）。
dsl.Delete(User).Records([]model.User{{Id: 1}, {Id: 2}}).Batch(100).UseDB(gdb.DB()).Exec(ctx)
```

### Upsert 与 Returning

```go
// MySQL：ON DUPLICATE KEY UPDATE。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.ID).DoUpdate(User.Name, "x").ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)

// MySQL：INSERT IGNORE。
dsl.Insert(User).Columns(User.Name).Values("a").DoNothing().ToSql(gooq.DialectMySQL)
// INSERT IGNORE INTO `user` (`name`) VALUES (?)

// PG：ON CONFLICT。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.ID).DoUpdate(User.Name, "x").ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3
dsl.Insert(User).Columns(User.Name).Values("a").
    OnConflictKey(User.ID).DoNothing().ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING

// Returning：PG / SQLite（MySQL 渲染报错）。
dsl.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).
    Returning(User.ID).ToSql(gooq.DialectPgsql)
// UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"
```

## 执行（gooq + gdb 融合）

```go
// 查询 + 扫描到 struct 切片 / 标量。
users := []model.User{}
err := dsl.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).Where(User.Age.Gt(18)).Order(User.ID.Desc()).Limit(10).
    Scan(ctx, &users)

count := int64(0)
err = dsl.Select(fn.Count(User.ID)).From(User).UseDB(gdb.DB()).Scan(ctx, &count)

// 写操作。
_, err = dsl.Insert(User).Record(model.User{Name: "john"}).UseDB(gdb.DB()).Exec(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).UseDB(gdb.DB()).Exec(ctx)

// 事务：UseTX 绑定事务连接。
tx, _ := gdb.DB().Begin(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).UseTX(tx).Exec(ctx)
tx.Commit()

// 便捷方法：Count 自动补全 COUNT(*)，Exists 包装 SELECT EXISTS(...)。
total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).UseDB(gdb.DB()).Count(ctx)
exists, err := dsl.Select(User.ID).From(User).Where(User.Account.Eq("x")).UseDB(gdb.DB()).Exists(ctx)

// 行级类型化读取：类型 T 在编译期被消费。
row, err := dsl.Select(User.ID, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).UseDB(gdb.DB()).Row(ctx)
id := gooq.Get(row, User.ID)      // int64
name := gooq.Get(row, User.Name)  // string

// 方言从 gdb 驱动名自动推导；UseDB 可重新绑定以支持多库/读写分离。
```

### 缓存

```go
// 全局注入缓存适配器（如 gredis 实现）。
dsl.SetCacheAdapter(redisAdapter)
dsl.SetHashCacheAdapter(redisHashAdapter)

// 单查询缓存：所有查询方法（Scan/Row/Rows/Count/Exists）均走缓存。
// Cache() 无参或零值 option 不开启；自定义 Name 固定 key（便于手动失效）。
err := dsl.Select(User.AllFields()).From(User).
    Cache(dsl.CacheOption{Duration: time.Minute}).
    UseDB(gdb.DB()).Scan(ctx, &users)

// 复合查询：count 先行、rows 后查，count=0 短路不查 rows。
rows, total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// PageCache 缓存复合查询为一条 hash 记录（field 为 "count" 与 "rows"）。
// 需要 SetHashCacheAdapter；key 含 limit/offset，各页独立缓存。
// RowsField/CountField 可覆盖 field 名；Force 开启后 count=0 也会缓存（默认跳过空结果）。
rows, total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).RowsAndCount(ctx)

// ScanAndCount 是类型化变体：数据扫入 dest 并返回总数。
// 与 RowsAndCount 共享同一份数据缓存（统一存 Result JSON，数据 field 默认 "rows"）。
var vips []User
total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.ID.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).UseDB(gdb.DB()).ScanAndCount(ctx, &vips)
```

## 方言

| 方言 | 占位符 | 引号 | 分页 |
| --- | --- | --- | --- |
| mysql | `?` | `` ` `` | LIMIT |
| pgsql | `$n` | `"` | LIMIT |
| sqlite | `?` | `"` | LIMIT |

- 未注册方言回退默认渲染；驱动可 `RegisterDialect` 增量覆盖内置方言。
- 带 schema 的表渲染 `schema.table.column` 三级限定（别名遮蔽 schema）；gooq-gen 仅对 PG 填充 schema（`current_schema()`）。
- 方言敏感行为均内置处理：分页（LIMIT/OFFSET）、`NullsFirst/NullsLast`（仅 PG）、行锁（`FOR UPDATE`/`LOCK IN SHARE MODE`/`FOR SHARE`）、LATERAL 映射（SQLite `INNER JOIN LATERAL` → `CROSS JOIN LATERAL`）、Upsert 语法、`DATE_FORMAT`/`TO_CHAR`/`strftime`、`GROUP_CONCAT`/`STRING_AGG`。

## gooq-gen（代码生成工具）

```bash
cd cmd/gooq-gen && go run . init                                    # 初始化 hack/gooq.yaml + hack/template/table.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

- `init` 生成 `hack/gooq.yaml` 与 `hack/template/table.tmpl` 供定制；已存在的文件不覆盖。
- 配置文件：读取 `hack/gooq.yaml` 的 `gooq.gen.table` 节点；命令行参数优先。
- 模板：`<hack>/template/table.tmpl` 优先于内置模板（删除即回退内置）。
- `-l/--link` 数据库连接；`-p/--path` 输出目录（默认 `internal`）。
- 仅生成 gooq 类型化表对象（`table/`）；`do/`/`entity/` 交由 `gf gen dao`。
- 元数据推导：主键（`PRI`）、自增（`auto_increment`）、软删（列名约定）、唯一（`UNI`）、`LocalType` 标记；Go 命名规范（`id` → `ID`）。
- 内置驱动：mysql/pgsql/sqlite；其他驱动取消 `internal/cmd/cmd.go` 中 import 注释启用。

## 测试

```bash
go test ./ -count=1                     # 主库（渲染断言）
cd cmd/gooq-gen && go test ./...        # gooq-gen 端到端（sqlite）
cd test && go test ./...                # 集成测试（真实 MySQL，使用 test/generate 产物）
```

## License

MIT
