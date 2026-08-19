# gooq e2e tests

End-to-end smoke tests that verify the full pipeline: gooq DSL builds SQL → gdb executes it against a real MySQL.

## Prerequisites

- Docker (or any local MySQL 8 on port 3307)
- A clean working copy of the repository (no uncommitted files that break compilation)

## Run

```bash
# 1. Start MySQL (port 3307, database gooq_test)
docker run -d --name gooq-mysql \
  -e MYSQL_ROOT_PASSWORD=root123 \
  -e MYSQL_DATABASE=gooq_test \
  -p 3307:3306 mysql:8

# 2. Run tests
cd database/gooq/e2e
go test ./...
```

The tests create and truncate the `user` table automatically. Connection config is hardcoded in `gooq_e2e_test.go` (`127.0.0.1:3307`, `root/root123`).

## If the root module fails to compile

This repository's root module may contain uncommitted work-in-progress files (e.g. `net/ghttp` router cache) that break the build. Since `contrib/drivers/mysql` imports `frame/g` → `ghttp`, the e2e module cannot compile against such a working copy. Workaround: use a clean git worktree:

```bash
git worktree add C:/path/to/gf-gooq-clean -b gooq-e2e-clean feature/gooq
```

then point the `replace` directive in `go.mod` at that path:

```
replace github.com/gogf/gf/v2 => C:/path/to/gf-gooq-clean
```

## What is covered

| Test | Coverage |
| --- | --- |
| TestE2E_CRUD | Insert → Select (Scan to struct) → All (map) → Update → Delete (soft delete, auto-filter) → Unscoped |
| TestE2E_PageCount | Count / Page execution, page cache closed loop (first query hits DB and writes cache, second query hits cache) |
