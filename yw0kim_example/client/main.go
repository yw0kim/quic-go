package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucas-clemente/quic-go/h2quic"
	"github.com/lucas-clemente/quic-go/internal/utils"
	jsonstruct "github.com/lucas-clemente/quic-go/yw0kim_example"
	"github.com/lucas-clemente/quic-go/yw0kim_example/tlsdata"
	"golang.org/x/net/http2"
)

func getHTTPClient(proto string) *http.Client {
	var hclient http.Client
	tlsConfig := &tls.Config{
		RootCAs: tlsdata.GetRootCA(),
	}

	switch proto {
	case "h1":
		hclient.Transport = &http.Transport{
			TLSClientConfig: tlsConfig,
		}
	case "h2":
		hclient.Transport = &http2.Transport{
			TLSClientConfig: tlsConfig,
		}
	case "h3":
		/* h2quic
		roundTripper = &h2quic.RoundTripper{
			TLSClientConfig: tlsConfig,
		}
		hclient = http.Client{
			Transport: roundTripper,
		}
		*/
		hclient.Transport = &h2quic.RoundTripper{
			TLSClientConfig: tlsConfig,
		}
	}

	return &hclient
}

func loadFile(path string, params map[string]string) bytes.Buffer {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	var retBody bytes.Buffer
	multipartWriter := multipart.NewWriter(&retBody)
	fileWriter, err := multipartWriter.CreateFormFile("filename", filepath.Base(path))
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

func makeRequest(command, pathOrMsg, reqURL, proto string) http.Request {
	var err error
	var req *http.Request
	var reqBody bytes.Buffer

	getURL := func(params map[string]string) url.URL {
		var url url.URL
		url.Scheme = "https"
		url.Host = reqURL[0:strings.IndexAny(reqURL, "/")]
		url.Path = reqURL[strings.IndexAny(reqURL, "/"):]
		switch proto {
		case "h1":
			url.Host += ":6001"
		case "h2":
			url.Host += ":6002"
		case "h3":
			url.Host += ":6003"
		}

		query := url.Query()
		for key, val := range params {
			query.Set(key, val)
		}
		url.RawQuery = query.Encode()

		return url
	}

	// var urlParams map[string]string
	// urlParams = map[string]string{}
	switch command {
	case "E": // echo
		// urlParams["query"] = "echo"
		url := getURL(nil)
		reqBody = *bytes.NewBufferString(pathOrMsg)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	case "L": // GET
		// urlParams["query"] = "list"
		url := getURL(nil)
		req, err = http.NewRequest(http.MethodGet, url.String(), nil)
	case "W": // POST
		// urlParams["query"] = "write"
		url := getURL(nil)
		// var fileParams map[string]string
		reqBody = loadFile(pathOrMsg, nil)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	// File is only can be read
	case "R": // GET
		// urlParams["query"] = "read"
		url := getURL(nil)
		req, err = http.NewRequest(http.MethodGet, "", nil)
		req.URL = &url
	case "D": //DELETE
		// urlParams["query"] = "delete"
		url := getURL(nil)
		req, err = http.NewRequest(http.MethodDelete, url.String(), nil)
	}

	if err != nil {
		log.Fatalln(err)
	}

	return *req
}

func handleGetResponse(body *bytes.Buffer) {
	var fInfos jsonstruct.FileInfos
	json.Unmarshal(body.Bytes(), &fInfos)

	for _, fileInfo := range fInfos {
		fmt.Printf(
			"%s\t%16d\t%s\t%s\n",
			fileInfo.Mode,
			fileInfo.Size,
			fileInfo.ModTime.Format("Jan 2 15:04 2006"),
			fileInfo.Name,
		)
	}
}

func handleResponse(resp *http.Response) {

	switch resp.Request.Method {
	case "GET":
		handleGetResponse(body)
	case "POST":
	case "DELETE":
	}
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	// quiet := flag.Bool("q", false, "don't print the data")
	echo := flag.String("e", "not set", "echo msg for test")
	proto := flag.String("p", "h1", "Request Protocol h1(http/1), h2(http/2), h3(http/3)\n")
	command := flag.String("c", "L", "W/R/L/D/E\n"+
		"W(Write/POST) needs local file path,\n"+
		"R(Read/GET) needs remote file path,\n"+
		"L(List/HEAD) needs remote path(file or dir),\n"+
		"D(Delete/DELETE) needs remote path(file or dir)\n"+
		"E(Echo/POST) echo for test\n")
	file := flag.String("f", "", "local or remote path(file or dir)\n")
	flag.Parse()
	urls := flag.Args()

	logger := utils.DefaultLogger

	if *verbose {
		logger.SetLogLevel(utils.LogLevelDebug)
	} else {
		logger.SetLogLevel(utils.LogLevelInfo)
	}
	logger.SetLogTimeFormat("")

	if *echo != "not set" && *command == "E" {
		*file = *echo
	}

	var req http.Request
	req = makeRequest(*command, *file, urls[0], *proto)
	reqStr := func(req http.Request) string {
		return fmt.Sprintf("Request : %s %s %s.", req.Method, req.URL.String(), *file)
	}

	logger.Infof(reqStr(req))

	hclient := getHTTPClient(*proto)
	rsp, err := hclient.Do(&req)
	if err != nil {
		panic(err)
	}

	/*
		httputil.DumpRequestOut(&req, true)
		rsp, err := http.DefaultClient.Do(&req)
		httputil.DumpResponse(rsp, true)
	*/
	//testBody, _ := ioutil.ReadAll(rsp.Body)

	logger.Infof("Got response for %s: %#v", urls[0], rsp)
	// logger.Infof("body Response Body: %d bytes", body.Len())
	// logger.Infof("testBody Response Body: %d bytes", len(testBody))
	if *command != "E" { // L/R/W/D
		handleResponse(rsp)
	} else { // Echo
		body := &bytes.Buffer{}
		_, err = io.Copy(body, rsp.Body)
		if err != nil {
			panic(err)
		}
		logger.Infof("Echo Mesg : %s", body.Bytes())
	}

	/*
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
	*/
}
