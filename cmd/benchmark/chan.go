package main

import "sync"

func wait(nodes ...*node) {
	var wg sync.WaitGroup
	wg.Add(len(nodes))

	for _, node := range nodes {
		node := node

		go func() {
			node.wait()
			wg.Done()
		}()
	}

	wg.Wait()
}
