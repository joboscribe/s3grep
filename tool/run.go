package tool

import (
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
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

// Config has all the settings necessary for the tool to run
type Config struct {
	Bucket          string
	CaseInsensitive bool
	KeepDir         string
	KeyRegexp       *regexp.Regexp
	NumWorkers      int
	Region          string
	Regexps         []*regexp.Regexp
	RegexStrings    regxStrs
	Keys            []string
}

// Instance performs the retrieval of objects, text searching and,
// if applicable, local retention of objects that contain matches.
type Instance struct {
	config      Config
	done        chan s3Result
	downloader  *s3manager.Downloader
	stopChan    chan struct{}
	workerQueue chan chan string
	workQueue   chan string
	workers     []worker
	stopped     chan struct{}
	sess        *session.Session
}

// NewInstance generates a toolInstance using the passed-in Config
func NewInstance(config Config) (Instance, error) {
	var ti = Instance{
		config: config,
	}

	ti.stopChan = make(chan struct{}, 1)
	ti.workQueue = make(chan string, config.NumWorkers)
	ti.stopped = make(chan struct{}, config.NumWorkers)

	ti.sess = session.New(
		&aws.Config{
			Region: aws.String(ti.config.Region),
		})

	return ti, nil
}

// GetKeys pulls all the object keys down from the S3 bucket,
// compares them to KeyRegexp and adds any matches to
// Config.Keys
func (ti *Instance) GetKeys() error {
	service := s3.New(ti.sess)

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(ti.config.Bucket),
	}
	result, err := service.ListObjectsV2(input)
	if err != nil {
		return fmt.Errorf("error listing ojects: %v", err)
	}
	objects := result.Contents
	for result.IsTruncated != nil && *result.IsTruncated {
		input = input.SetContinuationToken(*result.ContinuationToken)
		result, err = service.ListObjectsV2(input)
		if err != nil {
			return fmt.Errorf("error listing ojects: %v", err)
		}
		objects = append(objects, result.Contents...)
	}

	keys := make([]string, 0, len(objects))
	for i := range objects {
		if objects[i].Key != nil && ti.config.KeyRegexp.MatchString(*objects[i].Key) {
			keys = append(keys, *objects[i].Key)
		}
	}
	ti.config.Keys = keys
	return nil
}

// Run does the final work of pulling objects down and matching them
// against the regex(es)
func (ti Instance) Run() (map[string][]string, []error) {
	errs := make([]error, 0)

	ti.downloader = s3manager.NewDownloader(ti.sess)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ti.startWorkerPool()

	ti.done = make(chan s3Result, len(ti.config.Keys))

Keys:
	for _, key := range ti.config.Keys {
		select {
		case <-sigs:
			fmt.Fprintln(os.Stdout, "exiting")
			ti.stopChan <- struct{}{}
			break Keys
		default:
			ti.workQueue <- key
		}
	}

	results := make([]s3Result, 0, len(ti.config.Keys))
	stoppedWorkers := 0
Results:
	for len(results) < len(ti.config.Keys) {
		select {
		case r := <-ti.done:
			results = append(results, r)
		case <-ti.stopped:
			stoppedWorkers++
			if stoppedWorkers == ti.config.NumWorkers {
				break Results
			}
		}
	}

	s3Results := make(map[string][]string)

	for _, r := range results {
		if r.err != nil {
			errs = append(errs, fmt.Errorf("%s: %v", r.key, r.err))
			continue
		}
		if !r.success {
			continue
		}
		lines := make([]string, 0, len(r.matchLines))
		for _, l := range r.matchLines {
			lines = append(lines, l)
		}
		s3Results[r.key] = lines
	}

	return s3Results, errs
}

func (ti *Instance) startWorkerPool() {
	ti.workerQueue = make(chan chan string, ti.config.NumWorkers)

	for i := 0; i < ti.config.NumWorkers; i++ {
		worker := newWorker(i+1, ti)
		worker.Start()
		ti.workers = append(ti.workers, worker)
	}

	go func() {
		for {
			select {
			case work := <-ti.workQueue:
				worker := <-ti.workerQueue
				worker <- work
			case <-ti.stopChan:
				for i := range ti.workers {
					ti.workers[i].Stop()
				}
				return
			}
		}
	}()
}
