package pull

import (
	"context"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/service/pull/client"
)

// ReadFeedItemsFn is responsible for reading a feed from an HTTP server and
// converting the result to fusion-native data types. The error return value
// is for request errors (e.g. HTTP errors).
type ReadFeedItemsFn func(ctx context.Context, feedURL string, options model.FeedRequestOptions) (client.FeedFetchResult, error)

// UpdateFeedFn is responsible for saving the result of a feed fetch to a data
// store. If the fetch failed, it records that in the data store. If the fetch
// succeeds, it stores the latest build time in the data store and adds any new
// feed items to the datastore.
type UpdateFeedFn func(feed *model.Feed, items []*model.Item, requestError error) error

type SingleFeedPuller struct {
	readFeed   ReadFeedItemsFn
	updateFeed UpdateFeedFn
}

// NewSingleFeedPuller creates a new SingleFeedPuller with the given ReadFeedItemsFn and UpdateFeedFn.
func NewSingleFeedPuller(readFeed ReadFeedItemsFn, updateFeed UpdateFeedFn) SingleFeedPuller {
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

	var feedURL string
	if feed.Link != nil {
		feedURL = *feed.Link
	}
	fetchResult, err := p.readFeed(ctx, feedURL, feed.FeedRequestOptions)
	if err != nil {
		// If there's an error from readFeed, pass it through
		return err
	}

	return p.updateFeed(feed, fetchResult.Items, nil)
}
