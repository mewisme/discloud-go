package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"
)

// runHealthcheck GETs local /readyz (Postgres + Valkey). Exit 0 if ready.
func runHealthcheck() error {
	port := os.Getenv("DISCLOUD_PORT")
	if port == "" {
		port = "8080"
	}
	url := "http://127.0.0.1:" + port + "/readyz"

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("readyz: %s", res.Status)
	}
	return nil
}
