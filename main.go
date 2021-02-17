package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/jamiealquiza/tachymeter"
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

	envWindow := os.Getenv("NUM_REQUESTS_WINDOW")
	if envWindow == "" {
		log.Fatal("NUM_REQUESTS_WINDOW env var is missing.")
	}

	window, err := strconv.Atoi(envWindow)
	if err != nil {
		log.Fatal("NUM_REQUESTS_WINDOW env var cannot be converted to integer.")
	}

	tach := tachymeter.New(&tachymeter.Config{Size: window})
	go controller(tach)

	backendUrl, err := url.Parse("http://localhost:" + bp)
	if err != nil {
		log.Fatalf("Error parsing backend url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(backendUrl)
	http.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		startTime := time.Now()
		proxy.ServeHTTP(rw, req)
		duration := time.Now().Sub(startTime)
		tach.AddTime(duration)
	})

	err = http.ListenAndServe(fmt.Sprintf(":%v", fp), nil)
	if err != nil {
		log.Fatalf("Error serving reverse proxy: %v", err)
	}
}

func controller(tach *tachymeter.Tachymeter) {
	for range time.Tick(time.Second * 1) {
		metrics := tach.Calc()
		fmt.Printf("[%s] p50: %s, p95: %s\n", time.Now().Format(time.StampMilli), metrics.Time.P50, metrics.Time.P95)
	}
}
