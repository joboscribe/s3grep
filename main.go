package main

import (
	"os"

	"github.com/joboscribe/s3grep/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
