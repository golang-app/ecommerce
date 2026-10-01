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

// Healthy reports process liveness. In container orchestrators (e.g. Kubernetes),
// liveness failure causes the pod to be killed and restarted. To prevent pod restart
// storms during transient database outages, Healthy does not ping external network resources.
func (p sqlDep) Healthy(ctx context.Context) bool {
	return true
}

// Ready reports whether the database is accessible to serve traffic.
// If the database ping fails, Ready returns false so orchestrators
// can temporarily take this pod out of the load balancer rotation without killing it.
func (p sqlDep) Ready(ctx context.Context) bool {
	err := p.pinger.PingContext(ctx)
	return err == nil
}

func (p sqlDep) Close() error {
	return p.pinger.Close()
}
