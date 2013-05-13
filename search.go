package main

import (
	"crypto/md5"
	"fmt"
	"github.com/iand/feedparser"
	"github.com/placetime/datastore"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

type ProfileSearchResults []*datastore.Profile

type ItemSearchResults []*datastore.Item

func ProfileSearch(srch string) ProfileSearchResults {
	s := datastore.NewRedisStore()
	defer s.Close()

	plist, _ := s.FindProfilesBySubstring(srch)

	return plist
}

func MultiplexedSearch(srch string) ItemSearchResults {
	results := make(ItemSearchResults, 0)

	items := make(chan ItemSearchResults)

	go func() { items <- searchYoutubeVidoes(srch) }()

	timeout := time.After(5000 * time.Millisecond)
	for i := 0; i < 1; i++ {
		select {
		case result := <-items:
			results = append(results, result...)
		case <-timeout:
			log.Println("Search timed out")
			break
		}
	}

	return results

}

func searchYoutubeChannels(srch string) ProfileSearchResults {
	plist := make([]*datastore.Profile, 0)

	url := fmt.Sprintf("https://gdata.youtube.com/feeds/api/channels?q=%s&v=2", srch)
	resp, err := http.Get(url)

	if err != nil {
		log.Printf("Fetch of feed got http error  %s", err.Error())
		return plist
	}

	defer resp.Body.Close()

	feed, err := feedparser.NewFeed(resp.Body)

	if err != nil {
		log.Printf("Fetch of feed got http error  %s", err.Error())
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

func searchYoutubeVidoes(srch string) ItemSearchResults {
	items := make([]*datastore.Item, 0)

	url := fmt.Sprintf("https://gdata.youtube.com/feeds/api/videos?v=2&q=%s", url.QueryEscape(srch))

	log.Printf("Fetching %s", url)

	resp, err := http.Get(url)

	if err != nil {
		log.Printf("Fetch of feed got http error  %s", err.Error())
		return items
	}

	defer resp.Body.Close()
	log.Printf("Response %s", resp.Status)

	feed, err := feedparser.NewFeed(resp.Body)

	if err != nil {
		log.Printf("Fetch of feed got http error  %s", err.Error())
		return items

	}

	if feed != nil {
		for _, item := range feed.Items {
			hasher := md5.New()
			io.WriteString(hasher, item.Id)
			id := fmt.Sprintf("%x", hasher.Sum(nil))
			items = append(items, &datastore.Item{Id: id, Pid: "youtube", Event: 0, Text: item.Title, Link: item.Link, Media: "video", Image: item.Image})
		}
	}
	return items

}
