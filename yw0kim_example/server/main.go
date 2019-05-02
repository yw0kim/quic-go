package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"

	"github.com/gorilla/mux"
	"github.com/lucas-clemente/quic-go/http3"
	jsonstruct "github.com/lucas-clemente/quic-go/yw0kim_example"
	"github.com/lucas-clemente/quic-go/yw0kim_example/tlsdata"

	_ "net/http/pprof"

	"github.com/lucas-clemente/quic-go/internal/utils"
)

type binds []string

func (b binds) String() string {
	return strings.Join(b, ",")
}

func (b *binds) Set(v string) error {
	*b = strings.Split(v, ",")
	return nil
}

// Size is needed by the /demo/upload handler to determine the size of the uploaded file
type Size interface {
	Size() int64
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	//dumpR, _ := httputil.DumpRequest(r, true)
	//fmt.Printf("echo req: %s\n", string(dumpR))
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		fmt.Printf("error reading body while handling /echo: %s\n", err.Error())
	}
	resp := append(body, "(From Server)"...)
	w.Write(resp)
}

func getFileInfos(path string) jsonstruct.FileInfos {
	var fInfos jsonstruct.FileInfos

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		fInfos = append(fInfos, jsonstruct.FileInfo{
			Name:    info.Name(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
		return nil
	})
	if err != nil {
		fmt.Printf("error walking dir while handling /: %s\n", err.Error())
		return nil
	}

	return fInfos
}

func getHandler(w http.ResponseWriter, r *http.Request) {
	//dumpR, _ := httputil.DumpRequest(r, true)
	//fmt.Printf("echo req: %s\n", string(dumpR))
	path := r.URL.Path[1:]
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	fi, _ := file.Stat()
	if fi.IsDir() { // GET Directory : return files' info
		fInfos := getFileInfos(path)
		jsonBytes, err := json.Marshal(fInfos)
		if err != nil {
			panic(err)
		}
		w.Header().Set("IsDir", "true")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(jsonBytes)))
		fmt.Printf("%s\n %#v\n", strconv.Itoa(len(jsonBytes)), strconv.Itoa(len(jsonBytes)))
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonBytes)
		fmt.Printf("header.get: %s", w.Header().Get("Cotent-Legth"))
	} else {
		fBytes := make([]byte, fi.Size())
		nBytes, err := file.Read(fBytes)
		if err != nil {
			panic(err)
		}
		mime := http.DetectContentType(fBytes)
		w.Header().Set("IsDir", "false")
		fname := filepath.Base(path)
		w.Header().Set("File-Name", fname)
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", "attachment; filename="+fname+"")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Transfer-Encoding", "binary")
		w.Header().Set("Content-Length", strconv.Itoa(nBytes))
		w.Header().Set("Content-Control", "private, no-transform, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(fBytes)
	}

	if err != nil {
		panic(err)
	}
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	fname := r.Header.Get("File-Name")
	file, err := os.Create("./data/" + fname)
	if err != nil {
		panic(err)
	}

	contentLength, _ := strconv.Atoi(r.Header.Get("Content-Length"))
	bytesFile, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}

	w.Header().Set("File-Name", fname)
	nBytes, err := file.Write(bytesFile)
	if err != nil {
		panic(err)
	} else if contentLength != nBytes {
		fmt.Println("contentLength != nBytes")
		// fail
		w.WriteHeader(http.StatusForbidden)
		rspStr := "Writing " + fname + " is failed."
		_, err = w.Write([]byte(rspStr))
		return
	}
	file.Sync()

	w.Header().Set("Written-File-Size", strconv.Itoa(nBytes))
	w.WriteHeader(http.StatusOK)
	rspStr := fname + " is written in server."
	_, err = w.Write([]byte(rspStr))
}

func requestLogger(targetMux http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		targetMux(w, r)
		// log request by who(IP address)
		requesterIP := r.RemoteAddr

		log.Printf(
			"%s\t%s\t\t\t%s\t%v",
			r.Method,
			r.RequestURI,
			requesterIP,
			time.Since(start),
		)
	})
}

func main() {
	verbose := flag.Bool("v", false, "verbose")
	bs := flag.String("bind", "quic.yw.com", "bind address")
	// rootDir := flag.String("dir", "./data", "data root directory")
	// tcp := flag.Bool("tcp", false, "also listen on TCP")
	proto := flag.String("p", "h1", "h1(http/1.1), h2(http/2), h3(http/3), a(All protocol work)")
	flag.Parse()

	logger := utils.DefaultLogger

	if *verbose {
		logger.SetLogLevel(utils.LogLevelDebug)
	} else {
		logger.SetLogLevel(utils.LogLevelInfo)
	}
	logger.SetLogTimeFormat("")

	// http.Handle("/", http.FileServer(http.Dir(*rootDir)))

	/*
		if len(bs) == 0 {
			bs = binds{url + ":6003"}
		}
	*/
	r := mux.NewRouter()
	r.PathPrefix("/echo").HandlerFunc(requestLogger(echoHandler)).Methods("POST")
	r.PathPrefix("/data").HandlerFunc(requestLogger(getHandler)).Methods("GET")
	r.PathPrefix("/data").HandlerFunc(requestLogger(postHandler)).Methods("POST")
	http.Handle("/", r)
	var err error
	certFile, keyFile := tlsdata.GetCertificatePaths()
	switch *proto {
	case "h1":
		bCap := *bs + ":6001"
		err = http.ListenAndServeTLS(bCap, certFile, keyFile, nil)
	case "h2":
		bCap := *bs + ":6002"
		server := http.Server{
			Addr: bCap,
		}
		http2.ConfigureServer(&server, nil)
		err = server.ListenAndServeTLS(certFile, keyFile)
	case "h3":
		bCap := *bs + ":6003"
		// tcp
		err = http3.ListenAndServe(bCap, certFile, keyFile, nil)
		/* pure http/3
		server := http3.Server{
			Server: &http.Server{Addr: bCap},
		}
		err = server.ListenAndServeTLS(certFile, keyFile)
		*/
	}

	if err != nil {
		fmt.Println(err)
	}

	/*
		var wg sync.WaitGroup
		wg.Add(len(bs))
		for _, b := range bs {
			bCap := b
			go func() {
				var err error
				if *tcp {
					certFile, keyFile := tlsdata.GetCertificatePaths()
					// ListenAndServe listens on the given network address for both, TLS and QUIC
					// connetions in parallel. It returns if one of the two returns an error.
					// http.DefaultServeMux is used when handler is nil.
					// The correct Alt-Svc headers for QUIC are set.
					err = h2quic.ListenAndServe(bCap, certFile, keyFile, nil)
				} else {
					server := h2quic.Server{
						Server: &http.Server{Addr: bCap},
					}
					// ListenAndServeTLS listens on the UDP address s.Addr and calls s.Handler to handle HTTP/2 requests on incoming connections.
					// ListenAndServeTLS -> serveImpl -> quicListenAddr
					err = server.ListenAndServeTLS(tlsdata.GetCertificatePaths())
				}
				if err != nil {
					fmt.Println(err)
				}
				wg.Done()
			}()
		}
		wg.Wait()
	*/
}
