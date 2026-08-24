English | [简体中文](README.zh_CN.MD)

# ggen

`ggen` is the code generation tool for [gooq](https://github.com/lanceadd/gooq), derived from the GoFrame CLI `gen dao`. It connects to a database and generates all three kinds of artifacts for every table in one shot:

- `do/` — DO structs for data operations (all field types are `any`, tagged with `g.Meta`).
- `entity/` — Entity structs with `json`/`orm` tags, time fields mapped to stdlib `time.Time`.
- `table/` — Typed gooq table objects (`Field[T]` + `TableMeta`).

## Usage

```bash
go run . -l "mysql:root:12345678@tcp(127.0.0.1:3306)/test"
go run . -l "sqlite::@file(./test.db)" -p internal
```

Built-in drivers: `mysql`, `pgsql`, `sqlite`. To enable other drivers (e.g. `dm`), uncomment the import in `internal/cmd/cmd.go` and run `go mod tidy`.

## Options

| Option | Description |
| --- | --- |
| `-l, --link` | database link, same as the ORM configuration of GoFrame (required) |
| `-p, --path` | output directory, default `internal` |

All tables of the database are generated — output subdirectories are fixed to `table/`, `do/`, `entity/` under the output directory.

## Generated examples

Given a table `user`:

- `table/user.go` — gooq typed table object:

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

- `do/user.go`:

```go
type User struct {
    g.Meta `orm:"table:user, do:true"`
    Id     any //
    Name   any //
}
```

- `entity/user.go`:

```go
type User struct {
    Id        int       `json:"id" orm:"id"`
    Name      string    `json:"name" orm:"name"`
    CreatedAt time.Time `json:"createdAt" orm:"created_at"`
}
```

## Tests

```bash
go test ./... -count=1
```

The e2e test creates a sqlite database, runs generation, and asserts the content of all three kinds of artifacts.
