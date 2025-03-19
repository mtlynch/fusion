package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

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

type FetchItemsResult struct {
	LastBuild *time.Time
	Items     []*model.Item
}

func (c FeedClient) FetchItems(ctx context.Context, feedURL string, options *model.FeedRequestOptions) (FetchItemsResult, error) {
	resp, err := c.httpRequestFn(ctx, feedURL, options)
	if err != nil {
		return FetchItemsResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return FetchItemsResult{}, fmt.Errorf("got status code %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchItemsResult{}, err
	}

	feed, err := gofeed.NewParser().ParseString(string(data))
	if err != nil {
		return FetchItemsResult{}, err
	}

	return FetchItemsResult{
		LastBuild: feed.UpdatedParsed,
		Items:     ParseGoFeedItems(feed.Items),
	}, nil
}
