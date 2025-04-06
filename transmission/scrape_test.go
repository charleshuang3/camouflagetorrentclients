package transmission

import (
	"net/url"
	"testing"

	"net/http"
	"net/http/httptest"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

func TestScrapeURL(t *testing.T) {
	infoHash := "1234567890abcdefghij" // 20 bytes
	escapedInfoHash := url.QueryEscape(infoHash)

	testCases := []struct {
		name                string
		announceURLStr      string
		infoHash            string
		privateTrackerQuery string
		expectedScrapeURL   string // Expected URL string, or empty if nil expected
	}{
		{
			name:                "HTTP announce URL",
			announceURLStr:      "http://tracker.example.com/announce",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "http://tracker.example.com/scrape?info_hash=" + escapedInfoHash,
		},
		{
			name:                "HTTPS announce URL",
			announceURLStr:      "https://secure.tracker.org:8080/announce",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "https://secure.tracker.org:8080/scrape?info_hash=" + escapedInfoHash,
		},
		{
			name:                "Announce URL with existing query",
			announceURLStr:      "http://tracker.example.com/announce?passkey=xyz",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "http://tracker.example.com/scrape?info_hash=" + escapedInfoHash, // Original query should be replaced
		},
		{
			name:                "Announce URL not ending in /announce",
			announceURLStr:      "http://tracker.example.com/announce_extra",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "", // Should return nil
		},
		{
			name:                "Announce URL path only /",
			announceURLStr:      "http://tracker.example.com/",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "", // Should return nil
		},
		{
			name:                "Announce URL no path",
			announceURLStr:      "http://tracker.example.com",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "", // Should return nil
		},
		{
			name:                "With private tracker query",
			announceURLStr:      "http://private.tracker/announce",
			infoHash:            infoHash,
			privateTrackerQuery: "passkey=abc&uid=123",
			expectedScrapeURL:   "http://private.tracker/scrape?passkey=abc&uid=123&info_hash=" + escapedInfoHash,
		},
		{
			name:                "Announce URL with complex path",
			announceURLStr:      "http://tracker.example.com/tracker/announce",
			infoHash:            infoHash,
			privateTrackerQuery: "",
			expectedScrapeURL:   "http://tracker.example.com/tracker/scrape?info_hash=" + escapedInfoHash,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			announceURL, err := url.Parse(tc.announceURLStr)
			if err != nil {
				t.Fatalf("Failed to parse announce URL '%s': %v", tc.announceURLStr, err)
			}

			actualURL := scrapeURL(announceURL, tc.infoHash, tc.privateTrackerQuery)

			if tc.expectedScrapeURL == "" {
				assert.Nil(t, actualURL, "Expected nil URL")
			} else {
				assert.NotNil(t, actualURL, "Expected non-nil URL")
				if actualURL != nil {
					expectedParsedURL, _ := url.Parse(tc.expectedScrapeURL)
					assert.Equal(t, expectedParsedURL.Scheme, actualURL.Scheme, "Scheme mismatch")
					assert.Equal(t, expectedParsedURL.Host, actualURL.Host, "Host mismatch")
					assert.Equal(t, expectedParsedURL.Path, actualURL.Path, "Path mismatch")
					assert.Equal(t, expectedParsedURL.Query(), actualURL.Query(), "Query mismatch")
					assert.Equal(t, tc.expectedScrapeURL, actualURL.String(), "Full URL string mismatch")
				}
			}
		})
	}
}

func TestScrapeTaskRun_Success(t *testing.T) {
	requestReceived := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "Expected GET request")
		assert.Contains(t, r.URL.Path, "/scrape", "Expected /scrape path")
		assert.Equal(t, "Transmission/4.0.6", r.Header.Get("User-Agent"), "Expected User-Agent header")
		assert.Equal(t, "*/*", r.Header.Get("Accept"), "Expected Accept header")
		assert.NotEmpty(t, r.Header.Get("Accept-Encoding"), "Expected Accept-Encoding header")
		assert.Equal(t, "test_info_hash", r.URL.Query().Get("info_hash"), "Expected info_hash query param")
		assert.Equal(t, "a_key", r.URL.Query().Get("auth"), "Expected auth query param")

		requestReceived <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL + "/announce")
	query := url.Values{}
	query.Add("info_hash", "wrong_hash")
	query.Add("auth", "wrong_key")
	serverURL.RawQuery = query.Encode()

	tr := &mimickTransmission{
		// Allow requests immediately for the test
		scrapeRateLimiter: rate.NewLimiter(rate.Inf, 1),
	}

	task := newScrapeTask(tr, serverURL, "test_info_hash", "auth=a_key")

	go task.run()

	select {
	case <-requestReceived:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for the mock server to receive the scrape request")
	}
}
