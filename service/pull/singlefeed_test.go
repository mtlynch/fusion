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
	result        client.FetchItemsResult
	err           error
	lastFeedURL   string
	lastOptions   model.FeedRequestOptions
	shouldTimeout bool
}

func (m *mockFeedReader) Read(ctx context.Context, feedURL string, options model.FeedRequestOptions) (client.FetchItemsResult, error) {
	m.lastFeedURL = feedURL
	m.lastOptions = options

	// Simulate timeout if configured
	if m.shouldTimeout {
		// Instead of waiting for the context to time out, we'll just return a context.DeadlineExceeded error
		return client.FetchItemsResult{}, context.DeadlineExceeded
	}

	return m.result, m.err
}

// mockStoreUpdater is a mock implementation of UpdateFeedInStoreFn
type mockStoreUpdater struct {
	err              error
	lastFeedID       uint
	lastItems        []*model.Item
	lastLastBuild    *time.Time
	lastRequestError error
	called           bool
}

func (m *mockStoreUpdater) Update(feedID uint, items []*model.Item, lastBuild *time.Time, requestError error) error {
	m.called = true
	m.lastFeedID = feedID
	m.lastItems = items
	m.lastLastBuild = lastBuild
	m.lastRequestError = requestError

	return m.err
}

func TestSingleFeedPullerPull(t *testing.T) {
	for _, tt := range []struct {
		description          string
		feed                 model.Feed
		readFeedResult       client.FetchItemsResult
		readErr              error
		readFeedTimeout      bool
		updateFeedInStoreErr error
		expectedErrMsg       string
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
		},
		{
			description: "readFeed returns error",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult: client.FetchItemsResult{},
			readErr:        errors.New("network error"),
			expectedErrMsg: "",
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
			readErr:        errors.New("HTTP 404"),
			expectedErrMsg: "",
		},
		{
			description: "context timeout during readFeed",
			feed: model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult:  client.FetchItemsResult{},
			readFeedTimeout: true,
			expectedErrMsg:  "",
		},
	} {
		t.Run(tt.description, func(t *testing.T) {
			mockRead := &mockFeedReader{
				result:        tt.readFeedResult,
				err:           tt.readErr,
				shouldTimeout: tt.readFeedTimeout,
			}

			mockUpdate := &mockStoreUpdater{err: tt.updateFeedInStoreErr}

			err := pull.NewSingleFeedPuller(mockRead.Read, mockUpdate.Update).Pull(context.Background(), &tt.feed)

			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}

			// Verify ReadFeed was called with correct parameters
			var expectedURL string
			if tt.feed.Link != nil {
				expectedURL = *tt.feed.Link
			}
			assert.Equal(t, expectedURL, mockRead.lastFeedURL)
			assert.Equal(t, tt.feed.FeedRequestOptions, mockRead.lastOptions)

			// Verify UpdateFeed call behavior
			assert.True(t, mockUpdate.called, "UpdateFeed should be called")
			assert.Equal(t, tt.feed.ID, mockUpdate.lastFeedID)
			assert.Equal(t, tt.readFeedResult.Items, mockUpdate.lastItems)
			assert.Equal(t, tt.readFeedResult.LastBuild, mockUpdate.lastLastBuild)

			// Check that the correct error was passed to Update
			var expectedRequestError error
			if tt.readFeedTimeout {
				expectedRequestError = context.DeadlineExceeded
			} else {
				expectedRequestError = tt.readErr
			}
			assert.Equal(t, expectedRequestError, mockUpdate.lastRequestError)
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
