package cli

import (
	"flag"
	"fmt"
	"regexp"

	"github.com/joboscribe/s3grep/tool"
)

// FromArgs takes the command-line parameters
// and returns a new tool.Instance
func FromArgs(args []string) (tool.Instance, error) {
	ti := tool.Instance{}

	config, err := Parse(args)
	if err != nil {
		return ti, err
	}

	return tool.NewInstance(config)
}

// Parse will generate a new tool.Config
// with the flags and parameters entered
func Parse(args []string) (tool.Config, error) {
	var config = tool.Config{}

	flags := flag.NewFlagSet("s3grep", flag.ContinueOnError)
	const (
		ciUsage = "Perform case insensitive matching.  By default, s3grep is case sensitive."
		eUsage  = "Additional `regexp` to match against."
		nUsage  = "Number of cuncurrent S3 operations to perform (default 10)"
	)
	flags.BoolVar(&config.CaseInsensitive, "i", false, ciUsage)
	flags.BoolVar(&config.CaseInsensitive, "ignore-case", false, ciUsage)
	flags.StringVar(&config.KeepDir, "k", "", "Relative file path for storing files with matches.")
	flags.Var(&config.RegexStrings, "e", eUsage)
	flags.Var(&config.RegexStrings, "regexp", eUsage)
	flags.IntVar(&config.NumWorkers, "n", 10, nUsage)
	flags.IntVar(&config.NumWorkers, "num-workers", 10, nUsage)

	flags.Parse(args)

	if len(flags.Args()) < 4 {
		return config, fmt.Errorf("s3grep [-i] [-e pattern] [-k path] [-n num] [--ignore-case] [--keep=path] [--num-workers=num] [--regexp=pattern] [pattern] [bucket] [key] [region]")
	}

	config.RegexStrings = append(config.RegexStrings, flags.Arg(0))
	config.Bucket = flags.Arg(1)
	keyStr := flags.Arg(2)
	var err error
	config.KeyRegexp, err = regexp.Compile(keyStr)
	if err != nil {
		return config, fmt.Errorf("could not parse key pattern: %v", err)
	}
	for i, rStr := range config.RegexStrings {
		if config.CaseInsensitive {
			rStr = `(?i)` + rStr
		}
		rx, err := regexp.Compile(rStr)
		if err != nil {
			return config, fmt.Errorf("could not parse pattern %s: %v", config.RegexStrings[i], err)
		}
		config.Regexps = append(config.Regexps, rx)
	}
	config.Region = flags.Arg(3)
	return config, nil
}
