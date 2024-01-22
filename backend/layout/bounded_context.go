package layout

import (
	_ "embed"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/sirupsen/logrus"
)

func New(logger logrus.FieldLogger, cartSrv cartService, catalogSrv catalogService, shipmentSrv shipmentService, authSrv authService) application.BoundedContext {
	return &boundedContext{
		handler: httpHandler{
			cartSrv:     cartSrv,
			catalogSrv:  catalogSrv,
			authSrv:     authSrv,
			shipmentSrv: shipmentSrv,
		},
		logger: logger,
	}
}

type boundedContext struct {
	handler httpHandler
	logger  logrus.FieldLogger
}
