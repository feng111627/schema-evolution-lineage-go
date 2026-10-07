// Package lineage expands and tracks column-level lineage across the datasets
// described by a catalog.
//
// Four conditions are reported explicitly and never silently resolved:
//
//   - cycle: a column (transitively) derives from itself;
//   - duplicate_reference: the same upstream column is listed twice;
//   - same_name_different_source: the same qualified column name exists in
//     more than one input document, or two distinct targets share one name;
//   - unknown_reference: a source points at a dataset/column that does not
//     exist.
//
// All root-to-target chains are enumerated, so conclusions stay correct on
// long, fan-out/fan-in graphs; output order is fully deterministic.
package lineage

import (
	"fmt"
	"sort"
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// IssueKind classifies a lineage problem.
type IssueKind string

const (
	IssueCycle        IssueKind = "cycle"
	IssueDuplicateRef IssueKind = "duplicate_reference"
	IssueNameConflict IssueKind = "same_name_different_source"
	IssueUnknownRef   IssueKind = "unknown_reference"
)

// Node is one resolved dataset column.
type Node struct {
	Dataset string
	Column  string
}

func nodeOf(r model.ColumnRef) Node { return Node{Dataset: r.Dataset, Column: r.Column} }

func (n Node) String() string { return n.Dataset + "." + n.Column }

// Issue flags one lineage problem at a specific node.
type Issue struct {
	Kind    IssueKind
	Node    Node
	Message string
}

// Chain is one fully-expanded upstream path, source-first
// (root ... -> target).
type Chain struct {
	Target Node
	Path   []Node
}

// Report is the lineage analysis result.
type Report struct {
	// Edges maps each derived target to its immediate sources, in declaration
	// order (duplicates preserved so callers can see them).
	Edges map[Node][]Node
	// Chains lists every root-to-target path, sorted for stable output.
	Chains []Chain
	Issues []Issue
}

type entry struct {
	col    model.Column
	origin string
}

// Analyze expands lineage for every column in the catalog.
func Analyze(cat *model.Catalog) *Report {
	r := &Report{Edges: map[Node][]Node{}}
	if cat == nil {
		return r
	}

	// index: qualified node -> all definitions seen (for same-name conflicts)
	defs := map[Node][]entry{}
	// datasets present, for better unknown-reference messages.
	datasets := map[string]bool{}
	for _, sch := range cat.Schemas {
		datasets[sch.Name] = true
		for _, c := range sch.Columns {
			n := Node{Dataset: sch.Name, Column: c.Name}
			defs[n] = append(defs[n], entry{col: c, origin: sch.Origin})
		}
	}

	// Deterministic node ordering.
	var allNodes []Node
	for n := range defs {
		allNodes = append(allNodes, n)
	}
	sort.Slice(allNodes, func(i, j int) bool {
		if allNodes[i].Dataset != allNodes[j].Dataset {
			return allNodes[i].Dataset < allNodes[j].Dataset
		}
		return allNodes[i].Column < allNodes[j].Column
	})

	// same-name different-source: multiple definitions from different origins.
	for _, n := range allNodes {
		es := defs[n]
		differentOrigin := false
		origin := es[0].origin
		for _, e := range es[1:] {
			if e.origin != origin {
				differentOrigin = true
			}
		}
		if differentOrigin {
			origins := originsOf(es)
			r.Issues = append(r.Issues, Issue{
				Kind: IssueNameConflict,
				Node: n,
				Message: fmt.Sprintf(
					"column %s is defined in multiple input sources [%s]; cannot pick one implicitly",
					n, strings.Join(origins, ", ")),
			})
		}
	}

	// Build direct edges; detect duplicate and unknown references.
	targetOf := map[Node]model.Column{}
	for _, n := range allNodes {
		targetOf[n] = defs[n][0].col
	}
	for _, n := range allNodes {
		c := targetOf[n]
		if len(c.Sources) == 0 {
			continue
		}
		seen := map[Node]int{} // first occurrence index
		var edges []Node
		for _, ref := range c.Sources {
			sn := nodeOf(ref)
			if _, ok := defs[sn]; !ok {
				if datasets[sn.Dataset] {
					r.Issues = append(r.Issues, Issue{
						Kind: IssueUnknownRef,
						Node: n,
						Message: fmt.Sprintf(
							"source %s referenced by %s does not exist in dataset %q",
							sn, n, sn.Dataset),
					})
				} else {
					r.Issues = append(r.Issues, Issue{
						Kind: IssueUnknownRef,
						Node: n,
						Message: fmt.Sprintf(
							"source %s referenced by %s points at unknown dataset %q",
							sn, n, sn.Dataset),
					})
				}
				continue
			}
			if first, dup := seen[sn]; dup {
				r.Issues = append(r.Issues, Issue{
					Kind: IssueDuplicateRef,
					Node: n,
					Message: fmt.Sprintf(
						"target %s references %s more than once (occurrences #%d and #%d)",
						n, sn, first+1, len(edges)+1),
				})
			} else {
				seen[sn] = len(edges)
			}
			edges = append(edges, sn)
		}
		r.Edges[n] = edges
	}

	// Expand full chains for every derived target.
	cycleSeen := map[string]bool{}
	for _, n := range allNodes {
		if len(r.Edges[n]) == 0 {
			continue
		}
		expand(n, n, []Node{n}, r.Edges, defs, r, cycleSeen)
	}

	sortIssues(r.Issues)
	sortChains(r.Chains)
	return r
}

// expand DFS-walks upstream edges from target. path holds the current DFS
// stack (target first, walking toward sources). A node reappearing on the
// stack closes a cycle; that path is recorded as a chain ending where the
// cycle closes and flagged exactly once per (target,cycle) pair.
func expand(root, cur Node, path []Node, edges map[Node][]Node, defs map[Node][]entry, r *Report, cycleSeen map[string]bool) {
	up, ok := edges[cur]
	if !ok || len(up) == 0 {
		// cur is a root source node: emit the path reversed (source-first).
		chain := Chain{Target: root, Path: reversePath(path)}
		r.Chains = append(r.Chains, chain)
		return
	}
	for _, sn := range up {
		// cycle detection against the current stack
		idx := indexOfNode(path, sn)
		if idx >= 0 {
			cyclic := append(append([]Node{}, path...), sn)
			key := cycleKey(root, path[idx:])
			if !cycleSeen[key] {
				cycleSeen[key] = true
				r.Issues = append(r.Issues, Issue{
					Kind: IssueCycle,
					Node: root,
					Message: fmt.Sprintf("dependency cycle: %s",
						joinNodes(append(append([]Node{}, path[idx:]...), sn))),
				})
			}
			r.Chains = append(r.Chains, Chain{Target: root, Path: reversePath(cyclic)})
			continue
		}
		expand(root, sn, append(append([]Node{}, path...), sn), edges, defs, r, cycleSeen)
	}
}

func originsOf(es []entry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range es {
		if !seen[e.origin] {
			seen[e.origin] = true
			out = append(out, e.origin)
		}
	}
	sort.Strings(out)
	return out
}

func indexOfNode(path []Node, n Node) int {
	for i := range path {
		if path[i] == n {
			return i
		}
	}
	return -1
}

func reversePath(p []Node) []Node {
	out := make([]Node, len(p))
	for i := range p {
		out[len(p)-1-i] = p[i]
	}
	return out
}

func joinNodes(ns []Node) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = n.String()
	}
	return strings.Join(parts, " -> ")
}

// cycleKey identifies a cycle independently of traversal start so it is
// reported once: rotate the node sequence to its lexicographically smallest
// member.
func cycleKey(root Node, cyc []Node) string {
	if len(cyc) == 0 {
		return root.String()
	}
	min := 0
	for i := 1; i < len(cyc); i++ {
		if cyc[i].String() < cyc[min].String() {
			min = i
		}
	}
	rotated := make([]Node, 0, len(cyc)+1)
	rotated = append(rotated, cyc[min:]...)
	rotated = append(rotated, cyc[:min]...)
	rotated = append(rotated, rotated[0])
	return joinNodes(rotated)
}

func sortIssues(is []Issue) {
	sort.SliceStable(is, func(i, j int) bool {
		if is[i].Node != is[j].Node {
			return is[i].Node.String() < is[j].Node.String()
		}
		if is[i].Kind != is[j].Kind {
			return is[i].Kind < is[j].Kind
		}
		return is[i].Message < is[j].Message
	})
}

func sortChains(cs []Chain) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Target != cs[j].Target {
			return cs[i].Target.String() < cs[j].Target.String()
		}
		return chainPathLess(cs[i].Path, cs[j].Path)
	})
}

func chainPathLess(a, b []Node) bool {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for k := 0; k < n; k++ {
		if a[k] != b[k] {
			return a[k].String() < b[k].String()
		}
	}
	return len(a) < len(b)
}
