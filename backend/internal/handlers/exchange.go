package handlers

import "caiyun/internal/services"

type ExchangeHandler struct {
	exchangeService  *services.ExchangeService
	productService   *services.ProductService
	operationService *services.OperationService
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
