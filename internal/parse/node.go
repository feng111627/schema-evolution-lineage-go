package parse

// nkind tags the dynamically-typed node tree both decoders produce.
type nkind int

const (
	nNull nkind = iota
	nBool
	nNum
	nStr
	nArr
	nObj
)

// node is a generic, format-independent syntax tree. Values are kept as
// strings for scalars (with the original kind preserved) so that "true" in a
// string field can be distinguished from JSON true.
type node struct {
	kind nkind
	str  string       // scalar text
	arr  []*node      // nArr elements
	obj  []objMember  // nObj members, in document order
}

type objMember struct {
	key  string
	val  *node
}

func (n *node) field(name string) *node {
	if n == nil || n.kind != nObj {
		return nil
	}
	for i := range n.obj {
		if n.obj[i].key == name {
			return n.obj[i].val
		}
	}
	return nil
}

// normalizeCatalog validates a catalog tree and converts it to the model.
func normalizeCatalog(root *node, origin string) (interface{}, []*RejectError) {
	_ = root
	_ = origin
	return nil, nil
}

// normalizeSchema validates a single-schema tree and converts it to the model.
func normalizeSchema(root *node, origin string) (interface{}, []*RejectError) {
	_ = root
	_ = origin
	return nil, nil
}
