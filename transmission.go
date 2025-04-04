package camouflagetorrentclients

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"

	"github.com/anacrolix/log"
)

const (
	transmissionV406Bep20 = "-TR4060-"
)

var (
	transmissionLogger = log.NewLogger("transmission")
)

type perTorrent struct {
	peerID string
	key    string
}

// Transmission builds the announce request query parameters in the same fixed order
// and format as the Transmission BitTorrent client.
//
// Transmission 4.0.6:
//
// https://github.com/transmission/transmission/blob/38c164933e9f77c110b48fe745861c3b98e3d83e/libtransmission/announcer-http.cc#L185
type Transmission struct {
	// info_hash -> peer_id, key
	torrents map[string]*perTorrent
}

func NewTransmission() *Transmission {
	return &Transmission{
		torrents: map[string]*perTorrent{},
	}
}

func (s *Transmission) HttpRequestDirector(r *http.Request) error {
	q := r.URL.Query()

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

	pt, exists := s.torrents[infoHash]
	if event == EventStarted {
		// It is a bug if exists.
		if exists {
			transmissionLogger.Levelf(log.Error, "start a torrent already started")
		}
		pt = createPerTorrent()
		s.torrents[infoHash] = pt
	} else if event == EventStopped {
		// If stopped, remove the torrent entry
		delete(s.torrents, infoHash)
		// If it didn't exist before stopping, we might not have peer_id/key,
		// but the request might still be valid if the tracker doesn't require them on stop.
		// For now, we proceed without setting them if pt is nil.
	}

	if pt == nil {
		transmissionLogger.Levelf(log.Error, "torrent not started")
		return fmt.Errorf("missing per-torrent data for info_hash %s and event '%s'", infoHash, event)
	}

	q.Set("peer_id", pt.peerID)
	q.Set("key", pt.key)

	queryDefs := []*queryDef{
		mustHaveDef("info_hash"),
		mustHaveDef("peer_id"),
		mustHaveDef("port"),
		mustHaveDef("uploaded"),
		mustHaveDef("downloaded"),
		mustHaveDef("left"),
		mustHaveDef("numwant"),
		mustHaveDef("key"),
		mustHaveDef("compact"),
		mustHaveDef("supportcrypto"),
		optionalDef("requirecrypto"),
		optionalDef("event"),
		optionalDef("corrupt"),
		optionalDef("trackerid"),
	}

	params, err := processQuery(queryDefs, q)
	if err != nil {
		return err
	}

	r.URL.RawQuery = params.str()
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
