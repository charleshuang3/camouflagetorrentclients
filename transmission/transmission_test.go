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
	totalTrackers   = 2
	testTimeout     = 15 * time.Second
	testTorrentFile = "../test-torrents/test-public.torrent"
	port            = 3456
)

func TestAnnounceRequest(t *testing.T) {
	tempDir := t.TempDir()

	var wg sync.WaitGroup

	// Setup test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Note: We might receive multiple requests here, potentially concurrently.
		// Assertions should be thread-safe or carefully managed if state is involved.
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
		q, err := commons.QueryParamsFromRawQueryStr(r.URL.RawQuery)
		require.NoError(t, err)
		assert.Len(t, q, 11)

		infoHash, err := url.QueryUnescape("%A9%BFz%B1%BB%05%91%9A%23J5%13Y%95%14%89f%08_9")
		require.NoError(t, err)
		assert.Equal(t, q[0], &commons.QueryParam{Name: "info_hash", Value: infoHash})

		assert.Equal(t, q[1].Name, "peer_id")
		assert.Len(t, q[1].Value, 20)
		assert.True(t, strings.HasPrefix(q[1].Value, transmissionV406Bep20))

		assert.Equal(t, q[2], &commons.QueryParam{Name: "port", Value: strconv.Itoa(port)})
		assert.Equal(t, q[3], &commons.QueryParam{Name: "uploaded", Value: "0"})
		assert.Equal(t, q[4], &commons.QueryParam{Name: "downloaded", Value: "0"})
		assert.Equal(t, q[5], &commons.QueryParam{Name: "left", Value: "7159086"})
		assert.Equal(t, q[6], &commons.QueryParam{Name: "numwant", Value: "80"})
		assert.Equal(t, q[7].Name, "key")
		assert.Len(t, q[7].Value, 8)
		assert.Equal(t, q[8], &commons.QueryParam{Name: "compact", Value: "1"})
		assert.Equal(t, q[9], &commons.QueryParam{Name: "supportcrypto", Value: "1"})
		assert.Equal(t, q[10], &commons.QueryParam{Name: "event", Value: "started"})

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

	mi, err := metainfo.LoadFromFile(testTorrentFile)
	require.NoError(t, err)
	_, err = c.AddTorrent(mi)
	require.NoError(t, err)

	// Set the WaitGroup counter
	wg.Add(totalTrackers)

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
		t.Fatalf("timed out waiting for %d announce requests", totalTrackers)
	}
}
