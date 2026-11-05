package walker

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestScrapeTTFB(t *testing.T) {
	const delay = 50 * time.Millisecond
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			// a slow redirect must not count towards the final response's ttfb
			time.Sleep(delay)
			http.Redirect(w, r, "/page", http.StatusFound)
			return
		}
		time.Sleep(delay)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(delay) // the body comes later, ttfb ends with the headers
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	baseURL, _ := url.Parse(server.URL)
	pool := newClientPool(1, "test", false)
	chanResult := make(chan scrapeResultAndClient, 1)
	go scrape(pool.clients[0], server.URL+"/start", baseURL, "", nil, nil, nil, chanResult)
	result := (<-chanResult).result

	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if len(result.Redirects) != 1 {
		t.Fatalf("expected one redirect, got %v", result.Redirects)
	}
	if result.TTFB < delay || result.TTFB >= 2*delay {
		t.Errorf("ttfb %v, expected between %v and %v", result.TTFB, delay, 2*delay)
	}
	if result.Duration < 2*delay {
		t.Errorf("duration %v should include the redirect", result.Duration)
	}
}
