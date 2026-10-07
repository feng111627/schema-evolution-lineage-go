// Package parse reads the supported local schema/contract description formats
// and normalizes them into the model package.
//
// Two formats are supported:
//
//	JSON - a {"catalog":{"schemas":[...]}} style contract, or a bare schema.
//	XML  - the <catalog>/<schema> dialect described in docs/formats.md.
//
// All failures are reported as *RejectError / *RejectErrors so callers can
// distinguish malformed syntax, missing required fields and type mismatches.
// Underlying library errors are summarized and never exposed verbatim.
package parse

import (
	"errors"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// Format selects the input dialect.
type Format string

const (
	// FormatAuto detects the format from the first non-space byte
	// ('{' => JSON, '<' => XML).
	FormatAuto Format = ""
	FormatJSON Format = "json"
	FormatXML  Format = "xml"
)

// Options controls parsing.
type Options struct {
	Format Format
	// Origin is attached to the resulting schemas for conflict diagnostics
	// (typically the file path).
	Origin string
}

// ParseCatalog parses a document describing one or more datasets. Both
// formats accept either a catalog wrapper or a single bare schema, which is
// returned as a one-schema catalog.
func ParseCatalog(data []byte, opts Options) (*model.Catalog, error) {
	root, isXML, err := decodeDocument(data, opts.Format)
	if err != nil {
		return nil, err
	}
	origin := opts.Origin
	if origin == "" {
		origin = "catalog"
	}
	switch classify(root) {
	case docSchema:
		sch, errs := normalizeSchema(root, origin, isXML)
		if len(errs) > 0 {
			return nil, &RejectErrors{Items: errs}
		}
		return &model.Catalog{Schemas: []model.Schema{*sch}}, nil
	case docInvalid:
		return nil, &RejectError{
			Reason:  ReasonSyntax,
			Message: `document is neither a catalog (with "schemas") nor a schema (with "columns")`,
		}
	}
	cat, errs := normalizeCatalog(root, origin, isXML)
	if len(errs) > 0 {
		return nil, &RejectErrors{Items: errs}
	}
	return cat, nil
}

// ParseSchema parses a single-dataset document. A catalog wrapper containing
// exactly one schema is accepted as well.
func ParseSchema(data []byte, opts Options) (*model.Schema, error) {
	root, isXML, err := decodeDocument(data, opts.Format)
	if err != nil {
		return nil, err
	}
	origin := opts.Origin
	if origin == "" {
		origin = "schema"
	}
	switch classify(root) {
	case docSchema:
		sch, errs := normalizeSchema(root, origin, isXML)
		if len(errs) > 0 {
			return nil, &RejectErrors{Items: errs}
		}
		return sch, nil
	case docInvalid:
		return nil, &RejectError{
			Reason:  ReasonSyntax,
			Message: `document is neither a catalog (with "schemas") nor a schema (with "columns")`,
		}
	}
	cat, errs := normalizeCatalog(root, origin, isXML)
	if len(errs) > 0 {
		return nil, &RejectErrors{Items: errs}
	}
	if len(cat.Schemas) != 1 {
		return nil, &RejectError{
			Reason: ReasonSyntax,
			Message: "expected a single schema document but the catalog contains " +
				itoa(len(cat.Schemas)) + " schemas",
		}
	}
	sch := cat.Schemas[0]
	return &sch, nil
}

// AsRejectErrors unwraps a parse error into the structured rejection list.
// A single *RejectError is returned as a one-element list.
func AsRejectErrors(err error) []*RejectError {
	if err == nil {
		return nil
	}
	var agg *RejectErrors
	if errors.As(err, &agg) {
		return agg.Items
	}
	var one *RejectError
	if errors.As(err, &one) {
		return []*RejectError{one}
	}
	return []*RejectError{{
		Reason:  ReasonSyntax,
		Message: "unclassified parse failure",
		Detail:  safeDetail(err),
	}}
}

func safeDetail(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func decodeDocument(data []byte, f Format) (root *node, isXML bool, err error) {
	switch f {
	case FormatJSON:
		root, err = decodeJSON(data)
	case FormatXML:
		root, err = decodeXML(data)
		isXML = true
	case FormatAuto:
		switch sniff(data) {
		case 'j':
			root, err = decodeJSON(data)
		case 'x':
			root, err = decodeXML(data)
			isXML = true
		default:
			return nil, false, &RejectError{
				Reason:  ReasonSyntax,
				Message: "cannot detect document format; expected JSON ('{') or XML ('<')",
			}
		}
	default:
		return nil, false, &RejectError{
			Reason:  ReasonSyntax,
			Message: "unsupported format: " + string(f),
		}
	}
	return root, isXML, err
}

// sniff returns 'j' or 'x' for the first significant byte, 0 otherwise.
func sniff(data []byte) byte {
	// Skip a UTF-8 BOM if present.
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		data = data[3:]
	}
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return 'j'
		case '<':
			return 'x'
		default:
			return 0
		}
	}
	return 0
}

// classify reports whether a normalized tree is a catalog wrapper or a
// schema. Only an explicit "schemas" member selects the catalog shape.
func classify(root *node) docKind {
	if root == nil || root.kind != nObj {
		return docInvalid
	}
	if root.field("schemas") != nil {
		return docCatalog
	}
	// Every other object is parsed as a schema; missing "columns"/"name" are
	// then reported as missing-required rather than as a syntax error.
	return docSchema
}

type docKind int

const (
	docInvalid docKind = iota
	docSchema
	docCatalog
)
