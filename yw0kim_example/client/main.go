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

	if fi, _ := file.Stat(); fi.IsDir() {
		fmt.Println(path + " is dir. Please post file.")
	}

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

func getPostRequest(filePath, url string) (*http.Request, error) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalln(err)
	}

	fi, _ := file.Stat()
	if fi.IsDir() {
		fmt.Println(filePath + " is dir. Please post file.")
	}

	fName := filepath.Base(filePath)
	fSize := int(fi.Size())
	bar := pb.New(fSize).Prefix(fName)
	bar.Units = pb.U_BYTES
	bar.ShowSpeed = true
	bar.ShowCounters = true
	bar.RefreshRate = time.Millisecond * 10

	bar.Start()
	proxyReader := bar.NewProxyReader(file)

	req, err := http.NewRequest(http.MethodPost, url, proxyReader)
	req.Header.Set("File-Name", fName)
	req.Header.Set("File-Size", strconv.Itoa(fSize))

	bufMime := make([]byte, 512)
	n, err := file.Read(bufMime)
	if err != nil && err != io.EOF {
		return req, err
	}
	file.Seek(0, 0)
	mime := http.DetectContentType(bufMime[:n])
	req.Header.Set("Content-Type", mime)

	return req, err
}

func makeRequest(command, pathOrMsg, reqURL, proto string) *http.Request {
	var err error
	var req *http.Request

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

	switch command {
	case "E", "e": // echo
		reqBody := *bytes.NewBufferString(pathOrMsg)
		req, err = http.NewRequest(http.MethodPost, url.String(), &reqBody)
	case "W", "w": // POST
		req, err = getPostRequest(pathOrMsg, url.String())
	case "R", "r": // GET
		req, err = http.NewRequest(http.MethodGet, url.String(), nil)
	}
	if err != nil {
		fmt.Printf("http.NewRequest error : %s\n", err.Error())
		log.Fatalln(err)
	}

	switch proto {
	case "h1":
		req.Proto = "HTTP/1.1"
		req.ProtoMajor = 1
		req.ProtoMinor = 1
	case "h2":
		req.Proto = "HTTP/2.0"
		req.ProtoMajor = 2
		req.ProtoMinor = 0
	case "h3":
		req.Proto = "HTTP/3.0"
		req.ProtoMajor = 3
		req.ProtoMinor = 0
	}

	return req
}

func handleGetDirResponse(rsp *http.Response) {
	var fInfos jsonstruct.FileInfos

	body, err := ioutil.ReadAll(rsp.Body)
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

func handleGetFileResponse(rsp *http.Response) {
	fname := rsp.Header.Get("File-Name")
	file, err := os.Create("./downloads/" + fname)
	if err != nil {
		panic(err)
	}

	contentLength, _ := strconv.Atoi(rsp.Header.Get("Content-Length"))
	bar := pb.New(contentLength).Prefix(fname)
	bar.Units = pb.U_BYTES
	bar.ShowCounters = true
	bar.ShowSpeed = true
	bar.RefreshRate = time.Millisecond * 10
	bar.Start()
	proxyReader := bar.NewProxyReader(rsp.Body)
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

func handlePostResponse(rsp *http.Response) {
	/*
		writtenFileSize, _ := strconv.Atoi(rsp.Header.Get("Written-File-Size"))
		bar := pb.New(writtenFileSize).Prefix(rsp.Header.Get("File-Name"))
		bar.Units = pb.U_BYTES
		bar.ShowCounters = true
		bar.RefreshRate = time.Millisecond * 10
		bar.Start()
		proxyReader := bar.NewProxyReader(rsp.Body)
	*/
	//rspBody, err := ioutil.ReadAll(rsp.Body)
	//if err != nil {
	//		panic(err)
	//	}
	// fmt.Printf("Write file response : %s", string(rspBody))
}

func handleResponse(Method string, rsp *http.Response) {
	switch Method {
	case "GET":
		if rsp.Header.Get("IsDir") == "true" {
			handleGetDirResponse(rsp)
		} else {
			handleGetFileResponse(rsp)
		}
	case "POST":
		handlePostResponse(rsp)
	}
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	// quiet := flag.Bool("q", false, "don't print the data")
	echo := flag.String("e", "not set", "echo msg for test")
	proto := flag.String("p", "h1", "Request Protocol h1(http/1), h2(http/2), h3(http/3)\n")
	command := flag.String("c", "L", "W/R/E\n"+
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

	if *echo != "not set" && (*command == "E" || *command == "e") {
		*file = *echo
	}

	if len(urls) <= 0 {
		fmt.Println("Please enter the url.")
		return
	} else if strings.IndexAny(urls[0], "/") == -1 {
		fmt.Println("Please enter the url path.")
		return
	}

	var req *http.Request
	req = makeRequest(*command, *file, urls[0], *proto)
	start := time.Now()
	hclient := getHTTPClient(*proto)
	// fmt.Printf("hclient : %#v\n", hclient)
	// fmt.Printf("reqBody : %#v\n %s\n", req.Body, req.Body.Read())
	rsp, err := hclient.Do(req)
	if err != nil {
		fmt.Printf("hclient.Do error : %s\n", err.Error())
		panic(err)
	}

	var reqDump []byte
	if *command == "w" || *command == "W" {
		reqDump, err = httputil.DumpRequest(req, false)
	} else {
		reqDump, err = httputil.DumpRequest(req, true)
	}
	if err != nil {
		panic(err)
	}
	logger.Infof("\nRequest: ")
	logger.Infof(string(reqDump))
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
	logger.Infof("Got response for %s: \n%s", urls[0], string(rspDump))
	// logger.Infof("body Response Body: %d bytes", body.Len())
	// logger.Infof("testBody Response Body: %d bytes", len(testBody))
	if *command != "E" && *command != "e" { // L/R/W/D
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

	var contentLength int64
	if *command == "W" || *command == "w" {
		contentLength, _ = strconv.ParseInt(req.Header.Get("File-Size"), 10, 64)
	} else {
		contentLength, _ = strconv.ParseInt(rsp.Header.Get("Content-Length"), 10, 64)
	}
	logger.Infof("Elapsed Time : %s\n", elpasedTime)
	logger.Infof("throughput   : %.2fMiB/s\n", (float64(contentLength)/(1024*1024))/(elpasedTime.Seconds()))
}
