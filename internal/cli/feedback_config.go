package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/charlesnpx/paperclip/internal/domain"
	"github.com/charlesnpx/paperclip/internal/ledger"
)

type feedbackConfig struct {
	Includes []string `json:"includes"`
	Excludes []string `json:"excludes"`
}

func loadFeedbackConfig(path string) (feedbackConfig, error) {
	config := feedbackConfig{Includes: []string{}, Excludes: []string{}}
	explicit := path != ""
	if !explicit {
		repository, err := ledger.NewFeedback()
		if err != nil {
			return config, err
		}
		path = filepath.Join(filepath.Dir(repository.Path()), "config.json")
	}
	body, err := os.ReadFile(path)
	if !explicit && errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, fmt.Errorf("feedback config %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("feedback config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return config, errors.New("feedback config must contain one JSON object")
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) || config.Includes == nil || config.Excludes == nil {
		return config, errors.New("feedback config must be an object; includes and excludes must be arrays, not null")
	}
	for _, names := range [][]string{config.Includes, config.Excludes} {
		for _, name := range names {
			if err := domain.ValidateSkillName(name); err != nil {
				return config, fmt.Errorf("feedback config: %w", err)
			}
		}
	}
	return config, nil
}

func (config feedbackConfig) reason(skill string) string {
	if slices.Contains(config.Excludes, skill) {
		return "excluded by configuration"
	}
	if len(config.Includes) != 0 && !slices.Contains(config.Includes, skill) {
		return "not included by configuration"
	}
	return ""
}
