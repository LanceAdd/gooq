package gooq

import (
	"fmt"
	"strings"
	"sync"
)

type Dialect string

const (
	DialectMySQL  Dialect = "mysql"
	DialectPgsql  Dialect = "pgsql"
	DialectSQLite Dialect = "sqlite"
)

type DialectInfo struct {
	Placeholder string
	QuoteChar   string
	ShareLock   string
	Returning   string
	RenderLimit func(rc *renderContext, limit, offset int) string
	CastTypes   map[LocalType]string // CAST 类型名映射（未注册类型回退 LocalType 大写）。
}

var dialectRegistry = make(map[string]*DialectInfo)

var dialectRegistryMu sync.RWMutex

func RegisterDialect(name string, info DialectInfo) {
	dialectRegistryMu.Lock()
	defer dialectRegistryMu.Unlock()
	dialectRegistry[name] = &info
}

func getDialectInfo(name string) *DialectInfo {
	dialectRegistryMu.RLock()
	defer dialectRegistryMu.RUnlock()
	return dialectRegistry[name]
}

func init() {
	RegisterDialect("mysql", DialectInfo{
		Placeholder: "?",
		QuoteChar:   "`",
		ShareLock:   "LOCK IN SHARE MODE",
		CastTypes: map[LocalType]string{
			LocalTypeString:   "CHAR",
			LocalTypeInt:      "SIGNED",
			LocalTypeInt32:    "SIGNED",
			LocalTypeInt64:    "SIGNED",
			LocalTypeBigInt:   "SIGNED",
			LocalTypeUint:     "UNSIGNED",
			LocalTypeUint32:   "UNSIGNED",
			LocalTypeUint64:   "UNSIGNED",
			LocalTypeFloat32:  "DECIMAL(10, 4)",
			LocalTypeFloat64:  "DECIMAL(10, 4)",
			LocalTypeBool:     "UNSIGNED",
			LocalTypeDate:     "DATE",
			LocalTypeDatetime: "DATETIME",
			LocalTypeTime:     "TIME",
			LocalTypeJson:     "CHAR",
			LocalTypeJsonb:    "CHAR",
			LocalTypeBytes:    "BINARY",
			LocalTypeUUID:     "CHAR(36)",
		},
	})
	RegisterDialect("pgsql", DialectInfo{
		Placeholder: "$%d",
		QuoteChar:   `"`,
		ShareLock:   "FOR SHARE",
		Returning:   "RETURNING",
		CastTypes: map[LocalType]string{
			LocalTypeString:   "TEXT",
			LocalTypeInt:      "INTEGER",
			LocalTypeInt32:    "INTEGER",
			LocalTypeInt64:    "BIGINT",
			LocalTypeBigInt:   "BIGINT",
			LocalTypeUint:     "BIGINT",
			LocalTypeUint32:   "BIGINT",
			LocalTypeUint64:   "NUMERIC(20)",
			LocalTypeFloat32:  "REAL",
			LocalTypeFloat64:  "DOUBLE PRECISION",
			LocalTypeBool:     "BOOLEAN",
			LocalTypeDate:     "DATE",
			LocalTypeDatetime: "TIMESTAMP",
			LocalTypeTime:     "TIME",
			LocalTypeJson:     "JSON",
			LocalTypeJsonb:    "JSONB",
			LocalTypeBytes:    "BYTEA",
			LocalTypeUUID:     "UUID",
		},
	})
	RegisterDialect("sqlite", DialectInfo{
		Placeholder: "?",
		QuoteChar:   `"`,
		Returning:   "RETURNING",
		CastTypes: map[LocalType]string{
			LocalTypeString:   "TEXT",
			LocalTypeInt:      "INTEGER",
			LocalTypeInt32:    "INTEGER",
			LocalTypeInt64:    "INTEGER",
			LocalTypeBigInt:   "INTEGER",
			LocalTypeUint:     "INTEGER",
			LocalTypeUint32:   "INTEGER",
			LocalTypeUint64:   "INTEGER",
			LocalTypeFloat32:  "REAL",
			LocalTypeFloat64:  "REAL",
			LocalTypeBool:     "INTEGER",
			LocalTypeDate:     "TEXT",
			LocalTypeDatetime: "TEXT",
			LocalTypeTime:     "TEXT",
			LocalTypeJson:     "TEXT",
			LocalTypeJsonb:    "TEXT",
			LocalTypeBytes:    "BLOB",
			LocalTypeUUID:     "TEXT",
		},
	})
}

func castTypeOf(rc *renderContext, t LocalType) string {
	if rc.dialectInfo != nil && rc.dialectInfo.CastTypes != nil {
		if v, ok := rc.dialectInfo.CastTypes[t]; ok {
			return v
		}
	}
	return strings.ToUpper(string(t))
}

func (rc *renderContext) placeholder(n int) string {
	if rc.dialectInfo != nil {
		if strings.Contains(rc.dialectInfo.Placeholder, "%d") {
			return fmt.Sprintf(rc.dialectInfo.Placeholder, n)
		}
		return rc.dialectInfo.Placeholder
	}
	return "?"
}

func (rc *renderContext) shareLockKeyword() string {
	if rc.dialectInfo != nil && rc.dialectInfo.ShareLock != "" {
		return rc.dialectInfo.ShareLock
	}
	return "FOR SHARE"
}
