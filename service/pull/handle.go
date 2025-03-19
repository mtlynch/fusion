package pull

import (
	"context"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/httpx"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/service/pull/client"
)

// ReadFeedItems implements ReadFeedItemsFn for SingleFeedPuller and is exported for use by other packages.
func ReadFeedItems(ctx context.Context, feedURL string, options model.FeedRequestOptions) (FeedFetchResult, error) {
	fetchResult, reqErr := client.NewFeedClient(httpx.FusionRequest).FetchItems(ctx, feedURL, &options)
	if reqErr != nil {
		return FeedFetchResult{}, reqErr
	}

	return FeedFetchResult{
		LastBuild: fetchResult.LastBuild,
		Items:     fetchResult.Items,
	}, nil
}

// updateFeed implements UpdateFeedFn for SingleFeedPuller.
func (p *Puller) updateFeed(feed *model.Feed, items []*model.Item, requestError error) error {
	if requestError != nil {
		return p.feedRepo.Update(feed.ID, &model.Feed{
			Failure: ptr.To(requestError.Error()),
		})
	}

	if len(items) > 0 {
		// Set the correct feed ID for all items.
		for _, item := range items {
			item.FeedID = feed.ID
		}

		if err := p.itemRepo.Insert(items); err != nil {
			return err
		}
	}

	// Update the feed with the new LastBuild time and clear any failure.
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

	err := NewSingleFeedPuller(ReadFeedItems, p.updateFeed).Pull(ctx, f)
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
