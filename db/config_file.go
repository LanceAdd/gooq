package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gmap"
	"github.com/gogf/gf/v2/container/gtype"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/glog"
	"github.com/gogf/gf/v2/util/gconv"
	"github.com/gogf/gf/v2/util/gvalid"
)

const runtimeConfigFileName = "gooq"

var (
	configLoaded       = gtype.NewBool(false)
	groupLoggerConfigs = gmap.NewKVMap[string, map[string]any](true)
)

// fileConfig 是配置文件里 gooq 节点的形状。
// gcfg 先用 gjson 把 yaml、toml、json 等解析成同一棵树，Scan 再填进这个结构体。
type fileConfig struct {
	Logger   loggerConfig    `json:"logger"`
	Database []databaseGroup `json:"database"`
}

// loggerConfig 只校验文件里的 level。其余 logger 字段原样交给 glog.SetConfigWithMap。
type loggerConfig struct {
	Level string `json:"level" v:"in:all,dev,develop,prod,product,debu,debug,info,noti,notice,warn,warning,erro,error,crit,critical"`
}

// databaseGroup 是一个配置组。Name 是 Instance 使用的组名。
type databaseGroup struct {
	Name      string           `json:"name" v:"required"`
	Endpoints []endpointConfig `json:"endpoints" v:"required"`
}

// endpointConfig 是一个连接节点。字段名与配置文件一致，并兼容 GoFrame 的连接池字段名。
type endpointConfig struct {
	Link             string        `json:"link" v:"required"`
	Role             string        `json:"role" v:"in:master,slave"`
	Weight           int           `json:"weight"`
	Debug            bool          `json:"debug"`
	DryRun           bool          `json:"dryRun"`
	Charset          string        `json:"charset"`
	Timezone         string        `json:"timezone"`
	MaxOpen          int           `json:"maxOpen"`
	MaxIdle          int           `json:"maxIdle"`
	MaxLifetime      time.Duration `json:"maxLifetime"`
	MaxIdleTime      time.Duration `json:"maxIdleTime"`
	MaxOpenConnCount int           `json:"maxOpenConnCount"`
	MaxIdleConnCount int           `json:"maxIdleConnCount"`
	MaxConnLifeTime  time.Duration `json:"maxConnLifeTime"`
	MaxIdleConnTime  time.Duration `json:"maxIdleConnTime"`
}

func (e endpointConfig) toConfig() Config {
	cfg := Config{
		Link:            e.Link,
		Role:            Role(strings.ToLower(e.Role)),
		Weight:          e.Weight,
		Debug:           e.Debug,
		DryRun:          e.DryRun,
		Charset:         e.Charset,
		Timezone:        e.Timezone,
		MaxOpenConns:    e.MaxOpen,
		MaxIdleConns:    e.MaxIdle,
		ConnMaxLifetime: e.MaxLifetime,
		MaxIdleTime:     e.MaxIdleTime,
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = e.MaxOpenConnCount
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = e.MaxIdleConnCount
	}
	if cfg.ConnMaxLifetime == 0 {
		cfg.ConnMaxLifetime = e.MaxConnLifeTime
	}
	if cfg.MaxIdleTime == 0 {
		cfg.MaxIdleTime = e.MaxIdleConnTime
	}
	return cfg
}

// LoadConfig 通过 gcfg 读取 gooq.database 和 gooq.logger。
// database 是配置组列表。每组有 name 和 endpoints。name 是 Instance 使用的组名。
// 不传配置时只读取名为 gooq 的配置文件，不读取应用的 config 文件。
// 已经注册的组保持不变，只补写 logger。
// gooq.logger 配置本次从文件载入的每一组的 SQL 日志器。
// 节点上的 debug 会在该组打开后打开 SQL 日志。
func LoadConfig(ctx context.Context, source ...*gcfg.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	config := runtimeConfig()
	if len(source) > 0 && source[0] != nil {
		config = source[0]
	}
	if config == nil {
		return fmt.Errorf("gooq: configuration is unavailable")
	}
	value, err := config.Get(ctx, "gooq")
	if err != nil {
		return err
	}
	if value == nil || value.IsEmpty() {
		return nil
	}
	if err = rejectMapDatabase(value); err != nil {
		return err
	}
	var file fileConfig
	if err = value.Scan(&file); err != nil {
		return err
	}
	for i := range file.Database {
		file.Database[i].Name = strings.TrimSpace(file.Database[i].Name)
	}
	if err = gvalid.New().Ci().Data(file).Run(ctx); err != nil {
		return fmt.Errorf("gooq: %w", err)
	}
	loggerCfg := loggerMap(value)
	if len(loggerCfg) > 0 {
		if err = glog.New().SetConfigWithMap(loggerCfg); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(file.Database))
	for _, group := range file.Database {
		if seen[group.Name] {
			return fmt.Errorf("gooq: database group %q is duplicated", group.Name)
		}
		seen[group.Name] = true
		nodes := make([]Config, len(group.Endpoints))
		for i, endpoint := range group.Endpoints {
			nodes[i] = endpoint.toConfig()
		}
		if groupConfigs.Contains(group.Name) {
			if len(loggerCfg) == 0 {
				continue
			}
			groupMu.Lock()
			groupLoggerConfigs.Set(group.Name, loggerCfg)
			database := groupInstances.Get(group.Name)
			groupMu.Unlock()
			if database != nil {
				if err = applyGroupLogger(database); err != nil {
					return err
				}
			}
			continue
		}
		if err = SetConfig(group.Name, nodes...); err != nil {
			return err
		}
		if len(loggerCfg) > 0 {
			groupLoggerConfigs.Set(group.Name, loggerCfg)
		}
	}
	return nil
}

func rejectMapDatabase(value *gvar.Var) error {
	raw, ok := value.Map()["database"]
	if !ok {
		for key, item := range value.Map() {
			if strings.EqualFold(key, "database") {
				raw = item
				break
			}
		}
	}
	switch raw.(type) {
	case map[string]any, map[string]string, map[any]any:
		return fmt.Errorf("gooq: gooq.database must be a list of groups with name and endpoints")
	default:
		return nil
	}
}

func clearGroupLogger(group string) {
	groupLoggerConfigs.Remove(group)
}

func applyGroupLogger(database *DB) error {
	logger := database.Logger()
	if database == nil || logger == nil {
		return nil
	}
	cfg := groupLoggerConfigs.Get(database.group)
	if len(cfg) == 0 {
		return nil
	}
	return logger.SetConfigWithMap(cfg)
}

func loggerMap(value *gvar.Var) map[string]any {
	raw, ok := value.Map()["logger"]
	if !ok {
		for key, item := range value.Map() {
			if strings.EqualFold(key, "logger") {
				raw = item
				ok = true
				break
			}
		}
	}
	if !ok || raw == nil {
		return nil
	}
	return gconv.Map(raw)
}

// runtimeConfig 是运行时配置。文件名固定为 gooq，由 gcfg 按扩展名搜索。
func runtimeConfig() *gcfg.Config {
	return gcfg.Instance(runtimeConfigFileName)
}

func loadConfigOnce(ctx context.Context) error {
	if configLoaded.Val() {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	config := runtimeConfig()
	if config != nil && config.Available(ctx) {
		if err := LoadConfig(ctx, config); err != nil && gerror.Code(err) != gcode.CodeNotFound {
			return err
		}
	}
	configLoaded.Set(true)
	return nil
}
