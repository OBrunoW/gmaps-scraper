package gmaps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/gosom/scrapemate"
	"github.com/mxschmitt/playwright-go"
)

const reviewRPC = "qv9Egd"

type reviewEventPage interface {
	OnResponse(func(playwright.Response))
	OnRequestFinished(func(playwright.Request))
	RemoveListener(string, any)
}

type browserReviewCapture struct {
	mu         sync.Mutex
	pending    map[playwright.Request]playwright.Response
	finished   []playwright.Response
	page       reviewEventPage
	onResponse func(playwright.Response)
	onFinished func(playwright.Request)
	pages      [][]byte
	limited    bool
	ids        map[string]struct{}
}

func startReviewCapture(page scrapemate.BrowserPage) *browserReviewCapture {
	rawPage, ok := page.Unwrap().(reviewEventPage)
	if !ok {
		return nil
	}

	capture := &browserReviewCapture{
		page: rawPage, pending: make(map[playwright.Request]playwright.Response), ids: make(map[string]struct{}),
	}
	capture.onResponse = func(response playwright.Response) {
		if response.Status() != 200 || !isReviewResponseURL(response.URL()) {
			return
		}

		capture.mu.Lock()
		defer capture.mu.Unlock()

		capture.pending[response.Request()] = response
	}
	capture.onFinished = func(request playwright.Request) {
		capture.mu.Lock()
		defer capture.mu.Unlock()

		if response, exists := capture.pending[request]; exists {
			capture.finished = append(capture.finished, response)
			delete(capture.pending, request)
		}
	}
	rawPage.OnResponse(capture.onResponse)
	rawPage.OnRequestFinished(capture.onFinished)

	return capture
}

func (c *browserReviewCapture) close() {
	if c == nil {
		return
	}

	c.page.RemoveListener("response", c.onResponse)
	c.page.RemoveListener("requestfinished", c.onFinished)
}

func (c *browserReviewCapture) drain(ctx context.Context) {
	if c == nil {
		return
	}

	c.mu.Lock()
	finished := c.finished
	c.finished = nil
	c.mu.Unlock()

	for _, response := range finished {
		if ctx.Err() != nil {
			return
		}

		body, err := response.Body()
		if err != nil {
			continue
		}

		pages, limited, err := parseBrowserReviewResponse(body)
		if err != nil && len(pages) == 0 {
			continue
		}

		c.pages = append(c.pages, pages...)
		c.limited = c.limited || limited

		for _, page := range pages {
			reviews := extractReviews(page)
			for i := range reviews {
				review := &reviews[i]
				if review.ReviewID != "" {
					c.ids[review.ReviewID] = struct{}{}
				}
			}
		}
	}
}

func isReviewResponseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "www.google.com" || u.Path != "/maps/_/MapsWizUi/data/batchexecute" {
		return false
	}

	for _, rpc := range strings.Split(u.Query().Get("rpcids"), ",") {
		if rpc == reviewRPC || rpc == "/MapsUgcPostService.ListUgcPosts" {
			return true
		}
	}

	return false
}

func parseBrowserReviewResponse(body []byte) (pages [][]byte, limited bool, err error) {
	body = bytes.TrimSpace(body)
	body = bytes.TrimPrefix(body, []byte(")]}'"))
	decoder := json.NewDecoder(bytes.NewReader(body))

	for {
		var frame any
		if err := decoder.Decode(&frame); err != nil {
			if err == io.EOF {
				return pages, limited, nil
			}

			return pages, limited, fmt.Errorf("decode browser review frame: %w", err)
		}

		frames, ok := frame.([]any)
		if !ok {
			continue
		}

		for _, value := range frames {
			item, ok := value.([]any)
			if !ok || len(item) < 3 || item[0] != "wrb.fr" ||
				(item[1] != reviewRPC && item[1] != "/MapsUgcPostService.ListUgcPosts") {
				continue
			}

			payload, ok := item[2].(string)
			if !ok || !json.Valid([]byte(payload)) {
				continue
			}

			var data []any
			if err := json.Unmarshal([]byte(payload), &data); err != nil {
				continue
			}

			limited = limited || getNthElementAndCast[bool](data, 5) || getNthElementAndCast[bool](data, 6, 0)
			if len(getNthElementAndCast[[]any](data, 2)) > 0 {
				pages = append(pages, []byte(payload))
			}
		}
	}
}
