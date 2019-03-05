//picpuce/picpuce-scenario-runner/runner.go

package main

// Import the generated protobuf code
import (
	"bytes"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptrace"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	pr "github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
	micro "github.com/micro/go-micro"
	k8s "github.com/micro/kubernetes/go/micro"
	"golang.org/x/net/http2"
)

const (
	httpsTemplate = `` +
		`  DNS Lookup   TCP Connection   TLS Handshake   Server Processing   Content Transfer` + "\n" +
		`[%s  |     %s  |    %s  |        %s  |       %s  ]` + "\n" +
		`            |                |               |                   |                  |` + "\n" +
		`   namelookup:%s      |               |                   |                  |` + "\n" +
		`                       connect:%s     |                   |                  |` + "\n" +
		`                                   pretransfer:%s         |                  |` + "\n" +
		`                                                     starttransfer:%s        |` + "\n" +
		`                                                                                total:%s` + "\n"

	httpTemplate = `` +
		`   DNS Lookup   TCP Connection   Server Processing   Content Transfer` + "\n" +
		`[ %s  |     %s  |        %s  |       %s  ]` + "\n" +
		`             |                |                   |                  |` + "\n" +
		`    namelookup:%s      |                   |                  |` + "\n" +
		`                        connect:%s         |                  |` + "\n" +
		`                                      starttransfer:%s        |` + "\n" +
		`                                                                 total:%s` + "\n"

	port = ":50052"
)

// type IScenario interface {
// 	RunScenario(*pr.Scenario) (*pr.Response, error)
// }

// type Scenario struct {
// }

type Service struct {
	mu          sync.RWMutex
	savedChunks map[string]map[string][]*pr.Chunk
}

func (s *Service) Reset() error {
	s.savedChunks = make(map[string]map[string][]*pr.Chunk)
	return nil
}

func (s *Service) GetScenarioChunks(scenarioID string) map[string][]*pr.Chunk {
	for id, scenarioChunks := range s.savedChunks {
		if id == scenarioID {
			return scenarioChunks
		}
	}
	return make(map[string][]*pr.Chunk)
}

func (s *Service) AddChunk(ctx context.Context, stream pr.Runner_AddChunkStream) error {
	var totalContentSize int32
	nbChunks := 0

	log.Println("_____ Adding chunk  _____")
	startTime := time.Now()
	endTime := time.Now()
	for {
		chunkReceived, err := stream.Recv()
		if err == io.EOF {
			endTime = time.Now()
			stream.SendMsg(&pr.Response{
				StatusCode:        200,
				TotalTransferTime: int32(endTime.Sub(startTime)),
				TotalContentSize:  totalContentSize,
			})

			return err
		}
		if err != nil {
			return err
		}
		nbChunks++
		log.Printf("Chunk # : %s", nbChunks)
		s.mu.Lock()
		log.Printf("Chunk # : %s after lock", nbChunks)
		if s.savedChunks[chunkReceived.ScenarioId] == nil {
			s.savedChunks[chunkReceived.ScenarioId] = make(map[string][]*pr.Chunk)
		}
		if s.savedChunks[chunkReceived.ScenarioId][chunkReceived.FileId] == nil {
			s.savedChunks[chunkReceived.ScenarioId][chunkReceived.FileId] = make([]*pr.Chunk, 0)
		}
		s.savedChunks[chunkReceived.ScenarioId][chunkReceived.FileId] = append(s.savedChunks[chunkReceived.ScenarioId][chunkReceived.FileId], chunkReceived)
		totalContentSize = totalContentSize + int32(len(s.savedChunks[chunkReceived.ScenarioId][chunkReceived.FileId]))
		s.mu.Unlock()
		log.Printf("_________Chunk # : %s total content is now %s", nbChunks, totalContentSize)
		errst := stream.SendMsg(&pr.Response{
			StatusCode:        200,
			TotalTransferTime: int32(endTime.Sub(startTime)),
			TotalContentSize:  totalContentSize,
		})

		if errst != nil {
			return errst
		}
	}

	return nil
}

func (s *Service) RunScenario(ctx context.Context, req *pr.Scenario, resp *pr.Response) error {

	log.Printf("Run Scenario # : %s", req.Id)
	startTime := time.Now()
	var totalTransferSize int

	for index, artifact := range s.GetScenarioChunks(req.Id) {
		sort.Slice(artifact, func(i, j int) bool {
			return artifact[i].Id < artifact[j].Id
		})

		fileName := fmt.Sprintf("%s%s%s%s", "test", req.Id, index, ".txt")
		var fileToSend []byte
		for _, content := range artifact {
			for _, part := range content.Content {
				fileToSend = append(fileToSend, part)
			}
		}
		totalTransferSize += len(fileToSend)
		log.Printf("Send File # : %s", fileName)
		res, err := SendFile(fmt.Sprintf("%s%s", req.ArtifactoryUrl, fileName), req.ApiKey, fileToSend, "txt")
		if err != nil {
			log.Fatalf("unable to send file to host %v: %v", res, err)
			return err
		}
		log.Printf("Sent: %s", fileName)
	}

	endTime := time.Now()
	resp.TotalContentSize = int32(totalTransferSize)
	resp.StatusCode = 200

	resp.TotalTransferTime = int32(endTime.Sub(startTime).Seconds())

	s.Reset()
	return nil
}

func SendFile(url string, specialHeader string, bytesToSend []byte, filetype string) (string, error) {

	var t0, t1, t2, t3, t4, t5, t6 time.Time

	req, err := http.NewRequest("PUT", url, bytes.NewBuffer(bytesToSend))
	if err != nil {
		return url, err
	}
	req.Header.Set("X-JFrog-Art-Api", specialHeader)

	trace := &httptrace.ClientTrace{
		DNSStart: func(_ httptrace.DNSStartInfo) { t0 = time.Now() },
		DNSDone:  func(_ httptrace.DNSDoneInfo) { t1 = time.Now() },
		ConnectStart: func(_, _ string) {
			if t1.IsZero() {
				// connecting to IP
				t1 = time.Now()
			}
		},
		ConnectDone: func(net, addr string, err error) {
			if err != nil {
				log.Fatalf("unable to connect to host %v: %v", addr, err)
			}
			t2 = time.Now()

			printf("\n%s%s\n", color.GreenString("Connected to "), color.CyanString(addr))
		},
		GotConn:              func(_ httptrace.GotConnInfo) { t3 = time.Now() },
		GotFirstResponseByte: func() { t4 = time.Now() },
		TLSHandshakeStart:    func() { t5 = time.Now() },
		TLSHandshakeDone:     func(_ tls.ConnectionState, _ error) { t6 = time.Now() },
	}
	req = req.WithContext(httptrace.WithClientTrace(context.Background(), trace))

	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	switch req.URL.Scheme {
	case "https":

		// Because we create a custom TLSClientConfig, we have to opt-in to HTTP/2.
		// See https://github.com/golang/go/issues/14275
		err = http2.ConfigureTransport(tr)
		if err != nil {
			log.Fatalf("failed to prepare transport for HTTP/2: %v", err)
		}
	}

	// Submit the request
	client := &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// always refuse to follow redirects, visit does that
			// manually if required.
			return http.ErrUseLastResponse
		},
	}

	res, err := client.Do(req)
	if err != nil {
		log.Fatalf("failed to read response: %v", err)
	}
	defer res.Body.Close()
	bodyMsg := readResponseBody(req, res)

	t7 := time.Now() // after read body
	if t0.IsZero() {
		// we skipped DNS
		t0 = t1
	}

	// print status line and headers
	printf("\n%s%s%s\n", color.GreenString("HTTP"), grayscale(14)("/"), color.CyanString("%d.%d %s", res.ProtoMajor, res.ProtoMinor, res.Status))

	names := make([]string, 0, len(res.Header))
	for k := range res.Header {
		names = append(names, k)
	}
	sort.Sort(headers(names))
	for _, k := range names {
		printf("%s %s\n", grayscale(14)(k+":"), color.CyanString(strings.Join(res.Header[k], ",")))
	}

	if bodyMsg != "" {
		printf("\n%s\n", bodyMsg)
	}

	fmta := func(d time.Duration) string {
		return color.CyanString("%7dms", int(d/time.Millisecond))
	}

	fmtb := func(d time.Duration) string {
		return color.CyanString("%-9s", strconv.Itoa(int(d/time.Millisecond))+"ms")
	}

	colorize := func(s string) string {
		v := strings.Split(s, "\n")
		v[0] = grayscale(16)(v[0])
		return strings.Join(v, "\n")
	}

	fmt.Println()

	switch req.URL.Scheme {
	case "https":
		printf(colorize(httpsTemplate),
			fmta(t1.Sub(t0)), // dns lookup
			fmta(t2.Sub(t1)), // tcp connection
			fmta(t6.Sub(t5)), // tls handshake
			fmta(t4.Sub(t3)), // server processing
			fmta(t7.Sub(t4)), // content transfer
			fmtb(t1.Sub(t0)), // namelookup
			fmtb(t2.Sub(t0)), // connect
			fmtb(t3.Sub(t0)), // pretransfer
			fmtb(t4.Sub(t0)), // starttransfer
			fmtb(t7.Sub(t0)), // total
		)
	case "http":
		printf(colorize(httpTemplate),
			fmta(t1.Sub(t0)), // dns lookup
			fmta(t3.Sub(t1)), // tcp connection
			fmta(t4.Sub(t3)), // server processing
			fmta(t7.Sub(t4)), // content transfer
			fmtb(t1.Sub(t0)), // namelookup
			fmtb(t3.Sub(t0)), // connect
			fmtb(t4.Sub(t0)), // starttransfer
			fmtb(t7.Sub(t0)), // total
		)
	}

	return res.Status, err
}

type headers []string

func (h headers) String() string {
	var o []string
	for _, v := range h {
		o = append(o, "-H "+v)
	}
	return strings.Join(o, " ")
}

func (h *headers) Set(v string) error {
	*h = append(*h, v)
	return nil
}

func (h headers) Len() int      { return len(h) }
func (h headers) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h headers) Less(i, j int) bool {
	a, b := h[i], h[j]

	// server always sorts at the top
	if a == "Server" {
		return true
	}
	if b == "Server" {
		return false
	}

	endtoend := func(n string) bool {
		// https://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html#sec13.5.1
		switch n {
		case "Connection",
			"Keep-Alive",
			"Proxy-Authenticate",
			"Proxy-Authorization",
			"TE",
			"Trailers",
			"Transfer-Encoding",
			"Upgrade":
			return false
		default:
			return true
		}
	}

	x, y := endtoend(a), endtoend(b)
	if x == y {
		// both are of the same class
		return a < b
	}
	return x
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] URL\n\n", os.Args[0])
	fmt.Fprintln(os.Stderr, "OPTIONS:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "ENVIRONMENT:")
	fmt.Fprintln(os.Stderr, "  HTTP_PROXY    proxy for HTTP requests; complete URL or HOST[:PORT]")
	fmt.Fprintln(os.Stderr, "                used for HTTPS requests if HTTPS_PROXY undefined")
	fmt.Fprintln(os.Stderr, "  HTTPS_PROXY   proxy for HTTPS requests; complete URL or HOST[:PORT]")
	fmt.Fprintln(os.Stderr, "  NO_PROXY      comma-separated list of hosts to exclude from proxy")
}

func printf(format string, a ...interface{}) (n int, err error) {
	return fmt.Fprintf(color.Output, format, a...)
}

func grayscale(code color.Attribute) func(string, ...interface{}) string {
	return color.New(code + 232).SprintfFunc()
}

func isRedirect(resp *http.Response) bool {
	return resp.StatusCode > 299 && resp.StatusCode < 400
}

// readResponseBody consumes the body of the response.
// readResponseBody returns an informational message about the
// disposition of the response body's contents.
func readResponseBody(req *http.Request, resp *http.Response) string {
	if isRedirect(resp) || req.Method == http.MethodHead {
		return ""
	}

	w := ioutil.Discard
	msg := color.CyanString("Body discarded")

	if _, err := io.Copy(w, resp.Body); err != nil && w != ioutil.Discard {
		log.Fatalf("failed to read response body: %v", err)
	}

	return msg
}

func main() {
	srv := k8s.NewService(

		// This name must match the package name given in your protobuf definition
		micro.Name("go.micro.srv.runner"),
	)

	// Init will parse the command line flags.
	srv.Init()
	s := new(Service)
	s.savedChunks = make(map[string]map[string][]*pr.Chunk)
	// Register handler
	pr.RegisterRunnerHandler(srv.Server(), s)

	// Run the server
	if err := srv.Run(); err != nil {
		fmt.Println(err)
	}
}
