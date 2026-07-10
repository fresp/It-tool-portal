package models

import "time"

// AuditLog records every token exchange attempt (success + failure).
type AuditLog struct {
	ID        string    `bson:"_id,omitempty" json:"id"`
	UserID    string    `bson:"user_id" json:"user_id"`
	ToolID    string    `bson:"tool_id" json:"tool_id"`
	IP        string    `bson:"ip" json:"ip"`
	UserAgent string    `bson:"user_agent" json:"user_agent"`
	Result    string    `bson:"result" json:"result"` // "success" or "denied"
	Reason    string    `bson:"reason,omitempty" json:"reason,omitempty"`
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
}
