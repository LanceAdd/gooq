package db

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/glog"

	"github.com/lanceadd/gooq"
)

func TestLoadConfigAndDryRun(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cfg.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	content := "gooq:\n  database:\n    cfggroup:\n      link: '" + link + "'\n      debug: true\n      dryRun: true\n"
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
	logger := glog.New()
	logger.SetWriter(&buf)
	logger.SetStdoutPrint(false)
	logger.SetLevel(glog.LEVEL_ALL)
	database.SetLogger(logger)
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CREATE TABLE item") {
		t.Fatalf("debug log %q", buf.String())
	}
	if _, err = database.Exec(ctx, `INSERT INTO item(id) VALUES (?)`, 1); err != nil {
		t.Fatal(err)
	}
	database.SetDryRun(false)
	var count int64
	if err = database.GetScan(ctx, &count, `SELECT COUNT(*) FROM item`); err == nil {
		t.Fatal("dry run should not create the table")
	}
}

func TestLoadConfigLoggerAndDebug(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "log.db")
	link := "sqlite::@file(" + filepath.ToSlash(path) + ")"
	content := "gooq:\n  logger:\n    level: all\n    stdout: false\n  database:\n    loggroup:\n      link: '" + link + "'\n      debug: true\n"
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
	var buf bytes.Buffer
	database.Logger().SetWriter(&buf)
	if _, err = database.Exec(ctx, `CREATE TABLE item (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CREATE TABLE item") {
		t.Fatalf("debug log %q", buf.String())
	}

	bad, err := gcfg.NewAdapterContent("gooq:\n  logger:\n    level: nope\n  database:\n    badgroup:\n      link: '" + link + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = LoadConfig(ctx, gcfg.NewWithAdapter(bad)); err == nil {
		t.Fatal("expected invalid logger level")
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
