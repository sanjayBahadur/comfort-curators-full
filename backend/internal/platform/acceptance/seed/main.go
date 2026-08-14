package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"comfort-curators-backend/internal/platform/acceptance"
)

func main() {
	if err := run(os.Getenv, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "acceptance-seed: %v\n", err)
		os.Exit(1)
	}
}

func run(getenv func(string) string, stderr io.Writer) error {
	if getenv("CC_ENV") != "acceptance" {
		return errors.New("refusing to run: CC_ENV must be 'acceptance'")
	}

	baseURL := getenv("CC_BASE_URL")
	if baseURL == "" {
		port := getenv("CC_HTTP_PORT")
		if port == "" {
			port = "8080"
		}
		baseURL = "http://127.0.0.1:" + port
	}

	path := getenv("CC_FIXTURE_PATH")
	if path == "" {
		path = acceptance.DefaultFixturePath
	}

	fixture := acceptance.Generate(acceptance.GenerateOptions{BaseURL: baseURL})
	if err := acceptance.WriteFixture(path, fixture); err != nil {
		return err
	}

	fmt.Fprintf(stderr, "wrote acceptance fixture to %s\n", path)
	return nil
}
