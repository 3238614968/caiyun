package services

import "caiyun/internal/ws"

func (s *TaskService) sendToUser(userID uint, message ws.Message) {
	if s != nil && s.eventHub != nil {
		s.eventHub.SendToUser(userID, message)
	}
}
