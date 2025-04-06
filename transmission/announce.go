package transmission

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/anacrolix/log"
	"github.com/charleshuang3/camouflagetorrentclients/commons"
	"github.com/madflojo/tasks"
	"golang.org/x/time/rate"
)

const (
	transmissionV406Bep20 = "-TR4060-"
)

var (
	logger = log.NewLogger("transmission")
)

type perTorrent struct {
	peerID string
	key    string
}

// mimickTransmission builds the announce request query parameters in the same fixed order
// and format as the mimickTransmission BitTorrent client.
//
// transmission 4.0.6:
//
// https://github.com/mimickTransmission/mimickTransmission/blob/38c164933e9f77c110b48fe745861c3b98e3d83e/libtransmission/announcer-http.cc#L185
type mimickTransmission struct {
	// info_hash -> peer_id, key
	torrents          sync.Map
	scheduler         *tasks.Scheduler
	scrapeRateLimiter *rate.Limiter
}

func New() *mimickTransmission {
	return &mimickTransmission{
		torrents:          sync.Map{},
		scheduler:         tasks.New(),
		scrapeRateLimiter: rate.NewLimiter(rate.Limit(maxScrapesPerSecond), maxScrapesPerSecond),
	}
}

func (s *mimickTransmission) HttpRequestDirector(r *http.Request) error {
	// Do nothing for scrape request. anacrolix/torrent does not call HttpRequestDirector right now.
	// Just incase the behavior changed.
	parts := strings.Split(r.URL.Path, "/")
	if parts[len(parts)-1] == "scrape" {
		return nil
	}

	err := s.modifyQuery(r)
	if err != nil {
		return err
	}
	return modifyHeaders(r)
}

func (s *mimickTransmission) modifyQuery(r *http.Request) error {
	q := r.URL.Query()

	// RawQuery may contains private tracker's query at the beginning.
	// before "&compact"
	index := strings.Index(r.URL.RawQuery, "&compact")
	privateTrackerQuery := ""
	if index != -1 {
		privateTrackerQuery = r.URL.RawQuery[0:index]
	}

	// transmission use fixed value for "numwant", "compact", "supportcrypto".
	// anacrolix/torrent does not provide "numwant", and assign fixed value for "compact", "supportcrypto".
	// Ensure this behavior does not change.
	if q.Has("numwant") {
		return fmt.Errorf("anacrolix/torrent provides numwant")
	}
	if q.Get("compact") != "1" {
		return fmt.Errorf("anacrolix/torrent provides compact!=1")
	}
	if q.Get("supportcrypto") != "1" {
		return fmt.Errorf("anacrolix/torrent provides supportcrypto!=1")
	}

	q.Set("numwant", "80")

	infoHash := q.Get("info_hash")
	if infoHash == "" {
		return fmt.Errorf("missing info_hash")
	}
	event := q.Get("event")

	id := perTrackerTorrentID(r.URL, infoHash)
	got, exists := s.torrents.LoadOrStore(id, createPerTorrent())
	if event == commons.EventStarted {
		// It is a bug if exists.
		if exists {
			logger.Levelf(log.Error, "start a torrent already started")
		}
	} else if event == commons.EventStopped {
		s.torrents.Delete(id)
		s.scheduler.Del(id)
	}
	// Announce not following a started event is possible, when seeding a finished torrent.

	// schedule scrape requests.
	if !exists {
		s.scheduleScrape(id, newScrapeTask(s, r.URL, infoHash, privateTrackerQuery))
	}

	pt := got.(*perTorrent)

	q.Set("peer_id", pt.peerID)
	q.Set("key", pt.key)

	queryDefs := []*commons.QueryDef{
		commons.MustHaveDef("info_hash"),
		commons.MustHaveDef("peer_id"),
		commons.MustHaveDef("port"),
		commons.MustHaveDef("uploaded"),
		commons.MustHaveDef("downloaded"),
		commons.MustHaveDef("left"),
		commons.MustHaveDef("numwant"),
		commons.MustHaveDef("key"),
		commons.MustHaveDef("compact"),
		commons.MustHaveDef("supportcrypto"),
		commons.OptionalDef("requirecrypto"),
		commons.OptionalDef("event"),
		commons.OptionalDef("corrupt"),
		commons.OptionalDef("trackerid"),
	}

	params, err := commons.ProcessQuery(queryDefs, q)
	if err != nil {
		return err
	}

	if privateTrackerQuery != "" {
		r.URL.RawQuery = privateTrackerQuery + "&" + params.Str()
	} else {
		r.URL.RawQuery = params.Str()
	}

	return nil
}

func modifyHeaders(r *http.Request) error {
	// Clear existing headers
	for k := range r.Header {
		delete(r.Header, k)
	}

	// Add new headers
	r.Header.Set("Accept-Encoding", "deflate, gzip, br, zstd")
	r.Header.Set("User-Agent", "Transmission/4.0.6")
	r.Header.Set("Accept", "*/*")

	return nil
}

func createPerTorrent() *perTorrent {
	// https://github.com/transmission/transmission/blob/ac5c9e082da257e102eb4ff18f2e433976a585d1/libtransmission/session.cc#L194
	// peer_id should be "-TRxyzb-" + 12 random alphanumeric char. Per session.
	// But anacrolix/torrent is per client.
	charSet := "0123456789abcdefghijklmnopqrstuvwxyz"

	// On transimission, key is random uint32 in 08X format. Per session.
	// But anacrolix/torrent is per client.

	// Generate peer_id
	peerID := make([]byte, 12)
	for i := range peerID {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charSet))))
		if err != nil {
			// crypto/rand should not fail on Linux/macOS. Panic if it does.
			panic(fmt.Errorf("failed to generate random int for peer ID: %w", err))
		}
		peerID[i] = charSet[n.Int64()]
	}

	// Generate key
	keyBytes := make([]byte, 4) // 4 bytes for uint32
	_, err := rand.Read(keyBytes)
	if err != nil {
		// crypto/rand should not fail on Linux/macOS. Panic if it does.
		panic(fmt.Errorf("failed to generate random bytes for key: %w", err))
	}
	key := fmt.Sprintf("%08X", keyBytes) // Format as 8-char uppercase hex

	return &perTorrent{
		peerID: transmissionV406Bep20 + string(peerID),
		key:    key,
	}
}

func announceURL(u *url.URL) string {
	urlCopy := *u
	urlCopy.RawQuery = ""
	return urlCopy.String()
}

func perTrackerTorrentID(u *url.URL, infoHash string) string {
	return announceURL(u) + "--" + infoHash
}
