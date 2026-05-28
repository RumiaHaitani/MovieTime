package room

import (
	"context"
	"errors"
	"movietime-backend/internal/models"
	"movietime-backend/internal/utils"
)

type Service struct {
	store *RoomStore
}

func NewService(store *RoomStore) *Service {
	return &Service{store: store}
}

func (s *Service) CreateRoom(ctx context.Context, req models.CreateRoomRequest) (roomID, hostID string, err error) {
	roomID = utils.GenerateRoomID()
	hostID = utils.GenerateUserID()
	room := NewRoom(roomID, hostID)
	room.Users[hostID] = models.User{ID: hostID, Nickname: req.HostNickname}
	err = s.store.Create(ctx, room)
	return roomID, hostID, err
}

func (s *Service) GetRoom(ctx context.Context, id string) (*Room, error) {
	room, ok := s.store.Get(ctx, id)
	if !ok {
		return nil, errors.New("room not found")
	}
	return room, nil
}

func (s *Service) UpdateRoom(ctx context.Context, room *Room) error {
	return s.store.Update(ctx, room)
}
