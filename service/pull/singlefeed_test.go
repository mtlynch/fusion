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
	"github.com/0x2e/fusion/repo"
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

// mockFeedData is a shared data structure for both mocks to access
type mockFeedData struct {
	feed         *model.Feed
	items        []*model.Item
	lastBuild    *time.Time
	requestError error
}

// mockFeedRepo is a mock implementation of the FeedRepo interface
type mockFeedRepo struct {
	err   error
	feeds map[uint]*mockFeedData
}

func newMockFeedRepo(err error) *mockFeedRepo {
	return &mockFeedRepo{
		err:   err,
		feeds: make(map[uint]*mockFeedData),
	}
}

// List implements the FeedRepo interface
func (m *mockFeedRepo) List(filter *repo.FeedListFilter) ([]*model.Feed, error) {
	return nil, nil // Not used in tests
}

// Get implements the FeedRepo interface
func (m *mockFeedRepo) Get(id uint) (*model.Feed, error) {
	if data, ok := m.feeds[id]; ok && data.feed != nil {
		return data.feed, nil
	}
	return nil, errors.New("not found")
}

// Update implements the FeedRepo interface
func (m *mockFeedRepo) Update(id uint, feed *model.Feed) error {
	if m.feeds == nil {
		m.feeds = make(map[uint]*mockFeedData)
	}

	if _, ok := m.feeds[id]; !ok {
		m.feeds[id] = &mockFeedData{
			feed: feed,
		}
	} else {
		m.feeds[id].feed = feed
	}

	// Store lastBuild and failure for test verification
	if feed.LastBuild != nil {
		m.feeds[id].lastBuild = feed.LastBuild
	}

	if feed.Failure != nil {
		var requestErr error
		if *feed.Failure != "" {
			requestErr = errors.New(*feed.Failure)
		}
		m.feeds[id].requestError = requestErr
	}

	return m.err
}

// ReadLastBuild returns the stored last build time for a given feedID
func (m *mockFeedRepo) ReadLastBuild(feedID uint) (*time.Time, error) {
	if data, ok := m.feeds[feedID]; ok {
		return data.lastBuild, nil
	}
	return nil, errors.New("not found")
}

// ReadRequestError returns the stored request error for a given feedID
func (m *mockFeedRepo) ReadRequestError(feedID uint) (error, error) {
	if data, ok := m.feeds[feedID]; ok {
		return data.requestError, nil
	}
	return nil, errors.New("not found")
}

// mockItemRepo is a mock implementation of the ItemRepo interface
type mockItemRepo struct {
	err   error
	feeds map[uint]*mockFeedData
}

func newMockItemRepo(err error, feedData map[uint]*mockFeedData) *mockItemRepo {
	return &mockItemRepo{
		err:   err,
		feeds: feedData,
	}
}

// Insert implements the ItemRepo interface
func (m *mockItemRepo) Insert(items []*model.Item) error {
	if len(items) == 0 {
		return nil
	}

	feedID := items[0].FeedID
	if m.feeds == nil {
		m.feeds = make(map[uint]*mockFeedData)
	}

	if _, ok := m.feeds[feedID]; !ok {
		m.feeds[feedID] = &mockFeedData{
			items: items,
		}
	} else {
		m.feeds[feedID].items = items
	}

	return m.err
}

// ReadItems returns the stored items for a given feedID
func (m *mockItemRepo) ReadItems(feedID uint) ([]*model.Item, error) {
	if data, ok := m.feeds[feedID]; ok {
		return data.items, nil
	}
	return nil, errors.New("not found")
}

func TestSingleFeedPullerPull(t *testing.T) {
	for _, tt := range []struct {
		description         string
		feed                model.Feed
		mockFeedReader      *mockFeedReader
		mockDbErr           error
		expectedErrMsg      string
		expectedStoredItems []*model.Item
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
			mockFeedReader: &mockFeedReader{
				result: client.FetchItemsResult{
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
				err: nil,
			},
			mockDbErr: nil,
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
			mockFeedReader: &mockFeedReader{
				err: errors.New("dummy feed read error"),
			},
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
			mockFeedReader: &mockFeedReader{
				result: client.FetchItemsResult{
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
				err: nil,
			},
			mockDbErr:           errors.New("dummy database error"),
			expectedErrMsg:      "dummy database error",
			expectedStoredItems: nil,
		},
	} {
		t.Run(tt.description, func(t *testing.T) {
			// Create separate mock repositories that share the same data store
			mockFeedRepo := newMockFeedRepo(tt.mockDbErr)
			mockItemRepo := newMockItemRepo(tt.mockDbErr, mockFeedRepo.feeds)

			err := pull.NewSingleFeedPuller(tt.mockFeedReader.Read, mockFeedRepo, mockItemRepo).Pull(context.Background(), &tt.feed)

			if tt.expectedErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, *tt.feed.Link, tt.mockFeedReader.lastFeedURL)
			assert.Equal(t, tt.feed.FeedRequestOptions, tt.mockFeedReader.lastOptions)

			// Only check stored data if updateFeedInStore succeeded.
			if tt.mockDbErr == nil {
				items, err := mockItemRepo.ReadItems(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedStoredItems, items)

				lastBuild, err := mockFeedRepo.ReadLastBuild(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.mockFeedReader.result.LastBuild, lastBuild)

				// Check that the correct error was passed to Update
				requestError, err := mockFeedRepo.ReadRequestError(tt.feed.ID)
				require.NoError(t, err)
				assert.Equal(t, tt.mockFeedReader.err, requestError)
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
