package camouflagetorrentclients

import (
	"testing"

	"github.com/anacrolix/torrent"
	"github.com/charleshuang3/camouflagetorrentclients/transmission"
)

func TestNewDirectors(t *testing.T) {
	d := NewDirectors(transmission.New())
	cfg := torrent.NewDefaultClientConfig()
	cfg.HttpRequestDirector = d.ChangeHttpRequest
}
