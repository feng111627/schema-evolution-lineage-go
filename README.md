# seline — 本地 schema 演进校验与列级血缘分析

一个零外部依赖的本地命令行工具：

- 把多种写法的本地 schema/契约描述解析并**归一到统一模型**；
- 判定两个版本之间的**兼容性**（新增列 / 删列 / 改名 / 类型收窄或放宽）；
- 做**列级血缘**的展开与追踪，显式标记循环依赖、重复引用、同名不同源冲突。

只使用 Go 标准库，不访问任何网络或外部服务，所有结论都可用仓库内的 `testdata/` 在本地复现。

## 构建

```bash
go build ./...
# 可选：安装到 $GOPATH/bin
go install ./cmd/seline
```

## 支持的描述格式

| 格式  | 形态                                   | 版本声明              | 改名标记                         |
|-------|----------------------------------------|-----------------------|----------------------------------|
| json  | JSON 对象，含 `schema/version/columns` | `version` 字段        | `renamed_from` 字段              |
| dsl   | 行式 `key: value` 契约（支持 `#` 注释）| `version: x`          | `renamed_from=old` 列内 token    |
| sql   | `CREATE TABLE` DDL 子集                | `-- version: x` 注释  | `RENAMED_FROM old` 列尾子句      |

三种格式解析后归一到同一模型：`{schema, version, columns[{name,type,nullable,renamed_from}]}`。

类型别名会被归一到统一逻辑类型，例如 `bigint/int64/long → long`、`varchar(n)/text/char → string`、`real → float`、`binary → bytes`。
无法识别的类型一律拒绝（不会被静默跳过）。

### JSON 示例

```json
{
  "schema": "orders",
  "version": "v2",
  "columns": [
    {"name": "id", "type": "long", "nullable": false},
    {"name": "amount_cents", "type": "double", "nullable": true, "renamed_from": "amount"}
  ]
}
```

### DSL 示例

```
schema: orders
version: v2
column: id bigint not null
column: amount_cents double null renamed_from=amount
```

### SQL 示例

```sql
-- version: v2
CREATE TABLE orders (
  id BIGINT NOT NULL,
  amount_cents DOUBLE NULL RENAMED_FROM amount
);
```

### 解析拒绝原因（不静默跳过）

所有解析错误都被封装为统一的 `parser.Error`（带 `Code/Format/Entry/Message`），底层库的原始报错只挂在 `Cause` 上（可 `errors.As/Unwrap`），**不会原样透给调用方**。可区分的原因码：

| Code                     | 含义                                       |
|--------------------------|--------------------------------------------|
| `malformed_input`        | 整体语法非法（坏 JSON、非 CREATE TABLE 等）|
| `missing_required_field` | 必填字段/指令缺失或为空                    |
| `type_mismatch`          | 字段类型不符、类型无法识别                 |
| `empty_entry`            | 列清单为空、列定义为空                     |
| `duplicate_entry`        | 同一 schema 内列名重复                     |
| `unsupported_format`     | 请求了 json/dsl/sql 之外的格式             |

血缘文件使用独立的 `lineage.LoadError`，原因码为 `malformed_input / missing_required_field / type_mismatch`。

## 兼容性规则

判定方向：**新版本相对旧版本是否向后兼容**（旧数据与旧消费者能否平滑过渡）。
判定是纯函数，对同一输入重复运行得到完全一致的结论与输出顺序。

| 变更                                   | 结论        | 说明                                                     |
|----------------------------------------|-------------|----------------------------------------------------------|
| 新增可空列                             | 兼容        | 旧行可填 NULL                                            |
| 新增非空列                             | **破坏**    | 旧行没有值                                               |
| 删列                                   | **破坏**    | 旧消费者读不到该列                                       |
| 改名（显式 `renamed_from`）            | **破坏**    | 标记为 `renamed`，旧名引用方需更新                       |
| 无声明地“旧名消失 + 相似新名出现”      | 删列 + 新增 | **不做猜测**，与改名明确区分开                           |
| `renamed_from` 指向不存在的旧列        | **破坏**    | 改名无法证实，显式标记，不能当新增静默放过               |
| 数值类型放宽 `int<long<float<double`   | 兼容        | 旧值仍可表示，标记 `type_widened`                        |
| 数值类型收窄（逆方向）                 | **破坏**    | 旧值可能被截断，标记 `type_narrowed`                     |
| 跨家族变化（如 `string→long`、`int→date`） | **破坏** | 不是放宽，按不兼容处理                                   |
| `NOT NULL → NULL`                      | 兼容        | `nullability_loosened`                                   |
| `NULL → NOT NULL`                      | **破坏**    | 已有 NULL 行会失败，`nullability_tightened`              |

改名列会同时附带类型/可空性变化结论（例如改名 + 类型收窄，原因中两点都列出）。

## 列级血缘

血缘文件格式：

```json
{
  "edges": [
    {"target": "net", "sources": ["gross", "tax"]},
    {"target": "gross", "sources": ["unit_price", "qty"]}
  ]
}
```

`Trace(target)` 迭代式 DFS 展开全部上游（去重、顺序稳定），长链条也不会递归过深。
以下情况**全部显式标记为冲突，绝不静默任选一个**：

| Conflict               | 触发条件                                       |
|------------------------|------------------------------------------------|
| `cycle`                | 存在回边（含自环），如 `a→b→c→a`               |
| `duplicate_reference`  | 同一条边内来源重复，或同名 target 用同一来源集重复定义 |
| `ambiguous_source`     | 同名 target 出现**不同**来源集（同名不同源）   |

## CLI 用法

```bash
# 解析并归一化
go run ./cmd/seline parse json testdata/schema/orders/v1.json
go run ./cmd/seline parse dsl  testdata/schema/orders/v2.dsl
go run ./cmd/seline parse sql  testdata/schema/orders/v2.sql

# 版本兼容性（兼容退出码 0；不兼容退出码 3）
go run ./cmd/seline compat json testdata/schema/orders/v1.json testdata/schema/orders/v2.json

# 血缘：先给全局冲突，再展开指定目标列（无冲突退出码 0；有冲突退出码 4）
go run ./cmd/seline lineage testdata/lineage/clean.json net
go run ./cmd/seline lineage testdata/lineage/cycle.json a
```

解析/加载失败退出码为 1，参数错误为 2。所有结果以缩进 JSON 打印到 stdout，错误打印到 stderr。

## 本地验证（测试）

```bash
# 全量测试；-v 日志可看到每条用例覆盖的业务点与结果
go test -v ./...

# 只看某一类边界
go test -v ./internal/parser   # 畸形输入 / 缺字段 / 类型不匹配 / 重复
go test -v ./internal/compat    # 兼容性边界 + 重复运行一致性
go test -v ./internal/lineage   # 环 / 重复引用 / 同名不同源 / 5000 长链
```

测试重点：

- **畸形输入**：三种格式各覆盖坏语法、缺顶层/列级必填、类型不匹配、空条目、重复列、未知指令、底层库异常被封装不透传；
- **兼容性边界**：新增可空/非空列、删列、改名与“删+加”区分、改名来源不存在、类型放宽/收窄/跨家族、可空性收紧/放宽、重复 21 次结果一致；
- **血缘冲突**：自环/多节点环/带环分支、边内重复、重复定义、同名不同源、菱形去重、5000 节点长链完整展开。

每条用例均通过 `t.Logf` 输出“覆盖业务 + 用例名 + 结果/拒绝原因”，`go test -v` 日志中可直接核对覆盖面。

## 目录结构

```
cmd/seline/            CLI 入口（parse / compat / lineage）
internal/model/        统一模型与归一化类型常量
internal/parser/       json / dsl / sql 解析器 + 统一错误封装
internal/compat/       两版本兼容性判定
internal/lineage/      血缘图加载、冲突检测与上游展开
testdata/              本地可复现的 schema 与血缘样例（含各类冲突）
```
