package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	flags := map[string]bool{}
	args := []string{}
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-") {
			if len(arg) > 0 {
				flags[arg[1:]] = true
			}
		} else {
			args = append(args, arg)
		}
	}

	args = args[1:]

	var err error
	var response string

	command := os.Args[1]
	switch command {
	case "init":
		err = initCommand()
	case "cat-file":
		response, err = catFileCommand(args[1])
	case "hash-object":
		response, err = hashFileCommand(args[1], flags)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Error occurred while executing %s: %s", command, err.Error())
		os.Exit(1)
	}

	if response != "" {
		fmt.Print(response)
	}
}
