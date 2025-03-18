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

	"github.com/mmcdole/gofeed"
)

// ReadFeed implements ReadFeedFn for SingleFeedPuller and is exported for use by other packages
func ReadFeed(ctx context.Context, feedURL string, options model.FeedRequestOptions) (FeedFetchResult, error) {
	if feedURL == "" {
		return FeedFetchResult{}, nil
	}

	fetched, err := NewFeedClient(httpx.FusionRequest).Fetch(ctx, feedURL, &options)
	if err != nil {
		// Return the error directly, let the caller handle it
		return FeedFetchResult{
			RequestError: err,
		}, nil
	}

	// We successfully retrieved the feed and parsed the result, but it was empty.
	if fetched == nil {
		return FeedFetchResult{}, nil
	}

	// Convert the fetched items to model.Item objects
	var items []*model.Item
	if len(fetched.Items) > 0 {
		// We need a feed ID for ParseGoFeedItems, but we don't have one here
		// The feed ID will be set correctly when the items are saved
		items = ParseGoFeedItems(fetched.Items, 0)
	}

	// Return the result
	return FeedFetchResult{
		State: &model.Feed{
			LastBuild: fetched.UpdatedParsed,
		},
		Items:        items,
		RequestError: nil,
	}, nil
}

// updateFeed implements UpdateFeedFn for SingleFeedPuller
func (p *Puller) updateFeed(feed *model.Feed, items []*model.Item, requestError error) error {
	if requestError != nil {
		return p.feedRepo.Update(feed.ID, &model.Feed{
			Failure: ptr.To(requestError.Error()),
		})
	}

	// If we have items and they need to be saved
	if len(items) > 0 {
		// Set the correct feed ID for all items
		for _, item := range items {
			item.FeedID = feed.ID
		}

		// Save the items
		if err := p.itemRepo.Insert(items); err != nil {
			return err
		}
	}

	// Update the feed with the new LastBuild time and clear any failure
	return p.feedRepo.Update(feed.ID, &model.Feed{
		LastBuild: feed.LastBuild,
		Failure:   ptr.To(""),
	})
}

func (p *Puller) do(ctx context.Context, f *model.Feed, force bool) error {
	logger := pullLogger.With("feed_id", f.ID, "feed_name", f.Name)

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

	err := NewSingleFeedPuller(ReadFeed, p.updateFeed).Pull(ctx, f)
	if err != nil {
		return err
	}

	logger.Infof("fetched feed successfully")
	return nil
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
