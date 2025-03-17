package pull

import (
	"context"
	"time"

	"github.com/0x2e/fusion/model"
)

type FeedFetchResult struct {
	State        *model.Feed
	Items        []*model.Item
	RequestError error
}

// ReadFeedFn is responsible for reading a feed from an HTTP server and
// converting the result to fusion-native data types.
type ReadFeedFn func(ctx context.Context, feedURL *string, options model.FeedRequestOptions) (FeedFetchResult, error)

// UpdateFeedFn is responsible for saving the result of a feed fetch to a data
// store. If the fetch failed, it records that in the data store. If the fetch
// succeeds, it stores the latest build time in the data store and adds any new
// feed items to the datastore.
type UpdateFeedFn func(feed *model.Feed, items []*model.Item, RequestError error) error

type SingleFeedPuller struct {
	readFeed   ReadFeedFn
	updateFeed UpdateFeedFn
}

// NewSingleFeedPuller creates a new SingleFeedPuller with the given ReadFeedFn and UpdateFeedFn.
func NewSingleFeedPuller(readFeed ReadFeedFn, updateFeed UpdateFeedFn) SingleFeedPuller {
	return SingleFeedPuller{
		readFeed:   readFeed,
		updateFeed: updateFeed,
	}
}

func (p SingleFeedPuller) Pull(ctx context.Context, feed *model.Feed) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Note: We don't decide whether to fetch/skip here, as that's handled before
	// this function gets called.

	fetchResult, err := p.readFeed(ctx, feed.Link, feed.FeedRequestOptions)
	if err != nil {
		return err
	}

	return p.updateFeed(fetchResult.State, fetchResult.Items, fetchResult.RequestError)
}
