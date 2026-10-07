// Package parse reads the supported local schema/contract description formats
// and normalizes them into the model package.
//
// All failures are reported as *RejectError / *RejectErrors so callers can
// distinguish malformed syntax, missing required fields and type mismatches.
// Underlying library errors are summarized and never exposed verbatim.
package parse

import "github.com/feng111627/schema-evolution-lineage-go/internal/model"

// Format selects the input dialect.
type Format string

const (
	// FormatAuto detects the format from the leading non-space byte
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

// ParseCatalog parses a document that may describe several datasets.
func ParseCatalog(data []byte, opts Options) (*model.Catalog, error) {
	_ = data
	_ = opts
	return nil, nil
}

// ParseSchema parses a single-dataset document.
func ParseSchema(data []byte, opts Options) (*model.Schema, error) {
	_ = data
	_ = opts
	return nil, nil
}
