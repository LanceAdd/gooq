package db

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/glog"

	"github.com/lanceadd/gooq"
)

func TestLoadConfigAndDryRun(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cfg.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	content := "gooq:\n  database:\n    - name: cfggroup\n      endpoints:\n        - link: '" + link + "'\n          debug: true\n          dryRun: true\n"
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	config := gcfg.NewWithAdapter(adapter)
	if err = LoadConfig(ctx, config); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveConfig("cfggroup"); err != nil {
			t.Error(err)
		}
	})
	database, err := Instance("cfggroup")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	logger := database.Logger()
	logger.SetWriter(&buf)
	logger.SetStdoutPrint(false)
	logger.SetLevel(glog.LEVEL_ALL)
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CREATE TABLE item") {
		t.Fatalf("debug log %q", buf.String())
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id) VALUES (?)`, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Query(ctx, `SELECT 1`); err == nil || !strings.Contains(err.Error(), "not available in dry run") {
		t.Fatalf("dry run query err = %v", err)
	}
	database.SetDryRun(false)
	var count int64
	if err = database.Value(ctx, &count, `SELECT COUNT(*) FROM item`); err == nil {
		t.Fatal("dry run should not create the table")
	}
}

func TestLoadConfigLoggerAndDebug(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "log.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	content := "gooq:\n  logger:\n    level: all\n    stdout: false\n  database:\n    - name: loggroup\n      endpoints:\n        - link: '" + link + "'\n          debug: true\n"
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveConfig("loggroup"); err != nil {
			t.Error(err)
		}
	})
	database, err := Instance("loggroup")
	if err != nil {
		t.Fatal(err)
	}
	if database.Logger().GetConfig().StdoutPrint {
		t.Fatal("logger stdout was not applied")
	}
	var buf bytes.Buffer
	database.Logger().SetWriter(&buf)
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CREATE TABLE item") {
		t.Fatalf("debug log %q", buf.String())
	}

	bad, err := gcfg.NewAdapterContent("gooq:\n  logger:\n    level: nope\n  database:\n    - name: badgroup\n      endpoints:\n        - link: '" + link + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(bad)); err == nil {
		t.Fatal("expected invalid logger level")
	}
}

func TestLoadConfigSQLFormat(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "inline.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	content := "gooq:\n  logger:\n    level: all\n    stdout: false\n    sqlFormat: inline\n  database:\n    - name: inlinegroup\n      endpoints:\n        - link: '" + link + "'\n          debug: true\n"
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := RemoveConfig("inlinegroup"); err != nil {
			t.Error(err)
		}
		loggerSQLFormat.Set(string(SQLFormatArgs))
	})
	database, err := Instance("inlinegroup")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	database.Logger().SetWriter(&buf)
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id) VALUES (?)`, 1); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, `INSERT INTO item(id) VALUES (1)`) {
		t.Fatalf("inline log %q", got)
	}
	if strings.Contains(got, "args (1):") {
		t.Fatalf("inline log still lists args %q", got)
	}

	bad, err := gcfg.NewAdapterContent("gooq:\n  logger:\n    sqlFormat: values\n  database:\n    - name: badformat\n      endpoints:\n        - link: '" + link + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(bad)); err == nil || !strings.Contains(err.Error(), "logger.sqlFormat") {
		t.Fatalf("invalid sqlFormat err = %v", err)
	}
}

func isolateRuntimeConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := gcfg.Instance(runtimeConfigFileName)
	if cfg == nil {
		t.Fatal("runtime config is unavailable")
	}
	previous := cfg.GetAdapter()
	loaded := configLoaded.Val()
	adapter, err := gcfg.NewAdapterFile(runtimeConfigFileName)
	if err != nil {
		t.Fatal(err)
	}
	if err = adapter.SetPath(dir); err != nil {
		t.Fatal(err)
	}
	cfg.SetAdapter(adapter)
	configLoaded.Set(false)
	t.Cleanup(func() {
		cfg.SetAdapter(previous)
		configLoaded.Set(loaded)
	})
}

func TestInstanceWithoutConfigFile(t *testing.T) {
	isolateRuntimeConfig(t, t.TempDir())

	const group = "no-such-group"
	want := `gooq: empty database configuration for group "no-such-group"`
	for i := 0; i < 2; i++ {
		_, err := Instance(group)
		if err == nil {
			t.Fatal("expected missing group error")
		}
		if strings.Contains(err.Error(), "possible config files") || strings.Contains(err.Error(), "not found in resource manager") {
			t.Fatalf("config file error leaked: %s", err)
		}
		if err.Error() != want {
			t.Fatalf("got %q", err.Error())
		}
	}
}

func TestInstanceKeepsConfigParseError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gooq.yaml"), []byte("gooq: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("gooq:\n  database: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isolateRuntimeConfig(t, dir)

	for i := 0; i < 2; i++ {
		_, err := Instance("parse-group")
		if err == nil {
			t.Fatal("expected config parse error")
		}
		if strings.Contains(err.Error(), "empty database configuration") {
			t.Fatalf("parse error was dropped: %s", err)
		}
		if !strings.Contains(err.Error(), "load config file") && !strings.Contains(err.Error(), gfile.Basename(dir)) {
			t.Fatalf("got %q", err.Error())
		}
	}
}

func TestRuntimeConfigIgnoresAppConfig(t *testing.T) {
	dir := t.TempDir()
	link := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "app.db")) + ")"
	gooqFile := "gooq:\n  database:\n    - name: from-gooq\n      endpoints:\n        - link: '" + link + "'\n"
	appFile := "gooq:\n  database:\n    - name: from-config\n      endpoints:\n        - link: '" + link + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "gooq.yaml"), []byte(gooqFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(appFile), 0o644); err != nil {
		t.Fatal(err)
	}
	isolateRuntimeConfig(t, dir)
	t.Cleanup(func() { _ = RemoveConfig("from-gooq") })

	database, err := Instance("from-gooq")
	if err != nil {
		t.Fatal(err)
	}
	if database.Group() != "from-gooq" {
		t.Fatalf("group = %s", database.Group())
	}
	if _, err = Instance("from-config"); err == nil || !strings.Contains(err.Error(), "empty database configuration") {
		t.Fatalf("app config group err = %v", err)
	}
}

func TestLoadConfigDatabaseShape(t *testing.T) {
	ctx := context.Background()
	_ = RemoveConfig("default")
	_ = RemoveConfig("orders")
	dir := t.TempDir()
	master := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "master.db")) + ")"
	slave := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "slave.db")) + ")"
	content := "gooq:\n  database:\n    - name: default\n      endpoints:\n        - link: '" + master + "'\n          role: master\n        - link: '" + slave + "'\n          role: slave\n          weight: 2\n    - name: orders\n      endpoints:\n        - link: '" + master + "'\n"
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = RemoveConfig("default")
		_ = RemoveConfig("orders")
	})
	database, err := Instance("orders")
	if err != nil {
		t.Fatal(err)
	}
	if database.Group() != "orders" {
		t.Fatalf("group = %s", database.Group())
	}
	primary, err := Instance()
	if err != nil {
		t.Fatal(err)
	}
	if len(primary.masters) != 1 || len(primary.slaves) != 1 || primary.slaves[0].weight != 2 {
		t.Fatalf("masters=%d slaves=%d", len(primary.masters), len(primary.slaves))
	}

	repeated := "gooq:\n  database:\n    - name: default\n      endpoints:\n        - link: '" + master + "'\n    - name: default\n      endpoints:\n        - link: '" + slave + "'\n"
	again, err := gcfg.NewAdapterContent(repeated)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(again)); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate group err = %v", err)
	}

	legacy := "gooq:\n  database:\n    default:\n      - link: '" + master + "'\n"
	old, err := gcfg.NewAdapterContent(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(old)); err == nil || !strings.Contains(err.Error(), "list of groups") {
		t.Fatalf("legacy database err = %v", err)
	}

	missingName := "gooq:\n  database:\n    - endpoints:\n        - link: '" + master + "'\n    - endpoints:\n        - link: '" + slave + "'\n"
	unnamed, err := gcfg.NewAdapterContent(missingName)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(unnamed)); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("missing name err = %v", err)
	}
	missingLink := "gooq:\n  database:\n    - name: broken\n      endpoints:\n        - role: master\n"
	nolink, err := gcfg.NewAdapterContent(missingLink)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(nolink)); err == nil || !strings.Contains(err.Error(), "link") {
		t.Fatalf("missing link err = %v", err)
	}
}

func TestLoadConfigDefaultGroup(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	link := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "one.db")) + ")"
	orders := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "orders.db")) + ")"
	report := "sqlite::@file(" + filepath.ToSlash(filepath.Join(dir, "report.db")) + ")"
	t.Cleanup(func() {
		SetDefaultGroup("")
		_ = RemoveConfig(DefaultGroupName)
		_ = RemoveConfig("orders")
		_ = RemoveConfig("report")
	})

	_ = RemoveConfig(DefaultGroupName)
	unnamed := "gooq:\n  database:\n    - endpoints:\n        - link: '" + link + "'\n"
	adapter, err := gcfg.NewAdapterContent(unnamed)
	if err != nil {
		t.Fatal(err)
	}
	SetDefaultGroup("orders")
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	database, err := Instance()
	if err != nil {
		t.Fatal(err)
	}
	if database.Group() != DefaultGroupName {
		t.Fatalf("group = %s", database.Group())
	}
	if err = RemoveConfig(DefaultGroupName); err != nil {
		t.Fatal(err)
	}

	namedOnly := "gooq:\n  database:\n    - name: orders\n      endpoints:\n        - link: '" + orders + "'\n"
	adapter, err = gcfg.NewAdapterContent(namedOnly)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	if _, err = Instance(); err == nil || !strings.Contains(err.Error(), "empty database configuration") {
		t.Fatalf("single named group err = %v", err)
	}
	if _, err = Instance("orders"); err != nil {
		t.Fatal(err)
	}

	chosen := "gooq:\n  defaultGroup: orders\n  database:\n    - name: orders\n      endpoints:\n        - link: '" + orders + "'\n    - name: report\n      endpoints:\n        - link: '" + report + "'\n"
	adapter, err = gcfg.NewAdapterContent(chosen)
	if err != nil {
		t.Fatal(err)
	}
	SetDefaultGroup("report")
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err != nil {
		t.Fatal(err)
	}
	database, err = Instance()
	if err != nil {
		t.Fatal(err)
	}
	if database.Group() != "orders" {
		t.Fatalf("group = %s", database.Group())
	}
	SetDefaultGroup("report")
	database, err = Instance()
	if err != nil || database.Group() != "report" {
		t.Fatalf("override group = %v, %v", database, err)
	}

	unknown := "gooq:\n  defaultGroup: missing\n  database:\n    - name: orders\n      endpoints:\n        - link: '" + orders + "'\n"
	adapter, err = gcfg.NewAdapterContent(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(adapter)); err == nil || !strings.Contains(err.Error(), `default group "missing" is not configured`) {
		t.Fatalf("unknown default group err = %v", err)
	}
}

func TestEmptyGroupNameUsesCurrentDefault(t *testing.T) {
	link := "sqlite::@file(" + filepath.ToSlash(filepath.Join(t.TempDir(), "empty-name.db")) + ")"
	SetDefaultGroup("orders")
	t.Cleanup(func() {
		SetDefaultGroup("")
		_ = RemoveConfig("orders")
	})
	if err := SetConfig("", Config{Link: link}); err != nil {
		t.Fatal(err)
	}
	database, err := Instance()
	if err != nil {
		t.Fatal(err)
	}
	if database.Group() != "orders" {
		t.Fatalf("group = %s", database.Group())
	}
	if err = RemoveConfig(""); err != nil {
		t.Fatal(err)
	}
	if _, err = Instance(); err == nil || !strings.Contains(err.Error(), "empty database configuration") {
		t.Fatalf("removed default group err = %v", err)
	}
}

func TestSchemaRouteClosesOwnedPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema.db")
	database, err := Open(Config{Link: "sqlite::@file(" + path + ")"})
	if err != nil {
		t.Fatal(err)
	}
	database.schema = "other"
	routed := database.Master()
	if routed.shared {
		t.Fatal("schema route lost ownership of its pool")
	}
	if err = routed.Close(); err != nil {
		t.Fatal(err)
	}
	if err = routed.masters[0].sql.Ping(); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("pool ping = %v", err)
	}
}

func TestPrepareAndSchema(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stmt.db")
	database, err := Open(Config{Link: "sqlite::@file(" + path + ")"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id, name) VALUES (1, 'john')`); err != nil {
		t.Fatal(err)
	}
	stmt, err := database.Prepare(ctx, `SELECT name FROM item WHERE id = ?`, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stmt.Close(); err != nil {
			t.Error(err)
		}
	})
	rows, err := stmt.Query(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatal("prepared query returned no row")
	}
	var name string
	if err = rows.Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err = rows.Close(); err != nil {
		t.Fatal(err)
	}
	if name != "john" {
		t.Fatalf("name %q", name)
	}
	view, err := database.Schema("main")
	if err != nil {
		t.Fatal(err)
	}
	if view.GetSchema() != "main" {
		t.Fatalf("schema %q", view.GetSchema())
	}
	if err = view.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if len(database.Stats()) == 0 {
		t.Fatal("expected pool stats")
	}
	sqlText, err := schemaSQL(gooq.DialectMySQL, "app")
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != "USE `app`" {
		t.Fatalf("mysql schema sql %q", sqlText)
	}
	sqlText, err = schemaSQL(gooq.DialectPgsql, "app")
	if err != nil {
		t.Fatal(err)
	}
	if sqlText != `SET search_path TO "app"` {
		t.Fatalf("pgsql schema sql %q", sqlText)
	}
}
