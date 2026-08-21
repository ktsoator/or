package engine

import "github.com/ktsoator/or/coding/internal/snapshot"

// RequestSnapshot reconstructs one inspectable provider exchange from the
// committed transcript. Live streaming state is deliberately excluded.
func (s *Session) RequestSnapshot(providerRequestID string) (snapshot.Snapshot, error) {
	entries, _ := s.journal.entriesSnapshot()
	return snapshot.FromTranscript(s.sessionID, providerRequestID, entries)
}
