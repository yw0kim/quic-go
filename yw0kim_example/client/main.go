package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/internal/utils"
	"github.com/lucas-clemente/quic-go/yw0kim_example/tlsdata"
)

func getHTTPClient(proto string) *http.Client {
	var hclient http.Client
	var roundTripper http.RoundTripper

	switch proto {
	case "h1":
	case "h2":
	case "h3":
		roundTripper = &h2quic.RoundTripper{
			TLSClientConfig: &tls.Config{
				RootCAs: tlsdata.GetRootCA(),
			},
		}
	}

	hclient = http.Client{
		Transport: roundTripper,
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

func loadFile(path string, params map[string]string) bytes.Buffer {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	var retBody bytes.Buffer
	multipartWriter := multipart.NewWriter(&retBody)
	fileWriter, err := multipartWriter.CreateFormFile("file_field", filepath.Base(path))
	if err != nil {
		log.Fatalln(err)
	}

	_, err = io.Copy(fileWriter, file)
	if err != nil {
		log.Fatalln(err)
	}

	for key, val := range params {
		_ = multipartWriter.WriteField(key, val)
	}
	err = multipartWriter.Close()
	if err != nil {
		log.Fatalln(err)
	}

	return retBody
}

func makeRequest(command, pathOrMsg, reqHost string) http.Request {
	var err error
	var req *http.Request
	var reqBody bytes.Buffer

	getURL := func(params map[string]string) url.URL {
		var url url.URL
		url.Scheme = "https"
		url.Host = reqHost
		query := url.Query()
		for key, val := range params {
			query.Set(key, val)
		}
		url.RawQuery = query.Encode()
		if command != "E" {
			url.Path = pathOrMsg
		}

		return url
	}

	var urlParams map[string]string
	switch command {
	case "E":
		reqBody = *bytes.NewBufferString(pathOrMsg)
		url := getURL(urlParams)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	case "L":
		urlParams["query"] = "list"
		url := getURL(urlParams)
		req, err = http.NewRequest(http.MethodGet, url.String(), nil)
	case "W":
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	// File is only can be read
	case "R":
		urlParams["query"] = "read"
		url := getURL(urlParams)
		req, err = http.NewRequest(http.MethodGet, url.String(), nil)
	case "D":
		urlParams["query"] = "delete"
		url := getURL(urlParams)
		req, err = http.NewRequest(http.MethodDelete, url.String(), nil)
	}

	return *req
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	quiet := flag.Bool("q", false, "don't print the data")
	echo := flag.String("e", "not set", "echo option for test")
	proto := flag.String("p", "h1", "Request Protocol h1(http/1), h2(http/2), h3(http/3)\n")
	command := flag.String("c", "L", "W/R/L/D\n"+
		"W(Write/POST) needs local file path,\n"+
		"R(Read/GET) needs remote file path,\n"+
		"L(List/HEAD) needs remote path(file or dir),\n"+
		"D(Delete/DELETE) needs remote path(file or dir)\n")
	filePath := flag.String("f", "file path", "local or remote path(file or dir)\n")
	flag.Parse()
	urls := flag.Args()

	logger := utils.DefaultLogger

	if *verbose {
		logger.SetLogLevel(utils.LogLevelDebug)
	} else {
		logger.SetLogLevel(utils.LogLevelInfo)
	}
	logger.SetLogTimeFormat("")

	if *echo != "not set" {
		*command = "E"
	} else if *filePath == "file path" {
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
