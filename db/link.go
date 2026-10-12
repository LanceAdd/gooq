package db

import (
	"fmt"
	"regexp"
	"strings"
)

// linkPattern matches type:user:pass@protocol(host:port)/name?extra.
var linkPattern = regexp.MustCompile(`^(\w+):(.*?):(.*?)@(\w+?)\((.+?)\)/{0,1}([^\?]*)\?{0,1}(.*?)$`)

// ParsedLink is a GoFrame database link split into its parts.
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

// splitHostPort splits host:port. Brackets mark an IPv6 host, so colons inside
// them are not a port separator. The port is the :port after the closing bracket.
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
