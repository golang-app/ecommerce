package layout

import (
	"log"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
)

func (handler httpHandler) AllShipmentAddresses(w http.ResponseWriter, r *http.Request) {
	addresses, err := handler.shipmentSrv.List(r.Context(), "")
	if err != nil {
		https.InternalError(w, "internal-error", "cannot get list of all shipment addresses")
		log.Printf("cannot get list of all shipment addresses: %s", err)
		return
	}

	handler.renderTemplate(w, r, "shipment/addressList", map[string]interface{}{
		"Addresses": addresses,
	})
}

func (handler httpHandler) NewShipment(w http.ResponseWriter, r *http.Request) {
	handler.renderTemplate(w, r, "shipment/addressNew", map[string]interface{}{})
}
