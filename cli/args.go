package cli

import (
	"flag"
	"fmt"
	"regexp"

	"github.com/joboscribe/s3grep/tool"
)

// Parse will generate a new tool.Instance
// with the flags and parameters entered
func Parse(args []string) (tool.Instance, error) {
	ti := tool.Instance{}

	var config = tool.Config{}

	flags := flag.NewFlagSet("s3grep", flag.ContinueOnError)
	const (
		ciUsage = "Perform case insensitive matching.  By default, s3grep is case sensitive."
		eUsage  = "Additional `regexp` to match against."
	)
	flags.BoolVar(&config.CaseInsensitive, "i", false, ciUsage)
	flags.BoolVar(&config.CaseInsensitive, "ignore-case", false, ciUsage)
	flags.StringVar(&config.KeepDir, "k", "", "Relative file path for storing files with matches.")
	flags.Var(&config.RegexStrings, "e", eUsage)
	flags.Var(&config.RegexStrings, "regexp", eUsage)

	flags.Parse(args)

	if len(flags.Args()) < 4 {
		return ti, fmt.Errorf("s3grep [-ike] [--ignore-case] [--keep=path] [--regexp=pattern] [pattern] [bucket] [key] [region]")
	}

	config.RegexStrings = append(config.RegexStrings, flags.Arg(0))
	config.Bucket = flags.Arg(1)
	keyStr := flags.Arg(2)
	var err error
	config.KeyRegexp, err = regexp.Compile(keyStr)
	if err != nil {
		return ti, fmt.Errorf("could not parse key pattern: %v", err)
	}
	for i, rStr := range config.RegexStrings {
		if config.CaseInsensitive {
			rStr = `(?i)` + rStr
		}
		rx, err := regexp.Compile(rStr)
		if err != nil {
			return ti, fmt.Errorf("could not parse pattern %s: %v", config.RegexStrings[i], err)
		}
		config.Regexps = append(config.Regexps, rx)
	}
	config.Region = flags.Arg(3)
	config.NumWorkers = 10
	return tool.NewInstance(config)
}
