package httputil

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

type Client struct {
	client *http.Client
}

func New(timeoutSeconds int) *Client {
	return &Client{
		client: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
	}
}

// Post 发送 HTTP POST 请求
// 返回 (statusCode, responseBody, error)
func (c *Client) Post(url string, body []byte, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}
