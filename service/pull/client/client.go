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

type FeedHTTPRequest func(ctx context.Context, link string, options *model.FeedRequestOptions) (*http.Response, error)

// FeedClient retrieves a feed given a feed URL and parses the result.
type FeedClient struct {
	httpRequestFn FeedHTTPRequest
}

func NewFeedClient(httpRequestFn FeedHTTPRequest) FeedClient {
	return FeedClient{
		httpRequestFn: httpRequestFn,
	}
}

type FetchItemsResult struct {
	LastBuild *time.Time
	Items     []*model.Item
}

// fetchFeed is a helper function that handles the common logic of fetching and parsing a feed.
// It returns the parsed feed or an error.
func (c FeedClient) fetchFeed(ctx context.Context, feedURL string, options *model.FeedRequestOptions) (*gofeed.Feed, error) {
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

func (c FeedClient) FetchItems(ctx context.Context, feedURL string, options *model.FeedRequestOptions) (FetchItemsResult, error) {
	feed, err := c.fetchFeed(ctx, feedURL, options)
	if err != nil {
		return FetchItemsResult{}, err
	}

	return FetchItemsResult{
		LastBuild: feed.UpdatedParsed,
		Items:     ParseGoFeedItems(feed.Items),
	}, nil
}

func (c FeedClient) FetchTitle(ctx context.Context, feedURL string, options *model.FeedRequestOptions) (string, error) {
	feed, err := c.fetchFeed(ctx, feedURL, options)
	if err != nil {
		return "", err
	}

	return feed.Title, nil
}
