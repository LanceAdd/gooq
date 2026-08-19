# gooq 端到端测试

验证完整链路的冒烟测试：gooq 构建 SQL → 标准库 database/sql 对真实 MySQL 执行。

## 前提

- Docker（或本地 3307 端口的 MySQL 8）
- 干净的仓库工作副本（无破坏编译的未提交文件）

## 运行

```bash
# 1. 启动 MySQL（端口 3307，数据库 gooq_test）
docker run -d --name gooq-mysql \
  -e MYSQL_ROOT_PASSWORD=root123 \
  -e MYSQL_DATABASE=gooq_test \
  -p 3307:3306 mysql:8

# 2. 运行测试
cd database/gooq/e2e
go test ./...
```

测试自动创建并清空 `user` 表。连接配置硬编码在 `gooq_e2e_test.go`（`127.0.0.1:3307`，`root/root123`）。

## 根模块编译失败时

根模块可能包含未提交的进行中文件（如 `net/ghttp` 路由缓存）导致构建失败。由于 `contrib/drivers/mysql` 会 import `frame/g` → `ghttp`，e2e 模块无法针对这种工作副本编译。解决办法：使用干净的 git worktree：

```bash
git worktree add C:/path/to/gf-gooq-clean -b gooq-e2e-clean feature/gooq
```

然后将 `go.mod` 中的 `replace` 指向该路径：

```
replace github.com/gogf/gf/v2 => C:/path/to/gf-gooq-clean
```

## 覆盖内容

| 测试 | 覆盖 |
| --- | --- |
| TestE2E_CRUD | Insert → Select（Scan 到结构体）→ All（map）→ Update → Delete（软删除，自动过滤）→ Unscoped |
| TestE2E_PageCount | Count / Page 执行，分页缓存闭环（首次查库写缓存，二次命中缓存） |
