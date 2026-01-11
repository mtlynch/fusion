package pull

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/pkg/ptr"
	"github.com/0x2e/fusion/repo"
)

var (
	interval = 30 * time.Minute
)

type FeedRepo interface {
	List(filter *repo.FeedListFilter) ([]*model.Feed, error)
	Get(id uint) (*model.Feed, error)
	Update(id uint, feed *model.Feed) error
}

type ItemRepo interface {
	Insert(items []*model.Item) error
}

type Puller struct {
	feedRepo FeedRepo
	itemRepo ItemRepo
}

// FeedPullError captures a feed-specific pull error.
type FeedPullError struct {
	Feed *model.Feed
	Err  error
}

// TODO: cache favicon

func NewPuller(feedRepo FeedRepo, itemRepo ItemRepo) *Puller {
	return &Puller{
		feedRepo: feedRepo,
		itemRepo: itemRepo,
	}
}

func (p *Puller) Run() {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		p.PullAll(context.Background(), false)

		<-ticker.C
	}
}

func (p *Puller) PullAll(ctx context.Context, force bool) error {
	ctx, cancel := context.WithTimeout(ctx, interval/2)
	defer cancel()

	feeds, err := p.feedRepo.List(nil)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			err = nil
		}
		return err
	}
	if len(feeds) == 0 {
		return nil
	}

	errs := PullFeeds(ctx, feeds, force, 10, p.do)
	for _, pullErr := range errs {
		slog.Error("failed to pull feed", "error", pullErr.Err, "feed_id", pullErr.Feed.ID, "feed_link", ptr.From(pullErr.Feed.Link))
	}
	return nil
}

func (p *Puller) PullOne(ctx context.Context, id uint) error {
	f, err := p.feedRepo.Get(id)
	if err != nil {
		return err
	}

	return p.do(ctx, f, true)
}

// PullFeeds pulls a batch of feeds concurrently and returns any per-feed errors.
func PullFeeds(ctx context.Context, feeds []*model.Feed, force bool, maxWorkers int, pullFeed func(context.Context, *model.Feed, bool) error) []FeedPullError {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}

	routinePool := make(chan struct{}, maxWorkers)
	defer close(routinePool)

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []FeedPullError
	)

	for _, f := range feeds {
		routinePool <- struct{}{}
		wg.Add(1)
		go func(f *model.Feed) {
			defer func() {
				wg.Done()
				<-routinePool
			}()

			if err := pullFeed(ctx, f, force); err != nil {
				mu.Lock()
				errs = append(errs, FeedPullError{
					Feed: f,
					Err:  err,
				})
				mu.Unlock()
			}
		}(f)
	}
	wg.Wait()

	return errs
}
