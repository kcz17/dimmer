package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func main() {
	fp := os.Getenv("FE_PORT")
	if fp == "" {
		log.Fatal("FE_PORT env var is missing.")
	}

	bp := os.Getenv("BE_PORT")
	if bp == "" {
		log.Fatal("BE_PORT env var is missing.")
	}

	backendUrl, err := url.Parse("http://localhost:" + bp)
	if err != nil {
		log.Fatalf("Error parsing backend url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendUrl)
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		startTime := time.Now()
		proxy.ServeHTTP(rw, req)
		duration := time.Now().Sub(startTime)
		http.SetCookie(rw, &http.Cookie{
			Name:  "Test",
			Value: fmt.Sprintf("response: %s", duration),
		})
		log.Printf("request: %s", duration)
	})

	err = http.ListenAndServe(fmt.Sprintf(":%v", fp), nil)
	if err != nil {
		log.Fatalf("Error serving reverse proxy: %v", err)
	}
}
