package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	tvdb "shared/media"
)

type TorrenterClient struct {
	Host string
}

func NewTorrenterClient(host string) *TorrenterClient {
	return &TorrenterClient{Host: host}
}

func (c *TorrenterClient) Download(req tvdb.Media) error {
	url := fmt.Sprintf("http://%s/download", c.Host)
	fmt.Printf("Sending download request to %s with media: %+v\n", url, req)
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(req); err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", buf)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: failed to close response body: %v\n", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}
	return nil
}

func (c *TorrenterClient) MediaExists(hash string) (bool, error) {
	url := fmt.Sprintf("http://%s/media/%s", c.Host, hash)
	resp, err := http.Get(url)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: failed to close response body: %v\n", cerr)
		}
	}()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// MediaExistsBatch checks multiple media items at once. It POSTs an array of
// tvdb.Media to the torrenter's /media/exists endpoint and expects a JSON
// response with `exists` and optional `in_progress` maps keyed by a unique
// identifier (we use the Media.Hash field if present).
func (c *TorrenterClient) MediaExistsBatch(items []tvdb.Media) (map[string]bool, map[string]bool, error) {
	url := fmt.Sprintf("http://%s/media/exists", c.Host)
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(items); err != nil {
		return nil, nil, err
	}
	resp, err := http.Post(url, "application/json", buf)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: failed to close response body: %v\n", cerr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("torrenter returned status %d", resp.StatusCode)
	}

	var body struct {
		Exists     map[string]bool `json:"exists"`
		InProgress map[string]bool `json:"in_progress"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, nil, err
	}

	// Normalize nil maps to empty maps for caller convenience
	if body.Exists == nil {
		body.Exists = make(map[string]bool)
	}
	if body.InProgress == nil {
		body.InProgress = make(map[string]bool)
	}

	return body.Exists, body.InProgress, nil
}
