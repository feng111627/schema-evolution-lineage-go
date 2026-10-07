package parser

import (
	"errors"
	"testing"

	"github.com/feng111627/schema-evolution-lineage-go/internal/model"
)

// parseCase 覆盖一条“业务点 -> 输入 -> 预期拒绝原因/成功”的用例。
type parseCase struct {
	name    string
	biz     string // 覆盖的业务点
	input   string
	wantErr ErrorCode
	want    *model.Schema
}

func TestJSONParser(t *testing.T) {
	cases := []parseCase{
		{
			name: "json 合法输入归一化", biz: "JSON 正常解析 + 类型别名归一 + renamed_from",
			input: `{"schema":"orders","version":"v1","columns":[
				{"name":"id","type":"bigint","nullable":false},
				{"name":"amt","type":"double","renamed_from":"amount"}]}`,
			want: &model.Schema{Name: "orders", Format: "json", Version: "v1", Columns: []model.Column{
				{Name: "id", Type: model.TypeLong, Nullable: false},
				{Name: "amt", Type: model.TypeDouble, Nullable: true, RenamedFrom: "amount"},
			}},
		},
		{name: "json 整体非法 JSON", biz: "malformed_input：底层 JSON 库异常被封装", input: `{not json`, wantErr: ErrMalformed},
		{name: "json 文档不是对象", biz: "malformed_input", input: `[1,2,3]`, wantErr: ErrMalformed},
		{name: "json 缺 schema", biz: "missing_required_field：顶层必填", input: `{"version":"v1","columns":[{"name":"a","type":"int"}]}`, wantErr: ErrMissingField},
		{name: "json schema 不是字符串", biz: "type_mismatch：字段类型不匹配", input: `{"schema":1,"version":"v1","columns":[]}`, wantErr: ErrTypeMismatch},
		{name: "json 缺 columns", biz: "missing_required_field", input: `{"schema":"s","version":"v1"}`, wantErr: ErrMissingField},
		{name: "json columns 不是数组", biz: "type_mismatch", input: `{"schema":"s","version":"v1","columns":{}}`, wantErr: ErrTypeMismatch},
		{name: "json columns 为空", biz: "empty_entry：至少一列", input: `{"schema":"s","version":"v1","columns":[]}`, wantErr: ErrEmptyEntry},
		{name: "json 列不是对象", biz: "type_mismatch", input: `{"schema":"s","version":"v1","columns":[42]}`, wantErr: ErrTypeMismatch},
		{name: "json 列缺 name", biz: "missing_required_field：列级必填", input: `{"schema":"s","version":"v1","columns":[{"type":"int"}]}`, wantErr: ErrMissingField},
		{name: "json 列缺 type", biz: "missing_required_field", input: `{"schema":"s","version":"v1","columns":[{"name":"a"}]}`, wantErr: ErrMissingField},
		{name: "json type 无法识别", biz: "type_mismatch：未知类型不被静默跳过", input: `{"schema":"s","version":"v1","columns":[{"name":"a","type":"weird"}]}`, wantErr: ErrTypeMismatch},
		{name: "json type 括号不闭合", biz: "malformed/type 校验", input: `{"schema":"s","version":"v1","columns":[{"name":"a","type":"varchar(16"}]}`, wantErr: ErrTypeMismatch},
		{name: "json nullable 非布尔", biz: "type_mismatch", input: `{"schema":"s","version":"v1","columns":[{"name":"a","type":"int","nullable":"yes"}]}`, wantErr: ErrTypeMismatch},
		{name: "json 重复列名", biz: "duplicate_entry", input: `{"schema":"s","version":"v1","columns":[{"name":"a","type":"int"},{"name":"a","type":"long"}]}`, wantErr: ErrDuplicateEntry},
		{name: "json renamed_from 非字符串", biz: "type_mismatch", input: `{"schema":"s","version":"v1","columns":[{"name":"a","type":"int","renamed_from":7}]}`, wantErr: ErrTypeMismatch},
	}
	runParseCases(t, JSONParser{}, cases)
}

func TestDSLParser(t *testing.T) {
	cases := []parseCase{
		{
			name: "dsl 合法输入归一化", biz: "DSL 正常解析 + 注释/空行跳过 + not null + renamed_from=",
			input: "# header\n\nschema: orders\nversion: v2\ncolumn: id bigint not null\ncolumn: amt double null renamed_from=amount\n",
			want: &model.Schema{Name: "orders", Format: "dsl", Version: "v2", Columns: []model.Column{
				{Name: "id", Type: model.TypeLong, Nullable: false},
				{Name: "amt", Type: model.TypeDouble, Nullable: true, RenamedFrom: "amount"},
			}},
		},
		{name: "dsl 行缺少冒号", biz: "malformed_input", input: "schema orders\n", wantErr: ErrMalformed},
		{name: "dsl 未知指令", biz: "malformed_input：不静默忽略", input: "schema: s\nversion: v\nfoobar: x\ncolumn: a int\n", wantErr: ErrMalformed},
		{name: "dsl 缺 schema", biz: "missing_required_field", input: "version: v\ncolumn: a int\n", wantErr: ErrMissingField},
		{name: "dsl 缺 version", biz: "missing_required_field", input: "schema: s\ncolumn: a int\n", wantErr: ErrMissingField},
		{name: "dsl 没有列", biz: "empty_entry", input: "schema: s\nversion: v\n", wantErr: ErrEmptyEntry},
		{name: "dsl 列定义为空", biz: "empty_entry", input: "schema: s\nversion: v\ncolumn:\n", wantErr: ErrEmptyEntry},
		{name: "dsl 列只有名字", biz: "missing_required_field：缺少类型", input: "schema: s\nversion: v\ncolumn: a\n", wantErr: ErrMissingField},
		{name: "dsl 类型未知", biz: "type_mismatch", input: "schema: s\nversion: v\ncolumn: a weird\n", wantErr: ErrTypeMismatch},
		{name: "dsl 非法 token", biz: "malformed_input", input: "schema: s\nversion: v\ncolumn: a int blah\n", wantErr: ErrMalformed},
		{name: "dsl not 后非 null", biz: "malformed_input", input: "schema: s\nversion: v\ncolumn: a int not varchar\n", wantErr: ErrMalformed},
		{name: "dsl renamed_from 值为空", biz: "missing_required_field", input: "schema: s\nversion: v\ncolumn: a int renamed_from=\n", wantErr: ErrMissingField},
		{name: "dsl 重复列名", biz: "duplicate_entry", input: "schema: s\nversion: v\ncolumn: a int\ncolumn: a long\n", wantErr: ErrDuplicateEntry},
	}
	runParseCases(t, DSLParser{}, cases)
}

func TestSQLParser(t *testing.T) {
	cases := []parseCase{
		{
			name: "sql 合法输入归一化", biz: "SQL 子集解析 + 版本注释 + VARCHAR(n) + RENAMED_FROM",
			input: "-- version: v1\nCREATE TABLE orders (\n  id BIGINT NOT NULL,\n  amt DOUBLE NULL RENAMED_FROM amount\n);\n",
			want: &model.Schema{Name: "orders", Format: "sql", Version: "v1", Columns: []model.Column{
				{Name: "id", Type: model.TypeLong, Nullable: false},
				{Name: "amt", Type: model.TypeDouble, Nullable: true, RenamedFrom: "amount"},
			}},
		},
		{name: "sql 缺版本注释", biz: "missing_required_field", input: "CREATE TABLE a (id INT);", wantErr: ErrMissingField},
		{name: "sql 非 CREATE TABLE 开头", biz: "malformed_input", input: "-- version: v1\nDROP TABLE a;", wantErr: ErrMalformed},
		{name: "sql 缺少括号列清单", biz: "malformed_input", input: "-- version: v1\nCREATE TABLE a;", wantErr: ErrMalformed},
		{name: "sql 列清单为空", biz: "empty_entry", input: "-- version: v1\nCREATE TABLE a ();", wantErr: ErrEmptyEntry},
		{name: "sql 列缺少类型", biz: "missing_required_field", input: "-- version: v1\nCREATE TABLE a (id);", wantErr: ErrMissingField},
		{name: "sql 类型未知", biz: "type_mismatch", input: "-- version: v1\nCREATE TABLE a (id WEIRD);", wantErr: ErrTypeMismatch},
		{name: "sql NOT 后缺 NULL", biz: "malformed_input", input: "-- version: v1\nCREATE TABLE a (id INT NOT VARCHAR);", wantErr: ErrMalformed},
		{name: "sql 表级约束不支持", biz: "malformed_input：不静默接受", input: "-- version: v1\nCREATE TABLE a (id INT, PRIMARY KEY (id));", wantErr: ErrMalformed},
		{name: "sql 非法列子句", biz: "malformed_input", input: "-- version: v1\nCREATE TABLE a (id INT FOOBAR);", wantErr: ErrMalformed},
		{name: "sql RENAMED_FROM 缺列名", biz: "missing_required_field", input: "-- version: v1\nCREATE TABLE a (id INT RENAMED_FROM);", wantErr: ErrMissingField},
		{name: "sql COMMENT 缺文本", biz: "malformed_input", input: "-- version: v1\nCREATE TABLE a (id INT COMMENT);", wantErr: ErrMalformed},
		{name: "sql 重复列名", biz: "duplicate_entry", input: "-- version: v1\nCREATE TABLE a (id INT, id BIGINT);", wantErr: ErrDuplicateEntry},
	}
	runParseCases(t, SQLParser{}, cases)
}

func runParseCases(t *testing.T, p Parser, cases []parseCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.Parse([]byte(tc.input))
			if tc.wantErr != "" {
				var pe *Error
				if !errors.As(err, &pe) {
					t.Fatalf("[业务:%s] 期望 *parser.Error，实际 %v", tc.biz, err)
				}
				if pe.Code != tc.wantErr {
					t.Fatalf("[业务:%s] 拒绝原因 code=%s，期望 %s (entry=%s, msg=%s)", tc.biz, pe.Code, tc.wantErr, pe.Entry, pe.Message)
				}
				t.Logf("PASS 业务[%s] 用例[%s] 被拒绝 code=%s entry=%q msg=%q", tc.biz, tc.name, pe.Code, pe.Entry, pe.Message)
				return
			}
			if err != nil {
				t.Fatalf("[业务:%s] 未期望错误: %v", tc.biz, err)
			}
			if !schemaEqual(got, tc.want) {
				t.Fatalf("[业务:%s] 归一化结果不符:\n got=%+v\nwant=%+v", tc.biz, got, tc.want)
			}
			t.Logf("PASS 业务[%s] 用例[%s] 解析成功 schema=%s version=%s 列数=%d", tc.biz, tc.name, got.Name, got.Version, len(got.Columns))
		})
	}
}

func schemaEqual(a, b *model.Schema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || a.Format != b.Format || a.Version != b.Version || len(a.Columns) != len(b.Columns) {
		return false
	}
	for i := range a.Columns {
		if a.Columns[i] != b.Columns[i] {
			return false
		}
	}
	return true
}

func TestForName(t *testing.T) {
	if _, err := ForName("yaml"); err == nil {
		t.Fatal("不支持的格式应返回 ErrUnsupported，而不是被静默接受")
	} else if pe, ok := err.(*Error); !ok || pe.Code != ErrUnsupported {
		t.Fatalf("期望 ErrUnsupported，实际 %v", err)
	} else {
		t.Logf("PASS 业务[格式分发] 不支持格式被拒绝 code=%s", pe.Code)
	}
	for _, f := range []string{"json", "dsl", "sql"} {
		p, err := ForName(f)
		if err != nil || p.Format() != f {
			t.Fatalf("格式 %s 分发失败: %v", f, err)
		}
		t.Logf("PASS 业务[格式分发] 格式 %s -> %T", f, p)
	}
}

func TestUnderlyingErrorWrappedNotLeaked(t *testing.T) {
	_, err := JSONParser{}.Parse([]byte("{bad"))
	pe, ok := err.(*Error)
	if !ok {
		t.Fatalf("期望封装错误，实际 %T", err)
	}
	if pe.Cause == nil {
		t.Fatal("封装错误应保留底层 cause 以便 Unwrap，但不应直接展示给调用方")
	}
	if pe.Error() == pe.Cause.Error() {
		t.Fatal("封装后的 Error() 不应与底层库报错完全一致")
	}
	t.Logf("PASS 业务[底层异常封装] 对外文案=%q（底层 cause 仅可 Unwrap）", pe.Error())
}
