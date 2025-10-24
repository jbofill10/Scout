package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	tvdb "shared/media"
)

type TVDBProxyClient struct {
	Host string
}

func NewTVDBProxyClient(host string) *TVDBProxyClient {
	return &TVDBProxyClient{Host: host}
}

func (c *TVDBProxyClient) Search(mediaType, mediaName string) ([]tvdb.Media, error) {
	url := fmt.Sprintf("http://%s/series?mediaType=%s&mediaName=%s", c.Host, mediaType, mediaName)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: failed to close response body: %v\n", cerr)
		}
	}()
	var results []tvdb.Media
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&results); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *TVDBProxyClient) GetExtendedInfo(mediaId string) (tvdb.TVDBSeriesExtendedResponse, error) {
	url := fmt.Sprintf("http://%s/series/%s/extended", c.Host, mediaId)
	resp, err := http.Get(url)
	if err != nil {
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			fmt.Printf("warning: failed to close response body: %v\n", cerr)
		}
	}()
	var info tvdb.TVDBSeriesExtendedResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&info); err != nil {
		return tvdb.TVDBSeriesExtendedResponse{}, err
	}
	return info, nil
}
