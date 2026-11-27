// main 只做装配与调用，不含业务分支。
package main

import (
	"os"

	"ritsu/internal/surface/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
