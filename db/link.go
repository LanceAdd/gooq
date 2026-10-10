package db

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// linkPattern matches type:user:pass@protocol(host:port)/name?extra.
var linkPattern = regexp.MustCompile(`^(\w+):(.*?):(.*?)@(\w+?)\((.+?)\)/{0,1}([^\?]*)\?{0,1}(.*?)$`)

type parsedLink struct {
	Type     string
	User     string
	Pass     string
	Protocol string
	Host     string
	Port     string
	Name     string
	Extra    string
}

func parseLink(link string) (parsedLink, error) {
	match := linkPattern.FindStringSubmatch(link)
	if len(match) < 7 {
		return parsedLink{}, fmt.Errorf("gooq: invalid link %q, want type:user:pass@protocol(host:port)/name", link)
	}
	parsed := parsedLink{
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

func mysqlDSN(link parsedLink) (string, error) {
	dsn := fmt.Sprintf("%s:%s@%s(%s:%s)/%s?parseTime=true", link.User, link.Pass, link.Protocol, link.Host, link.Port, link.Name)
	if link.Port == "" {
		dsn = fmt.Sprintf("%s:%s@%s(%s)/%s?parseTime=true", link.User, link.Pass, link.Protocol, link.Host, link.Name)
	}
	if link.Extra != "" {
		extra, err := url.ParseQuery(link.Extra)
		if err != nil {
			return "", fmt.Errorf("gooq: mysql link extra: %w", err)
		}
		if extra.Get("parseTime") == "" {
			extra.Set("parseTime", "true")
		}
		dsn = fmt.Sprintf("%s:%s@%s(%s:%s)/%s?%s", link.User, link.Pass, link.Protocol, link.Host, link.Port, link.Name, extra.Encode())
	}
	return dsn, nil
}

func postgresDSN(link parsedLink) (string, error) {
	port := link.Port
	if port == "" {
		port = "5432"
	}
	parts := []string{
		"host=" + link.Host,
		"port=" + port,
		"user=" + link.User,
		"password=" + link.Pass,
		"dbname=" + link.Name,
		"sslmode=disable",
	}
	if link.Extra != "" {
		extra, err := url.ParseQuery(link.Extra)
		if err != nil {
			return "", fmt.Errorf("gooq: pgsql link extra: %w", err)
		}
		if mode := extra.Get("sslmode"); mode != "" {
			parts[5] = "sslmode=" + mode
		}
	}
	return strings.Join(parts, " "), nil
}

func sqliteDSN(link parsedLink) (string, error) {
	path := link.Name
	if path == "" {
		return "", fmt.Errorf("gooq: sqlite link is missing a file path")
	}
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return path, nil
	}
	return "file:" + filepath.ToSlash(path), nil
}
