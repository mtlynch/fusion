package web

import (
	"html/template"
	"time"
)

type BasePageData struct {
	Title   string
	Sidebar SidebarView
	Flash   string
}

type ItemsListPageData struct {
	BasePageData
	HeaderTitle   string
	HeaderActions []ActionButton
	ShowSearch    bool
	ItemList      ItemListView
}

type SearchPageData struct {
	BasePageData
	Keyword  string
	ItemList ItemListView
}

type ItemDetailPageData struct {
	BasePageData
	Item           ItemDetailView
	ContentHTML    template.HTML
	PublishedAt    string
	PrevItemURL    string
	NextItemURL    string
	ReturnURL      string
	UnreadLabel    string
	BookmarkLabel  string
}

type FeedImportPageData struct {
	BasePageData
	Groups        []*GroupView
	ManualError   string
	ManualLink    string
	ManualName    string
	ManualProxy   string
	ManualGroupID uint
	ManualCandidates []ValidityView
	OPMLError     string
	OPMLLog       []OPMLLogEntry
}

type FeedSettingsPageData struct {
	BasePageData
	Feed   FeedSettingsView
	Groups []*GroupView
	Error  string
}

type SettingsPageData struct {
	BasePageData
	Groups []*GroupView
}

type SidebarView struct {
	SystemLinks     []NavLink
	Groups          []SidebarGroup
	Version         string
	ShowThemeToggle bool
	ThemeIsDark     bool
}

type SidebarGroup struct {
	ID    uint
	Name  string
	Open  bool
	Feeds []SidebarFeed
}

type SidebarFeed struct {
	ID          uint
	Name        string
	UnreadCount int
	FaviconURL  string
	Active      bool
	StatusClass string
}

type NavLink struct {
	Label  string
	URL    string
	Active bool
}

type ItemListView struct {
	Rows            []ItemRowView
	Pagination      PaginationView
	HighlightUnread bool
	EmptyMessage    string
	ListPath        string
	Filter          FilterView
	Loading         bool
}

type ItemRowView struct {
	ID             uint
	ItemURL        string
	Title          string
	Link           string
	FeedName       string
	FaviconURL     string
	TimeAgo        string
	DimTitle       bool
	UnreadLabel    string
	BookmarkLabel  string
	ReturnURL      string
	HighlightUnread bool
}

type PaginationView struct {
	Show         bool
	PrevURL      string
	NextURL      string
	PrevDisabled bool
	NextDisabled bool
	Pages        []PageLink
}

type PageLink struct {
	Label      string
	URL        string
	Active     bool
	IsEllipsis bool
}

type FilterView struct {
	Page        int
	PageSize    int
	HiddenFields []Field
}

type Field struct {
	Name  string
	Value string
}

type ActionButton struct {
	Label  string
	Method string
	URL    string
	Fields []Field
}

type ListFilter struct {
	Page     int
	PageSize int
	Keyword  string
	FeedID   *uint
	GroupID  *uint
	Unread   *bool
	Bookmark *bool
}

type ItemView struct {
	ID       uint
	Title    string
	Link     string
	Unread   bool
	Bookmark bool
	PubDate  time.Time
	Feed     ItemFeedView
}

type ItemDetailView struct {
	ID       uint
	Title    string
	Link     string
	Content  string
	Unread   bool
	Bookmark bool
	PubDate  *time.Time
	Feed     ItemFeedView
}

type ItemFeedView struct {
	ID   uint
	Name string
	Link string
}

type GroupView struct {
	ID   uint
	Name string
}

type FeedSettingsView struct {
	ID        uint
	Name      string
	Link      string
	GroupID   uint
	ReqProxy  string
	Suspended bool
}

type ValidityView struct {
	Title string
	Link  string
}

type OPMLGroup struct {
	Name  string
	Feeds []OPMLFeed
}

type OPMLFeed struct {
	Name string
	Link string
}

type OPMLLogEntry struct {
	Message string
	IsError bool
}
