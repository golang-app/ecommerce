package dependency

import (
	"context"
	"io"
)

type sqlDep struct {
	pinger pinger
}

type pinger interface {
	io.Closer
	PingContext(context.Context) error
}

func NewSQL(pinger pinger) sqlDep {
	return sqlDep{pinger: pinger}
}

func (p sqlDep) Healthy(ctx context.Context) bool {
	err := p.pinger.PingContext(ctx)
	return err != nil
}

func (p sqlDep) Ready(ctx context.Context) bool {
	err := p.pinger.PingContext(ctx)
	return err != nil
}

func (p sqlDep) Close() error {
	return p.pinger.Close()
}
