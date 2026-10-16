package gen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/encoding/gyaml"
	"github.com/gogf/gf/v2/os/gfile"
)

const tablesFileName = "gooq.tables.yaml"

type tablesConfig struct {
	Single []string     `yaml:"single"`
	Groups []tableGroup `yaml:"groups"`
}

type tableGroup struct {
	Name  string   `yaml:"name"`
	Table []string `yaml:"table"`
}

// TablesFilePath 返回已存在的 hack/gooq.tables.yaml，没有时返回空串。
func TablesFilePath() string {
	dir := hackDirPath()
	if dir == "" {
		return ""
	}
	path := gfile.Join(dir, tablesFileName)
	if !gfile.Exists(path) {
		return ""
	}
	return path
}

func loadTablesFile(path string) (tablesConfig, error) {
	var cfg tablesConfig
	if err := gyaml.DecodeTo(gfile.GetBytes(path), &cfg); err != nil {
		return cfg, fmt.Errorf("reading %s: %w", path, err)
	}
	return cfg, nil
}

func writeInitTablesFile(path string, tables []string) error {
	names := append([]string(nil), tables...)
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("# 单一表\n")
	if len(names) == 0 {
		b.WriteString("single: []\n")
	} else {
		b.WriteString("single:\n")
		for _, name := range names {
			fmt.Fprintf(&b, "  - %q\n", name)
		}
	}
	b.WriteString("# 分表\ngroups: []\n")
	return gfile.PutContents(path, b.String())
}

func specsFromTables(cfg tablesConfig) []tableSpec {
	var specs []tableSpec
	for _, name := range cfg.Single {
		name = strings.TrimSpace(name)
		specs = append(specs, tableSpec{Physical: name, Logical: name})
	}
	for _, group := range cfg.Groups {
		var shards []string
		for _, name := range group.Table {
			shards = append(shards, strings.TrimSpace(name))
		}
		specs = append(specs, tableSpec{
			Physical: shards[0],
			Logical:  strings.TrimSpace(group.Name),
			Sharding: true,
			Shards:   shards,
		})
	}
	return specs
}

type tablesReport struct {
	Duplicate []string
	Missing   []string
	Extra     []string
	Logical   []string
	Invalid   []string
}

func (r tablesReport) empty() bool {
	return len(r.Duplicate) == 0 && len(r.Missing) == 0 && len(r.Extra) == 0 && len(r.Logical) == 0 && len(r.Invalid) == 0
}

func (r tablesReport) String() string {
	var b strings.Builder
	writeSection(&b, "重复:", r.Duplicate)
	writeSection(&b, "缺失:", r.Missing)
	writeSection(&b, "多余:", r.Extra)
	writeSection(&b, "逻辑名冲突:", r.Logical)
	writeSection(&b, "分组错误:", r.Invalid)
	return strings.TrimRight(b.String(), "\n")
}

func writeSection(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "%s\n", title)
	for _, line := range lines {
		fmt.Fprintf(b, "  %s\n", line)
	}
}

func checkTables(cfg tablesConfig, dbTables []string) tablesReport {
	var report tablesReport
	type place struct {
		name string
		loc  string
	}
	var places []place
	add := func(name, loc string) {
		name = strings.TrimSpace(name)
		if name == "" {
			report.Invalid = append(report.Invalid, `""  表名是空的`)
			return
		}
		places = append(places, place{name: name, loc: loc})
	}
	for _, name := range cfg.Single {
		add(name, "single")
	}
	groupNames := map[string]int{}
	singleNames := map[string]bool{}
	for _, name := range cfg.Single {
		name = strings.TrimSpace(name)
		if name != "" {
			singleNames[name] = true
		}
	}
	for _, group := range cfg.Groups {
		gname := strings.TrimSpace(group.Name)
		if gname == "" {
			report.Invalid = append(report.Invalid, `""  组名是空的`)
		} else {
			groupNames[gname]++
			if singleNames[gname] {
				report.Logical = append(report.Logical, gname+"  组名与 single 里的表同名")
			}
		}
		var physical int
		for _, name := range group.Table {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				physical++
				if gname != "" && !strings.Contains(trimmed, gname) {
					report.Invalid = append(report.Invalid, fmt.Sprintf("%s  不包含组名 %s", trimmed, gname))
				}
			}
			loc := gname
			if loc == "" {
				loc = `""`
			}
			add(name, loc)
		}
		if gname != "" && physical == 0 {
			report.Invalid = append(report.Invalid, gname+"  没有物理表")
		}
	}
	for name, count := range groupNames {
		if count > 1 {
			report.Logical = append(report.Logical, name+"  有两个组都叫这个名字")
		}
	}
	report.Logical = uniqueStable(report.Logical)
	sort.Strings(report.Logical)

	locs := map[string][]string{}
	var order []string
	for _, item := range places {
		if _, ok := locs[item.name]; !ok {
			order = append(order, item.name)
		}
		locs[item.name] = append(locs[item.name], item.loc)
	}
	for _, name := range order {
		where := locs[name]
		if len(where) < 2 {
			continue
		}
		if samePlace(where) {
			report.Duplicate = append(report.Duplicate, fmt.Sprintf("%s  在 %s 里写了两次", name, where[0]))
			continue
		}
		text := "在 " + where[0]
		for _, loc := range uniqueStable(where[1:]) {
			text += "，也在 " + loc
		}
		report.Duplicate = append(report.Duplicate, name+"  "+text)
	}

	inFile := map[string]bool{}
	for _, name := range order {
		inFile[name] = true
	}
	inDB := map[string]bool{}
	for _, name := range dbTables {
		inDB[name] = true
		if !inFile[name] {
			report.Missing = append(report.Missing, name)
		}
	}
	for _, name := range order {
		if !inDB[name] {
			report.Extra = append(report.Extra, name)
		}
	}
	sort.Strings(report.Missing)
	sort.Strings(report.Extra)
	return report
}

func samePlace(where []string) bool {
	for _, loc := range where[1:] {
		if loc != where[0] {
			return false
		}
	}
	return true
}

func uniqueStable(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

type tablesDiff struct {
	New   []string
	Stale []string
}

func (d tablesDiff) empty() bool { return len(d.New) == 0 && len(d.Stale) == 0 }

func (d tablesDiff) String() string {
	var b strings.Builder
	writeSection(&b, "新表:", d.New)
	writeSection(&b, "不用的表:", d.Stale)
	return strings.TrimRight(b.String(), "\n")
}

func diffTables(cfg tablesConfig, dbTables []string) tablesDiff {
	inFile := map[string]bool{}
	for _, name := range cfg.Single {
		if name = strings.TrimSpace(name); name != "" {
			inFile[name] = true
		}
	}
	for _, group := range cfg.Groups {
		for _, name := range group.Table {
			if name = strings.TrimSpace(name); name != "" {
				inFile[name] = true
			}
		}
	}
	var diff tablesDiff
	inDB := map[string]bool{}
	for _, name := range dbTables {
		inDB[name] = true
		if !inFile[name] {
			diff.New = append(diff.New, name)
		}
	}
	for name := range inFile {
		if !inDB[name] {
			diff.Stale = append(diff.Stale, name)
		}
	}
	sort.Strings(diff.New)
	sort.Strings(diff.Stale)
	return diff
}
