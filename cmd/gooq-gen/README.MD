English | [简体中文](README.zh_CN.MD)

# gooq-gen

`gooq-gen` is the code generation tool for [gooq](https://github.com/lanceadd/gooq). It connects to a database and generates typed table objects (`table/`) plus `user_model.go` and `user_record.go` for every table.

## Usage

```bash
# Optional: initialize hack/gooq.yaml and hack/template/{table,sharding,model,record}.tmpl for customization.
gooq-gen init

# Generate via CLI options.
gooq-gen -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
gooq-gen -l "sqlite::@file(./test.db)" -p internal
```

Or via the config file `hack/gooq.yaml` created by `init` (CLI options take priority):

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

Built-in drivers `mysql`, `pgsql`, and `sqlite` are imported by `main.go`. Another database implements `db.Driver`, calls `db.Register` from `init`, and is blank-imported from `main.go`. Rendering differences implement `gooq.DialectSpec` and `gooq.RegisterDialect`.

## Commands & Options

| Command / Option | Description |
| --- | --- |
| `gooq-gen init` | create `hack/gooq.yaml` and `hack/template/table.tmpl`, `sharding.tmpl`, `model.tmpl`, `record.tmpl` for customization; existing files are never overwritten |
| `-l, --link` | database link, same as the ORM configuration of GoFrame (or set `gooq.gen.table.link`) |
| `-p, --path` | output directory, default `internal` (or set `gooq.gen.table.path`) |
| `-d, --debug` | show internal detailed debugging information |

All tables of the database are generated into the fixed `table/` and `model/` subdirectories of the output directory. `model/user_model.go` defines `UserModel`. `model/user_record.go` defines `UserRecord`.

## Template customization

Run `gooq-gen init` to create `hack/template/table.tmpl`, `sharding.tmpl`, `model.tmpl`, and `record.tmpl`. Generation prefers those files when present and falls back to the embedded template — edit one and re-run `gooq-gen` to see the effect. Delete it to fall back to the built-in template.

## Generated example

Given a table `user`, `table/user.go` is generated as:

```go
// UserTable is the typed table object for table "user".
type UserTable struct {
    *gooq.TableBase

    Id   gooq.Field[int64]
    Name gooq.Field[string]
    ...
}

var UserMeta = &gooq.TableMeta{ /* static metadata */ }

func newUserTable(alias ...string) *UserTable { ... }

// User is the globally accessible table object for table user.
var User = newUserTable()

func (t *UserTable) As(alias string) *UserTable { return newUserTable(alias) }
func (t *UserTable) Clone() *UserTable          { return newUserTable() }
```

Run `gooq-gen init` once to scaffold `hack/gooq.yaml`, then `gooq-gen` to generate `table/` and `model/`. `UserModel` is the plain scan struct. `UserRecord` is the partial write row.

## Tests

```bash
go test ./... -count=1
```

The e2e tests create a sqlite database, run generation, and assert the generated table artifacts; the `init` command, config file reading and template overriding are covered as well.
