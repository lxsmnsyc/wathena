// Package clientdata loads the client data tables in this folder
// (strategy section 10.5). The web client reads the same YAML files.
package clientdata

import (
	_ "embed"
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/assets"
)

//go:embed jobs.yml
var jobsYAML []byte

// Look is one body sprite.
type Look struct {
	Look      string `yaml:"look"`
	GRFSprite string `yaml:"grf_sprite"`
	Race      string `yaml:"race"`
}

// Job maps a job ID to its looks.
type Job struct {
	ID       int    `yaml:"id"`
	Constant string `yaml:"constant"`
	Look     string `yaml:"look"`
	Mount    string `yaml:"mount"`
}

// Jobs is jobs.yml.
type Jobs struct {
	Looks []Look `yaml:"looks"`
	Jobs  []Job  `yaml:"jobs"`
}

// LoadJobs parses and checks the embedded jobs.yml.
func LoadJobs() (*Jobs, error) {
	return ParseJobs(jobsYAML)
}

// ParseJobs parses and checks a jobs table.
func ParseJobs(data []byte) (*Jobs, error) {
	var j Jobs
	if err := yaml.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("jobs.yml: %w", err)
	}
	looks := map[string]bool{}
	for _, l := range j.Looks {
		if looks[l.Look] {
			return nil, fmt.Errorf("jobs.yml: look %q is listed twice", l.Look)
		}
		looks[l.Look] = true
		if err := assets.ValidName("sprite/body/" + l.Look + "/m"); err != nil || l.GRFSprite == "" {
			return nil, fmt.Errorf("jobs.yml: look %q is invalid", l.Look)
		}
		if l.Race != "human" && l.Race != "doram" {
			return nil, fmt.Errorf("jobs.yml: look %q has race %q", l.Look, l.Race)
		}
	}
	ids := map[int]bool{}
	for _, job := range j.Jobs {
		if ids[job.ID] {
			return nil, fmt.Errorf("jobs.yml: job %d is listed twice", job.ID)
		}
		ids[job.ID] = true
		if !looks[job.Look] || (job.Mount != "" && !looks[job.Mount]) {
			return nil, fmt.Errorf("jobs.yml: job %d refers to a missing look", job.ID)
		}
	}
	return &j, nil
}
