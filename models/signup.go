package models

type Signup struct {
	Event string `json:"event"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}
