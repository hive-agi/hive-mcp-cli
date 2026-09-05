package main

import "github.com/hive-agi/hive-mcp-cli/internal/hive"

func main() {
	// Exec, not Run. Run takes the argument list as its parameters and seeks
	// the leaf command in THEM, so Run() with none always resolves to the root
	// and prints its help: `hive detect` and `hive setup` did nothing and
	// exited 0. Exec reads os.Args, dispatches, and sets the exit status.
	hive.Cmd.Exec()
}
