package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"waystone-web/api"
	"waystone-web/config"
	"waystone-web/db"
	"waystone-web/middleware"
)

func main() {
	middleware.InitAuth()

	if err := db.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}

	api.RegisterRoutes()

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
