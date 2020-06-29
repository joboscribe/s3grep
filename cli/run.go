package cli

import (
	"fmt"
	"os"
)

// Run takes in the command-line arguments
// parses them to create a tool.Instance
// runs the tool.Instance then prints out
// any errors that occurred and, if any,
// any matches found
func Run(args []string) int {
	toolInstance, err := FromArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 2
	}
	toolInstance.GetKeys()
	results, errs := toolInstance.Run()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "%v\n", e)
		}
		printResults(results)
		return 1
	}
	printResults(results)
	return 0
}

func printResults(results map[string][]string) {
	for k, matches := range results {
		for _, m := range matches {
			fmt.Fprintf(os.Stdout, "%s: %s\n", k, m)
		}
	}
}
