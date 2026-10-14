package db

import "waystone-web/models"

func GetAllSystems() ([]models.System, error) {
	return GetStore().GetAllSystems()
}

func CreateSystem(sys models.System) error {
	return GetStore().CreateSystem(sys)
}

func RenameSystem(id, name string) error {
	return GetStore().RenameSystem(id, name)
}

func DeleteSystem(id string) error {
	return GetStore().DeleteSystem(id)
}

func SetSystemLogo(id string, data []byte, contentType string) error {
	return GetStore().SetSystemLogo(id, data, contentType)
}

func GetSystemLogo(id string) ([]byte, string, error) {
	return GetStore().GetSystemLogo(id)
}
