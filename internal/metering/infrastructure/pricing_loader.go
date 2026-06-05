package infrastructure

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// loadPricingConfig reads a pricing configuration from a JSON file.
func loadPricingConfig(path string) (PricingFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PricingFile{}, fmt.Errorf("read pricing file: %w", err)
	}
	var config PricingFile
	if err := json.Unmarshal(data, &config); err != nil {
		return PricingFile{}, fmt.Errorf("parse pricing JSON: %w", err)
	}
	return config, nil
}

// fetchPricingConfig fetches a pricing configuration from a remote HTTP URL.
func fetchPricingConfig(url string) (PricingFile, error) {
	resp, err := http.Get(url)
	if err != nil {
		return PricingFile{}, fmt.Errorf("fetch pricing from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return PricingFile{}, fmt.Errorf("pricing endpoint returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return PricingFile{}, fmt.Errorf("read pricing response: %w", err)
	}

	var config PricingFile
	if err := json.Unmarshal(data, &config); err != nil {
		return PricingFile{}, fmt.Errorf("parse pricing JSON from remote: %w", err)
	}
	return config, nil
}
