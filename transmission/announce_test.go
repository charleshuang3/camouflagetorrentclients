package transmission

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/charleshuang3/camouflagetorrentclients/commons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTimeout = 15 * time.Second
	port        = 3456
)

func TestAnnounceRequest(t *testing.T) {
	testCase := []struct {
		name          string
		torrentFile   string
		infoHash      string
		totalTrackers int
		hasAuthQuery  bool
	}{
		{
			name:          "test-public-torrent",
			torrentFile:   "../test-torrents/test-public.torrent",
			infoHash:      "%A9%BFz%B1%BB%05%91%9A%23J5%13Y%95%14%89f%08_9",
			totalTrackers: 2,
			hasAuthQuery:  false,
		},
		{
			name:          "test-private-torrent",
			torrentFile:   "../test-torrents/test-private.torrent",
			infoHash:      "1%83%CA%D9%2B%93%5C%82%D2%0F%24%3A%88JDp%8B%3B%2B%22",
			totalTrackers: 1,
			hasAuthQuery:  true,
		},
	}

	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()

			var wg sync.WaitGroup

			// Setup test HTTP server
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Basic checks:
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, r.URL.Path, "/tracker/announce")

				// Header checks:
				assert.Len(t, r.Header, 4)
				assert.Equal(t, "deflate, gzip, br, zstd", r.Header.Get("Accept-Encoding"))
				assert.Equal(t, "Transmission/4.0.6", r.Header.Get("User-Agent"))
				assert.Equal(t, "*/*", r.Header.Get("Accept"))
				// [TODO]: transmission does not have this header.
				assert.Equal(t, "close", r.Header.Get("Connection"))

				// Query parameter checks:
				infoHash, err := url.QueryUnescape(tc.infoHash)
				require.NoError(t, err)

				q, err := commons.QueryParamsFromRawQueryStr(r.URL.RawQuery)
				require.NoError(t, err)
				if tc.hasAuthQuery {
					assert.Len(t, q, 12)
				} else {
					assert.Len(t, q, 11)
				}

				if tc.hasAuthQuery {
					assert.Equal(t, q[0], &commons.QueryParam{Name: "auth", Value: "123"})
				}

				beginIndex := 0
				if tc.hasAuthQuery {
					beginIndex = 1
				}

				assert.Equal(t, q[beginIndex+0], &commons.QueryParam{Name: "info_hash", Value: infoHash})

				assert.Equal(t, q[beginIndex+1].Name, "peer_id")
				assert.Len(t, q[beginIndex+1].Value, 20)
				assert.True(t, strings.HasPrefix(q[beginIndex+1].Value, transmissionV406Bep20))

				assert.Equal(t, q[beginIndex+2], &commons.QueryParam{Name: "port", Value: strconv.Itoa(port)})
				assert.Equal(t, q[beginIndex+3], &commons.QueryParam{Name: "uploaded", Value: "0"})
				assert.Equal(t, q[beginIndex+4], &commons.QueryParam{Name: "downloaded", Value: "0"})
				assert.Equal(t, q[beginIndex+5], &commons.QueryParam{Name: "left", Value: "7159086"})
				assert.Equal(t, q[beginIndex+6], &commons.QueryParam{Name: "numwant", Value: "80"})
				assert.Equal(t, q[beginIndex+7].Name, "key")
				assert.Len(t, q[beginIndex+7].Value, 8)
				assert.Equal(t, q[beginIndex+8], &commons.QueryParam{Name: "compact", Value: "1"})
				assert.Equal(t, q[beginIndex+9], &commons.QueryParam{Name: "supportcrypto", Value: "1"})
				assert.Equal(t, q[beginIndex+10], &commons.QueryParam{Name: "event", Value: "started"})

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("d8:intervali1800e5:peersld2:ip9:127.0.0.14:porti6881e6:peer id20:-TR3000-012345678901e")) // Example minimal tracker response

				// Signal that a request was processed
				wg.Done()
			}))
			defer ts.Close()

			cfg := torrent.NewDefaultClientConfig()
			cfg.DataDir = tempDir // Use the temp directory
			tr := New()
			cfg.HttpRequestDirector = tr.HttpRequestDirector
			cfg.TrackerDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Redirect all HTTP tracker requests to our test server
				return (&net.Dialer{}).DialContext(ctx, network, ts.URL[len("http://"):])
			}
			cfg.ListenPort = port

			c, err := torrent.NewClient(cfg)
			require.NoError(t, err)
			defer c.Close()

			mi, err := metainfo.LoadFromFile(tc.torrentFile)
			require.NoError(t, err)
			_, err = c.AddTorrent(mi)
			require.NoError(t, err)

			// Set the WaitGroup counter
			wg.Add(tc.totalTrackers)

			// Wait for all announce requests to be received by the test server or timeout
			waitChan := make(chan struct{})
			go func() {
				wg.Wait()
				close(waitChan)
			}()

			select {
			case <-waitChan:
				// All requests received
			case <-time.After(testTimeout):
				t.Fatalf("timed out waiting for %d announce requests", tc.totalTrackers)
			}
		})
	}
}

func TestCreatePerTorrent(t *testing.T) {
	runs := 10 // Run multiple times to check randomness
	previousPeerIDs := make(map[string]bool)
	previousKeys := make(map[string]bool)

	for i := 0; i < runs; i++ {
		pt := createPerTorrent()
		require.NotNil(t, pt, "createPerTorrent returned nil on run %d", i+1)

		// Peer ID checks
		assert.Len(t, pt.peerID, 20, "Peer ID length mismatch on run %d", i+1)
		assert.True(t, strings.HasPrefix(pt.peerID, transmissionV406Bep20), "Peer ID prefix mismatch on run %d", i+1)
		randomPartPeerID := pt.peerID[len(transmissionV406Bep20):]
		assert.Len(t, randomPartPeerID, 12, "Peer ID random part length mismatch on run %d", i+1)
		for _, char := range randomPartPeerID {
			assert.True(t, (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9'),
				"Peer ID random part contains invalid character '%c' on run %d", char, i+1)
		}

		// Key checks
		assert.Len(t, pt.key, 8, "Key length mismatch on run %d", i+1)
		for _, char := range pt.key {
			assert.True(t, (char >= '0' && char <= '9') || (char >= 'A' && char <= 'F'),
				"Key contains invalid character '%c' on run %d", char, i+1)
		}

		// Check for uniqueness (highly likely)
		assert.False(t, previousPeerIDs[pt.peerID], "Duplicate Peer ID generated: %s", pt.peerID)
		assert.False(t, previousKeys[pt.key], "Duplicate Key generated: %s", pt.key)
		previousPeerIDs[pt.peerID] = true
		previousKeys[pt.key] = true
	}
}

func TestHttpRequestDirector_Scrape(t *testing.T) {
	rd := New()
	req, err := http.NewRequest("GET", "http://example.com/tracker/scrape?info_hash=123&unrelated_args=456", nil)
	req.Header.Set("User-Agent", "Teapot/1.0")
	require.NoError(t, err)

	// Store original values for comparison
	originalURL := req.URL.String()
	originalHeader := req.Header.Clone()

	err = rd.HttpRequestDirector(req)
	require.NoError(t, err)

	// Assert that the request was not modified
	assert.Equal(t, originalURL, req.URL.String(), "URL should not be modified for scrape requests")
	assert.Equal(t, originalHeader, req.Header, "Headers should not be modified for scrape requests")
}

// TestHttpRequestDirector_Announce tests the modification of headers and query parameters
// for announce requests, including parameter ordering and handling of private tracker auth keys.
func TestHttpRequestDirector_Announce(t *testing.T) {
	infoHash := "%A9%BFz%B1%BB%05%91%9A%23J5%13Y%95%14%89f%08_9"
	infoHashUnescaped, _ := url.QueryUnescape(infoHash)

	testCases := []struct {
		name               string
		rawQuery           string
		hasAuth            bool
		expectedParamCount int
	}{
		{
			name: "Public Torrent",
			rawQuery: fmt.Sprintf(
				"compact=1&downloaded=0&event=started&info_hash=%s&key=OLD_KEY&left=7159086&peer_id=OLD_PEER_ID&port=3456&supportcrypto=1&uploaded=0",
				infoHash),
			hasAuth:            false,
			expectedParamCount: 11, // Standard 11 params
		},
		{
			name: "Private Torrent",
			rawQuery: fmt.Sprintf(
				"auth=123&compact=1&downloaded=0&event=started&info_hash=%s&key=OLD_KEY&left=7159086&peer_id=OLD_PEER_ID&port=3456&supportcrypto=1&uploaded=0",
				infoHash),
			hasAuth:            true,
			expectedParamCount: 12, // Standard 11 params + auth
		},
	}

	// Expected order based on queryDefs in transmission.go (excluding auth)
	baseExpectedOrder := []string{
		"info_hash", "peer_id", "port", "uploaded", "downloaded",
		"left", "numwant", "key", "compact", "supportcrypto", "event",
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rd := New()
			dummyURL := "http://example.com/tracker/announce?" + tc.rawQuery

			req, err := http.NewRequest("GET", dummyURL, nil)
			require.NoError(t, err)
			// Add some initial headers to ensure they are overwritten/removed
			req.Header.Set("User-Agent", "OldAgent/1.0")
			req.Header.Set("X-Custom-Header", "ShouldBeRemoved")

			err = rd.HttpRequestDirector(req)
			require.NoError(t, err)

			// --- Header Assertions (same for all cases) ---
			assert.Equal(t, "Transmission/4.0.6", req.Header.Get("User-Agent"), "User-Agent header mismatch")
			assert.Equal(t, "deflate, gzip, br, zstd", req.Header.Get("Accept-Encoding"), "Accept-Encoding header mismatch")
			assert.Equal(t, "*/*", req.Header.Get("Accept"), "Accept header mismatch")
			assert.Empty(t, req.Header.Get("X-Custom-Header"), "Custom header should have been removed")
			assert.Len(t, req.Header, 3, "Incorrect number of headers") // User-Agent, Accept-Encoding, Accept

			// --- Query Parameter Assertions ---
			q, err := commons.QueryParamsFromRawQueryStr(req.URL.RawQuery)
			require.NoError(t, err)
			require.Len(t, q, tc.expectedParamCount, "Incorrect number of query parameters")

			expectedOrder := baseExpectedOrder
			offset := 0
			if tc.hasAuth {
				// Check auth param first
				assert.Equal(t, "auth", q[0].Name, "First param should be auth for private")
				assert.Equal(t, "123", q[0].Value, "Auth param value mismatch")
				offset = 1 // Shift index for checking the rest
			}

			// Check order and values for the rest
			for i, expectedName := range expectedOrder {
				actualParam := q[i+offset]
				assert.Equal(t, expectedName, actualParam.Name, "Parameter name mismatch at index %d", i)

				switch expectedName {
				case "info_hash":
					assert.Equal(t, infoHashUnescaped, actualParam.Value, "info_hash value mismatch")
				case "peer_id":
					assert.Len(t, actualParam.Value, 20, "peer_id length mismatch")
					assert.True(t, strings.HasPrefix(actualParam.Value, transmissionV406Bep20), "peer_id prefix mismatch")
				case "port":
					assert.Equal(t, "3456", actualParam.Value, "port value mismatch")
				case "uploaded":
					assert.Equal(t, "0", actualParam.Value, "uploaded value mismatch")
				case "downloaded":
					assert.Equal(t, "0", actualParam.Value, "downloaded value mismatch")
				case "left":
					assert.Equal(t, "7159086", actualParam.Value, "left value mismatch")
				case "numwant":
					assert.Equal(t, "80", actualParam.Value, "numwant value mismatch")
				case "key":
					assert.Len(t, actualParam.Value, 8, "key length mismatch")
					for _, char := range actualParam.Value {
						assert.True(t, (char >= '0' && char <= '9') || (char >= 'A' && char <= 'F'), "key contains invalid hex character '%c'", char)
					}
				case "compact":
					assert.Equal(t, "1", actualParam.Value, "compact value mismatch")
				case "supportcrypto":
					assert.Equal(t, "1", actualParam.Value, "supportcrypto value mismatch")
				case "event":
					assert.Equal(t, "started", actualParam.Value, "event value mismatch")
				}
			}
		})
	}
}

// TestHttpRequestDirector_PerTorrentHandling tests the logic related to
// generating, storing, reusing, and removing per-torrent data (peer_id, key).
func TestHttpRequestDirector_PerTorrentHandling(t *testing.T) {
	tr := New()
	announce := "http://example.com/tracker/announce"
	infoHash := "%A9%BFz%B1%BB%05%91%9A%23J5%13Y%95%14%89f%08_9"
	rawQuery := fmt.Sprintf(
		"?compact=1&downloaded=0&event=started&info_hash=%s&key=OLD_KEY&left=7159086&peer_id=OLD_PEER_ID&port=3456&supportcrypto=1&uploaded=0",
		infoHash)
	dummyURL := announce + rawQuery
	infoHashUnescaped, err := url.QueryUnescape(infoHash)
	require.NoError(t, err)

	// --- Initial call (event=started) ---
	req1, err := http.NewRequest("GET", dummyURL, nil)
	require.NoError(t, err)
	err = tr.HttpRequestDirector(req1)
	require.NoError(t, err)

	q1 := req1.URL.Query()
	generatedPeerID := q1.Get("peer_id")
	require.NotEmpty(t, generatedPeerID)
	generatedKey := q1.Get("key")
	require.NotEmpty(t, generatedKey)

	// Check stored data after first call
	id1 := announce + "--" + infoHashUnescaped
	storedData, ok := tr.torrents.Load(id1)
	require.True(t, ok, "PerTorrent data not found in map after first call")
	pt, ok := storedData.(*perTorrent)
	require.True(t, ok, "Stored data is not of type *perTorrent")
	assert.Equal(t, generatedPeerID, pt.peerID, "Stored peerID does not match generated peerID")
	assert.Equal(t, generatedKey, pt.key, "Stored key does not match generated key")

	_, task1Exists := tr.scheduler.Tasks()[id1]
	assert.True(t, task1Exists, "scrape task scheduled")

	// --- Subsequent call (event=started or no event) - should reuse ---
	// Use a query without event=started to ensure reuse happens even without the explicit event
	rawQueryNoEvent := fmt.Sprintf(
		"?compact=1&downloaded=10&info_hash=%s&key=OLD_KEY&left=7159076&peer_id=OLD_PEER_ID&port=3456&supportcrypto=1&uploaded=10",
		infoHash)
	dummyURLNoEvent := announce + rawQueryNoEvent
	req2, err := http.NewRequest("GET", dummyURLNoEvent, nil)
	require.NoError(t, err)
	err = tr.HttpRequestDirector(req2)
	require.NoError(t, err)

	q2 := req2.URL.Query()
	assert.Equal(t, generatedPeerID, q2.Get("peer_id"), "peer_id should be reused on second call")
	assert.Equal(t, generatedKey, q2.Get("key"), "key should be reused on second call")

	// Verify data still exists and is unchanged
	storedData2, ok := tr.torrents.Load(id1)
	require.True(t, ok, "PerTorrent data not found in map after second call")
	pt2, ok := storedData2.(*perTorrent)
	require.True(t, ok, "Stored data is not of type *perTorrent after second call")
	assert.Equal(t, generatedPeerID, pt2.peerID, "Stored peerID should not change after second call")
	assert.Equal(t, generatedKey, pt2.key, "Stored key should not change after second call")

	_, task1Exists = tr.scheduler.Tasks()[id1]
	assert.True(t, task1Exists, "scrape task still scheduled")

	// --- Call with 'stopped' event - should remove data ---
	stoppedQuery := strings.Replace(rawQuery, "event=started", "event=stopped", 1)
	stoppedURL := announce + stoppedQuery
	req3, err := http.NewRequest("GET", stoppedURL, nil)
	require.NoError(t, err)
	err = tr.HttpRequestDirector(req3)
	require.NoError(t, err)

	q3 := req3.URL.Query()
	assert.Equal(t, generatedPeerID, q3.Get("peer_id"), "peer_id should be reused on remove call")
	assert.Equal(t, generatedKey, q3.Get("key"), "key should be reused on remove call")

	_, ok = tr.torrents.Load(id1)
	assert.False(t, ok, "PerTorrent data should be removed after 'stopped' event")

	_, task1Exists = tr.scheduler.Tasks()[id1]
	assert.False(t, task1Exists, "scrape task stopped")

	// --- Call after 'stopped' - should generate new data ---
	req4, err := http.NewRequest("GET", dummyURL, nil) // event=started again
	require.NoError(t, err)
	err = tr.HttpRequestDirector(req4)
	require.NoError(t, err)

	q4 := req4.URL.Query()
	newGeneratedPeerID := q4.Get("peer_id")
	require.NotEmpty(t, generatedPeerID)
	newGeneratedKey := q4.Get("key")
	require.NotEmpty(t, generatedKey)

	assert.NotEqual(t, generatedPeerID, newGeneratedPeerID, "New peer_id should be generated after stopped event")
	assert.NotEqual(t, generatedKey, newGeneratedKey, "New key should be generated after stopped event")

	// Check stored data after fourth call
	storedData4, ok := tr.torrents.Load(id1)
	require.True(t, ok, "PerTorrent data not found in map after fourth call")
	pt4, ok := storedData4.(*perTorrent)
	require.True(t, ok, "Stored data is not of type *perTorrent after fourth call")
	assert.Equal(t, newGeneratedPeerID, pt4.peerID, "Stored peerID does not match newly generated peerID")
	assert.Equal(t, newGeneratedKey, pt4.key, "Stored key does not match newly generated key")

	_, task1Exists = tr.scheduler.Tasks()[id1]
	assert.True(t, task1Exists, "new scrape task scheduled")

	// --- Call with different tracker, same infohash ---
	announce2 := "http://another-tracker.com/announce"
	dummyURLTracker2 := announce2 + rawQuery // Use the same query params (including event=started)
	req5, err := http.NewRequest("GET", dummyURLTracker2, nil)
	require.NoError(t, err)
	err = tr.HttpRequestDirector(req5)
	require.NoError(t, err)

	q5 := req5.URL.Query()
	tracker2PeerID := q5.Get("peer_id")
	require.NotEmpty(t, tracker2PeerID)
	tracker2Key := q5.Get("key")
	require.NotEmpty(t, tracker2Key)

	// Verify new peer_id and key are different from the ones for the first tracker
	assert.NotEqual(t, newGeneratedPeerID, tracker2PeerID, "PeerID should be different for different tracker")
	assert.NotEqual(t, newGeneratedKey, tracker2Key, "Key should be different for different tracker")

	// Verify a new entry exists for the new tracker/infohash combo
	id2 := announce2 + "--" + infoHashUnescaped
	storedData5, ok := tr.torrents.Load(id2)
	require.True(t, ok, "PerTorrent data not found in map for second tracker")
	pt5, ok := storedData5.(*perTorrent)
	require.True(t, ok, "Stored data is not of type *perTorrent for second tracker")
	assert.Equal(t, tracker2PeerID, pt5.peerID, "Stored peerID does not match generated peerID for second tracker")
	assert.Equal(t, tracker2Key, pt5.key, "Stored key does not match generated key for second tracker")

	_, task2Exists := tr.scheduler.Tasks()[id2]
	assert.True(t, task2Exists, "new scrape task scheduled")

	// Verify the entry for the first tracker still exists (from req4)
	_, ok = tr.torrents.Load(id1)
	assert.True(t, ok, "PerTorrent data for the first tracker should still exist")

	_, task1Exists = tr.scheduler.Tasks()[id1]
	assert.True(t, task1Exists, "scrape task still scheduled")
}
