// 本文件实现 gooq-gen 主流程：连接数据库 → 解析全部表 → 生成 table 与 model 产物。
package gen

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gview"

	"github.com/lanceadd/gooq/cmd/gooq-gen/internal/mlog"
	"github.com/lanceadd/gooq/db"
)

type Input struct {
	Link string // 如 mysql:root:pass@tcp(127.0.0.1:3306)/db。
	Path string
	// TablesFile 是 hack/gooq.tables.yaml。空表示没有分组文件，每张表生成一个普通对象。
	TablesFile string
	// Tenant 由 CLI 从 gooq.gen.tenant 读出。零值不识别租户列。
	Tenant TenantConfig
}

const (
	dirPathTable    = "table"
	tplFileTable    = "table.tmpl"
	tplFileSharding = "sharding.tmpl"
	tplFileModel    = "model.tmpl"
	tplFileRecord   = "record.tmpl"
	hackDirName     = "hack"
	tplDirName      = "template"
	configFileName  = "gooq.yaml"
)

const initConfigContent = `# gooq-gen configuration; CLI options take priority over this file.
gooq:
  gen:
    table:
      # Database link, same format as the ORM configuration of GoFrame.
      # link: "mysql:root:pass@tcp(127.0.0.1:3306)/db"
      path: internal
    # Tenant columns are recognized only when tenant.column is set.
    # tenant:
    #   column: tenant_id
    #   tables:
    #     shop_order: org_id
    #     dict: ""
`

// tplView 是模板渲染视图（跨表生成复用）。
var tplView = gview.New()

// hackDirPath 向上查找 hack 目录（不存在返回空串）。
func hackDirPath() string {
	dir, _ := gfile.Search(hackDirName)
	return dir
}

// getTemplate 读取模板内容：优先使用 hack/template/ 下的本地模板（便于定制），
// 否则回退到内嵌模板。
func getTemplate(name string) string {
	if dir := hackDirPath(); dir != "" {
		if path := gfile.Join(dir, tplDirName, name); gfile.Exists(path) {
			return gfile.GetContents(path)
		}
	}
	return embeddedTemplate(name)
}

func embeddedTemplate(name string) string {
	content, err := gooqTemplateFS.ReadFile("template/" + name)
	if err != nil {
		mlog.Fatalf(`reading embedded template "%s" failed: %+v`, name, err)
	}
	return string(content)
}

// Init 在当前工作目录创建 hack/gooq.yaml，以及 hack/template 下的 table/model/record 模板；已存在的文件不覆盖。
func Init() {
	for _, f := range []struct{ path, content string }{
		{gfile.Join(hackDirName, configFileName), initConfigContent},
		{gfile.Join(hackDirName, tplDirName, tplFileTable), embeddedTemplate(tplFileTable)},
		{gfile.Join(hackDirName, tplDirName, tplFileSharding), embeddedTemplate(tplFileSharding)},
		{gfile.Join(hackDirName, tplDirName, tplFileModel), embeddedTemplate(tplFileModel)},
		{gfile.Join(hackDirName, tplDirName, tplFileRecord), embeddedTemplate(tplFileRecord)},
	} {
		if gfile.Exists(f.path) {
			mlog.Print("skipped, already exists:", gfile.RealPath(f.path))
			continue
		}
		if err := gfile.PutContents(f.path, f.content); err != nil {
			mlog.Fatalf(`writing "%s" failed: %+v`, f.path, err)
		}
		mlog.Print("generated:", gfile.RealPath(f.path))
	}
	mlog.Print(fmt.Sprintf(`edit "%s" to set the database link, then run gooq-gen`, gfile.Join(hackDirName, configFileName)))
}

func Generate(ctx context.Context, in Input) {
	if in.Path == "" {
		in.Path = "internal"
	}
	if in.Link == "" {
		mlog.Fatalf(`database link is required, use -l/--link or set "gooq.gen.table.link" in hack/gooq.yaml`)
	}
	if dirRealPath := gfile.RealPath(in.Path); dirRealPath == "" {
		mlog.Fatalf(`path "%s" does not exist`, in.Path)
	}
	database, err := db.Open(db.Config{Link: in.Link})
	if err != nil {
		mlog.Fatalf(`database initialization failed: %+v`, err)
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			mlog.Fatalf(`closing database failed: %+v`, closeErr)
		}
	}()

	tableNames, err := database.Tables(ctx)
	if err != nil {
		mlog.Fatalf("fetching tables failed: %+v", err)
	}
	specs := specsForGenerate(in.TablesFile, tableNames)
	if err = validateShardSchemas(ctx, database, specs); err != nil {
		mlog.Fatalf("sharding tables validation failed: %+v", err)
	}
	generateTable(ctx, database, specs, gfile.Join(in.Path, dirPathTable), in.Tenant)
	generateModel(ctx, database, specs, gfile.Join(in.Path, dirPathModel))

	mlog.Print("done!")
}

func specsForGenerate(tablesFile string, tableNames []string) []tableSpec {
	if tablesFile == "" {
		specs := make([]tableSpec, 0, len(tableNames))
		for _, name := range tableNames {
			specs = append(specs, tableSpec{Physical: name, Logical: name})
		}
		return specs
	}
	cfg, err := loadTablesFile(tablesFile)
	if err != nil {
		mlog.Fatalf("%+v", err)
	}
	if report := checkTables(cfg, tableNames); !report.empty() {
		mlog.Print(report.String())
		mlog.Fatalf("tables config check failed")
	}
	return specsFromTables(cfg)
}
