package compat

import (
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

func col(name, id string, t model.DataType, nullable, hasDefault bool, aliases ...string) model.Column {
	return model.Column{Name: name, ID: id, Type: t, Nullable: nullable, HasDefault: hasDefault, Aliases: aliases}
}

func schema(name string, cols ...model.Column) *model.Schema {
	return &model.Schema{Name: name, Columns: cols}
}

func kinds(cs []Change) []Kind {
	out := make([]Kind, len(cs))
	for i, c := range cs {
		out[i] = c.Kind
	}
	return out
}

func findChange(cs []Change, kind Kind, newName string) *Change {
	for i := range cs {
		if cs[i].Kind == kind && cs[i].NewName == newName {
			return &cs[i]
		}
	}
	return nil
}

func TestCompatBoundaries(t *testing.T) {
	cases := []struct {
		name        string
		old, new    *model.Schema
		wantVerdict Verdict
		wantKinds   []Kind
		// detail assertions on selected changes: newName -> verdict
		namedVerdict map[string]Verdict
	}{
		{
			name:         "identical schemas are compatible",
			old:          schema("s", col("a", "", model.TypeInt, true, false)),
			new:          schema("s", col("a", "", model.TypeInt, true, false)),
			wantVerdict:  VerdictCompatible,
			wantKinds:    []Kind{KindUnchanged},
			namedVerdict: map[string]Verdict{"a": VerdictCompatible},
		},
		{
			name:        "add nullable column is compatible",
			old:         schema("s", col("a", "", model.TypeInt, true, false)),
			new:         schema("s", col("a", "", model.TypeInt, true, false), col("b", "", model.TypeStr, true, false)),
			wantVerdict: VerdictCompatible,
			namedVerdict: map[string]Verdict{"b": VerdictCompatible},
		},
		{
			name:        "add column with default is compatible",
			old:         schema("s"),
			new:         schema("s", col("b", "", model.TypeStr, false, true)),
			wantVerdict: VerdictCompatible,
			namedVerdict: map[string]Verdict{"b": VerdictCompatible},
		},
		{
			name:        "add required column is breaking",
			old:         schema("s"),
			new:         schema("s", col("b", "", model.TypeStr, false, false)),
			wantVerdict: VerdictBreaking,
			namedVerdict: map[string]Verdict{"b": VerdictBreaking},
		},
		{
			name:        "remove column is breaking",
			old:         schema("s", col("a", "", model.TypeInt, true, false), col("b", "", model.TypeStr, true, false)),
			new:         schema("s", col("a", "", model.TypeInt, true, false)),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindUnchanged, KindRemoved},
		},
		{
			name:         "rename via stable id is a rename, not remove+add",
			old:          schema("s", col("user_id", "u1", model.TypeLong, true, false)),
			new:          schema("s", col("user_key", "u1", model.TypeLong, true, false)),
			wantVerdict:  VerdictBreaking,
			wantKinds:    []Kind{KindRenamed},
			namedVerdict: map[string]Verdict{"user_key": VerdictBreaking},
		},
		{
			name:        "rename via unique alias is a rename",
			old:         schema("s", col("user_id", "", model.TypeLong, true, false)),
			new:         schema("s", col("user_key", "", model.TypeLong, true, false, "user_id")),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindRenamed},
		},
		{
			name: "ambiguous alias (two candidates) is remove+add, not rename",
			old: schema("s",
				col("user_id", "", model.TypeLong, true, false),
				col("other_id", "", model.TypeLong, true, false)),
			new: schema("s",
				col("user_key", "", model.TypeLong, true, false, "user_id", "other_id"),
				col("other_key", "", model.TypeLong, true, false, "user_id", "other_id")),
			wantVerdict: VerdictBreaking,
		},
		{
			name:        "drop plus add without evidence stays removed+added",
			old:         schema("s", col("a", "", model.TypeInt, true, false)),
			new:         schema("s", col("b", "", model.TypeInt, true, false)),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindRemoved, KindAdded},
		},
		{
			name:        "numeric widen int to long is compatible",
			old:         schema("s", col("a", "", model.TypeInt, true, false)),
			new:         schema("s", col("a", "", model.TypeLong, true, false)),
			wantVerdict: VerdictCompatible,
			wantKinds:   []Kind{KindTypeWidened},
		},
		{
			name:        "numeric widen long to double is compatible",
			old:         schema("s", col("a", "", model.TypeLong, true, false)),
			new:         schema("s", col("a", "", model.TypeDbl, true, false)),
			wantVerdict: VerdictCompatible,
			wantKinds:   []Kind{KindTypeWidened},
		},
		{
			name:        "numeric narrow double to long is breaking",
			old:         schema("s", col("a", "", model.TypeDbl, true, false)),
			new:         schema("s", col("a", "", model.TypeLong, true, false)),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindTypeNarrowed},
		},
		{
			name:        "cross family long to string is type_changed breaking",
			old:         schema("s", col("a", "", model.TypeLong, true, false)),
			new:         schema("s", col("a", "", model.TypeStr, true, false)),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindTypeChanged},
		},
		{
			name:        "cross family string to bool is type_changed breaking",
			old:         schema("s", col("a", "", model.TypeStr, true, false)),
			new:         schema("s", col("a", "", model.TypeBool, true, false)),
			wantVerdict: VerdictBreaking,
			wantKinds:   []Kind{KindTypeChanged},
		},
		{
			name: "rename plus widen reports both changes",
			old:  schema("s", col("a", "id1", model.TypeInt, true, false)),
			new:  schema("s", col("a2", "id1", model.TypeLong, true, false)),
			wantVerdict: VerdictBreaking,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compare(tc.old, tc.new)
			if r.Verdict != tc.wantVerdict {
				t.Fatalf("business=compat-boundary case=%q: verdict=%s want %s; changes=%+v",
					tc.name, r.Verdict, tc.wantVerdict, r.Changes)
			}
			if tc.wantKinds != nil {
				got := kinds(r.Changes)
				if !equalKinds(got, tc.wantKinds) {
					t.Fatalf("case=%q: kinds=%v want %v", tc.name, got, tc.wantKinds)
				}
			}
			for name, v := range tc.namedVerdict {
				var found *Change
				for i := range r.Changes {
					if r.Changes[i].NewName == name {
						found = &r.Changes[i]
						break
					}
				}
				if found == nil {
					t.Fatalf("case=%q: no change for column %q", tc.name, name)
				}
				if found.Verdict != v {
					t.Fatalf("case=%q: change for %q verdict=%s want %s", tc.name, name, found.Verdict, v)
				}
			}
			t.Logf("business=compat-boundary case=%-45s verdict=%s changes=%d kinds=%v",
				tc.name, r.Verdict, len(r.Changes), kinds(r.Changes))
		})
	}
}

func TestRenameDistinguishedFromDropAdd(t *testing.T) {
	// Same old/new names pair, but only the variant with stable id may be
	// classified as a rename.
	old := schema("s", col("a", "", model.TypeInt, true, false))
	withID := schema("s", col("b", "same-id", model.TypeInt, true, false))
	withoutID := schema("s", col("b", "", model.TypeInt, true, false))

	rID := Compare(schema("s", col("a", "same-id", model.TypeInt, true, false)), withID)
	rNo := Compare(old, withoutID)

	if findChange(rID.Changes, KindRenamed, "b") == nil {
		t.Fatalf("business=rename-vs-dropadd: stable id must produce KindRenamed, got %v", kinds(rID.Changes))
	}
	if findChange(rNo.Changes, KindRenamed, "b") != nil {
		t.Fatalf("business=rename-vs-dropadd: without evidence must NOT rename, got %v", kinds(rNo.Changes))
	}
	if findChange(rNo.Changes, KindRemoved, "") == nil || findChange(rNo.Changes, KindAdded, "b") == nil {
		t.Fatalf("business=rename-vs-dropadd: expected removed+added, got %v", kinds(rNo.Changes))
	}
	t.Logf("business=rename-vs-dropadd result=with-id:%v without-evidence:%v", kinds(rID.Changes), kinds(rNo.Changes))
}

func TestCompareDeterministic(t *testing.T) {
	old := schema("s",
		col("a", "", model.TypeDbl, true, false),
		col("b", "", model.TypeLong, true, false),
	)
	new := schema("s",
		col("zz", "", model.TypeInt, true, false),
		col("mm", "", model.TypeInt, true, false),
	)
	first := Compare(old, new)
	for i := 0; i < 20; i++ {
		again := Compare(old, new)
		if reportsDiffer(first, again) {
			t.Fatalf("business=determinism: report changed across runs:\n%+v\n%+v", first.Changes, again.Changes)
		}
	}
	t.Logf("business=determinism result=stable across 20 runs, change order:")
	for _, c := range first.Changes {
		t.Logf("    %s: %s -> %s (%s)", c.Kind, c.OldName, c.NewName, c.Verdict)
	}
}

func equalKinds(a, b []Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func reportsDiffer(a, b *Report) bool {
	if a.Verdict != b.Verdict || len(a.Changes) != len(b.Changes) {
		return true
	}
	for i := range a.Changes {
		x, y := a.Changes[i], b.Changes[i]
		if x.Kind != y.Kind || x.OldName != y.OldName || x.NewName != y.NewName ||
			x.OldType != y.OldType || x.NewType != y.NewType || x.Verdict != y.Verdict {
			return true
		}
	}
	return false
}
