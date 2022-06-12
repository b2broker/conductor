package conductor_test

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/b2broker/conductor"
)

var data = `---
version: "0.0.0-alpha3+build001"
resources:
  - name: resource
    image: registry.docker.org:8080/path/to/project:0.0.1-alpine
    environment:
      - FIRST_VARIABLE=${first}
      - SECOND_VARIABLE=${second:-optional}
      - THIRD_VARIABLE=${third:?mandatory}
      - FOURTH_VARIABLE=4
...`

func TestUnmarshalToStructure(t *testing.T) {
	config := conductor.ConductorConfig{}
	err := yaml.Unmarshal([]byte(data), &config)
	if err != nil {
		t.Errorf("%v", err)
	}

	t.Run("version check", func(t *testing.T) {
		want := "0.0.0-alpha3+build001"
		if got := config.Version; string(got) != want {
			t.Errorf("wanted: '%s', got: '%s'", want, got)
		}
	})

	t.Run("resources length", func(t *testing.T) {
		want := 1
		if got := config.Resources; len(got) != want {
			t.Errorf("wanted: %d, got: %d", want, len(got))
		}
	})

	t.Run("resource name", func(t *testing.T) {
		want := "resource"
		if got := config.Resources[0].Name; string(got) != want {
			t.Errorf("wanted: %s, got: %s", want, got)
		}
	})

	t.Run("resource image", func(t *testing.T) {
		want := "registry.docker.org:8080/path/to/project:0.0.1-alpine"
		if got := config.Resources[0].Image; string(got) != want {
			t.Errorf("wanted: %s, got: %s", want, got)
		}
	})

	t.Run("resource environment", func(t *testing.T) {
		want := 4
		if got := config.Resources[0].Environment; len(got) != want {
			t.Errorf("wanted: %d, got: %d", want, len(got))
		}
	})

	t.Run("resource environment 0 line", func(t *testing.T) {
		want := "FIRST_VARIABLE=${first}"
		if got := config.Resources[0].Environment[0]; string(*got) != want {
			t.Errorf("wanted: %s, got: %s", want, string(*got))
		}
	})

}
