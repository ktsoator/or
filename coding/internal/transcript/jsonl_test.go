package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ktsoator/or/agent"
	"github.com/ktsoator/or/llm"
)

var errInjectedAppendFailure = errors.New("injected append failure")

type partialWriteFile struct {
	appendFile
}

func (f *partialWriteFile) Write(data []byte) (int, error) {
	limit := len(data) / 2
	if limit == 0 {
		limit = len(data)
	}
	written, err := f.appendFile.Write(data[:limit])
	if err != nil {
		return written, err
	}
	return written, errInjectedAppendFailure
}

type syncFailureFile struct {
	appendFile
}

type shortWriteFile struct {
	appendFile
}

func (f *shortWriteFile) Write(data []byte) (int, error) {
	return f.appendFile.Write(data[:len(data)/2])
}

func (f *syncFailureFile) Sync() error {
	return errInjectedAppendFailure
}

type closeFailureFile struct {
	appendFile
}

func (f *closeFailureFile) Close() error {
	if err := f.appendFile.Close(); err != nil {
		return err
	}
	return errInjectedAppendFailure
}

func TestJSONLLoadRejectsLegacyMessagesWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	legacy := []llm.Message{
		&llm.UserMessage{Content: []llm.UserContent{&llm.TextContent{Text: "hello"}}},
		&llm.AssistantMessage{Content: []llm.AssistantContent{&llm.TextContent{Text: "world"}}, StopReason: llm.StopReasonStop},
	}
	var data bytes.Buffer
	for _, message := range legacy {
		line, err := llm.MarshalMessage(message)
		if err != nil {
			t.Fatal(err)
		}
		data.Write(line)
		data.WriteByte('\n')
	}
	if err := os.WriteFile(path, data.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewJSONL(path)
	if _, err := store.Load(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid session header") {
		t.Fatalf("Load() error = %v, want invalid session header", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data.Bytes()) {
		t.Fatal("legacy session was rewritten")
	}
}

func TestJSONLRejectsUnsupportedVersions(t *testing.T) {
	for _, version := range []int{2, 3, 4, 5, 6, 8} {
		t.Run(fmt.Sprintf("version_%d", version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			data := []byte(fmt.Sprintf("{\"type\":\"session\",\"version\":%d}\n", version))
			if err := os.WriteFile(path, data, 0o644); err != nil {
				t.Fatal(err)
			}

			store := NewJSONL(path)
			if _, err := store.Load(context.Background()); err == nil ||
				!strings.Contains(err.Error(), fmt.Sprintf("unsupported session version %d", version)) {
				t.Fatalf("Load() error = %v, want unsupported version %d", err, version)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("rejected version %d was rewritten:\n%s", version, got)
			}
		})
	}
}

func TestJSONLAppendRejectsV6WithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	data := []byte(`{"type":"session","version":6}` + "\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	entry := sequencedForTest(NewMessage(agent.UserMessage("current")))[0]
	if err := NewJSONL(path).Append(context.Background(), entry); err == nil ||
		!strings.Contains(err.Error(), "unsupported session version 6") {
		t.Fatalf("Append() error = %v, want unsupported version 6", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("rejected v6 append rewrote the transcript:\n%s", got)
	}
}

func TestJSONLLoadRejectsMissingOrDiscontinuousSequence(t *testing.T) {
	entry := sequencedForTest(NewMessage(agent.UserMessage("hello")))[0]
	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	header, err := json.Marshal(NewHeader())
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		line []byte
		want string
	}{
		{
			name: "missing",
			line: bytes.Replace(encoded, []byte(`"seq":0,`), nil, 1),
			want: "sequence is missing",
		},
		{
			name: "discontinuous",
			line: bytes.Replace(encoded, []byte(`"seq":0`), []byte(`"seq":1`), 1),
			want: "sequence 1, want 0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			data := append(append(append([]byte(nil), header...), '\n'), test.line...)
			data = append(data, '\n')
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewJSONL(path).Load(context.Background()); err == nil ||
				!strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestJSONLAppendRejectsSequenceGapWithoutChangingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := NewJSONL(path)
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))
	if err := store.Append(context.Background(), first...); err != nil {
		t.Fatal(err)
	}
	gap := sequencedForTest(NewMessage(agent.UserMessage("gap")))
	gap[0].Seq = 2
	if err := store.Append(context.Background(), gap...); err == nil ||
		!strings.Contains(err.Error(), "sequence 2, want 1") {
		t.Fatalf("Append() error = %v, want sequence mismatch", err)
	}
	loaded, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Seq != 0 {
		t.Fatalf("entries after rejected append = %#v", loaded)
	}
}

func TestJSONLAppendRollsBackFailedBatch(t *testing.T) {
	tests := []struct {
		name    string
		wrap    func(appendFile) appendFile
		want    string
		wantErr error
	}{
		{
			name:    "partial write error",
			wrap:    func(file appendFile) appendFile { return &partialWriteFile{appendFile: file} },
			want:    "append",
			wantErr: errInjectedAppendFailure,
		},
		{
			name:    "short write",
			wrap:    func(file appendFile) appendFile { return &shortWriteFile{appendFile: file} },
			want:    "append",
			wantErr: io.ErrShortWrite,
		},
		{
			name:    "sync",
			wrap:    func(file appendFile) appendFile { return &syncFailureFile{appendFile: file} },
			want:    "sync",
			wantErr: errInjectedAppendFailure,
		},
		{
			name:    "close",
			wrap:    func(file appendFile) appendFile { return &closeFailureFile{appendFile: file} },
			want:    "close",
			wantErr: errInjectedAppendFailure,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			store := NewJSONL(path)
			first := sequencedForTest(NewMessage(agent.UserMessage("committed")))[0]
			if err := store.Append(context.Background(), first); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			next := NewMessage(agent.UserMessage("retry me"))
			next.Seq = 1
			store.openFile = func(path string, flag int, mode os.FileMode) (appendFile, error) {
				file, err := openAppendFile(path, flag, mode)
				if err != nil {
					return nil, err
				}
				return test.wrap(file), nil
			}
			if err := store.Append(context.Background(), next); err == nil ||
				!strings.Contains(err.Error(), test.want) ||
				!errors.Is(err, test.wantErr) {
				t.Fatalf("Append() error = %v, want injected %s failure", err, test.want)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("failed append changed committed bytes:\n%s", after)
			}

			store.openFile = openAppendFile
			if err := store.Append(context.Background(), next); err != nil {
				t.Fatalf("retry Append() error = %v", err)
			}
			loaded, err := NewJSONL(path).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(loaded) != 2 || loaded[0].ID != first.ID || loaded[1].ID != next.ID {
				t.Fatalf("entries after retry = %#v", loaded)
			}
		})
	}
}

func TestJSONLLoadRecoversUnterminatedTailAtRecordBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := NewJSONL(path)
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))[0]
	if err := store.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	second := NewMessage(agent.UserMessage("second"))
	second.Seq = 1
	third := NewMessage(agent.UserMessage("third"))
	third.Seq = 2
	secondLine, err := encodeEntries([]Entry{second})
	if err != nil {
		t.Fatal(err)
	}
	thirdLine, err := encodeEntries([]Entry{third})
	if err != nil {
		t.Fatal(err)
	}
	torn := thirdLine[:len(thirdLine)/2]
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, privateFileMode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(append(append([]byte(nil), secondLine...), torn...)); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewJSONL(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].ID != first.ID || loaded[1].ID != second.ID {
		t.Fatalf("recovered entries = %#v", loaded)
	}
	wantBytes := append(append([]byte(nil), committed...), secondLine...)
	gotBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("recovered transcript bytes:\n%s\nwant:\n%s", gotBytes, wantBytes)
	}
	if err := NewJSONL(path).Append(context.Background(), third); err != nil {
		t.Fatalf("append after recovery: %v", err)
	}
}

func TestJSONLLoadDropsCompleteButUnterminatedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := NewJSONL(path)
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))[0]
	if err := store.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	second := NewMessage(agent.UserMessage("not committed"))
	second.Seq = 1
	line, err := encodeEntries([]Entry{second})
	if err != nil {
		t.Fatal(err)
	}
	line = bytes.TrimSuffix(line, []byte{'\n'})
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, privateFileMode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(line); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewJSONL(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != first.ID {
		t.Fatalf("loaded entries = %#v", loaded)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("unterminated entry was treated as committed:\n%s", after)
	}
}

func TestJSONLLoadRejectsTerminatedCorruptionWithoutRewriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := NewJSONL(path)
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))[0]
	if err := store.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, privateFileMode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("{\"seq\":1\n")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewJSONL(path).Load(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "decode session line") {
		t.Fatalf("Load() error = %v, want committed corruption", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("committed corruption was rewritten:\n%s", after)
	}
}

func TestJSONLLoadDoesNotTruncateUncommittedHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	data := []byte(`{"type":"session","version":7}`)
	if err := os.WriteFile(path, data, privateFileMode); err != nil {
		t.Fatal(err)
	}
	if _, err := NewJSONL(path).Load(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "no committed header") {
		t.Fatalf("Load() error = %v, want missing committed header", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, data) {
		t.Fatalf("uncommitted header was truncated:\n%s", after)
	}
}

func TestJSONLAppendRejectsEntryIDFromEarlierBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	first := sequencedForTest(NewMessage(agent.UserMessage("first")))[0]
	store := NewJSONL(path)
	if err := store.Append(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	duplicate := NewMessage(agent.UserMessage("duplicate"))
	duplicate.ID = first.ID
	duplicate.Seq = 1
	tests := []struct {
		name  string
		store *JSONL
	}{
		{name: "loaded instance", store: store},
		{name: "reopened instance", store: NewJSONL(path)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.store.Append(context.Background(), duplicate); err == nil ||
				!strings.Contains(err.Error(), "repeats existing entry id") {
				t.Fatalf("Append() error = %v, want existing entry id rejection", err)
			}
		})
	}

	loaded, err := NewJSONL(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != first.ID {
		t.Fatalf("entries after rejected appends = %#v", loaded)
	}
}

func TestJSONLRoundTripsLifecycleTiming(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	startedAt := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(2 * time.Second)
	runStart := NewRunStart("run-1")
	runEnd := NewRunEnd("run-1", LifecycleCompleted, "")
	runStart.Timestamp = startedAt
	runEnd.Timestamp = completedAt
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(runStart, runEnd)...); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"version":7`)) {
		t.Fatalf("session header is not v7:\n%s", data)
	}
	if !bytes.Contains(data, []byte(`"seq":0`)) || !bytes.Contains(data, []byte(`"seq":1`)) {
		t.Fatalf("session entries have no durable sequence:\n%s", data)
	}
	if bytes.Contains(data, []byte(`"parentId"`)) {
		t.Fatalf("linear session contains parentId:\n%s", data)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Type != RunStartEntry || entries[1].Type != RunEndEntry {
		t.Fatalf("entries = %#v", entries)
	}
	if !entries[0].Timestamp.Equal(startedAt) || !entries[1].Timestamp.Equal(completedAt) {
		t.Fatalf("run timing = %v..%v", entries[0].Timestamp, entries[1].Timestamp)
	}
}

func TestJSONLRoundTripsLifecycleBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	entry := NewStepEnd(
		"run-1",
		"turn-1",
		"step-1",
		LifecycleFailed,
		"provider_request_failed",
	)
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(entry)...); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != StepEndEntry || entries[0].Lifecycle == nil {
		t.Fatalf("entries = %#v", entries)
	}
	got := entries[0].Lifecycle
	if got.RunID != "run-1" || got.TurnID != "turn-1" || got.StepID != "step-1" ||
		got.Status != LifecycleFailed || got.Reason != "provider_request_failed" {
		t.Fatalf("lifecycle = %#v", got)
	}
}

func TestJSONLRoundTripsToolCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	entry := NewToolCall(ToolCall{
		ToolCallID: "call-17",
		ToolName:   "write_file",
		Arguments:  json.RawMessage(`{"path":"notes.txt","text":"hello"}`),
	})
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(entry)...); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != ToolCallEntry || entries[0].ToolCall == nil {
		t.Fatalf("entries = %#v", entries)
	}
	got := entries[0].ToolCall
	if got.ToolCallID != "call-17" || got.ToolName != "write_file" ||
		!bytes.Equal(got.Arguments, entry.ToolCall.Arguments) {
		t.Fatalf("tool call = %#v", got)
	}
}

func TestToolCallEntryValidatesRequiredFieldsAndArguments(t *testing.T) {
	tests := []struct {
		name string
		call ToolCall
	}{
		{name: "tool call id", call: ToolCall{ToolName: "read", Arguments: json.RawMessage(`{}`)}},
		{name: "tool name", call: ToolCall{ToolCallID: "call-1", Arguments: json.RawMessage(`{}`)}},
		{name: "arguments", call: ToolCall{ToolCallID: "call-1", ToolName: "read"}},
		{
			name: "invalid arguments",
			call: ToolCall{
				ToolCallID: "call-1", ToolName: "read", Arguments: json.RawMessage(`{"unterminated"`),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := NewToolCall(test.call).Validate(); err == nil {
				t.Fatal("Validate() succeeded for an invalid tool call")
			}
		})
	}
}

func TestJSONLReplaceAtomicallyRewritesCompleteLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := NewJSONL(path)
	first := NewMessage(agent.UserMessage("first"))
	discarded := NewMessage(agent.UserMessage("discarded"))
	initial := sequencedForTest(first, discarded)
	first, discarded = initial[0], initial[1]
	if err := store.Append(context.Background(), initial...); err != nil {
		t.Fatal(err)
	}
	replacement := NewMessage(agent.UserMessage("replacement"))
	rewritten := sequencedForTest(first, replacement)
	if err := store.Replace(context.Background(), rewritten); err != nil {
		t.Fatal(err)
	}

	loaded, err := NewJSONL(path).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].ID != first.ID || loaded[1].ID != replacement.ID {
		t.Fatalf("replaced entries = %#v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(discarded.ID)) || bytes.Contains(data, []byte("discarded")) {
		t.Fatalf("discarded entry remains in replacement:\n%s", data)
	}
	assertPrivateStoragePermissions(t, filepath.Dir(path), path)
}

func TestJSONLRoundTripsToolOutcome(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	exitCode := 17
	entry := NewToolOutcome(ToolOutcome{
		ToolCallID: "call-17",
		Status:     agent.ToolOutcomeFailed,
		ErrorCode:  "command_exit_nonzero",
		ExitCode:   &exitCode,
		DataKind:   "generic",
		Data:       json.RawMessage(`{"command":"go test ./..."}`),
	})
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(entry)...); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != ToolOutcomeEntry || entries[0].ToolOutcome == nil {
		t.Fatalf("entries = %#v", entries)
	}
	got := entries[0].ToolOutcome
	if got.ToolCallID != "call-17" || got.Status != agent.ToolOutcomeFailed ||
		got.ErrorCode != "command_exit_nonzero" || got.ExitCode == nil || *got.ExitCode != 17 ||
		got.DataKind != "generic" || !bytes.Equal(got.Data, entry.ToolOutcome.Data) {
		t.Fatalf("tool outcome = %#v", got)
	}
}

func TestToolOutcomeEntryValidatesRequiredFieldsAndData(t *testing.T) {
	tests := []struct {
		name    string
		outcome ToolOutcome
	}{
		{name: "tool call id", outcome: ToolOutcome{Status: agent.ToolOutcomeSuccess}},
		{name: "status", outcome: ToolOutcome{ToolCallID: "call-1"}},
		{
			name: "data",
			outcome: ToolOutcome{
				ToolCallID: "call-1",
				Status:     agent.ToolOutcomeSuccess,
				DataKind:   "generic",
				Data:       json.RawMessage(`{"unterminated"`),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := NewToolOutcome(test.outcome).Validate(); err == nil {
				t.Fatal("Validate() succeeded for an invalid tool outcome")
			}
		})
	}
}

func TestJSONLUsesPrivatePermissionsAndSecuresExistingStorage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sessions")
	path := filepath.Join(dir, "session.jsonl")
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(NewMessage(agent.UserMessage("secret")))...); err != nil {
		t.Fatal(err)
	}
	assertPrivateStoragePermissions(t, dir, path)

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewJSONL(path).Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPrivateStoragePermissions(t, dir, path)
}

func assertPrivateStoragePermissions(t *testing.T, dir, path string) {
	t.Helper()
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != privateDirectoryMode {
		t.Fatalf("directory permissions = %04o, want %04o", got, privateDirectoryMode)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fileInfo.Mode().Perm(); got != privateFileMode {
		t.Fatalf("file permissions = %04o, want %04o", got, privateFileMode)
	}
}

func TestJSONLRoundTripsContextAttachment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	entry := NewContext(ContextAttachment{
		AttachmentID: "session:2:abc",
		Epoch:        2,
		Kind:         "session",
		Placement:    "prefix",
		Revision:     "abc",
		Rendered:     `<or-context kind="session">rules</or-context>`,
	})
	store := NewJSONL(path)
	if err := store.Append(context.Background(), sequencedForTest(entry)...); err != nil {
		t.Fatal(err)
	}

	entries, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Type != ContextEntry || entries[0].Context == nil {
		t.Fatalf("entries = %#v", entries)
	}
	got := entries[0].Context
	if got.AttachmentID != entry.Context.AttachmentID ||
		got.Epoch != 2 ||
		got.Kind != "session" ||
		got.Placement != "prefix" ||
		got.Revision != "abc" ||
		got.Rendered != entry.Context.Rendered {
		t.Fatalf("context attachment = %#v", got)
	}
}
