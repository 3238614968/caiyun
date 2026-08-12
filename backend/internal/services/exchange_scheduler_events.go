package services

import "caiyun/internal/ws"

func (s *ExchangeScheduler) sendToUser(userID uint, message ws.Message) {
	if s != nil && s.hub != nil {
		s.hub.SendToUser(userID, message)
	}
}

func (s *ExchangeScheduler) broadcast(message ws.Message) {
	if s != nil && s.hub != nil {
		s.hub.Broadcast(message)
	}
}
