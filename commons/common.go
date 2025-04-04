package commons

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	EventStarted = "started"
	EventStopped = "stopped"
)

type QueryDef struct {
	name    string
	process func(q url.Values) (*QueryParam, error)
	value   string
}

func MustHaveDef(name string) *QueryDef {
	d := &QueryDef{name: name}
	d.process = d.mustHave
	return d
}

func OptionalDef(name string) *QueryDef {
	d := &QueryDef{name: name}
	d.process = d.optional
	return d
}

func FixedDef(name, value string) *QueryDef {
	d := &QueryDef{name: name, value: value}
	d.process = d.fixed
	return d
}

func (d *QueryDef) mustHave(q url.Values) (*QueryParam, error) {
	if !q.Has(d.name) {
		return nil, fmt.Errorf("query %s not found", d.name)
	}
	return &QueryParam{Name: d.name, Value: q.Get(d.name)}, nil
}

func (d *QueryDef) optional(q url.Values) (*QueryParam, error) {
	if !q.Has(d.name) {
		return nil, nil
	}
	return &QueryParam{Name: d.name, Value: q.Get(d.name)}, nil
}

func (d *QueryDef) fixed(q url.Values) (*QueryParam, error) {
	return &QueryParam{Name: d.name, Value: d.value}, nil
}

type QueryParam struct {
	Name  string
	Value string
}

type QueryParams []*QueryParam

func ProcessQuery(defs []*QueryDef, q url.Values) (QueryParams, error) {
	res := QueryParams{}
	for _, def := range defs {
		param, err := def.process(q)
		if err != nil {
			return nil, err
		}
		if param != nil {
			res = append(res, param)
		}
	}
	return res, nil
}

func (l QueryParams) Str() string {
	sb := strings.Builder{}

	for i, q := range l {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(url.QueryEscape(q.Name))
		sb.WriteString("=")
		sb.WriteString(url.QueryEscape(q.Value))
	}
	return sb.String()
}

func QueryParamsFromRawQueryStr(s string) (QueryParams, error) {
	res := QueryParams{}
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
		res = append(res, &QueryParam{Name: key, Value: value})
	}
	return res, nil
}
