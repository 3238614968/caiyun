package handlers

import "caiyun/internal/services"

type ExchangeHandler struct {
	exchangeService  *services.ExchangeService
	productService   *services.ProductService
	operationService *services.OperationService
	prizeService     *services.PrizeCenterService
}

func NewExchangeHandler(exchangeService *services.ExchangeService, productService *services.ProductService) *ExchangeHandler {
	return &ExchangeHandler{
		exchangeService: exchangeService,
		productService:  productService,
	}
}

func (h *ExchangeHandler) SetOperationService(service *services.OperationService) {
	h.operationService = service
}

func (h *ExchangeHandler) SetPrizeCenterService(service *services.PrizeCenterService) {
	h.prizeService = service
}
