package pull_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/service/pull"
	"github.com/0x2e/fusion/service/pull/client"
)

// mockFeedReader is a mock implementation of ReadFeedItemsFn
type mockFeedReader struct {
	result      client.FetchItemsResult
	err         error
	lastFeedURL string
	lastOptions model.FeedRequestOptions
}

func (m *mockFeedReader) Read(ctx context.Context, feedURL string, options model.FeedRequestOptions) (client.FetchItemsResult, error) {
	m.lastFeedURL = feedURL
	m.lastOptions = options

	return m.result, m.err
}

// mockStoreUpdater is a mock implementation of UpdateFeedInStoreFn
type mockStoreUpdater struct {
	err   error
	feeds map[uint]struct {
		items        []*model.Item
		lastBuild    *time.Time
		requestError error
	}
}

func newMockStoreUpdater(err error) *mockStoreUpdater {
	return &mockStoreUpdater{
		err: err,
		feeds: make(map[uint]struct {
			items        []*model.Item
			lastBuild    *time.Time
			requestError error
		}),
	}
}

func (m *mockStoreUpdater) Update(feedID uint, items []*model.Item, lastBuild *time.Time, requestError error) error {
	if m.feeds == nil {
		m.feeds = make(map[uint]struct {
			items        []*model.Item
			lastBuild    *time.Time
			requestError error
		})
	}

	m.feeds[feedID] = struct {
		items        []*model.Item
		lastBuild    *time.Time
		requestError error
	}{
		items:        items,
		lastBuild:    lastBuild,
		requestError: requestError,
	}

	return m.err
}

// ReadItems returns the stored items for a given feedID
func (m mockStoreUpdater) ReadItems(feedID uint) ([]*model.Item, error) {
	if feed, ok := m.feeds[feedID]; ok {
		return feed.items, nil
	}
	return nil, errors.New("not found")
}

// ReadLastBuild returns the stored last build time for a given feedID
func (m mockStoreUpdater) ReadLastBuild(feedID uint) (*time.Time, error) {
	if feed, ok := m.feeds[feedID]; ok {
		return feed.lastBuild, nil
	}
	return nil, errors.New("not found")
}

// ReadRequestError returns the stored request error for a given feedID
func (m mockStoreUpdater) ReadRequestError(feedID uint) (error, error) {
	if feed, ok := m.feeds[feedID]; ok {
		return feed.requestError, nil
	}
	return nil, errors.New("not found")
}

func TestSingleFeedPullerPull(t *testing.T) {
	for _, tt := range []struct {
		description          string
		feed                 model.Feed
		readFeedResult       client.FetchItemsResult
		readErr              error
		updateFeedInStoreErr error
		expectedErrMsg       string
		expectedStoredItems  []*model.Item
	}{
		{
			description: "successful pull with no errors",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
				FeedRequestOptions: model.FeedRequestOptions{
					ReqProxy: ptr.To("http://proxy.example.com"),
				},
			},
			readFeedResult: client.FetchItemsResult{
				LastBuild: mustParseTime("2025-01-01T12:00:00Z"),
				Items: []*model.Item{
					{
						Title:   ptr.To("Test Item 1"),
						GUID:    ptr.To("guid1"),
						Link:    ptr.To("https://example.com/item1"),
						Content: ptr.To("Content 1"),
						FeedID:  42,
					},
					{
						Title:   ptr.To("Test Item 2"),
						GUID:    ptr.To("guid2"),
						Link:    ptr.To("https://example.com/item2"),
						Content: ptr.To("Content 2"),
						FeedID:  42,
					},
				},
			},
			readErr:              nil,
			updateFeedInStoreErr: nil,
			expectedStoredItems: []*model.Item{
				{
					Title:   ptr.To("Test Item 1"),
					GUID:    ptr.To("guid1"),
					Link:    ptr.To("https://example.com/item1"),
					Content: ptr.To("Content 1"),
					FeedID:  42,
				},
				{
					Title:   ptr.To("Test Item 2"),
					GUID:    ptr.To("guid2"),
					Link:    ptr.To("https://example.com/item2"),
					Content: ptr.To("Content 2"),
					FeedID:  42,
				},
			},
		},
		{
			description: "readFeed returns error",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult:      client.FetchItemsResult{},
			readErr:             errors.New("network error"),
			expectedErrMsg:      "",
			expectedStoredItems: nil,
		},
		{
			description: "readFeed succeeds but updateFeedInStore fails",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult: client.FetchItemsResult{
				LastBuild: mustParseTime("2025-01-01T12:00:00Z"),
				Items: []*model.Item{
					{
						Title:   ptr.To("Test Item 1"),
						GUID:    ptr.To("guid1"),
						Link:    ptr.To("https://example.com/item1"),
						Content: ptr.To("Content 1"),
						FeedID:  42,
					},
				},
			},
			readErr:              nil,
			updateFeedInStoreErr: errors.New("dummy database error"),
			expectedErrMsg:       "dummy database error",
			expectedStoredItems:  nil, // Don't check items when updateFeedInStore fails
		},
		{
			description: "readFeed returns request error",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult: client.FetchItemsResult{
				LastBuild: mustParseTime("2025-01-01T12:00:00Z"),
				Items:     nil,
			},
			readErr:             errors.New("HTTP 404"),
			expectedErrMsg:      "",
			expectedStoredItems: nil,
		},
		{
			description: "context timeout during readFeed",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult:      client.FetchItemsResult{},
			readErr:             context.DeadlineExceeded,
			expectedErrMsg:      "",
			expectedStoredItems: nil,
		},
	} {
		t.Run(tt.description, func(t *testing.T) {
			mockRead := &mockFeedReader{
				result: tt.readFeedResult,
				err:    tt.readErr,
			}

			mockUpdate := newMockStoreUpdater(tt.updateFeedInStoreErr)

			err := pull.NewSingleFeedPuller(mockRead.Read, mockUpdate.Update).Pull(context.Background(), &tt.feed)

			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, *tt.feed.Link, mockRead.lastFeedURL)
			assert.Equal(t, tt.feed.FeedRequestOptions, mockRead.lastOptions)

			// Only check stored data if updateFeedInStore succeeded.
			if tt.updateFeedInStoreErr == nil {
				items, err := mockUpdate.ReadItems(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStoredItems, items)

				lastBuild, err := mockUpdate.ReadLastBuild(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.readFeedResult.LastBuild, lastBuild)

				// Check that the correct error was passed to Update
				requestError, err := mockUpdate.ReadRequestError(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.readErr, requestError)
			}

		})
	}
}

func mustParseTime(iso8601 string) *time.Time {
	t, err := time.Parse(time.RFC3339, iso8601)
	if err != nil {
		panic(err)
	}
	return &t
}
