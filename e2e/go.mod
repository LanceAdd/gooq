module github.com/lanceadd/gooq/e2e

go 1.23.0

require (
	github.com/go-sql-driver/mysql v1.7.1
	github.com/gogf/gf/v2 v2.10.2
	github.com/lanceadd/gooq v0.0.0
)

require (
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/text v0.25.0 // indirect
)

replace (
	github.com/gogf/gf/v2 => ../../gf2
	github.com/lanceadd/gooq => ../
)
