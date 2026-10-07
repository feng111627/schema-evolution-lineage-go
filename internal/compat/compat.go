// Package compat decides whether a "new" schema version is compatible with an
// "old" one and emits a stable, per-column list of changes.
//
// Renames are distinguished from remove+add using explicit evidence:
//
//  1. if both sides carry the same non-empty stable column id, the columns are
//     the same logical column and a different name is a rename;
//  2. otherwise, when the new column's aliases contain exactly one old column
//     name (and that old column is not also kept under its old name), it is a
//     rename;
//  3. without such evidence the change is reported as removed + added.
package compat

import (
	"sort"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// Verdict is the stable compatibility conclusion for one change or for the
// whole schema pair.
type Verdict string

const (
	// VerdictCompatible: old readers and old writers keep working.
	VerdictCompatible Verdict = "compatible"
	// VerdictBreaking: at least one old reader or writer breaks.
	VerdictBreaking Verdict = "breaking"
)

// Kind is the stable change category of a single column.
type Kind string

const (
	KindAdded          Kind = "added"
	KindRemoved        Kind = "removed"
	KindRenamed        Kind = "renamed"
	KindTypeWidened    Kind = "type_widened"
	KindTypeNarrowed   Kind = "type_narrowed"
	KindTypeChanged    Kind = "type_changed"
	KindNullability    Kind = "nullability_changed"
	KindDefaultChanged Kind = "default_changed"
	KindUnchanged      Kind = "unchanged"
)

// Change describes what happened to one column between two versions.
type Change struct {
	Kind    Kind
	OldName string
	NewName string
	OldType model.DataType
	NewType model.DataType
	Verdict Verdict
	Detail  string
}

// Report is the full result of comparing two schema versions. Changes are
// emitted in a fixed order (old-column order first, then new additions by
// name) so repeated runs on the same input produce identical reports.
type Report struct {
	OldName string
	NewName string
	Changes []Change
	Verdict Verdict
}

// widenRank orders the numeric promotion chain. A higher rank can hold every
// value of every lower rank without loss. Types outside the numeric family
// have no rank and are only compatible with themselves.
var widenRank = map[model.DataType]int{
	model.TypeInt:  1,
	model.TypeLong: 2,
	model.TypeFlt:  3,
	model.TypeDbl:  4,
}

// Compare returns a deterministic compatibility report for old vs new.
//
// Compatibility viewpoint: an old consumer of the old schema reading data
// produced under the new schema, and an old writer's records still being
// acceptable. Additions are compatible only when the new column is nullable or
// has a default; removals and renames (the old name disappears), narrowing and
// cross-family type changes are breaking.
func Compare(old, new *model.Schema) *Report {
	r := &Report{}
	if old != nil {
		r.OldName = old.Name
	}
	if new != nil {
		r.NewName = new.Name
	}

	pairs, removed, added := matchColumns(old, new)
	r.Changes = make([]Change, 0, len(pairs)+len(removed)+len(added))
	overall := VerdictCompatible

	// Matched pairs follow old column declaration order.
	for _, p := range pairs {
		c := describePair(p.old, p.new)
		r.Changes = append(r.Changes, c...)
		for _, ch := range c {
			if ch.Verdict == VerdictBreaking {
				overall = VerdictBreaking
			}
		}
	}
	// Removals follow old declaration order.
	for _, c := range removed {
		r.Changes = append(r.Changes, Change{
			Kind:    KindRemoved,
			OldName: c.Name,
			OldType: c.Type,
			Verdict: VerdictBreaking,
			Detail:  "column removed; old readers expecting it will fail",
		})
		overall = VerdictBreaking
	}
	// Additions are sorted by new name for stable output.
	sort.Slice(added, func(i, j int) bool { return added[i].Name < added[j].Name })
	for _, c := range added {
		ch := Change{
			Kind:    KindAdded,
			NewName: c.Name,
			NewType: c.Type,
		}
		if c.Nullable || c.HasDefault {
			ch.Verdict = VerdictCompatible
			ch.Detail = "new column is nullable or has a default; old readers/writers keep working"
		} else {
			ch.Verdict = VerdictBreaking
			ch.Detail = "new column is required (not nullable and no default); old writers cannot populate it"
		}
		r.Changes = append(r.Changes, ch)
		if ch.Verdict == VerdictBreaking {
			overall = VerdictBreaking
		}
	}

	r.Verdict = overall
	return r
}

type pair struct{ old, new model.Column }

// matchColumns returns matched pairs (in old declaration order), old columns
// without a match and new columns without a match.
func matchColumns(old, new *model.Schema) (pairs []pair, removed []model.Column, added []model.Column) {
	if old == nil {
		old = &model.Schema{}
	}
	if new == nil {
		new = &model.Schema{}
	}
	usedNew := make(map[int]bool)
	matchedOld := make(map[int]bool)

	// Pass 1: stable ids.
	oldByID := map[string]int{}
	for i, c := range old.Columns {
		if c.ID != "" {
			oldByID[c.ID] = i
		}
	}
	newByID := map[string]int{}
	for j, c := range new.Columns {
		if c.ID != "" {
			newByID[c.ID] = j
		}
	}
	for id, oi := range oldByID {
		if nj, ok := newByID[id]; ok {
			pairs = append(pairs, pair{old.Columns[oi], new.Columns[nj]})
			matchedOld[oi] = true
			usedNew[nj] = true
		}
	}
	// Stable-id pairs must still follow old declaration order.
	sort.SliceStable(pairs, func(i, j int) bool {
		return indexOfColumn(old.Columns, pairs[i].old) < indexOfColumn(old.Columns, pairs[j].old)
	})

	// Pass 2: name equality among unmatched columns.
	for i := range old.Columns {
		if matchedOld[i] {
			continue
		}
		for j := range new.Columns {
			if usedNew[j] {
				continue
			}
			if old.Columns[i].Name == new.Columns[j].Name {
				pairs = append(pairs, pair{old.Columns[i], new.Columns[j]})
				matchedOld[i] = true
				usedNew[j] = true
				break
			}
		}
	}

	// Pass 3: alias-based rename detection, only when unambiguous.
	for i := range old.Columns {
		if matchedOld[i] {
			continue
		}
		candidates := []int{}
		for j := range new.Columns {
			if usedNew[j] {
				continue
			}
			for _, a := range new.Columns[j].Aliases {
				if a == old.Columns[i].Name {
					candidates = append(candidates, j)
					break
				}
			}
		}
		if len(candidates) == 1 {
			j := candidates[0]
			pairs = append(pairs, pair{old.Columns[i], new.Columns[j]})
			matchedOld[i] = true
			usedNew[j] = true
		}
	}

	// Remaining columns are removals / additions, preserving old order and
	// letting the caller sort additions.
	for i, c := range old.Columns {
		if !matchedOld[i] {
			removed = append(removed, c)
		}
	}
	for j, c := range new.Columns {
		if !usedNew[j] {
			added = append(added, c)
		}
	}
	return pairs, removed, added
}

func indexOfColumn(cols []model.Column, target model.Column) int {
	for i := range cols {
		if cols[i].Name == target.Name && cols[i].ID == target.ID {
			return i
		}
	}
	return -1
}

// describePair reports every change between two matched columns, renames
// included. A pair can carry several changes (rename + widen, for example).
func describePair(o, n model.Column) []Change {
	var out []Change
	if o.Name != n.Name {
		out = append(out, Change{
			Kind:    KindRenamed,
			OldName: o.Name,
			NewName: n.Name,
			OldType: o.Type,
			NewType: n.Type,
			Verdict: VerdictBreaking,
			Detail:  "column renamed; references to the old name are not resolved",
		})
	}
	if o.Type != n.Type {
		c := Change{
			Kind:    typeKind(o.Type, n.Type),
			OldName: o.Name,
			NewName: n.Name,
			OldType: o.Type,
			NewType: n.Type,
		}
		switch c.Kind {
		case KindTypeWidened:
			c.Verdict = VerdictCompatible
			c.Detail = "numeric type widened; all old values remain valid"
		case KindTypeNarrowed, KindTypeChanged:
			c.Verdict = VerdictBreaking
			if c.Kind == KindTypeNarrowed {
				c.Detail = "numeric type narrowed; old values may not fit"
			} else {
				c.Detail = "type family changed; old values cannot be interpreted as the new type"
			}
		}
		out = append(out, c)
	}
	if o.Nullable != n.Nullable {
		c := Change{
			Kind:    KindNullability,
			OldName: o.Name,
			NewName: n.Name,
			Verdict: VerdictCompatible,
			Detail:  "nullability annotation changed; structural compatibility preserved",
		}
		out = append(out, c)
	}
	if o.HasDefault != n.HasDefault {
		c := Change{
			Kind:    KindDefaultChanged,
			OldName: o.Name,
			NewName: n.Name,
			Verdict: VerdictCompatible,
			Detail:  "default-presence annotation changed; structural compatibility preserved",
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		out = append(out, Change{
			Kind:    KindUnchanged,
			OldName: o.Name,
			NewName: n.Name,
			OldType: o.Type,
			NewType: n.Type,
			Verdict: VerdictCompatible,
			Detail:  "no semantic change",
		})
	}
	return out
}

// typeKind classifies a type transition.
func typeKind(a, b model.DataType) Kind {
	ra, oka := widenRank[a]
	rb, okb := widenRank[b]
	if oka && okb {
		switch {
		case rb > ra:
			return KindTypeWidened
		case rb < ra:
			return KindTypeNarrowed
		default:
			return KindUnchanged
		}
	}
	return KindTypeChanged
}
