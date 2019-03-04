//picpuce/picpuce-server/main.go
package main

import (
	"io"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"

	// Import the generated protobuf code

	pr "github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
	micro "github.com/micro/go-micro"

	utils "github.com/jroquelaure/picpuce/picpuce-server/utils"
	"github.com/micro/go-micro/cmd"
	k8s "github.com/micro/kubernetes/go/micro"
	"golang.org/x/net/context"
)

const (
	defaultFilename = "scenarioDesc.json"
	// artifactoryURL = "http://localhost:8081/artifactory/generic-local/test/"
	// APIKey         = "AKCp5bB3YhxXqcWTHyFksyqvpczd3Mx8uPepC8yfZFvPAsFcZ5AZrCmr2c3zWWT5DxsV6S9qU"

	responseTemplateHeader = `` +
		` Size (byte)   Time (s)` + "\n"

	responseTemplateLine = `` +
		`[%d    |     %d    | ]` + "\n"
)

type service struct {
	scenarioDescription IScenarioDescription
}

type IScenarioDescription interface {
	Create(*pr.Scenario) (*pr.Scenario, error)
	GetScenarios() []*pr.Scenario
}

type BinDescription struct {
	id      string
	minSize int32
	maxSize int32
}

type ResponseServer struct {
	StatusCode        int32
	TotalTransferTime int32
}

type Chunk struct {
	content []byte
}

func (s *service) UploadRandomArtifacts(ctx context.Context, req *utils.ScenarioDescription, resp *ResponseServer) error {

	log.Println("_____ New Scenario Loaded _____")
	srv := k8s.NewService(
		// This name must match the package name given in your protobuf definition
		micro.Name("picpuce.runner"),
	)
	srv.Init()

	client := pr.NewRunnerService("picpuce.runner", srv.Client())

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

func SendChunk(client pr.RunnerService, chunk *pr.Chunk, id string, scenarioId string) {
	chunk.FileId = id

	//ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//defer cancel()
	//client.RunScenario(ctx, &pr.Scenario{})
	stream, err := client.AddChunk(context.Background())
	if err != nil {
		log.Fatalf("%v.AddChunk(_) = _, %v", client, err)
	}
	//chunkMaxSize := 20024
	chunkMaxSize := 1048576
	index := 0

	for i := 0; i < len(chunk.Content); i += chunkMaxSize {
		end := i + chunkMaxSize
		index++
		log.Printf("Send Chunk # %s ", strconv.Itoa(index))
		if end > len(chunk.Content) {
			end = len(chunk.Content)
		}
		time.Sleep(5)
		if err := stream.Send(&pr.Chunk{ScenarioId: scenarioId, FileId: id, Id: string(index), Content: chunk.Content[i:end]}); err != nil {
			log.Fatalf("%v.Send(%v) = %v", stream, chunk.Content[i:end], err)
		}
	}

	log.Printf("Waiting Response after %s chunks sent", strconv.Itoa(index))
	reply := &pr.Response{}
	err = stream.RecvMsg(reply)
	log.Printf(" %s chunks sent", strconv.Itoa(index))
	if err != nil {
		if err != io.EOF {
			log.Fatalf("%v.CloseAndRecv() got error %v, want %v", stream, err, nil)
		}
		stream.Close()
		//log.Printf(responseTemplateLine, reply.TotalContentSize, reply.TotalTransferTime)
	}
	log.Printf(responseTemplateLine, reply.TotalContentSize, reply.TotalTransferTime)
}

func (s *service) RunAll(client pr.RunnerService) (*ResponseServer, error) {

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

func RunScenario(response chan<- *pr.Response, wg *sync.WaitGroup, scenario *pr.Scenario, client pr.RunnerService) {
	ctx, cancel := context.WithTimeout(context.Background(), 3600*time.Second)
	defer cancel()
	r, err := client.RunScenario(ctx, scenario)
	if err != nil {
		log.Println("Failed to run scenario: %s", scenario.Id)
		return
	}

	scenario.Done = true
	//log.Printf(responseTemplateLine, r.TotalContentSize, r.TotalTransferTime)
	response <- r
	wg.Done()
}

//CreateRandomArtifact creates a random binary according to BinDescription object
func CreateRandomArtifact(binDescription *BinDescription) (*pr.Chunk, error) {

	//save the bin
	var irange int
	irange = int(binDescription.maxSize - binDescription.minSize)
	nbBytes := rand.Intn(irange) + int(binDescription.minSize)
	randBytes := make([]byte, nbBytes*1)
	rand.Read(randBytes)

	return &pr.Chunk{Content: randBytes}, nil
}

func main() {
	// Contact the server and print out its response.
	file := defaultFilename
	if len(os.Args) > 1 {
		file = os.Args[1]
	}

	scenarioDesc, err := utils.ParseFile(file)

	if err != nil {
		log.Fatalf("Could not parse file: %v", err)
	}

	cmd.Init()

	srv := service{scenarioDescription: &scenarioDesc}
	resp := ResponseServer{}
	srv.UploadRandomArtifacts(context.Background(), &scenarioDesc, &resp)
	// Create new greeter client
	if err != nil {
		log.Fatalf("Could not greet: %v", err)
	}
	log.Printf("Created: %s", resp.StatusCode)
}
