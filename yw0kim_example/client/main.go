package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lucas-clemente/quic-go/http3"
	"github.com/lucas-clemente/quic-go/internal/utils"
	jsonstruct "github.com/lucas-clemente/quic-go/yw0kim_example"
	"github.com/lucas-clemente/quic-go/yw0kim_example/tlsdata"
	"golang.org/x/net/http2"
	pb "gopkg.in/cheggaaa/pb.v1"
)

func getHTTPClient(proto string) http.Client {
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
		hclient.Transport = &http3.RoundTripper{
			TLSClientConfig: tlsConfig,
		}
	}

	return hclient
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

func makeRequest(command, pathOrMsg, reqURL, proto string) *http.Request {
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
			// req.Proto = "HTTP/2"
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
	url := getURL(nil)
	switch command {
	case "E": // echo
		// urlParams["query"] = "echo"
		reqBody = *bytes.NewBufferString(pathOrMsg)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	case "W": // POST
		reqBody = loadFile(pathOrMsg, nil)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	case "R": // GET
		req, err = http.NewRequest(http.MethodGet, url.String(), nil)
	}

	if err != nil {
		fmt.Printf("http.NewRequest error : %s\n", err.Error())
		log.Fatalln(err)
	}

	return req
}

func handleGetDirResponse(resp *http.Response) {
	var fInfos jsonstruct.FileInfos

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("GET Dir body read error : %s\n", err.Error())
		panic(err)
	}
	json.Unmarshal(body, &fInfos)

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

func handleGetFileResponse(resp *http.Response) {
	fname := resp.Header.Get("FileName")
	file, err := os.Create("./downloads/" + fname)
	if err != nil {
		panic(err)
	}

	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	bar := pb.New(contentLength).Prefix(fname)
	bar.Units = pb.U_BYTES
	bar.ShowCounters = true
	bar.RefreshRate = time.Millisecond * 10
	bar.Start()
	proxyReader := bar.NewProxyReader(resp.Body)
	bytesFile, err := ioutil.ReadAll(proxyReader)
	if err != nil {
		panic(err)
	}

	_, err = file.Write(bytesFile)
	if err != nil {
		panic(err)
	}
	file.Sync()
}

func handleResponse(Method string, resp *http.Response) {
	switch Method {
	case "GET":
		if resp.Header.Get("IsDir") == "true" {
			handleGetDirResponse(resp)
		} else {
			handleGetFileResponse(resp)
		}
	case "POST":
	}
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	// quiet := flag.Bool("q", false, "don't print the data")
	echo := flag.String("e", "not set", "echo msg for test")
	proto := flag.String("p", "h1", "Request Protocol h1(http/1), h2(http/2), h3(http/3)\n")
	command := flag.String("c", "L", "W/R/D/E\n"+
		"W(Write/POST),\n"+
		"R(Read/GET),\n"+
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

	var req *http.Request
	req = makeRequest(*command, *file, urls[0], *proto)
	reqStr := func(req http.Request) string {
		reqDump, err := httputil.DumpRequest(&req, true)
		if err != nil {
			panic(err)
		}
		return string(reqDump) + "\n"
	}

	start := time.Now()
	hclient := getHTTPClient(*proto)
	// fmt.Printf("hclient : %#v\n", hclient)
	// fmt.Printf("reqBody : %#v\n %s\n", req.Body, req.Body.Read())
	rsp, err := hclient.Do(req)
	if err != nil {
		fmt.Printf("hclient.Do error : %s\n", err.Error())
		panic(err)
	}
	logger.Infof("Request: ")
	logger.Infof(reqStr(*req))
	logger.Infof("---------------------------------")

	var rspDump []byte
	// fmt.Printf("response : %#v\n ", rsp)
	// fmt.Printf("rsp.Header : %#v", rsp.Header)
	if req.Method == "GET" && rsp.Header.Get("IsDir") == "false" {
		rspDump, err = httputil.DumpResponse(rsp, false)
	} else {
		rspDump, err = httputil.DumpResponse(rsp, true)
	}
	// logger.Infof("after rspdump")
	if err != nil {
		fmt.Printf("response dump error : %s\n", err.Error())
		panic(err)
	}
	logger.Infof("Got response for %s: %s", urls[0], string(rspDump))
	// logger.Infof("body Response Body: %d bytes", body.Len())
	// logger.Infof("testBody Response Body: %d bytes", len(testBody))
	if *command != "E" { // L/R/W/D
		handleResponse(req.Method, rsp)
	} else { // Echo
		body := &bytes.Buffer{}
		_, err = io.Copy(body, rsp.Body)
		if err != nil {
			panic(err)
		}
		logger.Infof("Echo Msg : %s", body.Bytes())
	}
	elpasedTime := time.Since(start)
	contentLength, _ := strconv.Atoi(rsp.Header.Get("Content-Length"))
	logger.Infof("Elapsed Time : %s", elpasedTime)
	logger.Infof("throughput   : %sMiB/s", (contentLength/1000*1000)/elpasedTime)
}
