package db

import (
	"waystone-web/models"
)

func SaveSignup(signup models.Signup) error {
	return GetStore().SaveSignup(signup)
}

func GetAllSignups() ([]models.Signup, error) {
	return GetStore().GetAllSignups()
}
