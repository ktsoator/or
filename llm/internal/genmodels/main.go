// Command genmodels builds llm's checked-in model catalog from public model
// catalogs. It includes implemented protocols plus selected catalog-only
// protocols planned for future adapters.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func main() {
	output := flag.String("output", "catalog.generated.json", "generated JSON catalog")
	timeout := flag.Duration("timeout", 45*time.Second, "HTTP timeout")
	allowPartial := flag.Bool("allow-partial", false, "allow output when catalog sources are unavailable")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := &http.Client{Timeout: *timeout}

	if err := generateCatalog(ctx, client, *output, collectOptions{
		AllowPartial: *allowPartial,
		Warnings:     os.Stderr,
	}, os.Stdout); err != nil {
		fatal(err)
	}
}

func generateCatalog(
	ctx context.Context,
	client *http.Client,
	output string,
	options collectOptions,
	stdout io.Writer,
) error {
	models, err := collect(ctx, client, options)
	if err != nil {
		return err
	}
	generated, err := render(models)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(output, generated, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", output, err)
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "generated %s with %d models\n", output, len(models))
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err = temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return nil
}

func fatal(err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintln(os.Stderr, "genmodels:", err)
	os.Exit(1)
}
