package httpServer

import (
	"github.com/koestler/go-mqtt-to-influxdb/statistics"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

type HttpServer struct {
	config Config
}

type Config interface {
	Bind() string
	Port() int
	LogRequests() bool
}

func Run(config Config, env *Environment) (httpServer *HttpServer) {
	var logger io.Writer
	if config.LogRequests() {
		logger = os.Stdout
	}

	go func() {
		router := newRouter(logger, env)
		address := config.Bind() + ":" + strconv.Itoa(config.Port())

		log.Printf("httpServer: listening on %v", address)
		log.Fatal(router, http.ListenAndServe(address, router))
	}()

	return &HttpServer{
		config: config,
	}
}

// Our application wide data containers
type Environment struct {
	Statistics *statistics.Statistics
}

// Error represents a handler error. It provides methods for a HTTP status
// code and embeds the built-in error interface.
type Error interface {
	error
	Status() int
}

// StatusError represents an error with an associated HTTP status code.
type StatusError struct {
	Code int
	Err  error
}

// Allows StatusError to satisfy the error interface.
func (statusError StatusError) Error() string {
	return statusError.Err.Error()
}

// Returns our HTTP status code.
func (statusError StatusError) Status() int {
	return statusError.Code
}

// define an extended version of http.HandlerFunc
type HandlerHandleFunc func(e *Environment, w http.ResponseWriter, r *http.Request) Error

// The Handler struct that takes a configured Environment and a function matching
// our useful signature.
type Handler struct {
	Env    *Environment
	Handle HandlerHandleFunc
}

// ServeHTTP allows our Handler type to satisfy httpServer.Handler.
func (handler Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	err := handler.Handle(handler.Env, w, r)

	if err != nil {
		log.Printf("ServeHTTP err=%v", err)

		switch e := err.(type) {
		case Error:
			// We can retrieve the status here and write out a specific
			// HTTP status code.
			log.Printf("HTTP %d - %s", e.Status(), e)
			http.Error(w, http.StatusText(e.Status()), e.Status())
			return
		default:
			// Any error types we don't specifically look out for default
			// to serving a HTTP 500
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		}
	}
}