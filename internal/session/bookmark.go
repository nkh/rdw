package session

import (
	"fmt"
	"sync"
	"time"
)

// Bookmark marks a line in a scrollback buffer by its insertion index.
type Bookmark struct {
	Name      string    `json:"name"`
	LineIndex int       `json:"line_index"`
	CreatedAt time.Time `json:"created_at"`
}

// BookmarkStore is a thread-safe named-bookmark set for one pane.
type BookmarkStore struct {
	mu    sync.RWMutex
	marks map[string]Bookmark
}

// NewBookmarkStore returns an empty BookmarkStore.
func NewBookmarkStore() *BookmarkStore {
	return &BookmarkStore{marks: make(map[string]Bookmark)}
}

// Add creates or replaces a bookmark by name.
func (s *BookmarkStore) Add(name string, lineIndex int) error {
	if name == "" {
		return fmt.Errorf("bookmark name must not be empty")
	}

	if lineIndex < 0 {
		return fmt.Errorf("line_index must be >= 0")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.marks[name] = Bookmark{Name: name, LineIndex: lineIndex, CreatedAt: time.Now().UTC()}

	return nil
}

// Remove deletes a bookmark. Returns an error if it does not exist.
func (s *BookmarkStore) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.marks[name]; !ok {
		return fmt.Errorf("bookmark %q not found", name)
	}

	delete(s.marks, name)

	return nil
}

// Get returns the bookmark by name.
func (s *BookmarkStore) Get(name string) (Bookmark, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.marks[name]

	return b, ok
}

// All returns a copy of every bookmark, sorted by LineIndex.
func (s *BookmarkStore) All() []Bookmark {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Bookmark, 0, len(s.marks))
	for _, b := range s.marks {
		out = append(out, b)
	}

	// Insertion-order sort by line index.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].LineIndex < out[j-1].LineIndex; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}

	return out
}

// Len returns the number of bookmarks.
func (s *BookmarkStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.marks)
}
