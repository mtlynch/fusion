package client

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/0x2e/fusion/model"
	"github.com/mmcdole/gofeed"
)

type feedHTTPRequest func(ctx context.Context, link string, options *model.FeedRequestOptions) (*http.Response, error)

// FeedClient retrieves a feed given a feed URL and parses the result.
type FeedClient struct {
	httpRequestFn feedHTTPRequest
}

func NewFeedClient(httpRequestFn feedHTTPRequest) FeedClient {
	return FeedClient{
		httpRequestFn: httpRequestFn,
	}
}

func (c FeedClient) Fetch(ctx context.Context, feedURL string, options *model.FeedRequestOptions) (*gofeed.Feed, error) {
	resp, err := c.httpRequestFn(ctx, feedURL, options)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return gofeed.NewParser().ParseString(string(data))
}
