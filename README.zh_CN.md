# gooq

`gooq` 是 GoFrame 的类型化 SQL 查询 DSL，灵感来自 [jOOQ](https://www.jooq.org/)。

它以类型安全的方式构建 SQL——`Select`/`From`/条件/子查询/函数/离线渲染，不依赖任何数据库实例。生成的 SQL 由调用方自选执行方式（标准库 `database/sql`、`gdb` 等）。

## 特性

- **纯 SQL 构建器**：渲染不连库，`ToSql(dialect)` 离线产出 SQL + 参数。
- **类型化字段**：`Field[T]` 在编译期携带列类型，比较方法（`Eq(v T)`、`Gt(v T)` 等）编译期拦截类型不匹配；表达式操作数（子查询、列比较、`Raw`）走显式 `EqExpr`/`InExpr` 方法。
- **条件一等对象**：条件可独立构建、复用、动态组装，配合 `AND(...)`/`OR(...)`/`NOT(...)`。
- **方言注册表**：内置 MySQL/PG/SQLite；驱动通过 `RegisterDialect` 注册方言以覆盖渲染细节。
- **离线设计**：软删除自动条件、自增列跳过、`AllFields()` 全列派生全部来自 `TableMeta`——生成期静态化的元数据，运行时零猜测。

## 快速开始

```go
import (
    "github.com/gogf/gf/v2/database/gooq"
)

// 类型化表对象（由 ggen 生成，或参考 gooq_example_table.go）。
var User = ... // UserTable{ID: Field[int64], Name: Field[string], ...}

// 离线构建 SQL。
sql, args, err := gooq.Select(User.ID, User.Name).
    From(User).
    Where(User.Age.Gt(18)).
    Order(User.ID.Desc()).
    Limit(10).
    ToSql(gooq.DialectMySQL)
// sql:  SELECT id, name FROM user WHERE age > ? AND deleted_at IS NULL ORDER BY id DESC LIMIT 10
// args: []any{18}
```

SQL 的执行方式任选：

```go
// 标准库
rows, _ := db.Query(sql, args...)

// 或 gdb
result, _ := gdb.Instance().GetAll(ctx, sql, args...)
```

## 表对象

类型化表对象由 `ggen` 工具（`database/gooq/cmd/ggen`）从数据库表结构生成，也可用 `TableBase` + `TableMeta` 手写：

```go
type UserTable struct {
    *gooq.TableBase
    ID        gooq.Field[int64]
    Name      gooq.Field[string]
    Age       gooq.Field[int]
    DeletedAt gooq.Field[*gtime.Time]
}

var User = &UserTable{
    TableBase: gooq.NewTableBase(&gooq.TableMeta{
        TableName: "user",
        Fields: []gooq.FieldMeta{
            {ColumnName: "id", LocalType: gooq.LocalTypeInt64, Primary: true, AutoIncrement: true},
            {ColumnName: "name", LocalType: gooq.LocalTypeString},
            {ColumnName: "deleted_at", LocalType: gooq.LocalTypeDatetime, SoftDelete: true},
        },
    }),
    ID:        gooq.NewField[int64]("user", "id"),
    Name:      gooq.NewField[string]("user", "name"),
    Age:       gooq.NewField[int]("user", "age"),
    DeletedAt: gooq.NewField[*gtime.Time]("user", "deleted_at"),
}
```

- `As(alias)` 返回带别名的副本（用于 JOIN 与自连接）；`Clone()` 返回无别名副本。
- `AllFields()` 从 `TableMeta` 派生全列 `[]Field[any]`——可直接 `Select(User.AllFields())`，也可组合：`Select(u.AllFields(), o.Amount)`。
- `TableMeta` 静态承载列元数据（主键/自增/软删/唯一/注释），供软删除自动条件、自增列跳过、`FieldsEx` 差集使用。

## 查询 DSL

| 能力 | 示例 |
| --- | --- |
| 入口 | `Select(字段...).From(t)` / `SelectFrom(t)` |
| 字段 | `Fields(...)` / `FieldsEx(...)`（集合差集）/ `字段.As("别名")` / `AllFields()` |
| 条件 | `Where(条件...)`（默认 AND）/ `And(...)` / `Or(...)` |
| 字段操作符 | `Eq/Ne/Gt/Gte/Lt/Lte/Like/NotLike/In/NotIn/Between/IsNull/IsNotNull`（值类型强约束） |
| 表达式操作符 | `EqExpr/NeExpr/GtExpr/.../BetweenExpr`（子查询、列比较、`Raw`） |
| 组合 | `AND(...)` / `OR(...)` / `NOT(...)` |
| 子查询 | `IN (SELECT ...)` / 标量子查询 / 派生表（`As("t")`） |
| EXISTS | `Exists(sub)` / `NotExists(sub)` |
| JOIN | `LeftJoin/RightJoin/InnerJoin/FullJoin(o).On(...)` |
| 分组扩展 | `GroupRollup/GroupCube/GroupingSets`（方言感知） |
| 排序/分页 | `Order(字段.Desc()/Asc().NullsFirst()/NullsLast())` / `Group` / `Having` / `Limit` / `Offset` / `Page` |
| 集合操作 | `Union/UnionAll/Intersect/Except` |
| CTE | `With(name, sub).From(Cte(name))` / `WithRecursive` |
| 行锁 | `LockForUpdate()` / `LockInShareMode()` |
| 去重 | `Distinct()` |

## 表达式与函数

- **算术**：`字段.Mul(2)` / 包级 `Add/Sub/Mul/Div/Negate`（支持嵌套）。
- **条件表达式**：`Case().When(条件).Then(v).Else(v).End().As("别名")`。
- **函数库（30+）**：字符串（`Concat/Substring/Upper/Lower/Trim/Replace/Length`）、数学（`Abs/Round/Ceil/Floor/Mod`）、日期（`CurDate/DateAdd/DateDiff`）、聚合（`Count/Sum/Avg/Min/Max/CountDistinct`）、通用（`Coalesce/IfNull/Now`）。
- **窗口函数**：`Rank/RowNumber/DenseRank/Ntile/Lag/Lead` + `Over(partitionBy, orderBy)` + `OverFrame`。
- **字符串聚合**：`GroupConcatFunc`（按方言渲染 GROUP_CONCAT / STRING_AGG）。
- **自定义操作符**：`OperatorFunc(name, impl, drivers...)` 注册 + `Func(name, args...)` 调用。
- **Raw**：`Raw(sql, args...)` 结构化 SQL，支持参数绑定。

## 写操作（DML）

| 操作 | 说明 |
| --- | --- |
| `Insert(t)` + `Record(实体)` / `Records([]实体)` | 实体结构体字面量，零值字段跳过，自动跳过自增列 |
| `Insert(t)` + `Columns(字段...).Values(值...)` | 列值位置匹配，多次 `Values` 即批量 |
| `Batch(size)` | 批量插入分批执行，`RowsAffected` 聚合 |
| `InsertFrom(t, 子查询)` | INSERT ... SELECT |
| `Update(t)` + `Set(字段, 值)` | 单字段链式更新 |
| `Update(t)` + `Record(实体)` | 非零字段转为 SET（gorm 风格） |
| `Update(t)` + `Data(map)` | map 全量更新 |
| `Update(t)` + `Records([]实体)` | 按主键（或 `Keys(...)`）批量更新，`RowsAffected` 聚合 |
| `Update ... Join` | 多表 UPDATE（MySQL `JOIN`、PG/SQLite `FROM`） |
| `Delete(t)` | 软删表自动转 `UPDATE deleted_at`；`Unscoped()` 真 DELETE |
| `Returning(字段...)` | PG/SQLite `RETURNING`（MySQL 渲染报错） |
| Upsert | `OnConflictKey(...)` + `DoUpdate(...)` / `DoNothing()`（MySQL `INSERT IGNORE`/`ON DUPLICATE KEY UPDATE`，PG `ON CONFLICT`） |
| 软删除 | 自动 `deleted_at IS NULL`；显式引用列名接管；`Unscoped()` 绕过 |

## 方言

| 方言 | 占位符 | 引号 | 分页 | 共享锁 |
| --- | --- | --- | --- | --- |
| mysql | `?` | `` ` `` | LIMIT | LOCK IN SHARE MODE |
| pgsql | `$n` | `"` | LIMIT | FOR SHARE |
| sqlite | `?` | `"` | LIMIT | — |

未注册方言回退默认渲染；驱动可 `RegisterDialect` 增量覆盖内置方言。

## ggen（代码生成工具）

`cmd/ggen` 连接数据库后一次性生成所有表的三类产物：`do/`（DO 结构体）、`entity/`（带 `json`/`orm` tag 的实体，`time.Time`）、`table/`（gooq 类型化表对象）：

```bash
cd cmd/ggen && go run . -l "mysql:root:pass@tcp(127.0.0.1:3306)/db"
```

- 参数：`-l/--link`（数据库连接，必填）、`-p/--path`（输出目录，默认 `internal`）、`-t/--tpl`（导出内置模板到 `./template` 便于定制后退出；生成时本地模板优先于内置模板）。
- 元数据推导：主键（`PRI`）、自增（`auto_increment`）、软删（列名约定）、唯一（`UNI`）、`LocalType` 标记；Go 命名规范（`id` → `ID`）。
- 内置驱动：mysql/pgsql/sqlite；其他驱动取消 `internal/cmd/cmd.go` 中 import 注释启用。

## 测试

```bash
go test ./ -count=1                # 主库（渲染断言）
cd cmd/ggen && go test ./...       # ggen 端到端（sqlite）
cd test && go test ./...           # 集成测试（真实 MySQL，使用 test/generate 产物）
```

`test/generate/` 存放 ggen 从 MySQL `test` 库生成的产物（连接配置见 `test/generate_test.go`）；重新生成：

```bash
cd cmd/ggen && go run . -l "mysql:root:xxx@tcp(127.0.0.1:3306)/test?loc=Local&parseTime=true" -p ../test/generate
```

## 执行（gooq + gdb 融合）

`gooq` 构建 SQL 并可直接对 `gdb` 执行——`UseDB`/`UseTX` 绑定数据库后 `Scan`/`Exec`；方言从 gdb 驱动名自动推导，离线 `ToSql` 路径保留：

```go
// 查询 + 映射到 struct 切片。
users := []model.User{}
err := gooq.Select(User.AllFields()).From(User).
    UseDB(gdb.DB()).
    Where(User.Age.Gt(18)).
    Order(User.ID.Desc()).
    Limit(10).
    Scan(ctx, &users)

// 标量（COUNT 等）。
count := int64(0)
err := gooq.Select(CountFunc(User.ID)).From(User).
    UseDB(gdb.DB()).Scan(ctx, &count)

// 写操作。
_, err = gooq.Insert(User).Record(model.User{Name: "john"}).UseDB(gdb.DB()).Exec(ctx)
_, err = gooq.Insert(User).Columns(User.Name, User.Level).Values("john", 2).UseDB(gdb.DB()).Exec(ctx)

// 事务：UseTX 绑定事务连接，提交后外部可见。
tx, _ := db.Begin(ctx)
_, err = gooq.Update(User).Set(User.Status, "vip").Where(User.ID.Eq(1)).UseTX(tx).Exec(ctx)
tx.Commit()

// 离线路径不变（不绑定数据库，仅渲染）。
sql, args, _ := gooq.Select(User.ID).From(User).Where(User.ID.Eq(1)).ToSql(gooq.DialectMySQL)
```

`Scan` 复用 gdb 现有 struct 转换（`orm`/`json` tag）；`UseDB` 可重新绑定以支持多库/读写分离；未绑定数据库时调用 `Scan`/`Exec` 返回明确错误。

行级类型化读取（字段类型 T 在编译期被消费）：

```go
row, err := gooq.Select(User.ID, User.Name, User.Age).From(User).
    Where(User.Name.Eq("john")).UseDB(gdb.DB()).Row(ctx)
id := gooq.Get(row, User.ID)     // int64 —— 类型来自 Field[int64]
name := gooq.Get(row, User.Name) // string
age := gooq.Get(row, User.Age)   // int
```

缓存：配置 `Cache` 选项并注入全局 `CacheAdapter` 后，`Scan` 走缓存读取（未命中执行并回填；缓存故障不阻断主查询）。
