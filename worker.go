package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
)

func newWorker(id int, workerQueue chan chan string) worker {
	w := worker{
		ID:          id,
		Work:        make(chan string),
		WorkerQueue: workerQueue,
		QuitChan:    make(chan bool),
	}
	return w
}

type worker struct {
	ID          int
	Work        chan string
	WorkerQueue chan chan string
	QuitChan    chan bool
}

func (w worker) Start() {
	go func() {
		attempts := 2
		for {
			w.WorkerQueue <- w.Work
			tries := 0
			success := false
			var rawBytes = make([]byte, 0)

			select {
			case key := <-w.Work:
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
					for _, rx := range regexps {
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
				if result.success && len(keepDir) > 0 {
					err := ioutil.WriteFile(filepath.Join(keepDir, key), rawBytes, 0644)
					if err != nil {
						fmt.Fprintln(os.Stderr, "could not write file for ", key)
					}
				}
				result.matchLines = matches
				done <- result
			case <-w.QuitChan:
				fmt.Fprintf(os.Stdout, "worker #%d of %d stopping\n", w.ID, numWorkers)
				return
			}
		}
	}()
}

func (w worker) getBytes(key string) ([]byte, error) {
	var awsBuffer = aws.NewWriteAtBuffer(make([]byte, 0))

	_, err := downloader.Download(awsBuffer,
		&s3.GetObjectInput{
			Bucket: aws.String(bucket),
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
		w.QuitChan <- true
	}()
}
