package httpx

import (
	"context"
	"net/http"
	"net/url"

	"github.com/0x2e/fusion/model"
)

var globalClient = NewClient()

// SendHTTPRequestFn is a function type for sending HTTP requests, matching http.Client's Do method
type SendHTTPRequestFn func(req *http.Request) (*http.Response, error)

// FusionRequestWithRequestSender makes an HTTP request using the provided request sender function
func FusionRequestWithRequestSender(ctx context.Context, sendRequest SendHTTPRequestFn, link string, options *model.FeedRequestOptions) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", link, nil)
	if err != nil {
		return nil, err
	}
	req.Close = true
	req.Header.Add("User-Agent", "fusion/1.0")

	return sendRequest(req)
}

// FusionRequest makes an HTTP request using the global client
func FusionRequest(ctx context.Context, link string, options *model.FeedRequestOptions) (*http.Response, error) {
	client := globalClient

	if options != nil {
		if options.ReqProxy != nil && *options.ReqProxy != "" {
			proxyURL, err := url.Parse(*options.ReqProxy)
			if err != nil {
				return nil, err
			}
			client = NewClient(func(transport *http.Transport) {
				transport.Proxy = http.ProxyURL(proxyURL)
			})
		}
	}

	return FusionRequestWithRequestSender(ctx, client.Do, link, options)
}
