# 审查问题修复方案（2026-10-19）

- 状态：已按本方案实施。带 build tag 的实库套件仍需有环境再跑。
- 日期：2026-10-19
- 依据：`docs/gooq-项目审查报告.md`，基线 `3207ea8`
- 范围：报告里已复现的 F01–F14。F01 和 F06 明确不改。不改租户、分片和查询 API 的现有约定。

openGauss 复合约束只关联 `conkey[1]` 的问题先不改。有实库后用 `PRIMARY KEY(a,b)` 和 `UNIQUE(c,d)` 核对 `TableFields`，再决定要不要动目录查询。

---

## 总原则

1. 静默写错的路径改成失败关闭，或改成语义明确的谓词。空 `IN` 除外，见下文明确不做。
2. 一种行为只留一份实现。普通删除和批量删除、各条查询出口，不各写一遍。驱动包不在此列：同类规则可以在每个驱动里各写一份，不为去重抽跨驱动的生产 helper 或测试壳。
3. 方言差异用能力接口或该方言自己的渲染方法。写不出的语句在 `ToSql` 失败，不生成执行不了的 SQL。
4. 报告里的复现用例改写成期望正确行为的回归测试。原来的「缺陷确认测试」不能原样留作通过标准。

## 明确不做

### F01 空 IN

`In()` 和 `NotIn()` 的值列表长度为 0 时，继续渲染成空字符串，`WHERE` 丢掉这段条件。`nil`、空切片和不传参数都一样。库不把它改成 `(1 = 0)`、`(1 = 1)` 或报错。列表是空的还是调用方没准备好，由调用点决定。要限制行数时，调用方在长度为 0 时不要执行这条语句。

`README.md` 和 `README.zh_CN.md` 写明这条约定。原来的句子只写了空切片和 `0=1`。

### F06 转换

不改 `Value` 和 `Assign` 的转换行为。gf 的 `ScanOption` 没有严格转换开关，只有 `ContinueOnError`、`OmitEmpty`、`OmitNil`。`ContinueOnError` 只决定结构体、切片或映射里某一项失败后还继不继续。`gconv.Scan` 固定带上 `ContinueOnError: true`。标量转换在 `Converter.Int8` 等函数里把 `Int64` 的结果直接收成目标宽度，溢出不返回错误，也没有选项可开。gooq 不再包一层范围检查。

注释与实现一致，不把转换失败写成一定会返回错误。`Assign`、`Value` 和 `ColumnOf` 只在 `time.Time` 解析失败、`sql.Scanner` 的 `Scan` 失败、以及接口赋值失败时返回错误。其余类型走 `gconv.Scan`，非法文本得到零值，整数溢出被截断，都返回 nil。`Get` 连 `Value` 返回的错误也改成零值。

## 第一批：静默写错

### F02 复合键不齐

`ValuesOf` 只允许 `Insert` 和 `Update`。`Delete`、`InsertFrom` 以及其他语句上调用时，立刻写入 `DMLBuilder.err`，和 `Values` 只允许 `Insert` 一样。不传记录也失败。`ToSql` 和 `Exec` 都返回这个错误，不生成 SQL。删除改用 `Where`。

多行 `Update` 在渲染任何一条语句之前，检查每一条记录都带齐全部主键，或 `Keys(...)` 指定的每一列。缺一列就失败，错误里带上记录序号和缺的列名。一条记录一个键都没有时也失败，不再 `continue` 跳过后面的记录。`Delete` 不再有这条批量路径。

键列出现了但值为 `NULL` 同样失败，不生成 `列 IS NULL`。批量键用来定位记录。复合键里有一列是 `NULL` 时，`IS NULL` 会匹配该列为空的所有行。要按空值匹配时，调用方自己写 `Where`，不走批量键。

键已经齐全、但没有任何非键列的 `UPDATE` 仍保持现在的 `continue`。这条记录没有要写的值，不升成错误。`INSERT` 不查主键齐不齐。

单条 `ValuesOf` 不走这套批量校验，也不从主键生成 `WHERE`。这是已知边界。`Update(testUser).ValuesOf(assignRow{setCol("id", 1)}).ToSql()` 仍是 `UPDATE user SET id = ?`，没有 `WHERE`。一行再写了显式 `Where` 时，键列留在 `SET` 里。现有测试锁住 `SET id = ?, name = ? WHERE (age > ?)`。`Set` 不写 `Where` 同样可以更新全表。F02 不把单条改成按主键生成 `WHERE`，也不因为记录里有键列就报错。要按主键更新一行，调用方自己写 `Where`，或传入至少两行走批量路径。

`README.md`、`README.zh_CN.md` 去掉 `Delete().ValuesOf` 的批量删除示例，改成 `Where`。`dsl/dml.go` 的 `ValuesOf` 注释不再把 `Delete` 算进批量路径。`dsl/dml_test.go`、`dsl/sharding_test.go`，以及 MySQL、MariaDB、TiDB、OceanBase 集成测试里的 `Delete().ValuesOf` 改为期望这个错误。

### F03 事务串库

日常查询继续从 `ctx` 加入事务。`ctx` 里的钥匙是每个连接池自己的 `scope`，不再是组名。手里已经有 `*Tx`、又要让某条 `dsl` 走这个连接时，显式调用 `Tx(tx)`。保存点、`RollbackTo` 和传播行为仍调用 `*Tx` 上的方法。

多个请求可以同时使用同一个 `default` 的 `*DB`。事务写在各自的 `ctx` 里，不写在这个 `*DB` 上。每次 `Transaction` 从连接池另取一条连接，生成自己的 `*Tx`，放进这次回调的子 `ctx`。父 `ctx` 不改。两个请求都从 `context.Background()` 开始，也看不到对方的事务。不要把带事务的 `ctx` 或那个 `*Tx` 交给另一个请求。同一个 `*Tx` 不能在多个 goroutine 里同时执行语句。

事务不再按组名放进 `ctx`。`db.Open` 的组名是空字符串，两次独立 `Open` 会拿到同一个键。已注册的 `orders` 和 `report` 本来就能共存，修复不能把「上下文里已有别的库的事务」当成错误。

每个 `openGroup` 分配一个不导出的 `scope` 指针，存在 `*DB` 上。`*DB` 和包级变量都不保存事务表。`scope` 的类型必须有非零大小，用 `struct{ _ byte }`。不要用 `struct{}`。Go 允许不同的零大小变量地址相同，`new(struct{})` 可能让两次 `Open` 得到同一个指针，事务键又撞在一起。

表是 `ctx` 里一个不导出 key 上的 `scope → *Tx` 快照。快照放进 `ctx` 之后不再修改。`WithTx` 和 `WithoutTx` 先复制快照，只改本 `scope` 的一项，再用 `context.WithValue` 放进子 `ctx`。父 `ctx` 仍是原来的快照。查找不加锁，也不给这张表加 `sync.Mutex`。不使用跨 `ctx` 共享的可变表：那种表要加锁，父 `ctx` 也会看见子调用的删除或覆盖。

提交和回滚不改这张表，也不回写父 `ctx`。外层 `Commit` / `Rollback` 把 `*Tx` 标成已关闭，之后按该 `scope` 查找得到 nil。保存点的提交或回滚不关闭 `*Tx`，也不删除该项。调用方继续用原来的 `ctx`。

- `Master`、`Slave`、`WithRoute` 是结构体拷贝，共用原指针，所以仍加入同一事务。
- `Schema` 的会话语句为空、继续共用连接池时，指针不变。SQLite 走这条路径。
- `Schema` 因会话语句另开连接池时，在返回前给这份拷贝分配新指针。之后的 `Master`、`Slave`、`WithRoute` 拷走的是新指针。新池上的调用不进入原事务。
- 每次 `Open` 和每个已注册配置组各有自己的指针。

`db.WithTx(ctx, tx)` 按 `tx` 自己的 `scope` 写入子 `ctx`。`tx` 或它的库为空时原样返回 `ctx`。它不按组名写入，也不清掉表里其他 `scope`。

同一 `scope` 的嵌套事务和具名保存点继续用同一个 `*Tx`。`(*Tx).Transaction` 只增加保存点层级，再把这个指针放进子 `ctx`。内层回滚到保存点后，外层 `ctx` 里仍是这个未关闭的事务。

`RequiresNew` 在子 `ctx` 的快照里用新的 `*Tx` 覆盖本 `scope`，其他 `scope` 保留。回调返回后子 `ctx` 丢弃，调用方的 `ctx` 仍指向挂起的外层事务，不把新事务写回去。新事务自己提交或回滚。

`NotSupported` 在子 `ctx` 的快照里删除本 `scope`。回调里这个库查不到事务，语句在自己的连接上执行。其他 `scope` 保留。回调返回后，调用方的 `ctx` 仍有原来的事务。

找不到自己的事务时，语句在当前库自己的连接上执行，不报错，也不使用另一个库的 `*sql.Tx`。

`dsl` 默认仍从 `ctx` 加入事务。`SelectBuilder` 和 `DMLBuilder` 增加 `Tx(tx *db.Tx)`，用来指定这条语句的连接。显式事务放在单独字段里，不写入 `executor`。`executor` 仍只给测试注入。

- 没调用 `Tx`，或调用了 `Tx(nil)` 时，显式字段保持为空，这条 `nil` 不生效。`resolveExecutor` 先用 `Instance` 拿到 `*DB`，再调用 `(*DB).TxFromContext`。已经写入的显式事务不会被后来的 `Tx(nil)` 清掉。
- 显式字段非 nil 时，语句走这个 `*Tx`，不再查 `ctx`。`Master`、`Slave` 不再改这条语句的连接。判断看这个字段，不看 `executor` 里是不是 `*db.Tx`。
- 非 nil 但 `DB()` 为 nil 的 `*Tx`，以及同一条语句又调用了 `DB(name)` 且 `name` 和 `tx.DB().Group()` 不同，都是构造错误。`Tx` 和 `DB` 只返回构建器，不能把错误直接交回调用方。`SelectBuilder` 写入 `buildErr`，`DMLBuilder` 写入 `err`。`ToSql` 和执行入口都返回这个错误，SQL 为空，不推迟到只有 `Exec` 才失败。`Tx(nil)` 不写这个错误。`Open` 出来的事务组名是空字符串，不调用 `DB` 就可以和它一起用。
- `Clone` 拷走显式字段，不从 `executor` 推导。
- 保存点、`RollbackTo` 和传播行为仍是 `*Tx` 上的方法，不进 `dsl`。

持有 `*DB` 的查找和清除都改成接收者方法，不能继续把 `d.group` 传给包级函数。包级 `TxFromContext(ctx, "")` 只解析默认组，`Open` 出来的库用它找不到自己的事务。必须改的调用：

- `db/tx.go` 的 `TransactionWithOptions`：第 79 行的查找，以及 `RequiresNew`、`NotSupported` 的 `WithoutTx`（第 97、99 行）。`Required`、`Supports`、`Mandatory`、`Never` 和 `Nested` 共用第 79 行。漏掉这里时，`Open` 出来的库在嵌套保存点时会再开一个事务。
- `db/db.go` 的 `executor`（第 398 行）。
- `db/stmt.go` 的 `Prepare`（第 22 行）。
- `dsl` 的 `resolveExecutor`：显式字段非 nil 时直接用它；字段为空时用上面的接收者方法。不要把 `executor` 转成 `*db.Tx` 当成显式事务。

包级 `TxFromContext(ctx, group)` 和 `WithoutTx(ctx, group)` 只解析已注册并且已经打开的配置组。空组名按 `SetConfig` 的规则表示当前默认组，不再匹配 `Open` 放进去的事务。组还没有实例时，查找返回 nil，清除原样返回 `ctx`。

注释和 README 一起改，不把修复后的行为继续写成「按组」：

- `DB.Transaction`（`db/tx.go` 第 66 行）：同一连接池的后续查询才加入。
- `WithTx`（第 272 行）：按 `tx` 所属连接池的 `scope` 写入。
- `WithoutTx`（第 283 行）：清掉已注册组对应 `scope` 的事务，不是按组名字符串当键。
- `TxFromContext`（第 291 行）：返回该组连接池上仍打开的事务。

另一个已注册组的事务出现在 `ctx` 里时，当前组在自己的连接上执行。这个行为要有默认测试，并写进 `README.md` 和 `README.zh_CN.md` 里「事务从 ctx 读取」的说明：事务只加入开启它的连接池；别的已注册组、另一次 `Open`、以及 `Schema` 另开的池都不使用该事务，也不因此失败。共用池的 `Master`、`Slave`、`WithRoute` 和会话语句为空的 `Schema` 仍加入。多个请求可以共用同一个 `*DB`，事务在各自的 `ctx` 里。不要把带事务的 `ctx` 或 `*Tx` 交给另一个请求，也不要让同一个 `*Tx` 并发执行。显式 `Tx(tx)` 让这一条 `dsl` 走传入的连接。

测试：

- 两次 `Open`：A 的事务回调里对 B 执行写入，写入落在 B；A 回滚后 B 的行还在。
- `WithTx` 把事务放进 `ctx` 后，同一 `scope` 的 `Exec` 加入该事务；另一个库的 `Exec` 不加入。
- 同一 `scope` 的嵌套事务回滚保存点后，外层查找仍得到原来的 `*Tx`，外层写入仍在这个事务上。
- `RequiresNew`：回调里查到的是新事务；回调返回后外层 `ctx` 仍是原来的未关闭事务。内层提交不提交外层。
- `NotSupported`：回调里本 `scope` 查不到事务，写入在自己的连接上并且不依赖外层提交；表里另一个 `scope` 的事务还在；回调返回后外层事务还在。
- SQLite `Schema` 共用池，加入原事务。`Master` 也加入。
- 另开池的 `Schema` 放在对应驱动的实库测试里：新池上的写入不进原事务，原事务回滚不影响新池已经提交的写入。
- 两个已注册组：`orders` 的事务回调里调用 `report`，`report` 在自己的连接上执行且不返回错误。`orders` 回滚不影响 `report` 已提交的写入。
- `Open` 的事务回调里，`dsl` 不调用 `Tx` 时写到 `Instance` 的默认组，不写进这次 `Open`。`.Tx(tx)` 写进这次 `Open`，并随事务回滚。
- `.Tx(nil)` 不设置显式字段，也不写入构造错误，语句仍从 `ctx` 加入事务。`.Tx(tx).DB("另一个组")` 的 `ToSql` 和执行都失败，SQL 为空。`ctx` 里有别的库的事务时，`.Tx(tx)` 仍用传入的这个事务。
- 两个 goroutine 同时对同一个文件库上的 `default` 组调用 `Transaction`，都从 `context.Background()` 进入。回调里只断言各自的 `ctx` 拿到自己的 `*Tx`，两个指针不同。不要用 `:memory:`，SQLite 里每个连接是独立库。不要让两个事务同时写入：`drivers/sqlite/sqlite.go` 的 DSN 没有 `busy_timeout`，重叠写入会得到 `SQLITE_BUSY`。写入安排在不重叠的时点，一方结束后另一方再写，并断言回滚只影响自己的行。

### F04 事务缓存

执行器是 `*db.Tx` 时，不读也不写共享缓存。判断放在实际读写缓存的函数上，不在每个公开方法上各写一次。先不做事务私有缓存。

`.Cache` 有三条读写入口：

- `cellWithCache`：`Value`、`Column` 经 `readCell` 进入；`Count`、`Exists` 经 `queryWithCache` 进入。`queryWithCache` 只服务 `Count` 和 `Exists`。事务判断只写在 `queryWithCache` 时，事务里的 `Value`、`Column` 仍读写共享缓存。
- `rowWithCache`：`Row`。
- `rowsWithCache`：`Rows`。

`.PageCache` 有两条入口：

- `rowsAndCountWithCache`：`RowsAndCount`。`StructsAndCount` 先调用 `RowsAndCount`，不再单独走缓存。
- `columnAndCountWithCache`：`ColumnAndCount`。它不经过 `RowsAndCount`。

### 第一批验收

- `Delete().ValuesOf` 在 `ToSql` 和 `Exec` 都失败，不生成 SQL。缺主键或键值为 `NULL` 的多行 `Update` 在执行前报错，数据库里的行不变。SQL 里不出现键列的 `IS NULL`。多条记录里的坏记录不会被跳过。只有键列、没有非键列的 `UPDATE` 不在此列。单条 `ValuesOf` 也不在此列，没有 `Where` 时仍是 `SET` 主键、不带 `WHERE`。
- 两个独立 `Open` 的库不会互相写入。`WithTx`、嵌套保存点、`RequiresNew` 和 `NotSupported` 只改本 `scope` 的子 `ctx`，返回后外层事务仍在。`Master` 和共用池的 `Schema` 仍加入原事务。另开池的 `Schema` 不加入。另一个已注册组上的调用在自己的连接上执行，不因上下文里已有事务而失败。多个请求同时使用同一个 `default` 组时，各自只看见自己的事务。`dsl` 不调用 `Tx` 时仍从 `ctx` 按 `scope` 加入；调用后走传入的 `*Tx`。README 写明这条范围。
- 回滚后，`Count`、`Exists`、`Row`、`Rows`、`Value`、`Column`、`RowsAndCount`、`StructsAndCount` 和 `ColumnAndCount` 的缓存查询仍是提交前的值。

## 第二批：结果和产物

### F05 软删赋值

普通删除和批量删除共用一个取值函数，类型清单与 `softDeleteSQL` 相同。

- `LocalTypeDate`、`LocalTypeTime`、`LocalTypeDatetime` 绑定当前时间。读取侧仍是 `IS NULL`。
- `LocalTypeInt`、`LocalTypeUint`、`LocalTypeInt32`、`LocalTypeUint32`、`LocalTypeInt64`、`LocalTypeUint64`、`LocalTypeBigInt` 绑定 `1`。读取侧仍是 `= 0`。
- `LocalTypeBool` 绑定 `true`。读取侧渲染 `= false`，不再渲染 `= 0`。`db/catalog.go` 把 `bool` 和 `boolean` 映射成 `LocalTypeBool`。PostgreSQL 上 `boolean = 0` 非法，`boolean = false` 合法。不按方言再分叉：整数类型仍是 `= 0`，布尔类型用布尔字面量。只改写入侧、读取侧留着 `= 0` 不行。

`LocalTypeInt64Bytes` 和 `LocalTypeUint64Bytes` 不在读取侧清单里，写入侧同样不接受。其余类型继续报错，不生成 SQL。

### F07 生成方法重名

冲突集合从即将写出的声明收集，不维护一份会漏项的保留字表。同一类型上的名字重复就拒绝生成，错误里写出原列名和冲突的声明。

放弃现在的重命名兜底是有意的。`goFieldName` 遇到保留名就加 `Field` 后缀，列 `meta` 现在生成 `MetaField`。修复后不再生成 `MetaField`，生成失败并写出原列名和冲突声明。`README.md` 和 `README.zh_CN.md` 里「同名加 Field 后缀」的句子一起改掉。无效 Go 标识符、两个列生成同一个名字，仍然报错。

`*XxxRecord` 纳入：每个列字段、每个 `SetXxx`、每个可空列的 `SetXxxNull`、`Assignments`、`ToModel`。

`*XxxTable` 始终纳入：每个列字段，模板始终生成的 `As`、`Clone`，以及嵌入 `*TableBase` 后提升的 `TableName`、`AliasName`、`Meta`、`AllColumns`、`AllFields`、`Field`、`SetShard`、`CopyShard`、`TableBase`、`BoundBase`。`SetShard` 和 `CopyShard` 在 `gooq_table.go`，嵌入后总是提升，不分片表也算。

`Physical` 和 `ShardBy` 只在 `spec.Sharding` 为真时生成，只在那时纳入。普通表没有这两个方法，无条件纳入会让合法列 `physical` 或 `shard_by` 被拒绝。

字段遮蔽提升方法在 Go 1.24 能编译：字段 `Foo` 和嵌入类型的 `Foo()` 可以同时存在。`BoundBase`、`Meta` 这类冲突 `go build` 抓不到，必须由生成器显式拒绝。

冲突校验放在返回 `error` 的函数里，测试直接调用并断言错误文本。不把校验只放在 `mlog.Fatalf` 里，进程内测试看不到进程退出。也可以像现有 `buildBinary` 那样跑子进程并断言退出码。临时模块的 `go build` 只覆盖成功产物，不作为拒绝路径的通过标准。成功产物仍然要能 `go build`。只检查文件里有没有某段字符串，不算通过。

### F08 文件名 sharding.go

分片注册器固定写成 `table/gooq_sharding.go`。任何一个逻辑表经 `formatFileName` 得到的文件名是 `gooq_sharding.go` 时，这次生成失败，错误里写出该逻辑表名。不改用另一个未检查的名字。

没有分片表时，只删除 `gooq_sharding.go`。不删除同目录下的表文件。逻辑表名是 `sharding` 时，`sharding.go` 必须留下。

### F13 组内排序参数

SQLite 仍忽略 `GroupConcat` 的 `ORDER BY`，不改成报错。判断使用 `fn` 里的小接口 `groupConcatOrderRenderer`，方法是 `GroupConcatOrder() bool`。它不放进 `DialectSpec`。

MySQL 家族、PostgreSQL、GaussDB 和达梦实现该接口并返回 `true`。SQLite 不实现。`RenderWith` 先做类型断言。接口不存在或返回 `false` 时，不调用排序表达式的 `rc.Render`，传给 `RenderGroupConcat` 的 `OrderSQL` 为空。不能先渲染再把 SQL 丢掉，否则排序里的参数会留在后面的 `WHERE` 里。

测试用一个带占位符的 `OrderBy`，再接一个带另一个占位符的 `WHERE`。SQLite 的 `ToSql` 参数列表里没有排序表达式的参数，`GROUP_CONCAT` 里也没有 `ORDER BY`。MySQL 的参数列表仍包含排序参数，并且排在 `WHERE` 参数之前。

## 第三批：方言和构造错误

### F09、F10 连接地址

MySQL、MariaDB、TiDB、OceanBase 各自修改自己的 `DSN`、端点函数和 `ApplySession`。不抽一份给四个驱动调用的 DSN 组装函数，也不共享测试壳。四个包的规则相同，实现各自保留。

查询参数用 `url.Values` 收集后再 `Encode`，不再把时区拼进字符串。

`Config.Timezone` 和链接 `extra` 里的 `loc` 同时存在时，`extra` 的 `loc` 优先，`Timezone` 不覆盖它。判断用 `url.Values.Has("loc")`，不用 `Get("loc") == ""`。显式写出的空 `loc` 也算存在，不能被 `Timezone` 补上。只有 extra 没有 `loc` 这个键时，才 `Set("loc", Timezone)`。两者都空则不加 `loc`。

`net.JoinHostPort` 只用于有端口的 `tcp`、`tcp4`、`tcp6`。没有端口时不要调用它：`JoinHostPort("::1", "")` 得到 `[::1]:`，MySQL 驱动的 `ensureHavePort` 看到已经能拆出端口就不再补 `3306`。无端口的 IPv6 保持 `tcp(::1)`，驱动会补成 `[::1]:3306`。有端口时用 `JoinHostPort`，例如 `tcp([::1]:3307)`。

`unix` 的路径放在 host 里，原样放进 `unix(path)`，不调用 `JoinHostPort`。`unix` 还带了端口则 DSN 生成失败。`file` 继续走现有的文件路径解析，不进这套主机拼接。

每个驱动包自己把生成的 DSN 交给底层驱动解析。TCP 用拨号回调核对两种 IPv6：有端口时期望 `[::1]:3307`，无端口时期望 `[::1]:3306`，并且 DSN 里仍是 `tcp(::1)` 而不是 `tcp([::1]:)`。`unix` 用一条不带端口的路径断言，不再调用 `JoinHostPort`。

### F11 构造错误

`renderSelect` 开头检查本构建器的 `buildErr`，有错误就 `rc.Fail` 并返回空字符串。顶层 `ToSql` 返回这个错误。各入口都进 `renderSelect`，传播方式如下。

| 入口 | 调用 | 错误如何回到顶层 |
| --- | --- | --- |
| 派生表 | `renderTableName` 调用子构建器的 `renderSelect` | 外层 `ToSql` 读取 `rc.Err()` |
| `WHERE` 子查询 | 条件渲染调用子构建器的 `Render`，再进 `renderSelect` | 同上 |
| `IN` / `EXISTS` 子查询 | 同上，走 `Render` | 同上 |
| CTE | 外层 `renderSelect` 调用 CTE 查询的 `renderSelect` | 同上 |
| 集合运算 | 外层 `renderSelect` 调用分支的 `renderSelect` | 同上 |
| `INSERT ... SELECT` | `renderInsertFrom` 调用选择构建器的 `renderSelect` | `DMLBuilder.ToSql` 读取 `rc.Err()` |

矩阵测试用 `Limit(-1)` 作为构造错误，六行都要断言 `ToSql` 返回该错误且 SQL 为空：派生表、`Where` 里的 `InExpr`、`Where` 里的 `Exists`、`With`、`Union`、`Insert.Select`。嵌套两层时，最内层的 `buildErr` 也要到达顶层。

### F12 SQLite 仅 OFFSET

只有 SQLite 在有 `OFFSET`、没有 `LIMIT` 时写成 `LIMIT -1 OFFSET n`。PostgreSQL 和 GaussDB 保持现在的 `OFFSET n`。README 里的 SQLite 示例改成可执行的形状，并加一条实际执行的测试。

### F14 SQLite 函数

`fn.Now()` 使用 `fn` 里的小接口 `nowRenderer`，方法是 `RenderNow() string`。不放进 `DialectSpec`。没实现该接口的方言在 `ToSql` 失败，不输出 `NOW()`。

各实现的语义写在方法注释里，不把它们说成同一种时间：

- MySQL 家族渲染 `NOW()`，值是会话时区的日期时间。
- PostgreSQL 和 GaussDB 渲染 `NOW()`，值是事务开始时的带时区时间。
- 达梦渲染 `SYSDATE`，值是数据库服务器的本地时间。
- SQLite 渲染 `CURRENT_TIMESTAMP`。它是 UTC 的 `YYYY-MM-DD HH:MM:SS` 文本，没有时区，也不是 MySQL 的会话时间。

`MEDIAN` 继续用现有的 `medianRenderer`。SQLite 去掉 `RenderMedian`，不再实现该接口。`dialectSupport` 在渲染 `fn.Median` 时 `rc.Fail`。`ToSql` 返回这个错误，SQL 为空。不把失败留到执行阶段，也不输出 `MEDIAN(...)`。MySQL 家族、PostgreSQL、GaussDB 和达梦仍实现该接口。

测试分两条。SQLite 实际执行 `fn.Now()`，SQL 是 `CURRENT_TIMESTAMP`，查询成功并返回值。SQLite 的 `fn.Median` 在 `ToSql` 就失败，错误非空且 SQL 为空，不进入执行。

## 实施顺序

1. 第一批 F01–F04，各自补回归测试。
2. 第二批 F05、F07、F08、F13。
3. 第三批 F09–F12、F14。

每一步独立可回滚。提交前根模块和 `cmd/gooq-gen` 都要 `go test ./...` 与 `go vet ./...` 通过。带 build tag 的实库套件不在默认测试里，有环境时再跑。
