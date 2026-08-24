package gooq

import (
	"fmt"
	"strings"
	"sync"
)

type PaginationStyle int

const (
	PaginationLimit PaginationStyle = iota
	PaginationFetch
)

type DialectInfo struct {
	Placeholder string
	QuoteChar   string
	Pagination  PaginationStyle
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
		Pagination:  PaginationLimit,
		ShareLock:   "LOCK IN SHARE MODE",
	})
	RegisterDialect("pgsql", DialectInfo{
		Placeholder: "$%d",
		QuoteChar:   `"`,
		Pagination:  PaginationLimit,
		ShareLock:   "FOR SHARE",
		Returning:   "RETURNING",
	})
	RegisterDialect("sqlite", DialectInfo{
		Placeholder: "?",
		QuoteChar:   `"`,
		Pagination:  PaginationLimit,
		Returning:   "RETURNING",
	})
	RegisterDialect("oracle", DialectInfo{
		Placeholder: ":%d",
		QuoteChar:   `"`,
		Pagination:  PaginationFetch,
	})
	RegisterDialect("mssql", DialectInfo{
		Placeholder: "?",
		QuoteChar:   `"`,
		Pagination:  PaginationFetch,
		Returning:   "OUTPUT",
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
