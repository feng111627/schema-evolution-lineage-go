# schema-evolution-lineage-go

本地、离线的 **数据结构（schema/契约）演进校验** 与 **列级血缘分析** 工具。
输入全部来自仓库内的本地描述文件，不访问任何外部服务，结论可在本机一条命令复现。

仅依赖 Go 标准库（`encoding/json`、`encoding/xml`），无需联网下载依赖。

## 能力一览

| 能力 | 说明 |
| --- | --- |
| 多格式解析归一 | 支持 JSON 契约与 XML 契约两种写法，统一归一到同一套模型；格式非法、必填缺失、类型不匹配等分别给出可区分的拒绝原因；底层解析库的报错不会原样透传 |
| 版本兼容性判定 | 对新增列、删列、改名、类型收窄/放宽给出稳定的 `compatible` / `breaking` 结论；**改名与“删列+加列”显式区分**；同一输入重复运行结果一致 |
| 列级血缘 | 展开到根节点的完整血缘链；**循环依赖、重复引用、同名不同源冲突** 全部显式标记，绝不静默取其中一个；长链与扇出/扇入结论正确 |
| 本地复现 | 命令行三个子命令 + 退出码，可直接放进 shell / CI |

## 目录结构

```
cmd/sel/            命令行入口
internal/model/     统一模型（Schema / Column / ColumnRef / Catalog）
internal/parse/     JSON/XML 解析、严格校验、拒绝原因封装
internal/compat/    两版本兼容性判定
internal/lineage/   列级血缘展开与冲突标记
internal/render/    稳定的文本输出
testdata/           本地样例（合法 / 畸形 / 两版本 / 血缘冲突）
docs/formats.md     支持的描述格式与字段规范
```

## 本地验证方法

要求 Go 1.25+（开发环境为 Go 1.26）。

```bash
# 1) 构建
go build ./...

# 2) 运行全部测试（-v 可看到每条用例覆盖的业务点与结果）
go test ./... -v

# 3) 构建 CLI
go build -o sel ./cmd/sel
```

### 解析并归一化

```bash
./sel parse --file testdata/catalog.xml
./sel parse --file testdata/malformed.json   # 退出码 1，逐条列出拒绝原因
```

### 两个版本的兼容性

```bash
./sel compat --old testdata/orders.v1.json --new testdata/orders.v2.json
# 也可以 JSON 与 XML 混用（按内容自动识别，或 --format 指定）
./sel compat --old testdata/orders.v1.xml  --new testdata/orders.v2.json
```

### 列级血缘

```bash
./sel lineage --file testdata/lineage.json          # 正常图，展示完整链
./sel lineage --file testdata/lineage-cycle.json    # 循环，退出码 1
./sel lineage --file testdata/lineage-dup.json      # 重复引用，退出码 1
```

**退出码约定**：`0` 正常；`1` 输入被拒绝 / 存在破坏性变更 / 存在血缘问题；`2` 命令行用法错误。

## 支持的描述格式

两种格式归一到同一模型，字段语义完全一致。详见 [docs/formats.md](docs/formats.md)。

JSON（单 schema 或 `{"schemas":[...]}` 目录）：

```json
{
  "name": "orders",
  "columns": [
    {
      "name": "amount", "id": "col-amount", "type": "double",
      "nullable": true, "has_default": true,
      "aliases": ["amount_cny"],
      "sources": ["payments.cents"]
    }
  ]
}
```

XML（`<schema>` 或 `<catalog>` 根）：

```xml
<schema name="orders">
  <columns>
    <column name="amount" id="col-amount" type="double" nullable="true" has_default="true">
      <aliases><alias>amount_cny</alias></aliases>
      <sources><source ref="payments.cents"/></sources>
    </column>
  </columns>
</schema>
```

### 拒绝原因（互不混淆，且会聚合输出）

| reason | 含义 |
| --- | --- |
| `syntax_error` | 文档本身不是合法 JSON/XML、出现未知字段/元素/属性、或结构不被接受 |
| `missing_required` | 必填字段缺失或为空（`name`、`columns`、列的 `name`/`type` 等） |
| `type_mismatch` | 字段值的标量类型不对（如 `type` 给成数字、`nullable` 给成 `"yes"`） |
| `unsupported_type` | 列类型不在归一类型集合内 |
| `duplicate_name` | 同一 schema 内列名重复 |
| `invalid_ref` | 血缘来源不是 `dataset.column` 的限定形式 |

每条拒绝都带定位 `path`（如 `columns[1].type`）。底层解析库的异常只保留安全摘要，
不会把库自身错误文本直接抛给调用方。

## 兼容性规则

判定视角：**老版本的读者/写者在新版本下是否仍然可用**。

| 变更 | 结论 | 规则 |
| --- | --- | --- |
| 新增可空列 / 带默认值列 | `compatible` | 老写者不写、老读者忽略都不影响 |
| 新增必填列（非空且无默认值） | `breaking` | 老写者无法产出该列 |
| 删除列 | `breaking` | 依赖旧列的读者会失败 |
| 改名 | `breaking`（标记为 `renamed`） | 旧列名消失，但与“删除+新增”明确区分（见下） |
| 数值类型放宽（沿 `int→long→float→double` 升级） | `compatible` | 所有旧值仍合法 |
| 数值类型收窄（反方向） | `breaking` | 旧值可能放不下 |
| 跨类型族变化（如 `long↔string`、`string→bool`） | `breaking`（`type_changed`） | 旧值无法按新类型解释 |
| 仅 `nullable` / `has_default` 标注变化 | `compatible` | 结构兼容性保留 |

### 改名如何与“删列 + 加列”区分

匹配按下列顺序进行，证据充分才认定为改名，否则一律按删除 + 新增报告：

1. **稳定 ID**：新旧列带相同且非空的 `id`，名字不同 ⇒ `renamed`；
2. **唯一别名**：没有 ID 时，新列的 `aliases` 中恰好命中唯一一个旧列名，
   且该旧列未以原名继续存在 ⇒ `renamed`；
3. 两个或以上候选（证据有歧义）或完全无证据 ⇒ 分别报告 `removed` 与 `added`。

报告中的列顺序固定：先按旧版本列顺序给出匹配项，再给删除项，新增项按新列名排序。
因此同一对输入重复运行，变更列表与总体结论完全一致。

## 血缘规则与冲突标记

- 每个衍生列的 `sources` 是一组 `dataset.column` 限定引用，按声明顺序构成直接边；
- 对每个衍生列做上游 DFS，枚举出 **所有从根列到目标列的完整链**（扇出/扇入会产生多条链）；
- 链与问题都按固定规则排序输出，重复运行结果一致。

| issue | 触发条件 |
| --- | --- |
| `cycle` | 某列（传递地）依赖回自身；同一环只报告一次，链在闭环处截断并展示 |
| `duplicate_reference` | 同一目标把同一个上游列列为来源超过一次（报告出现次序） |
| `same_name_different_source` | 同一限定列名在多个不同输入文件中被定义（来源不同），拒绝隐式二选一 |
| `unknown_reference` | 来源指向不存在的数据集或数据集中不存在的列 |

## 运行测试

测试以 **畸形输入、兼容性边界、血缘冲突** 为重点。用 `-v` 运行时，每条用例都会打印
覆盖的业务点（`business=...`）和该用例的判定结果：

```bash
go test ./... -v
```
