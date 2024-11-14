package tinyServer

import (
	"bytes"
	"fmt"
)

type Response struct {
	Status  int
	Body    *bytes.Buffer
	Headers Header
}

func NewResponse() *Response {
	return &Response{
		Status:  200,
		Body:    new(bytes.Buffer),
		Headers: make(Header),
	}
}

func (r *Response) Header() Header {
	return r.Headers
}

func (r *Response) Write(buf []byte) (int, error) {
	r.Body.Write(buf)
	return len(buf), nil
}

func (r *Response) WriteHeader(statusCode int) {
	if statusCode < 100 || statusCode > 999 {
		panic(fmt.Sprintf("Недопустимый статус код %v", statusCode))
	}
	r.Status = statusCode
}
