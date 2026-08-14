package main

import (
	"context"
	"fmt"
	"os"

	"comfort-curators-backend/internal/platform/app"
)

func main() {
	if err := app.RunAPI(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}
