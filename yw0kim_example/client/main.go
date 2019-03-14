package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"github.com/lucas-clemente/quic-go/yw0kim_example/tlsdata"
)

func getHTTPClient(proto string) *http.Client {
	var hclient http.Client

	switch proto {
	case "h1":
	case "h2":
	case "h3":
		roundTripper := &h2quic.RoundTripper{
			TLSClientConfig: &tls.Config{
				RootCAs: tlsdata.GetRootCA(),
			},
		}
		hclient = http.Client{
			Transport: roundTripper,
		}
	}

	return &hclient
}

func getRequestStr(command string, filePath string, proto string) string {
	var strMethod string
	switch command {
	case "L":
		strMethod = "HEAD"
	case "W":
		strMethod = "POST"
	case "R":
		strMethod = "GET"
	case "D":
		strMethod = "DELETE"
	}

	return fmt.Sprintf("REQUEST > %s %s %s", proto, strMethod, filePath)
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	quiet := flag.Bool("q", false, "don't print the data")
	proto := flag.String("p", "h1", "Request Protocol h1(http/1), h2(http/2), h3(http/3)")
	command := flag.String("c", "L", `W/R/L/D, 
		"W(Write/POST) needs local file path, 
		"R(Read/GET) needs remote file path, 
		"L(List/HEAD) needs remote path(file or dir), 
		"D(Delete/DELETE) needs remote path(file or dir) `)
	filePath := flag.String("f", "file path", "local or remote path(file or dir)")
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

	logger.Infof(getRequestStr(*command, *filePath, *proto))
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
