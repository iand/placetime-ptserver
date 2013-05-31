package main

import (
	"cgl.tideland.biz/applog"
	"crypto/md5"
	"fmt"
	"github.com/iand/feedparser"
	"github.com/placetime/datastore"
	"io"
	"net/http"
	"net/url"
	"time"
)

type SearchResults struct {
	Results interface{} `json:"results"`
}

type ProfileSearchResults []*datastore.Profile

type ItemSearchResults []*datastore.Item

type SearchFunc func(srch string, pid datastore.PidType) ItemSearchResults

func ProfileSearch(srch string) SearchResults {
	s := datastore.NewRedisStore()
	defer s.Close()

	plist, _ := s.FindProfilesBySubstring(srch)

	return SearchResults{Results: plist}
}

func ItemSearch(srch string, pid datastore.PidType) SearchResults {
	searches := []SearchFunc{
		searchYoutubeVidoes,
		searchEventfulEvents,
	}

	return MultiplexedSearch(srch, pid, searches)
}

func VideoSearch(srch string, pid datastore.PidType) SearchResults {

	searches := []SearchFunc{
		searchYoutubeVidoes,
	}

	return MultiplexedSearch(srch, pid, searches)
}

func AudioSearch(srch string, pid datastore.PidType) SearchResults {

	searches := []SearchFunc{}

	return MultiplexedSearch(srch, pid, searches)
}

func EventSearch(srch string, pid datastore.PidType) SearchResults {

	searches := []SearchFunc{
		searchEventfulEvents,
	}

	return MultiplexedSearch(srch, pid, searches)
}

func MultiplexedSearch(srch string, pid datastore.PidType, searches []SearchFunc) SearchResults {
	results := make(ItemSearchResults, 0)

	items := make(chan ItemSearchResults)

	for _, f := range searches {
		go func() { items <- f(srch, pid) }()
	}

	lists := make([]ItemSearchResults, 0)

	timeout := time.After(time.Duration(config.Search.Timeout) * time.Millisecond)
	for i := 0; i < len(searches); i++ {
		select {
		case result := <-items:
			lists = append(lists, result)
		case <-timeout:
			applog.Debugf("Search timed out")
			break
		}
	}

	i := 0
	added := true
	for added {
		added = false
		for _, list := range lists {
			if i < len(list) {
				results = append(results, list[i])
				added = true
			}
		}
		i++
	}

	return SearchResults{Results: results}

}

func searchYoutubeChannels(srch string, pid datastore.PidType) ProfileSearchResults {
	plist := make([]*datastore.Profile, 0)

	url := fmt.Sprintf("https://gdata.youtube.com/feeds/api/channels?q=%s&v=2", srch)
	resp, err := http.Get(url)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return plist
	}

	defer resp.Body.Close()

	feed, err := feedparser.NewFeed(resp.Body)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return plist

	}

	_ = feed
	//items := itemsFromFeed("fakepid", feed)

	// for _, item := range items {
	// 	log.Printf("%s", item)

	// 	//		s.AddItem(item.Pid, time.Unix(item.Event, 0), item.Text, item.Link, item.Image, item.Id)
	// }

	return plist

}

func searchYoutubeVidoes(srch string, pid datastore.PidType) ItemSearchResults {
	items := make([]*datastore.Item, 0)

	url := fmt.Sprintf("https://gdata.youtube.com/feeds/api/videos?v=2&q=%s", url.QueryEscape(srch))

	applog.Debugf("Fetching %s", url)

	resp, err := http.Get(url)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return items
	}

	defer resp.Body.Close()

	feed, err := feedparser.NewFeed(resp.Body)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return items

	}

	if feed != nil {
		for _, item := range feed.Items {
			hasher := md5.New()
			io.WriteString(hasher, item.Id)
			id := datastore.ItemIdType(fmt.Sprintf("%x", hasher.Sum(nil)))
			items = append(items, &datastore.Item{Id: id, Pid: pid, Event: 0, Text: item.Title, Link: item.Link, Media: "video", Image: item.Image})
		}
	}
	return items

}

func searchEventfulEvents(srch string, pid datastore.PidType) ItemSearchResults {
	items := make([]*datastore.Item, 0)

	url := fmt.Sprintf("http://api.eventful.com/rest/events/rss?app_key=%s&date=Future&keywords=%s", url.QueryEscape(config.Search.Eventful.AppKey), url.QueryEscape(srch))

	applog.Debugf("Fetching %s", url)

	resp, err := http.Get(url)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return items
	}

	defer resp.Body.Close()
	applog.Debugf("Response %s", resp.Status)

	feed, err := feedparser.NewFeed(resp.Body)

	if err != nil {
		applog.Errorf("Fetch of feed got http error  %s", err.Error())
		return items

	}

	if feed != nil {
		applog.Debugf("Received %d items from eventful matching %s", len(feed.Items), srch)
		for _, item := range feed.Items {
			hasher := md5.New()
			io.WriteString(hasher, item.Id)
			id := datastore.ItemIdType(fmt.Sprintf("%x", hasher.Sum(nil)))
			items = append(items, &datastore.Item{Id: id, Pid: pid, Event: datastore.FakeEventPrecision(item.When), Text: item.Title, Link: item.Link, Media: "text", Image: item.Image})
		}
	}
	return items

}
