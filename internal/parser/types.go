package parser

import (
	"strings"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// typeAliases 把各格式中的写法归一到统一逻辑类型。
var typeAliases = map[string]string{
	"bool":    model.TypeBoolean,
	"boolean": model.TypeBoolean,
	"int":     model.TypeInt,
	"int32":   model.TypeInt,
	"integer": model.TypeInt,
	"long":    model.TypeLong,
	"bigint":  model.TypeLong,
	"int64":   model.TypeLong,
	"float":   model.TypeFloat,
	"real":    model.TypeFloat,
	"double":  model.TypeDouble,
	"float64": model.TypeDouble,
	"string":  model.TypeString,
	"varchar": model.TypeString,
	"text":    model.TypeString,
	"char":    model.TypeString,
	"date":    model.TypeDate,
	"bytes":   model.TypeBytes,
	"binary":  model.TypeBytes,
}

// normalizeType 把类型写法归一化；无法识别时第二个返回值为 false。
// 对 varchar(n) / char(n) 这类带长度写法统一归为 string。
func normalizeType(raw string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return "", false
	}
	if i := strings.IndexByte(t, '('); i >= 0 {
		base := strings.TrimSpace(t[:i])
		if !strings.HasSuffix(t, ")") {
			return "", false
		}
		t = base
	}
	n, ok := typeAliases[t]
	return n, ok
}
