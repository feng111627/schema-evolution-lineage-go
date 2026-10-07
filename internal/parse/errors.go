package parse

// Reason categorizes why an entry/document was rejected. Callers can branch
// on the category without parsing error text.
type Reason string

const (
	// ReasonSyntax: the document is not well-formed for its format, or uses
	// an unknown field / unsupported construct.
	ReasonSyntax Reason = "syntax_error"
	// ReasonMissingRequired: a required field is absent or empty.
	ReasonMissingRequired Reason = "missing_required"
	// ReasonTypeMismatch: a field's value has the wrong JSON/XML type.
	ReasonTypeMismatch Reason = "type_mismatch"
	// ReasonUnsupportedType: a column type is not in the normalized type set.
	ReasonUnsupportedType Reason = "unsupported_type"
	// ReasonDuplicateName: two columns share a name inside one schema.
	ReasonDuplicateName Reason = "duplicate_name"
	// ReasonInvalidRef: a lineage source reference is malformed.
	ReasonInvalidRef Reason = "invalid_ref"
)

// RejectError is the single error shape exposed by this package. Error
// messages from the underlying parsing libraries are never passed through
// verbatim; they are summarized into Message, with optional detail in Detail.
type RejectError struct {
	Reason  Reason
	Message string
	// Path locates the offending entry using JSON/XML-ish names, e.g.
	// "schemas[1].columns[0].type". Empty for document-level errors.
	Path string
	// Detail is an optional, safe-to-display summary of the library cause.
	Detail string
}

func (e *RejectError) Error() string {
	p := e.Path
	if p != "" {
		p = " at " + p
	}
	d := ""
	if e.Detail != "" {
		d = ": " + e.Detail
	}
	return string(e.Reason) + p + ": " + e.Message + d
}

// RejectErrors aggregates one or more entry-level rejections. A document that
// contains several bad entries reports all of them instead of failing on the
// first.
type RejectErrors struct {
	Items []*RejectError
}

func (m *RejectErrors) Error() string {
	if len(m.Items) == 1 {
		return m.Items[0].Error()
	}
	return "document rejected with " + itoa(len(m.Items)) + " errors"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
