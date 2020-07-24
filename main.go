//
// Entry-point for our application.
//

package main

import (
	"context"
	"flag"
	"os"

	"github.com/google/subcommands"
)

//
// Setup our sub-commands and use them.
//
func main() {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&listCmd{}, "")
	subcommands.Register(&addCmd{}, "")
	subcommands.Register(&delCmd{}, "")
	subcommands.Register(&cronCmd{}, "")
	subcommands.Register(&sendCmd{}, "")
	subcommands.Register(&versionCmd{}, "")

	flag.Parse()
	ctx := context.Background()
	os.Exit(int(subcommands.Execute(ctx)))

}
