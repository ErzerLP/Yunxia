package rss

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	appsvc "yunxia/internal/application/service"
)

// Fetcher 使用 HTTP 拉取并解析 RSS/Atom feed。
type Fetcher struct {
	client *http.Client
}

// NewFetcher 创建 RSS fetcher。
func NewFetcher() *Fetcher {
	return &Fetcher{client: &http.Client{Timeout: 30 * time.Second}}
}

// Fetch 拉取并解析 feed 条目。
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) ([]appsvc.RSSFetchedItem, error) {
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Yunxia RSS/1.0")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("rss fetch status %d", response.StatusCode)
	}

	var feed xmlFeed
	if err := xml.NewDecoder(response.Body).Decode(&feed); err != nil {
		return nil, err
	}
	items := make([]appsvc.RSSFetchedItem, 0, len(feed.Channel.Items)+len(feed.Entries))
	for _, item := range feed.Channel.Items {
		items = append(items, convertRSSItem(item))
	}
	for _, entry := range feed.Entries {
		items = append(items, convertAtomEntry(entry))
	}
	return items, nil
}

type xmlFeed struct {
	XMLName xml.Name
	Channel struct {
		Items []xmlRSSItem `xml:"item"`
	} `xml:"channel"`
	Entries []xmlAtomEntry `xml:"entry"`
}

type xmlRSSItem struct {
	Title      string         `xml:"title"`
	Link       string         `xml:"link"`
	GUID       string         `xml:"guid"`
	PubDate    string         `xml:"pubDate"`
	Enclosures []xmlEnclosure `xml:"enclosure"`
}

type xmlEnclosure struct {
	URL string `xml:"url,attr"`
}

type xmlAtomEntry struct {
	Title     string        `xml:"title"`
	ID        string        `xml:"id"`
	Updated   string        `xml:"updated"`
	Published string        `xml:"published"`
	Links     []xmlAtomLink `xml:"link"`
}

type xmlAtomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func convertRSSItem(item xmlRSSItem) appsvc.RSSFetchedItem {
	enclosures := make([]string, 0, len(item.Enclosures))
	for _, enclosure := range item.Enclosures {
		if strings.TrimSpace(enclosure.URL) != "" {
			enclosures = append(enclosures, strings.TrimSpace(enclosure.URL))
		}
	}
	return appsvc.RSSFetchedItem{
		Title:       strings.TrimSpace(item.Title),
		Link:        strings.TrimSpace(item.Link),
		GUID:        strings.TrimSpace(item.GUID),
		PublishedAt: parseFeedTime(item.PubDate),
		Enclosures:  enclosures,
	}
}

func convertAtomEntry(entry xmlAtomEntry) appsvc.RSSFetchedItem {
	links := make([]string, 0, len(entry.Links))
	primary := ""
	for _, link := range entry.Links {
		href := strings.TrimSpace(link.Href)
		if href == "" {
			continue
		}
		if primary == "" && (link.Rel == "" || link.Rel == "alternate") {
			primary = href
		}
		links = append(links, href)
	}
	published := parseFeedTime(entry.Published)
	if published == nil {
		published = parseFeedTime(entry.Updated)
	}
	return appsvc.RSSFetchedItem{
		Title:       strings.TrimSpace(entry.Title),
		Link:        primary,
		GUID:        strings.TrimSpace(entry.ID),
		PublishedAt: published,
		Enclosures:  links,
	}
}

func parseFeedTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	layouts := []string{time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC3339Nano}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return &parsed
		}
	}
	return nil
}
