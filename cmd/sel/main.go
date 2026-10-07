// Command sel is a local, offline schema-evolution and column-lineage checker.
//
// Usage:
//
//	sel parse   --file <path> [--format json|xml]
//	sel compat  --old <path> --new <path>
//	sel lineage --file <catalog path>
//
// Exit code is 0 when there are no errors/breaking changes/issues and 1
// otherwise, so the conclusions are reproducible from a shell.
package main

import "fmt"

func main() {
	fmt.Println("sel: not implemented")
}
