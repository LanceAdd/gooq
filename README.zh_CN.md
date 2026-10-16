[English](README.md) | 简体中文

# gooq

`gooq` 是面向 [GoFrame](https://goframe.org) 的类型安全 SQL 查询 DSL，灵感来自 [jOOQ](https://www.jooq.org/)。

- **类型安全** — `Field[T]` 在编译期携带列类型，`User.Age.Gt(18)` 编译通过，`User.Age.Gt("x")` 直接报错。
- **离线渲染** — `ToSql(dialect)` 在任何环境产出 SQL + 参数，不连库；执行可用标准库或 `gooq/db`。
- **方言感知** — MySQL / MariaDB / TiDB / OceanBase / PostgreSQL / GaussDB / 达梦 / SQLite 各自实现 `DialectSpec`。新方言整体注册。连接驱动通过 `db.Register` 单独注册。
- **运行时零猜测** — 软删除自动条件、全列派生来自生成期静态化的 `TableMeta`。INSERT 和 UPDATE 只写被设置过的列。
- **分层子包** — `gooq`（核心：字段/表/表达式语言/方言与渲染契约）、`gooq/db`（连接、事务、扫描）、`gooq/dsl`（查询与 DML 构建器）、`gooq/fn`（函数库）；生成代码只依赖 `gooq`。

下文所有示例的渲染输出均来自测试断言，可直接运行验证。

## 行为变化

多个 `Where` 用 AND 连接。字段比较返回 `Condition`，可以继续 `.And`、`.Or`、`.Not`，链是左结合的。一组表达式用 `gooq.AND`、`gooq.OR`、`gooq.NOT`。`WhereOr` 已去掉。

## 快速开始

### 1. 生成表对象

```bash
cd cmd/gooq-gen && go run . init                                    # 可选：初始化 hack/gooq.yaml + hack/template/{table,model,record}.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

一次生成所有表的 gooq 类型化表对象（`table/`）和 `model/`。每张表生成 `user_model.go`（`UserModel`）和 `user_record.go`（`UserRecord`）。`gooq-gen` 也可不传命令行参数，改为从 `hack/gooq.yaml` 的 `gooq.gen.table` 节点读取配置。

### 2. 手写表对象（或直接用生成产物）

```go
type UserTable struct {
    *gooq.TableBase
    Id        gooq.Field[int64]
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
    t.Id = gooq.NewFieldAt[int64](t.TableBase, "id")
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

sql, args, err := dsl.Select(User.Id, User.Name).
    From(User).
    Where(User.Age.Gt(18)).
    Order(User.Id.Desc()).
    Limit(10).
    ToSql(gooq.DialectMySQL)
// sql:  SELECT `user`.`id`, `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL ORDER BY `user`.`id` DESC LIMIT 10
// args: []any{18}
```

`deleted_at IS NULL` 自动出现——软删条件来自 `TableMeta`，无需手动过滤。`ToSql()` 不传方言时默认 MySQL 渲染。

### 4. 执行

```go
import (
    "github.com/lanceadd/gooq/db"
    _ "github.com/lanceadd/gooq/drivers/mysql" // pgsql、sqlite 分别是 drivers/pgsql、drivers/sqlite
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

## 查询

### 基础

```go
// 全列 / 差集 / 别名 / 去重
dsl.Select(User.AllFields()).From(User)                                     // SELECT `user`.`id`, `user`.`name`, ...
dsl.Select(User.Id).From(User).FieldsEx(User.CreatedAt, User.DeletedAt)     // 差集：其余列
dsl.Select(User.Name.As("nickname")).From(User)                             // SELECT `user`.`name` AS `nickname`
dsl.Select(User.Id).From(User).Distinct()                                   // SELECT DISTINCT ...

// 分页
dsl.Select(User.Id).From(User).Limit(10)              // ... LIMIT 10
dsl.Select(User.Id).From(User).Offset(20).Limit(10)   // ... LIMIT 10 OFFSET 20
dsl.Select(User.Id).From(User).Offset(20)             // MySQL: LIMIT 18446744073709551615 OFFSET 20；PG/SQLite: OFFSET 20
dsl.Select(User.Id).From(User).Page(2, 10)            // ... LIMIT 10 OFFSET 10（页码从 1 起）

// 排序：方向属于表达式本身（Field.Asc()/Desc()、gooq.OrderAscExpr/DescExpr）
dsl.Select(User.Id).From(User).Order(User.Age.Desc(), User.Id.Asc()).ToSql(gooq.DialectPgsql)
// SELECT "user"."id" FROM "user" WHERE "user"."deleted_at" IS NULL ORDER BY "user"."age" DESC, "user"."id" ASC
dsl.Select(User.Id).From(User).OrderDesc(fn.Count(User.Id))       // 糖：逐条包装 DESC
// 表达式/片段排序 —— Order 原样渲染（NULLS 等语法用 Raw 表达）
dsl.Select(User.Id).From(User).OrderDesc(gooq.Raw("ABS(`user`.`age`)"))
// ... ORDER BY ABS(`user`.`age`) DESC
dsl.Select(User.Id).From(User).Order(gooq.Raw("`user`.`age` DESC NULLS LAST"))
// ... ORDER BY `user`.`age` DESC NULLS LAST
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
OrderItem.UserId.EqExpr(User.Id)      // `order_item`.`user_id` = `user`.`id`
User.Id.InExpr(subquery)              // `user`.`id` IN (SELECT ...)

// 比较结果是 Condition，可继续 And / Or / Not。链左结合。
User.Age.Gt(11).Or(User.Age.Lt(-1))
// (`user`.`age` > ? OR `user`.`age` < ?)
User.Age.Gt(11).Or(User.Age.Lt(-1).And(User.Status.Eq("vip")))
// (`user`.`age` > ? OR (`user`.`age` < ? AND `user`.`status` = ?))
User.Status.Eq("banned").Not()
// (NOT `user`.`status` = ?)

// 多个 Where 用 AND 连接。字段之间的 OR / AND 用链式写法。
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

// 左边不是 Field[T] 时用 gooq.Gt / Eq 等。Raw 先用 Cond 包成条件。
gooq.Gt(User.Age.Add(1), 11).Or(gooq.Lt(User.Age, 0))
// ((`user`.`age` + ?) > ? OR `user`.`age` < ?)
gooq.Cond(gooq.Raw("a = 1")).Or(User.Age.Lt(-1))
// (a = 1 OR `user`.`age` < ?)

// 动态组装：Clone 复用基准构建器。同一个 builder 不能并发使用，执行前会把执行器写回它。
base := dsl.Select(User.Id).From(User)
q1 := base.Clone().Where(User.Age.Gt(18))
q2 := base.Clone().Where(User.Status.Eq("vip")) // 互不干扰

// Fields 的字符串参数按 Raw 拼进 SQL，不做标识符转义。列对象走字段渲染。
```

### JOIN

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

// 自连接：同一表的两个别名实例。
u1 := User.As("u1")
u2 := User.As("u2")
dsl.Select(u1.Id, u2.Id).From(u1).InnerJoin(u2).On(u1.Id.EqExpr(u2.Id))
// SELECT `u1`.`id`, `u2`.`id` FROM `user` AS `u1` INNER JOIN `user` AS `u2` ON `u1`.`id` = `u2`.`id` WHERE `u1`.`deleted_at` IS NULL AND `u2`.`deleted_at` IS NULL

// LATERAL 派生表。SQLite 3.46 不支持 LATERAL，渲染期返回错误。
lt := dsl.Select(fn.Count(UserRole.UserId).As("cnt")).
    From(UserRole).Where(UserRole.UserId.EqExpr(u.Id)).As("lt")
dsl.Select(u.Id, lt.Field("cnt")).From(u).LeftJoinLateral(lt).On(gooq.Raw("1 = 1")).ToSql(gooq.DialectPgsql)
// ... LEFT JOIN LATERAL (SELECT COUNT("user_role"."user_id") AS "cnt" FROM "user_role" WHERE "user_role"."user_id" = "u"."id") AS "lt" ON 1 = 1 ...
```

### 子查询

```go
sub := dsl.Select(Role.Id).From(Role).Where(Role.Name.Eq("admin"))

dsl.Select(User.Id).From(User).Where(User.Id.InExpr(sub))
// ... WHERE `user`.`id` IN (SELECT `role`.`id` FROM `role` WHERE `role`.`name` = ? AND `role`.`deleted_at` IS NULL) ...

dsl.Select(User.Id).From(User).Where(gooq.Exists(sub))
// ... WHERE EXISTS (SELECT ...) ...
dsl.Select(User.Id).From(User).Where(gooq.NotExists(sub))

// 派生表。
dsl.Select(User.Id).From(
    dsl.Select(User.Id).From(User).Where(User.Age.Gt(18)).As("t"),
)
// SELECT `user`.`id` FROM (SELECT `user`.`id` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL) AS `t`

// 相关子查询：子查询内引用外层别名。
u := User.As("u")
ur := UserRole.As("ur")
dsl.Select(u.Id).From(u).Where(gooq.Exists(
    dsl.Select(ur.UserId).From(ur).Where(ur.UserId.EqExpr(u.Id)),
))
// ... WHERE EXISTS (SELECT `ur`.`user_id` FROM `user_role` AS `ur` WHERE `ur`.`user_id` = `u`.`id`) ...
```

### 分组聚合

```go
dsl.Select(User.Status, fn.Count(User.Id)).
    From(User).
    Group(User.Status).
    Having(gooq.Gt(fn.Count(User.Id), 2)).
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
dsl.Select(User.Id).From(User).Where(User.Age.Eq(1)).
    UnionAll(dsl.Select(User.Id).From(User).Where(User.Age.Eq(2)))
// ... UNION ALL ...
// 集合操作之后的 ORDER BY 只能写结果列名。带表名的列在 MySQL、TiDB、OceanBase 上报 1250，PostgreSQL 家族同样拒绝。
// dsl.Select(User.Id).From(User).UnionAll(...).Order(gooq.Raw("id"))

dsl.Select(User.Id).From(User).
    Intersect(dsl.Select(User.Id).From(User)).
    Except(dsl.Select(User.Id).From(User))
// ... INTERSECT ... EXCEPT ...

// CTE / 递归 CTE。
dsl.With("adults", dsl.Select(User.Id).From(User).Where(User.Age.Gt(18))).
    Fields(dsl.Cte("adults").Field("id")).From(dsl.Cte("adults")).ToSql(gooq.DialectPgsql)
// WITH adults AS (SELECT "user"."id" FROM "user" WHERE "user"."age" > $1 AND "user"."deleted_at" IS NULL) SELECT "adults"."id" FROM "adults"

dsl.WithRecursive("t", dsl.Select(User.Id).From(User).Where(User.Id.Eq(1)))
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
    Field: User.Name, Separator: "-", OrderBy: []gooq.Expression{User.Name.Asc()},
}).ToSql(...)  // GROUP_CONCAT(`user`.`name` ORDER BY `user`.`name` ASC SEPARATOR '-')

// 窗口函数。
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

// CASE 条件表达式。
gooq.Case().When(User.Age.Gt(60)).Then(gooq.Str("old")).
    When(User.Age.Gt(18)).Then(gooq.Str("adult")).
    Else(gooq.Str("young")).End().As("bucket")
// CASE WHEN `user`.`age` > ? THEN 'old' WHEN `user`.`age` > ? THEN 'adult' ELSE 'young' END AS `bucket`

// 算术与类型转换。
User.Age.Mul(2)                                 // (`user`.`age` * ?)
gooq.Add(User.Age, gooq.Mul(2, User.Age))       // (`user`.`age` + (? * `user`.`age`))
gooq.Negate(User.Age)                           // (-`user`.`age`)
User.Age.Cast(gooq.LocalTypeString)             // CAST(`user`.`age` AS CHAR)（PG: TEXT / SQLite: TEXT）

// Raw：结构化 SQL，参数绑定。占位符不会被改写。
// MySQL / SQLite 写 ?。PostgreSQL 保持原文，需要自己写 $n。
gooq.Raw("JSON_EXTRACT(data, ?)", "$.name")
dsl.Select(User.Id).From(User).Where(gooq.Raw("age > ?", 18))

// 通用函数按 NAME(args...) 原样渲染（无方言感知）。
dsl.Select(fn.New("JSON_EXTRACT", User.Name, gooq.Str("$.key"))).From(User)
// SELECT JSON_EXTRACT(`user`.`name`, '$.key') FROM ...

// 任意 FuncExpr 可挂方言渲染策略（实例级，无全局注册表）：
concatPipe := fn.New("CONCAT", User.Name, gooq.Str("-x")).
    RenderWith(func(rc *gooq.RenderContext, argsSQL []string) string {
        if rc.Dialect() == gooq.DialectPgsql {
            return "(" + argsSQL[0] + " || " + argsSQL[1] + ")"
        }
        return "CONCAT(" + argsSQL[0] + ", " + argsSQL[1] + ")"
    })
// 自定义表达式节点实现 gooq.Expression。Render 只返回 SQL，参数经 rc.AddArg 累积。
```

### 行锁

```go
dsl.Select(User.Id).From(User).LockForUpdate().ToSql(gooq.DialectMySQL)   // ... FOR UPDATE
dsl.Select(User.Id).From(User).LockInShareMode().ToSql(gooq.DialectMySQL) // ... LOCK IN SHARE MODE
dsl.Select(User.Id).From(User).LockInShareMode().ToSql(gooq.DialectPgsql) // ... FOR SHARE
dsl.Select(User.Id).From(User).LockForUpdate().ToSql(gooq.DialectSQLite)  // SQLite 忽略
```

## 写操作

### 插入

```go
// 只写入调用过的 setter。SetAge(0) 会写入 0，SetDeletedAtNull 写入 NULL。
dsl.Insert(User).ValuesOf(model.NewUserRecord().SetName("john").SetAge(0))
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?)

// 列值对应 + 多次 Values 批量。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).Values("b", 2)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?), (?, ?)

// 多行写入记录。Batch 分批执行并聚合 RowsAffected。各片独立提交，需要原子性时由调用方包在 db.Transaction 里。
dsl.Insert(User).ValuesOf(
    model.NewUserRecord().SetName("a"),
    model.NewUserRecord().SetName("b"),
).Batch(100).Exec(ctx)

// 各行设置的列不同时，Exec 按列集合拆成多条 INSERT。
// 没写的列留给表默认值。这种形状 ToSql 会报错，改走 Exec。
// Batch 在每个列集合内部再切分，不会用 NULL 填没写的列。

// INSERT ... SELECT。
dsl.InsertFrom(User, dsl.Select(User.Name).From(User).Where(User.Age.Gt(18)))
// INSERT INTO `user` (`name`) SELECT `user`.`name` FROM `user` WHERE `user`.`age` > ? AND `user`.`deleted_at` IS NULL
```

### 更新

```go
// 链式 Set。
dsl.Update(User).Set(User.Name, "x").Set(User.Age, 20).Where(User.Id.Eq(1))
// UPDATE `user` SET `name` = ?, `age` = ? WHERE `user`.`id` = ?

// map 全量更新。
dsl.Update(User).Data(map[string]any{"age": 1, "name": "x"})
// UPDATE `user` SET `age` = ?, `name` = ?

// 表达式值渲染为 SQL 片段（字段算术、Raw 等）。
dsl.Update(User).Set(User.Age, User.Age.Add(1)).Where(User.Id.Eq(1))
// UPDATE `user` SET `age` = (`user`.`age` + ?) WHERE `user`.`id` = ?

// 一行记录变成 SET。多行按主键分别更新，RowsAffected 聚合。
dsl.Update(User).ValuesOf(model.NewUserRecord().SetName("x").SetAge(0)).Where(User.Id.Eq(1))
dsl.Update(User).ValuesOf(
    model.NewUserRecord().SetId(1).SetName("a"),
    model.NewUserRecord().SetId(2).SetName("b"),
).Exec(ctx)
// UPDATE `user` SET `name` = ? WHERE `id` = ?  ×2

// 多表更新（MySQL JOIN / PG+SQLite FROM）。
u := User.As("u"); r := Role.As("r")
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectMySQL)
// UPDATE `user` AS `u` INNER JOIN `role` AS `r` ON `r`.`id` = `u`.`id` SET `status` = ?
dsl.Update(u).Set(u.Status, "vip").InnerJoin(r).On(r.Id.EqExpr(u.Id)).ToSql(gooq.DialectPgsql)
// UPDATE "user" AS "u" SET "status" = $1 FROM "role" AS "r" WHERE "r"."id" = "u"."id"
// 连接里只要有 LEFT / RIGHT / FULL，PostgreSQL、GaussDB、SQLite、达梦都在渲染期返回错误。
// MySQL 保留外连接，写在 SET 前面。
```

### 删除

```go
// 软删表：DELETE 自动转 UPDATE deleted_at。
dsl.Delete(User).Where(User.Id.Eq(1))
// UPDATE `user` SET `deleted_at` = ? WHERE `user`.`id` = ?

// Unscoped()：真删除。
dsl.Delete(User).Unscoped().Where(User.Id.Eq(1))
// DELETE FROM `user` WHERE `user`.`id` = ?

// 非软删表直接 DELETE。
dsl.Delete(UserRole).Where(UserRole.Id.Eq(1))
// DELETE FROM `user_role` WHERE `user_role`.`id` = ?

// 按主键批量删除（软删同样生效）。
dsl.Delete(User).ValuesOf(
    model.NewUserRecord().SetId(1),
    model.NewUserRecord().SetId(2),
).Exec(ctx)
```

### Upsert 与 Returning

```go
// MySQL：DoUpdate 写入传入的值。不需要 OnConflictKey。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.Id).DoUpdate(User.Name, "x").ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = ?

// 写回本次插入的值：MySQL 为 VALUES(col)，PostgreSQL / SQLite 为 EXCLUDED.col。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    DoUpdate(User.Name, gooq.Excluded(User.Name)).ToSql(gooq.DialectMySQL)
// INSERT INTO `user` (`name`, `age`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `name` = VALUES(`name`)

// MySQL：INSERT IGNORE。
dsl.Insert(User).Columns(User.Name).Values("a").DoNothing().ToSql(gooq.DialectMySQL)
// INSERT IGNORE INTO `user` (`name`) VALUES (?)

// PG：ON CONFLICT。
dsl.Insert(User).Columns(User.Name, User.Age).Values("a", 1).
    OnConflictKey(User.Id).DoUpdate(User.Name, "x").ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name", "age") VALUES ($1, $2) ON CONFLICT ("id") DO UPDATE SET "name" = $3
dsl.Insert(User).Columns(User.Name).Values("a").
    OnConflictKey(User.Id).DoNothing().ToSql(gooq.DialectPgsql)
// INSERT INTO "user" ("name") VALUES ($1) ON CONFLICT ("id") DO NOTHING

// Returning：PG / SQLite（MySQL 渲染报错）。
dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).
    Returning(User.Id).ToSql(gooq.DialectPgsql)
// UPDATE "user" SET "status" = $1 WHERE "user"."id" = $2 RETURNING "user"."id"
```

## 执行

```go
// 查询填进结构体切片。不写 DB 时走默认组。
var users []model.UserModel
rows, err := dsl.Select(User.AllFields()).From(User).
    Where(User.Age.Gt(18)).Order(User.Id.Desc()).Limit(10).
    Rows(ctx)
err = rows.Structs(&users)

var count int64
err = dsl.Select(fn.Count(User.Id)).From(User).Value(ctx, &count)

// 单列。time.Time 和 sql.Null* 是单元格，用 Value 或 Column。
// 一行结构体用 Row 再 db.Record.Struct。
var created time.Time
err = dsl.Select(User.CreatedAt).From(User).Where(User.Id.Eq(1)).Value(ctx, &created)
var names []string
err = dsl.Select(User.Name).From(User).Column(ctx, &names)

// 写操作。
_, err = dsl.Insert(User).ValuesOf(model.NewUserRecord().SetName("john")).Exec(ctx)
_, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).Exec(ctx)

// Transaction 把事务放进 ctx，回调里的查询会自动加入。
database, err := db.Instance()
err = database.Transaction(ctx, func(ctx context.Context, tx *db.Tx) error {
    _, err = dsl.Update(User).Set(User.Status, "vip").Where(User.Id.Eq(1)).Exec(ctx)
    return err
})

// Count 统计去掉分页和排序后的结果行数。DISTINCT、分组或集合操作套子查询再 COUNT(*)。
// Exists 判断这批行是否至少有一行：选择列表换成 1，同样去掉分页和排序。
// 达梦是 SELECT COUNT(*) FROM (SELECT 1 ...) WHERE ROWNUM = 1，其他方言是 SELECT EXISTS (SELECT 1 ...)。
// Value 和 Column 要求恰好一列，RETURNING 也一样。Row 和 Value 在没有 Limit 时补 LIMIT 1；Limit 大于 1 时返回错误。
total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).Count(ctx)
exists, err := dsl.Select(User.Id).From(User).Where(User.Account.Eq("x")).Exists(ctx)

// 行级类型化读取：类型 T 在编译期被消费。
row, err := dsl.Select(User.Id, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).Row(ctx)
id := db.Get(row, User.Id)      // int64
name := db.Get(row, User.Name)  // string

// DB("orders") 使用该配置组。不写 DB 时走默认组。
// LoadConfig 通过 gcfg 读取 gooq.database、gooq.defaultGroup 和 gooq.logger。
// 运行时只读取名为 gooq 的配置文件，例如 gooq.yaml，不读取 config.yaml。
// database 是配置组列表：name 是组名，endpoints 是这一组的主从节点。
// 只有一组时可以不写 name，加载后记成 default。多于一组时每组都要写 name。
// defaultGroup 选择不写组名时使用的组，必须是本次文件里的组名；不写时是 default。
// SetDebug 打印 SQL，SetDryRun 跳过执行，Prepare 预处理语句，
// Schema 切换 MySQL 库名或 PostgreSQL search_path。
// 组内有从库时读走从库，写和 BEGIN 走主库。Master() / Slave() 强制指定。
// 事务从 ctx 读取。
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

### 缓存

```go
// 全局注入缓存适配器（如 gredis 实现）。
dsl.SetCacheAdapter(redisAdapter)
dsl.SetHashCacheAdapter(redisHashAdapter)
// 缓存值默认是 JSON。SetCacheCodec 换成别的实现，整个进程共用。
// Row 和 Rows 命中后的数字、时间类型由这个实现决定。JSON 下整数是 json.Number，时间是 RFC3339 字符串。
dsl.SetCacheCodec(codec)

// 单查询缓存：Value、Column、Row、Rows、Count、Exists 走缓存。
// Cache() 无参或零值 option 不开启；自定义 Name 固定 key（便于手动失效）。
// 参数 key 带类型名，int64(1) 与 "1" 不会共用一把 key。
// 自动生成的 key 含实际执行的配置组名。省略 DB() 时用当前默认组。自定义 Name 不加组名。
rows, err := dsl.Select(User.AllFields()).From(User).
    Cache(dsl.CacheOption{Duration: time.Minute}).
    Rows(ctx)

// 复合查询：count 先行、rows 后查，count=0 短路不查 rows。
rows, total, err := dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    Page(1, 10).RowsAndCount(ctx)

// PageCache 把带总数的分页缓存成一条 hash 记录（field 为 "count" 与 "rows"）。
// 用于 RowsAndCount、StructsAndCount、ColumnAndCount。
// 需要 SetHashCacheAdapter。key 由总数 SQL 和当前页 SQL 连同参数一起哈希，
// 选择列、条件、排序或页码不同都会得到不同的 key。
// RowsField/CountField 可覆盖 field 名；Force 开启后 count=0 也会缓存（默认跳过空结果）。
rows, total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).RowsAndCount(ctx)

// StructsAndCount 把当前页填进结构体切片并返回总数。
// 内部是 RowsAndCount 再 Result.Structs，并共用这份分页缓存。
var vips []User
total, err = dsl.SelectFrom(User).Where(User.Status.Eq("vip")).
    Order(User.Id.Desc()).
    PageCache(dsl.CacheOption{Duration: time.Minute}).
    Page(1, 10).StructsAndCount(ctx, &vips)
```

## 方言

| 方言 | 占位符 | 引号 | 分页 |
| --- | --- | --- | --- |
| mysql、mariadb、tidb、oceanbase | `?` | `` ` `` | LIMIT |
| pgsql、gaussdb | `$n` | `"` | LIMIT |
| sqlite | `?` | `"` | LIMIT |
| dm | `?` | `"`（大写） | LIMIT |

- `ToSql` 要求方言已注册为 `DialectSpec`。未知方言直接返回错误。可嵌入 `MySQLDialect`、`PostgresDialect` 或 `SQLiteDialect`，只覆盖有差异的方法，再调用 `RegisterDialect`。
- 打开连接需要空白导入对应驱动包（`drivers/mysql`、`drivers/mariadb`、`drivers/tidb`、`drivers/oceanbase`、`drivers/pgsql`、`drivers/gaussdb`、`drivers/dm`、`drivers/sqlite`）。驱动在 `init` 里调用 `db.Register`。`gooq-gen` 会导入这些驱动。
- MariaDB、TiDB 和 OceanBase 使用 MySQL 协议，语句形状与 MySQL 相同。MariaDB 从 `information_schema` 读列，因此 `longtext` 加上 `json_valid` 检查的 JSON 列会标成 `json`。`LockInShareMode` 在 TiDB 上默认报错；打开 `tidb_enable_noop_functions` 后语句能执行，但不会加共享锁。`LockForUpdate` 正常。
- GaussDB 指 openGauss。引号、占位符、行锁、`RETURNING`、删除和更新跟随 PostgreSQL。`DATE_ADD` 写成间隔乘法，不用 `make_interval`。冲突更新是 `ON DUPLICATE KEY UPDATE`，本次插入值写成 `VALUES(col)`。GaussDB 没有 `ON CONFLICT DO NOTHING`，`DoNothing` 返回错误。`database/sql` 的驱动名是 `opengauss`。
- 达梦（`dm`）把标识符渲染成大写双引号，对上未加引号建表时折成大写的名字。冲突插入写成 `MERGE`，必须给出 `OnConflictKey`，冲突列还要出现在插入列里。`DoNothing` 只在未匹配时插入。没有 `RETURNING`。`Exists` 写成 `SELECT COUNT(*) FROM (子查询) WHERE ROWNUM = 1`。`GROUP_CONCAT` 写成 `LISTAGG`，`DATE_FORMAT` 写成 `TO_CHAR`，`IFNULL` 写成 `NVL`，`DATE_ADD` 写成 `DATEADD`。`time.Time` 绑成时间字符串，零值写成 `NULL`。配置里的 IANA 时区换成分钟偏移，超出 ±12 小时会失败。带连接的 `DELETE` 和带外连接的 `UPDATE` 返回错误。自增列不会标成 `AutoIncrement`。`database/sql` 的驱动名是 `dm`。链接示例：`dm:SYSDBA:SYSDBA001@tcp(127.0.0.1:5236)/SYSDBA`。
- 带 schema 的表渲染 `schema.table.column` 三级限定（别名遮蔽 schema）；gooq-gen 仅对 PG 填充 schema（`current_schema()`）。
- `DialectSpec` 只约束语句形状：分页、行锁、`JOIN` 改写、upsert、删除、多表 `UPDATE`（`RenderUpdate`）、存在性查询（`RenderExists`）。`DATE_FORMAT`、`GROUP_CONCAT`、`IFNULL`、`CURDATE`、`DATE_ADD`、`DATEDIFF`、`ROUND`、`MEDIAN` 留在具体方言类型上。没有实现的方言在 `ToSql` 返回 `dialect %q does not support ...`。SQLite 的 upsert 使用 `ON CONFLICT`。
- 表别名、列别名和函数别名都走 `Quote`。`numeric` / `decimal` 生成 `string`。PostgreSQL 数组按元素类型生成切片。
- PostgreSQL 16 起可用 `STRING_AGG(DISTINCT ...)`、`JSON_ARRAYAGG` 和 `ANY_VALUE`。

## 分片

分组写在 `hack/gooq.tables.yaml`。`single` 是普通表，`groups` 是分表。没有这份文件时，每张物理表各生成一个对象。`gooq-gen tables init` 把库里的表都写进 `single`。`tables check` 打印重复、缺失和多余的表名。`tables diff` 打印库里新增和配置里已不用的表名。这两条命令都不改文件。

一组生成一个逻辑表、`RegisterXxxSharding`，以及包级的 `RegisterSharding`。同一种分片方式调用一次 `RegisterSharding`，它把这个规则赋给所有分片逻辑表。某张表算法不同时，再用 `RegisterXxxSharding` 盖掉。nil 会 panic，再次注册覆盖。规则挂在 `TableMeta.Sharding` 上。

```go
// 生成物 table/user.go —— 仅分片表
func RegisterUserSharding(rule gooq.ShardingRule) {
    if rule == nil {
        panic("gooq: sharding rule is required")
    }
    UserMeta.Sharding = rule
}

// 应用侧包（被空白导入，保证 init 先跑）
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

// ShardBy 只作用于主表（From / Insert / Update / Delete）
err := dsl.Select(table.User.Id, table.User.Name).From(table.User).
    ShardBy(uid).Value(ctx, &name)

// JOIN 的分片表自己带分片值；普通表不用
r := table.UserRole.ShardBy(uid)
err = dsl.Select(table.User.Name, r.Name).From(table.User).
    InnerJoin(r).On(r.UserId.EqExpr(table.User.Id)).
    ShardBy(uid).Value(ctx, &name)
```

- `TableName` 的第一个参数是 `ShardingTableMeta`。`LogicalName` 是逻辑表名，`ShardNames` 是生成时库里的物理分表名。返回值是一个未加引号的表名，例如 `user_03`。gooq 负责加引号。schema 留在 `TableMeta` 上，不要写进返回值。id 确定非负时可以直接 `uid % shards`。不要先取绝对值，否则 `-3` 和 `3` 会落在同一片。不要用 `len(ShardNames)` 当除数，列表只是当时存在的表。
- 查询上的 `ShardBy` 只作用于这条查询的主表。子查询、CTE、`UNION` 的每一支各自设置，不沿用外层的值。同一条查询里的其他分片表调用 `ShardBy`，返回新表对象；`As` 和 `Clone` 会带上这个值。还挂着规则、又没有 `ShardBy` 或 `Physical` 的表，在 `ToSql` 失败。
- `role.ShardBy(uid)` 不改原来的 `role`。`ON` 和选择列要用返回的新对象上的字段。`role.UserId` 仍绑在原表上，没有这次的分片值。
- 子查询里引用外层的分片表列时，外层那张表要用表级 `ShardBy`（`u := table.User.ShardBy(uid)`），子查询里也用新对象上的字段。外层只挂查询级 `ShardBy` 会在渲染时报缺值。
- 两张分片表可以各自带不同的分片值。库不检查它们是不是同一套规则，值对不上时 SQL 仍能执行。
- `Physical("user_05")` 清空规则并使用这个表名。空名字 panic。
- 生成时按组里第一张表比对列名、顺序、`LocalType`、可空、主键、自增和唯一。组里只有一张表时跳过。字符串长度、默认值、注释不比对。
- 分片查询可以用 `Cache` 和 `PageCache`。没写 `Name` 时，键在 SQL 渲染之后计算，包含物理表名、参数、结果形状和数据库组，不同片不会撞，适合多读、靠过期刷新的数据。写了 `Name` 时键就是这个字符串，SQL 和参数不再参与。包住查询的函数要用输入把名字区分开，例如 `user-name:19`，不必写成 `user_03`。固定的 `Name: "user-name"` 会让不同输入共用一条缓存。写入不会自动删缓存；用显式名字时，由应用删掉同一个名字。
- 约定由调用方自己遵守，库不拦截。库里不保留逻辑占位表；片键进 `WHERE`；不用自增主键（用业务键或嵌分片位的雪花 id）。分片场景不要做跨分片事务、跨库 join、自动归并或广播；用了这些，语句仍然能执行。跨分片读是应用层显式循环（逐片 `Physical`）。多行写入先按片分组，一次 `ShardBy` 只写一张物理表；不同片的行放进同一次写入，也会进这一张表，不会报错。`Batch` 只把这张表上的行分批提交。

## gooq-gen（代码生成工具）

```bash
go install github.com/lanceadd/gooq/cmd/gooq-gen@latest             # 安装已发布版本
# 或在仓库目录内运行：
cd cmd/gooq-gen && go run . init                                    # 初始化 hack/gooq.yaml + hack/template/{table,model,record}.tmpl
cd cmd/gooq-gen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db" -p internal
```

- `init` 生成 `hack/gooq.yaml`，以及 `hack/template/table.tmpl`、`model.tmpl`、`record.tmpl`；已存在的文件不覆盖。
- 配置文件：读取 `hack/gooq.yaml` 的 `gooq.gen.table` 节点；命令行参数优先。
- 模板：`<hack>/template/table.tmpl`、`model.tmpl`、`record.tmpl` 优先于内置模板（删除其中一个即回退内置）。
- `-l/--link` 数据库连接；`-p/--path` 输出目录（默认 `internal`）。
- 分片分组在 `hack/gooq.tables.yaml`，用 `gooq-gen tables init`、`tables check`、`tables diff` 维护。见上面的分片节。
- 生成类型化表对象（`table/`）和 `model/`。`model/user_model.go` 定义 `UserModel`，是带 `json`、`orm` 标签的普通结构体，给 `db.Result.Structs` 和 JSON 用。`model/user_record.go` 定义 `UserRecord`，字段是 `gooq.Property[T]`。`model.NewUserRecord().SetName("john")` 标记要写入的列，交给 `ValuesOf`。可空列另外生成 `SetXxxNull`。`ToModel()` 把已设置的值抄进 `UserModel`。`NewUserRecordFrom` 把 `UserModel` 的每一列做成全量快照。
- 元数据推导：主键（`PRI`）、自增（`auto_increment`）、软删（列名约定）、唯一（`UNI`）、`LocalType` 标记；字段命名（`id` → `Id`，与 `gf gen dao` 产物一致）；与表基方法同名的列加 `Field` 后缀（`meta` → `MetaField`）。
- 内置连接驱动在 `drivers/mysql`、`drivers/mariadb`、`drivers/tidb`、`drivers/oceanbase`、`drivers/pgsql`、`drivers/gaussdb`、`drivers/dm`、`drivers/sqlite`。`gooq-gen` 会导入它们；库代码按需空白导入。链接前缀分别是 `mysql`、`mariadb`、`tidb`、`oceanbase`、`pgsql`/`postgres`、`gaussdb`、`dm`、`sqlite`。

## 测试

```bash
go test ./ -count=1                     # 主库（渲染断言）
cd cmd/gooq-gen && go test ./...        # gooq-gen 端到端（sqlite）
cd test && go test ./...                # 集成测试（真实 MySQL，使用 test/generate 产物）
```

## License

MIT

gooq 的设计源于 GoFrame gdb 在发展中暴露出的不足，并结合了 [jOOQ](https://www.jooq.org/) 的理念；代码从 GoFrame 中的 gooq 原型（MIT License）演变而来，现已独立发展。
