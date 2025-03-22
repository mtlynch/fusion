package pull

import (
	"context"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/service/pull/client"
)

// ReadFeedItemsFn is responsible for reading a feed from an HTTP server and
// converting the result to fusion-native data types. The error return value
// is for request errors (e.g. HTTP errors).
type ReadFeedItemsFn func(ctx context.Context, feedURL string, options model.FeedRequestOptions) (client.FetchItemsResult, error)

// UpdateFeedInStoreFn is responsible for saving the result of a feed fetch to a data
// store. If the fetch failed, it records that in the data store. If the fetch
// succeeds, it stores the latest build time in the data store and adds any new
// feed items to the datastore.
type UpdateFeedInStoreFn func(feedID uint, items []*model.Item, lastBuild *time.Time, requestError error) error

type SingleFeedPuller struct {
	readFeed ReadFeedItemsFn
	feedRepo FeedRepo
	itemRepo ItemRepo
}

// NewSingleFeedPuller creates a new SingleFeedPuller with the given ReadFeedItemsFn and repositories.
func NewSingleFeedPuller(readFeed ReadFeedItemsFn, feedRepo FeedRepo, itemRepo ItemRepo) SingleFeedPuller {
	return SingleFeedPuller{
		readFeed: readFeed,
		feedRepo: feedRepo,
		itemRepo: itemRepo,
	}
}

func (p SingleFeedPuller) Pull(ctx context.Context, feed *model.Feed) error {
	logger := pullLogger.With("feed_id", feed.ID, "feed_name", feed.Name)

	// We don't exit on error, as we want to record any error in the data store.
	fetchResult, readErr := p.readFeed(ctx, *feed.Link, feed.FeedRequestOptions)

	if readErr == nil {
		logger.Infof("fetched %d items", len(fetchResult.Items))
	} else {
		logger.Infof("fetch failed: %v", readErr)
	}

	return p.updateFeedInStore(feed.ID, fetchResult.Items, fetchResult.LastBuild, readErr)
}

// updateFeedInStore saves the result of a feed fetch to the data store.
// If the fetch failed, it records that in the data store.
// If the fetch succeeds, it stores the latest build time and adds any new feed items.
func (p SingleFeedPuller) updateFeedInStore(feedID uint, items []*model.Item, lastBuild *time.Time, requestError error) error {
	if requestError != nil {
		return p.feedRepo.Update(feedID, &model.Feed{
			Failure: ptr.To(requestError.Error()),
		})
	}

	if len(items) > 0 {
		// Set the correct feed ID for all items.
		for _, item := range items {
			item.FeedID = feedID
		}

		if err := p.itemRepo.Insert(items); err != nil {
			return err
		}
	}

	return p.feedRepo.Update(feedID, &model.Feed{
		LastBuild: lastBuild,
		Failure:   ptr.To(""),
	})
}
