package tinyServer

import (
	"io"
	"net/url"
)

type Request struct {
	Method string
	Header Header
	URL    *url.URL
	Body   io.ReadCloser
}
