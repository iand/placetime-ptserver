/*
  This is free and unencumbered software released into the public domain. For more
  information, see <http://unlicense.org/> or the accompanying UNLICENSE file.
*/

// Finds the primary image featured on a webpage
package main

import (
	"cgl.tideland.biz/applog"
	"crypto/md5"
	"fmt"
	"github.com/iand/salience"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"image/png"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
)

type ImageInfo struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Url    string `json:"url"`
}

type ImageData struct {
	ImageInfo
	Img  image.Image
	Area int
}

type DetectionResult struct {
	Title      string      `json:"title"`
	Url        string      `json:"url"`
	Images     []string    `json:"images,omitempty"`
	Alternates []ImageInfo `json:"alternates,omitempty"`
	Media      string      `json:"media"`
	BestImage  string      `json:"bestImage"`
}

var titleRegexes = []string{
	`<meta property="og:title" content="([^"]+)">`,
	`<meta property="twitter:title" content="([^"]+)">`,
	`<title>([^<]+)</title>`,
}

func DetectUrl(url string, selectBest bool) (*DetectionResult, error) {
	mediaUrl, title, imageUrls, err := FindMedia(url)

	if err != nil {
		return nil, err
	}

	var data DetectionResult

	var bestImageFilename string

	if selectBest {
		best, images, err := SelectBestImage(url, imageUrls)

		if best.Img == nil || err != nil {
			return nil, err
		}

		imgOut := salience.Crop(best.Img, 460, 160)

		hasher := md5.New()
		io.WriteString(hasher, url)
		id := fmt.Sprintf("%x", hasher.Sum(nil))

		bestImageFilename = fmt.Sprintf("%s.png", id)

		foutName := path.Join(config.Image.Path, bestImageFilename)

		fout, err := os.OpenFile(foutName, os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			return nil, err
		}

		if err = png.Encode(fout, imgOut); err != nil {
			return nil, err
		}

		filteredImages := make([]ImageInfo, 0)
		for _, i := range images {
			if i.Url != best.Url {
				filteredImages = append(filteredImages, i)
			}
		}

		data = DetectionResult{
			Title:      title,
			Url:        url,
			Alternates: filteredImages,
			Media:      mediaUrl,
			BestImage:  bestImageFilename,
		}

	} else {

		data = DetectionResult{
			Title:     title,
			Url:       url,
			Images:    imageUrls,
			Media:     mediaUrl,
			BestImage: bestImageFilename,
		}
	}

	return &data, nil

}

// Look for the image that best represents the given page and also
// a url for any embedded media
func PickImage(pageUrl string) (image.Image, error) {
	var currentBest ImageData

	_, _, imageUrls, err := FindMedia(pageUrl)
	if err != nil {
		return nil, err
	}

	currentBest, _ = selectBest(imageUrls, currentBest)

	if currentBest.Img != nil {
		return currentBest.Img, nil
	}

	return image.NewRGBA(image.Rect(0, 0, 50, 50)), nil
}

func FindMedia(pageUrl string) (mediaUrl string, title string, imageUrls []string, err error) {

	base, err := url.Parse(pageUrl)
	if err != nil {
		return "", "", imageUrls, err
	}

	resp, err := http.Get(pageUrl)
	if err != nil {
		return "", "", imageUrls, err
	}

	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", imageUrls, err
	}

	title = cleanTitle(firstMatch(content, titleRegexes))

	seen := make(map[string]bool, 0)

	for _, url := range findYoutubeImages(content, base) {
		if _, exists := seen[url]; !exists {
			imageUrls = append(imageUrls, url)
			seen[url] = true
		}
	}

	for _, url := range findImageUrls(content, base) {
		if _, exists := seen[url]; !exists {
			imageUrls = append(imageUrls, url)
			seen[url] = true
		}
	}

	mediaUrl = detectMedia(content, base)

	return mediaUrl, title, imageUrls, err
}

func SelectBestImage(pageUrl string, imageUrls []string) (ImageData, []ImageInfo, error) {
	var currentBest ImageData
	var images []ImageInfo

	currentBest, images = selectBest(imageUrls, currentBest)

	if currentBest.Img != nil {
		return currentBest, images, nil
	}

	return ImageData{Img: image.NewRGBA(image.Rect(0, 0, 50, 50)), Area: 2500, ImageInfo: ImageInfo{Width: 50, Height: 50}}, images, nil
}

func resolveUrl(href string, base *url.URL) string {
	urlRef, err := url.Parse(href)
	if err != nil {
		return ""
	}

	srcUrl := base.ResolveReference(urlRef)
	return srcUrl.String()

}

func selectBest(urls []string, currentBest ImageData) (ImageData, []ImageInfo) {

	images := make([]ImageInfo, 0)
	urlchan := make(chan string, len(urls))
	results := make(chan *ImageData, 0)
	quit := make(chan bool, 0)

	go fetchImage(urlchan, results, quit)
	go fetchImage(urlchan, results, quit)
	go fetchImage(urlchan, results, quit)
	go fetchImage(urlchan, results, quit)

	for _, url := range urls {
		urlchan <- url
	}

	timeout := time.After(time.Duration(500) * time.Millisecond)
	for i := 0; i < len(urls); i++ {
		select {
		case result := <-results:
			if result != nil {
				images = append(images, ImageInfo{Url: result.Url, Width: result.Width, Height: result.Height})

				sizeRatio := float64(result.Width) / float64(result.Height)
				if sizeRatio > 2 || sizeRatio < 0.5 {
					continue
				}

				area := result.Width * result.Height
				if area < 5000 {
					continue
				}

				if area > currentBest.Area {
					currentBest = *result
				}
			}
		case <-timeout:
			applog.Debugf("Search timed out")
			close(quit)
			return currentBest, images
		}
	}

	applog.Debugf("Loop complete")
	close(quit)

	return currentBest, images

}

func fetchImage(urls chan string, results chan *ImageData, quit chan bool) {

	for {
		select {
		case url := <-urls:
			applog.Debugf("Fetching image %s", url)
			imgResp, err := http.Get(url)
			if err != nil {
				applog.Errorf("Error fetching image from %s: %s", url, err.Error())
				results <- nil
				continue
			}
			defer imgResp.Body.Close()
			img, _, err := image.Decode(imgResp.Body)
			if err != nil {
				applog.Errorf("Error decoding image from %s: %s", url, err.Error())
				results <- nil
				continue
			}
			r := img.Bounds()

			results <- &ImageData{
				ImageInfo: ImageInfo{
					Url:    url,
					Width:  (r.Max.X - r.Min.X),
					Height: (r.Max.Y - r.Min.Y),
				},
				Img:  img,
				Area: (r.Max.X - r.Min.X) * (r.Max.Y - r.Min.Y),
			}

		case <-quit:
			applog.Debugf("Image fetcher quitting")
			return
		}
	}
}

func findImageUrls(content []byte, base *url.URL) []string {

	relist := []string{
		`<img[^>]+src="([^"]+)"`,
		`<img[^>]+src='([^']+)'`,
	}

	var urls []string

	for _, match := range allMatches(content, relist) {
		srcUrl := resolveUrl(match, base)
		urls = append(urls, srcUrl)
	}

	return urls

}

func findYoutubeImages(content []byte, base *url.URL) []string {
	var urls []string

	re1, err := regexp.Compile(`//www.youtube.com/watch\?v=([A-Za-z0-9-]+)`)
	if err != nil {
		return urls
	}

	re2, err := regexp.Compile(`//www.youtube.com/embed/([A-Za-z0-9-]+)`)
	if err != nil {
		return urls
	}

	matches := re1.FindAllSubmatch(content, -1)
	for _, match := range matches {
		key := string(match[1])

		url := fmt.Sprintf("https://img.youtube.com/vi/%s/0.jpg", key)

		urls = append(urls, url)
	}

	matches = re2.FindAllSubmatch(content, -1)
	for _, match := range matches {
		key := string(match[1])

		url := fmt.Sprintf("https://img.youtube.com/vi/%s/0.jpg", key)

		urls = append(urls, url)
	}

	return urls

}

func detectMedia(content []byte, base *url.URL) string {

	switch {
	case base.Host == "youtube.com" || base.Host == "www.youtube.com":
		re, err := regexp.Compile(`<meta property="og:url" content="([^"]+)">`)
		if err != nil {
			return ""
		}

		matches := re.FindAllSubmatch(content, -1)
		if len(matches) > 0 {
			return string(matches[0][1])
		}

	}

	return ""
}

func firstMatch(content []byte, regexes []string) string {

	for _, r := range regexes {
		re, err := regexp.Compile(r)
		if err != nil {
			continue
		}

		matches := re.FindAllSubmatch(content, -1)
		if len(matches) > 0 {
			return string(matches[0][1])
		}

	}

	return ""

}

func allMatches(content []byte, regexes []string) []string {
	results := make([]string, 0)

	for _, r := range regexes {
		re, err := regexp.Compile(r)
		if err != nil {
			continue
		}

		matches := re.FindAllSubmatch(content, -1)
		for _, match := range matches {
			results = append(results, string(match[1]))
		}

	}

	return results

}

func cleanTitle(title string) string {
	if pos := strings.Index(title, " |"); pos != -1 {
		title = title[:pos]
	}

	if pos := strings.Index(title, " —"); pos != -1 {
		title = title[:pos]
	}

	if pos := strings.Index(title, " - "); pos != -1 {
		title = title[:pos]
	}

	if pos := strings.Index(title, "&nbsp;-&nbsp;"); pos != -1 {
		title = title[:pos]
	}

	title = strings.Trim(title, " ")
	return title
}
