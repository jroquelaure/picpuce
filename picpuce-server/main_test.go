package main

import (
	"fmt"
	"log"
	"math/rand"
	"testing"

	context "context"

	"github.com/golang/mock/gomock"
	//"github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
	pr "github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
	mock "github.com/jroquelaure/picpuce/picpuce-server/mock"
	client "github.com/micro/go-micro/client"
)

func TestCreateRandomArtifact(t *testing.T) {

	tables := []BinDescription{

		{
			minSize: 2*1024*1024*1024 - 1,
			maxSize: 2*1024*1024*1024 - 1,
		},
		{
			minSize: 1 * 1024 * 1024,
			maxSize: 100 * 1024 * 1024,
		},
		{
			minSize: 100 * 1024 * 1024,
			maxSize: 1024 * 1024 * 1024,
		},
		{
			minSize: 10 * 1024 * 1024,
			maxSize: 110 * 1024 * 1024,
		},
	}

	for _, table := range tables {
		chunk, _ := CreateRandomArtifact(&table)
		if int64(cap(chunk.Content)) < int64(table.minSize)*8 {
			t.Errorf("Too small, got: %d, want: %d.", len(chunk.Content), int64(table.minSize)*8)
		}
		if int64(cap(chunk.Content))-1 >= int64(table.maxSize)*8 {
			t.Errorf("Too large, got: %d, want: %d.", cap(chunk.Content), int64(table.maxSize)*8)
		}

		t.Log(fmt.Printf("Total size as expected : %d MB ", ByteToMB(cap(chunk.Content))))

	}
}

func TestSendChunk(t *testing.T) {
	//Init
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	nbBytes := 110 * 1024
	chunkMaxSize := 64 * 1024
	nbChunk := nbBytes / chunkMaxSize
	randBytes := make([]byte, nbBytes)
	rand.Read(randBytes)
	oneChunk := make([]byte, chunkMaxSize)
	rand.Read(oneChunk)
	halfChunk := make([]byte, chunkMaxSize/2)
	rand.Read(halfChunk)

	c := mock.NewMockRunnerService(ctrl)

	var tables = []struct {
		client        *mock.MockRunnerService
		chunk         pr.Chunk
		id            string
		scenarioID    string
		expectedChunk int
	}{
		{
			client:        c,
			chunk:         pr.Chunk{Id: "1", Content: randBytes, FileId: "1", ScenarioId: "1"},
			id:            "1",
			scenarioID:    "1",
			expectedChunk: nbChunk,
		},
		{
			client:        c,
			chunk:         pr.Chunk{Id: "1", Content: oneChunk, FileId: "1", ScenarioId: "1"},
			id:            "2",
			scenarioID:    "1",
			expectedChunk: 1,
		},
		{
			client:        c,
			chunk:         pr.Chunk{Id: "1", Content: halfChunk, FileId: "1", ScenarioId: "1"},
			id:            "2",
			scenarioID:    "1",
			expectedChunk: 1,
		},
	}
	for _, table := range tables {

		cs := mock.NewMockRunner_AddChunkService(ctrl)
		c.EXPECT().AddChunk(
			gomock.Any(), // expect any value for first parameter
		).Return(cs, nil)

		cs.EXPECT().Send(
			gomock.Any(),
		).Return(nil).Times(table.expectedChunk)

		cs.EXPECT().Close().Return(nil)

		cs.EXPECT().RecvMsg(
			gomock.Any(),
		).Return(nil).AnyTimes()
		//Test
		err := SendChunk(table.client, &table.chunk, table.id, table.scenarioID)
		//ASSERT
		t.Logf("Test case run")
		if err != nil {
			t.Errorf("Client is nil but function ran?")
		}

	}
}

func ByteToMB(bt int) int {
	result := bt / 1024 / 1024 / 8
	return result
}

type SendChunkInput struct {
	client        *mock.MockRunnerService
	chunk         pr.Chunk
	id            string
	scenarioID    string
	expectedChunk int
}

type clientMock struct {
	c    client.Client
	name string
}

func (c *clientMock) AddChunck(ctx context.Context, opts ...client.CallOption) (pr.Runner_AddChunkService, error) {
	return AddChunkServiceMock{}, nil
}

func (c *clientMock) RunScenario(ctx context.Context, in *pr.Scenario, opts ...client.CallOption) (*pr.Response, error) {
	out := new(pr.Response)
	return out, nil
}

type AddChunkServiceMock struct {
}

func (c AddChunkServiceMock) Close() error {
	log.Printf("Stream closed")
	return nil
}

func (c AddChunkServiceMock) SendMsg(interface{}) error {
	return nil
}
func (c AddChunkServiceMock) RecvMsg(interface{}) error {
	return nil
}
func (c AddChunkServiceMock) Send(*pr.Chunk) error {
	return nil
}
