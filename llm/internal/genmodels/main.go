// Command genmodels builds llm's checked-in model catalog from public model
// catalogs. It includes implemented protocols plus selected catalog-only
// protocols planned for future adapters.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"
)

func main() {
	output := flag.String("output", "catalog.generated.json", "generated JSON catalog")
	timeout := flag.Duration("timeout", 45*time.Second, "HTTP timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := &http.Client{Timeout: *timeout}

	models, err := collect(ctx, client)
	if err != nil {
		fatal(err)
	}
	generated, err := render(models)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*output, generated, 0o644); err != nil {
		fatal(fmt.Errorf("write %s: %w", *output, err))
	}
	fmt.Printf("generated %s with %d models\n", *output, len(models))
}

func fatal(err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintln(os.Stderr, "genmodels:", err)
	os.Exit(1)
}
