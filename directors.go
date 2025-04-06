package camouflagetorrentclients

import "net/http"

// HttpRequestDirector defines an interface for modifying HTTP requests.
type HttpRequestDirector interface {
	// ChangeHttpRequest modifies the given HTTP request.
	// It returns an error if the modification fails.
	ChangeHttpRequest(*http.Request) error
}

// Directors holds a list of HttpRequestDirector implementations.
type Directors struct {
	directors []HttpRequestDirector
}

// NewDirectors creates a new Directors instance with the given directors.
func NewDirectors(directors ...HttpRequestDirector) *Directors {
	return &Directors{directors: directors}
}

// ChangeHttpRequest iterates through the list of directors and calls their
// ChangeHttpRequest method on the provided request. It stops and returns
// the error if any director returns an error.
func (d *Directors) ChangeHttpRequest(req *http.Request) error {
	for _, director := range d.directors {
		if err := director.ChangeHttpRequest(req); err != nil {
			return err
		}
	}
	return nil
}
