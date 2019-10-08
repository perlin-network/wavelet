package main

import (
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/cmd/tui/tui/errdialog"
	"github.com/perlin-network/wavelet/cmd/tui/tui/filechooser"
	"github.com/perlin-network/wavelet/cmd/tui/tui/forms"
)

// this file (ab)uses global states from main.go

func keyStatus() {
	srv.Status()
}

func keyPay() {
	var (
		recipient       [wavelet.SizeAccountID]byte
		amount          int
		gasLimit        int
		additionalBytes string
	)

	form := forms.New()
	form.Add(
		getRecipientFormPair(recipient),
		forms.IntPair("Amount", &amount),
		forms.IntPair("Gas Limit", &gasLimit),
		forms.StringPair("Additional Bytes", &additionalBytes),
	)

	if !form.Spawn() {
		return
	}

	if _, err := srv.Pay(
		recipient, amount, gasLimit,
		[]byte(additionalBytes),
	); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}

func keyFind() {
	var address string

	pair := forms.StringPair("Address", &address)
	pair.Completer = srvCompletion
	pair.Validator = forms.ORValidators(
		forms.LetterValidator(), forms.IntValidator(),
	)

	form := forms.New()
	form.Add(pair)

	if !form.Spawn() {
		return
	}

	if err := srv.Find(address); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}

func keySpawn() {
	path := filechooser.Spawn()
	if path == "" {
		return
	}

	if err := srv.Spawn(path); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}

func keyPlaceStake() {
	var amount int

	form := forms.New()
	form.Add(forms.IntPair("Amount", &amount))

	if !form.Spawn() {
		return
	}

	if err := srv.PlaceStake(amount); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}

func keyWithdrawStake() {
	var amount int

	form := forms.New()
	form.Add(forms.IntPair("Amount", &amount))

	if !form.Spawn() {
		return
	}

	if err := srv.WithdrawStake(amount); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}

func keyWithdrawReward() {
	var amount int

	form := forms.New()
	form.Add(forms.IntPair("Amount", &amount))

	if !form.Spawn() {
		return
	}

	if err := srv.WithdrawReward(amount); err != nil {
		errdialog.CallDialog(err.Error(), nil)
	}
}
