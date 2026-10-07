package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/feng111627/schema-evolution-lineage-go/internal/compat"
	"github.com/feng111627/schema-evolution-lineage-go/internal/lineage"
	"github.com/feng111627/schema-evolution-lineage-go/internal/parser"
)

const usage = `seline - 本地 schema 演进校验与列级血缘工具

用法:
  seline parse  <format> <file>                     解析并归一化 schema 描述
  seline compat <format> <oldFile> <newFile>        判定两个版本的兼容性
  seline lineage <file> [target ...]                检测血缘冲突并展开目标列

format 取值: json | dsl | sql
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "parse":
		err = runParse(os.Args[2:])
	case "compat":
		err = runCompat(os.Args[2:])
	case "lineage":
		err = runLineage(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runParse(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("parse requires <format> <file>")
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	schema, err := parser.ParseFile(args[0], data)
	if err != nil {
		return err
	}
	return printJSON(schema)
}

func runCompat(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("compat requires <format> <oldFile> <newFile>")
	}
	oldData, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	newData, err := os.ReadFile(args[2])
	if err != nil {
		return err
	}
	oldSchema, err := parser.ParseFile(args[0], oldData)
	if err != nil {
		return fmt.Errorf("old version: %w", err)
	}
	newSchema, err := parser.ParseFile(args[0], newData)
	if err != nil {
		return fmt.Errorf("new version: %w", err)
	}
	report := compat.Compare(oldSchema, newSchema)
	if err := printJSON(report); err != nil {
		return err
	}
	if !report.Compatible {
		os.Exit(3)
	}
	return nil
}

func runLineage(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("lineage requires <file> [target ...]")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	graph, err := lineage.LoadGraph(data)
	if err != nil {
		return err
	}
	analyzer, conflicts := lineage.NewAnalyzer(graph)

	out := map[string]any{"global_conflicts": conflicts}
	if len(args) > 1 {
		traces := make([]*lineage.Lineage, 0, len(args)-1)
		for _, t := range args[1:] {
			traces = append(traces, analyzer.Trace(t))
		}
		out["traces"] = traces
	}
	if err := printJSON(out); err != nil {
		return err
	}
	if len(conflicts) > 0 {
		os.Exit(4)
	}
	return nil
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
