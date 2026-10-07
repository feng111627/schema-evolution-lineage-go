package parse

import (
	"sort"
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// Top-level member names allowed on the two document shapes. Anything else is
// a syntax rejection (a typo must not be silently ignored).
var (
	catalogAllowed = map[string]bool{"schemas": true}
	schemaAllowed  = map[string]bool{"name": true, "columns": true}
	columnAllowed  = map[string]bool{
		"name": true, "id": true, "type": true,
		"nullable": true, "has_default": true,
		"aliases": true, "sources": true,
	}
)

var validTypes = map[model.DataType]bool{
	model.TypeBool: true,
	model.TypeInt:  true,
	model.TypeLong: true,
	model.TypeFlt:  true,
	model.TypeDbl:  true,
	model.TypeStr:  true,
}

func normalizeCatalog(root *node, origin string, xmlMode bool) (*model.Catalog, []*RejectError) {
	var errs []*RejectError
	if root == nil || root.kind != nObj {
		return nil, []*RejectError{{
			Reason:  ReasonSyntax,
			Message: "top-level value must be an object",
		}}
	}
	errs = rejectUnknownMembers(root, "", catalogAllowed, errs)
	rawSchemas := root.field("schemas")
	if rawSchemas == nil {
		errs = append(errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    "schemas",
			Message: `catalog requires a "schemas" array`,
		})
		return nil, errs
	}
	if rawSchemas.kind != nArr {
		errs = append(errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    "schemas",
			Message: `"schemas" must be an array`,
		})
		return nil, errs
	}
	cat := &model.Catalog{}
	for i, sn := range rawSchemas.arr {
		sch, sErrs := readSchema(sn, "schemas["+itoa(i)+"]", origin, xmlMode)
		errs = append(errs, sErrs...)
		if sch != nil {
			cat.Schemas = append(cat.Schemas, *sch)
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return cat, nil
}

func normalizeSchema(root *node, origin string, xmlMode bool) (*model.Schema, []*RejectError) {
	if root == nil || root.kind != nObj {
		return nil, []*RejectError{{
			Reason:  ReasonSyntax,
			Message: "top-level value must be an object",
		}}
	}
	var errs []*RejectError
	errs = rejectUnknownMembers(root, "", schemaAllowed, errs)
	sch, sErrs := readSchema(root, "", origin, xmlMode)
	errs = append(errs, sErrs...)
	if len(errs) > 0 {
		return nil, errs
	}
	return sch, nil
}

// readSchema converts one schema node. It always appends all detected errors;
// it returns a partially built schema (or nil) so callers can continue.
func readSchema(n *node, path, origin string, xmlMode bool) (*model.Schema, []*RejectError) {
	var errs []*RejectError
	if n == nil || n.kind != nObj {
		errs = append(errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    dotPath(path),
			Message: "schema entry must be an object",
		})
		return nil, errs
	}
	errs = rejectUnknownMembers(n, path, schemaAllowed, errs)

	sch := &model.Schema{Origin: origin}
	nameNode := n.field("name")
	if nameNode == nil {
		errs = append(errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    dotPath(path, "name"),
			Message: `schema requires a non-empty "name"`,
		})
	} else if nameNode.kind != nStr {
		errs = append(errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    dotPath(path, "name"),
			Message: `"name" must be a string`,
		})
	} else if nameNode.str == "" {
		errs = append(errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    dotPath(path, "name"),
			Message: `schema requires a non-empty "name"`,
		})
	} else {
		sch.Name = nameNode.str
	}

	colsNode := n.field("columns")
	if colsNode == nil {
		errs = append(errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    dotPath(path, "columns"),
			Message: `schema requires a "columns" array`,
		})
		return sch, errs
	}
	if colsNode.kind != nArr {
		errs = append(errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    dotPath(path, "columns"),
			Message: `"columns" must be an array`,
		})
		return sch, errs
	}
	seen := map[string]int{}
	for i, cn := range colsNode.arr {
		cp := dotPath(path, "columns["+itoa(i)+"]")
		col, cErrs := readColumn(cn, cp, xmlMode)
		errs = append(errs, cErrs...)
		if col != nil {
			if col.Name != "" {
				if first, dup := seen[col.Name]; dup {
					errs = append(errs, &RejectError{
						Reason: ReasonDuplicateName,
						Path:   cp + ".name",
						Message: "duplicate column name " + quote(col.Name) +
							"; first seen at columns[" + itoa(first) + "]",
					})
				} else {
					seen[col.Name] = i
				}
			}
			col.Origin = origin
			sch.Columns = append(sch.Columns, *col)
		}
	}
	return sch, errs
}

func readColumn(n *node, path string, xmlMode bool) (*model.Column, []*RejectError) {
	var errs []*RejectError
	if n == nil || n.kind != nObj {
		return nil, []*RejectError{{
			Reason:  ReasonTypeMismatch,
			Path:    path,
			Message: "column entry must be an object",
		}}
	}
	errs = rejectUnknownMembers(n, path, columnAllowed, errs)
	col := &model.Column{}

	if v, ok := requiredNonEmptyString(n, "name", path, &errs); ok {
		col.Name = v
	}
	if nameNode := n.field("name"); nameNode != nil && nameNode.kind == nStr && nameNode.str == "" {
		errs = append(errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    path + ".name",
			Message: `column requires a non-empty "name"`,
		})
	}
	if v, present := optionalString(n, "id", path, &errs); present {
		col.ID = v
	}
	if t, present := requiredType(n, path, &errs); present {
		col.Type = t
	}
	if v, present := optionalBool(n, "nullable", path, &errs, xmlMode); present {
		col.Nullable = v
	}
	if v, present := optionalBool(n, "has_default", path, &errs, xmlMode); present {
		col.HasDefault = v
	}
	if an := n.field("aliases"); an != nil {
		aliases, aErrs := readStringList(an, path+".aliases", xmlMode, "name")
		errs = append(errs, aErrs...)
		col.Aliases = aliases
	}
	if sn := n.field("sources"); sn != nil {
		refs, sErrs := readRefList(sn, path+".sources", xmlMode)
		errs = append(errs, sErrs...)
		col.Sources = refs
	}
	return col, errs
}

func requiredType(n *node, path string, errs *[]*RejectError) (model.DataType, bool) {
	vn := n.field("type")
	if vn == nil {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    path + ".type",
			Message: `column requires a "type"`,
		})
		return "", false
	}
	if vn.kind != nStr {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    path + ".type",
			Message: `"type" must be a string`,
		})
		return "", false
	}
	t := model.DataType(vn.str)
	if !validTypes[t] {
		supported := make([]string, 0, len(validTypes))
		for t := range validTypes {
			supported = append(supported, string(t))
		}
		sort.Strings(supported)
		*errs = append(*errs, &RejectError{
			Reason: ReasonUnsupportedType,
			Path:   path + ".type",
			Message: "unsupported column type " + quote(vn.str) +
				"; supported: " + strings.Join(supported, ", "),
		})
		return "", false
	}
	return t, true
}

func readRefList(n *node, path string, xmlMode bool) ([]model.ColumnRef, []*RejectError) {
	var errs []*RejectError
	if n.kind != nArr {
		return nil, []*RejectError{{
			Reason:  ReasonTypeMismatch,
			Path:    path,
			Message: `"sources" must be an array`,
		}}
	}
	var refs []model.ColumnRef
	for i, en := range n.arr {
		ep := path + "[" + itoa(i) + "]"
		text := en
		if xmlMode {
			if en.kind != nObj {
				errs = append(errs, &RejectError{
					Reason:  ReasonTypeMismatch,
					Path:    ep,
					Message: `source reference must be a <source ref="dataset.column"/> element`,
				})
				continue
			}
			if rf := en.field("ref"); rf == nil {
				errs = append(errs, &RejectError{
					Reason:  ReasonMissingRequired,
					Path:    ep + ".ref",
					Message: `source element requires a "ref" attribute`,
				})
				continue
			} else {
				text = rf
			}
		}
		if text.kind != nStr {
			errs = append(errs, &RejectError{
				Reason:  ReasonTypeMismatch,
				Path:    ep,
				Message: "source reference must be a string",
			})
			continue
		}
		ref, ok := parseRef(text.str)
		if !ok {
			errs = append(errs, &RejectError{
				Reason: ReasonInvalidRef,
				Path:   ep,
				Message: "malformed source reference " + quote(en.str) +
					`; expected the qualified form "dataset.column"`,
			})
			continue
		}
		refs = append(refs, ref)
	}
	return refs, errs
}

// parseRef accepts exactly "dataset.column" with both sides non-empty and no
// extra dots.
func parseRef(s string) (model.ColumnRef, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return model.ColumnRef{}, false
	}
	return model.ColumnRef{Dataset: parts[0], Column: parts[1]}, true
}

func readStringList(n *node, path string, xmlMode bool, xmlAttr string) ([]string, []*RejectError) {
	var errs []*RejectError
	if n.kind != nArr {
		return nil, []*RejectError{{
			Reason:  ReasonTypeMismatch,
			Path:    path,
			Message: path[strings.LastIndex(path, ".")+1:] + " must be an array of strings",
		}}
	}
	var out []string
	for i, en := range n.arr {
		ep := path + "[" + itoa(i) + "]"
		text := en
		if xmlMode {
			if en.kind != nObj {
				errs = append(errs, &RejectError{
					Reason:  ReasonTypeMismatch,
					Path:    ep,
					Message: "array element must be an element with a string attribute",
				})
				continue
			}
			text = en.field(xmlAttr)
			if text == nil {
				errs = append(errs, &RejectError{
					Reason:  ReasonMissingRequired,
					Path:    ep + "." + xmlAttr,
					Message: "element requires a " + quote(xmlAttr) + " attribute",
				})
				continue
			}
		}
		if text.kind != nStr {
			errs = append(errs, &RejectError{
				Reason:  ReasonTypeMismatch,
				Path:    ep,
				Message: "array element must be a string",
			})
			continue
		}
		if text.str == "" {
			errs = append(errs, &RejectError{
				Reason:  ReasonMissingRequired,
				Path:    ep,
				Message: "array element must not be empty",
			})
			continue
		}
		out = append(out, text.str)
	}
	return out, errs
}

func rejectUnknownMembers(n *node, path string, allowed map[string]bool, errs []*RejectError) []*RejectError {
	for _, m := range n.obj {
		if !allowed[m.key] {
			errs = append(errs, &RejectError{
				Reason:  ReasonSyntax,
				Path:    dotPath(path, m.key),
				Message: "unknown field " + quote(m.key),
			})
		}
	}
	return errs
}

func requireString(n *node, path string, errs *[]*RejectError) (string, bool) {
	if n.kind != nStr {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    path,
			Message: "value must be a string",
		})
		return "", false
	}
	return n.str, true
}

func requiredNonEmptyString(obj *node, key, path string, errs *[]*RejectError) (string, bool) {
	n := obj.field(key)
	if n == nil {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonMissingRequired,
			Path:    path + "." + key,
			Message: `column requires a non-empty "` + key + `"`,
		})
		return "", false
	}
	if n.kind != nStr {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    path + "." + key,
			Message: `"` + key + `" must be a string`,
		})
		return "", false
	}
	if n.str == "" {
		return "", true // emptiness reported separately with stable wording
	}
	return n.str, true
}

func optionalString(obj *node, key, path string, errs *[]*RejectError) (string, bool) {
	n := obj.field(key)
	if n == nil || n.kind == nNull {
		return "", false
	}
	if n.kind != nStr {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    path + "." + key,
			Message: `"` + key + `" must be a string`,
		})
		return "", true
	}
	return n.str, true
}

func optionalBool(obj *node, key, path string, errs *[]*RejectError, xmlMode bool) (bool, bool) {
	n := obj.field(key)
	if n == nil || n.kind == nNull {
		return false, false
	}
	if xmlMode {
		// XML attribute values arrive as strings; only the literal spellings
		// true/false are accepted so that "yes"/"1" cannot slip through.
		if n.kind != nStr {
			*errs = append(*errs, &RejectError{
				Reason:  ReasonTypeMismatch,
				Path:    path + "." + key,
				Message: `"` + key + `" must be "true" or "false"`,
			})
			return false, false
		}
		switch n.str {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			*errs = append(*errs, &RejectError{
				Reason:  ReasonTypeMismatch,
				Path:    path + "." + key,
				Message: `"` + key + `" must be "true" or "false"`,
			})
			return false, false
		}
	}
	if n.kind != nBool {
		*errs = append(*errs, &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    path + "." + key,
			Message: `"` + key + `" must be a boolean (true/false)`,
		})
		return false, false
	}
	return n.str == "true", true
}

// dotPath joins an array-index path with a member name.
func dotPath(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 && p[0] != '[' {
			b.WriteByte('.')
		}
		b.WriteString(p)
	}
	return b.String()
}

func quote(s string) string {
	return `"` + s + `"`
}
