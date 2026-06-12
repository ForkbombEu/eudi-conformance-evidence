package main

import (
	"fmt"
	"os"

	"github.com/forkbombeu/eudi-conformance-evidence/cmd/extractcontext"
	"github.com/forkbombeu/eudi-conformance-evidence/cmd/webui"
)

var version = "dev"

const asciiArt = `
███████╗██╗   ██╗██████╗ ██╗      ██████╗ ██████╗ ███╗   ██╗███████╗
██╔════╝██║   ██║██╔══██╗██║     ██╔════╝██╔═══██╗████╗  ██║██╔════╝
█████╗  ██║   ██║██║  ██║██║     ██║     ██║   ██║██╔██╗ ██║█████╗
██╔══╝  ██║   ██║██║  ██║██║     ██║     ██║   ██║██║╚██╗██║██╔══╝
███████╗╚██████╔╝██████╔╝███████╗╚██████╗╚██████╔╝██║ ╚████║██║
╚══════╝ ╚═════╝ ╚═════╝ ╚══════╝ ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝

  ██████╗ ██████╗ ███╗   ██╗███████╗ ██████╗ ██████╗ ███╗   ███╗
  ██╔════╝██╔═══██╗████╗  ██║██╔════╝██╔═══██╗██╔══██╗████╗ ████║
  ██║     ██║   ██║██╔██╗ ██║█████╗  ██║   ██║██████╔╝██╔████╔██║
  ██║     ██║   ██║██║╚██╗██║██╔══╝  ██║   ██║██╔══██╗██║╚██╔╝██║
  ╚██████╗╚██████╔╝██║ ╚████║██║     ╚██████╔╝██║  ██║██║ ╚═╝ ██║
   ╚═════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝      ╚═════╝ ╚═╝  ╚═╝╚═╝     ╚═╝

  ███████╗██╗   ██╗██╗██████╗ ███████╗███╗   ██╗ ██████╗███████╗
  ██╔════╝██║   ██║██║██╔══██╗██╔════╝████╗  ██║██╔════╝██╔════╝
  █████╗  ██║   ██║██║██║  ██║█████╗  ██╔██╗ ██║██║     █████╗
  ██╔══╝  ╚██╗ ██╔╝██║██║  ██║██╔══╝  ██║╚██╗██║██║     ██╔══╝
  ███████╗ ╚████╔╝ ██║██████╔╝███████╗██║ ╚████║╚██████╗███████╗
  ╚══════╝  ╚═══╝  ╚═╝╚═════╝ ╚══════╝╚═╝  ╚═══╝ ╚═════╝╚══════╝
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, asciiArt)
		fmt.Fprintln(os.Stderr, "Usage: eudi-conformance-evidence <command>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  extract-context  Extract protocol context from pipeline input")
		fmt.Fprintln(os.Stderr, "  web              Start the browser extraction interface")
		fmt.Fprintln(os.Stderr, "  version          Print version")
		return fmt.Errorf("no command provided")
	}

	switch args[0] {
	case "extract-context":
		return extractcontext.Run(args[1:])
	case "web":
		return webui.Run(args[1:])
	case "version", "--version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}
