package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/0x2e/fusion/auth"
	"github.com/0x2e/fusion/server"
	"github.com/PuerkitoBio/goquery"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
	"github.com/microcosm-cc/bluemonday"
)

const sessionKeyName = "session-token"

var rssHubMap = map[string]string{
	"/papers/category/arxiv": "arxiv.org",
	"/trendingpapers/papers": "arxiv.org",
	"/github":              "github.com",
	"/google":              "google.com",
	"/dockerhub":           "hub.docker.com",
	"/imdb":                "imdb.com",
	"/hackernews":          "news.ycombinator.com",
	"/phoronix":            "phoronix.com",
	"/rsshub":              "rsshub.app",
	"/twitch":              "twitch.tv",
	"/youtube":             "youtube.com",
}

type Params struct {
	PasswordHash    *auth.HashedPassword
	UseSecureCookie bool
	ItemService     *server.Item
	FeedService     *server.Feed
	GroupService    *server.Group
	Version         string
}

type App struct {
	passwordHash    *auth.HashedPassword
	useSecureCookie bool
	itemService     *server.Item
	feedService     *server.Feed
	groupService    *server.Group
	version         string
}

func New(params Params) *App {
	return &App{
		passwordHash:    params.PasswordHash,
		useSecureCookie: params.UseSecureCookie,
		itemService:     params.ItemService,
		feedService:     params.FeedService,
		groupService:    params.GroupService,
		version:         params.Version,
	}
}

func (a *App) Register(r *echo.Echo) {
	r.GET("/login", a.LoginGet)
	r.POST("/login", a.LoginPost)
	r.POST("/logout", a.LogoutPost)

	r.GET("/", a.requireAuth(a.UnreadItems))
	r.GET("/all", a.requireAuth(a.AllItems))
	r.GET("/bookmarks", a.requireAuth(a.BookmarkItems))
	r.GET("/feeds/:id", a.requireAuth(a.FeedItems))
	r.GET("/feeds/:id/settings", a.requireAuth(a.FeedSettingsGet))
	r.POST("/feeds/:id/settings", a.requireAuth(a.FeedSettingsPost))
	r.POST("/feeds/:id/delete", a.requireAuth(a.FeedDeletePost))
	r.POST("/feeds/:id/refresh", a.requireAuth(a.FeedRefreshPost))
	r.GET("/feeds/import", a.requireAuth(a.FeedImportGet))
	r.POST("/feeds/import/manual", a.requireAuth(a.FeedImportManualPost))
	r.POST("/feeds/import/opml", a.requireAuth(a.FeedImportOPMLPost))
	r.POST("/feeds/refresh", a.requireAuth(a.FeedRefreshAllPost))
	r.GET("/feeds/export", a.requireAuth(a.FeedExportOPML))

	r.GET("/groups/:id", a.requireAuth(a.GroupItems))
	
	r.GET("/items/:id", a.requireAuth(a.ItemDetail))
	r.POST("/items/:id/toggle-unread", a.requireAuth(a.ToggleUnread))
	r.POST("/items/:id/toggle-bookmark", a.requireAuth(a.ToggleBookmark))
	r.POST("/items/mark-read", a.requireAuth(a.MarkRead))

	r.GET("/search", a.requireAuth(a.SearchItems))

	r.GET("/settings", a.requireAuth(a.SettingsGet))
	r.POST("/settings/groups", a.requireAuth(a.GroupCreatePost))
	r.POST("/settings/groups/:id", a.requireAuth(a.GroupUpdatePost))
	r.POST("/settings/groups/:id/delete", a.requireAuth(a.GroupDeletePost))
}

func (a *App) LoginGet(c echo.Context) error {
	if a.passwordHash == nil {
		return c.Redirect(http.StatusFound, "/")
	}
	return renderAuth(c, "templates/pages/login.html", map[string]interface{}{
		"Title": "Login",
	})
}

func (a *App) LoginPost(c echo.Context) error {
	if a.passwordHash == nil {
		return c.Redirect(http.StatusFound, "/")
	}

	password := strings.TrimSpace(c.FormValue("password"))
	if password == "" {
		return renderAuth(c, "templates/pages/login.html", map[string]interface{}{
			"Title": "Login",
			"Error": "Password is required",
		})
	}

	attemptedPasswordHash, err := auth.HashPassword(password)
	if err != nil {
		return renderAuth(c, "templates/pages/login.html", map[string]interface{}{
			"Title": "Login",
			"Error": "Invalid password",
		})
	}
	if correctPasswordHash := a.passwordHash; !attemptedPasswordHash.Equals(*correctPasswordHash) {
		return renderAuth(c, "templates/pages/login.html", map[string]interface{}{
			"Title": "Login",
			"Error": "Wrong password",
		})
	}

	sess, err := session.Get(sessionKeyName, c)
	if err != nil {
		return err
	}
	if !a.useSecureCookie {
		sess.Options.Secure = false
		sess.Options.SameSite = http.SameSiteDefaultMode
	}
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return err
	}

	return c.Redirect(http.StatusFound, "/")
}

func (a *App) LogoutPost(c echo.Context) error {
	sess, err := session.Get(sessionKeyName, c)
	if err != nil {
		return err
	}
	sess.Options.MaxAge = -1
	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/login")
}

func (a *App) requireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	if a.passwordHash == nil {
		return next
	}

	return func(c echo.Context) error {
		sess, err := session.Get(sessionKeyName, c)
		if err != nil || sess.IsNew {
			return c.Redirect(http.StatusFound, "/login")
		}
		return next(c)
	}
}

func (a *App) UnreadItems(c echo.Context) error {
	filter := parseListFilter(c)
	unread := true
	filter.Unread = &unread
	return a.renderItemsPage(c, "Unread", "/", filter, nil)
}

func (a *App) AllItems(c echo.Context) error {
	filter := parseListFilter(c)
	return a.renderItemsPage(c, "All items", "/all", filter, nil)
}

func (a *App) BookmarkItems(c echo.Context) error {
	filter := parseListFilter(c)
	bookmark := true
	filter.Bookmark = &bookmark
	return a.renderItemsPage(c, "Bookmarks", "/bookmarks", filter, nil)
}

func (a *App) FeedItems(c echo.Context) error {
	filter := parseListFilter(c)
	feedID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	filter.FeedID = &feedID
	feed, err := a.feedService.Get(c.Request().Context(), &server.ReqFeedGet{ID: feedID})
	if err != nil {
		return err
	}

	actions := []ActionButton{
		{Label: "Refresh", Method: "post", URL: fmt.Sprintf("/feeds/%d/refresh", feedID)},
		{Label: "Settings", Method: "get", URL: fmt.Sprintf("/feeds/%d/settings", feedID)},
	}
	return a.renderItemsPage(c, derefString(feed.Name, "Feed"), "/feeds/"+strconv.FormatUint(uint64(feedID), 10), filter, actions)
}

func (a *App) GroupItems(c echo.Context) error {
	filter := parseListFilter(c)
	groupID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	filter.GroupID = &groupID
	groupName := "Group"
	groups, err := a.groupService.All(c.Request().Context())
	if err == nil {
		for _, group := range groups.Groups {
			if group.ID == groupID {
				groupName = derefString(group.Name, "Group")
				break
			}
		}
	}
	return a.renderItemsPage(c, groupName, "/groups/"+strconv.FormatUint(uint64(groupID), 10), filter, nil)
}

func (a *App) SearchItems(c echo.Context) error {
	filter := parseListFilter(c)
	filter.Keyword = strings.TrimSpace(c.QueryParam("keyword"))
	filter.Page = maxInt(1, filter.Page)

	listData, err := a.buildItemList(c.Request().Context(), filter, "/search")
	if err != nil {
		return err
	}
	if isHTMX(c) {
		return renderPartial(c, "templates/partials/item_list.html", "item-list.html", listData)
	}
	pageData := SearchPageData{
		BasePageData: BasePageData{
			Title:   "Search",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Keyword:  filter.Keyword,
		ItemList: listData,
	}
	return renderPage(c, "templates/layouts/base.html", "templates/pages/search.html", pageData)
}

func (a *App) renderItemsPage(c echo.Context, title, path string, filter ListFilter, actions []ActionButton) error {
	listData, err := a.buildItemList(c.Request().Context(), filter, path)
	if err != nil {
		return err
	}
	if isHTMX(c) {
		return renderPartial(c, "templates/partials/item_list.html", "item-list.html", listData)
	}
	actions = append(actions, a.markReadActions(filter)...)
	pageData := ItemsListPageData{
		BasePageData: BasePageData{
			Title:   title,
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		HeaderTitle:  title,
		HeaderActions: actions,
		ShowSearch:   path == "/",
		ItemList:     listData,
	}
	return renderPage(c, "templates/layouts/base.html", "templates/pages/items_list.html", pageData)
}

func (a *App) markReadActions(filter ListFilter) []ActionButton {
	fields := []Field{
		{Name: "page", Value: strconv.Itoa(filter.Page)},
		{Name: "page_size", Value: strconv.Itoa(filter.PageSize)},
	}
	if filter.Keyword != "" {
		fields = append(fields, Field{Name: "keyword", Value: filter.Keyword})
	}
	if filter.FeedID != nil {
		fields = append(fields, Field{Name: "feed_id", Value: strconv.FormatUint(uint64(*filter.FeedID), 10)})
	}
	if filter.GroupID != nil {
		fields = append(fields, Field{Name: "group_id", Value: strconv.FormatUint(uint64(*filter.GroupID), 10)})
	}
	if filter.Unread != nil {
		fields = append(fields, Field{Name: "unread", Value: strconv.FormatBool(*filter.Unread)})
	}
	if filter.Bookmark != nil {
		fields = append(fields, Field{Name: "bookmark", Value: strconv.FormatBool(*filter.Bookmark)})
	}

	actions := []ActionButton{
		{
			Label:  "Mark page read",
			Method: "post",
			URL:    "/items/mark-read",
			Fields: append(fields, Field{Name: "mode", Value: "page"}),
		},
	}
	if filter.FeedID != nil {
		actions = append(actions, ActionButton{
			Label:  "Mark feed read",
			Method: "post",
			URL:    "/items/mark-read",
			Fields: append(fields, Field{Name: "mode", Value: "all"}),
		})
	}
	return actions
}

func (a *App) ItemDetail(c echo.Context) error {
	itemID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	item, err := a.itemService.Get(c.Request().Context(), &server.ReqItemGet{ID: itemID})
	if err != nil {
		return err
	}

	fromFilter := parseItemDetailFilter(c)
	prevURL, nextURL := a.findPrevNext(c.Request().Context(), itemID, fromFilter)

	content := renderItemContent(derefString(item.Content, ""), derefString(item.Link, ""))
	itemView := ItemDetailView{
		ID:       item.ID,
		Title:    derefString(item.Title, ""),
		Link:     derefString(item.Link, ""),
		Content:  derefString(item.Content, ""),
		Unread:   derefBool(item.Unread, true),
		Bookmark: derefBool(item.Bookmark, false),
		PubDate:  item.PubDate,
		Feed: ItemFeedView{
			ID:   item.Feed.ID,
			Name: derefString(item.Feed.Name, ""),
			Link: derefString(item.Feed.Link, ""),
		},
	}
	pageData := ItemDetailPageData{
		BasePageData: BasePageData{
			Title:   derefString(item.Title, "Item"),
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Item:         itemView,
		ContentHTML:  content,
		PublishedAt:  formatDateTime(item.PubDate),
		PrevItemURL:  prevURL,
		NextItemURL:  nextURL,
		ReturnURL:    c.Request().Referer(),
		UnreadLabel:  unreadLabel(item.Unread),
		BookmarkLabel: bookmarkLabel(item.Bookmark),
	}

	return renderPage(c, "templates/layouts/base.html", "templates/pages/item_detail.html", pageData)
}

func (a *App) ToggleUnread(c echo.Context) error {
	itemID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	item, err := a.itemService.Get(c.Request().Context(), &server.ReqItemGet{ID: itemID})
	if err != nil {
		return err
	}

	newUnread := !derefBool(item.Unread, true)
	ids := []uint{itemID}
	if err := a.itemService.UpdateUnread(c.Request().Context(), &server.ReqItemUpdateUnread{IDs: ids, Unread: &newUnread}); err != nil {
		return err
	}

	if !isHTMX(c) {
		target := strings.TrimSpace(c.FormValue("return"))
		if target == "" {
			target = c.Request().Referer()
		}
		if target == "" {
			target = "/"
		}
		return c.Redirect(http.StatusFound, target)
	}
	return a.renderItemRow(c, itemID)
}

func (a *App) ToggleBookmark(c echo.Context) error {
	itemID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	item, err := a.itemService.Get(c.Request().Context(), &server.ReqItemGet{ID: itemID})
	if err != nil {
		return err
	}

	newBookmark := !derefBool(item.Bookmark, false)
	if err := a.itemService.UpdateBookmark(c.Request().Context(), &server.ReqItemUpdateBookmark{ID: itemID, Bookmark: &newBookmark}); err != nil {
		return err
	}

	if !isHTMX(c) {
		target := strings.TrimSpace(c.FormValue("return"))
		if target == "" {
			target = c.Request().Referer()
		}
		if target == "" {
			target = "/"
		}
		return c.Redirect(http.StatusFound, target)
	}
	return a.renderItemRow(c, itemID)
}

func (a *App) renderItemRow(c echo.Context, itemID uint) error {
	item, err := a.itemService.Get(c.Request().Context(), &server.ReqItemGet{ID: itemID})
	if err != nil {
		return err
	}

	highlightUnread := c.FormValue("highlight_unread") == "1"
	returnURL := strings.TrimSpace(c.FormValue("return"))
	if returnURL == "" {
		returnURL = c.Request().Referer()
	}
	if returnURL == "" {
		returnURL = "/"
	}
	filter, listPath := parseFilterFromURL(returnURL)
	if listPath == "" {
		listPath = "/"
	}

	row := buildItemRow(itemViewFromGet(item), filter, listPath, highlightUnread)
	return renderPartial(c, "templates/partials/item_row.html", "item-row.html", row)
}

func (a *App) MarkRead(c echo.Context) error {
	if err := c.Request().ParseForm(); err != nil {
		return err
	}
	filter := parseListFilterFromValues(c.Request().Form)
	mode := strings.TrimSpace(c.FormValue("mode"))

	if mode == "all" {
		if filter.FeedID == nil {
			return c.Redirect(http.StatusFound, c.Request().Referer())
		}
		if err := a.markAllFeedRead(c.Request().Context(), *filter.FeedID); err != nil {
			return err
		}
		return c.Redirect(http.StatusFound, c.Request().Referer())
	}

	resp, err := a.itemService.List(c.Request().Context(), &server.ReqItemList{
		Paginate: server.Paginate{Page: filter.Page, PageSize: filter.PageSize},
		Keyword:  optionalString(filter.Keyword),
		FeedID:   filter.FeedID,
		GroupID:  filter.GroupID,
		Unread:   filter.Unread,
		Bookmark: filter.Bookmark,
	})
	if err != nil {
		return err
	}
	ids := make([]uint, 0, len(resp.Items))
	for _, item := range resp.Items {
		ids = append(ids, item.ID)
	}
	read := false
	if len(ids) > 0 {
		if err := a.itemService.UpdateUnread(c.Request().Context(), &server.ReqItemUpdateUnread{IDs: ids, Unread: &read}); err != nil {
			return err
		}
	}
	return c.Redirect(http.StatusFound, c.Request().Referer())
}

func (a *App) markAllFeedRead(ctx context.Context, feedID uint) error {
	unread := true
	read := false
	for {
		resp, err := a.itemService.List(ctx, &server.ReqItemList{
			Paginate: server.Paginate{Page: 1, PageSize: 200},
			FeedID:   &feedID,
			Unread:   &unread,
		})
		if err != nil {
			return err
		}
		if len(resp.Items) == 0 {
			break
		}
		ids := make([]uint, 0, len(resp.Items))
		for _, item := range resp.Items {
			ids = append(ids, item.ID)
		}
		if err := a.itemService.UpdateUnread(ctx, &server.ReqItemUpdateUnread{IDs: ids, Unread: &read}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) FeedSettingsGet(c echo.Context) error {
	feedID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	feed, err := a.feedService.Get(c.Request().Context(), &server.ReqFeedGet{ID: feedID})
	if err != nil {
		return err
	}
	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}
	pageData := FeedSettingsPageData{
		BasePageData: BasePageData{
			Title:   "Feed settings",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Feed: FeedSettingsView{
			ID:        feed.ID,
			Name:      derefString(feed.Name, ""),
			Link:      derefString(feed.Link, ""),
			GroupID:   feed.Group.ID,
			ReqProxy:  derefString(feed.ReqProxy, ""),
			Suspended: derefBool(feed.Suspended, false),
		},
		Groups: groupViewsFromForms(groups.Groups),
	}
	return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_settings.html", pageData)
}

func (a *App) FeedSettingsPost(c echo.Context) error {
	feedID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(c.FormValue("name"))
	link := strings.TrimSpace(c.FormValue("link"))
	groupID, _ := strconv.ParseUint(c.FormValue("group_id"), 10, 64)
	proxy := strings.TrimSpace(c.FormValue("req_proxy"))
	suspended := c.FormValue("suspended") == "true"

	req := &server.ReqFeedUpdate{
		ID:        feedID,
		Name:      optionalString(name),
		Link:      optionalString(link),
		GroupID:   optionalUint(uint(groupID)),
		ReqProxy:  optionalString(proxy),
		Suspended: optionalBool(suspended),
	}
	if err := a.feedService.Update(c.Request().Context(), req); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/feeds/%d/settings", feedID))
}

func (a *App) FeedDeletePost(c echo.Context) error {
	feedID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	if err := a.feedService.Delete(c.Request().Context(), &server.ReqFeedDelete{ID: feedID}); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/")
}

func (a *App) FeedRefreshPost(c echo.Context) error {
	feedID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	if err := a.feedService.Refresh(c.Request().Context(), &server.ReqFeedRefresh{ID: &feedID}); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, c.Request().Referer())
}

func (a *App) FeedRefreshAllPost(c echo.Context) error {
	all := true
	if err := a.feedService.Refresh(c.Request().Context(), &server.ReqFeedRefresh{All: &all}); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, c.Request().Referer())
}

func (a *App) FeedImportGet(c echo.Context) error {
	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}
	pageData := FeedImportPageData{
		BasePageData: BasePageData{
			Title:   "Import feeds",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Groups: groupViewsFromForms(groups.Groups),
	}
	return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
}

func (a *App) FeedImportManualPost(c echo.Context) error {
	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}

	link := strings.TrimSpace(c.FormValue("link"))
	name := strings.TrimSpace(c.FormValue("name"))
	proxy := strings.TrimSpace(c.FormValue("proxy"))
	groupID, _ := strconv.ParseUint(c.FormValue("group_id"), 10, 64)
	candidate := strings.TrimSpace(c.FormValue("candidate"))

	pageData := FeedImportPageData{
		BasePageData: BasePageData{
			Title:   "Import feeds",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Groups:        groupViewsFromForms(groups.Groups),
		ManualLink:    link,
		ManualName:    name,
		ManualProxy:   proxy,
		ManualGroupID: uint(groupID),
	}

	if link == "" {
		pageData.ManualError = "Link is required"
		return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
	}

	candidateLink := candidate
	if candidateLink == "" {
		resp, err := a.feedService.CheckValidity(c.Request().Context(), &server.ReqFeedCheckValidity{
			Link: link,
			RequestOptions: server.FeedRequestOptions{Proxy: optionalString(proxy)},
		})
		if err != nil {
			pageData.ManualError = err.Error()
			return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
		}
		if len(resp.FeedLinks) == 0 {
			pageData.ManualError = "No valid feeds found"
			return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
		}
		if len(resp.FeedLinks) > 1 {
			pageData.ManualCandidates = validityViewsFromItems(resp.FeedLinks)
			return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
		}
		candidateLink = derefString(resp.FeedLinks[0].Link, link)
		if name == "" {
			name = derefString(resp.FeedLinks[0].Title, "")
		}
	}

	if name == "" {
		if u, err := url.Parse(candidateLink); err == nil {
			name = u.Hostname()
		}
	}

	req := &server.ReqFeedCreate{
		GroupID: uint(groupID),
		Feeds: []struct {
			Name           *string "json:\"name\" validate:\"required\""
			Link           *string "json:\"link\" validate:\"required\""
			RequestOptions server.FeedRequestOptions "json:\"request_options\""
		}{
			{
				Name:           optionalString(name),
				Link:           optionalString(candidateLink),
				RequestOptions: server.FeedRequestOptions{Proxy: optionalString(proxy)},
			},
		},
	}
	resp, err := a.feedService.Create(c.Request().Context(), req)
	if err != nil {
		pageData.ManualError = err.Error()
		return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
	}
	if len(resp.IDs) > 0 {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/feeds/%d", resp.IDs[0]))
	}
	return c.Redirect(http.StatusFound, "/")
}

func (a *App) FeedImportOPMLPost(c echo.Context) error {
	file, err := c.FormFile("opml")
	if err != nil {
		return err
	}
	opened, err := file.Open()
	if err != nil {
		return err
	}
	defer opened.Close()

	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}

	opmlGroups, err := parseOPML(opened)
	pageData := FeedImportPageData{
		BasePageData: BasePageData{
			Title:   "Import feeds",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Groups: groupViewsFromForms(groups.Groups),
	}
	if err != nil {
		pageData.OPMLError = err.Error()
		return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
	}

	logEntries := []OPMLLogEntry{}
	for _, group := range opmlGroups {
		groupID := findGroupID(pageData.Groups, group.Name)
		if groupID == 0 {
			resp, err := a.groupService.Create(c.Request().Context(), &server.ReqGroupCreate{Name: optionalString(group.Name)})
			if err != nil {
				logEntries = append(logEntries, OPMLLogEntry{Message: err.Error(), IsError: true})
				continue
			}
			groupID = resp.ID
		}

		feeds := make([]struct {
			Name           *string "json:\"name\" validate:\"required\""
			Link           *string "json:\"link\" validate:\"required\""
			RequestOptions server.FeedRequestOptions "json:\"request_options\""
		}, 0, len(group.Feeds))
		for _, feed := range group.Feeds {
			feeds = append(feeds, struct {
				Name           *string "json:\"name\" validate:\"required\""
				Link           *string "json:\"link\" validate:\"required\""
				RequestOptions server.FeedRequestOptions "json:\"request_options\""
			}{
				Name:           optionalString(feed.Name),
				Link:           optionalString(feed.Link),
				RequestOptions: server.FeedRequestOptions{},
			})
		}

		if len(feeds) == 0 {
			continue
		}

		if _, err := a.feedService.Create(c.Request().Context(), &server.ReqFeedCreate{GroupID: groupID, Feeds: feeds}); err != nil {
			logEntries = append(logEntries, OPMLLogEntry{Message: err.Error(), IsError: true})
			for _, feed := range group.Feeds {
				logEntries = append(logEntries, OPMLLogEntry{Message: "Failed: " + feed.Link, IsError: true})
			}
			continue
		}

		for _, feed := range group.Feeds {
			logEntries = append(logEntries, OPMLLogEntry{Message: "Imported: " + feed.Link})
		}
	}

	pageData.OPMLLog = logEntries
	return renderPage(c, "templates/layouts/base.html", "templates/pages/feed_import.html", pageData)
}

func (a *App) FeedExportOPML(c echo.Context) error {
	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}
	feeds, err := a.feedService.List(c.Request().Context(), &server.ReqFeedList{})
	if err != nil {
		return err
	}

	groupMap := map[uint]*OPMLGroup{}
	for _, group := range groups.Groups {
		groupMap[group.ID] = &OPMLGroup{Name: derefString(group.Name, "Group")}
	}
	for _, feed := range feeds.Feeds {
		group, ok := groupMap[feed.Group.ID]
		if !ok {
			group = &OPMLGroup{Name: derefString(feed.Group.Name, "Group")}
			groupMap[feed.Group.ID] = group
		}
		group.Feeds = append(group.Feeds, OPMLFeed{Name: derefString(feed.Name, ""), Link: derefString(feed.Link, "")})
	}
	output := buildOPML(sortedGroups(groupMap))

	c.Response().Header().Set("Content-Type", "application/xml")
	c.Response().Header().Set("Content-Disposition", "attachment; filename=feeds.opml")
	return c.String(http.StatusOK, output)
}

func (a *App) SettingsGet(c echo.Context) error {
	groups, err := a.groupService.All(c.Request().Context())
	if err != nil {
		return err
	}
	pageData := SettingsPageData{
		BasePageData: BasePageData{
			Title:   "Settings",
			Sidebar: a.sidebarData(c.Request().Context(), c.Request().URL.Path),
		},
		Groups: groupViewsFromForms(groups.Groups),
	}
	return renderPage(c, "templates/layouts/base.html", "templates/pages/settings.html", pageData)
}

func (a *App) GroupCreatePost(c echo.Context) error {
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return c.Redirect(http.StatusFound, "/settings#groups")
	}
	_, err := a.groupService.Create(c.Request().Context(), &server.ReqGroupCreate{Name: optionalString(name)})
	if err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/settings#groups")
}

func (a *App) GroupUpdatePost(c echo.Context) error {
	groupID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(c.FormValue("name"))
	if name == "" {
		return c.Redirect(http.StatusFound, "/settings#groups")
	}
	if err := a.groupService.Update(c.Request().Context(), &server.ReqGroupUpdate{ID: groupID, Name: optionalString(name)}); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/settings#groups")
}

func (a *App) GroupDeletePost(c echo.Context) error {
	groupID, err := parseUintParam(c, "id")
	if err != nil {
		return err
	}
	if err := a.groupService.Delete(c.Request().Context(), &server.ReqGroupDelete{ID: groupID}); err != nil {
		return err
	}
	return c.Redirect(http.StatusFound, "/settings#groups")
}

func (a *App) buildItemList(ctx context.Context, filter ListFilter, listPath string) (ItemListView, error) {
	filter.Page = maxInt(1, filter.Page)
	filter.PageSize = defaultInt(filter.PageSize, 10)

	resp, err := a.itemService.List(ctx, &server.ReqItemList{
		Paginate: server.Paginate{Page: filter.Page, PageSize: filter.PageSize},
		Keyword:  optionalString(filter.Keyword),
		FeedID:   filter.FeedID,
		GroupID:  filter.GroupID,
		Unread:   filter.Unread,
		Bookmark: filter.Bookmark,
	})
	if err != nil {
		return ItemListView{}, err
	}

	rows := make([]ItemRowView, 0, len(resp.Items))
	for _, item := range resp.Items {
		rows = append(rows, buildItemRow(itemViewFromList(item), filter, listPath, filter.Unread != nil && *filter.Unread))
	}

	pagination := buildPagination(filter, listPath, derefInt(resp.Total, 0))
	return ItemListView{
		Rows:         rows,
		Pagination:   pagination,
		HighlightUnread: filter.Unread != nil && *filter.Unread,
		EmptyMessage: "No items",
		ListPath:     listPath,
		Filter:       buildFilterView(filter),
	}, nil
}

func (a *App) sidebarData(ctx context.Context, currentPath string) SidebarView {
	groupsResp, err := a.groupService.All(ctx)
	if err != nil {
		return SidebarView{}
	}
	feedsResp, err := a.feedService.List(ctx, &server.ReqFeedList{})
	if err != nil {
		return SidebarView{}
	}

	groupMap := map[uint]*SidebarGroup{}
	for _, group := range groupsResp.Groups {
		groupMap[group.ID] = &SidebarGroup{ID: group.ID, Name: derefString(group.Name, "Group"), Open: true}
	}
	for _, feed := range feedsResp.Feeds {
		group, ok := groupMap[feed.Group.ID]
		if !ok {
			group = &SidebarGroup{ID: feed.Group.ID, Name: derefString(feed.Group.Name, "Group"), Open: true}
			groupMap[feed.Group.ID] = group
		}
		statusClass := ""
		if feed.Suspended != nil && *feed.Suspended {
			statusClass = "text-neutral-content/60"
		}
		if feed.Failure != nil && *feed.Failure != "" {
			statusClass = "text-error"
		}
		group.Feeds = append(group.Feeds, SidebarFeed{
			ID:          feed.ID,
			Name:        derefString(feed.Name, ""),
			UnreadCount: feed.UnreadCount,
			FaviconURL:  faviconURL(derefString(feed.Link, "")),
			Active:      currentPath == "/feeds/"+strconv.FormatUint(uint64(feed.ID), 10),
			StatusClass: statusClass,
		})
	}

	groups := make([]SidebarGroup, 0, len(groupMap))
	for _, group := range groupMap {
		sort.Slice(group.Feeds, func(i, j int) bool {
			return group.Feeds[i].Name < group.Feeds[j].Name
		})
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})

	systemLinks := []NavLink{
		{Label: "Unread", URL: "/", Active: currentPath == "/"},
		{Label: "Bookmarks", URL: "/bookmarks", Active: currentPath == "/bookmarks"},
		{Label: "All", URL: "/all", Active: currentPath == "/all"},
		{Label: "Search", URL: "/search", Active: currentPath == "/search"},
		{Label: "Settings", URL: "/settings", Active: currentPath == "/settings"},
	}

	return SidebarView{
		SystemLinks:    systemLinks,
		Groups:         groups,
		Version:        a.version,
		ShowThemeToggle: true,
	}
}

func (a *App) findPrevNext(ctx context.Context, currentID uint, filter ListFilter) (string, string) {
	filter.Page = 1
	filter.PageSize = 100
	resp, err := a.itemService.List(ctx, &server.ReqItemList{
		Paginate: server.Paginate{Page: filter.Page, PageSize: filter.PageSize},
		Keyword:  optionalString(filter.Keyword),
		FeedID:   filter.FeedID,
		GroupID:  filter.GroupID,
		Unread:   filter.Unread,
		Bookmark: filter.Bookmark,
	})
	if err != nil {
		return "", ""
	}
	ids := make([]uint, 0, len(resp.Items))
	for _, item := range resp.Items {
		ids = append(ids, item.ID)
	}
	for i, id := range ids {
		if id == currentID {
			var prevURL, nextURL string
			if i > 0 {
				prevURL = buildItemURL(ids[i-1], filter)
			}
			if i < len(ids)-1 {
				nextURL = buildItemURL(ids[i+1], filter)
			}
			return prevURL, nextURL
		}
	}
	return "", ""
}

func parseItemDetailFilter(c echo.Context) ListFilter {
	filter := ListFilter{}
	from := strings.TrimSpace(c.QueryParam("from"))
	switch from {
	case "unread":
		unread := true
		filter.Unread = &unread
	case "bookmarks":
		bookmark := true
		filter.Bookmark = &bookmark
	case "feeds":
		if id, err := strconv.ParseUint(c.QueryParam("feed_id"), 10, 64); err == nil {
			feedID := uint(id)
			filter.FeedID = &feedID
		}
	case "groups":
		if id, err := strconv.ParseUint(c.QueryParam("group_id"), 10, 64); err == nil {
			groupID := uint(id)
			filter.GroupID = &groupID
		}
	case "search":
		filter.Keyword = strings.TrimSpace(c.QueryParam("keyword"))
	}
	return filter
}

func renderPage(c echo.Context, layout, page string, data interface{}) error {
	files := []string{
		layout,
		"templates/partials/sidebar.html",
		"templates/partials/item_list.html",
		"templates/partials/item_row.html",
		"templates/partials/pagination.html",
		page,
	}
	if layout == "templates/layouts/auth.html" {
		files = []string{layout, page}
	}
	for i := range files {
		files[i] = normalizeTemplatePath(files[i])
	}
	funcs := template.FuncMap{
		"eq":       func(a, b interface{}) bool { return a == b },
		"deref":    func(s *string, fallback string) string { return derefString(s, fallback) },
		"timeago":  timeAgo,
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}
	tmpl, err := template.New("base.html").Funcs(funcs).ParseFS(Templates, files...)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(c.Response(), pathFromLayout(layout), data)
}

func renderAuth(c echo.Context, page string, data interface{}) error {
	return renderPage(c, "templates/layouts/auth.html", page, data)
}

func renderPartial(c echo.Context, file, name string, data interface{}) error {
	files := []string{normalizeTemplatePath(file)}
	if name == "item-list.html" {
		files = append(files,
			normalizeTemplatePath("templates/partials/item_row.html"),
			normalizeTemplatePath("templates/partials/pagination.html"),
		)
	}
	tmpl, err := template.New(name).ParseFS(Templates, files...)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(c.Response(), name, data)
}

func pathFromLayout(layout string) string {
	switch layout {
	case "templates/layouts/auth.html":
		return "auth.html"
	default:
		return "base.html"
	}
}

func normalizeTemplatePath(path string) string {
	return strings.TrimPrefix(path, "templates/")
}

func parseListFilter(c echo.Context) ListFilter {
	filter := ListFilter{}
	filter.Page = parseQueryInt(c.QueryParam("page"), 1)
	filter.PageSize = parseQueryInt(c.QueryParam("page_size"), 10)
	if keyword := strings.TrimSpace(c.QueryParam("keyword")); keyword != "" {
		filter.Keyword = keyword
	}
	if feedID := strings.TrimSpace(c.QueryParam("feed_id")); feedID != "" {
		if id, err := strconv.ParseUint(feedID, 10, 64); err == nil {
			idVal := uint(id)
			filter.FeedID = &idVal
		}
	}
	if groupID := strings.TrimSpace(c.QueryParam("group_id")); groupID != "" {
		if id, err := strconv.ParseUint(groupID, 10, 64); err == nil {
			idVal := uint(id)
			filter.GroupID = &idVal
		}
	}
	if unread := strings.TrimSpace(c.QueryParam("unread")); unread != "" {
		if unread == "true" || unread == "1" {
			val := true
			filter.Unread = &val
		} else if unread == "false" || unread == "0" {
			val := false
			filter.Unread = &val
		}
	}
	if bookmark := strings.TrimSpace(c.QueryParam("bookmark")); bookmark != "" {
		if bookmark == "true" || bookmark == "1" {
			val := true
			filter.Bookmark = &val
		} else if bookmark == "false" || bookmark == "0" {
			val := false
			filter.Bookmark = &val
		}
	}
	return filter
}

func parseListFilterFromValues(values url.Values) ListFilter {
	filter := ListFilter{}
	filter.Page = parseQueryInt(values.Get("page"), 1)
	filter.PageSize = parseQueryInt(values.Get("page_size"), 10)
	if keyword := strings.TrimSpace(values.Get("keyword")); keyword != "" {
		filter.Keyword = keyword
	}
	if feedID := strings.TrimSpace(values.Get("feed_id")); feedID != "" {
		if id, err := strconv.ParseUint(feedID, 10, 64); err == nil {
			idVal := uint(id)
			filter.FeedID = &idVal
		}
	}
	if groupID := strings.TrimSpace(values.Get("group_id")); groupID != "" {
		if id, err := strconv.ParseUint(groupID, 10, 64); err == nil {
			idVal := uint(id)
			filter.GroupID = &idVal
		}
	}
	if unread := strings.TrimSpace(values.Get("unread")); unread != "" {
		if unread == "true" || unread == "1" {
			val := true
			filter.Unread = &val
		} else if unread == "false" || unread == "0" {
			val := false
			filter.Unread = &val
		}
	}
	if bookmark := strings.TrimSpace(values.Get("bookmark")); bookmark != "" {
		if bookmark == "true" || bookmark == "1" {
			val := true
			filter.Bookmark = &val
		} else if bookmark == "false" || bookmark == "0" {
			val := false
			filter.Bookmark = &val
		}
	}
	return filter
}

func parseFilterFromURL(raw string) (ListFilter, string) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ListFilter{}, ""
	}
	return parseListFilterFromValues(parsed.Query()), parsed.Path
}

func parseQueryInt(val string, fallback int) int {
	if val == "" {
		return fallback
	}
	n, err := strconv.Atoi(val)
	if err != nil || n == 0 {
		return fallback
	}
	return n
}

func parseUintParam(c echo.Context, name string) (uint, error) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}

func buildItemRow(item ItemView, filter ListFilter, listPath string, highlightUnread bool) ItemRowView {
	itemURL := buildItemURL(item.ID, filter)
	return ItemRowView{
		ID:             item.ID,
		ItemURL:        itemURL,
		Title:          item.Title,
		Link:           item.Link,
		FeedName:       item.Feed.Name,
		FaviconURL:     faviconURL(item.Feed.Link),
		TimeAgo:        timeAgo(item.PubDate),
		DimTitle:       highlightUnread && !item.Unread,
		UnreadLabel:    unreadLabel(optionalBool(item.Unread)),
		BookmarkLabel:  bookmarkLabel(optionalBool(item.Bookmark)),
		ReturnURL:      buildListURL(listPath, filter, nil),
		HighlightUnread: highlightUnread,
	}
}

func buildItemURL(itemID uint, filter ListFilter) string {
	vals := url.Values{}
	from := "all"
	if filter.Unread != nil && *filter.Unread {
		from = "unread"
	}
	if filter.Bookmark != nil && *filter.Bookmark {
		from = "bookmarks"
	}
	if filter.FeedID != nil {
		from = "feeds"
		vals.Set("feed_id", strconv.FormatUint(uint64(*filter.FeedID), 10))
	}
	if filter.GroupID != nil {
		from = "groups"
		vals.Set("group_id", strconv.FormatUint(uint64(*filter.GroupID), 10))
	}
	if filter.Keyword != "" {
		from = "search"
		vals.Set("keyword", filter.Keyword)
	}
	if filter.Page > 0 {
		vals.Set("page", strconv.Itoa(filter.Page))
	}
	if filter.PageSize > 0 {
		vals.Set("page_size", strconv.Itoa(filter.PageSize))
	}
	vals.Set("from", from)
	return fmt.Sprintf("/items/%d?%s", itemID, vals.Encode())
}

func buildPagination(filter ListFilter, listPath string, total int) PaginationView {
	pageSize := defaultInt(filter.PageSize, 10)
	totalPages := total / pageSize
	if total%pageSize != 0 {
		totalPages++
	}
	if totalPages <= 1 {
		return PaginationView{}
	}

	current := maxInt(1, filter.Page)
	pages := buildPageNumbers(current, totalPages)
	links := make([]PageLink, 0, len(pages))
	for _, page := range pages {
		if page == 0 {
			links = append(links, PageLink{IsEllipsis: true})
			continue
		}
		url := buildListURL(listPath, filter, map[string]string{"page": strconv.Itoa(page)})
		links = append(links, PageLink{Label: strconv.Itoa(page), URL: url, Active: page == current})
	}

	prevURL := ""
	if current > 1 {
		prevURL = buildListURL(listPath, filter, map[string]string{"page": strconv.Itoa(current - 1)})
	}
	nextURL := ""
	if current < totalPages {
		nextURL = buildListURL(listPath, filter, map[string]string{"page": strconv.Itoa(current + 1)})
	}

	return PaginationView{
		Show:         true,
		Pages:        links,
		PrevURL:      prevURL,
		NextURL:      nextURL,
		PrevDisabled: current == 1,
		NextDisabled: current == totalPages,
	}
}

func buildPageNumbers(current, total int) []int {
	if total <= 7 {
		pages := make([]int, 0, total)
		for i := 1; i <= total; i++ {
			pages = append(pages, i)
		}
		return pages
	}

	pages := []int{1}
	if current > 3 {
		pages = append(pages, 0)
	}
	start := maxInt(2, current-1)
	end := minInt(total-1, current+1)
	for i := start; i <= end; i++ {
		pages = append(pages, i)
	}
	if current < total-2 {
		pages = append(pages, 0)
	}
	pages = append(pages, total)
	return pages
}

func buildListURL(path string, filter ListFilter, overrides map[string]string) string {
	vals := url.Values{}
	if filter.Page > 0 {
		vals.Set("page", strconv.Itoa(filter.Page))
	}
	if filter.PageSize > 0 {
		vals.Set("page_size", strconv.Itoa(filter.PageSize))
	}
	if filter.Keyword != "" {
		vals.Set("keyword", filter.Keyword)
	}
	if filter.FeedID != nil {
		vals.Set("feed_id", strconv.FormatUint(uint64(*filter.FeedID), 10))
	}
	if filter.GroupID != nil {
		vals.Set("group_id", strconv.FormatUint(uint64(*filter.GroupID), 10))
	}
	if filter.Unread != nil {
		vals.Set("unread", strconv.FormatBool(*filter.Unread))
	}
	if filter.Bookmark != nil {
		vals.Set("bookmark", strconv.FormatBool(*filter.Bookmark))
	}
	for k, v := range overrides {
		if v == "" {
			vals.Del(k)
			continue
		}
		vals.Set(k, v)
	}
	if len(vals) == 0 {
		return path
	}
	return path + "?" + vals.Encode()
}

func buildFilterView(filter ListFilter) FilterView {
	fields := []Field{}
	if filter.Keyword != "" {
		fields = append(fields, Field{Name: "keyword", Value: filter.Keyword})
	}
	if filter.FeedID != nil {
		fields = append(fields, Field{Name: "feed_id", Value: strconv.FormatUint(uint64(*filter.FeedID), 10)})
	}
	if filter.GroupID != nil {
		fields = append(fields, Field{Name: "group_id", Value: strconv.FormatUint(uint64(*filter.GroupID), 10)})
	}
	if filter.Unread != nil {
		fields = append(fields, Field{Name: "unread", Value: strconv.FormatBool(*filter.Unread)})
	}
	if filter.Bookmark != nil {
		fields = append(fields, Field{Name: "bookmark", Value: strconv.FormatBool(*filter.Bookmark)})
	}
	fields = append(fields, Field{Name: "page", Value: strconv.Itoa(filter.Page)})
	return FilterView{Page: filter.Page, PageSize: filter.PageSize, HiddenFields: fields}
}

func unreadLabel(unread *bool) string {
	if unread == nil || *unread {
		return "Mark read"
	}
	return "Mark unread"
}

func bookmarkLabel(bookmark *bool) string {
	if bookmark != nil && *bookmark {
		return "Unbookmark"
	}
	return "Bookmark"
}

func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	diff := time.Since(t)
	if diff < 0 {
		return "?"
	}
	hours := int(diff.Hours())
	days := hours / 24
	months := days / 30
	years := days / 365
	if years > 0 {
		return strconv.Itoa(years) + "y"
	}
	if months > 0 {
		return strconv.Itoa(months) + "m"
	}
	if days > 0 {
		return strconv.Itoa(days) + "d"
	}
	if hours > 0 {
		return strconv.Itoa(hours) + "h"
	}
	return "now"
}

func formatDateTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Local().Format(time.RFC1123)
}

func renderItemContent(content, baseLink string) template.HTML {
	policy := bluemonday.UGCPolicy()
	clean := policy.Sanitize(content)
	clean = rewriteRelativeLinks(clean, baseLink)
	clean = wrapTables(clean)
	clean = embedYouTube(clean, baseLink)
	return template.HTML(clean)
}

func rewriteRelativeLinks(content, baseLink string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content
	}

	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		s.RemoveAttr("class")
		s.RemoveAttr("style")
	})

	attrs := map[string][]string{
		"a":      {"href"},
		"img":    {"src"},
		"audio":  {"src"},
		"source": {"src"},
		"video":  {"src"},
		"embed":  {"src"},
		"object": {"data"},
	}

	for tag, list := range attrs {
		doc.Find(tag).Each(func(_ int, s *goquery.Selection) {
			for _, attr := range list {
				if val, exists := s.Attr(attr); exists {
					if abs := tryAbsURL(val, baseLink); abs != "" {
						s.SetAttr(attr, abs)
					}
				}
			}
		})
	}

	html, err := doc.Find("body").Html()
	if err != nil {
		return content
	}
	return html
}

func wrapTables(content string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content
	}
	doc.Find("table").Each(func(_ int, s *goquery.Selection) {
		s.WrapHtml("<div class=\"overflow-x-auto\"></div>")
	})
	html, err := doc.Find("body").Html()
	if err != nil {
		return content
	}
	return html
}

func embedYouTube(content, link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return content
	}
	host := parsed.Hostname()
	if !(strings.HasSuffix(host, "youtube.com") || strings.HasSuffix(host, "youtu.be")) {
		return content
	}
	videoID := parsed.Query().Get("v")
	if videoID == "" {
		return content
	}
	iframe := fmt.Sprintf(`<iframe style="aspect-ratio: 16 / 9; width: 100%% !important;" src="http://www.youtube.com/embed/%s" title="YouTube video player" frameborder="0" allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share" referrerpolicy="strict-origin-when-cross-origin" allowfullscreen></iframe>`, videoID)
	return iframe + content
}

func tryAbsURL(raw, base string) string {
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.IsAbs() {
		return parsed.String()
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return raw
	}
	return baseURL.ResolveReference(parsed).String()
}

func faviconURL(link string) string {
	parsed, err := url.Parse(link)
	if err != nil {
		return ""
	}
	host := parsed.Hostname()
	if strings.Contains(host, "rsshub") {
		for prefix, mapped := range rssHubMap {
			if strings.HasPrefix(parsed.Path, prefix) {
				host = mapped
				break
			}
		}
	}
	return "https://www.google.com/s2/favicons?sz=32&domain=" + host
}

func parseOPML(r io.Reader) ([]OPMLGroup, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	body := doc.Find("body").First()
	if body.Length() == 0 {
		return nil, errors.New("invalid OPML file")
	}

	groups := map[string]*OPMLGroup{}
	defaultGroup := &OPMLGroup{Name: "Default"}
	groups["Default"] = defaultGroup

	var dfs func(parent *OPMLGroup, sel *goquery.Selection)
	dfs = func(parent *OPMLGroup, sel *goquery.Selection) {
		if goquery.NodeName(sel) != "outline" {
			return
		}
		typeAttr, _ := sel.Attr("type")
		if strings.EqualFold(typeAttr, "rss") {
			if parent == nil {
				parent = defaultGroup
			}
			name := attrOrEmpty(sel, "title")
			if name == "" {
				name = attrOrEmpty(sel, "text")
			}
			link := attrOrEmpty(sel, "xmlurl")
			if link == "" {
				link = attrOrEmpty(sel, "htmlurl")
			}
			parent.Feeds = append(parent.Feeds, OPMLFeed{Name: name, Link: link})
			return
		}
		if sel.Children().Length() == 0 {
			return
		}
		name := attrOrEmpty(sel, "text")
		if name == "" {
			name = attrOrEmpty(sel, "title")
		}
		if name == "" {
			name = "Group"
		}
		groupName := name
		if parent != nil {
			groupName = parent.Name + "/" + name
		}
		group, ok := groups[groupName]
		if !ok {
			group = &OPMLGroup{Name: groupName}
			groups[groupName] = group
		}
		sel.Children().Each(func(_ int, child *goquery.Selection) {
			dfs(group, child)
		})
	}

	body.Children().Each(func(_ int, child *goquery.Selection) {
		dfs(nil, child)
	})

	result := make([]OPMLGroup, 0, len(groups))
	for _, group := range groups {
		if len(group.Feeds) == 0 {
			continue
		}
		result = append(result, *group)
	}

	return result, nil
}

func buildOPML(groups []OPMLGroup) string {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	buf.WriteString(`<opml version="1.0"><head><title>Feeds exported from Fusion</title></head><body>`)
	for _, group := range groups {
		buf.WriteString(`<outline text="`)
		buf.WriteString(escapeXML(group.Name))
		buf.WriteString(`" title="`)
		buf.WriteString(escapeXML(group.Name))
		buf.WriteString(`">`)
		for _, feed := range group.Feeds {
			buf.WriteString(`<outline type="rss" text="`)
			buf.WriteString(escapeXML(feed.Name))
			buf.WriteString(`" title="`)
			buf.WriteString(escapeXML(feed.Name))
			buf.WriteString(`" xmlUrl="`)
			buf.WriteString(escapeXML(feed.Link))
			buf.WriteString(`" htmlUrl="`)
			buf.WriteString(escapeXML(feed.Link))
			buf.WriteString(`" />`)
		}
		buf.WriteString(`</outline>`)
	}
	buf.WriteString(`</body></opml>`)
	return buf.String()
}

func escapeXML(input string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(input)
}

func sortedGroups(groupMap map[uint]*OPMLGroup) []OPMLGroup {
	groups := make([]OPMLGroup, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups
}

func findGroupID(groups []*GroupView, name string) uint {
	for _, group := range groups {
		if group.Name == name {
			return group.ID
		}
	}
	return 0
}

func attrOrEmpty(sel *goquery.Selection, key string) string {
	if val, ok := sel.Attr(key); ok {
		return val
	}
	return ""
}

func optionalString(val string) *string {
	if val == "" {
		return nil
	}
	return &val
}

func optionalUint(val uint) *uint {
	if val == 0 {
		return nil
	}
	return &val
}

func optionalBool(val bool) *bool {
	return &val
}

func derefString(val *string, fallback string) string {
	if val == nil {
		return fallback
	}
	return *val
}

func derefBool(val *bool, fallback bool) bool {
	if val == nil {
		return fallback
	}
	return *val
}

func derefTime(val *time.Time) time.Time {
	if val == nil {
		return time.Time{}
	}
	return *val
}

func derefInt(val *int, fallback int) int {
	if val == nil {
		return fallback
	}
	return *val
}

func defaultInt(val, fallback int) int {
	if val == 0 {
		return fallback
	}
	return val
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func groupViewsFromForms(forms []*server.GroupForm) []*GroupView {
	views := make([]*GroupView, 0, len(forms))
	for _, group := range forms {
		views = append(views, &GroupView{
			ID:   group.ID,
			Name: derefString(group.Name, "Group"),
		})
	}
	return views
}

func validityViewsFromItems(items []server.ValidityItem) []ValidityView {
	views := make([]ValidityView, 0, len(items))
	for _, item := range items {
		views = append(views, ValidityView{
			Title: derefString(item.Title, ""),
			Link:  derefString(item.Link, ""),
		})
	}
	return views
}

func itemViewFromList(item *server.ItemForm) ItemView {
	return ItemView{
		ID:       item.ID,
		Title:    derefString(item.Title, ""),
		Link:     derefString(item.Link, ""),
		Unread:   derefBool(item.Unread, true),
		Bookmark: derefBool(item.Bookmark, false),
		PubDate:  derefTime(item.PubDate),
		Feed: ItemFeedView{
			ID:   item.Feed.ID,
			Name: derefString(item.Feed.Name, ""),
			Link: derefString(item.Feed.Link, ""),
		},
	}
}

func itemViewFromGet(item *server.RespItemGet) ItemView {
	return ItemView{
		ID:       item.ID,
		Title:    derefString(item.Title, ""),
		Link:     derefString(item.Link, ""),
		Unread:   derefBool(item.Unread, true),
		Bookmark: derefBool(item.Bookmark, false),
		PubDate:  derefTime(item.PubDate),
		Feed: ItemFeedView{
			ID:   item.Feed.ID,
			Name: derefString(item.Feed.Name, ""),
			Link: derefString(item.Feed.Link, ""),
		},
	}
}

func isHTMX(c echo.Context) bool {
	return strings.EqualFold(c.Request().Header.Get("HX-Request"), "true")
}
