package fn

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/dsl"
)

type shapeOnlyDialect struct{}

func (shapeOnlyDialect) Name() gooq.Dialect { return "shape-only" }
func (shapeOnlyDialect) Quote(ident string) string {
	return `"` + ident + `"`
}
func (shapeOnlyDialect) Placeholder(int) string { return "?" }
func (shapeOnlyDialect) CastType(localType gooq.LocalType) string {
	return strings.ToUpper(string(localType))
}
func (shapeOnlyDialect) QuoteLiteral(s string) string { return "'" + s + "'" }
func (shapeOnlyDialect) ShareLock() string            { return "FOR SHARE" }
func (shapeOnlyDialect) Returning() string            { return "" }
func (shapeOnlyDialect) RenderLimit(_ *gooq.RenderContext, limit, offset int) string {
	if offset > 0 {
		return fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	return fmt.Sprintf(" LIMIT %d", limit)
}
func (shapeOnlyDialect) InsertKeyword(bool) string { return "INSERT" }
func (shapeOnlyDialect) RenderUpsert(*gooq.RenderContext, gooq.UpsertClause) (string, error) {
	return "", nil
}
func (shapeOnlyDialect) RenderExcluded(*gooq.RenderContext, string) string { return "" }
func (shapeOnlyDialect) RenderDelete(_ *gooq.RenderContext, target string, _ []gooq.JoinRender, where string) (string, error) {
	sqlText := "DELETE FROM " + target
	if where != "" {
		sqlText += " WHERE " + where
	}
	return sqlText, nil
}
func (shapeOnlyDialect) RenderUpdate(_ *gooq.RenderContext, target, sets, where string, _ []gooq.JoinRender) (string, error) {
	sqlText := "UPDATE " + target + " SET " + sets
	if where != "" {
		sqlText += " WHERE " + where
	}
	return sqlText, nil
}
func (shapeOnlyDialect) JoinKeyword(keyword string) string { return keyword }
func (shapeOnlyDialect) SupportsGroup(string) error        { return nil }
func (shapeOnlyDialect) RollupWith() bool                  { return false }
func (shapeOnlyDialect) OmitLock() bool                    { return false }

func TestFn_UnsupportedDialect(t *testing.T) {
	gooq.RegisterDialect(shapeOnlyDialect{})
	_, _, err := dsl.Select(DateFormat(testUser.CreatedAt, "%Y-%m-%d")).From(testUser).ToSql("shape-only")
	if err == nil || !strings.Contains(err.Error(), `dialect "shape-only" does not support DateFormat`) {
		t.Fatalf("DateFormat err=%v", err)
	}
}
