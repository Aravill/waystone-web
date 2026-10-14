package models

type CampaignStatus string

const (
	Pitch     CampaignStatus = "Pitch"
	Ongoing   CampaignStatus = "Ongoing"
	Finished  CampaignStatus = "Finished"
	OnHiatus  CampaignStatus = "On Hiatus"
	Cancelled CampaignStatus = "Cancelled"
)

type CampaignType string

const (
	OneShot      CampaignType = "One-shot"
	Adventure    CampaignType = "Adventure"
	LongCampaign CampaignType = "Campaign"
)

// IsValid reports whether t is a known campaign type.
func (t CampaignType) IsValid() bool {
	switch t {
	case OneShot, Adventure, LongCampaign:
		return true
	}
	return false
}

type Campaign struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Status             CampaignStatus `json:"status"`
	Type               CampaignType   `json:"type"`
	Summary            string         `json:"summary"`
	Description        string         `json:"description"`
	Players            []string       `json:"players"`
	DM                 string         `json:"dm"`
	DesiredPlayerCount string         `json:"desired_player_count"`
	SignUpsOpen        bool           `json:"sign_ups_open"`
}

// CampaignStatusTransitions maps each status to the statuses reachable from it.
var CampaignStatusTransitions = map[CampaignStatus][]CampaignStatus{
	Pitch:     {Ongoing, Cancelled},
	Ongoing:   {OnHiatus, Finished, Cancelled},
	OnHiatus:  {Ongoing, Finished, Cancelled},
	Finished:  {Ongoing},
	Cancelled: {Ongoing},
}

// AllowedTransitions returns the statuses reachable from s (never nil, for JSON).
func (s CampaignStatus) AllowedTransitions() []CampaignStatus {
	if next, ok := CampaignStatusTransitions[s]; ok {
		return next
	}
	return []CampaignStatus{}
}

// CanTransitionTo reports whether s may change to next.
func (s CampaignStatus) CanTransitionTo(next CampaignStatus) bool {
	for _, n := range CampaignStatusTransitions[s] {
		if n == next {
			return true
		}
	}
	return false
}
