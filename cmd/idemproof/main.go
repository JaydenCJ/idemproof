// Command idemproof proves a command is idempotent by running it twice
// (or more) and diffing filesystem and output effects between runs.
package main

import (
	"os"

	"github.com/JaydenCJ/idemproof/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
