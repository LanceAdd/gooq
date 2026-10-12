package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/glog"
	"github.com/gogf/gf/v2/util/gconv"
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

// SetDryRun 打开后跳过 Query、Exec、预编译和读取。Begin、Commit、Rollback 仍会执行。
func (d *DB) SetDryRun(enabled bool) {
	if d.dryRun == nil {
		return
	}
	d.dryRun.Set(enabled)
}

// SetLogger 替换写出 SQL 调试行的日志器。传入 nil 时不替换。
func (d *DB) SetLogger(logger *glog.Logger) {
	if logger == nil {
		return
	}
	d.logger = logger
}

// Logger 返回 SQL 日志器。
func (d *DB) Logger() *glog.Logger {
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
	if d == nil || d.logger == nil || d.debug == nil || !d.debug.Val() {
		return
	}
	line := fmt.Sprintf("[%d ms] [%s] %s", cost.Milliseconds(), d.group, formatSQL(query, args))
	if err != nil {
		d.logger.Error(ctx, line+"\nError: "+err.Error())
		return
	}
	d.logger.Debug(ctx, line)
}

func formatSQL(query string, args []any) string {
	if len(args) == 0 || !strings.Contains(query, "?") {
		return query
	}
	var builder strings.Builder
	argIndex := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' && argIndex < len(args) {
			builder.WriteString(gconv.String(args[argIndex]))
			argIndex++
			continue
		}
		builder.WriteByte(query[i])
	}
	return builder.String()
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
