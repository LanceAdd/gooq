English | [简体中文](README.zh_CN.MD)

# gooq-gen

`gooq-gen` is the code generation tool for [gooq](https://github.com/lanceadd/gooq), derived from the GoFrame CLI `gen dao`. It connects to a database and generates typed gooq table objects for every table in one shot. The `do/` and `entity/` structs are left to GoFrame's own `gf gen dao`.

## Usage

```bash
# Optional: initialize hack/gooq.yaml and hack/template/table.tmpl for config file & template customization.
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

Built-in drivers: `mysql`, `pgsql`, `sqlite`. To enable other drivers (e.g. `dm`), uncomment the import in `internal/cmd/cmd.go` and run `go mod tidy`.

## Commands & Options

| Command / Option | Description |
| --- | --- |
| `gooq-gen init` | create `hack/gooq.yaml` and `hack/template/table.tmpl` for customization; existing files are never overwritten |
| `-l, --link` | database link, same as the ORM configuration of GoFrame (or set `gooq.gen.table.link`) |
| `-p, --path` | output directory, default `internal` (or set `gooq.gen.table.path`) |
| `-d, --debug` | show internal detailed debugging information |

All tables of the database are generated into the fixed `table/` subdirectory of the output directory.

## Template customization

Run `gooq-gen init` to create the template `hack/template/table.tmpl`. Generation prefers `<hack>/template/table.tmpl` when present and falls back to the embedded template — edit it and re-run `gooq-gen` to see the effect. Delete it to fall back to the built-in template.

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

Typical workflow: generate `dao/do/entity` with GoFrame's own `gf gen dao`, then generate the typed table objects with `gooq-gen` (run `gooq-gen init` once to scaffold our own `hack/gooq.yaml`).

## Tests

```bash
go test ./... -count=1
```

The e2e tests create a sqlite database, run generation, and assert the generated table artifacts; the `init` command, config file reading and template overriding are covered as well.
