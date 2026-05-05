package api

import (
	"net/http"
	"os"
	"path/filepath"
	"waystone-web/middleware"
)

// ServePageWithFallback returns an HTTP handler that serves a specific page
// If the page doesn't exist, it serves index.html as fallback for SPA routing
func ServePageWithFallback(pageName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Construct the file path
		filePath := filepath.Join("./static", pageName)

		// Check if file exists
		if _, err := os.Stat(filePath); err == nil {
			// File exists, serve it
			http.ServeFile(w, r, filePath)
		} else {
			// File doesn't exist, return 404
			http.NotFound(w, r)
		}
	}
}

func RegisterRoutes() {
	// Public auth endpoints
	http.HandleFunc("/auth/login", HandleLoginStart)
	http.HandleFunc("/auth/callback", HandleCallback)
	http.HandleFunc("/auth/logout", HandleLogout)
	http.HandleFunc("/auth/current-user", HandleGetCurrentUser)

	// Protected API endpoints (wrapped with auth middleware)
	http.Handle("/api/events", middleware.AuthMiddleware(http.HandlerFunc(HandleGetEvents)))
	http.Handle("/api/signup", middleware.AuthMiddleware(http.HandlerFunc(HandleSignup)))

	// Protected role management endpoints (wrapped with auth middleware)
	http.Handle("/api/roles", middleware.AuthMiddleware(http.HandlerFunc(HandleAssignRoles)))
	http.Handle("/api/user-roles", middleware.AuthMiddleware(http.HandlerFunc(HandleGetUserRoles)))
	http.Handle("/api/users", middleware.AuthMiddleware(http.HandlerFunc(HandleListUsers)))

	// Protected user management endpoints
	http.Handle("/api/users/create", middleware.AuthMiddleware(http.HandlerFunc(HandleCreateUser)))

	// Static files - MUST be registered BEFORE "/" handler to take precedence
	fs := http.FileServer(http.Dir("./static"))
	
	// Individual static file handlers for CSS, JS (no auth required)
	http.HandleFunc("/dashboard.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/dashboard.css")
	})
	http.HandleFunc("/dashboard.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/dashboard.js")
	})
	http.HandleFunc("/styles.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/styles.css")
	})
	http.HandleFunc("/script.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/script.js")
	})
	http.HandleFunc("/login.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/login.html")
	})
	
	// /static/ prefix for any other static assets
	http.Handle("/static/", http.StripPrefix("/static/", fs))

	// Protected page routes - serve authenticated pages
	http.Handle("/", middleware.AuthMiddleware(http.HandlerFunc(ServePageWithFallback("dashboard.html"))))
	http.Handle("/campaigns", middleware.AuthMiddleware(http.HandlerFunc(ServePageWithFallback("campaigns.html"))))
}

