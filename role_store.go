package main

import (
	"sync"
)

// RoleStore is a simple in-memory store mapping roles to namespaces and pods
// using those roles in the related namespace.
type RoleStore struct {
	Store map[string]map[string]map[string]struct{}
	sync.RWMutex
}

// NewRoleStore initializes a new RoleStore.
func NewRoleStore() *RoleStore {
	return &RoleStore{
		Store: make(map[string]map[string]map[string]struct{}),
	}
}

// Exists if the role is found for the specified namespace in the store.
func (s *RoleStore) Exists(role, namespace string) bool {
	s.RLock()
	defer s.RUnlock()

	if ns, ok := s.Store[role]; ok {
		if _, ok := ns[namespace]; ok {
			return true
		}
	}
	return false
}

// Add adds a role and related pod and namespace to the store.
func (s *RoleStore) Add(role, namespace, name string) {
	s.Lock()
	defer s.Unlock()

	if ns, ok := s.Store[role]; ok {
		if pods, ok := ns[namespace]; ok {
			pods[name] = struct{}{}
		} else {
			ns[namespace] = map[string]struct{}{
				name: struct{}{},
			}
		}
	} else {
		s.Store[role] = map[string]map[string]struct{}{
			namespace: map[string]struct{}{
				name: struct{}{},
			},
		}
	}
}

// Remove removes a role and related namespace and pod name mapping from the
// store.
func (s *RoleStore) Remove(role, namespace, name string) {
	s.Lock()
	defer s.Unlock()

	if ns, ok := s.Store[role]; ok {
		if pods, ok := ns[namespace]; ok {
			if _, ok := pods[name]; ok && len(pods) == 1 {
				delete(ns, namespace)
			}

			if len(ns) == 0 {
				delete(s.Store, role)
			}
		}
	}
}
