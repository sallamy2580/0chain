package event

import (
	"errors"
	"fmt"

	"0chain.net/chaincore/currency"
	common2 "0chain.net/smartcontract/common"
	"gorm.io/gorm/clause"

	"github.com/guregu/null"
	"gorm.io/gorm"

	"0chain.net/core/common"
	"0chain.net/smartcontract/dbs"
)

type Sharder struct {
	gorm.Model
	SharderID         string `gorm:"uniqueIndex"`
	N2NHost           string `gorm:"column:n2n_host"`
	Host              string
	Port              int
	Path              string
	PublicKey         string
	ShortName         string
	BuildTag          string
	TotalStaked       currency.Coin
	Delete            bool
	DelegateWallet    string
	ServiceCharge     float64
	NumberOfDelegates int
	MinStake          currency.Coin
	MaxStake          currency.Coin
	LastHealthCheck   common.Timestamp
	Rewards           currency.Coin
	TotalReward       currency.Coin
	Fees              currency.Coin
	Active            bool
	Longitude         float64
	Latitude          float64
}

// swagger:model SharderGeolocation
type SharderGeolocation struct {
	SharderID string  `json:"sharder_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (edb *EventDb) GetSharder(id string) (Sharder, error) {

	var sharder Sharder

	return sharder, edb.Store.Get().
		Model(&Sharder{}).
		Where(&Sharder{SharderID: id}).
		First(&sharder).Error
}

func (edb *EventDb) GetShardersFromQuery(query *Sharder) ([]Sharder, error) {

	var sharders []Sharder

	result := edb.Store.Get().
		Model(&Sharder{}).
		Where(query).
		Find(&sharders)

	return sharders, result.Error
}

func (edb *EventDb) GetSharders() ([]Sharder, error) {

	var sharders []Sharder

	result := edb.Store.Get().
		Model(&Sharder{}).
		Find(&sharders)

	return sharders, result.Error
}

func (edb *EventDb) CountActiveSharders() (int64, error) {

	var count int64

	result := edb.Store.Get().
		Model(&Sharder{}).
		Where("active = ?", true).
		Count(&count)

	return count, result.Error
}

func (edb *EventDb) CountInactiveSharders() (int64, error) {

	var count int64

	result := edb.Store.Get().
		Model(&Sharder{}).
		Where("active = ?", false).
		Count(&count)

	return count, result.Error
}

func (edb *EventDb) GetShardersTotalStake() (int64, error) {
	var count int64

	err := edb.Store.Get().Table("sharders").Select("sum(total_staked)").Row().Scan(&count)
	return count, err
}

func (edb *EventDb) addSharder(sharder Sharder) error {

	result := edb.Store.Get().Create(&sharder)

	return result.Error
}

func (edb *EventDb) sharderAggregateStats(id string) (*providerAggregateStats, error) {
	var sharder providerAggregateStats
	result := edb.Store.Get().
		Model(&Sharder{}).
		Where(&Sharder{SharderID: id}).
		First(&sharder)
	if result.Error != nil {
		return nil, fmt.Errorf("error retrieving sharder %v, error %v",
			id, result.Error)
	}

	return &sharder, nil
}

func (edb *EventDb) overwriteSharder(sharder Sharder) error {

	result := edb.Store.Get().
		Model(&Sharder{}).
		Where(&Sharder{SharderID: sharder.SharderID}).
		Updates(map[string]interface{}{
			"n2n_host":            sharder.N2NHost,
			"host":                sharder.Host,
			"port":                sharder.Port,
			"path":                sharder.Path,
			"public_key":          sharder.PublicKey,
			"short_name":          sharder.ShortName,
			"build_tag":           sharder.BuildTag,
			"total_staked":        sharder.TotalStaked,
			"delete":              sharder.Delete,
			"delegate_wallet":     sharder.DelegateWallet,
			"service_charge":      sharder.ServiceCharge,
			"number_of_delegates": sharder.NumberOfDelegates,
			"min_stake":           sharder.MinStake,
			"max_stake":           sharder.MaxStake,
			"last_health_check":   sharder.LastHealthCheck,
			"rewards":             sharder.Rewards,
			"fees":                sharder.Fees,
			"active":              sharder.Active,
			"longitude":           sharder.Longitude,
			"latitude":            sharder.Latitude,
		})

	return result.Error
}

func (edb *EventDb) addOrOverwriteSharder(sharder Sharder) error {

	exists, err := sharder.exists(edb)
	if err != nil {
		return err
	}
	if exists {
		return edb.overwriteSharder(sharder)
	}

	err = edb.addSharder(sharder)

	return err
}

func (sh *Sharder) exists(edb *EventDb) (bool, error) {

	var sharder Sharder

	result := edb.Get().
		Model(&Sharder{}).
		Where(&Sharder{SharderID: sh.SharderID}).
		Take(&sharder)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil
	} else if result.Error != nil {
		return false, fmt.Errorf("error searching for sharder %v, error %v",
			sh.SharderID, result.Error)
	}

	return true, nil
}

type SharderQuery struct {
	gorm.Model
	SharderID         null.String
	N2NHost           null.String
	Host              null.String
	Port              null.Int
	Path              null.String
	PublicKey         null.String
	ShortName         null.String
	BuildTag          null.String
	TotalStaked       null.Int
	Delete            null.Bool
	DelegateWallet    null.String
	ServiceCharge     null.Float
	NumberOfDelegates null.Int
	MinStake          null.Int
	MaxStake          null.Int
	LastHealthCheck   null.Int
	Rewards           null.Int
	Fees              null.Int
	Active            null.Bool
	Longitude         null.Int
	Latitude          null.Int
}

func (edb *EventDb) GetShardersWithFilterAndPagination(filter SharderQuery, p common2.Pagination) ([]Sharder, error) {
	var sharders []Sharder
	query := edb.Get().Model(&Sharder{}).Where(&filter).Offset(p.Offset).Limit(p.Limit).Order(clause.OrderByColumn{
		Column: clause.Column{Name: "created_at"},
		Desc:   p.IsDescending,
	})
	return sharders, query.Scan(&sharders).Error
}

func (edb *EventDb) GetSharderGeolocations(filter SharderQuery, p common2.Pagination) ([]SharderGeolocation, error) {
	var sharderLocations []SharderGeolocation
	query := edb.Get().Model(&Sharder{}).Where(&filter).Offset(p.Offset).Limit(p.Limit).Order(clause.OrderByColumn{
		Column: clause.Column{Name: "created_at"},
		Desc:   p.IsDescending,
	})

	result := query.Scan(&sharderLocations)

	return sharderLocations, result.Error
}

func (edb *EventDb) updateSharder(updates dbs.DbUpdates) error {

	var sharder = Sharder{SharderID: updates.Id}
	exists, err := sharder.exists(edb)

	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("sharder %v not in database cannot update",
			sharder.SharderID)
	}

	result := edb.Store.Get().
		Model(&Sharder{}).
		Where(&Sharder{SharderID: sharder.SharderID}).
		Updates(updates.Updates)

	return result.Error
}

func (edb *EventDb) deleteSharder(id string) error {

	result := edb.Store.Get().
		Where(&Sharder{SharderID: id}).
		Delete(&Sharder{})

	return result.Error
}
