package main

import (
	"context"
	"fmt"
	"os"

	"comfort-curators-backend/internal/platform/app"
)

func main() {
	if err := app.RunWorker(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "worker: %v\n", err)
		os.Exit(1)
	}
}
