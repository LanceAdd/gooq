package gen

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/text/gregex"
	"github.com/gogf/gf/v2/text/gstr"

	"github.com/lanceadd/gooq/db"
)

type tableSpec struct {
	Physical string
	Logical  string
	Sharding bool
	Shards   []string
}

func tableSpecs(tableNames, patterns []string) ([]tableSpec, error) {
	sortedPatterns := append([]string(nil), patterns...)
	sort.Slice(sortedPatterns, func(i, j int) bool {
		return len(sortedPatterns[i]) > len(sortedPatterns[j])
	})
	for _, pattern := range sortedPatterns {
		if !strings.Contains(pattern, "?") {
			return nil, fmt.Errorf(`sharding pattern %q has no "?"`, pattern)
		}
	}
	sortedNames := append([]string(nil), tableNames...)
	sort.Strings(sortedNames)
	groupOf := map[string]string{}
	for _, name := range sortedNames {
		for _, pattern := range sortedPatterns {
			match, err := gregex.MatchString(gstr.Replace(pattern, "?", `(.+)`), name)
			if err != nil {
				return nil, fmt.Errorf("invalid sharding pattern %q: %v", pattern, err)
			}
			if len(match) < 2 {
				continue
			}
			groupOf[name] = gstr.Trim(gstr.Replace(pattern, "?", ""), "_.-")
			break
		}
	}
	var specs []tableSpec
	seen := map[string]int{}
	for _, name := range sortedNames {
		logical, sharded := groupOf[name]
		if !sharded {
			specs = append(specs, tableSpec{Physical: name, Logical: name})
			continue
		}
		if index, ok := seen[logical]; ok {
			specs[index].Shards = append(specs[index].Shards, name)
			continue
		}
		specs = append(specs, tableSpec{Physical: name, Logical: logical, Sharding: true, Shards: []string{name}})
		seen[logical] = len(specs) - 1
	}
	return specs, nil
}

func validateShardSchemas(ctx context.Context, database *db.DB, specs []tableSpec) error {
	for _, spec := range specs {
		if !spec.Sharding || len(spec.Shards) < 2 {
			continue
		}
		base, err := database.TableFields(ctx, spec.Shards[0])
		if err != nil {
			return err
		}
		for _, shard := range spec.Shards[1:] {
			columns, err := database.TableFields(ctx, shard)
			if err != nil {
				return err
			}
			if !sameColumns(base, columns) {
				return fmt.Errorf("shards of table %q differ: %q and %q", spec.Logical, spec.Shards[0], shard)
			}
		}
	}
	return nil
}

func sameColumns(a, b []db.Column) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].DBType != b[i].DBType {
			return false
		}
	}
	return true
}
