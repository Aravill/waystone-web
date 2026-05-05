package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"waystone-web/db"
	"waystone-web/middleware"
	"waystone-web/models"
	"net/http"
	"time"
)

func HandleLoginStart(w http.ResponseWriter, r *http.Request) {
	config := middleware.GetOAuth2Config()
	state := fmt.Sprintf("%d", time.Now().UnixNano())

	authURL := config.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing authorization code", http.StatusBadRequest)
		return
	}

	config := middleware.GetOAuth2Config()
	verifier := middleware.GetVerifier()

	ctx := context.Background()
	token, err := config.Exchange(ctx, code)
	if err != nil {
		log.Printf("failed to exchange token: %v", err)
		http.Error(w, "failed to exchange token", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusInternalServerError)
		return
	}

	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		log.Printf("failed to verify token: %v", err)
		http.Error(w, "failed to verify token", http.StatusInternalServerError)
		return
	}

	var claims middleware.Claims
	if err := idToken.Claims(&claims); err != nil {
		log.Printf("failed to parse claims: %v", err)
		http.Error(w, "failed to parse claims", http.StatusInternalServerError)
		return
	}

	// Check if user exists by email (whitelist check)
	existingUser, err := db.GetUserByEmail(claims.Email)
	if err != nil {
		log.Printf("error checking existing user: %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "error.html", time.Now(), nil)
		return
	}

	if existingUser == nil {
		// User not whitelisted - show error page
		log.Printf("login attempt from non-whitelisted email: %s", claims.Email)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		errorHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Access Denied - Waystone</title>
    <style>
        body {
            background: linear-gradient(135deg, #000000 0%%, #1a1a1a 100%%);
            color: #ffffff;
            font-family: 'Fira Code', monospace;
            margin: 0;
            padding: 20px;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .error-container {
            text-align: center;
            padding: 40px;
            border: 1px solid #333333;
            border-radius: 5px;
            max-width: 500px;
            background: #0d0d0d;
        }
        h1 {
            color: #ff6b6b;
            margin-top: 0;
        }
        p {
            line-height: 1.6;
            margin: 20px 0;
        }
        .email {
            color: #667eea;
            font-weight: bold;
        }
        a {
            display: inline-block;
            margin-top: 20px;
            padding: 12px 30px;
            background: #667eea;
            color: white;
            text-decoration: none;
            border-radius: 5px;
            transition: background 0.3s;
        }
        a:hover {
            background: #764ba2;
        }
    </style>
</head>
<body>
    <div class="error-container">
        <h1>Access Denied</h1>
        <p>Your email <span class="email">%s</span> is not whitelisted.</p>
        <p>Contact an administrator to request access.</p>
        <a href="/login.html">Back to Login</a>
    </div>
</body>
</html>`, claims.Email)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(errorHTML))
		return
	}

	// User is whitelisted - update Google ID and proceed
	user := models.User{
		ID:        existingUser.ID,
		GoogleID:  claims.GoogleID,
		Email:     claims.Email,
		Name:      claims.Name,
		Picture:   claims.Picture,
		CreatedAt: existingUser.CreatedAt,
		UpdatedAt: time.Now(),
		Roles:     existingUser.Roles,
	}

	if err := db.SaveUser(user); err != nil {
		log.Printf("failed to save user: %v", err)
		http.Error(w, "failed to save user", http.StatusInternalServerError)
		return
	}

	if err := middleware.SetSession(w, r, user.ID, user.GoogleID, user.Email, user.Name, user.Picture, user.Roles); err != nil {
		log.Printf("failed to set session: %v", err)
		http.Error(w, "failed to set session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func HandleLogout(w http.ResponseWriter, r *http.Request) {
	if err := middleware.ClearSession(w, r); err != nil {
		log.Printf("failed to clear session: %v", err)
		http.Error(w, "failed to logout", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "logged out"})
}

func HandleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	session, err := middleware.GetSession(r)
	if err != nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	// Get roles from session, default to empty if not present
	roles := []string{}
	if rolesVal, ok := session["roles"].([]string); ok {
		roles = rolesVal
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":  session["user_id"],
		"google_id": session["google_id"],
		"email":     session["email"],
		"name":      session["name"],
		"picture":   session["picture"],
		"roles":     roles,
	})
}
