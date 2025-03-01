package cmd

import (
	cstate "0chain.net/chaincore/chain/state"
	"0chain.net/chaincore/currency"
	"0chain.net/chaincore/state"
	"0chain.net/chaincore/transaction"
	"0chain.net/smartcontract/minersc"
	"github.com/0chain/common/core/util"
)

func mockUpdateState(txn *transaction.Transaction, balances cstate.StateContextI) {
	_ = balances.AddTransfer(state.NewTransfer(
		txn.ClientID, txn.ToClientID, txn.Value),
	)
	_ = balances.AddTransfer(state.NewTransfer(
		txn.ClientID, minersc.ADDRESS, txn.Fee),
	)

	clientState := balances.GetState()
	for _, transfer := range balances.GetTransfers() {
		mockTransferAmount(
			[]byte(transfer.ClientID),
			[]byte(transfer.ToClientID),
			transfer.Amount,
			clientState,
			balances,
		)
	}

	for _, signedTransfer := range balances.GetSignedTransfers() {
		mockTransferAmount(
			[]byte(signedTransfer.ClientID),
			[]byte(signedTransfer.ToClientID),
			signedTransfer.Amount,
			clientState,
			balances,
		)
	}

	for _, mint := range balances.GetMints() {
		mockMint(
			[]byte(mint.ToClientID),
			mint.Amount,
			clientState,
			balances,
		)
	}
}

func mockMint(
	to []byte,
	amount currency.Coin,
	clientState util.MerklePatriciaTrieI,
	balances cstate.StateContextI,
) {
	toState := state.State{}
	err := clientState.GetNodeValue(util.Path(to), &toState)
	if err != nil {
		return
	}
	_ = balances.SetStateContext(&toState)

	newBal, err := currency.AddCoin(toState.Balance, amount)
	if err != nil {
		return
	}
	toState.Balance = newBal
	_, _ = clientState.Insert(util.Path(to), &toState)
}

func mockTransferAmount(
	from, to []byte,
	amount currency.Coin,
	clientState util.MerklePatriciaTrieI,
	balances cstate.StateContextI,
) {
	fromState := state.State{}
	err := clientState.GetNodeValue(util.Path(from), &fromState)
	if err != nil && err != util.ErrValueNotPresent {
		return
	}
	if err != util.ErrValueNotPresent {
		_ = balances.SetStateContext(&fromState)
		fromState.Balance -= amount
		_, _ = clientState.Insert(util.Path(from), &fromState)
	}

	toState := state.State{}
	err = clientState.GetNodeValue(util.Path(to), &toState)
	if err != nil {
		return
	}
	_ = balances.SetStateContext(&toState)

	newBal, err := currency.AddCoin(toState.Balance, amount)
	if err != nil {
		return
	}
	toState.Balance = newBal
	_, _ = clientState.Insert(util.Path(to), &toState)
}
