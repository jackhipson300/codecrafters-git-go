package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	var err error
	var response string

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: mygit <command> [<args>...]\n")
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to read cwd %s\n", err.Error())
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

	args = args[2:]

	command := os.Args[1]
	switch command {
	case "init":
		err = initCommand()
	case "cat-file":
		response, err = catFileCommand(args[0])
	case "hash-object":
		response, err = hashFileCommand(args[0], flags["w"])
	case "ls-tree":
		response, err = lsTreeCommand(args[0])
	case "write-tree":
		response, err = writeTreeCommand(cwd)
	case "commit-tree":
		response, err = commitTreeCommand(
			args[0],
			args[1],
			args[2],
			"Jack Hipson",
			"not@real.email",
			time.Now().UnixMilli(),
			"-0500",
		)
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
