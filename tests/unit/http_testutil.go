package unit

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"time"
)

type handlerRoundTripper struct {
	handler http.Handler
}

func (rt handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

type errorRoundTripper struct {
	err error
}

func (rt errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	if rt.err == nil {
		return nil, errors.New("network error")
	}
	return nil, rt.err
}

type timeoutRoundTripper struct{}

func (rt timeoutRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func newHandlerClient(handler http.Handler, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: handlerRoundTripper{handler: handler},
	}
}
