// Package model defines the unified, format-independent representation of a
// local schema/contract description. Parsers normalize every supported input
// format into these types; compatibility and lineage analysis consume only
// this model and never touch the raw input.
package model

// DataType is the normalized primitive type of a column. Every input format
// maps its own type spellings onto this closed set.
type DataType string

const (
	TypeBool DataType = "bool"
	TypeInt  DataType = "int"
	TypeLong DataType = "long"
	TypeFlt  DataType = "float"
	TypeDbl  DataType = "double"
	TypeStr  DataType = "string"
)

// Column is one field/column of a schema.
//
// ID is an optional stable identifier set by the contract author. When present
// it survives a rename, which lets rename detection be unambiguous. When it is
// absent the analyzer can still match renames via Aliases, but only when the
// evidence is unique.
type Column struct {
	Name       string   `json:"name"`
	ID         string   `json:"id,omitempty"`
	Type       DataType `json:"type"`
	Nullable   bool     `json:"nullable,omitempty"`
	HasDefault bool     `json:"has_default,omitempty"`
	// Aliases are former names of the same logical column, newest first.
	// They are only used for rename matching when ID is absent.
	Aliases []string `json:"aliases,omitempty"`
	// Sources are the qualified column references this column is derived from,
	// in declaration order ("dataset.column"). Empty for a source column.
	Sources []ColumnRef `json:"sources,omitempty"`
	// Origin records which input file/format the column came from, for
	// conflict diagnostics. It is not part of the contract semantics.
	Origin string `json:"origin,omitempty"`
}

// Schema is one named dataset/table.
type Schema struct {
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	// Origin records the input source (path or "<format>:<name>").
	Origin string `json:"origin,omitempty"`
}

// ColumnRef points at a column of another (or the same) dataset.
type ColumnRef struct {
	Dataset string `json:"dataset"`
	Column  string `json:"column"`
}

// String renders the qualified "dataset.column" form.
func (r ColumnRef) String() string { return r.Dataset + "." + r.Column }

// Catalog is a document describing several datasets, the unit lineage is
// resolved over.
type Catalog struct {
	Schemas []Schema `json:"schemas"`
}
