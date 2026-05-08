package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"waystone-web/db"
)

func HandleGetCampaigns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	campaigns, err := db.GetAllCampaigns()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, `{"error": "failed to retrieve campaigns"}`)
		return
	}

	w.WriteHeader(http.StatusOK)
	data, _ := json.Marshal(campaigns)
	w.Write(data)
}
