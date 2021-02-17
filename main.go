package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
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

	backendUrl, err := url.Parse("http://localhost:" + bp)
	if err != nil {
		log.Fatalf("Error parsing backend url: %v", err)
	}

	tach := tachymeter.New(&tachymeter.Config{Size: 50})
	go controller(tach)

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
