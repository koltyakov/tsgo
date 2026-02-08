// Monaco editor integration for tsgo TypeScript playground.
package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	pool.init()

	srv := newServer()
	httpServer := &http.Server{
		Addr:    defaultAddr,
		Handler: srv.routes(),
	}

	fmt.Printf("Monaco editor available at http://%s\n", defaultAddr)
	fmt.Println("Select a sample to load context and code")
	fmt.Println("\nPress Ctrl+C to stop")

	log.Fatal(httpServer.ListenAndServe())
}
