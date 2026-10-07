package compat

import (
	"reflect"
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

func col(name, typ string, nullable bool, renamedFrom ...string) model.Column {
	c := model.Column{Name: name, Type: typ, Nullable: nullable}
	if len(renamedFrom) > 0 {
		c.RenamedFrom = renamedFrom[0]
	}
	return c
}

func schema(version string, cols ...model.Column) *model.Schema {
	return &model.Schema{Name: "orders", Version: version, Columns: cols}
}

type compatCase struct {
	name       string
	biz        string
	old, new   *model.Schema
	wantCompat bool
	wantKinds  map[string]ChangeKind // 列名 -> 变更类型
}

func TestCompare(t *testing.T) {
	cases := []compatCase{
		{
			name: "新增可空列兼容", biz: "added + nullable => 兼容",
			old:        schema("v1", col("id", model.TypeLong, false)),
			new:        schema("v2", col("id", model.TypeLong, false), col("note", model.TypeString, true)),
			wantCompat: true, wantKinds: map[string]ChangeKind{"id": ChangeNone, "note": ChangeAdded},
		},
		{
			name: "新增非空列破坏", biz: "added + NOT NULL => 破坏（旧行无值）",
			old:        schema("v1", col("id", model.TypeLong, false)),
			new:        schema("v2", col("id", model.TypeLong, false), col("tenant", model.TypeLong, false)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"tenant": ChangeAdded},
		},
		{
			name: "删列破坏", biz: "dropped => 破坏，旧消费者读不到",
			old:        schema("v1", col("id", model.TypeLong, false), col("note", model.TypeString, true)),
			new:        schema("v2", col("id", model.TypeLong, false)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"note": ChangeDropped},
		},
		{
			name: "改名区别于删+加", biz: "renamed_from 显式声明 => renamed，而非 dropped+added",
			old:        schema("v1", col("amount", model.TypeLong, true)),
			new:        schema("v2", col("amount_cents", model.TypeLong, true, "amount")),
			wantCompat: false, wantKinds: map[string]ChangeKind{"amount_cents": ChangeRenamed},
		},
		{
			name: "无声明的同名消失被判定为删列而非改名", biz: "不做猜测：没有标记就是 dropped，不静默当改名",
			old:        schema("v1", col("amount", model.TypeLong, true)),
			new:        schema("v2", col("amount_cents", model.TypeLong, true)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"amount": ChangeDropped, "amount_cents": ChangeAdded},
		},
		{
			name: "renamed_from 指向不存在的旧列", biz: "改名声明无法证实 => 破坏并标记，不能当新增静默放过",
			old:        schema("v1", col("id", model.TypeLong, false)),
			new:        schema("v2", col("id", model.TypeLong, false), col("x", model.TypeLong, true, "ghost")),
			wantCompat: false, wantKinds: map[string]ChangeKind{"x": ChangeRenamed},
		},
		{
			name: "数值类型放宽 int->long 兼容", biz: "type_widened => 兼容",
			old:        schema("v1", col("x", model.TypeInt, true)),
			new:        schema("v2", col("x", model.TypeLong, true)),
			wantCompat: true, wantKinds: map[string]ChangeKind{"x": ChangeTypeWiden},
		},
		{
			name: "数值链放宽 long->double 兼容", biz: "type_widened（整数到浮点）=> 兼容",
			old:        schema("v1", col("x", model.TypeLong, true)),
			new:        schema("v2", col("x", model.TypeDouble, true)),
			wantCompat: true, wantKinds: map[string]ChangeKind{"x": ChangeTypeWiden},
		},
		{
			name: "数值类型收窄 long->int 破坏", biz: "type_narrowed => 破坏",
			old:        schema("v1", col("x", model.TypeLong, true)),
			new:        schema("v2", col("x", model.TypeInt, true)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"x": ChangeTypeNarrow},
		},
		{
			name: "跨家族类型变化破坏", biz: "string->long 不是放宽 => 按收窄/不兼容处理",
			old:        schema("v1", col("x", model.TypeString, true)),
			new:        schema("v2", col("x", model.TypeLong, true)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"x": ChangeTypeNarrow},
		},
		{
			name: "同 rank 跨家族 int->date 破坏", biz: "即便宽度序号相同，家族不同也不兼容",
			old:        schema("v1", col("x", model.TypeInt, true)),
			new:        schema("v2", col("x", model.TypeDate, true)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"x": ChangeTypeNarrow},
		},
		{
			name: "可空性放宽 null 兼容", biz: "NOT NULL -> NULL => 兼容",
			old:        schema("v1", col("x", model.TypeLong, false)),
			new:        schema("v2", col("x", model.TypeLong, true)),
			wantCompat: true, wantKinds: map[string]ChangeKind{"x": ChangeNullLoose},
		},
		{
			name: "可空性收紧破坏", biz: "NULL -> NOT NULL => 破坏（已有 NULL 行失败）",
			old:        schema("v1", col("x", model.TypeLong, true)),
			new:        schema("v2", col("x", model.TypeLong, false)),
			wantCompat: false, wantKinds: map[string]ChangeKind{"x": ChangeNullTight},
		},
		{
			name: "完全一致", biz: "unchanged => 兼容",
			old:        schema("v1", col("id", model.TypeLong, false)),
			new:        schema("v2", col("id", model.TypeLong, false)),
			wantCompat: true, wantKinds: map[string]ChangeKind{"id": ChangeNone},
		},
		{
			name: "改名同时类型收窄", biz: "renamed + narrowed => 破坏原因同时包含两点",
			old:        schema("v1", col("amount", model.TypeDouble, true)),
			new:        schema("v2", col("amount_i", model.TypeInt, true, "amount")),
			wantCompat: false, wantKinds: map[string]ChangeKind{"amount_i": ChangeRenamed},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Compare(tc.old, tc.new)
			if r.Compatible != tc.wantCompat {
				t.Fatalf("[业务:%s] compatible=%v 期望 %v；changes=%+v", tc.biz, r.Compatible, tc.wantCompat, r.Changes)
			}
			gotKinds := map[string]ChangeKind{}
			for _, ch := range r.Changes {
				if _, exists := gotKinds[ch.Column]; exists {
					t.Fatalf("[业务:%s] 列 %s 出现多条变更，结论不稳定: %+v", tc.biz, ch.Column, r.Changes)
				}
				gotKinds[ch.Column] = ch.Kind
				// 每条破坏性变更都必须有明确原因。
				if ch.Breaking && ch.Reason == "" {
					t.Fatalf("[业务:%s] 破坏性变更缺少原因: %+v", tc.biz, ch)
				}
			}
			for colName, wantKind := range tc.wantKinds {
				if gotKinds[colName] != wantKind {
					t.Fatalf("[业务:%s] 列 %s kind=%s 期望 %s", tc.biz, colName, gotKinds[colName], wantKind)
				}
			}
			t.Logf("PASS 业务[%s] compatible=%v 变更数=%d", tc.biz, r.Compatible, len(r.Changes))
			for _, ch := range r.Changes {
				t.Logf("    - 列=%-14s kind=%-22s breaking=%-5v %s", ch.Column, ch.Kind, ch.Breaking, ch.Reason)
			}
		})
	}
}

func TestCompareDeterministic(t *testing.T) {
	old := schema("v1", col("a", model.TypeLong, true), col("b", model.TypeLong, true), col("c", model.TypeLong, true))
	new := schema("v2",
		col("a", model.TypeDouble, true),
		col("b2", model.TypeLong, true, "b"),
		col("d", model.TypeString, true),
	)
	first := Compare(old, new)
	for i := 0; i < 20; i++ {
		r := Compare(old, new)
		if !reflect.DeepEqual(first, r) {
			t.Fatalf("第 %d 次运行结果与首次不一致，判定不稳定", i+1)
		}
	}
	t.Logf("PASS 业务[重复运行一致性] 连续 21 次 Compare 结果完全一致，变更数=%d", len(first.Changes))
}
