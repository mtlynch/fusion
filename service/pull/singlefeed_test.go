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
)

// mockFeedReader is a mock implementation of ReadFeedFn
type mockFeedReader struct {
	result        pull.FeedFetchResult
	err           error
	lastFeedURL   *string
	lastOptions   model.FeedRequestOptions
	lastContext   context.Context
	shouldTimeout bool
}

func (m *mockFeedReader) Read(ctx context.Context, feedURL *string, options model.FeedRequestOptions) (pull.FeedFetchResult, error) {
	m.lastFeedURL = feedURL
	m.lastOptions = options
	m.lastContext = ctx

	// Simulate timeout if configured
	if m.shouldTimeout {
		// Instead of waiting for the context to time out, we'll just return a context.DeadlineExceeded error
		return pull.FeedFetchResult{}, context.DeadlineExceeded
	}

	return m.result, m.err
}

// mockStoreUpdater is a mock implementation of UpdateFeedFn
type mockStoreUpdater struct {
	err              error
	lastFeed         *model.Feed
	lastItems        []*model.Item
	lastRequestError error
	called           bool
}

func (m *mockStoreUpdater) Update(feed *model.Feed, items []*model.Item, requestError error) error {
	m.called = true
	m.lastFeed = feed
	m.lastItems = items
	m.lastRequestError = requestError

	return m.err
}

func TestSingleFeedPullerPull(t *testing.T) {
	for _, tt := range []struct {
		description      string
		feed             *model.Feed
		readFeedResult   pull.FeedFetchResult
		readFeedErr      error
		readFeedTimeout  bool
		updateFeedErr    error
		expectedErr      string
		shouldCallUpdate bool
	}{
		{
			description: "successful pull with no errors",
			feed: &model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
				FeedRequestOptions: model.FeedRequestOptions{
					ReqProxy: ptr.To("http://proxy.example.com"),
				},
			},
			readFeedResult: pull.FeedFetchResult{
				State: &model.Feed{
					ID:   42,
					Name: ptr.To("Test Feed"),
					Link: ptr.To("https://example.com/feed.xml"),
				},
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
				RequestError: nil,
			},
			readFeedErr:      nil,
			updateFeedErr:    nil,
			shouldCallUpdate: true,
		},
		{
			description: "readFeed returns error",
			feed: &model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult:   pull.FeedFetchResult{},
			readFeedErr:      errors.New("network error"),
			expectedErr:      "network error",
			shouldCallUpdate: false,
		},
		{
			description: "readFeed succeeds but updateFeed fails",
			feed: &model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult: pull.FeedFetchResult{
				State: &model.Feed{
					ID:   42,
					Name: ptr.To("Test Feed"),
					Link: ptr.To("https://example.com/feed.xml"),
				},
				Items: []*model.Item{
					{
						Title:   ptr.To("Test Item 1"),
						GUID:    ptr.To("guid1"),
						Link:    ptr.To("https://example.com/item1"),
						Content: ptr.To("Content 1"),
						FeedID:  42,
					},
				},
				RequestError: nil,
			},
			readFeedErr:      nil,
			updateFeedErr:    errors.New("database error"),
			expectedErr:      "database error",
			shouldCallUpdate: true,
		},
		{
			description: "readFeed returns request error which is passed to updateFeed",
			feed: &model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult: pull.FeedFetchResult{
				State: &model.Feed{
					ID:   42,
					Name: ptr.To("Test Feed"),
					Link: ptr.To("https://example.com/feed.xml"),
				},
				Items:        nil,
				RequestError: errors.New("HTTP 404"),
			},
			readFeedErr:      nil,
			updateFeedErr:    nil,
			shouldCallUpdate: true,
		},
		{
			description: "context timeout during readFeed",
			feed: &model.Feed{
				ID:   42,
				Name: ptr.To("Test Feed"),
				Link: ptr.To("https://example.com/feed.xml"),
			},
			readFeedResult:   pull.FeedFetchResult{},
			readFeedTimeout:  true,
			expectedErr:      "context deadline exceeded",
			shouldCallUpdate: false,
		},
	} {
		t.Run(tt.description, func(t *testing.T) {
			// Set up mocks
			mockRead := &mockFeedReader{
				result:        tt.readFeedResult,
				err:           tt.readFeedErr,
				shouldTimeout: tt.readFeedTimeout,
			}

			mockUpdate := &mockStoreUpdater{
				err: tt.updateFeedErr,
			}

			// Create the puller with mocks
			puller := pull.NewSingleFeedPuller(mockRead.Read, mockUpdate.Update)

			// Execute the Pull method
			err := puller.Pull(context.Background(), tt.feed)

			// Verify error behavior
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
			}

			// Verify ReadFeed was called with correct parameters
			assert.Equal(t, tt.feed.Link, mockRead.lastFeedURL)
			assert.Equal(t, tt.feed.FeedRequestOptions, mockRead.lastOptions)

			// Verify context has timeout set
			deadline, hasDeadline := mockRead.lastContext.Deadline()
			assert.True(t, hasDeadline, "Context should have a deadline")
			assert.True(t, deadline.After(time.Now()), "Deadline should be in the future")

			// Verify UpdateFeed call behavior
			if tt.shouldCallUpdate {
				assert.True(t, mockUpdate.called, "UpdateFeed should be called")
				assert.Equal(t, tt.readFeedResult.State, mockUpdate.lastFeed)
				assert.Equal(t, tt.readFeedResult.Items, mockUpdate.lastItems)
				assert.Equal(t, tt.readFeedResult.RequestError, mockUpdate.lastRequestError)
			} else {
				assert.False(t, mockUpdate.called, "UpdateFeed should not be called")
			}
		})
	}
}
