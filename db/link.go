package db

import (
	"fmt"
	"regexp"
	"strings"
)

// linkPattern 匹配 type:user:pass@protocol(host:port)/name?extra。
var linkPattern = regexp.MustCompile(`^(\w+):(.*?):(.*?)@(\w+?)\((.+?)\)/{0,1}([^\?]*)\?{0,1}(.*?)$`)

// ParsedLink 是拆开后的 GoFrame 数据库链接。
type ParsedLink struct {
	Type     string
	User     string
	Pass     string
	Protocol string
	Host     string
	Port     string
	Name     string
	Extra    string
}

func parseLink(link string) (ParsedLink, error) {
	match := linkPattern.FindStringSubmatch(link)
	if len(match) < 7 {
		return ParsedLink{}, fmt.Errorf("gooq: invalid link %q, want type:user:pass@protocol(host:port)/name", link)
	}
	parsed := ParsedLink{
		Type:     match[1],
		User:     match[2],
		Pass:     match[3],
		Protocol: match[4],
		Extra:    match[7],
	}
	if parsed.Protocol == "file" {
		parsed.Name = match[5]
		return parsed, nil
	}
	parsed.Host, parsed.Port = splitHostPort(match[5])
	parsed.Name = match[6]
	return parsed, nil
}

// splitHostPort 拆分 host:port。方括号标出 IPv6 主机，括号内的冒号不作为端口分隔符。
// 端口是右括号后面的 :port。
func splitHostPort(hostPort string) (host, port string) {
	if strings.HasPrefix(hostPort, "[") {
		if end := strings.IndexByte(hostPort, ']'); end >= 0 {
			host = hostPort[1:end]
			rest := hostPort[end+1:]
			if strings.HasPrefix(rest, ":") {
				port = rest[1:]
			}
			return host, port
		}
	}
	parts := strings.Split(hostPort, ":")
	host = parts[0]
	if len(parts) == 2 {
		port = parts[1]
	}
	return host, port
}
