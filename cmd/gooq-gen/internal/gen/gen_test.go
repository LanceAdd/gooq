// 本文件为 gooq-gen 的端到端测试：sqlite 建库 → Generate/init 子命令 → 产物断言。
package gen

import (
	"context"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/test/gtest"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
	_ "github.com/lanceadd/gooq/drivers/sqlite"
)

// initSqlite 创建 sqlite 测试库并建表（SetConfigGroup 整组替换，可跨测试重复运行）。
func initSqlite(t *gtest.T, dbPath string) {
	database, err := db.Open(db.Config{Link: "sqlite::@file(" + dbPath + ")"})
	t.AssertNil(err)
	defer func() {
		t.AssertNil(database.Close())
	}()
	_, err = database.Exec(context.Background(), `
CREATE TABLE IF NOT EXISTS user (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    age        INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    alias      TEXT,
    deleted_at DATETIME,
    created_at DATETIME
)`)
	t.AssertNil(err)
}

// goTool 返回 go 可执行文件路径（优先 PATH，回退 GOROOT/bin）。
func goTool() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(runtime.GOROOT(), "bin", "go.exe")
	}
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

var (
	binOnce sync.Once
	binPath string
)

// buildBinary 编译一次 gooq-gen 二进制（跨测试复用，位于独立临时目录避免被用例清理）。
func buildBinary(t *gtest.T) string {
	binOnce.Do(func() {
		// 模块根目录经文件位置推导（不受工作目录影响）。
		_, file, _, _ := runtime.Caller(0)
		moduleDir := filepath.Join(filepath.Dir(file), "..", "..")
		path := filepath.Join(gfile.Temp("gooq-gen-bin"), "gooq-gen.exe")
		if err := gfile.Mkdir(gfile.Dir(path)); err != nil {
			panic(err)
		}
		cmd := exec.Command(goTool(), "build", "-o", path, ".")
		cmd.Dir = moduleDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			panic("build gooq-gen failed: " + err.Error() + "\n" + string(out))
		}
		binPath = path
	})
	return binPath
}

// TestGen_Sqlite 验证 sqlite 建库 → Generate → 生成 gooq table 产物 → 产物内容断言。
func TestGen_Sqlite(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			ctx     = context.Background()
			tmpDir  = gfile.Temp("gooq-gen-test", gtime.TimestampNanoStr())
			dbPath  = filepath.Join(tmpDir, "gooq_test.db")
			genPath = filepath.Join(tmpDir, "gen")
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(genPath))
		initSqlite(t, dbPath)

		Generate(ctx, Input{
			Link: "sqlite::@file(" + dbPath + ")",
			Path: genPath,
		})

		// gooq table 产物。
		generated := filepath.Join(genPath, "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		content := gfile.GetContents(generated)

		// 类型化表对象骨架。
		t.Assert(gstr.Contains(content, `type UserTable struct {`), true)
		t.Assert(gstr.Contains(content, `*gooq.TableBase`), true)
		// 全列由 TableBase.AllFields() 从元数据派生，生成代码不重复维护列名列表。
		t.Assert(gstr.Contains(content, `ALL_FIELDS`), false)

		// 字段定义（Go 类型映射：sqlite INTEGER → int，datetime → time.Time）。
		t.Assert(gstr.Contains(content, `gooq.Field[int]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[string]`), true)
		t.Assert(gstr.Contains(content, `gooq.Field[time.Time]`), true)

		// 全局表对象与元数据（软删标记由列名约定识别，与驱动无关）。
		t.Assert(gstr.Contains(content, `var User = newUserTable(UserMeta, "")`), true)
		// 没有 hack/gooq.tables.yaml 时不生成任何分片件。
		t.Assert(gstr.Contains(content, "Sharding"), false)
		t.Assert(gstr.Contains(content, `gooq.TableMeta{`), true)
		t.Assert(gstr.Contains(content, `TableName: "user"`), true)
		t.Assert(gstr.Contains(content, `LocalType: gooq.LocalTypeInt`), true)
		t.Assert(gstr.Contains(content, `SoftDelete: true`), true)

		// NewFieldAt 赋值与 As/Clone。
		t.Assert(gstr.Contains(content, `t.Id = gooq.NewFieldAt[int](t.TableBase, "id")`), true)
		t.Assert(gstr.Contains(content, `t.Alias = gooq.NewFieldAt[string](t.TableBase, "alias")`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) As(alias string) *UserTable {`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) Clone() *UserTable {`), true)

		// FieldMeta 含软删列。
		t.Assert(gstr.Contains(content, `"deleted_at"`), true)

		// 生成的文件头标识。
		t.Assert(gstr.Contains(content, `Code generated and maintained by gooq-gen. DO NOT EDIT.`), true)

		// 普通模型带 json/orm 标签。写入记录单独成文件，可空列提供 SetXxxNull。
		modelFile := filepath.Join(genPath, "model", "user_model.go")
		recordFile := filepath.Join(genPath, "model", "user_record.go")
		t.Assert(gfile.Exists(modelFile), true)
		t.Assert(gfile.Exists(recordFile), true)
		t.Assert(gfile.Exists(filepath.Join(genPath, "model", "user.go")), false)
		modelContent := gfile.GetContents(modelFile)
		t.Assert(gstr.Contains(modelContent, `type UserModel struct {`), true)
		t.Assert(gstr.Contains(modelContent, "`json:\"name\" orm:\"name\"`"), true)
		t.Assert(gstr.Contains(modelContent, "`json:\"alias\" orm:\"alias\"`"), true)
		t.Assert(gstr.Contains(modelContent, "`json:\"deletedAt\" orm:\"deleted_at\"`"), true)
		t.Assert(gstr.Contains(modelContent, "*string"), true)
		t.Assert(gstr.Contains(modelContent, "gooq.Property"), false)
		recordContent := gfile.GetContents(recordFile)
		t.Assert(gstr.Contains(recordContent, `type UserRecord struct {`), true)
		t.Assert(gstr.Contains(recordContent, "gooq.Property[string]"), true)
		t.Assert(gstr.Contains(recordContent, `func NewUserRecord() *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) SetName(v string) *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) SetAge(v int) *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) SetAliasNull() *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) SetDeletedAtNull() *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) SetNameNull()`), false)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) Assignments() []gooq.Assignment`), true)
		t.Assert(gstr.Contains(recordContent, `func (r *UserRecord) ToModel() UserModel`), true)
		t.Assert(gstr.Contains(recordContent, `func NewUserRecordFrom(row UserModel) *UserRecord`), true)
		t.Assert(gstr.Contains(recordContent, `r.Alias = gooq.Null[string]()`), true)
		t.Assert(gstr.Contains(recordContent, `row.Alias = &value`), true)

		// 不再生成 do/entity 产物。
		t.Assert(gfile.Exists(filepath.Join(genPath, "do")), false)
		t.Assert(gfile.Exists(filepath.Join(genPath, "entity")), false)
	})
}

// initShardedSqlite 创建 sqlite 分片测试库：结构相同的 user_00 / user_01。
func initShardedSqlite(t *gtest.T, dbPath string) {
	database, err := db.Open(db.Config{Link: "sqlite::@file(" + dbPath + ")"})
	t.AssertNil(err)
	defer func() {
		t.AssertNil(database.Close())
	}()
	for _, name := range []string{"user_00", "user_01"} {
		_, err = database.Exec(context.Background(), `
CREATE TABLE IF NOT EXISTS `+name+` (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    age        INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'active',
    alias      TEXT,
    deleted_at DATETIME,
    created_at DATETIME
)`)
		t.AssertNil(err)
	}
}

// TestGen_Sharding_Sqlite 验证分组文件：分片表只生成一个逻辑表对象，带注册函数和 Physical、ShardBy。
func TestGen_Sharding_Sqlite(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			ctx     = context.Background()
			tmpDir  = gfile.Temp("gooq-gen-shard-test", gtime.TimestampNanoStr())
			dbPath  = filepath.Join(tmpDir, "gooq_shard.db")
			genPath = filepath.Join(tmpDir, "gen")
			yaml    = filepath.Join(tmpDir, "gooq.tables.yaml")
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(genPath))
		initShardedSqlite(t, dbPath)
		t.AssertNil(gfile.PutContents(yaml, "# 单一表\nsingle: []\n# 分表\ngroups:\n  - name: user\n    table:\n      - user_00\n      - user_01\n"))

		Generate(ctx, Input{
			Link:       "sqlite::@file(" + dbPath + ")",
			Path:       genPath,
			TablesFile: yaml,
		})

		generated := filepath.Join(genPath, "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		t.Assert(gfile.Exists(filepath.Join(genPath, "table", "user_00.go")), false)
		t.Assert(gfile.Exists(filepath.Join(genPath, "table", "user_01.go")), false)
		content := gfile.GetContents(generated)
		t.Assert(gstr.Contains(content, `func RegisterUserSharding(rule gooq.ShardingRule) {`), true)
		t.Assert(gstr.Contains(content, `UserMeta.Sharding = rule`), true)
		t.Assert(gstr.Contains(content, `TableName: "user"`), true)
		t.Assert(gstr.Contains(content, `ShardNames: []string{`), true)
		t.Assert(gstr.Contains(content, `"user_00",`), true)
		t.Assert(gstr.Contains(content, `"user_01",`), true)
		t.Assert(gstr.Contains(content, "NewShardingSlot"), false)
		t.Assert(gstr.Contains(content, `func (t *UserTable) Physical(name string) *UserTable {`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) ShardBy(value any) *UserTable {`), true)
		register := gfile.GetContents(filepath.Join(genPath, "table", "gooq_sharding.go"))
		t.Assert(gstr.Contains(register, `func RegisterSharding(rule gooq.ShardingRule) {`), true)
		t.Assert(gstr.Contains(register, `UserMeta.Sharding = rule`), true)
		t.Assert(gstr.Contains(content, `func (t *UserTable) Named(name string) *UserTable {`), false)
		t.Assert(gstr.Contains(content, `type UserTable struct {`), true)

		// 模型同样只生成逻辑表。
		t.Assert(gfile.Exists(filepath.Join(genPath, "model", "user_model.go")), true)
		t.Assert(gfile.Exists(filepath.Join(genPath, "model", "user_00_model.go")), false)
	})
}

func TestGen_ConfigTablesCheckIgnore(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			tmpDir  = gfile.Temp("gooq-gen-check-ignore", gtime.TimestampNanoStr())
			dbPath  = filepath.Join(tmpDir, "gooq_test.db")
			workDir = filepath.Join(tmpDir, "work")
			bin     = buildBinary(t)
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(workDir))
		t.AssertNil(gfile.Mkdir(filepath.Join(workDir, "hack")))
		t.AssertNil(gfile.Mkdir(filepath.Join(workDir, "out")))

		database, err := db.Open(db.Config{Link: "sqlite::@file(" + dbPath + ")"})
		t.AssertNil(err)
		for _, ddl := range []string{
			`CREATE TABLE user_1 (id INTEGER PRIMARY KEY, name TEXT)`,
			`CREATE TABLE user_2 (id INTEGER PRIMARY KEY, name TEXT)`,
		} {
			_, err = database.Exec(context.Background(), ddl)
			t.AssertNil(err)
		}
		t.AssertNil(database.Close())

		cfg := "gooq:\n  gen:\n" +
			"    link: \"sqlite::@file(" + filepath.ToSlash(dbPath) + ")\"\n" +
			"    path: out\n" +
			"  tables:\n    check:\n      ignore:\n        - \"user_*\"\n"
		t.AssertNil(gfile.PutContents(filepath.Join(workDir, "hack", "gooq.yaml"), cfg))
		t.AssertNil(gfile.PutContents(
			filepath.Join(workDir, "hack", "gooq.tables.yaml"),
			"single: []\ngroups:\n  - name: user\n    table:\n      - user_1\n",
		))

		cmd := exec.Command(bin)
		cmd.Dir = workDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run with dynamic shard failed: %v\n%s", err, out)
		}
		t.Assert(gfile.Exists(filepath.Join(workDir, "out", "table", "user.go")), true)
		t.Assert(gfile.Exists(filepath.Join(workDir, "out", "table", "user_2.go")), false)
	})
}

func TestCheckTables(t *testing.T) {
	cfg := tablesConfig{
		Single: []string{"role", "user_00"},
		Groups: []tableGroup{
			{Name: "user", Table: []string{"user_00", "user_01", "user_01"}},
			{Name: "user", Table: []string{}},
			{Name: "role", Table: []string{"role_00"}},
			{Name: "order", Table: []string{"user_001"}},
		},
	}
	report := checkTables(cfg, []string{"role", "user_00", "user_01", "order_00"}, TablesCheckConfig{})
	text := report.String()
	for _, want := range []string{
		"user_00  在 single，也在 user",
		"user_01  在 user 里写了两次",
		"order_00",
		"role  组名与 single 里的表同名",
		"user  有两个组都叫这个名字",
		"user  没有物理表",
		"user_001  不包含组名 order",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	diff := diffTables(cfg, []string{"role", "user_00", "user_01", "order_00"}, TablesCheckConfig{})
	if !reflect.DeepEqual(diff.New, []string{"order_00"}) {
		t.Fatalf("new %v", diff.New)
	}
	if !reflect.DeepEqual(diff.Stale, []string{"role_00", "user_001"}) {
		t.Fatalf("stale %v", diff.Stale)
	}
}

func TestTablesCheckIgnore(t *testing.T) {
	cfg := tablesConfig{
		Groups: []tableGroup{
			{Name: "employee", Table: []string{"employee_1", "employee_2"}},
		},
	}
	check := TablesCheckConfig{Ignore: []string{"employee_*"}}

	report := checkTables(cfg, []string{"employee_1", "employee_2", "employee_3"}, check)
	if !report.empty() {
		t.Fatalf("dynamic shard should be ignored:\n%s", report.String())
	}

	report = checkTables(cfg, []string{"employee_1", "employee_2", "employee_3", "order_1"}, check)
	if !reflect.DeepEqual(report.Missing, []string{"order_1"}) {
		t.Fatalf("missing %v", report.Missing)
	}

	// ignore 不能掩盖文件里列出的表不存在。
	report = checkTables(cfg, []string{"employee_1"}, check)
	if !reflect.DeepEqual(report.Extra, []string{"employee_2"}) {
		t.Fatalf("extra %v", report.Extra)
	}

	diff := diffTables(cfg, []string{"employee_1", "employee_2", "employee_3"}, check)
	if len(diff.New) != 0 {
		t.Fatalf("new %v", diff.New)
	}
	diff = diffTables(cfg, []string{"employee_1"}, check)
	if !reflect.DeepEqual(diff.Stale, []string{"employee_2"}) {
		t.Fatalf("stale %v", diff.Stale)
	}
}

func TestParseTablesCheckConfig(t *testing.T) {
	cfg, err := ParseTablesCheckConfig(map[string]any{
		"ignore": []any{" employee_* ", "archive_?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Ignore, []string{"employee_*", "archive_?"}) {
		t.Fatalf("ignore %v", cfg.Ignore)
	}
	if !cfg.Ignored("employee_3") || cfg.Ignored("staff_3") {
		t.Fatalf("match %v", cfg.Ignore)
	}

	for name, raw := range map[string]any{
		"not map":       "employee_*",
		"unknown field": map[string]any{"unknown": true},
		"not list":      map[string]any{"ignore": "employee_*"},
		"not string":    map[string]any{"ignore": []any{1}},
		"empty":         map[string]any{"ignore": []any{" "}},
		"bad pattern":   map[string]any{"ignore": []any{"["}},
	} {
		if _, err := ParseTablesCheckConfig(raw); err == nil {
			t.Fatalf("%s: expected error", name)
		}
	}

	cfg, err = ParseTablesCheckConfig(nil)
	if err != nil || len(cfg.Ignore) != 0 {
		t.Fatalf("nil: %v %v", cfg, err)
	}
	cfg, err = ParseTablesCheckConfig(map[string]any{})
	if err != nil || len(cfg.Ignore) != 0 {
		t.Fatalf("empty map: %v %v", cfg, err)
	}
}

func TestValidateShardSchemas(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			ctx    = context.Background()
			tmpDir = gfile.Temp("gooq-gen-shard-check", gtime.TimestampNanoStr())
			dbPath = filepath.Join(tmpDir, "s.db")
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(tmpDir))
		database, err := db.Open(db.Config{Link: "sqlite::@file(" + dbPath + ")"})
		t.AssertNil(err)
		defer func() { t.AssertNil(database.Close()) }()
		_, err = database.Exec(ctx, `CREATE TABLE user_00 (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)`)
		t.AssertNil(err)
		_, err = database.Exec(ctx, `CREATE TABLE user_01 (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, extra TEXT)`)
		t.AssertNil(err)
		_, err = database.Exec(ctx, `CREATE TABLE user_02 (id INTEGER, name TEXT NOT NULL)`)
		t.AssertNil(err)
		_, err = database.Exec(ctx, `CREATE TABLE user_03 (id INTEGER, name TEXT)`)
		t.AssertNil(err)

		ok := tableSpec{Physical: "user_00", Logical: "user", Sharding: true, Shards: []string{"user_00"}}
		t.AssertNil(validateShardSchemas(ctx, database, []tableSpec{ok}))

		bad := tableSpec{Physical: "user_00", Logical: "user", Sharding: true, Shards: []string{"user_00", "user_01"}}
		err = validateShardSchemas(ctx, database, []tableSpec{bad})
		t.Assert(gstr.Contains(err.Error(), "column count"), true)

		nullable := tableSpec{Physical: "user_02", Logical: "user", Sharding: true, Shards: []string{"user_02", "user_03"}}
		err = validateShardSchemas(ctx, database, []tableSpec{nullable})
		t.Assert(gstr.Contains(err.Error(), `column "name" nullable`), true)
	})
}

func TestGenerateSpecialTypes(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		database, err := db.Open(db.Config{Link: "sqlite::@file(:memory:)"})
		t.AssertNil(err)
		defer func() { t.AssertNil(database.Close()) }()
		columns := []db.Column{
			{Name: "amount", LocalType: gooq.LocalTypeDecimal},
			{Name: "tags", LocalType: gooq.LocalTypeStringSlice},
			{Name: "ids", LocalType: gooq.LocalTypeUUIDSlice},
			{Name: "span", LocalType: gooq.LocalTypeString},
			{Name: "email", LocalType: gooq.LocalTypeString, Unique: true},
		}
		content, err := generateTableContent(context.Background(), database, tableSpec{Physical: "sample", Logical: "sample"}, "table", columns, tenantSettings{})
		t.AssertNil(err)
		t.Assert(gstr.Contains(content, "gooq.Field[string]"), true)
		t.Assert(gstr.Contains(content, `LocalType: gooq.LocalTypeDecimal`), true)
		t.Assert(gstr.Contains(content, "gooq.Field[[]string]"), true)
		t.Assert(gstr.Contains(content, "gooq.Field[[]uuid.UUID]"), true)
		t.Assert(gstr.Contains(content, `"github.com/google/uuid"`), true)
		t.Assert(gstr.Contains(content, "Unique: true"), true)
		t.Assert(gstr.Contains(content, "gooq.Field[float64]"), false)

		modelContent, recordContent, err := generateModelContent(context.Background(), "sample", "model", columns)
		t.AssertNil(err)
		t.Assert(gstr.Contains(modelContent, `"github.com/google/uuid"`), true)
		t.Assert(gstr.Contains(recordContent, `"github.com/google/uuid"`), true)
		t.Assert(gstr.Contains(recordContent, "[]uuid.UUID"), true)
		t.Assert(gstr.Contains(recordContent, "gooq.Property[string]"), true)
	})
}

// TestInit 验证 init 子命令：生成 hack/gooq.yaml 与 table/model/record 三个模板，已存在文件不覆盖。
func TestInit(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			tmpDir = gfile.Temp("gooq-gen-init-test", gtime.TimestampNanoStr())
			bin    = buildBinary(t)
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(tmpDir))

		run := func(args ...string) ([]byte, error) {
			cmd := exec.Command(bin, args...)
			cmd.Dir = tmpDir
			return cmd.CombinedOutput()
		}

		cfgPath := filepath.Join(tmpDir, "hack", "gooq.yaml")
		tplPath := filepath.Join(tmpDir, "hack", "template", "table.tmpl")
		modelPath := filepath.Join(tmpDir, "hack", "template", "model.tmpl")
		recordPath := filepath.Join(tmpDir, "hack", "template", "record.tmpl")

		// 首次 init：生成配置与三个模板。
		out, err := run("init")
		if err != nil {
			t.Fatalf("init failed: %v\n%s", err, out)
		}
		t.Assert(gfile.Exists(cfgPath), true)
		t.Assert(gfile.Exists(tplPath), true)
		t.Assert(gfile.Exists(modelPath), true)
		t.Assert(gfile.Exists(recordPath), true)
		cfgContent := gfile.GetContents(cfgPath)
		t.Assert(gstr.Contains(cfgContent, "gooq:"), true)
		t.Assert(gstr.Contains(cfgContent, "gen:"), true)
		t.Assert(gstr.Contains(cfgContent, "path: internal"), true)
		t.Assert(gstr.Contains(cfgContent, "ignore:"), true)
		t.Assert(gstr.Contains(gfile.GetContents(tplPath), "TableBase"), true)

		// 二次 init：已存在文件不覆盖。
		t.AssertNil(gfile.PutContents(cfgPath, "# CUSTOM_CONFIG\n"))
		out, err = run("init")
		if err != nil {
			t.Fatalf("second init failed: %v\n%s", err, out)
		}
		t.Assert(gfile.GetContents(cfgPath), "# CUSTOM_CONFIG\n")
		t.Assert(gstr.Contains(string(out), "skipped"), true)
	})
}

func TestFieldNames(t *testing.T) {
	used := map[string]string{}
	_, err := goFieldName("bound_base", tableConflicts(false), used)
	if err == nil || !strings.Contains(err.Error(), "bound_base") || !strings.Contains(err.Error(), "BoundBase") {
		t.Fatalf("bound_base err=%v", err)
	}
	if _, err = goFieldName("meta", tableConflicts(false), map[string]string{}); err == nil || !strings.Contains(err.Error(), "Meta") {
		t.Fatalf("meta err=%v", err)
	}
	if _, err = goFieldName("set_shard", tableConflicts(false), map[string]string{}); err == nil || !strings.Contains(err.Error(), "SetShard") {
		t.Fatalf("set_shard err=%v", err)
	}
	physical, err := goFieldName("physical", tableConflicts(false), map[string]string{})
	if err != nil || physical != "Physical" {
		t.Fatalf("physical name=%s err=%v", physical, err)
	}
	if _, err = goFieldName("physical", tableConflicts(true), map[string]string{}); err == nil {
		t.Fatal("sharded table accepted Physical")
	}
	if _, err = generatedTableFile("gooq_sharding"); err == nil || !strings.Contains(err.Error(), "gooq_sharding") {
		t.Fatalf("file err=%v", err)
	}
	if fileName, err := generatedTableFile("sharding"); err != nil || fileName != "sharding" {
		t.Fatalf("sharding file=%s err=%v", fileName, err)
	}
	if !isSoftDeleteColumn("DELETED_AT") || !isSoftDeleteColumn("deletedAt") || !isSoftDeleteColumn("delete_at") {
		t.Fatal("soft delete column was not recognized")
	}
	if _, err = goFieldName("2fa_enabled", nil, map[string]string{}); err == nil {
		t.Fatal("expected an invalid Go name")
	}
	used = map[string]string{}
	if _, err = goFieldName("col1", nil, used); err != nil {
		t.Fatal(err)
	}
	if _, err = goFieldName("col_1", nil, used); err == nil {
		t.Fatal("expected a generated field name collision")
	}
}

// TestGen_Config 验证 hack/gooq.yaml（gooq.gen）配置读取与 hack/template 本地模板覆盖：
// 无参执行时 link/path 来自配置文件；本地模板优先于内置模板；无配置且无参数时报错退出。
func TestGen_Config(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var (
			tmpDir  = gfile.Temp("gooq-gen-cfg-test", gtime.TimestampNanoStr())
			dbPath  = filepath.Join(tmpDir, "gooq_test.db")
			workDir = filepath.Join(tmpDir, "work")
			bin     = buildBinary(t)
		)
		defer gfile.Remove(tmpDir)
		t.AssertNil(gfile.Mkdir(workDir))
		initSqlite(t, dbPath)

		// hack/gooq.yaml（sqlite link 使用正斜杠避免 YAML 转义）+ 输出目录。
		cfg := "gooq:\n  gen:\n" +
			"    link: \"sqlite::@file(" + filepath.ToSlash(dbPath) + ")\"\n" +
			"    path: out\n"
		t.AssertNil(gfile.PutContents(filepath.Join(workDir, "hack", "gooq.yaml"), cfg))
		t.AssertNil(gfile.Mkdir(filepath.Join(workDir, "out")))

		run := func() ([]byte, error) {
			cmd := exec.Command(bin)
			cmd.Dir = workDir
			return cmd.CombinedOutput()
		}

		// 无参数执行：link/path 全部来自配置文件。
		out, err := run()
		if err != nil {
			t.Fatalf("run with config failed: %v\n%s", err, out)
		}
		generated := filepath.Join(workDir, "out", "table", "user.go")
		t.Assert(gfile.Exists(generated), true)
		t.Assert(gstr.Contains(gfile.GetContents(generated), `type UserTable struct {`), true)

		// hack/template/table.tmpl 本地模板优先于内置模板。
		t.AssertNil(gfile.PutContents(
			filepath.Join(workDir, "hack", "template", "table.tmpl"),
			"// CUSTOM_TEMPLATE_MARKER\npackage {{.TplPackageName}}\n\n"+
				"var {{.TplTableNameCamelCase}}Generated = \"{{.TplTableName}}\"\n",
		))
		out, err = run()
		if err != nil {
			t.Fatalf("run with local template failed: %v\n%s", err, out)
		}
		content := gfile.GetContents(generated)
		t.Assert(gstr.Contains(content, "CUSTOM_TEMPLATE_MARKER"), true)
		t.Assert(gstr.Contains(content, `UserGenerated = "user"`), true)

		// 无配置且无参数：报错退出。
		noCfgDir := filepath.Join(tmpDir, "no-cfg")
		t.AssertNil(gfile.Mkdir(noCfgDir))
		cmd := exec.Command(bin)
		cmd.Dir = noCfgDir
		_, err = cmd.CombinedOutput()
		t.Assert(err != nil, true)
	})
}
