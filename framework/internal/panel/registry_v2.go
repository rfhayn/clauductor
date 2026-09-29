package panel

import "errors"

// errNoRecord means Update found no record for the lane (it was stopped or
// forgotten meanwhile); the caller must not recreate it.
var errNoRecord = errors.New("no registered lane")

// Update changes one record under the registry's lock and writes the file. fn edits
// the record in place and returns false to leave it unchanged. Unlike Get then Put,
// it cannot resurrect a lane that was deleted between the two, or overwrite a field
// another action changed meanwhile.
func (r *Registry) Update(id string, fn func(*LaneRecord) bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.lanes[id]
	if !ok {
		return errNoRecord
	}
	prev := l
	if !fn(&l) {
		return nil
	}
	r.lanes[id] = l
	if err := r.writeLocked(); err != nil {
		r.lanes[id] = prev
		return err
	}
	return nil
}
