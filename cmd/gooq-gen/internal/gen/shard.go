package gen

import (
	"context"
	"fmt"

	"github.com/lanceadd/gooq/db"
)

type tableSpec struct {
	Physical string
	Logical  string
	Sharding bool
	Shards   []string
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
			if diff := columnsDiff(base, columns); diff != "" {
				return fmt.Errorf("shards of table %q differ: %q and %q: %s", spec.Logical, spec.Shards[0], shard, diff)
			}
		}
	}
	return nil
}

func columnsDiff(base, other []db.Column) string {
	if len(base) != len(other) {
		return fmt.Sprintf("column count %d and %d", len(base), len(other))
	}
	for i := range base {
		if diff := columnDiff(base[i], other[i]); diff != "" {
			if base[i].Name == other[i].Name {
				return fmt.Sprintf("column %q %s", base[i].Name, diff)
			}
			return fmt.Sprintf("column %d %s", i+1, diff)
		}
	}
	return ""
}

func columnDiff(base, other db.Column) string {
	switch {
	case base.Name != other.Name:
		return fmt.Sprintf("name %q and %q", base.Name, other.Name)
	case base.LocalType != other.LocalType:
		return fmt.Sprintf("type %s and %s", base.LocalType, other.LocalType)
	case base.Nullable != other.Nullable:
		return fmt.Sprintf("nullable %v and %v", base.Nullable, other.Nullable)
	case base.Primary != other.Primary:
		return fmt.Sprintf("primary %v and %v", base.Primary, other.Primary)
	case base.AutoIncrement != other.AutoIncrement:
		return fmt.Sprintf("autoIncrement %v and %v", base.AutoIncrement, other.AutoIncrement)
	case base.Unique != other.Unique:
		return fmt.Sprintf("unique %v and %v", base.Unique, other.Unique)
	default:
		return ""
	}
}
