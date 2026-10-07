package parser

import (
	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// Parser 把某种本地描述格式解析为统一模型。
type Parser interface {
	Format() string
	Parse(data []byte) (*model.Schema, error)
}

// ForName 按格式名返回解析器；不支持的格式返回 ErrUnsupported。
func ForName(format string) (Parser, error) {
	switch format {
	case "json":
		return JSONParser{}, nil
	case "dsl":
		return DSLParser{}, nil
	case "sql":
		return SQLParser{}, nil
	default:
		return nil, newError(ErrUnsupported, format, "", "supported formats: json, dsl, sql", nil)
	}
}

// ParseFile 按指定格式解析字节内容。
func ParseFile(format string, data []byte) (*model.Schema, error) {
	p, err := ForName(format)
	if err != nil {
		return nil, err
	}
	return p.Parse(data)
}
