package event

import (
	"gorm.io/gorm/clause"
)

type ChallengePool struct {
	ID           string `gorm:"primarykey"`
	AllocationID string `gorm:"uniqueIndex"`
	Balance      int64  `json:"balance"`
	StartTime    int64  `json:"start_time"`
	Expiration   int64  `json:"expiration"`
	Finalized    bool   `json:"finalized"`
}

func (edb *EventDb) addOrUpdateChallengePool(cp ChallengePool) error {
	updateFields := []string{"balance", "start_time", "expiration", "finalized"}

	return edb.Store.Get().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(updateFields), // column needed to be updated
	}).Create(&cp).Error
}

func (edb *EventDb) GetChallengePool(allocationID string) (*ChallengePool, error) {
	var cp ChallengePool
	return &cp, edb.Store.Get().Model(&ChallengePool{}).
		Where("allocation_id = ?", allocationID).
		Take(&cp).Error
}
