package model

// Role represents a user's access level.
type Role string

const (
	RoleDM     Role = "dm"
	RolePlayer Role = "player"
)

// User is stored in the "users" collection.
// No createdAt field — consistent with all other resources which expose only updatedAt.
type User struct {
	ID           string `bson:"_id"          json:"id"`
	Email        string `bson:"email"        json:"email"`
	PasswordHash string `bson:"passwordHash" json:"-"`
	Role         Role   `bson:"role"         json:"role"`
}
