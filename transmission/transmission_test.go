package transmission

import (
	"context"
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
				beginIndex := 0
				if tc.hasAuthQuery {
					beginIndex = 1
				}

				q, err := commons.QueryParamsFromRawQueryStr(r.URL.RawQuery)
				require.NoError(t, err)
				if tc.hasAuthQuery {
					assert.Len(t, q, 12)
				} else {
					assert.Len(t, q, 11)
				}

				infoHash, err := url.QueryUnescape(tc.infoHash)
				require.NoError(t, err)

				if tc.hasAuthQuery {
					assert.Equal(t, q[0], &commons.QueryParam{Name: "auth", Value: "123"})
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
