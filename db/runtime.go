package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/glog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type dryResult struct{}

func (dryResult) LastInsertId() (int64, error) { return 0, nil }

func (dryResult) RowsAffected() (int64, error) { return 0, nil }

// SetDebug 打开或关闭这个数据库及其路由视图的 SQL 日志。debug 还没初始化时什么都不做。
func (d *DB) SetDebug(enabled bool) {
	if d.debug == nil {
		return
	}
	d.debug.Set(enabled)
}

// SetDryRun 打开后跳过 Exec、预编译和读取。Query 返回错误。Begin、Commit、Rollback 仍会执行。
func (d *DB) SetDryRun(enabled bool) {
	if d.dryRun == nil {
		return
	}
	d.dryRun.Set(enabled)
}

// Logger 返回打开时确定的 SQL 日志器。日志器在运行期间不替换。
func (d *DB) Logger() *glog.Logger {
	if d == nil {
		return nil
	}
	return d.logger
}

func (d *DB) observe(ctx context.Context, query string, args []any, fn func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if d != nil && d.dryRun != nil && d.dryRun.Val() {
		d.logSQL(ctx, query, args, 0, nil)
		return nil
	}
	ctx, span := d.startSpan(ctx, query)
	started := time.Now()
	err := fn(ctx)
	d.finishSpan(span, err, time.Since(started))
	d.logSQL(ctx, query, args, time.Since(started), err)
	return err
}

func (d *DB) logSQL(ctx context.Context, query string, args []any, cost time.Duration, err error) {
	logger := d.Logger()
	if d == nil || logger == nil || d.debug == nil || !d.debug.Val() {
		return
	}
	line := fmt.Sprintf("[%d ms] [%s] %s", cost.Milliseconds(), d.group, formatSQL(query, args))
	if err != nil {
		logger.Error(ctx, line+"\n  error: "+strings.ReplaceAll(err.Error(), "\n", "\n    "))
		return
	}
	logger.Debug(ctx, line)
}

const maxArgLength = 2000

func formatSQL(query string, args []any) string {
	if len(args) == 0 {
		return query
	}
	var builder strings.Builder
	builder.WriteString(query)
	fmt.Fprintf(&builder, "\n  args (%d):", len(args))
	for i, arg := range args {
		fmt.Fprintf(&builder, "\n    %d: %s", i+1, formatArg(arg))
	}
	return builder.String()
}

func formatArg(arg any) string {
	switch value := arg.(type) {
	case nil:
		return "NULL"
	case string:
		return formatStringArg(value)
	case time.Time:
		return value.Format(time.RFC3339Nano)
	case []byte:
		return formatBytesArg(value)
	default:
		return formatDefaultArg(value)
	}
}

func formatStringArg(value string) string {
	runes := []rune(value)
	if len(runes) > maxArgLength {
		return fmt.Sprintf("%q (truncated)", string(runes[:maxArgLength]))
	}
	return fmt.Sprintf("%q", value)
}

func formatBytesArg(value []byte) string {
	length := len(value)
	if length > maxArgLength {
		value = value[:maxArgLength]
	}
	text := fmt.Sprintf("<%d bytes> 0x%x", length, value)
	if length > maxArgLength {
		text += " (truncated)"
	}
	return text
}

func formatDefaultArg(value any) string {
	text := fmt.Sprintf("%v", value)
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\r", "\\r")
	runes := []rune(text)
	if len(runes) > maxArgLength {
		text = string(runes[:maxArgLength]) + " (truncated)"
	}
	return text
}

func (d *DB) startSpan(ctx context.Context, query string) (context.Context, trace.Span) {
	if d == nil || gtrace.IsUsingDefaultProvider() {
		return ctx, nil
	}
	tracer := otel.GetTracerProvider().Tracer("github.com/lanceadd/gooq/db")
	ctx, span := tracer.Start(ctx, "db", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(
		attribute.String("db.group", d.group),
		attribute.String("db.statement", query),
	)
	return ctx, span
}

func (d *DB) finishSpan(span trace.Span, err error, cost time.Duration) {
	if span == nil {
		return
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
	}
	span.SetAttributes(attribute.String("db.cost", cost.String()))
	span.End()
}
