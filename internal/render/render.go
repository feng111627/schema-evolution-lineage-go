// Package render renders analysis results as deterministic, human-readable
// text for the command line. The ordering rules match those of the analyzers,
// so identical inputs always produce identical output.
package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/compat"
	"github.com/feng111627/schema-evolution-lineage-go/internal/lineage"
	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
	"github.com/feng111627/schema-evolution-lineage-go/internal/parse"
)

// ParseErrors prints structured rejection reasons.
func ParseErrors(w io.Writer, err error) {
	if err == nil {
		return
	}
	for i, r := range parse.AsRejectErrors(err) {
		path := r.Path
		if path == "" {
			path = "-"
		}
		fmt.Fprintf(w, "  [%d] reason=%s path=%s\n        %s\n", i+1, r.Reason, path, r.Message)
		if r.Detail != "" {
			fmt.Fprintf(w, "        detail: %s\n", r.Detail)
		}
	}
}

// Catalog prints a normalized catalog back in a stable listing.
func Catalog(w io.Writer, cat *model.Catalog) {
	for _, s := range cat.Schemas {
		fmt.Fprintf(w, "dataset %s\n", s.Name)
		for _, c := range s.Columns {
			flags := []string{}
			if c.Nullable {
				flags = append(flags, "nullable")
			}
			if c.HasDefault {
				flags = append(flags, "default")
			}
			if c.ID != "" {
				flags = append(flags, "id="+c.ID)
			}
			extra := ""
			if len(flags) > 0 {
				extra = "  (" + strings.Join(flags, ", ") + ")"
			}
			fmt.Fprintf(w, "  - %-20s %s%s\n", c.Name, c.Type, extra)
			for _, a := range c.Aliases {
				fmt.Fprintf(w, "      alias: %s\n", a)
			}
			for _, ref := range c.Sources {
				fmt.Fprintf(w, "      from:  %s\n", ref)
			}
		}
	}
}

// CompatReport prints the per-column changes and the overall verdict.
func CompatReport(w io.Writer, r *compat.Report) {
	fmt.Fprintf(w, "schema: %s -> %s\n", r.OldName, r.NewName)
	for _, c := range r.Changes {
		label := string(c.Kind)
		switch c.Kind {
		case compat.KindRenamed:
			label = fmt.Sprintf("rename %s -> %s", c.OldName, c.NewName)
		case compat.KindAdded:
			label = "add    " + c.NewName
		case compat.KindRemoved:
			label = "remove " + c.OldName
		case compat.KindTypeWidened, compat.KindTypeNarrowed, compat.KindTypeChanged:
			label = fmt.Sprintf("%s %s: %s -> %s", c.Kind, c.NewName, c.OldType, c.NewType)
		default:
			label = string(c.Kind) + " " + c.NewName
		}
		fmt.Fprintf(w, "  [%-10s] %s\n", c.Verdict, label)
		if c.Detail != "" {
			fmt.Fprintf(w, "               %s\n", c.Detail)
		}
	}
	fmt.Fprintf(w, "overall: %s\n", r.Verdict)
}

// LineageReport prints edges, expanded chains and flagged issues.
func LineageReport(w io.Writer, r *lineage.Report) {
	targets := make([]string, 0, len(r.Edges))
	byName := map[string][]lineage.Node{}
	for t, edges := range r.Edges {
		targets = append(targets, t.String())
		byName[t.String()] = edges
	}
	sort.Strings(targets)
	fmt.Fprintln(w, "direct lineage:")
	anyEdge := false
	for _, name := range targets {
		refs := byName[name]
		if len(refs) == 0 {
			continue
		}
		anyEdge = true
		parts := make([]string, len(refs))
		for i, n := range refs {
			parts[i] = n.String()
		}
		fmt.Fprintf(w, "  %s <- %s\n", name, strings.Join(parts, ", "))
	}
	if !anyEdge {
		fmt.Fprintln(w, "  (no derived columns)")
	}
	fmt.Fprintf(w, "\nexpanded chains (%d):\n", len(r.Chains))
	for _, ch := range r.Chains {
		nodes := make([]string, len(ch.Path))
		for i, n := range ch.Path {
			nodes[i] = n.String()
		}
		fmt.Fprintf(w, "  %s => %s\n", ch.Target, strings.Join(nodes, " -> "))
	}
	fmt.Fprintf(w, "\nissues (%d):\n", len(r.Issues))
	if len(r.Issues) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, is := range r.Issues {
		fmt.Fprintf(w, "  [%-27s] at %s\n               %s\n", is.Kind, is.Node, is.Message)
	}
}
