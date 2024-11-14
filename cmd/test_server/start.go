package main

import (
	"fmt"
	"io"
	"tiny_http1.1_server/pkg/tinyServer"
)

func main() {
	server := tinyServer.NewHTTPServer()

	server.HandleFunc("/", helloHandler)
	server.HandleFunc("GET /users", helloHandler)
	server.HandleFunc("POST /users", helloHandler)

	server.ListenAndServe(":8888")
}

func helloHandler(res *tinyServer.Response, req *tinyServer.Request) {
	fmt.Println(req.Method)
	fmt.Println(req.Header)
	fmt.Println(req.URL.Query())

	res.Write([]byte("hello world\n"))

	b, _ := io.ReadAll(req.Body)
	fmt.Fprintf(res, "Тело запроса: %s", b)
}
