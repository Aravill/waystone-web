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
	if err := db.Initialize(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}

	callbackURL := os.Getenv("OAUTH_CALLBACK_URL")
	if callbackURL == "" {
		callbackURL = fmt.Sprintf("http://localhost:%s/auth/callback", port)
	}

	if err := middleware.InitAuth(callbackURL); err != nil {
		log.Printf("Warning: OAuth not fully configured: %v\n", err)
		log.Println("OAuth features will not be available until GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are set")
	}

	api.RegisterRoutes()

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Server starting on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

