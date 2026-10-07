package lineage

import (
	"encoding/json"
	"fmt"
)

// LoadErrorCode 区分血缘文件的拒绝原因。
type LoadErrorCode string

const (
	LoadMalformed    LoadErrorCode = "malformed_input"
	LoadMissingField LoadErrorCode = "missing_required_field"
	LoadTypeMismatch LoadErrorCode = "type_mismatch"
)

// LoadError 统一封装血缘文件解析异常。
type LoadError struct {
	Code    LoadErrorCode
	Entry   string
	Message string
	Cause   error
}

func (e *LoadError) Error() string {
	return fmt.Sprintf("lineage load error: code=%s entry=%q: %s", e.Code, e.Entry, e.Message)
}

func (e *LoadError) Unwrap() error { return e.Cause }

// LoadGraph 解析血缘 JSON：
//
//	{"edges": [{"target": "net", "sources": ["gross", "tax"]}]}
func LoadGraph(data []byte) (Graph, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return Graph{}, &LoadError{Code: LoadMalformed, Entry: "", Message: "document is not a valid JSON object", Cause: err}
	}
	raw, ok := top["edges"]
	if !ok {
		return Graph{}, &LoadError{Code: LoadMissingField, Entry: "edges", Message: "field is required"}
	}
	var edgeMsgs []json.RawMessage
	if err := json.Unmarshal(raw, &edgeMsgs); err != nil {
		return Graph{}, &LoadError{Code: LoadTypeMismatch, Entry: "edges", Message: "must be an array of edge objects", Cause: err}
	}
	if len(edgeMsgs) == 0 {
		return Graph{}, &LoadError{Code: LoadMissingField, Entry: "edges", Message: "at least one edge is required"}
	}

	g := Graph{Edges: make([]Edge, 0, len(edgeMsgs))}
	for i, em := range edgeMsgs {
		entry := fmt.Sprintf("edges[%d]", i)
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(em, &fields); err != nil {
			return Graph{}, &LoadError{Code: LoadTypeMismatch, Entry: entry, Message: "edge must be a JSON object", Cause: err}
		}
		var e Edge
		targetRaw, ok := fields["target"]
		if !ok {
			return Graph{}, &LoadError{Code: LoadMissingField, Entry: entry + ".target", Message: "field is required"}
		}
		if err := json.Unmarshal(targetRaw, &e.Target); err != nil || e.Target == "" {
			return Graph{}, &LoadError{Code: LoadTypeMismatch, Entry: entry + ".target", Message: "must be a non-empty string"}
		}
		srcRaw, ok := fields["sources"]
		if !ok {
			return Graph{}, &LoadError{Code: LoadMissingField, Entry: entry + ".sources", Message: "field is required"}
		}
		if err := json.Unmarshal(srcRaw, &e.Sources); err != nil {
			return Graph{}, &LoadError{Code: LoadTypeMismatch, Entry: entry + ".sources", Message: "must be an array of strings", Cause: err}
		}
		g.Edges = append(g.Edges, e)
	}
	return g, nil
}
