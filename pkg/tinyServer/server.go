package tinyServer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Method string

const (
	GET    Method = "GET"
	POST   Method = "POST"
	PUT    Method = "PUT"
	PATCH  Method = "PATCH"
	DELETE Method = "DELETE"
)

type HTTPServer struct {
	paths map[string]Route
	mu    sync.RWMutex
}

type Route struct {
	handler HTTPHandler
	methods map[Method]struct{}
}

type Pattern struct {
	path    string
	methods map[Method]struct{}
}

type HTTPHandler func(res *Response, req *Request)

func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		paths: make(map[string]Route),
		mu:    sync.RWMutex{},
	}
}

func (s *HTTPServer) HandleFunc(pattern string, handler HTTPHandler) {
	parsedPattern := s.parsePattern(pattern)
	s.registerRoute(parsedPattern, handler)
}

func (s *HTTPServer) ListenAndServe(address string) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		panic(err)
	}

	baseContext := context.Background()

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go s.handleConn(conn, baseContext)
	}
}

func (s *HTTPServer) handleConn(conn net.Conn, _ context.Context) {

	defer conn.Close()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Panic recovered: %v", r)
			conn.Write([]byte("HTTP/1.1 500 Internal Server Error" + "\r\nContent-Length: 0"))
			return
		}
	}()

	request, err := s.readRequest(bufio.NewReader(conn))

	if err != nil {
		log.Println(err)
		return
	}

	path := request.URL.Path
	route, ok := s.paths[path]

	if !ok {
		conn.Write([]byte("HTTP/1.1 404 Not Found" + "\r\nContent-Length: 0"))
		return
	}

	if _, ok := route.methods[Method(request.Method)]; !ok {
		conn.Write([]byte("HTTP/1.1 405 Method Not Allowed" + "\r\nContent-Length: 0"))
		return
	}

	response := NewResponse()
	route.handler(response, request)

	statusText := http.StatusText(response.Status)
	if statusText == "" {
		statusText = "Unknown Status"
	}

	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", response.Status, statusText)
	if _, err := conn.Write([]byte(statusLine)); err != nil {
		panic("ошибка при отправке статусной строки")
	}

	response.Headers.Add("Content-Length", fmt.Sprintf("%d", response.Body.Len()))

	for key, values := range response.Headers {
		for _, value := range values {
			headerLine := fmt.Sprintf("%s: %s\r\n", key, value)
			if _, err := conn.Write([]byte(headerLine)); err != nil {
				panic("ошибка при отправке заголовка")
			}
		}
	}

	if _, err := conn.Write([]byte("\r\n")); err != nil {
		panic("ошибка при отправке разделителя заголовков")
	}

	if response.Body != nil {
		if _, err := io.Copy(conn, response.Body); err != nil {
			panic("ошибка при отправке тела ответа")
		}
	}
}

func (s *HTTPServer) readRequest(reader *bufio.Reader) (*Request, error) {
	requestLine, err := reader.ReadString('\n')

	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать строку запроса: %v", err)
	}

	requestLine = strings.TrimSpace(requestLine)
	parts := strings.Split(requestLine, " ")

	if len(parts) != 3 {
		return nil, fmt.Errorf("неверный формат строки запроса")
	}

	method, rawURL := parts[0], parts[1]

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("не удалось распарсить URL: %v", err)
	}

	headers := make(map[string][]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("ошибка чтения заголовков: %v", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			return nil, fmt.Errorf("неверный формат заголовка: %s", line)
		}
		key := strings.TrimSpace(line[:colonIndex])
		value := strings.TrimSpace(line[colonIndex+1:])

		headers[key] = append(headers[key], value)
	}

	var body io.ReadCloser

	if clValues, ok := headers["Content-Length"]; ok && len(clValues) > 0 {
		contentLength := clValues[len(clValues)-1]
		var length int64
		_, err := fmt.Sscanf(contentLength, "%d", &length)
		if err != nil {
			return nil, errors.New("некорректный Content-Length")
		}
		body = io.NopCloser(io.LimitReader(reader, length))
	} else if teValues, ok := headers["Transfer-Encoding"]; ok && len(teValues) > 0 && strings.ToLower(teValues[len(teValues)-1]) == "chunked" {
		body = io.NopCloser(reader)
	} else {
		body = io.NopCloser(strings.NewReader(""))
	}

	request := &Request{
		Method: method,
		URL:    parsedURL,
		Header: headers,
		Body:   body,
	}

	return request, nil
}

// parse "/users" or "GET /users" patterns
func (s *HTTPServer) parsePattern(pattern string) Pattern {
	if pattern == "" {
		panic("паттерн не найден")
	}

	patternChunks := strings.Split(pattern, " ")
	methods := map[Method]struct{}{GET: {}, POST: {}, PUT: {}, PATCH: {}, DELETE: {}}

	if len(patternChunks) > 2 {
		panic("недопустимый паттерн: " + pattern)
	}

	methodAndPatternExist := func() bool { return len(patternChunks) == 2 }

	if methodAndPatternExist() {
		currentMethod := Method(patternChunks[0])

		deleteUnusedMethods := func() {
			for m := range methods {
				if m != currentMethod {
					delete(methods, m)
				}
			}

			if len(methods) == 0 {
				panic("недопустимый метод: " + pattern)
			}
		}

		deleteUnusedMethods()
		pattern = patternChunks[1]
	}

	return Pattern{pattern, methods}
}

func (s *HTTPServer) registerRoute(pattern Pattern, handler HTTPHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	route := Route{handler, pattern.methods}

	existRoute, ok := s.paths[pattern.path]

	if !ok {
		s.paths[pattern.path] = route
		return
	}

	for m := range pattern.methods {
		if _, exists := existRoute.methods[m]; exists {
			panic("Метод уже существует: " + m)
		}
		existRoute.methods[m] = struct{}{}
	}
}
