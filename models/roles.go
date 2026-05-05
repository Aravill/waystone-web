package models

// HasRole checks if a user has a specific role
func (u *User) HasRole(role string) bool {
	if u == nil || u.Roles == nil {
		return false
	}
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if a user has any of the provided roles
func (u *User) HasAnyRole(roles ...string) bool {
	if u == nil || u.Roles == nil {
		return false
	}
	for _, requiredRole := range roles {
		for _, userRole := range u.Roles {
			if userRole == requiredRole {
				return true
			}
		}
	}
	return false
}

// HasAllRoles checks if a user has all of the provided roles
func (u *User) HasAllRoles(roles ...string) bool {
	if u == nil || u.Roles == nil {
		return false
	}
	for _, requiredRole := range roles {
		found := false
		for _, userRole := range u.Roles {
			if userRole == requiredRole {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsAdmin checks if a user has admin role
func (u *User) IsAdmin() bool {
	return u.HasRole("admin")
}

// IsDungeonMaster checks if a user has dungeon-master role
func (u *User) IsDungeonMaster() bool {
	return u.HasRole("dungeon-master")
}

// Defined roles
const (
	RoleUser          = "user"
	RoleAdmin         = "admin"
	RoleDungeonMaster = "dungeon-master"
)
