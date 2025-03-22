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

// mockSingleFeedRepo is a mock implementation of the SingleFeedRepo interface
type mockSingleFeedRepo struct {
	err           error
	items         []*model.Item
	lastBuild     *time.Time
	requestError  error
	insertCalled  bool
	successCalled bool
	failureCalled bool
}

func newMockSingleFeedRepo(err error) *mockSingleFeedRepo {
	return &mockSingleFeedRepo{
		err: err,
	}
}

// InsertItems implements the SingleFeedRepo interface
func (m *mockSingleFeedRepo) InsertItems(items []*model.Item) error {
	m.insertCalled = true
	if m.err != nil {
		return m.err
	}
	m.items = items
	return nil
}

// RecordSuccess implements the SingleFeedRepo interface
func (m *mockSingleFeedRepo) RecordSuccess(lastBuild *time.Time) error {
	m.successCalled = true
	if m.err != nil {
		return m.err
	}
	m.lastBuild = lastBuild
	m.requestError = nil
	return nil
}

// RecordFailure implements the SingleFeedRepo interface
func (m *mockSingleFeedRepo) RecordFailure(readErr error) error {
	m.failureCalled = true
	if m.err != nil {
		return m.err
	}
	m.requestError = readErr
	return nil
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
			// Create a mock SingleFeedRepo
			mockRepo := newMockSingleFeedRepo(tt.mockDbErr)

			err := pull.NewSingleFeedPuller(tt.mockFeedReader.Read, mockRepo).Pull(context.Background(), &tt.feed)

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
				if tt.mockFeedReader.err != nil {
					// Should have called RecordFailure
					assert.True(t, mockRepo.failureCalled)
					assert.Equal(t, tt.mockFeedReader.err, mockRepo.requestError)
				} else {
					// Should have called InsertItems and RecordSuccess
					if len(tt.mockFeedReader.result.Items) > 0 {
						assert.True(t, mockRepo.insertCalled)
						assert.Equal(t, tt.expectedStoredItems, mockRepo.items)
					}
					assert.True(t, mockRepo.successCalled)
					assert.Equal(t, tt.mockFeedReader.result.LastBuild, mockRepo.lastBuild)
				}
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
