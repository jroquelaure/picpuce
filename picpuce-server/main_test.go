package main

import (
	"fmt"
	"testing"
)

func TestCreateRandomArtifact(t *testing.T) {

	tables := []BinDescription{

		{
			minSize: 1024 * 1024 * 1024,
			maxSize: 1024 * 1024 * 1024,
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

func ByteToMB(bt int) int {
	result := bt / 1024 / 1024 / 8
	return result
}
