package api

import (
	"encoding/json"
	"fmt"
	"waystone-web/db"
	"net/http"
)

func HandleGetEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	events, err := db.GetAllEvents()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve events"}`)
		return
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(events)
	w.Write(data)
}
