English | [简体中文](README.zh_CN.MD)

# gooq-gen

`gooq-gen` is the code generation tool for [gooq](https://github.com/lanceadd/gooq). It connects to a database and generates typed table objects (`table/`) plus scan and write models (`model/`).

Library-side configuration, workflow, sharding groups, and tenant integration are documented in the [root README](../README.md#gooq-gen-codegen-tool).

## Install

```bash
go install github.com/lanceadd/gooq/cmd/gooq-gen@latest
```

From a checkout, run it with `cd cmd/gooq-gen && go run .`.

## Usage

```bash
# Create hack/gooq.yaml and the four templates. Existing files are kept.
gooq-gen init

# Generate with CLI options.
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)" -p internal

# Debug logging and help.
gooq-gen -d -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -h
```

Without `-l` / `-p`, values come from `hack/gooq.yaml`. `gooq-gen` searches upward from the current directory for a `hack` directory. CLI flags take priority.

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

- `gooq.gen.link` / `gooq.gen.path`: database link and output directory (`internal` by default).
- `gooq.gen.tenant`: optional. Marks tenant columns; see Tenant below.
- `gooq.tables.check.ignore`: optional glob list; see Table grouping below.

## Commands

| Command | Description |
| --- | --- |
| `gooq-gen` | Generate `table/` and `model/` into the output directory. |
| `gooq-gen init` | Create `hack/gooq.yaml` and `hack/template/{table,meta,sharding,register,model,record}.tmpl`. Existing files are never overwritten. |
| `gooq-gen tables init` | Create `hack/gooq.tables.yaml` with every database table under `single` and `groups: []`. Requires a `hack` directory (`gooq-gen init` creates it) and refuses to overwrite an existing file. |
| `gooq-gen tables check` | Check that `hack/gooq.tables.yaml` matches the database. Silent and exits 0 when it matches; prints the report and exits non-zero otherwise. |
| `gooq-gen tables diff` | Report tables added in the database or no longer listed in the file. Exits non-zero when there is a difference. |

`tables check` and `tables diff` require `hack/gooq.tables.yaml`; `tables check`, `tables diff`, and `tables init` all connect to the database. `tables check` reports duplicate, missing, extra, logical-name conflicts, and grouping errors.

| Option | Applies to | Description |
| --- | --- | --- |
| `-l, --link` | generate, tables | Database link, same format as GoFrame's ORM configuration (or set `gooq.gen.link`). |
| `-p, --path` | generate | Output directory, default `internal` (or set `gooq.gen.path`). |
| `-d, --debug` | generate | Show internal debug information. |
| `-h, --help` | all | Show command help. |

## Table grouping

Without `hack/gooq.tables.yaml`, every physical table generates its own object and no sharding helpers are produced.

With the file, generation only covers the listed tables. `single` lists ordinary tables; `groups` lists sharded tables. The file must list every database table, except tables matching `gooq.tables.check.ignore`.

```yaml
single:
  - "dict"
  - "user"

groups:
  - name: "order"        # logical table name
    table:
      - "order_00"       # the first table supplies the schema
      - "order_01"
```

- `groups[].name` becomes the generated type name, the file base name, and `TableMeta.TableName`.
- The first table in a group supplies the column definitions. Every table in the group must match it: column name, order, type, nullability, primary key, auto-increment, and uniqueness.
- Every physical table name must contain its group name; the check is case-sensitive.
- `groups[].name` and a `single` table cannot share a name.
- Files for tables removed from the database or the file are not deleted. `table/gooq_sharding.go` is removed automatically when no sharded groups remain.

`gooq.tables.check.ignore` accepts glob patterns (`path.Match`, case-sensitive). A database table that is not listed in the file and matches a pattern is skipped by generation, `tables check`, and `tables diff`. A table that is listed in the file must still exist, even if it matches a pattern.

Typical workflow:

```bash
gooq-gen tables init     # first time only
# edit groups in hack/gooq.tables.yaml
gooq-gen tables diff     # see added/removed tables
gooq-gen tables check    # verify
gooq-gen                 # regenerate
```

## Generated output

For `user`, generation writes separate files.

`table/user.table.go` contains the typed object and its query-time helpers:

```go
type UserTable struct {
    *gooq.TableBase

    Id   gooq.Field[int64]
    Name gooq.Field[string]
    // ...
}

func newUserTable(meta *gooq.TableMeta, alias string) *UserTable { /* ... */ }

var User = newUserTable(UserMeta, "")

func (t *UserTable) As(alias string) *UserTable { /* ... */ }
func (t *UserTable) Clone() *UserTable          { /* ... */ }
```

`table/user.meta.go` contains the static metadata:

```go
var UserMeta = &gooq.TableMeta{
    TableName: "user",
    Fields:    []gooq.FieldMeta{ /* ... */ },
}
```

A sharded group also writes `table/user.sharding.go`:

```go
func RegisterUserSharding(rule gooq.ShardingRule) { /* ... */ }

func (t *UserTable) Physical(name string) *UserTable { /* ... */ }
func (t *UserTable) ShardBy(value any) *UserTable    { /* ... */ }
```

`UserMeta.ShardNames` records the physical tables that existed at generation time. `table/gooq_sharding.go` defines `RegisterSharding(rule)`, which applies one rule to every sharded logical table.

`model/user.model.go` defines `UserModel`, a plain struct tagged with `json` and `orm` for `db.Result.Structs` and JSON. `model/user.record.go` defines `UserRecord`, whose fields are `gooq.Property[T]`. `NewUserRecord().SetName("john")` marks columns for `dsl.Insert(...).ValuesOf`; nullable columns also get `SetXxxNull`.

## Tenant

Set `gooq.gen.tenant.column` to mark tenant columns at generation time. A missing or null node leaves generation unchanged. `tables` overrides the column for a logical table; an empty string turns the mark off for that table.

```yaml
gooq:
  gen:
    tenant:
      column: tenant_id
      tables:
        shop_order: org_id
        dict: ""
```

Tenant column matching ignores case and underscores. Allowed types are `string`, `uuid.UUID`, and the integer widths. Runtime filtering and write checks are documented in the root README.

## Templates

Run `gooq-gen init` to create `hack/template/table.tmpl`, `meta.tmpl`, `sharding.tmpl`, `register.tmpl`, `model.tmpl`, and `record.tmpl`. `table.tmpl` renders `<logical>.table.go`, `meta.tmpl` renders `<logical>.meta.go`, `sharding.tmpl` renders `<logical>.sharding.go` for sharded tables, and `register.tmpl` renders the package-level `gooq_sharding.go`. Generation prefers these files and falls back to the embedded template. Edit and re-run to apply; delete one to fall back. `init` never overwrites existing files.

## Drivers

`main.go` imports `drivers/mysql`, `mariadb`, `tidb`, `oceanbase`, `pgsql`, `gaussdb`, `dm`, and `sqlite`. Link prefixes are `mysql`, `mariadb`, `tidb`, `oceanbase`, `pgsql`/`postgres`, `gaussdb`, `dm`, and `sqlite`.

Another database implements `db.Driver`, calls `db.Register` from `init`, and is blank-imported from `main.go`. Rendering differences implement `gooq.DialectSpec` and call `gooq.RegisterDialect`.

## Tests

```bash
go test ./... -count=1
```

The e2e tests create a sqlite database, run generation, and assert the generated table artifacts. They also cover `init`, config reading, template overriding, and `gooq.tables.check.ignore`.
