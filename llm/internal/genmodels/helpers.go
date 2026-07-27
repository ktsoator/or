package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func getJSON(ctx context.Context, client *http.Client, url string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "or-genmodels/1")
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	decoder := json.NewDecoder(response.Body)
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func inputModalities(values []string) []string {
	result := []string{"text"}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), "image") || strings.EqualFold(value, "vision") {
			return append(result, "image")
		}
	}
	return result
}

func contains(values []string, target string) bool {
	return slices.Contains(values, target)
}

func defaultInt(value, fallback int64) int64 {
	if value > 0 {
		return value
	}
	return fallback
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func cloneMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	result := make(map[string]string, len(source))
	for k, v := range source {
		result[k] = v
	}
	return result
}

func perMillion(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed * 1_000_000
}

func anyPerMillion(value any) float64 {
	switch v := value.(type) {
	case json.Number:
		parsed, _ := v.Float64()
		return parsed * 1_000_000
	case string:
		return perMillion(v)
	case float64:
		return v * 1_000_000
	}
	return 0
}
