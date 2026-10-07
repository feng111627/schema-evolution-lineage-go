package parse

import (
	"bytes"
	"encoding/xml"
	"fmt"
)

// Known XML vocabulary. Anything outside these sets is a syntax rejection so
// that typos cannot be silently ignored.
var (
	xmlKnownElems = map[string]bool{
		"catalog": true, "schemas": true, "schema": true,
		"columns": true, "column": true,
		"sources": true, "source": true,
		"aliases": true, "alias": true,
	}
	xmlSchemaAttrs = map[string]bool{"name": true}
	xmlColumnAttrs = map[string]bool{
		"name": true, "id": true, "type": true,
		"nullable": true, "has_default": true,
	}
	xmlSourceAttrs = map[string]bool{"dataset": true, "column": true}
	xmlAliasAttrs  = map[string]bool{"name": true}
)

// decodeXML parses one XML document into the generic node tree. Encoding/xml
// errors are summarized into ReasonSyntax RejectErrors instead of being
// exposed verbatim.
//
// Wrapper elements (<schemas>, <columns>, <sources>, <aliases>) are
// transparent: their children are collected straight into the logical parent
// under the wrapper member name, so the tree mirrors the JSON shape.
func decodeXML(data []byte) (*node, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	root, tag, err := readXMLRoot(dec)
	if err != nil {
		return nil, err
	}
	for {
		tok, perr := dec.Token()
		if perr != nil {
			if perr == ioEOF {
				break
			}
			return nil, summarizeXML(perr)
		}
		if _, ok := tok.(xml.StartElement); ok {
			return nil, &RejectError{Reason: ReasonSyntax, Message: "multiple root elements in XML document"}
		}
	}
	if tag != "catalog" && tag != "schema" {
		return nil, &RejectError{
			Reason:  ReasonSyntax,
			Message: fmt.Sprintf("XML root element must be <catalog> or <schema>, got <%s>", tag),
		}
	}
	return root, nil
}

// sink describes where a child node must be appended.
type sink struct {
	n   *node  // logical parent node
	tag string // logical parent element name
}

// readXMLRoot reads the document's single root element.
func readXMLRoot(dec *xml.Decoder) (*node, string, error) {
	tok, err := dec.Token()
	if err != nil {
		if err == ioEOF {
			return nil, "", &RejectError{Reason: ReasonSyntax, Message: "XML document is empty"}
		}
		return nil, "", summarizeXML(err)
	}
	start, ok := tok.(xml.StartElement)
	if !ok {
		return nil, "", &RejectError{Reason: ReasonSyntax, Message: "expected an XML root element"}
	}
	return readXMLBody(dec, start, nil)
}

// readXMLBody consumes an element whose StartElement has just been read.
// parent is the logical sink (wrappers skipped). Returns the element's own
// node (nil for transparent wrappers) and its tag name.
func readXMLBody(dec *xml.Decoder, start xml.StartElement, parent *sink) (*node, string, error) {
	name := start.Name.Local
	if !xmlKnownElems[name] {
		return nil, name, &RejectError{
			Reason:  ReasonSyntax,
			Message: "unknown XML element <" + name + ">",
		}
	}
	n := &node{kind: nObj}
	attrs := knownAttrsFor(name)
	for _, a := range start.Attr {
		an := a.Name.Local
		if a.Name.Space != "" || !attrs[an] {
			return nil, name, &RejectError{
				Reason:  ReasonSyntax,
				Message: fmt.Sprintf("unknown XML attribute %q on <%s>", an, name),
			}
		}
		n.obj = append(n.obj, objMember{key: an, val: &node{kind: nStr, str: a.Value}})
	}

	wrapper := name == "schemas" || name == "columns" || name == "sources" || name == "aliases"
	// A wrapper keeps no node: children sink into its own logical parent.
	self := &sink{n: n, tag: name}
	if wrapper {
		self = parent
	}

	var text bytes.Buffer
	for {
		t, err := dec.Token()
		if err != nil {
			return nil, name, summarizeXML(err)
		}
		switch tt := t.(type) {
		case xml.StartElement:
			child, childTag, cerr := readXMLBody(dec, tt, self)
			if cerr != nil {
				return nil, name, cerr
			}
			if child != nil && !wrapper {
				placeXMLChild(n, name, childTag, child)
			}
		case xml.CharData:
			text.Write(tt)
		case xml.EndElement:
			if tt.Name.Local != name {
				return nil, name, &RejectError{
					Reason:  ReasonSyntax,
					Message: fmt.Sprintf("mismatched XML closing tag </%s>", tt.Name.Local),
				}
			}
			if wrapper {
				return nil, name, nil
			}
			if name == "alias" {
				if v := bytes.TrimSpace(text.Bytes()); len(v) > 0 {
					n.obj = append(n.obj, objMember{key: "name", val: &node{kind: nStr, str: string(v)}})
				}
				return n, name, nil
			}
			if len(bytes.TrimSpace(text.Bytes())) > 0 {
				return nil, name, &RejectError{
					Reason:  ReasonSyntax,
					Message: "unexpected text inside <" + name + ">",
				}
			}
			return n, name, nil
		}
	}
}

// placeXMLChild attaches a parsed non-wrapper child under its parent.
func placeXMLChild(parent *node, parentTag, childTag string, child *node) {
	if childTag == "schema" && (parentTag == "catalog" || parentTag == "schemas") {
		addArrMember(parent, "schemas", child)
		return
	}
	if childTag == "column" && (parentTag == "schema" || parentTag == "columns") {
		addArrMember(parent, "columns", child)
		return
	}
	if childTag == "source" && (parentTag == "column" || parentTag == "sources") {
		addArrMember(parent, "sources", child)
		return
	}
	if childTag == "alias" && (parentTag == "column" || parentTag == "aliases") {
		addArrMember(parent, "aliases", child)
	}
}

func knownAttrsFor(elem string) map[string]bool {
	switch elem {
	case "schema":
		return xmlSchemaAttrs
	case "column":
		return xmlColumnAttrs
	case "source":
		return xmlSourceAttrs
	case "alias":
		return xmlAliasAttrs
	default:
		return map[string]bool{}
	}
}

func addArrMember(n *node, key string, v *node) {
	for i := range n.obj {
		if n.obj[i].key == key {
			n.obj[i].val.arr = append(n.obj[i].val.arr, v)
			return
		}
	}
	n.obj = append(n.obj, objMember{key: key, val: &node{kind: nArr, arr: []*node{v}}})
}

func summarizeXML(err error) *RejectError {
	return &RejectError{
		Reason:  ReasonSyntax,
		Message: "document could not be parsed as XML",
		Detail:  "the XML structure is malformed",
	}
}
