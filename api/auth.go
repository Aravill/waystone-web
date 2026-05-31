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
		// User not whitelisted - redirect to not-whitelisted page
		log.Printf("login attempt from non-whitelisted email: %s", claims.Email)
		http.Redirect(w, r, "/not-whitelisted.html", http.StatusSeeOther)
		return
	}

	if existingUser.Blocked {
		log.Printf("login attempt from blocked user: %s", claims.Email)
		http.Redirect(w, r, "/blocked.html", http.StatusSeeOther)
		return
	}

	// User is whitelisted - update Google ID and proceed
	user := models.User{
		ID:        existingUser.ID,
		GoogleID:  claims.GoogleID,
		Email:     claims.Email,
		Name:      claims.Name,
		Nickname:  existingUser.Nickname,
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

	userID, ok := session["user_id"].(string)
	if !ok {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}

	// Reload fresh user data from DB
	user, err := db.GetUserByID(userID)
	if err != nil {
		log.Printf("error fetching user: %v", err)
		http.Error(w, "failed to fetch user", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User was deleted - clear session and return 401
		middleware.ClearSession(w, r)
		http.Error(w, "user not found", http.StatusUnauthorized)
		return
	}

	if user.Blocked {
		middleware.ClearSession(w, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "account blocked"})
		return
	}

	// Compute display_name: nickname > name > email
	displayName := user.Nickname
	if displayName == "" {
		displayName = user.Name
	}
	if displayName == "" {
		displayName = user.Email
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":      user.ID,
		"google_id":    user.GoogleID,
		"email":        user.Email,
		"name":         user.Name,
		"nickname":     user.Nickname,
		"display_name": displayName,
		"picture":      user.Picture,
		"roles":        user.Roles,
	})
}
