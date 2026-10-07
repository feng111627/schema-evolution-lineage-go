package parser

import (
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// SQLParser 解析 SQL DDL 子集，版本通过行注释声明：
//
//	-- version: v1
//	CREATE TABLE orders (
//	  id BIGINT NOT NULL,
//	  amount BIGINT NULL COMMENT '金额',
//	  amount_cents BIGINT NOT NULL RENAMED_FROM amount
//	);
type SQLParser struct{}

func (SQLParser) Format() string { return "sql" }

func (SQLParser) Parse(data []byte) (*model.Schema, error) {
	text := string(data)
	version := ""
	var bodyLines []string

	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") {
			comment := strings.TrimSpace(strings.TrimPrefix(trimmed, "--"))
			if k, v, ok := strings.Cut(comment, ":"); ok && strings.TrimSpace(k) == "version" {
				version = strings.TrimSpace(v)
			}
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	if version == "" {
		return nil, newError(ErrMissingField, "sql", "version", "declare it with a '-- version: x' comment", nil)
	}

	body := strings.TrimSpace(strings.Join(bodyLines, " "))
	body = strings.TrimSuffix(body, ";")
	lower := strings.ToLower(body)
	if !strings.HasPrefix(lower, "create table") {
		return nil, newError(ErrMalformed, "sql", "", "statement must start with CREATE TABLE", nil)
	}
	rest := strings.TrimSpace(body[len("create table"):])

	open := strings.IndexByte(rest, '(')
	if open < 0 || !strings.HasSuffix(rest, ")") {
		return nil, newError(ErrMalformed, "sql", "", "CREATE TABLE requires a parenthesised column list", nil)
	}
	tableName := strings.Trim(strings.TrimSpace(rest[:open]), "`\"[]")
	if tableName == "" {
		return nil, newError(ErrMissingField, "sql", "table", "table name is required", nil)
	}
	colText := rest[open+1 : len(rest)-1]
	parts := splitColumnDefs(colText)
	if len(parts) == 0 {
		return nil, newError(ErrEmptyEntry, "sql", "columns", "at least one column is required", nil)
	}

	schema := &model.Schema{Name: tableName, Format: "sql", Version: version}
	seen := map[string]struct{}{}
	for i, part := range parts {
		col, err := parseSQLColumn(part, i)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[col.Name]; dup {
			return nil, newError(ErrDuplicateEntry, "sql", "columns["+itoaSQL(i)+"]("+col.Name+")", "column name appears more than once", nil)
		}
		seen[col.Name] = struct{}{}
		schema.Columns = append(schema.Columns, col)
	}
	return schema, nil
}

// splitColumnDefs 按顶层逗号拆分列定义，忽略类型长度括号内的逗号。
func splitColumnDefs(s string) []string {
	var defs []string
	depth := 0
	var cur strings.Builder
	inStr := false
	for _, r := range s {
		switch {
		case r == '\'':
			inStr = !inStr
			cur.WriteRune(r)
		case inStr:
			cur.WriteRune(r)
		case r == '(':
			depth++
			cur.WriteRune(r)
		case r == ')':
			depth--
			cur.WriteRune(r)
		case r == ',' && depth == 0:
			if d := strings.TrimSpace(cur.String()); d != "" {
				defs = append(defs, d)
			}
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if d := strings.TrimSpace(cur.String()); d != "" {
		defs = append(defs, d)
	}
	return defs
}

func parseSQLColumn(def string, idx int) (model.Column, error) {
	entry := "columns[" + itoaSQL(idx) + "]"
	tokens := tokenizeSQL(def)
	if len(tokens) < 2 {
		return model.Column{}, newError(ErrMissingField, "sql", entry, "column requires at least 'name TYPE'", nil)
	}

	first := strings.ToLower(tokens[0])
	switch first {
	case "primary", "foreign", "unique", "constraint", "key", "check":
		return model.Column{}, newError(ErrMalformed, "sql", entry, "table-level constraints are not supported in the SQL subset", nil)
	}

	col := model.Column{Name: strings.Trim(tokens[0], "`\"[]"), Nullable: true}
	norm, ok := normalizeType(tokens[1])
	if !ok {
		return model.Column{}, newError(ErrTypeMismatch, "sql", entry+"("+col.Name+").type", "unrecognized type: "+tokens[1], nil)
	}
	col.Type = norm

	i := 2
	for i < len(tokens) {
		tok := tokens[i]
		upper := strings.ToUpper(tok)
		switch upper {
		case "NULL":
			col.Nullable = true
			i++
		case "NOT":
			if i+1 >= len(tokens) || strings.ToUpper(tokens[i+1]) != "NULL" {
				return model.Column{}, newError(ErrMalformed, "sql", entry+"("+col.Name+")", `expected NULL after NOT`, nil)
			}
			col.Nullable = false
			i += 2
		case "COMMENT":
			if i+1 >= len(tokens) {
				return model.Column{}, newError(ErrMalformed, "sql", entry+"("+col.Name+").COMMENT", "comment text is missing", nil)
			}
			i += 2
		case "RENAMED_FROM":
			if i+1 >= len(tokens) {
				return model.Column{}, newError(ErrMissingField, "sql", entry+"("+col.Name+").RENAMED_FROM", "source column name is missing", nil)
			}
			col.RenamedFrom = strings.Trim(tokens[i+1], "`\"[]")
			i += 2
		default:
			return model.Column{}, newError(ErrMalformed, "sql", entry+"("+col.Name+")", "unexpected token: "+tok, nil)
		}
	}
	return col, nil
}

// tokenizeSQL 把列定义切成 token，单引号字符串作为整体。
func tokenizeSQL(def string) []string {
	var toks []string
	var cur strings.Builder
	inStr := false
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for _, r := range def {
		switch {
		case r == '\'':
			inStr = !inStr
			cur.WriteRune(r)
		case inStr:
			cur.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return toks
}

func itoaSQL(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
