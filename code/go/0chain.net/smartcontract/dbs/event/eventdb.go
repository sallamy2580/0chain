package event

import (
	"time"

	"0chain.net/chaincore/config"
	"0chain.net/core/common"
	"0chain.net/smartcontract/dbs"
	"0chain.net/smartcontract/dbs/postgresql"
)

const DefaultQueryTimeout = 5 * time.Second

func NewEventDb(config config.DbAccess) (*EventDb, error) {
	db, err := postgresql.GetPostgresSqlDb(config)
	if err != nil {
		return nil, err
	}
	eventDb := &EventDb{
		Store:         db,
		eventsChannel: make(chan blockEvents, 1),
	}
	go eventDb.addEventsWorker(common.GetRootContext())

	if err := eventDb.AutoMigrate(); err != nil {
		return nil, err
	}
	return eventDb, nil
}

type EventDb struct {
	dbs.Store
	eventsChannel chan blockEvents
}

type blockEvents struct {
	block     string
	blockSize int
	round     int64
	events    []Event
}

func (edb *EventDb) AutoMigrate() error {
	if err := edb.Store.Get().AutoMigrate(
		&Event{},
		&Blobber{},
		&User{},
		&Transaction{},
		&WriteMarker{},
		&Validator{},
		&ReadMarker{},
		&Block{},
		&Error{},
		&Miner{},
		&Sharder{},
		&Curator{},
		&DelegatePool{},
		&Allocation{},
		&AllocationTerm{},
		&Reward{},
		&Authorizer{},
		&Challenge{},
		&AllocationBlobberTerm{},
		ChallengePool{},
	); err != nil {
		return err
	}
	return nil
}
