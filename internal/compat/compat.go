package compat

import (
	"fmt"
	"sort"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// ChangeKind 标识变更类型。
type ChangeKind string

const (
	ChangeAdded      ChangeKind = "added"
	ChangeDropped    ChangeKind = "dropped"
	ChangeRenamed    ChangeKind = "renamed"
	ChangeTypeWiden  ChangeKind = "type_widened"
	ChangeTypeNarrow ChangeKind = "type_narrowed"
	ChangeNullTight  ChangeKind = "nullability_tightened"
	ChangeNullLoose  ChangeKind = "nullability_loosened"
	ChangeNone       ChangeKind = "unchanged"
)

// Change 描述单列在两个版本间的演进结论。
type Change struct {
	Kind     ChangeKind `json:"kind"`
	Column   string     `json:"column"`
	OldName  string     `json:"old_name,omitempty"`
	OldType  string     `json:"old_type,omitempty"`
	NewType  string     `json:"new_type,omitempty"`
	Breaking bool       `json:"breaking"`
	Reason   string     `json:"reason"`
}

// Report 是两个版本之间的兼容性判定结果。
type Report struct {
	OldVersion string   `json:"old_version"`
	NewVersion string   `json:"new_version"`
	Compatible bool     `json:"compatible"`
	Changes    []Change `json:"changes"`
}

// 数值类型的宽度顺序：只允许沿箭头方向放宽。
var numericRank = map[string]int{
	model.TypeInt:    1,
	model.TypeLong:   2,
	model.TypeFloat:  3,
	model.TypeDouble: 4,
}

// Compare 判定 new 相对 old 的向后兼容性（旧数据与旧消费者能否平滑过渡到新版本）。
// 判定是纯函数，同样输入必然得到同样顺序与结论。
func Compare(old, new *model.Schema) *Report {
	report := &Report{OldVersion: old.Version, NewVersion: new.Version, Changes: []Change{}}

	oldCols := indexColumns(old.Columns)
	newCols := indexColumns(new.Columns)

	// 1) 改名映射：显式 renamed_from 声明优先，必须指向旧版本中存在的列。
	renameTo := map[string]string{} // 旧名 -> 新名
	usedSources := map[string]bool{}
	declaredRename := map[string]bool{} // 声明了 renamed_from 的新列名（即使声明非法）
	for _, c := range new.Columns {
		if c.RenamedFrom == "" {
			continue
		}
		declaredRename[c.Name] = true
		if _, ok := oldCols[c.RenamedFrom]; !ok {
			report.Changes = append(report.Changes, Change{
				Kind:     ChangeRenamed,
				Column:   c.Name,
				OldName:  c.RenamedFrom,
				NewType:  c.Type,
				Breaking: true,
				Reason:   fmt.Sprintf("renamed_from %q does not exist in old version; cannot be distinguished from drop+add", c.RenamedFrom),
			})
			continue
		}
		if usedSources[c.RenamedFrom] {
			report.Changes = append(report.Changes, Change{
				Kind:     ChangeRenamed,
				Column:   c.Name,
				OldName:  c.RenamedFrom,
				Breaking: true,
				Reason:   fmt.Sprintf("multiple columns claim to rename %q", c.RenamedFrom),
			})
			continue
		}
		usedSources[c.RenamedFrom] = true
		renameTo[c.RenamedFrom] = c.Name
	}

	// 2) 新增列。
	added := []string{}
	for name := range newCols {
		if _, inOld := oldCols[name]; inOld || isRenameTarget(renameTo, name) || declaredRename[name] {
			continue
		}
		added = append(added, name)
	}
	sort.Strings(added)
	for _, name := range added {
		c := newCols[name]
		ch := Change{Kind: ChangeAdded, Column: name, NewType: c.Type}
		if c.Nullable {
			ch.Reason = "nullable column can be populated as NULL for existing rows"
		} else {
			ch.Breaking = true
			ch.Reason = "new NOT NULL column has no value for existing rows"
		}
		report.Changes = append(report.Changes, ch)
	}

	// 3) 删列（旧名消失且没有改名去向，显式区别于改名）。
	dropped := []string{}
	for name := range oldCols {
		if _, inNew := newCols[name]; inNew {
			continue
		}
		if _, renamed := renameTo[name]; renamed {
			continue
		}
		dropped = append(dropped, name)
	}
	sort.Strings(dropped)
	for _, name := range dropped {
		c := oldCols[name]
		report.Changes = append(report.Changes, Change{
			Kind:     ChangeDropped,
			Column:   name,
			OldType:  c.Type,
			Breaking: true,
			Reason:   "column dropped with no rename destination",
		})
	}

	// 4) 改名列：连同类型/可空性变化一并判定。
	renamedOld := make([]string, 0, len(renameTo))
	for oldName := range renameTo {
		renamedOld = append(renamedOld, oldName)
	}
	sort.Strings(renamedOld)
	for _, oldName := range renamedOld {
		newName := renameTo[oldName]
		oc, nc := oldCols[oldName], newCols[newName]
		ch := Change{
			Kind: ChangeRenamed, Column: newName, OldName: oldName,
			OldType: oc.Type, NewType: nc.Type, Breaking: true,
			Reason: "column renamed; consumers referencing the old name must update",
		}
		appendTypeNotes(&ch, oc, nc)
		report.Changes = append(report.Changes, ch)
	}

	// 5) 同名保留列：判定类型与可空性变化。
	common := []string{}
	for name := range oldCols {
		if _, ok := newCols[name]; ok {
			common = append(common, name)
		}
	}
	sort.Strings(common)
	for _, name := range common {
		oc, nc := oldCols[name], newCols[name]
		ch := Change{Column: name, OldType: oc.Type, NewType: nc.Type}
		switch {
		case oc.Type != nc.Type:
			if isNumericWiden(oc.Type, nc.Type) {
				ch.Kind = ChangeTypeWiden
				ch.Reason = fmt.Sprintf("type widened %s -> %s; old values remain representable", oc.Type, nc.Type)
			} else {
				ch.Kind = ChangeTypeNarrow
				ch.Breaking = true
				ch.Reason = fmt.Sprintf("type narrowed/changed %s -> %s; old values may not be representable", oc.Type, nc.Type)
			}
		case oc.Nullable != nc.Nullable:
			if nc.Nullable {
				ch.Kind = ChangeNullLoose
				ch.Reason = "nullability loosened; old rows unaffected"
			} else {
				ch.Kind = ChangeNullTight
				ch.Breaking = true
				ch.Reason = "nullable column tightened to NOT NULL; existing NULL rows fail"
			}
		default:
			ch.Kind = ChangeNone
			ch.Reason = "no change"
		}
		report.Changes = append(report.Changes, ch)
	}

	report.Compatible = true
	for _, c := range report.Changes {
		if c.Breaking {
			report.Compatible = false
			break
		}
	}
	return report
}

func indexColumns(cols []model.Column) map[string]model.Column {
	m := make(map[string]model.Column, len(cols))
	for _, c := range cols {
		m[c.Name] = c
	}
	return m
}

func isRenameTarget(renameTo map[string]string, newName string) bool {
	for _, to := range renameTo {
		if to == newName {
			return true
		}
	}
	return false
}

// isNumericWiden 只接受数值家族内部沿宽度顺序的放宽。
func isNumericWiden(oldType, newType string) bool {
	ro, o := numericRank[oldType]
	rn, n := numericRank[newType]
	return o && n && rn > ro
}

// appendTypeNotes 把改名列附带的类型/可空性破坏信息叠加到理由中。
func appendTypeNotes(ch *Change, oc, nc model.Column) {
	if oc.Type != nc.Type {
		if isNumericWiden(oc.Type, nc.Type) {
			ch.Reason += fmt.Sprintf("; type widened %s -> %s", oc.Type, nc.Type)
		} else {
			ch.Breaking = true
			ch.Reason += fmt.Sprintf("; type narrowed/changed %s -> %s", oc.Type, nc.Type)
		}
	}
	if oc.Nullable && !nc.Nullable {
		ch.Breaking = true
		ch.Reason += "; nullability tightened to NOT NULL"
	}
}
