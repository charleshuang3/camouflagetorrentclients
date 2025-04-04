package commons

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryDef_MustHave(t *testing.T) {
	def := MustHaveDef("required")
	q := url.Values{}
	q.Set("required", "value1")

	param, err := def.process(q)
	require.NoError(t, err, "mustHaveDef failed unexpectedly")
	require.NotNil(t, param, "mustHaveDef returned nil param unexpectedly")
	assert.Equal(t, "required", param.Name, "mustHaveDef returned incorrect param name")
	assert.Equal(t, "value1", param.Value, "mustHaveDef returned incorrect param value")

	qMissing := url.Values{}
	paramMissing, errMissing := def.process(qMissing)
	require.Error(t, errMissing, "mustHaveDef did not return error when query param was missing")
	assert.Nil(t, paramMissing, "mustHaveDef returned non-nil param when query param was missing")
}

func TestQueryDef_Optional(t *testing.T) {
	def := OptionalDef("optional")
	q := url.Values{}
	q.Set("optional", "value2")

	param, err := def.process(q)
	require.NoError(t, err, "optionalDef failed unexpectedly")
	require.NotNil(t, param, "optionalDef returned nil param unexpectedly")
	assert.Equal(t, "optional", param.Name, "optionalDef returned incorrect param name")
	assert.Equal(t, "value2", param.Value, "optionalDef returned incorrect param value")

	qMissing := url.Values{}
	paramMissing, errMissing := def.process(qMissing)
	assert.NoError(t, errMissing, "optionalDef returned error when query param was missing")
	assert.Nil(t, paramMissing, "optionalDef returned non-nil param when query param was missing")
}

func TestQueryDef_Fixed(t *testing.T) {
	def := FixedDef("fixed", "fixedValue")
	q := url.Values{} // Should ignore this

	param, err := def.process(q)
	require.NoError(t, err, "fixedDef failed unexpectedly")
	require.NotNil(t, param, "fixedDef returned nil param unexpectedly")
	assert.Equal(t, "fixed", param.Name, "fixedDef returned incorrect param name")
	assert.Equal(t, "fixedValue", param.Value, "fixedDef returned incorrect param value")
}

func TestProcessQuery(t *testing.T) {
	defs := []*QueryDef{
		MustHaveDef("req"),
		OptionalDef("opt"),
		FixedDef("fix", "valFix"),
		OptionalDef("opt_missing"),
	}

	q := url.Values{}
	q.Set("req", "valReq")
	q.Set("opt", "valOpt")
	// opt_missing is not set
	// fix is handled by fixedDef

	expectedParams := QueryParams{
		&QueryParam{Name: "req", Value: "valReq"},
		&QueryParam{Name: "opt", Value: "valOpt"},
		&QueryParam{Name: "fix", Value: "valFix"},
	}

	params, err := ProcessQuery(defs, q)
	require.NoError(t, err, "processQuery failed unexpectedly")
	assert.Equal(t, expectedParams, params, "processQuery returned incorrect params")

	// Test missing required param
	qMissing := url.Values{}
	qMissing.Set("opt", "valOpt")
	_, errMissing := ProcessQuery(defs, qMissing)
	require.Error(t, errMissing, "processQuery did not return error when required param was missing")
}

func TestQueryParams_Str(t *testing.T) {
	tests := []struct {
		name     string
		params   QueryParams
		expected string
	}{
		{
			name:     "empty",
			params:   QueryParams{},
			expected: "",
		},
		{
			name: "single param",
			params: QueryParams{
				&QueryParam{Name: "key1", Value: "value1"},
			},
			expected: "key1=value1",
		},
		{
			name: "multiple params",
			params: QueryParams{
				&QueryParam{Name: "key1", Value: "value1"},
				&QueryParam{Name: "key2", Value: "value2"},
				&QueryParam{Name: "key3", Value: "value3"},
			},
			expected: "key1=value1&key2=value2&key3=value3",
		},
		{
			name: "params needing escape",
			params: QueryParams{
				&QueryParam{Name: "k ey1", Value: "v&l=ue 1"},
				&QueryParam{Name: "key2", Value: "value2"},
			},
			expected: "k+ey1=v%26l%3Due+1&key2=value2", // Note: url.QueryEscape uses '+' for space
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.params.Str()
			assert.Equal(t, tt.expected, result, "queryParams.str() returned incorrect string")
		})
	}
}

func TestQueryParamsFromRawQueryStr(t *testing.T) {
	tests := []struct {
		name     string
		rawQuery string
		expected QueryParams
		wantErr  bool
	}{
		{
			name:     "empty string",
			rawQuery: "",
			expected: QueryParams{},
			wantErr:  false,
		},
		{
			name:     "single param",
			rawQuery: "key1=value1",
			expected: QueryParams{
				&QueryParam{Name: "key1", Value: "value1"},
			},
			wantErr: false,
		},
		{
			name:     "multiple params unsorted",
			rawQuery: "key2=value2&key1=value1",
			expected: QueryParams{ // Expected unsorted output
				&QueryParam{Name: "key2", Value: "value2"},
				&QueryParam{Name: "key1", Value: "value1"},
			},
			wantErr: false,
		},
		{
			name:     "params needing decoding",
			rawQuery: "k%20ey=v%26l%3Due",
			expected: QueryParams{
				&QueryParam{Name: "k ey", Value: "v&l=ue"},
			},
			wantErr: false,
		},
		{
			name:     "duplicate key",
			rawQuery: "key1=value1&key1=value2",
			expected: QueryParams{
				&QueryParam{Name: "key1", Value: "value1"},
				&QueryParam{Name: "key1", Value: "value2"},
			},
			wantErr: false,
		},
		{
			name:     "empty value",
			rawQuery: "key1=",
			expected: QueryParams{
				&QueryParam{Name: "key1", Value: ""},
			},
			wantErr: false,
		},
		{
			name:     "key only",
			rawQuery: "key1",
			expected: QueryParams{},
			wantErr:  true,
		},
		{
			name:     "mixed",
			rawQuery: "b=2&a=1&c=",
			expected: QueryParams{
				&QueryParam{Name: "b", Value: "2"},
				&QueryParam{Name: "a", Value: "1"},
				&QueryParam{Name: "c", Value: ""},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := QueryParamsFromRawQueryStr(tt.rawQuery)

			if tt.wantErr {
				require.Error(t, err, "QueryParamsFromRawQueryStr() did not return an expected error")
			} else {
				require.NoError(t, err, "QueryParamsFromRawQueryStr() returned an unexpected error")
				assert.Equal(t, tt.expected, result, "QueryParamsFromRawQueryStr() returned incorrect params")
			}
		})
	}
}
