package models

type CampaignStatus string

const (
	Pitch     CampaignStatus = "Pitch"
	Ongoing   CampaignStatus = "Ongoing"
	Finished  CampaignStatus = "Finished"
	OnHiatus  CampaignStatus = "On Hiatus"
	Cancelled CampaignStatus = "Cancelled"
)

type Campaign struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Status             CampaignStatus `json:"status"`
	Summary            string         `json:"summary"`
	Description        string         `json:"description"`
	Players            []string       `json:"players"`
	DM                 string         `json:"dm"`
	DesiredPlayerCoutn string         `json:"string"`
	SignUpsOpen        bool           `json:"sign_ups_open"`
}
