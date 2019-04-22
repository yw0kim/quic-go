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
	"github.com/lucas-clemente/quic-go/h2quic"
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
	body, err := ioutil.ReadAll(r.Body)
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

func listFileHandler(w http.ResponseWriter, r *http.Request) {
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
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonBytes)
	} else {
		fBytes := make([]byte, fi.Size())
		nBytes, err := file.Read(fBytes)
		if err != nil {
			panic(err)
		}

		mime := http.DetectContentType(fBytes)
		w.Header().Set("IsDir", "false")
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(path)+"")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Transfer-Encoding", "binary")
		w.Header().Set("Content-Length", strconv.Itoa(nBytes))
		w.Header().Set("Content-Control", "private, no-transform, no-store, must-revalidate")
		_, err = w.Write(fBytes)
	}

	if err != nil {
		panic(err)
	}
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
	proto := flag.String("p", "h3", "h1(http/1.1), h2(http/2), h3(http/3)")
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
	r.PathPrefix("/data").HandlerFunc(requestLogger(listFileHandler)).Methods("GET")
	// r.PathPrefix("/data/").HandlerFunc(requestLogger(listFileHandler)).Methods("GET")
	http.Handle("/", r)
	var err error
	certFile, keyFile := tlsdata.GetCertificatePaths()
	switch *proto {
	case "h1":
		bCap := *bs + ":6001"
		err = http.ListenAndServeTLS(bCap, certFile, keyFile, nil)
	case "h2":
		bCap := *bs + ":6002"
		var server http.Server
		server.Addr = bCap
		http2.ConfigureServer(&server, nil)
		err = server.ListenAndServeTLS(certFile, keyFile)
	case "h3":
		bCap := *bs + ":6003"
		server := h2quic.Server{
			Server: &http.Server{Addr: bCap},
		}
		err = server.ListenAndServeTLS(certFile, keyFile)
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

/*
func init() {
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("error reading body while handling /echo: %s\n", err.Error())
		}
		resp := append(body, "(From Server)"...)
		w.Write(resp)
	})

	// accept file uploads and return the MD5 of the uploaded file
	// maximum accepted file size is 2 GB
	http.HandleFunc("/demo/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			err := r.ParseMultipartForm(1 << 31) // (1<<30, 1 GB)
			if err == nil {
				var file multipart.File
				var fileHeader *multipart.FileHeader
				file, fileHeader, err = r.FormFile("uploadfile")
				if err == nil {
					var size int64
					if sizeInterface, ok := file.(Size); ok {
						uploadedFileName := fileHeader.Filename
						var saveFile *os.File
						saveFile, e := os.Create("./" + uploadedFileName) // always truncate
						if e != nil {
							e = errors.New("couldn't create file")
							return
						}
						defer saveFile.Close()
						defer file.Close()

						size = sizeInterface.Size()
						b := make([]byte, size)
						rBytes, _ := file.Read(b)
						md5 := md5.Sum(b)
						fmt.Fprintf(w, "%x", md5)

						wBytes, _ := saveFile.Write(b)

						if rBytes == wBytes {
							fmt.Printf("Write Bytes: %d", wBytes)
						}

						return
					}
					err = errors.New("couldn't get uploaded file size")
				}
			}
			if err != nil {
				utils.DefaultLogger.Infof("Error receiving upload: %#v", err)
			}
		}
		io.WriteString(w, `<html><body><form action="/demo/upload" method="post" enctype="multipart/form-data">
				<input type="file" name="uploadfile"><br>
				<input type="submit">
			</form></body></html>`)
	})
}
*/
