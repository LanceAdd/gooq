[English](README.MD) | 简体中文

# ggen

`ggen` 是 [gooq](https://github.com/lanceadd/gooq) 的代码生成工具，由 GoFrame CLI 的 `gen dao` 派生而来。连接数据库后一次性生成所有表的三类产物：

- `do/` —— DO 结构体（数据操作用，字段类型统一为 `any`，带 `g.Meta` 标记）。
- `entity/` —— Entity 结构体（带 `json`/`orm` tag，时间字段映射为 stdlib `time.Time`）。
- `table/` —— gooq 类型化表对象（`Field[T]` + `TableMeta`）。

## 使用

```bash
go run . -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
go run . -l "sqlite::@file(./test.db)" -p internal
```

内置驱动：`mysql`、`pgsql`、`sqlite`。如需其他驱动（如 `dm`），取消 `internal/cmd/cmd.go` 中对应导入的注释并执行 `go mod tidy`。

## 参数

| 参数 | 说明 |
| --- | --- |
| `-l, --link` | 数据库连接，与 GoFrame 的 ORM 配置一致（必填） |
| `-p, --path` | 输出目录，默认 `internal` |
| `-t, --tpl` | 将内置模板导出到当前目录 `./template` 后退出 |

默认生成数据库全部表；输出目录下固定为 `table/`、`do/`、`entity/` 三个子目录。

## 模板定制

运行 `ggen -t` 将内置模板（`template/do.tpl`、`template/entity.tpl`、`template/gooq_table.tpl`）导出到当前工作目录。生成时优先使用 `./template` 下的本地模板，不存在时回退到内置模板——修改导出的模板后重新运行 `ggen` 即可生效。

## 产物示例

以表 `user` 为例：

- `table/user.go` —— gooq 类型化表对象：

```go
var User = &UserTable{
    TableBase: gooq.NewTableBase(&gooq.TableMeta{
        TableName: "user",
        Fields: []gooq.FieldMeta{
            {ColumnName: "id", LocalType: gooq.LocalType("int"), Primary: true, AutoIncrement: true},
            ...
        },
    }),
    ...
}
```

- `do/user.go`：

```go
type User struct {
    g.Meta `orm:"table:user, do:true"`
    Id     any //
    Name   any //
}
```

- `entity/user.go`：

```go
type User struct {
    Id        int       `json:"id" orm:"id"`
    Name      string    `json:"name" orm:"name"`
    CreatedAt time.Time `json:"createdAt" orm:"created_at"`
}
```

## 测试

```bash
go test ./... -count=1
```

端到端测试创建 sqlite 数据库并运行生成，断言三类产物的内容。
