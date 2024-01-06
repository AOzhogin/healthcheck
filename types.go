package healthcheck

import (
	"context"
	"sync"
	"time"
)

type (
	CheckContext struct {
		Func  HCFunc
		Notes string
	}
	HCFunc func(ctx context.Context) error

	checkResult struct {
		LastAction time.Time `json:"time"`
		Result     string    `json:"result,omitempty"`
		Status     string    `json:"status"`
		Time       float64   `json:"exec"`
		Notes      string    `json:"notes"`
	}

	checkResults struct {
		code   int                    `json:"-"`
		Status string                 `json:"status"`
		Checks map[string]checkResult `json:"checks"`
	}

	checkList struct {
		List map[string]CheckContext
		sync.Mutex
	}

	resultError struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Checks any    `json:"checks"`
	}
)

func newCheckList() checkList {
	return checkList{
		List:  make(map[string]CheckContext),
		Mutex: sync.Mutex{},
	}
}
