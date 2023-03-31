package healthcheck

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/xerrors"
)

type AccessURLReport struct {
	Reachable       bool
	StatusCode      int
	HealthzResponse string
	Err             error
}

func (r *AccessURLReport) Run(ctx context.Context, accessURL *url.URL) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	accessURL, err := accessURL.Parse("/healthz")
	if err != nil {
		r.Err = xerrors.Errorf("parse healthz endpoint: %w", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, "GET", accessURL.String(), nil)
	if err != nil {
		r.Err = xerrors.Errorf("create healthz request: %w", err)
		return
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		r.Err = xerrors.Errorf("get healthz endpoint: %w", err)
		return
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		r.Err = xerrors.Errorf("read healthz response: %w", err)
		return
	}

	r.Reachable = true
	r.StatusCode = res.StatusCode
	r.HealthzResponse = string(body)
}
