package db

import "waystone-web/models"

func GetUserRights(userID string) ([]string, error)  { return GetStore().GetUserRights(userID) }
func GetAllUserRights() (map[string][]string, error) { return GetStore().GetAllUserRights() }
func SetUserRight(userID, action string, on bool) error {
	return GetStore().SetUserRight(userID, action, on)
}
func GetAllCodexAccess() ([]models.CodexAccess, error) { return GetStore().GetAllCodexAccess() }
func GetCodexAccessForUser(userID string) (map[string]string, error) {
	return GetStore().GetCodexAccessForUser(userID)
}
func SetCodexAccess(codexID, userID, level string) error {
	return GetStore().SetCodexAccess(codexID, userID, level)
}

func GetPageCodexID(pageID string) (string, error) { return GetStore().GetPageCodexID(pageID) }
func GetImageCodexID(file string) (string, error)  { return GetStore().GetImageCodexID(file) }
