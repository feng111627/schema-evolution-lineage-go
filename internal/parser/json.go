package parser

import (
	"encoding/json"
	"fmt"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// JSONParser 解析 JSON 格式的 schema 描述：
//
//	{
//	  "schema": "orders",
//	  "version": "v1",
//	  "columns": [
//	    {"name": "id", "type": "long", "nullable": false},
//	    {"name": "amount_cents", "type": "long", "renamed_from": "amount"}
//	  ]
//	}
type JSONParser struct{}

func (JSONParser) Format() string { return "json" }

func (JSONParser) Parse(data []byte) (*model.Schema, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		// 不把底层库报错原样透传：统一封装为 malformed_input。
		return nil, newError(ErrMalformed, "json", "", "document is not a valid JSON object", err)
	}

	name, err := requireString(top, "schema", "json")
	if err != nil {
		return nil, err
	}
	version, err := requireString(top, "version", "json")
	if err != nil {
		return nil, err
	}

	rawCols, ok := top["columns"]
	if !ok {
		return nil, newError(ErrMissingField, "json", "columns", "field is required", nil)
	}
	var colMsgs []json.RawMessage
	if err := json.Unmarshal(rawCols, &colMsgs); err != nil {
		return nil, newError(ErrTypeMismatch, "json", "columns", "must be an array of column objects", err)
	}
	if len(colMsgs) == 0 {
		return nil, newError(ErrEmptyEntry, "json", "columns", "at least one column is required", nil)
	}

	seen := make(map[string]struct{}, len(colMsgs))
	cols := make([]model.Column, 0, len(colMsgs))
	for i, rc := range colMsgs {
		entry := fmt.Sprintf("columns[%d]", i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(rc, &fields); err != nil {
			return nil, newError(ErrTypeMismatch, "json", entry, "column must be a JSON object", err)
		}

		colName, perr := requireString(fields, "name", "json")
		if perr != nil {
			perr.Entry = entry + "." + perr.Entry
			return nil, perr
		}
		entry = fmt.Sprintf("columns[%d](%s)", i, colName)
		if _, dup := seen[colName]; dup {
			return nil, newError(ErrDuplicateEntry, "json", entry, "column name appears more than once", nil)
		}
		seen[colName] = struct{}{}

		rawType, ok := fields["type"]
		if !ok {
			return nil, newError(ErrMissingField, "json", entry+".type", "field is required", nil)
		}
		var typeStr string
		if err := json.Unmarshal(rawType, &typeStr); err != nil {
			return nil, newError(ErrTypeMismatch, "json", entry+".type", "must be a string", err)
		}
		norm, ok := normalizeType(typeStr)
		if !ok {
			return nil, newError(ErrTypeMismatch, "json", entry+".type", "unrecognized type: "+typeStr, nil)
		}

		nullable := true
		if rawNullable, has := fields["nullable"]; has {
			if err := json.Unmarshal(rawNullable, &nullable); err != nil {
				return nil, newError(ErrTypeMismatch, "json", entry+".nullable", "must be a boolean", err)
			}
		}

		col := model.Column{Name: colName, Type: norm, Nullable: nullable}
		if rawRename, has := fields[model.RenameMarker]; has {
			if err := json.Unmarshal(rawRename, &col.RenamedFrom); err != nil {
				return nil, newError(ErrTypeMismatch, "json", entry+"."+model.RenameMarker, "must be a string", err)
			}
		}
		cols = append(cols, col)
	}

	return &model.Schema{Name: name, Format: "json", Version: version, Columns: cols}, nil
}

func requireString(m map[string]json.RawMessage, field, format string) (string, *Error) {
	raw, ok := m[field]
	if !ok {
		return "", newError(ErrMissingField, format, field, "field is required", nil)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", newError(ErrTypeMismatch, format, field, "must be a string", err)
	}
	if s == "" {
		return "", newError(ErrMissingField, format, field, "must not be empty", nil)
	}
	return s, nil
}
