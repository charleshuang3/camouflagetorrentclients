package camouflagetorrentclients

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	totalTrackers   = 2
	testTimeout     = 15 * time.Second
	testTorrentFile = "test-torrents/test.torrent"
	port            = 3456
)

func TestAnnounceRequest(t *testing.T) {
	tempDir := t.TempDir()

	var wg sync.WaitGroup

	// Setup test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Note: We might receive multiple requests here, potentially concurrently.
		// Assertions should be thread-safe or carefully managed if state is involved.
		// Basic query parameter checks:
		assert.Equal(t, "GET", r.Method)
		assert.Contains(t, r.URL.Path, "/announce")

		q, err := queryParamsFromRawQueryStr(r.URL.RawQuery)
		require.NoError(t, err)
		assert.Len(t, q, 11)

		infoHash, err := url.QueryUnescape("%A9%BFz%B1%BB%05%91%9A%23J5%13Y%95%14%89f%08_9")
		require.NoError(t, err)
		assert.Equal(t, q[0], &queryParam{name: "info_hash", value: infoHash})

		assert.Equal(t, q[1].name, "peer_id")
		assert.Len(t, q[1].value, 20)
		assert.True(t, strings.HasPrefix(q[1].value, transmissionV406Bep20))

		assert.Equal(t, q[2], &queryParam{name: "port", value: strconv.Itoa(port)})
		assert.Equal(t, q[3], &queryParam{name: "uploaded", value: "0"})
		assert.Equal(t, q[4], &queryParam{name: "downloaded", value: "0"})
		assert.Equal(t, q[5], &queryParam{name: "left", value: "7159086"})
		assert.Equal(t, q[6], &queryParam{name: "numwant", value: "80"})
		assert.Equal(t, q[7].name, "key")
		assert.Len(t, q[7].value, 8)
		assert.Equal(t, q[8], &queryParam{name: "compact", value: "1"})
		assert.Equal(t, q[9], &queryParam{name: "supportcrypto", value: "1"})
		assert.Equal(t, q[10], &queryParam{name: "event", value: "started"})

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("d8:intervali1800e5:peersld2:ip9:127.0.0.14:porti6881e6:peer id20:-TR3000-012345678901e")) // Example minimal tracker response

		// Signal that a request was processed
		wg.Done()
	}))
	defer ts.Close()

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = tempDir // Use the temp directory
	tr := NewTransmission()
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
