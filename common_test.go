package camouflagetorrentclients

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryDef_MustHave(t *testing.T) {
	def := mustHaveDef("required")
	q := url.Values{}
	q.Set("required", "value1")

	param, err := def.process(q)
	require.NoError(t, err, "mustHaveDef failed unexpectedly")
	require.NotNil(t, param, "mustHaveDef returned nil param unexpectedly")
	assert.Equal(t, "required", param.name, "mustHaveDef returned incorrect param name")
	assert.Equal(t, "value1", param.value, "mustHaveDef returned incorrect param value")

	qMissing := url.Values{}
	paramMissing, errMissing := def.process(qMissing)
	require.Error(t, errMissing, "mustHaveDef did not return error when query param was missing")
	assert.Nil(t, paramMissing, "mustHaveDef returned non-nil param when query param was missing")
}

func TestQueryDef_Optional(t *testing.T) {
	def := optionalDef("optional")
	q := url.Values{}
	q.Set("optional", "value2")

	param, err := def.process(q)
	require.NoError(t, err, "optionalDef failed unexpectedly")
	require.NotNil(t, param, "optionalDef returned nil param unexpectedly")
	assert.Equal(t, "optional", param.name, "optionalDef returned incorrect param name")
	assert.Equal(t, "value2", param.value, "optionalDef returned incorrect param value")

	qMissing := url.Values{}
	paramMissing, errMissing := def.process(qMissing)
	assert.NoError(t, errMissing, "optionalDef returned error when query param was missing")
	assert.Nil(t, paramMissing, "optionalDef returned non-nil param when query param was missing")
}

func TestQueryDef_Fixed(t *testing.T) {
	def := fixedDef("fixed", "fixedValue")
	q := url.Values{} // Should ignore this

	param, err := def.process(q)
	require.NoError(t, err, "fixedDef failed unexpectedly")
	require.NotNil(t, param, "fixedDef returned nil param unexpectedly")
	assert.Equal(t, "fixed", param.name, "fixedDef returned incorrect param name")
	assert.Equal(t, "fixedValue", param.value, "fixedDef returned incorrect param value")
}

func TestProcessQuery(t *testing.T) {
	defs := []*queryDef{
		mustHaveDef("req"),
		optionalDef("opt"),
		fixedDef("fix", "valFix"),
		optionalDef("opt_missing"),
	}

	q := url.Values{}
	q.Set("req", "valReq")
	q.Set("opt", "valOpt")
	// opt_missing is not set
	// fix is handled by fixedDef

	expectedParams := queryParams{
		&queryParam{name: "req", value: "valReq"},
		&queryParam{name: "opt", value: "valOpt"},
		&queryParam{name: "fix", value: "valFix"},
	}

	params, err := processQuery(defs, q)
	require.NoError(t, err, "processQuery failed unexpectedly")
	assert.Equal(t, expectedParams, params, "processQuery returned incorrect params")

	// Test missing required param
	qMissing := url.Values{}
	qMissing.Set("opt", "valOpt")
	_, errMissing := processQuery(defs, qMissing)
	require.Error(t, errMissing, "processQuery did not return error when required param was missing")
}

func TestQueryParams_Str(t *testing.T) {
	tests := []struct {
		name     string
		params   queryParams
		expected string
	}{
		{
			name:     "empty",
			params:   queryParams{},
			expected: "",
		},
		{
			name: "single param",
			params: queryParams{
				&queryParam{name: "key1", value: "value1"},
			},
			expected: "key1=value1",
		},
		{
			name: "multiple params",
			params: queryParams{
				&queryParam{name: "key1", value: "value1"},
				&queryParam{name: "key2", value: "value2"},
				&queryParam{name: "key3", value: "value3"},
			},
			expected: "key1=value1&key2=value2&key3=value3",
		},
		{
			name: "params needing escape",
			params: queryParams{
				&queryParam{name: "k ey1", value: "v&l=ue 1"},
				&queryParam{name: "key2", value: "value2"},
			},
			expected: "k+ey1=v%26l%3Due+1&key2=value2", // Note: url.QueryEscape uses '+' for space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.params.str()
			assert.Equal(t, tt.expected, result, "queryParams.str() returned incorrect string")
		})
	}
}

// TestTorrentFile verifies that a specific torrent file ("test-torrents/test-public.torrent")
// conforms to expected specifications.
func TestTorrentFile(t *testing.T) {
	mi, err := metainfo.LoadFromFile("test-torrents/test-public.torrent")
	require.NoError(t, err)
	assert.Len(t, mi.AnnounceList, 2)
	assert.Equal(t, []string{"http://127.0.0.1:3456/tracker/announce"}, mi.AnnounceList[0])
	assert.Equal(t, []string{"http://127.0.0.1:7890/tracker/announce"}, mi.AnnounceList[1])
	assert.Equal(t, "a9bf7ab1bb05919a234a35135995148966085f39", mi.HashInfoBytes().HexString())

	info, err := mi.UnmarshalInfo()
	require.NoError(t, err)
	files := []string{}
	for _, f := range info.Files {
		files = append(files, f.DisplayPath(&info))
	}
	assert.Len(t, files, 3)
	slices.Sort(files)
	assert.Equal(t, []string{"1.txt", "dir/2.txt", "large.file.txt"}, files)
}

func queryParamsFromRawQueryStr(s string) (queryParams, error) {
	res := queryParams{}
	if s == "" {
		return res, nil
	}
	for _, pair := range strings.Split(s, "&") {
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid query param %s", pair)
		}
		key, err := url.QueryUnescape(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid query param %s", pair)
		}
		value, err := url.QueryUnescape(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid query param %s", pair)
		}
		res = append(res, &queryParam{name: key, value: value})
	}
	return res, nil
}
