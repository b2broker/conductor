package conductor_test

import (
	"log"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/b2broker/conductor"
)

var data = `---
version: "0.0.0-alpha3+300bellsand100whistles"
resources:
  - name: resource
    image: registry.mydocker.com:8080/path/to/project:1.0.2-alpine
    environment:
      - FIRST_VARIABLE=${first}
      - SECOND_VARIABLE=${second:-optional}
      - THIRD_VARIABLE=${third:?mandatory}
      - FOURTH_VARIABLE=4
...`

func TestYamlUnmarshal(t *testing.T) {
	config := conductor.ConductorConfig{}

	err := yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		t.Errorf("%v", err)
	}

	log.Printf("%v", config)

	want := "yaml"
	if got := "yaml"; got != want {
		t.Errorf("wrong")
	}
}
