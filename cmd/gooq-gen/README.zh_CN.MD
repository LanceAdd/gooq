[English](README.MD) | 简体中文

# gooq-gen

`gooq-gen` 是 [gooq](https://github.com/lanceadd/gooq) 的代码生成工具。连接数据库后一次性为所有表生成类型化表对象（`table/`），以及 `user_model.go` 和 `user_record.go`。

## 使用

```bash
# 可选：初始化 hack/gooq.yaml 与 hack/template/{table,model,record}.tmpl（配置文件与模板定制用）。
gooq-gen init

# 通过命令行参数生成。
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)" -p internal
```

或通过 `init` 生成的配置文件 `hack/gooq.yaml`（命令行参数优先）：

```yaml
gooq:
  gen:
    table:
      link: "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
      path: internal
```

```bash
gooq-gen
```

内置驱动 `mysql`、`pgsql`、`sqlite` 由 `main.go` 导入。其他数据库实现 `db.Driver`，在 `init` 中调用 `db.Register`，再从 `main.go` 空白导入。渲染差异实现 `gooq.DialectSpec` 并调用 `gooq.RegisterDialect`。

## 命令与参数

| 命令 / 参数 | 说明 |
| --- | --- |
| `gooq-gen init` | 生成 `hack/gooq.yaml` 与 `hack/template/table.tmpl` 供定制；已存在的文件不覆盖 |
| `-l, --link` | 数据库连接，与 GoFrame 的 ORM 配置一致（或配置 `gooq.gen.table.link`） |
| `-p, --path` | 输出目录，默认 `internal`（或配置 `gooq.gen.table.path`） |
| `-d, --debug` | 输出内部调试信息 |

默认生成数据库全部表，产物位于输出目录下固定的 `table/` 与 `model/` 子目录。`model/user_model.go` 定义 `UserModel`，`model/user_record.go` 定义 `UserRecord`。

## 模板定制

运行 `gooq-gen init` 生成模板 `hack/template/table.tmpl`。生成时该文件存在则优先使用，否则回退内置模板——修改后重新运行 `gooq-gen` 即可生效，删除该文件即回退内置模板。

## 产物示例

以表 `user` 为例，生成的 `table/user.go`：

```go
// UserTable is the typed table object for table "user".
type UserTable struct {
    *gooq.TableBase

    Id   gooq.Field[int64]
    Name gooq.Field[string]
    ...
}

var UserMeta = &gooq.TableMeta{ /* 静态元数据 */ }

func newUserTable(alias ...string) *UserTable { ... }

// User is the globally accessible table object for table user.
var User = newUserTable()

func (t *UserTable) As(alias string) *UserTable { return newUserTable(alias) }
func (t *UserTable) Clone() *UserTable          { return newUserTable() }
```

首次运行 `gooq-gen init` 生成 `hack/gooq.yaml`，再运行 `gooq-gen` 生成 `table/` 与 `model/`。`UserModel` 是用于扫描的普通结构体，`UserRecord` 是部分写入的记录。

## 测试

```bash
go test ./... -count=1
```

端到端测试创建 sqlite 数据库并运行生成，断言表对象产物内容；同时覆盖 `init` 命令、配置文件读取与本地模板覆盖。
