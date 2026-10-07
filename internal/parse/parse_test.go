package parse

import (
	"strings"
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// validJSON is the canonical well-formed single-schema contract used as a
// base for mutation tests.
const validJSON = `{
  "name": "orders",
  "columns": [
    {"name": "id", "id": "col-id", "type": "long", "nullable": false},
    {"name": "amount_total", "type": "double", "nullable": true, "has_default": true,
     "aliases": ["amount"], "sources": ["payments.cents"]}
  ]
}`

const validXML = `<catalog>
  <schemas>
    <schema name="orders">
      <columns>
        <column name="id" id="col-id" type="long" nullable="false"/>
        <column name="amount_total" type="double" nullable="true" has_default="true">
          <aliases><alias>amount</alias></aliases>
          <sources><source ref="payments.cents"/></sources>
        </column>
      </columns>
    </schema>
  </schemas>
</catalog>`

func TestParseValidDocuments(t *testing.T) {
	cases := []struct {
		name   string
		data   string
		format Format
	}{
		{"json schema auto", validJSON, FormatAuto},
		{"json schema explicit", validJSON, FormatJSON},
		{"xml catalog auto", validXML, FormatAuto},
		{"xml catalog explicit", validXML, FormatXML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, err := ParseCatalog([]byte(tc.data), Options{Origin: "fixture"})
			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if len(cat.Schemas) != 1 || cat.Schemas[0].Name != "orders" {
				t.Fatalf("unexpected catalog: %+v", cat.Schemas)
			}
			cols := cat.Schemas[0].Columns
			if len(cols) != 2 {
				t.Fatalf("expected 2 columns, got %d", len(cols))
			}
			id, amt := cols[0], cols[1]
			if id.Name != "id" || id.ID != "col-id" || id.Type != model.TypeLong || id.Nullable {
				t.Fatalf("id column normalized incorrectly: %+v", id)
			}
			if amt.Name != "amount_total" || amt.Type != model.TypeDbl || !amt.Nullable ||
				!amt.HasDefault || len(amt.Aliases) != 1 || amt.Aliases[0] != "amount" {
				t.Fatalf("amount column normalized incorrectly: %+v", amt)
			}
			if len(amt.Sources) != 1 || amt.Sources[0] != (model.ColumnRef{Dataset: "payments", Column: "cents"}) {
				t.Fatalf("source ref normalized incorrectly: %+v", amt.Sources)
			}
			t.Logf("business=normalization format=%s: schema=%s columns=%d OK", tc.name, "orders", len(cols))
		})
	}
}

func TestParseRejections(t *testing.T) {
	cases := []struct {
		name       string
		data       string
		format     Format
		wantReason Reason
		wantPath   string
	}{
		// Malformed syntax.
		{"json broken syntax", `{"name": "x", columns: []}`, FormatJSON, ReasonSyntax, ""},
		{"json trailing tokens", `{} {}`, FormatJSON, ReasonSyntax, ""},
		{"json top level array", `[1,2]`, FormatJSON, ReasonSyntax, ""},
		{"json unknown column field", `{"name":"x","columns":[{"name":"a","type":"int","width":4}]}`, FormatJSON, ReasonSyntax, "columns[0].width"},
		{"json unknown schema field", `{"name":"x","version":3,"columns":[]}`, FormatJSON, ReasonSyntax, "version"},
		{"undetectable format", `name: x`, FormatAuto, ReasonSyntax, ""},
		{"xml broken syntax", `<schema name="x">`, FormatXML, ReasonSyntax, ""},
		{"xml unknown element", `<schema name="x"><colums/></schema>`, FormatXML, ReasonSyntax, ""},
		{"xml unknown attribute", `<schema name="x" title="t"/>`, FormatXML, ReasonSyntax, ""},
		{"xml unexpected text", `<schema name="x">oops</schema>`, FormatXML, ReasonSyntax, ""},
		{"xml wrong root", `<other/>`, FormatXML, ReasonSyntax, ""},

		// Missing required fields.
		{"missing schema name", `{"columns":[]}`, FormatJSON, ReasonMissingRequired, "name"},
		{"empty schema name", `{"name":"","columns":[]}`, FormatJSON, ReasonMissingRequired, "name"},
		{"missing columns", `{"name":"x"}`, FormatJSON, ReasonMissingRequired, "columns"}, {"missing column name", `{"name":"x","columns":[{"type":"int"}]}`, FormatJSON, ReasonMissingRequired, "columns[0].name"},
		{"missing column type", `{"name":"x","columns":[{"name":"a"}]}`, FormatJSON, ReasonMissingRequired, "columns[0].type"},

		// Type mismatches (distinct from missing and from bad values).
		{"name wrong type", `{"name":7,"columns":[]}`, FormatJSON, ReasonTypeMismatch, "name"},
		{"columns not array", `{"name":"x","columns":{}}`, FormatJSON, ReasonTypeMismatch, "columns"},
		{"column entry not object", `{"name":"x","columns":[42]}`, FormatJSON, ReasonTypeMismatch, "columns[0]"},
		{"type wrong kind", `{"name":"x","columns":[{"name":"a","type":123}]}`, FormatJSON, ReasonTypeMismatch, "columns[0].type"},
		{"nullable wrong kind", `{"name":"x","columns":[{"name":"a","type":"int","nullable":"yes"}]}`, FormatJSON, ReasonTypeMismatch, "columns[0].nullable"},
		{"sources element wrong kind", `{"name":"x","columns":[{"name":"a","type":"int","sources":[9]}]}`, FormatJSON, ReasonTypeMismatch, "columns[0].sources[0]"},
		{"xml nullable wrong kind", `<schema name="x"><columns><column name="a" type="int" nullable="yes"/></columns></schema>`, FormatXML, ReasonTypeMismatch, "columns[0].nullable"},

		// Domain value validation.
		{"unsupported type", `{"name":"x","columns":[{"name":"a","type":"bytes"}]}`, FormatJSON, ReasonUnsupportedType, "columns[0].type"},
		{"duplicate column name", `{"name":"x","columns":[{"name":"a","type":"int"},{"name":"a","type":"str"}]}`, FormatJSON, ReasonDuplicateName, "columns[1].name"},
		{"malformed source ref", `{"name":"x","columns":[{"name":"a","type":"int","sources":["oops"]}]}`, FormatJSON, ReasonInvalidRef, "columns[0].sources[0]"},
		{"source ref extra dots", `{"name":"x","columns":[{"name":"a","type":"int","sources":["a.b.c"]}]}`, FormatJSON, ReasonInvalidRef, "columns[0].sources[0]"},
		{"xml unsupported type", `<schema name="x"><columns><column name="a" type="bytes"/></columns></schema>`, FormatXML, ReasonUnsupportedType, "columns[0].type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSchema([]byte(tc.data), Options{Format: tc.format})
			if err == nil {
				t.Fatalf("business=malformed-input case=%s: expected rejection, got success", tc.name)
			}
			rejects := AsRejectErrors(err)
			if len(rejects) == 0 {
				t.Fatalf("case=%s: error without structured reasons: %v", tc.name, err)
			}
			found := false
			for _, r := range rejects {
				if r.Reason == tc.wantReason {
					if tc.wantPath != "" && r.Path != tc.wantPath {
						t.Errorf("case=%s: reason matched but path=%q, want %q", tc.name, r.Path, tc.wantPath)
					}
					found = true
				}
				if strings.Contains(r.Message, "json:") || strings.Contains(r.Message, "xml:") {
					t.Errorf("case=%s: underlying library error leaked: %q", tc.name, r.Message)
				}
			}
			if !found {
				t.Fatalf("case=%s: wanted reason %s, got %+v", tc.name, tc.wantReason, rejects)
			}
			t.Logf("business=malformed-input case=%-28s result=rejected reason=%s path=%q", tc.name, rejects[0].Reason, rejects[0].Path)
		})
	}
}

func TestParseEmptyObjectRejection(t *testing.T) {
	// An empty object is treated as a schema and both required fields are
	// reported missing explicitly - nothing is silently skipped.
	_, err := ParseSchema([]byte(`{}`), Options{Format: FormatJSON})
	rejects := AsRejectErrors(err)
	seen := map[string]bool{}
	for _, r := range rejects {
		if r.Reason == ReasonMissingRequired {
			seen[r.Path] = true
		}
	}
	if !seen["name"] || !seen["columns"] {
		t.Fatalf("business=missing-required: empty object must report missing name and columns, got %+v", rejects)
	}
	t.Logf("business=missing-required case=empty-object result=%d missing-field errors", len(rejects))
}

func TestParseMultipleErrorsReported(t *testing.T) {
	// Two bad entries in one document must both be reported, not silently
	// skipped after the first failure.
	doc := `{"name":"x","columns":[
	  {"type":"int"},
	  {"name":"b","type":"bytes"}
	]}`
	_, err := ParseSchema([]byte(doc), Options{Format: FormatJSON})
	rejects := AsRejectErrors(err)
	want := map[Reason]bool{ReasonMissingRequired: false, ReasonUnsupportedType: false}
	for _, r := range rejects {
		if _, ok := want[r.Reason]; ok {
			want[r.Reason] = true
		}
	}
	for reason, seen := range want {
		if !seen {
			t.Fatalf("business=error-aggregation: reason %s not reported in %+v", reason, rejects)
		}
	}
	t.Logf("business=error-aggregation result=%d errors reported: %v", len(rejects), want)
}
