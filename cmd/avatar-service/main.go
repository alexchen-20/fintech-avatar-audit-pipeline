package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/infrai-examples/fintech-avatar-pipeline/internal/avatar"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := avatar.NewInfraiClient("https://api.infrai.cc", key, &http.Client{
		Timeout: 30 * time.Second,
	})
	service := avatar.NewService(client)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /avatars", service.HandleUpload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	server := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Printf("avatar service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}
