package room

import (
	"context"
	"sync"
)

type RoomStore struct {
	mu    sync.RWMutex
	rooms map[string]*Room
}

func NewRoomStore() *RoomStore {
	return &RoomStore{
		rooms: make(map[string]*Room),
	}
}

func (s *RoomStore) Create(ctx context.Context, room *Room) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rooms[room.ID] = room
	return nil
}

func (s *RoomStore) Get(ctx context.Context, id string) (*Room, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.rooms[id]
	return r, ok
}

func (s *RoomStore) Update(ctx context.Context, room *Room) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rooms[room.ID] = room
	return nil
}
