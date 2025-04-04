package camouflagetorrentclients

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	EventStarted = "started"
	EventStopped = "stopped"
)

type queryDef struct {
	name    string
	process func(q url.Values) (*queryParam, error)
	value   string
}

func mustHaveDef(name string) *queryDef {
	d := &queryDef{name: name}
	d.process = d.mustHave
	return d
}

func optionalDef(name string) *queryDef {
	d := &queryDef{name: name}
	d.process = d.optional
	return d
}

func fixedDef(name, value string) *queryDef {
	d := &queryDef{name: name, value: value}
	d.process = d.fixed
	return d
}

func (d *queryDef) mustHave(q url.Values) (*queryParam, error) {
	if !q.Has(d.name) {
		return nil, fmt.Errorf("query %s not found", d.name)
	}
	return &queryParam{name: d.name, value: q.Get(d.name)}, nil
}

func (d *queryDef) optional(q url.Values) (*queryParam, error) {
	if !q.Has(d.name) {
		return nil, nil
	}
	return &queryParam{name: d.name, value: q.Get(d.name)}, nil
}

func (d *queryDef) fixed(q url.Values) (*queryParam, error) {
	return &queryParam{name: d.name, value: d.value}, nil
}

type queryParam struct {
	name  string
	value string
}

type queryParams []*queryParam

func processQuery(defs []*queryDef, q url.Values) (queryParams, error) {
	res := queryParams{}
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

func (l queryParams) str() string {
	sb := strings.Builder{}

	for i, q := range l {
		if i > 0 {
			sb.WriteString("&")
		}
		sb.WriteString(url.QueryEscape(q.name))
		sb.WriteString("=")
		sb.WriteString(url.QueryEscape(q.value))
	}
	return sb.String()
}
