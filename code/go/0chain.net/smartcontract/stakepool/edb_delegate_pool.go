package stakepool

import (
	"0chain.net/smartcontract/stakepool/spenum"

	cstate "0chain.net/chaincore/chain/state"
	"0chain.net/smartcontract/dbs"
	"0chain.net/smartcontract/dbs/event"
)

type DelegatePoolUpdate dbs.DelegatePoolUpdate

func newDelegatePoolUpdate(poolID, pId string, pType spenum.Provider) *DelegatePoolUpdate {
	var spu DelegatePoolUpdate
	spu.PoolId = poolID
	spu.ProviderId = pId
	spu.ProviderType = int(pType)
	spu.Updates = make(map[string]interface{})
	return &spu
}

func (dp DelegatePool) emitNew(
	poolId, providerId string,
	providerType spenum.Provider,
	balances cstate.StateContextI,
) {
	data := &event.DelegatePool{
		Balance:      dp.Balance,
		PoolID:       poolId,
		ProviderType: int(providerType),
		ProviderID:   providerId,
		DelegateID:   dp.DelegateID,

		Status:       int(dp.Status),
		RoundCreated: balances.GetBlock().Round,
	}

	balances.EmitEvent(
		event.TypeStats,
		event.TagAddOrOverwriteDelegatePool,
		providerId,
		data,
	)
}

func (dpu DelegatePoolUpdate) emitUpdate(
	balances cstate.StateContextI,
) error {

	balances.EmitEvent(
		event.TypeStats,
		event.TagUpdateDelegatePool,
		dpu.PoolId,
		delegatePoolUpdateToDbsDelegatePoolUpdate(dpu),
	)
	return nil
}

func delegatePoolUpdateToDbsDelegatePoolUpdate(dpu DelegatePoolUpdate) dbs.DelegatePoolUpdate {
	return dbs.DelegatePoolUpdate{
		DelegatePoolId: dpu.DelegatePoolId,
		Updates:        dpu.Updates,
	}
}
