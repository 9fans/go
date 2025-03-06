package srv9p

import "sync"

type incDecRef interface {
	incRef()
	decRef()
}

// A refMap is a map from K to V, with reference counting on the V's.
type refMap[K comparable, V incDecRef] struct {
	rw sync.RWMutex
	m  map[K]V
}

func newRefMap[K comparable, V incDecRef]() *refMap[K, V] {
	return &refMap[K, V]{
		m: make(map[K]V),
	}
}

// insert inserts the key, val pair into the pool.
// The caller's reference to val is taken over by the pool.
// If there was already a value with the given key in the pool
// replaced by the insert, insert returns that value.
// Otherwise insert returns the zero T.
func (m *refMap[K, V]) insert(key K, val V) (old V) {
	m.rw.Lock()
	defer m.rw.Unlock()

	old = m.m[key]
	m.m[key] = val
	return old
}

// tryInsert tries to insert the key, val pair into the pool.
// The caller's reference to val is taken over by the pool.
// On success, tryInsert returns true.
// If there is already a value with the given id in the pool,
// that value is left in place, and tryInsert returns false.
func (m *refMap[K, V]) tryInsert(key K, val V) bool {
	m.rw.Lock()
	defer m.rw.Unlock()

	if _, ok := m.m[key]; ok {
		return false
	}
	m.m[key] = val
	return true
}

// lookup returns the value with the given key, or else the zero T.
// If the value is found in the pool, lookup returns a new reference to it
// (calls incRef before returning the value).
func (m *refMap[K, V]) lookup(key K) V {
	m.rw.RLock()
	defer m.rw.RUnlock()

	v, ok := m.m[key]
	if ok {
		v.incRef()
	}
	return v
}

// delete removes the value with the given id
// from the pool and returns it.
// If there is no value with that id, remove returns nil.
func (m *refMap[K, V]) delete(key K) V {
	m.rw.Lock()
	defer m.rw.Unlock()

	v, ok := m.m[key]
	if !ok {
		var zero V
		return zero
	}
	delete(m.m, key)
	return v
}

// drop removes the value with the given id
// from the pool and decRefs it.
// If there is no value with that id, drop is a no-op.
func (m *refMap[K, V]) drop(key K) {
	m.rw.Lock()
	defer m.rw.Unlock()

	if v, ok := m.m[key]; ok {
		delete(m.m, key)
		v.decRef()
	}
}

// clear removes all values from the pool, decRef'ing them all.
func (m *refMap[K, V]) clear() {
	m.rw.Lock()
	all := m.m
	m.m = make(map[K]V)
	m.rw.Unlock()

	for _, v := range all {
		v.decRef()
	}
}
