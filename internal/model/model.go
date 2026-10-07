package model

// Column 是归一化后的列描述。
type Column struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Nullable    bool   `json:"nullable"`
	RenamedFrom string `json:"schema_renamed_from,omitempty"`
}

// Schema 是归一化后的 schema/契约描述。
type Schema struct {
	Name    string
	Format  string
	Version string
	Columns []Column
}

// 归一化后的逻辑类型名。
const (
	TypeBoolean = "boolean"
	TypeInt     = "int"
	TypeLong    = "long"
	TypeFloat   = "float"
	TypeDouble  = "double"
	TypeString  = "string"
	TypeDate    = "date"
	TypeBytes   = "bytes"
)

// RenameMarker 是列描述中声明改名来源的字段名。
const RenameMarker = "renamed_from"
