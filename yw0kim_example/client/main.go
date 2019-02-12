package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"io"
	"net/http"
	"sync"

	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/internal/testdata"
	"github.com/lucas-clemente/quic-go/internal/utils"
)

func getHTTPClient(proto string) *http.Client {
	var hclient http.Client

	switch proto {
	case "http/1":
	case "http/2":
	case "http/3":
		roundTripper := &h2quic.RoundTripper{
			TLSClientConfig: &tls.Config{
				RootCAs: testdata.GetRootCA(),
			},
		}
		hclient = http.Client{
			Transport: roundTripper,
		}
	}

	return &hclient
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	quiet := flag.Bool("q", false, "don't print the data")
	proto := flag.String("p", "http/1", "Request Protocol http/1, http/2, http/3")
	command := flag.String("c", "L", `W/R/L/D, 
		"W(Write/POST) needs local file path, 
		"R(Read/GET) needs remote file path, 
		"L(List/HEAD) needs remote path(file/dir), 
		"D(Delete/DELETE) needs remote path(file/dir) `)
	filePath := flag.String("f", "file path", "local or remote path(file/dir)")
	flag.Parse()
	urls := flag.Args()

	logger := utils.DefaultLogger

	if *verbose {
		logger.SetLogLevel(utils.LogLevelDebug)
	} else {
		logger.SetLogLevel(utils.LogLevelInfo)
	}
	logger.SetLogTimeFormat("")

	if *filePath == "file path" {
		logger.Infof("file path is not set")
		return
	}

	hclient := getHTTPClient(*proto)

	var wg sync.WaitGroup
	wg.Add(len(urls))
	for _, addr := range urls {
		logger.Infof("GET %s", addr)
		go func(addr string) {
			rsp, err := hclient.Get(addr)
			if err != nil {
				panic(err)
			}
			logger.Infof("Got response for %s: %#v", addr, rsp)

			body := &bytes.Buffer{}
			_, err = io.Copy(body, rsp.Body)
			if err != nil {
				panic(err)
			}
			if *quiet {
				logger.Infof("Request Body: %d bytes", body.Len())
			} else {
				logger.Infof("Request Body:")
				logger.Infof("%s", body.Bytes())
			}
			wg.Done()
		}(addr)
	}
	wg.Wait()
}
