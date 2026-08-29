package main

import (
	"fmt"
	"os"

	"safebox/internal/cli"
	"safebox/internal/trace"
	"safebox/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "%s safebox: missing command\n\n", ui.StyleDenied.Render("ERROR"))
		cli.PrintUsageTo(os.Stderr)
		os.Exit(1)
	}

	tr := trace.New(!cli.HasQuiet(os.Args))
	code := cli.Dispatch(os.Args[1:], tr)
	os.Exit(code)
}
