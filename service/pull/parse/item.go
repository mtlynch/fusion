package parse

import (
	"github.com/0x2e/fusion/model"

	"github.com/mmcdole/gofeed"
)

func GoFeedItems(gfItems []*gofeed.Item, feedID uint) []*model.Item {
	items := make([]*model.Item, 0, len(gfItems))
	for _, i := range gfItems {
		// Skip nil items
		if i == nil {
			continue
		}

		unread := true
		content := i.Content
		if content == "" {
			content = i.Description
		}
		guid := i.GUID
		if guid == "" {
			guid = i.Link
		}
		items = append(items, &model.Item{
			Title:   &i.Title,
			GUID:    &guid,
			Link:    &i.Link,
			Content: &content,
			PubDate: i.PublishedParsed,
			Unread:  &unread,
			FeedID:  feedID,
		})
	}

	return items
}
