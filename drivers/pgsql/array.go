package pgsql

import (
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/lanceadd/gooq/db"
)

func (postgresDriver) Bind(v any) any {
	switch typed := v.(type) {
	case []bool, []float32, []float64, []int64, []int32, []string, [][]byte:
		return pq.Array(typed)
	case []int:
		copied := make([]int64, len(typed))
		for i, n := range typed {
			copied[i] = int64(n)
		}
		return pq.Array(copied)
	case []uuid.UUID:
		return pq.GenericArray{A: typed}
	default:
		return v
	}
}

func (postgresDriver) Decode(v any) any {
	column, ok := v.(db.ColumnValue)
	if !ok {
		if decoded, handled := decodeArrayValue("", v); handled {
			return decoded
		}
		return v
	}
	if decoded, handled := decodeArrayValue(column.DBType, column.Value); handled {
		return decoded
	}
	return column
}

func decodeArrayValue(dbType string, value any) (any, bool) {
	elem, ok := arrayElementType(dbType)
	if !ok || value == nil {
		return nil, false
	}
	text, ok := arrayText(value)
	if !ok {
		return nil, false
	}
	decoded, err := parsePGArray(elem, text)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func arrayElementType(dbType string) (string, bool) {
	typed := strings.ToLower(strings.TrimSpace(dbType))
	switch {
	case strings.HasPrefix(typed, "_"):
		typed = strings.TrimPrefix(typed, "_")
	case strings.HasSuffix(typed, "[]"):
		typed = strings.TrimSuffix(typed, "[]")
	default:
		return "", false
	}
	switch typed {
	case "int2", "int4", "int", "int8", "text", "varchar", "bpchar", "bool", "float4", "float8", "bytea", "uuid":
		return typed, true
	default:
		return "", false
	}
}

func arrayText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		return "", false
	}
}

func parsePGArray(elem, text string) (any, error) {
	elements, err := splitPGArray(text)
	if err != nil {
		return nil, err
	}
	switch elem {
	case "int2", "int4", "int":
		out := make([]int, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			n, err := strconv.ParseInt(*element, 10, 0)
			if err != nil {
				return nil, err
			}
			out[i] = int(n)
		}
		return out, nil
	case "int8":
		out := make([]int64, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			n, err := strconv.ParseInt(*element, 10, 64)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case "text", "varchar", "bpchar":
		out := make([]string, len(elements))
		for i, element := range elements {
			if element != nil {
				out[i] = *element
			}
		}
		return out, nil
	case "bool":
		out := make([]bool, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			switch strings.ToLower(*element) {
			case "t", "true", "1":
				out[i] = true
			case "f", "false", "0":
				out[i] = false
			default:
				return nil, strconv.ErrSyntax
			}
		}
		return out, nil
	case "float4":
		out := make([]float32, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			n, err := strconv.ParseFloat(*element, 32)
			if err != nil {
				return nil, err
			}
			out[i] = float32(n)
		}
		return out, nil
	case "float8":
		out := make([]float64, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			n, err := strconv.ParseFloat(*element, 64)
			if err != nil {
				return nil, err
			}
			out[i] = n
		}
		return out, nil
	case "bytea":
		out := make([][]byte, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			raw := *element
			if strings.HasPrefix(raw, `\x`) {
				raw = strings.TrimPrefix(raw, `\x`)
				decoded, err := hex.DecodeString(raw)
				if err != nil {
					return nil, err
				}
				out[i] = decoded
				continue
			}
			out[i] = []byte(raw)
		}
		return out, nil
	case "uuid":
		out := make([]uuid.UUID, len(elements))
		for i, element := range elements {
			if element == nil {
				continue
			}
			id, err := uuid.Parse(*element)
			if err != nil {
				return nil, err
			}
			out[i] = id
		}
		return out, nil
	default:
		return nil, strconv.ErrSyntax
	}
}

// splitPGArray parses a one-dimensional PostgreSQL array literal.
// A nil element means SQL NULL. An empty array returns an empty slice.
func splitPGArray(text string) ([]*string, error) {
	text = strings.TrimSpace(text)
	if len(text) < 2 || text[0] != '{' || text[len(text)-1] != '}' {
		return nil, strconv.ErrSyntax
	}
	body := text[1 : len(text)-1]
	if body == "" {
		return []*string{}, nil
	}
	var (
		out       []*string
		buf       strings.Builder
		quoted    bool
		wasQuoted bool
		escape    bool
		token     bool
	)
	flush := func() {
		if !token {
			out = append(out, nil)
			return
		}
		value := buf.String()
		buf.Reset()
		token = false
		if !wasQuoted && strings.EqualFold(value, "NULL") {
			wasQuoted = false
			out = append(out, nil)
			return
		}
		wasQuoted = false
		copied := value
		out = append(out, &copied)
	}
	for i := 0; i < len(body); i++ {
		ch := body[i]
		if escape {
			buf.WriteByte(ch)
			token = true
			escape = false
			continue
		}
		if quoted {
			switch ch {
			case '\\':
				escape = true
			case '"':
				quoted = false
			default:
				buf.WriteByte(ch)
				token = true
			}
			continue
		}
		switch ch {
		case '"':
			quoted = true
			token = true
			wasQuoted = true
		case '\\':
			escape = true
		case ',':
			flush()
		case ' ', '\n', '\t':
			if token {
				buf.WriteByte(ch)
			}
		default:
			if ch == '{' {
				return nil, strconv.ErrSyntax
			}
			buf.WriteByte(ch)
			token = true
		}
	}
	if quoted || escape {
		return nil, strconv.ErrSyntax
	}
	flush()
	return out, nil
}
