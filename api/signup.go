package api

import (
	"encoding/json"
	"fmt"
	"waystone-web/db"
	"waystone-web/models"
	"net/http"
)

func HandleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	var signup models.Signup
	err := json.NewDecoder(r.Body).Decode(&signup)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"error": "invalid request"}`)
		return
	}

	if err := db.SaveSignup(signup); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to save signup"}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, `{"status": "success", "message": "Signup received"}`)
}
