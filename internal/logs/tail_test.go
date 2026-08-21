package logs_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/querysheriff/collector/internal/logs"
)

// How long to wait for the tailer to open a freshly appeared file before writing to it.
const tailSettle = 300 * time.Millisecond

const pgLogFilename = "postgresql-%Y-%m-%d_%H%M%S.log"

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func appendToFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove file: %v", err)
	}
}

func setModTime(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func startTail(t *testing.T, location string) <-chan []byte {
	t.Helper()

	return startTailWith(t, location, false, false)
}

func startTailWith(t *testing.T, location string, deleteRotated, jsonOnly bool) <-chan []byte {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	stream, err := logs.NewTailer(discardLogger(), pgLogFilename, deleteRotated, jsonOnly).Tail(ctx, location)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}

	return stream
}

func wantLine(t *testing.T, stream <-chan []byte, want string) {
	t.Helper()
	select {
	case got, ok := <-stream:
		if !ok {
			t.Fatalf("stream closed while waiting for %q", want)
		}
		if string(got) != want {
			t.Fatalf("got line %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for line %q", want)
	}
}

func wantNoLine(t *testing.T, stream <-chan []byte) {
	t.Helper()
	select {
	case got, ok := <-stream:
		if !ok {
			t.Fatalf("stream closed unexpectedly")
		}
		t.Fatalf("unexpected line %q", got)
	case <-time.After(time.Second):
	}
}

func TestTailExistingEmptyFileReceivesWrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "postgresql.json"), "")

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	appendToFile(t, filepath.Join(dir, "postgresql.json"), "first line\n")
	wantLine(t, stream, "first line")
}

func TestTailReopensAfterDeleteAndRecreate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "postgresql.json")
	writeFile(t, file, "")

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	appendToFile(t, file, "before delete\n")
	wantLine(t, stream, "before delete")

	removeFile(t, file)
	time.Sleep(tailSettle) // let the current follower notice the file is gone

	writeFile(t, file, "") // created again
	time.Sleep(tailSettle)

	appendToFile(t, file, "after recreate\n")
	wantLine(t, stream, "after recreate")
}

func TestTailDirectoryPicksUpCreatedFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir() // empty at startup

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	file := filepath.Join(dir, "postgresql.json")
	writeFile(t, file, "") // created empty so the tailer opens it at end before any line arrives
	time.Sleep(tailSettle)

	appendToFile(t, file, "born line\n")
	wantLine(t, stream, "born line")
}

func TestTailDirectoryExistingFileSkipsHistory(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "postgresql.json")
	writeFile(t, file, "old line\n")

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	appendToFile(t, file, "new line\n")
	wantLine(t, stream, "new line")
}

func TestTailDirectorySwitchesToWrittenFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	b := filepath.Join(dir, "b.json")
	writeFile(t, a, "")
	writeFile(t, b, "")

	now := time.Now().UTC()
	setModTime(t, a, now.Add(-2*time.Minute)) // older
	setModTime(t, b, now.Add(-1*time.Minute)) // newer, so it is followed at startup

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	// Writing to a.json makes it the newest file; the tailer switches to it, seeking to end, so this line is skipped.
	appendToFile(t, a, "trigger switch\n")
	time.Sleep(tailSettle)

	appendToFile(t, a, "after switch\n")
	wantLine(t, stream, "after switch")
}

func TestTailDirectoryFollowsNewestAsFilesAppear(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.json")
	writeFile(t, a, "")

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	appendToFile(t, a, "from a\n")
	wantLine(t, stream, "from a")

	b := filepath.Join(dir, "b.json")
	writeFile(t, b, "") // newer than a, so the tailer switches to it
	time.Sleep(tailSettle)
	appendToFile(t, b, "from b\n")
	wantLine(t, stream, "from b")

	c := filepath.Join(dir, "c.json")
	writeFile(t, c, "") // newer than b, so the tailer switches to it
	time.Sleep(tailSettle)
	appendToFile(t, c, "from c\n")
	wantLine(t, stream, "from c")
}

func TestTailDirectoryIgnoresBadExtensions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stderrLog := filepath.Join(dir, "postgresql.log")
	csv := filepath.Join(dir, "postgresql.csv")
	writeFile(t, stderrLog, "")
	writeFile(t, csv, "")

	stream := startTail(t, dir)
	time.Sleep(tailSettle)

	appendToFile(t, stderrLog, "should be ignored\n")
	appendToFile(t, csv, "should be ignored\n")
	wantNoLine(t, stream)
}

func TestTailDirectoryDeletesRotatedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "postgresql-2026-07-24_100000.json")
	b := filepath.Join(dir, "postgresql-2026-07-24_110000.json")
	writeFile(t, a, "")
	writeFile(t, b, "")

	now := time.Now().UTC()
	setModTime(t, a, now.Add(-3*time.Minute)) // oldest
	setModTime(t, b, now.Add(-2*time.Minute)) // newer, followed at startup

	stream := startTailWith(t, dir, true, false)
	time.Sleep(tailSettle)

	c := filepath.Join(dir, "postgresql-2026-07-24_120000.json")
	writeFile(t, c, "")
	time.Sleep(tailSettle)

	appendToFile(t, c, "after rotation\n")
	wantLine(t, stream, "after rotation")

	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Errorf("%s should have been deleted, stat err = %v", filepath.Base(a), err)
	}
	if _, err := os.Stat(b); !os.IsNotExist(err) {
		t.Errorf("%s should have been deleted, stat err = %v", filepath.Base(b), err)
	}
	if _, err := os.Stat(c); err != nil {
		t.Errorf("%s (current) should still exist, stat err = %v", filepath.Base(c), err)
	}
}

func TestTailDirectoryDeletesRotatedLogFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	oldJSON := filepath.Join(dir, "postgresql-2026-07-24_110000.json")
	oldLog := filepath.Join(dir, "postgresql-2026-07-24_100000.log")
	writeFile(t, oldJSON, "")
	writeFile(t, oldLog, "")

	now := time.Now().UTC()
	setModTime(t, oldJSON, now.Add(-2*time.Minute)) // followed at startup
	setModTime(t, oldLog, now.Add(-3*time.Minute))  // oldest

	stream := startTailWith(t, dir, true, false)
	time.Sleep(tailSettle)

	b := filepath.Join(dir, "postgresql-2026-07-24_120000.json")
	writeFile(t, b, "")
	time.Sleep(tailSettle)

	appendToFile(t, b, "after rotation\n")
	wantLine(t, stream, "after rotation")

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Errorf("%s should have been deleted, stat err = %v", filepath.Base(oldLog), err)
	}
	if _, err := os.Stat(b); err != nil {
		t.Errorf("%s (current) should still exist, stat err = %v", filepath.Base(b), err)
	}
}

func TestTailDirectoryKeepsForeignLogFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pgOld := filepath.Join(dir, "postgresql-2026-07-24_110000.json")
	repmgr := filepath.Join(dir, "repmgr.log")
	backup := filepath.Join(dir, "backup.json")
	writeFile(t, pgOld, "")
	writeFile(t, repmgr, "")
	writeFile(t, backup, "")

	now := time.Now().UTC()
	setModTime(t, pgOld, now.Add(-2*time.Minute))  // followed at startup
	setModTime(t, repmgr, now.Add(-3*time.Minute)) // older, but not a Postgres log
	setModTime(t, backup, now.Add(-3*time.Minute)) // older, but not a Postgres log

	stream := startTailWith(t, dir, true, false)
	time.Sleep(tailSettle)

	c := filepath.Join(dir, "postgresql-2026-07-24_120000.json")
	writeFile(t, c, "")
	time.Sleep(tailSettle)

	appendToFile(t, c, "after rotation\n")
	wantLine(t, stream, "after rotation")

	if _, err := os.Stat(pgOld); !os.IsNotExist(err) {
		t.Errorf("%s should have been deleted, stat err = %v", filepath.Base(pgOld), err)
	}
	if _, err := os.Stat(repmgr); err != nil {
		t.Errorf("%s should have been kept, stat err = %v", filepath.Base(repmgr), err)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("%s should have been kept, stat err = %v", filepath.Base(backup), err)
	}
}

func TestTailDirectoryDeletesRotatedJSONOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	oldJSON := filepath.Join(dir, "postgresql-2026-07-24_100000.json")
	oldLog := filepath.Join(dir, "postgresql-2026-07-24_100000.log")
	writeFile(t, oldJSON, "")
	writeFile(t, oldLog, "")

	now := time.Now().UTC()
	setModTime(t, oldJSON, now.Add(-3*time.Minute))
	setModTime(t, oldLog, now.Add(-3*time.Minute))

	writeFile(t, filepath.Join(dir, "postgresql-2026-07-24_110000.json"), "")
	setModTime(t, filepath.Join(dir, "postgresql-2026-07-24_110000.json"), now.Add(-2*time.Minute))

	stream := startTailWith(t, dir, true, true)
	time.Sleep(tailSettle)

	c := filepath.Join(dir, "postgresql-2026-07-24_120000.json")
	writeFile(t, c, "")
	time.Sleep(tailSettle)

	appendToFile(t, c, "after rotation\n")
	wantLine(t, stream, "after rotation")

	if _, err := os.Stat(oldJSON); !os.IsNotExist(err) {
		t.Errorf("%s should have been deleted, stat err = %v", filepath.Base(oldJSON), err)
	}
	if _, err := os.Stat(oldLog); err != nil {
		t.Errorf("%s should have been kept, stat err = %v", filepath.Base(oldLog), err)
	}
}
