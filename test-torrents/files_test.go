package testtorrents

import (
	"slices"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTorrentFiles(t *testing.T) {
	testCases := []struct {
		name         string
		torrentFile  string
		infoHash     string
		announceList []string
		files        []string
		private      bool
	}{
		{
			"test-public-torrent",
			"test-public.torrent",
			"a9bf7ab1bb05919a234a35135995148966085f39",
			[]string{"http://127.0.0.1:3456/tracker/announce", "http://127.0.0.1:7890/tracker/announce"},
			[]string{"1.txt", "dir/2.txt", "large.file.txt"},
			false,
		},
		{
			"test-private-torrent",
			"test-private.torrent",
			"3183cad92b935c82d20f243a884a44708b3b2b22",
			[]string{"http://127.0.0.1:3456/tracker/announce?auth=123"},
			[]string{"1.txt", "dir/2.txt", "large.file.txt"},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mi, err := metainfo.LoadFromFile(tc.torrentFile)
			require.NoError(t, err)
			assert.Len(t, mi.AnnounceList, len(tc.announceList))
			announceList := []string{}
			for _, list := range mi.AnnounceList {
				announceList = append(announceList, list[0])
			}
			assert.Equal(t, tc.announceList, announceList)
			assert.Equal(t, tc.infoHash, mi.HashInfoBytes().HexString())

			info, err := mi.UnmarshalInfo()
			require.NoError(t, err)
			files := []string{}
			for _, f := range info.Files {
				files = append(files, f.DisplayPath(&info))
			}
			assert.Len(t, files, 3)
			slices.Sort(files)
			assert.Equal(t, tc.files, files)

			if tc.private {
				assert.NotNil(t, info.Private)
				assert.Equal(t, tc.private, *info.Private)

			} else {
				if info.Private != nil {
					assert.Equal(t, tc.private, *info.Private)
				}
			}
		})
	}

}
