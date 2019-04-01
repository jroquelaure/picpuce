package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/micro/go-web"

	// Import the generated protobuf code
	pr "github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
	utils "github.com/jroquelaure/picpuce/picpuce-server/utils"
	micro "github.com/micro/go-micro"
	k8s "github.com/micro/kubernetes/go/micro"
	k8sweb "github.com/micro/kubernetes/go/web"
	"golang.org/x/net/context"
)

const (
	chunkMaxSize = 64 * 1024

	defaultFilename = "scenarioDesc.json"
	// artifactoryURL = "http://localhost:8081/artifactory/generic-local/test/"
	// APIKey         = "AKCp5bB3YhxXqcWTHyFksyqvpczd3Mx8uPepC8yfZFvPAsFcZ5AZrCmr2c3zWWT5DxsV6S9qU"

	responseTemplateHeader = `` +
		` Size (byte)   Time (s)` + "\n"

	responseTemplateLine = `` +
		`[%d    |     %d    | ]` + "\n"
)

//Service interface
type Service struct {
	scenarioDescription IScenarioDescription
}

//IScenarioDescription Scenario description interface
type IScenarioDescription interface {
	Create(*pr.Scenario) (*pr.Scenario, error)
	GetScenarios() []*pr.Scenario
}

//BinDescription Binary description
type BinDescription struct {
	id      string
	minSize int32
	maxSize int32
}

//ResponseServer Response server
type ResponseServer struct {
	StatusCode        int32
	TotalTransferTime int32
}

// type Chunk struct {
// 	content []byte
// }

//UploadRandomArtifacts upload randon artifacts
func (s *Service) UploadRandomArtifacts(ctx context.Context, req *utils.ScenarioDescription, resp *ResponseServer) error {

	log.Println("_____ New Scenario Loaded _____")
	srv := k8s.NewService(
		// This name must match the package name given in your protobuf definition
		micro.Name("picpuce-runner"),
	)
	srv.Init()

	client := pr.NewRunnerService("picpuce-runner", srv.Client())

	var binDesc = &BinDescription{minSize: req.MinSize, maxSize: req.MaxSize}

	//foreach scenario, create number of file to according to parameter
	for n := 0; n < int(req.NbThreads); n++ {
		//for all parallel create a scenario and add to scenarioDesc list of scenarios
		scenario := &pr.Scenario{Id: strconv.Itoa(n), ArtifactoryUrl: req.ArtifactoryUrl, ApiKey: req.ApiKey}
		scenario, err := s.scenarioDescription.Create(scenario)
		if err != nil {
			return err
		}

		for m := 0; m < int(req.NbFiles); m++ {
			//for number of files specified create and add files in scenario
			chunk, err := CreateRandomArtifact(binDesc)
			if err != nil {
				return err
			}
			SendChunk(client, chunk, strconv.Itoa(m), scenario.Id)
		}

	}

	r, err := s.RunAll(client)
	//run each scenario from desc using the runner service
	resp = &ResponseServer{StatusCode: r.StatusCode, TotalTransferTime: r.TotalTransferTime}
	return err
}

//SendChunk send chunk
func SendChunk(client pr.RunnerService, chunk *pr.Chunk, id string, scenarioID string) error {

	if client == nil {
		return errors.New("Client is nil")
	}

	chunk.FileId = id
	reply := &pr.Response{}
	//ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//defer cancel()
	//client.RunScenario(ctx, &pr.Scenario{})
	stream, err := client.AddChunk(context.Background())
	defer stream.Close()
	if err != nil {
		log.Fatalf("%v.AddChunk(_) = _, %v", client, err)
	}
	//chunkMaxSize := 20024
	index := 0

	for i := 0; i < len(chunk.Content); i += chunkMaxSize {
		end := i + chunkMaxSize
		index++
		log.Printf("Send Chunk # %s ", strconv.Itoa(index))
		if end > len(chunk.Content) {
			end = len(chunk.Content)
		}
		//time.Sleep(5)
		if err := stream.Send(&pr.Chunk{ScenarioId: scenarioID, FileId: id, Id: string(index), Content: chunk.Content[i:end]}); err != nil {
			log.Fatalf("%v.Send(%v) = %v", stream, chunk.Content[i:end], err)
		}

		err = stream.RecvMsg(reply)
		log.Printf(" %s chunks sent", strconv.Itoa(index))
		if err != nil {
			fmt.Println("recv err", err)
			if err != io.EOF {
				log.Fatalf("%v.CloseAndRecv() got error %v, want %v", stream, err, nil)
			}
			break
		}

	}
	return nil
}

//RunAll run all scenarios
func (s *Service) RunAll(client pr.RunnerService) (*ResponseServer, error) {

	scenarios := s.scenarioDescription.GetScenarios()
	log.Printf(responseTemplateHeader)
	r := make(chan *pr.Response, len(scenarios))
	var wg sync.WaitGroup
	wg.Add(len(scenarios))

	for _, element := range scenarios {
		if element.Done != true {
			go RunScenario(r, &wg, element, client)
			element.Done = true

		}
	}
	wg.Wait()
	close(r)

	// select {
	// case respond := <-r:
	// 	if respond != nil {
	// 		log.Printf(responseTemplateLine, respond.TotalContentSize, respond.TotalTransferTime)
	// 	}
	// 	if respond == nil {
	// 		log.Printf("Failed to read response from scenario run")
	// 	}
	// case <-time.After(3600 * time.Second):
	// 	fmt.Println("A timeout occurred for query")
	// }

	for respond := range r {
		if respond != nil {
			log.Printf(responseTemplateLine, respond.TotalContentSize, respond.TotalTransferTime)
		}
		if respond == nil {
			log.Printf("Failed to read response from scenario run")
		}

	}

	return &ResponseServer{StatusCode: 200, TotalTransferTime: 1}, nil
}

//RunScenario run scenario
func RunScenario(response chan<- *pr.Response, wg *sync.WaitGroup, scenario *pr.Scenario, client pr.RunnerService) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()
	r, err := client.RunScenario(ctx, scenario)
	if err != nil {
		log.Printf("Failed to run scenario: %s", scenario.Id)
		return
	}

	scenario.Done = true
	//log.Printf(responseTemplateLine, r.TotalContentSize, r.TotalTransferTime)
	response <- r
	wg.Done()
}

//CreateRandomArtifact creates a random binary with sith in MB according to BinDescription object
func CreateRandomArtifact(binDescription *BinDescription) (*pr.Chunk, error) {

	//save the bin
	var irange int
	var maxMB int64
	var minMB int64
	maxMB = int64(binDescription.maxSize) * 8
	minMB = int64(binDescription.minSize) * 8
	irange = int(maxMB - minMB)
	nbBytes := int(minMB)
	if irange > 0 {
		nbBytes = nbBytes + rand.Intn(irange)
	}
	randBytes := make([]byte, nbBytes)
	rand.Read(randBytes)

	return &pr.Chunk{Content: randBytes}, nil
}

func main() {

	service := k8sweb.NewService(
		web.Name("go.micro.srv.server"),
	)

	s := new(Service)

	service.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "This is a website server by a Go HTTP server.")
	})
	service.HandleFunc("/LoadScenario", func(rsp http.ResponseWriter, req *http.Request) {
		scenarioDesc := &utils.ScenarioDescription{}

		body, err := ioutil.ReadAll(req.Body)
		if err != nil {
			panic(err)
		}
		log.Println(string(body))
		err = json.Unmarshal(body, scenarioDesc)
		if err != nil {
			panic(err)
		}
		log.Println(scenarioDesc.MaxSize)
		s.scenarioDescription = scenarioDesc

		resp := &ResponseServer{}
		err = s.UploadRandomArtifacts(context.Background(), scenarioDesc, resp)
		if err != nil {
			rsp.Write([]byte("error"))
			log.Fatal(err)
		}
		rsp.Write([]byte("done"))
	})

	service.Init()

	if err := service.Run(); err != nil {
		fmt.Println(err)
	}

}
