package tables

import "time"

// TableReasoningEffortCuration is the local, hand-curated reasoning-effort
// entry. One row per provider and model; no user, key, scope, or pattern.
type TableReasoningEffortCuration struct {
	Provider      string    `gorm:"primaryKey;type:varchar(255);not null" json:"provider"`
	Model         string    `gorm:"primaryKey;type:varchar(255);not null" json:"model"`
	AliasesJSON   string    `gorm:"type:text" json:"-"`
	UpstreamField string    `gorm:"type:varchar(255);not null" json:"upstream_field"`
	LevelsJSON    string    `gorm:"type:text;not null" json:"-"`
	LevelMapJSON  string    `gorm:"type:text;not null" json:"-"`
	CreatedAt     time.Time `gorm:"index;not null" json:"created_at"`
	UpdatedAt     time.Time `gorm:"index;not null" json:"updated_at"`
}

func (TableReasoningEffortCuration) TableName() string { return "governance_reasoning_effort_curation" }
