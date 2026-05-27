package main

import (
	"fmt"
	"os"

	"github.com/forkbombeu/eudi-conformance-evidence/cmd/extractcontext"
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
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, asciiArt)
		fmt.Fprintln(os.Stderr, "Usage: eudi-conformance-evidence <command>")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  extract-context  Extract protocol context from pipeline input")
		fmt.Fprintln(os.Stderr, "  version          Print version")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "extract-context":
		if err := extractcontext.Run(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version":
		fmt.Println(version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
