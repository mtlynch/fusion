package server

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"

	"github.com/0x2e/fusion/model"
	"github.com/0x2e/fusion/repo"
	"github.com/0x2e/fusion/service/pull"
	"github.com/0x2e/fusion/service/sniff"
)

type FeedRepo interface {
	List(filter *repo.FeedListFilter) ([]*model.Feed, error)
	Get(id uint) (*model.Feed, error)
	Create(feed []*model.Feed) error
	Update(id uint, feed *model.Feed) error
	Delete(id uint) error
}

type Feed struct {
	repo FeedRepo
}

func NewFeed(repo FeedRepo) *Feed {
	return &Feed{
		repo: repo,
	}
}

func (f Feed) List(ctx context.Context, req *ReqFeedList) (*RespFeedList, error) {
	filter := &repo.FeedListFilter{
		HaveUnread:   req.HaveUnread,
		HaveBookmark: req.HaveBookmark,
	}
	data, err := f.repo.List(filter)
	if err != nil {
		return nil, err
	}

	feeds := make([]*FeedForm, 0, len(data))
	for _, v := range data {
		feeds = append(feeds, &FeedForm{
			ID:        v.ID,
			Name:      v.Name,
			Link:      v.Link,
			Failure:   v.Failure,
			Suspended: v.Suspended,
			ReqProxy:  v.ReqProxy,
			UpdatedAt: v.UpdatedAt,
			Group:     GroupForm{ID: v.GroupID, Name: v.Group.Name},
		})
	}
	return &RespFeedList{
		Feeds: feeds,
	}, nil
}

func (f Feed) Get(ctx context.Context, req *ReqFeedGet) (*RespFeedGet, error) {
	data, err := f.repo.Get(req.ID)
	if err != nil {
		return nil, err
	}

	return &RespFeedGet{
		ID:        data.ID,
		Name:      data.Name,
		Link:      data.Link,
		Failure:   data.Failure,
		Suspended: data.Suspended,
		ReqProxy:  data.ReqProxy,
		UpdatedAt: data.UpdatedAt,
		Group:     GroupForm{ID: data.GroupID, Name: data.Group.Name},
	}, nil
}

func (f Feed) Create(ctx context.Context, req *ReqFeedCreate) error {
	feeds := make([]*model.Feed, 0, len(req.Feeds))
	for _, r := range req.Feeds {
		feeds = append(feeds, &model.Feed{
			Name:    r.Name,
			Link:    r.Link,
			GroupID: req.GroupID,
		})
	}
	if len(feeds) == 0 {
		return nil
	}

	if err := f.repo.Create(feeds); err != nil {
		return err
	}

	puller := pull.NewPuller(repo.NewFeed(repo.DB), repo.NewItem(repo.DB))
	if len(feeds) >= 1 {
		go func() {
			routinePool := make(chan struct{}, 10)
			defer close(routinePool)
			wg := sync.WaitGroup{}
			for _, feed := range feeds {
				routinePool <- struct{}{}
				wg.Add(1)
				go func() {
					// NOTE: do not use the incoming ctx, as it will be Done() automatically
					// by api timeout middleware
					puller.PullOne(context.Background(), feed.ID)
					<-routinePool
					wg.Done()
				}()
			}
			wg.Wait()
		}()
		return nil
	}
	return puller.PullOne(ctx, feeds[0].ID)
}

func (f Feed) CheckValidity(ctx context.Context, req *ReqFeedCheckValidity) (*RespFeedCheckValidity, error) {
	link := req.Link
	validLinks := make([]ValidityItem, 0)

	_, err := pull.ReadFeedItems(ctx, link, model.FeedRequestOptions{})
	if err == nil {
		// Since we no longer have State.Name, we'll use the link as the title
		validLinks = append(validLinks, ValidityItem{
			Title: &req.Link,
			Link:  &req.Link,
		})
	} else {
		target, err := url.Parse(req.Link)
		if err != nil {
			return nil, err
		}
		sniffed, err := sniff.Sniff(ctx, target)
		if err != nil {
			return nil, err
		}
		for _, l := range sniffed {
			validLinks = append(validLinks, ValidityItem{
				Title: &l.Title,
				Link:  &l.Link,
			})
		}
	}
	return &RespFeedCheckValidity{
		FeedLinks: validLinks,
	}, nil
}

func (f Feed) Update(ctx context.Context, req *ReqFeedUpdate) error {
	data := &model.Feed{
		Name:      req.Name,
		Link:      req.Link,
		Suspended: req.Suspended,
		FeedRequestOptions: model.FeedRequestOptions{
			ReqProxy: req.ReqProxy,
		},
	}
	if req.GroupID != nil {
		data.GroupID = *req.GroupID
	}
	err := f.repo.Update(req.ID, data)
	if errors.Is(err, repo.ErrDuplicatedKey) {
		err = NewBizError(err, http.StatusBadRequest, "link is not allowed to be the same as other feeds")
	}
	return err
}

func (f Feed) Delete(ctx context.Context, req *ReqFeedDelete) error {
	return f.repo.Delete(req.ID)
}

func (f Feed) Refresh(ctx context.Context, req *ReqFeedRefresh) error {
	pull := pull.NewPuller(repo.NewFeed(repo.DB), repo.NewItem(repo.DB))
	if req.ID != nil {
		return pull.PullOne(ctx, *req.ID)
	}
	if req.All != nil && *req.All {
		// NOTE: do not use the incoming ctx, as it will be Done() automatically
		// by api timeout middleware
		go pull.PullAll(context.Background(), true)
	}
	return nil
}
