package lineage

import (
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

func lcol(name string, refs ...string) model.Column {
	c := model.Column{Name: name, Type: model.TypeStr}
	for _, r := range refs {
		c.Sources = append(c.Sources, mustRef(r))
	}
	return c
}

func mustRef(s string) model.ColumnRef {
	d, c, ok := split(s)
	if !ok {
		panic("bad ref in test: " + s)
	}
	return model.ColumnRef{Dataset: d, Column: c}
}

func split(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

func mkSchema(origin, name string, cols ...model.Column) model.Schema {
	s := model.Schema{Name: name, Origin: origin, Columns: cols}
	return s
}

func issueKinds(r *Report) map[IssueKind]int {
	m := map[IssueKind]int{}
	for _, i := range r.Issues {
		m[i.Kind]++
	}
	return m
}

func TestLineageLongChain(t *testing.T) {
	// raw -> l1 -> l2 -> l3 -> l4 : a 5-node straight chain must expand to
	// exactly one chain per derived node reaching back to raw.
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("raw"),
			lcol("l1", "d.raw"),
			lcol("l2", "d.l1"),
			lcol("l3", "d.l2"),
			lcol("l4", "d.l3"),
		),
	}}
	r := Analyze(cat)
	if len(r.Issues) != 0 {
		t.Fatalf("business=long-chain: expected no issues, got %+v", r.Issues)
	}
	targetChains := map[Node]int{}
	for _, ch := range r.Chains {
		targetChains[ch.Target]++
		if ch.Path[0] != (Node{"d", "raw"}) {
			t.Fatalf("business=long-chain: chain for %s must start at raw, got %v", ch.Target, ch.Path)
		}
	}
	for _, name := range []string{"l1", "l2", "l3", "l4"} {
		if targetChains[Node{"d", name}] != 1 {
			t.Fatalf("business=long-chain: expected exactly 1 chain for d.%s, got %d", name, targetChains[Node{"d", name}])
		}
	}
	l4 := Chain{Target: Node{"d", "l4"}}
	for _, ch := range r.Chains {
		if ch.Target == l4.Target && len(ch.Path) != 5 {
			t.Fatalf("business=long-chain: d.l4 chain length=%d want 5", len(ch.Path))
		}
	}
	t.Logf("business=long-chain result=%d chains, deepest path length 5; all roots=d.raw", len(r.Chains))
}

func TestLineageFanOutFanIn(t *testing.T) {
	// Two roots merge into m, which then fans into two targets: chain counts
	// must follow the product of paths.
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("a"),
			lcol("b"),
			lcol("m", "d.a", "d.b"),
			lcol("x", "d.m"),
			lcol("y", "d.m"),
		),
	}}
	r := Analyze(cat)
	if len(r.Issues) != 0 {
		t.Fatalf("business=fan: unexpected issues %+v", r.Issues)
	}
	count := map[Node]int{}
	for _, ch := range r.Chains {
		count[ch.Target]++
	}
	if count[Node{"d", "m"}] != 2 {
		t.Fatalf("business=fan: m should have 2 root chains, got %d", count[Node{"d", "m"}])
	}
	if count[Node{"d", "x"}] != 2 || count[Node{"d", "y"}] != 2 {
		t.Fatalf("business=fan: x/y should each have 2 chains, got x=%d y=%d",
			count[Node{"d", "x"}], count[Node{"d", "y"}])
	}
	t.Logf("business=fan result=chains: m=%d x=%d y=%d",
		count[Node{"d", "m"}], count[Node{"d", "x"}], count[Node{"d", "y"}])
}

func TestLineageSelfCycle(t *testing.T) {
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d", lcol("c", "d.c")),
	}}
	r := Analyze(cat)
	kinds := issueKinds(r)
	if kinds[IssueCycle] != 1 {
		t.Fatalf("business=cycle-self: expected 1 cycle issue, got %+v", r.Issues)
	}
	t.Logf("business=cycle-self result=%s", r.Issues[0].Message)
}

func TestLineageTwoNodeAndLongCycle(t *testing.T) {
	// a -> b -> c -> b is a 3-node cycle reachable from a.
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("a", "d.b"),
			lcol("b", "d.c"),
			lcol("c", "d.b"),
		),
	}}
	r := Analyze(cat)
	kinds := issueKinds(r)
	if kinds[IssueCycle] < 1 {
		t.Fatalf("business=cycle-multi: expected a cycle issue, got %+v", r.Issues)
	}
	for _, i := range r.Issues {
		if i.Kind == IssueCycle {
			t.Logf("business=cycle-multi result=%s (at %s)", i.Message, i.Node)
		}
	}

	// Same graph analyzed twice must produce identical cycle reporting.
	r2 := Analyze(cat)
	if len(r.Issues) != len(r2.Issues) {
		t.Fatalf("business=cycle-stability: issue counts differ")
	}
	for i := range r.Issues {
		if r.Issues[i] != r2.Issues[i] {
			t.Fatalf("business=cycle-stability: issue %d differs: %+v vs %+v", i, r.Issues[i], r2.Issues[i])
		}
	}
}

func TestLineageDuplicateReference(t *testing.T) {
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("a"),
			lcol("b", "d.a", "d.a"),
		),
	}}
	r := Analyze(cat)
	kinds := issueKinds(r)
	if kinds[IssueDuplicateRef] != 1 {
		t.Fatalf("business=duplicate-ref: expected 1 duplicate issue, got %+v", r.Issues)
	}
	t.Logf("business=duplicate-ref result=%s", r.Issues[0].Message)
}

func TestLineageUnknownReference(t *testing.T) {
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("a", "d.missing"),
			lcol("b", "other.thing"),
		),
	}}
	r := Analyze(cat)
	kinds := issueKinds(r)
	if kinds[IssueUnknownRef] != 2 {
		t.Fatalf("business=unknown-ref: expected 2 unknown issues (missing column + missing dataset), got %+v", r.Issues)
	}
	t.Logf("business=unknown-ref result=%d unknown references flagged", kinds[IssueUnknownRef])
}

func TestLineageSameNameDifferentOrigin(t *testing.T) {
	// Two input files define the same qualified name d.a. Analyze must flag
	// the conflict instead of silently choosing one.
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file-a.json", "d", lcol("a", "d.root1"), lcol("root1")),
		mkSchema("file-b.json", "d", lcol("a", "d.root2"), lcol("root2")),
	}}
	r := Analyze(cat)
	kinds := issueKinds(r)
	if kinds[IssueNameConflict] != 1 {
		t.Fatalf("business=same-name-conflict: expected 1 conflict issue, got %+v", r.Issues)
	}
	found := false
	for _, i := range r.Issues {
		if i.Kind == IssueNameConflict && i.Node == (Node{"d", "a"}) {
			found = true
			t.Logf("business=same-name-conflict result=%s", i.Message)
		}
	}
	if !found {
		t.Fatalf("business=same-name-conflict: conflict not attached to d.a: %+v", r.Issues)
	}
}

func TestLineageSameOriginSameNameIsNotConflict(t *testing.T) {
	// A single input file cannot contain a duplicate qualified name because
	// parsing rejects duplicate columns, but two schemas of the same dataset
	// name in one file is also unusual; ensure genuinely identical origin is
	// not a false positive.
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("same.json", "d", lcol("a"), lcol("b", "d.a")),
	}}
	r := Analyze(cat)
	for _, i := range r.Issues {
		if i.Kind == IssueNameConflict {
			t.Fatalf("business=conflict-negative: unexpected conflict for single origin: %+v", i)
		}
	}
	t.Logf("business=conflict-negative result=no false-positive conflict")
}

func TestLineageDeterministicOrder(t *testing.T) {
	cat := &model.Catalog{Schemas: []model.Schema{
		mkSchema("file1", "d",
			lcol("z", "d.r"),
			lcol("m", "d.r"),
			lcol("r"),
		),
	}}
	first := Analyze(cat)
	for i := 0; i < 15; i++ {
		again := Analyze(cat)
		if !chainsEqual(first.Chains, again.Chains) || !issuesEqual(first.Issues, again.Issues) {
			t.Fatalf("business=lineage-determinism: output changed across runs")
		}
	}
	t.Logf("business=lineage-determinism result=%d chains stable across 15 runs", len(first.Chains))
}

func chainsEqual(a, b []Chain) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Target != b[i].Target || len(a[i].Path) != len(b[i].Path) {
			return false
		}
		for j := range a[i].Path {
			if a[i].Path[j] != b[i].Path[j] {
				return false
			}
		}
	}
	return true
}

func issuesEqual(a, b []Issue) bool {
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
