package utils

import (
	"encoding/json"
	"io/ioutil"
	"log"

	pr "github.com/jroquelaure/picpuce/picpuce-scenario-runner/proto/runner"
)

type ScenarioDescription struct {
	ScenariosParallel []*pr.Scenario
	MinSize           int32
	MaxSize           int32
	NbThreads         int32
	NbFiles           int32
	ArtifactoryUrl    string
	ApiKey            string
}

//Create a new scenario and add it to the list
func (scenarioDescription *ScenarioDescription) Create(scenario *pr.Scenario) (*pr.Scenario, error) {
	generated := append(scenarioDescription.ScenariosParallel, scenario)
	scenarioDescription.ScenariosParallel = generated
	return scenario, nil
}

func (scenarioDescription *ScenarioDescription) GetScenarios() []*pr.Scenario {
	return scenarioDescription.ScenariosParallel
}

func ParseFile(file string) (ScenarioDescription, error) {
	var scenarioDescription ScenarioDescription
	data, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatalf("Could not parse file: %v", err)
	}
	json.Unmarshal(data, &scenarioDescription)
	return scenarioDescription, err
}
