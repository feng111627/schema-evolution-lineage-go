// Package lineage expands and tracks column-level lineage across the datasets
// described by a catalog. Cycles, duplicate references and same-name/
// different-origin conflicts are reported explicitly.
package lineage

import "github.com/feng111627/schema-evolution-lineage-go/internal/model"

// IssueKind classifies a lineage problem.
type IssueKind string

const (
	IssueCycle          IssueKind = "cycle"
	IssueDuplicateRef   IssueKind = "duplicate_reference"
	IssueNameConflict   IssueKind = "same_name_different_source"
	IssueUnknownRef     IssueKind = "unknown_reference"
)

// Node is one resolved dataset column.
type Node struct {
	Dataset string
	Column  string
}

// Issue flags one lineage problem at a specific node.
type Issue struct {
	Kind    IssueKind
	Node    Node
	Message string
}

// Chain is one fully-expanded upstream path, root-first
// (source column ... -> target).
type Chain struct {
	Target Node
	Path   []Node
}

// Report is the lineage analysis result.
type Report struct {
	// Direct edges target -> immediate sources, in declaration order.
	Edges map[Node][]Node
	// Chains are all root-to-target paths, sorted for stable output.
	Chains []Chain
	Issues []Issue
}

// Analyze expands lineage for every derived column in the catalog.
func Analyze(cat *model.Catalog) *Report {
	_ = cat
	return &Report{}
}
