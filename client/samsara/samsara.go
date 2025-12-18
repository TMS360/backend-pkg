package samsara

import (
	"github.com/TMS360/backend-pkg/config"

	"net/http"
)

type Client struct {
	httpClient *http.Client
	host       string
	apiKey     string
}

func NewClient(cfg config.SamsaraConfig) (*Client, error) {
	return &Client{
		httpClient: &http.Client{},
		host:       cfg.Host,
		apiKey:     cfg.ApiKey,
	}, nil
}
