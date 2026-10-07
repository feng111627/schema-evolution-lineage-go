package lineage

import "sort"

// Edge 表示一条列级血缘：Target 由 Sources 推导而来。
type Edge struct {
	Target  string   `json:"target"`
	Sources []string `json:"sources"`
}

// Graph 是列级血缘图。
type Graph struct {
	Edges []Edge `json:"edges"`
}

// ConflictKind 标识血缘冲突种类。
type ConflictKind string

const (
	ConflictCycle     ConflictKind = "cycle"
	ConflictDuplicate ConflictKind = "duplicate_reference"
	ConflictAmbiguous ConflictKind = "ambiguous_source"
)

// Conflict 标记血缘中的异常，绝不静默忽略。
type Conflict struct {
	Kind   ConflictKind `json:"kind"`
	Column string       `json:"column"`
	Detail string       `json:"detail"`
}

// Lineage 是一个目标列的血缘展开结果。
type Lineage struct {
	Target    string     `json:"target"`
	Upstreams []string   `json:"upstreams"`
	Conflicts []Conflict `json:"conflicts"`
}

// Analyzer 持有血缘图并做展开与冲突检测。
type Analyzer struct {
	adj       map[string][]string
	conflicts []Conflict
}

// NewAnalyzer 构建分析器并检测全图冲突（环、重复引用、同名不同源）。
// 所有冲突都会显式保留，调用方据此决定是否拒绝结果。
func NewAnalyzer(g Graph) (*Analyzer, []Conflict) {
	a := &Analyzer{adj: map[string][]string{}}

	// 同一 target 出现多条 edge 定义：同名不同源（来源集合不一致）或重复定义。
	byTarget := map[string][]Edge{}
	for _, e := range g.Edges {
		byTarget[e.Target] = append(byTarget[e.Target], e)
	}
	targets := make([]string, 0, len(byTarget))
	for t := range byTarget {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	for _, t := range targets {
		es := byTarget[t]
		if len(es) == 1 {
			a.adj[t] = append(a.adj[t], es[0].Sources...)
			continue
		}
		// 多条 edge：逐条比对来源集合。
		allSame := true
		base := normalizedSources(es[0].Sources)
		for _, e := range es[1:] {
			if !sameSourceSet(base, normalizedSources(e.Sources)) {
				allSame = false
				break
			}
		}
		if allSame {
			a.addConflict(ConflictDuplicate, t, "target defined multiple times with identical source set")
			a.adj[t] = append(a.adj[t], es[0].Sources...)
		} else {
			a.addConflict(ConflictAmbiguous, t, "target defined multiple times with different source sets; refusing to pick one")
			// 冲突时保留第一组来源用于继续展开，但冲突已显式标记。
			a.adj[t] = append(a.adj[t], es[0].Sources...)
		}
	}

	// 单条 edge 内部的重复引用。
	for _, t := range targets {
		srcs := a.adj[t]
		counts := map[string]int{}
		var dup []string
		for _, s := range srcs {
			counts[s]++
			if counts[s] == 2 {
				dup = append(dup, s)
			}
		}
		sort.Strings(dup)
		for _, s := range dup {
			a.addConflict(ConflictDuplicate, t, "source referenced more than once: "+s)
		}
	}

	// 环检测（全图 DFS），按列名稳定输出。
	state := map[string]int{} // 0=未访问 1=在栈上 2=完成
	var dfs func(n string, stack []string)
	dfs = func(n string, stack []string) {
		state[n] = 1
		stack = append(stack, n)
		next := append([]string{}, a.adj[n]...)
		sort.Strings(next)
		for _, m := range next {
			switch state[m] {
			case 1:
				cyc := append([]string{}, stack...)
				cyc = append(cyc, m)
				a.addConflict(ConflictCycle, m, "dependency cycle: "+joinCycle(cyc))
			case 0:
				dfs(m, stack)
			}
		}
		state[n] = 2
	}
	allNodes := map[string]bool{}
	for t, ss := range a.adj {
		allNodes[t] = true
		for _, s := range ss {
			allNodes[s] = true
		}
	}
	nodes := make([]string, 0, len(allNodes))
	for n := range allNodes {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	for _, n := range nodes {
		if state[n] == 0 {
			dfs(n, nil)
		}
	}
	return a, append([]Conflict(nil), a.conflicts...)
}

// Trace 展开目标列的全部上游（去重、按首次可达顺序稳定排列）。
// 使用显式 frame 的迭代 DFS，避免长血缘链导致递归过深。
func (a *Analyzer) Trace(target string) *Lineage {
	out := &Lineage{Target: target, Upstreams: []string{}}

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	pathIndex := map[string]int{} // gray 节点在 path 中的位置
	var path []string             // 当前 DFS 调用栈
	type frame struct {
		node string
		ci   int
	}
	stack := []frame{{node: target}}
	color[target] = gray
	pathIndex[target] = 0
	path = []string{target}

	for len(stack) > 0 {
		top := &stack[len(stack)-1]
		srcs := sortedAdj(a.adj, top.node)
		if top.ci >= len(srcs) {
			color[top.node] = black
			delete(pathIndex, top.node)
			if top.node != target {
				out.Upstreams = appendUnique(out.Upstreams, top.node)
			}
			stack = stack[:len(stack)-1]
			path = path[:len(path)-1]
			continue
		}
		child := srcs[top.ci]
		top.ci++
		switch color[child] {
		case gray:
			// 回边：从 path 中 child 的位置到栈顶构成环。
			cycle := append(append([]string{}, path[pathIndex[child]:]...), child)
			out.Conflicts = append(out.Conflicts, Conflict{
				Kind:   ConflictCycle,
				Column: child,
				Detail: "cycle reached while tracing " + target + ": " + joinCycle(cycle),
			})
		case white:
			color[child] = gray
			pathIndex[child] = len(path)
			path = append(path, child)
			stack = append(stack, frame{node: child})
		}
	}
	return out
}

func sortedAdj(m map[string][]string, node string) []string {
	srcs := append([]string{}, m[node]...)
	sort.Strings(srcs)
	return srcs
}

func appendUnique(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}

func (a *Analyzer) addConflict(kind ConflictKind, col, detail string) {
	a.conflicts = append(a.conflicts, Conflict{Kind: kind, Column: col, Detail: detail})
}

func normalizedSources(srcs []string) []string {
	out := append([]string{}, srcs...)
	sort.Strings(out)
	return out
}

func sameSourceSet(a, b []string) bool {
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

func joinCycle(cycle []string) string {
	s := ""
	for i, n := range cycle {
		if i > 0 {
			s += " -> "
		}
		s += n
	}
	return s
}
