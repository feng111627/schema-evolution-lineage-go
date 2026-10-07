# 支持的描述格式

解析器把两种写法的本地文件归一到同一套模型
（`internal/model`：`Schema` / `Column` / `ColumnRef` / `Catalog`）。
格式按首个非空白字符自动识别：`{` 开头按 JSON，`<` 开头按 XML；也可用
`--format json|xml` 显式指定。

两种根形态都被接受：

- **目录（catalog）**：描述多个数据集，是血缘分析的输入单位；
- **单 schema**：描述一个数据集。用于 `compat`；`parse` 时会包成单元素目录。

## 归一化类型集合

`bool`、`int`、`long`、`float`、`double`、`string`。

数值提升链（兼容性判定使用）：

```
int  ->  long  ->  float  ->  double
```

## 字段规范

### Schema

| 字段 | 必填 | 类型 | 说明 |
| --- | --- | --- | --- |
| `name` | 是 | string | 数据集名，非空 |
| `columns` | 是 | array | 列数组；单 schema 文档中必须存在 |

目录形态下顶层字段为 `schemas`（schema 数组，必填）。

### Column

| 字段 | 必填 | 类型 | 说明 |
| --- | --- | --- | --- |
| `name` | 是 | string | 列名，同一 schema 内不可重复 |
| `type` | 是 | string | 归一类型之一，否则 `unsupported_type` |
| `id` | 否 | string | 跨版本稳定标识，用于可靠识别改名 |
| `nullable` | 否 | bool | 是否可空，默认 false |
| `has_default` | 否 | bool | 是否带默认值，默认 false |
| `aliases` | 否 | string[] | 历史列名（用于无 id 时的改名识别） |
| `sources` | 否 | string[] | 血缘上游，元素必须是 `dataset.column` |

规则：

- 出现任何未知字段 ⇒ `syntax_error`（不会静默忽略拼写错误）；
- 标量类型不符 ⇒ `type_mismatch`（例如 JSON 里 `"type": 123`、XML 里 `nullable="yes"`）；
- 多个条目的错误会 **聚合** 一次性报告，不会在第一个错误处中断。

## JSON 写法

单 schema：

```json
{
  "name": "orders",
  "columns": [
    {
      "name": "amount_total",
      "id": "col-amount",
      "type": "double",
      "nullable": true,
      "has_default": false,
      "aliases": ["amount_cny"],
      "sources": ["payments.cents"]
    }
  ]
}
```

目录：

```json
{
  "schemas": [
    {"name": "payments", "columns": [
      {"name": "cents", "type": "long"}
    ]},
    {"name": "orders", "columns": [
      {"name": "amount", "type": "double", "sources": ["payments.cents"]}
    ]}
  ]
}
```

注意：JSON 布尔值必须是字面量 `true`/`false`；`"true"`（字符串）会被判为类型不匹配。

## XML 写法

元素白名单：`catalog`、`schemas`、`schema`、`columns`、`column`、
`sources`、`source`、`aliases`、`alias`。任何其他元素或属性都是 `syntax_error`。
`<schemas>/<columns>/<sources>/<aliases>` 只是分组包装元素，可省略也可保留。

- `schema`：属性 `name`
- `column`：属性 `name`、`id`、`type`、`nullable`、`has_default`
- `source`：属性 `ref`，取值为 `dataset.column`
- `alias`：可用 `name` 属性，也可直接写文本内容

单 schema：

```xml
<schema name="orders">
  <columns>
    <column name="amount_total" id="col-amount" type="double"
            nullable="true" has_default="false">
      <aliases><alias name="amount_cny"/></aliases>
      <sources><source ref="payments.cents"/></sources>
    </column>
  </columns>
</schema>
```

目录：

```xml
<catalog>
  <schemas>
    <schema name="payments">
      <columns>
        <column name="cents" type="long"/>
      </columns>
    </schema>
    <schema name="orders">
      <columns>
        <column name="amount" type="double">
          <sources><source ref="payments.cents"/></sources>
        </column>
      </columns>
    </schema>
  </schemas>
</catalog>
```

XML 的布尔属性只接受字面量 `true` / `false`（`yes`/`1` 都会被拒绝）。

## 错误定位

每条拒绝带一个 `path`：

- JSON：`schemas[1].columns[0].type`
- XML：内部归一为同样的逻辑路径，如 `columns[0].nullable`

错误不直接暴露 `encoding/json`、`encoding/xml` 的原始报错文本，
只输出稳定的原因类别、定位与安全摘要，便于调用方按 `reason` 分支处理。
