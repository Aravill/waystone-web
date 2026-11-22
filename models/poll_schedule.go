package models

// PollSlot is one date of a repeating poll's pattern: a date (YYYY-MM-DD), start time (HH:MM)
// and duration in minutes, taken from the source poll when Repeat was turned on.
type PollSlot struct {
	Date     string `json:"date"`
	Time     string `json:"time"`
	Duration int    `json:"duration"`
}

// PollSchedule makes a game post a new poll every EveryWeeks weeks, LeadDays days ahead.
// Cycle k is the slots shifted by k*EveryWeeks*7 days; NextCycle is the next one to handle.
type PollSchedule struct {
	CampaignID string     `json:"campaign_id"`
	EveryWeeks int        `json:"every_weeks"`
	LeadDays   int        `json:"lead_days"`
	Timezone   string     `json:"timezone"`
	Slots      []PollSlot `json:"slots"`
	NextCycle  int        `json:"next_cycle"`
	Paused     bool       `json:"paused"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
}
