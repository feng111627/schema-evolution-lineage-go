// Package e2e contains end-to-end tests driven by the real files under
// testdata/. They prove the local reproduction workflow: parse the files, run
// compatibility/lineage, and assert the documented conclusions.
package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/compat"
	"github.com/feng111627/schema-evolution-lineage-go/internal/lineage"
	"github.com/feng111627/schema-evolution-lineage-go/internal/parse"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..", "testdata")
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot(t), name))
	if err != nil {
		t.Fatalf("cannot read fixture %s: %v", name, err)
	}
	return data
}

func TestE2ECompatibilityEvolution(t *testing.T) {
	// orders.v1 -> orders.v2 demonstrates rename, widen, removal, a safe
	// nullable addition and a breaking required addition in one run.
	oldS, err := parse.ParseSchema(readFixture(t, "orders.v1.json"), parse.Options{Format: parse.FormatJSON})
	if err != nil {
		t.Fatalf("v1 must parse: %v", err)
	}
	newS, err := parse.ParseSchema(readFixture(t, "orders.v2.json"), parse.Options{Format: parse.FormatJSON})
	if err != nil {
		t.Fatalf("v2 must parse: %v", err)
	}
	r := compat.Compare(oldS, newS)
	if r.Verdict != compat.VerdictBreaking {
		t.Fatalf("business=e2e-compat: expected breaking, got %s", r.Verdict)
	}
	wantKinds := map[compat.Kind]bool{
		compat.KindRenamed:     false,
		compat.KindTypeWidened: false,
		compat.KindRemoved:     false,
		compat.KindAdded:       false,
		compat.KindUnchanged:   false,
	}
	for _, c := range r.Changes {
		if _, ok := wantKinds[c.Kind]; ok {
			wantKinds[c.Kind] = true
		}
		t.Logf("business=e2e-compat change=%-13s %s->%s verdict=%s", c.Kind, c.OldName, c.NewName, c.Verdict)
	}
	for k, seen := range wantKinds {
		if !seen {
			t.Errorf("business=e2e-compat: expected at least one %s change", k)
		}
	}
	// The rename specifically must be present (user_id -> user_key via id).
	foundRename := false
	for _, c := range r.Changes {
		if c.Kind == compat.KindRenamed && c.OldName == "user_id" && c.NewName == "user_key" {
			foundRename = true
		}
	}
	if !foundRename {
		t.Fatalf("business=e2e-compat: user_id->user_key must be a rename, not drop+add")
	}
	t.Log("business=e2e-compat result=overall breaking; rename correctly distinguished")
}

func TestE2EXMLAndJSONAreEquivalent(t *testing.T) {
	js := readFixture(t, "orders.v1.json")
	xmlDoc := readFixture(t, "orders.v1.xml")
	sJSON, err := parse.ParseSchema(js, parse.Options{})
	if err != nil {
		t.Fatalf("json parse: %v", err)
	}
	sXML, err := parse.ParseSchema(xmlDoc, parse.Options{})
	if err != nil {
		t.Fatalf("xml parse: %v", err)
	}
	if sJSON.Name != sXML.Name || len(sJSON.Columns) != len(sXML.Columns) {
		t.Fatalf("business=e2e-format-parity: normalized shapes differ")
	}
	for i := range sJSON.Columns {
		a, b := sJSON.Columns[i], sXML.Columns[i]
		if a.Name != b.Name || a.Type != b.Type || a.ID != b.ID || a.Nullable != b.Nullable {
			t.Fatalf("business=e2e-format-parity: column %d differs: %+v vs %+v", i, a, b)
		}
	}
	t.Logf("business=e2e-format-parity result=%d columns normalized identically across JSON/XML", len(sJSON.Columns))
}

func TestE2ELineageIssuesFromFiles(t *testing.T) {
	cases := []struct {
		file      string
		wantIssue lineage.IssueKind
	}{
		{"lineage-cycle.json", lineage.IssueCycle},
		{"lineage-dup.json", lineage.IssueDuplicateRef},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			cat, err := parse.ParseCatalog(readFixture(t, tc.file), parse.Options{})
			if err != nil {
				t.Fatalf("%s must parse: %v", tc.file, err)
			}
			r := lineage.Analyze(cat)
			found := false
			for _, is := range r.Issues {
				t.Logf("business=e2e-lineage file=%s issue=%s at=%s :: %s",
					tc.file, is.Kind, is.Node, is.Message)
				if is.Kind == tc.wantIssue {
					found = true
				}
			}
			if !found {
				t.Fatalf("business=e2e-lineage file=%s: expected issue %s, got %d issues",
					tc.file, tc.wantIssue, len(r.Issues))
			}
		})
	}
}

func TestE2EMalformedFixtureIsRejectedWithReasons(t *testing.T) {
	_, err := parse.ParseCatalog(readFixture(t, "malformed.json"), parse.Options{})
	rejects := parse.AsRejectErrors(err)
	want := map[parse.Reason]bool{
		parse.ReasonUnsupportedType: false,
		parse.ReasonTypeMismatch:    false,
		parse.ReasonMissingRequired: false,
		parse.ReasonInvalidRef:      false,
	}
	for _, r := range rejects {
		if _, ok := want[r.Reason]; ok {
			want[r.Reason] = true
		}
		t.Logf("business=e2e-malformed reason=%-17s path=%-24s %s", r.Reason, r.Path, r.Message)
	}
	for reason, seen := range want {
		if !seen {
			t.Errorf("business=e2e-malformed: expected reason %s to be reported", reason)
		}
	}
}

func TestE2ECleanLineageHasNoIssues(t *testing.T) {
	cat, err := parse.ParseCatalog(readFixture(t, "lineage.json"), parse.Options{})
	if err != nil {
		t.Fatalf("lineage.json must parse: %v", err)
	}
	r := lineage.Analyze(cat)
	if len(r.Issues) != 0 {
		t.Fatalf("business=e2e-clean-lineage: expected no issues, got %d", len(r.Issues))
	}
	if len(r.Chains) != 5 {
		t.Fatalf("business=e2e-clean-lineage: expected 5 expanded chains, got %d", len(r.Chains))
	}
	t.Logf("business=e2e-clean-lineage result=%d chains, 0 issues", len(r.Chains))
}
