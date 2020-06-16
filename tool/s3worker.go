package tool

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func newWorker(id int, tool *toolInstance) worker {
	w := worker{
		id:       id,
		work:     make(chan string),
		quitChan: make(chan bool),
		tool:     tool,
	}
	return w
}

type worker struct {
	id       int
	work     chan string
	quitChan chan bool
	tool     *toolInstance
}

func (w worker) Start() {
	go func() {
		attempts := 2
		for {
			w.tool.workerQueue <- w.work
			tries := 0
			success := false
			var rawBytes = make([]byte, 0)

			select {
			case key := <-w.work:
				for tries < attempts && !success {
					var err error
					rawBytes, err = w.getBytes(key)
					if err != nil {
						tries++
					} else {
						success = true

					}
				}
				if tries == attempts {
					fmt.Fprintln(os.Stderr, "aborting ", key)
				}
				scanner := bufio.NewScanner(bytes.NewBuffer(rawBytes))
				result := s3Result{key: key}
				matches := make([]string, 0)
				for scanner.Scan() {
					l := scanner.Text()
					for _, rx := range w.tool.regexps {
						if !rx.MatchString(l) {
							continue
						}
						matches = append(matches, l)
					}
				}
				if err := scanner.Err(); err != nil {
					result.err = err
				}
				result.success = len(matches) > 0
				if result.success && len(w.tool.keepDir) > 0 {
					err := ioutil.WriteFile(filepath.Join(w.tool.keepDir, key), rawBytes, 0644)
					if err != nil {
						fmt.Fprintln(os.Stderr, "could not write file for ", key)
					}
				}
				result.matchLines = matches
				time.Sleep(10 * time.Second)
				w.tool.done <- result
			case <-w.quitChan:
				fmt.Fprintf(os.Stdout, "worker #%d of %d stopping\n", w.id, w.tool.numWorkers)
				w.tool.stopped <- struct{}{}
				return
			}
		}
	}()
}

func (w worker) getBytes(key string) ([]byte, error) {
	var awsBuffer = aws.NewWriteAtBuffer(make([]byte, 0))

	_, err := w.tool.downloader.Download(awsBuffer,
		&s3.GetObjectInput{
			Bucket: aws.String(w.tool.bucket),
			Key:    aws.String(key),
		})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to download %s: %v", key, err)
		return awsBuffer.Bytes(), err
	}

	return awsBuffer.Bytes(), nil
}

func (w worker) Stop() {
	go func() {
		w.quitChan <- true
	}()
}
