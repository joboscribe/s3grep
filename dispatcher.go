package main

func startDispatcher(nworkers int) {
	workerQueue = make(chan chan string, nworkers)

	for i := 0; i < nworkers; i++ {
		worker := newWorker(i+1, workerQueue)
		worker.Start()
		workers = append(workers, worker)
	}

	go func() {
		for {
			select {
			case work := <-workQueue:
				go func() {
					worker := <-workerQueue
					worker <- work
				}()
			case <-stopChan:
				for i := range workers {
					workers[i].Stop()
				}
			}
		}
	}()
}
