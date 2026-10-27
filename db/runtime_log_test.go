package db

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/os/glog"

	"github.com/lanceadd/gooq"
)

func newLogTestDatabase(format SQLFormat) (*DB, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := glog.NewWithWriter(&buf)
	logger.SetStdoutPrint(false)
	logger.SetLevel(glog.LEVEL_ALL)
	return &DB{
		group:     "test",
		dialect:   gooq.DialectPgsql,
		debug:     gtype.NewBool(true),
		sqlFormat: gtype.NewString(string(format)),
		logger:    logger,
	}, &buf
}

func TestLogSQLFormatModes(t *testing.T) {
	ctx := context.Background()
	query := `SELECT "id" FROM "user" WHERE "id" = $1`
	args := []any{int64(7)}

	t.Run("args", func(t *testing.T) {
		database, buf := newLogTestDatabase(SQLFormatArgs)
		database.logSQL(ctx, query, args, time.Millisecond, nil)
		got := buf.String()
		if !strings.Contains(got, query) || !strings.Contains(got, "args (1):") || !strings.Contains(got, "1: 7") {
			t.Fatalf("args log %q", got)
		}
	})

	t.Run("inline", func(t *testing.T) {
		database, buf := newLogTestDatabase(SQLFormatInline)
		database.logSQL(ctx, query, args, time.Millisecond, nil)
		got := buf.String()
		if !strings.Contains(got, `WHERE "id" = 7`) || strings.Contains(got, "args (") {
			t.Fatalf("inline log %q", got)
		}
	})

	t.Run("debug off", func(t *testing.T) {
		database, buf := newLogTestDatabase(SQLFormatInline)
		database.SetDebug(false)
		database.logSQL(ctx, query, args, time.Millisecond, nil)
		if buf.Len() != 0 {
			t.Fatalf("debug off wrote %q", buf.String())
		}
	})
}
