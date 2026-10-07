package parser

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// DSLParser 解析行式契约描述：
//
//	schema: orders
//	version: v2
//	column: id long not null
//	column: amount_cents long null renamed_from=amount
type DSLParser struct{}

func (DSLParser) Format() string { return "dsl" }

func (DSLParser) Parse(data []byte) (*model.Schema, error) {
	schema := &model.Schema{Format: "dsl"}
	seen := map[string]struct{}{}
	hasSchema, hasVersion := false, false

	scanner := bufio.NewScanner(bytes.NewReader(data))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entry := lineEntry(lineNo)

		key, val, found := strings.Cut(line, ":")
		if !found {
			return nil, newError(ErrMalformed, "dsl", entry, "expected 'key: value' or 'column: ...' directive", nil)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		switch key {
		case "schema":
			if val == "" {
				return nil, newError(ErrMissingField, "dsl", entry+".schema", "must not be empty", nil)
			}
			schema.Name = val
			hasSchema = true
		case "version":
			if val == "" {
				return nil, newError(ErrMissingField, "dsl", entry+".version", "must not be empty", nil)
			}
			schema.Version = val
			hasVersion = true
		case "column":
			if val == "" {
				return nil, newError(ErrEmptyEntry, "dsl", entry, "column definition is empty", nil)
			}
			col, err := parseDSLColumn(val, lineNo)
			if err != nil {
				return nil, err
			}
			if _, dup := seen[col.Name]; dup {
				return nil, newError(ErrDuplicateEntry, "dsl", lineEntry(lineNo)+"("+col.Name+")", "column name appears more than once", nil)
			}
			seen[col.Name] = struct{}{}
			schema.Columns = append(schema.Columns, col)
		default:
			return nil, newError(ErrMalformed, "dsl", entry, "unknown directive: "+key, nil)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, newError(ErrMalformed, "dsl", "", "failed to read document", err)
	}

	if !hasSchema {
		return nil, newError(ErrMissingField, "dsl", "schema", "directive is required", nil)
	}
	if !hasVersion {
		return nil, newError(ErrMissingField, "dsl", "version", "directive is required", nil)
	}
	if len(schema.Columns) == 0 {
		return nil, newError(ErrEmptyEntry, "dsl", "column", "at least one column is required", nil)
	}
	return schema, nil
}

func lineEntry(n int) string { return "line " + strconv.Itoa(n) }

func parseDSLColumn(val string, lineNo int) (model.Column, error) {
	tokens := strings.Fields(val)
	if len(tokens) < 2 {
		return model.Column{}, newError(ErrMissingField, "dsl", lineEntry(lineNo), "column requires at least 'name type'", nil)
	}
	col := model.Column{Name: tokens[0], Nullable: true}
	norm, ok := normalizeType(tokens[1])
	if !ok {
		return model.Column{}, newError(ErrTypeMismatch, "dsl", lineEntry(lineNo)+"("+tokens[0]+").type", "unrecognized type: "+tokens[1], nil)
	}
	col.Type = norm

	for _, tok := range tokens[2:] {
		switch {
		case tok == "null":
			col.Nullable = true
		case tok == "not":
			// 需要与下一个 token 组合成 not null
		case tok == "notnull" || tok == "not_null":
			col.Nullable = false
		default:
			if strings.HasPrefix(tok, model.RenameMarker+"=") {
				from := strings.TrimPrefix(tok, model.RenameMarker+"=")
				if from == "" {
					return model.Column{}, newError(ErrMissingField, "dsl", lineEntry(lineNo)+"("+col.Name+").renamed_from", "must not be empty", nil)
				}
				col.RenamedFrom = from
				continue
			}
			return model.Column{}, newError(ErrMalformed, "dsl", lineEntry(lineNo)+"("+col.Name+")", "unexpected token: "+tok, nil)
		}
	}
	// 处理 "not null" 双 token 写法
	for i := 2; i+1 < len(tokens); i++ {
		if tokens[i] == "not" && tokens[i+1] == "null" {
			col.Nullable = false
		}
		if tokens[i] == "not" && tokens[i+1] != "null" {
			return model.Column{}, newError(ErrMalformed, "dsl", lineEntry(lineNo)+"("+col.Name+")", `expected "null" after "not"`, nil)
		}
	}
	return col, nil
}
