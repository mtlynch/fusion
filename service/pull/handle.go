package pull

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/httpx"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/service/pull/parse"

	"github.com/mmcdole/gofeed"
)

func (p *Puller) do(ctx context.Context, f *model.Feed, force bool) error {
	logger := pullLogger.With("feed_id", f.ID, "feed_name", f.Name)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	updateAction, skipReason := DecideFeedUpdateAction(f, time.Now())
	if skipReason == &SkipReasonSuspended {
		logger.Infof("skip: %s", skipReason)
		return nil
	}
	if !force {
		switch updateAction {
		case ActionSkipUpdate:
			logger.Infof("skip: %s", skipReason)
			return nil
		case ActionFetchUpdate:
			// Proceed to perform the fetch.
		default:
			panic("unexpected FeedUpdateAction")
		}
	}

	fetchResult, err := fetchAndParseFeed(ctx, f)
	if err != nil {
		if dbErr := p.feedRepo.Update(f.ID, &model.Feed{Failure: ptr.To(err.Error())}); dbErr != nil {
			logger.Errorf("failed to record feed fetch update in store: %v", dbErr)
		}
		return err
	}

	isLatestBuild := f.LastBuild != nil && fetchResult.FeedUpdateTime != nil &&
		fetchResult.FeedUpdateTime.Equal(*f.LastBuild)
	if !isLatestBuild {
		logger.Infof("fetched %d items", len(fetchResult.FeedItems))
		if err := p.itemRepo.Insert(fetchResult.FeedItems); err != nil {
			return err
		}
	}

	return p.feedRepo.Update(f.ID, &model.Feed{
		LastBuild: fetchResult.FeedPublishedTime,
		Failure:   ptr.To(""),
	})
}

// FeedUpdateAction represents the action to take when considering checking a
// feed for updates.
type FeedUpdateAction uint8

const (
	ActionFetchUpdate FeedUpdateAction = iota
	ActionSkipUpdate
)

// FeedSkipReason represents a reason for skipping a feed update.
type FeedSkipReason struct {
	reason string
}

func (r FeedSkipReason) String() string {
	return r.reason
}

var (
	SkipReasonSuspended        = FeedSkipReason{"user suspended feed updates"}
	SkipReasonLastUpdateFailed = FeedSkipReason{"last update failed"}
	SkipReasonTooSoon          = FeedSkipReason{"feed was updated too recently"}
)

func DecideFeedUpdateAction(f *model.Feed, now time.Time) (FeedUpdateAction, *FeedSkipReason) {
	if f.IsSuspended() {
		return ActionSkipUpdate, &SkipReasonSuspended
	} else if f.IsFailed() {
		return ActionSkipUpdate, &SkipReasonLastUpdateFailed
	} else if now.Sub(f.UpdatedAt) < interval {
		return ActionSkipUpdate, &SkipReasonTooSoon
	}
	return ActionFetchUpdate, nil
}

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

func FetchFeed(ctx context.Context, f *model.Feed) (*gofeed.Feed, error) {
	return NewFeedClient(httpx.FusionRequest).Fetch(ctx, *f.Link, &f.FeedRequestOptions)
}

type fetchResult struct {
	FeedItems         []*model.Item
	FeedUpdateTime    *time.Time
	FeedPublishedTime *time.Time
}

func fetchAndParseFeed(ctx context.Context, f *model.Feed) (fetchResult, error) {
	fetched, err := FetchFeed(ctx, f)
	if err != nil {
		return fetchResult{}, err
	}
	if fetched == nil {
		return fetchResult{}, nil
	}
	return fetchResult{
		FeedItems:         parse.GoFeedItems(fetched.Items, f.ID),
		FeedUpdateTime:    fetched.UpdatedParsed,
		FeedPublishedTime: fetched.PublishedParsed,
	}, nil
}
