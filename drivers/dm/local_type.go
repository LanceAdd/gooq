package dm

import (
	"strings"

	"github.com/lanceadd/gooq"
	"github.com/lanceadd/gooq/db"
)

// localTypes 是达梦类型名到本地类型的映射。
// 先去掉括号宽度，再按整段名字匹配，避免 INTERVAL 因为名字里有 int 被当成整数。
var localTypes = map[string]gooq.LocalType{
	"char": gooq.LocalTypeString, "character": gooq.LocalTypeString,
	"varchar": gooq.LocalTypeString, "varchar2": gooq.LocalTypeString,
	"nchar": gooq.LocalTypeString, "nvarchar": gooq.LocalTypeString,
	"nvarchar2": gooq.LocalTypeString,
	"text":      gooq.LocalTypeString, "long": gooq.LocalTypeString,
	"longvarchar": gooq.LocalTypeString, "clob": gooq.LocalTypeString,
	"rowid": gooq.LocalTypeString, "byte": gooq.LocalTypeString,
	"image": gooq.LocalTypeString, "bfile": gooq.LocalTypeString,

	"numeric": gooq.LocalTypeDecimal, "decimal": gooq.LocalTypeDecimal,
	"dec": gooq.LocalTypeDecimal, "number": gooq.LocalTypeDecimal,
	"money": gooq.LocalTypeDecimal,

	"int": gooq.LocalTypeInt, "integer": gooq.LocalTypeInt,
	"bigint": gooq.LocalTypeInt64, "smallint": gooq.LocalTypeInt,
	"tinyint": gooq.LocalTypeInt, "pls_integer": gooq.LocalTypeInt,

	"float": gooq.LocalTypeFloat64, "double": gooq.LocalTypeFloat64,
	"double precision": gooq.LocalTypeFloat64, "real": gooq.LocalTypeFloat64,

	"bit": gooq.LocalTypeBool, "boolean": gooq.LocalTypeBool, "bool": gooq.LocalTypeBool,
	"binary": gooq.LocalTypeBytes, "varbinary": gooq.LocalTypeBytes, "raw": gooq.LocalTypeBytes,
	"longvarbinary": gooq.LocalTypeBytes, "blob": gooq.LocalTypeBytes,

	"json": gooq.LocalTypeJson, "jsonb": gooq.LocalTypeJsonb,

	"date": gooq.LocalTypeDate, "time": gooq.LocalTypeTime,
	"timestamp": gooq.LocalTypeDatetime, "datetime": gooq.LocalTypeDatetime,
	"time with time zone":            gooq.LocalTypeDatetime,
	"datetime with time zone":        gooq.LocalTypeDatetime,
	"timestamp with time zone":       gooq.LocalTypeDatetime,
	"timestamp with local time zone": gooq.LocalTypeDatetime,

	"interval":                  gooq.LocalTypeString,
	"interval year":             gooq.LocalTypeString,
	"interval year to month":    gooq.LocalTypeString,
	"interval month":            gooq.LocalTypeString,
	"interval day":              gooq.LocalTypeString,
	"interval day to hour":      gooq.LocalTypeString,
	"interval day to minute":    gooq.LocalTypeString,
	"interval day to second":    gooq.LocalTypeString,
	"interval hour":             gooq.LocalTypeString,
	"interval hour to minute":   gooq.LocalTypeString,
	"interval hour to second":   gooq.LocalTypeString,
	"interval minute":           gooq.LocalTypeString,
	"interval minute to second": gooq.LocalTypeString,
	"interval second":           gooq.LocalTypeString,
}

func localTypeOf(dbType string) gooq.LocalType {
	typed := strings.ToLower(strings.TrimSpace(dbType))
	if index := strings.IndexByte(typed, '('); index > 0 {
		typed = strings.TrimSpace(typed[:index])
	}
	if localType, ok := localTypes[typed]; ok {
		return localType
	}
	return db.LocalTypeOf(dbType)
}
