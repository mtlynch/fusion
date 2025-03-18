package pull_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/service/pull"
)

// mockReadCloser is a mock io.ReadCloser that can return either data or an error.
type mockReadCloser struct {
	result string
	errMsg string
	reader *strings.Reader
}

func TestDecideFeedUpdateAction(t *testing.T) {
	// Helper function to parse ISO8601 string to time.Time.
	parseTime := func(iso8601 string) time.Time {
		t, err := time.Parse(time.RFC3339, iso8601)
		if err != nil {
			panic(err)
		}
		return t
	}

	for _, tt := range []struct {
		description        string
		currentTime        time.Time
		feed               model.Feed
		expectedAction     pull.FeedUpdateAction
		expectedSkipReason *pull.FeedSkipReason
	}{
		{
			description: "suspended feed should skip update",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Suspended: ptr.To(true),
				UpdatedAt: parseTime("2025-01-01T12:00:00Z"),
			},
			expectedAction:     pull.ActionSkipUpdate,
			expectedSkipReason: &pull.SkipReasonSuspended,
		},
		{
			description: "failed feed should skip update",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Failure:   ptr.To("dummy previous error"),
				Suspended: ptr.To(false),
				UpdatedAt: parseTime("2025-01-01T12:00:00Z"),
			},
			expectedAction:     pull.ActionSkipUpdate,
			expectedSkipReason: &pull.SkipReasonLastUpdateFailed,
		},
		{
			description: "recently updated feed should skip update",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Failure:   ptr.To(""),
				Suspended: ptr.To(false),
				UpdatedAt: parseTime("2025-01-01T11:45:00Z"), // 15 minutes before current time
			},
			expectedAction:     pull.ActionSkipUpdate,
			expectedSkipReason: &pull.SkipReasonTooSoon,
		},
		{
			description: "feed should be updated when conditions are met",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Failure:   ptr.To(""),
				Suspended: ptr.To(false),
				UpdatedAt: parseTime("2025-01-01T11:15:00Z"), // 45 minutes before current time
			},
			expectedAction:     pull.ActionFetchUpdate,
			expectedSkipReason: nil,
		},
		{
			description: "feed with nil failure should be updated",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Failure:   nil,
				Suspended: ptr.To(false),
				UpdatedAt: parseTime("2025-01-01T11:15:00Z"), // 45 minutes before current time
			},
			expectedAction:     pull.ActionFetchUpdate,
			expectedSkipReason: nil,
		},
		{
			description: "feed with nil suspended should be updated",
			currentTime: parseTime("2025-01-01T12:00:00Z"),
			feed: model.Feed{
				Failure:   ptr.To(""),
				Suspended: nil,
				UpdatedAt: parseTime("2025-01-01T11:15:00Z"), // 45 minutes before current time
			},
			expectedAction:     pull.ActionFetchUpdate,
			expectedSkipReason: nil,
		},
	} {
		t.Run(tt.description, func(t *testing.T) {
			action, skipReason := pull.DecideFeedUpdateAction(&tt.feed, tt.currentTime)
			assert.Equal(t, tt.expectedAction, action)
			assert.Equal(t, tt.expectedSkipReason, skipReason)
		})
	}
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.errMsg != "" {
		return 0, errors.New(m.errMsg)
	}
	if m.reader == nil {
		m.reader = strings.NewReader(m.result)
	}
	return m.reader.Read(p)
}

func (m *mockReadCloser) Close() error {
	return nil
}

type mockHTTPClient struct {
	resp        *http.Response
	err         error
	lastFeedURL string
	lastOptions *model.FeedRequestOptions
}

func (m *mockHTTPClient) Get(ctx context.Context, link string, options *model.FeedRequestOptions) (*http.Response, error) {
	// Store the last feed URL and options for assertions.
	m.lastFeedURL = link
	m.lastOptions = options

	if m.err != nil {
		return nil, m.err
	}

	return m.resp, nil
}
