package agent

// Subscribe registers a listener for run events and returns a function that
// removes it. Listeners are called synchronously from the goroutine running
// Prompt, in event order.
func (a *Agent) Subscribe(listener func(AgentEvent)) (unsubscribe func()) {
	a.mu.Lock()
	defer a.mu.Unlock()

	id := a.nextListenerID
	a.nextListenerID++
	a.listeners[id] = listener

	return func() {
		a.mu.Lock()
		delete(a.listeners, id)
		a.mu.Unlock()
	}
}

// dispatch snapshots the listeners under the lock and calls them outside it, so
// a listener may call back into the agent without deadlocking.
func (a *Agent) dispatch(event AgentEvent) {
	a.mu.Lock()
	listeners := make([]func(AgentEvent), 0, len(a.listeners))
	for _, listener := range a.listeners {
		listeners = append(listeners, listener)
	}
	a.mu.Unlock()

	for _, listener := range listeners {
		listener(event)
	}
}
