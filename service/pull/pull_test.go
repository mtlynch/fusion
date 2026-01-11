package pull_test

import (
	"context"
	"errors"
	"testing"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/service/pull"
)

func TestPullFeeds(t *testing.T) {
	errOne := errors.New("fetch failed")
	errTwo := errors.New("update failed")

	for _, tt := range []struct {
		explanation    string
		feeds          []*model.Feed
		force          bool
		maxWorkers     int
		pullFeed       func(context.Context, *model.Feed, bool) error
		errorsExpected map[uint]error
	}{
		{
			explanation: "all feeds succeed",
			feeds: []*model.Feed{
				{ID: 1},
				{ID: 2},
			},
			force:      false,
			maxWorkers: 1,
			pullFeed: func(_ context.Context, _ *model.Feed, _ bool) error {
				return nil
			},
			errorsExpected: map[uint]error{},
		},
		{
			explanation: "errors are returned per feed",
			feeds: []*model.Feed{
				{ID: 10},
				{ID: 20},
				{ID: 30},
			},
			force:      true,
			maxWorkers: 1,
			pullFeed: func(_ context.Context, feed *model.Feed, _ bool) error {
				if feed.ID == 10 {
					return errOne
				}
				if feed.ID == 30 {
					return errTwo
				}
				return nil
			},
			errorsExpected: map[uint]error{
				10: errOne,
				30: errTwo,
			},
		},
		{
			explanation: "non-positive maxWorkers defaults to one",
			feeds: []*model.Feed{
				{ID: 7},
			},
			force:      false,
			maxWorkers: 0,
			pullFeed: func(_ context.Context, _ *model.Feed, _ bool) error {
				return errOne
			},
			errorsExpected: map[uint]error{
				7: errOne,
			},
		},
	} {
		t.Run(tt.explanation, func(t *testing.T) {
			errs := pull.PullFeeds(context.Background(), tt.feeds, tt.force, tt.maxWorkers, tt.pullFeed)

			if got, want := len(errs), len(tt.errorsExpected); got != want {
				t.Fatalf("len(errs)=%d, want=%d", got, want)
			}

			gotErrors := make(map[uint]error)
			for _, pullErr := range errs {
				gotErrors[pullErr.Feed.ID] = pullErr.Err
			}

			for feedID, errExpected := range tt.errorsExpected {
				if got, want := gotErrors[feedID], errExpected; got != want {
					t.Errorf("err for feed %d=%v, want=%v", feedID, got, want)
				}
			}
		})
	}
}
