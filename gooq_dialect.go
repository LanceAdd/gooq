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
	})
	RegisterDialect("pgsql", DialectInfo{
		Placeholder: "$%d",
		QuoteChar:   `"`,
		ShareLock:   "FOR SHARE",
		Returning:   "RETURNING",
	})
	RegisterDialect("sqlite", DialectInfo{
		Placeholder: "?",
		QuoteChar:   `"`,
		Returning:   "RETURNING",
	})
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
