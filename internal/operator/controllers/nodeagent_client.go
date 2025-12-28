package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type NodeAgentClient struct {
	BaseURL    string
	AuthHeader string
	AuthValue  string
	HTTP       *http.Client
}

func NewNodeAgentClient(cfg Config) *NodeAgentClient {
	return &NodeAgentClient{
		BaseURL:    cfg.NodeAgentBaseURL,
		AuthHeader: cfg.AuthHeader,
		AuthValue:  cfg.AuthValue,
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *NodeAgentClient) do(ctx context.Context, method, path string, body any, out any, q url.Values) error {
	u := c.BaseURL + path
	if q != nil {
		u += "?" + q.Encode()
	}
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.AuthHeader != "" && c.AuthValue != "" {
		req.Header.Set(c.AuthHeader, c.AuthValue)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("node-agent %s %s failed: %s", method, path, string(b))
	}
	if out != nil {
		_ = json.Unmarshal(b, out)
	}
	return nil
}
