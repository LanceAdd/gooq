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
	hostPort := strings.Split(match[5], ":")
	parsed.Host = hostPort[0]
	if len(hostPort) == 2 {
		parsed.Port = hostPort[1]
	}
	parsed.Name = match[6]
	return parsed, nil
}
