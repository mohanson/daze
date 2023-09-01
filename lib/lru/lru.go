// Package lru implements an LRU cache.
package lru

import (
	"sync"
)

// Elem is an element of a linked list.
type Elem[K comparable, V any] struct {
	Next, Prev *Elem[K, V]
	K          K
	V          V
}

// List represents a doubly linked list.
type List[K comparable, V any] struct {
	Root Elem[K, V]
	Size int
}

// Init initializes or clears list l.
func (l *List[K, V]) Init() *List[K, V] {
	l.Root.Next = &l.Root
	l.Root.Prev = &l.Root
	l.Size = 0
	return l
}

// Insert inserts e after at, increments l.len, and returns e.
func (l *List[K, V]) Insert(e, at *Elem[K, V]) *Elem[K, V] {
	e.Prev = at
	e.Next = at.Next
	e.Prev.Next = e
	e.Next.Prev = e
	l.Size++
	return e
}

// Move moves e to next to at.
func (l *List[K, V]) Move(e, at *Elem[K, V]) {
	if e == at || e == at.Next {
		return
	}
	e.Prev.Next = e.Next
	e.Next.Prev = e.Prev
	e.Prev = at
	e.Next = at.Next
	e.Prev.Next = e
	e.Next.Prev = e
}

// Remove removes e from its list, decrements l.len
func (l *List[K, V]) Remove(e *Elem[K, V]) {
	e.Prev.Next = e.Next
	e.Next.Prev = e.Prev
	e.Prev = nil // avoid memory leaks
	e.Next = nil // avoid memory leaks
	l.Size--
}

// Lru cache. It is safe for concurrent access.
type Lru[K comparable, V any] struct {
	// Size is the maximum number of cache entries before
	// an item is evicted. Zero means no limit.
	Size int
	List *List[K, V]
	C    map[K]*Elem[K, V]
	M    *sync.Mutex
}

// Set adds a value to the cache.
func (l *Lru[K, V]) Set(k K, v V) {
	l.M.Lock()
	defer l.M.Unlock()
	if e, ok := l.C[k]; ok {
		l.List.Move(e, &l.List.Root)
		e.K = k
		e.V = v
		return
	}
	l.C[k] = l.List.Insert(&Elem[K, V]{K: k, V: v}, &l.List.Root)
	if l.List.Size > l.Size {
		delete(l.C, l.List.Root.Prev.K)
		l.List.Remove(l.List.Root.Prev)
	}
}

// Get looks up a key's value from the cache.
func (l *Lru[K, V]) GetExists(k K) (v V, ok bool) {
	l.M.Lock()
	defer l.M.Unlock()
	var e *Elem[K, V]
	e, ok = l.C[k]
	if ok {
		l.List.Move(e, &l.List.Root)
		v = e.V
	}
	return
}

// Get looks up a key's value from the cache.
func (l *Lru[K, V]) Get(k K) (v V) {
	v, _ = l.GetExists(k)
	return
}

// Del removes the provided key from the cache.
func (l *Lru[K, V]) Del(k K) {
	l.M.Lock()
	defer l.M.Unlock()
	if e, ok := l.C[k]; ok {
		l.List.Remove(e)
		delete(l.C, k)
	}
}

// Len returns the number of items in the cache.
func (l *Lru[K, V]) Len() int {
	l.M.Lock()
	defer l.M.Unlock()
	return l.List.Size
}

// New returns a new LRU cache. If size is zero, the cache has no limit.
func New[K comparable, V any](size int) *Lru[K, V] {
	return &Lru[K, V]{
		Size: size,
		List: new(List[K, V]).Init(),
		C:    map[K]*Elem[K, V]{},
		M:    &sync.Mutex{},
	}
}
