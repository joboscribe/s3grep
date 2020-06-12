package main

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

var workers []worker
var workerQueue chan chan string
var workQueue = make(chan string)
var stopChan = make(chan struct{}, 1)
var downloader *s3manager.Downloader
var bucket string
var done chan s3Result
var numWorkers = 10
var keepDir string
var ci bool
var rStrs regxStrs
var regexps []*regexp.Regexp

func init() {
	const (
		ciUsage = "Perform case insensitive matching.  By default, s3grep is case sensitive."
		eUsage  = "Additional `regexp` to match against."
	)
	flag.BoolVar(&ci, "i", false, ciUsage)
	flag.BoolVar(&ci, "-ignore-case", false, ciUsage)
	flag.StringVar(&keepDir, "k", "./", "Relative file path for storing files with matches.")
	flag.Var(&rStrs, "e", eUsage)
	flag.Var(&rStrs, "-regexp", eUsage)
}

func main() {
	var err error
	var region string
	flag.Parse()
	if len(flag.Args()) < 4 {
		fmt.Fprint(os.Stdout, "s3grp [-ike] [--ignore-case] [--keep=path] [--regexp=pattern] [pattern] [bucket] [key] [region]\n")
		os.Exit(0)
	}
	rStrs = append(rStrs, flag.Arg(0))
	bucket = flag.Arg(1)
	keyStr := flag.Arg(2)
	keyReg, err := regexp.Compile(keyStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not parse key pattern: ", err)
		os.Exit(1)
	}
	for i, rStr := range rStrs {
		if ci {
			rStr = `(?i)` + rStr
		}
		rx, err := regexp.Compile(rStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not parse pattern %s: %v", rStrs[i], err)
			os.Exit(1)
		}
		regexps = append(regexps, rx)
	}
	if len(flag.Args()) > 3 {
		region = flag.Arg(3)
	} else {
		region, err = s3manager.GetBucketRegion(aws.BackgroundContext(), session.Must(session.NewSession()), bucket, "")
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NotFound" {
			fmt.Fprintf(os.Stderr, "bucket %s's region\n", bucket)
		}
	}

	sess := session.New(
		&aws.Config{
			Region: aws.String(region),
		})
	service := s3.New(sess)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	result, err := service.ListObjectsV2(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing ojects: %v\n", err)
		os.Exit(1)
	}
	objects := result.Contents
	for result.IsTruncated != nil && *result.IsTruncated {
		input = input.SetContinuationToken(*result.ContinuationToken)
		result, err = service.ListObjectsV2(input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error listing ojects: %v\n", err)
			os.Exit(1)
		}
		objects = append(objects, result.Contents...)
	}

	keys := make([]string, 0, len(objects))
	for i := range objects {
		if objects[i].Key != nil && keyReg.MatchString(*objects[i].Key) {
			keys = append(keys, *objects[i].Key)
		}
	}

	downloader = s3manager.NewDownloader(sess)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	startDispatcher(numWorkers)

	done = make(chan s3Result, len(keys))

	for _, key := range keys {
		select {
		case <-sigs:
			fmt.Fprintln(os.Stdout, "exiting")
			stopChan <- struct{}{}
		default:
			workQueue <- key
		}
	}

	results := make([]s3Result, 0, len(keys))
	for i := 0; i < len(keys); i++ {
		r := <-done
		results = append(results, r)
		i++
	}

	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", r.key, r.err)
			continue
		}
		if !r.success {
			continue
		}
		for _, l := range r.matchLines {
			fmt.Fprintf(os.Stdout, "%s: %s\n", r.key, l)
		}
	}
}
