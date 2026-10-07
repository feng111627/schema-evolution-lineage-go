package lineage

import (
	"reflect"
	"testing"
)

func edge(target string, sources ...string) Edge { return Edge{Target: target, Sources: sources} }

func kinds(cs []Conflict) map[ConflictKind]int {
	m := map[ConflictKind]int{}
	for _, c := range cs {
		m[c.Kind]++
	}
	return m
}

func TestLineageConflicts(t *testing.T) {
	cases := []struct {
		name       string
		biz        string
		graph      Graph
		wantGlobal map[ConflictKind]int
	}{
		{
			name: "无环无冲突图", biz: "干净血缘：不应报任何冲突",
			graph: Graph{Edges: []Edge{
				edge("net", "gross", "tax"), edge("gross", "price", "qty"), edge("tax", "gross", "rate"),
			}},
			wantGlobal: map[ConflictKind]int{},
		},
		{
			name: "直接自环", biz: "cycle：自引用必须显式标记",
			graph:      Graph{Edges: []Edge{edge("a", "a")}},
			wantGlobal: map[ConflictKind]int{ConflictCycle: 1},
		},
		{
			name: "三节点环", biz: "cycle：a->b->c->a 必须被发现",
			graph:      Graph{Edges: []Edge{edge("a", "b"), edge("b", "c"), edge("c", "a")}},
			wantGlobal: map[ConflictKind]int{ConflictCycle: 1},
		},
		{
			name: "带环分支不影响干净分支", biz: "cycle：仅环分支被标记",
			graph:      Graph{Edges: []Edge{edge("a", "b"), edge("b", "a"), edge("x", "y")}},
			wantGlobal: map[ConflictKind]int{ConflictCycle: 1},
		},
		{
			name: "单 edge 内重复引用", biz: "duplicate_reference：同一来源出现两次",
			graph:      Graph{Edges: []Edge{edge("net", "gross", "gross", "tax")}},
			wantGlobal: map[ConflictKind]int{ConflictDuplicate: 1},
		},
		{
			name: "同名 target 相同来源集重复定义", biz: "duplicate_reference：重复定义不静默去重了事",
			graph:      Graph{Edges: []Edge{edge("net", "gross", "tax"), edge("net", "gross", "tax")}},
			wantGlobal: map[ConflictKind]int{ConflictDuplicate: 1},
		},
		{
			name: "同名 target 不同来源集", biz: "ambiguous_source：同名不同源必须显式冲突，不能任选其一",
			graph:      Graph{Edges: []Edge{edge("net", "gross", "tax"), edge("net", "gross", "discount")}},
			wantGlobal: map[ConflictKind]int{ConflictAmbiguous: 1},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, conflicts := NewAnalyzer(tc.graph)
			got := kinds(conflicts)
			if !reflect.DeepEqual(got, tc.wantGlobal) {
				t.Fatalf("[业务:%s] 全局冲突种类计数=%v 期望 %v；明细=%+v", tc.biz, got, tc.wantGlobal, conflicts)
			}
			if len(conflicts) == 0 {
				t.Logf("PASS 业务[%s] 无冲突", tc.biz)
			}
			for _, c := range conflicts {
				t.Logf("PASS 业务[%s] 标记冲突 kind=%s column=%s detail=%q", tc.biz, c.Kind, c.Column, c.Detail)
			}
		})
	}
}

func TestTraceUpstreams(t *testing.T) {
	g := Graph{Edges: []Edge{
		edge("net", "gross", "tax"),
		edge("gross", "unit_price", "qty"),
		edge("tax", "gross", "rate"),
	}}
	a, conflicts := NewAnalyzer(g)
	if len(conflicts) != 0 {
		t.Fatalf("干净图不应有冲突: %+v", conflicts)
	}
	lin := a.Trace("net")
	want := []string{"qty", "unit_price", "gross", "rate", "tax"}
	if !reflect.DeepEqual(lin.Upstreams, want) {
		t.Fatalf("[业务:上游展开] net 上游=%v 期望 %v", lin.Upstreams, want)
	}
	if len(lin.Conflicts) != 0 {
		t.Fatalf("干净链路不应有 trace 冲突: %+v", lin.Conflicts)
	}
	t.Logf("PASS 业务[上游展开去重] net -> %v（gross 虽被 net/tax 共用只出现一次）", lin.Upstreams)
}

func TestTraceLongChain(t *testing.T) {
	// 构造长度 5000 的单链，验证长血缘链结论仍然完整正确。
	const n = 5000
	edges := make([]Edge, 0, n)
	edges = append(edges, edge("c0000", "root"))
	for i := 1; i <= n; i++ {
		edges = append(edges, edge(chainName(i), chainName(i-1)))
	}
	a, conflicts := NewAnalyzer(Graph{Edges: edges})
	if len(conflicts) != 0 {
		t.Fatalf("长链不应有冲突: %+v", conflicts)
	}
	lin := a.Trace(chainName(n))
	// 上游 = c0000..c(n-1) 共 n 个链节点 + root。
	if len(lin.Upstreams) != n+1 {
		t.Fatalf("[业务:长血缘链] 上游数=%d 期望 %d", len(lin.Upstreams), n+1)
	}
	// root 必须可达，最深节点的直接上游 c4999 必须包含。
	foundRoot, foundDeep := false, false
	for _, u := range lin.Upstreams {
		if u == "root" {
			foundRoot = true
		}
		if u == chainName(n-1) {
			foundDeep = true
		}
	}
	if !foundRoot || !foundDeep {
		t.Fatalf("[业务:长血缘链] 根可达=%v 深上游可达=%v", foundRoot, foundDeep)
	}
	t.Logf("PASS 业务[长血缘链] 链长=%d 展开上游数=%d root 与最深上游均可达", n, len(lin.Upstreams))
}

func TestTraceCycleMarked(t *testing.T) {
	g := Graph{Edges: []Edge{edge("a", "b"), edge("b", "c"), edge("c", "a")}}
	a, _ := NewAnalyzer(g)
	lin := a.Trace("a")
	if len(lin.Conflicts) == 0 {
		t.Fatal("[业务:环路径 trace] 环上的 trace 必须显式带冲突")
	}
	for _, c := range lin.Conflicts {
		if c.Kind != ConflictCycle {
			t.Fatalf("期望 cycle 冲突，实际 %s", c.Kind)
		}
		t.Logf("PASS 业务[环路径 trace] column=%s detail=%q", c.Column, c.Detail)
	}
}

func chainName(i int) string {
	const digits = "0123456789"
	s := ""
	if i == 0 {
		return "c0000"
	}
	var b [4]byte
	for k := 3; k >= 0; k-- {
		b[k] = digits[i%10]
		i /= 10
	}
	s = "c" + string(b[:])
	return s
}

func TestLoadGraphRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		biz  string
		in   string
		code LoadErrorCode
	}{
		{"非 JSON", "malformed_input：底层库异常封装", "{", LoadMalformed},
		{"文档是数组", "malformed_input", "[]", LoadMalformed},
		{"缺 edges", "missing_required_field", `{}`, LoadMissingField},
		{"edges 不是数组", "type_mismatch", `{"edges":{}}`, LoadTypeMismatch},
		{"edges 为空", "missing_required_field", `{"edges":[]}`, LoadMissingField},
		{"edge 缺 target", "missing_required_field", `{"edges":[{"sources":["a"]}]}`, LoadMissingField},
		{"target 非字符串", "type_mismatch", `{"edges":[{"target":1,"sources":["a"]}]}`, LoadTypeMismatch},
		{"edge 缺 sources", "missing_required_field", `{"edges":[{"target":"x"}]}`, LoadMissingField},
		{"sources 非数组", "type_mismatch", `{"edges":[{"target":"x","sources":"a"}]}`, LoadTypeMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadGraph([]byte(tc.in))
			le, ok := err.(*LoadError)
			if !ok {
				t.Fatalf("[业务:%s] 期望 *LoadError，实际 %v", tc.biz, err)
			}
			if le.Code != tc.code {
				t.Fatalf("[业务:%s] code=%s 期望 %s", tc.biz, le.Code, tc.code)
			}
			t.Logf("PASS 业务[%s] 拒绝 code=%s entry=%q msg=%q", tc.biz, le.Code, le.Entry, le.Message)
		})
	}
}
