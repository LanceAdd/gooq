[English](README.MD) | 简体中文

# gooq-gen

`gooq-gen` 是 [gooq](https://github.com/lanceadd/gooq) 的代码生成工具。连接数据库后生成类型化表对象（`table/`）和扫描/写入模型（`model/`）。

库侧的配置、工作流、分片分组和租户接入见 [根 README](../README.zh_CN.md#gooq-gen代码生成工具)。

## 安装

```bash
go install github.com/lanceadd/gooq/cmd/gooq-gen@latest
```

从仓库运行：`cd cmd/gooq-gen && go run .`。

## 使用

```bash
# 生成 hack/gooq.yaml 与四个模板，已存在的文件不覆盖。
gooq-gen init

# 通过命令行参数生成。
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)" -p internal

# 调试日志与帮助。
gooq-gen -d -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -h
```

不传 `-l` / `-p` 时，从 `hack/gooq.yaml` 读取。`gooq-gen` 从当前目录向上查找 `hack` 目录；命令行参数优先。

```yaml
gooq:
  gen:
    link: "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
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

- `gooq.gen.link` / `gooq.gen.path`：数据库连接和输出目录（默认 `internal`）。
- `gooq.gen.tenant`：可选，生成时标记租户列，见下面的“租户”。
- `gooq.tables.check.ignore`：可选通配符列表，见下面的“表分组”。

## 命令

| 命令 | 说明 |
| --- | --- |
| `gooq-gen` | 在输出目录生成 `table/` 和 `model/`。 |
| `gooq-gen init` | 生成 `hack/gooq.yaml` 和 `hack/template/{table,sharding,model,record}.tmpl`；已存在的文件不覆盖。 |
| `gooq-gen tables init` | 生成 `hack/gooq.tables.yaml`，把库里的全部表写进 `single`，`groups: []`；需要已有 `hack` 目录（`gooq-gen init` 会创建），文件已存在时拒绝覆盖。 |
| `gooq-gen tables check` | 检查 `hack/gooq.tables.yaml` 与数据库是否一致。一致时静默退出 0；否则打印报告并退出非 0。 |
| `gooq-gen tables diff` | 报告库里新增的表和文件里已不用的表。有差异时退出非 0。 |

`tables check` 和 `tables diff` 要求 `hack/gooq.tables.yaml` 已存在；`tables init`、`tables check`、`tables diff` 都会连数据库。`tables check` 报告重复、缺失、多余、逻辑名冲突和分组错误。

| 参数 | 适用命令 | 说明 |
| --- | --- | --- |
| `-l, --link` | generate、tables | 数据库连接，与 GoFrame 的 ORM 配置一致（或配置 `gooq.gen.link`）。 |
| `-p, --path` | generate | 输出目录，默认 `internal`（或配置 `gooq.gen.path`）。 |
| `-d, --debug` | generate | 输出内部调试信息。 |
| `-h, --help` | 全部 | 查看命令帮助。 |

## 表分组

没有 `hack/gooq.tables.yaml` 时，每张物理表各生成一个对象，不生成分片辅助方法。

有这个文件时，只为其中列出的表生成。`single` 是普通表，`groups` 是分表。文件必须列出库里的全部表，匹配 `gooq.tables.check.ignore` 的表除外。

```yaml
single:
  - "dict"
  - "user"

groups:
  - name: "order"        # 逻辑表名
    table:
      - "order_00"       # 组里第一张表提供列结构
      - "order_01"
```

- `groups[].name` 同时作为生成的类型名、文件名和 `TableMeta.TableName`。
- 组里第一张表提供列结构；组内每张表都必须与它一致：列名、顺序、类型、可空、主键、自增、唯一。
- 每张物理表名必须包含所属组名，比较区分大小写。
- `groups[].name` 不能与 `single` 里的表同名。
- 库里或文件里删除的表，旧生成文件不会自动删除。没有分片组时 `table/gooq_sharding.go` 会自动删除。

`gooq.tables.check.ignore` 接受通配符（`path.Match`，区分大小写）。库里的表如果不在文件中且匹配某个模式，生成、`tables check` 和 `tables diff` 都会跳过它；文件里列出的表即使匹配模式，也必须真实存在。

常用流程：

```bash
gooq-gen tables init     # 只在第一次执行
# 编辑 hack/gooq.tables.yaml 的 groups
gooq-gen tables diff     # 查看新增/删除的表
gooq-gen tables check    # 校验
gooq-gen                 # 重新生成
```

## 产物

以 `user` 为例，`table/user.go` 包含：

```go
type UserTable struct {
    *gooq.TableBase

    Id   gooq.Field[int64]
    Name gooq.Field[string]
    // ...
}

var UserMeta = &gooq.TableMeta{
    TableName: "user",
    Fields:    []gooq.FieldMeta{ /* ... */ },
}

func newUserTable(meta *gooq.TableMeta, alias string) *UserTable { /* ... */ }

var User = newUserTable(UserMeta, "")

func (t *UserTable) As(alias string) *UserTable { /* ... */ }
func (t *UserTable) Clone() *UserTable          { /* ... */ }
```

分片组还会生成：

```go
func RegisterUserSharding(rule gooq.ShardingRule) { /* ... */ }

func (t *UserTable) Physical(name string) *UserTable { /* ... */ }
func (t *UserTable) ShardBy(value any) *UserTable    { /* ... */ }
```

`UserMeta.ShardNames` 记录生成时存在的物理分表。`table/gooq_sharding.go` 定义 `RegisterSharding(rule)`，一次给所有分片逻辑表设置同一个规则。

`model/user_model.go` 定义 `UserModel`，是带 `json`、`orm` 标签的普通结构体，给 `db.Result.Structs` 和 JSON 用。`model/user_record.go` 定义 `UserRecord`，字段是 `gooq.Property[T]`。`NewUserRecord().SetName("john")` 标记要写入的列，交给 `dsl.Insert(...).ValuesOf`；可空列另外生成 `SetXxxNull`。

## 租户

配置 `gooq.gen.tenant.column` 后，生成时会把租户列标记进元数据。没有这个节点或为 null 时行为不变。`tables` 可以给单个逻辑表换列名；空字符串表示这张表不标记。

```yaml
gooq:
  gen:
    tenant:
      column: tenant_id
      tables:
        shop_order: org_id
        dict: ""
```

匹配租户列时忽略大小写和下划线。允许的类型是 `string`、`uuid.UUID` 和各宽度整数。运行时的过滤和写入检查见根目录 README。

## 模板

运行 `gooq-gen init` 生成 `hack/template/table.tmpl`、`sharding.tmpl`、`model.tmpl`、`record.tmpl`。生成时优先使用这些文件，缺失时回退内置模板。修改后重新运行即可生效；删除其中一个即回退。`init` 不覆盖已有文件。

## 驱动

`main.go` 导入了 `drivers/mysql`、`mariadb`、`tidb`、`oceanbase`、`pgsql`、`gaussdb`、`dm`、`sqlite`。链接前缀是 `mysql`、`mariadb`、`tidb`、`oceanbase`、`pgsql`/`postgres`、`gaussdb`、`dm`、`sqlite`。

其他数据库实现 `db.Driver`，在 `init` 中调用 `db.Register`，再从 `main.go` 空白导入。渲染差异实现 `gooq.DialectSpec` 并调用 `gooq.RegisterDialect`。

## 测试

```bash
go test ./... -count=1
```

端到端测试创建 sqlite 数据库、运行生成并断言表对象产物，同时覆盖 `init`、配置读取、本地模板覆盖和 `gooq.tables.check.ignore`。
