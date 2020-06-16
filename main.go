package main

import (
	"os"

	"github.com/joboscribe/s3grep/tool"
)

func main() {
	os.Exit(tool.CLI(os.Args[1:]))
}
