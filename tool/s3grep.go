package tool

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type s3Result struct {
	key        string
	success    bool
	matchLines []string
	err        error
}

type regxStrs []string

func (r *regxStrs) String() string {
	return fmt.Sprintf("%v", *r)
}

func (r *regxStrs) Set(value string) error {
	*r = append(*r, value)
	return nil
}

type toolInstance struct {
	workers         []worker
	workerQueue     chan chan string
	workQueue       chan string
	stopChan        chan struct{}
	downloader      *s3manager.Downloader
	bucket          string
	done            chan s3Result
	stopped         chan struct{}
	numWorkers      int
	keepDir         string
	caseInsensitive bool
	regexStrings    regxStrs
	regexps         []*regexp.Regexp
	keyReg          *regexp.Regexp
	region          string
}

// CLI takes in the command-line args, runs the tool and prints out the results.
// Returns 0 for successful execution, 1 if a run-time error occurred and 2
// if the arguments could not be parsed.
func CLI(args []string) int {
	var t toolInstance
	err := t.fromArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	errs := t.run()
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "%v\n", e)
		}
		return 1
	}
	return 0
}

func (tool *toolInstance) fromArgs(args []string) error {
	const (
		ciUsage = "Perform case insensitive matching.  By default, s3grep is case sensitive."
		eUsage  = "Additional `regexp` to match against."
	)
	flag.BoolVar(&tool.caseInsensitive, "i", false, ciUsage)
	flag.BoolVar(&tool.caseInsensitive, "ignore-case", false, ciUsage)
	flag.StringVar(&tool.keepDir, "k", "", "Relative file path for storing files with matches.")
	flag.Var(&tool.regexStrings, "e", eUsage)
	flag.Var(&tool.regexStrings, "regexp", eUsage)

	flag.Parse()

	if len(flag.Args()) < 4 {
		return fmt.Errorf("s3grep [-ike] [--ignore-case] [--keep=path] [--regexp=pattern] [pattern] [bucket] [key] [region]")
	}
	tool.regexStrings = append(tool.regexStrings, flag.Arg(0))
	tool.bucket = flag.Arg(1)
	keyStr := flag.Arg(2)
	var err error
	tool.keyReg, err = regexp.Compile(keyStr)
	if err != nil {
		return fmt.Errorf("could not parse key pattern: %v", err)
	}
	for i, rStr := range tool.regexStrings {
		if tool.caseInsensitive {
			rStr = `(?i)` + rStr
		}
		rx, err := regexp.Compile(rStr)
		if err != nil {
			return fmt.Errorf("could not parse pattern %s: %v", tool.regexStrings[i], err)
		}
		tool.regexps = append(tool.regexps, rx)
	}

	if len(flag.Args()) > 3 {
		tool.region = flag.Arg(3)
	} else {
		tool.region, err = s3manager.GetBucketRegion(aws.BackgroundContext(), session.Must(session.NewSession()), tool.bucket, "")
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() != "NotFound" {
			return fmt.Errorf("could not get %s's region", tool.bucket)
		}
	}

	tool.numWorkers = 10
	tool.stopChan = make(chan struct{}, 1)
	tool.workQueue = make(chan string, tool.numWorkers)
	tool.stopped = make(chan struct{}, tool.numWorkers)
	return nil
}

func (tool toolInstance) run() []error {
	errs := make([]error, 0)
	flag.Parse()

	sess := session.New(
		&aws.Config{
			Region: aws.String(tool.region),
		})
	service := s3.New(sess)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(tool.bucket),
	}
	result, err := service.ListObjectsV2(input)
	if err != nil {
		return append(errs, fmt.Errorf("error listing ojects: %v", err))
	}
	objects := result.Contents
	for result.IsTruncated != nil && *result.IsTruncated {
		input = input.SetContinuationToken(*result.ContinuationToken)
		result, err = service.ListObjectsV2(input)
		if err != nil {
			return append(errs, fmt.Errorf("error listing ojects: %v", err))
		}
		objects = append(objects, result.Contents...)
	}

	keys := make([]string, 0, len(objects))
	for i := range objects {
		if objects[i].Key != nil && tool.keyReg.MatchString(*objects[i].Key) {
			keys = append(keys, *objects[i].Key)
		}
	}

	tool.downloader = s3manager.NewDownloader(sess)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	tool.startWorkerPool()

	tool.done = make(chan s3Result, len(keys))

Keys:
	for _, key := range keys {
		select {
		case <-sigs:
			fmt.Fprintln(os.Stdout, "exiting")
			tool.stopChan <- struct{}{}
			break Keys
		default:
			tool.workQueue <- key
		}
	}

	results := make([]s3Result, 0, len(keys))
	stoppedWorkers := 0
Results:
	for len(results) < len(keys) {
		select {
		case r := <-tool.done:
			results = append(results, r)
		case <-tool.stopped:
			stoppedWorkers++
			if stoppedWorkers == tool.numWorkers {
				break Results
			}
		}
	}

	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %v", r.key, r.err))
			continue
		}
		if !r.success {
			continue
		}
		for _, l := range r.matchLines {
			fmt.Fprintf(os.Stdout, "%s: %s\n", r.key, l)
		}
	}
	return nil
}

func (tool *toolInstance) startWorkerPool() {
	tool.workerQueue = make(chan chan string, tool.numWorkers)

	for i := 0; i < tool.numWorkers; i++ {
		worker := newWorker(i+1, tool)
		worker.Start()
		tool.workers = append(tool.workers, worker)
	}

	go func() {
		for {
			select {
			case work := <-tool.workQueue:
				worker := <-tool.workerQueue
				worker <- work
			case <-tool.stopChan:
				for i := range tool.workers {
					tool.workers[i].Stop()
				}
				return
			}
		}
	}()
}
