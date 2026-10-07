package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// decodeJSON parses one JSON document into the generic node tree. Every
// encoding/json failure is translated into a *RejectError with ReasonSyntax;
// the library's own error text is never passed to the caller.
func decodeJSON(data []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw interface{}
	if err := dec.Decode(&raw); err != nil {
		return nil, summarizeJSON(err)
	}
	// Reject trailing tokens after the top-level value.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != ioEOF {
		if err == nil {
			return nil, &RejectError{
				Reason:  ReasonSyntax,
				Message: "multiple top-level JSON values; expected exactly one document",
			}
		}
		return nil, summarizeJSON(err)
	}
	return toNode(raw), nil
}

// summarizeJSON maps an encoding/json error onto a safe RejectError.
func summarizeJSON(err error) *RejectError {
	switch e := err.(type) {
	case *json.SyntaxError:
		return &RejectError{
			Reason:  ReasonSyntax,
			Message: "document is not valid JSON",
			Detail:  fmt.Sprintf("malformed near byte offset %d", e.Offset),
		}
	case *json.UnmarshalTypeError:
		// decode into interface{} should not produce these, but stay defensive.
		return &RejectError{
			Reason:  ReasonTypeMismatch,
			Path:    e.Field,
			Message: "JSON value has an unexpected type",
		}
	default:
		return &RejectError{
			Reason:  ReasonSyntax,
			Message: "document could not be parsed as JSON",
		}
	}
}

func toNode(v interface{}) *node {
	switch t := v.(type) {
	case nil:
		return &node{kind: nNull}
	case bool:
		return &node{kind: nBool, str: strconv.FormatBool(t)}
	case json.Number:
		return &node{kind: nNum, str: t.String()}
	case float64:
		return &node{kind: nNum, str: strconv.FormatFloat(t, 'g', -1, 64)}
	case string:
		return &node{kind: nStr, str: t}
	case []interface{}:
		n := &node{kind: nArr, arr: make([]*node, 0, len(t))}
		for _, e := range t {
			n.arr = append(n.arr, toNode(e))
		}
		return n
	case map[string]interface{}:
		n := &node{kind: nObj}
		for k, e := range t {
			n.obj = append(n.obj, objMember{key: k, val: toNode(e)})
		}
		return n
	default:
		return &node{kind: nNull}
	}
}
