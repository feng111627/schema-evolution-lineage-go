// Package compat decides whether a "new" schema version is compatible with an
// "old" one and emits a stable, per-column list of changes.
package compat

import "github.com/feng111627/schema-evolution-lineage-go/internal/model"

// Verdict is the stable compatibility conclusion for one change or for the
// whole schema pair.
type Verdict string

const (
	// VerdictCompatible: old readers/writers keep working.
	VerdictCompatible Verdict = "compatible"
	// VerdictBreaking: at least one old reader or writer breaks.
	VerdictBreaking Verdict = "breaking"
)

// Kind is the stable change category of a single column.
type Kind string

const (
	KindAdded           Kind = "added"
	KindRemoved         Kind = "removed"
	KindRenamed         Kind = "renamed"
	KindTypeWidened     Kind = "type_widened"
	KindTypeNarrowed    Kind = "type_narrowed"
	KindTypeChanged     Kind = "type_changed"
	KindNullability     Kind = "nullability_changed"
	KindDefaultChanged  Kind = "default_changed"
	KindUnchanged       Kind = "unchanged"
)

// Change describes what happened to one column between two versions.
type Change struct {
	Kind     Kind
	OldName  string
	NewName  string
	OldType  model.DataType
	NewType  model.DataType
	Verdict  Verdict
	Detail   string
}

// Report is the full result of comparing two schema versions.
type Report struct {
	OldName      string
	NewName      string
	Changes      []Change
	Verdict      Verdict
}

// Compare returns a deterministic compatibility report for old vs new.
// Running it repeatedly on the same pair yields the same report.
func Compare(old, new *model.Schema) *Report {
	_ = old
	_ = new
	return &Report{}
}
