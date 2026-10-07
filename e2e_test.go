package bootstrap_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/compat"
	"github.com/feng111627/schema-evolution-lineage-go/internal/lineage"
	"github.com/feng111627/schema-evolution-lineage-go/internal/parser"
)

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", p, err)
	}
	return data
}

// TestFormatsAgree 验证 json/dsl/sql 三种写法对同一演进给出一致结论。
func TestFormatsAgree(t *testing.T) {
	base := filepath.Join("testdata", "schema", "orders")
	var first *compat.Report
	for _, format := range []string{"json", "dsl", "sql"} {
		oldS, err := parser.ParseFile(format, mustRead(t, filepath.Join(base, "v1."+format)))
		if err != nil {
			t.Fatalf("%s 解析 v1 失败: %v", format, err)
		}
		newS, err := parser.ParseFile(format, mustRead(t, filepath.Join(base, "v2."+format)))
		if err != nil {
			t.Fatalf("%s 解析 v2 失败: %v", format, err)
		}
		r := compat.Compare(oldS, newS)
		t.Logf("PASS 业务[三格式一致] format=%s compatible=%v 变更数=%d", format, r.Compatible, len(r.Changes))
		if first == nil {
			first = r
			continue
		}
		if !reflect.DeepEqual(first.Changes, r.Changes) {
			t.Fatalf("格式 %s 的变更结论与 json 不一致:\n%+v\nvs\n%+v", format, first.Changes, r.Changes)
		}
	}
	if first.Compatible {
		t.Fatal("orders v1->v2 含删列/改名/可空收紧，应为不兼容")
	}
}

// TestLineageFixtures 对本地血缘文件逐一断言冲突种类。
func TestLineageFixtures(t *testing.T) {
	base := filepath.Join("testdata", "lineage")
	cases := []struct {
		file string
		biz  string
		want lineage.ConflictKind
		n    int
	}{
		{"clean.json", "干净血缘无冲突", "", 0},
		{"long_chain.json", "长链条无冲突且可完整展开", "", 0},
		{"cycle.json", "循环依赖显式标记", lineage.ConflictCycle, 1},
		{"duplicate.json", "重复引用显式标记", lineage.ConflictDuplicate, 1},
		{"ambiguous.json", "同名不同源显式标记", lineage.ConflictAmbiguous, 1},
	}
	for _, tc := range cases {
		g, err := lineage.LoadGraph(mustRead(t, filepath.Join(base, tc.file)))
		if err != nil {
			t.Fatalf("[%s] 加载失败: %v", tc.file, err)
		}
		a, conflicts := lineage.NewAnalyzer(g)
		count := 0
		for _, c := range conflicts {
			if tc.want != "" && c.Kind == tc.want {
				count++
			}
			t.Logf("    [%s] 冲突 kind=%s column=%s detail=%q", tc.file, c.Kind, c.Column, c.Detail)
		}
		if count != tc.n {
			t.Fatalf("[业务:%s] 文件 %s 的 %s 冲突数=%d 期望 %d", tc.biz, tc.file, tc.want, count, tc.n)
		}
		t.Logf("PASS 业务[%s] 文件=%s 冲突符合预期(数量=%d)", tc.biz, tc.file, tc.n)
		if tc.file == "long_chain.json" {
			lin := a.Trace("n5")
			if len(lin.Upstreams) != 6 {
				t.Fatalf("长链 n5 上游数=%d 期望 6", len(lin.Upstreams))
			}
			t.Logf("PASS 业务[长链条本地复现] n5 上游=%v", lin.Upstreams)
		}
	}
}
