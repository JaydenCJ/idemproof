// idemproof — proves a command is idempotent by double-running it and
// diffing filesystem and output effects.
//
// version:    0.1.0
// author:     JaydenCJ
// license:    MIT
// repository: https://github.com/JaydenCJ/idemproof
// keywords:   idempotency, testing, cli, sre, infrastructure, migrations, shell
//
// Zero runtime dependencies: standard library only.
module github.com/JaydenCJ/idemproof

go 1.22
