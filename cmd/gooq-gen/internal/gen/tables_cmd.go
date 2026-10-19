package gen

import (
	"context"

	"github.com/gogf/gf/v2/os/gfile"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	"github.com/lanceadd/gooq/db"
)

// RunTables 执行 gooq-gen tables init|check|diff。
func RunTables(ctx context.Context, link, action string) {
	switch action {
	case "init":
		tablesInit(ctx, link)
	case "check":
		tablesCheck(ctx, link)
	case "diff":
		runTablesDiff(ctx, link)
	default:
		mlog.Fatalf("usage: gooq-gen tables {init|check|diff}")
	}
}

func tablesInit(ctx context.Context, link string) {
	dir := hackDirPath()
	if dir == "" {
		mlog.Fatalf("hack directory not found, run gooq-gen init first")
	}
	path := gfile.Join(dir, tablesFileName)
	if gfile.Exists(path) {
		mlog.Fatalf("%s already exists", path)
	}
	database := openDatabase(link)
	defer closeDatabase(database)
	names, err := database.Tables(ctx)
	if err != nil {
		mlog.Fatalf("fetching tables failed: %+v", err)
	}
	if err = writeInitTablesFile(path, names); err != nil {
		mlog.Fatalf(`writing "%s" failed: %+v`, path, err)
	}
	mlog.Print("generated:", gfile.RealPath(path))
}

func tablesCheck(ctx context.Context, link string) {
	cfg, names := loadTablesAgainstDB(ctx, link)
	report := checkTables(cfg, names)
	if report.empty() {
		return
	}
	mlog.Print(report.String())
	mlog.Fatalf("tables check failed")
}

func runTablesDiff(ctx context.Context, link string) {
	cfg, names := loadTablesAgainstDB(ctx, link)
	diff := diffTables(cfg, names)
	if diff.empty() {
		mlog.Print("没有差异")
		return
	}
	mlog.Print(diff.String())
	mlog.Fatalf("tables diff failed")
}

func loadTablesAgainstDB(ctx context.Context, link string) (tablesConfig, []string) {
	path := TablesFilePath()
	if path == "" {
		if hackDirPath() == "" {
			mlog.Fatalf("hack directory not found, run gooq-gen init first")
		}
		mlog.Fatalf("hack/%s not found, run gooq-gen tables init first", tablesFileName)
	}
	cfg, err := loadTablesFile(path)
	if err != nil {
		mlog.Fatalf("%+v", err)
	}
	database := openDatabase(link)
	defer closeDatabase(database)
	names, err := database.Tables(ctx)
	if err != nil {
		mlog.Fatalf("fetching tables failed: %+v", err)
	}
	return cfg, names
}

func openDatabase(link string) *db.DB {
	if link == "" {
		mlog.Fatalf("database link is required, use -l/--link or set %q in hack/gooq.yaml", ConfigKeyTableLink)
	}
	database, err := db.Open(db.Config{Link: link})
	if err != nil {
		mlog.Fatalf(`database initialization failed: %+v`, err)
	}
	return database
}

func closeDatabase(database *db.DB) {
	if err := database.Close(); err != nil {
		mlog.Fatalf(`closing database failed: %+v`, err)
	}
}
